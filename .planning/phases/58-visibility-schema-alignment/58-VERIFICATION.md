---
phase: 58-visibility-schema-alignment
verified: 2026-04-16T15:00:00Z
status: passed
score: 5/5 must-haves verified
overrides_applied: 0
---

# Phase 58: Visibility Schema Alignment Verification Report

**Phase Goal:** Bring the ent schemas into agreement with the empirical visibility baseline — every auth-gated entity discovered in Phase 57 has a visibility-bearing field that the privacy policy can key off.
**Verified:** 2026-04-16T15:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

Phase 58 is a confirmation + documentation phase. The Phase 57 diff proved that the only auth-gated surfaces are `poc` row-level visibility (already covered by `poc.visible`) and `ixlan.ixf_ixp_member_list_url` (already covered by `ixlan.ixf_ixp_member_list_url_visible`). No new ent fields were needed. The phase's work was: lock that finding into a regression test, persist the `<field>_visible` + NULL-handling conventions in both `PROJECT.md` and `CLAUDE.md`, and confirm `go generate ./...` produces zero drift. All five must-haves verify.

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Phase 57 diff.json mechanically asserted to contain no auth-gated schema surfaces beyond poc + ixlan.ixf_ixp_member_list_url | VERIFIED | `TestSchemaAlignmentWithPhase57Diff` passes with 5/5 sub-tests green; allowlist covers exactly the two expected surfaces |
| 2 | Existing ent schema unchanged: `poc.visible` and `ixlan.ixf_ixp_member_list_url_visible` remain the only visibility-bearing fields | VERIFIED | `grep "_visible\|\"visible\"" ent/schema/` returns exactly 2 matches: `poc.go:53` (`field.String("visible").Default("Public")`) and `ixlan.go:45` (`field.String("ixf_ixp_member_list_url_visible").Default("Private")`) |
| 3 | PROJECT.md Key Decisions carries a v1.14 Phase 58 entry documenting no new fields, `<field>_visible` convention, and NULL-treats-as-default (D-07) | VERIFIED | 4 rows ending `✓ Validated Phase 58` at lines 197-200; footer updated at line 232 |
| 4 | CLAUDE.md ent conventions document the `<field>_visible` pattern and NULL handling | VERIFIED | New `### Schema & Visibility` subsection at CLAUDE.md:66, between `### Code Generation` (line 55) and `### Middleware` (line 77); references both fields, `field.String` convention, and NULL-as-default rule |
| 5 | `go generate ./...` leaves working tree clean (no drift in ent/, gen/, graph/, internal/web/templates/) | VERIFIED | `git status --porcelain -- ent/ gen/ graph/ internal/web/templates/` returns empty; full `git status --porcelain` is clean (no uncommitted changes anywhere) |

**Score:** 5/5 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/visbaseline/schema_alignment_test.go` | Regression test `TestSchemaAlignmentWithPhase57Diff` guarding the Phase 57 diff against future drift | VERIFIED | File exists (6,219 bytes); package `visbaseline`; reuses `Report`/`TypeReport`/`FieldDelta` from `diff.go` (no parallel struct decls); 5 sub-tests all pass; allowlist correctly scoped to `beta/poc` (10 fields) + `beta/ixlan` (ixf_ixp_member_list_url); failure message includes "gsd-plan-phase 58" + references `<field>_visible` convention |
| `.planning/PROJECT.md` | 4 new v1.14 Phase 58 Key Decision rows + footer update | VERIFIED | Rows at lines 197-200 match plan verbatim; "Last updated" footer at line 232 reflects Phase 58; Evolution section below (line 202) untouched |
| `CLAUDE.md` | New `### Schema & Visibility` subsection under `## Conventions` | VERIFIED | Subsection at line 66 (between Code Generation at 55 and Middleware at 77); references `poc.visible`, `ixlan.ixf_ixp_member_list_url_visible`, `field.String`, `schema_alignment_test.go`, and NULL-as-default rule |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `internal/visbaseline/schema_alignment_test.go` | `testdata/visibility-baseline/diff.json` | `os.ReadFile` + `json.Unmarshal` into `visbaseline.Report` | WIRED | Line 28: `diffPath := filepath.Join("..", "..", "testdata", "visibility-baseline", "diff.json")`; line 30: `os.ReadFile(diffPath)`; line 36: `json.Unmarshal(raw, &report)`. File exists at the computed path (diff.json, 3,914 bytes). Test runs green, confirming wiring fires end-to-end. |
| `internal/visbaseline/schema_alignment_test.go` | `ent/schema/poc.go` + `ent/schema/ixlan.go` | `allowedAuthGated` allowlist encoded as test data | WIRED | Lines 75-83: allowlist keys are `beta/poc` (10 field names — all PeeringDB poc fields) and `beta/ixlan` (`ixf_ixp_member_list_url`). Sub-test `no_unexpected_auth_gated_fields` iterates every AuthOnly field across all 13 types and fails on anything outside the allowlist. Green, so the allowlist matches reality. |

### Data-Flow Trace (Level 4)

