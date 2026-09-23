package pdbcompat

import "github.com/dotwaffle/peeringdb-plus/internal/peeringdb"

// RowSize holds the conservative estimated serialized size in bytes per
// entity type at depth=0 (list-shape) and depth=2 (expanded-shape).
// Values are doubled from measured means so the budget check
// prefers false-positive 413s over OOM. Recalibrated every major
// milestone; drift >20% triggers a refresh.
type RowSize struct {
	Depth0 int
	Depth2 int
}

// typicalRowBytes is the conservative lookup table populated from
// BenchmarkRowSize in bench_row_size_test.go, then DOUBLED to cover
// worst-case rows (unusually long notes, multi-paragraph aka, etc.).
//
// Update procedure:
//  1. Run `go test -run=NONE -bench=BenchmarkRowSize ./internal/pdbcompat`
//     with -benchtime=20x -count=3 (seeds are deterministic; 20 iters
//     × 3 runs gives stable medians).
//  2. Record bytes/op from b.ReportMetric output for each entity × depth.
//  3. Double the measured mean, round UP to the nearest 64 bytes.
//  4. Commit the updated map in the same PR as the bench run.
//
// Last calibrated: 2026-09-23, after net and netixlan gained the PeeringDB
// 2.83.0 meta document ("meta":{} on every net and netixlan object, nested
// ones included). Raised: netixlan Depth0/Depth2, the poc and netfac Depth2
// rows (they embed a net object), and ixfac Depth2, which had drifted above
// its 2026-06-08 figure. No entry was lowered: org Depth2 keeps its
// 2026-06-08 value, which is above the new measurement.
//
// Raw measurements (median bytes/op, 2026-09-23 run):
//
//	entity       Depth0  Depth2
//	org             317    3498   ← Depth2 expands every *_set (largest row)
//	net             817    1280
//	fac             648    1684
//	ix              625    1318
//	poc             186    1399
//	ixlan           262    1277
//	ixpfx           163    1115
//	netixlan        326    2491
//	netfac          176    2401
//	ixfac           173    2216
//	carrier         228     815
//	carrierfac      139    1754
//	campus          269    1332
//
// The Depth2 column feeds the detail-path budget check
// (serveDetail → CheckBudget(1, type, 2, …)); lists are pinned to
// depth 0 by the list-depth guardrail and never consult it. A
// bare /api/org/<id> at the default depth=2 bills roughly 8.6 KiB for
// the single expanded org, so only a degenerately small budget would
// 413 it — the check is a floor, not a bound on _set cardinality.
var typicalRowBytes = map[string]RowSize{
	// Depth0 calibrated 2026-04-19; Depth2 recalibrated 2026-06-08 after the
	// depth-parity work, both from seed.Full at benchtime=20x × count=3.
	// 2026-09-23 raised the entries that had drifted (see above).
	// Values = ceil(2 × measured_bytes_per_op / 64) * 64.
	peeringdb.TypeOrg:        {Depth0: 704, Depth2: 8448},  // org (Depth2 expands net/fac/ix/carrier/campus sets → largest row in the table). Depth0 bumped from 640 → 704 — seed.Full mean is 325 bytes vs bench's single-row 317, so 2× rounds up one 64-byte bucket higher.
	peeringdb.TypeNet:        {Depth0: 1664, Depth2: 2560}, // Depth0/Depth2 bumped one 64-byte bucket each when ixp_update_exclude joined NetworkSerializer (2.80.1 parity).
	peeringdb.TypeFac:        {Depth0: 1344, Depth2: 3392},
	peeringdb.TypeIX:         {Depth0: 1280, Depth2: 2688},
	peeringdb.TypePoc:        {Depth0: 384, Depth2: 2816},
	peeringdb.TypeIXLan:      {Depth0: 576, Depth2: 2560},
	peeringdb.TypeIXPfx:      {Depth0: 384, Depth2: 2240},
	peeringdb.TypeNetIXLan:   {Depth0: 704, Depth2: 4992}, // Depth0 640 → 704 when meta joined NetworkIXLanSerializer (2.83.0 parity).
	peeringdb.TypeNetFac:     {Depth0: 384, Depth2: 4864},
	peeringdb.TypeIXFac:      {Depth0: 384, Depth2: 4480},
	peeringdb.TypeCarrier:    {Depth0: 512, Depth2: 1664},
	peeringdb.TypeCarrierFac: {Depth0: 320, Depth2: 3520},
	peeringdb.TypeCampus:     {Depth0: 576, Depth2: 2688},
}

// defaultRowSize is the fail-closed fallback for unknown entity names
// (future types not yet calibrated). Chosen at the upper end of the
// measured range so budget checks remain conservative even under drift.
const defaultRowSize = 4096

// TypicalRowBytes returns the conservative estimated serialized size
// per row for the named entity at the given depth. depth collapses to two
// buckets: 0 (the bare row) and >=1 (expanded), where the expanded estimate is
// the depth=2 figure. That is a safe over-estimate for ?depth=1, whose
// reverse-relation ID lists are smaller than the depth=2 full objects but
// larger than the bare row. Unknown entities return defaultRowSize to
// fail-closed against surprise types (e.g. a new type added to Registry before
// this map is updated).
func TypicalRowBytes(entity string, depth int) int {
	rs, ok := typicalRowBytes[entity]
	if !ok {
		return defaultRowSize
	}
	if depth >= 1 {
		return rs.Depth2
	}
	return rs.Depth0
}
