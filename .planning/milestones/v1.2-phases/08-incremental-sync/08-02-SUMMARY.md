---
phase: 08-incremental-sync
plan: 02
subsystem: sync
tags: [sqlite, cursor, metrics, otel, incremental-sync]

# Dependency graph
requires:
  - phase: 01-data-layer
    provides: "ent client, SQLite database, sync infrastructure"
  - phase: 08-incremental-sync/01
    provides: "since parameter support in PeeringDB client"
provides:
  - "sync_cursors table for per-type cursor tracking"
  - "GetCursor function for retrieving last successful sync timestamp"
  - "UpsertCursor function for insert/update of sync cursors"
  - "SyncTypeFallback OTel counter metric"
affects: [08-incremental-sync/03]

# Tech tracking
tech-stack:
  added: []
  patterns: ["ON CONFLICT upsert for SQLite cursor persistence", "status-filtered cursor reads"]

key-files:
  created: ["internal/sync/status_test.go"]
  modified: ["internal/sync/status.go", "internal/otel/metrics.go"]

key-decisions:
  - "Cursor table uses TEXT PRIMARY KEY on type column with ON CONFLICT for upsert"
  - "GetCursor filters by last_status='success' so failed syncs force full re-fetch"

patterns-established:
  - "Cursor CRUD pattern: GetCursor/UpsertCursor pair for per-type sync tracking"

requirements-completed: [SYNC-03]

# Metrics
duration: 4min
completed: 2026-03-23
---

# Phase 08 Plan 02: Cursor Persistence & Fallback Metric Summary

**Per-type sync cursor persistence via sync_cursors table with success-filtered reads, plus SyncTypeFallback OTel counter for incremental-to-full fallback tracking**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-23T22:43:06Z
- **Completed:** 2026-03-23T22:47:30Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Extended InitStatusTable to create sync_cursors table alongside sync_status
- Added GetCursor/UpsertCursor functions with success-status filtering and ON CONFLICT upsert
- Registered SyncTypeFallback counter metric in InitMetrics for observability
- Full test coverage with 5 table-driven tests including race detection

## Task Commits

Each task was committed atomically:

1. **Task 1: Add sync_cursors table and cursor CRUD functions** - `2fac2b2` (test: RED), `bf592e7` (feat: GREEN)
2. **Task 2: Add SyncTypeFallback counter metric** - `32bada9` (feat)

**Plan metadata:** TBD (docs: complete plan)

_Note: Task 1 used TDD with RED/GREEN commits_

## Files Created/Modified
- `internal/sync/status.go` - Extended InitStatusTable with sync_cursors DDL, added GetCursor and UpsertCursor functions
- `internal/sync/status_test.go` - 5 test cases covering table creation, CRUD, and status filtering
- `internal/otel/metrics.go` - Added SyncTypeFallback Int64Counter registration

## Decisions Made
- Cursor table uses TEXT PRIMARY KEY on type column -- one row per object type, upserted with ON CONFLICT
- GetCursor only returns cursors where last_status='success' -- a failed sync for a type means next sync will be a full re-fetch (zero time cursor)
- UpsertCursor accepts an explicit status parameter so callers can record both success and failure

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- GetCursor and UpsertCursor are ready to be called by the sync worker loop (wired in Plan 03)
- SyncTypeFallback metric ready for Plan 03 to call .Add() on incremental-to-full fallback events

## Self-Check: PASSED

All files exist, all commits verified.

---
*Phase: 08-incremental-sync*
*Completed: 2026-03-23*
