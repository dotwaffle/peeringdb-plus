---
phase: 43-dense-tables-with-sorting-and-flags
plan: 03
subsystem: ui
tags: [templ, tables, country-flags, sort, dense-tables, org, campus, carrier]

# Dependency graph
requires:
  - phase: 43-01
    provides: CountryFlag component, sort JS/CSS infrastructure, flag-icons CDN
provides:
  - Org detail page list templates converted to tables (5 templates)
  - Campus detail page facilities list converted to sortable table
  - Carrier detail page facilities list converted to single-column table
  - Fragment test assertions for table HTML structure on all 7 converted templates
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: [single-column name-only table for simple lists without thead or sortable class, 2-column sortable table for networks (Name+ASN), 3-column sortable facility table with CountryFlag]

key-files:
  created: []
  modified:
    - internal/web/templates/detail_org.templ
    - internal/web/templates/detail_org_templ.go
    - internal/web/templates/detail_campus.templ
    - internal/web/templates/detail_campus_templ.go
    - internal/web/templates/detail_carrier.templ
    - internal/web/templates/detail_carrier_templ.go
    - internal/web/detail_test.go

key-decisions:
  - "OrgNetworksList is 2-column (Name, ASN) without Country -- all networks on an org page share the org's country, making it redundant"
  - "Single-column tables (OrgIXPs, OrgCampuses, OrgCarriers, CarrierFacilities) use entity-type accent colors on hover (sky, rose, cyan, violet)"
  - "Test assertions scoped to Plan 03 converted templates only since Plan 02 runs in parallel and hasn't merged into this worktree"

patterns-established:
  - "Single-column name-only table: no thead, no sortable class, entity-type hover color"
  - "2-column network table: Name (alpha sort, default asc) + ASN (numeric sort, font-mono)"
  - "3-column facility table: Name + City (hidden md:table-cell) + Country (alpha sort, default asc, CountryFlag)"

requirements-completed: [DENS-01, DENS-02, DENS-03, SORT-01, SORT-02, SORT-03, FLAG-01]

# Metrics
duration: 5min
completed: 2026-03-26
---

# Phase 43 Plan 03: Org/Campus/Carrier Table Conversion Summary

**All 7 remaining detail page list templates (5 org, 1 campus, 1 carrier) converted from card layouts to dense tables with sortable headers and country flags**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-26T20:50:31Z
- **Completed:** 2026-03-26T20:55:59Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- OrgNetworksList converted to 2-column sortable table (Name linked to /ui/asn/{ASN}, ASN numeric)
- OrgFacilitiesList and CampusFacilitiesList converted to 3-column sortable tables with CountryFlag component and responsive City column hiding
- OrgIXPsList, OrgCampusesList, OrgCarriersList, and CarrierFacilitiesList converted to single-column name-only tables with entity-type accent hover colors
- Fragment tests updated with table structure assertions: data-sortable presence/absence, fi fi- flag class, data-sort-value attributes, and old card pattern absence

## Task Commits

Each task was committed atomically:

1. **Task 1: Convert Org, Campus, and Carrier detail templates to tables** - `da03996` (feat)
2. **Task 2: Update fragment tests to assert table HTML structure** - `a553748` (test)

## Files Created/Modified
- `internal/web/templates/detail_org.templ` - 5 list templates rewritten as tables (2 sortable, 3 single-column)
- `internal/web/templates/detail_org_templ.go` - Regenerated from detail_org.templ
- `internal/web/templates/detail_campus.templ` - CampusFacilitiesList rewritten as 3-column sortable table
- `internal/web/templates/detail_campus_templ.go` - Regenerated from detail_campus.templ
- `internal/web/templates/detail_carrier.templ` - CarrierFacilitiesList rewritten as single-column table
- `internal/web/templates/detail_carrier_templ.go` - Regenerated from detail_carrier.templ
- `internal/web/detail_test.go` - Table structure assertions added to TestFragments_AllTypes and TestFragments_OrgCampusesAndCarriers

## Decisions Made
- OrgNetworksList uses 2-column layout (Name, ASN) without Country per research recommendation -- all networks on an org page share the org's country
- Single-column tables use entity-type accent colors for hover: sky-400 for IX links, rose-400 for campus links, cyan-400 for carrier links, violet-400 for facility links
- Test assertions scoped to templates converted in this plan only (not ix/net/fac which are handled by Plan 02 running in parallel)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Scoped test assertions to Plan 03 templates only**
- **Found during:** Task 2 (test updates)
- **Issue:** Plan specified adding table assertions for all 16 fragments, but Plan 02 (ix/net/fac conversion) runs in parallel and hasn't merged into this worktree -- those templates still use old card layout
- **Fix:** Added table assertions only for the 7 templates converted by this plan (org networks, org ixps, org facilities, org campuses, org carriers, campus facilities, carrier facilities)
- **Files modified:** internal/web/detail_test.go
- **Verification:** All tests pass with -race
- **Committed in:** a553748 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Necessary adjustment for parallel execution. Plan 02 will add its own test assertions when it runs.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All 7 remaining list templates converted to table layout
- Combined with Plan 01 (infrastructure) and Plan 02 (ix/net/fac), all 16 child-entity list templates across all 6 detail pages will be table-based
- Full test suite green with -race

## Self-Check: PASSED

All 7 modified files verified present. Both task commits (da03996, a553748) verified in git log. SUMMARY.md exists.

---
*Phase: 43-dense-tables-with-sorting-and-flags*
*Completed: 2026-03-26*
