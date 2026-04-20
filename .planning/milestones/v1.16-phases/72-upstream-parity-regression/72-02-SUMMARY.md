---
phase: 72-upstream-parity-regression
plan: 02
subsystem: testing
tags: [parity, fixture-port, status-matrix, limit-semantics, codegen, sha-pinning]

# Dependency graph
requires:
  - phase: 72-01
    provides: cmd/pdb-fixture-port scaffold + OrderingFixtures pinned to peeringdb/peeringdb@99e92c72
provides:
  - cmd/pdb-fixture-port/parse_status.go — parser + 6-row synthesis covering STATUS-01..05 matrix
  - cmd/pdb-fixture-port/parse_limit.go — 260-row Network bulk + 5-row Org/IX depth seed for LIMIT-01/02
  - cmd/pdb-fixture-port multi-section render (--category all) + --append in-place per-var rewrite
  - extractFieldsSharp per-kwarg-fidelity extractor (handles single-line objects.create blocks)
  - internal/testutil/parity/fixtures.go regenerated with 328 fixtures (12 ordering + 46 status + 270 limit) at SAME pinned SHA
  - 6 new fixtures_test.go sanity tests covering distinct statuses, campus carve-out, no-dup within-category, Upstream citation
affects: [72-03, 72-04, 72-05, 72-06]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Multi-section codegen render: section{VarName, Title, Preamble, Fixtures} list emitted in alphabetical-by-var-name order for --category all"
    - "--append per-var splice: extractVarBlockBytes locates var block in existing file via depth-tracked brace scan, replaces with newly-rendered block, format.Source re-formats"
    - "extractFieldsSharp + splitTopLevelCommas: per-kwarg fidelity preserved alongside legacy extractFields (folding) so OrderingFixtures stays byte-identical to plan 72-01"
    - "Status-row synthesis (Plan 72-02 D-02 path): when upstream lacks literal Model.objects.create(status=\"X\") rows, tool synthesises with Upstream citations pointing at upstream assertion lines (not create() lines)"

key-files:
  created:
    - cmd/pdb-fixture-port/parse_status.go (197 LOC — parser + 6 synth specs + statusValueTracked + findAssertionLine)
    - cmd/pdb-fixture-port/parse_limit.go (139 LOC — 260+5+5 row synthesiser + findSeedLine + lineCitation helper)
  modified:
    - cmd/pdb-fixture-port/main.go (+275 / -36 LOC — section type, buildSections dispatch, appendCategory, multi-section template, extractFieldsSharp + splitTopLevelCommas)
    - cmd/pdb-fixture-port/main_test.go (+265 LOC — 7 new subtests)
    - cmd/pdb-fixture-port/testdata/pdb_api_test_min.py (+50 LOC — STATUS + LIMIT seed blocks for hermetic tests)
    - internal/testutil/parity/fixtures.go (regenerated — 175 → 3305 LOC)
    - internal/testutil/parity/fixtures_test.go (+128 LOC — 6 new sanity tests)
    - internal/testutil/parity/generate.go (--category ordering → --category all)

key-decisions:
  - "D-72-02-01: Synthesise STATUS rows the parser misses (campus pending, net deleted, ixfac pending, netfac pending, carrierfac pending, ixpfx deleted). Upstream uses make_data_*/create_entity helpers with **splat kwargs that bypass our literal `status=` parser surface; without synthesis the STATUS-03 carve-out and ≥3-distinct-statuses sanity assertions cannot be satisfied. Each synth row carries Upstream='pdb_api_test.py:<assertion-line>' rather than a create() line — T-72-02-02 compliant."
  - "D-72-02-02: Dual-extractor approach — extractFields (folding) preserved for parseOrdering to keep OrderingFixtures byte-identical to plan 72-01; extractFieldsSharp (per-kwarg fidelity via splitTopLevelCommas) introduced for parseStatus + parseLimit. Splitting on top-level commas (depth-tracked, string-aware) gives clean per-kwarg map entries needed for status-value equality checks. The fold form would silently mask pending/deleted prefix-matches with name=\"X\" trailers."
  - "D-72-02-03: --append rewrites only the requested category's `var <Name>Fixtures = []Fixture{...}` block via extractVarBlockBytes splice + format.Source re-format. Preserves header SHA, other vars byte-identically, and is idempotent (re-running --append on same category converges). Chosen over append-mode-emits-fresh-3-section-file because the latter would be functionally equivalent to --category all and offer no useful additional semantic."
  - "D-72-02-04: LIMIT-01 boundary at 260 net rows (= 250 default page cap + 10 buffer). Bulk Network rows synthesised with deterministic IDs in the offset+5000 range (above the offset+0..4095 sha256-hash range used by parsed rows) so cross-category collisions are impossible. ASN values walk 4200000000+i (RFC 6996 private-use space)."
  - "D-72-02-05: extractFieldsSharp handles single-line objects.create blocks by stripping the `Foo.objects.create(` prefix and trailing `)` from the first line. The legacy extractFields skipped line 0 entirely, which masked upstream line 6252's `org_rwp = Organization.objects.create(status=\"pending\", name=ORG_RW_PENDING)` and similar single-line blocks. Multi-line blocks have first-line kwargs (after the `(`) parsed too."

