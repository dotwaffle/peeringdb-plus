---
phase: 68-status-since-matrix
plan: 02
subsystem: sync
tags: [soft-delete, tombstones, sync, ent, cycleStart, STATUS-03]

# Dependency graph
requires:
  - phase: 68-status-since-matrix (plan 01)
    provides: IncludeDeleted wiring removed; unified upsert path unconditionally persists status=deleted rows; intermediate TestSyncPersistsDeletedRowsUnconditional test in place awaiting replacement
provides:
  - 13 markStaleDeleted* soft-delete functions in internal/sync/delete.go running UPDATE ... SET status='deleted', updated=cycleStart instead of DELETE FROM
  - syncStep.deleteFn signature extended to carry cycleStart time.Time; syncDeletePass signature extended additively
  - Single cycleStart per cycle reused from the pre-existing start := time.Now() at worker.go:293 (no new clock reading)
  - TestSync_SoftDeleteMarksRows 2-cycle round-trip test replacing the intermediate TestSyncPersistsDeletedRowsUnconditional
  - TestSyncHardDelete renamed + rewritten to TestSyncSoftDeletesStale; TestSyncDeletesStaleRecords and TestSyncDeletesFKIntegrity assertions flipped to soft-delete semantics
  - Info log renamed "deleted stale" -> "marked stale deleted" with count attribute "deleted" -> "marked"
affects: [68-03-status-matrix-pdbcompat, 68-04-changelog]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Soft-delete via ent Update().Where(X.IDNotIn(chunk...)).SetStatus('deleted').SetUpdated(cycleStart).Save(ctx) — returns (int, error) matching the Delete().Exec(ctx) shape so the deleteStaleChunked helper signature survives verbatim."
    - "One-cycleStart-per-cycle: a single time.Time captured at Worker.Sync entry is plumbed through syncDeletePass -> syncStep.deleteFn so all 13 entity types tombstone with an identical updated value, keeping ?since=N query windows atomic."
    - "Soft-delete round-trip test: fixture-serve N rows -> sync cycle 1 -> setFixtureData to drop a row -> sync cycle 2 -> assert row count unchanged AND dropped row has status='deleted' AND updated >= cycle2Start. Pattern reusable for follow-up entity-specific tombstone tests."

key-files:
  created: []
  modified:
    - internal/sync/delete.go
    - internal/sync/worker.go
    - internal/sync/integration_test.go
    - internal/sync/worker_test.go

key-decisions:
  - "Reuse the existing start := time.Now() at worker.go:293 as cycleStart rather than adding a second clock reading — keeps one source-of-truth for the cycle timestamp and avoids subtle sub-millisecond drift between the sync_status LastSyncAt value and the tombstone updated values."
  - "Dropped the planned inline // cycleStart := start comment at the syncDeletePass call site because it pushed Worker.Sync to 102 lines and tripped TestWorkerSync_LineBudget (REFAC-03 100-line cap). The syncDeletePass godoc already documents the cycleStart semantic; the extra comment would have been redundant with a harder-to-maintain location."
  - "TestSyncHardDelete renamed to TestSyncSoftDeletesStale rather than deleted — the intent (verify that rows absent from cycle-N+1's remoteIDs are reconciled) is preserved, just the reconciliation is soft-delete now. Deleting it would have lost the per-field assertion pattern that the integration test's 3-cycle variant doesn't cover."
  - "TestSyncDeletesStaleRecords and TestSyncDeletesFKIntegrity left in place with updated comment + assertion counts (1 -> 3 orgs; 1 -> 2 IXes). Under soft-delete the FK-integrity invariant becomes trivially true (no rows physically removed = no possible dangling FKs), but retaining foreign_key_check inside the tx is cheap and future-proofs against any later hybrid policy."
  - "deleteStaleChunked helper name preserved (not renamed to markStaleDeletedChunked) — it's a private generic chunk-runner, and renaming would ripple through 13 callers without semantic benefit. Only its doc comment updated to reflect the new closure body. Preserves grep-ability for Phase 68 research Open Question 4 (SEED-004 >32K fallback)."

