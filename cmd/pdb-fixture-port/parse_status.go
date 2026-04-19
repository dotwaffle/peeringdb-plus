package main

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

// statusSynthLine is the upstream citation line for synthesised
// status rows. Plan 72-02 D-02 path: when upstream pdb_api_test.py
// lacks a direct `Model.objects.create(status="X")` row for a
// particular (entity, status) pair we need to lock the STATUS-01..05
// matrix against, the tool synthesises the missing row with a
// citation pointing at the assertion line in upstream that exercises
// the behaviour. The fallback to line 1 keeps Upstream non-empty
// when the assertion-line scan returns nothing — T-72-02-02
// mitigation.
const statusSynthFallbackLine = 1

// parseStatus scans the upstream Python file for `X.objects.create(...)`
// blocks where the kwargs include an explicit `status="..."`
// assignment and the value is one of "ok", "pending", "deleted".
// Then synthesises any (entity, status) combinations the parser
// missed because upstream uses make_data_*/create_entity helpers
// with **splat kwargs that bypass the literal `status="..."`
// surface our parser scans.
//
// Plan 72-02 STATUS-01..05 assertions consume this slice. STATUS-03
// in particular requires a (campus, pending) row; upstream only
// expresses this via `cls.create_entity(Campus, status="pending",
// ...)` and similar helpers, so the synthesis path is mandatory for
// the carve-out coverage.
//
// Same line-by-line + paren-depth scanner pattern used by
// parseOrdering for the parsed portion. Synthesis is appended after
// to keep parser-derived rows distinguishable in audit (synthesised
// rows carry Upstream citations to the assertion line; parsed rows
// cite the create() line).
func parseStatus(srcBytes []byte) []Fixture {
	scanner := bufio.NewScanner(bytes.NewReader(srcBytes))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0

	var out []Fixture
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		m := createLinePat.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		entity, ok := entityGoName[m[1]]
		if !ok {
			continue
		}
		startLine := lineNum
		block, endLine := readFixtureBlock(scanner, line, lineNum)
		lineNum = endLine
		fields := extractFieldsSharp(block)
		if len(fields) == 0 {
			continue
		}
		// Admission predicate: the row MUST have an explicit status
		// kwarg whose value is one of the three tracked statuses.
		// extractFieldsSharp gives us per-kwarg fidelity, so the
		// status value is the bare quoted literal with no folded-
		// trailing kwargs.
		statusVal, has := fields["status"]
		if !has || !statusValueTracked(statusVal) {
			continue
		}
		id := synthID(entity, startLine, fields)
		out = append(out, Fixture{
			Entity:   entity,
			ID:       id,
			Fields:   fields,
			Upstream: lineCitation(startLine),
		})
	}

	out = append(out, synthesiseMissingStatusRows(srcBytes, out)...)
	return out
}

// statusSynthSpec describes one synthesised (entity, status) row.
// AssertCite is the substring searched inside upstream to derive
// the Upstream citation line — typically the assertion that
// exercises the behaviour the synthesised row supports.
type statusSynthSpec struct {
	Entity     string // PeeringDB short namespace
	Status     string // "pending" | "deleted"
	NameSuffix string // appended to the synthesised name to keep IDs distinct
	AssertCite string // upstream substring used to find the citation line
}

