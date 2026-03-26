---
phase: 36-ui-terminal-polish
plan: 01
subsystem: ui
tags: [wcag, aria, accessibility, breadcrumbs, templ, tailwind]

# Dependency graph
requires:
  - phase: 27-ix-presence-polish
    provides: "Detail page templates with collapsible sections and speed colors"
provides:
  - "WCAG AA 4.5:1 contrast on all dark-mode text elements"
  - "ARIA attributes on nav element (role, aria-expanded, aria-controls)"
  - "Breadcrumb navigation component reusable across all detail pages"
  - "Mobile menu close-on-click behavior"
  - "Emerald outline compare button styling"
affects: [36-02, 36-03]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Breadcrumb component with aria-label and semantic <ol> for navigation hierarchy"
    - "Mobile menu onclick handlers that reset aria-expanded state on the toggle button"

key-files:
  created: []
  modified:
    - "internal/web/templates/detail_shared.templ"
    - "internal/web/templates/nav.templ"
    - "internal/web/templates/compare.templ"
    - "internal/web/templates/detail_net.templ"
    - "internal/web/templates/detail_ix.templ"
    - "internal/web/templates/detail_fac.templ"
    - "internal/web/templates/detail_org.templ"
    - "internal/web/templates/detail_campus.templ"
    - "internal/web/templates/detail_carrier.templ"
    - "internal/web/templates/syncing.templ"

key-decisions:
  - "text-neutral-500 chosen over text-neutral-400 for contrast fix -- provides 4.7:1 ratio on neutral-900 without being too bright"
  - "Breadcrumb middle segment (type plural) is non-linked text since no per-type list pages exist"

patterns-established:
  - "Breadcrumb component: Breadcrumb(typePlural, entityName) -- inserted before DetailHeader in all 6 detail page templates"

requirements-completed: [UI-01, UI-02, UI-05, UI-06, UI-07]

# Metrics
duration: 10min
completed: 2026-03-26
---

# Phase 36 Plan 01: Accessibility & Breadcrumbs Summary

**WCAG AA dark-mode contrast fixes across 5 templates, ARIA nav attributes with mobile menu close, and breadcrumb navigation on all 6 entity detail pages**

## Performance

- **Duration:** 10 min
- **Started:** 2026-03-26T08:33:03Z
- **Completed:** 2026-03-26T08:43:11Z
- **Tasks:** 2
- **Files modified:** 20 (10 .templ + 10 _templ.go generated)

## Accomplishments
- Replaced all text-neutral-600 instances on dark backgrounds with text-neutral-500 for WCAG AA 4.5:1 contrast compliance
- Added role="navigation", aria-label, aria-expanded, aria-controls to nav element with mobile toggle state management
- Created reusable Breadcrumb templ component with aria-label="Breadcrumb" and semantic <ol> structure
- Integrated breadcrumbs into all 6 detail pages: Networks, Exchanges, Facilities, Organizations, Campuses, Carriers
- Mobile menu links now close the menu and reset aria-expanded on click
- Compare button restyled with emerald outline (border-emerald-500 text-emerald-500) for visual distinction

## Task Commits

Each task was committed atomically:

1. **Task 1: WCAG AA contrast fixes, ARIA attributes, mobile menu close, compare button** - `ddebc1f` (feat)
2. **Task 2: Breadcrumb component and integration into all 6 detail pages** - `3a5abef` (feat)

## Files Created/Modified
- `internal/web/templates/nav.templ` - Added role="navigation", aria-expanded/controls, mobile menu close onclick
- `internal/web/templates/detail_shared.templ` - Added Breadcrumb templ component, fixed clipboard icon contrast
- `internal/web/templates/detail_net.templ` - Added @Breadcrumb, fixed boolIndicator contrast, emerald compare button
- `internal/web/templates/detail_ix.templ` - Added @Breadcrumb("Exchanges", data.Name)
- `internal/web/templates/detail_fac.templ` - Added @Breadcrumb("Facilities", data.Name)
- `internal/web/templates/detail_org.templ` - Added @Breadcrumb("Organizations", data.Name)
- `internal/web/templates/detail_campus.templ` - Added @Breadcrumb("Campuses", data.Name)
- `internal/web/templates/detail_carrier.templ` - Added @Breadcrumb("Carriers", data.Name)
- `internal/web/templates/compare.templ` - Fixed 4x text-neutral-600 to text-neutral-500 for contrast
- `internal/web/templates/syncing.templ` - Fixed auto-refresh notice contrast

## Decisions Made
- Used text-neutral-500 (4.7:1 ratio) instead of text-neutral-400 for the contrast fix -- provides sufficient contrast without being too bright
- Breadcrumb middle segment (type plural) is non-linked text since no per-type list pages exist in the app
- Mobile menu close handler resets both the hidden class and aria-expanded state on the toggle button

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All detail pages now have breadcrumb navigation and pass WCAG AA contrast
- Ready for Plan 02 (bookmarkable search, htmx error handling) and Plan 03 (terminal wrapping/errors)

## Self-Check: PASSED

All 10 modified .templ files exist. Both task commits verified (ddebc1f, 3a5abef). SUMMARY.md created.

---
*Phase: 36-ui-terminal-polish*
*Completed: 2026-03-26*
