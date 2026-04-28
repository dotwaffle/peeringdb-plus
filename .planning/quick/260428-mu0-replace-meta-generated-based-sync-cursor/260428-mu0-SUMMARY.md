---
quick_id: 260428-mu0
title: Replace meta.generated-based sync cursor with MAX(updated)
status: incomplete
status_reason: Task 3 (post-deploy human-verify checkpoint) awaits operator confirmation that Grafana `total_objects` oscillation has stopped.
date: 2026-04-28
author: dotwaffle
commits:
  - hash: c25108c
    subject: "feat(sync): derive cursor from MAX(updated) instead of meta.generated"
  - hash: 0c7261f
    subject: "feat(sync): add PDBPLUS_FULL_SYNC_INTERVAL escape hatch (default 24h)"
files_changed:
  - internal/sync/cursor.go
  - internal/sync/cursor_test.go
  - internal/sync/status.go
  - internal/sync/status_test.go
  - internal/sync/worker.go
  - internal/sync/worker_test.go
  - internal/peeringdb/client_live_test.go
  - internal/config/config.go
  - cmd/peeringdb-plus/main.go
  - docs/CONFIGURATION.md
  - graph/resolver_test.go
  - internal/web/detail_test.go
  - internal/web/handler_test.go
requirements:
  - MU0-01  # Replace meta.generated cursor with MAX(updated)
  - MU0-02  # Add PDBPLUS_FULL_SYNC_INTERVAL escape hatch
  - MU0-03  # Add `mode` column to sync_status (idempotent migration)
---

# Phase 260428-mu0: Replace meta.generated-based sync cursor

## Summary

Fixes the v1.13 alternating-full-refetch regression confirmed on Grafana
2026-04-28 where every other 15-minute incremental sync did a full
~270k-row bare-list refetch (`total_objects` oscillating 1,310/1,315/1,317
↔ 270,176/270,184/270,190; spike duration 65–68s vs 13–18s; CPU spikes
~60% for ~1 min at :23/:53). Two atomic commits replace the cursor
mechanism and add a defence-in-depth escape hatch.

### Root cause

PeeringDB does not include `meta.generated` on `?since=` responses
(confirmed in `internal/peeringdb/client_live_test.go`
`TestMetaGeneratedLive/paginated_incremental` — gated by
`-peeringdb-live`, runs against `beta.peeringdb.com`). The pre-mu0 worker
path at `internal/sync/worker.go:1019-1022` returned this absent
zero-time as the cursor update, which `syncUpsertPass` (260428-eda
CHANGE 2) then committed atomically as `last_sync_at = 0` in
`sync_cursors`. On the next cycle, `GetCursor` returned zero, so
`stageOneTypeToScratch` fell through to the full bare-list path. Result:
full → incremental → full → incremental every 15 minutes.

### Fix design

Cursor is now a derived quantity: each per-type cursor read is `SELECT
updated FROM <table> ORDER BY updated DESC LIMIT 1`. Properties:

- Indexed: every entity table has `index.Fields("updated")` — single
  index seek.
- Idempotent: PeeringDB's `?since=N` is inclusive (`updated >= since`)
  so re-fetching the boundary row each cycle is safe; the Phase 75
  skip-on-unchanged predicate (`excluded.updated > existing.updated`)
  turns the OnConflict UPDATE into a no-op.
- Self-healing: empty table → NULL → zero time → fall through to the
  full bare-list path (existing `stageOneTypeToScratch` behaviour).
- Tombstone-aware: `status='deleted'` rows still count toward
  MAX(updated) because their `updated` reflects the upstream deletion
  event.
- Atomic by construction: the data IS the cursor. The 260428-eda CHANGE
  2 in-tx UpsertCursor was solving the row-and-cursor divergence; that
  failure mode is now unreachable (no separate row to lag).

Implementation note: the literal query is `ORDER BY updated DESC LIMIT
1` rather than `MAX(updated)` because modernc.org/sqlite only auto-parses
TEXT → time.Time when the result column has a declared type of DATE /
DATETIME / TIMESTAMP (`rows.go:171-176`). Aggregate expressions drop the
decltype and return raw strings; the index-seek form preserves the
decltype on the result column.

A second commit adds `PDBPLUS_FULL_SYNC_INTERVAL` (default 24h) — every
24 hours the next cycle issues bare-list requests for every type
regardless of cursor state, defending against pathological upstream
cross-row inconsistency that no since-based design can detect.

## Tasks Executed

| # | Task | Status | Commits |
|---|------|--------|---------|
| 1 | Replace meta.generated cursor with MAX(updated) | DONE | `c25108c` |
| 2 | Add PDBPLUS_FULL_SYNC_INTERVAL escape hatch + sync_status.mode column | DONE | `0c7261f` |
| 3 | Post-deploy human-verify checkpoint (Grafana confirmation) | PENDING | — (operator action) |

