---
phase: 31-differentiators-shell-integration
plan: 02
subsystem: api
tags: [terminal, shell, curl, section-filtering, width-adaptation, progressive-disclosure]

# Dependency graph
requires:
  - phase: 31-differentiators-shell-integration
    plan: 01
    provides: Renderer struct, RenderPage, all 6 entity detail renderers, RenderShort, FormatFreshness
provides:
  - ParseSections function with alias normalization for ?section= query parameter
  - ShouldShowSection guard for conditional section rendering
  - ShouldShowField with column priority thresholds for ?w= width adaptation
  - All 6 entity renderers gated with section and width guards
  - renderPage parsing ?section= and ?w= across all terminal modes
affects: [31-03, future-shell-integration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Section filtering via nil-means-show-all map pattern with alias normalization"
    - "Width adaptation via threshold map lookup -- entire columns dropped, never truncated"
    - "Per-request state on Renderer struct (Sections, Width) avoids signature explosion"

key-files:
  created:
    - internal/web/termrender/sections.go
    - internal/web/termrender/sections_test.go
    - internal/web/termrender/width.go
    - internal/web/termrender/width_test.go
  modified:
    - internal/web/termrender/renderer.go
    - internal/web/termrender/network.go
    - internal/web/termrender/ix.go
    - internal/web/termrender/facility.go
    - internal/web/termrender/org.go
    - internal/web/termrender/campus.go
    - internal/web/termrender/carrier.go
    - internal/web/render.go

key-decisions:
  - "Section aliases support both short (ix) and long (exchanges) forms for user convenience"
  - "Width thresholds use progressive column dropping -- values never truncated with ellipsis"
  - "Key-value header sections unaffected by width parameter (only list sections adapt)"
  - "Sections and Width as exported struct fields on Renderer to avoid method signature explosion"

patterns-established:
  - "ShouldShowSection nil-map guard pattern: nil means show all, map lookup means selective"
  - "ShouldShowField context-field-width triple lookup: unknown context or field = always show"
  - "Column priority thresholds defined per entity-section context (net-ix, ix-participants, etc.)"

requirements-completed: [DIF-03, DIF-04]

# Metrics
duration: 5min
completed: 2026-03-26
---

# Phase 31 Plan 02: Section Filtering + Width Adaptation Summary

**Section filtering (?section=ix,fac) and width-adaptive column dropping (?w=N) for terminal detail views**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-26T03:02:32Z
- **Completed:** 2026-03-26T03:07:30Z
- **Tasks:** 2
- **Files modified:** 12

## Accomplishments
- ParseSections with alias normalization: "exchanges" maps to "ix", "participants" maps to "net", etc.
- Progressive column dropping at narrow widths: IPv6 drops at <100, crossrefs at <90, IPs at <80, RS at <70, speed at <50
- All 6 entity renderers (network, IX, facility, org, campus, carrier) gated with ShouldShowSection
- Network and IX renderers additionally gated with ShouldShowField for per-column width adaptation
- renderPage parses ?section= and ?w= for Rich, Plain, Short, and WHOIS modes consistently
- 26 tests covering section parsing, alias resolution, width thresholds, and edge cases

## Task Commits

Each task was committed atomically:

1. **Task 1: Section filtering and width adaptation core + tests (TDD)** - `56fc656` (feat)
2. **Task 2: Wire section filtering + width into renderers and renderPage** - `5f1db3f` (feat)

## Files Created/Modified
- `internal/web/termrender/sections.go` - ParseSections with alias map, ShouldShowSection guard
- `internal/web/termrender/sections_test.go` - 12 tests: empty, single, multiple, aliases, whitespace, unknown, case-insensitive
- `internal/web/termrender/width.go` - columnThresholds map and ShouldShowField helper
- `internal/web/termrender/width_test.go` - 14 tests: wide, narrow, default, no-width, unknown context/field, exact threshold, ix-participants
- `internal/web/termrender/renderer.go` - Added Sections and Width exported fields
- `internal/web/termrender/network.go` - Section gates on IX/Fac blocks, width gates on crossref/RS/speed/IPv4/IPv6
- `internal/web/termrender/ix.go` - Section gates on Participants/Facilities/Prefixes, width gates on per-row fields
- `internal/web/termrender/facility.go` - Section gates on Networks/IXPs/Carriers, width gates on crossrefs
- `internal/web/termrender/org.go` - Section gates on all 5 child sections
- `internal/web/termrender/campus.go` - Section gate on Facilities
- `internal/web/termrender/carrier.go` - Section gate on Facilities
- `internal/web/render.go` - Added strconv import, ?section= and ?w= parsing in Short/Rich/Plain/WHOIS branches

## Decisions Made
- Section aliases support both short (ix) and long (exchanges) forms -- users type what feels natural
- Width thresholds use progressive column dropping: values are never truncated with ellipsis, only entire columns are removed
- Key-value header sections are unaffected by width parameter -- only list/table sections adapt
- Sections and Width are exported struct fields on Renderer set by caller, avoiding method signature changes across all renderers

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Section filtering and width adaptation ready for shell integration
- Plan 31-03 (help text and format discovery) can proceed
- All terminal render modes now support ?section= and ?w= parameters

---
*Phase: 31-differentiators-shell-integration*
*Completed: 2026-03-26*
