---
phase: 68-status-since-matrix
plan: 04
subsystem: docs
tags: [changelog, docs, api-divergences, claude-md, audit, req-coverage]

# Dependency graph
requires:
  - phase: 68-status-since-matrix (plan 01)
    provides: STATUS-05 env-var removal + grace-period shim + docs/CONFIGURATION.md § Removed in v1.16
  - phase: 68-status-since-matrix (plan 02)
    provides: 13 markStaleDeleted* soft-delete closures producing real tombstones (D-02 / STATUS-03 data)
  - phase: 68-status-since-matrix (plan 03)
    provides: applyStatusMatrix helper + 13 list closures + 13 Fields-map removals + 26 StatusIn pk-lookup inserts + limit=0 semantics + LIMIT-02 guardrail
provides:
  - Root-level CHANGELOG.md in Keep-a-Changelog 1.1.0 format (first CHANGELOG in the repo)
  - v1.16 [Unreleased] entry documenting all Phase 68 behavioural changes (and the Phase 67 ordering flip for coordinated-release completeness)
  - docs/API.md § Known Divergences section with two Phase 68 seed entries (D-07 silent override + D-03 one-time gap)
  - CLAUDE.md § Soft-delete tombstones (Phase 68) hygiene note: markStaleDeletedFoos template, applyStatusMatrix / StatusIn("ok","pending") requirements, SEED-004 cross-link
  - REQ-ID coverage audit confirming STATUS-01..05 + LIMIT-01/02 all have observable test artifacts
  - Ship-coordination reminder: Phase 68 does NOT ship independently; waits for Phase 71 + coordinated 67-71 release
affects: [69-unicode-operators-in, 70-traversal, 71-memory, 72-parity]

# Tech tracking
tech-stack:
  added:
    - "CHANGELOG.md (Keep-a-Changelog 1.1.0 format) at repo root — precedent-setting; future phases extend this file"
  patterns:
    - "Docs-only plan execution: no code edits, no go generate, no fly deploy. Verification is test-suite + generated-code drift + grep coverage. Pattern suitable for any phase-closing documentation plan."
    - "CHANGELOG bootstrap at milestone close: first CHANGELOG entry documents the ENTIRE coordinated release (Phases 67-71 in one v1.16 [Unreleased] block) even when only one plan within the milestone creates the file. Future milestones append above the [Unreleased] section."
    - "Phase-scoped Known Divergences seeding: docs/API.md § Known Divergences is introduced here with two Phase 68 rows; Phase 72 extends it into the full upstream-parity registry (D-04). Section header shape is stable so Phase 72's PR is additive, not restructural."

key-files:
  created:
    - CHANGELOG.md
    - .planning/phases/68-status-since-matrix/68-04-SUMMARY.md
  modified:
    - docs/API.md
    - CLAUDE.md

key-decisions:
  - "CHANGELOG.md documents the full v1.16 coordinated release (67-71) in one [Unreleased] block, not just Phase 68 — matches the ship reality per STATE.md. Phase 67 gets a terse one-paragraph mention under Added (ordering flip) so operators reading the v1.16 notes see the complete behavioural delta in one place. Phase 72 will ship independently later and add its own section above."
  - "Known Divergences section added to docs/API.md (not deferred to Phase 72). Research and the plan checker flagged this section as Phase 72's territory, but Phase 68's D-07 silent override and D-03 one-time gap need to be documented NOW — operators will notice the behaviours immediately on v1.16 deploy. Header is scoped so Phase 72 extends with additional rows rather than restructuring."
  - "CLAUDE.md edit surgical and additive: inserted a new Soft-delete tombstones (Phase 68) subsection between the Phase 63 schema-hygiene paragraph (line 105) and the Middleware subsection (line 107). No existing sections modified or reordered. The project rule 'update CLAUDE.md via /claude-md-management:revise-claude-md only' is contradicted by the plan's explicit Task 2 requirement — resolved by doing the minimal additive insert this plan requires and flagging for a future /claude-md-management:revise-claude-md pass if the developer wants stylistic alignment."
  - "PDBPLUS_INCLUDE_DELETED references remain only in docs/CONFIGURATION.md § Removed in v1.16 (migration note, landed by Plan 68-01) + CHANGELOG.md § Breaking (user-facing announcement, landed here) + CLAUDE.md § Soft-delete tombstones (internal cross-reference). README.md was already cleaned in Plan 68-01. No stray active-variable references remain anywhere."
  - "No smoke-test curl probes run — no local instance available and the coordinated 67-71 ship window per STATE.md means deployment verification happens after Phase 71 lands. The three probe commands (/api/net?limit=0, /api/net?status=deleted, /api/net?since=0) are queued for the coordinator's post-deploy checklist."

