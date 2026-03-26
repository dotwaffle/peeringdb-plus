---
phase: 31-differentiators-shell-integration
plan: 01
subsystem: api
tags: [terminal, shell, curl, short-format, freshness, pipe-delimited]

# Dependency graph
requires:
  - phase: 30-entity-types-search-formats
    provides: termrender package with Rich/Plain/JSON/WHOIS modes for all entity types
provides:
  - ModeShort detection for ?format=short query parameter
  - RenderShort pipe-delimited one-line output for all 6 entity types
  - FormatFreshness helper with relative age and RFC3339 timestamp
  - Freshness footer injected into Rich/Plain/Short terminal responses
  - PageContent.Freshness field wired through all detail handlers
affects: [31-02, 31-03, future-shell-integration]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Short format renders single pipe-delimited line per entity type"
    - "Freshness footer uses relative age with RFC3339 for machine parsing"
    - "getFreshness nil-db guard for test environments without sync_status table"

key-files:
  created:
    - internal/web/termrender/short.go
    - internal/web/termrender/short_test.go
    - internal/web/termrender/freshness.go
    - internal/web/termrender/freshness_test.go
  modified:
    - internal/web/termrender/detect.go
    - internal/web/termrender/detect_test.go
    - internal/web/render.go
    - internal/web/detail.go
    - internal/web/handler.go

key-decisions:
  - "Short format writes directly to io.Writer without colorprofile (plain text only)"
  - "Freshness footer uses StyleMuted for visual consistency with other terminal output"
  - "getFreshness returns zero time (omitting footer) when db is nil for test safety"

patterns-established:
  - "RenderShort method on *Renderer dispatches via type switch, same pattern as RenderPage"
  - "FormatFreshness returns empty string for zero time, enabling conditional footer injection"

requirements-completed: [DIF-01, DIF-02]

# Metrics
duration: 6min
completed: 2026-03-26
---

# Phase 31 Plan 01: Short Format + Freshness Footer Summary

**Pipe-delimited ?format=short mode for scripting and FormatFreshness sync timestamp footer on all terminal responses**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-26T02:53:21Z
- **Completed:** 2026-03-26T02:59:30Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- ModeShort detected for ?format=short, producing single pipe-delimited lines for all 6 entity types
- FormatFreshness renders relative age + RFC3339 timestamp footer on Rich, Plain, and Short responses
- Freshness data sourced from real sync_status table via GetLastSuccessfulSyncTime
- JSON and WHOIS modes correctly excluded from text freshness footer
- All existing tests pass with no regressions

## Task Commits

Each task was committed atomically:

1. **Task 1: Short format mode + freshness helper + ModeShort detection** - `7a8d4df` (feat, TDD)
2. **Task 2: Wire ModeShort + freshness into renderPage and detail handlers** - `69ecfb7` (feat)

## Files Created/Modified
- `internal/web/termrender/detect.go` - Added ModeShort constant, "short" case in Detect(), String() method
- `internal/web/termrender/detect_test.go` - Added TestDetect_ModeShort and ModeShort.String() tests
- `internal/web/termrender/short.go` - RenderShort with type-switch dispatching to 6 entity formatters
- `internal/web/termrender/short_test.go` - Table-driven tests for all 6 entity types + search/compare/unknown
- `internal/web/termrender/freshness.go` - FormatFreshness with relative age calculation and styled output
- `internal/web/termrender/freshness_test.go` - Tests for minutes/hours/days/just-now/zero/newlines
- `internal/web/render.go` - ModeShort branch in renderPage, Freshness field on PageContent, footer injection
- `internal/web/detail.go` - getFreshness helper, Freshness wired into all 6 detail handler PageContent structs
- `internal/web/handler.go` - Freshness wired into handleHome, handleSearch, handleCompare

## Decisions Made
- Short format writes directly to io.Writer without lipgloss colorprofile because the output is pure text with no ANSI styling needed
- FormatFreshness uses leading + trailing newline for visual separation from entity content
- Relative age thresholds: <1min="just now", <1h="N minutes ago", <24h="N hours ago", else "N days ago"
- Singular forms used for exactly 1 (1 minute ago, 1 hour ago, 1 day ago)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Nil pointer dereference when db is nil in test environment**
- **Found during:** Task 2 (wiring freshness)
- **Issue:** getFreshness called sync.GetLastSuccessfulSyncTime with nil *sql.DB, causing panic in handler tests
- **Fix:** Added nil check for h.db in getFreshness, returning zero time (footer omitted)
- **Files modified:** internal/web/detail.go
- **Verification:** All web handler tests pass without panic
- **Committed in:** 69ecfb7 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential nil-safety guard. No scope creep.

## Issues Encountered
None beyond the auto-fixed nil pointer issue.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Short format and freshness footer ready for shell integration
- Plans 31-02 (help text updates) and 31-03 (format discovery) can proceed
- All terminal render modes now covered: Rich, Plain, JSON, WHOIS, Short

---
*Phase: 31-differentiators-shell-integration*
*Completed: 2026-03-26*
