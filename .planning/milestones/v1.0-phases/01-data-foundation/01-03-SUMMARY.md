---
phase: 01-data-foundation
plan: 03
subsystem: api-client
tags: [peeringdb, http-client, rate-limiting, pagination, retry, json, generics]

# Dependency graph
requires:
  - phase: 01-data-foundation/01-01
    provides: Go module with golang.org/x/time dependency
provides:
  - Rate-limited PeeringDB API client at internal/peeringdb
  - Go struct types for all 13 PeeringDB object types with JSON tags
  - Generic Response[T] wrapper for PeeringDB API envelope
  - FetchAll pagination with retry and backoff
  - FetchType[T] generic function for typed deserialization
affects: [01-04, 01-05, 01-06]

# Tech tracking
tech-stack:
  added: [golang.org/x/time/rate v0.15.0]
  patterns: [rate-limited HTTP client with exponential backoff, generic response wrapper, pagination until empty data array, table-driven httptest server tests]

key-files:
  created:
    - internal/peeringdb/types.go
    - internal/peeringdb/types_test.go
    - internal/peeringdb/client.go
    - internal/peeringdb/client_test.go
  modified: []

key-decisions:
  - "FetchType is a package-level generic function (not a method) because Go does not allow type parameters on methods"
  - "retryBaseDelay field exposed on Client struct (unexported) to allow fast test execution without real backoff waits"
  - "Unknown JSON fields silently dropped by encoding/json default behavior, satisfying D-08 without extra logic"

patterns-established:
  - "PeeringDB types: Go-style field names with snake_case JSON tags matching API responses"
  - "Nullable fields: pointer types (*int, *string, *float64, *time.Time, *bool) for optional/nillable API fields"
  - "Rate limiter: rate.NewLimiter(rate.Every(3*time.Second), 1) for 20 req/min"
  - "Pagination: loop limit=250&skip=N&depth=0 until len(data)==0"
  - "Retry: exponential backoff (2s*4^attempt) on 429/5xx, max 3 attempts, no retry on 4xx"
  - "Test pattern: httptest.NewServer with atomic request counters and overridden limiter/delay for speed"

requirements-completed: [DATA-04]

# Metrics
duration: 11min
completed: 2026-03-22
---

# Phase 01 Plan 03: PeeringDB API Client Summary

**Rate-limited HTTP client with pagination and retry for all 13 PeeringDB object types, using golang.org/x/time/rate at 20 req/min with exponential backoff on transient errors**

## Performance

- **Duration:** 11 min
- **Started:** 2026-03-22T15:02:49Z
- **Completed:** 2026-03-22T15:14:16Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- All 13 PeeringDB object types defined as Go structs with complete field coverage matching live API responses
- Generic Response[T] wrapper handles the PeeringDB {meta, data} JSON envelope
- Rate-limited HTTP client enforces 20 req/min via token bucket limiter
- Pagination loops through pages of 250 until empty, accumulating all objects
- Exponential backoff retry (3 attempts, 2s/8s/32s) on 429 and 5xx status codes
- 15 tests covering pagination, retry logic, rate limiting, context cancellation, URL format, error handling, and typed deserialization

## Task Commits

Each task was committed atomically:

1. **Task 1: Create PeeringDB API response types** - `f7c299d` (feat)
2. **Task 2: Create rate-limited HTTP client with pagination, retry, and tests** - `a1a75e3` (feat)

## Files Created/Modified
- `internal/peeringdb/types.go` - Response[T] generic wrapper, SocialMedia type, 13 object type structs (Organization, Network, Facility, InternetExchange, Poc, IxLan, IxPrefix, NetworkIxLan, NetworkFacility, IxFacility, Carrier, CarrierFacility, Campus), type path constants
- `internal/peeringdb/types_test.go` - Deserialization tests for Organization, json.RawMessage, unknown fields, nullable fields, type constants
- `internal/peeringdb/client.go` - Client struct with rate limiter, FetchAll pagination, doWithRetry with exponential backoff, FetchType[T] generic function
- `internal/peeringdb/client_test.go` - 13 tests using httptest servers for pagination, retry (429, 5xx), max retries, no retry on 4xx, context cancellation, depth=0 URL, empty first page, multi-page accumulation, rate limiter timing, unknown fields, typed deserialization, User-Agent header

## Decisions Made
- FetchType[T] implemented as package-level generic function instead of Client method because Go does not allow type parameters on methods
- Client exposes unexported retryBaseDelay field to allow tests to use 1ms delay instead of real 2s backoff
- Unknown fields are silently ignored by Go's encoding/json default behavior (no DisallowUnknownFields), which satisfies D-08 without additional code

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed FetchType from method to package function**
- **Found during:** Task 2 (implementation)
- **Issue:** Go does not allow type parameters on methods; `func (c *Client) FetchType[T any]` is a compile error
- **Fix:** Changed to package-level function `func FetchType[T any](ctx, client, objectType)`
- **Files modified:** internal/peeringdb/client.go, internal/peeringdb/client_test.go
- **Verification:** go build succeeds, test passes
- **Committed in:** a1a75e3 (Task 2 commit)

**2. [Rule 1 - Bug] Fixed infinite loop in unknown fields test**
- **Found during:** Task 2 (testing)
- **Issue:** Test server always returned 1 item; FetchAll looped forever since data was never empty
- **Fix:** Added atomic request counter; return empty response on second request
- **Files modified:** internal/peeringdb/client_test.go
- **Verification:** Test completes in <1s
- **Committed in:** a1a75e3 (Task 2 commit)

**3. [Rule 1 - Bug] Fixed context cancellation test hanging on server.Close**
- **Found during:** Task 2 (testing)
- **Issue:** httptest server blocked in Close() waiting for handler goroutine sleeping 5 seconds
- **Fix:** Replaced blocking handler with pre-cancelled context approach that tests rate limiter early exit
- **Files modified:** internal/peeringdb/client_test.go
- **Verification:** Test completes instantly
- **Committed in:** a1a75e3 (Task 2 commit)

---

**Total deviations:** 3 auto-fixed (3 bugs)
**Impact on plan:** All auto-fixes were necessary for correct compilation and test execution. No scope creep.

## Issues Encountered
- Go's prohibition on type parameters for methods required FetchType to be a standalone generic function; this is a minor API ergonomics difference from the plan's `client.FetchType[Organization]` to `FetchType[Organization](ctx, client, TypeOrg)`

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- PeeringDB API client package is complete and ready for the sync worker (Plan 04/05) to consume
- FetchAll returns []json.RawMessage for flexible processing; FetchType[T] provides typed access
- All 13 object type structs are defined with correct JSON tags for API response deserialization
- Rate limiting, retry, and pagination are tested and production-ready

## Self-Check: PASSED

All 4 key files verified present. Both task commits (f7c299d, a1a75e3) verified in git log.

---
*Phase: 01-data-foundation*
*Completed: 2026-03-22*
