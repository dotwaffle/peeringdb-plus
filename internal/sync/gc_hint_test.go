package sync

import (
	"fmt"
	"runtime/metrics"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

// forcedGCCycles returns the process-wide count of GC cycles forced by
// runtime.GC.
func forcedGCCycles(t *testing.T) uint64 {
	t.Helper()
	s := []metrics.Sample{{Name: "/gc/cycles/forced:gc-cycles"}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		t.Fatalf("metric %s unsupported (kind %v)", s[0].Name, s[0].Value.Kind())
	}
	return s[0].Value.Uint64()
}

// TestSyncUpsertPass_GCHintGatedByCount locks the gcHintMinRows gate:
// syncUpsertPass forces a GC after a type that upserted at least
// gcHintMinRows rows, and not after a smaller type. Each case syncs one
// fac row plus an org list just below or at the threshold, so the
// forced-GC count equals the number of types at or above it.
//
// Not parallel: the forced-GC counter is process-wide. Go runs a
// top-level test that does not call t.Parallel alone, so no other sync
// in this package adds to the count.
func TestSyncUpsertPass_GCHintGatedByCount(t *testing.T) {
	tests := []struct {
		name       string
		orgs       int
		wantForced uint64
	}{
		{name: "below_threshold", orgs: gcHintMinRows - 1, wantForced: 0},
		{name: "at_threshold", orgs: gcHintMinRows, wantForced: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			orgs := make([]any, 0, tt.orgs)
			for id := 1; id <= tt.orgs; id++ {
				orgs = append(orgs, makeOrg(id, fmt.Sprintf("Org %d", id), "ok"))
			}
			f.responses["org"] = orgs
			f.responses["fac"] = []any{makeFac(1, 1, "Fac 1", "ok")}
			w, _ := newTestWorker(t, f)

			before := forcedGCCycles(t)
			if err := w.Sync(t.Context(), config.SyncModeFull); err != nil {
				t.Fatalf("sync: %v", err)
			}
			if got := forcedGCCycles(t) - before; got != tt.wantForced {
				t.Errorf("forced GC cycles = %d, want %d", got, tt.wantForced)
			}
		})
	}
}
