---
phase: 34-query-optimization-architecture
plan: 01
subsystem: api, database
tags: [search, sqlite, indexes, reflect, field-projection, ent]

# Dependency graph
requires:
  - phase: 14-search
    provides: SearchService with parallel type queries
  - phase: 06-pdbcompat
    provides: Field projection via itemToMap
provides:
  - Limit+1 search pattern (6 queries instead of 12)
  - HasMore bool replacing TotalCount int across search types
  - Database indexes on updated/created for all 13 schemas
  - Reflect-based field projection in pdbcompat (no JSON roundtrip)
affects: [35-http-caching-benchmarks, 36-ui-terminal-polish]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "limit+1 hasMore pattern for paginated search without Count queries"
    - "reflect-based struct-to-map with sync.Map field accessor caching"

key-files:
  created:
    - internal/pdbcompat/search_test.go
  modified:
    - internal/web/search.go
    - internal/web/handler.go
    - internal/web/templates/searchtypes.go
    - internal/web/templates/search_results.templ
    - internal/web/termrender/search.go
    - internal/pdbcompat/search.go
    - ent/schema/*.go (all 13 schemas)
    - ent/migrate/schema.go

key-decisions:
  - "displayLimit const (10) with limit+1 fetch eliminates all Count queries"
  - "HasMore bool replaces TotalCount int across TypeResult, SearchGroup, and all renderers"
  - "reflect-based itemToMap with sync.Map caching replaces json.Marshal/Unmarshal roundtrip"
  - "hasMoreSuffix helper in searchtypes.go shared by templ and termrender"

patterns-established:
  - "limit+1 hasMore pattern: fetch N+1, truncate to N, use overflow as signal"
  - "sync.Map field accessor caching for reflect-based struct-to-map conversion"

requirements-completed: [PERF-01, PERF-03, PERF-05]

# Metrics
duration: 10min
completed: 2026-03-26
---

# Phase 34 Plan 01: Query Optimization Summary

**Halved search queries (12 to 6) via limit+1 pattern, added updated/created indexes to all 13 schemas, replaced JSON roundtrip field projection with reflect-based accessor map**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-26T07:08:24Z
- **Completed:** 2026-03-26T07:18:30Z
- **Tasks:** 2
- **Files modified:** 27

## Accomplishments
- Search queries reduced from 12 to 6 per request (limit+1 replaces item+count pattern)
- All 13 ent schemas now have updated and created indexes for filtered query performance
- Field projection in pdbcompat uses reflect-based accessor map cached via sync.Map, no JSON overhead
- All tests updated and passing across web, termrender, and pdbcompat packages

## Task Commits

Each task was committed atomically:

1. **Task 1: Search limit+1 optimization + schema indexes + ent codegen** - `aa81dff` (feat)
2. **Task 2: Reflect-based field projection replacing JSON roundtrip** - `00e4d98` (feat)

## Files Created/Modified
- `internal/web/search.go` - Limit+1 query pattern, displayLimit const, HasMore bool
- `internal/web/handler.go` - convertToSearchGroups uses HasMore
- `internal/web/templates/searchtypes.go` - SearchGroup.HasMore bool, hasMoreSuffix helper
- `internal/web/templates/search_results.templ` - Count badge shows "N+" when HasMore
- `internal/web/termrender/search.go` - Group header shows "N+ results" when HasMore
- `internal/pdbcompat/search.go` - reflect-based itemToMap, getFieldMap with sync.Map cache
- `internal/pdbcompat/search_test.go` - Tests for reflect conversion, projection, caching
- `ent/schema/*.go` (13 files) - Added updated and created indexes
- `ent/migrate/schema.go` - Regenerated migration with new indexes
- Test files updated: search_test.go, handler_test.go, search_test.go, renderer_test.go, short_test.go

## Decisions Made
- displayLimit = 10 as named constant rather than magic number throughout
- HasMore bool replaces TotalCount int -- simpler, no wasted query
- hasMoreSuffix helper placed in searchtypes.go (shared by templ and termrender)
- fieldAccessor struct uses field index (not reflect.StructField) for minimal allocation
- sync.Map for field map caching -- safe for concurrent reads, lazy initialization

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Search optimization complete, ready for HTTP caching (Phase 35)
- Database indexes in place for filtered query benchmarking
- Reflect-based field projection ready for performance measurement

## Self-Check: PASSED

All key files verified present. Both task commits (aa81dff, 00e4d98) confirmed in git log.

---
*Phase: 34-query-optimization-architecture*
*Completed: 2026-03-26*
