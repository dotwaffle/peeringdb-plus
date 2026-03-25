---
phase: 23-connectrpc-services
plan: 01
subsystem: api
tags: [connectrpc, grpc, protobuf, pagination, ent, conversion]

# Dependency graph
requires:
  - phase: 22-proto-generation-pipeline plan 03
    provides: ConnectRPC handler interfaces, proto message types, request/response types
provides:
  - internal/grpcserver package with ent-to-proto conversion helpers
  - Pagination logic with offset-based base64 cursors (100 default, 1000 max)
  - NetworkService handler implementing Get and List RPCs
  - Template pattern for remaining 12 service implementations
affects: [23-connectrpc-services plan 02, 23-connectrpc-services plan 03]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Typed conversion helpers (stringVal, int64PtrVal, timestampPtrVal) for ent-to-proto field mapping"
    - "Offset-based pagination with base64-encoded opaque cursors and fetch-one-extra pattern"
    - "NetworkService as template: struct with *ent.Client, Get/List methods, entityToProto converter"

key-files:
  created:
    - internal/grpcserver/convert.go
    - internal/grpcserver/pagination.go
    - internal/grpcserver/pagination_test.go
    - internal/grpcserver/network.go
    - internal/grpcserver/grpcserver_test.go
  modified:
    - go.mod

key-decisions:
  - "Used testutil.SetupClient for SQLite driver registration instead of inline setup"
  - "networkToProto as unexported function since conversion is package-internal"
  - "Fetch pageSize+1 rows to detect next page without a separate COUNT query"

patterns-established:
  - "Service struct: type XxxService struct { Client *ent.Client }"
  - "Get RPC: client.Xxx.Get(ctx, id) with ent.IsNotFound -> connect.CodeNotFound"
  - "List RPC: normalizePageSize, decodePageToken, Query().Order(Asc(FieldID)).Limit(n+1).Offset(off)"
  - "Conversion: xxxToProto mapping every ent field to proto field using typed helpers"

requirements-completed: [API-01, API-02, OBS-01]

# Metrics
duration: 8min
completed: 2026-03-25
---

# Phase 23 Plan 01: gRPC Server Foundation Summary

**Ent-to-proto conversion helpers, offset pagination with base64 cursors, and NetworkService Get/List RPCs as template for all 13 entity types**

## Performance

- **Duration:** 8 min
- **Started:** 2026-03-25T02:30:26Z
- **Completed:** 2026-03-25T02:38:31Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Created internal/grpcserver package with 8 typed conversion helpers for ent-to-proto field mapping
- Implemented pagination with 100 default / 1000 max page size and base64 offset cursors
- Built NetworkService with GetNetwork (success + NOT_FOUND) and ListNetworks (pagination + ordering)
- All 17 tests pass with -race detector: 7 pagination tests, 2 GetNetwork tests, 4 ListNetworks tests, 4 remaining

## Task Commits

Each task was committed atomically:

1. **Task 1: Conversion helpers and pagination logic** - `799b438` (feat)
2. **Task 2 RED: Failing NetworkService tests** - `e4932aa` (test)
3. **Task 2 GREEN: NetworkService implementation** - `a66247e` (feat)

## Files Created/Modified
- `internal/grpcserver/convert.go` - 8 typed conversion helpers: stringVal, stringPtrVal, int64Val, int64PtrVal, boolPtrVal, float64PtrVal, timestampVal, timestampPtrVal
- `internal/grpcserver/pagination.go` - normalizePageSize, decodePageToken, encodePageToken with 100/1000 defaults
- `internal/grpcserver/pagination_test.go` - Table-driven tests for all pagination functions with roundtrip
- `internal/grpcserver/network.go` - NetworkService implementing GetNetwork and ListNetworks RPCs
- `internal/grpcserver/grpcserver_test.go` - Tests for Get (success, NOT_FOUND) and List (pagination, default size, invalid token, ordering)
- `go.mod` - connectrpc.com/connect upgraded to v1.19.1

## Decisions Made
- Used testutil.SetupClient(t) from existing test infrastructure rather than rolling inline SQLite setup -- consistent with rest of codebase
- networkToProto is unexported since only called within the grpcserver package -- Plan 02 services will follow same pattern
- Fetch pageSize+1 rows to detect next page existence without separate COUNT query -- simpler and avoids double roundtrip

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Merged phase-23 branch into worktree for gen/ directory access**
- **Found during:** Task 2 (tests needed gen/peeringdb/v1 proto types)
- **Issue:** Worktree was based on main branch, missing Phase 22 generated code (gen/ directory, proto files)
- **Fix:** Merged gsd/phase-23-connectrpc-services branch to get all Phase 22 artifacts
- **Files modified:** Multiple (gen/, proto/, .planning/ directories)
- **Verification:** go build ./... compiles clean, all tests pass

**2. [Rule 3 - Blocking] Used testutil.SetupClient instead of inline enttest setup**
- **Found during:** Task 2 (SQLite driver not registered)
- **Issue:** enttest.Open requires SQLite driver to be registered; inline setup with enttest.TestClient type was incorrect
- **Fix:** Switched to existing testutil.SetupClient(t) which handles driver registration
- **Files modified:** internal/grpcserver/grpcserver_test.go
- **Verification:** All tests pass with -race

---

**Total deviations:** 2 auto-fixed (2 blocking)
**Impact on plan:** Both auto-fixes necessary for compilation and test execution. No scope creep.

## Issues Encountered
- ConnectRPC ecosystem deps (otelconnect, grpcreflect, grpchealth, cors) were installed but removed by go mod tidy since no code imports them yet -- they will be pulled in during Plan 03 when service registration and middleware are implemented.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- NetworkService pattern established and tested -- Plan 02 will replicate for 12 remaining entity types
- Conversion helpers and pagination logic are shared infrastructure ready for reuse
- All patterns proven end-to-end: ent query, error mapping, field conversion, pagination cursors

## Self-Check: PASSED

All 5 created files verified present, all 3 commit hashes found in git log.

---
*Phase: 23-connectrpc-services*
*Completed: 2026-03-25*
