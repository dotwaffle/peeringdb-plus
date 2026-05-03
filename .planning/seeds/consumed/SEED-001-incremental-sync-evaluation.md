---
id: SEED-001
slug: incremental-sync-evaluation
planted: 2026-04-17
planted_after: v1.14
surface_at: v1.15+
status: consumed
consumed_in: v1.17.0
consumed_by: quick-task-260426-pms
priority: active
resolved_unknown: 2026-04-26
triggers:
  - name: deletion_semantics_confirmed
    condition: "Upstream ?since= empirically confirmed to emit status='deleted' tombstones with bumped updated timestamps"
    signal: "Live spike against www.peeringdb.com 2026-04-26 — see Empirical findings below"
    fired: true
  - name: peak_heap_pressure
    condition: "Peak Go heap sustained >380 MiB on authenticated full sync"
    signal: "/readyz metric or Grafana pdbplus-overview.json dashboard"
    fired: false
  - name: rate_limit_throttle
    condition: "PeeringDB returns 429s despite authenticated-sync utilisation ~4%"
    signal: "RateLimitError log lines from internal/peeringdb client"
    fired: false
  - name: sync_duration_pressure
    condition: "Full sync wall-clock approaches PDBPLUS_SYNC_INTERVAL (default 1h)"
    signal: "sync worker duration span in OTel traces"
    fired: false
---

# SEED-001: Switch PDBPLUS_SYNC_MODE default to incremental

## Status (2026-04-26)

**Active.** The blocking unknown ("does `?since=` emit tombstones for deleted rows") is resolved positively. Pressure-based triggers haven't fired, but the original "wait until forced" rationale is moot — the change is now small, safe, and offers genuine wins (sub-second cycles, lower upstream load, faster deletion convergence than 15-minute full re-fetch).

## Empirical findings (2026-04-26 spike)

Live request: `GET https://www.peeringdb.com/api/poc?since=<30d-ago>&limit=200`

Result: 101 rows returned. **96 with `status="ok"`, 5 with `status="deleted"`** (no `pending`). Sample tombstones:
- `id=80069`, `status="deleted"`, `updated=2026-03-28T17:45:40Z`
- `id=64287`, `status="deleted"`, `updated=2026-03-30T08:01:49Z`
- `id=80378`, `status="deleted"`, `updated=2026-03-30T22:02:17Z`

Tombstones have `name=""` (PII scrubbed on delete — GDPR-style soft-delete with field-level scrub). Upstream behaviour confirmed:

- **No `?since`**: returns `status='ok'` only.
- **With `?since=<ts>`**: returns `status IN (ok, deleted)`, plus `pending` for campus.

This matches our existing `applyStatusMatrix` port (`internal/pdbcompat/filter.go:80-94`) of upstream `rest.py:694-727`.

## What this resolves

1. ~~"Stale-row cleanup gap" (was: incremental misses deletions)~~ — **resolved.** Tombstones arrive on the same cursor as ordinary updates. Existing upsert builders (`internal/sync/upsert.go`) all chain `.SetStatus(x.Status)`; `OnConflict().UpdateNewValues()` flips `'ok'` → `'deleted'` on receipt. No separate deletion code path required.
2. ~~"Hybrid schedule decision" (23×incremental + 1×full, etc.)~~ — **unnecessary.** Pure incremental cycles converge on deletions naturally.

## Plumbing already in place

- **Cursor source**: `internal/peeringdb/client.go:174-179` parses upstream `meta.generated` (Unix epoch float).
- **Cursor advance**: `internal/peeringdb/client.go:156-158` uses the **earliest** `meta.generated` across paginated pages — defensive; prevents the cursor jumping past rows in unprocessed earlier pages.
- **Cursor persistence**: `internal/sync/worker.go:715-718` `UpsertCursor` stores per-type watermarks in the DB.
- **Cursor read + fallback**: `internal/sync/worker.go:842-866` `stageOneTypeToScratch` — incremental falls back to full sync when cursor is zero (first sync, or cursor data lost).
- **Status field threading**: 13 entity upsert builders all chain `.SetStatus(x.Status)`.
- **Soft-delete tombstone storage**: Phase 68 already migrated 13 `deleteStale*` → `markStaleDeleted*`; rows with `status='deleted'` and `cycleStart` updated timestamp are first-class.

## Remaining work (small phase)

1. **Default flip**. `internal/config/config.go` `PDBPLUS_SYNC_MODE` default `full` → `incremental`. Update CLAUDE.md env var table.
2. **httptest conformance test (regression guard)**. Stand up a fake upstream that:
   - Emits a row, lets sync persist it.
   - Removes the row from `?since=` responses by replacing it with a `status='deleted'` tombstone with bumped `updated`.
   - Asserts the DB row is flipped to `status='deleted'` and the next anonymous list (no `?since`) excludes it.
   Empirical behaviour is already confirmed; this is a regression guard.
3. **`markStaleDeleted*` interaction audit**. On incremental cycles, the diff-pass finds no rows to tombstone (the `?since=` window is partial — most IDs are absent from the fetch). Confirm it's a no-op on incremental, not a bug. Either gate the diff-pass to full sync only, or document the no-op.
4. **PII handling test**. Tombstones have `name=""`. Confirm `unifold.Fold("")` setters and ent schema constraints accept empty strings on the 6 folded entities (org, network, facility, ix, carrier, campus).
5. **Cursor recovery story**. If a cursor is lost (DB recreated, replica-cold-boot edge case, operator pgreset), `stageOneTypeToScratch` already falls back to full sync via the `!cursor.IsZero()` gate. Document and test.
6. **Observability**. Per-cycle metric label `mode={full,incremental}` so dashboards distinguish; sync duration histograms split. Already mostly in place via `pdbplus_sync_operations_total{status}`; add `mode` if not present.

## Out of scope (future work)

- **Tombstone GC** ([SEED-004](./SEED-004-tombstone-gc.md)). Independent of this flip.
- **Full sync removal**. Keep `full` as an explicit override for first-sync, recovery, and operator escape hatch. The default flips; the mode stays optional.

## Scope estimate

- 1 quick task or small phase. Default flip + conformance test + audit + observability label = ~3-5 plans of work. Most plumbing exists.

## References

- Empirical spike: this document, `## Empirical findings` section above.
- Upstream code: `peeringdb_server/rest.py:694-727` (status × since matrix); ported to `internal/pdbcompat/filter.go:80-94`.
- Memory: `memory/project_sync_mode_decision.md`
- Milestone context: `.planning/milestones/v1.14-MILESTONE-AUDIT.md`
- Relevant code: `internal/sync/worker.go` (Sync entry, syncFetchPass, stageOneTypeToScratch, markStaleDeleted* functions, cursor management); `internal/peeringdb/client.go` (Generated parsing, paginated cursor advance); `internal/sync/upsert.go` (status threading).
- Feature flag: `PDBPLUS_SYNC_MODE=full|incremental` in `internal/config/config.go`
