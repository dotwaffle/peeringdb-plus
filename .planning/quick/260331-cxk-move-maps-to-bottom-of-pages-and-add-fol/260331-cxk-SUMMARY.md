---
phase: quick
plan: 260331-cxk
subsystem: ui
tags: [templ, tailwind, leaflet, htmx, collapsible, map]

requires: []
provides:
  - "Maps below collapsible sections on fac/ix/net/compare pages"
  - "Chevron expand/collapse indicators on CollapsibleSection components"
affects: []

tech-stack:
  added: []
  patterns:
    - "Tailwind group/group-open for CSS-only disclosure chevron rotation"

key-files:
  created: []
  modified:
    - internal/web/templates/detail_shared.templ
    - internal/web/templates/detail_fac.templ
    - internal/web/templates/detail_ix.templ
    - internal/web/templates/detail_net.templ
    - internal/web/templates/compare.templ

key-decisions:
  - "list-none on summary hides default browser disclosure triangle cross-browser"
  - "Chevron uses group-open:rotate-90 for pure CSS rotation without JS"

patterns-established:
  - "Collapsible sections: chevron SVG + list-none summary + group/group-open pattern"

requirements-completed: []

duration: 2min
completed: 2026-03-31
---

# Quick Task 260331-cxk: Move Maps Below Collapsibles and Add Chevron Indicators Summary

**Collapsible sections now show chevron expand indicators and maps render below data-dense sections on all detail/compare pages**

## Performance

- **Duration:** 2 min
- **Started:** 2026-03-31T03:40:55Z
- **Completed:** 2026-03-31T03:42:53Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- Added right-pointing chevron SVG to CollapsibleSection and CollapsibleSectionWithBandwidth that rotates 90 degrees on expand
- Moved maps below collapsible sections on facility, IX, network detail pages, and the compare page
- Hidden browser default disclosure triangle via list-none class

## Task Commits

Each task was committed atomically:

1. **Task 1: Add chevron indicators to collapsible sections** - `40e6a5e` (feat)
2. **Task 2: Move maps below collapsible sections on all detail pages** - `8745004` (feat)

## Files Created/Modified
- `internal/web/templates/detail_shared.templ` - Added chevron SVG, group class, list-none to both collapsible components
- `internal/web/templates/detail_shared_templ.go` - Regenerated
- `internal/web/templates/detail_fac.templ` - Map moved after Networks/IXPs/Carriers collapsibles
- `internal/web/templates/detail_fac_templ.go` - Regenerated
- `internal/web/templates/detail_ix.templ` - Map moved after Participants/Facilities/Prefixes collapsibles
- `internal/web/templates/detail_ix_templ.go` - Regenerated
- `internal/web/templates/detail_net.templ` - Map moved after IX Presences/Facilities/Contacts collapsibles
- `internal/web/templates/detail_net_templ.go` - Regenerated
- `internal/web/templates/compare.templ` - Map moved after IXPs/Facilities/Campuses tables
- `internal/web/templates/compare_templ.go` - Regenerated

## Decisions Made
- Used `list-none` on summary elements to hide browser default disclosure triangle (most reliable cross-browser approach)
- Chevron uses pure CSS rotation via Tailwind `group-open:rotate-90` -- no JavaScript needed

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None.

## Self-Check: PASSED

All 10 files verified present. Both commit hashes (40e6a5e, 8745004) found in git log.

---
*Quick task: 260331-cxk*
*Completed: 2026-03-31*
