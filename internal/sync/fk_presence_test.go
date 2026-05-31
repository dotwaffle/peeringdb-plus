package sync

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestFKHasParent_PresenceCache verifies that fkHasParent memoises a
// confirmed DB presence for the rest of the sync cycle, so sibling child
// rows that share an untouched parent skip the repeat Exist() query
// (audit P4). The cache is proven by deleting the row after the first
// check: a fresh DB query would then miss, but the cached answer holds,
// and resetFKState clears it.
func TestFKHasParent_PresenceCache(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	ts := time.Unix(1700000000, 0)
	org, err := client.Organization.Create().
		SetName("CacheOrg").
		SetNameFold("cacheorg").
		SetStatus("ok").
		SetCreated(ts).
		SetUpdated(ts).
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	tx, err := client.Tx(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })

	w := &Worker{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	w.resetFKState()

	// First check hits the DB and caches the positive result.
	if !w.fkHasParent(ctx, tx, peeringdb.TypeOrg, org.ID) {
		t.Fatal("expected parent present on first check")
	}
	// Delete the row inside the tx; a fresh Exist() would now miss.
	if err := tx.Organization.DeleteOneID(org.ID).Exec(ctx); err != nil {
		t.Fatalf("delete org: %v", err)
	}
	if w.dbHasRecord(ctx, tx, peeringdb.TypeOrg, org.ID) {
		t.Fatal("sanity: dbHasRecord should report the deleted row absent")
	}
	// Cached presence still answers true — the repeat query was skipped.
	if !w.fkHasParent(ctx, tx, peeringdb.TypeOrg, org.ID) {
		t.Fatal("expected cached presence after row deleted")
	}
	// A miss is never cached: an unseen ID reports absent.
	if w.fkHasParent(ctx, tx, peeringdb.TypeOrg, 999999) {
		t.Fatal("unseen id should report absent")
	}
	// resetFKState clears the cache: the deleted row now reports absent.
	w.resetFKState()
	if w.fkHasParent(ctx, tx, peeringdb.TypeOrg, org.ID) {
		t.Fatal("after reset, deleted row should report absent")
	}
}