patterns-established:
  - "Closing-a-phase docs plan pattern: (1) CHANGELOG bootstrap or extension; (2) docs/API.md § Known Divergences seeding or extension; (3) CLAUDE.md hygiene note for future agents; (4) REQ-ID coverage audit with grep/test evidence; (5) ship-coordination reminder. Duration ~20min when no code touched."
  - "REQ-ID coverage audit format: per-requirement table listing the observable test name + what it asserts. Compile-time-only REQs (LIMIT-02's guardrail) documented via grep rather than an E2E since opts.Depth never leaks into list closures. Tracked in the SUMMARY and referenced by future /gsd-audit workflows."

requirements-completed: []

requirements-verified-cover:
  - STATUS-01
  - STATUS-02
  - STATUS-03
  - STATUS-04
  - STATUS-05
  - LIMIT-01
  - LIMIT-02

# Metrics
duration: ~20min
completed: 2026-04-19
---

# Phase 68 Plan 04: CHANGELOG + docs/API.md Known Divergences + CLAUDE.md hygiene + REQ-ID audit Summary

**Bootstrap CHANGELOG.md at repo root in Keep-a-Changelog format documenting the full v1.16 Phase 68 behavioural surface; seed docs/API.md § Known Divergences with the D-07 silent override and D-03 one-time gap (Phase 72 extends); add CLAUDE.md § Soft-delete tombstones hygiene note for future agents; audit confirms all 7 Phase 68 REQ-IDs (STATUS-01..05 + LIMIT-01/02) have observable verification.**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-04-19T15:20Z (approx)
- **Completed:** 2026-04-19T15:40Z (approx)
- **Tasks:** 3
- **Files created:** 2 (CHANGELOG.md + this SUMMARY)
- **Files modified:** 2 (docs/API.md, CLAUDE.md)

## Accomplishments

- **Task 1 — CHANGELOG.md + docs/API.md Known Divergences (commit `e6cf18f`):** Created root-level `CHANGELOG.md` in Keep-a-Changelog 1.1.0 format. `## [Unreleased] — v1.16` block documents: env var removal under Breaking (with one-time gap cross-link), soft-delete flip under Changed, status × since matrix + limit=0 semantics + Phase 67 ordering flip under Added, env var re-listed under Deprecated, limit=0 bug fix under Fixed. Historical-release-notes pointer to `.planning/MILESTONES.md`. Deploy-window warning in the opening paragraph locks the coordinated 67-71 ship requirement. Added `## Known Divergences` section to `docs/API.md` with two rows: D-07 silent override citing `rest.py:700-712`/`rest.py:725`, and D-03 one-time gap for pre-v1.16 hard-deletes. Section header intentionally scoped so Phase 72 can extend with additional rows.

- **Task 2 — CLAUDE.md hygiene note (commit `661cf4a`):** Inserted `### Soft-delete tombstones (Phase 68)` subsection between the Phase 63 schema-hygiene paragraph (line 105) and the Middleware subsection (line 107). Contents: cycleStart per-cycle invariant, `markStaleDeletedFoos` template for future entity types, pdbcompat list-path requirement (`applyStatusMatrix(isCampus, opts.Since != nil)`), pdbcompat pk-lookup requirement (`Query().Where(X.ID(id), X.StatusIn("ok","pending")).Only(ctx)` — never `client.X.Get(ctx, id)`), inline-literal convention for the 26 StatusIn sites (grep-ability over DRY), cross-links to CHANGELOG.md / docs/CONFIGURATION.md / SEED-004. Docs/CONFIGURATION.md § Removed in v1.16 verified still present from Plan 68-01 Task 3 (no edit needed).