patterns-established:
  - "Soft-delete flip pattern (future phases): rename deleteStale* -> markStaleDeleted*, add cycleStart time.Time as the 4th closure parameter, swap Delete().Exec(ctx) for Update().Where(IDNotIn(chunk...)).SetStatus('deleted').SetUpdated(cycleStart).Save(ctx). Outer (int, error) preserved; helper chunker unchanged."
  - "cycleStart plumbing (future phases with timestamp-dependent work): reuse Worker.Sync's start := time.Now() capture rather than adding new readings; plumb additively through step-list signatures (deleteFn -> syncDeletePass). Never call time.Now() inside per-entity closures — sync-cycle atomicity requires one timestamp."

requirements-completed: []

# Metrics
duration: 18min
completed: 2026-04-19
---

# Phase 68 Plan 02: Sync Soft-Delete Flip Summary

**13 sync delete closures flipped from `DELETE FROM` to `UPDATE ... SET status='deleted', updated=cycleStart` via ent Update builder; single cycleStart plumbed once through syncDeletePass; 2-cycle TestSync_SoftDeleteMarksRows round-trip test locks the new D-02 behaviour.**

## Performance

- **Duration:** ~18 min
- **Started:** 2026-04-19T14:10Z (approx)
- **Completed:** 2026-04-19T14:28Z (approx)
- **Tasks:** 3
- **Files modified:** 4

## Accomplishments

- All 13 `deleteStale*` functions in `internal/sync/delete.go` renamed to `markStaleDeleted*` and rewritten to run `tx.X.Update().Where(x.IDNotIn(chunk...)).SetStatus("deleted").SetUpdated(cycleStart).Save(ctx)` — zero `DELETE FROM` statements remain on the main-DB delete pass (the single survivor at `worker.go:711` is the scratch SQLite cleanup path and out of scope per D-02).
- `syncStep.deleteFn` extended additively with `cycleStart time.Time` as the 4th parameter; return variable renamed from `deleted` to `marked` to reflect the new semantic.
- `syncDeletePass` signature extended; call site inside `Worker.Sync` reuses the existing `start := time.Now()` at worker.go:293 rather than taking a second clock reading — all 13 types tombstone with one identical `updated` value per cycle.
- `TestSync_SoftDeleteMarksRows` implements the 2-cycle round-trip test: cycle 1 syncs 3 orgs, cycle 2 drops org 1 from fixture, asserts count stays 3, org 1 now `status='deleted'` with `updated >= cycle2Start`, org 2 stays `'ok'`, org 3 (fixture pre-existing tombstone) stays `'deleted'`.
- Three pre-existing tests flipped in-line to soft-delete semantics: `TestSyncHardDelete` renamed to `TestSyncSoftDeletesStale`, `TestSyncDeletesStaleRecords` and `TestSyncDeletesFKIntegrity` assertions updated from physical-row-count-decrement to soft-delete-count-stable-plus-status-transition.

## Task Commits

Each task was committed atomically:

1. **Task 1: Rename + flip 13 deleteStale functions to markStaleDeleted with cycleStart parameter** — `8bd31a4` (refactor)
2. **Task 2: Plumb cycleStart through syncStep + syncDeletePass + 13 syncSteps entries** — `4d20735` (refactor)
3. **Task 3: Replace TestSyncPersistsDeletedRowsUnconditional + flip three pre-existing tests** — `f8f1899` (test)

## Files Created/Modified

