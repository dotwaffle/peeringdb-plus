---
phase: 69-unicode-operator-in-robustness
plan: 04
subsystem: pdbcompat
tags:
  - pdbcompat
  - filter
  - unicode
  - in-rewrite
requires:
  - 69-01 (unifold.Fold primitive)
  - 69-02 (_fold shadow columns in 6 ent schemas)
  - 69-03 (sync populates _fold columns)
provides:
  - coerceToCaseInsensitive helper (UNICODE-02)
  - foldAwarePredicate routing via <field>_fold on 16 fields across 6 types (UNICODE-01)
  - json_each(?) __in rewrite (IN-01)
  - QueryOptions.EmptyResult + errEmptyIn sentinel (IN-02)
  - ParseFilters(url.Values, TypeConfig) ([]func(*sql.Selector), bool, error) signature
affects:
  - internal/pdbcompat/filter.go
  - internal/pdbcompat/filter_test.go
  - internal/pdbcompat/fuzz_test.go
  - internal/pdbcompat/handler.go
  - internal/pdbcompat/handler_test.go (seed rows updated to populate name_fold)
  - internal/pdbcompat/registry.go (TypeConfig.FoldedFields + QueryOptions.EmptyResult)
  - internal/pdbcompat/registry_funcs.go (13 list closures gain EmptyResult guard)
tech-stack:
  added: []
  patterns:
    - "sentinel error + caller short-circuit flag (errEmptyIn → emptyResult bool)"
    - "sql.ExprP for raw-SQL fragment with single JSON-array bind"
    - "parallel config map on TypeConfig (FoldedFields) consulted by ParseFilters via nil-safe map read"
key-files:
  created:
    - internal/pdbcompat/phase69_filter_test.go
  modified:
    - internal/pdbcompat/filter.go
    - internal/pdbcompat/filter_test.go
    - internal/pdbcompat/fuzz_test.go
    - internal/pdbcompat/handler.go
    - internal/pdbcompat/handler_test.go
    - internal/pdbcompat/registry.go
    - internal/pdbcompat/registry_funcs.go
decisions:
  - "Empty-__in design: B (QueryOptions.EmptyResult flag + errEmptyIn sentinel) — cleaner no-SQL path than WHERE 1=0 and keeps the 13 list closures' exit-fast pattern uniform"
  - "sql.ExprP chosen over sql.P + manual builder — single-line expression, args... bind cleanly"
  - "FoldedFields lives on TypeConfig (not a parallel map) — nil-safe reads, per-type config stays together"
  - "Guard inner-loop parseErr naming in buildIn — avoids future shadow bug if an outer err is introduced"
metrics:
  duration: "~50 minutes"
  completed_date: 2026-04-19
  tasks: 2
  files_modified: 7
  files_created: 1
  commits: 2
---

# Phase 69 Plan 04: pdbcompat filter layer — coerce + fold + json_each + empty sentinel

One-liner: Four tightly-coupled filter-layer behaviours land in `internal/pdbcompat/filter.go` — operator coercion per rest.py:638-641, shadow-column routing to `<field>_fold` on 16 fields across 6 entity types, `__in` rewrite to SQLite `json_each(?)` single-bind, and an empty-`__in` sentinel that short-circuits the whole request without running SQL.

## What shipped

All four requirements (UNICODE-01, UNICODE-02, IN-01, IN-02) ship in a single plan because they co-edit `ParseFilters` / `buildPredicate` / `buildContains` / `buildStartsWith` / `buildIn`. Splitting them would have created serialised merge-conflict thrash inside one file.

### UNICODE-02 — operator coercion (D-04)

```go
func coerceToCaseInsensitive(op string) string {
    switch op {
    case "contains":
        return "icontains"
    case "startswith":
        return "istartswith"
    }
    return op
}
```

Called as the first statement inside `buildPredicate`. Scope is strict per the decision: `__contains` / `__startswith` only. `__exact`, `__iexact`, `__in`, `__gt/lt/gte/lte` pass through untouched.