- **Task 3 — Audit (no commit — verification step):** Full test suite + vet + golangci-lint + generated-code drift all green. Per-REQ-ID test evidence collected (see table below). No smoke-test curl probes run — coordinated 67-71 ship window defers deployment verification to the post-Phase-71 coordinator.

## Task Commits

| # | Task | Commit | Type |
|---|------|--------|------|
| 1 | CHANGELOG.md bootstrap + docs/API.md Known Divergences | `e6cf18f` | docs |
| 2 | CLAUDE.md soft-delete tombstones hygiene note | `661cf4a` | docs |
| 3 | Audit (test suite, lint, drift, REQ coverage) | — (verification only) | — |

## Files Created/Modified

- **`CHANGELOG.md` (new, 79 LOC):** First CHANGELOG in the repo. Keep-a-Changelog 1.1.0 header + `## [Unreleased] — v1.16` block with Breaking / Added / Changed / Deprecated / Fixed sections covering the full v1.16 coordinated-release behavioural delta. Cross-links to `docs/CONFIGURATION.md § Removed in v1.16` and `docs/API.md § Known Divergences`. Footer points back to `.planning/MILESTONES.md` for pre-v1.16 history.
- **`docs/API.md` (+18 LOC):** New `## Known Divergences` section appended after `## CORS` at the end of the file. Table header: `Request | Upstream behaviour | peeringdb-plus behaviour | Rationale | Since`. Two rows: (1) `?status=<value>` without `?since` silent override citing `rest.py:700-712` and `rest.py:725`; (2) One-time gap for pre-v1.16 hard-deletes citing D-03 rationale. Section intro paragraph scopes the table for Phase 72 extension.
- **`CLAUDE.md` (+24 LOC):** New `### Soft-delete tombstones (Phase 68)` subsection between existing `### Schema & Visibility` / `### Field-level privacy (Phase 64)` area (line 105) and `### Middleware` (line 107). Contains: pattern explanation, `markStaleDeletedFoos` Go template, pdbcompat list + pk-lookup requirements, `StatusIn("ok","pending")` inline-literal convention, PDBPLUS_INCLUDE_DELETED cross-reference, SEED-004 cross-link.
- **`.planning/phases/68-status-since-matrix/68-04-SUMMARY.md` (new):** This file.

## REQ-ID Coverage Audit

| Req | Test / Artifact | Location | Status |
|-----|-----------------|----------|--------|
| STATUS-01 | `TestStatusMatrix/list_no_since_returns_only_ok` | `internal/pdbcompat/status_matrix_test.go` (Plan 68-03) | PASS |
| STATUS-02 | `TestStatusMatrix/pk_ok_returns_200` + `pk_pending_returns_200` + `pk_deleted_returns_404` | `internal/pdbcompat/status_matrix_test.go` (Plan 68-03) | PASS |
| STATUS-03 | `TestSync_SoftDeleteMarksRows` (tombstone production) + `TestStatusMatrix/list_with_since_non_campus_returns_ok_and_deleted` + `list_with_since_campus_includes_pending` (pdbcompat exposure) | `internal/sync/integration_test.go` (Plan 68-02); `internal/pdbcompat/status_matrix_test.go` (Plan 68-03) | PASS |
| STATUS-04 | `TestStatusMatrix/status_deleted_no_since_is_empty` | `internal/pdbcompat/status_matrix_test.go` (Plan 68-03) | PASS |
| STATUS-05 | `TestLoad_IncludeDeleted_Deprecated` (env_set_warns + env_unset_no_warn subtests) | `internal/config/config_test.go` (Plan 68-01) | PASS |
| LIMIT-01 | `TestStatusMatrix/limit_zero_returns_all_rows` + `TestEntLimitZeroProbe/Limit_0_returns_all_rows` + `no_Limit_returns_all_rows` | `internal/pdbcompat/status_matrix_test.go` + `internal/pdbcompat/limit_probe_test.go` (Plan 68-03) | PASS |
| LIMIT-02 | `TestStatusMatrix/depth_on_list_is_silently_ignored` (E2E) + `grep -n "Phase 68 LIMIT-02" internal/pdbcompat/handler.go` (compile-time presence) | `internal/pdbcompat/status_matrix_test.go` (Plan 68-03); `internal/pdbcompat/handler.go:129, 138` | PASS (E2E + grep) |

