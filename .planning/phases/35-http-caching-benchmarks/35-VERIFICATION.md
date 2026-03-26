---
phase: 35-http-caching-benchmarks
verified: 2026-03-26T08:10:00Z
status: passed
score: 3/3 must-haves verified
---

# Phase 35: HTTP Caching & Benchmarks Verification Report

**Phase Goal:** Browsers and HTTP clients can cache API responses between sync cycles, and a benchmark suite establishes performance baselines on the optimized code
**Verified:** 2026-03-26T08:10:00Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | API responses include Cache-Control and ETag headers derived from the last sync timestamp, and conditional If-None-Match returns 304 Not Modified | VERIFIED | `caching.go` sets `Cache-Control: public, max-age=N` and `ETag: W/"..."` on GET/HEAD; returns 304 on matching If-None-Match. 9 test cases pass with -race. Middleware wired in main.go between readiness and mux. |
| 2 | Running `go test -bench ./...` exercises benchmarks for search, field projection, gRPC entity conversion, and sync upsert | VERIFIED | Four `*_bench_test.go` files produce ns/op output: BenchmarkSearch (3 patterns), BenchmarkApplyFieldProjection (3 sizes), BenchmarkListNetworks (3 data sizes), BenchmarkUpsertOrganizations (2 batch sizes). |
| 3 | Benchmark results are stable across runs and can be compared via benchstat | VERIFIED | All benchmarks use in-memory SQLite or mock data with no external I/O. b.Loop() pattern (Go 1.26) and b.ResetTimer() after setup ensure benchstat-compatible output. |

