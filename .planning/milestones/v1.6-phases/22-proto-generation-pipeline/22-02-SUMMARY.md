---
phase: 22-proto-generation-pipeline
plan: 02
subsystem: api
tags: [protobuf, entproto, code-generation, grpc, connectrpc]

# Dependency graph
requires:
  - phase: 22-proto-generation-pipeline plan 01
    provides: buf toolchain config, entproto extension in entc.go, common.proto SocialMedia message
provides:
  - entproto.Message, entproto.Field(N), entproto.Skip() annotations on all 13 ent schemas
  - proto/peeringdb/v1/v1.proto with all 13 PeeringDB message definitions
  - gen/peeringdb/v1/v1.pb.go with generated Go protobuf types
  - gen/peeringdb/v1/common.pb.go with SocialMedia Go protobuf type
affects: [23-connectrpc-services]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "entproto.Field(1) required on id fields (not auto-assigned as documented)"
    - "WithProtoDir(../proto) -- entproto creates package subdir peeringdb/v1/ automatically"
    - "go_package must be consistent across all .proto files in the same package for buf lint"

key-files:
  created:
    - proto/peeringdb/v1/v1.proto
    - gen/peeringdb/v1/v1.pb.go
    - gen/peeringdb/v1/common.pb.go
  modified:
    - ent/schema/organization.go
    - ent/schema/network.go
    - ent/schema/facility.go
    - ent/schema/internetexchange.go
    - ent/schema/carrier.go
    - ent/schema/campus.go
    - ent/schema/carrierfacility.go
    - ent/schema/ixfacility.go
    - ent/schema/ixlan.go
    - ent/schema/ixprefix.go
    - ent/schema/networkfacility.go
    - ent/schema/networkixlan.go
    - ent/schema/poc.go
    - ent/entc.go
    - proto/peeringdb/v1/common.proto

key-decisions:
  - "entproto.Field(1) required explicitly on id fields -- entproto does not auto-assign field 1"
  - "WithProtoDir changed from ../proto/peeringdb/v1 to ../proto -- entproto creates package directory structure"
  - "go_package in common.proto updated to match entproto-generated v1.proto for buf lint consistency"
  - "No ConnectRPC connect.go files generated -- entproto produces message definitions only, services added in Phase 23"

patterns-established:
  - "entproto annotation pattern: Field(1) on id, Field(N) starting at 2 for non-ID fields, Skip() for JSON custom structs and edges"
  - "Generated proto file named v1.proto (from package version), not entpb.proto as originally expected"

requirements-completed: [PROTO-01, PROTO-04]

# Metrics
duration: 22min
completed: 2026-03-25
---

# Phase 22 Plan 02: Schema Annotations & Generation Pipeline Summary

**All 13 ent schemas annotated with entproto producing v1.proto with 227 typed fields, compiled to Go protobuf types via buf generate**

## Performance

- **Duration:** 22 min
- **Started:** 2026-03-25T01:00:29Z
- **Completed:** 2026-03-25T01:22:44Z
- **Tasks:** 2
- **Files modified:** 20

## Accomplishments
- Annotated all 13 ent schemas with entproto.Message, 240 entproto.Field(N), and 40 entproto.Skip() annotations
- Generated proto/peeringdb/v1/v1.proto containing all 13 PeeringDB message definitions with proper types (Timestamp, wrappers for nullable fields, repeated strings for JSON arrays)
- Generated gen/peeringdb/v1/v1.pb.go and common.pb.go with compilable Go protobuf types
- Full project builds and all tests pass with -race detector

## Task Commits

Each task was committed atomically:

1. **Task 1: Annotate all 13 ent schemas with entproto annotations** - `d82f4e7` (feat)
2. **Task 2: Run generation pipeline and verify compilation** - `a2d0fe6` (feat)

