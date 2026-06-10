package sync_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestSync_IncrementalNoCursorFallsBackToBareList asserts that
// incremental sync with no prior cursor (fresh DB) falls through to the
// bare /api/<type> path — NOT the v1.18.2 ?since=1 bootstrap that
// tripped upstream's API_THROTTLE_REPEATED_REQUEST throttle and was
// reverted in v1.18.3. Historical-delete capture is deferred to a
// proper multi-cycle bootstrap design (v1.19+); the FK backfill catches
// orphans on demand.
func TestSync_IncrementalNoCursorFallsBackToBareList(t *testing.T) {
	t.Parallel()

	// Capture every URL the sync worker hits.
	var urls atomic.Pointer[[]string]
	urls.Store(&[]string{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Snapshot-and-replace pattern (atomic) so the slice grows safely
		// across the worker's serial fetch goroutine.
		old := *urls.Load()
		next := append([]string(nil), old...)
		next = append(next, r.URL.String())
		urls.Store(&next)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())

	// Fresh DB → no cursor for any type → all 13 should bare-list.
	if err := w.Sync(t.Context(), config.SyncModeIncremental); err != nil {
		t.Fatalf("sync: %v", err)
	}

	captured := *urls.Load()
	if len(captured) == 0 {
		t.Fatal("no URLs captured — sync didn't hit the test server")
	}
	// No URL may contain since= when cursor is zero (v1.18.3 contract).
	for _, u := range captured {
		if strings.Contains(u, "since=") {
			t.Errorf("URL has since= but cursor is zero (v1.18.2 bootstrap regression): %s", u)
		}
	}
}

// TestSync_FullModeStillBare asserts that mode=full on a FRESH database
// (zero cursor for every type) does NOT add since= — the bare /api/<type>
// path, same as the no-cursor incremental fallback (post-v1.18.3). On a
// populated database, full mode DOES issue an additional ?since=<cursor>
// fetch per type to capture the window's tombstones — see
// TestSync_FullModeFetchesTombstoneWindow.
func TestSync_FullModeStillBare(t *testing.T) {
	t.Parallel()

	var urls atomic.Pointer[[]string]
	urls.Store(&[]string{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		old := *urls.Load()
		next := append([]string(nil), old...)
		next = append(next, r.URL.String())
		urls.Store(&next)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
	}))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)

	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	captured := *urls.Load()
	for _, u := range captured {
		if strings.Contains(u, "since=") {
			t.Errorf("full sync URL unexpectedly contains since= (bootstrap leaked into full mode): %s", u)
		}
	}
}

// tombstoneTestOrgRow builds a full-shape upstream org JSON object for the
// tombstone-window tests below.
func tombstoneTestOrgRow(id int, status string, updated time.Time) map[string]any {
	return map[string]any{
		"id": id, "name": fmt.Sprintf("Org%d", id), "aka": "", "name_long": "",
		"website": "", "social_media": []any{}, "notes": "",
		"address1": "", "address2": "", "city": "", "state": "", "country": "US",
		"zipcode": "", "suite": "", "floor": "",
		"created": "2026-04-01T00:00:00Z", "updated": updated.Format(time.RFC3339),
		"status": status,
	}
}

