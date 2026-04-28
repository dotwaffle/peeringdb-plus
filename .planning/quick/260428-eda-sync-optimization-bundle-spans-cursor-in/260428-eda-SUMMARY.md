---
phase: 260428-eda
plan: 01
subsystem: internal/sync
tags: [perf, observability, atomicity, sqlite, litefs]
requires:
  - internal/sync/worker.go (Worker.Sync orchestrator, ~620 line file)
  - internal/sync/upsert.go (13 OnConflict sites)
  - internal/sync/status.go (UpsertCursor)
  - internal/sync/initialcounts.go (gauge cache primer)
  - internal/database/database.go (DSN)
  - cmd/peeringdb-plus/main.go (call sites)
provides:
  - 1 LiteFS-replicated commit per sync (was 14: upserts + 13 post-commit cursor writes)
  - sync-commit / sync-finalize / sync-cursor-updates / sync-record-status / sync-on-complete OTel spans
  - per-row skip-on-unchanged via ON CONFLICT DO UPDATE WHERE predicate
  - 1 UNION ALL replacing 13 sequential ent Count() calls
  - synchronous=NORMAL + cache_size=-32000 + temp_store=MEMORY DSN pragmas
  - PRAGMA cache_spill=OFF per-tx
  - pdbplus.sync.cursor_write_caused_rollback span attr (postmortem visibility)
affects: []
tech-stack:
  added: []
  patterns:
    - sql.UpdateWhere(sql.P(...)) on bulk upsert OnConflict path
key-files:
  created:
    - .planning/seeds/SEED-005-periodic-full-sync-schedule.md
  modified:
    - internal/database/database.go
    - internal/sync/worker.go
    - internal/sync/upsert.go
    - internal/sync/upsert_test.go
    - internal/sync/status.go
    - internal/sync/status_test.go
    - internal/sync/initialcounts.go
    - internal/sync/initialcounts_test.go
    - internal/sync/integration_test.go
    - internal/sync/worker_test.go
    - cmd/peeringdb-plus/main.go
decisions:
  - Strict `>` (not `>=`) in skip-on-unchanged predicate: `>=` would defeat the optimisation since PeeringDB ?since=N is inclusive. Same-second-drift caveat → SEED-005 (periodic full-sync escape-hatch).
  - Skip-predicate guard for zero-time uses `<= '1900-01-01'` — empirically verified modernc stores time.Time{} as TEXT '0001-01-01 00:00:00+00:00'.
  - Cursor writes moved INTO main tx; failure rolls back the whole cycle. SyncWithRetry handles retries; no in-loop retry. Span attr `pdbplus.sync.cursor_write_caused_rollback` flags the failure mode.
  - UpsertCursor signature changed from *sql.DB to *ent.Tx. sync_status row update stays on raw *sql.DB (outcome record, not data state — by design, D-19 atomicity preserved).
  - InitialObjectCounts switched from 13 ent Count() calls to one raw-SQL UNION ALL, dropping the privctx.TierUsers elevation entirely (raw SQL bypasses ent privacy). POC count doubling/halving cross-reference preserved in godoc.
metrics:
  duration: 50min
  completed: 2026-04-28
---

# Phase 260428-eda Plan 01: Sync Optimization Bundle Summary

**One-liner:** Drop sync wall time tail (post-commit black hole + always-update upserts) by folding cursor writes into the main tx, instrumenting the post-commit phase with named OTel spans, gating ON CONFLICT updates on `excluded.updated > existing.updated`, collapsing 13 startup Count() calls into one UNION ALL, and tuning SQLite pragmas for the bulk-write workload.

## What changed (per task)

### Task 1 — DSN pragmas (commit `dd1514f`)
- `internal/database/database.go`: appended `synchronous(NORMAL)`, `cache_size(-32000)` (32 MB), `temp_store(MEMORY)` to the DSN
- Rationale embedded in the `Open` godoc; `synchronous=NORMAL` is safe under LiteFS-replicated WAL because LiteFS provides durability via streaming replication
- Tests: `go test -race ./internal/database/... ./internal/sync/...` clean
- Lint: clean

