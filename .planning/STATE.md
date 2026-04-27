---
gsd_state_version: 1.0
milestone: null
milestone_name: null
status: between-milestones
last_updated: "2026-04-27T18:00:00Z"
last_activity: 2026-04-27 -- Completed quick task 260427-ojm: pdbplus_data_type_count gauge live row counts
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-22)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## Current Position

**Between milestones.** v1.18.0 (Cleanup & Observability Polish) shipped + archived 2026-04-27. Six phases (73-78), 14 plans, 15 requirements all closed. Tag `v1.18.0` will be created at `/gsd-complete-milestone` end. No active milestone — start v1.19+ via `/gsd-new-milestone` when ready.

**Latest deploy:** commit `846c3df` (mid-Phase-77/78 deploy on 2026-04-27T04:11:12Z). All 8 fleet machines healthy. Phase 78's docs/tests-only changes don't require a separate deploy.

**Production state:** v1.17.0 release tag (pre-v1.18.0). Default sync mode = `incremental` running ~15min cadence on the primary. Fleet is the v1.15+ asymmetric configuration: 1 primary `lhr` (persistent volume) + 7 replicas `iad/lax/nrt/sin/syd/gru/jnb` (ephemeral, LiteFS HTTP cold-sync on boot).

## Outstanding Human Verification

All v1.18.0 Phase 78 items closed 2026-04-27 — see archived `.planning/milestones/v1.18.0-phases/78-uat-closeout/UAT-RESULTS.md`.

Remaining backlog (out of scope for v1.18.0; deferred to a future "UI verification sweep" milestone):

- v1.6 / v1.7 / v1.11 UI/visual items (~33 items combined). See `memory/project_human_verification.md`.

## Operator follow-ups (optional, non-blocking)

These were surfaced during v1.18.0 execution and are not required for milestone closure:

- **CSP enforcement flip.** Plan 78-02 verified the CSP policy autonomously (PASS). The operator can flip `PDBPLUS_CSP_ENFORCE=true` via `fly secrets set` at any maintenance window with no expected behavioural change. UAT-RESULTS.md provides the rollback recipe if any unexpected violations surface.
- **`OTEL_BSP_*` documentation drift.** `internal/otel/provider.go:54` comment claims env-var tunability but the explicit `WithBatchTimeout` / `WithMaxExportBatchSize` options override env defaults. Values are correct (5s/512); only the comment is wrong. Doc-only fix candidate for a future quick task.
- **Empirical Tempo trace size validation.** Phase 77 UAT Test 9 (max trace size <2 MB via TraceQL) is structurally argued but not directly empirical (the grafana-cloud MCP didn't expose Tempo TraceQL tools in the verifying session). Operator can validate via the Grafana UI when convenient using the queries listed in 77-UAT.md.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260427-ojm | replace OnSyncComplete `len(items)` with current-table `Count(ctx)` for `pdbplus_data_type_count` gauge | 2026-04-27 | 18b2337 | [260427-ojm-replace-onsynccomplete-len-items-with-cu](./quick/260427-ojm-replace-onsynccomplete-len-items-with-cu/) |

## Accumulated Context

### Seeds

- **SEED-001** — incremental sync evaluation. **Consumed 2026-04-26** by quick task 260426-pms (default `PDBPLUS_SYNC_MODE=incremental` shipped in v1.17.0).
- **SEED-003** — primary HA hot-standby. **Dormant.** Extended 2026-04-27 with the IAD-preferred-primary variant (cost analysis verified against `fly status`; correction that Consul leases are already configured; trigger added for sync-upstream-RTT regression). See `.planning/seeds/SEED-003-primary-ha-hot-standby.md`.
- **SEED-004** — tombstone GC. **Dormant.** Triggers haven't fired (storage growth <5% MoM, tombstone ratio <10%, no operator request). See `.planning/seeds/SEED-004-tombstone-gc.md`.

### Notes for next milestone

- Run `/gsd-new-milestone` to start v1.19+. Theme TBD — possible directions surfaced during v1.18.0 execution: UI verification sweep (~33 items), tag-and-release hygiene (the OTEL_BSP doc drift + similar comment-vs-code mismatches), or a feature cycle now that observability is solid.
- A fresh `.planning/REQUIREMENTS.md` will be created by `/gsd-new-milestone`. The v1.18.0 requirements are archived at `.planning/milestones/v1.18.0-REQUIREMENTS.md`.
