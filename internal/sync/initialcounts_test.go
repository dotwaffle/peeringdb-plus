// Phase 75 Plan 01 (OBS-01): tests for InitialObjectCounts.
//
// These tests lock the contract that InitialObjectCounts populates the SAME
// 13 keys as the existing sync-completion path (worker.syncSteps()), so the
// shared atomic.Pointer cache in cmd/peeringdb-plus/main.go can be primed by
// either path without key drift.

package sync

import (
	"context"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// TestInitialObjectCounts_AllThirteenTypes asserts that on a seeded DB
// the helper returns exactly 13 keys, every value non-zero. seed.Full
// inserts at least one row of every entity type per CLAUDE.md § Testing.
func TestInitialObjectCounts_AllThirteenTypes(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seed.Full(t, client)

	counts, err := InitialObjectCounts(context.Background(), client)
	if err != nil {
		t.Fatalf("InitialObjectCounts: %v", err)
	}
	if got := len(counts); got != 13 {
		t.Fatalf("len(counts) = %d, want 13", got)
	}
	for name, n := range counts {
		if n == 0 {
			t.Errorf("counts[%q] = 0, want non-zero (seed.Full should populate every type)", name)
		}
	}
}

// TestInitialObjectCounts_EmptyDB asserts that on an empty DB the helper
// returns 13 keys with explicit-zero values rather than a missing-key map.
// The OTel ObservableGauge callback in InitObjectCountGauges iterates the
// returned map; missing keys would suppress the corresponding type label
// and produce "No data" panels instead of a flat-zero line.
func TestInitialObjectCounts_EmptyDB(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	// No seed — expect 13 keys, all zero.

	counts, err := InitialObjectCounts(context.Background(), client)
	if err != nil {
		t.Fatalf("InitialObjectCounts: %v", err)
	}
	if got := len(counts); got != 13 {
		t.Fatalf("len(counts) = %d, want 13", got)
	}
	for name, n := range counts {
		if n != 0 {
			t.Errorf("counts[%q] = %d, want 0", name, n)
		}
	}
}

// TestInitialObjectCounts_KeyParityWithSyncSteps locks the key set against
// the canonical 13-name list emitted by worker.syncSteps(). If syncSteps()
// grows a 14th type, len(counts) flips off 13 and this test catches it on
// the initial-counts side.
func TestInitialObjectCounts_KeyParityWithSyncSteps(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)

	counts, err := InitialObjectCounts(context.Background(), client)
	if err != nil {
		t.Fatalf("InitialObjectCounts: %v", err)
	}

	want := map[string]bool{
		"org": true, "campus": true, "fac": true, "carrier": true,
		"carrierfac": true, "ix": true, "ixlan": true, "ixpfx": true,
		"ixfac": true, "net": true, "poc": true, "netfac": true, "netixlan": true,
	}
	if len(counts) != len(want) {
		t.Fatalf("counts has %d keys, want %d", len(counts), len(want))
	}
	for name := range want {
		if _, ok := counts[name]; !ok {
			t.Errorf("counts missing key %q", name)
		}
	}
}

// TestInitialObjectCounts_PocPolicyBypass locks the regression fix for the
// `pdbplus_data_type_count{type="poc"}` 2x/0.5x oscillation documented in
// .planning/debug/poc-count-doubling-halving.md.
//
// seed.Full creates 3 POCs: 1 visible="Public" (ID 500) + 2 visible="Users"
// (IDs 9000, 9001). Without the privctx.TierUsers stamp inside
// InitialObjectCounts, the Poc.Policy() filter would drop the 2 Users-tier
// rows from the count (TierPublic is the fail-closed default for any
// un-stamped context per privctx.TierFrom), yielding 1 instead of 3.
//
// The OnSyncComplete writer counts all 3 rows because it runs under
// privacy.DecisionContext(ctx, privacy.Allow). If the two writers
// disagree, the gauge cache holds different values on instances that
// never sync (replicas) versus instances that just synced (primary),
// and `max by(type)` across the 8-instance fleet oscillates between
// the two values every sync cycle. Locking POC count = 3 here proves
// the startup primer is symmetric with the sync-completion writer.
func TestInitialObjectCounts_PocPolicyBypass(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	seed.Full(t, client)

	counts, err := InitialObjectCounts(context.Background(), client)
	if err != nil {
		t.Fatalf("InitialObjectCounts: %v", err)
	}

	// 3 = 1 Public (Result.Poc, ID 500) + 2 Users (Result.UsersPoc 9000,
	// Result.UsersPoc2 9001) per seed.Full's mixed-visibility contract
	// locked by TestFull_HasUsersPocs in the seed package.
	if got := counts["poc"]; got != 3 {
		t.Fatalf("counts[\"poc\"] = %d, want 3 (1 Public + 2 Users); "+
			"a value of 1 means the Poc.Policy() filter still applies — "+
			"InitialObjectCounts must stamp privctx.TierUsers before Count "+
			"to match the OnSyncComplete writer's privacy.DecisionContext "+
			"bypass. See .planning/debug/poc-count-doubling-halving.md.", got)
	}
}
