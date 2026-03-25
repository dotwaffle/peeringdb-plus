---
gsd_state_version: 1.0
milestone: v1.6
milestone_name: ConnectRPC / gRPC API
status: v1.6 milestone complete
stopped_at: Completed 24-02-PLAN.md remaining list filters
last_updated: "2026-03-25T04:00:28.357Z"
progress:
  total_phases: 4
  completed_phases: 4
  total_plans: 9
  completed_plans: 9
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-24)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Current focus:** Phase 24 — list-filtering

## Current Position

Phase: 24
Plan: Not started

## Performance Metrics

**Velocity:**

| Phase 01 P01 | 8min | 2 tasks | 43 files |
| Phase 01 P02 | 9min | 2 tasks | 123 files |
| Phase 01 P05 | 10min | 2 tasks | 10 files |
| Phase 01 P03 | 11min | 2 tasks | 4 files |
| Phase 01 P04 | 18min | 2 tasks | 8 files |
| Phase 01 P06 | 3min | 1 tasks | 5 files |
| Phase 01 P07 | 6min | 2 tasks | 14 files |
| Phase 02 P01 | 7min | 2 tasks | 22 files |
| Phase 02 P03 | 3min | 2 tasks | 5 files |
| Phase 02 P02 | 6min | 2 tasks | 7 files |
| Phase 02 P04 | 13min | 2 tasks | 5 files |
| Phase 03 P02 | 4min | 2 tasks | 4 files |
| Phase 03 P01 | 7min | 2 tasks | 11 files |
| Phase 03 P03 | 5min | 2 tasks | 5 files |
| Phase 06 P01 | 8min | 2 tasks | 7 files |
| Phase 06 P02 | 5min | 2 tasks | 3 files |
| Phase 06 P03 | 9min | 2 tasks | 6 files |
| Phase 08 P01 | 5min | 2 tasks | 4 files |
| Phase 08 P02 | 4min | 2 tasks | 3 files |
| Phase 08 P03 | 8min | 2 tasks | 4 files |
| Phase 11 P01 | 4min | 1 tasks | 4 files |
| Phase 11 P02 | 3min | 1 tasks | 1 files |
| Phase 12 P01 | 4min | 2 tasks | 3 files |
| Phase 15 P01 | 6min | 2 tasks | 8 files |
| Phase 15 P02 | 13min | 2 tasks | 12 files |
| Phase 22 P01 | 3min | 2 tasks | 6 files |

## Accumulated Context

### Decisions

All decisions archived in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- [v1.6]: ConnectRPC over standard gRPC -- handlers are http.Handler, mount on existing mux
- [v1.6]: Remove LiteFS proxy to enable native gRPC wire protocol via h2c
- [v1.6]: entproto + buf toolchain for proto generation, not protoc-gen-entgrpc
- [v1.6]: Hand-written service implementations querying ent directly
- [Phase 21]: http.Protocols (Go 1.24+ stdlib) for h2c instead of x/net/http2/h2c
- [Phase 21]: fly-replay gated on FLY_REGION presence; 503 not primary for local non-primary nodes
- [Phase 21]: LiteFS proxy removed; app serves traffic directly on :8080 for h2c/gRPC support
- [Phase 22]: ConnectRPC simple option for cleaner handler signatures
- [Phase 22]: entproto SkipGenFile with buf toolchain instead of protoc go:generate
- [Phase 22]: Manual common.proto for SocialMedia -- entproto cannot handle custom struct JSON fields
- [Phase 22]: entproto.Field(1) required explicitly on id fields -- not auto-assigned as documented
- [Phase 22]: WithProtoDir ../proto -- entproto creates package subdir peeringdb/v1/ from PackageName
- [Phase 22]: Generated proto file named v1.proto (package version), not entpb.proto -- entproto default naming
- [Phase 22]: Hand-written services.proto for ConnectRPC -- entproto generates messages only, not service/RPC definitions
- [Phase 23]: Used testutil.SetupClient for SQLite driver registration -- consistent with existing test patterns
- [Phase 23]: Fetch pageSize+1 rows for next-page detection -- avoids separate COUNT query
- [Phase 23]: Cross-referenced every proto field type against generated v1.pb.go to catch wrapper vs direct type mismatches
- [Phase 23]: connectcors helpers for CORS header merging with existing app config
- [Phase 23]: gRPC health check bypasses readiness middleware; manages own NOT_SERVING/SERVING state
- [Phase 24]: No country filter on ListNetworksRequest -- Network ent schema has no country field
- [Phase 24]: Predicate accumulation pattern: []predicate.T with entity.And() for filter composition
- [Phase 24]: Consistent predicate accumulation pattern across all 13 handlers for maintainability

### Pending Todos

None.

### Blockers/Concerns

- LiteFS in maintenance mode -- monitor for issues
- Research flag: entproto + custom types (FlexDate, FlexInt) undocumented -- validate in Phase 22
- Research flag: JSON fields (social_media, info_types) need manual proto definitions in Phase 22
- Research flag: Filtering (API-03, Phase 24) has no established pattern for typed filter fields to ent predicates

## Session Continuity

Last session: 2026-03-25T03:49:05.907Z
Stopped at: Completed 24-02-PLAN.md remaining list filters
Resume file: None
