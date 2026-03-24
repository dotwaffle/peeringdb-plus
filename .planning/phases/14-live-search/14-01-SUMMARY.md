---
phase: 14-live-search
plan: 01
subsystem: search
tags: [errgroup, sqlite, search, ent, sql-containsfold]

# Dependency graph
requires:
  - phase: 13-foundation
    provides: web UI handler structure, ent client integration
provides:
  - SearchService with parallel fan-out across 6 entity types
  - TypeResult and SearchHit types for grouped search results
  - buildSearchPredicate for case-insensitive LIKE queries
  - formatLocation helper for city/country display
affects: [14-02-handler, search-endpoint, web-ui]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "errgroup fan-out for parallel queries across entity types"
    - "Pre-allocated slice with distinct indices for lock-free concurrent writes"
    - "Local buildSearchPredicate to avoid cross-package coupling"

key-files:
  created:
    - internal/web/search.go
    - internal/web/search_test.go
  modified: []

key-decisions:
  - "Duplicated buildSearchPredicate locally instead of importing from pdbcompat to avoid cross-package coupling"
  - "Pre-allocated results slice with distinct indices eliminates need for mutex in concurrent fan-out"

patterns-established:
  - "SearchService pattern: service struct with ent.Client, parallel query via errgroup"
  - "Type configuration table: searchTypes slice defines all searchable entities with metadata"

requirements-completed: [SRCH-01, SRCH-02, SRCH-03, SRCH-04]

# Metrics
duration: 5min
completed: 2026-03-24
---

# Phase 14 Plan 01: Search Backend Summary

**SearchService with errgroup fan-out querying 6 PeeringDB entity types in parallel, returning grouped results with count badges**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-24T05:41:00Z
- **Completed:** 2026-03-24T05:46:13Z
- **Tasks:** 1 (TDD: RED + GREEN)
- **Files modified:** 2

## Accomplishments
- SearchService queries Networks, IXPs, Facilities, Organizations, Campuses, Carriers in parallel via errgroup
- TypeResult includes TypeName, TypeSlug, AccentColor, up to 10 Results, and exact TotalCount for badge display
- Detail URLs follow CONTEXT.md patterns: /ui/asn/{asn} for networks, /ui/{type}/{id} for others
- Subtitles show ASN for networks, city/country for IXPs/facilities/campuses, empty for orgs/carriers
- 2-character minimum query length prevents query storms
- 17 tests pass with -race flag covering all entity types, edge cases, and result ordering

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): Failing tests for SearchService** - `36a21a3` (test)
2. **Task 1 (GREEN): Implement SearchService** - `95727b9` (feat)

## Files Created/Modified
- `internal/web/search.go` - SearchService with fan-out query logic, TypeResult/SearchHit types, buildSearchPredicate, formatLocation helper (310 lines)
- `internal/web/search_test.go` - 17 table-driven tests with seeded test data covering all 6 entity types (578 lines)

## Decisions Made
- Duplicated buildSearchPredicate locally (10 lines) rather than importing from pdbcompat -- avoids cross-package coupling per research Open Question 3 option b
- Pre-allocated results slice with distinct indices per goroutine -- no mutex needed, race-free by design
- Networks use ASN in URL (/ui/asn/{asn}) not ID, per CONTEXT.md "Numeric input -> direct redirect" decision

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None - all functionality is fully wired to ent queries.

## Next Phase Readiness
- SearchService is ready for Plan 02 (search handler endpoint)
- TypeResult and SearchHit types are exported for use by the handler and templ templates
- The handler will call SearchService.Search and render results as HTML fragments for htmx

## Self-Check: PASSED

All files exist, all commits verified.

---
*Phase: 14-live-search*
*Completed: 2026-03-24*
