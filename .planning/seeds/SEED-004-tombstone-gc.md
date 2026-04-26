---
id: SEED-004
slug: tombstone-gc
planted: 2026-04-19
planted_after: v1.16
surface_at: v1.18.0+
status: dormant
priority: watch
triggers:
  - name: storage_growth
    condition: "DB size growth >5% month-over-month sustained for 3+ months past expected upstream growth"
    signal: "pdbplus_data_type_count gauge divergence from primary volume size, or fly volume dashboard"
  - name: tombstone_ratio
    condition: "Count of rows with status='deleted' exceeds 10% of total rows for any entity type"
    signal: "SELECT COUNT(*) FILTER (WHERE status='deleted') / COUNT(*) > 0.10 GROUP BY entity"
  - name: operator_request
    condition: "Operator requests ability to purge aged tombstones (compliance, storage cost, or replica cold-sync duration)"
    signal: "External requirement, not internal inference"
---

# SEED-004: Tombstone garbage collection policy

## Context

v1.16 Phase 68 flipped `internal/sync/worker.go` from hard-delete (`DELETE FROM <table> WHERE id NOT IN (?)`) to soft-delete (`UPDATE <table> SET status='deleted', updated=NOW() WHERE id NOT IN (?)`). This was required to serve upstream-compatible `?status=deleted + since>0` queries per STATUS-03.

Tombstone rows accumulate forever — the sync worker never revisits them. PeeringDB upstream exhibits the same pattern (Django model soft-delete), and their production DB has accumulated tombstones over ~10 years with no observable impact. But:

1. Our DB is full-replicated to edges via LiteFS every sync cycle. Tombstone bloat impacts cold-sync duration on replica boot.
2. Replica VMs are 256 MB ephemeral. DB size matters for working-set.
3. SQLite VACUUM is not free and locks the DB.

## Why not include GC in Phase 68

- No observed pressure yet — tombstone count at Phase 68 ship time = 0 (fresh state post-flip).
- Purge policy is a separate design discussion (by-age? by-ratio? configurable? FK-safe?). Can't be rushed into the milestone.
- FK constraints: deleting a tombstone might orphan FK-dependent rows (netixlan.net_id referencing a deleted network). Requires careful design to avoid breaking the data-integrity invariants.

## Trigger conditions that flip the recommendation

Design and implement GC when ANY trigger fires:

- **storage_growth**: observed DB growth rate exceeds expected upstream-data growth by >5% MoM sustained for 3+ months
- **tombstone_ratio**: any entity type reaches 10%+ tombstone-to-live ratio
- **operator_request**: external compliance / cost / operational requirement

## Prerequisites before flipping (work for the triggering milestone)

1. **FK-safe purge design**: delete tombstones only after confirming no dependent rows reference them (or cascade-delete dependents; TBD).
2. **Age threshold config**: `PDBPLUS_TOMBSTONE_TTL` with sane default (e.g. 90 days). Must be > typical `since` query window operators actually use.
3. **Scheduler design**: daily background task vs sync-cycle-piggyback vs manual `/gsd-ship-quick` task. Must be FOSS / restart-safe.
4. **VACUUM strategy**: after purge, DB has free pages. Decide whether to trigger `VACUUM` (locks DB) or `PRAGMA auto_vacuum`, or accept growth.
5. **Observability**: OTel span for each purge cycle, rows-purged gauge, tombstone-age histogram.

## Scope estimate

- 1 phase (~2-3 plans). Mostly test infrastructure (FK-cascade fixtures) + scheduler + docs.

## References

- Phase 68 CONTEXT.md § D-02 (soft-delete flip)
- `internal/sync/worker.go` `markStaleDeleted*` family (once Phase 68 ships)
- SEED-001 (`.planning/seeds/SEED-001-incremental-sync-evaluation.md`) — similar "deferred until trigger" pattern
