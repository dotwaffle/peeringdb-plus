---
phase: 65-asymmetric-fly-fleet
verified: 2026-04-18T08:15:00Z
status: passed
score: 10/10 must-haves verified
overrides_applied: 0
re_verification:
  previous_status: null
  note: Initial verification — no prior VERIFICATION.md
requirements:
  - id: INFRA-01
    status: satisfied
    evidence: "fly.toml has [processes] + 2x [[vm]] scoped primary/replica; live fleet shows 1 primary @ shared-cpu-2x/512MB (LHR) + 7 replicas @ shared-cpu-1x/256MB"
  - id: INFRA-02
    status: satisfied
    evidence: "[[mounts]] scoped to processes=[\"primary\"]; live fleet shows 7 replicas with mounts=0; exactly 1 volume in LHR; all 8 regions /readyz=200"
  - id: INFRA-03
    status: satisfied
    evidence: "docs/DEPLOYMENT.md §Asymmetric fleet + docs/ARCHITECTURE.md Fleet topology + CLAUDE.md §Deployment + .planning/PROJECT.md Key Decisions row; SEED-002 moved to consumed/ with status: consumed + consumed: 2026-04-18"
---

# Phase 65: Asymmetric Fly fleet — process groups + ephemeral replicas — Verification Report

**Phase Goal:** Transition from uniform 8-machine fleet to asymmetric layout per SEED-002. 1 primary (LHR, shared-cpu-2x/512MB, persistent volume) + 7 replicas (other regions, shared-cpu-1x/256MB, ephemeral rootfs).

**Verified:** 2026-04-18T08:15:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | fly.toml declares two process groups (primary, replica) with per-group [[vm]] sizing | VERIFIED | fly.toml L35-37 `[processes] primary = "" replica = ""`; L77-87 two `[[vm]]` blocks scoped `processes = ["primary"]` and `processes = ["replica"]` |
| 2 | [[mounts]] scoped to processes=["primary"] only | VERIFIED | fly.toml L41-49 `[[mounts]] ... processes = ["primary"]` |
| 3 | Live fleet: exactly 1 primary in LHR, 7 replicas in distinct non-LHR regions | VERIFIED | `fly machine list --app peeringdb-plus --json` shows 1 machine LHR `fly_process_group=primary` + 7 machines in {iad, lax, nrt, gru, jnb, sin, syd}, each tagged `fly_process_group=replica` |
| 4 | VM sizing: primary shared-2x/512MB, replicas shared-1x/256MB | VERIFIED | LHR: cpu_kind=shared, cpus=2, memory_mb=512; 7 replicas: cpu_kind=shared, cpus=1, memory_mb=256 |
| 5 | Exactly 1 volume (LHR), 0 non-LHR volumes | VERIFIED | `fly volumes list --app peeringdb-plus --json` returns 1 volume: vol_rk19g1xyxz5q5224 (region=lhr, attached=48e1ddea215398) |
| 6 | LiteFS operational — primary has 15 tables; replica has real data | VERIFIED | Primary sqlite3 `.tables` returns exactly 15 tables (13 entity + sync_cursors + sync_status); IAD replica `SELECT count(*) FROM organizations` returns 33483 (cold-sync validated) |
| 7 | Health: /readyz = 200 from all 8 regions | VERIFIED | lhr/iad/nrt/syd/lax/jnb/sin/gru all returned 200 at verification time |
| 8 | SEED-002 consumed, SEED-003 present | VERIFIED | `.planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md` exists with `status: consumed`, `consumed: 2026-04-18`; `.planning/seeds/SEED-003-primary-ha-hot-standby.md` present as dormant |
| 9 | litefs.yml unchanged | VERIFIED | `git log -- litefs.yml` returns only pre-Phase-65 commits (last touch was `bbc6d4e` v1.6); `candidate: ${FLY_REGION == PRIMARY_REGION}` preserved |
| 10 | Doc sweep — all 4 docs updated | VERIFIED | DEPLOYMENT.md L202 "## Asymmetric fleet"; ARCHITECTURE.md L375 "### Fleet topology (v1.15+)"; CLAUDE.md L171 "Asymmetric fleet (v1.15+)" + L179 "Volume-only-on-primary"; PROJECT.md L220 "Phase 65 asymmetric Fly fleet" row |

