---
phase: 02-graphql-api
plan: 03
subsystem: api
tags: [graphql, gqlgen, cors, middleware, slog, http]

# Dependency graph
requires:
  - phase: 02-graphql-api/plan-01
    provides: "GraphQL schema, generated code (NewExecutableSchema, Config), resolver struct"
provides:
  - "CORS middleware with configurable origins via rs/cors"
  - "Structured request logging middleware (method, path, status, duration) via slog"
  - "Panic recovery middleware with stack trace logging"
  - "GraphQL handler factory with complexity limit (500), depth limit (15), and error presenter"
  - "GraphiQL playground handler"
  - "Extended config with CORSOrigins, DrainTimeout, and PDBPLUS_PORT support"
affects: [02-graphql-api/plan-04]

# Tech tracking
tech-stack:
  added: [github.com/rs/cors, github.com/oyyblin/gqlgen-depth-limit-extension]
  patterns: ["func(http.Handler) http.Handler middleware pattern", "CORSInput struct per CS-5 for config args", "Error presenter with machine-readable code extensions per D-16"]

key-files:
  created:
    - internal/middleware/cors.go
    - internal/middleware/logging.go
    - internal/middleware/recovery.go
    - internal/graphql/handler.go
  modified:
    - internal/config/config.go

key-decisions:
  - "Used rs/cors library for CORS -- well-maintained, stdlib-compatible"
  - "Complexity limit 500, depth limit 15 per Claude discretion (D-04)"
  - "Error classifier uses string-matching for ent errors (NOT_FOUND, VALIDATION_ERROR, BAD_REQUEST, INTERNAL_ERROR)"
  - "PDBPLUS_PORT takes precedence over PDBPLUS_LISTEN_ADDR for backward compatibility"
  - "Used strings.Contains for error classification instead of custom substring search"

patterns-established:
  - "Middleware pattern: func(http.Handler) http.Handler for stdlib composability"
  - "CORSInput struct pattern per CS-5 for functions with configuration"
  - "Error presenter pattern: classify errors with machine-readable codes in gqlerror.Extensions"

requirements-completed: [API-07, OPS-06]

# Metrics
duration: 3min
completed: 2026-03-22
---

# Phase 02 Plan 03: HTTP Middleware and GraphQL Handler Summary

**HTTP middleware (CORS, logging, recovery), GraphQL handler factory with complexity/depth limits and D-16 error presenter, and config extensions for port, CORS origins, and drain timeout**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-22T16:54:32Z
- **Completed:** 2026-03-22T16:57:54Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Created HTTP middleware package with CORS (configurable origins via rs/cors), structured request logging (slog with method/path/status/duration), and panic recovery (stack trace logging + 500 response)
- Created GraphQL handler factory with FixedComplexityLimit(500), FixedDepthLimit(15), custom error presenter per D-16, and GraphiQL playground handler
- Extended application config with CORSOrigins, DrainTimeout, and PDBPLUS_PORT environment variable support

## Task Commits

Each task was committed atomically:

1. **Task 1: Create HTTP middleware package (CORS, logging, recovery)** - `4a34cac` (feat)
2. **Task 2: Create GraphQL handler factory with error presenter and extend config** - `6083ac5` (feat)

## Files Created/Modified
- `internal/middleware/cors.go` - CORS middleware using rs/cors with configurable origins via CORSInput struct
- `internal/middleware/logging.go` - Structured request logging capturing method, path, status, duration via slog
- `internal/middleware/recovery.go` - Panic recovery with stack trace logging and JSON 500 response
- `internal/graphql/handler.go` - GraphQL handler factory with complexity/depth limits, error presenter, and playground
- `internal/config/config.go` - Extended with CORSOrigins, DrainTimeout, and PDBPLUS_PORT fields

## Decisions Made
- Used `rs/cors` library for CORS middleware -- well-maintained, stdlib-compatible Handler method
- Set complexity limit to 500 and depth limit to 15 per Claude discretion (CONTEXT.md D-04)
- Error classifier uses string matching on error messages to categorize as NOT_FOUND, VALIDATION_ERROR, BAD_REQUEST, or INTERNAL_ERROR -- sufficient for ent errors
- PDBPLUS_PORT env var takes precedence over PDBPLUS_LISTEN_ADDR for backward compatibility with Phase 1
- Used `strings.Contains` instead of plan's custom `searchString` helper -- cleaner, idiomatic Go (MD-1: prefer stdlib)

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Used strings.Contains instead of custom substring helper**
- **Found during:** Task 2 (GraphQL handler factory)
- **Issue:** Plan suggested a custom `contains`/`searchString` helper to avoid importing strings. This is over-engineering; `strings` is stdlib and the import is negligible.
- **Fix:** Used `strings.Contains` directly per MD-1 (prefer stdlib)
- **Files modified:** internal/graphql/handler.go
- **Verification:** go build passes
- **Committed in:** 6083ac5

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Simplified code by using stdlib. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All middleware and handler infrastructure ready for Plan 04 (main.go wiring)
- CORS, logging, recovery middleware compose with stdlib http.Handler
- GraphQL handler factory accepts resolver from Plan 01/02 and returns http.Handler
- Config has all fields needed for server startup

## Self-Check: PASSED

All 5 created/modified files verified on disk. Both task commits (4a34cac, 6083ac5) verified in git log.

---
*Phase: 02-graphql-api*
*Completed: 2026-03-22*