Not applicable — this phase produces a test file and documentation updates. The test file reads a committed fixture and asserts on its contents; there is no dynamic data-rendering artifact.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Schema-alignment test passes | `TMPDIR=/tmp/claude-1000 go test -race -run TestSchemaAlignmentWithPhase57Diff ./internal/visbaseline/...` | `ok  github.com/dotwaffle/peeringdb-plus/internal/visbaseline  1.016s` | PASS |
| All 5 sub-tests green | `go test -race -v -run TestSchemaAlignmentWithPhase57Diff ./internal/visbaseline/...` | `--- PASS` for schema_version_matches, all_13_types_present, no_unexpected_auth_gated_fields, poc_visible_drifts_public_to_users, poc_visible_field_not_authonly | PASS |
| `go vet` clean on visbaseline | `TMPDIR=/tmp/claude-1000 go vet ./internal/visbaseline/...` | empty output (exit 0) | PASS |
| Zero codegen drift | `git status --porcelain -- ent/ gen/ graph/ internal/web/templates/` | empty output | PASS |
| Overall tree clean | `git status --porcelain` | empty output | PASS |
| PROJECT.md Phase 58 row count | `grep -c "Validated Phase 58" .planning/PROJECT.md` | `4` | PASS |
| CLAUDE.md section ordering | awk check Code Generation < Schema & Visibility < Middleware | `ok: cg=55 sv=66 mw=77` | PASS |
| Only 2 `_visible` fields in ent schemas | `grep "_visible\|\"visible\"" ent/schema/` | 2 matches: `poc.go:53` + `ixlan.go:45` | PASS |
| Task commits present | `git log --oneline` | `e603c66 test(58-01)`, `184fa4b docs(58-01)`, `b306acc docs(58-01)` all present | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| VIS-03 | 58-01-PLAN.md | ent schemas have visibility-bearing fields for every auth-gated entity identified by VIS-02 (`poc.visible` already exists; add others as the diff surfaces them and regenerate ent) | SATISFIED | Phase 57 diff.json surfaced only `poc` row-level (covered by `poc.visible`) and `ixlan.ixf_ixp_member_list_url` (covered by `ixlan.ixf_ixp_member_list_url_visible`); all other 11 types show zero auth-gated fields. The regression test `TestSchemaAlignmentWithPhase57Diff` asserts this mechanically and will fail on any future diff drift. REQUIREMENTS.md line 82 maps VIS-03 → Phase 58 exclusively; no orphans. |

No orphaned requirements. REQUIREMENTS.md traceability table (line 82) maps VIS-03 to Phase 58 as the sole requirement for this phase; the plan's `requirements: [VIS-03]` frontmatter matches.

### ROADMAP Success Criteria

All four Phase 58 ROADMAP success criteria (ROADMAP.md:77-81) satisfied:

| # | Success Criterion | Status | Evidence |
|---|-------------------|--------|----------|
| SC-1 | `poc.visible` confirmed as existing visibility-bearing field for POCs and surveyed against Phase 57 diff | SATISFIED | Test sub-tests `poc_visible_drifts_public_to_users` + `poc_visible_field_not_authonly` verify anon sees only `Public`, auth sees `Public`+`Users`, and the `visible` field itself is not flagged AuthOnly |
| SC-2 | Any additional auth-gated fields surfaced by VIS-02 have ent schema fields that the privacy policy can use | SATISFIED | diff.json surfaces only `ixlan.ixf_ixp_member_list_url` beyond poc; already covered by pre-existing `ixlan.ixf_ixp_member_list_url_visible`. No new fields needed. |
| SC-3 | `go generate ./...` regenerates ent cleanly; committed `ent/` files byte-identical with what generator produces | SATISFIED | `git status --porcelain -- ent/ gen/ graph/ internal/web/templates/` empty; task 4 verification task documented in SUMMARY as "clean: no drift" |
| SC-4 | Findings documented in PROJECT.md Key Decisions so future maintainers can trace why each field exists | SATISFIED | Four new rows at PROJECT.md:197-200, each ending `✓ Validated Phase 58`; footer updated |

### Anti-Patterns Found

No blocker or warning anti-patterns introduced by this phase. Pre-existing lint debt in `internal/visbaseline/reportcli.go` and `redactcli.go` (5 issues — 1 exhaustive, 3 gosec G304, 1 nolintlint) exists on the base commit `60bf023` before any Phase 58 change, documented in `.planning/phases/58-visibility-schema-alignment/deferred-items.md` with the source commit hash. None of those issues are in the new `schema_alignment_test.go`.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `internal/visbaseline/schema_alignment_test.go` | n/a | (no issues) | — | New file lints clean and vets clean |

### Human Verification Required

None. All verification is programmatic: test passes, drift check clean, grep-verifiable documentation additions.

### Gaps Summary

No gaps. Phase 58's goal — align ent schemas with the empirical visibility baseline — is achieved by confirming (via a fixture-asserting regression test) that the existing schema already covers every auth-gated surface the Phase 57 diff surfaced, and by persisting the `<field>_visible` + NULL-handling conventions in both `PROJECT.md` Key Decisions and `CLAUDE.md` Conventions. The test is green, documentation is in place, and `go generate ./...` produces zero drift. Phase 59 (privacy policy) can proceed against a documented baseline: `poc.visible` for row-level gating on POCs and `ixlan.ixf_ixp_member_list_url_visible` for per-field gating on ixlan.

---

_Verified: 2026-04-16T15:00:00Z_
_Verifier: Claude (gsd-verifier)_
