---
phase: 28-terminal-detection-infrastructure
plan: 03
subsystem: web
tags: [terminal, content-negotiation, curl, wget, httpie, ansi, integration-tests]

# Dependency graph
requires:
  - phase: 28-02
    provides: "termrender package with Detect(), Renderer, RenderHelp(), RenderError(), RenderPage(), and renderPage 5-mode switch"
provides:
  - "Root handler (/) returns help text for terminal clients via termrender.Detect"
  - "Error pages (404, 500) render as text for terminal clients"
  - "Home page (/ui/) renders help text for terminal clients"
  - "JSON error responses for ?format=json on error pages"
  - "14 integration tests validating full content negotiation stack"
affects: [29-entity-renderers, terminal-cli]

# Tech tracking
tech-stack:
  added: []
  patterns: ["Title-based dispatch in renderPage for terminal error/help rendering"]

key-files:
  created: []
  modified:
    - cmd/peeringdb-plus/main.go
    - internal/web/render.go
    - internal/web/handler_test.go

key-decisions:
  - "Title-based switch in renderPage dispatches error and help rendering without adding fields to PageContent"
  - "Home page freshness omitted in renderPage (zero time) since db not accessible; root handler has freshness via pdbsync"

patterns-established:
  - "Title-based error dispatch: renderPage uses page.Title to route 'Not Found' and 'Server Error' to RenderError"

requirements-completed: [NAV-04]

# Metrics
duration: 3min
completed: 2026-03-25
---

# Phase 28 Plan 03: Root Handler and Integration Tests Summary

**Root handler terminal detection (NAV-04) with 14 integration tests covering curl/wget/HTTPie UA detection, Accept header negotiation, query param overrides, ANSI color control, and text error pages**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-25T23:46:35Z
- **Completed:** 2026-03-25T23:49:47Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- Root handler (/) returns help text with data freshness for terminal clients, JSON discovery for API clients, and browser redirect -- all three paths working
- Error handlers render text 404/500 for terminal clients via title-based dispatch in renderPage
- Help text renders at /ui/ for terminal clients via Home title detection
- JSON error pages return structured `{"error": "...", "status": N}` for ?format=json on error routes
- 14 integration tests validate the complete content negotiation stack end-to-end

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire root handler and error handlers for terminal detection** - `3ab8f11` (feat)
2. **Task 2: Integration tests for content negotiation stack** - `3fed921` (test)

## Files Created/Modified
- `cmd/peeringdb-plus/main.go` - Root handler wired with termrender.Detect for terminal client help text (NAV-04)
- `internal/web/render.go` - Title-based dispatch for error pages and home in terminal modes; JSON error responses
- `internal/web/handler_test.go` - 14 integration tests for terminal detection, content negotiation, ANSI control, error pages

## Decisions Made
- Title-based switch in renderPage to dispatch error and help rendering avoids adding new fields to PageContent struct while keeping the routing clear
- Home page freshness uses zero time in renderPage (omits the freshness line) since db access is not available there; the root handler in main.go has full freshness from pdbsync.GetLastStatus

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Phase 28 infrastructure complete: terminal detection, rendering, content negotiation, error pages, and integration tests all validated
- Ready for Phase 29: entity-specific terminal renderers for all 6 PeeringDB types (network, IX, facility, org, campus, carrier)
- The title-based dispatch pattern in renderPage provides the hook point for entity-specific rendering in Phase 29+

## Self-Check: PASSED

All files exist, all commits verified.

---
*Phase: 28-terminal-detection-infrastructure*
*Completed: 2026-03-25*
