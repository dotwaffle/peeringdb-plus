---
phase: 71-memory-safe-response-paths
plan: 06
subsystem: docs
tags: [docs, changelog, claude-md, requirements, roadmap, phase-close]
requires: [71-01, 71-02, 71-03, 71-04, 71-05]
provides:
  - "docs/ARCHITECTURE.md § Response Memory Envelope"
  - "CHANGELOG.md v1.16 [Unreleased] § Phase 71 bullets + coordinated-release note"
  - "CLAUDE.md § Response memory envelope (Phase 71) convention + PDBPLUS_RESPONSE_MEMORY_LIMIT env-var row"
  - "REQUIREMENTS.md MEMORY-01..04 all complete with grep-verifiable artefacts"
  - "ROADMAP.md Phase 71 closed (6/6 plans, Progress row Complete 2026-04-19)"
affects:
  - docs/ARCHITECTURE.md
  - docs/CONFIGURATION.md (verified — Plan 03 row still present)
  - CHANGELOG.md
  - CLAUDE.md
  - .planning/REQUIREMENTS.md
  - .planning/ROADMAP.md
key-files:
  created: []
  modified:
    - docs/ARCHITECTURE.md
    - CHANGELOG.md
    - CLAUDE.md
    - .planning/REQUIREMENTS.md
    - .planning/ROADMAP.md
decisions:
  - "Per-entity max_rows values computed at 128 MiB default and included in the ARCHITECTURE.md table — derived from rowsize.go, not duplicated as a second source-of-truth (table header cites rowsize.go as canonical)."
  - "CHANGELOG Phase 71 Added block lives below Phase 70's Added block, preserving the existing phase-ordered layout; coordinated-release callout moved to the TOP of [Unreleased] so the deploy-readiness signal is the first thing reviewers see."
  - "CLAUDE.md env-var table row slotted immediately after PDBPLUS_RSS_WARN_MIB (adjacent memory-related knob) — matches the pattern established by Phase 66 (PDBPLUS_HEAP_WARN_MIB/PDBPLUS_RSS_WARN_MIB clustering)."
  - "REQUIREMENTS.md MEMORY-04 row carries pointers to all 3 doc artefacts (ARCHITECTURE, CLAUDE, CHANGELOG) instead of 1 — lets a future auditor grep any of the three."
metrics:
  duration: "~45 minutes"
  completed: 2026-04-19
  tasks: 2
  commits: 2
  files_modified: 5
  files_created: 0
  loc_delta_additions: 228
  loc_delta_deletions: 9
---

# Phase 71 Plan 06: Close Phase 71 — docs + traceability

## One-liner

Docs-only phase close: `docs/ARCHITECTURE.md § Response Memory Envelope`
with per-entity max_rows table, plus CHANGELOG v1.16 Phase 71 bullets,
CLAUDE.md convention subsection, and REQUIREMENTS/ROADMAP flips that
mark the phase complete with all four MEMORY REQ-IDs grep-verifiable.

## Tasks

### Task 1 — `docs/ARCHITECTURE.md § Response Memory Envelope`

Inserted a new top-level section after `## OpenTelemetry instrumentation`.
Content:

- **The envelope** — derivation formula `256 MB replica − 80 MB Go
  baseline − 48 MB slack = 128 MiB PDBPLUS_RESPONSE_MEMORY_LIMIT default`.
- **The three moving parts** table — `stream.go` / `rowsize.go` /
  `budget.go` with one-line responsibility each.
- **Per-entity worst-case sizing** — 13-row table with columns
  `Depth=0 bytes/row` · `Max rows @ 128 MiB (D=0)` ·
  `Depth=2 bytes/row` · `Max rows @ 128 MiB (D=2)`. Values pulled from
  `internal/pdbcompat/rowsize.go` 2026-04-19 calibration; max_rows
  computed as `128 × 1024 × 1024 / bytes_per_row` (floor).
- **Request lifecycle** — 6-step flow from `GET /api/<type>?limit=0` to
  `StreamListResponse` flush every 100 rows.
- **Telemetry (MEMORY-03)** — points at `pdbplus.response.heap_delta_kib`
  OTel span attr, `pdbplus_response_heap_delta_kib` histogram, and
  Grafana panel id 36.
- **Out of scope** — D-07 pdbcompat-only scope with explicit notes on
  grpcserver / entrest / GraphQL / Web UI memory stories.
- **Extending** — 4-step checklist for adding a new entity type.

Commit: `9ddc8a6`.

### Task 2 — CHANGELOG + CLAUDE + REQUIREMENTS + ROADMAP

