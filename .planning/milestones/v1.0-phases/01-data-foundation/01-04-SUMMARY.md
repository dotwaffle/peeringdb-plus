---
phase: 01-data-foundation
plan: 04
subsystem: sync
tags: [sqlite, ent, peeringdb, sync, otel, transaction, retry, mutex]

# Dependency graph
requires:
  - phase: 01-data-foundation/01-02
    provides: "Generated ent client with all 13 types, bulk upsert, and OnConflictColumns"
  - phase: 01-data-foundation/01-03
    provides: "PeeringDB API client with FetchType generic, rate limiting, and retry"
provides:
  - "Sync worker that orchestrates full PeeringDB data sync into local SQLite"
  - "Bulk upsert functions for all 13 PeeringDB types with OnConflictColumns"
  - "Hard delete functions for stale row removal"
  - "sync_status metadata table for tracking sync operations"
  - "Mutex-protected sync with exponential backoff retry"
  - "Periodic scheduler via time.Ticker with SyncWithRetry"
  - "HasCompletedSync() for 503 readiness gating"
affects: [01-data-foundation/01-06, 01-data-foundation/01-07, 02-api-surfaces]

# Tech tracking
tech-stack:
  added: [go.opentelemetry.io/otel]
  patterns: [bulk-upsert-with-batching, hard-delete-stale-rows, sync-mutex-atomic-bool, exponential-backoff-retry, status-table-raw-sql]

key-files:
  created:
    - internal/sync/worker.go
    - internal/sync/upsert.go
    - internal/sync/delete.go
    - internal/sync/status.go
    - internal/sync/filter.go
    - internal/sync/worker_test.go
  modified:
    - internal/testutil/testutil.go
    - internal/peeringdb/client.go

key-decisions:
  - "Per-instance retry backoffs instead of package-level var to avoid data races in parallel tests"
  - "Raw SQL for sync_status table since it is operational metadata, not an ent-managed entity"
  - "Per-type filter functions instead of generic filter due to Go lacking field-access constraints on generics"
  - "Added SetRateLimit/SetRetryBaseDelay to peeringdb.Client and SetupClientWithDB to testutil for testing support"

patterns-established:
  - "Bulk upsert pattern: build slice of CreateBulk builders, batch in chunks of 500, OnConflictColumns(FieldID).UpdateNewValues()"
  - "Hard delete pattern: Delete().Where(type.IDNotIn(remoteIDs...)).Exec(ctx)"
  - "Sync step pattern: slice of syncStep{name, fn} iterated in FK dependency order"
  - "Status table pattern: raw SQL CREATE TABLE IF NOT EXISTS for operational metadata outside ent"
  - "Test fixture pattern: httptest.Server with configurable responses/failTypes maps, pagination-aware"

requirements-completed: [DATA-03, DATA-04]

# Metrics
duration: 18min
completed: 2026-03-22
---

# Phase 01 Plan 04: Sync Worker Summary

**Full PeeringDB sync worker with 13-type bulk upsert in FK dependency order, single-transaction atomicity, hard delete, mutex, 30s/2m/8m exponential backoff retry, and sync_status tracking**

## Performance

- **Duration:** 18 min
- **Started:** 2026-03-22T15:18:48Z
- **Completed:** 2026-03-22T15:36:40Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Bulk upsert functions for all 13 PeeringDB types with OnConflictColumns and 500-row batching
- Hard delete functions for stale row removal using IDNotIn predicate
- sync_status table with raw SQL for tracking sync metadata (start, complete, object counts, errors)
- Sync worker that orchestrates full sync in FK dependency order within a single database transaction
- Mutex via atomic.Bool CompareAndSwap preventing concurrent sync runs
- SyncWithRetry with 30s/2m/8m exponential backoff on failure
- Periodic scheduler using time.Ticker calling SyncWithRetry
- HasCompletedSync() for 503 readiness gating
- OTel spans around full sync and per-type operations
- Per-object-type progress logging with structured slog
- status=deleted filtering configurable via WorkerConfig.IncludeDeleted
- 16 passing tests with -race covering all behaviors

