package sync_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/time/rate"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestSync_IncrementalBootstrapUsesSince1 asserts that incremental sync
// with no prior cursor (fresh DB) bootstraps with ?since=1 instead of
// hitting the bare /api/<type> path. This is the load-bearing fix from
// quick task 260428-2zl: bare /api/<type> returns status='ok' only,
// leaving us blind to historical tombstones that occurred before our
// observability started.
func TestSync_IncrementalBootstrapUsesSince1(t *testing.T) {
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

	// Fresh DB → no cursor for any type → all 13 should bootstrap with since=1.
	if err := w.Sync(t.Context(), config.SyncModeIncremental); err != nil {
		t.Fatalf("sync: %v", err)
	}

	captured := *urls.Load()
	if len(captured) == 0 {
		t.Fatal("no URLs captured — sync didn't hit the test server")
	}
	// Every captured URL must contain since=1 (T2 contract). At minimum,
	// the first 13 URLs (one per entity type) must — incremental fallback
	// would re-fire as a bare /api path after a since= attempt, but our
	// server returns 200 for everything so no fallback should fire.
	for _, u := range captured {
		if !strings.Contains(u, "since=1") {
			t.Errorf("URL missing since=1 (incremental bootstrap regressed): %s", u)
		}
	}
}

// TestSync_FullModeStillBare asserts that mode=full does NOT add since=1.
// Full sync uses the bare /api/<type> path so the upstream returns its
// default status='ok'-only result set; the inference-by-absence delete
// pass (removed in T6) used to fill in the deleted set. Post-T6 full
// sync is the operator escape-hatch for fresh-bootstrap from a known-good
// snapshot; bootstrap-with-since=1 is the steady-state default.
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

// silenceContext exists to keep the context import live across edits.
var _ = context.Background