- `internal/sync/delete.go` — 13 functions renamed + flipped from `Delete().Where(...).Exec(ctx)` to `Update().Where(...).SetStatus("deleted").SetUpdated(cycleStart).Save(ctx)`; `"time"` import added; `deleteStaleChunked` helper name preserved, only its doc comment updated to reflect new soft-delete semantic and the pre-existing >32K ID silent-no-op fallback (SEED-004 follow-up).
- `internal/sync/worker.go` — `syncStep.deleteFn` signature grew `cycleStart time.Time`; `syncSteps()` references 13 renamed `markStaleDeleted*` functions; `syncDeletePass` signature extended; call site inside `Sync` passes `start` (no new clock reading); local variable `deleted` renamed to `marked` inside the delete-pass loop; info log changed from `"deleted stale"` to `"marked stale deleted"` with count attribute `"deleted"` -> `"marked"`; error wrap string updated to `"mark stale deleted %s"`.
- `internal/sync/integration_test.go` — `TestSyncPersistsDeletedRowsUnconditional` entirely replaced by `TestSync_SoftDeleteMarksRows` (2-cycle round-trip); `TestSyncDeletesStaleRecords` assertions flipped from `orgCount != 1` + `ixCount != 1` to `orgCount != 3` + `ixCount != 2` + per-row `status="deleted"` checks; `TestSyncDeletesFKIntegrity` assertions flipped from `orgCount != 1` to `orgCount != 3` + org-2-status-deleted check; `"time"` import added.
- `internal/sync/worker_test.go` — `TestSyncHardDelete` renamed to `TestSyncSoftDeletesStale` with updated doc comment and assertion set: `count != 2` -> `count != 3` (soft-delete preserves rows) plus three new sub-assertions (`ok` count = 2, `deleted` count = 1, org 2 status transitions ok -> deleted).

## Decisions Made

- **Reused existing `start := time.Now()` as cycleStart (not a new capture):** The plan explicitly calls this out and the rationale holds — a single source-of-truth for the cycle timestamp keeps the `sync_status.LastSyncAt` value aligned with the tombstone `updated` values. Avoids sub-millisecond drift and keeps the Sync body under the REFAC-03 100-line budget.
- **Dropped the inline `// cycleStart := start` comment at the syncDeletePass call site:** The plan asked for it but adding it pushed Worker.Sync to 102 lines and tripped `TestWorkerSync_LineBudget`. The `syncDeletePass` godoc already explains the cycleStart semantic. This is a Rule 3 auto-fix (blocking issue — line-budget test failure) tracked below.
- **`TestSyncHardDelete` renamed + rewritten (not deleted):** The intent (verify cycle-N+1 reconciliation of rows absent from remoteIDs) is preserved; only the reconciliation mechanism (soft vs. hard delete) changed. Keeping the test with an updated name and assertions preserves per-field coverage that the integration-level test doesn't give us.
- **`deleteStaleChunked` helper name unchanged:** It's a private chunk-runner used by all 13 closures. Renaming would ripple through 13 callers with zero semantic benefit. Only its doc comment was updated to reflect the new soft-delete closure body. The pre-existing >32K silent-no-op fallback is flagged for SEED-004 follow-up (tombstone GC will subsume that edge case).
- **`SyncTypeDeleted` OTel metric name preserved:** Operator semantics for a row absent from the visible list are still "it's gone" even though the row physically remains as a tombstone. Renaming the metric would force dashboards to migrate across v1.15 -> v1.16 unnecessarily.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Dropped inline comment at syncDeletePass call site to stay under REFAC-03 100-line budget**
- **Found during:** Task 2 (`go test ./internal/sync -run TestWorkerSync_LineBudget` reported `Worker.Sync body is 102 lines; REFAC-03 budget is 100 lines`)
- **Issue:** The plan's Task 2 action #4 said "Add an inline comment at the call site: `// cycleStart := start (Phase 68 D-02 soft-delete uses the same timestamp captured at Sync entry)`". That 2-line comment pushed the Sync body from 100 to 102 lines, breaking the structural line-budget test introduced in Plan 54-01 Commit B.
- **Fix:** Removed the inline comment. The `syncDeletePass` godoc already documents the cycleStart semantic verbatim: "cycleStart is the single timestamp stamped on every row marked status='deleted' during this cycle (Phase 68 D-02) — reused from the Worker.Sync-entry time.Now() so all 13 types see identical timestamps." Discoverability is preserved via the godoc that any reader of the call site would click through to.
- **Files modified:** `internal/sync/worker.go`
- **Verification:** `go test ./internal/sync -race -count=1 -run 'TestWorkerSync_LineBudget'` → `ok`.
- **Committed in:** `f8f1899` (Task 3 commit — rolled into the test-fix commit since the line-budget test lives alongside the other sync tests).

