---
phase: 69-unicode-operator-in-robustness
reviewed: 2026-04-19T00:00:00Z
depth: standard
files_reviewed: 19
files_reviewed_list:
  - internal/unifold/unifold.go
  - internal/unifold/unifold_test.go
  - ent/schema/network.go
  - ent/schema/facility.go
  - ent/schema/internetexchange.go
  - ent/schema/organization.go
  - ent/schema/campus.go
  - ent/schema/carrier.go
  - internal/sync/upsert.go
  - internal/sync/upsert_test.go
  - internal/pdbcompat/filter.go
  - internal/pdbcompat/filter_test.go
  - internal/pdbcompat/registry.go
  - internal/pdbcompat/handler.go
  - internal/pdbcompat/registry_funcs.go
  - internal/pdbcompat/phase69_filter_test.go
  - internal/pdbcompat/fuzz_test.go
  - internal/pdbcompat/filter_bench.go
  - internal/pdbcompat/bench_test.go
findings:
  critical: 0
  warning: 0
  info: 3
  total: 3
status: findings_found
---

# Phase 69: Code Review Report

**Reviewed:** 2026-04-19
**Depth:** standard
**Files Reviewed:** 19
**Status:** findings_found (3 INFO only — no HIGH/MEDIUM/LOW blocking issues)

## Summary

Phase 69 (Unicode folding, operator coercion, `__in` robustness) is clean on
every focus area the context requested. No Critical or Warning findings.

**Focus-area verification (all passed):**

1. **SQL injection surface** — `internal/pdbcompat/filter.go:264` composed
   expression `s.C(field)+" IN (SELECT value FROM json_each(?))"` is safe.
   `field` is validated against the static `tc.Fields` registry map at
   `ParseFilters` `filter.go:95` before reaching `buildIn`; keys in the
   Registry are hard-coded literals in `registry.go`. `s.C(field)` quotes
   the identifier via ent's `Builder.Ident` (entgo.io/ent v0.14.6
   `dialect/sql/builder.go:2241`). The JSON payload binds via a single `?`
   placeholder (`sql.ExprP`) with `json.Marshal`-produced UTF-8. No
   user-controlled input reaches the SQL string concatenation. Matches the
   T-69-04-01 threat-model mitigation declared in-code.

2. **Privacy/visibility — 5-surface _fold leakage check** — All 16 `*_fold`
   shadow columns across 6 ent schemas carry `entgql.Skip(entgql.SkipAll)` and
   `entrest.WithSkip(true)` annotations.
   - `ent/rest/openapi.json`: 0 occurrences of "fold"
   - `graph/schema.graphql`: 0 occurrences of "fold"
   - `gen/peeringdb/v1/**` and `proto/**`: 0 occurrences (proto frozen via
     `entproto.SkipGenFile` per ent/entc.go)
   - `internal/pdbcompat/serializer.go`: 0 references to `Fold` — pdbcompat
     uses explicit field-by-field serialization, cannot leak via reflection
   - `internal/web/**`: 0 references to `*Fold`

3. **Concurrency** — No new package-level mutable production state.
   `Registry` (`registry.go:85`) is written only via `setFuncs`
   (`registry_funcs.go:31-36`) from `init()`; no runtime mutation. The
   bench-path driver-registration toggle lives entirely under `//go:build
   bench` in `filter_bench.go` and `bench_test.go` — never compiled into
   production (`go build ./...`). `benchDriverOnce sync.Once` + pre-check
   for existing "sqlite3" driver (`bench_test.go:38-46`) correctly guards
   duplicate-register panics. No new goroutines.

4. **Error handling — `buildIn` parseErr/err shadow (W1)** — `filter.go:245`
   uses `parseErr` rather than re-declaring `err` inside the int-conversion
   loop. The inline comment on `filter.go:243-244` explicitly records the
   W1 fix rationale. Error propagation correct: loop error returns
   immediately; outer `marshalErr` captured from `json.Marshal` and checked
   at `filter.go:255`. No shadow/drop.

5. **Unicode edge cases** — `unifold.Fold` is total (no panics):
   - Invalid UTF-8 bytes (`\xff`, `\xfe`): `for _, r := range s` yields
     `utf8.RuneError` (U+FFFD) per Go spec; NFKD preserves U+FFFD, not an
     Mn, ToLower is identity. Returns 3-byte UTF-8 encoding of U+FFFD.
   - Null bytes (`\x00`): fast path accepts (below 0x80, not A-Z); full
     path: NFKD identity, not Mn, ToLower identity. Consistent.
   - 70 KB input (`strings.Repeat("A", 70_000)`): `strings.Builder.Grow(len(s))`
     pre-allocates; capital A hits the slow path; NFKD of ASCII is a no-op
     but allocates a copy. ~140 KB peak for a 70 KB input — acceptable;
     not in a hot loop (sync upsert happens 13× per hour).
   - Zalgo (1000 combining marks on one base): Mn guard strips all 1000;
     output is 1-byte 'a'. Memory bounded by input.
   - Fast-path ⟷ slow-path consistency: verified by inspection — any
     input that passes `asciiLowerFastPath` would be a no-op through NFKD
     → Mn-drop → ToLower.

