package sync

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestScheduler_ReplicaLatchRecovers locks the 2026-06-10 audit fix: a
// replica that boots before the primary's first successful sync starts
// with the readiness latch unset, and must flip it once sync history
// appears in the (LiteFS-replicated) database — previously the latch was
// evaluated once at StartScheduler entry and never again, so such a
// replica served 503 on every data route for the life of the process.
func TestScheduler_ReplicaLatchRecovers(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	w := NewWorker(nil, client, db, WorkerConfig{
		IsPrimary: func() bool { return false }, // permanent replica
	}, slog.Default())

	go w.StartScheduler(ctx, 20*time.Millisecond)

	// Boot state: no sync history → not ready.
	time.Sleep(60 * time.Millisecond)
	if w.HasCompletedSync() {
		t.Fatal("replica reported ready with no sync history")
	}

	// Simulate LiteFS replication delivering the primary's first
	// successful sync.
	syncTime := time.Now().UTC()
	id, err := RecordSyncStart(ctx, db, syncTime, "full")
	if err != nil {
		t.Fatalf("record sync start: %v", err)
	}
	if err := RecordSyncComplete(ctx, db, id, Status{
		LastSyncAt: syncTime,
		Duration:   time.Second,
		Status:     "success",
	}); err != nil {
		t.Fatalf("record sync complete: %v", err)
	}

	// The next heartbeat must observe the history and flip the latch.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if w.HasCompletedSync() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("replica latch never recovered after sync history appeared")
}
