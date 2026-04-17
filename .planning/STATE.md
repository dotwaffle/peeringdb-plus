---
gsd_state_version: 1.0
milestone: v1.15
milestone_name: "Infrastructure Polish & Schema Hygiene"
status: Roadmap defined
stopped_at: Roadmap defined for v1.15
last_updated: "2026-04-17T00:00:00Z"
last_activity: "2026-04-17 — v1.15 roadmap created"
progress:
  total_phases: 4
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-17)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Current focus:** v1.15 Infrastructure Polish & Schema Hygiene — 4 phases (63-66), 11 requirements. No new user-facing features. See `.planning/ROADMAP.md`.

## Current Position

**Milestone:** v1.15 Infrastructure Polish & Schema Hygiene
**Phase:** none started — roadmap just defined
**Next action:** pick a phase (63, 64, 65, or 66) and run `/gsd-discuss-phase` or `/gsd-plan-phase`.

**Phases (63-66):**

- **Phase 63** — Schema hygiene: drop 3 vestigial ent fields (`ixprefix.notes`, `organization.fac_count`, `organization.net_count`). Also resolves v1.14-deferred `ixpfx.notes` pdbcompat divergence. Requirements: HYGIENE-01, HYGIENE-02, HYGIENE-03. ~1 plan.
- **Phase 64** — Field-level privacy: add `ixlan.ixf_ixp_member_list_url` URL data field across struct + schema + sync + serializer; establish serializer-layer field-level redaction pattern across 5 surfaces. Requirements: VIS-08, VIS-09. ~2-3 plans.
- **Phase 65** — Asymmetric Fly fleet: activate SEED-002. Process groups + ephemeral replicas. Requirements: INFRA-01, INFRA-02, INFRA-03. ~2-3 plans.
- **Phase 66** — Observability + sqlite3 tooling: prod `sqlite3` binary, heap-threshold monitoring, Grafana heap panel, SEED-001 escalation docs. Requirements: OBS-04, OBS-05, DOC-04. ~1-2 plans.

**Parallelism:** Phases 65 and 66 can run in parallel with 63/64 (no file overlap). Phase 64 is trivially sequenced after Phase 63 (same codegen pipeline).

## Recently Shipped

**v1.14 Authenticated Sync & Visibility Layer** — 6 phases (57-62), 21 plans, 17/17 requirements, audit PASSED.

- **Commit range:** `8511805..c496b72` (132 commits)
- **Files changed:** 258 files, +164243 / -373 LOC (bulk is Phase 57 baseline fixture commits)
- **Timeline:** 2026-04-16 → 2026-04-17
- **Archive:** [`.planning/milestones/v1.14-ROADMAP.md`](./milestones/v1.14-ROADMAP.md)
- **Requirements archive:** [`.planning/milestones/v1.14-REQUIREMENTS.md`](./milestones/v1.14-REQUIREMENTS.md)
- **Audit:** [`.planning/v1.14-MILESTONE-AUDIT.md`](./v1.14-MILESTONE-AUDIT.md)

Post-v1.14 follow-ups surfaced in v1.15:

- `ixpfx.notes` pdbcompat divergence → resolved by Phase 63 (drop the field).
- Missing `ixlan.ixf_ixp_member_list_url` URL data field → closed by Phase 64.
- Peak-heap watch story tied to SEED-001 → Phase 66 makes the trigger observable and documents the escalation path.

## Outstanding Human Verification

Deferred items tracked for manual confirmation:

- **Phase 52 (v1.13):** Chrome devtools CSP check on `/ui/`, `/ui/asn/13335`, `/ui/compare`
- **Phase 53 (v1.13):** curl HSTS / X-Frame-Options / X-Content-Type-Options headers, 2 MB body-cap REST vs gRPC skip-list, slowloris TCP smoke test

Phase 57 + Phase 62 (v1.14) UAT items all resolved 2026-04-17 — see `.planning/phases/57-visibility-baseline-capture/57-HUMAN-UAT.md` and `.planning/phases/62-api-key-default-docs/62-HUMAN-UAT.md` (both `status: resolved`).

See `memory/project_human_verification.md` for the full backlog across v1.6, v1.7, v1.11, v1.13.

## Accumulated Context

### Decisions

All decisions archived in PROJECT.md Key Decisions table (46+ decisions across 15 milestones).

v1.14 decisions captured in PROJECT.md rows added during Phase 58 (schema sufficiency, `<field>_visible` naming, NULL-as-schema-default, regression test locks empirical assumption).

v1.15 decisions pending — to be logged at phase transitions. Likely candidates:

- Field-level redaction substrate location (package choice: `internal/privfield` vs extend `internal/privctx` vs per-serializer helpers)
- Asymmetric Fly fleet process-group names (`primary` / `replica` chosen per SEED-002)
- Heap-threshold surfacing mechanism (`/readyz` degraded signal vs OTel span attribute — TBD in Phase 66 plan)

### Seeds

- **SEED-001** — incremental sync evaluation. Dormant. No trigger fired (peak heap ~84 MB vs 380 MiB). Phase 66 wires the trigger observability.
- **SEED-002** — asymmetric Fly fleet. **Activated** 2026-04-17 for v1.15 Phase 65. Moves to `.planning/seeds/consumed/` when Phase 65 ships.

### Pending Todos

None. Check `.planning/HANDOFF.json` and `memory/` for parked ideas.

### Blockers/Concerns

None. All 4 phases have clear scope, requirements, and success criteria.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |
| 20260417-v114-lint-cleanup | Clear 7 golangci-lint findings post-v1.14 (gosec/exhaustive/nolintlint/revive); resolves Phase 58 deferred-items.md | 2026-04-17 | d15dd02 | [20260417-v114-lint-cleanup](./quick/20260417-v114-lint-cleanup/) |

## Session Continuity

Last session: 2026-04-17
Stopped at: Roadmap defined for v1.15
Resume: pick a phase (63-66) and run `/gsd-discuss-phase` or `/gsd-plan-phase`.
