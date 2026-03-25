---
phase: 28-terminal-detection-infrastructure
plan: 01
subsystem: api
tags: [lipgloss, colorprofile, ansi, terminal, content-negotiation, user-agent]

# Dependency graph
requires: []
provides:
  - "termrender package with Detect() priority chain for terminal client detection"
  - "Renderer type with colorprofile-based ANSI output control"
  - "Style constants mapping Tailwind color tiers to ANSI 256-color codes"
  - "RenderMode enum (HTML, HTMX, Rich, Plain, JSON)"
affects: [28-02, 28-03, 29-network-detail-reference-implementation, 30-entity-types-search-formats]

# Tech tracking
tech-stack:
  added: [charm.land/lipgloss/v2@v2.0.2, github.com/charmbracelet/colorprofile@v0.4.2]
  patterns: [colorprofile.Writer forced profile for HTTP non-TTY output, DetectInput struct for CS-5 compliance]

key-files:
  created:
    - internal/web/termrender/detect.go
    - internal/web/termrender/detect_test.go
    - internal/web/termrender/renderer.go
    - internal/web/termrender/renderer_test.go
    - internal/web/termrender/styles.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "Used colorprofile.NoTTY (not ASCII) for plain/noColor modes to strip ALL ANSI including bold/underline"
  - "lipgloss v2 imported via vanity domain charm.land/lipgloss/v2 per upstream convention"

patterns-established:
  - "Detection priority chain: query params > Accept header > User-Agent > HX-Request > HTML default"
  - "colorprofile.Writer with forced Profile for HTTP responses (non-TTY writers)"
  - "Separate HasNoColor() modifier function from mode detection (noColor is orthogonal to mode)"

requirements-completed: [DET-01, DET-02, DET-03, DET-04, RND-01, RND-18]

# Metrics
duration: 4min
completed: 2026-03-25
---

# Phase 28 Plan 01: Terminal Detection Infrastructure Summary

**Terminal client detection priority chain with lipgloss v2 ANSI rendering engine and Tailwind-to-ANSI256 color tier mapping**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-25T23:30:32Z
- **Completed:** 2026-03-25T23:34:46Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- Detect() implements full priority chain (query params > Accept header > User-Agent > HX-Request) with 6 terminal UA prefixes
- Renderer with colorprofile.Writer forces ANSI256 for Rich mode, NoTTY for Plain/noColor to control ANSI output on non-TTY HTTP writers
- 5 port speed color tiers, 8 general-purpose colors, 3 peering policy colors, 7 predefined lipgloss styles
- 43 table-driven subtests across 12 test functions, all passing with -race

## Task Commits

Each task was committed atomically:

1. **Task 1: Create detection logic with table-driven tests** - `36c6ee3` (feat) - TDD: RED then GREEN
2. **Task 2: Create renderer engine and style definitions** - `8ca25c6` (feat)

## Files Created/Modified
- `internal/web/termrender/detect.go` - Terminal client detection with RenderMode enum and priority chain
- `internal/web/termrender/detect_test.go` - 33 table-driven detection tests with t.Parallel()
- `internal/web/termrender/renderer.go` - Renderer type with colorprofile ANSI control and RenderJSON helper
- `internal/web/termrender/renderer_test.go` - 10 renderer tests covering Rich/Plain/NoColor/JSON/Border
- `internal/web/termrender/styles.go` - lipgloss style constants and Tailwind-to-ANSI color tier mappings
- `go.mod` - Added charm.land/lipgloss/v2 v2.0.2 and transitive dependencies
- `go.sum` - Updated checksums

## Decisions Made
- Used `colorprofile.NoTTY` instead of `colorprofile.ASCII` for Plain/noColor modes because ASCII only strips color codes while NoTTY strips ALL ANSI codes including bold and underline attributes
- lipgloss v2 imported via vanity domain `charm.land/lipgloss/v2` (not github.com path) per upstream convention

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] colorprofile.ASCII does not strip bold/underline ANSI codes**
- **Found during:** Task 2 (renderer engine)
- **Issue:** Plan specified `colorprofile.ASCII` for Plain/noColor modes, but ASCII profile only strips color codes. Bold (`\x1b[1m`) and underline codes pass through, breaking the "no ANSI codes" contract.
- **Fix:** Changed to `colorprofile.NoTTY` which strips ALL ANSI escape sequences including style attributes.
- **Files modified:** internal/web/termrender/renderer.go
- **Verification:** TestRendererWrite_PlainMode and TestRendererWrite_NoColor both confirm no ANSI escapes in output
- **Committed in:** 8ca25c6 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential correctness fix. NoTTY is the correct profile for stripping all terminal formatting.

## Issues Encountered
None beyond the colorprofile constant selection documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- termrender package fully tested and ready for import by renderPage() in Plan 02
- Detect(), NewRenderer(), and style constants are the integration points for Plan 02
- RenderJSON() ready for ?format=json responses

## Self-Check: PASSED

All 5 created files verified on disk. Both task commits (36c6ee3, 8ca25c6) found in git history.

---
*Phase: 28-terminal-detection-infrastructure*
*Completed: 2026-03-25*