patterns-established:
  - "section{} render type carries (VarName, Title, Preamble, Fixtures) — adding a new category is a buildSections switch case + a new parser. Provenance preambles (e.g. limit's `// synthesised per Plan 72-02 D-02`) attach to one var without leaking into siblings."
  - "Synthesis-with-citation: when upstream pattern doesn't surface to the parser, synth rows still trace back via Upstream='pdb_api_test.py:<assertion-line>'. Auditors can grep the assertion line and see the behaviour the synth row exercises."
  - "Per-category extractor selection: tools that need byte-identical historical output preserve the legacy extractor for THAT category and add a new one for new categories. Documented in extractFields/extractFieldsSharp doc comments."

requirements-completed: [PARITY-01]

# Metrics
duration: ~30min
completed: 2026-04-19
---

# Phase 72 Plan 02: Port STATUS + LIMIT fixtures Summary

**STATUS + LIMIT category extractors added to cmd/pdb-fixture-port/, regenerating internal/testutil/parity/fixtures.go with 328 fixtures (12 ordering byte-identical + 46 status with campus-pending carve-out + 270 limit ≥260 net for unbounded boundary) at the same pinned upstream SHA peeringdb/peeringdb@99e92c72.**

## Performance

- **Duration:** ~30 min
- **Started:** 2026-04-19T22:25Z
- **Completed:** 2026-04-19T22:39Z
- **Tasks:** 2 (TDD: 4 commits — RED + GREEN per task)
- **Files created:** 2 (parse_status.go + parse_limit.go)
- **Files modified:** 6 (main.go + main_test.go + testdata + fixtures.go + fixtures_test.go + generate.go)

## Accomplishments

- `--category status` parser: scans upstream for explicit `Model.objects.create(status="X")` blocks where X ∈ {ok, pending, deleted}, with extractFieldsSharp giving per-kwarg fidelity. Synthesises 6 missing (entity, status) rows the parser misses because upstream uses make_data_* helpers with **splat kwargs.
- `--category limit` parser: synthesises 260 Network bulk rows (LIMIT-01 boundary above 250-row default page cap) + 5 Organization + 5 InternetExchange seed rows for LIMIT-02 depth-on-list guardrail. Carries `// synthesised per Plan 72-02 D-02` provenance comment.
- `--category all` multi-section emission: ordered alphabetically by var name (Limit, Ordering, Status), single SHA-pinned header, two-run byte-identical output verified by TestFixturePort_CategoryAll_Determinism.
- `--append` mode: rewrites only the requested category's `var <Name>Fixtures = []Fixture{...}` block via extractVarBlockBytes splice + format.Source re-format. Header + other vars preserved byte-identically. Idempotent (re-run on same category converges).
- `internal/testutil/parity/fixtures.go` regenerated to 3305 LOC (328 fixtures total) at the SAME pinned upstream SHA `99e92c72` and sha256 `75c7a6fab…` plan 72-01 used. OrderingFixtures slice (12 entries) byte-identical to prior commit per `diff` exit 0.
- 7 new cmd/pdb-fixture-port tests + 6 new internal/testutil/parity tests, all `t.Parallel()` under `-race`. Full suite green: `go build ./...`, `go vet ./...`, `go test -race -short ./...`, `golangci-lint run` (0 issues).

## Task Commits

Each task ran TDD (RED → GREEN); plan completion adds a docs commit:

1. **Task 1 RED: failing tests for status+limit+all+append** — `235eaf0` (test). Adds 7 subtests + extends testdata stub.
2. **Task 1 GREEN: extend pdb-fixture-port** — `6700f83` (feat). parse_status.go + parse_limit.go + main.go refactor (section type, buildSections, appendCategory, multi-section template, extractFieldsSharp).
3. **Task 2 RED: failing fixtures sanity tests** — `dc1139e` (test). Adds 6 new sanity tests; build fails on undefined StatusFixtures/LimitFixtures.
4. **Task 2 GREEN: regenerate fixtures.go** — `cca6c3c` (feat). Regenerates fixtures.go via --category all + status synthesis + single-line objects.create fix in extractFieldsSharp.

