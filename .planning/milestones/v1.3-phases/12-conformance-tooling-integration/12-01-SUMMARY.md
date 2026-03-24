---
phase: 12-conformance-tooling-integration
plan: 01
subsystem: tooling
tags: [peeringdb, api-key, conformance, cli, integration-test]

# Dependency graph
requires:
  - phase: 11-api-key-rate-limiting
    provides: "API key auth pattern (Authorization: Api-Key header), PDBPLUS_PEERINGDB_API_KEY env var"
provides:
  - "pdbcompat-check CLI with --api-key flag and env var fallback"
  - "Live conformance test with conditional auth header and sleep duration"
  - "401/403 auth error handling in conformance CLI"
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "CLI flag with env var fallback for API key (flag takes precedence)"
    - "Conditional sleep duration based on authentication status in live tests"

key-files:
  created:
    - cmd/pdbcompat-check/main_test.go
  modified:
    - cmd/pdbcompat-check/main.go
    - internal/conformance/live_test.go

key-decisions:
  - "No rate limit change in CLI -- keep 3s sleep regardless of auth (simple, CLI is not performance-sensitive)"
  - "Live test reduces sleep from 3s to 1s when authenticated (60 req/min safe for 13 requests)"
  - "resolveAPIKey helper function for testable flag/env precedence logic"

patterns-established:
  - "CLI flag with env var fallback: flag.StringVar then os.Getenv fallback after flag.Parse()"
  - "Auth header injection: if apiKey != '' then set Authorization: Api-Key header"

requirements-completed: [CONFORM-01, CONFORM-02]

# Metrics
duration: 4min
completed: 2026-03-24
---

# Phase 12 Plan 01: Conformance Tooling API Key Integration Summary

**API key auth wired into pdbcompat-check CLI (--api-key flag + env fallback) and live conformance test (conditional 1s/3s sleep, auth header injection)**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-24T01:18:10Z
- **Completed:** 2026-03-24T01:22:51Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- pdbcompat-check CLI accepts --api-key flag with PDBPLUS_PEERINGDB_API_KEY env var fallback
- CLI sends Authorization: Api-Key header on all PeeringDB requests when key is configured
- CLI returns specific error message mentioning "API key may be invalid" on 401/403 responses
- Live conformance test uses 1s inter-request sleep when authenticated, 3s when unauthenticated
- Live conformance test sends auth header when PDBPLUS_PEERINGDB_API_KEY env var is set
- Full unit test suite for CLI auth behavior (header injection, error handling, flag precedence)

## Task Commits

Each task was committed atomically:

1. **Task 1: Add API key support to pdbcompat-check CLI with tests** - `732f753` (test: TDD RED) + `5f2f102` (feat: TDD GREEN)
2. **Task 2: Add API key support to live integration test** - `4d1d39e` (feat)

_Note: Task 1 used TDD flow with separate RED/GREEN commits._

## Files Created/Modified
- `cmd/pdbcompat-check/main.go` - Added apiKey field, --api-key flag, env var fallback, auth header injection, 401/403 error handling
- `cmd/pdbcompat-check/main_test.go` - New file with TestCheckTypeAuthHeader, TestCheckTypeAuthErrors, TestAPIKeyFlagPrecedence, TestAPIKeyEnvVarFallback
- `internal/conformance/live_test.go` - Added API key env var reading, conditional sleep duration, auth header injection

## Decisions Made
- No rate limit change in CLI -- keep the existing 3s inter-request sleep regardless of authentication. The CLI makes 13 sequential requests and is not performance-sensitive.
- Live test reduces sleep from 3s to 1s when authenticated. At 60 req/min (PeeringDB authenticated limit), 1 req/s is safe for 13 requests.
- Created resolveAPIKey helper function to make flag/env precedence logic testable without t.Setenv (which conflicts with t.Parallel).

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed t.Setenv / t.Parallel conflict in TestAPIKeyFlagPrecedence**
- **Found during:** Task 1 (TDD GREEN phase)
- **Issue:** Go testing does not allow t.Setenv in parallel subtests (panics at runtime)
- **Fix:** Extracted flag/env precedence logic into resolveAPIKey helper function; parallel subtests test the helper directly. Added separate non-parallel TestAPIKeyEnvVarFallback test for actual env var reading.
- **Files modified:** cmd/pdbcompat-check/main_test.go
- **Verification:** All tests pass with -race
- **Committed in:** 5f2f102 (Task 1 GREEN commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Test structure adjusted for Go testing constraints. No scope creep.

## Issues Encountered
None beyond the t.Setenv/t.Parallel conflict noted above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- v1.3 milestone feature complete: API key support wired through sync client (Phase 11) and conformance tooling (Phase 12)
- All conformance tools respect PDBPLUS_PEERINGDB_API_KEY env var
- Ready for milestone completion

## Known Stubs
None - all functionality is fully wired.

---
*Phase: 12-conformance-tooling-integration*
*Completed: 2026-03-24*