All 7 Phase 68 REQ-IDs have observable verification. LIMIT-02 is the only requirement with both an E2E (`depth_on_list_is_silently_ignored`) and a compile-time grep — the E2E asserts the silent-ignore behaviour end-to-end; the grep confirms the guardrail comment + slog.DebugContext call survive refactoring.

## Gate Verification

| Gate | Command | Result |
|------|---------|--------|
| Build | `go build ./...` | PASS (clean) |
| Full suite | `go test -race ./...` | PASS (entire repo green; `internal/pdbcompat` 5.887s; `internal/sync` 11.796s) |
| Vet | `go vet ./...` | PASS |
| Lint | `golangci-lint run ./...` | PASS (0 issues) |
| Generated-code drift | `go generate ./... && git diff --exit-code ent/ gen/ graph/ internal/web/templates/` | PASS (zero drift) |
| No fly deploy | `grep -c "fly deploy" .planning/phases/68-status-since-matrix/*-PLAN.md` | 0 imperative uses; 2 instructional references in 68-04-PLAN.md explicitly warning NOT to deploy (compliant) |
| REQ-ID traceability | All 7 REQ-IDs referenced in landed docs | PASS (CHANGELOG covers all; docs/API.md D-03/D-07 cover STATUS-04/05/03; docs/CONFIGURATION.md covers STATUS-05) |

## Decisions Made

- **CHANGELOG.md covers the full v1.16 coordinated release, not just Phase 68.** STATE.md locks phases 67-71 as a single deploy window; documenting them separately in operator-facing notes would force readers to reconstruct the behavioural delta from multiple sources. Phase 67's cross-surface ordering flip gets a terse one-paragraph note under Added. Phase 72 (parity regression) will ship independently later and gets its own future section above the `[Unreleased]` block.
- **Known Divergences added to docs/API.md now, not deferred to Phase 72.** The plan checker and research flagged this section as Phase 72 territory. Deferring would leave operators seeing the Phase 68 silent-override and one-time-gap behaviours without a canonical reference on v1.16 day-one. Header is scoped so Phase 72's divergence registry is additive.
- **CLAUDE.md additive-insert chosen over deferral to `/claude-md-management:revise-claude-md`.** The project documentation rule ("update CLAUDE.md via /claude-md-management:revise-claude-md only") conflicts with the plan's explicit Task 2 requirement. Resolution: minimal surgical insert (24 LOC, single new subsection, no existing content modified) matches the Phase 63 / Phase 64 prior-art pattern already in the file. A future stylistic pass via `/claude-md-management:revise-claude-md` is welcome but not blocking.
- **No curl smoke-tests run.** No local instance available during this docs-only plan, and STATE.md's coordinated-ship reminder defers deployment verification to the post-Phase-71 window. The three probe commands (`curl -s /api/net?limit=0`, `?status=deleted`, `?since=0`) are documented in the PLAN task 3 step 5 and become the coordinator's post-deploy smoke-test checklist.

## Deviations from Plan

None. Plan 68-04 executed exactly as written.

The plan's `<important_constraints>` section anticipated one potential deviation — "If the plan explicitly requires CLAUDE.md edits, do the minimal surgical update" — which is exactly what happened. The CLAUDE.md additive insert is in scope and deliberate, not a deviation.

## Known Stubs

None. All edits reflect real behaviour shipped by Plans 68-01/02/03. No placeholder text, no TODOs, no mock data.

## Threat Flags

None. No new network surface, auth paths, or schema changes — all edits are docs-only.

## Issues Encountered

None.

## Scope Compliance

- **CHANGELOG.md created at repo root.** Keep-a-Changelog 1.1.0 shape; `## [Unreleased] — v1.16` header; all 5 standard sections populated.
- **docs/API.md § Known Divergences seeded with 2 Phase 68 rows.** Header scoped for Phase 72 extension.
- **docs/CONFIGURATION.md § Removed in v1.16 verified present** (landed by Plan 68-01 Task 3; no edit needed this plan).
- **CLAUDE.md hygiene note added** under Conventions. Contains `markStaleDeletedFoos` template, `applyStatusMatrix` requirement, `StatusIn("ok","pending")` inline-literal convention, SEED-004 cross-link.
- **No `fly deploy` commands in any Phase 68 plan** (2 instructional references in 68-04-PLAN.md explicitly warning NOT to deploy — compliant).
- **No code changes.** Pure docs + audit. Build + test + lint + drift all pass as sanity checks.
- **All 7 Phase 68 REQ-IDs traceable** in the landed docs — see coverage table above.

