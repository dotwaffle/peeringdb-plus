package pdbcompat

import (
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
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
