---
phase: 15-record-detail
plan: 01
subsystem: ui
tags: [templ, htmx, detail-pages, lazy-loading, ent-queries]

# Dependency graph
requires:
  - phase: 14-live-search
    provides: "Web UI handler dispatch, renderPage, search templates, testutil"
provides:
  - "6 detail page data types (NetworkDetail, IXDetail, FacilityDetail, OrgDetail, CampusDetail, CarrierDetail)"
  - "16 related record row types for fragment templates"
  - "Shared templ components: CollapsibleSection, DetailHeader, StatBadge, DetailField, DetailLink"
  - "formatSpeed helper for Mbps to human-readable conversion"
  - "Dispatch routing for all 6 entity types + fragment endpoint"
  - "Complete Network detail page at /ui/asn/{asn} with lazy-loaded sections"
  - "Fragment endpoints for network IX presences, facilities, contacts"
affects: [15-record-detail/02, 16-asn-compare]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "CollapsibleSection with hx-trigger='toggle once from:closest details' for lazy loading"
    - "Fragment endpoints return bare HTML (no layout wrapper) for htmx partial updates"
    - "Detail data types in templates package to avoid circular imports"
    - "Prefix-based dispatch routing via switch{} with strings.HasPrefix"

key-files:
  created:
    - internal/web/templates/detailtypes.go
    - internal/web/templates/detail_shared.templ
    - internal/web/templates/detail_net.templ
    - internal/web/detail.go
    - internal/web/detail_test.go
  modified:
    - internal/web/handler.go

key-decisions:
  - "Used First() instead of Only() for ASN lookup per Research Pitfall 3 (handles non-singular edge case)"
  - "Fragment endpoints bypass renderPage and write directly to ResponseWriter (per Research Pitfall 4)"
  - "Count stats use pre-computed ix_count/fac_count fields; PocCount uses separate count query"
  - "Empty collapsible sections shown with (0) count badge and static 'None' text, no hx-get attribute"

patterns-established:
  - "CollapsibleSection pattern: HTML details/summary with htmx lazy-load on first expand"
  - "Fragment URL pattern: /ui/fragment/{parent_type}/{parent_id}/{relation}"
  - "Detail handler pattern: parse ID, query ent, map to template type, renderPage"

requirements-completed: [DETL-01, DETL-02, DETL-03, DETL-04, DETL-05]

# Metrics
duration: 6min
completed: 2026-03-24
---

# Phase 15 Plan 01: Detail Infrastructure & Network Page Summary

**Network detail page at /ui/asn/{asn} with lazy-loaded collapsible sections for IX presences, facilities, and contacts via htmx fragment endpoints**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-24T06:20:44Z
- **Completed:** 2026-03-24T06:26:44Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Defined all 6 detail page data types and 16 related record row types for the full Phase 15 scope
- Created shared templ components (CollapsibleSection, DetailHeader, StatBadge, DetailField, DetailLink) reusable by all 6 entity types
- Extended dispatch routing for all 6 entity types plus fragment endpoint with prefix-based matching
- Implemented complete Network detail page with ASN-based lookup, org link, stats badges, detail fields, and 3 collapsible lazy-loaded sections
- Created 3 fragment endpoints returning bare HTML for IX presences, facilities, and contacts with cross-links to related detail pages
- Added 8 tests covering full page render, htmx fragment, 404/invalid ASN, org link, and all 3 fragment types

## Task Commits

Each task was committed atomically:

1. **Task 1: Create detail data types, shared templates, and dispatch routing** - `def9b51` (feat)
2. **Task 2: Implement Network detail page handlers, template, fragment endpoints, and tests** - `6d719e1` (feat)

## Files Created/Modified
- `internal/web/templates/detailtypes.go` - 6 detail page types and 16 row types for all entity templates
- `internal/web/templates/detail_shared.templ` - Shared components: CollapsibleSection, DetailHeader, StatBadge, DetailField, DetailLink, formatSpeed
- `internal/web/templates/detail_net.templ` - Network detail page template with lazy-load sections and fragment list templates
- `internal/web/detail.go` - Network detail handler, fragment dispatcher, 3 network fragment handlers, 5 entity type stubs
- `internal/web/detail_test.go` - 8 tests for network detail page and fragment endpoints
- `internal/web/handler.go` - Dispatch function extended with prefix-based routing for all 6 types + fragments

## Decisions Made
- Used `First()` instead of `Only()` for ASN lookup per Research Pitfall 3 to handle potential non-singular edge cases gracefully
- Fragment endpoints bypass `renderPage` and write directly to ResponseWriter per Research Pitfall 4 -- fragments are always htmx-requested
- Pre-computed count fields (`ix_count`, `fac_count`) used for header stats; `PocCount` requires separate count query since no pre-computed field exists
- Empty collapsible sections display with `(0)` count badge and static "None" text without `hx-get` attribute to avoid unnecessary requests

## Deviations from Plan

None - plan executed exactly as written.

## Known Stubs

The following stubs are intentional and will be resolved by Plan 02:

| File | Line | Stub | Resolution |
|------|------|------|------------|
| `internal/web/detail.go` | 96 | `handleIXDetail` calls `handleNotFound` | Plan 02 |
| `internal/web/detail.go` | 101 | `handleFacilityDetail` calls `handleNotFound` | Plan 02 |
| `internal/web/detail.go` | 106 | `handleOrgDetail` calls `handleNotFound` | Plan 02 |
| `internal/web/detail.go` | 111 | `handleCampusDetail` calls `handleNotFound` | Plan 02 |
| `internal/web/detail.go` | 116 | `handleCarrierDetail` calls `handleNotFound` | Plan 02 |
| `internal/web/detail.go` | 134-135 | Fragment stubs for ix, fac, org, campus, carrier | Plan 02 |

These stubs do NOT prevent Plan 01's goal (Network detail page) from being achieved. The Network detail page is fully functional.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All infrastructure (data types, shared components, dispatch routing, fragment pattern) established for Plan 02
- Plan 02 implements the remaining 5 entity types (IX, Facility, Organization, Campus, Carrier) following the exact same pattern as the Network reference implementation
- Fragment endpoint dispatcher is pre-wired for all 6 parent types

---
*Phase: 15-record-detail*
*Completed: 2026-03-24*
