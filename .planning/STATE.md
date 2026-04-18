---
gsd_state_version: 1.0
milestone: v1.15
milestone_name: — Infrastructure Polish & Schema Hygiene
status: Executing Phase 64
stopped_at: /gsd-autonomous running — pre-phase quick task done, about to start Phase 63.
last_updated: "2026-04-18T03:37:08.169Z"
last_activity: 2026-04-18
progress:
  total_phases: 5
  completed_phases: 1
  total_plans: 4
  completed_plans: 1
  percent: 25
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-17)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Current focus:** Phase 64 — Field-level privacy — ixlan.ixf_ixp_member_list_url

## Current Position

Phase: 64 (Field-level privacy — ixlan.ixf_ixp_member_list_url) — EXECUTING
Plan: 1 of 3
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
- **SEED-003** — primary HA hot-standby. Dormant. Planted 2026-04-17 when the v1.15 decision was made to keep LHR as sole primary candidate. Triggers: LHR extended outage, maintenance burden, compliance, Fly capacity pressure.

### Pre-phase quick task (do this BEFORE Phase 65)

- **sqlite3 in prod image** — ✅ **Done 2026-04-18 (quick task `260418-1cn`, commit `4dfc52a`).** `Dockerfile.prod` now installs `sqlite` alongside `fuse3`; deployed to all 8 machines; verified `sqlite3 /litefs/peeringdb-plus.db ".tables"` on LHR primary + NRT replica. Phase 65 fleet migration is unblocked.

### Pending Todos

None.

### Blockers/Concerns

None. All 4 phases have locked CONTEXT.md with full discuss-phase decisions captured (commit `e1fa426`) — `/gsd-autonomous` or `/gsd-plan-phase` can proceed straight to planning without re-asking grey areas.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |
| 20260417-v114-lint-cleanup | Clear 7 golangci-lint findings post-v1.14 (gosec/exhaustive/nolintlint/revive); resolves Phase 58 deferred-items.md | 2026-04-17 | d15dd02 | [20260417-v114-lint-cleanup](./quick/20260417-v114-lint-cleanup/) |
| 260418-1cn | Add sqlite3 to Dockerfile.prod + fly deploy + verify on primary & replica (pre-Phase-65 prep) | 2026-04-18 | 4dfc52a | [260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de](./quick/260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de/) |

## Session Continuity

Last session: 2026-04-18
Last activity: 2026-04-18
Stopped at: /gsd-autonomous running — pre-phase quick task done, about to start Phase 63.

**Resume via `/gsd-autonomous`:**

Each phase has `has_context: true` so the autonomous workflow skips discuss-phase and goes straight to plan → execute for each of phases 63-66. Execution order and key locked decisions:

1. ~~Pre-phase quick task — sqlite3 in prod image~~ ✅ done (quick task `260418-1cn`, commit `4dfc52a`).
2. **Phase 63 — Schema hygiene.** Drop 3 vestigial fields (full removal). Planner MUST read entgo.io/docs/migrate/ to verify whether `schema.WithDropColumn(true)` flag is needed for ent to emit `ALTER TABLE DROP COLUMN` (D-04 in 63-CONTEXT.md).
3. **Phase 64 — Field-level privacy.** New `internal/privfield` package with fail-closed default. Serializer-layer redaction across 5 surfaces. Omit key entirely for anon. Leave `_visible` companion exposed.
4. **Phase 65 — Asymmetric Fly fleet.** Big-bang rollout; sqlite3 now in the image. Scripted volume cleanup inline in SUMMARY. Primary HA stays LHR-only (SEED-003 captures future work).
5. **Phase 66 — Observability + sqlite3.** Both OTel attrs + slog.Warn on heap threshold. Defaults: `PDBPLUS_HEAP_WARN_MIB=400`, `PDBPLUS_RSS_WARN_MIB=384`. In-repo grafana/pdbplus-overview.json update.

**Fleet memory baseline for reference (observed 2026-04-17, do not re-gather):**

- Primary (LHR): RSS 68.8 MB, peak VmHWM 83.8 MB
- Replicas (7 regions): ~58-59 MB steady
- DB size: 88 MB

**Autonomous entry command:** `/gsd-autonomous` — no flags needed; it'll discover Phase 63 as next incomplete and walk through.
