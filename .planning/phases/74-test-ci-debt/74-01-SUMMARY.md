---
phase: 74-test-ci-debt
plan: 01
subsystem: testing
tags: [test, codegen, schema, derivation-driven, allow-list-removal]

# Dependency graph
requires:
  - phase: 67-list-perf
    provides: "(-updated, -id) ORDER BY composite index that landed the always-on `updated` index in generateIndexes"
provides:
  - "Exported ExpectedIndexesFor(apiPath, ot) helper as the test-side mirror of generateIndexes"
  - "Derivation-driven TestGenerateIndexes that loads schema/peeringdb.json and asserts every emitted index is structurally valid (real field, always-on synthetic, or documented apiPath special-case)"
  - "Strict-ascending + dedup invariant locked as a test assertion"
affects: [future schema additions, FK-field additions, index-rule changes, drift-gate authors]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "derivation-driven test pattern: assertion source = code source (eliminates allow-list rot)"

key-files:
  created: []
  modified:
    - cmd/pdb-schema-generate/main.go
    - cmd/pdb-schema-generate/main_test.go

key-decisions:
  - "ExpectedIndexesFor implemented as a thin wrapper around generateIndexes (not a parallel rule re-implementation) — drift protection comes from the structural sanity loop, not from re-deriving the same rules in two places"
  - "Strict-ascending duplicate-detection used in place of a sort+dedup re-check (cheaper and grep-friendlier)"
  - "Loop-variable shadowing (`apiPath, ot := apiPath, ot`) omitted vs. plan snippet — Go 1.22+ made loop variables per-iteration, project pins Go 1.26+, the shadow is dead code"

patterns-established:
  - "Derivation-driven codegen test: when a generator and its expected output are both derivable from the same source, write the test to load the source and compare actual emit vs. derived expectation per-input. Allow-lists rot; sources don't."
  - "Structural sanity loop: when the rule-set is small and stable, factor the rules into the assertion (alwaysOn synthetics + apiPathSpecials maps) so a future PR adding a new rule must touch both the generator and the test (intentional friction)."

requirements-completed: [TEST-01]

# Metrics
duration: ~10min
completed: 2026-04-26
---

# Phase 74 Plan 01: Schema-derived TestGenerateIndexes Summary

**Replaced the hand-maintained allow-list in `cmd/pdb-schema-generate/TestGenerateIndexes` with a per-entity derivation-driven assertion that loads `schema/peeringdb.json` and structurally validates every index `generateIndexes` emits.**

## Performance

- **Duration:** ~10 min
- **Started:** 2026-04-26T22:24:00Z (approx — first edit timestamp)
- **Completed:** 2026-04-26T22:34:11Z
- **Tasks:** 2 (both with TDD RED→GREEN gates)
- **Files modified:** 2

## Accomplishments

- Eliminated the "test-rot from schema evolution" failure class — the same one that bit Phase 67 when `"updated"` was added to the indexes and the allow-list silently drifted until 260426-pms patched it post-hoc.
- Per-entity sub-tests now run for every `ObjectType` in `schema/peeringdb.json` (13 entities: org, fac, net, ix, carrier, campus, ixfac, carrierfac, netixlan, netfac, poc, ixpfx, ixlan).
- Locked the strict-ascending + dedup invariant on `generateIndexes` output as a test assertion.
- Added `ExpectedIndexesFor` as the public test-side mirror of `generateIndexes` so the contract "expected = what the generator emits given the same input" is grep-able, not implicit.

## Task Commits

Each task was committed atomically following TDD RED→GREEN:

1. **Task 1 RED — Failing test for ExpectedIndexesFor helper** — `0d8fa18` (test)
2. **Task 1 GREEN — Export ExpectedIndexesFor wrapper** — `45a7a5d` (feat)
3. **Task 2 — Rewrite TestGenerateIndexes derivation-driven** — `6de3c94` (test)

_Task 1 had no separate REFACTOR — the wrapper is already minimal._
_Task 2 RED was the build-fail induced by `slicesEqual` not yet existing; the new test body and helper landed in the same commit per plan instruction (the `wantIndexes:=map[string]bool` allow-list pattern was the structural pre-existing failure mode being torn out)._

**Plan metadata commit:** appended below by the trailing SUMMARY.md commit.

## Files Created/Modified

- `cmd/pdb-schema-generate/main.go` — added `ExpectedIndexesFor(apiPath string, ot ObjectType) []string` (18 LOC), thin wrapper around `generateIndexes`, with rationale comment citing Phase 74 D-01.
- `cmd/pdb-schema-generate/main_test.go` — replaced 32-line allow-list-driven `TestGenerateIndexes` with a 95-line derivation-driven body containing `legacy_net_fixture` and `per_entity_from_schema_source` sub-tests; added local `slicesEqual` helper (12 LOC, no existing helper found via `grep -rn 'func slicesEqual' cmd/pdb-schema-generate/`).

## Why the trivial `ExpectedIndexesFor` wrapper is the right design choice

Plan output spec asked for explicit confirmation of this rationale:

The naive read of "auto-derive expected indexes from schema source" is "re-implement `generateIndexes` in the test, line-for-line". That is the wrong instinct here. Two parallel implementations of the same rule-set are guaranteed to drift; the test then either (a) catches its own drift instead of the generator's drift (false positive), or (b) silently agrees with itself even when the generator has a real bug (false negative).

The right design is: **the test asserts what we can verify structurally, not what we can re-derive procedurally**. The structural assertions are:

1. Every emitted index references a real field in `ot.Fields`, OR is one of the documented always-on synthetics (`status`, `updated`), OR is one of the documented `apiPath` special-cases (`net→asn`, `ixpfx→prefix`, `poc→role`).
2. The slice is strictly ascending (sorted + deduped invariant).
3. The legacy minimal "net" fixture still produces exactly `[asn, name, org_id, status, updated]` (pin against accidental rule deletions).

These three checks catch every realistic generator bug — typo'd field names, accidental duplicates, indexes on dropped fields, sort-order regressions — without re-deriving the rules. `ExpectedIndexesFor` exists as the contract-level identity (`expected == generated for same input`), and the structural loop is what makes that identity meaningful. If a future maintainer adds a 4th `apiPath` special-case to `generateIndexes` without updating the `apiPathSpecials` map in the test, the test fails on that entity — exactly the intentional friction that keeps the two halves of the contract in sync.

## Decisions Made

- **Wrapper over re-implementation** (see above section).
- **Strict-ascending check vs. sort+dedup re-derivation**: chose `for i := 1; i < len(got); i++ { if got[i-1] >= got[i] ...}` — `>=` catches both unsorted AND duplicate cases in one pass, vs. running `slices.Sort` + dedup on a copy and comparing. Cheaper, more grep-friendly, identical guarantee.
- **Loop-variable shadow omitted**: the plan snippet had `apiPath, ot := apiPath, ot` for closure capture safety. Go 1.22+ made loop variables per-iteration; project pins Go 1.26+. The shadow is dead code and would be flagged by `gocritic` / `revive`. Confirmed `golangci-lint run ./cmd/pdb-schema-generate/...` returns `0 issues.` without it.

## Deviations from Plan

**None.** Plan executed exactly as written, with the documented Go 1.26 idiom adjustment noted above (loop variable shadow omission — well within the planner's "use the existing one and drop the local copy if it exists" spirit when applied to language-level idioms).

## Issues Encountered

- **Parallel `golangci-lint` contention** with another worktree executor: first lint invocation hit "parallel golangci-lint is running" error. Resolved by waiting for the other run to finish and re-invoking — the lint result was clean (`0 issues.`). No code change required.

## Acceptance Criteria — All Confirmed

| Criterion | Result |
|---|---|
| `grep -c '^func ExpectedIndexesFor(' cmd/pdb-schema-generate/main.go` | `1` ✅ |
| `grep -c 'Phase 74 D-01' cmd/pdb-schema-generate/main.go` | `1` ✅ |
| `grep -c 'Phase 74 D-01' cmd/pdb-schema-generate/main_test.go` | `1` ✅ |
| `grep -c 'per_entity_from_schema_source' cmd/pdb-schema-generate/main_test.go` | `1` ✅ |
| `grep -c 'legacy_net_fixture' cmd/pdb-schema-generate/main_test.go` | `1` ✅ |
| `grep -c 'wantIndexes\s*:=\s*map\[string\]bool' cmd/pdb-schema-generate/main_test.go` | `0` ✅ (allow-list pattern gone) |
| `go test -count=1 ./cmd/pdb-schema-generate/...` | `ok 0.014s` ✅ |
| `go vet ./cmd/pdb-schema-generate/...` | clean ✅ |
| `golangci-lint run ./cmd/pdb-schema-generate/...` | `0 issues.` ✅ |
| Per-entity sub-tests ran | 13 entities ✅ (verified via `-v` output) |
| Drift gate consideration | unaffected — only test code + non-templated exported helper modified |

## Follow-on Candidates

Surfaced during execution — not in plan scope, but worth flagging for a future quick task or phase 74 plan:

1. **`cmd/pdb-schema-generate/TestLoadSchema`** uses a local fixture rather than driving from `schema/peeringdb.json`. Same risk class as the issue this plan fixed — could grow a fixture-vs-real-schema drift over time. Lower priority since `loadSchema` is much simpler than `generateIndexes`.
2. **`cmd/pdb-schema-generate/TestGenerateTypesFile`** does string-contains matching against generated output (`"socialMediaSchema()"`, `"ent/schematypes"`). Not the same allow-list pattern but does have a similar fragility — a generator change that renames `socialMediaSchema` would break the test in a "magic string" way rather than a structural way. Consider parsing the output via `go/parser` and asserting on AST shape instead. Out of scope for Phase 74 D-01 closure.
3. **No similar pattern was found in the wider repo** via `grep -rn 'wantIndexes\s*:=\s*map\[string\]bool'` (returned zero matches outside this file).

## Next Phase Readiness

- TEST-01 closed. Phase 74 plans 02 (TEST-02 dashboard region variable) and 03 (TEST-03 visbaseline lint) remain independent and unblocked.
- CI drift gate green (no codegen inputs touched).
- Repo lint clean for `cmd/pdb-schema-generate/...`.

## Self-Check: PASSED

- `cmd/pdb-schema-generate/main.go` modified — `ExpectedIndexesFor` present at expected location ✅ (verified via `grep -c '^func ExpectedIndexesFor('`)
- `cmd/pdb-schema-generate/main_test.go` modified — derivation-driven body + sub-tests present ✅
- Commit `0d8fa18` (RED test) — present in `git log` ✅
- Commit `45a7a5d` (GREEN feat) — present in `git log` ✅
- Commit `6de3c94` (rewrite test) — present in `git log` ✅
- All acceptance criteria pass ✅
- All verification commands pass ✅

---
*Phase: 74-test-ci-debt*
*Completed: 2026-04-26*