### Task 2 — `prepareTxPragmas` helper + per-tx `cache_spill = OFF` (commit `5646d75`)
- New file-local helper `prepareTxPragmas(ctx, tx)` runs both `defer_foreign_keys = ON` (existing) AND new `cache_spill = OFF`
- Replaces inline `tx.ExecContext("PRAGMA defer_foreign_keys = ON")` block in `Worker.Sync` (-1 line net, buys headroom for Task 4)
- `cache_spill=OFF` keeps dirty pages in the connection's page cache (bounded by `cache_size`) instead of spilling to WAL between writes during the bulk-upsert burst
- Tests: `TestWorkerSync_LineBudget` still passes (Sync line count went DOWN)
- Lint: clean

### Task 3 — `InitialObjectCounts` UNION ALL (commit `eaabb00`)
- `internal/sync/initialcounts.go`: rewrote to issue ONE `db.QueryContext` against `*sql.DB` with a UNION ALL across the 13 entity tables (held as `const initialCountsQuery` for grep-ability)
- Dropped `privctx.WithTier(ctx, TierUsers)` elevation — raw SQL bypasses ent's Privacy policy entirely (no Privacy Hook fires on `db.QueryContext`)
- Updated both call sites in `cmd/peeringdb-plus/main.go` (startup primer + OnSyncComplete callback) to pass `*sql.DB`
- Removed unused `privctx` and `ent` imports from initialcounts.go; added `database/sql`
- Added `TestInitialCountsQuery_TableNamesMatchSchema` to fail-fast on entgo table-name drift via `sqlite_master` introspection
- Preserved POC count doubling/halving cross-reference and link to `.planning/debug/poc-count-doubling-halving.md`
- Tests: 4 InitialObjectCounts subtests pass (AllThirteen, EmptyDB, KeyParity, PocPolicyBypass) + the new TableNamesMatchSchema test
- Lint: clean

### Task 4 — Post-commit observability spans (commit `88e7fb8`)
- New `commitWithSpan(ctx, tx)` helper: wraps `tx.Commit()` in a `sync-commit` OTel span, returns the commit error
- Wraps `recordSuccess` body in a `sync-finalize` span and reassigns ctx so sub-spans parent under finalize
- Three sub-spans inside `recordSuccess`: `sync-cursor-updates` (around the cursor-write loop, will move with the loop in Task 5), `sync-record-status` (around `RecordSyncComplete`), `sync-on-complete` (around the `OnSyncComplete` callback)
- Pattern matches existing per-step span style at worker.go:917 (`_, span := otel.Tracer("sync").Start(...)`); no unused ctx variables
- Pure observability; no behavioural change
- Tests: `TestWorkerSync_LineBudget` still passes
- Lint: clean

### Task 5 — Cursor writes embedded in main upsert tx (commit `76bb885`)
- `UpsertCursor` signature converted from `*sql.DB` to `*ent.Tx`; SQL is byte-identical
- Added `ent` import to `internal/sync/status.go`; expanded godoc with the in-tx semantic and failure-mode shift documentation
- `cursorUpdates` threaded into `syncUpsertPass` as a new parameter; the per-type cursor rows are written via `UpsertCursor(ctx, tx, ...)` before `syncUpsertPass` returns, inside a `sync-cursor-updates` span (migrated from `recordSuccess`)
- On cursor-write failure: stamp the root span with `pdbplus.sync.cursor_write_caused_rollback=true` (Tempo postmortem visibility), return error so the orchestrator rolls back the whole tx
- Removed the post-commit cursor loop and span from `recordSuccess`; dropped the `cursorUpdates` parameter from `recordSuccess`
- Updated `recordSuccess` godoc; expanded `syncUpsertPass` godoc with the failure-mode shift; placed brief note in Sync to keep within `TestWorkerSync_LineBudget` 100-line cap (Sync ends at 97 lines)
- Status tests rewritten for the new signature: `TestUpsertCursor_InsertAndGet`, `TestUpsertCursor_UpdateExisting`, `TestGetCursor_ReturnsRegardlessOfStatus` open `client.Tx`, call `UpsertCursor`, commit, then read back via `*sql.DB`. `TestUpsertCursor_DBError` rewritten to exercise use-after-rollback via `tx.Rollback()` (preserves fault-injection coverage; modernc surfaces use-after-rollback as the wrapped "upsert cursor for" error)
- Tests: all sync tests pass; line budget intact
- Lint: clean

