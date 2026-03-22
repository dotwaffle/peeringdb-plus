---
phase: 03-production-readiness
plan: 01
subsystem: infra
tags: [opentelemetry, otel, slog, metrics, tracing, logging, autoexport]

# Dependency graph
requires:
  - phase: 01-data-foundation
    provides: "Basic OTel TracerProvider, slog logging, config framework"
  - phase: 02-graphql-api
    provides: "HTTP server with middleware stack, GraphQL handler"
provides:
  - "Centralized OTel setup with TracerProvider + MeterProvider + LoggerProvider via autoexport"
  - "Dual slog handler (stdout JSON + OTel log pipeline)"
  - "Custom sync metrics: pdbplus.sync.duration histogram, pdbplus.sync.operations counter"
  - "Extended config: OTelSampleRate (0.0-1.0) and SyncStaleThreshold"
  - "W3C TraceContext propagation for distributed tracing"
  - "Go runtime metrics collection (goroutines, memory, GC)"
  - "Fly.io resource attributes in OTel resource"
affects: [03-production-readiness]

# Tech tracking
tech-stack:
  added:
    - "go.opentelemetry.io/contrib/exporters/autoexport v0.67.0"
    - "go.opentelemetry.io/contrib/bridges/otelslog v0.17.0"
    - "go.opentelemetry.io/contrib/instrumentation/runtime v0.67.0"
    - "go.opentelemetry.io/otel/sdk/log v0.18.0"
  patterns:
    - "autoexport for environment-driven OTel exporter configuration"
    - "fanoutHandler pattern for dispatching slog records to multiple handlers"
    - "SetupInput/SetupOutput structs for OTel initialization per CS-5"

key-files:
  created:
    - "internal/otel/logger.go"
    - "internal/otel/logger_test.go"
    - "internal/otel/metrics.go"
    - "internal/otel/metrics_test.go"
    - "internal/otel/provider_test.go"
    - "internal/config/config_test.go"
  modified:
    - "internal/otel/provider.go"
    - "internal/config/config.go"
    - "cmd/peeringdb-plus/main.go"
    - "go.mod"
    - "go.sum"

key-decisions:
  - "Removed OTelEndpoint config field -- autoexport reads OTEL_EXPORTER_OTLP_ENDPOINT directly"
  - "fanoutHandler dispatches best-effort: individual handler errors are silently ignored to avoid cascading failures"
  - "Shutdown order is reverse initialization: LoggerProvider, MeterProvider, TracerProvider"

patterns-established:
  - "autoexport for all OTel exporters: configure via OTEL_*_EXPORTER env vars, no hardcoded exporters"
  - "fanoutHandler for dual slog output: each log record goes to both stdout and OTel pipeline"
  - "Custom metric registration via InitMetrics() with explicit bucket boundaries"

requirements-completed: [OPS-01, OPS-02, OPS-03]

# Metrics
duration: 7min
completed: 2026-03-22
---

# Phase 03 Plan 01: OTel Foundation Summary

**Autoexport-driven OTel initialization with all three signals (traces, metrics, logs), dual slog handler for stdout+OTel output, and custom sync metrics with configurable sampling**

## Performance

- **Duration:** 7 min
- **Started:** 2026-03-22T17:48:33Z
- **Completed:** 2026-03-22T17:55:54Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments
- Rewrote OTel provider to support TracerProvider, MeterProvider, and LoggerProvider using autoexport for environment-driven configuration
- Built dual slog handler (fanoutHandler) that sends every log record to both stdout JSON and OTel log pipeline simultaneously
- Registered custom sync metrics (duration histogram with explicit bucket boundaries, operations counter by status)
- Extended config with OTelSampleRate (validated 0.0-1.0) and SyncStaleThreshold (default 24h)
- Added W3C TraceContext/Baggage propagation and Go runtime metrics collection
- Included Fly.io resource attributes (FLY_REGION, FLY_MACHINE_ID, FLY_APP_NAME) in OTel resource

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend config and rewrite OTel provider with all three signals** - `db705a8` (feat)
2. **Task 2: Create dual slog logger and custom sync metrics** - `54ba49a` (feat)

## Files Created/Modified
- `internal/otel/provider.go` - Centralized OTel setup with autoexport, configurable sampling, W3C propagation, runtime metrics
- `internal/otel/provider_test.go` - Tests for Setup, buildResource, shutdown, global provider verification
- `internal/otel/logger.go` - Dual slog handler dispatching to stdout JSON and OTel log pipeline
- `internal/otel/logger_test.go` - Tests for fanoutHandler Enabled/Handle/WithAttrs/WithGroup and dual logger output
- `internal/otel/metrics.go` - Custom sync metrics (duration histogram, operations counter)
- `internal/otel/metrics_test.go` - Tests for metric registration and recording without panic
- `internal/config/config.go` - Added OTelSampleRate, SyncStaleThreshold; removed OTelEndpoint; added parseFloat64
- `internal/config/config_test.go` - Table-driven tests for sample rate validation and stale threshold parsing
- `cmd/peeringdb-plus/main.go` - Updated to use new Setup API with SetupInput/SetupOutput, dual logger
- `go.mod` - Added autoexport, otelslog bridge, runtime instrumentation, SDK log dependencies
- `go.sum` - Updated dependency checksums

## Decisions Made
- Removed OTelEndpoint config field since autoexport reads OTEL_EXPORTER_OTLP_ENDPOINT directly from environment
- fanoutHandler ignores individual handler errors (best-effort) to prevent one failing handler from blocking the other
- Shutdown calls providers in reverse initialization order (log, metric, trace) using errors.Join for error aggregation
- Tests cannot use t.Parallel() with t.Setenv in Go 1.26 -- subtests run sequentially instead

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Resolved genproto version ambiguity**
- **Found during:** Task 1 (dependency installation)
- **Issue:** google.golang.org/genproto had split modules causing ambiguous imports with OTel's gRPC dependencies
- **Fix:** Explicitly upgraded genproto and added googleapis/api + googleapis/rpc sub-modules
- **Files modified:** go.mod, go.sum
- **Verification:** go mod tidy succeeds, all packages resolve
- **Committed in:** db705a8 (Task 1 commit)

**2. [Rule 1 - Bug] Created placeholder logger.go for build compatibility**
- **Found during:** Task 1 (main.go update)
- **Issue:** main.go references NewDualLogger from Task 2 but needs to compile for Task 1 tests
- **Fix:** Created minimal stub NewDualLogger that returns stdout-only logger, replaced in Task 2
- **Files modified:** internal/otel/logger.go
- **Verification:** go build ./... passes
- **Committed in:** db705a8 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (1 blocking dependency, 1 build compatibility)
**Impact on plan:** Both fixes necessary for task execution. No scope creep.

## Issues Encountered
- Go 1.26 prevents t.Setenv in t.Parallel subtests -- removed t.Parallel from config tests that modify environment

## Known Stubs
None -- all functionality is fully wired.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- OTel foundation complete, ready for HTTP middleware instrumentation (Plan 02)
- Health endpoints can use SyncStaleThreshold config (Plan 02)
- Sync worker can record metrics via SyncDuration/SyncOperations (Plan 02/03)

## Self-Check: PASSED

All 9 key files verified present. Both task commits (db705a8, 54ba49a) verified in git log. All 25 tests pass with -race flag.

---
*Phase: 03-production-readiness*
*Completed: 2026-03-22*
