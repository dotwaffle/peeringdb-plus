---
phase: 04-observability-foundations
plan: 02
subsystem: observability
tags: [otel, metrics, opentelemetry, sync, gauge, histogram, counter]

# Dependency graph
requires:
  - phase: 03-production-readiness
    provides: OTel MeterProvider initialization and existing SyncDuration/SyncOperations instruments
provides:
  - 5 per-type sync metric instruments (duration, objects, deleted, fetch_errors, upsert_errors)
  - Sync-level metric recording (SyncDuration.Record, SyncOperations.Add with status attribute)
  - Freshness observable gauge computing seconds-since-last-sync on demand
affects: [04-observability-foundations]

# Tech tracking
tech-stack:
  added: []
  patterns: [per-type metric recording with type attribute, observable gauge callback pattern, fetch vs upsert error classification by error prefix]

key-files:
  created: []
  modified:
    - internal/otel/metrics.go
    - internal/otel/metrics_test.go
    - internal/sync/worker.go
    - internal/sync/worker_test.go
    - cmd/peeringdb-plus/main.go

key-decisions:
  - "Use callback-based InitFreshnessGauge to avoid circular dependency between otel and sync packages"
  - "Classify fetch vs upsert errors by checking error message prefix per existing convention"
  - "Non-parallel metric tests to avoid data races on package-level metric vars per CC-3"

patterns-established:
  - "Per-type metrics: use metric.WithAttributes(attribute.String('type', step.name)) for all per-step instruments"
  - "Observable gauge: accept callback function to compute value on demand, no background goroutine"

requirements-completed: [OBS-01, OBS-03, OBS-04]

# Metrics
duration: 5min
completed: 2026-03-22
---

# Phase 4 Plan 2: Sync Metrics Wiring Summary

**5 per-type sync metric instruments registered and wired with SyncDuration/SyncOperations recording and freshness observable gauge computing seconds-since-last-sync on demand**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-22T21:37:03Z
- **Completed:** 2026-03-22T21:42:00Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Registered 5 new per-type metric instruments (SyncTypeDuration histogram, SyncTypeObjects/SyncTypeDeleted/SyncTypeFetchErrors/SyncTypeUpsertErrors counters) with pdbplus.sync.type.* naming per D-08
- Wired SyncDuration.Record() and SyncOperations.Add() in sync worker at both success and failure paths with status attribute per D-06
- Added per-step metric recording in sync loop: duration, objects synced, objects deleted, and fetch/upsert error classification per D-07/D-10
- Created InitFreshnessGauge with observable gauge callback that computes seconds-since-last-sync on demand per D-09
- Tests verify actual metric values via ManualReader, not just no-panic checks

## Task Commits

Each task was committed atomically:

1. **Task 1: Register per-type metric instruments and freshness gauge in metrics.go** - `401f583` (feat)
2. **Task 2: Wire metric recording in sync worker and freshness gauge in main.go** - `7b06ecd` (feat)

## Files Created/Modified
- `internal/otel/metrics.go` - 5 new per-type metric vars, InitMetrics registration, InitFreshnessGauge function
- `internal/otel/metrics_test.go` - Tests for per-type instruments, freshness gauge, ManualReader value verification
- `internal/sync/worker.go` - Per-step metric recording, sync-level metric recording, fetch/upsert error classification
- `internal/sync/worker_test.go` - TestSyncRecordsMetrics and TestSyncRecordsFailureMetrics with ManualReader assertions
- `cmd/peeringdb-plus/main.go` - InitFreshnessGauge call with callback querying sync_status table

## Decisions Made
- Used callback function parameter for InitFreshnessGauge to avoid circular import between otel and sync packages
- Classified fetch vs upsert errors by checking if error.Error() starts with "fetch " -- this matches the existing convention where all sync step methods wrap fetch errors as "fetch {type}: ..."
- Made TestSyncRecordsMetrics and TestSyncRecordsFailureMetrics non-parallel to avoid data races on package-level metric variables (CC-3)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Race condition detected when running metric tests in parallel due to concurrent writes to package-level metric vars via InitMetrics(). Fixed by removing t.Parallel() from the two new metric tests per CC-3.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All sync metric instruments are now registered and actively recording values
- Freshness gauge provides on-demand computation of data staleness
- Ready for dashboard creation and alerting configuration in future phases

## Self-Check: PASSED

All 5 files verified present. Both commit hashes (401f583, 7b06ecd) verified in git log.

---
*Phase: 04-observability-foundations*
*Completed: 2026-03-22*