### Task 6 — Per-row skip-on-unchanged (commit `8a9b5cb`)
- New `skipUnchangedPredicate(table)` helper at the top of `internal/sync/upsert.go`: emits the SQL `excluded.updated > <table>.updated OR <table>.updated IS NULL OR <table>.updated <= '1900-01-01'` predicate
- Empirically verified the zero-time storage representation: modernc.org/sqlite stores `time.Time{}` as TEXT `'0001-01-01 00:00:00+00:00'`, `typeof = "text"`, `SELECT (updated <= '1900-01-01') = 1`. Documented in helper godoc.
- Strict `>` rather than `>=` deliberately: PeeringDB `?since=N` is inclusive, `>=` would defeat the optimisation. Same-second-drift caveat documented; periodic full-sync escape-hatch tracked in **SEED-005**
- All 13 OnConflict sites converted from `OnConflictColumns(...).UpdateNewValues()` to the explicit `OnConflict(ConflictColumns(...), ResolveWithNewValues(), UpdateWhere(skipUnchangedPredicate(<table>)))` form
- Predicate emission: `b.Ident("updated")` is intentional — ent's UpdateWhere predicate is emitted with the table qualifier active, so `Ident` produces `<table>.updated` automatically. The literal `excluded.updated` references SQLite's pseudo-table inside ON CONFLICT DO UPDATE
- Added `TestUpsert_SkipOnUnchanged` with 4 sub-tests: skip / update_on_newer / update_on_zero / insert_when_absent (all PASS)
- Updated 7 existing tests in `worker_test.go` to bump `updated` on second-pass payloads (TestSyncUpsertUpdatesExisting, TestSyncPersistsExplicitTombstone, TestIncrementalSync, TestIncrementalSkipsDeleteStale, TestSync_IncrementalDeletionTombstone, TestSync_IncrementalRoleTombstone, TestUpsertPopulatesFoldColumns); added `bumpUpdated(map, ts)` helper using `maps.Copy`
- Updated 3 fixture timestamps in `integration_test.go` (TestSyncTombstonesExplicitDeletedRecords, TestSync_TombstonePersistedFromExplicitPayload, TestSyncFKIntegrity_AfterTombstoneCycle) — match real PeeringDB behaviour where `updated` always bumps on content changes
- Created **`.planning/seeds/SEED-005-periodic-full-sync-schedule.md`** (dormant) sketching the same-second-drift convergence escape-hatch
- Tests: all 21+ subtests + integration tests pass
- Lint: clean (added `entgo.io/ent/dialect/sql` import; `maps` stdlib import for the helper)

## Production trace expectations

After deploy via `fly deploy`, the next sync trace in Tempo should show:

- A new `sync-commit` child span under `sync-{full,incremental}` with duration < 5s in steady state (down from ~60s when the post-commit cursor writes were dominating tx lifetime)
- A new `sync-finalize` child span with three siblings: `sync-record-status`, `sync-on-complete` (and `sync-cursor-updates` is now PARENT-side under `sync-upsert-*` rather than under finalize)
- A `sync-cursor-updates` span inside `syncUpsertPass` covering the 13 `sync_cursors` upserts — should run < 100ms (single tx, no LiteFS round-trips)
- `pdbplus.sync.peak_heap_bytes` and `pdbplus.sync.peak_rss_bytes` attrs on the sync root span (unchanged)
- On a hypothetical cursor-write failure: `pdbplus.sync.cursor_write_caused_rollback=true` on the root span — this is the new postmortem signal
- `pdbplus.sync.type.objects` gauges should stay at the same row counts as before (no behavioural change to data) but the post-commit tail (~23s in trace `1c22931e65d639516a6987c848657283`) should disappear
- Steady-state cycles where zero rows changed: tx commit duration drops dramatically because the SQL `UpdateWhere` predicate skips the no-op rewrites of all ~270k rows

