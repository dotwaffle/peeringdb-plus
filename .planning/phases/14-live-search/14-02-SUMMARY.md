---
phase: 14-live-search
plan: 02
subsystem: ui
tags: [htmx, templ, search, tailwind, hx-trigger, hx-sync, hx-replace-url]

# Dependency graph
requires:
  - phase: 14-live-search
    provides: SearchService with TypeResult/SearchHit types, fan-out query logic
  - phase: 13-foundation
    provides: web handler dispatch, templ templates, renderPage with HX-Request detection
provides:
  - Homepage search form with htmx live-as-you-type
  - Search results template with grouped results, colored badges, count badges
  - Search endpoint at /ui/search?q= returning HTML fragments
  - Bookmarked URL support (/ui/?q=term pre-renders results on page load)
  - HX-Replace-Url header for browser URL sync without history pollution
  - ASN redirect script for numeric input + Enter
affects: [15-detail-pages, web-ui, search-ux]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Template display types in templates package to avoid circular imports (web -> templates)"
    - "convertToSearchGroups adapter bridges SearchService types to template types"
    - "HX-Replace-Url response header for URL state sync instead of hx-push-url"
    - "htmx indicator CSS with 150ms transition delay prevents spinner flicker"

key-files:
  created:
    - internal/web/templates/searchtypes.go
    - internal/web/templates/search_results.templ
    - internal/web/templates/search_results_templ.go
  modified:
    - internal/web/templates/home.templ
    - internal/web/templates/home_templ.go
    - internal/web/handler.go
    - internal/web/handler_test.go

key-decisions:
  - "Defined SearchGroup/SearchResult in templates package to avoid circular imports between web and templates"
  - "Used HX-Replace-Url response header instead of hx-replace-url attribute for correct URL updates"
  - "Added htmx indicator CSS with 150ms delay to prevent spinner flicker on fast SQLite queries"

patterns-established:
  - "Template display types: define render-only structs in templates package, convert in handler"
  - "Bookmarked URL pattern: handler checks ?q= on page load and pre-renders results"
  - "Search form htmx wiring: hx-get, hx-trigger input changed delay:300ms, hx-sync this:replace"

requirements-completed: [SRCH-01, SRCH-02, SRCH-03, SRCH-04]

# Metrics
duration: 5min
completed: 2026-03-24
---

# Phase 14 Plan 02: Search UI Summary

**Homepage search form with htmx live-as-you-type, grouped results template with colored type badges, and search endpoint with bookmarked URL support**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-24T05:49:28Z
- **Completed:** 2026-03-24T05:54:00Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Homepage search form with htmx attributes: hx-get, hx-trigger (input changed delay:300ms), hx-sync (this:replace), hx-indicator with 150ms flicker prevention
- Search results template renders grouped results with per-type colored badges (emerald/sky/violet/amber/rose/cyan), count badges showing total matches, and clickable detail links
- Search endpoint at GET /ui/search?q= returns HTML fragments with HX-Replace-Url header for browser URL sync
- Bookmarked URLs (/ui/?q=cloudflare) pre-render search results on page load
- ASN redirect script: numeric input + Enter navigates directly to /ui/asn/{number}
- 8 new integration tests pass with -race flag alongside 27 existing tests (35 total)

## Task Commits

Each task was committed atomically:

1. **Task 1: Create search results template and update homepage template** - `0da51d8` (feat)
2. **Task 2: Wire search endpoint into handler and add integration tests** - `dbc2f50` (feat)

## Files Created/Modified
- `internal/web/templates/searchtypes.go` - SearchGroup and SearchResult types avoiding circular imports (28 lines)
- `internal/web/templates/search_results.templ` - Grouped search results with colored badges, count badges, detail links (91 lines)
- `internal/web/templates/search_results_templ.go` - Generated Go code from search_results.templ
- `internal/web/templates/home.templ` - Updated with search form, htmx attributes, ASN redirect script, indicator CSS (88 lines)
- `internal/web/templates/home_templ.go` - Generated Go code from home.templ
- `internal/web/handler.go` - Added searcher field, handleSearch, handleHome ?q= support, convertToSearchGroups (120 lines)
- `internal/web/handler_test.go` - 8 new search integration tests, updated for Home() signature change (324 lines)

## Decisions Made
- Defined SearchGroup/SearchResult in templates package rather than passing web.TypeResult directly -- avoids circular import (web imports templates for rendering, so templates cannot import web)
- Used HX-Replace-Url response header set server-side rather than hx-replace-url attribute on the input -- ensures correct URL (/ui/?q=value) rather than the htmx request URL (/ui/search?q=value)
- Added htmx indicator CSS with 150ms transition delay -- prevents spinner flicker on fast SQLite responses per research Pitfall 6

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Updated handleNotFound to use new Home() signature**
- **Found during:** Task 2
- **Issue:** handleNotFound called templates.Home() with no arguments but Home() signature changed to Home(query string, groups []SearchGroup)
- **Fix:** Updated to templates.Home("", nil)
- **Files modified:** internal/web/handler.go
- **Verification:** go build ./... passes
- **Committed in:** dbc2f50 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Necessary fix to maintain compilation after Home() signature change. No scope creep.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Known Stubs

None - all search UI components are fully wired to SearchService and ent queries.

## Next Phase Readiness
- Live search is fully functional: type in homepage, see results update as you type
- Search results link to detail pages (/ui/asn/{asn}, /ui/ix/{id}, /ui/fac/{id}, etc.) which will 404 until Phase 15 implements detail page handlers
- All search requirements (SRCH-01 through SRCH-04) are complete

## Self-Check: PASSED

All files exist, all commits verified.

---
*Phase: 14-live-search*
*Completed: 2026-03-24*
