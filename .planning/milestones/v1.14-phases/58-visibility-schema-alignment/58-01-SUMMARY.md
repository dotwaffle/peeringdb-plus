---
phase: 58-visibility-schema-alignment
plan: 01
subsystem: schema
tags: [ent, visibility, privacy, regression-test, peeringdb]

# Dependency graph
requires:
  - phase: 57-visibility-baseline-capture
    provides: testdata/visibility-baseline/diff.json — machine-readable auth vs anon diff
provides:
  - Regression test asserting diff.json holds no auth-gated surface beyond poc.visible + ixlan.ixf_ixp_member_list_url_visible
  - v1.14 Key Decisions documenting empirical finding, <field>_visible convention, and NULL-treats-as-default rule
  - CLAUDE.md Schema & Visibility convention for future Claude sessions
  - Confirmation that existing ent schema is sufficient — no new fields required
affects: [59-privacy-policy, 60-authenticated-sync-enablement]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Regression tests against committed fixtures as schema-alignment guards"
    - "<field>_visible naming convention for per-field visibility (field.String, not Enum)"
    - "Privacy policy treats NULL *_visible as schema default on upgrade"

key-files:
  created:
    - internal/visbaseline/schema_alignment_test.go
    - .planning/phases/58-visibility-schema-alignment/deferred-items.md
    - .planning/phases/58-visibility-schema-alignment/58-01-SUMMARY.md
  modified:
    - .planning/PROJECT.md (Key Decisions +4 rows, footer updated)
    - CLAUDE.md (new ### Schema & Visibility subsection)

key-decisions:
  - "Phase 58 schema alignment: existing ent fields sufficient — no new fields needed"
  - "<field>_visible naming convention uses field.String (not Enum) to avoid codegen churn"
  - "Privacy policy treats NULL <field>_visible as schema default (Public), not Users, to avoid post-upgrade row hiding"
  - "Regression test in internal/visbaseline/schema_alignment_test.go guards the empirical assumption against future diff.json drift"

patterns-established:
  - "Regression test against committed empirical artifact — if testdata/visibility-baseline/diff.json is regenerated and surfaces a new auth-gated field, TestSchemaAlignmentWithPhase57Diff fails and forces re-planning before downstream phases ship"
  - "Phase 58 scope pattern: confirmation + documentation phase when predecessor artifact proves the expected work is already done"

requirements-completed: [VIS-03]

# Metrics
duration: 6min
completed: 2026-04-17
---

# Phase 58 Plan 01: Visibility schema alignment Summary

**Confirmed existing ent schema already covers all auth-gated PeeringDB surfaces — no new fields required; locked the finding into a regression test and documented the `<field>_visible` + NULL-handling conventions for Phase 59.**

## Performance

- **Duration:** ~6 min
- **Started:** 2026-04-17T00:08:00Z
- **Completed:** 2026-04-17T00:14:13Z
- **Tasks:** 4 (3 implementation + 1 verification-only)
- **Files modified:** 3 (1 created test, 2 doc updates)

## Accomplishments

- **Regression guard shipped.** `internal/visbaseline/schema_alignment_test.go` parses the committed `testdata/visibility-baseline/diff.json`, asserts all 13 PeeringDB types appear under the `beta/` prefix, and asserts that the *only* auth-gated field surfaces are `poc`'s 10 row-level fields (a row-visibility signal, not a schema gap) and `ixlan.ixf_ixp_member_list_url` (already gated by the pre-existing `ixf_ixp_member_list_url_visible` field). Any future re-capture that surfaces a new auth-gated field fails this test with an actionable message directing the reader to re-run `/gsd-plan-phase 58`.
- **PROJECT.md updated.** Four new `✓ Validated Phase 58` rows added to the Key Decisions table: the empirical finding, the `<field>_visible` naming convention, the NULL-treats-as-default rule (D-07), and the regression-test safeguard.
- **CLAUDE.md updated.** New `### Schema & Visibility` subsection under `## Conventions`, positioned between `### Code Generation` and `### Middleware`, documents the two visibility-bearing ent fields (`poc.visible`, `ixlan.ixf_ixp_member_list_url_visible`), the `<field>_visible` convention using `field.String`, and the NULL-handling rule.
- **Zero codegen drift.** `go generate ./...` leaves `ent/`, `gen/`, `graph/`, and `internal/web/templates/` byte-identical with committed files — confirming no ent schema changes were made and Phase 58 SC #3 is satisfied. CI's "Generated code drift check" job will pass.

## Task Commits

Each task was committed atomically with `--no-verify` (worktree executor convention):

1. **Task 1: Schema-alignment regression test** — `e603c66` (test)
2. **Task 2: PROJECT.md Key Decisions +4 rows** — `184fa4b` (docs)
3. **Task 3: CLAUDE.md Schema & Visibility subsection** — `b306acc` (docs)
4. **Task 4: `go generate ./...` drift check** — verification-only, no commit (zero drift confirmed)

## Files Created/Modified

- **Created** `internal/visbaseline/schema_alignment_test.go` — `TestSchemaAlignmentWithPhase57Diff` with 5 sub-tests (schema_version_matches, all_13_types_present, no_unexpected_auth_gated_fields, poc_visible_drifts_public_to_users, poc_visible_field_not_authonly). Reuses `Report`/`TypeReport`/`FieldDelta` from `diff.go` — no parallel struct declarations. stdlib only (testing, encoding/json, os, path/filepath, slices).
- **Created** `.planning/phases/58-visibility-schema-alignment/deferred-items.md` — documents pre-existing lint issues in `internal/visbaseline/reportcli.go` and `redactcli.go` that are out of scope for Phase 58.
- **Created** `.planning/phases/58-visibility-schema-alignment/58-01-SUMMARY.md` — this file.
- **Modified** `.planning/PROJECT.md` — 4 new rows appended to Key Decisions table (verbatim per plan Task 2); footer "Last updated" line updated from "milestone v1.14 ... started" to "Phase 58 visibility schema alignment validated existing schema sufficient".
- **Modified** `CLAUDE.md` — new `### Schema & Visibility` subsection inserted between `### Code Generation` and `### Middleware`.

### Four Key Decisions rows added to PROJECT.md (verbatim)

```
| Phase 58 schema alignment: existing ent fields sufficient | Phase 57 empirical diff surfaced only two auth-gated surfaces — poc row-level visibility (already covered by poc.visible per D-01/D-02) and ixlan.ixf_ixp_member_list_url (already covered by ixlan.ixf_ixp_member_list_url_visible). All other 11 types show zero field-level deltas. No new ent fields required | ✓ Validated Phase 58 |
| <field>_visible naming convention for per-field visibility | Established by ixlan.ixf_ixp_member_list_url_visible (pre-existing) and confirmed in Phase 58 as the canonical pattern for any future auth-gated field additions. field.String (not Enum) per D-02 — avoids entgql/entrest/entproto codegen churn; validation happens in the privacy policy layer | ✓ Validated Phase 58 |
| Privacy policy treats NULL <field>_visible as schema default | D-07: ent auto-migrate adds new *_visible columns with declared defaults, but existing rows synced before the column existed will have NULL until the next sync rewrites them. The Phase 59 privacy policy MUST treat NULL as the column default (`Public` per the upstream-default rule) rather than as `Users`, to prevent a post-upgrade flood of suddenly-hidden rows | ✓ Validated Phase 58 |
| Phase 58 regression test guards the empirical assumption | internal/visbaseline/schema_alignment_test.go asserts diff.json contains only the two known auth-gated surfaces. If a future Phase 57 re-capture surfaces a new auth-gated field, the test fails and forces Phase 58 to be re-opened before Phase 59's privacy policy ships against a stale assumption | ✓ Validated Phase 58 |
```

## Explicit Scope Note

**No ent schema changes made. No `go generate` output committed. Phase 58 was a confirmation + documentation phase** — the Phase 57 diff.json is the work order, and it showed that every auth-gated surface is already covered by existing ent fields.

## Decisions Made

None beyond those already captured in `58-CONTEXT.md` (D-01 through D-09) and re-logged in PROJECT.md Key Decisions per Task 2. Plan executed exactly as specified.

## Deviations from Plan

None — plan executed exactly as written. Task 1's `tdd="true"` flag was interpreted as "test-only addition" since the behaviour under test is a committed fixture (no production code to write); a single `test(...)` commit captures the regression guard.

## Test Results

```
=== RUN   TestSchemaAlignmentWithPhase57Diff
=== RUN   TestSchemaAlignmentWithPhase57Diff/schema_version_matches
=== RUN   TestSchemaAlignmentWithPhase57Diff/all_13_types_present
=== RUN   TestSchemaAlignmentWithPhase57Diff/no_unexpected_auth_gated_fields
=== RUN   TestSchemaAlignmentWithPhase57Diff/poc_visible_drifts_public_to_users
=== RUN   TestSchemaAlignmentWithPhase57Diff/poc_visible_field_not_authonly
--- PASS: TestSchemaAlignmentWithPhase57Diff
PASS
ok      github.com/dotwaffle/peeringdb-plus/internal/visbaseline        2.558s
```

Full `go test -race ./internal/visbaseline/...` green. `go vet ./...` clean across the whole module.

## `go generate ./...` Drift Check

Ran `TMPDIR=/tmp/claude-1000 go generate ./...` (full pipeline: ent + entgql + entrest + entproto, buf generate, templ generate, schema regenerate). Result:

```
git status --porcelain -- ent/ gen/ graph/ internal/web/templates/
(empty — zero drift)
```

**Clean: no drift.** CI's "Generated code drift check" will pass on this tree.

## Issues Encountered

**Pre-existing lint debt in `internal/visbaseline` (out of scope).** `golangci-lint run ./internal/visbaseline/...` reports 5 issues (1 exhaustive, 3 gosec G304, 1 nolintlint) in `reportcli.go` and `redactcli.go` — all on the base commit before any Phase 58 change. Logged in `.planning/phases/58-visibility-schema-alignment/deferred-items.md` per the executor's scope-boundary rule. The new `schema_alignment_test.go` introduces no new lint issues.

## User Setup Required

None — no external service configuration required.

## Next Phase Readiness

**Phase 59 (ent Privacy policy) is unblocked.** The two visibility-bearing ent fields it will read are:

- `poc.visible` (`ent/schema/poc.go`, line 53) — row-level gate. `Public` rows served to anonymous callers; `Users` rows filtered.
- `ixlan.ixf_ixp_member_list_url_visible` (`ent/schema/ixlan.go`, line 45) — per-field gate. When not `Public`, the privacy policy nulls the sibling `ixf_ixp_member_list_url` value on anonymous responses.

The Phase 58 regression test `TestSchemaAlignmentWithPhase57Diff` is now the early-warning system: if a future re-capture of the visibility baseline surfaces any additional auth-gated field, the test fails in CI and forces re-opening of Phase 58 before Phase 59 ships privacy logic built on a stale assumption.

## Self-Check: PASSED

- `internal/visbaseline/schema_alignment_test.go` — **FOUND**
- `.planning/phases/58-visibility-schema-alignment/deferred-items.md` — **FOUND**
- Commit `e603c66` (test Task 1) — **FOUND** in `git log`
- Commit `184fa4b` (docs Task 2) — **FOUND** in `git log`
- Commit `b306acc` (docs Task 3) — **FOUND** in `git log`
- PROJECT.md contains 4x `Validated Phase 58` — **VERIFIED** (`grep -c` == 4)
- CLAUDE.md contains `### Schema & Visibility` — **VERIFIED**
- `go generate ./...` drift in generated dirs — **ZERO** (verified via `git status --porcelain -- ent/ gen/ graph/ internal/web/templates/`)
- `go test -race ./internal/visbaseline/...` — **PASS**
- `go vet ./...` — **PASS**

---
*Phase: 58-visibility-schema-alignment*
*Completed: 2026-04-17*
