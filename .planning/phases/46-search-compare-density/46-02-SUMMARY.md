---
phase: 46-search-compare-density
plan: 02
subsystem: ui
tags: [templ, tailwind, sortable-tables, comparison, country-flags]

# Dependency graph
requires:
  - phase: 43-dense-tables-with-sorting-and-flags
    provides: CountryFlag component, sort JS/CSS, table patterns, formatSpeed/speedColorClass
provides:
  - Three sortable comparison tables (IXP 9-col, Facility 5-col, Campus 2-col)
  - speedSortValue and facASNSortValue helper functions
  - Entity-type-specific link colors in comparison tables (sky=IX, violet=fac, rose=campus)
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Comparison table pattern: section container + overflow-x-auto + sortable table with Phase 43 classes"
    - "Opacity dimming on tr for non-shared rows in Full View mode"

key-files:
  created: []
  modified:
    - internal/web/templates/compare.templ
    - internal/web/templates/compare_templ.go

key-decisions:
  - "IX links use sky-400 (not emerald) matching entity-type accent convention"
  - "Facility links use violet-400 matching entity-type accent convention"
  - "Campus links use rose-400 matching entity-type accent convention"
  - "Campus shared count as plain number in font-mono, no badge wrapping"

patterns-established:
  - "Comparison table reuse: same Phase 43 header/body/cell/sort patterns across IXP/Facility/Campus"

requirements-completed: [DENS-05]

# Metrics
duration: 4min
completed: 2026-03-26
---

# Phase 46 Plan 02: Compare Table Conversion Summary

**Three ASN comparison sections (IXP, Facility, Campus) converted from div-based layouts to Phase 43-style sortable tables with country flags, speed color tiers, responsive column hiding, and opacity dimming**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-26T23:07:13Z
- **Completed:** 2026-03-26T23:11:30Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- IXP comparison section now renders as a 9-column sortable table (IX Name, Spd A/B, IPv4 A/B, IPv6 A/B, RS A/B) with speed color tiers and responsive hiding
- Facility comparison section now renders as a 5-column sortable table (Name, Country, City, ASN A/B) with CountryFlag component and responsive hiding
- Campus comparison section now renders as a 2-column sortable table (Campus Name, Shared count) with rose-colored links
- Non-shared rows dimmed with opacity-40 on the table row element in Full View mode
- Removed three deprecated templ functions (compareIXPRow, compareIXPresenceDetail, compareFacilityRow)

## Task Commits

Each task was committed atomically:

1. **Task 1: Convert IXP and Facility comparison sections to sortable tables** - `a1be086` (feat)
2. **Task 2: Convert Campus comparison section to sortable table** - `618d78c` (feat)

## Files Created/Modified
- `internal/web/templates/compare.templ` - Rewrote all three comparison sections from div layouts to sortable tables; added speedSortValue and facASNSortValue helpers; added strings import
- `internal/web/templates/compare_templ.go` - Regenerated Go code from templ changes

## Decisions Made
- IX links use sky-400 color (not emerald) to match the entity-type accent color convention established in search results and detail pages
- Facility links use violet-400, campus links use rose-400, per the same convention
- Campus shared count rendered as plain number in font-mono text-neutral-400, no badge wrapping (per UI-SPEC Claude's Discretion)
- IXP table columns: Speed and RS hidden below md, IPv4/IPv6 hidden below lg (per UI-SPEC responsive contract)
- Facility table columns: City and ASN A/B hidden below md (per UI-SPEC responsive contract)
- Campus table: all columns visible at all breakpoints (only 2 columns)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Known Stubs
None - all data is wired from existing CompareData struct fields.

## Next Phase Readiness
- Phase 46 plan 02 complete; both comparison table plans finished
- Combined with 46-01 (search density), this completes the Phase 46 search and compare density overhaul
- v1.11 milestone (Web UI Density & Interactivity) ready for completion once 46-01 is also done

---
*Phase: 46-search-compare-density*
*Completed: 2026-03-26*
