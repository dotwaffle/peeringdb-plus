---
id: SEED-002
slug: fly-asymmetric-fleet
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.15+
status: consumed
activated: 2026-04-17
consumed: 2026-04-18
consumed_by: v1.15-phase-65
priority: opportunistic
triggers:
  - name: infra_cost_reduction_pass
    condition: "Explicit milestone goal to reduce Fly.io spend"
    signal: "Milestone requirements or operational review"
  - name: deploy_pain_accumulation
    condition: "Volume orphan cleanup, stuck volume attaches, or replica recovery friction becomes recurrent toil"
    signal: "Multiple Fly volume-related incidents in STATE.md or debug/"
  - name: db_size_growth
    condition: "DB grows past ~500 MB and cold-sync becomes a meaningful boot-time concern"
    signal: "ls -lh /litefs on primary, or replica startup latency from OTel traces"
---

# SEED-002: Asymmetric Fly Fleet — Process Groups + Ephemeral Replicas

## Context

Current fleet: 8 uniform machines (`shared-cpu-2x`, 512 MB, 1 GB persistent volume each) — one app `peeringdb-plus`. Only LHR is a LiteFS primary candidate (`lease.candidate: ${FLY_REGION == PRIMARY_REGION}`). The 7 other regions are read-only replicas that never run the sync worker.

**Observed memory (2026-04-17 post-v1.14):**
- Primary (LHR): 68.8 MB RSS, 83.8 MB peak VmHWM
- Replicas (7 regions): 58-59 MB steady, no peak excursions

**DB size:** 88 MB total (SQLite + LiteFS LTX retention window).

The replicas are heavily over-provisioned on both memory (512 MB vs 60 MB actual) and on persistent storage (1 GB volume for an 88 MB replicated DB that could cold-sync from LHR in seconds).

## Proposal

Single app, two Fly process groups, asymmetric `[[vm]]` sizing, ephemeral storage on replicas.

```toml
# fly.toml sketch

[processes]
  primary = "litefs mount"
  replica = "litefs mount"

[[vm]]
  processes = ["primary"]
  size = "shared-cpu-2x"
  memory = "512mb"

[[vm]]
  processes = ["replica"]
  size = "shared-cpu-1x"
  memory = "256mb"

[[mounts]]
  source = "litefs_data"
  destination = "/var/lib/litefs"
  processes = ["primary"]   # only primary attaches the volume
  # ... existing auto-extend policy stays
```

Scale:
```
fly scale count primary=1 --region lhr
fly scale count replica=7 --region iad,nrt,syd,lax,jnb,sin,gru
```

## Why this works unchanged

- **LiteFS candidacy**: already region-gated via `FLY_REGION == PRIMARY_REGION`. Moving machines into process groups doesn't touch this.
- **Consul election**: single primary candidate (LHR) means election is trivial. Consul keeps working as-is.
- **Replica hydration**: ephemeral replicas cold-sync the 88 MB DB from primary via LiteFS HTTP replication. `/readyz` fail-closes while cold, so Fly routes traffic away until the replica is warm. Self-healing.
- **Write forwarding**: `fly-replay` header already routes `POST /sync` to primary within the same app. Process groups don't change this.
- **Deploy**: single `fly deploy` still covers both groups. `max_unavailable = 0.5` behaviour unchanged.

## Cost delta

| Line item | Before | After |
|-----------|--------|-------|
| 8 × shared-cpu-2x/512 MB | $56/mo | — |
| 1 × shared-cpu-2x/512 MB (primary) + 7 × shared-cpu-1x/256 MB (replicas) | — | $20.60/mo |
| 8 × 1 GB volumes | $1.20/mo | — |
| 1 × 1 GB volume (primary only) | — | $0.15/mo |
| **Total** | **$57.20/mo** | **$20.75/mo** |

**Savings: ~$36/mo (~63%).** Modest in absolute terms but the operational win is the real gain.

## Operational wins

1. **No replica volume orphans.** Fly volumes persist through machine destroy-and-recreate cycles; orphan cleanup has been a recurring minor irritant.
2. **Simpler machine churn.** `fly machine clone`, scaling moves, region migrations — all work cleanly without volume attach/detach semantics on replicas.
3. **Faster replica recovery from corruption.** Destroy-and-recreate completes in seconds; no manual volume zeroing step.
4. **Graceful degradation path.** If replica memory ever comes under pressure, bump one `memory =` line in one `[[vm]]` section. Doesn't touch primary.
5. **Right-sized reasoning.** Treats primary as the one stateful pet, replicas as cattle. Matches the actual architecture.

## Risks and mitigations

| Risk | Mitigation |
|------|-----------|
| Cold replica boot adds ~5-45s hydration time (region-dependent) | `/readyz` already fail-closes when DB not hydrated → Fly excludes from routing → user never sees half-hydrated state |
| Full-fleet redeploy produces burst egress from LHR (~350 MB with 4-concurrent rollout) | Well within single-machine capacity. Intra-Fly egress is free. |
| Primary down + cold replica boot = replica serves nothing | Acceptable. With persistent volumes a replica could serve stale data, but for read-only data that's at most 1h old the difference is academic. Fail-closed is cleaner. |
| shared-cpu-1x may bottleneck read traffic on a replica | Soft concurrency limit is 10 req/machine. Read path is light (SQLite query + template render). Benchmark first; revert that one line if wrong. |
| `primary` process group could fail elsewhere | Static region pin (`--region lhr`); Consul election still runs but only one candidate. If LHR machine dies, need to manually restart or add HA recovery strategy. (Already the case today — regional pinning is existing behaviour.) |

## Prerequisites / validation before flipping

1. **Benchmark replica response times at shared-cpu-1x / 256 MB.** Run a small machine at target size against prod-like traffic for a day. Confirm p99 latency stays within current bounds.
2. **Verify LiteFS cold-sync behaviour.** Destroy and recreate one replica (or `fly machine clone`); measure hydration time per region; confirm `/readyz` correctly gates traffic during hydration.
3. **Audit redeploy path.** One full rolling deploy with the new config in a staging-equivalent setup (or cautiously on prod with rollback ready). Confirm the 4-concurrent hydration burst from LHR doesn't tip primary over any budget.
4. **Document the volume-only-on-primary contract.** Update `docs/DEPLOYMENT.md` + CLAUDE.md so future operators know replica recovery is destroy-and-recreate.

## Scope estimate

One small phase (~2-3 plans):

1. fly.toml restructuring (process groups, asymmetric `[[vm]]`, mount scoping)
2. CLAUDE.md + docs/DEPLOYMENT.md updates for the new operational story
3. Staged rollout: first a single test replica, then the remaining 6 after observation window

Code changes: zero. Infrastructure and docs only.

## References

- Current fly.toml: single `[[vm]]` block, `[mounts]` unscoped
- Current litefs.yml: `lease.candidate: ${FLY_REGION == PRIMARY_REGION}` — the region gate this plan leverages
- Observed memory baseline: see `memory/project_sync_mode_decision.md` "Observed memory baseline" section
- DB size: 88 MB (primary `/litefs/peeringdb-plus.db`, 2026-04-17)
- Related: SEED-001 (incremental sync) — orthogonal. Could land before, after, or independently.
