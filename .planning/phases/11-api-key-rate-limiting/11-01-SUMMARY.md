---
phase: 11-api-key-rate-limiting
plan: 01
subsystem: api
tags: [peeringdb, api-key, rate-limiting, http-client, functional-options]

# Dependency graph
requires:
  - phase: 01-data-foundation
    provides: "peeringdb.NewClient, Config.Load, doWithRetry flow"
provides:
  - "ClientOption type and WithAPIKey functional option"
  - "Authorization: Api-Key header injection on PeeringDB HTTP requests"
  - "Rate limiter upgrade from 20 req/min to 60 req/min when authenticated"
  - "401/403 auth error handling with WARN log and no retry"
  - "Config.PeeringDBAPIKey loaded from PDBPLUS_PEERINGDB_API_KEY"
affects: [11-02, cmd/peeringdb-plus/main.go]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Functional options pattern (ClientOption) on NewClient constructor"
    - "Auth error early-exit before retryable check in doWithRetry"

key-files:
  created: []
  modified:
    - "internal/peeringdb/client.go"
    - "internal/peeringdb/client_test.go"
    - "internal/config/config.go"
    - "internal/config/config_test.go"

key-decisions:
  - "ClientOption func(*Client) type with variadic opts on NewClient -- backward-compatible, consistent with existing FetchOption pattern"
  - "401/403 check placed between body-discard and isRetryable check -- avoids modifying isRetryable, clear separation of auth vs transient errors"

patterns-established:
  - "ClientOption pattern: new options added as ClientOption funcs without breaking callers"
  - "Auth error handling: 401/403 logged at WARN and returned immediately, never retried"

requirements-completed: [KEY-01, KEY-02, RATE-01, RATE-02, VALIDATE-01]

# Metrics
duration: 4min
completed: 2026-03-24
---

# Phase 11 Plan 01: API Key Config & Client Summary

**PeeringDB API key support via WithAPIKey functional option with 60 req/min authenticated rate limit, Authorization header injection, and 401/403 immediate-fail handling**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-24T00:49:17Z
- **Completed:** 2026-03-24T00:53:47Z
- **Tasks:** 1
- **Files modified:** 4

## Accomplishments
- Config loads PeeringDBAPIKey from PDBPLUS_PEERINGDB_API_KEY env var with empty string default
- Client accepts WithAPIKey functional option, injects Authorization: Api-Key header on all requests when set
- Rate limiter upgrades from 20 req/min (1 per 3s) to 60 req/min (1 per 1s) when API key is configured
- 401/403 responses logged at WARN level and returned immediately without retry
- API key value never logged (SEC-2 compliance)
- 7 new tests covering all behaviors, all passing with -race

## Task Commits

Each task was committed atomically:

1. **Task 1 (RED): Failing tests** - `1210e00` (test)
2. **Task 1 (GREEN): Implementation** - `f4ea311` (feat)

_TDD task: RED/GREEN commits. No refactor needed._

## Files Created/Modified
- `internal/config/config.go` - Added PeeringDBAPIKey field to Config struct and PDBPLUS_PEERINGDB_API_KEY loading in Load()
- `internal/config/config_test.go` - Added TestLoad_PeeringDBAPIKey table-driven test
- `internal/peeringdb/client.go` - Added apiKey field, ClientOption type, WithAPIKey function, variadic NewClient, header injection, 401/403 handling
- `internal/peeringdb/client_test.go` - Added 7 tests: header injection, no-header, auth/unauth rate limits, 401/403 not retried, backward compat

## Decisions Made
- Used ClientOption func(*Client) type with variadic opts on NewClient -- backward-compatible with all existing callers (Go variadic parameter addition is non-breaking)
- Placed 401/403 check between body-discard and isRetryable check in doWithRetry -- avoids modifying the isRetryable function, clear separation of auth errors from transient errors
- API key never logged per SEC-2; only status code and URL in WARN log

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Plan 11-02 can now wire WithAPIKey into main.go where NewClient is called
- NewClient signature is backward-compatible; no callers need updating unless they want to pass an API key
- All existing tests pass unchanged (verified with full `go test ./... -count=1 -race`)

---
*Phase: 11-api-key-rate-limiting*
*Completed: 2026-03-24*
