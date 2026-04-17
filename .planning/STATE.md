---
gsd_state_version: 1.0
milestone: none
milestone_name: ""
status: milestone complete
stopped_at: v1.14 archived
last_updated: "2026-04-17T00:00:00Z"
last_activity: "2026-04-17 — v1.14 Authenticated Sync & Visibility Layer shipped"
progress:
  total_phases: 0
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-17)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Current focus:** None — v1.14 shipped. Run `/gsd-new-milestone` to start the next cycle.

## Recently Shipped

**v1.14 Authenticated Sync & Visibility Layer** — 6 phases (57-62), 21 plans, 17/17 requirements, audit PASSED.

- **Commit range:** `8511805..c496b72` (132 commits)
- **Files changed:** 258 files, +164243 / -373 LOC (bulk is Phase 57 baseline fixture commits)
- **Timeline:** 2026-04-16 → 2026-04-17
- **Archive:** [`.planning/milestones/v1.14-ROADMAP.md`](./milestones/v1.14-ROADMAP.md)
- **Requirements archive:** [`.planning/milestones/v1.14-REQUIREMENTS.md`](./milestones/v1.14-REQUIREMENTS.md)
- **Audit:** [`.planning/v1.14-MILESTONE-AUDIT.md`](./v1.14-MILESTONE-AUDIT.md)

Execution summary:

- **Phase 57** — Empirical visibility baseline captured (fixtures + diff committed; PII guard PASS)
- **Phase 58** — Schema alignment regression test against diff.json; no new ent fields needed
- **Phase 59** — ent Privacy policy + sync bypass (FeaturePrivacy, poc.Policy() with NULL defense, single-call-site bypass, PDBPLUS_PUBLIC_TIER env var, internal/privctx package, PrivacyTier middleware, D-15 E2E across 5 surfaces; introduced ent/schematypes to break import cycle)
- **Phase 60** — Surface integration tests (seed.Full mixed visibility; per-surface leak tests; pdbcompat anon parity via 13-type fixture replay; conformance anon-vs-anon; no-key sync test)
- **Phase 61** — Operator observability (startup sync-mode slog + WARN on override; /about Privacy & Sync section; OTel pdbplus.privacy.tier attr)
- **Phase 62** — API key default & docs (4 docs updated; fly secret rollout + smoke test verified on production)

Post-milestone follow-ups: none yet. Next milestone: TBD (v1.15 OAuth is the canonical successor per v1.14 scope decisions).

**v1.13 Security & Sync Hardening** — 6 phases, 16 plans, 16 requirements. Merged via PR #8 (commit 18d3735) on 2026-04-11.

Post-v1.13 follow-ups merged into main:

- #9 Go 1.26 modernization pass
- #10 fix(sync): reap stale running rows on primary startup
- #11 feat(otel): reduce metric cardinality ~30-55%
- 99ce22b fix(search): match networks by ASN literal
- ac6e330 docs: generate 8 canonical docs

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

### Pending Todos

None. Check `.planning/HANDOFF.json` and `memory/` for parked ideas.

### Blockers/Concerns

None. Awaiting next milestone selection.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |
| 20260417-v114-lint-cleanup | Clear 7 golangci-lint findings post-v1.14 (gosec/exhaustive/nolintlint/revive); resolves Phase 58 deferred-items.md | 2026-04-17 | d15dd02 | [20260417-v114-lint-cleanup](./quick/20260417-v114-lint-cleanup/) |

## Session Continuity

Last session: 2026-04-17
Stopped at: v1.14 archived
Resume: run `/gsd-new-milestone` to start the next milestone cycle.
