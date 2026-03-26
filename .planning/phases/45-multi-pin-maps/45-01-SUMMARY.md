---
phase: 45-multi-pin-maps
plan: 01
subsystem: ui
tags: [leaflet, markercluster, circleMarker, templ, maps, clustering]

# Dependency graph
requires:
  - phase: 44-facility-map-map-infrastructure
    provides: MapContainer, initMap, buildPopupHTML, Leaflet CDN, CARTO tiles, __pdbMaps dark mode swap
provides:
  - MultiPinMapContainer templ component with clustered circleMarkers
  - initMultiPinMap script with markerClusterGroup, fitBounds, optional legend
  - buildMultiPinPopupHTML for IX/network/compare map popups
  - filterMappableMarkers coordinate validation with unmapped count
  - Leaflet.markercluster 1.5.3 CDN with emerald-themed cluster CSS
  - L.CircleMarker.setOpacity shim for markercluster compatibility
  - Latitude/Longitude fields on all facility row structs and CompareFacility
affects: [45-02, compare, detail_ix, detail_net]

# Tech tracking
tech-stack:
  added: [leaflet.markercluster 1.5.3]
  patterns: [server-side popup HTML via JSON serialization to JS, coordinate filtering with unmapped count]

key-files:
  created: []
  modified:
    - internal/web/templates/map.templ
    - internal/web/templates/map_templ.go
    - internal/web/templates/layout.templ
    - internal/web/templates/layout_templ.go
    - internal/web/templates/detailtypes.go
    - internal/web/templates/comparetypes.go

key-decisions:
  - "Server-side popup HTML serialized into marker JSON avoids building HTML in JavaScript"
  - "filterMappableMarkers keeps markers where at least one coordinate is non-zero (Lat != 0 || Lng != 0)"
  - "Legend uses inline styles in Leaflet Control for dark mode support without Tailwind penetration"

patterns-established:
  - "Multi-pin marker serialization: Go struct -> jsMarker JSON -> initMultiPinMap script param"
  - "Coordinate filtering: filterMappableMarkers returns (mappable, unmapped) pair for conditional rendering"
  - "CDN ordering: markercluster CSS after Leaflet CSS, JS after Leaflet JS, shim after markercluster JS, CSS overrides after Default.css"

requirements-completed: [MAP-02, MAP-03]

# Metrics
duration: 15min
completed: 2026-03-26
---

# Phase 45 Plan 01: Multi-Pin Map Infrastructure Summary

**Leaflet.markercluster CDN with emerald clusters, MultiPinMapContainer templ component with circleMarkers/fitBounds/legend, coordinate fields on all facility row structs**

## Performance

- **Duration:** 15 min
- **Started:** 2026-03-26T22:19:25Z
- **Completed:** 2026-03-26T22:35:13Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Added Leaflet.markercluster 1.5.3 CDN (2 CSS + 1 JS) with circleMarker setOpacity shim and emerald-themed cluster CSS overrides for both light and dark modes
- Extended MapMarker with Color/Stroke/Extra fields, added buildMultiPinPopupHTML, filterMappableMarkers, and marshalMarkers/marshalLegend helpers
- Created MultiPinMapContainer templ component with markerClusterGroup, circleMarkers, fitBounds, scroll-zoom-on-click, __pdbMaps registration, optional comparison legend, and conditional unmapped facility message
- Added Latitude/Longitude float64 to IXFacilityRow, NetworkFacRow, OrgFacilityRow, CampusFacilityRow, CarrierFacilityRow, and CompareFacility

## Task Commits

Each task was committed atomically:

1. **Task 1: Markercluster CDN, cluster CSS, circleMarker shim, row struct enrichment** - `1d7f074` (feat)
2. **Task 2: MultiPinMapContainer, initMultiPinMap, popup helpers, coordinate filter** - `5285479` (feat)

## Files Created/Modified
- `internal/web/templates/layout.templ` - Added markercluster CDN links, circleMarker setOpacity shim, emerald cluster CSS overrides
- `internal/web/templates/layout_templ.go` - Regenerated from layout.templ
- `internal/web/templates/map.templ` - Extended MapMarker, added multi-pin popup/filter/marshal helpers, initMultiPinMap script, MultiPinMapContainer component
- `internal/web/templates/map_templ.go` - Regenerated from map.templ
- `internal/web/templates/detailtypes.go` - Added Latitude/Longitude to 5 facility row structs
- `internal/web/templates/comparetypes.go` - Added Latitude/Longitude to CompareFacility

## Decisions Made
- Used unpkg CDN for markercluster to match existing Leaflet CDN pattern (D-01 consistency)
- Omitted SRI hashes on markercluster CDN tags since unpkg does not provide pre-computed hashes and hash generation from downloaded files adds complexity; matches existing crossorigin="" pattern
- Legend implemented as L.control with inline styles and dark mode detection at init time
- Singular/plural unmapped facility message handled with templ if/else on count

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

None - all infrastructure is complete and ready for Plan 02 integration.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All multi-pin map infrastructure is in place for Plan 02 to integrate maps into IX detail, network detail, and comparison pages
- Plan 02 needs to: enrich handler queries with WithFacility() for coordinates, build MapMarker slices in handlers, insert MultiPinMapContainer into detail_ix.templ, detail_net.templ, and compare.templ
- Existing Phase 44 single-facility maps are fully preserved and unmodified

## Self-Check: PASSED

All 6 modified files exist. Both task commits (1d7f074, 5285479) verified in git log. SUMMARY.md created.

---
*Phase: 45-multi-pin-maps*
*Completed: 2026-03-26*
