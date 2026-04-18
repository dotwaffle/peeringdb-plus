---
phase: 65-asymmetric-fly-fleet
plan: 01
subsystem: infra
tags: [infra, fly, litefs, deployment, docs]
requirements_completed: [INFRA-01, INFRA-03]
dependency_graph:
  requires:
    - SEED-002 proposal (now consumed)
    - SEED-003 planted (orthogonal future primary-HA work, remains active)
  provides:
    - asymmetric-fleet fly.toml (two process groups, per-group [[vm]], primary-scoped [[mounts]])
    - operator runbook for ephemeral replicas
    - volume-only-on-primary contract documented in 3 places
  affects:
    - Plan 02 (live migration) can now assume fly.toml is the next-deploy target
tech_stack:
  added: []
  patterns:
    - Fly process groups + per-group [[vm]] sizing (new to this repo)
    - [[mounts]] scoped via processes = [...] (new to this repo)
key_files:
  created:
    - .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md
  modified:
    - fly.toml
    - docs/DEPLOYMENT.md
    - docs/ARCHITECTURE.md
    - CLAUDE.md
    - .planning/PROJECT.md
  renamed:
    - .planning/seeds/SEED-002-fly-asymmetric-fleet.md → .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md
key_decisions:
  - Empty [processes] command strings (Dockerfile ENTRYPOINT wins at runtime) — accepted verbatim per RESEARCH target; `fly config validate` confirmed OK
  - [http_service].processes = ["primary", "replica"] made explicit (was implicit in single-group config)
  - [[vm]] replica sizing shared-cpu-1x / 256 MB — 4x headroom over observed 58-59 MB RSS
  - litefs.yml untouched — region-gated `lease.candidate: ${FLY_REGION == PRIMARY_REGION}` is the keystone
metrics:
  duration: ~10 minutes
  tasks: 3
  files_touched: 6
  insertions: 102
  deletions: 9
  completed: 2026-04-18
---

# Phase 65 Plan 01: Config and docs (asymmetric fly fleet) Summary

Repo-only groundwork for the v1.15 Phase 65 asymmetric-fleet migration: fly.toml rewritten to declare two Fly process groups (`primary` + `replica`) with per-group [[vm]] sizing and primary-scoped [[mounts]], operator/architecture/CLAUDE/PROJECT docs updated to describe the ephemeral-replica operational model, and SEED-002 moved to `consumed/`. Zero runtime code change, zero Fly.io state change — Plan 02 inherits a clean, already-merged baseline.

## Overview

Three atomic tasks, three commits, zero deviations:

1. **Task 1 (`1767ee4`)** — `fly.toml` rewrite. Replaced single-group uniform topology with:
   - `[processes]` table declaring `primary = ""` + `replica = ""` (empty commands — Dockerfile `ENTRYPOINT ["litefs", "mount"]` provides runtime)
   - `[[mounts]]` (double-bracket table-array, previously `[mounts]` singular table) scoped to `processes = ["primary"]`
   - Two `[[vm]]` blocks: `primary` keeps `shared-cpu-2x` / 512 MB, `replica` downsized to `shared-cpu-1x` / 256 MB
   - `[http_service].processes = ["primary", "replica"]` made explicit (was implicit)
   - `fly config validate --config fly.toml` → `Configuration is valid`

2. **Task 2 (`a44e0ba`)** — documentation sweep across 4 files, all additive (no deletions):
   - `docs/DEPLOYMENT.md` — new `## Asymmetric fleet` subsection (between `## LiteFS` and `## Regional rollout`) covering process-group split, per-region cold-sync expectations table (5-45s range), volume-only-on-primary contract, destroy-and-recreate recovery, sync_status staleness remediation, sizing rationale, cost delta
   - `docs/ARCHITECTURE.md` — new `### Fleet topology (v1.15+)` subsection appended to LiteFS primary/replica detection section, reinforcing that process groups DO NOT replace the region-gated candidacy in litefs.yml
   - `CLAUDE.md` §Deployment — added Asymmetric fleet (v1.15+) bullet + Volume-only-on-primary bullet
   - `.planning/PROJECT.md` Key Decisions — appended Phase 65 row with rationale + cost

3. **Task 3 (`7c651ed`)** — SEED-002 consumed:
   - `git mv .planning/seeds/SEED-002-fly-asymmetric-fleet.md → .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md`
   - Frontmatter updated: `status: activated` → `status: consumed`; added `consumed: 2026-04-18`
   - `consumed_by: v1.15-phase-65` was already present
   - SEED-003 (primary-HA hot-standby) confirmed still present in `.planning/seeds/` — orthogonal future work

## Diff surface area

```
 .planning/PROJECT.md                               |  1 +
 .planning/seeds/{ → consumed/}/SEED-002-*.md       |  3 +-
 CLAUDE.md                                          | 11 +++++
 docs/ARCHITECTURE.md                               | 12 ++++++
 docs/DEPLOYMENT.md                                 | 47 ++++++++++++++++++++++
 fly.toml                                           | 37 +++++++++++++----
 6 files changed, 102 insertions(+), 9 deletions(-)
```

