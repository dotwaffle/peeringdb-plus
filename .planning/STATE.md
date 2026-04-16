---
gsd_state_version: 1.0
milestone: v1.14
milestone_name: Authenticated Sync & Visibility Layer
status: Roadmap defined
stopped_at: Phase 57-62 contexts gathered
last_updated: "2026-04-16T19:50:34.649Z"
last_activity: 2026-04-16 — ROADMAP.md created mapping 17 requirements to 6 phases (57-62)
progress:
  total_phases: 6
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-16)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Current focus:** v1.14 Authenticated Sync & Visibility Layer — close the privacy hole that would otherwise leak `Users`-tier POCs once an API key is enabled, then make authenticated sync the recommended deployment.

## Current Position

Milestone: v1.14 Authenticated Sync & Visibility Layer
Phase: 57 (not yet started — roadmap defined, awaiting `/gsd-discuss-phase 57`)
Plan: —
Status: Roadmap defined
Last activity: 2026-04-16 — ROADMAP.md created mapping 17 requirements to 6 phases (57-62)

Progress (v1.14): [          ] 0% (0/6 phases complete)
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

v1.14 scope decisions captured during `/gsd-new-milestone` (will move to PROJECT.md Key Decisions as phases land):

- OAuth deferred to v1.15 — PeeringDB OAuth is identity-only and a clean follow-on milestone after the privacy floor exists
- Phase 57 baseline capture: beta first, prod confirmation for high-signal types (poc, org, net)
- Authenticated sync becomes the recommended deployment after v1.14 ships
- New env var `PDBPLUS_PUBLIC_TIER` (default `public`) — when set to `users`, anonymous callers are treated as Users-tier (private-instance escape hatch); WARN at startup
- No-key sync remains a first-class supported configuration

v1.14 roadmap decisions (2026-04-16):

- 6 phases (57-62), serial dependency chain except Phases 60 and 61 which can run in parallel after Phase 59 lands (60 = test files per surface, 61 = startup/about/OTel — no file overlap)
- Phase 57 is rate-limit-bound (≥ 1 hour wall-clock) — explicit sleeps + resumability mandatory; not a planning artifact to be compressed
- Phase 59 is the gate for everything visible to users — VIS-04, VIS-05, SYNC-03 land together so the policy + sync bypass + tier override come up as one coherent surface
- Phase 62 is intentionally minimal-code (fly secret + 3 doc edits + manual verification) — flagged in roadmap to prevent over-planning

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

Last session: 2026-04-16T19:50:34.643Z
Stopped at: Phase 57-62 contexts gathered
Resume file: .planning/phases/57-visibility-baseline-capture/57-CONTEXT.md