**Score:** 10/10 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `fly.toml` | Asymmetric fleet declaration (`[processes]`, 2x `[[vm]]`, scoped `[[mounts]]`) | VERIFIED | All structural greps pass: 1x `[processes]`, 2x `[[vm]]`, 1x `[[mounts]]`, both sizings present |
| `docs/DEPLOYMENT.md` | `## Asymmetric fleet` operator runbook | VERIFIED | L202 subsection present with 47 inserted lines covering cold-sync table, volume-only-on-primary contract, sizing rationale, cost |
| `docs/ARCHITECTURE.md` | Fleet topology note | VERIFIED | L375 `### Fleet topology (v1.15+)` subsection reinforces litefs.yml candidacy gate as source of truth |
| `CLAUDE.md` | §Deployment asymmetric fleet + volume-only-on-primary bullets | VERIFIED | L171-181 both bullets present; existing content preserved |
| `.planning/PROJECT.md` | Key Decisions row for Phase 65 | VERIFIED | L220 table row with cost/rationale/rollback |
| `.planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md` | Consumed seed archive | VERIFIED | Frontmatter: `status: consumed`, `activated: 2026-04-17`, `consumed: 2026-04-18`, `consumed_by: v1.15-phase-65` |
| `.planning/seeds/SEED-003-primary-ha-hot-standby.md` | Future primary-HA seed planted | VERIFIED | Present as `status: dormant`, `surface_at: v1.16+` |
| `.planning/phases/65-asymmetric-fly-fleet/65-01-config-and-docs-SUMMARY.md` | Plan 01 runbook | VERIFIED | 154 lines; all self-checks PASSED |
| `.planning/phases/65-asymmetric-fly-fleet/65-02-live-migration-SUMMARY.md` | Plan 02 migration runbook | VERIFIED | 300+ lines; records 3 deviations, final topology table, cold-sync timings, volume destroy trace |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `fly.toml` | `litefs.yml` | PRIMARY_REGION env drives LiteFS candidate gating | VERIFIED | fly.toml `PRIMARY_REGION = "lhr"` unchanged; litefs.yml `candidate: ${FLY_REGION == PRIMARY_REGION}` unchanged (no commits touching litefs.yml in Phase 65 range) |
| Plan 02 Task 2 (metadata retag) | Plan 02 Task 4 (fly deploy) | Pre-tag prevents `fly deploy` from destroying untagged machines | VERIFIED | 65-02-SUMMARY.md records all 8 machines tagged before deploy; LHR primary `48e1ddea215398` preserved volume throughout (mtime of `/var/lib/litefs/dbs/peeringdb-plus.db/` predates migration at 2026-03-25 18:41) |
| Plan 02 Task 5 (monitoring) | Plan 02 Task 6 (volume cleanup) | Volume destroy only after /readyz green | VERIFIED | 65-02-SUMMARY.md: all 8 regions on 200 at iter=1 of smoke window BEFORE volume destroys; destroy trace shows 7 volumes removed |
| Task 6 jq safeguard | Volume destroy list | Never destroy LHR or attached volumes | VERIFIED | 65-02-SUMMARY.md destroy-selection.txt contains 7 non-LHR volume IDs; final state shows LHR vol_rk19g1xyxz5q5224 intact |

### Live Fleet State (Production Verification)

| Region | Machine ID | State | Group | VM | Mounts |
|--------|------------|-------|-------|----|----|
| lhr | 48e1ddea215398 | started | primary | shared-2x/512MB | 1 |
| iad | d8d3e31b367d18 | started | replica | shared-1x/256MB | 0 |
| lax | d8d2406bed6098 | started | replica | shared-1x/256MB | 0 |
| nrt | 148e007db27758 | started | replica | shared-1x/256MB | 0 |
| gru | 2865ed3c613378 | started | replica | shared-1x/256MB | 0 |
| jnb | 78432e6f12edd8 | started | replica | shared-1x/256MB | 0 |
| sin | 90803d9dced487 | started | replica | shared-1x/256MB | 0 |
| syd | 6835719b799958 | started | replica | shared-1x/256MB | 0 |