**Files NOT modified (by design):**
- `litefs.yml` — verified unchanged (`candidate: ${FLY_REGION == PRIMARY_REGION}` remains). This is the keystone — without region-gated candidacy, the process-group split would not be safe.
- `Dockerfile.prod` — already ships `litefs` + `sqlite3`; no image change needed
- Any Go source — asymmetric fleet is a deployment-time concern, not a code-time concern

## Verification

All plan-level gates pass:

| Check | Result |
|-------|--------|
| `grep -c '^\[processes\]' fly.toml` → 1 | OK |
| `grep -cE '^\[\[vm\]\]' fly.toml` → 2 | OK |
| `grep -cE '^\[\[mounts\]\]' fly.toml` → 1 | OK |
| `grep 'processes = ["primary"]' fly.toml` | matches on both `[[mounts]]` and primary `[[vm]]` |
| `grep 'processes = ["replica"]' fly.toml` | matches on replica `[[vm]]` |
| `grep 'processes = ["primary", "replica"]' fly.toml` | matches on `[http_service]` |
| `fly config validate --config fly.toml` | Configuration is valid |
| `grep '## Asymmetric fleet' docs/DEPLOYMENT.md` | OK |
| `grep 'Fleet topology (v1.15+)' docs/ARCHITECTURE.md` | OK |
| `grep 'Asymmetric fleet (v1.15+)' CLAUDE.md` | OK |
| `grep 'Phase 65 asymmetric Fly fleet' .planning/PROJECT.md` | OK |
| SEED-002 moved with `status: consumed` + `consumed: 2026-04-18` | OK |
| SEED-003 still present (prerequisite from STATE.md) | OK |
| `litefs.yml` unchanged | OK — `candidate: ${FLY_REGION == PRIMARY_REGION}` present |
| `go build ./...` | Clean (no output, exit 0) |

`flyctl` was available on the executor path, so `fly config validate` ran inline — Plan 02 inherits a lexically-valid config file.

## Deviations from Plan

None — plan executed exactly as written. The RESEARCH target fly.toml was copied verbatim; the doc additions matched the plan's literal content blocks; the seed move used the exact `git mv` invocation prescribed.

## Known Stubs

None. This plan is config + docs only.

## Pointer to Plan 02

Plan 02 (`65-02-live-migration-PLAN.md`, not yet executed) carries the live production migration: `fly machine update --metadata fly_process_group=<group>` pre-tagging, `fly deploy` to roll the fleet, 15-min health window, and the volume-cleanup runbook that destroys the 7 now-orphaned replica volumes. Plan 02 executors can now assume:

- `fly.toml` is the next-deploy target — no in-flight edits required before `fly deploy`
- Operator docs (`docs/DEPLOYMENT.md` § Asymmetric fleet) are merged — can be read by whoever runs the migration
- SEED-002 is consumed — planning state reflects "phase in progress"
- `litefs.yml` is explicitly unchanged — the region-gated candidacy the migration relies on is intact

If Plan 02 is delayed indefinitely, the next unrelated `fly deploy` will already apply the new topology. This was intentional per the plan split rationale (reversible repo edits landed separately from irreversible production migration).

## Commits

| Task | Commit | Subject |
|------|--------|---------|
| 1 | `1767ee4` | feat(infra): Phase 65 — asymmetric fly fleet (process groups) |
| 2 | `a44e0ba` | docs(65-01): document asymmetric fly fleet across 4 files |
| 3 | `7c651ed` | docs(65-01): consume SEED-002 — asymmetric fly fleet activated |

## Self-Check: PASSED

- `fly.toml` exists with expected structure (FOUND)
- `docs/DEPLOYMENT.md` contains "## Asymmetric fleet" (FOUND)
- `docs/ARCHITECTURE.md` contains "Fleet topology (v1.15+)" (FOUND)
- `CLAUDE.md` contains "Asymmetric fleet (v1.15+)" + "Volume-only-on-primary" (FOUND)
- `.planning/PROJECT.md` contains "Phase 65 asymmetric Fly fleet" row (FOUND)
- `.planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md` exists with `status: consumed` + `consumed: 2026-04-18` (FOUND)
- `.planning/seeds/SEED-002-fly-asymmetric-fleet.md` does NOT exist (CONFIRMED ABSENT)
- `.planning/seeds/SEED-003-primary-ha-hot-standby.md` exists (FOUND)
- `litefs.yml` unchanged — `candidate:` line still reads `${FLY_REGION == PRIMARY_REGION}` (CONFIRMED)
- All 3 commits present in `git log`: `1767ee4`, `a44e0ba`, `7c651ed` (FOUND)
- `go build ./...` exits 0 (CONFIRMED)