- **CHANGELOG.md** — two additions to `[Unreleased] — v1.16`:
  - Blockquote near the top: "Coordinated release window: v1.16 phases
    67-71 are now complete and ready to deploy as a bundle. `limit=0`
    unbounded semantics (Phase 68) are safe in prod only with the
    Phase 71 memory budget in place — do NOT ship 67-70 without 71.
    Phase 72 (parity regression test lock-in) ships independently as
    a follow-up."
  - Under `### Added`, a full Phase 71 block covering streaming JSON
    emission, `PDBPLUS_RESPONSE_MEMORY_LIMIT` + RFC 9457 413 pre-flight
    check, per-request heap-delta telemetry, and the ARCHITECTURE doc
    pointer. Closes MEMORY-01..04.
- **CLAUDE.md** — new `### Response memory envelope (Phase 71)`
  subsection after `### Cross-entity __ traversal (Phase 70)`, before
  `### Middleware`. Mirrors the Phase 66 / 68 / 70 prior-art template:
  - Prose: where the code lives, the 3 moving parts, the 128 MiB
    default derivation, per-entity row sizing, pairing ListFunc with
    CountFunc via shared predicates helper, telemetry discipline
    (single `ReadMemStats` call site).
  - Maintainer checklist: 4 steps to add a new entity type
    (rowsize → registry → ARCHITECTURE table → integration test).
  - Do NOT list: 5 entries covering per-row `ReadMemStats` prohibition,
    trusted-entity exemption prohibition, per-endpoint budget override
    prohibition, Count/List predicate divergence prohibition, scope
    creep prohibition (no entrest/grpcserver/GraphQL extension).
  - Env-var table row for `PDBPLUS_RESPONSE_MEMORY_LIMIT` slotted after
    `PDBPLUS_RSS_WARN_MIB`.
- **REQUIREMENTS.md** — flipped `MEMORY-04` row from pending to
  complete with a multi-artefact pointer set (ARCHITECTURE + CLAUDE +
  CHANGELOG). All four MEMORY REQ-IDs now show `| 71 | complete (…)`
  with grep-verifiable file references.
- **ROADMAP.md**:
  - Top-level phase list: Phase 71 checkbox flipped `[ ]` → `[x]`.
  - Phase 71 plan list: 71-01/02/03/06 checkboxes flipped `[ ]` →
    `[x]`; plan synopses filled in with file refs; header changed
    from "6 plans" to "6/6 plans executed".
  - Progress table: row flipped from `3/6 | In progress | -` to
    `6/6 | Complete | 2026-04-19`.

Commit: `6e9ea3a`.

## Deviations from Plan

None — plan executed exactly as written. Two minor in-plan judgement calls:

1. The per-entity `max_rows` column in the ARCHITECTURE table was
   computed at execute time (Python one-liner against the 13
   `typicalRowBytes` pairs from `internal/pdbcompat/rowsize.go`)
   rather than hand-transcribed from an external calibration report.
   The rowsize.go godoc already carries the raw bench output; the
   doc table cites rowsize.go as canonical so there is no
   double-source-of-truth drift risk.
2. The CLAUDE.md env-var row was placed adjacent to the existing
   `PDBPLUS_RSS_WARN_MIB` row rather than alphabetical — mirrors the
   Phase 66 memory-knob clustering (sync_memory_limit / heap_warn /
   rss_warn already live side by side).

## Verification

All 12 plan verification criteria pass:

| # | Check | Expected | Actual |
|---|-------|----------|--------|
| 1 | `grep -c "Response Memory Envelope" docs/ARCHITECTURE.md` | >= 1 | 1 |
| 2 | `grep -c "Phase 71" CHANGELOG.md` | >= 1 | 5 |
| 3 | `grep -c "ready to deploy" CHANGELOG.md` | >= 1 | 1 |
| 4 | `grep -c "Response memory envelope (Phase 71)" CLAUDE.md` | == 1 | 1 |
| 5 | `grep -c "PDBPLUS_RESPONSE_MEMORY_LIMIT" CLAUDE.md` | >= 1 | 2 |
| 6 | `grep -cE "MEMORY-0[1234] \| 71 \| complete" REQUIREMENTS.md` | == 4 | 4 |
| 7 | `grep -cF "[x] **Phase 71" ROADMAP.md` | == 1 | 1 |
| 8 | `grep -c "71-06-PLAN.md" ROADMAP.md` | == 1 | 1 |
| 9 | `grep "71. Memory-safe response paths \| 6/6 \| Complete"` | match | matches |
| 10 | `go build ./...` | clean | clean |
| 11 | `go test ./... -short -count=1` | all pass | all pass |
| 12 | `grep "fly deploy"` imperative uses in plan | 0 | 0 (3 instructional refs only, all documented) |

## Self-Check: PASSED

Created files: none (docs-only edit).
Modified files: 5 (docs/ARCHITECTURE.md, CHANGELOG.md, CLAUDE.md,
.planning/REQUIREMENTS.md, .planning/ROADMAP.md) — all verified via
`git status --short`.
Commits: `9ddc8a6` (ARCHITECTURE) and `6e9ea3a` (traceability close) —
both verified via `git log --oneline`.
