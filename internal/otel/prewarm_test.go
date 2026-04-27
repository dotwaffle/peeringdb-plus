package otel

import (
	"context"
	"slices"
	"testing"
)

func TestPrewarmCounters_NoError(t *testing.T) {
	// PrewarmCounters reads package-level Counter vars populated by
	// InitMetrics(). Post-WR-03 the function nil-guards each counter and
	// surfaces missing instruments via otel.Handle rather than panicking,
	// so this test locks the happy path: a clean post-InitMetrics call
	// doesn't error and doesn't panic.
	//
	// Match the package-wide convention (see internal/otel/metrics_test.go:
	// 10 occurrences) of pinning OTEL_METRICS_EXPORTER=none so InitMetrics()
	// does not attempt to dial an OTLP endpoint via autoexport during the
	// test — REVIEW WR-01.
	t.Setenv("OTEL_METRICS_EXPORTER", "none")
	if err := InitMetrics(); err != nil {
		t.Fatalf("InitMetrics: %v", err)
	}
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

// TestPeeringDBEntityTypes_Parity enforces the Phase 75 D-02 invariant:
// internal/otel.PeeringDBEntityTypes MUST stay in lock-step with the
// canonical 13-entity list used by internal/sync/initialcounts.go (the
// `queries` slice — the per-entity Count(ctx) closures keyed by the
// PeeringDB type name). internal/otel cannot import internal/sync
// (would create a cycle), so the canonical list below is the same
// hand-copied golden the _Cardinality sibling uses; this test adds a
// set-equality (order-agnostic) assertion on top of the count check
// so a same-cardinality rename (e.g. DEFER-70-06-01's "campus" →
// "campuses") does not silently split the metric series.
//
// REVIEW WR-04. Cites Phase 75 D-02
// (.planning/phases/75-code-side-observability/CONTEXT.md).
func TestPeeringDBEntityTypes_Parity(t *testing.T) {
	t.Parallel()
	// Canonical 13 entity type names. Source of truth:
	// internal/sync/initialcounts.go `queries` slice (the names the
	// startup-time Count(ctx) helpers key into the gauge cache by).
	canonical := []string{
		"org", "campus", "fac", "carrier", "carrierfac",
		"ix", "ixlan", "ixpfx", "ixfac",
		"net", "poc", "netfac", "netixlan",
	}
	gotSorted := slices.Sorted(slices.Values(PeeringDBEntityTypes))
	wantSorted := slices.Sorted(slices.Values(canonical))
	if !slices.Equal(gotSorted, wantSorted) {
		t.Errorf("PeeringDBEntityTypes drift vs canonical (Phase 75 D-02):\n  got  (sorted) = %v\n  want (sorted) = %v\nUpdate internal/otel/prewarm.go AND internal/sync/initialcounts.go together.",
			gotSorted, wantSorted)
	}
}