6. **Fuzz corpus scope (Plan 05 D-07)** — `fuzz_test.go:41-69` covers all
   declared categories:
   - Diacritics (Zürich, Straße), CJK (日本語, 中文), Hangul (한글),
     RTL (עברית), combining marks (e + U+0301, a + U+0308),
     null+invalid UTF-8, RLO/LRO (U+202E/U+202D), ZWJ (U+200D).
   - IN-01/IN-02 edges: empty value, 1200-element list (>999-var SQLite
     cap), string IN with unicode, all-empty IN parts (`,,,`), 1000 empty
     strings.
   - Long-string stress: 70 KB "x" repeat, 5000 zalgo repeats.
   TypeConfig marks `name` as folded so both the UNICODE-01 shadow-route and
   non-shadow paths are exercised. Seed corpus is comprehensive.

7. **Phase 68 status × since matrix regression risk** — All 13 list closures
   in `registry_funcs.go` append `applyStatusMatrix(isCampus, opts.Since !=
   nil)` to `preds` (verified at lines 68, 105, 142, 179, 216, 253, 290,
   327, 364, 401, 438, 475, 513 — Campus correctly sets `isCampus=true`,
   all others false). Every closure also short-circuits on `opts.EmptyResult`
   BEFORE building `preds`, which is the correct ordering because empty-set
   ∧ status-matrix = empty-set.

## Info

### IN-01: unused range variable suppression in TestFold_NoPanic

**File:** `internal/unifold/unifold_test.go:75-81`
**Issue:** The loop body writes `got := Fold(in)` then `_ = got` and `_ = i`,
neither of which is necessary. Go permits unused range variables, and the
test contract (no panic) doesn't require retaining the result.
**Fix:**
```go
for _, in := range inputs {
    // Contract: Fold must not panic on any input. We do not assert
    // the output value — see TestFold for behavioural cases.
    _ = Fold(in)
}
```
**Severity rationale:** cosmetic; test correctness unaffected.

### IN-02: asciiLowerFastPath doc-comment imprecision

**File:** `internal/unifold/unifold.go:96-102`
**Issue:** The function name and the first sentence of the doc-comment
("every byte of s is in the ASCII range and not an upper-case letter")
describe the check correctly, but the adjacent "Fast-path contract" prose
in `Fold` (`unifold.go:54-59`) says "already pure ASCII lower-case" — this
is slightly misleading because the predicate also accepts digits,
punctuation, and control chars (any `[0x00, 0x40] ∪ [0x5B, 0x7F] ∪
'a'-'z'`). The behaviour is correct — Fold is idempotent on this set —
but a reader comparing the Fold doc-block against the predicate may
notice the inconsistency.
**Fix:** Tighten the Fold doc-block on line 55 to match the predicate:
```go
// Fast-path contract: an input whose bytes are all in the closed ranges
// [0x00, 0x40] ∪ [0x5B, 0x7F] ∪ ['a', 'z'] (i.e. pure ASCII excluding
// capital letters) is returned unchanged without allocating.
```
**Severity rationale:** doc-only; no behavioural change.

### IN-03: TestPhase68StatusMatrix_Phase69Layering covers status=ok only

**File:** `internal/pdbcompat/phase69_filter_test.go:345-373`
**Issue:** The test name advertises coverage of the "Phase 68 status × since
matrix × Phase 69 filter layering" but exercises only the `status=ok`
matrix branch (no `since` param). The matrix's other branch —
`status IN ('ok','deleted')` when `?since=N` is set — is not asserted
alongside the Phase 69 shadow-routing path. A future regression that
breaks shadow routing under `?since=` would not be caught by this test.
**Fix:** Add a sub-test case with both `?since=<epoch>` and `?name__contains=foo`
that asserts both the "ok" and "deleted" rows return (shadow routing must
still fold the name filter on both). Example:
```go
// Phase 68 status+since × Phase 69 shadow routing
since := strconv.FormatInt(now.Add(-time.Hour).Unix(), 10)
ids := phase69FetchIDs(t, srv.URL+"/api/net?since="+since+"&name__contains=foo")
if !sameIDs(ids, []int{1, 2}) {
    t.Errorf("since+name__contains: got %v, want [1 2] (ok + deleted)", ids)
}
```
**Severity rationale:** test-coverage gap; no production bug. Existing
`status_matrix_test.go` still asserts the Phase 68 invariant independently.

---

_Reviewed: 2026-04-19_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
