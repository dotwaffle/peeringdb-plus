---
phase: 25-streaming-rpcs
plan: 01
subsystem: api
tags: [grpc, connectrpc, protobuf, streaming, otel]

# Dependency graph
requires:
  - phase: 24-connectrpc-list-handlers
    provides: "13 ConnectRPC List handlers with predicate accumulation pattern"
provides:
  - "13 Stream* RPC definitions in services.proto"
  - "13 Stream*Request messages with optional filter fields"
  - "Generated ConnectRPC handler interfaces with Stream* methods"
  - "13 stub Stream* handler implementations returning Unimplemented"
  - "StreamTimeout config field (PDBPLUS_STREAM_TIMEOUT, default 60s)"
  - "streamBatchSize constant (500) for batched keyset pagination"
  - "OTel WithoutTraceEvents for streaming RPC trace suppression"
affects: [25-02, 25-03]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Stream* stub pattern: return connect.NewError(connect.CodeUnimplemented, ...)"
    - "StreamTimeout field on service structs for deadline enforcement"

key-files:
  created: []
  modified:
    - "proto/peeringdb/v1/services.proto"
    - "gen/peeringdb/v1/peeringdbv1connect/services.connect.go"
    - "gen/peeringdb/v1/services.pb.go"
    - "internal/config/config.go"
    - "internal/config/config_test.go"
    - "internal/grpcserver/pagination.go"
    - "internal/grpcserver/network.go"
    - "internal/grpcserver/facility.go"
    - "internal/grpcserver/campus.go"
    - "internal/grpcserver/carrier.go"
    - "internal/grpcserver/carrierfacility.go"
    - "internal/grpcserver/internetexchange.go"
    - "internal/grpcserver/ixfacility.go"
    - "internal/grpcserver/ixlan.go"
    - "internal/grpcserver/ixprefix.go"
    - "internal/grpcserver/networkfacility.go"
    - "internal/grpcserver/networkixlan.go"
    - "internal/grpcserver/organization.go"
    - "internal/grpcserver/poc.go"
    - "cmd/peeringdb-plus/main.go"

key-decisions:
  - "Stub Stream* methods use underscore params to avoid unused variable warnings"
  - "OTel WithoutTraceEvents applied globally to interceptor (affects all RPCs, not just streaming)"

patterns-established:
  - "StreamTimeout field: service structs carry timeout config for streaming deadline enforcement"
  - "Stream* stub: returns CodeUnimplemented, replaced with real logic in Plan 02"

requirements-completed: [STRM-01, STRM-06, STRM-07]

# Metrics
duration: 9min
completed: 2026-03-25
---

# Phase 25 Plan 01: Streaming RPC Foundation Summary

**13 streaming RPC definitions in proto schema with stub handlers, StreamTimeout config, OTel trace event suppression, and streamBatchSize constant**

## Performance

- **Duration:** 9 min
- **Started:** 2026-03-25T06:19:13Z
- **Completed:** 2026-03-25T06:28:52Z
- **Tasks:** 2
- **Files modified:** 20

## Accomplishments
- Added 13 Stream* RPCs and 13 Stream*Request messages to services.proto with format negotiation documentation
- Regenerated ConnectRPC code via buf generate with all 13 handler interfaces updated
- Added stub Stream* methods to all 13 handler files returning CodeUnimplemented
- Added StreamTimeout config field with PDBPLUS_STREAM_TIMEOUT env var (default 60s) and table-driven tests
- Added streamBatchSize constant (500) in pagination.go for batched keyset iteration
- Updated OTel interceptor with WithoutTraceEvents to suppress per-message trace events
- Wired cfg.StreamTimeout to all 13 service struct registrations in main.go

## Task Commits

Each task was committed atomically:

1. **Task 1: Proto schema + codegen + config + constants** - `04a0a95` (feat)
2. **Task 2: Stub handlers + OTel update + service struct wiring** - `fbb8dde` (feat)

## Files Created/Modified
- `proto/peeringdb/v1/services.proto` - 13 Stream* RPCs and Stream*Request messages with format negotiation comments
- `gen/peeringdb/v1/peeringdbv1connect/services.connect.go` - Generated handler interfaces with Stream* methods
- `gen/peeringdb/v1/services.pb.go` - Generated protobuf types for Stream*Request messages
- `internal/config/config.go` - StreamTimeout field with PDBPLUS_STREAM_TIMEOUT parsing
- `internal/config/config_test.go` - TestLoad_StreamTimeout with 4 table-driven test cases
- `internal/grpcserver/pagination.go` - streamBatchSize = 500 constant
- `internal/grpcserver/*.go` (13 files) - StreamTimeout field + stub Stream* method per handler
- `cmd/peeringdb-plus/main.go` - WithoutTraceEvents on OTel interceptor + StreamTimeout in all 13 registrations

## Decisions Made
- OTel WithoutTraceEvents applied globally to the single interceptor (simplest approach; all RPCs benefit from reduced trace overhead)
- Stub methods use underscore params (`_ context.Context`) to pass go vet without unused variable warnings
- StreamTimeout wired as struct field (not global) to support per-service override in future if needed

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All 13 Stream* interfaces are satisfied with stubs -- project compiles cleanly
- Plan 02 can now implement real streaming handler logic using the predicate accumulation pattern from List handlers
- Plan 03 can implement tests knowing the exact method signatures
- streamBatchSize constant ready for use in batched keyset pagination loops

## Self-Check: PASSED

All files verified present. Both commit hashes confirmed in git log. Key content patterns (StreamNetworks, streamBatchSize, StreamTimeout, WithoutTraceEvents) confirmed in target files.

---
*Phase: 25-streaming-rpcs*
*Completed: 2026-03-25*
