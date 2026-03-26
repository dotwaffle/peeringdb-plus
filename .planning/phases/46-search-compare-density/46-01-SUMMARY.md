---
phase: 46-search-compare-density
plan: 01
subsystem: ui
tags: [templ, htmx, tailwind, search, country-flags, flag-icons]

# Dependency graph
requires:
  - phase: 14-live-search
    provides: Search structs, query functions, and search results template
provides:
  - Decomposed Country/City/ASN fields on SearchResult and SearchHit structs
  - CountryFlag templ component for flag-icons CSS rendering
  - flag-icons CSS CDN delivery in layout.templ
  - Compact single-line search result layout with metadata badges
affects: [46-02-PLAN, compare-templates, detail-pages-with-flags]

# Tech tracking
tech-stack:
  added: [flag-icons CSS 7.5.0 via CDN]
  patterns: [decomposed metadata fields over formatted subtitle strings, inline metadata badges with responsive hiding]

key-files:
  created: []
  modified:
    - internal/web/templates/searchtypes.go
    - internal/web/search.go
    - internal/web/handler.go
    - internal/web/search_test.go
    - internal/web/handler_test.go
    - internal/web/templates/search_results.templ
    - internal/web/templates/search_results_templ.go
    - internal/web/templates/detail_shared.templ
    - internal/web/templates/detail_shared_templ.go
    - internal/web/templates/layout.templ
    - internal/web/templates/layout_templ.go
    - internal/web/termrender/search.go
    - internal/web/termrender/search_test.go
    - internal/web/termrender/renderer_test.go

key-decisions:
  - "Decompose Subtitle into Country/City/ASN for type-safe metadata rendering"
  - "Networks get ASN only (no org join per D-07) keeping queries simple"
  - "Organizations enriched with Country/City from org entity fields"

patterns-established:
  - "CountryFlag component: reusable templ component in detail_shared.templ rendering flag-icons CSS flags with ISO alpha-2 codes"
  - "Decomposed metadata: prefer strongly-typed fields (Country string, City string, ASN int) over pre-formatted display strings"

requirements-completed: [DENS-04, FLAG-02]

# Metrics
duration: 9min
completed: 2026-03-26
---

# Phase 46 Plan 01: Dense Search Results Summary

**Compact single-line search results with Country/City/ASN metadata badges and flag-icons CSS country flags**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-26T23:06:11Z
- **Completed:** 2026-03-26T23:15:30Z
- **Tasks:** 2
- **Files modified:** 14

## Accomplishments
- Replaced Subtitle string field with decomposed Country, City, ASN fields across SearchResult, SearchHit, and all 6 query functions
- Rewrote search results template from bordered cards to compact single-line rows with inline metadata badges (flag + country, city with responsive hiding, ASN)
- Created reusable CountryFlag templ component and added flag-icons CSS CDN delivery
- Updated all search, handler, and termrender tests for decomposed field assertions
- Added new TestSearchNetworkASNField test confirming networks get ASN without Country/City

## Task Commits

Each task was committed atomically:

1. **Task 1: Enrich search structs, query functions, bridge, and tests** - `f26434d` (feat)
2. **Task 2: Rewrite search results template to compact rows with metadata badges** - `d4b82ab` (feat)

## Files Created/Modified
- `internal/web/templates/searchtypes.go` - SearchResult: Country/City/ASN replace Subtitle
- `internal/web/search.go` - SearchHit: Country/City/ASN replace Subtitle; formatLocation removed
- `internal/web/handler.go` - convertToSearchGroups maps new fields
- `internal/web/search_test.go` - Updated all assertions, added TestSearchNetworkASNField
- `internal/web/handler_test.go` - Updated test fixtures, assertion classes for new template
- `internal/web/templates/search_results.templ` - Compact divider rows with CountryFlag, City, ASN badges
- `internal/web/templates/search_results_templ.go` - Regenerated from templ
- `internal/web/templates/detail_shared.templ` - Added CountryFlag component
- `internal/web/templates/detail_shared_templ.go` - Regenerated from templ
- `internal/web/templates/layout.templ` - Added flag-icons CSS CDN link
- `internal/web/templates/layout_templ.go` - Regenerated from templ
- `internal/web/termrender/search.go` - Updated for decomposed metadata rendering
- `internal/web/termrender/search_test.go` - Updated fixtures and assertions
- `internal/web/termrender/renderer_test.go` - Updated JSON test fixtures

## Decisions Made
- Decompose Subtitle into Country/City/ASN for type-safe metadata rendering instead of pre-formatted strings
- Networks get ASN only with no Country/City (no org join per D-07) to keep search queries simple and fast
- Organizations enriched with Country/City from their own entity fields (schema confirms org has both)
- CountryFlag component uses strings.ToLower for case-insensitive country code handling

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Created missing CountryFlag component and flag-icons CSS**
- **Found during:** Task 1 (pre-implementation analysis)
- **Issue:** Plan references CountryFlag component from detail_shared.templ and flag-icons CSS as existing from Phase 43, but neither exists in codebase (Phase 43 not yet implemented)
- **Fix:** Added CountryFlag templ component to detail_shared.templ and flag-icons CSS CDN link to layout.templ head
- **Files modified:** internal/web/templates/detail_shared.templ, internal/web/templates/layout.templ
- **Verification:** templ generate succeeds, go build passes, CountryFlag renders in search results
- **Committed in:** f26434d (Task 1 commit)

**2. [Rule 3 - Blocking] Updated termrender search and tests for Subtitle removal**
- **Found during:** Task 1 (build failed due to Subtitle references in termrender)
- **Issue:** internal/web/termrender/search.go and test files referenced removed Subtitle field
- **Fix:** Updated termrender to render ASN, Country, City as separate metadata items; updated test fixtures and assertions
- **Files modified:** internal/web/termrender/search.go, internal/web/termrender/search_test.go, internal/web/termrender/renderer_test.go
- **Verification:** go test -race ./internal/web/termrender passes
- **Committed in:** f26434d (Task 1 commit)

**3. [Rule 3 - Blocking] Updated handler_test.go for Subtitle removal and template class changes**
- **Found during:** Task 2 (build verification)
- **Issue:** handler_test.go referenced Subtitle field and old card-border CSS classes
- **Fix:** Updated test fixtures to use ASN field; updated class assertions for new divider/hover classes
- **Files modified:** internal/web/handler_test.go
- **Verification:** go test -race ./internal/web/ passes
- **Committed in:** f26434d (Task 1 commit)

---

**Total deviations:** 3 auto-fixed (all Rule 3 - blocking)
**Impact on plan:** All auto-fixes necessary to make the build compile after struct field changes. No scope creep -- the CountryFlag component and flag-icons CSS are required dependencies for this plan's template output.

## Issues Encountered
None beyond the auto-fixed deviations documented above.

## Known Stubs
None -- all data fields are wired to real entity data from ent queries.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- CountryFlag component and flag-icons CSS are available for 46-02-PLAN comparison table work
- Search result structs with decomposed fields ready for any downstream consumers
- Template regeneration complete, all tests green with -race flag

## Self-Check: PASSED

- All 14 modified files exist on disk
- Commit f26434d (Task 1) verified in git log
- Commit d4b82ab (Task 2) verified in git log
- go test -race ./internal/web/... passes (web + termrender)
- No Subtitle references remain in any .go file
- No resultBadgeClasses references remain in templates

---
*Phase: 46-search-compare-density*
*Completed: 2026-03-26*