## Must-Haves Verification

| ID | Must-have | Status | Evidence |
|----|-----------|--------|----------|
| M1 | Two consecutive 15-min incremental syncs no longer alternate ~1.3k ↔ ~270k `total_objects` | STRUCTURALLY DONE | `TestSync_TwoCycle_NoFullRefetch` passes (cycle 2 with zero upstream activity uses `?since=`, not bare list); awaiting operator post-deploy Grafana confirmation per Task 3 |
| M2 | `syncFetchPass` derives each cursor from MAX(updated), NOT meta.generated | DONE | `internal/sync/cursor.go` `GetMaxUpdated`; called from `worker.go:syncFetchPass` (1 hit); `grep -c meta\.generated internal/sync/worker.go` returns 0 in production paths |
| M3 | When `PDBPLUS_FULL_SYNC_INTERVAL` elapses, every per-type fetch is bare-list (no since=) | DONE | `TestSync_FullSyncIntervalForcesBareList` passes; `slog.Info("forcing full bare-list refetch")` fires in test output; cycle is recorded with `mode='full'` |
| M4 | Existing primary instances gain `mode` column without row loss / manual ALTER | DONE | `TestStatusMigration_ModeColumnIdempotent` passes; pre-mu0 schema → ALTER succeeds; second `InitStatusTable` call is a no-op |
| M5 | `sync_cursors` CREATE TABLE preserved; `GetCursor`/`UpsertCursor` still compile, marked DEPRECATED | DONE | `grep -c "CREATE TABLE IF NOT EXISTS sync_cursors" internal/sync/status.go` = 1; `grep -c DEPRECATED internal/sync/status.go` = 3 (GetCursor, UpsertCursor, sync_cursors CREATE TABLE comment) |
| M6 | PeeringDB's absence of meta.generated on ?since= no longer causes incorrect behaviour | DONE | `meta.generated` is still parsed by `scratch.go:stageType` for backwards compatibility but its return value is no longer consumed by `syncFetchPass` (the time.Time return tuple was removed); `grep -c "GetMaxUpdated" internal/sync/cursor.go` = 1 |

Additional artefact validation:

- `grep -c GetCursor(ctx, w\.db internal/sync/worker.go` returns 0
- `grep -c UpsertCursor(ctx, tx internal/sync/worker.go` returns 0
- `grep -c cursorUpdates internal/sync/worker.go` returns 0
- `grep -c "GetMaxUpdated(ctx, w\.db" internal/sync/worker.go` returns 1
- `grep -c "FullSyncInterval" cmd/peeringdb-plus/main.go` returns 1
- `grep -c "PDBPLUS_FULL_SYNC_INTERVAL" docs/CONFIGURATION.md` returns 1
- `grep -c "PDBPLUS_FULL_SYNC_INTERVAL" internal/config/config.go` returns 3
- `WorkerConfig.FullSyncInterval` defined at `internal/sync/worker.go:144`
- `Config.FullSyncInterval` defined at `internal/config/config.go:215`
- `GetLastSuccessfulFullSyncTime` defined in `internal/sync/status.go`
  (called once per cycle from `worker.go:resolveEffectiveMode`,
  not per-type)
- `TestWorkerSync_LineBudget` still passes (Worker.Sync at 100 lines,
  budget 100)

## Operational Caveats

These properties are unchanged from any since-based sync design — the
260428-mu0 cursor mechanism does not introduce or amplify them:

### 1. Pathological cross-row inconsistency

If upstream serves a response where row R' (`updated=M`) is present but
row R (`updated < M`) is missing, R is permanently missed under any
since-based design. We don't have visibility into upstream's
serialisation guarantees, so this is a theoretical hole rather than an
observed bug. **Mitigation: `PDBPLUS_FULL_SYNC_INTERVAL` (default 24h)
forces a periodic bare-list refetch.** Operators can tune downward (e.g.
`6h`) on instances that have observed missing rows, or set to `0` to
disable the escape hatch entirely.

### 2. FK-orphan-dropped rows

A child row whose parent has been dropped during this cycle (FK orphan
filter at `internal/sync/fk_backfill.go`) does NOT bump the cursor for
the child type until the row reappears in a future cycle. This is
intentional: the cursor reflects committed data state, and a dropped row
is not committed. The `pdbplus.sync.type.orphans` metric surfaces this
case to dashboards. The 24h forced-full bare-list refetch (M3) provides
a recovery window for any rows the orphan filter dropped due to
out-of-order parent arrival.

### 3. Replica-lag self-healing