## Rollback recipe

Each commit reverts cleanly in REVERSE order. The riskiest is CHANGE 3 (skip-on-unchanged); reverting just that commit cleanly disables the optimisation while leaving the others in place:

```bash
# Highest risk first — disables skip-on-unchanged but keeps everything else
git revert 8a9b5cb        # Task 6: skip ON CONFLICT updates
# Behavioural changes — revert these together if reverting all
git revert 76bb885        # Task 5: cursor writes in main tx
git revert 88e7fb8        # Task 4: observability spans
git revert eaabb00        # Task 3: UNION ALL primer
git revert 5646d75        # Task 2: cache_spill OFF
git revert dd1514f        # Task 1: DSN pragmas
```

## Same-second drift callout

The skip-on-unchanged predicate uses strict `>` so a row edited at upstream within the same second as the prior cursor advance will skip on the next incremental cycle. Reconciliation paths:

1. **Next upstream change** to the row will bump `updated` past the cursor and the row reconciles naturally.
2. **Operator escape-hatch:** run `PDBPLUS_SYNC_MODE=full` to retry the full upstream set; the predicate still gates writes but the cycle re-evaluates every row's `updated`.
3. **Periodic full-sync schedule** (out of scope for this plan) — see **`.planning/seeds/SEED-005-periodic-full-sync-schedule.md`** for the dormant proposal.

## Deviations from Plan

**Test fixture updates (Rule 1 — Bug):**

Seven existing worker tests + three integration test fixtures had to be updated to bump `updated` on second-pass payloads because they were silently relying on the old "always update" upsert semantics. After CHANGE 3 the predicate correctly skips same-`updated` re-upserts (matching real PeeringDB behaviour where content changes always bump `updated`). These were not pre-existing bugs — the tests were correct against the prior unconditional upsert path — but the contract changed. Documented in each touched site with a `260428-eda CHANGE 3:` comment.

**Sync orchestrator note slimmed (Rule 3 — Blocking, line budget):**

The plan instructed adding a multi-paragraph "NOTE (260428-eda CHANGE 2)" docstring inline at the cursor-write site in `Worker.Sync`. That pushed Sync to 107 lines, breaching the `TestWorkerSync_LineBudget` 100-line cap (Rule 3 unblock). Moved the long failure-mode-shift documentation to the `syncUpsertPass` godoc (the function that now owns the cursor-write loop) and kept a 3-line cross-reference comment in Sync. Sync ends at 97 lines.

**Probe test (helper for STEP 0):**

Created and removed a temporary `zprobe_test.go` to empirically observe modernc's zero-time storage representation. Output (TEXT `'0001-01-01 00:00:00 +0000 UTC'`) was used to author the predicate's third clause (`<= '1900-01-01'`). The probe was deleted before commit; only the documented finding survives in the helper godoc.

## Authentication gates

None.

## Self-Check: PASSED

Verified all paths/commits exist:

- `internal/database/database.go` (committed in `dd1514f`)
- `internal/sync/worker.go` (committed in `5646d75`, `88e7fb8`, `76bb885`)
- `internal/sync/upsert.go` (committed in `8a9b5cb`)
- `internal/sync/upsert_test.go` (committed in `8a9b5cb`)
- `internal/sync/status.go` (committed in `76bb885`)
- `internal/sync/status_test.go` (committed in `76bb885`)
- `internal/sync/initialcounts.go` (committed in `eaabb00`)
- `internal/sync/initialcounts_test.go` (committed in `eaabb00`)
- `internal/sync/integration_test.go` (committed in `8a9b5cb`)
- `internal/sync/worker_test.go` (committed in `8a9b5cb`)
- `cmd/peeringdb-plus/main.go` (committed in `eaabb00`)
- `.planning/seeds/SEED-005-periodic-full-sync-schedule.md` (committed in `8a9b5cb`)

Six commits land cleanly above the assigned base `73b7efd`. `go build ./...`, `go vet ./...`, `go test -race -count=1 -timeout=600s ./...`, and `golangci-lint run` all green. All 10 load-bearing tokens grepped per the plan's `<verification>` checklist resolve.
