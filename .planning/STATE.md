---
gsd_state_version: 1.0
milestone: v1.13
milestone_name: Security & Sync Hardening
status: shipped
stopped_at: v1.13 shipped 2026-04-11
last_updated: "2026-04-14T21:10:00Z"
last_activity: 2026-04-14
progress:
  total_phases: 6
  completed_phases: 6
  total_plans: 16
  completed_plans: 16
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-26)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Current focus:** None — v1.13 shipped. Run `/gsd-new-milestone` to begin the next cycle.

## Current Position

Milestone: v1.13 Security & Sync Hardening (shipped 2026-04-11)
Phase: all 6 complete (51-56)
Status: idle
Last activity: 2026-04-14 - Quick task 260414-2rc (OTel metric cardinality, PR #11); search ASN-literal fix (99ce22b); generated 8 canonical docs under docs/ + CONTRIBUTING.md (ac6e330); CLAUDE.md now points at the new docs and documents the /ui/ ANSI curl gotcha

Progress (v1.13): [██████████] 100%
Cumulative shipped: 56 phases across v1.0-v1.13.

## Recently Shipped

**v1.13 Security & Sync Hardening** — 6 phases, 16 plans, 16 requirements (SEC-04 through SEC-11, PERF-04 through PERF-07, REFAC-03/04/05, DEBT-03). Merged via PR #8 (commit 18d3735) on 2026-04-11. Key outcomes:

- Security hardening: constant-time sync token compare, strict PeeringDB URL validator, parseASN via ParseUint, CSP enforcement feature flag, ReadTimeout=30s, generic /readyz body, global 1 MB body cap, HSTS/XFO/XCTO headers
- Sync worker overhaul: Phase A/B split with fetch barrier, streaming JSON decoder, scratch-SQLite fallback, 535 → 380 MiB peak heap under 400 MiB gate
- Perf: delta streams skip COUNT preflight, ETag cache wired through OnSyncComplete
- Filter consolidation: generic filterFn[REQ] runner with zero any-boxing on hot paths
- Production incident fix: dropped UNIQUE(organizations.name), added Retry-After handling (12 → 1 req/hour on /api/org)

Post-milestone follow-ups already merged into main:
- #9 Go 1.26 modernization pass (2026-04-11)
- #10 fix(sync): reap stale running rows on primary startup (2026-04-11)
- #11 feat(otel): reduce metric cardinality ~30-55% (2026-04-14, quick task 260414-2rc)
- 99ce22b fix(search): match networks by ASN literal in /ui and /api/net (2026-04-14, direct to main)
- ac6e330 docs: generate 8 canonical docs (ARCHITECTURE/CONFIGURATION/GETTING-STARTED/DEVELOPMENT/TESTING/API/DEPLOYMENT/CONTRIBUTING) via /gsd-docs-update — 228/229 claims verified against the live codebase (2026-04-14)

## Outstanding Human Verification

Deferred from v1.13 autonomous run, tracked for manual confirmation:

- **Phase 52:** Chrome devtools CSP check on `/ui/`, `/ui/asn/13335`, `/ui/compare` with `PDBPLUS_CSP_ENFORCE=true` (Tailwind v4 JIT runtime behaviour)
- **Phase 53:** curl HSTS / X-Frame-Options / X-Content-Type-Options headers, 2 MB body-cap REST vs gRPC skip-list, slowloris TCP smoke test

See `memory/project_human_verification.md` for the full backlog across v1.6, v1.7, v1.11, and v1.13.

## Accumulated Context

### Decisions

All decisions archived in PROJECT.md Key Decisions table (42+ decisions across 13 milestones).

### Pending Todos

None on main. Check `.planning/HANDOFF.json` and `memory/` for parked ideas.

### Blockers/Concerns

None.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |

## Session Continuity

Last session: 2026-04-14
Stopped at: v1.13 shipped — no active milestone work; post-ship polish continues directly to main (search fix, doc generation, CLAUDE.md upkeep)
Resume file: None