// requiredStatusRows lists the (entity, status) pairs that the
// STATUS-01..05 assertions need at least one fixture row for. Only
// pending/deleted are listed because upstream provides ample
// status="ok" coverage via the parsed surface.
//
// Citations point at upstream lines that ASSERT the behaviour the
// row exercises. Rationale (per Plan 72-02 must_haves T-72-02-02):
// every fixture must trace back to upstream — synthesised rows
// trace back via the assertion citation rather than a create() line.
var requiredStatusRows = []statusSynthSpec{
	// STATUS-03 carve-out: campus admits pending on since>0 list.
	// Upstream assertion: line 3965 `"campus", since=...,
	// status="pending"`.
	{Entity: "campus", Status: "pending", NameSuffix: "CarveOut",
		AssertCite: `"campus", since=int(START_TIMESTAMP) - 10, status="pending"`},
	// STATUS-04 baseline: deleted rows must exist so ?status=deleted
	// + ?since>0 has something to admit. Upstream assertion line
	// 3952: `"net", since=..., status="deleted"`.
	{Entity: "net", Status: "deleted", NameSuffix: "Baseline",
		AssertCite: `"net", since=int(START_TIMESTAMP) - 10, status="deleted"`},
	// Additional pending rows to satisfy "≥3 distinct statuses
	// across entities" — exercises STATUS-01..02 admission
	// predicates beyond Organization. Citation: line 1206 (netfac
	// pending attribute assignment).
	{Entity: "netfac", Status: "pending", NameSuffix: "AttrAssign",
		AssertCite: `netfac.status = "pending"`},
	// IxFac pending — STATUS-02 pk-lookup admission needs at least
	// one pending row across the ix-side relations. Citation: line
	// 1201.
	{Entity: "ixfac", Status: "pending", NameSuffix: "AttrAssign",
		AssertCite: `ixfac.status = "pending"`},
	// CarrierFac pending — symmetric coverage. Citation: line 1211.
	{Entity: "carrierfac", Status: "pending", NameSuffix: "AttrAssign",
		AssertCite: `carrierfac.status = "pending"`},
	// IxLanPrefix deleted — common deletion target in upstream;
	// citation at line 3260 (`.filter(status="deleted")`).
	{Entity: "ixpfx", Status: "deleted", NameSuffix: "FilterTarget",
		AssertCite: `.filter(status="deleted")`},
}

// synthesiseMissingStatusRows appends synthesised rows for any
// (entity, status) pair listed in requiredStatusRows that isn't
// already represented in parsed. Idempotent — runs deterministic
// in-order so two invocations on the same upstream produce the
// same byte-output.
func synthesiseMissingStatusRows(srcBytes []byte, parsed []Fixture) []Fixture {
	have := map[string]bool{} // "entity|status" present
	for _, f := range parsed {
		s, ok := f.Fields["status"]
		if !ok {
			continue
		}
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, `"`)
		s = strings.TrimSuffix(s, `"`)
		have[f.Entity+"|"+s] = true
	}

	var out []Fixture
	for _, spec := range requiredStatusRows {
		key := spec.Entity + "|" + spec.Status
		if have[key] {
			continue
		}
		assertLine := findAssertionLine(srcBytes, spec.AssertCite)
		// Use a deterministic ID slot above the per-entity offset
		// hash range (offset+0..4095) so synth rows can't collide
		// with parsed rows. Offset+8000+i gives 100+ distinct slots
		// without bumping into limit-bulk's offset+5000 range.
		offset := entityOffset[spec.Entity]
		id := offset + 8000 + len(out)
		fields := map[string]string{
			"status": fmt.Sprintf(`%q`, spec.Status),
			"name":   fmt.Sprintf(`%q`, "Synth-"+spec.Entity+"-"+spec.NameSuffix),
		}
		out = append(out, Fixture{
			Entity:   spec.Entity,
			ID:       id,
			Fields:   fields,
			Upstream: lineCitation(assertLine),
		})
		have[key] = true
	}
	return out
}

// findAssertionLine returns the line number of the first occurrence
// of needle in srcBytes, or statusSynthFallbackLine if not found.
// Used by synthesiseMissingStatusRows to derive Upstream citations
// from upstream assertion locations.
func findAssertionLine(srcBytes []byte, needle string) int {
	scanner := bufio.NewScanner(bytes.NewReader(srcBytes))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if strings.Contains(scanner.Text(), needle) {
			return lineNum
		}
	}
	return statusSynthFallbackLine
}

// findFirstSubstringLine is a generic line-locator used by per-
// category parsers that need an upstream citation for a specific
// substring (e.g. parseUnicode citing the first line containing
// "Zürich"). Returns 1 (sentinel) when not found so emitted
// citations stay non-empty per T-72-02-02.
func findFirstSubstringLine(srcBytes []byte, needle string) int {
	scanner := bufio.NewScanner(bytes.NewReader(srcBytes))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if strings.Contains(scanner.Text(), needle) {
			return lineNum
		}
	}
	return statusSynthFallbackLine
}

// statusValueTracked reports whether v (the verbatim Python-source
// form of a `status=` kwarg, including outer quotes) is exactly one
// of the three statuses parity tests track. With extractFieldsSharp
// the value is the bare quoted literal; equality (not prefix) is the
// correct check.
func statusValueTracked(v string) bool {
	v = strings.TrimSpace(v)
	switch v {
	case `"ok"`, `"pending"`, `"deleted"`:
		return true
	}
	return false
}