**Score:** 3/3 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/middleware/caching.go` | Caching middleware with CachingInput, Caching, computeETag, etagMatch | VERIFIED | 77 lines. Exports CachingInput struct and Caching function. SHA-256 weak ETag. All exported symbols documented. |
| `internal/middleware/caching_test.go` | Table-driven tests for all caching behaviors | VERIFIED | 225 lines. TestCaching (7 subtests), TestCachingETagFormat, TestCachingETagChangesWithSyncTime. All parallel, all pass with -race. |
| `cmd/peeringdb-plus/main.go` | Caching middleware wired into middleware chain | VERIFIED | Lines 356-363: cachingMiddleware constructed with SyncTimeFn and SyncInterval, inserted as innermost middleware. Chain comment updated. |
| `internal/web/search_bench_test.go` | Search benchmark across 6 entity types | VERIFIED | 170 lines. Seeds 125 entities (20 orgs, 25 networks, 20 IXPs, 20 facilities, 20 campuses, 20 carriers). 3 sub-benchmarks. |
| `internal/pdbcompat/projection_bench_test.go` | Field projection benchmark | VERIFIED | 75 lines. 3 sub-benchmarks (3 fields, 10 fields, no projection) on 100 items. |
| `internal/grpcserver/list_bench_test.go` | gRPC list benchmark with entity conversion | VERIFIED | 84 lines. BenchmarkListNetworks with 3 sub-benchmarks (100/1000/0 items). Exercises real query + protobuf conversion via networkToProto. |
| `internal/sync/upsert_bench_test.go` | Sync upsert benchmark with in-memory SQLite | VERIFIED | 87 lines. 2 sub-benchmarks (100/500 orgs). Transactional upsert with enttest in-memory client. No duplicate TestMain. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| cmd/peeringdb-plus/main.go | internal/middleware/caching.go | `middleware.Caching(middleware.CachingInput{...})` | WIRED | Line 356: CachingInput constructed with SyncTimeFn and SyncInterval, applied at line 363. |
| internal/middleware/caching.go | internal/sync/status.go | SyncTimeFn closure calling GetLastSuccessfulSyncTime | WIRED | Line 358 in main.go: `pdbsync.GetLastSuccessfulSyncTime(context.Background(), db)` called inside SyncTimeFn closure. |
| search_bench_test.go | internal/web/search.go | NewSearchService + Search method | WIRED | Line 132: `NewSearchService(client)`, lines 144/154/164: `svc.Search(ctx, query)`. |
| projection_bench_test.go | internal/pdbcompat/search.go | applyFieldProjection function | WIRED | Lines 56/64/72: `applyFieldProjection(data, fields)`. Function exists at search.go:38. |
| list_bench_test.go | internal/grpcserver/network.go | ListNetworks method on NetworkService | WIRED | Line 40: `&NetworkService{Client: client}`, lines 54/66/78: `svc.ListNetworks(ctx, req)`. Method at network.go:253. |
| upsert_bench_test.go | internal/sync/upsert.go | upsertOrganizations function | WIRED | Lines 56/78: `upsertOrganizations(ctx, tx, orgs)`. Function at upsert.go:47. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Caching tests pass with -race | `go test -race ./internal/middleware/ -run TestCaching` | PASS (all 9 subtests) | PASS |
| Project builds | `go build ./cmd/peeringdb-plus/` | exit 0 | PASS |
| Search benchmark produces ns/op | `go test -bench=BenchmarkSearch -benchtime=100ms ./internal/web/` | 3 sub-benchmarks: 1.72-1.99 ms/op | PASS |
| Projection benchmark produces ns/op | `go test -bench=BenchmarkApplyFieldProjection -benchtime=100ms ./internal/pdbcompat/` | 3 sub-benchmarks: 2.2 ns - 169 us/op | PASS |
| gRPC list benchmark produces ns/op | `go test -bench=BenchmarkListNetworks -benchtime=100ms ./internal/grpcserver/` | 3 sub-benchmarks: 113 us - 1.05 ms/op | PASS |
| Upsert benchmark produces ns/op | `go test -bench=BenchmarkUpsert -benchtime=100ms ./internal/sync/` | 2 sub-benchmarks: 5.4 ms - 65.9 ms/op | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| PERF-02 | 35-01 | API responses include HTTP caching headers (Cache-Control, ETag) derived from sync timestamp | SATISFIED | caching.go sets both headers on GET/HEAD. 304 support for conditional requests. Wired in main.go. Tests pass. |
| PERF-04 | 35-02 | Benchmark suite covers search, field projection, gRPC streaming conversion, and sync upsert hot paths | SATISFIED | Four benchmark files cover all four hot paths. ListNetworks exercises the same query+convert pipeline as "gRPC streaming conversion". All produce benchstat-compatible ns/op. |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | - | - | No anti-patterns detected in any of the 7 phase files |

### Human Verification Required

### 1. Cache Headers on Live Deployment

**Test:** Deploy to Fly.io, run `curl -I https://peeringdb-plus.fly.dev/api/net` after a sync completes. Verify response includes `Cache-Control: public, max-age=3720` and `ETag: W/"..."` headers.
**Expected:** Both headers present with correct max-age value.
**Why human:** Requires live deployment with actual sync cycle to verify end-to-end header propagation through the full middleware chain.

### 2. 304 Not Modified on Live Deployment

**Test:** After getting an ETag from step 1, run `curl -I -H "If-None-Match: <etag>" https://peeringdb-plus.fly.dev/api/net`. Verify 304 status with no body.
**Expected:** HTTP 304 response, ETag header present, empty body.
**Why human:** Requires live server to verify conditional request handling under real conditions.

### Gaps Summary

No gaps found. All three ROADMAP success criteria are verified. All seven artifacts exist, are substantive, and are properly wired. Both requirements (PERF-02, PERF-04) are satisfied. All behavioral spot-checks pass. Five commits verified.

One minor deviation from PLAN was noted (documented in SUMMARY): the gRPC benchmark uses `ListNetworks` instead of the aspirational `ListEntities` generic function, because the codebase uses per-entity handlers, not a shared generic. This exercises the same hot path and satisfies the requirement.

---

_Verified: 2026-03-26T08:10:00Z_
_Verifier: Claude (gsd-verifier)_
