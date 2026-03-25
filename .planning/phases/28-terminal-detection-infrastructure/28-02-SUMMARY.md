---
phase: 28-terminal-detection-infrastructure
plan: 02
subsystem: ui
tags: [termrender, terminal, cli, curl, ansi, lipgloss, content-negotiation]

# Dependency graph
requires:
  - phase: 28-01
    provides: "termrender package with Detect(), Renderer, styles, and color constants"
provides:
  - "Terminal-aware renderPage() with detection branch for Rich/Plain/JSON/HTMX/HTML"
  - "PageContent.Data field for passing raw data structs to terminal renderers"
  - "RenderHelp method with wttr.in-inspired help text for terminal clients"
  - "RenderError method for styled terminal 404/500 error pages"
  - "RenderPage method for generic terminal page output"
affects: [28-03, 29-entity-renderers]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Terminal detection integrated into renderPage via switch on RenderMode"
    - "PageContent.Data carries raw structs for terminal/JSON rendering alongside templ.Component"
    - "Vary: HX-Request, User-Agent, Accept on all responses for cache safety"

key-files:
  created:
    - internal/web/termrender/help.go
    - internal/web/termrender/help_test.go
    - internal/web/termrender/error.go
    - internal/web/termrender/error_test.go
  modified:
    - internal/web/render.go
    - internal/web/detail.go
    - internal/web/handler.go
    - internal/web/about.go
    - internal/web/handler_test.go
    - internal/web/termrender/renderer.go

key-decisions:
  - "Vary header expanded to HX-Request, User-Agent, Accept on all branches (effectively disables shared caching, acceptable with no CDN)"
  - "RenderPage generic method as placeholder until Phase 29 entity-specific renderers"
  - "handleHome Data stays nil so terminal clients get help text instead of empty data"

patterns-established:
  - "Terminal rendering integrated at renderPage level, not per-handler dispatch"
  - "Data field on PageContent enables terminal renderers without changing handler signatures"

requirements-completed: [DET-05, NAV-01, NAV-02, NAV-03]

# Metrics
duration: 4min
completed: 2026-03-25
---

# Phase 28 Plan 02: Render Pipeline Integration Summary

**Terminal detection wired into renderPage with Rich/Plain/JSON branching, wttr.in-style help text, and styled error pages**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-25T23:39:01Z
- **Completed:** 2026-03-25T23:43:30Z
- **Tasks:** 2
- **Files modified:** 10

## Accomplishments
- Replaced renderPage() with terminal-aware version using termrender.Detect priority chain
- Extended PageContent with Data field; all 6 entity detail handlers pass their data structs
- Created RenderHelp with endpoint listing, format options, curl examples, and data freshness footer
- Created RenderError for styled terminal 404/500 pages with help hint
- All existing web tests pass with updated Vary header expectations

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend PageContent and wire renderPage() terminal branch** - `273ddf2` (feat)
2. **Task 2: Create help text and error page renderers** - `c4dddfd` (feat)

## Files Created/Modified
- `internal/web/render.go` - Terminal-aware renderPage with Detect/HasNoColor and mode switch
- `internal/web/detail.go` - All 6 entity handlers pass Data through PageContent
- `internal/web/handler.go` - Search and compare handlers pass Data; home Data nil for help
- `internal/web/about.go` - Passes freshness struct as Data
- `internal/web/handler_test.go` - Updated Vary header assertions to include User-Agent, Accept
- `internal/web/termrender/renderer.go` - Added RenderPage generic method
- `internal/web/termrender/help.go` - RenderHelp with wttr.in-inspired terminal help text
- `internal/web/termrender/help_test.go` - Tests for rich/plain mode and zero timestamp
- `internal/web/termrender/error.go` - RenderError for styled terminal error pages
- `internal/web/termrender/error_test.go` - Tests for 404/500 in rich/plain mode

## Decisions Made
- Vary header expanded to `HX-Request, User-Agent, Accept` on all four response branches (HTML, HTMX, terminal, JSON) -- ensures caches never serve wrong content type; acceptable trade-off since no CDN layer exists
- RenderPage is a generic placeholder that shows title and suggests `?format=json` -- entity-specific rich renderers come in Phase 29
- handleHome Data stays nil so terminal clients hitting `/ui/` receive help text rather than empty data output

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated existing Vary header test assertions**
- **Found during:** Task 2 verification (full test suite)
- **Issue:** Three existing tests (TestHomeHandler_HtmxFragment, TestHomeHandler_VaryHeader, TestSearchEndpoint_VaryHeader) expected `Vary: HX-Request` but renderPage now sets `Vary: HX-Request, User-Agent, Accept`
- **Fix:** Updated all three test assertions to expect the new Vary header value
- **Files modified:** internal/web/handler_test.go
- **Verification:** `go test -race ./internal/web/...` passes
- **Committed in:** c4dddfd (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Test assertions needed updating to match intentional Vary header change. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Terminal detection fully wired into HTTP response path
- Help text and error pages ready for terminal clients
- Phase 29 can implement entity-specific rich renderers using the Data field and RenderPage pattern
- Plan 28-03 can add integration tests for the full detection-to-render pipeline

## Self-Check: PASSED

All 10 files verified present. Both commit hashes (273ddf2, c4dddfd) confirmed in git log.

---
*Phase: 28-terminal-detection-infrastructure*
*Completed: 2026-03-25*
