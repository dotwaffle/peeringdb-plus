package sync

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
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

	s, err := openScratchDB(ctx, "")
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

	s, err := openScratchDB(ctx, "")
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

	s, err := openScratchDB(ctx, "")
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	defer closeScratchDB(ctx, s, slog.Default())

	stats, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}, discardRows)
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

	s, err := openScratchDB(ctx, "")
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	defer closeScratchDB(ctx, s, slog.Default())

	if _, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}, discardRows); err != nil {
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

// TestScratchDB_FailedStage asserts what a stageType call that fails part
// way leaves in the table. With discardRows, the table keeps only the rows
// that it had before the call. The table is larger than the 2 MiB page
// cache, and the failed stream replaces every row, so SQLite writes
// changed pages of the table to the file before the error. Without a
// transaction, the rows before the error stay. With journal_mode=OFF, the
// rollback cannot restore the written pages. With keepRows, every row
// streamed before the error stays.
func TestScratchDB_FailedStage(t *testing.T) {
	t.Parallel()

	const n = 2000
	pad := strings.Repeat("x", 2000)
	body := func(name string, truncate bool) []byte {
		var buf strings.Builder
		buf.WriteString(`{"meta":{},"data":[`)
		for i := 1; i <= n; i++ {
			if i > 1 {
				buf.WriteString(",")
			}
			buf.WriteString(`{"id":`)
			buf.WriteString(strconv.Itoa(i))
			buf.WriteString(`,"name":"` + name + `","pad":"` + pad + `"}`)
		}
		if truncate {
			buf.WriteString(`,{"id":`) // The decoder fails here.
			return []byte(buf.String())
		}
		buf.WriteString(`]}`)
		return []byte(buf.String())
	}
	good, bad := body("old", false), body("new", true)

	tests := []struct {
		name        string
		onFailure   stageFailure
		wantChanged int
	}{
		{"discardRows", discardRows, 0},
		{"keepRows", keepRows, n},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if requests.Add(1) == 1 {
					_, _ = w.Write(good)
					return
				}
				_, _ = w.Write(bad)
			}))
			defer server.Close()

			client := peeringdb.NewClient(server.URL, slog.Default())
			client.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
			client.SetRetryBaseDelay(0)

			s, err := openScratchDB(ctx, "")
			if err != nil {
				t.Fatalf("openScratchDB: %v", err)
			}
			defer closeScratchDB(ctx, s, slog.Default())

			if _, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}, discardRows); err != nil {
				t.Fatalf("first stageType: %v", err)
			}
			if _, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}, tt.onFailure); err == nil {
				t.Fatal("second stageType: got nil error for a truncated body")
			}

			rows, _, err := s.drainChunk(ctx, peeringdb.TypeOrg, 0, 2*n)
			if err != nil {
				t.Fatalf("drainChunk: %v", err)
			}
			if len(rows) != n {
				t.Fatalf("staged %d rows after the failed stage, want %d", len(rows), n)
			}
			var changed int
			for _, r := range rows {
				var v struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(r.raw, &v); err != nil {
					t.Fatalf("row %d unmarshal: %v", r.id, err)
				}
				if v.Name != "old" {
					changed++
				}
			}
			if changed != tt.wantChanged {
				t.Errorf("%d of %d rows changed by the failed stage, want %d", changed, n, tt.wantChanged)
			}
		})
	}
}

// TestScratchDB_StageErrorSource asserts that a stageType error carries
// errScratchDB when the scratch DB fails, and not when the upstream body
// fails. A caller tolerates only the second kind (windowFailureTolerated).
// The scratch DB fails here because max_page_count stops it from growing.
func TestScratchDB_StageErrorSource(t *testing.T) {
	t.Parallel()

	var buf strings.Builder
	buf.WriteString(`{"meta":{},"data":[`)
	for i := 1; i <= 200; i++ {
		if i > 1 {
			buf.WriteString(",")
		}
		buf.WriteString(`{"id":` + strconv.Itoa(i) + `,"pad":"` + strings.Repeat("x", 2000) + `"}`)
	}
	large := []byte(buf.String() + `]}`)

	truncated := []byte(`{"meta":{},"data":[{"id":1},{"id":`)
	tests := []struct {
		name        string
		body        []byte
		limitPages  bool
		onFailure   stageFailure
		wantScratch bool
	}{
		// discardRows makes no commit, so only the failed insert marks
		// the error.
		{"scratch db full, discardRows", large, true, discardRows, true},
		{"scratch db full, keepRows", large, true, keepRows, true},
		{"truncated upstream body, discardRows", truncated, false, discardRows, false},
		{"truncated upstream body, keepRows", truncated, false, keepRows, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(tt.body)
			}))
			defer server.Close()

			client := peeringdb.NewClient(server.URL, slog.Default())
			client.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
			client.SetRetryBaseDelay(0)

			s, err := openScratchDB(ctx, "")
			if err != nil {
				t.Fatalf("openScratchDB: %v", err)
			}
			defer closeScratchDB(ctx, s, slog.Default())

			if tt.limitPages {
				var pages int
				if err := s.db.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
					t.Fatalf("page_count: %v", err)
				}
				if _, err := s.db.ExecContext(ctx, fmt.Sprintf("PRAGMA max_page_count = %d", pages)); err != nil {
					t.Fatalf("max_page_count: %v", err)
				}
			}

			_, err = s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}, tt.onFailure)
			if err == nil {
				t.Fatal("stageType: got nil error")
			}
			if got := errors.Is(err, errScratchDB); got != tt.wantScratch {
				t.Errorf("errors.Is(%v, errScratchDB) = %v, want %v", err, got, tt.wantScratch)
			}
		})
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

	s, err := openScratchDB(ctx, "")
	if err != nil {
		t.Fatalf("openScratchDB: %v", err)
	}
	defer closeScratchDB(ctx, s, slog.Default())

	if _, err := s.stageType(ctx, client, peeringdb.TypeOrg, time.Time{}, discardRows); err != nil {
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
