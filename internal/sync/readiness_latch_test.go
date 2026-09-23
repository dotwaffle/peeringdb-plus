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
//
// The test steps the scheduler through its IsPrimary role checks: the
// entry check follows the boot-time sync_status read, and each
// heartbeat's check precedes that heartbeat's re-read. Counting role
// checks orders the assertions against the scheduler without sleeps.
func TestScheduler_ReplicaLatchRecovers(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	// roleChecks receives once per IsPrimary call. The unbuffered send
	// holds the scheduler until the test takes the value.
	roleChecks := make(chan struct{})
	w := NewWorker(nil, client, db, WorkerConfig{
		IsPrimary: func() bool {
			select {
			case roleChecks <- struct{}{}:
			case <-ctx.Done():
			}
			return false // permanent replica
		},
	}, slog.Default())
	awaitRoleChecks := func(n int) {
		t.Helper()
		for i := range n {
			select {
			case <-roleChecks:
			case <-time.After(10 * time.Second):
				t.Fatalf("scheduler stalled: role check %d of %d never came", i+1, n)
			}
		}
	}

	done := make(chan struct{})
	go func() {
		w.StartScheduler(ctx, time.Millisecond)
		close(done)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("scheduler did not stop after cancel")
		}
	}()

	// Boot state: no sync history → not ready. Check 1 follows the boot
	// read; check 3 follows heartbeat 1's re-read.
	awaitRoleChecks(3)
	if w.HasCompletedSync() {
		t.Fatal("replica reported ready with no sync history")
	}

	// Simulate LiteFS replication delivering the primary's first
	// successful sync. The boot read already ran, so only the heartbeat
	// re-read can flip the latch from here.
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

	// The heartbeat released by the next check re-reads after the
	// insert; the check after that follows its re-read.
	awaitRoleChecks(2)
	if !w.HasCompletedSync() {
		t.Fatal("replica latch never recovered after sync history appeared")
	}
}
