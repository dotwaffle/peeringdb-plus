package main

import (
	"fmt"
)

// inBulkNetworkCount is the number of synthesised Network rows
// emitted into InFixtures. Must be ≥ 5001 so IN-01 (large `__in`
// list bypasses the SQLite 999-variable limit via json_each rewrite)
// can be exercised across the boundary. 5001 = the smallest count
// that proves the rewrite handles ≥5× the SQLite limit.
const inBulkNetworkCount = 5001

// inBulkBaseID is the starting ID of the contiguous IN-01 bulk
// block. Test consumers hit `?id__in=100000,100001,...,105000` and
// assert all 5001 rows return in one response. The block is
// deliberately ABOVE all other category ID ranges (status synth at
// offset+8000 → ~30k, limit bulk at offset+5000 → ~28k) so cross-
// category collisions are impossible.
const inBulkBaseID = 100000

// inSentinelID is the empty-__in probe row. Test consumers query
// `?id__in=` (empty value) and assert zero rows returned. The
// sentinel is included in InFixtures so downstream seeders pre-
// populate the row, but ?id__in= must short-circuit to empty
// without touching it (Phase 69 D-06).
const inSentinelID = 999999

// inBaseASN walks 4300000000+i (RFC 6996 private-use ASN space,
// disjoint from limit bulk's 4200000000+i range so cross-category
// fixtures don't accidentally share ASN keys when both sets are
// loaded into the same ent client).
const inBaseASN = 4300000000

// parseIn emits the IN-category fixture set:
//   - 5001 contiguous Network rows at IDs 100000..105000 for the
//     IN-01 large-list boundary
//   - 1 Network sentinel at ID=999999 with __marker="empty_in_probe"
//     for the IN-02 empty-__in test
//
// Per Plan 72-03 D-02 path (synthesis): upstream pdb_api_test.py
// does not contain a literal 5001-row block. Each synth entry
// carries a per-row Upstream citation pointing at a representative
// seed line — when the upstream stub or real file declares an
// "InBulkNet-Seed" Network row, that line; otherwise rest.py's
// json_each call site sentinel via fallback.
//
// Synthesised IDs are NOT offset-based — IN bulk uses literal
// 100000+i so test query strings stay grep-able ("?id__in=
// 100000,100001,...").
func parseIn(srcBytes []byte) []Fixture {
	seedLine := findSeedLine(srcBytes, "Network", "InBulkNet")

	out := make([]Fixture, 0, inBulkNetworkCount+1)
	for i := range inBulkNetworkCount {
		out = append(out, Fixture{
			Entity: "net",
			ID:     inBulkBaseID + i,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   fmt.Sprintf(`"InBulkNet-%05d"`, i),
				"asn":    fmt.Sprintf("%d", inBaseASN+i),
			},
			Upstream: lineCitation(seedLine),
		})
	}

	// Empty-__in sentinel. The __marker key is consumed by the
	// downstream parity test to detect this row without re-deriving
	// the magic ID; the seeder still creates the row with status=ok
	// so a control query (?id=999999) returns it (proving the
	// short-circuit at ?id__in= empty path is doing the right
	// thing — empty list returns zero, NOT all rows).
	out = append(out, Fixture{
		Entity: "net",
		ID:     inSentinelID,
		Fields: map[string]string{
			"status":   `"ok"`,
			"name":     `"InEmptyProbe"`,
			"asn":      fmt.Sprintf("%d", inBaseASN+inBulkNetworkCount),
			"__marker": `"empty_in_probe"`,
		},
		Upstream: lineCitation(seedLine),
	})

	return out
}