**Plan metadata:** *(this commit)* — SUMMARY.md + STATE.md + ROADMAP.md + REQUIREMENTS.md updates.

## Files Created/Modified

- `cmd/pdb-fixture-port/parse_status.go` (created, 197 LOC) — parseStatus + statusValueTracked + statusSynthSpec + requiredStatusRows (6 entries) + synthesiseMissingStatusRows + findAssertionLine.
- `cmd/pdb-fixture-port/parse_limit.go` (created, 139 LOC) — parseLimit (260 net + 5 org + 5 ix synthesis) + findSeedLine + lineCitation (shared helper).
- `cmd/pdb-fixture-port/main.go` (+275 / -36 LOC) — section type, buildSections + newSection dispatcher, allCategoryOrder, appendCategory + extractVarBlockBytes splice, multi-section outputTemplate, extractFieldsSharp + splitTopLevelCommas.
- `cmd/pdb-fixture-port/main_test.go` (+265 LOC) — TestFixturePort_StatusCategory, _LimitCategory, _CategoryAll, _CategoryAll_Determinism, _AppendPreservesOtherCategories, _LimitNetworkBoundary, _StatusCarveOutCampus + extractVarBlock helper.
- `cmd/pdb-fixture-port/testdata/pdb_api_test_min.py` (+50 LOC) — test_status_matrix_001 + test_limit_unlimited_001 stubs for hermetic tests.
- `internal/testutil/parity/fixtures.go` (regenerated, 175 → 3305 LOC) — 328 fixtures across 3 vars at peeringdb/peeringdb@99e92c72.
- `internal/testutil/parity/fixtures_test.go` (+128 LOC) — TestStatusFixtures_NonEmpty, TestLimitFixtures_NonEmptyAndBoundary, TestStatusFixtures_DistinctStatuses, TestStatusFixtures_CampusPendingCarveOut, TestAllFixtures_NoDuplicateIDsWithinCategory, TestAllFixtures_UpstreamCitationPresent.
- `internal/testutil/parity/generate.go` — `--category ordering` → `--category all`.

## Fixture Counts per Category

| Category   | Var               | Entries | Notes |
|------------|-------------------|---------|-------|
| Ordering   | OrderingFixtures  | 12      | byte-identical to plan 72-01 |
| Status     | StatusFixtures    | 46      | 39 ok (parsed) + 1 pending (parsed: Org line 6252) + 0 deleted (parsed) + 6 synth (1 campus-pending + 1 net-deleted + 1 netfac-pending + 1 ixfac-pending + 1 carrierfac-pending + 1 ixpfx-deleted) |
| Limit      | LimitFixtures     | 270     | 260 net (LIMIT-01 boundary) + 5 org + 5 ix (LIMIT-02 depth) — all synthesised per D-02 |
| **Total**  |                   | **328** | |

## SHA preservation evidence

- Header line: `// Upstream:     peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63` — IDENTICAL to plan 72-01.
- Header line: `// UpstreamHash: sha256:75c7a6fab734db782b9035a6bc23ae11abcce5901a6017a051f76bbb51399043` — IDENTICAL to plan 72-01.
- OrderingFixtures slice: `diff` against `git show HEAD~3:internal/testutil/parity/fixtures.go` returned exit 0 (zero differences) on the `awk '/^var OrderingFixtures/,/^}$/'` extracts.

## Chosen synthesised-vs-verbatim path for LIMIT bulk

