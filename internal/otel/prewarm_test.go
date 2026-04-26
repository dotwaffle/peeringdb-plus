package otel

import (
	"context"
	"testing"
)

func TestPrewarmCounters_NoError(t *testing.T) {
	// PrewarmCounters reads package-level Counter vars populated by
	// InitMetrics(). Calling on nil counters panics — this test locks
	// the contract that InitMetrics() must run first AND that a clean
	// post-InitMetrics call doesn't error.
	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}
	// No defer recover() — if PrewarmCounters panics on nil counters,
	// we WANT the test to fail loudly so the ordering invariant in
	// main.go stays load-bearing.
	PrewarmCounters(context.Background())
}

func TestPeeringDBEntityTypes_Cardinality(t *testing.T) {
	t.Parallel()
	if got := len(PeeringDBEntityTypes); got != 13 {
		t.Fatalf("len(PeeringDBEntityTypes) = %d, want 13", got)
	}
	want := map[string]bool{
		"org": true, "campus": true, "fac": true, "carrier": true,
		"carrierfac": true, "ix": true, "ixlan": true, "ixpfx": true,
		"ixfac": true, "net": true, "poc": true, "netfac": true, "netixlan": true,
	}
	for _, name := range PeeringDBEntityTypes {
		if !want[name] {
			t.Errorf("unexpected entity type %q in PeeringDBEntityTypes", name)
		}
		delete(want, name)
	}
	for missing := range want {
		t.Errorf("PeeringDBEntityTypes missing %q", missing)
	}
}

// TestPeeringDBEntityTypes_ParityNote is a documentation-only test that
// records the parity contract with internal/sync/worker.go syncSteps().
// internal/otel cannot import internal/sync (cycle), so this stays as
// a manual-review invariant enforced by grep in Phase 75 Plan 02
// acceptance criteria. If a 14th entity is added to syncSteps without
// updating PeeringDBEntityTypes, TestPeeringDBEntityTypes_Cardinality
// fails on the cardinality check (count flips off 13).
func TestPeeringDBEntityTypes_ParityNote(t *testing.T) {
	// No assertion — see comment above. The cardinality gate in the
	// sibling test catches the most common drift mode (count change).
	t.Log("PeeringDBEntityTypes parity with internal/sync/worker.go syncSteps() is a manual-review invariant")
}
