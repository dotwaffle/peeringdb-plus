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

// TestSync_FullModeStillBare asserts that mode=full does NOT add since=.
// Full sync uses the bare /api/<type> path; this is also the fallback
// path when incremental has no cursor (post-v1.18.3).
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
