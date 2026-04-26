---
gsd_state_version: 1.0
milestone: v1.18.0
milestone_name: Cleanup & Observability Polish
status: planning
last_updated: "2026-04-26T20:11:03.192Z"
last_activity: 2026-04-26
progress:
  total_phases: 6
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-22)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

**Current focus:** v1.18.0 roadmap created 2026-04-26. 6 phases (73-78), 15 requirements (BUG/TEST/OBS/UAT). Theme is cleanup & observability polish — no new feature scope. Ready for `/gsd-plan-phase 73`.

## Current Position

Phase: Not started (roadmap defined)
Plan: —
Status: Ready for `/gsd-plan-phase 73`
Last activity: 2026-04-26 — v1.18.0 ROADMAP.md written, REQUIREMENTS.md traceability filled, 15/15 requirements mapped

## v1.18.0 Phase Map

| Phase | Goal | Requirements | Depends on |
|-------|------|--------------|------------|
| 73 — Code Defect Fixes | Fix campus inflection 500 + drop `poc.role` NotEmpty validator | BUG-01, BUG-02 | — |
| 74 — Test & CI Debt | Three deferred test failures + 5 lint findings cleared | TEST-01, TEST-02, TEST-03 | — |
| 75 — Code-side Observability Fixes | Cold-start gauge, zero-rate counter pre-warm, http.route middleware | OBS-01, OBS-02, OBS-04 | — |
| 76 — Dashboard Hardening | `service_name` filter sweep + post-canonicalisation metric flow confirmation | OBS-03, OBS-05 | Phase 75 (soft) |
| 77 — Telemetry Audit & Cleanup | Loki log-level audit + Tempo trace sampling/batching review | OBS-06, OBS-07 | Phase 75 |
| 78 — UAT Closeout | v1.13 CSP + headers verification, v1.5 Phase 20 archive | UAT-01, UAT-02, UAT-03 | — |

Phase numbering continues from v1.16 (last phase 72). v1.17.0 was a release tag (quick task 260426-pms), not a milestone — no phases consumed.

## Locked Decisions (abbreviated)

_No active milestone decisions yet — Phase 73 CONTEXT lock pending `/gsd-plan-phase 73`._

## Recently Shipped

**v1.16 Django-compat Correctness** — shipped 2026-04-19, archived 2026-04-22. 6 phases (67-72), 25 requirements. `pdbcompat` surface now has full behavioural parity with upstream PeeringDB.

**v1.15 Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18. 4 phases (63-66), 11 requirements.

**v1.14 Authenticated Sync & Visibility Layer** — 6 phases (57-62), 21 plans, 17/17 requirements.

## Outstanding Human Verification

Tracked under v1.18.0 Phase 78 (UAT-01, UAT-02, UAT-03):

- **Phase 52 (v1.13):** Chrome devtools CSP check on `/ui/`, `/ui/asn/13335`, `/ui/compare` — under UAT-01
- **Phase 53 (v1.13):** curl HSTS / X-Frame-Options / X-Content-Type-Options headers, 2 MB body-cap REST vs gRPC skip-list, slowloris TCP smoke test — under UAT-02
- **Phase 20 (v1.5):** stale deferred-items pointer dir relocation — under UAT-03

See `memory/project_human_verification.md` for the full backlog (the v1.6/v1.7/v1.11 ~33 UI/visual items are explicitly out of scope per REQUIREMENTS.md and deferred to a future "UI verification sweep" milestone).

## Accumulated Context

### Seeds

- **SEED-001** — incremental sync evaluation. **Consumed 2026-04-26** by quick task 260426-pms (default `PDBPLUS_SYNC_MODE=incremental` shipped in v1.17.0).
- **SEED-002** — asymmetric Fly fleet. **Consumed** by v1.15 Phase 65.
- **SEED-003** — primary HA hot-standby. Dormant.
- **SEED-004** — tombstone garbage collection. **Planted 2026-04-19**. Explicitly out of scope for v1.18.0 per user 2026-04-26 ("data volume tiny").

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

v1.18.0 ROADMAP.md written 2026-04-26 — 6 phases (73-78), 15/15 requirements mapped, no orphans, no duplicates. REQUIREMENTS.md traceability table filled. Ready for `/gsd-plan-phase 73` (Code Defect Fixes — BUG-01 campus inflection + BUG-02 `poc.role` NotEmpty drop).