// tombstoneTestServer stubs upstream for the tombstone-window tests: the
// bare /api/org list returns only the status='ok' org 1 (upstream filters
// bare lists to ok rows), and /api/org?since= returns the window's
// deletion event for org 2 — or a 500 when failSince is set.
func tombstoneTestServer(sawSince *atomic.Bool, failSince bool, deletedAt time.Time) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		typeName := strings.TrimPrefix(r.URL.Path, "/api/")
		skip := r.URL.Query().Get("skip")
		if typeName != "org" || (skip != "" && skip != "0") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":[]}`))
			return
		}
		if r.URL.Query().Get("since") != "" {
			sawSince.Store(true)
			if failSince {
				http.Error(w, "boom", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"meta":{},"data":` +
				string(mustJSON([]any{tombstoneTestOrgRow(2, "deleted", deletedAt)})) + `}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"meta":{},"data":` +
			string(mustJSON([]any{tombstoneTestOrgRow(1, "ok", deletedAt.Add(-24*time.Hour))})) + `}`))
	}))
}

// seedTombstoneTestOrgs seeds orgs 1 and 2 as status='ok' so the org table
// has a non-zero derived cursor before the full-mode cycle runs.
func seedTombstoneTestOrgs(t *testing.T, client *ent.Client, updated time.Time) {
	t.Helper()
	for id := 1; id <= 2; id++ {
		if _, err := client.Organization.Create().
			SetID(id).SetName(fmt.Sprintf("Org%d", id)).
			SetCreated(updated).SetUpdated(updated).SetStatus("ok").
			Save(t.Context()); err != nil {
			t.Fatalf("create org %d: %v", id, err)
		}
	}
}

// TestSync_FullModeFetchesTombstoneWindow asserts that a full-mode cycle
// over a populated table (non-zero derived cursor) issues a follow-up
// ?since=<cursor> fetch on top of the bare snapshot and lands the window's
// tombstones. Bare lists carry only status='ok' rows (upstream filters
// them), and committing the snapshot advances the derived MAX(updated)
// cursor past the pre-cycle window — so without this fetch, upstream
// deletes occurring between the last incremental cycle and a forced-full
// cycle (default: daily) were permanently lost (2026-06-10 audit).
func TestSync_FullModeFetchesTombstoneWindow(t *testing.T) {
	t.Parallel()

	seeded := time.Date(2026, 4, 1, 6, 0, 0, 0, time.UTC)
	deletedAt := seeded.Add(time.Hour)

	var sawSince atomic.Bool
	server := tombstoneTestServer(&sawSince, false, deletedAt)
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	seedTombstoneTestOrgs(t, client, seeded)

	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	if !sawSince.Load() {
		t.Fatal("full-mode cycle never issued the ?since= tombstone-window fetch")
	}
	org2, err := client.Organization.Get(t.Context(), 2)
	if err != nil {
		t.Fatalf("get org 2: %v", err)
	}
	if org2.Status != "deleted" {
		t.Errorf("org 2 status = %q, want %q (tombstone from the since window)", org2.Status, "deleted")
	}
	org1, err := client.Organization.Get(t.Context(), 1)
	if err != nil {
		t.Fatalf("get org 1: %v", err)
	}
	if org1.Status != "ok" {
		t.Errorf("org 1 status = %q, want %q", org1.Status, "ok")
	}
}

// TestSync_FullModeTombstoneWindowFailureFailsCycle asserts the fail-loud
// contract: when the tombstone-window fetch fails, the cycle must fail
// rather than commit the snapshot — committing would advance the derived
// cursor past deletes that were never seen, making the loss permanent.
func TestSync_FullModeTombstoneWindowFailureFailsCycle(t *testing.T) {
	t.Parallel()

	seeded := time.Date(2026, 4, 1, 6, 0, 0, 0, time.UTC)

	var sawSince atomic.Bool
	server := tombstoneTestServer(&sawSince, true, seeded.Add(time.Hour))
	defer server.Close()

	client, db := testutil.SetupClientWithDB(t)
	seedTombstoneTestOrgs(t, client, seeded)

	pdbClient := peeringdb.NewClient(server.URL, slog.Default())
	pdbClient.SetRateLimit(rate.NewLimiter(rate.Inf, 1))
	pdbClient.SetRetryBaseDelay(0)

	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := sync.NewWorker(pdbClient, client, db, sync.WorkerConfig{}, slog.Default())
	if err := w.Sync(t.Context(), config.SyncModeFull); err == nil {
		t.Fatal("sync succeeded despite tombstone-window fetch failure; cursor would advance past unseen deletes")
	}
	if !sawSince.Load() {
		t.Fatal("full-mode cycle never issued the ?since= tombstone-window fetch")
	}
	org2, err := client.Organization.Get(t.Context(), 2)
	if err != nil {
		t.Fatalf("get org 2: %v", err)
	}
	if org2.Status != "ok" {
		t.Errorf("org 2 status = %q, want %q (failed cycle must not land partial state)", org2.Status, "ok")
	}
}

// silenceContext exists to keep the context import live across edits.
var _ = context.Background
