---
phase: 08-incremental-sync
plan: 03
subsystem: sync
tags: [sync, incremental, cursor, fallback, config, otel]

# Dependency graph
requires:
  - phase: 08-incremental-sync/01
    provides: "SyncMode config, FetchAll functional options, WithSince, FetchResult"
  - phase: 08-incremental-sync/02
    provides: "sync_cursors table, GetCursor, UpsertCursor, SyncTypeFallback metric"
provides:
  - "Mode-aware Sync(ctx, mode) with incremental/full branching"
  - "13 per-type incremental sync methods using fetchIncremental generic helper"
  - "Per-type fallback from incremental to full with counter and WARN log"
  - "Cursor persistence after successful commit (not on rollback)"
  - "POST /sync ?mode=full|incremental query param override"
  - "SyncWithRetry and StartScheduler pass mode through"
affects: [deployment, observability]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "fetchIncremental[T] generic helper for incremental fetch + unmarshal + generated timestamp"
    - "syncStep struct with dual fn/incrementalFn for full/incremental branching"
    - "cursorUpdates map collected during sync loop, flushed after commit"

key-files:
  created: []
  modified:
    - internal/sync/worker.go
    - internal/sync/worker_test.go
    - internal/sync/integration_test.go
    - cmd/peeringdb-plus/main.go

key-decisions:
  - "fetchIncremental[T] calls FetchAll directly (not FetchType) to access FetchResult.Meta.Generated"
  - "Cursor updates collected in map during sync loop, written only after tx.Commit succeeds"
  - "Full sync also writes cursors so first full sync establishes cursors for subsequent incremental syncs"
  - "failIncremental test fixture pattern: fail all ?since= requests, succeed without ?since="

patterns-established:
  - "syncStep dual-function pattern: fn for full sync, incrementalFn for incremental sync"
  - "Mode parameter threading: Sync -> SyncWithRetry -> StartScheduler all accept/pass config.SyncMode"

requirements-completed: [SYNC-01, SYNC-04, SYNC-05]

# Metrics
duration: 8min
completed: 2026-03-23
---

# Phase 08 Plan 03: Sync Worker Orchestration Summary

**Mode-aware sync worker with per-type incremental fetch via WithSince, automatic fallback to full on failure, and cursor persistence only after successful commit**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-23T22:52:24Z
- **Completed:** 2026-03-23T23:00:32Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Sync method branches between incremental and full paths based on SyncMode parameter
- 13 incremental sync methods created using fetchIncremental[T] generic helper -- each calls FetchAll with WithSince, upserts without deleteStale, returns generated timestamp
- Incremental failure triggers immediate full fallback per type with SyncTypeFallback counter increment and WARN log
- Per-type cursors updated only after successful transaction commit (never on rollback)
- POST /sync handler accepts ?mode=full|incremental query param with validation (400 on invalid)
- SyncWithRetry and StartScheduler pass mode parameter through the call chain
- 7 new tests covering incremental sync, first-sync-full behavior, fallback, cursor persistence, rollback protection, mode passthrough, and delete-stale skipping
- Full test suite passes with -race across all packages

## Task Commits

Each task was committed atomically:

1. **Task 1+2: Mode-aware sync orchestration + main.go wiring** - `b7a973c` (feat)

_Note: Tasks 1 and 2 committed together because main.go changes were required for compilation of the sync worker changes._

## Files Created/Modified
- `internal/sync/worker.go` - Added SyncMode to WorkerConfig, mode parameter to Sync/SyncWithRetry/StartScheduler, syncStep.incrementalFn field, 13 incremental sync methods, fetchIncremental generic helper, cursor update logic after commit
- `internal/sync/worker_test.go` - Updated all existing tests to pass SyncModeFull, added fixtureWithMeta test helper, 7 new tests for incremental behavior
- `internal/sync/integration_test.go` - Updated all Sync calls to pass config.SyncModeFull
- `cmd/peeringdb-plus/main.go` - Added SyncMode to WorkerConfig, ?mode= query param parsing in POST /sync handler with validation

## Decisions Made
- fetchIncremental[T] calls FetchAll directly rather than FetchType to access FetchResult.Meta.Generated for cursor timestamps
- Cursor updates collected in a map during the sync loop and written only after tx.Commit succeeds -- ensures atomicity
- Full sync also writes cursors so that the first full sync establishes cursors for subsequent incremental syncs
- Used failIncremental fixture pattern (fail all ?since= requests) instead of failOnce to work around PeeringDB client retry logic

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] main.go changes required in Task 1 for compilation**
- **Found during:** Task 1 (mode-aware sync orchestration)
- **Issue:** Changing Sync/SyncWithRetry signatures broke all callers including main.go. Task 2 changes were required for the project to compile.
- **Fix:** Applied Task 2's main.go changes (WorkerConfig SyncMode field, ?mode= query param parsing) as part of the Task 1 commit.
- **Files modified:** cmd/peeringdb-plus/main.go
- **Verification:** `go build ./...` and `go test -race ./...` pass
- **Committed in:** b7a973c (combined commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Minimal -- Task 2 was merged into Task 1 commit for compilation reasons. All acceptance criteria for both tasks are met.

## Issues Encountered
- Initial TestIncrementalFallback used failOnce which didn't work because PeeringDB client retry logic (3 retries with exponential backoff) transparently retried the 500 error. Switched to failIncremental pattern that fails all requests with ?since= parameter while allowing non-since requests to succeed.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 08 incremental sync is complete -- all 3 plans delivered
- SyncMode can be configured via PDBPLUS_SYNC_MODE env var or overridden per-request via POST /sync?mode=
- Ready for phase transition to Phase 09 (golden file tests) or Phase 10 (CI pipeline)

## Self-Check: PASSED

- All 4 source files exist
- Task commit verified (b7a973c)
- SUMMARY.md created
- All acceptance criteria met (SyncModeIncremental, WithSince, SyncTypeFallback, UpsertCursor, GetCursor, incrementalFn, TestIncrementalSync, TestIncrementalFallback)

---
*Phase: 08-incremental-sync*
*Completed: 2026-03-23*
