---
phase: 22-proto-generation-pipeline
plan: 01
subsystem: infra
tags: [protobuf, buf, connectrpc, entproto, grpc, code-generation]

# Dependency graph
requires:
  - phase: 21-infrastructure
    provides: project foundation with h2c support for gRPC transport
provides:
  - buf.yaml and buf.gen.yaml workspace config for proto generation
  - protoc-gen-go and protoc-gen-connect-go as Go tool dependencies
  - connectrpc.com/connect runtime dependency
  - entproto extension wired into entc.go with SkipGenFile and WithProtoDir
  - proto/peeringdb/v1/common.proto with SocialMedia message definition
affects: [22-proto-generation-pipeline plan 02, 23-connectrpc-services]

# Tech tracking
tech-stack:
  added:
    - connectrpc.com/connect v1.19.1
    - google.golang.org/protobuf/cmd/protoc-gen-go (tool)
    - connectrpc.com/connect/cmd/protoc-gen-connect-go (tool)
    - entgo.io/contrib/entproto (already in go.mod via entgo.io/contrib)
    - buf CLI (via go run, not installed)
  patterns:
    - Go 1.24+ tool dependencies via go get -tool
    - buf v2 config with managed go_package_prefix
    - Local plugin resolution via [go, tool, plugin-name] in buf.gen.yaml
    - entproto SkipGenFile to use buf instead of protoc go:generate directives

key-files:
  created:
    - buf.yaml
    - buf.gen.yaml
    - proto/peeringdb/v1/common.proto
  modified:
    - go.mod
    - go.sum
    - ent/entc.go

key-decisions:
  - "ConnectRPC simple option for cleaner handler signatures -- (ctx, *Request) -> (*Response, error)"
  - "entproto SkipGenFile to use buf toolchain instead of protoc go:generate directives"
  - "Manual common.proto for SocialMedia since entproto cannot handle custom struct JSON fields"
  - "PACKAGE_VERSION_SUFFIX excluded from buf lint since entproto generates peeringdb.v1 not peeringdb.v1beta1"

patterns-established:
  - "Proto toolchain: buf.yaml + buf.gen.yaml at project root with proto/ module directory"
  - "Tool deps: protoc plugins tracked in go.mod tool block, resolved via [go, tool, name] in buf.gen.yaml"
  - "entproto extension: SkipGenFile + WithProtoDir relative to ent/ directory (../proto/peeringdb/v1)"

requirements-completed: [PROTO-02, PROTO-03]

# Metrics
duration: 3min
completed: 2026-03-25
---

# Phase 22 Plan 01: Proto Toolchain Setup Summary

**Protobuf toolchain with buf v2 config, protoc-gen-go + protoc-gen-connect-go tool deps, entproto extension in entc.go, and manual SocialMedia proto message**

## Performance

- **Duration:** 3 min
- **Started:** 2026-03-25T00:51:39Z
- **Completed:** 2026-03-25T00:55:10Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Added protoc-gen-go and protoc-gen-connect-go as Go tool dependencies with connectrpc.com/connect runtime
- Created buf.yaml (v2) and buf.gen.yaml (v2) with local plugin resolution and managed go_package_prefix
- Created proto/peeringdb/v1/common.proto with SocialMedia message for fields entproto cannot generate
- Wired entproto extension into entc.go alongside entgql and entrest with SkipGenFile and WithProtoDir

## Task Commits

Each task was committed atomically:

1. **Task 1: Add tool and runtime dependencies, create buf config and common.proto** - `4657ab2` (chore)
2. **Task 2: Wire entproto extension into entc.go** - `d66dcb9` (feat)

## Files Created/Modified
- `go.mod` - Added tool block (protoc-gen-go, protoc-gen-connect-go) and connectrpc.com/connect dependency
- `go.sum` - Updated checksums for new dependencies
- `buf.yaml` - buf v2 workspace config with proto module, STANDARD lint rules, PACKAGE_VERSION_SUFFIX exception
- `buf.gen.yaml` - buf v2 generation config with protoc-gen-go and protoc-gen-connect-go plugins, managed go_package_prefix
- `proto/peeringdb/v1/common.proto` - Manual SocialMedia message definition (service + identifier fields)
- `ent/entc.go` - Added entproto import and extension with SkipGenFile + WithProtoDir

## Decisions Made
- Used `simple` option on protoc-gen-connect-go for cleaner Go handler signatures
- Used `SkipGenFile()` on entproto to prevent generation of `//go:generate protoc` directives -- buf handles generation instead
- Wrote manual `common.proto` for SocialMedia because entproto only supports `[]string`, `[]int32`, `[]int64`, `[]uint32`, `[]uint64` JSON types, not custom structs
- Excluded `PACKAGE_VERSION_SUFFIX` from buf lint rules since entproto generates `peeringdb.v1` package naming
- Used `../proto/peeringdb/v1` for WithProtoDir because entc.go runs from the `ent/` directory

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- Protobuf toolchain is fully configured and ready for Plan 02 (schema annotations)
- Plan 02 will annotate all 13 ent schemas with entproto.Message and entproto.Field annotations
- After Plan 02, `go generate ./ent/...` will produce .proto files and `buf generate` will compile them

## Self-Check: PASSED

All 5 artifacts found, both commit hashes verified.

---
*Phase: 22-proto-generation-pipeline*
*Completed: 2026-03-25*
