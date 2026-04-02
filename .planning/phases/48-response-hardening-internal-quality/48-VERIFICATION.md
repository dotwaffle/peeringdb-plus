---
phase: 48-response-hardening-internal-quality
verified: 2026-04-02T05:10:00Z
status: passed
score: 4/4 must-haves verified
---

# Phase 48: Response Hardening & Internal Quality Verification Report

**Phase Goal:** The application serves responses with security headers and compression, and internal error handling uses type-safe patterns instead of string matching
**Verified:** 2026-04-02T05:10:00Z
**Status:** passed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Web UI responses include a Content-Security-Policy-Report-Only header with directives allowing the known CDN origins (jsdelivr, unpkg) while reporting violations | VERIFIED | `internal/middleware/csp.go` sets `Content-Security-Policy-Report-Only` header on `/ui` and `/ui/*` paths; policy includes `cdn.jsdelivr.net`, `unpkg.com`, `basemaps.cartocdn.com`; wired in `main.go:394-397`; 10 table-driven tests pass including directive verification |
| 2 | HTML and JSON responses are gzip-compressed while gRPC content types are NOT compressed by the HTTP middleware | VERIFIED | `internal/middleware/compression.go` uses `gzhttp.NewWrapper` with `ExceptContentTypes` for `application/grpc`, `application/grpc+proto`, `application/connect+proto`; wired in `main.go:384-385`; 6 table-driven tests verify gzip encoding for HTML/JSON and exclusion for gRPC types |
| 3 | The metrics type count gauge returns cached values computed at sync completion time, not live COUNT queries on each scrape | VERIFIED | `internal/otel/metrics.go:150` `InitObjectCountGauges` takes `countsFn func() map[string]int64` (no ent.Client); `main.go:140` creates `atomic.Pointer[map[string]int64]`; `main.go:146` passes cache reader; `main.go:169-174` sync worker callback stores updated counts; 3 gauge tests pass |
| 4 | GraphQL errors for not-found entities return a structured error with appropriate classification, using sentinel error checks instead of string matching | VERIFIED | `internal/graphql/handler.go:63-69` uses `ent.IsNotFound`, `ent.IsValidationError`, `ent.IsConstraintError`; zero `strings.Contains` calls in the file; 7 table-driven tests including wrapped error cases pass |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/middleware/csp.go` | CSP middleware with per-route policies | VERIFIED | 38 lines, exports `CSP` and `CSPInput`, path-based switch for /ui/ and /graphql |
| `internal/middleware/csp_test.go` | CSP middleware unit tests | VERIFIED | 147 lines, `TestCSP` (8 cases) + `TestCSPUIDirectives` (directive verification) |
| `internal/middleware/compression.go` | Gzip compression middleware using klauspost/gzhttp | VERIFIED | 31 lines, exports `Compression`, excludes 3 gRPC content types |
| `internal/middleware/compression_test.go` | Compression middleware unit tests | VERIFIED | 115 lines, `TestCompression` (6 cases) including gzip decode verification |
| `internal/graphql/handler.go` | Type-safe classifyError using ent error type checks | VERIFIED | `ent.IsNotFound`/`IsValidationError`/`IsConstraintError` at lines 63-69; no string matching |
| `internal/graphql/handler_test.go` | Tests for classifyError with ent error types | VERIFIED | 38 lines, `TestClassifyError` (7 cases) including wrapped errors |
| `internal/otel/metrics.go` | Cached object count gauge reading from callback | VERIFIED | `InitObjectCountGauges(countsFn func() map[string]int64)` at line 150; no ent import |
| `internal/sync/worker.go` | Post-sync count cache update callback | VERIFIED | `OnSyncComplete func(counts map[string]int)` in `WorkerConfig` line 32; invoked at line 331-332 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `cmd/peeringdb-plus/main.go` | `internal/middleware/csp.go` | `middleware.CSP` wrapping handlers | WIRED | Line 394: `middleware.CSP(middleware.CSPInput{...})(handler)` |
| `cmd/peeringdb-plus/main.go` | `internal/middleware/compression.go` | `middleware.Compression` in chain | WIRED | Line 384: `compressionMiddleware := middleware.Compression()` |
| `internal/graphql/handler.go` | `ent` | `ent.IsNotFound/IsValidationError/IsConstraintError` | WIRED | Lines 63-67: all three ent type check functions present |
| `internal/sync/worker.go` | `internal/otel/metrics.go` | `OnSyncComplete` callback updates cache | WIRED | Worker line 331-332 calls callback; main.go line 169-174 stores into atomic cache; main.go line 146-147 passes cache reader to `InitObjectCountGauges` |
| `internal/otel/metrics.go` | `cmd/peeringdb-plus/main.go` | `InitObjectCountGauges` accepts cache function | WIRED | main.go line 146: `pdbotel.InitObjectCountGauges(func() map[string]int64 { return *objectCountCache.Load() })` |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `internal/otel/metrics.go` | `countsFn()` map | `atomic.Pointer` in main.go, populated by sync worker callback | Yes -- sync worker populates `objectCounts` map during sync steps | FLOWING |
| `internal/graphql/handler.go` | `classifyError(err)` | ent error types from database operations | Yes -- ent queries produce real typed errors | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Middleware tests pass | `go test -race ./internal/middleware/ -run "TestCSP\|TestCompression"` | 17 passed | PASS |
| ClassifyError tests pass | `go test -race ./internal/graphql/ -run TestClassifyError` | 8 passed | PASS |
| Metrics gauge tests pass | `go test -race ./internal/otel/ -run TestInitObjectCountGauges` | 3 passed | PASS |
| Full build succeeds | `go build ./...` | Success | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| SEC-03 | 48-01 | Content-Security-Policy-Report-Only header served with CDN allowlist on web UI responses | SATISFIED | `csp.go` sets Report-Only header with jsdelivr/unpkg/cartocdn allowlist; wired in main.go; tests verify exact directives |
| PERF-01 | 48-01 | HTTP responses compressed via gzip middleware, excluding gRPC content types | SATISFIED | `compression.go` uses gzhttp with ExceptContentTypes for gRPC; wired in main.go chain; tests verify compression and exclusion |
| PERF-02 | 48-02 | Metrics type count gauge reads cached values computed at sync time, not per-scrape COUNT queries | SATISFIED | `InitObjectCountGauges` takes `countsFn` callback (no ent.Client); atomic cache in main.go; sync worker OnSyncComplete updates cache |
| PERF-03 | 48-02 | GraphQL error presenter classifies errors via ent.IsNotFound / errors.Is instead of string matching | SATISFIED | `classifyError` uses `ent.IsNotFound`/`IsValidationError`/`IsConstraintError`; zero `strings.Contains` calls remain; wrapped errors handled correctly |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | -- | -- | -- | -- |

No TODOs, FIXMEs, placeholders, empty returns, or string-matching patterns found in any modified files.

### Human Verification Required

### 1. CSP Header in Browser

**Test:** Load `/ui/` in a browser with DevTools Network tab open; inspect the `Content-Security-Policy-Report-Only` response header.
**Expected:** Header present with directives matching: `default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; ...`
**Why human:** Browser rendering and actual header inspection cannot be verified programmatically without a running server.

### 2. Gzip Compression on Live Responses

**Test:** `curl -H "Accept-Encoding: gzip" -sI https://peeringdb-plus.fly.dev/ui/ | grep Content-Encoding`
**Expected:** `Content-Encoding: gzip`
**Why human:** Requires deployed application to verify end-to-end compression through the full middleware chain.

### 3. GraphQL Error Classification in Practice

**Test:** Execute a GraphQL query for a non-existent ASN: `{ networkByAsn(asn: 999999999) { name } }` and inspect the error response extensions.
**Expected:** Error includes `"extensions": {"code": "NOT_FOUND"}`
**Why human:** Requires a running server with data to produce real ent errors through the GraphQL pipeline.

### Gaps Summary

No gaps found. All 4 observable truths verified. All 4 requirements (SEC-03, PERF-01, PERF-02, PERF-03) satisfied. All artifacts exist, are substantive, are wired, and have data flowing through them. All tests pass with -race. Build succeeds. Commits verified: ee07a3d, 72be127, d45e483, 16b308c.

---

_Verified: 2026-04-02T05:10:00Z_
_Verifier: Claude (gsd-verifier)_
