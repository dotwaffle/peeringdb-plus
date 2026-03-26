---
phase: 35-http-caching-benchmarks
plan: 02
subsystem: testing
tags: [benchmark, benchstat, sqlite, grpc, search, upsert, projection]

# Dependency graph
requires: []
provides:
  - "Performance benchmark suite covering 4 hot paths: search, field projection, gRPC list, sync upsert"
  - "Baseline ns/op measurements for regression detection via benchstat"
affects: [35-http-caching-benchmarks]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "b.Loop() pattern for Go 1.26 benchmarks (replaces b.N)"
    - "enttest.Open with testing.B for benchmark database setup"
    - "In-memory SQLite for deterministic benchmark isolation"

key-files:
  created:
    - internal/web/search_bench_test.go
    - internal/pdbcompat/projection_bench_test.go
    - internal/grpcserver/list_bench_test.go
    - internal/sync/upsert_bench_test.go
  modified: []

key-decisions:
  - "Benchmarked ListNetworks instead of aspirational ListEntities (generic.go does not exist)"
  - "Used enttest.Open directly with testing.B instead of testutil.SetupClient (accepts *testing.T only)"

patterns-established:
  - "Benchmark seeding: create data outside loop, b.ResetTimer(), b.Loop() body"
  - "Per-benchmark in-memory SQLite with unique DSN for isolation"

requirements-completed: [PERF-04]

# Metrics
duration: 4min
completed: 2026-03-26
---

# Phase 35 Plan 02: Benchmark Suite Summary

**Four benchmark files covering search (3 query patterns, 125 entities), field projection (3 field counts), gRPC list pagination (100/1000 items), and sync upsert (100/500 orgs) with Go 1.26 b.Loop() for benchstat-compatible output**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-26T07:53:42Z
- **Completed:** 2026-03-26T07:58:23Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Search benchmark exercises broad/narrow/no-match queries across 125 seeded entities in 6 types
- Field projection benchmark covers 3-field, 10-field, and no-projection scenarios on 100 items
- gRPC ListNetworks benchmark tests 100/1000 item datasets with real ent queries and protobuf conversion
- Sync upsert benchmark tests 100/500 organization batch sizes with transactional in-memory SQLite
- All benchmarks produce stable ns/op output suitable for `benchstat` comparison

## Task Commits

Each task was committed atomically:

1. **Task 1: Search and field projection benchmarks** - `0fda668` (test)
2. **Task 2: gRPC list and sync upsert benchmarks** - `b68c664` (test)

## Files Created/Modified
- `internal/web/search_bench_test.go` - BenchmarkSearch with 3 query patterns on 125 seeded entities
- `internal/pdbcompat/projection_bench_test.go` - BenchmarkApplyFieldProjection with 3 projection sizes
- `internal/grpcserver/list_bench_test.go` - BenchmarkListNetworks with 3 data/page size combos
- `internal/sync/upsert_bench_test.go` - BenchmarkUpsertOrganizations with 100/500 batch sizes

## Decisions Made
- Benchmarked `ListNetworks` method directly instead of aspirational `ListEntities` generic function -- the codebase uses per-entity handler methods, not a shared generic function
- Used `enttest.Open(b, ...)` directly in benchmarks since `testutil.SetupClient` requires `*testing.T` -- both approaches produce equivalent in-memory SQLite clients

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Adapted ListEntities benchmark to ListNetworks**
- **Found during:** Task 2 (gRPC list benchmark)
- **Issue:** Plan referenced `generic.go` with `ListEntities` function and `ListParams` type, but neither exists in the grpcserver package. The codebase uses per-entity List methods (e.g., `ListNetworks`, `ListOrganizations`)
- **Fix:** Benchmarked `ListNetworks` with real ent queries against in-memory SQLite, which exercises the same hot path (query + convert + paginate)
- **Files modified:** internal/grpcserver/list_bench_test.go
- **Verification:** Benchmark runs successfully with ns/op output
- **Committed in:** b68c664

**2. [Rule 1 - Bug] Fixed PageSize type from *int32 to int32**
- **Found during:** Task 2 (gRPC list benchmark)
- **Issue:** Initial code used `*int32` for PageSize field, but protobuf generates `int32` (non-optional field)
- **Fix:** Changed to plain int32 literal assignment
- **Files modified:** internal/grpcserver/list_bench_test.go
- **Verification:** Compilation succeeds, benchmark produces results
- **Committed in:** b68c664

---

**Total deviations:** 2 auto-fixed (1 blocking, 1 bug)
**Impact on plan:** Both fixes necessary. ListNetworks benchmark covers the same hot path pattern as the aspirational ListEntities. No scope creep.

## Issues Encountered
None beyond the deviations documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Benchmark baselines established for 4 hot paths
- Run `go test -bench=. -benchtime=1s -count=6 ./internal/...` to produce benchstat-compatible output
- Future optimizations can compare against these baselines

---
*Phase: 35-http-caching-benchmarks*
*Completed: 2026-03-26*
