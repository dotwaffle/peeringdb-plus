package pdbcompat

import "github.com/dotwaffle/peeringdb-plus/internal/peeringdb"

// RowSize holds the conservative estimated serialized size in bytes per
// entity type at depth=0 (list-shape) and depth=2 (expanded-shape).
// Values are doubled from measured means per D-03 so the budget check
// prefers false-positive 413s over OOM. Recalibrated every major
// milestone; drift >20% triggers a refresh plan.
type RowSize struct {
	Depth0 int
	Depth2 int
}

// typicalRowBytes is the D-03 lookup table populated from
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
// Last calibrated: 2026-04-19 (Phase 71 Plan 02 — seed.Full scale, 2×
// multiplier per D-03, rounded up to nearest 64 bytes).
//
// Raw measurements (median bytes/op from the 2026-04-19 calibration):
//
//	entity       Depth0  Depth2
//	org             317    4287   ← Depth2 expands every *_set (largest row)
//	net             783    1154
//	fac             648    1302
//	ix              625    1241
//	poc             186     968
//	ixlan           262     926
//	ixpfx           163     429
//	netixlan        316    1364
//	netfac          176    1615
//	ixfac           173    1483
//	carrier         228     728
//	carrierfac      139    1035
//	campus          269    1254
//
// Hosts the /api/org budget check's worst case: an unfiltered
// ?depth=2 list bills roughly 8.6 KiB per row, so the 128 MiB default
// caps org@depth=2 at ~15k rows — comfortably above the ~35 live
// orgs that currently carry populated child sets.
var typicalRowBytes = map[string]RowSize{
	// Calibrated 2026-04-19 from seed.Full at benchtime=20x × count=3.
	// Values = ceil(2 × measured_bytes_per_op / 64) * 64.
	// Raw measurements preserved in .planning/phases/71-memory-safe-response-paths/71-02-SUMMARY.md.
	peeringdb.TypeOrg:        {Depth0: 704, Depth2: 8576}, // org (Depth2 expands net/fac/ix/carrier/campus sets → largest row in the table). Depth0 bumped from 640 → 704 (Phase 71 WR-02 — seed.Full mean is 325 bytes vs bench's single-row 317, so 2× rounds up one 64-byte bucket higher).
	peeringdb.TypeNet:        {Depth0: 1600, Depth2: 2368},
	peeringdb.TypeFac:        {Depth0: 1344, Depth2: 2624},
	peeringdb.TypeIX:         {Depth0: 1280, Depth2: 2496},
	peeringdb.TypePoc:        {Depth0: 384, Depth2: 1984},
	peeringdb.TypeIXLan:      {Depth0: 576, Depth2: 1856},
	peeringdb.TypeIXPfx:      {Depth0: 384, Depth2: 896},
	peeringdb.TypeNetIXLan:   {Depth0: 640, Depth2: 2752},
	peeringdb.TypeNetFac:     {Depth0: 384, Depth2: 3264},
	peeringdb.TypeIXFac:      {Depth0: 384, Depth2: 3008},
	peeringdb.TypeCarrier:    {Depth0: 512, Depth2: 1472},
	peeringdb.TypeCarrierFac: {Depth0: 320, Depth2: 2112},
	peeringdb.TypeCampus:     {Depth0: 576, Depth2: 2560},
}

// defaultRowSize is the fail-closed fallback for unknown entity names
// (future types not yet calibrated). Chosen at the upper end of the
// measured range so budget checks remain conservative even under drift.
const defaultRowSize = 4096

// TypicalRowBytes returns the conservative estimated serialized size
// per row for the named entity at the given depth. depth is clamped to
// {0, 2} — the only depths pdbcompat actually serves. Unknown entities
// return defaultRowSize to fail-closed against surprise types (e.g.
// a new type added to Registry before this map is updated).
func TypicalRowBytes(entity string, depth int) int {
	rs, ok := typicalRowBytes[entity]
	if !ok {
		return defaultRowSize
	}
	if depth >= 2 {
		return rs.Depth2
	}
	return rs.Depth0
}
