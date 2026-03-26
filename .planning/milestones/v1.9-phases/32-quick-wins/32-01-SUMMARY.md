---
phase: 32-quick-wins
plan: 01
subsystem: infra
tags: [middleware, cors, otel, slog, structured-logging]

# Dependency graph
requires:
  - phase: 31-differentiators-shell-integration
    provides: "Current middleware chain and slog patterns"
provides:
  - "CORS-before-OTel middleware ordering eliminates preflight trace noise"
  - "slog.Any error logging preserves error interface through OTel pipeline"
affects: [observability, middleware, logging]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "slog.Any for error attributes instead of slog.String with .Error()"
    - "CORS outermost after Recovery to short-circuit preflights before tracing"

key-files:
  created: []
  modified:
    - cmd/peeringdb-plus/main.go
    - cmd/pdbcompat-check/main.go
    - internal/web/detail.go
    - internal/web/handler.go
    - internal/web/about.go
    - internal/sync/worker.go

key-decisions:
  - "CORS before OTel in middleware chain to avoid tracing OPTIONS preflight requests"
  - "slog.Any preserves error interface for OTel slog bridge and custom handlers"

patterns-established:
  - "slog.Any(\"error\", err) pattern for all error logging across the codebase"
  - "Middleware chain: Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> mux"

requirements-completed: [ARCH-03, QUAL-02]

# Metrics
duration: 3min
completed: 2026-03-26
---

# Phase 32 Plan 01: Quick Wins Summary

**CORS middleware reordered before OTel tracing to eliminate preflight trace noise, plus 90 slog.String error calls replaced with slog.Any to preserve structured error type information**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-26T05:13:33Z
- **Completed:** 2026-03-26T05:17:15Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Reordered middleware chain so CORS short-circuits OPTIONS preflights before OTel creates trace spans or Logging emits log lines
- Replaced all 90 instances of `slog.String("error", err.Error())` with `slog.Any("error", err)` across 6 files
- Preserved all variable names (err, facErr, cursorErr, stepErr, rbErr) in the replacement

## Task Commits

Each task was committed atomically:

1. **Task 1: Reorder middleware chain -- CORS before OTel** - `2062bf2` (fix)
2. **Task 2: Replace slog.String error with slog.Any across all files** - `dd66438` (fix)

## Files Created/Modified
- `cmd/peeringdb-plus/main.go` - Middleware chain reorder + 13 slog replacements
- `cmd/pdbcompat-check/main.go` - 1 slog replacement
- `internal/web/detail.go` - 63 slog replacements (including facErr)
- `internal/web/handler.go` - 2 slog replacements
- `internal/web/about.go` - 1 slog replacement
- `internal/sync/worker.go` - 10 slog replacements (including cursorErr, stepErr, rbErr)

## Decisions Made
None - followed plan as specified.

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Middleware chain is correctly ordered for all current and future middleware additions
- Error logging pattern established for all future slog calls
- Ready for Phase 32 Plan 02 (if any) or Phase 33

---
*Phase: 32-quick-wins*
*Completed: 2026-03-26*