**2. [Rule 1 - Bug] Flipped three pre-existing sync tests whose row-count assertions depended on hard-delete**
- **Found during:** Task 3 (`go test -race ./internal/sync -count=1` reported 3 failures after the soft-delete flip: `TestSyncHardDelete`, `TestSyncDeletesStaleRecords`, `TestSyncDeletesFKIntegrity`)
- **Issue:** All three tests asserted that rows absent from the second sync cycle's fixture were physically removed (`orgCount != 1`, `orgCount != 2`, etc.). Post-Phase-68 D-02 the rows stay in place as tombstones — row counts remain unchanged; only the `status` column transitions to `'deleted'`.
- **Fix:**
  * `TestSyncHardDelete` (worker_test.go) renamed to `TestSyncSoftDeletesStale`; `count != 2` replaced by `count != 3` + three new per-status sub-assertions (`ok=2`, `deleted=1`, org 2's status transitions `ok` -> `deleted`).
  * `TestSyncDeletesStaleRecords` (integration_test.go) assertions updated: `orgCount != 1` -> `orgCount != 3`, `ixCount != 1` -> `ixCount != 2`, added explicit `org.Status == "ok"` check on org 1 and `org2.Status == "deleted"` tombstone check.
  * `TestSyncDeletesFKIntegrity` (integration_test.go) assertions updated: `orgCount != 1` -> `orgCount != 3`, added `org2.Status == "deleted"` check; preserved the foreign_key_check pass (trivially true under soft-delete but regression-locks the two-pass sync invariant).
- **Files modified:** `internal/sync/integration_test.go`, `internal/sync/worker_test.go`
- **Verification:** `go test -race ./internal/sync -count=1` → `ok`, all 75 sync tests pass.
- **Committed in:** `f8f1899` (Task 3 commit)

---

**Total deviations:** 2 auto-fixed (1 Rule 3 blocking — line-budget failure forced drop of an explicitly-planned inline comment; 1 Rule 1 bug — three pre-existing tests' assertions were stale against the new soft-delete semantic). Both deviations are mechanical consequences of the plan's actual semantics (switching the delete pass to soft-delete necessarily changes row-count assertions on every existing test that exercised the stale-delete path; the REFAC-03 100-line budget is pre-existing and enforces Sync-orchestrator-not-doer).
**Impact on plan:** No scope creep. All auto-fixes are in-scope test maintenance for the D-02 flip. No new code paths introduced beyond what the plan specified.

## Issues Encountered

- `TestWorkerSync_LineBudget` breakage caused by the plan's explicitly-requested inline comment: resolved by dropping the comment (see Deviation #1). This was predictable in hindsight — the REFAC-03 budget has been a steady constraint since Plan 54-01.
- Three existing tests needed assertion flips rather than deletions: none of them became obsolete because the reconciliation-invariant they test still holds under soft-delete (cycle-N+1 absent rows are reconciled), just via a different mechanism (status transition vs. physical removal).

## Scope Compliance

- **D-02 implemented in full:** 13 functions flipped, hard-delete path removed from the main data path.
- **D-03 preserved:** No retroactive reconstruction; the first post-Phase-68 sync soft-deletes normally from that point onward.
- **Out-of-scope items untouched:**
  * pdbcompat (Plan 68-03 territory) — zero changes.
  * Docs (Plan 68-04 territory) — zero changes.
  * Proto (frozen since v1.6) — zero changes.
  * Config (already done in Plan 68-01) — zero changes.
- **>32K silent no-op fallback preserved:** `deleteStaleChunked`'s pre-existing fallback stays verbatim, documented inline for SEED-004 pickup. No Phase 68 attempt to resolve Open Question 4.
- **Scratch DB `DELETE FROM`:** `internal/sync/worker.go:711` uses `DELETE FROM %q` against the in-memory scratch SQLite during incremental-fallback recovery (clearing partial staging rows). That is NOT the ent/LiteFS main data path targeted by D-02, and the objective grep criterion (`grep -c "DELETE FROM" internal/sync/ returns 0 outside testdata`) was intended for the main-DB delete pass. Flagged here for transparency; no action required.

## User Setup Required

None — soft-delete flip is transparent to operators. The first post-Phase-68 sync cycle will start producing tombstones; the one-time gap (rows hard-deleted before the upgrade are gone forever per D-03) is documented in Plan 68-04's CHANGELOG entry (not landed yet).

## Next Phase Readiness

Plan 68-02 delivers STATUS-03's data prerequisite: deleted rows now exist in the DB with `status='deleted'` and a post-deletion `updated` timestamp. Pre-conditions for the remaining Phase 68 plans:

- **Plan 68-03 (status × since matrix in pdbcompat):** The soft-delete flip means `client.X.Query().Where(status="deleted", updated >= since)` will now return real rows for the first time since v1.0. Plan 68-03's new `applyStatusMatrix` helper can assume tombstones exist. The STATUS-01 (list default `status=ok`) and STATUS-07 (`?status=deleted` without `since` = empty) rules filter the tombstones back out for normal list traffic, so pdbcompat behaviour is unchanged for callers who don't pass `since=N`.
- **Plan 68-04 (CHANGELOG.md bootstrap):** The soft-delete-flip is the headline breaking/behavioural change for v1.16. The one-time gap (D-03: pre-Phase-68 hard-deleted rows are gone forever) is the migration note that Plan 68-04 will surface to operators.

No blockers for the rest of Phase 68.

---
*Phase: 68-status-since-matrix*
*Completed: 2026-04-19*

## Self-Check: PASSED

- FOUND: internal/sync/delete.go (13 markStaleDeleted* functions; `"time"` import; deleteStaleChunked helper preserved)
- FOUND: internal/sync/worker.go (syncStep.deleteFn signature with cycleStart; syncDeletePass passes cycleStart; Sync call site reuses start)
- FOUND: internal/sync/integration_test.go (TestSync_SoftDeleteMarksRows at line ~367; `"time"` import added)
- ABSENT: TestSyncPersistsDeletedRowsUnconditional (replaced entirely)
- ABSENT: TestSyncIncludeDeleted (removed in Plan 68-01)
- FOUND: internal/sync/worker_test.go (TestSyncSoftDeletesStale renamed from TestSyncHardDelete)
- FOUND: commit 8bd31a4 (Task 1 — 13 functions flipped)
- FOUND: commit 4d20735 (Task 2 — cycleStart plumbing)
- FOUND: commit f8f1899 (Task 3 — test flip + line-budget fix)
- PASS: `go build ./...`
- PASS: `go vet ./...`
- PASS: `go test -race ./... -count=1 -timeout 300s` (entire repo; internal/sync 12.683s)
- PASS: `golangci-lint run ./internal/sync/...` (0 issues)
- PASS: `grep -c "^func markStaleDeleted" internal/sync/delete.go` = 13
- PASS: `grep -c "SetStatus(\"deleted\")" internal/sync/delete.go` = 13
- PASS: `grep -c "SetUpdated(cycleStart)" internal/sync/delete.go` = 13
- PASS: `grep -c "\.Delete()\.Where" internal/sync/delete.go` = 0
- PASS: `grep -n "deleteStale[A-Z]" internal/sync/worker.go` = no matches
- PASS: `grep -c "markStaleDeleted" internal/sync/worker.go` = 13
- PASS: Verbose run of TestSync_SoftDeleteMarksRows emits 13 lines `"marked stale deleted type=X marked=1"` during cycle 2.
