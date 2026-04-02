---
phase: 47-server-request-hardening
plan: 01
subsystem: infra
tags: [http-server, sqlite, config-validation, security, slowloris, connection-pool]

# Dependency graph
requires:
  - phase: 46-search-compare-density
    provides: existing HTTP server and config infrastructure
provides:
  - HTTP server timeout protection (slowloris, idle reaping)
  - SQLite connection pool configuration
  - Config startup validation (ListenAddr, PeeringDBBaseURL, DrainTimeout)
  - POST body size limits on /graphql and /sync
affects: [47-server-request-hardening]

# Tech tracking
tech-stack:
  added: []
  patterns: [http.MaxBytesReader for per-endpoint body limits, connection pool tuning for SQLite WAL]

key-files:
  created: []
  modified:
    - cmd/peeringdb-plus/main.go
    - internal/config/config.go
    - internal/config/config_test.go
    - internal/database/database.go

key-decisions:
  - "ReadHeaderTimeout(10s) + IdleTimeout(120s) without WriteTimeout or ReadTimeout per CONTEXT.md (streaming RPCs)"
  - "SQLite pool hardcoded (MaxOpenConns 10, MaxIdleConns 5, ConnMaxLifetime 5m) -- infrastructure constants, not user-tunable"
  - "1 MB body limit via http.MaxBytesReader on POST /graphql and POST /sync only"
  - "URL validation requires scheme (url.Parse + scheme check) to reject bare hostnames"

patterns-established:
  - "Per-endpoint body limiting: wrap r.Body with http.MaxBytesReader before delegating to handler"
  - "Config validation in validate() method: fail fast at startup with descriptive error messages"

requirements-completed: [SRVR-01, SRVR-02, SRVR-03, SRVR-04]

# Metrics
duration: 3min
completed: 2026-04-02
---

# Phase 47 Plan 01: Server Request Hardening Summary

**HTTP server timeouts (slowloris/idle), SQLite pool config, config startup validation, and 1 MB POST body limits on /graphql and /sync**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-02T04:19:30Z
- **Completed:** 2026-04-02T04:23:23Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- HTTP server hardened with ReadHeaderTimeout(10s) against slowloris and IdleTimeout(120s) for connection reaping
- SQLite connection pool configured with MaxOpenConns(10), MaxIdleConns(5), ConnMaxLifetime(5m) for WAL mode
- Config.validate() rejects invalid ListenAddr (missing colon), PeeringDBBaseURL (no scheme or invalid URL), and non-positive DrainTimeout at startup
- POST /graphql and POST /sync enforce 1 MB body limit via http.MaxBytesReader; oversized payloads get 413

## Task Commits

Each task was committed atomically:

1. **Task 1: Config validation, SQLite pool, and server timeouts** - `8601d06` (feat) -- TDD: RED at earlier commit, GREEN merged
2. **Task 2: POST body size limits on /graphql and /sync** - `4c79acc` (feat)

## Files Created/Modified
- `cmd/peeringdb-plus/main.go` - Server timeouts, maxRequestBodySize constant, MaxBytesReader on POST endpoints
- `internal/config/config.go` - ListenAddr, PeeringDBBaseURL, DrainTimeout validation in validate()
- `internal/config/config_test.go` - TestLoad_Validate with 10 table-driven cases
- `internal/database/database.go` - SQLite connection pool configuration

## Decisions Made
- ReadHeaderTimeout(10s) + IdleTimeout(120s) set; WriteTimeout and ReadTimeout intentionally omitted to avoid killing streaming RPCs and large GraphQL queries
- SQLite pool values hardcoded as infrastructure constants (not configurable via env vars)
- URL validation checks both url.Parse success and non-empty scheme to reject bare hostnames like "just-a-hostname"
- Body limit applied per-endpoint (not global middleware) since different endpoints may need different limits

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Known Stubs
None

## Next Phase Readiness
- Server request hardening complete
- Ready for response hardening and internal quality improvements (47-02)

---
*Phase: 47-server-request-hardening*
*Completed: 2026-04-02*
