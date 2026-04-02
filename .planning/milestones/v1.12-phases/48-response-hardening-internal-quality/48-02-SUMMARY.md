---
phase: 48-response-hardening-internal-quality
plan: 02
subsystem: api, observability
tags: [graphql, otel, metrics, ent, error-handling, caching]

requires:
  - phase: 48-01
    provides: "Error handling hardening foundation, CONTEXT.md decisions for PERF-02 and PERF-03"
provides:
  - "Type-safe GraphQL error classification using ent error type checks (GO-ERR-2 compliant)"
  - "Cached metrics object count gauge reading pre-computed values instead of live COUNT queries"
  - "OnSyncComplete callback on sync worker for post-sync cache updates"
affects: [sync, otel, graphql]

tech-stack:
  added: []
  patterns:
    - "Cached observable gauge pattern: sync worker pushes counts via callback, metrics reads from atomic cache"
    - "ent type-safe error classification: ent.IsNotFound/IsValidationError/IsConstraintError instead of string matching"

key-files:
  created: [internal/graphql/handler_test.go]
  modified: [internal/graphql/handler.go, internal/otel/metrics.go, internal/otel/metrics_test.go, internal/sync/worker.go, cmd/peeringdb-plus/main.go]

key-decisions:
  - "ValidationError test construction: wrapped in fmt.Errorf to avoid panicking on unexported nil err field"
  - "OnSyncComplete callback only fires from Sync method (after actual sync), not from startup detection paths"

patterns-established:
  - "Cached observable gauge: sync worker callback updates atomic.Pointer cache, gauge reads from cache function"

requirements-completed: [PERF-02, PERF-03]

duration: 7min
completed: 2026-04-02
---

# Phase 48 Plan 02: GraphQL Error Classification and Cached Metrics Gauge Summary

**Type-safe ent error classification in GraphQL (fixing GO-ERR-2) and cached object count gauge eliminating 13 live COUNT queries per metrics scrape**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-02T04:43:53Z
- **Completed:** 2026-04-02T04:51:12Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Replaced string-matching classifyError with ent.IsNotFound/IsValidationError/IsConstraintError type checks (GO-ERR-2 fix)
- Added CONSTRAINT_ERROR code for constraint violations (new classification)
- Converted InitObjectCountGauges from live ent.Client COUNT queries to cached countsFn callback (PERF-02)
- Added OnSyncComplete callback to WorkerConfig for post-sync cache updates
- Wired atomic.Pointer cache in main.go between sync worker and metrics gauge
- 8 new test cases for classifyError covering nil, direct, wrapped, and unknown errors

## Task Commits

Each task was committed atomically:

1. **Task 1: Replace string-matching classifyError with ent type checks** - `d45e483` (feat)
   - TDD RED: `test(48-02): add failing tests` (separate commit)
   - TDD GREEN: `feat(48-02): replace string-matching classifyError with ent type checks`
2. **Task 2: Cached metrics object count gauge with sync worker callback** - `16b308c` (feat)

## Files Created/Modified
- `internal/graphql/handler.go` - classifyError now uses ent type checks instead of strings.Contains
- `internal/graphql/handler_test.go` - New: 8 table-driven tests for classifyError
- `internal/otel/metrics.go` - InitObjectCountGauges takes countsFn callback instead of ent.Client
- `internal/otel/metrics_test.go` - Updated tests to use cache-based API
- `internal/sync/worker.go` - OnSyncComplete callback in WorkerConfig, invoked after successful sync
- `cmd/peeringdb-plus/main.go` - atomic.Pointer cache wiring between sync worker and metrics gauge

## Decisions Made
- ValidationError has unexported err field; test constructs it via fmt.Errorf wrapping to avoid nil dereference panic
- OnSyncComplete callback only fires from the Sync method (after actual sync completes), not from startup detection paths (lines 457, 466 in worker.go) that set synced=true from prior data
- Removed BAD_REQUEST case from classifyError per CONTEXT.md (limit/offset validation covered by ent validation error check)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Updated existing otel metrics tests for new API**
- **Found during:** Task 2
- **Issue:** Three existing test functions in metrics_test.go referenced old InitObjectCountGauges(client *ent.Client) signature
- **Fix:** Updated TestInitObjectCountGauges_NoError, TestInitObjectCountGauges_RecordsValues, and TestInitObjectCountGauges_ErrorInCallback to use new countsFn-based API; removed unused testutil import
- **Files modified:** internal/otel/metrics_test.go
- **Verification:** All 33 otel tests pass with -race
- **Committed in:** 16b308c (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Necessary to maintain existing test coverage with new API signature. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- GraphQL error classification is now type-safe and handles wrapped errors correctly
- Metrics gauge reads from cache, eliminating per-scrape database load
- Both PERF-02 and PERF-03 requirements complete

---
*Phase: 48-response-hardening-internal-quality*
*Completed: 2026-04-02*