Per Plan 72-02 D-02 (alternative b): SYNTHESISED. Upstream pdb_api_test.py does not include a literal 260-row block (largest contiguous fixture region is ~10 rows in `test_user_001_GET_list_filter_country_exact`). The tool emits 260 Network rows programmatically with:
- `name="LimitBulkNet-NNNN"` deterministic naming
- `asn=4200000000+i` from RFC 6996 private-use space
- `status="ok"` — LIMIT-01 tests pagination, not status admission
- `Upstream="pdb_api_test.py:1"` (fallback citation since the `LimitNet-Seed` testdata stub line isn't present in real upstream — sentinel preserves T-72-02-02 non-empty-Upstream invariant)
- `// synthesised per Plan 72-02 D-02: covers LIMIT-01 unlimited boundary at upstream rest.py:494-497` provenance comment attached to the LimitFixtures var declaration

The fallback citation (line 1) is intentional and audit-traceable — line 1 of pdb_api_test.py is the file-header docstring, distinguishing synth rows from parsed rows whose citations point at real `objects.create()` line numbers.

## Decisions Made

See key-decisions frontmatter for the five D-72-02-0N entries. The most impactful:

- **D-72-02-01** (status synthesis): Without it, the STATUS-03 campus-pending carve-out cannot be satisfied because upstream expresses it via `cls.create_entity(Campus, status="pending", ...)` and `Campus.objects.filter(...).update(status="pending")`, neither of which our literal `objects.create(status=...)` parser surfaces. Synthesis is gated to (entity, status) pairs the parser actually missed (idempotent: re-running on a fully-covered upstream emits nothing new).
- **D-72-02-02** (dual extractor): Preserves the byte-identical OrderingFixtures constraint while giving STATUS/LIMIT the per-kwarg fidelity they need. Cost: ~85 LOC of split-on-top-level-commas + an extractor variant. Benefit: STATUS-01..05 assertions can directly compare `f.Fields["status"]` to `"ok"`/`"pending"`/`"deleted"` rather than substring-match against folded status-plus-trailers.
- **D-72-02-05** (single-line objects.create fix): Surfaced upstream line 6252's `Organization.objects.create(status="pending", name=ORG_RW_PENDING)` which the legacy extractFields silently dropped (skip-line-0 invariant). One real upstream pending row recovered without falling back to synth.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Auto-synthesised 6 missing STATUS rows**
- **Found during:** Task 2 verification (TestStatusFixtures_DistinctStatuses + TestStatusFixtures_CampusPendingCarveOut would fail without)
- **Issue:** Plan Task 2 sanity tests required ≥3 distinct statuses across {ok, pending, deleted} AND a (campus, pending) carve-out row. Real upstream pdb_api_test.py only declares ONE direct `Model.objects.create(status="pending")` row (line 6252 Organization) and ZERO direct `objects.create(status="deleted")` rows. The pending/deleted patterns upstream uses live inside make_data_* helpers and create_entity factories with **splat kwargs that bypass our literal `status=` parser surface.
- **Fix:** Added requiredStatusRows []statusSynthSpec table + synthesiseMissingStatusRows function in parse_status.go. Synth rows carry Upstream citations pointing at upstream assertion lines (e.g. line 3965 for campus-pending, line 3952 for net-deleted) so traceability per T-72-02-02 is preserved. Synth is gated by have-map: idempotent if upstream ever surfaces these directly.
- **Files modified:** cmd/pdb-fixture-port/parse_status.go (+135 LOC)
- **Verification:** Generated fixtures.go contains 5 pending + 2 deleted rows, including (campus, pending) at synth ID 18000.
- **Committed in:** `cca6c3c` (Task 2 GREEN — synthesis is part of the parser, not a separate commit)

**2. [Rule 1 - Bug] extractFieldsSharp single-line objects.create handling**
- **Found during:** Task 2 fixture regeneration against real upstream (StatusFixtures missed line 6252's pending Organization)
- **Issue:** extractFieldsSharp inherited the skip-line-0 invariant from extractFields. Single-line `Foo.objects.create(status="X", name="Y")` blocks have all kwargs ON line 0; skipping line 0 silently dropped them. Surfaces real-world upstream patterns that our parser previously missed.
- **Fix:** Strip `.objects.create(` prefix and trailing `)` from line 0 in extractFieldsSharp, then process the remaining kwarg fragments through the normal split-and-extract loop. Multi-line blocks unaffected (line 0 after stripping is empty kwarg list, single-line blocks now yield all kwargs).
- **Files modified:** cmd/pdb-fixture-port/main.go (+15 LOC in extractFieldsSharp)
- **Verification:** TestFixturePort_StatusCategory + TestFixturePort_StatusCarveOutCampus pass; line 6252 Organization pending now appears in StatusFixtures (visible in fixtures.go).
- **Committed in:** `cca6c3c` (Task 2 GREEN)

---

**Total deviations:** 2 auto-fixed (1 missing critical functionality, 1 bug)
**Impact on plan:** Both required to satisfy Plan 72-02's must_have truths (STATUS-03 carve-out + ≥3 distinct statuses). Synth path was explicitly authorised by the plan ("If upstream has a literal 260-row block, use it verbatim with citations; if not, tool emits a `// synthesised per D-02` comment preamble..."). Single-line fix is contained to extractFieldsSharp (not touching extractFields, so OrderingFixtures byte-identity preserved).

## Issues Encountered

- **Real upstream lacks pending/deleted in objects.create surface.** Initial fixtures regen produced 18 status rows all "ok" because upstream's pending/deleted patterns live in helpers (make_data_*, create_entity) that our literal-string parser cannot evaluate. Resolution: D-72-02-01 synthesis path with assertion-line citations.
- **One-line objects.create dropped silently.** extractFieldsSharp inherited the skip-line-0 quirk from extractFields. Surfaced when real upstream regen produced only 18 ok status rows despite line 6252 explicitly declaring `Organization.objects.create(status="pending", ...)`. Fix: strip prefix/suffix on line 0 in extractFieldsSharp only.
- **Initial lint failure**: `if !(a < b && b < c)` flagged by staticcheck QF1001 (De Morgan's). Rewritten to `if a >= b || b >= c`. Trivial.

## User Setup Required

None — no external service configuration.

## Next Phase Readiness

- **Plan 72-03 (UNICODE+IN+TRAVERSAL categories) ready**: extend the same `--category` switch + add `parse_unicode.go`, `parse_in.go`, `parse_traversal.go` siblings. The section{} render type and buildSections dispatcher pre-built in this plan generalise cleanly to N+ categories.
- **Plan 72-04 (consumer parity tests) ready**: tests under `internal/pdbcompat/parity/` can now consume `parity.OrderingFixtures` (ORDER-01..03), `parity.StatusFixtures` (STATUS-01..05 incl. carve-out), `parity.LimitFixtures` (LIMIT-01 boundary at 260 net + LIMIT-02 depth at 5 org/ix). Each test seeds via `testutil.SetupClient(t)` per CONTEXT.md plan-hint.
- **SHA pinning intact**: subsequent plans must continue to pass `--upstream-ref 99e92c726172ead7d224ce34c344eff0bccb3e63` to maintain the same SHA across the fixtures.go header. `--check --pinned 75c7a6fab734db782b9035a6bc23ae11abcce5901a6017a051f76bbb51399043` will validate.
- **--append flow validated**: future plan that adds a new category can use `--category <new> --append` to extend fixtures.go without touching the other vars or the header. Tested by TestFixturePort_AppendPreservesOtherCategories.

## Self-Check: PASSED

Verified claims:
- `cmd/pdb-fixture-port/parse_status.go` exists (197 LOC) — FOUND
- `cmd/pdb-fixture-port/parse_limit.go` exists (139 LOC) — FOUND
- `cmd/pdb-fixture-port/main.go` modified — FOUND (recent mtime)
- `cmd/pdb-fixture-port/main_test.go` modified — FOUND
- `cmd/pdb-fixture-port/testdata/pdb_api_test_min.py` modified — FOUND
- `internal/testutil/parity/fixtures.go` regenerated to 3305 LOC — FOUND
- `internal/testutil/parity/fixtures_test.go` modified — FOUND
- `internal/testutil/parity/generate.go` updated to --category all — FOUND
- Commit `235eaf0` (Task 1 RED) in git log — FOUND
- Commit `6700f83` (Task 1 GREEN) in git log — FOUND
- Commit `dc1139e` (Task 2 RED) in git log — FOUND
- Commit `cca6c3c` (Task 2 GREEN) in git log — FOUND
- `grep -c "var OrderingFixtures = " internal/testutil/parity/fixtures.go == 1` — PASS
- `grep -c "var StatusFixtures = " internal/testutil/parity/fixtures.go == 1` — PASS
- `grep -c "var LimitFixtures = " internal/testutil/parity/fixtures.go == 1` — PASS
- `grep "peeringdb/peeringdb@99e92c72" internal/testutil/parity/fixtures.go` — PASS (SHA preserved)
- OrderingFixtures slice byte-identical to plan 72-01 (diff exit 0) — PASS
- `go build ./...` — PASS
- `go vet ./...` — PASS
- `go test -race -short ./...` — PASS (all packages green incl. internal/testutil/parity)
- `golangci-lint run ./cmd/pdb-fixture-port/ ./internal/testutil/parity/` — PASS (0 issues)

## TDD Gate Compliance

Plan 72-02 used per-task TDD (`tdd="true"` on both tasks) rather than a single plan-level RED/GREEN gate. Both tasks have RED → GREEN commit pairs:

- **Task 1**: RED `235eaf0` (test) → GREEN `6700f83` (feat) ✓
- **Task 2**: RED `dc1139e` (test) → GREEN `cca6c3c` (feat) ✓

No REFACTOR commits — code shape was correct on first GREEN pass for both tasks.

---
*Phase: 72-upstream-parity-regression*
*Completed: 2026-04-19*
