---
phase: 34-query-optimization-architecture
plan: 03
subsystem: architecture
tags: [termrender, dispatch, detail-handlers, refactor, generics]

# Dependency graph
requires:
  - phase: 29-network-detail-reference
    provides: "Type-switch dispatch in RenderPage, entity renderer methods"
  - phase: 30-entity-types-search-formats
    provides: "All 8 entity-type renderers registered in type-switch"
provides:
  - "Generic Register function for terminal renderer dispatch"
  - "Registered function map replacing type-switch in RenderPage"
  - "queryXxx methods separating data fetching from HTTP handling in detail.go"
affects: [36-ui-terminal-polish]

# Tech tracking
tech-stack:
  added: []
  patterns: [registered-function-map-dispatch, query-method-extraction]

key-files:
  created:
    - internal/web/termrender/dispatch.go
    - internal/web/termrender/dispatch_test.go
  modified:
    - internal/web/termrender/renderer.go
    - internal/web/detail.go

key-decisions:
  - "reflect.TypeOf dispatch map over interface-based polymorphism for terminal renderers"
  - "queryXxx methods return templates.XxxDetail structs (same types consumed by templates and renderers)"
  - "Updated slog.String to slog.Any in handler error logging as QUAL-02 improvement"

patterns-established:
  - "Register[T] generic function for adding new entity renderers without modifying RenderPage"
  - "queryXxx(ctx, id) pattern returning typed data structs for testable query separation"

requirements-completed: [ARCH-04, QUAL-04]

# Metrics
duration: 9min
completed: 2026-03-26
---

# Phase 34 Plan 03: Renderer Dispatch & Detail Handler Refactor Summary

**Generic Register function replaces type-switch dispatch in terminal RenderPage; all 6 detail handlers refactored to 29-line bodies with extracted queryXxx methods**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-26T07:08:14Z
- **Completed:** 2026-03-26T07:17:38Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Replaced type-switch in RenderPage with reflect.TypeOf registered function map (ARCH-04)
- All 8 entity types registered via generic Register[T] function in init()
- Extracted queryNetwork, queryIX, queryFacility, queryOrg, queryCampus, queryCarrier methods
- All 6 handler bodies reduced from 68-170 lines to 29 lines each (QUAL-04)

## Task Commits

Each task was committed atomically:

1. **Task 1: Interface-based terminal renderer dispatch via registered function map** - `22bc36f` (feat)
2. **Task 2: Extract query logic from detail handlers into queryXxx methods** - `1bb641a` (refactor)

## Files Created/Modified
- `internal/web/termrender/dispatch.go` - Generic Register function, renderer registry map, init() registrations for all 8 types
- `internal/web/termrender/dispatch_test.go` - Table-driven dispatch tests, custom type registration test
- `internal/web/termrender/renderer.go` - RenderPage now uses dispatch map instead of type-switch
- `internal/web/detail.go` - 6 queryXxx methods extracted, handlers reduced to parse-query-render pattern

## Decisions Made
- Used reflect.TypeOf dispatch map over interface-based polymorphism -- simpler, no interface to implement per renderer, and preserves existing method signatures unchanged
- queryXxx methods return the same templates.XxxDetail types consumed by templates and terminal renderers, maintaining the existing data flow
- Updated slog.String("error", err.Error()) to slog.Any("error", err) in the refactored handler error logging (QUAL-02 compliance on touched lines)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] slog.String to slog.Any for error logging (QUAL-02)**
- **Found during:** Task 2 (detail handler refactoring)
- **Issue:** Pre-existing slog.String("error", err.Error()) calls in the handlers being refactored lose error type information
- **Fix:** Updated to slog.Any("error", err) in all touched handler and query method error logging
- **Files modified:** internal/web/detail.go
- **Verification:** go vet passes, tests pass
- **Committed in:** 1bb641a (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Minimal scope addition -- only changed error logging pattern on lines already being modified.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Terminal renderer is now extensible via Register without modifying dispatch code
- Detail handlers are clean and testable with separated query methods
- Ready for Phase 35 (HTTP Caching & Benchmarks) and Phase 36 (UI & Terminal Polish)

## Self-Check: PASSED

- [x] internal/web/termrender/dispatch.go exists
- [x] internal/web/termrender/dispatch_test.go exists
- [x] .planning/phases/34-query-optimization-architecture/34-03-SUMMARY.md exists
- [x] Commit 22bc36f found
- [x] Commit 1bb641a found

---
*Phase: 34-query-optimization-architecture*
*Completed: 2026-03-26*