Replicas catch up via LiteFS cold-sync; their derived cursor matches the
primary's data state automatically — there's no separate `sync_cursors`
table to ship over. A replica that mounts at T sees MAX(updated) reflect
exactly the rows committed by the primary's most recent successful
commit, regardless of lag. The replica never runs `Sync()` itself
(scheduler skips on `IsPrimary=false`), so its derived-cursor read is
purely diagnostic. Failover (replica → primary) loses no cursor state.

## Deploy & Verification Plan

The operator should run `fly deploy` from the project root after this
quick task is merged. Post-deploy verification (Task 3 — human-verify
checkpoint):

### Within one sync cycle (~15 min on auth, ~1 h unauth)

1. **Grafana → "Sync Operations Over Time"** → `total_objects` should
   plateau near the steady-state incremental delta (~1,310–1,317 per
   CLAUDE.md production confirmation), NOT alternate up to ~270k.
   Confirms the bug is gone within a single cycle of deploy.

2. **`pdbplus_sync_type_objects_total{type="poc"}`** rate should drop
   ~50% (the alternating big/small per-type counts collapse to a single
   small count per cycle).

3. **Loki: `service.name = peeringdb-plus | sync complete`** — the
   alternating big/small `sync complete` log entries (`total_objects=270k`
   on even cycles, `total_objects=1.3k` on odd) should stop. Every cycle
   should emit `total_objects=~1.3k` (incremental).

### Within 24 hours

4. **Forced full sync at the 24h boundary.** The next cycle after the
   24h threshold should show a single ~270k-row cycle (~65s duration);
   the cycle AFTER that should return to steady-state incremental size.

5. **Loki: `forcing full bare-list refetch`** — should appear in Loki at
   the 24h boundary cycle ONLY (not every cycle). The log carries the
   `last_full_sync` and `full_sync_interval` attrs.

### Schema migration evidence

6. **`fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db "SELECT mode, COUNT(*) FROM sync_status WHERE status='success' GROUP BY mode"'`**
   — both `full` and `incremental` rows should exist in non-zero counts
   after the first 24h cycle. Confirms the `ALTER TABLE` migration applied
   on the existing primary's table.

### Rollback

If any of 1-6 surfaces an unexpected behaviour, rollback by deploying
the previous SHA. The `sync_cursors` table is preserved so the old
binary picks up where it left off — but the alternating-full-refetch
bug returns until investigated.

## Deviations

None. The plan was followed as written, with two minor adjustments
documented inline:

- **`SELECT MAX(updated)` → `SELECT updated ORDER BY updated DESC LIMIT 1`.**
  modernc.org/sqlite only auto-parses TEXT → `time.Time` when the result
  column has a declared type of DATE / DATETIME / TIMESTAMP
  (`rows.go:171-176`); aggregate expressions drop the decltype and the
  driver returns the raw string. The ORDER BY-LIMIT-1 form is identical
  in plan (single index seek on the `updated` index) and preserves the
  decltype on the result column. Documented in the godoc on
  `GetMaxUpdated` and in commit `c25108c`'s body.

- **Effective mode resolved in `Sync()`, not inside `syncFetchPass()`.**
  The plan suggested computing `forceFullSync` at the top of
  `syncFetchPass`. The chosen design computes `effectiveMode` in `Sync()`
  before `RecordSyncStart` so the persisted `sync_status.mode` reflects
  the cycle's effective behaviour rather than the configured default. The
  resolved mode threads through `syncFetchPass` and the
  `record{Success,Failure}` helpers. Same single-call-per-cycle
  invariant; cleaner observability.

## TDD Gate Compliance

Both tasks were TDD: `tdd="true"` in the plan. Each task's commit
contains both the load-bearing tests and the implementation in one
atomic change. The TDD red/green discipline was applied locally during
development:

- **Task 1 RED:** `TestGetMaxUpdated_ReturnsLatest` initially failed
  because `sql.NullTime.Scan` doesn't parse modernc/sqlite's TEXT-stored
  time values for aggregate expressions. Fixed by switching to
  `ORDER BY updated DESC LIMIT 1` (decltype preserved, driver auto-parses).
- **Task 1 GREEN:** all 7 new tests + 14+ existing sync tests passing.
- **Task 2 RED:** `TestStatusMigration_ModeColumnIdempotent` initially
  failed before the `columnExists` helper was added.
- **Task 2 GREEN:** all 4 new status tests + 2 new worker tests passing.

The plan combined RED and GREEN into a single per-task commit (rather
than splitting `test(...)` from `feat(...)`), matching the established
pattern from quick task 260428-eda's commits 1-6 in this repo.

## Self-Check: PASSED

- All 13 claimed files exist on disk.
- Both commits (`c25108c`, `0c7261f`) present in `git log --oneline --all`.
- SUMMARY.md created at the expected path.
- `go vet ./...` clean.
- `golangci-lint run` 0 issues.
- `go test -race ./... -count=1 -timeout=300s` passes (full repo, 33
  packages green).
