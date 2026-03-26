---
phase: 44-facility-map-map-infrastructure
plan: 01
subsystem: ui
tags: [leaflet, maps, templ, htmx, carto, dark-mode, facility]

# Dependency graph
requires:
  - phase: 43-dense-tables-with-sorting-and-flags
    provides: "Sortable table infrastructure, CountryFlag component, flag-icons CDN"
provides:
  - "MapContainer templ component with MapMarker struct for single-pin maps"
  - "Leaflet 1.9.4 CDN delivery with SRI integrity hashes"
  - "CARTO tile layer with live dark mode swap via window.__pdbMaps"
  - "Latitude/Longitude fields on FacilityDetail struct"
  - "Conditional map rendering on facility detail page"
affects: [45-multi-pin-maps, facility-detail]

# Tech tracking
tech-stack:
  added: [leaflet-1.9.4, carto-tiles]
  patterns: [window.__pdbMaps dark mode tile registry, inline-style popup HTML for Leaflet DOM]

key-files:
  created:
    - internal/web/templates/map.templ
    - internal/web/templates/map_templ.go
  modified:
    - internal/web/templates/layout.templ
    - internal/web/templates/layout_templ.go
    - internal/web/templates/detailtypes.go
    - internal/web/templates/detail_fac.templ
    - internal/web/templates/detail_fac_templ.go
    - internal/web/detail.go

key-decisions:
  - "window.__pdbMaps array pattern for multi-map dark mode tile swap (forward-compatible with Phase 45)"
  - "Inline styles in Leaflet popup HTML because Tailwind classes do not penetrate Leaflet popup DOM"
  - "Treat (0,0) and nil lat/lng as missing data -- no real facility exists at null island"

patterns-established:
  - "MapMarker struct as reusable map pin data carrier for all entity types"
  - "initMap templ script for Leaflet initialization with CARTO tiles and scroll-zoom-on-click"
  - "buildPopupHTML pure Go function for XSS-safe Leaflet popup content"

requirements-completed: [MAP-01, MAP-04, MAP-05]

# Metrics
duration: 3min
completed: 2026-03-26
---

# Phase 44 Plan 01: Facility Map & Map Infrastructure Summary

**Interactive Leaflet map on facility detail pages with CARTO tiles, dark mode tile swap, and reusable MapContainer component**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-26T21:40:23Z
- **Completed:** 2026-03-26T21:43:33Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Leaflet 1.9.4 CDN (CSS + JS) globally available with SRI integrity hashes
- MapContainer templ component renders interactive map with clickable pin and rich popup
- Dark mode toggle live-swaps between CARTO Voyager and Dark Matter tile layers
- Facility detail pages conditionally show map when lat/lng are non-zero

## Task Commits

Each task was committed atomically:

1. **Task 1: Leaflet CDN, data plumbing, and dark mode tile swap** - `0acec39` (feat)
2. **Task 2: MapContainer component and facility page integration** - `2af8a2f` (feat)

## Files Created/Modified
- `internal/web/templates/map.templ` - MapMarker struct, initMap script, MapContainer component, buildPopupHTML helper
- `internal/web/templates/map_templ.go` - Generated Go code from map.templ
- `internal/web/templates/layout.templ` - Leaflet CDN links in head, dark mode tile swap hook
- `internal/web/templates/layout_templ.go` - Generated Go code from layout.templ changes
- `internal/web/templates/detailtypes.go` - Latitude/Longitude float64 fields on FacilityDetail
- `internal/web/templates/detail_fac.templ` - Conditional MapContainer rendering between notes and collapsible sections
- `internal/web/templates/detail_fac_templ.go` - Generated Go code from detail_fac.templ changes
- `internal/web/detail.go` - Populate lat/lng from ent Facility entity, treating nil and (0,0) as missing

## Decisions Made
- Used `window.__pdbMaps` array (not single `window.__pdbMap`) for forward compatibility with Phase 45 multi-pin maps
- Inline styles in Leaflet popup HTML because Tailwind classes do not reliably penetrate Leaflet's popup DOM
- Treat both nil and (0,0) lat/lng as missing data -- no real PeeringDB facility exists at null island
- Popup closed by default on single-facility maps -- user clicks pin to open

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all data sources are wired to live ent queries.

## Next Phase Readiness
- MapContainer component and MapMarker struct ready for Phase 45 multi-pin map reuse
- window.__pdbMaps array supports multiple maps on a single page
- initMap script pattern can be extended for fitBounds with multiple markers

---
*Phase: 44-facility-map-map-infrastructure*
*Completed: 2026-03-26*

## Self-Check: PASSED