## Files Created/Modified
- `ent/schema/organization.go` - 22 entproto annotations (20 Field + 1 Skip + 1 Message)
- `ent/schema/network.go` - 42 entproto annotations (39 Field + 1 Skip + 1 Message + 1 id)
- `ent/schema/facility.go` - 40 entproto annotations (37 Field + 1 Skip + 1 Message + 1 id)
- `ent/schema/internetexchange.go` - 36 entproto annotations (33 Field + 1 Skip + 1 Message + 1 id)
- `ent/schema/carrier.go` - 15 entproto annotations (12 Field + 1 Skip + 1 Message + 1 id)
- `ent/schema/campus.go` - 18 entproto annotations (15 Field + 1 Skip + 1 Message + 1 id)
- `ent/schema/carrierfacility.go` - 9 entproto annotations (7 Field + 1 Message + 1 id)
- `ent/schema/ixfacility.go` - 11 entproto annotations (9 Field + 1 Message + 1 id)
- `ent/schema/ixlan.go` - 15 entproto annotations (13 Field + 1 Message + 1 id)
- `ent/schema/ixprefix.go` - 11 entproto annotations (9 Field + 1 Message + 1 id)
- `ent/schema/networkfacility.go` - 12 entproto annotations (10 Field + 1 Message + 1 id)
- `ent/schema/networkixlan.go` - 20 entproto annotations (18 Field + 1 Message + 1 id)
- `ent/schema/poc.go` - 13 entproto annotations (11 Field + 1 Message + 1 id)
- `ent/entc.go` - Fixed WithProtoDir from ../proto/peeringdb/v1 to ../proto
- `proto/peeringdb/v1/common.proto` - Updated go_package for consistency
- `proto/peeringdb/v1/v1.proto` - Generated: 13 messages, ~227 fields, no social_media
- `gen/peeringdb/v1/v1.pb.go` - Generated Go protobuf types for all 13 messages
- `gen/peeringdb/v1/common.pb.go` - Generated Go protobuf type for SocialMedia
- `go.mod` - Updated after go mod tidy
- `go.sum` - Updated after go mod tidy

## Decisions Made
- Added entproto.Field(1) to all 13 id fields -- entproto requires explicit field number annotation on every field including id (contrary to plan's assumption of auto-assignment)
- Changed WithProtoDir from `../proto/peeringdb/v1` to `../proto` -- entproto creates the package directory structure (`peeringdb/v1/`) from the PackageName annotation
- Updated common.proto go_package to match entproto's generated go_package for buf lint consistency
- No ConnectRPC .connect.go files generated because entproto only generates proto message definitions, not service definitions -- ConnectRPC handler interfaces will be created manually in Phase 23

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] entproto requires explicit Field(1) on id fields**
- **Found during:** Task 2 (go generate)
- **Issue:** Plan stated "Field number 1 is auto-assigned to id by entproto -- do NOT annotate the id field" but entproto v0.7.0 requires explicit entproto.Field annotation on every field including id
- **Fix:** Added `Annotations(entproto.Field(1))` to all 13 id fields
- **Files modified:** All 13 ent/schema/*.go files
- **Verification:** go generate succeeds after fix
- **Committed in:** a2d0fe6 (Task 2 commit)

**2. [Rule 3 - Blocking] WithProtoDir creates nested package directory**
- **Found during:** Task 2 (go generate)
- **Issue:** WithProtoDir("../proto/peeringdb/v1") caused entproto to generate at proto/peeringdb/v1/peeringdb/v1/v1.proto (double-nested)
- **Fix:** Changed to WithProtoDir("../proto") so entproto places output at proto/peeringdb/v1/v1.proto
- **Files modified:** ent/entc.go
- **Verification:** Proto file appears at correct path, buf lint passes
- **Committed in:** a2d0fe6 (Task 2 commit)

**3. [Rule 3 - Blocking] go_package mismatch between proto files**
- **Found during:** Task 2 (buf lint)
- **Issue:** common.proto had go_package "gen/peeringdb/v1" while generated v1.proto had "ent/proto/peeringdb/v1"
- **Fix:** Updated common.proto go_package to match v1.proto (buf managed mode overrides both during generation)
- **Files modified:** proto/peeringdb/v1/common.proto
- **Verification:** buf lint passes clean
- **Committed in:** a2d0fe6 (Task 2 commit)

---

**Total deviations:** 3 auto-fixed (3 blocking)
**Impact on plan:** All fixes were necessary to make the generation pipeline work. No scope creep -- the plan's assumptions about entproto behavior were incorrect for the actual v0.7.0 API.

## Issues Encountered
- Generated proto file named `v1.proto` (from package version) not `entpb.proto` as plan expected -- this is normal entproto naming behavior
- No ConnectRPC .connect.go files generated -- expected, since entproto only generates message types, not gRPC/ConnectRPC services

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All 13 PeeringDB types have proto message definitions ready for use
- Go protobuf types available at gen/peeringdb/v1/ for service implementations
- Phase 23 will define service definitions and implement ConnectRPC handlers

## Self-Check: PASSED

All 3 created artifacts found, both commit hashes verified, all 13 schemas confirmed annotated.

---
*Phase: 22-proto-generation-pipeline*
*Completed: 2026-03-25*
