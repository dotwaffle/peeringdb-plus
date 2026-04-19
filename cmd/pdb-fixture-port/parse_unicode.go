package main

import (
	"fmt"
)

// unicodeFoldedEntities is the set of PeeringDB types that carry
// Phase-69 fold columns. UNICODE-01/02 assertions iterate this matrix
// against representative non-ASCII inputs to confirm the unifold
// pipeline normalises them identically across all five entity classes
// (network/facility/ix are the dominant 3; campus/carrier/org cover
// the long tail).
var unicodeFoldedEntities = []string{
	"net", "fac", "ix", "org", "campus", "carrier",
}

// unicodeFoldedFields is the per-entity field surface UNICODE-01/02
// must cover. Matches the Phase 69 unifold shadow-column set: the
// folding pipeline rewrites these field values into `<field>_fold`
// columns so case-/diacritic-insensitive __icontains/__istartswith
// queries hit indexes. The 4-field matrix below is the reviewable
// minimum; per-entity field availability is a runtime concern (the
// fixture rows carry a "name"/"aka"/"name_long"/"city" key
// regardless — the query layer ignores keys that don't exist on a
// given entity).
var unicodeFoldedFields = []string{"name", "aka", "name_long", "city"}

// unicodeSamples is the set of input strings exercising the fold
// matrix. The pairings (diacritic + ASCII baseline) per row let the
// downstream parity test assert fold-equivalence: searching for
// "Zurich" must match "Zürich GmbH". CJK and Greek samples cover
// scripts where unidecode produces non-trivial transliterations.
//
// Each sample carries an Upstream citation. When upstream
// pdb_api_test.py contains a literal occurrence of a sample
// (rare — checked at parse time via findSampleLine), use that
// line; otherwise cite the rest.py:576 unidecode call site that
// authorises the fold behaviour we exercise.
var unicodeSamples = []struct {
	Input string // verbatim, will be rendered as a Go-quoted string
	Cite  string // upstream substring searched for citation derivation
}{
	{Input: "Zürich GmbH", Cite: "Zürich"},
	{Input: "Zurich GmbH", Cite: "Zurich"},
	{Input: "München AG", Cite: "München"},
	{Input: "Munchen AG", Cite: "Munchen"},
	{Input: "Paris", Cite: "Paris"},
	{Input: "東京", Cite: "東京"},
	{Input: "上海", Cite: "上海"},
	{Input: "Αθήνα", Cite: "Αθήνα"},
	// Combining-mark form — Zürich written with U+0308 COMBINING
	// DIAERESIS rather than the precomposed ü. The fold pipeline
	// must normalise both forms to the same fold value.
	{Input: "Zu\u0308rich", Cite: "Zürich"},
}

// parseUnicode emits the UNICODE-category fixture set: 6 entities ×
// 4 fields × ≥4 sample inputs = ~144 baseline entries. Matrix
// produces enough variety for parity tests to assert per-field /
// per-entity coverage without bloating fixtures.go beyond the
// CONTEXT.md size envelope.
//
// Plan 72-03 must_have: ≥32 entries with diacritic + CJK coverage.
// The matrix above generates 6 × 4 × 9 = 216 entries; coverage
// requirement is met with margin.
//
// Per-entity ID slots use the offset+6000 range (above status synth
// at offset+8000? — actually status synth uses offset+8000+i and
// limit bulk uses offset+5000+i; unicode uses offset+6000+i to
// avoid collisions with both). IDs walk monotonically across the
// (entity × field × sample) cartesian product so two runs produce
// the same ordering before the section's sortFixtures pass.
func parseUnicode(srcBytes []byte) []Fixture {
	var out []Fixture
	for _, entity := range unicodeFoldedEntities {
		offset := entityOffset[entity]
		idx := 0
		for _, field := range unicodeFoldedFields {
			for _, sample := range unicodeSamples {
				cite := findFirstSubstringLine(srcBytes, sample.Cite)
				out = append(out, Fixture{
					Entity: entity,
					ID:     offset + 6000 + idx,
					Fields: map[string]string{
						field:    fmt.Sprintf("%q", sample.Input),
						"status": `"ok"`,
					},
					Upstream: lineCitation(cite),
				})
				idx++
			}
		}
	}
	return out
}
