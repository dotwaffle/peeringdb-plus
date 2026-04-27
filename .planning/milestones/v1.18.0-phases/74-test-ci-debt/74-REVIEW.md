---
phase: 74-test-ci-debt
reviewed: 2026-04-26T23:00:00Z
depth: standard
files_reviewed: 7
files_reviewed_list:
  - cmd/pdb-schema-generate/main.go
  - cmd/pdb-schema-generate/main_test.go
  - deploy/grafana/dashboards/pdbplus-overview.json
  - deploy/grafana/dashboard_test.go
  - internal/visbaseline/redactcli.go
  - internal/visbaseline/reportcli.go
  - internal/visbaseline/checkpoint.go
findings:
  blocker: 0
  warning: 3
  info: 4
  total: 7
status: issues_found
---

# Phase 74: Code Review Report

**Reviewed:** 2026-04-26T23:00:00Z
**Depth:** standard
**Files Reviewed:** 7
**Status:** issues_found

## Summary

All three plans deliver on their stated requirements (TEST-01, TEST-02, TEST-03)
and the repo lint+test gates are green. However, two of the new tests have
subtle false-negative holes that undermine the "structural invariant" claim
made in the SUMMARY documents, and one minor security consideration was
overlooked in the visbaseline G304 disposition. None are blockers; all are
quality issues worth flagging before they accrete more cruft.

Highlights:

- `TestDashboard_NoOrphanTemplateVars` uses unbounded `strings.Contains`, so
  template variables whose names are a prefix of another variable's name will
  be falsely marked as referenced.
- `TestGenerateIndexes` legacy_net_fixture sub-test contains a tautological
  assertion (`got == ExpectedIndexesFor(...)`) that can never fail given the
  trivial-wrapper implementation.
- visbaseline change quality (filepath.Clean + canonical comments) is good;
  the `G122` nolint suppression is honest and well-documented.

## Warnings

### WR-01: Orphan-template-var test has substring-prefix false-negative

**File:** `deploy/grafana/dashboard_test.go:359-363`
**Issue:** The orphan check builds `dollarRef := "$" + v.Name` and runs
`strings.Contains(exprBlob, dollarRef)`. This is unbounded substring
matching: a future variable named `type` would be considered "referenced"
if any query contains `$type_total`, `$typecount`, etc., because
`"$type"` is a substring of those longer names. Same risk for the
`braceRef` form (`${type}` would match `${type:csv}`, `${type_count}`).

This directly contradicts the test's stated promise (Phase 74 D-02) of
catching "the whole class of orphan-template-var accumulation". Today the
three live vars (`datasource`, `type`, `process_group`) have no
prefix-overlap, so the test passes. The hole is dormant, not active.

**Fix:** Match on a bounded form. One option:

```go
// Build a list of all reference patterns and require an exact post-name
// boundary character (or end-of-string).
needles := []string{
    "$" + v.Name,            // legacy bare form
    "${" + v.Name + "}",     // basic brace form
    "${" + v.Name + ":",     // formatted brace form (e.g. ${var:csv})
}
referenced := false
for _, n := range needles {
    if !strings.Contains(haystack, n) {
        continue
    }
    // For "$" + v.Name, also require the next byte (if any) is not
    // a valid name continuation char [A-Za-z0-9_].
    referenced = true
    break
}
```

A cleaner alternative is a precompiled regex per variable:
`regexp.MustCompile(`\$(?:\{` + regexp.QuoteMeta(v.Name) + `[}:]|` +
regexp.QuoteMeta(v.Name) + `\b)`).MatchString(haystack)`. Either way the
substring check needs a boundary.

### WR-02: Orphan test only scans `Expr` and `Datasource.UID`, missing `legendFormat`, panel `title`, `description`, alert annotations

**File:** `deploy/grafana/dashboard_test.go:336-347`
**Issue:** Grafana template variables are also valid in `legendFormat`,
panel `title`, panel `description`, and many alert/annotation surfaces.
A future variable used only in a legend (e.g. `legendFormat: "{{type}}
$process_group"`) would be flagged as orphan even though it drives a
visible UI element. False positive.

Today none of the three live vars are referenced outside `Expr` /
`Datasource.UID`, so the test passes. Like WR-01, this is a dormant
hole that contradicts the structural-invariant promise.

**Fix:** Extend the haystack to include `tgt.LegendFormat` plus a couple
of panel-level fields. The `panel` struct will need additional JSON
tags (`Description string `json:"description"`` etc). Alternatively,
walk the raw JSON as `map[string]any` and search every string value —
catches every present and future variable site without per-field
plumbing.

### WR-03: `legacy_net_fixture` sub-test contains a tautological assertion

**File:** `cmd/pdb-schema-generate/main_test.go:586-591`
**Issue:** The sub-test asserts
```go
got := generateIndexes("net", ot)
exp := ExpectedIndexesFor("net", ot)
if !slicesEqual(got, exp) { ... }
```
But `ExpectedIndexesFor` is literally `return generateIndexes(apiPath, ot)`
(`main.go:613-615`). The assertion compares `f(x)` to `f(x)` — it cannot
fail. Test code that cannot detect any defect is dead code dressed as a
guarantee.

