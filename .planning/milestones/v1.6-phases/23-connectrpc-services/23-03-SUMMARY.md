---
phase: 23-connectrpc-services
plan: 03
subsystem: api
tags: [connectrpc, grpc, cors, otelconnect, grpcreflect, grpchealth, observability]

# Dependency graph
requires:
  - phase: 23-connectrpc-services plan 01
    provides: "ConnectRPC ecosystem dependencies and first 3 service handler implementations"
  - phase: 23-connectrpc-services plan 02
    provides: "Remaining 10 service handler implementations in internal/grpcserver"
provides:
  - "All 13 ConnectRPC services registered on HTTP mux with OTel interceptor"
  - "gRPC reflection (v1 + v1alpha) for grpcurl/grpcui service discovery"
  - "gRPC health check reporting SERVING/NOT_SERVING based on sync readiness"
  - "CORS middleware supporting Connect, gRPC, and gRPC-Web protocol headers"
affects: [24-connectrpc-filtering, deployment, observability]

# Tech tracking
tech-stack:
  added: [connectrpc.com/otelconnect v0.9.0, connectrpc.com/grpcreflect v1.3.0, connectrpc.com/grpchealth v1.4.0, connectrpc.com/cors v0.1.0]
  patterns: [registerService helper for ConnectRPC handler mounting, health status goroutine tied to sync readiness]

key-files:
  created: []
  modified:
    - internal/middleware/cors.go
    - internal/middleware/cors_test.go
    - cmd/peeringdb-plus/main.go

key-decisions:
  - "connectcors helpers for CORS -- AllowedHeaders/AllowedMethods/ExposedHeaders merged with existing app headers"
  - "registerService helper function for clean ConnectRPC handler registration (avoids composite literal limitation)"
  - "gRPC health check bypasses readiness middleware to report NOT_SERVING status during sync"
  - "Background goroutine polls HasCompletedSync every 1s to transition health to SERVING"

patterns-established:
  - "ConnectRPC service registration via registerService helper accepting (string, http.Handler) pairs"
  - "gRPC health check lifecycle: default NOT_SERVING, goroutine transitions to SERVING after sync"

requirements-completed: [API-04, OBS-01, OBS-02, OBS-03, OBS-04]

# Metrics
duration: 7min
completed: 2026-03-25
---

# Phase 23 Plan 03: Service Integration Summary

**All 13 ConnectRPC services wired into HTTP mux with otelconnect interceptor, gRPC reflection, health checking, and Connect-aware CORS**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-25T02:57:31Z
- **Completed:** 2026-03-25T03:04:41Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- CORS middleware updated to allow Connect-Protocol-Version, X-Grpc-Web, Grpc-Timeout headers and expose Grpc-Status response headers via connectrpc.com/cors helpers
- All 13 ConnectRPC service handlers registered on HTTP mux with otelconnect interceptor producing rpc.system/rpc.service/rpc.method OTel attributes
- gRPC reflection enabled (v1 + v1alpha) for grpcurl/grpcui service discovery of all 13 services
- gRPC health check reports NOT_SERVING until first sync completes, then transitions to SERVING via background goroutine
- JSON discovery endpoint updated to include connectrpc path prefix

## Task Commits

Each task was committed atomically:

1. **Task 1: Update CORS middleware for Connect/gRPC/gRPC-Web protocols** - `6a97666` (feat)
2. **Task 2: Register all 13 ConnectRPC services, otelconnect, reflection, and health check in main.go** - `c03c854` (feat)

## Files Created/Modified
- `internal/middleware/cors.go` - Merged connectcors.AllowedHeaders/AllowedMethods/ExposedHeaders with existing CORS config
- `internal/middleware/cors_test.go` - Added TestCORSConnectProtocolHeaders verifying Connect protocol headers in preflight and exposed headers
- `cmd/peeringdb-plus/main.go` - ConnectRPC service registration, otelconnect interceptor, gRPC reflection, health check, readiness bypass

## Decisions Made
- Used `connectcors` package helpers (AllowedHeaders, AllowedMethods, ExposedHeaders) rather than manually listing Connect protocol headers -- maintained by ConnectRPC team, covers all three protocols
- registerService helper function pattern for clean handler registration since Go does not allow multi-return function calls in composite literals
- gRPC health check path (`/grpc.health.v1.Health/`) bypasses readiness middleware because the health check manages its own NOT_SERVING/SERVING state based on sync completion
- Background goroutine with 1-second ticker polls HasCompletedSync to transition health status -- simple and deterministic

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed CORS test to use lowercase Access-Control-Request-Headers**
- **Found during:** Task 1 (CORS test)
- **Issue:** rs/cors v1.11.1 uses the Fetch standard convention where Access-Control-Request-Headers values are lowercase per spec. The test sent mixed-case header names ("Connect-Protocol-Version") which rs/cors correctly rejected (the SortedSet stores lowercase-normalized entries).
- **Fix:** Changed test to send lowercase header names in Access-Control-Request-Headers, matching browser behavior per the Fetch standard. Also split preflight (checks Allow-Headers) and POST (checks Expose-Headers) into separate assertions.
- **Files modified:** internal/middleware/cors_test.go
- **Verification:** All CORS tests pass with -race
- **Committed in:** 6a97666 (Task 1 commit)

**2. [Rule 2 - Missing Critical] Added gRPC health check readiness bypass**
- **Found during:** Task 2 (health check registration)
- **Issue:** gRPC health check path would be blocked by readiness middleware during sync, preventing load balancers from querying health status.
- **Fix:** Added `/grpc.health.v1.Health/` prefix to readiness middleware bypass list. The health check itself reports NOT_SERVING during sync, so it provides correct status without readiness gating.
- **Files modified:** cmd/peeringdb-plus/main.go
- **Verification:** Build and all tests pass
- **Committed in:** c03c854 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 missing critical)
**Impact on plan:** Both auto-fixes necessary for correctness. No scope creep.

## Issues Encountered
None beyond the deviations documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 13 ConnectRPC services are live and queryable via Connect, gRPC, or gRPC-Web protocols
- Phase 23 (ConnectRPC Services) is complete -- all 3 plans executed
- Ready for Phase 24 (ConnectRPC Filtering) which adds typed filter fields to List RPCs

## Self-Check: PASSED

All files exist, all commits verified, all content checks pass.

---
*Phase: 23-connectrpc-services*
*Completed: 2026-03-25*
