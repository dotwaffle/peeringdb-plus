---
id: SEED-003
slug: primary-ha-hot-standby
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.16+
status: dormant
priority: watch
triggers:
  - name: lhr_extended_outage
    condition: "LHR region suffers >30min outage and the fleet cannot sync or serve writes during that window"
    signal: "Fly status page + internal incident log"
  - name: maintenance_burden
    condition: "Primary-region maintenance (LiteFS upgrade, DB migration, machine replacement) requires a scheduled downtime window"
    signal: "Operator-observed friction; e.g. docs/DEPLOYMENT.md acquires a 'scheduled downtime' section"
  - name: compliance_requirement
    condition: "Regulatory / customer requirement for multi-region write path or sub-N-minute RPO"
    signal: "External requirement, not internal inference"
  - name: fly_capacity_pressure
    condition: "Fly removes or degrades LHR region, or we need to migrate primary to a different region for cost / latency reasons"
    signal: "Fly platform announcement or sustained LHR-specific latency regression"
---

# SEED-003: Primary HA — Hot Standby for LiteFS Primary

## Context

v1.15 Phase 65 established the asymmetric Fly fleet (1 primary in LHR, 7 ephemeral replicas). LHR remains the sole primary candidate in `litefs.yml` (`lease.candidate: ${FLY_REGION == PRIMARY_REGION}`). If the LHR machine or region goes down, the fleet keeps serving stale reads but cannot sync from PeeringDB until LHR is restored.

**Current state (as of v1.15 ship):**
- Sync worker: runs only on primary
- Writes to DB: only via primary + LiteFS LTX replication
- Fly-replay header: routes POST /sync to primary within-app
- If primary down: reads degrade gracefully via replicas (data becomes stale); writes fail entirely
- Recovery: wait for LHR machine/region to come back, or manually reassign primary candidacy to another region

**Observed baseline (2026-04-17):**
- LHR machine is a single `shared-cpu-2x` / 512 MB instance
- Primary sync cycle: ~1 hour (PDBPLUS_SYNC_INTERVAL), full re-fetch
- Peak heap: ~84 MB; peak RSS: ~84 MB (4.5x headroom to 380 MiB SEED-001 trigger)
- No planned maintenance events in the v1.14 → v1.15 window

## Why we're not acting on this now

1. **No trigger fired.** LHR has been stable; no operator friction on maintenance.
2. **SLO permits it.** Current informal SLO is "public PeeringDB data available from the nearest edge within ~1h of upstream change". An LHR outage degrades freshness; it doesn't break read availability.
3. **Adding a hot standby has real complexity.** Needs Consul election tuning, sync-worker contention-check, cross-region LTX replication latency awareness. Not a single-phase change.
4. **v1.15 theme is tidy-up.** Adding HA changes architecture, not a tidy-up.

## What flipping the trigger would look like (scope sketch for future planning)

1. **Add a second primary candidate region** (likely IAD or FRA for geographical diversity from LHR).
2. **Update `litefs.yml`**: change `lease.candidate` to allow both regions (`${FLY_REGION == PRIMARY_REGION || FLY_REGION == STANDBY_REGION}` or similar). Consul election picks one at a time.
3. **Sync-worker leader check**: `internal/sync/worker.go` already guards via `IsPrimary()`; the standby machine would run the binary but sit idle until elected.
4. **Write forwarding**: `fly-replay` header already routes to the elected primary; works unchanged if primary moves.
5. **Asymmetric fleet composition**: `primary` process group scales to 2 machines (1 LHR active, 1 standby); `replica` group stays at 7.
6. **Cost delta**: +$7/mo for the standby. Acceptable.
7. **Testing**: Cold-failover drill — manually fail LHR, confirm standby promotes, confirm sync resumes on new primary, confirm fleet-wide /readyz recovers.

## Prerequisites before flipping

1. **Consul election tuning audit** — ensure election timeout is reasonable for an unplanned LHR failure (not too aggressive that it flaps, not too slow that sync stalls).
2. **Sync-worker contention test** — verify the binary handles a fast primary→standby promotion without dual-writing or dropping the in-flight sync cycle.
3. **Cross-region LTX replication latency check** — primary candidate regions must have low-enough latency for LTX to propagate within the sync window.
4. **Operational runbook** — document manual promotion command (e.g. via Consul), monitoring to watch, what log lines signal a promotion event.

## Scope estimate

One phase, ~2-3 plans. Small code delta (litefs.yml edit + possibly a `PDBPLUS_STANDBY_REGION` env var + docs). Heavy on operational testing.

## References