**Volumes:** 1 — `vol_rk19g1xyxz5q5224` in lhr, attached to `48e1ddea215398`.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Per-region /readyz probe (all 8 regions) | `curl -H "Fly-Prefer-Region: $r" https://peeringdb-plus.fly.dev/readyz` | 200 from lhr, iad, nrt, syd, lax, jnb, sin, gru | PASS |
| Primary LiteFS mount has expected schema | `fly ssh console --machine 48e1ddea215398 -C 'sqlite3 /litefs/peeringdb-plus.db ".tables"'` | 15 tables (13 ent + sync_cursors + sync_status) | PASS |
| Replica cold-sync produced real data (IAD) | `fly ssh console --machine d8d3e31b367d18 -C 'sqlite3 /litefs/peeringdb-plus.db "SELECT count(*) FROM organizations"'` | 33483 | PASS |
| fly.toml structural validity | grep structural checks from Plan 01 verify block | All 8 pass (1x [processes], 2x [[vm]], 1x [[mounts]], primary/replica sizing) | PASS |
| litefs.yml unchanged | `git log --oneline -- litefs.yml` | Last commit `bbc6d4e` (v1.6) predates Phase 65 | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| INFRA-01 | 65-01, 65-02 | Fly.io process groups introduced with per-group VM sizing | SATISFIED | fly.toml has `[processes]` + 2x `[[vm]]` scoped; production fleet has 1 primary @ shared-cpu-2x/512MB + 7 replicas @ shared-cpu-1x/256MB |
| INFRA-02 | 65-02 | Replicas ephemeral, volume-only-on-primary, /readyz gating | SATISFIED | 7 replicas mounts=0; 7 replica volumes destroyed; /readyz=200 across all 8 regions; 65-02 readyz-smoke.log captured 30x8=240 probes, all 200 by iter=1 |
| INFRA-03 | 65-01, 65-02 | Operational docs updated + SEED-002 consumed | SATISFIED | DEPLOYMENT.md + ARCHITECTURE.md + CLAUDE.md + PROJECT.md all carry Phase 65 sections; SEED-002 at `.planning/seeds/consumed/` with `status: consumed` + `consumed: 2026-04-18` |

All 3 requirements mapped to Phase 65 are satisfied. No orphaned requirements.

### Anti-Patterns Found

None. Plan 02 SUMMARY documents 3 executor deviations, all with root-cause analysis and matching target state:

1. **Zsh word-splitting in Task 2 loop** — transient, retried successfully under bash. No production impact.
2. **flyctl v0.4.35 pre-check refused deploy with mounted replicas** — worked around by destroying replicas first, then deploying. Net outcome identical to plan target.
3. **fly deploy placed new replicas in LHR instead of regional spread** — corrected via `fly scale count replica=7 --region iad,nrt,syd,lax,jnb,sin,gru --process-group replica`; 2 stray LHR replicas then destroyed.

Final topology matches plan target exactly. Deviations are operational (not correctness) and are fully traced in 65-02-SUMMARY.md.

### Human Verification Required

None. User explicitly requested fully-autonomous execution (per 65-02 PLAN `user_setup` and plan-level note: "User has chosen FULL AUTONOMOUS execution. No checkpoints inside the migration sequence."). All must-haves verified programmatically against:

- `fly machine list --app peeringdb-plus --json` (live fleet state)
- `fly volumes list --app peeringdb-plus --json` (volume cleanup)
- `curl https://peeringdb-plus.fly.dev/readyz` with `Fly-Prefer-Region` headers (per-region health)
- `fly ssh console -C 'sqlite3 ...'` (LiteFS cold-sync validation on primary + replica)
- Static grep/file checks for fly.toml, docs, seed files, litefs.yml stability

### Gaps Summary

No gaps. Phase 65 achieved its goal:

- Config declared (fly.toml, Plan 01)
- Docs updated across 4 files (Plan 01)
- SEED-002 consumed (Plan 01)
- SEED-003 planted (pre-phase)
- Live migration executed with zero sustained downtime (Plan 02)
- 7 orphaned replica volumes destroyed cleanly with jq safeguard
- /readyz green across all 8 regions at verification time
- LiteFS cold-sync validated on real replica (IAD has 33,483 organizations)
- litefs.yml unchanged (keystone preserved)
- Plan SUMMARY runbook quality: 65-02-SUMMARY.md records per-region time-to-200, volume IDs destroyed, final fleet topology table, and all 3 executor deviations with root cause

Cost delta realised: uniform 8x shared-cpu-2x/512MB + 8x 1 GB volumes → 1x shared-cpu-2x/512MB + 7x shared-cpu-1x/256MB + 1x 1 GB volume. Expected ~$36/mo savings will appear on next Fly.io invoice.

---

_Verified: 2026-04-18T08:15:00Z_
_Verifier: Claude (gsd-verifier)_
