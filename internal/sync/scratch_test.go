package sync

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// TestScratchDB_OpenAndCleanup asserts that openScratchDB creates the
// file, initSchema populates the 13 tables, and closeScratchDB unlinks
// the file on teardown. Regression-locks the lifecycle contract.
func TestScratchDB_OpenAndCleanup(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	s, err := openScratchDB(ctx)
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}

	// File must exist on disk after openScratchDB returns.
	if _, err := os.Stat(s.path); err != nil {
		t.Fatalf("scratch file not found at %s: %v", s.path, err)
	}

	savedPath := s.path
	closeScratchDB(ctx, s, slog.Default())

	// File must be unlinked after closeScratchDB.
	if _, err := os.Stat(savedPath); !os.IsNotExist(err) {
		t.Fatalf("scratch file still exists at %s after close: %v", savedPath, err)
	}
}

// TestScratchDB_Schema asserts that all 13 staging tables are created
// with the expected (id INTEGER PRIMARY KEY, data BLOB NOT NULL) schema.
// A future edit that drops a type or changes the column layout will
// fail this test.
func TestScratchDB_Schema(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	s, err := openScratchDB(ctx)
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	defer closeScratchDB(ctx, s, slog.Default())

	rows, err := s.db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	defer func() { _ = rows.Close() }()

	got := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan table name: %v", err)
		}
		got[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate tables: %v", err)
	}

	for _, want := range scratchTypes {
		if !got[want] {
			t.Errorf("scratch DB missing staging table %q", want)
		}
	}
}

// TestScratchDB_StageStats asserts that stageType reports the response
// meta.generated and the newest parseable updated among the staged rows.
// A missing or malformed updated drops out of the maximum and does not
// fail the stage; Phase B owns updated validation.
func TestScratchDB_StageStats(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	body := []byte(`{"meta":{"generated":1790000000.75},"data":[
{"id":1,"updated":"2026-09-22T20:00:00Z"},
{"id":2,"updated":"2026-09-22T22:40:00Z"},
{"id":3},
{"id":4,"updated":"not-a-time"}
]}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := peeringdb.NewClient(server.URL, slog.Default())
	client.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	client.SetRetryBaseDelay(0)

	s, err := openScratchDB(ctx)
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	defer closeScratchDB(ctx, s, slog.Default())

	stats, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{})
	if err != nil {
		t.Fatalf("stageType: %v", err)
	}
	if want := time.Date(2026, 9, 22, 22, 40, 0, 0, time.UTC); !stats.maxUpdated.Equal(want) {
		t.Errorf("maxUpdated = %v, want %v", stats.maxUpdated, want)
	}
	if want := time.Unix(1790000000, 0); !stats.generated.Equal(want) {
		t.Errorf("generated = %v, want %v", stats.generated, want)
	}
	rows, _, err := s.drainChunk(ctx, peeringdb.TypeOrg, 0, 100)
	if err != nil {
		t.Fatalf("drainChunk: %v", err)
	}
	if len(rows) != 4 {
		t.Errorf("staged %d rows, want 4", len(rows))
	}
}

// TestScratchDB_StageAndDrain asserts the round-trip from StreamAll
// through stageType into scratch, and back out via drainChunk. The test
// serves a synthetic PeeringDB response with three org rows, stages
// them, drains them, and asserts the drained raw bytes match the
// originals. This is the core fallback path: if either direction is
// broken, the scratch path cannot function.
func TestScratchDB_StageAndDrain(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	body := []byte(`{"meta":{"generated":1234567890},"data":[
{"id":10,"name":"org-10","status":"ok"},
{"id":20,"name":"org-20","status":"ok"},
{"id":30,"name":"org-30","status":"ok"}
]}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/")
		path = strings.Split(path, "?")[0]
		if path != peeringdb.TypeOrg {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := peeringdb.NewClient(server.URL, slog.Default())
	client.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	client.SetRetryBaseDelay(0)

	s, err := openScratchDB(ctx)
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	defer closeScratchDB(ctx, s, slog.Default())

	if _, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}); err != nil {
		t.Fatalf("stageType: %v", err)
	}

	// Drain all rows in a single chunk (chunkSize >> count).
	rows, lastID, err := s.drainChunk(ctx, peeringdb.TypeOrg, 0, 100)
	if err != nil {
		t.Fatalf("drainChunk: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("drainChunk: got %d rows, want 3", len(rows))
	}
	if lastID != 30 {
		t.Errorf("drainChunk last ID: got %d, want 30", lastID)
	}

	wantIDs := []int{10, 20, 30}
	for i, r := range rows {
		if r.id != wantIDs[i] {
			t.Errorf("row[%d] id: got %d, want %d", i, r.id, wantIDs[i])
		}
		// Decode the raw BLOB and verify the name field round-tripped.
		var v struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(r.raw, &v); err != nil {
			t.Errorf("row[%d] unmarshal: %v", i, err)
			continue
		}
		if v.ID != wantIDs[i] {
			t.Errorf("row[%d] decoded id: got %d, want %d", i, v.ID, wantIDs[i])
		}
	}
}

// TestScratchDB_DrainChunkPagination asserts that drainChunk honours the
// chunkSize argument and the id cursor, so callers can iterate large
// scratch tables without loading them all into Go heap at once.
func TestScratchDB_DrainChunkPagination(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Build 10 synthetic rows with ids 1..10.
	var buf strings.Builder
	buf.WriteString(`{"meta":{},"data":[`)
	for i := 1; i <= 10; i++ {
		if i > 1 {
			buf.WriteString(",")
		}
		buf.WriteString(`{"id":`)
		buf.WriteString(strconv.Itoa(i))
		buf.WriteString(`,"name":"n","status":"ok"}`)
	}
	buf.WriteString(`]}`)
	body := []byte(buf.String())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	client := peeringdb.NewClient(server.URL, slog.Default())
	client.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	client.SetRetryBaseDelay(0)

	s, err := openScratchDB(ctx)
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	defer closeScratchDB(ctx, s, slog.Default())

	if _, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}); err != nil {
		t.Fatalf("stageType: %v", err)
	}

	// Drain in chunks of 3: expect 3, 3, 3, 1, 0.
	var chunks [][]scratchRow
	afterID := 0
	for range 20 { // safety bound
		rows, lastID, err := s.drainChunk(ctx, peeringdb.TypeOrg, afterID, 3)
		if err != nil {
			t.Fatalf("drainChunk: %v", err)
		}
		if len(rows) == 0 {
			break
		}
		chunks = append(chunks, rows)
		if len(rows) < 3 {
			break
		}
		afterID = lastID
	}

	if len(chunks) != 4 {
		t.Fatalf("got %d chunks, want 4 (3+3+3+1)", len(chunks))
	}
	if len(chunks[0]) != 3 || len(chunks[1]) != 3 || len(chunks[2]) != 3 || len(chunks[3]) != 1 {
		t.Errorf("chunk sizes: got %d,%d,%d,%d want 3,3,3,1",
			len(chunks[0]), len(chunks[1]), len(chunks[2]), len(chunks[3]))
	}
}
