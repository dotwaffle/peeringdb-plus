---
id: SEED-001
slug: incremental-sync-evaluation
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.15+
status: dormant
priority: watch
triggers:
  - name: peak_heap_pressure
    condition: "Peak Go heap sustained >380 MiB on authenticated full sync"
    signal: "/readyz metric or Grafana pdbplus-overview.json dashboard"
  - name: rate_limit_throttle
    condition: "PeeringDB returns 429s despite authenticated-sync utilisation ~4%"
    signal: "RateLimitError log lines from internal/peeringdb client"
  - name: sync_duration_pressure
    condition: "Full sync wall-clock approaches PDBPLUS_SYNC_INTERVAL (default 1h)"
    signal: "sync worker duration span in OTel traces"
---

# SEED-001: Evaluate PDBPLUS_SYNC_MODE=incremental for production

## Context

v1.14 shipped authenticated sync on 2026-04-17. DB now holds full PeeringDB dataset (Public + Users-tier, ~22,320 POCs + scale on other types); ent privacy policy filters Users-tier anonymously. `PDBPLUS_SYNC_MODE=full` is the default and remains unoverridden in `fly.toml`.

Every sync cycle (`PDBPLUS_SYNC_INTERVAL`, default 1h) does a full re-fetch of all 13 PeeringDB types. Post-rollout:
- Rate budget: ~100 req/cycle × 24 cycles/day vs 57,600/day authenticated ceiling ≈ 4% utilised
- Bandwidth: few MB per sync, negligible on Fly
- Memory (observed 2026-04-17, 512 MB VMs): primary peak VmHWM **~84 MB**; 7 replicas steady ~58-59 MB. ~4.5× headroom below the 380 MiB flip trigger. v1.13's 380 MiB was Go runtime heap; OS-level RSS is lower.

## Why keep full for now

1. **Stale-row cleanup gap.** `internal/sync/worker.go` has `deleteStaleOrganizations`, `deleteStaleFacilities`, etc. that run at the end of every full sync and remove rows that vanished upstream by diffing the fetched set against the DB. Incremental's `since` cursor pulls only modified rows; deletions get missed. Whether PeeringDB's `?since=` response emits tombstones for deleted rows is not confirmed — no conformance test exists.
2. **No cost pressure.** All three budgets (rate, bandwidth, memory) comfortably within bounds at authenticated full-sync scale.
3. **Switching is not a flag flip.** Needs a dedicated phase with prerequisites (below).

## Trigger conditions that flip the recommendation

Switch to incremental becomes worth implementing when ANY trigger fires:

- **peak_heap_pressure**: peak heap sustained >380 MiB on authenticated full sync. Watch `/readyz` heap metric or Grafana dashboard.
- **rate_limit_throttle**: upstream returns 429s despite ~4% utilisation (e.g. new per-endpoint sub-throttle).
- **sync_duration_pressure**: full sync wall-clock approaches the 1h interval (it shouldn't at current scale, but upstream can grow).

## Prerequisites before flipping (work for the triggering milestone)

1. **Conformance test for deletion semantics.** Seed a row via httptest fake upstream, delete it (remove from fixture), run incremental, assert row removed from DB. If `?since=` doesn't emit tombstones, document that a periodic full pass is required and implement a hybrid schedule.
2. **Hybrid schedule decision.** Options:
   - 23 × incremental + 1 × full per day (incremental catches modifications, nightly full catches deletions)
   - 6h full / 4h incremental alternating (faster deletion convergence)
   - Operator-configurable
3. **Audit `deleteStale*` + cursor management interaction.** Ensure incremental runs don't advance the cursor past unprocessed deletions; ensure the periodic full pass correctly resets/coexists with the cursor.

## Scope estimate

- 1 small phase (~1-2 plans). Mostly test infrastructure (fake upstream + deletion fixture) + scheduler refactor. Not a major undertaking if triggers force it.

## References

- Full rationale: `memory/project_sync_mode_decision.md`
- Milestone context: `.planning/milestones/v1.14-MILESTONE-AUDIT.md`
- Relevant code: `internal/sync/worker.go` (Sync entry, runSyncCycle, deleteStale* functions, cursor management)
- Feature flag: `PDBPLUS_SYNC_MODE=full|incremental` in `internal/config/config.go`
