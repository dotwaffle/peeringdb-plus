---
phase: 03-production-readiness
plan: 03
subsystem: infra
tags: [otel, litefs, fly-io, docker, health-checks, otelhttp, trace-correlation]

# Dependency graph
requires:
  - phase: 03-production-readiness/03-01
    provides: OTel Setup, NewDualLogger, InitMetrics in internal/otel/
  - phase: 03-production-readiness/03-02
    provides: Health handlers, LiteFS primary detection in internal/health/, internal/litefs/
provides:
  - Full application wiring with OTel, health endpoints, LiteFS detection in main.go
  - Production Dockerfile with LiteFS as entrypoint
  - LiteFS configuration with Consul leasing and proxy passthrough
  - Fly.io deployment configuration with volume, machine sizing, health checks
  - Trace-correlated HTTP logging middleware
  - Write forwarding via Fly-Replay header for replica sync requests
affects: [deployment, monitoring, operations]

# Tech tracking
tech-stack:
  added: [otelhttp v0.67.0]
  patterns: [litefs-process-supervisor, fly-replay-write-forwarding, dual-port-proxy-architecture]

key-files:
  created:
    - Dockerfile.prod
    - litefs.yml
    - fly.toml
  modified:
    - cmd/peeringdb-plus/main.go
    - internal/middleware/logging.go

key-decisions:
  - "OTel HTTP middleware positioned between Recovery and Logging in stack for automatic span creation before log emission"
  - "LiteFS IsPrimaryWithFallback replaces cfg.IsPrimary for production detection with local-dev fallback"
  - "sync_status table init guarded by isPrimary (replicas should not create tables)"
  - "Fly-Replay write forwarding returns 307 on replica /sync instead of relying on LiteFS proxy"
  - "IAD (US East) as default primary region for single-region start"

patterns-established:
  - "Dual-port pattern: LiteFS proxy on :8080, app on :8081 for transparent write forwarding"
  - "Health passthrough: /healthz and /readyz bypass both LiteFS consistency and readiness middleware"
  - "Trace correlation: logging middleware extracts trace_id/span_id from OTel context for structured log correlation"

requirements-completed: [OPS-01, OPS-02, OPS-03, OPS-04, OPS-05, STOR-02]

# Metrics
duration: 5min
completed: 2026-03-22
---

# Phase 3 Plan 3: Integration and Deployment Artifacts Summary

**Full application wiring with OTel observability, health endpoints, LiteFS primary detection, otelhttp middleware, Fly-Replay write forwarding, and Fly.io deployment artifacts (Dockerfile.prod, litefs.yml, fly.toml)**

## Performance

- **Duration:** 5 min
- **Started:** 2026-03-22T17:58:56Z
- **Completed:** 2026-03-22T18:03:58Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- Wired OTel Setup, dual logger, custom metrics, and otelhttp middleware into main.go
- Replaced cfg.IsPrimary with litefs.IsPrimaryWithFallback for production LiteFS detection
- Added /healthz liveness and /readyz readiness endpoints replacing /health
- Added Fly-Replay write forwarding for replica /sync requests per D-26
- Added trace_id and span_id to logging middleware via OTel trace context
- Created Dockerfile.prod with LiteFS as entrypoint/process supervisor
- Created litefs.yml with Consul leasing, FUSE mount, proxy passthrough
- Created fly.toml with shared-cpu-1x, 512MB, IAD region, persistent volume

## Task Commits

Each task was committed atomically:

1. **Task 1: Wire OTel, health, LiteFS into main.go and add trace-correlated logging** - `b7542a1` (feat)
2. **Task 2: Create Fly.io deployment artifacts** - `7be81ef` (feat)

## Files Created/Modified
- `cmd/peeringdb-plus/main.go` - Full application wiring with OTel Setup, dual logger, InitMetrics, health endpoints, LiteFS detection, otelhttp middleware, Fly-Replay write forwarding
- `internal/middleware/logging.go` - Added trace_id and span_id extraction from OTel span context using LogAttrs API
- `Dockerfile.prod` - Multi-stage build with LiteFS binary, fuse3, litefs mount entrypoint
- `litefs.yml` - LiteFS config: Consul leasing, FUSE at /litefs, proxy :8080 -> :8081, health passthrough
- `fly.toml` - Fly.io config: shared-cpu-1x, 512MB, IAD region, volume mount, liveness health check

## Decisions Made
- OTel HTTP middleware positioned between Recovery and Logging (outermost -> innermost: Recovery -> OTel HTTP -> Logging -> CORS -> Readiness -> mux) so spans exist before logging middleware emits
- sync_status table initialization guarded by isPrimary (replicas should not attempt DDL on read-only LiteFS mount)
- Fly-Replay header returns HTTP 307 on replica /sync rather than relying on LiteFS proxy write forwarding, giving the app explicit control over sync routing per D-26
- IAD (US East) selected as default primary_region, matching common infrastructure convention

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Guarded sync_status table init by isPrimary**
- **Found during:** Task 1 (main.go wiring)
- **Issue:** Plan showed sync_status InitStatusTable unconditionally, but on replicas the LiteFS FUSE mount is read-only and DDL would fail
- **Fix:** Wrapped InitStatusTable call with `if isPrimary { ... }` to match schema migration guard
- **Files modified:** cmd/peeringdb-plus/main.go
- **Verification:** Build passes, logic consistent with schema migration guard
- **Committed in:** b7542a1 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 missing critical)
**Impact on plan:** Essential for correctness on read-only LiteFS replicas. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Application is fully wired with OTel observability, health endpoints, and LiteFS detection
- All Fly.io deployment artifacts are created and ready for `fly deploy`
- Phase 3 (Production Readiness) is complete: OTel foundation, health/LiteFS packages, and integration/deployment all done
- Secrets (PDBPLUS_SYNC_TOKEN, OTEL_* env vars) must be set via `fly secrets set` before first deploy

## Self-Check: PASSED

All files verified present, all commits verified in git log.

---
*Phase: 03-production-readiness*
*Completed: 2026-03-22*
