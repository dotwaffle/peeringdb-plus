---
phase: 49-refactoring-tech-debt
plan: 01
subsystem: web
tags: [refactoring, go, detail-handlers, file-splitting]

requires:
  - phase: none
    provides: existing detail.go with 1422 lines of mixed query and handler logic
provides:
  - 6 per-entity query files (query_network.go, query_ix.go, query_facility.go, query_org.go, query_campus.go, query_carrier.go)
  - slimmed detail.go with only handlers, fragments, and getFreshness
affects: [internal/web]

tech-stack:
  added: []
  patterns:
    - "Per-entity query files in internal/web/ separate data fetching from HTTP handling"

key-files:
  created:
    - internal/web/query_network.go
    - internal/web/query_ix.go
    - internal/web/query_facility.go
    - internal/web/query_org.go
    - internal/web/query_campus.go
    - internal/web/query_carrier.go
  modified:
    - internal/web/detail.go

key-decisions:
  - "Query files get only the queryXxx function, no fragment handlers -- fragments are routing logic that stays in detail.go"

patterns-established:
  - "query_*.go naming convention for per-entity data fetching methods on Handler"

requirements-completed: [REFAC-01]

duration: 5min
completed: 2026-04-02
---

# Phase 49 Plan 01: Split detail.go into per-entity query files Summary

**Split 1422-line detail.go into 6 focused query files (61-177 lines each) plus 769-line handler/fragment file**

## Performance

- **Duration:** 5min
- **Started:** 2026-04-02T05:03:55Z
- **Completed:** 2026-04-02T05:09:20Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Extracted 6 queryXxx functions into dedicated per-entity files, all under 300 lines
- Reduced detail.go from 1422 to 769 lines (handlers, fragments, getFreshness remain)
- All 479 existing tests pass with race detector, go vet clean

## Task Commits

Each task was committed atomically:

1. **Task 1: Extract query functions into per-entity files** - `7bee10f` (refactor)
2. **Task 2: Verify no file exceeds 300 lines and all tests pass** - verification only, no code changes needed

**Plan metadata:** (pending)

## Files Created/Modified
- `internal/web/query_network.go` - queryNetwork and network-specific helpers (142 lines)
- `internal/web/query_ix.go` - queryIX and IX-specific helpers (148 lines)
- `internal/web/query_facility.go` - queryFacility and facility-specific helpers (127 lines)
- `internal/web/query_org.go` - queryOrg and org-specific helpers (177 lines)
- `internal/web/query_campus.go` - queryCampus and campus-specific helpers (75 lines)
- `internal/web/query_carrier.go` - queryCarrier and carrier-specific helpers (61 lines)
- `internal/web/detail.go` - retained handleXxxDetail, handleFragment, all fragment handlers, getFreshness (769 lines)

## Decisions Made
- Query files contain only the queryXxx function per entity, no fragment handlers -- fragments are routing/rendering logic that stays in detail.go
- detail.go retains all imports needed by fragment handlers (the 300-line limit applies to query files, not the handler file)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- File structure established: future per-entity changes can be made in isolated files
- Remaining plans in phase 49 (upsert dedup, test coverage, about rendering, seed consolidation) are independent

## Self-Check: PASSED

---
*Phase: 49-refactoring-tech-debt*
*Completed: 2026-04-02*