- Full architectural context: `memory/project_sync_mode_decision.md`
- Current primary-gated code: `internal/sync/worker.go` `wasPrimary` checks
- LiteFS candidacy: `litefs.yml` line with `lease.candidate: ${FLY_REGION == PRIMARY_REGION}`
- v1.15 Phase 65 asymmetric fleet: `.planning/phases/65-asymmetric-fly-fleet/65-CONTEXT.md` (deliberately left primary as single-region)
- Related: SEED-001 (incremental sync) — orthogonal. SEED-002 (asymmetric fleet) — shipped via Phase 65.

## 2026-04-27 update — IAD-preferred variant

Specific shape we'd most likely implement when this seed flips: **IAD as preferred primary, LHR as standby candidate** (rather than the original "LHR primary + somewhere standby" framing).

### Rationale

- PeeringDB upstream runs in AWS us-east-1 (Virginia). IAD (Ashburn, VA) is the closest Fly region — ~10ms RTT vs ~75ms from LHR. Primary→upstream RTT is the only cost paid per sync cycle that scales with API call count, so post-Phase-73 (incremental sync, smaller deltas, more frequent fetches) this matters slightly more than it did at v1.15.
- Geographical-diversity argument from SEED-003's original sketch (LHR + FRA) doesn't help availability: if AWS us-east-1 is down, PeeringDB upstream is down, and the sync worker is idle regardless of which Fly region holds the lease.
- IAD and LHR are both in Fly's NA/EU pricing group — egress cost neutral.

### Cost analysis (verified 2026-04-27 from `fly status`)

Current fleet: primary in lhr; replicas in iad, lax, nrt, sin, syd, gru, jnb.

Moving to IAD-primary + LHR-standby (8 receivers total):

- Cold-sync fanout (88 MB DB × 8 receivers @ $0.006/GB egress, NA group): **~$0.0042/event** vs current $0.0037. Sub-cent delta.
- Steady-state LiteFS WAL streaming: same $0.006/GB egress source rate, one extra receiver. Sub-cent/month delta.
- Second persistent volume for LHR standby: **+$0.15/mo** (1 GB at $0.15/GB/mo). This is the entire incremental cost.
- Asymmetric pricing groups stay neutral: APAC/Oceania/SA replicas (nrt/sin/syd/gru) still receive at $0.015/GB equivalent (billed at source per Fly's egress-from-source model — confirm with Fly support before committing, the docs don't disambiguate cross-group).

Cost is essentially free. The decision is operational, not financial.

### Smaller config change than originally sketched

SEED-003's body says "Consul election tuning, sync-worker contention-check" — implying Consul wasn't configured. Re-check on 2026-04-27 confirmed `litefs.yml:18-26` already has `type: consul` + `promote: true`. The candidate filter on line 20 is `${FLY_REGION == PRIMARY_REGION}` — that's the single-line gate.

To unlock IAD+LHR candidacy:

1. `litefs.yml` candidate predicate: broaden to allow both regions, e.g.
   ```yaml
   candidate: ${FLY_REGION == "iad" || FLY_REGION == "lhr"}
   ```
   Or set `PRIMARY_REGION` per-machine and keep the `==` form (cleaner, leaves litefs.yml region-agnostic).
2. `fly.toml` primary process group: `regions = ["iad", "lhr"]`, `count = 2`. Existing `[[mounts]] processes = ["primary"]` automatically scopes a volume to each primary-eligible machine — no change needed.
3. `PRIMARY_REGION` env: change from `lhr` to `iad` so IAD wins lease on cold start; LHR sits as candidate.

### Asymmetric-fleet erosion (the real cost)

Phase 65's design intent is "1 pet (LHR) + 7 cattle." This variant is "2 pets (IAD, LHR) + 6 cattle." Recovery semantics bifurcate: replicas remain destroy-and-recreate; primary candidates need volume-management discipline (snapshots, reattach during machine replacement, etc.). Worth an explicit ADR rather than rolling into a phase as an implementation detail.

### Split-brain bound

Already mitigated by Consul-arbitrated leases — only one candidate holds the write lease at a time, contingent on Consul being reachable. Failure mode requiring two-primary write windows: asymmetric partition where one candidate can reach Consul and the other can't, AND that partition resolves with a stale primary still believing it's authoritative. LiteFS lease TTL bounds this; Consul defaults are sane. Non-zero risk but small.

### When to flip

Original SEED-003 triggers still apply (LHR extended outage, maintenance burden, compliance). Add one IAD-specific trigger:

- **Sync upstream-RTT regression**: if PeeringDB sync wall-clock grows materially due to upstream latency (post-Phase-73 incremental cycles getting chatty enough that the 65ms LHR→us-east-1 RTT is the bottleneck). Signal: `pdbplus.sync.duration_seconds` p95 trending up while `pdbplus.sync.peak_heap_bytes` stays flat.