### UNICODE-01 — shadow-column routing (D-01, D-03)

`TypeConfig.FoldedFields map[string]bool` is populated on 6 of 13 entity configs:

| Type | Folded fields |
|---|---|
| `org` | name, aka, city |
| `net` | name, aka, name_long |
| `fac` | name, aka, city |
| `ix` | name, aka, name_long, city |
| `carrier` | name, aka |
| `campus` | name |

Total: 16 fields. `ParseFilters` threads `folded := tc.FoldedFields[field]` (nil-map safe) into `buildPredicate`, which routes string `buildContains` / `buildStartsWith` / `buildExact` to the `<field>_fold` column with `unifold.Fold(value)` on the RHS. Non-folded fields and non-string fields are untouched.

The 7 entity types with no folded fields (poc, ixlan, ixpfx, netixlan, netfac, ixfac, carrierfac) leave `FoldedFields` nil — map reads return `false` without a nil-check.

### IN-01 — json_each rewrite (D-05)

```go
s.Where(sql.ExprP(s.C(field)+" IN (SELECT value FROM json_each(?))", jsonStr))
```

The entire comma-separated input is marshalled to a JSON array (`[1,2,3,...]` or `["a","b","c"]`) and passes as a single parameter bind. SQLite's `json_each` table-valued function expands the array server-side. This bypasses `SQLITE_MAX_VARIABLE_NUMBER` entirely — modernc.org/sqlite v1.48.2 compiles with 32766, not 999, but the rewrite is correctness-first: O(1) binds per request regardless of list size.

`EXPLAIN QUERY PLAN` test (`TestInJsonEach_ExplainQueryPlan`) confirms the driver keeps `json_each` as a table-valued function reference in the plan output — no fallback to expanded parameters.

### IN-02 — empty __in sentinel (D-06)

`buildIn` returns `errEmptyIn` (sentinel) when `value == ""`. `ParseFilters` catches it via `errors.Is(err, errEmptyIn)` and returns `(nil, true, nil)`. Handler threads `emptyResult` into `QueryOptions.EmptyResult`. All 13 list closures in `registry_funcs.go` gain a first-statement guard:

```go
// Phase 69 IN-02: empty __in returns empty set per D-06.
if opts.EmptyResult {
    return []any{}, 0, nil
}
```

Zero SQL issued. Response envelope: `{"meta":{}, "data":[]}`.

## Step 0 grep output (W2 fix — raw-SQL spelling pre-check)

Before touching filter.go, the plan mandated a grep against `$(go env GOMODCACHE)/entgo.io/ent@v0.14.6/dialect/sql/` for `ExprP` / `P(` to confirm spelling and avoid try-and-fail compile cycles:

```
$ grep -rn "^func ExprP\|^func P(" "$(go env GOMODCACHE)"/entgo.io/ent@v0.14.6/dialect/sql/
/home/dotwaffle/go/pkg/mod/entgo.io/ent@v0.14.6/dialect/sql/builder.go:760:func P(fns ...func(*Builder)) *Predicate {
/home/dotwaffle/go/pkg/mod/entgo.io/ent@v0.14.6/dialect/sql/builder.go:767:func ExprP(exr string, args ...any) *Predicate {
```

Both symbols are available. **Chose `sql.ExprP`** — it accepts the expression string and `args ...any` directly, which fits the single-line `s.Where(sql.ExprP("... IN (SELECT value FROM json_each(?))", jsonStr))` pattern. `sql.P` requires writing a `func(*Builder)` callback which would be more verbose for this one-liner.

Code compiled on first try — no fallback loop.

## Empty-`__in` design rationale

**Decision: B — `QueryOptions.EmptyResult` flag + `errEmptyIn` sentinel** (rejected: A — `WHERE 1=0` sentinel predicate).

