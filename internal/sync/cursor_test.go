// Quick task 260428-mu0 cursor tests. Mirrors the structure of
// initialcounts_test.go — both files exercise raw-SQL paths against the
// underlying *sql.DB outside ent.

package sync

import (
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestGetMaxUpdated_NullReturnsZero asserts that an empty table → NULL →
// zero time, which is the contract that lets stageOneTypeToScratch fall
// through to the bare-list path on first sync.
func TestGetMaxUpdated_NullReturnsZero(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	got, err := GetMaxUpdated(ctx, db, "organizations")
	if err != nil {
		t.Fatalf("GetMaxUpdated: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time on empty table, got %v", got)
	}
}

// TestGetMaxUpdated_ReturnsLatest seeds three rows with monotonic `updated`
// timestamps and asserts MAX returns the latest. Tolerates ±1µs rounding
// from SQLite datetime serialisation.
func TestGetMaxUpdated_ReturnsLatest(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	t1 := now.Add(-2 * time.Hour)
	t2 := now.Add(-1 * time.Hour)
	t3 := now

	if _, err := client.Organization.Create().
		SetID(1).SetName("Org1").SetCreated(t1).SetUpdated(t1).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("create org 1: %v", err)
	}
	if _, err := client.Organization.Create().
		SetID(2).SetName("Org2").SetCreated(t2).SetUpdated(t2).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("create org 2: %v", err)
	}
	if _, err := client.Organization.Create().
		SetID(3).SetName("Org3").SetCreated(t3).SetUpdated(t3).SetStatus("ok").
		Save(ctx); err != nil {
		t.Fatalf("create org 3: %v", err)
	}

	got, err := GetMaxUpdated(ctx, db, "organizations")
	if err != nil {
		t.Fatalf("GetMaxUpdated: %v", err)
	}
	delta := got.Sub(t3)
	if delta < -time.Microsecond || delta > time.Microsecond {
		t.Errorf("MAX(updated) = %v, want %v (±1µs)", got, t3)
	}
}

// TestEntityTablesMatchSchema introspects sqlite_master to assert every
// value in entityTables exists as a real table. Catches table-name drift
// from an entgo upgrade (e.g. inflection regression) at test time, not at
// deploy time when the SELECT MAX(updated) starts erroring.
func TestEntityTablesMatchSchema(t *testing.T) {
	t.Parallel()
	_, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()

	for typeName, table := range entityTables {
		var got string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`,
			table,
		).Scan(&got)
		if err != nil {
			t.Errorf("entityTables[%q]=%q not found in schema: %v", typeName, table, err)
		}
	}
}

// TestEntityTablesCoverSyncSteps locks the closed-set invariant: every type
// emitted by canonicalStepOrder has a row in entityTables. If a 14th type
// is added, this fails fast in the same package — the analogue of
// TestInitialObjectCounts_KeyParityWithSyncSteps for the cursor side.
func TestEntityTablesCoverSyncSteps(t *testing.T) {
	t.Parallel()
	for _, name := range canonicalStepOrder {
		if _, ok := entityTables[name]; !ok {
			t.Errorf("entityTables missing entry for sync step %q", name)
		}
	}
	if len(entityTables) != len(canonicalStepOrder) {
		t.Errorf("entityTables size = %d, want %d (one per sync step)", len(entityTables), len(canonicalStepOrder))
	}
}
