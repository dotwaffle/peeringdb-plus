---
phase: 30-entity-types-search-formats
plan: 01
subsystem: ui
tags: [termrender, lipgloss, ansi, terminal, ix, facility, eager-loading]

# Dependency graph
requires:
  - phase: 29-network-detail-reference-implementation
    provides: "RenderNetworkDetail reference pattern, styles, helpers (writeKV, CrossRef, FormatSpeed, etc.)"
provides:
  - "RenderIXDetail with participant table, facility list, prefix list"
  - "RenderFacilityDetail with address, network list, IX list, carrier list"
  - "Child row slice fields on all 5 detail structs (IX, Facility, Org, Campus, Carrier)"
  - "Eager-loading in all 5 entity detail handlers"
  - "RenderPage type-switch dispatching all entity types"
  - "handleHome search data passthrough for terminal clients"
  - "ModeWHOIS detection and routing"
  - "Stubs for Org, Campus, Carrier, Search, Compare renderers (Plan 02/03)"
affects: [30-02, 30-03, 30-04]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Rich layout renderer pattern: title + writeKV header + child entity sections with cross-refs"
    - "formatLocation/formatAddress helpers for consistent location display"
    - "formatProtocols for boolean-to-string protocol conversion"

key-files:
  created:
    - "internal/web/termrender/ix.go"
    - "internal/web/termrender/facility.go"
    - "internal/web/termrender/ix_test.go"
    - "internal/web/termrender/facility_test.go"
  modified:
    - "internal/web/templates/detailtypes.go"
    - "internal/web/detail.go"
    - "internal/web/handler.go"
    - "internal/web/render.go"
    - "internal/web/termrender/detect.go"
    - "internal/web/termrender/renderer.go"

key-decisions:
  - "Eager-load unconditionally in handlers (not gated by render mode) per research anti-pattern note"
  - "Compute aggregate BW and build participant rows in single query pass to avoid re-querying"
  - "formatLocation as termrender-local helper (not shared with web package) for package independence"

patterns-established:
  - "Rich layout: title with location, right-aligned KV header, entity sections with counts and cross-refs"
  - "Stub pattern: renderStub method for not-yet-implemented type-switch cases, replaced in subsequent plans"

requirements-completed: [RND-03, RND-04, RND-10]

# Metrics
duration: 8min
completed: 2026-03-26
---

# Phase 30 Plan 01: Data Plumbing & IX/Facility Renderers Summary

**IX and Facility rich terminal renderers with full data plumbing across all 5 entity types, handler eager-loading, type-switch dispatch, and search/WHOIS mode detection**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-26T02:03:37Z
- **Completed:** 2026-03-26T02:11:47Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- IX renderer shows participants with speed colors, RS badges, IPv4/IPv6, and cross-references to /ui/asn/{asn}
- Facility renderer shows address, networks, IXPs, and carriers with cross-references
- All 5 detail structs extended with child row slice fields (json tags, doc comments)
- All 5 entity handlers eager-load child rows into detail structs
- RenderPage type-switch dispatches 8 data types (6 entities + SearchGroup + CompareData)
- handleHome now passes search groups as Data for terminal clients hitting /ui/?q=...
- ModeWHOIS added to detection chain (?format=whois)

## Task Commits

Each task was committed atomically:

1. **Task 1: Data plumbing -- struct fields, handler eager-loading, type-switch, handleHome fix** - `25b9afd` (feat)
2. **Task 2: IX and Facility terminal renderers with tests** - `6602eba` (test, RED), `a88da98` (feat, GREEN)

## Files Created/Modified
- `internal/web/termrender/ix.go` - RenderIXDetail with participant table, facility list, prefix list
- `internal/web/termrender/ix_test.go` - 9 test functions covering header, protocols, participants, RS badge, facilities, prefixes, empty, plain mode, aggregate BW
- `internal/web/termrender/facility.go` - RenderFacilityDetail with address, network/IX/carrier lists
- `internal/web/termrender/facility_test.go` - 7 test functions covering header, networks, IXPs, carriers, empty, plain mode, omitted fields
- `internal/web/templates/detailtypes.go` - Added child row slice fields to all 5 detail structs
- `internal/web/detail.go` - Added eager-loading to all 5 entity handlers
- `internal/web/handler.go` - handleHome passes search groups as Data, title set to "Search"
- `internal/web/render.go` - Added ModeWHOIS case routing to plain text renderer
- `internal/web/termrender/detect.go` - Added ModeWHOIS constant and ?format=whois detection
- `internal/web/termrender/renderer.go` - Extended type-switch, added stubs for Org/Campus/Carrier/Search/Compare

## Decisions Made
- Eager-load unconditionally in handlers rather than gating by render mode, per research noting the anti-pattern of conditional loading adds complexity for minimal savings
- Combined aggregate BW computation with participant row building in a single query pass in handleIXDetail to avoid re-querying the same data
- Added formatLocation as a termrender-local helper rather than importing from web package, keeping termrender self-contained

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added temporary IX/Facility stubs to renderer.go for Task 1 build**
- **Found during:** Task 1 (type-switch extension)
- **Issue:** RenderPage type-switch referenced RenderIXDetail and RenderFacilityDetail which did not exist yet (created in Task 2)
- **Fix:** Added temporary stub methods in renderer.go, removed them in Task 2 when real implementations were created
- **Files modified:** internal/web/termrender/renderer.go
- **Verification:** Build passes after both tasks
- **Committed in:** 25b9afd (Task 1), a88da98 (Task 2)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Minimal -- temporary stubs resolved naturally by Task 2. No scope creep.

## Known Stubs

The following stubs are intentional and will be replaced by subsequent plans:

| File | Stub | Resolved By |
|------|------|-------------|
| renderer.go | RenderOrgDetail -> renderStub | Plan 30-02 |
| renderer.go | RenderCampusDetail -> renderStub | Plan 30-02 |
| renderer.go | RenderCarrierDetail -> renderStub | Plan 30-02 |
| renderer.go | RenderSearch -> renderStub | Plan 30-03 |
| renderer.go | RenderCompare -> renderStub | Plan 30-03 |

These stubs do not prevent this plan's goal (IX/Facility renderers + data plumbing) from being achieved.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Data plumbing complete for all 5 entity types -- Plan 02 can implement Org/Campus/Carrier minimal renderers with zero file conflicts
- Type-switch is wired -- adding new renderers only requires replacing the stub method
- formatLocation and formatProtocols helpers available for reuse
- Search and Compare stubs ready for Plan 03

---
*Phase: 30-entity-types-search-formats*
*Completed: 2026-03-26*