Why B over A:
- **Zero SQL execution**: A list query with `?asn__in=` never reaches ent; the 13 closures short-circuit before `client.Network.Query()` is constructed. One less SQLite round-trip on a pathological URL.
- **Matches Django semantics precisely**: `Model.objects.filter(id__in=[])` in upstream returns an empty `QuerySet` without hitting the DB. A WHERE 1=0 predicate is indistinguishable server-side but does generate a query.
- **Thread the signal, don't side-effect it**: `ParseFilters` returns `(preds, emptyResult, err)` — three values, all return-path. No mutation of a shared pointer or hidden flag; compilers catch every caller on the signature change.
- **Uniform exit**: Every list closure already returns `(nil, 0, err)` on count failure. Adding `return []any{}, 0, nil` as the first statement is surgical and keeps the pattern consistent.

B's cost is the signature change rippling through 4 callers (handler.go, filter_test.go, phase69_filter_test.go, fuzz_test.go). That's a mechanical edit, not architectural churn.

## 13 registry_funcs.go closures that received the EmptyResult guard

Every list closure now has this guard as the first statement after the `func(...) ([]any, int, error) {` opener:

1. `wireOrgFuncs` — Organization (line ~58)
2. `wireNetFuncs` — Network (line ~95)
3. `wireFacFuncs` — Facility (line ~132)
4. `wireIXFuncs` — InternetExchange (line ~169)
5. `wirePocFuncs` — Poc (line ~206)
6. `wireIXLanFuncs` — IxLan (line ~243)
7. `wireIXPfxFuncs` — IxPrefix (line ~284)
8. `wireNetIXLanFuncs` — NetworkIxLan (line ~321)
9. `wireNetFacFuncs` — NetworkFacility (line ~358)
10. `wireIXFacFuncs` — IxFacility (line ~395)
11. `wireCarrierFuncs` — Carrier (line ~432)
12. `wireCarrierFacFuncs` — CarrierFacility (line ~469)
13. `wireCampusFuncs` — Campus (line ~502)

Acceptance grep: `grep -c 'opts.EmptyResult' internal/pdbcompat/registry_funcs.go` → 13.

## Phase 68 regression safety

- `grep 'applyStatusMatrix' internal/pdbcompat/registry_funcs.go | wc -l` → 13 (Phase 68 status × since matrix intact on every list closure).
- `TestStatusMatrix` subtests (9) all pass — `list_no_since_returns_only_ok`, `list_with_since_non_campus_returns_ok_and_deleted`, `list_with_since_campus_includes_pending`, pk_ok/pending/deleted, `depth_on_list_is_silently_ignored`, `status_deleted_no_since_is_empty`, `limit_zero_returns_all_rows`.
- `TestPhase68StatusMatrix_Phase69Layering` (new) asserts that `?status=deleted&name__contains=foo` drops `status=` (Phase 68 removed it from Fields) and filters correctly through the Phase 69 folded column.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] setupTestHandler seed rows lacked name_fold values**
- **Found during:** Task 2 GREEN verification (`TestQueryFilterContains` failed — expected 2 cloud matches, got 0)
- **Issue:** `handler_test.go:setupTestHandler` seeds networks via `client.Network.Create().SetName(...)` without `.SetNameFold(...)`. With UNICODE-01 routing live, `name__contains=cloud` now goes through `name_fold` column which is empty string, so 0 rows matched. Production sync worker populates `name_fold` automatically (per Plan 69-03); tests that bypass sync must populate it manually.
- **Fix:** Added `.SetNameFold(unifold.Fold(name))` to the three seed `Network.Create()` calls in `setupTestHandler`. Added `unifold` import.
- **Files modified:** `internal/pdbcompat/handler_test.go`
- **Commit:** 9aa661d (rolled into the GREEN commit; the test-fixture bug and the feature change belong to the same atomic unit)

### Other deviations
None. The plan executed as written.

## Known Stubs
None.

## TDD Gate Compliance

- RED gate: commit 9839273 `test(69-04): ...` — 8 test functions / 13 subtests, 5 demonstrably failing against Phase 68 `filter.go` (shadow routing gaps + empty-`__in` returning 400).
- GREEN gate: commit 9aa661d `feat(69-04): ...` — all RED cases pass; the fixture-bug fix (Rule 1) is rolled into the same commit because it's inseparable from the feature.

