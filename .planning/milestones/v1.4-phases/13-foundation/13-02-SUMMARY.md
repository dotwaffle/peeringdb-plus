---
phase: 13-foundation
plan: 02
subsystem: ui
tags: [web-ui, content-negotiation, templ, ci, middleware]

# Dependency graph
requires:
  - phase: 13-foundation-01
    provides: internal/web package with Handler, NewHandler, Register, templates, static assets
  - phase: 01-data-foundation
    provides: ent client for data access
provides:
  - Web UI wired into HTTP server at /ui/ and /static/ prefixes
  - Content negotiation on GET / (browsers redirect to /ui/, API clients get JSON)
  - HTML syncing page for browser requests before first sync completes
  - Static asset bypass in readiness middleware
  - templ drift detection in CI pipeline
affects: [14-live-search, 15-record-detail, 16-asn-comparison, 17-polish-accessibility]

# Tech tracking
tech-stack:
  added: []
  patterns: [content negotiation via Accept header, HTML syncing page in readiness middleware, templ CI drift detection alongside ent drift detection]

key-files:
  created: []
  modified:
    - cmd/peeringdb-plus/main.go
    - .github/workflows/ci.yml

key-decisions:
  - "Content negotiation on GET / uses Accept header: text/html triggers redirect, otherwise JSON discovery"
  - "JSON discovery response extended with ui field pointing to /ui/"
  - "Static assets bypass readiness middleware so syncing page CSS/JS loads correctly"
  - "CI templ drift check placed after ent drift check in same lint job"

patterns-established:
  - "Content negotiation: Accept header check for dual HTML/JSON responses"
  - "Readiness middleware: dual rendering with HTML syncing page for browsers, JSON 503 for API clients"

requirements-completed: [DSGN-01, DSGN-02, DSGN-03]

# Metrics
duration: 3min
completed: 2026-03-24
---

# Phase 13 Plan 02: Web Integration Summary

**Content negotiation on GET / with browser redirect to /ui/, HTML syncing page in readiness middleware, and templ drift detection in CI**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-24T05:11:57Z
- **Completed:** 2026-03-24T05:15:54Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Wired internal/web handler into HTTP server with /ui/ and /static/ route registration
- Implemented content negotiation on GET /: browsers see redirect to /ui/, API clients get JSON discovery with new "ui":"/ui/" field
- Updated readiness middleware to serve styled HTML syncing page for browser requests and bypass /static/ for asset loading
- Added templ CLI installation and drift detection to CI lint job

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire web handler into main.go and update middleware** - `13885b3` (feat)
2. **Task 2: Add templ drift detection to CI** - `cb858c5` (chore)

## Files Created/Modified
- `cmd/peeringdb-plus/main.go` - Web handler registration, content negotiation on GET /, readiness middleware HTML syncing page
- `.github/workflows/ci.yml` - templ CLI install and drift detection step, renamed ent drift check for clarity

## Decisions Made
- Content negotiation uses `strings.Contains(accept, "text/html")` check on the Accept header -- browsers get 302 redirect to /ui/, everything else gets JSON discovery
- JSON discovery response extended with `"ui":"/ui/"` field alongside existing endpoint URLs
- Static assets (`/static/` prefix) bypass readiness middleware so the syncing page can load htmx and render correctly
- Renamed existing CI drift check from "Check for generated code drift" to "Check for ent generated code drift" to distinguish from templ drift check

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Web UI fully wired into the application server and CI pipeline
- All existing tests pass with -race flag (including web handler tests from Plan 01)
- Ready for Phase 14 (live search) to build on the /ui/ route structure

## Self-Check: PASSED

All files exist, all commits found, all key content verified.

---
*Phase: 13-foundation*
*Completed: 2026-03-24*
