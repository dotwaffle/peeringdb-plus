---
phase: 34-query-optimization-architecture
plan: 02
subsystem: api
tags: [rfc-9457, problem-details, error-handling, http-errors, middleware]

# Dependency graph
requires:
  - phase: 34-query-optimization-architecture
    provides: "Phase context with ARCH-01 requirement and RFC 9457 decision"
provides:
  - "internal/httperr package with RFC 9457 ProblemDetail struct and WriteProblem helper"
  - "RFC 9457 error responses on pdbcompat, web JSON mode, and REST surfaces"
  - "restErrorMiddleware for entrest error rewriting"
affects: [pdbcompat, web, rest, error-handling]

# Tech tracking
tech-stack:
  added: []
  patterns: ["RFC 9457 Problem Details for HTTP API errors", "Error middleware with response capture and rewrite"]

key-files:
  created:
    - "internal/httperr/problem.go"
    - "internal/httperr/problem_test.go"
  modified:
    - "internal/pdbcompat/response.go"
    - "internal/pdbcompat/handler.go"
    - "internal/pdbcompat/handler_test.go"
    - "internal/web/render.go"
    - "internal/web/handler_test.go"
    - "cmd/peeringdb-plus/main.go"

key-decisions:
  - "WriteProblem wrapper in pdbcompat preserves X-Powered-By header while delegating to httperr"
  - "REST error middleware buffers error bodies and rewrites to RFC 9457 rather than customizing entrest internals"
  - "Web JSON error mode uses NewProblemDetail for struct embedding via RenderJSON (pretty-printed)"

patterns-established:
  - "httperr.WriteProblem for direct HTTP error responses with application/problem+json"
  - "httperr.NewProblemDetail for embedding problem details in other JSON structures"
  - "restErrorWriter pattern: buffer error bodies, pass through success responses"

requirements-completed: [ARCH-01]

# Metrics
duration: 9min
completed: 2026-03-26
---

# Phase 34 Plan 02: RFC 9457 Error Standardization Summary

**Shared httperr package with RFC 9457 Problem Details integrated into pdbcompat, web JSON mode, and REST error middleware; ConnectRPC and GraphQL unchanged**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-26T07:09:20Z
- **Completed:** 2026-03-26T07:18:25Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Created internal/httperr package with ProblemDetail struct, WriteProblem, and NewProblemDetail helpers
- Replaced PeeringDB error envelope in pdbcompat with RFC 9457 format including Instance field on all 8 error call sites
- Updated web JSON error mode (404 and 500) to use RFC 9457 ProblemDetail struct
- Added restErrorMiddleware wrapping entrest handler to rewrite error responses as RFC 9457
- All existing tests updated to expect new format; no changes to ConnectRPC or GraphQL

## Task Commits

Each task was committed atomically:

1. **Task 1: Create httperr package with RFC 9457 ProblemDetail** - `df0a6ec` (feat)
2. **Task 2: Integrate RFC 9457 errors into pdbcompat, web JSON mode, and REST middleware** - `d5f1cb1` (feat)

## Files Created/Modified
- `internal/httperr/problem.go` - RFC 9457 ProblemDetail struct, WriteProblem, NewProblemDetail
- `internal/httperr/problem_test.go` - Table-driven tests for serialization, default title, struct construction
- `internal/pdbcompat/response.go` - Removed errorMeta struct, added WriteProblem wrapper with X-Powered-By
- `internal/pdbcompat/handler.go` - Replaced 8 WriteError calls with WriteProblem including Instance field
- `internal/pdbcompat/handler_test.go` - Updated error tests to expect RFC 9457 format
- `internal/web/render.go` - JSON error mode uses httperr.NewProblemDetail
- `internal/web/handler_test.go` - TestTerminal404JSON updated for RFC 9457
- `cmd/peeringdb-plus/main.go` - Added restErrorMiddleware wrapping entrest handler

## Decisions Made
- Kept pdbcompat.WriteProblem as a thin wrapper that adds X-Powered-By before delegating to httperr.WriteProblem
- REST error middleware uses response capture pattern (buffer error body, write through on success) rather than hooking into entrest error handler configuration
- restErrorWriter implements http.Flusher and Unwrap per CLAUDE.md middleware conventions for gRPC compatibility

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed TestTerminal404JSON assertion for pretty-printed JSON**
- **Found during:** Task 2 (web JSON mode integration)
- **Issue:** Test used `strings.Contains(body, '"type":"about:blank"')` which fails because RenderJSON uses MarshalIndent (spaces around colon)
- **Fix:** Changed assertions to check for field name and value separately
- **Files modified:** internal/web/handler_test.go
- **Verification:** Test passes
- **Committed in:** d5f1cb1 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Minor test assertion format issue. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviation above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- RFC 9457 error format is now available across 3 of 5 HTTP surfaces (pdbcompat, web JSON, REST)
- ConnectRPC and GraphQL intentionally excluded per CONTEXT.md decision
- Ready for plan 03 (renderer interface and detail handler refactor)

---
*Phase: 34-query-optimization-architecture*
*Completed: 2026-03-26*