No REFACTOR commit required — the implementation landed clean on first pass against the pinned ent source spelling.

## Acceptance evidence

Verification runs (from `TMPDIR=/tmp/claude-1000 go test -race ./internal/pdbcompat/... -count=1`):

- `TestShadowRouting_Network_NameFold`: 5 subtests PASS (ascii_lowercase_fold_matches_diacritic, ascii_titlecase_fold_matches_diacritic_operator_coerced, substring_across_word_boundary_via__fold, non-matching_substring_returns_empty, startswith_on_folded_column)
- `TestShadowRouting_Network_NonFoldedField`: PASS (website field without fold column still routes via FieldContainsFold)
- `TestInJsonEach_Large_Bypasses_SQLite_Limit`: PASS (1500 rows, 1500-element `?asn__in=...&limit=0` round-trip in 4.9s)
- `TestInJsonEach_EmptyString_ReturnsEmpty`: PASS (`?asn__in=` returns data array of length 0)
- `TestInJsonEach_StringValues`: PASS (`?name__in=alpha,gamma` returns 2 rows on 3-row seed)
- `TestInJsonEach_ExplainQueryPlan`: PASS (EXPLAIN QUERY PLAN output mentions `json_each` — driver does NOT expand the bind)
- `TestCoerce_OnlyContainsAndStartswith_Untouched`: 5 subtests PASS (asn__gt / asn__lt / info_unicast / asn__gte / asn__lte untouched, D-04 scope guard holds)
- `TestPhase68StatusMatrix_Phase69Layering`: PASS (`?status=deleted&name__contains=foo` drops status, folds contains, returns ok row only)
- `TestParseFilters`: 20 subtests PASS (new sig, new empty-__in subtest)
- `TestParseFiltersErrorPaths`: PASS (error paths on new 3-return sig)
- `TestBuildExactErrors` / `TestBuildContainsErrors` / `TestBuildStartsWithErrors` / `TestBuildInErrors`: PASS (new `folded bool` param threaded through all error-path assertions)
- `TestStatusMatrix` (9 Phase 68 subtests): PASS (regression guard intact)

Full suite (`go test -race ./...`): every package PASS. `go vet ./...`: clean. `golangci-lint run`: 0 issues.

Acceptance greps:
- `grep -c 'opts.EmptyResult' internal/pdbcompat/registry_funcs.go` → **13** (one guard per list closure)
- `grep -c 'json_each' internal/pdbcompat/filter.go` → **2** (1 SQL literal + 1 docstring reference)
- `grep -c 'unifold.Fold' internal/pdbcompat/filter.go` → **7** (buildContains, buildStartsWith, buildExact's folded branch, comments)
- `grep 'applyStatusMatrix' internal/pdbcompat/registry_funcs.go | wc -l` → **13** (Phase 68 matrix intact on every list closure)

## Commits

- `9839273` — test(69-04): add Phase 69 filter tests (RED)
- `9aa661d` — feat(69-04): pdbcompat filter — UNICODE-02 coercion + UNICODE-01 shadow routing + IN-01 json_each + IN-02 empty sentinel (GREEN, includes setupTestHandler fixture fix)

## Self-Check: PASSED

- `internal/pdbcompat/filter.go` — FOUND
- `internal/pdbcompat/filter_test.go` — FOUND
- `internal/pdbcompat/fuzz_test.go` — FOUND
- `internal/pdbcompat/handler.go` — FOUND
- `internal/pdbcompat/handler_test.go` — FOUND
- `internal/pdbcompat/registry.go` — FOUND
- `internal/pdbcompat/registry_funcs.go` — FOUND
- `internal/pdbcompat/phase69_filter_test.go` — FOUND
- commit 9839273 — FOUND (`git log --oneline -3 | grep 9839273`)
- commit 9aa661d — FOUND (`git log --oneline -3 | grep 9aa661d`)
