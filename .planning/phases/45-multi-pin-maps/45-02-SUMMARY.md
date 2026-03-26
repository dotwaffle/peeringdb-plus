---
phase: 45-multi-pin-maps
plan: 02
subsystem: ui
tags: [leaflet, markercluster, circleMarker, templ, maps, multi-pin, comparison, eager-load]

# Dependency graph
requires:
  - phase: 45-multi-pin-maps
    plan: 01
    provides: MultiPinMapContainer, initMultiPinMap, buildMultiPinPopupHTML, filterMappableMarkers, MapMarker Color/Stroke/Extra fields, Latitude/Longitude on row structs
  - phase: 44-facility-map-map-infrastructure
    provides: MapContainer, initMap, Leaflet CDN, CARTO tiles
provides:
  - WithFacility() eager-loading on IX and network facility queries for coordinate data
  - Multi-pin maps on IX detail pages showing all associated facility locations
  - Multi-pin maps on network detail pages showing all facility presence locations
  - Three-color comparison map with emerald shared, sky net-A-only, amber net-B-only pins and legend
  - AllFacilities computed unconditionally for map rendering in both shared and full view modes
  - Coordinate propagation through computeSharedFacilities and computeAllFacilities
  - formatMapLocation helper for city/country popup display
  - Unit tests for buildMultiPinPopupHTML, filterMappableMarkers, formatMapLocation
  - Integration tests for IX and network map rendering
affects: [46-search-compare-density]

# Tech tracking
tech-stack:
  added: []
  patterns: [WithFacility eager-load for coordinate extraction, marker builder functions per page type, three-color comparison pin scheme]

key-files:
  created: []
  modified:
    - internal/web/detail.go
    - internal/web/compare.go
    - internal/web/templates/detail_ix.templ
    - internal/web/templates/detail_ix_templ.go
    - internal/web/templates/detail_net.templ
    - internal/web/templates/detail_net_templ.go
    - internal/web/templates/compare.templ
    - internal/web/templates/compare_templ.go
    - internal/web/templates/map.templ
    - internal/web/templates/map_templ.go
    - internal/web/templates/map_test.go
    - internal/web/detail_test.go

key-decisions:
  - "AllFacilities computed unconditionally (moved outside ViewMode if-block) so comparison map always shows all facilities regardless of shared/full toggle"
  - "Marker builder functions (ixFacilityMarkers, netFacilityMarkers, compareFacilityMarkers) kept in respective templ files rather than centralized, keeping page-specific logic close to templates"
  - "formatMapLocation helper added to map.templ for shared use across all three page types"

patterns-established:
  - "WithFacility() eager-load pattern: query.WithFacility().All() then nf.Edges.Facility nil-check for coordinate extraction"
  - "Three-color comparison map: emerald shared, sky (#38bdf8) net-A-only, amber (#f59e0b) net-B-only with Leaflet legend control"
  - "templ component delegation: main page template calls helper templ (ixFacilityMap) which calls Go func (ixFacilityMarkers) then MultiPinMapContainer"

requirements-completed: [MAP-02, MAP-03]

# Metrics
duration: 8min
completed: 2026-03-26
---

# Phase 45 Plan 02: Multi-Pin Map Page Integration Summary

**WithFacility() eager-loading wired into IX/network/compare handlers with multi-pin maps rendered on all three page types using three-color comparison pins**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-26T22:39:12Z
- **Completed:** 2026-03-26T22:47:40Z
- **Tasks:** 2
- **Files modified:** 12

## Accomplishments
- Enriched IX facility and network facility queries with WithFacility() to eager-load Facility entity coordinates, extracting lat/lng through Edges.Facility nil-safe pointer dereference
- Inserted MultiPinMapContainer into IX detail (map-ix-{id}), network detail (map-net-{id}), and comparison (map-compare) pages at the correct positions per D-07 and D-08
- Implemented three-color comparison pin scheme with emerald shared, sky net-A-only, amber net-B-only plus legend showing AS numbers
- Added comprehensive unit tests (15 cases across 3 functions) and integration tests verifying map container rendering in IX and network detail pages

## Task Commits

Each task was committed atomically:

1. **Task 1: Handler query enrichment and page template integration** - `e9ed252` (feat)
2. **Task 2: Unit tests and integration tests** - `928353f` (test)

## Files Created/Modified
- `internal/web/detail.go` - Added WithFacility() to IX/network facility queries, coordinate extraction from Facility edges
- `internal/web/compare.go` - Added WithFacility() to all 4 facility queries, propagated coordinates through computeSharedFacilities/computeAllFacilities, moved AllFacilities computation outside ViewMode if-block
- `internal/web/templates/detail_ix.templ` - Added ixFacilityMarkers helper and ixFacilityMap templ component between Notes and collapsible sections
- `internal/web/templates/detail_ix_templ.go` - Regenerated from detail_ix.templ
- `internal/web/templates/detail_net.templ` - Added netFacilityMarkers helper and netFacilityMap templ component between Notes and collapsible sections
- `internal/web/templates/detail_net_templ.go` - Regenerated from detail_net.templ
- `internal/web/templates/compare.templ` - Added compareFacilityMarkers and compareFacilityMap with three-color pins and legend between stat badges and IX section
- `internal/web/templates/compare_templ.go` - Regenerated from compare.templ
- `internal/web/templates/map.templ` - Added formatMapLocation helper function
- `internal/web/templates/map_templ.go` - Regenerated from map.templ
- `internal/web/templates/map_test.go` - Added TestBuildMultiPinPopupHTML (5 cases), TestFilterMappableMarkers (5 cases), TestFormatMapLocation (4 cases)
- `internal/web/detail_test.go` - Extended seed data with IxFacility/NetworkFacility links to coords facility, added IX and network map integration tests

## Decisions Made
- Moved `data.AllFacilities = computeAllFacilities(...)` outside the `if input.ViewMode == "full"` block so the comparison map always has all facilities regardless of view mode toggle. AllIXPs remains inside the if-block since it's only needed for the IXP table in full view mode.
- Used `extractCoords` closure inside `computeAllFacilities` to DRY the coordinate extraction pattern for network A and B facility entries.
- Added `formatMapLocation` to map.templ (not detail_net.templ) because it's used across IX, network, and comparison templates. The existing `formatFacLocation` in detail_net.templ is identical but scoped to non-map display.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Extended seed data for map integration tests**
- **Found during:** Task 2 (integration tests)
- **Issue:** Existing seed data had facility 32 with coordinates but no IxFacility or NetworkFacility links to it, making map rendering untestable
- **Fix:** Added IxFacility ID=401 and NetworkFacility ID=301 linking IX 20 and Network 10 to the coordinate-bearing facility 32
- **Files modified:** internal/web/detail_test.go
- **Verification:** Integration tests pass, map containers rendered correctly
- **Committed in:** 928353f (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Seed data extension was necessary for integration tests to verify actual map rendering. No scope creep.

## Known Stubs

None - all maps are fully wired with real coordinate data from eager-loaded Facility edges.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All multi-pin maps are integrated and tested across IX detail, network detail, and comparison pages
- Phase 45 is complete -- both Plan 01 (infrastructure) and Plan 02 (integration) are done
- Ready for Phase 46 (search/compare density improvements)

## Self-Check: PASSED

All 12 modified files exist. Both task commits (e9ed252, 928353f) verified in git log. SUMMARY.md created.

---
*Phase: 45-multi-pin-maps*
*Completed: 2026-03-26*
