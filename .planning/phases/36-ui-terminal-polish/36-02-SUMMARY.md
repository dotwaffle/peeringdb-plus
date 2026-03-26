---
phase: 36-ui-terminal-polish
plan: 02
subsystem: ui
tags: [htmx, search, url-history, error-handling, accessibility]

requires:
  - phase: 31-differentiators-shell-integration
    provides: web UI search form and collapsible detail sections
provides:
  - Bookmarkable search URLs with browser history push
  - htmx error handling with retry on collapsible sections
  - Accessible search input label
affects: []

tech-stack:
  added: []
  patterns:
    - "HX-Push-Url for search URL history entries"
    - "Global htmx:afterRequest handler for error recovery in lazy-loaded sections"

key-files:
  created: []
  modified:
    - internal/web/handler.go
    - internal/web/handler_test.go
    - internal/web/templates/home.templ
    - internal/web/templates/home_templ.go
    - internal/web/templates/layout.templ
    - internal/web/templates/layout_templ.go

key-decisions:
  - "HX-Push-Url over HX-Replace-Url for back/forward navigation support"
  - "DOM API for error UI construction to avoid XSS from string-based markup"

patterns-established:
  - "HX-Push-Url for search: server sends history-creating URL updates on search responses"
  - "Global error handler: single htmx:afterRequest listener handles all collapsible section failures"

requirements-completed: [UI-03, UI-04]

duration: 5min
completed: 2026-03-26
---

# Phase 36 Plan 02: Search URL Bookmarking & htmx Error Handling Summary

**Bookmarkable search URLs via HX-Push-Url with browser history, plus htmx error retry on failed collapsible section loads**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-26T08:32:02Z
- **Completed:** 2026-03-26T08:37:40Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Search results now create browser history entries via HX-Push-Url, enabling back/forward navigation and shareable URLs
- Failed collapsible section loads show "Failed to load. [Retry]" instead of stuck "Loading..." spinner
- Added accessible label for search input (sr-only screen-reader label with for/id association)

## Task Commits

Each task was committed atomically:

1. **Task 1: Bookmarkable search with URL history push** - `45bb126` (feat)
2. **Task 2: htmx error handling with retry on collapsible sections** - `0402ebe` (feat)

## Files Created/Modified
- `internal/web/handler.go` - Changed HX-Replace-Url to HX-Push-Url on search responses
- `internal/web/handler_test.go` - Updated test assertions for new header name
- `internal/web/templates/home.templ` - Added hx-push-url, search input id, accessible label
- `internal/web/templates/home_templ.go` - Regenerated templ output
- `internal/web/templates/layout.templ` - Added global htmx:afterRequest error handler with retry
- `internal/web/templates/layout_templ.go` - Regenerated templ output

## Decisions Made
- HX-Push-Url over HX-Replace-Url: push creates history entries for back/forward, replace only updates URL silently
- DOM API for error UI: createElement/appendChild avoids XSS risk from innerHTML string construction
- htmx.process() call after retry button insertion: ensures htmx recognizes dynamically-added hx-get attributes

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated test assertions from HX-Replace-Url to HX-Push-Url**
- **Found during:** Task 1 (Bookmarkable search with URL history push)
- **Issue:** Existing tests asserted HX-Replace-Url header; changing to HX-Push-Url broke them
- **Fix:** Updated test function names and header assertions to use HX-Push-Url
- **Files modified:** internal/web/handler_test.go
- **Verification:** go test -race passes
- **Committed in:** 45bb126 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug fix)
**Impact on plan:** Necessary test update for correctness. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Search URLs are now bookmarkable and shareable with browser history support
- Collapsible sections have error recovery, ready for production use

## Self-Check: PASSED

All files exist, all commits verified.

---
*Phase: 36-ui-terminal-polish*
*Completed: 2026-03-26*