## Task Commits

Each task was committed atomically:

1. **Task 1: Create sync upsert, delete, and status tracking logic** - `9fd89fe` (feat)
2. **Task 2: Create sync worker orchestrator (TDD RED)** - `b0aa2ae` (test)
3. **Task 2: Create sync worker orchestrator (TDD GREEN)** - `f8c93bf` (feat)

## Files Created/Modified
- `internal/sync/upsert.go` - Bulk upsert functions for all 13 PeeringDB types with OnConflictColumns
- `internal/sync/delete.go` - Hard delete functions for stale row removal (IDNotIn predicate)
- `internal/sync/status.go` - sync_status table management (init, start, complete, query)
- `internal/sync/worker.go` - Sync orchestrator with mutex, retry, scheduling, OTel spans
- `internal/sync/filter.go` - Per-type status=deleted filter functions
- `internal/sync/worker_test.go` - 16 tests covering all sync behaviors
- `internal/testutil/testutil.go` - Added SetupClientWithDB returning both *ent.Client and *sql.DB
- `internal/peeringdb/client.go` - Added SetRateLimit and SetRetryBaseDelay for testing

## Decisions Made
- Used per-instance `retryBackoffs` field on Worker instead of package-level var to avoid data races in parallel tests
- Used raw SQL for sync_status table since it is operational metadata outside ent's schema management
- Created per-type filter functions instead of a generic approach because Go does not support field-access constraints on generic type parameters
- Added SetRateLimit/SetRetryBaseDelay to peeringdb.Client to enable fast testing without rate limiter delays
- Added SetupClientWithDB to testutil to provide raw sql.DB alongside ent client for sync_status operations

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added SetRateLimit/SetRetryBaseDelay to peeringdb.Client**
- **Found during:** Task 2 (worker tests)
- **Issue:** PeeringDB client's hardcoded 1 req/3s rate limiter and 2s retry base delay made tests take minutes and timeout
- **Fix:** Added SetRateLimit() and SetRetryBaseDelay() methods to peeringdb.Client for test override
- **Files modified:** internal/peeringdb/client.go
- **Verification:** All tests pass within 5 seconds
- **Committed in:** f8c93bf

**2. [Rule 3 - Blocking] Added SetupClientWithDB to testutil**
- **Found during:** Task 2 (worker tests)
- **Issue:** ent.Client does not expose underlying *sql.DB needed for sync_status raw SQL operations in tests
- **Fix:** Added SetupClientWithDB() that returns both *ent.Client and *sql.DB via shared-cache in-memory SQLite
- **Files modified:** internal/testutil/testutil.go
- **Verification:** All tests use SetupClientWithDB successfully
- **Committed in:** f8c93bf

**3. [Rule 3 - Blocking] Per-instance retry backoffs instead of package-level var**
- **Found during:** Task 2 (worker tests with -race)
- **Issue:** Package-level syncRetryBackoffs var caused data races when parallel tests modified it
- **Fix:** Moved to per-instance retryBackoffs field on Worker with SetRetryBackoffs() method
- **Files modified:** internal/sync/worker.go, internal/sync/worker_test.go
- **Verification:** go test -race passes with all parallel tests
- **Committed in:** f8c93bf

---

**Total deviations:** 3 auto-fixed (3 blocking)
**Impact on plan:** All auto-fixes necessary for test correctness and parallel safety. No scope creep.

## Issues Encountered
- Mock HTTP server needed pagination awareness (return empty data on skip>0) to prevent infinite pagination loops in tests

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all sync functionality is fully wired and operational.

## Next Phase Readiness
- Sync worker is ready to be integrated into the main application (cmd/peeringdb-plus)
- The sync worker depends on Plans 02 (ent schemas) and 03 (PeeringDB client), both completed
- Ready for Plan 06 (server/main) which wires the sync worker into the HTTP server
- sync_status table provides data for health endpoints in Phase 3

## Self-Check: PASSED

All 6 created files verified present. All 3 commit hashes verified in git log.

---
*Phase: 01-data-foundation*
*Completed: 2026-03-22*
