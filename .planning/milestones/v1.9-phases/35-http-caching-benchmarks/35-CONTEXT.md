# Phase 35 Context: HTTP Caching & Benchmarks

## Requirements
- **PERF-02**: API responses include HTTP caching headers (Cache-Control, ETag)
- **PERF-04**: Benchmark suite for hot paths

## Decisions

### HTTP Caching Middleware
- **Scope**: Read-only endpoints only (GET/HEAD). Skip POST /sync and mutation paths.
- **ETag**: Hash of last successful sync completion timestamp. Changes when sync completes.
- **Cache-Control**: Dynamic max-age calculated from sync interval config + 2 minute buffer
  - Example: if sync interval is 3600s, max-age = 3600 + 120 = 3720
  - Aligns cache expiry with expected data freshness
- **304 Not Modified**: If `If-None-Match` header matches current ETag, return 304
- **Implementation**: Single middleware that wraps the mux, checks method, sets headers
- **Sync timestamp source**: Read from sync worker's last completion time (already tracked in sync_status or similar)

### Benchmark Suite
- Benchmarks live alongside tests in each package (not central directory)
- Files: `*_bench_test.go` in each package
- **Required benchmarks**:
  1. `web/search_bench_test.go` — search across 6 entity types
  2. `pdbcompat/projection_bench_test.go` — field projection with pre-built map
  3. `grpcserver/list_bench_test.go` — generic List with entity conversion
  4. `sync/upsert_bench_test.go` — bulk upsert batching
- Benchmarks must be stable (no flaky external I/O) and comparable via `benchstat`
- Use in-memory SQLite for database benchmarks
- Seed realistic data volumes (100+ entities per type)

## Scope Boundaries
- Do NOT add application-level caching (sync.Map, LRU, etc.) — HTTP caching is sufficient
- Do NOT cache GraphQL responses (POST requests, variable queries)
- Do NOT add Vary headers beyond what exists — User-Agent already set
- Benchmark targets are baselines, not performance gates — no CI enforcement yet