The SUMMARY (74-01) explicitly defends the trivial-wrapper choice
("drift protection comes from the structural sanity loop, not from
re-deriving the same rules in two places"), and that is sound. But the
identity assertion here doesn't add value; it just performs work and
makes future readers think there is a real check happening. The
structural sanity loop (lines 638-662) is what does the work.

**Fix:** Remove the `exp := ExpectedIndexesFor(...)` block from
`legacy_net_fixture` and from the per-entity loop (line 631 + 634-636
mirror this). The tests still cover what the SUMMARY claims they cover.

If you want to keep `ExpectedIndexesFor` exported as the documented
contract anchor, fine — but don't pretend the call site asserts
something. A single doc-comment in `main.go` already does that work.

## Info

### IN-01: `slicesEqual` reinvents `slices.Equal` from stdlib

**File:** `cmd/pdb-schema-generate/main_test.go:670-680`
**Issue:** Go 1.21+ ships `slices.Equal[S ~[]E, E comparable](s1, s2 S) bool`
in stdlib. Project pins Go 1.26+. The hand-rolled helper duplicates
stdlib without benefit (per CLAUDE.md GO-MD-1 "Prefer stdlib").

**Fix:** Drop the local helper, replace 3 call sites with
`slices.Equal(got, want)` and add `"slices"` to the import block.

### IN-02: `process_group` template variable populated from primary-only metric

**File:** `deploy/grafana/dashboards/pdbplus-overview.json:127, 136`
**Issue:** The `process_group` variable definition is
`label_values(pdbplus_sync_peak_heap_bytes, service_namespace)`. Per
CLAUDE.md "Sync observability" section, `pdbplus_sync_peak_heap_bytes`
is a watermark gauge emitted only by the primary instance. The variable
dropdown will therefore only show `primary` as a value (replicas don't
emit this metric). When wired into panel 35 (Live Heap by Instance)
via `service_namespace=~"$process_group"`, selecting `primary` filters
to primary instances only — fine. But selecting `replica` from the
dropdown is impossible because the dropdown never offers it.

The default `.*` allValue makes this work in practice. Still, a more
honest variable definition would query `go_memory_used_bytes` (the
metric panel 35 actually uses) or `up{service_name="peeringdb-plus"}`
so the dropdown covers both process groups.

**Fix:**
```json
"definition": "label_values(go_memory_used_bytes{service_name=\"peeringdb-plus\"}, service_namespace)",
"query": {
    "query": "label_values(go_memory_used_bytes{service_name=\"peeringdb-plus\"}, service_namespace)",
    "refId": "StandardVariableQuery"
}
```

This is a Phase 65 / observability decision, not a Phase 74 hygiene
issue, but it surfaced as part of the wire-in. Note in next dashboard
revision, no immediate action.

### IN-03: `filepath.Clean` is not a path-traversal containment check

**File:** `internal/visbaseline/redactcli.go:101, 113`,
`internal/visbaseline/reportcli.go:427, 445`,
`internal/visbaseline/checkpoint.go:120`
**Issue:** The leading comments at every site say "filepath.Clean handles
the genuine `..` traversal sub-class of gosec G304 risk". This wording
overstates what `filepath.Clean` does. `filepath.Clean("/safe/../etc/passwd")`
returns `/etc/passwd` — the `..` is RESOLVED, not REJECTED. Clean does
not contain a path within an expected base; it just normalises the
lexical form.

For visbaseline this is fine because:
1. `typeName` arrives via `filepath.Base(filepath.Dir(path))` which
   strips separators (so `..` cannot survive).
2. `path` comes from `filepath.WalkDir` which provides cleaned paths
   already.
3. The CLI tool's threat model accepts operator-supplied paths.

The Clean is genuinely defense-in-depth (it normalises odd forms
before logging/error messages, eliminates `//` and trailing `/`,
etc.) but the comment misrepresents it as a traversal-prevention step.

**Fix:** Reword the canonical comment to:
```go
// visbaseline is a CLI tool — paths are operator-supplied by contract.
// filepath.Clean normalises the path for consistent logging/error
// reporting and quiets gosec G304's static analyser; it does NOT
// constrain the path to any base directory (Clean resolves `..`,
// it does not reject them).
```

If actual base-directory containment is desired, use
`filepath.IsLocal` (Go 1.20+) or check
`!strings.HasPrefix(filepath.Clean(p), expectedBase)`.

### IN-04: Per-entity sub-test reads `schema/peeringdb.json` via cwd-relative path

**File:** `cmd/pdb-schema-generate/main_test.go:600`
**Issue:** `schemaPath := filepath.Join("..", "..", "schema", "peeringdb.json")`
relies on `go test` setting cwd to the package directory. This is true
for `go test ./cmd/pdb-schema-generate/...` and any normal invocation,
but a future test runner (e.g. a coverage tool, a custom harness, or
running the compiled test binary directly) that doesn't cd first will
fail with a confusing "reading ../../schema/peeringdb.json: no such
file or directory".

**Fix:** Use `runtime.Caller(0)` to anchor to the source file's
directory, or accept `os.Getenv("REPO_ROOT")` with a stable fallback.
Low priority — current usage is correct; this is a robustness nit.

```go
_, thisFile, _, _ := runtime.Caller(0)
schemaPath := filepath.Join(filepath.Dir(thisFile), "..", "..", "schema", "peeringdb.json")
```

---

_Reviewed: 2026-04-26T23:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