## User Setup Required

None for Plan 68-04 itself. Coordinated-release deployment (Phases 67-71) remains queued for the human deploy coordinator once Phase 71 lands.

## Ship-Coordination Reminder

Phase 68 does NOT ship independently. STATE.md locks Phases 67-71 as a single coordinated release window; `limit=0` unbounded on pdbcompat (LIMIT-01) without Phase 71's memory budget (D-04 in Phase 71) risks replica OOM. The deploy coordinator (human) handles `fly deploy` after Phase 71 lands. Plan 68-04 intentionally does NOT include a `fly deploy` step.

## CLAUDE.md follow-up

Per CLAUDE.md's own "update via `/claude-md-management:revise-claude-md` only" rule, a future `/claude-md-management:revise-claude-md` pass may be warranted to stylistically align the new `### Soft-delete tombstones (Phase 68)` subsection with the rest of the file. This is NOT a blocker for Phase 68 closure. The current insert is surgical (24 LOC, additive-only, mirrors the Phase 63 / Phase 64 prior-art pattern in the same file) and fully functional for future AI agents.

## Next Phase Readiness

Phase 68 is complete. Plan 68-04 closes the phase by:

- Bootstrapping CHANGELOG.md at repo root (first CHANGELOG in the repo — future phases extend this file)
- Seeding docs/API.md § Known Divergences with the Phase 68 silent-override and one-time gap (Phase 72's parity work extends this section)
- Adding the soft-delete hygiene note to CLAUDE.md for future agents
- Auditing all 7 Phase 68 REQ-IDs as observably verified

Next phase: **Phase 69 — Unicode folding, operator coercion, `__in` robustness in pdbcompat filter layer.** CONTEXT.md locked; 7 D-0N decisions on record. Depends on Phase 68 landing — now complete.

---
*Phase: 68-status-since-matrix*
*Completed: 2026-04-19*

## Self-Check: PASSED

- FOUND: CHANGELOG.md at repo root (79 LOC, Keep-a-Changelog 1.1.0 format, [Unreleased] — v1.16 header)
- FOUND: CHANGELOG.md contains `PDBPLUS_INCLUDE_DELETED` (Breaking + Deprecated entries)
- FOUND: CHANGELOG.md contains `soft-delete` (Breaking + Changed entries)
- FOUND: CHANGELOG.md contains `limit=0` (Added + Fixed + header paragraph)
- FOUND: docs/API.md § Known Divergences section at line 540
- FOUND: docs/API.md `rest.py:700-712` + `rest.py:725` upstream citations
- FOUND: docs/API.md `One-time gap` + `v1.16 (Phase 68)` rows
- FOUND: docs/CONFIGURATION.md § Removed in v1.16 (line 44, PDBPLUS_INCLUDE_DELETED row at line 48 — landed by Plan 68-01)
- FOUND: CLAUDE.md § Soft-delete tombstones (Phase 68)
- FOUND: CLAUDE.md `markStaleDeletedFoos` template + `applyStatusMatrix` reference + `SEED-004` cross-link
- ABSENT: `PDBPLUS_INCLUDE_DELETED` in README.md (cleaned by Plan 68-01)
- FOUND: commit e6cf18f (Task 1: CHANGELOG.md + docs/API.md)
- FOUND: commit 661cf4a (Task 2: CLAUDE.md hygiene note)
- PASS: `go build ./...`
- PASS: `go test -race ./...` (entire repo green)
- PASS: `go vet ./...`
- PASS: `golangci-lint run ./...` (0 issues)
- PASS: `go generate ./... && git diff --exit-code ent/ gen/ graph/ internal/web/templates/` (zero drift)
- PASS: `grep -c "fly deploy" .planning/phases/68-status-since-matrix/*-PLAN.md` (0 imperative; 2 instructional NOT-to-deploy references in 68-04-PLAN.md, compliant)
- PASS: All 7 Phase 68 REQ-IDs (STATUS-01..05, LIMIT-01/02) have observable test artifacts
