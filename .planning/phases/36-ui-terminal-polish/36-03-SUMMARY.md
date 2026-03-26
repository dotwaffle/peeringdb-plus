---
phase: 36-ui-terminal-polish
plan: 03
subsystem: ui
tags: [terminal, ansi, lipgloss, termrender, error-handling]

# Dependency graph
requires:
  - phase: 31-differentiators-shell-integration
    provides: TruncateName target location (width.go), ShouldShowField, Renderer struct with Width field
provides:
  - TruncateName exported helper for name truncation with ellipsis
  - Name wrapping in all 6 entity renderers for long names in terminal tables
  - Styled terminal error rendering for sync-not-ready in readinessMiddleware
  - ANSI styling test coverage for RenderError
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Name wrapping pattern: full name on dedicated line, truncated in table cell when r.Width > 0"

key-files:
  created: []
  modified:
    - internal/web/termrender/width.go
    - internal/web/termrender/width_test.go
    - internal/web/termrender/network.go
    - internal/web/termrender/ix.go
    - internal/web/termrender/facility.go
    - internal/web/termrender/org.go
    - internal/web/termrender/campus.go
    - internal/web/termrender/carrier.go
    - internal/web/termrender/error_test.go
    - cmd/peeringdb-plus/main.go

key-decisions:
  - "maxNameWidth = max(r.Width/3, 15) balances readability with space for other columns"
  - "Name wrapping only triggers when r.Width > 0 (explicit width set), preserving default behavior"

patterns-established:
  - "Name wrapping: print full name on own line then truncated in table cell for long entity names"

requirements-completed: [TUI-01, TUI-02]

# Metrics
duration: 7min
completed: 2026-03-26
---

# Phase 36 Plan 03: Name Wrapping & Terminal Error Styling Summary

**TruncateName helper with ellipsis truncation in all 6 entity renderers, plus styled terminal sync-not-ready detection in readinessMiddleware**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-26T08:31:43Z
- **Completed:** 2026-03-26T08:39:41Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- TruncateName function with 6 table-driven test cases covering fits, truncation, exact fit, guard, empty, and zero width
- Name wrapping integrated into all 6 entity renderers (network, ix, facility, org, campus, carrier) -- long names get full name on dedicated line before truncated inline version
- readinessMiddleware now detects terminal clients (curl, wget, HTTPie) and renders styled "Service Unavailable" text instead of raw JSON
- Added ANSI styling assertion test for RenderError in rich mode

## Task Commits

Each task was committed atomically:

1. **Task 1: TruncateName helper and name wrapping in entity renderers** - `a5a2f86` (feat) -- TDD: RED/GREEN/integrate
2. **Task 2: Styled terminal error responses and sync-not-ready terminal detection** - `a95d6bf` (feat)

## Files Created/Modified
- `internal/web/termrender/width.go` - Added TruncateName exported function
- `internal/web/termrender/width_test.go` - Added TestTruncateName table-driven tests
- `internal/web/termrender/network.go` - Name wrapping for IX presences and facility lists
- `internal/web/termrender/ix.go` - Name wrapping for participants and facilities
- `internal/web/termrender/facility.go` - Name wrapping for networks, IXPs, and carriers
- `internal/web/termrender/org.go` - Name wrapping for networks, IXPs, facilities, campuses, carriers
- `internal/web/termrender/campus.go` - Name wrapping for facilities
- `internal/web/termrender/carrier.go` - Name wrapping for facilities
- `internal/web/termrender/error_test.go` - Added TestRenderError_RichContainsANSI test
- `cmd/peeringdb-plus/main.go` - Terminal detection in readinessMiddleware for sync-not-ready

## Decisions Made
- maxNameWidth computed as max(r.Width/3, 15) -- gives name column roughly 1/3 of terminal width, minimum 15 chars
- Name wrapping only active when r.Width > 0 (explicit width parameter set) to preserve default unbounded behavior
- Test expectation corrected: TruncateName preserves characters as-is including trailing spaces before appending "..."

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All TUI-01 and TUI-02 requirements complete
- Terminal rendering package fully tested with name wrapping and error styling
- All 6 entity renderers consistently handle long names

## Self-Check: PASSED

All files exist, all commits verified.

---
*Phase: 36-ui-terminal-polish*
*Completed: 2026-03-26*
