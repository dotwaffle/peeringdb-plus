package pdbcompat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

func TestTypicalRowBytes_AllThirteenTypes(t *testing.T) {
	want := []string{
		peeringdb.TypeOrg, peeringdb.TypeNet, peeringdb.TypeFac,
		peeringdb.TypeIX, peeringdb.TypePoc, peeringdb.TypeIXLan,
		peeringdb.TypeIXPfx, peeringdb.TypeNetIXLan, peeringdb.TypeNetFac,
		peeringdb.TypeIXFac, peeringdb.TypeCarrier, peeringdb.TypeCarrierFac,
		peeringdb.TypeCampus,
	}
	if len(typicalRowBytes) != len(want) {
		t.Fatalf("typicalRowBytes has %d entries, want %d (13 PeeringDB types)", len(typicalRowBytes), len(want))
	}
	for _, name := range want {
		rs, ok := typicalRowBytes[name]
		if !ok {
			t.Errorf("typicalRowBytes missing entry for %q", name)
			continue
		}
		if rs.Depth0 <= 0 || rs.Depth2 <= 0 {
			t.Errorf("%s: both Depth0 and Depth2 must be > 0; got %+v", name, rs)
		}
	}
}

func TestTypicalRowBytes_UnknownFailsClosed(t *testing.T) {
	got := TypicalRowBytes("this-type-does-not-exist", 0)
	if got != defaultRowSize {
		t.Errorf("unknown entity: got %d, want defaultRowSize=%d", got, defaultRowSize)
	}
}

func TestTypicalRowBytes_Depth0LessThanDepth2(t *testing.T) {
	// Invariant: depth=2 is never smaller than depth=0 for the same type
	// (depth=2 expands nested _set fields, so it can only grow).
	for name, rs := range typicalRowBytes {
		if rs.Depth2 < rs.Depth0 {
			t.Errorf("%s: Depth2=%d < Depth0=%d (depth=2 should expand, not shrink)", name, rs.Depth2, rs.Depth0)
		}
	}
}

func TestTypicalRowBytes_DepthClamp(t *testing.T) {
	// depth values other than 0 or >=2 should still resolve to a sensible bucket.
	// Current behaviour: depth<2 uses Depth0; depth>=2 uses Depth2.
	if TypicalRowBytes(peeringdb.TypeNet, 0) != typicalRowBytes[peeringdb.TypeNet].Depth0 {
		t.Error("depth=0 lookup mismatch")
	}
	if TypicalRowBytes(peeringdb.TypeNet, 1) != typicalRowBytes[peeringdb.TypeNet].Depth0 {
		t.Error("depth=1 should bucket to Depth0 (pdbcompat does not expose depth=1)")
	}
	if TypicalRowBytes(peeringdb.TypeNet, 2) != typicalRowBytes[peeringdb.TypeNet].Depth2 {
		t.Error("depth=2 lookup mismatch")
	}
	if TypicalRowBytes(peeringdb.TypeNet, 5) != typicalRowBytes[peeringdb.TypeNet].Depth2 {
		t.Error("depth>2 should bucket to Depth2 (future-proof against depth=3)")
	}
}

// TestTypicalRowBytes_CalibrationDrift fails if a serializer grew since the
// last calibration (Phase 71 Plan 02) such that typicalRowBytes[name].Depth0
// no longer covers the 2× safety margin documented in rowsize.go D-03.
//
// The test seeds seed.Full, runs each entity's List at Limit=100, marshals
// every returned row to JSON, and asserts:
//
//	typicalRowBytes[name].Depth0 >= 2 × mean_bytes_measured
//
// A failure means either (a) a serializer added a new field / grew an
// existing one and the hardcoded rowsize table needs a fresh calibration
// via BenchmarkRowSize, or (b) the seed fixture gained a much larger row
// than the calibration saw. Either way, the 413 budget math in
// CheckBudget could under-count and let an OOM through — fix before
// merging.
//
// Gated on testing.Short() so `go test -short ./...` in CI's fast lane
// stays snappy; run via `go test ./internal/pdbcompat -run
// TestTypicalRowBytes_CalibrationDrift` or the default (non-short) CI
// matrix.
func TestTypicalRowBytes_CalibrationDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping calibration drift test under -short")
	}
	t.Parallel()

	client := testutil.SetupClient(t)
	seed.Full(t, client)
	ctx := context.Background()

	entities := []string{
		peeringdb.TypeOrg, peeringdb.TypeNet, peeringdb.TypeFac,
		peeringdb.TypeIX, peeringdb.TypePoc, peeringdb.TypeIXLan,
		peeringdb.TypeIXPfx, peeringdb.TypeNetIXLan, peeringdb.TypeNetFac,
		peeringdb.TypeIXFac, peeringdb.TypeCarrier, peeringdb.TypeCarrierFac,
		peeringdb.TypeCampus,
	}

	for _, name := range entities {
		t.Run(name, func(t *testing.T) {
			tc, ok := Registry[name]
			if !ok {
				t.Fatalf("registry missing %q", name)
			}
			rows, _, err := tc.List(ctx, client, QueryOptions{Limit: 100})
			if err != nil {
				t.Fatalf("list %s: %v", name, err)
			}
			if len(rows) == 0 {
				// A zero-row sample is an inadequate calibration input —
				// skip rather than produce a meaningless pass. Extending
				// seed.Full to cover the type is the right fix.
				t.Skipf("seed produced 0 rows for %s — extend seed.Full", name)
			}

			var totalBytes int
			for i, row := range rows {
				buf, err := json.Marshal(row)
				if err != nil {
					t.Fatalf("marshal %s row %d: %v", name, i, err)
				}
				totalBytes += len(buf)
			}
			mean := totalBytes / len(rows)

			want := typicalRowBytes[name].Depth0
			required := 2 * mean
			if want < required {
				t.Errorf("%s: Depth0 calibration drifted — typicalRowBytes[%q].Depth0 = %d, "+
					"but 2 × measured mean (%d bytes) = %d. Serializer grew since Phase 71 "+
					"Plan 02 calibration. Re-run BenchmarkRowSize (see rowsize.go update "+
					"procedure) and commit fresh values.",
					name, name, want, mean, required)
			}
		})
	}
}
