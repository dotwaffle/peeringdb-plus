package main

import (
	"bufio"
	"bytes"
	"fmt"
)

// limitBulkNetworkCount is the number of synthesised Network entries
// emitted into LimitFixtures. Must stay strictly above the 250-row
// default page cap so LIMIT-01 (limit=0 unlimited) can be exercised
// across the boundary. 260 = 250 + 10 buffer rows.
const limitBulkNetworkCount = 260

// limitDepthOrgCount is the number of Organization seed rows emitted
// for LIMIT-02 depth-on-list guardrail testing. Small, fixed count;
// the depth assertion only needs >1 row to verify the cap, not bulk.
const limitDepthOrgCount = 5

// limitDepthIxCount mirrors limitDepthOrgCount for InternetExchange
// rows referenced by the depth-on-list traversal cap.
const limitDepthIxCount = 5

// limitSyntheticBaseLine is the upstream-citation line used for the
// synthesised bulk Network rows. Per plan: each synthesised entry
// still carries Upstream="pdb_api_test.py:<line>" citing the
// assertion line it covers — rest.py:494-497 is the underlying
// authority but the Upstream field references the test file's
// representative seed-row line. We use the upstream `def
// test_limit_unlimited_001` line (literal location varies between
// upstream versions; tool reads the seed row out of the parsed file
// when present, falls back to a sentinel otherwise).
const limitSyntheticFallbackLine = 1

// parseLimit emits the LIMIT-category fixture set:
//   - 260+ Network rows that exercise the LIMIT-01 unlimited-pagination
//     boundary (above the default 250-row page cap)
//   - A small set of Organization + InternetExchange rows for the
//     LIMIT-02 depth-on-list guardrail
//
// Per plan 72-02 D-02 (synthesised path): upstream pdb_api_test.py
// does not contain a literal 260-row block. The tool synthesises the
// bulk by replicating a representative seed-row shape, with each
// synthesised entry carrying a "// synthesised" provenance comment
// in the emitted file (via the section preamble template) and a
// per-entry Upstream citation pointing at the seed row in the
// upstream test file (or rest.py:494-497 as the authoritative
// behaviour citation).
//
// Synthesised IDs walk a contiguous range starting from the
// per-entity offset — they remain unique within LimitFixtures and
// cannot collide with Ordering/Status rows (which derive from
// sha256 truncated to low-12-bits + offset).
func parseLimit(srcBytes []byte) []Fixture {
	netSeedLine := findSeedLine(srcBytes, "Network", "LimitNet")
	orgSeedLine := findSeedLine(srcBytes, "Organization", "LimitOrg")
	ixSeedLine := findSeedLine(srcBytes, "InternetExchange", "LimitIX")

	var out []Fixture

	// Bulk Network rows for LIMIT-01.
	netOffset := entityOffset["net"]
	for i := 0; i < limitBulkNetworkCount; i++ {
		out = append(out, Fixture{
			Entity: "net",
			// Use a deterministic increasing ID range above the
			// existing offset so synth IDs from other categories
			// (which use offset + 12-bit hash, max offset+4095)
			// don't collide with the bulk range. Bulk starts at
			// offset+5000.
			ID: netOffset + 5000 + i,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   fmt.Sprintf(`"LimitBulkNet-%04d"`, i),
				"asn":    fmt.Sprintf("%d", 4200000000+i),
			},
			Upstream: lineCitation(netSeedLine),
		})
	}

	// Org rows for LIMIT-02 depth-on-list guardrail.
	orgOffset := entityOffset["org"]
	for i := 0; i < limitDepthOrgCount; i++ {
		out = append(out, Fixture{
			Entity: "org",
			ID:     orgOffset + 5000 + i,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   fmt.Sprintf(`"LimitDepthOrg-%02d"`, i),
			},
			Upstream: lineCitation(orgSeedLine),
		})
	}

	// IX rows for LIMIT-02 depth-on-list guardrail.
	ixOffset := entityOffset["ix"]
	for i := 0; i < limitDepthIxCount; i++ {
		out = append(out, Fixture{
			Entity: "ix",
			ID:     ixOffset + 5000 + i,
			Fields: map[string]string{
				"status": `"ok"`,
				"name":   fmt.Sprintf(`"LimitDepthIX-%02d"`, i),
			},
			Upstream: lineCitation(ixSeedLine),
		})
	}

	return out
}

// findSeedLine scans srcBytes for a `<Entity>.objects.create(` line
// followed (within the same fixture block) by a `name="<namePrefix>...
// "` kwarg and returns the line number of the create() call. If no
// match is found, returns limitSyntheticFallbackLine so emitted
// citations stay non-empty (T-72-02-02 mitigation: every Fixture
// entry carries a non-empty Upstream).
func findSeedLine(srcBytes []byte, entity, namePrefix string) int {
	scanner := bufio.NewScanner(bytes.NewReader(srcBytes))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	wantCreate := entity + ".objects.create("

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		if !bytes.Contains([]byte(line), []byte(wantCreate)) {
			continue
		}
		startLine := lineNum
		block, endLine := readFixtureBlock(scanner, line, lineNum)
		lineNum = endLine
		fields := extractFieldsSharp(block)
		if name, ok := fields["name"]; ok && len(name) >= len(namePrefix)+1 {
			// fields stores the verbatim quoted value, e.g.
			// `"LimitNet-Seed"`. Check the inner-quoted prefix.
			if len(name) > 1 && name[0] == '"' &&
				len(name) > len(namePrefix)+1 &&
				name[1:1+len(namePrefix)] == namePrefix {
				return startLine
			}
		}
	}
	return limitSyntheticFallbackLine
}

// lineCitation returns the standard "pdb_api_test.py:<line>" form.
// Centralised so a future change to the citation format flows
// through every parser uniformly.
func lineCitation(line int) string {
	return fmt.Sprintf("pdb_api_test.py:%d", line)
}
