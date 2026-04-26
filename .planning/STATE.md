---
gsd_state_version: 1.0
milestone: null
milestone_name: null
status: ready
stopped_at: "v1.16 ARCHIVED 2026-04-22. 6 phases (67-72), 36 plans, 25 requirements complete. Ready for v1.18.0 (the v1.17.0 tag was used by quick task 260426-pms for the SEED-001 incremental-sync flip — not a milestone)."
last_updated: "2026-04-22T17:15:00.000Z"
last_activity: 2026-04-22
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

**Current focus:** Ready for v1.18.0 milestone definition. v1.16 Django-compat Correctness archived 2026-04-22; the v1.17.0 release tag was used by quick task 260426-pms (SEED-001 incremental-sync default flip) — not a milestone.

## Current Position

Phase: none
Plan: none
Status: Ready for /gsd-new-milestone to start v1.18.0.
Last activity: 2026-04-26 - Completed quick task 260426-pms: flip default PDBPLUS_SYNC_MODE to incremental (SEED-001), shipped as v1.17.0

## v1.18.0 Phase Map

_No phases defined for v1.18.0 yet._

## Locked Decisions (abbreviated)

_No active milestone decisions._

## Recently Shipped

**v1.16 Django-compat Correctness** — shipped 2026-04-19, archived 2026-04-22. 6 phases (67-72), 25 requirements. `pdbcompat` surface now has full behavioural parity with upstream PeeringDB.

**v1.15 Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18. 4 phases (63-66), 11 requirements.

**v1.14 Authenticated Sync & Visibility Layer** — 6 phases (57-62), 21 plans, 17/17 requirements.

## Outstanding Human Verification

Deferred items tracked for manual confirmation:

- **Phase 52 (v1.13):** Chrome devtools CSP check on `/ui/`, `/ui/asn/13335`, `/ui/compare`
- **Phase 53 (v1.13):** curl HSTS / X-Frame-Options / X-Content-Type-Options headers, 2 MB body-cap REST vs gRPC skip-list, slowloris TCP smoke test

See `memory/project_human_verification.md` for the full backlog.

## Accumulated Context

### Seeds

- **SEED-001** — incremental sync evaluation. **Consumed 2026-04-26** by quick task 260426-pms (default `PDBPLUS_SYNC_MODE=incremental` shipped in v1.17.0).
- **SEED-002** — asymmetric Fly fleet. **Consumed** by v1.15 Phase 65.
- **SEED-003** — primary HA hot-standby. Dormant.
- **SEED-004** — tombstone garbage collection. **Planted 2026-04-19**.

### Pending Todos

None.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |
| 20260417-v114-lint-cleanup | Clear 7 golangci-lint findings post-v1.14 | 2026-04-17 | d15dd02 | [20260417-v114-lint-cleanup](./quick/20260417-v114-lint-cleanup/) |
| 260418-1cn | Add sqlite3 to Dockerfile.prod + fly deploy | 2026-04-18 | 4dfc52a | [260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de](./quick/260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de/) |
| 260418-gf0 | Fix pdb-schema-generate — resolves backlog 999.1 | 2026-04-18 | 73bbe04 | [260418-gf0-fix-pdb-schema-generate-preserve-policy-](./quick/260418-gf0-fix-pdb-schema-generate-preserve-policy-/) |
| 260419-ski | Auth-conditional PDBPLUS_SYNC_INTERVAL default | 2026-04-19 | c242d90 | [260419-ski-auth-sync-interval](./quick/260419-ski-auth-sync-interval/) |
| 20260420-esb | Ent schema siblings refactor | 2026-04-20 | 559b5fa | [20260420-esb-ent-schema-siblings](./quick/20260420-esb-ent-schema-siblings/) |
| 20260422-remove-nightly-bench | Remove the "Nightly bench" GitHub Actions workflow | 2026-04-22 | 3ca8590 | [20260422-remove-nightly-bench](./quick/20260422-remove-nightly-bench/) |
| 260426-jke | Observability cleanup — log noise, trace overflow, redundant metric names, dashboard repair | 2026-04-26 | cef357a | [260426-jke-obs-cleanup](./quick/260426-jke-obs-cleanup/) |
| 260426-lod | Observability label gaps — GC-allowlisted resource attrs (service.namespace/cloud.region) + http.route via post-dispatch labeler | 2026-04-26 | cf1b558 | [260426-lod-observability-label-gaps-gc-allowlisted-](./quick/260426-lod-observability-label-gaps-gc-allowlisted-/) |
| 260426-mei | Production observability — Prom alert rules (6, 2-tier severity) + dashboard panel 35 re-sourced to per-instance go.memory.used | 2026-04-26 | d2d337d | [260426-mei-production-observability-alerts-per-inst](./quick/260426-mei-production-observability-alerts-per-inst/) |
| 260426-pms | Flip default PDBPLUS_SYNC_MODE to incremental (SEED-001) — codegen-layer NotEmpty() drop on `name` for 6 folded entities + tombstone conformance test + sync metrics `mode` label | 2026-04-26 | a3ae545 | [260426-pms-flip-default-pdbplus-sync-mode-to-increm](./quick/260426-pms-flip-default-pdbplus-sync-mode-to-increm/) |

## Session Continuity

Milestone v1.16 completed and archived. Ready for v1.18.0 definition. (v1.17.0 was used as a release tag for the SEED-001 incremental-sync flip via quick task 260426-pms — not a milestone, so the next milestone bumps to v1.18.0.)
