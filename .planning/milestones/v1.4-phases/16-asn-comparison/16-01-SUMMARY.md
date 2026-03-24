---
phase: 16-asn-comparison
plan: 01
subsystem: api
tags: [comparison, ent, errgroup, set-intersection, templates]

# Dependency graph
requires:
  - phase: 15-record-detail
    provides: detail page types, handler patterns, ent query patterns
provides:
  - CompareService with Compare method for two-network comparison
  - Template data types for comparison rendering (CompareData, CompareIXP, etc.)
  - Shared IXP/facility/campus computation via set intersection
  - Full view mode with union and shared flags
affects: [16-02-PLAN, compare-page, compare-handler]

# Tech tracking
tech-stack:
  added: []
  patterns: [errgroup fan-out for parallel presence queries, map-based set intersection, campus derivation from facility edges]

key-files:
  created:
    - internal/web/templates/comparetypes.go
    - internal/web/compare.go
    - internal/web/compare_test.go
  modified: []

key-decisions:
  - "Map-based set intersection for IX and facility overlap detection"
  - "Campus derivation queries facilities with HasCampus() + WithCampus() eager loading"
  - "Sort all result slices by name for deterministic output"

patterns-established:
  - "CompareService pattern: service struct with ent client, input struct, returns template types"
  - "Set intersection via map[int] lookup for O(n+m) overlap detection"

requirements-completed: [COMP-01, COMP-02, COMP-03, COMP-04]

# Metrics
duration: 4min
completed: 2026-03-24
---

# Phase 16 Plan 01: Comparison Service Summary

**CompareService with errgroup-parallel ent queries computing shared IXPs, facilities, and campuses via map-based set intersection**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-24T07:09:02Z
- **Completed:** 2026-03-24T07:13:25Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- CompareService loads IX and facility presences for two networks in parallel via errgroup
- Shared IXPs/facilities computed by map-based set intersection on IX ID and facility ID
- Shared campuses derived from facility campus edges with WithCampus() eager loading
- Full view mode produces union of all presences with shared flags
- 7 tests covering shared IXPs, shared facilities, shared campuses, no overlap, full view IXPs, full view facilities, and invalid ASN

## Task Commits

Each task was committed atomically:

1. **Task 1: Define comparison data types and comparison service** - `4709abc` (feat)
2. **Task 2: Add tests for CompareService** - `62c269c` (test)

## Files Created/Modified
- `internal/web/templates/comparetypes.go` - CompareData and all sub-types for template rendering
- `internal/web/compare.go` - CompareService with Compare method, set intersection, campus derivation
- `internal/web/compare_test.go` - 7 test functions with full data seeding

## Decisions Made
- Map-based set intersection for O(n+m) overlap detection instead of nested loops
- Campus derivation queries facilities with HasCampus() predicate and WithCampus() eager loading to avoid N+1
- All result slices sorted alphabetically by name for deterministic output and consistent rendering
- CompareInput struct used per CS-5 since Compare takes ctx + 3 parameters

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- CompareService ready for HTTP handler integration in plan 16-02
- Template types ready for templ component rendering
- All tests pass with -race flag

## Self-Check: PASSED

All 3 created files exist. Both task commits (4709abc, 62c269c) verified in git log.

---
*Phase: 16-asn-comparison*
*Completed: 2026-03-24*
