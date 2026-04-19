package main

import (
	"bufio"
	"bytes"
	"strings"
)

// parseStatus scans the upstream Python file for `X.objects.create(...)`
// blocks where the kwargs include an explicit `status="..."`
// assignment and the value is one of "ok", "pending", "deleted".
//
// Plan 72-02 STATUS-01..05 assertions consume this slice. The
// `(campus, pending)` carve-out (STATUS-03) requires at least one
// such pair; this parser does not synthesise — it relies on the
// upstream file (or testdata stub) declaring it. If absent, plan
// 72-02 must update the upstream-file/testdata to include one.
//
// Same line-by-line + paren-depth scanner pattern used by
// parseOrdering. Field extraction reuses extractFields(); only the
// post-extraction admission predicate differs.
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
	return out
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
