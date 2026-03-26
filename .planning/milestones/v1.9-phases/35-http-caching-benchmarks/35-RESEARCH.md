# Phase 35: HTTP Caching & Benchmarks - Research

**Researched:** 2026-03-26
**Domain:** HTTP caching middleware (ETag/304), Go benchmark suite
**Confidence:** HIGH

## Summary

This phase has two distinct work areas: (1) a caching middleware that adds `Cache-Control` and `ETag` headers to read-only GET/HEAD responses, returning 304 when data has not changed; and (2) a benchmark suite for four hot paths across different packages. Both are well-understood patterns with no library dependencies beyond stdlib.

The caching middleware is straightforward because the ETag is derived from a single value (last successful sync timestamp) rather than per-response content hashing. This makes the middleware stateless from its own perspective -- it reads the current sync timestamp, formats it as an ETag, and compares against `If-None-Match`. The `Cache-Control` max-age is computed from the configured sync interval plus a 2-minute buffer.

The benchmark suite uses Go 1.26's `b.Loop()` pattern (which no longer prevents inlining) with in-memory SQLite databases. Each benchmark file lives alongside its tests in the respective package. All four target functions are already well-understood from existing test code.

**Primary recommendation:** Build the caching middleware as a single function in `internal/middleware/caching.go` that accepts a sync-timestamp provider function and the sync interval duration. Benchmark files follow the `*_bench_test.go` naming convention using `b.Loop()` with `testutil.SetupClient` for database-backed benchmarks.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

#### HTTP Caching Middleware
- **Scope**: Read-only endpoints only (GET/HEAD). Skip POST /sync and mutation paths.
- **ETag**: Hash of last successful sync completion timestamp. Changes when sync completes.
- **Cache-Control**: Dynamic max-age calculated from sync interval config + 2 minute buffer
  - Example: if sync interval is 3600s, max-age = 3600 + 120 = 3720
  - Aligns cache expiry with expected data freshness
- **304 Not Modified**: If `If-None-Match` header matches current ETag, return 304
- **Implementation**: Single middleware that wraps the mux, checks method, sets headers
- **Sync timestamp source**: Read from sync worker's last completion time (already tracked in sync_status or similar)

#### Benchmark Suite
- Benchmarks live alongside tests in each package (not central directory)
- Files: `*_bench_test.go` in each package
- **Required benchmarks**:
  1. `web/search_bench_test.go` -- search across 6 entity types
  2. `pdbcompat/projection_bench_test.go` -- field projection with pre-built map
  3. `grpcserver/list_bench_test.go` -- generic List with entity conversion
  4. `sync/upsert_bench_test.go` -- bulk upsert batching
- Benchmarks must be stable (no flaky external I/O) and comparable via `benchstat`
- Use in-memory SQLite for database benchmarks
- Seed realistic data volumes (100+ entities per type)

### Scope Boundaries (OUT OF SCOPE)
- Do NOT add application-level caching (sync.Map, LRU, etc.) -- HTTP caching is sufficient
- Do NOT cache GraphQL responses (POST requests, variable queries)
- Do NOT add Vary headers beyond what exists -- User-Agent already set
- Benchmark targets are baselines, not performance gates -- no CI enforcement yet
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PERF-02 | API responses include HTTP caching headers (Cache-Control, ETag) derived from sync timestamp | Caching middleware pattern documented below; sync timestamp already available via `GetLastSuccessfulSyncTime`; middleware placement in chain identified |
| PERF-04 | Benchmark suite covers search, field projection, gRPC streaming conversion, and sync upsert hot paths | Four benchmark files documented with specific function targets, data seeding patterns, and `b.Loop()` usage |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| net/http (stdlib) | Go 1.26 | HTTP middleware, ResponseWriter, 304 handling | Project already uses stdlib for all middleware. No external caching library needed. |
| testing (stdlib) | Go 1.26 | Benchmark framework with `b.Loop()` | Native Go benchmarks, `b.Loop()` inlining fix in 1.26. |
| crypto/sha256 (stdlib) | Go 1.26 | ETag hash generation | Deterministic hash of sync timestamp for ETag value. |
| golang.org/x/perf/cmd/benchstat | latest | Benchmark comparison tool | Required by success criteria for comparing benchmark results across runs. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| internal/testutil | project | In-memory SQLite test client | Database-backed benchmarks (search, upsert) |
| encoding/hex (stdlib) | Go 1.26 | Hex encoding of ETag hash | Converting SHA-256 bytes to string for ETag |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| SHA-256 ETag | Unix timestamp string | SHA-256 is opaque (clients cannot guess next value), timestamp string leaks internal timing |
| `b.Loop()` | `b.N` loop | `b.N` prevents inlining in Go 1.26; `b.Loop()` is the recommended pattern |
| Per-request DB query for sync time | In-memory cache of sync time | DB query is fine -- sync time is queried once per middleware invocation, not per-row. The middleware could accept a provider function. |

## Architecture Patterns

### Caching Middleware Design

The middleware is a standard `func(http.Handler) http.Handler` that:
1. Checks if the request method is GET or HEAD -- passes through all others unchanged
2. Reads the current sync timestamp via a provider function
3. Computes the ETag from the timestamp (SHA-256 hash, quoted)
4. Sets `Cache-Control: public, max-age=N` where N = sync interval seconds + 120
5. Sets `ETag: "hash"`
6. Compares `If-None-Match` request header with current ETag
7. If match: returns 304 with ETag header, no body
8. If no match: calls next handler, headers already set

**Key design decisions:**

**Provider function pattern:** The middleware accepts a `func() time.Time` that returns the last successful sync time. This avoids coupling the middleware to `*sql.DB` or the sync package. The caller (main.go) closes over the DB and calls `GetLastSuccessfulSyncTime`. This is consistent with how the readiness middleware uses `syncReadiness` interface.

**ETag format:** Use weak ETag (`W/"hash"`) since the response may vary by `Vary` headers (User-Agent, Accept) but the underlying data staleness is the same. Per RFC 7232, weak comparison is appropriate when the semantic meaning is equivalent.

**Zero-time handling:** If no successful sync has occurred, skip setting caching headers entirely (let the readiness middleware handle 503).

### Middleware Chain Placement

Current chain (outermost first):
```
Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> mux
```

The caching middleware should go between Readiness and the mux:
```
Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> Caching -> mux
```

Rationale:
- After Readiness: No point caching 503 responses before sync completes
- Before mux: Applies uniformly to all GET/HEAD routes
- After Logging: Cache hits (304) should still be logged
- After OTel: Cache hits should still be traced

### Recommended Project Structure
```
internal/middleware/
  caching.go            # Cache-Control + ETag middleware
  caching_test.go       # Unit tests for caching middleware
internal/web/
  search_bench_test.go  # Benchmark: search across 6 entity types
internal/pdbcompat/
  projection_bench_test.go  # Benchmark: field projection
internal/grpcserver/
  list_bench_test.go    # Benchmark: generic List with conversion
internal/sync/
  upsert_bench_test.go  # Benchmark: bulk upsert batching
```

### Pattern: Caching Middleware

**What:** HTTP middleware that adds caching headers and handles conditional requests.
**When to use:** All GET/HEAD requests after readiness is confirmed.

```go
// CachingInput holds configuration for the HTTP caching middleware.
type CachingInput struct {
    // SyncTimeFn returns the last successful sync completion time.
    // Returns zero time if no successful sync has occurred.
    SyncTimeFn   func() time.Time
    // SyncInterval is the configured sync interval for max-age calculation.
    SyncInterval time.Duration
}

// Caching returns middleware that adds Cache-Control and ETag headers
// to GET/HEAD responses derived from the sync timestamp per PERF-02.
func Caching(in CachingInput) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Only cache GET and HEAD requests.
            if r.Method != http.MethodGet && r.Method != http.MethodHead {
                next.ServeHTTP(w, r)
                return
            }

            syncTime := in.SyncTimeFn()
            if syncTime.IsZero() {
                // No successful sync yet -- skip caching headers.
                next.ServeHTTP(w, r)
                return
            }

            etag := computeETag(syncTime)
            maxAge := int(in.SyncInterval.Seconds()) + 120

            w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", maxAge))
            w.Header().Set("ETag", etag)

            // Check If-None-Match for conditional request.
            if match := r.Header.Get("If-None-Match"); match != "" {
                if etagMatch(match, etag) {
                    w.WriteHeader(http.StatusNotModified)
                    return
                }
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

### Pattern: Go 1.26 Benchmark with b.Loop()

**What:** Modern benchmark using `b.Loop()` instead of `b.N`.
**When to use:** All new benchmarks in Go 1.26+.

```go
func BenchmarkSearch(b *testing.B) {
    client := testutil.SetupClient(b)
    // Seed data outside benchmark loop...
    svc := NewSearchService(client)
    ctx := context.Background()

    b.ResetTimer()
    for b.Loop() {
        _, err := svc.Search(ctx, "Cloud")
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

### Anti-Patterns to Avoid
- **Content-hashing ETag on every response:** Buffering the entire response body to hash it adds latency and memory pressure. The sync-timestamp-based ETag avoids this entirely.
- **Setting Cache-Control on POST /sync or mutation paths:** POST requests must never be cached. The method check in the middleware prevents this.
- **Storing ETag in a struct field:** The ETag changes when sync completes. Computing it from the provider function on each request ensures consistency without cache invalidation logic.
- **Using `b.N` in Go 1.26 benchmarks:** `b.Loop()` is the correct pattern; it no longer prevents inlining and keeps variables alive.
- **External I/O in benchmarks:** Benchmarks must use in-memory SQLite via `testutil.SetupClient`. No network calls, no file I/O.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ETag comparison | Custom string parsing with quote stripping | Simple equality check on quoted strings | ETags in this project are always the same format (weak, SHA-256). No need for full RFC 7232 list parsing. If-None-Match can contain comma-separated values but browsers send single values for simple GETs. |
| Benchmark statistics | Custom timing/averaging | `go test -bench` + `benchstat` | Stdlib benchmarks handle warmup, iteration count, and statistics. benchstat does A/B comparison. |
| In-memory test DB | Custom SQLite setup | `testutil.SetupClient(t/b)` | Already handles enttest.Open with in-memory SQLite, shared cache, foreign keys, and cleanup. |

**Key insight:** The ETag here is not a content hash -- it is a data-freshness indicator. This makes the middleware trivially simple compared to traditional ETag middleware that must buffer response bodies.

## Common Pitfalls

### Pitfall 1: ETag Must Be Quoted
**What goes wrong:** Setting `ETag: abc123` instead of `ETag: "abc123"` or `ETag: W/"abc123"`. Browsers and HTTP clients expect ETags to be quoted per RFC 7232.
**Why it happens:** Easy to forget the quoting requirement.
**How to avoid:** The `computeETag` function must always return a properly quoted string: `W/"hexdigest"`.
**Warning signs:** `If-None-Match` comparisons never match despite identical content.

### Pitfall 2: 304 Must Not Include a Body
**What goes wrong:** Calling `next.ServeHTTP(w, r)` after writing 304 status, which causes the downstream handler to write a body.
**Why it happens:** Forgetting to return early after `w.WriteHeader(http.StatusNotModified)`.
**How to avoid:** Return immediately after writing 304. Do not call the next handler.
**Warning signs:** Browsers show errors about unexpected content in 304 responses.

### Pitfall 3: Headers Must Be Set Before WriteHeader
**What goes wrong:** Setting `Cache-Control` and `ETag` after the response body has started writing.
**Why it happens:** Go's `http.ResponseWriter` sends headers on the first `Write()` call.
**How to avoid:** Set all caching headers before calling `next.ServeHTTP(w, r)`. Since headers are set on the `ResponseWriter` before delegation, they will be included when the downstream handler writes.
**Warning signs:** Missing headers in responses, but only intermittently.

### Pitfall 4: Benchmark Timer Includes Setup
**What goes wrong:** Benchmark includes database seeding time in measurements.
**Why it happens:** Forgetting `b.ResetTimer()` after setup code.
**How to avoid:** Always call `b.ResetTimer()` after data seeding and before the benchmark loop.
**Warning signs:** Unrealistically slow first-run benchmarks, high variance.

### Pitfall 5: SyncTimeFn Must Not Query DB on Every Request
**What goes wrong:** The provider function runs a SQL query on every HTTP request, adding latency.
**Why it happens:** Naive implementation that calls `GetLastSuccessfulSyncTime` directly.
**How to avoid:** Cache the sync time in the provider closure. The sync worker already tracks completion time via `sync_status` table and `w.synced` atomic. A simple approach: the provider reads from a cached `atomic.Value` that the sync worker updates after each successful sync. Alternative: accept the DB query cost (it is a single indexed query returning one row) -- for hourly syncs this is negligible.
**Warning signs:** Increased P99 latency on all GET requests.

**Recommended approach for SyncTimeFn:** Given the project's simplicity (hourly sync, SQLite on local disk), querying the DB each time is acceptable. The query is `SELECT completed_at FROM sync_status WHERE status = 'success' ORDER BY id DESC LIMIT 1` -- a single-row indexed lookup that takes microseconds on local SQLite. If profiling later shows this matters, it can be cached. Start simple.

### Pitfall 6: Benchstat Requires Multiple Runs
**What goes wrong:** Running benchmarks once and treating results as definitive.
**Why it happens:** Not understanding statistical confidence.
**How to avoid:** Run benchmarks with `-count=6` (or more) to get statistically meaningful results for benchstat comparison.
**Warning signs:** benchstat reports "not enough data" or shows `~` (no significant difference).

## Code Examples

### ETag Computation
```go
// computeETag generates a weak ETag from the sync completion timestamp.
// Uses SHA-256 to produce an opaque, deterministic identifier.
func computeETag(syncTime time.Time) string {
    h := sha256.Sum256([]byte(syncTime.Format(time.RFC3339Nano)))
    return fmt.Sprintf(`W/"%x"`, h[:16]) // 32 hex chars, truncated for brevity
}
```

### ETag Matching
```go
// etagMatch checks if the If-None-Match header value matches the given ETag.
// Handles the common case of a single ETag value (browsers send one for GET).
func etagMatch(ifNoneMatch, etag string) bool {
    // If-None-Match: * matches any ETag.
    if strings.TrimSpace(ifNoneMatch) == "*" {
        return true
    }
    // Direct comparison (most common case for browser GET requests).
    return strings.TrimSpace(ifNoneMatch) == etag
}
```

### Middleware Integration in main.go
```go
// In the middleware chain, after readiness:
cachingMiddleware := middleware.Caching(middleware.CachingInput{
    SyncTimeFn: func() time.Time {
        t, _ := pdbsync.GetLastSuccessfulSyncTime(context.Background(), db)
        return t
    },
    SyncInterval: cfg.SyncInterval,
})

handler := cachingMiddleware(mux)
handler = readinessMiddleware(syncWorker, handler)
handler = middleware.Logging(logger)(handler)
handler = otelhttp.NewMiddleware("peeringdb-plus")(handler)
handler = middleware.CORS(middleware.CORSInput{AllowedOrigins: cfg.CORSOrigins})(handler)
handler = middleware.Recovery(logger)(handler)
```

Note: Caching wraps the mux directly, so it is the innermost middleware (closest to the handler). Readiness is outside caching, so 503 responses before first sync do not get cache headers.

### Benchmark: Search
```go
func BenchmarkSearch_Cloud(b *testing.B) {
    client := testutil.SetupClient(b)
    ctx := context.Background()
    seedBenchData(b, client) // Creates 100+ entities per type
    svc := NewSearchService(client)

    b.ResetTimer()
    for b.Loop() {
        results, err := svc.Search(ctx, "Cloud")
        if err != nil {
            b.Fatal(err)
        }
        _ = results // keep alive
    }
}
```

### Benchmark: Field Projection
```go
func BenchmarkApplyFieldProjection(b *testing.B) {
    // Pre-build a realistic data slice (100 items).
    data := make([]any, 100)
    for i := range data {
        data[i] = peeringdb.Network{
            ID: i, Name: fmt.Sprintf("Network %d", i),
            ASN: 13335 + i, /* ... other fields ... */
        }
    }
    fields := []string{"name", "asn", "website"}

    b.ResetTimer()
    for b.Loop() {
        result := applyFieldProjection(data, fields)
        _ = result
    }
}
```

### Benchmark: Generic List with Conversion (Mock Callbacks)
```go
func BenchmarkListEntities(b *testing.B) {
    // Use mock callbacks (same pattern as generic_test.go).
    mockData := makeMockData(100)
    params := ListParams[mockEntity, mockProto]{
        EntityName:   "benchmark",
        PageSize:     50,
        ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
        Query: func(_ context.Context, _ []func(*sql.Selector), limit, offset int) ([]*mockEntity, error) {
            end := offset + limit
            if end > len(mockData) { end = len(mockData) }
            if offset >= len(mockData) { return nil, nil }
            return mockData[offset:end], nil
        },
        Convert: func(e *mockEntity) *mockProto {
            return &mockProto{Id: int64(e.ID), Name: e.Name}
        },
    }
    ctx := context.Background()

    b.ResetTimer()
    for b.Loop() {
        items, _, err := ListEntities(ctx, params)
        if err != nil { b.Fatal(err) }
        _ = items
    }
}
```

### Benchmark: Sync Upsert
```go
func BenchmarkUpsertOrganizations(b *testing.B) {
    client := testutil.SetupClient(b)
    ctx := context.Background()
    orgs := generateTestOrganizations(200) // realistic test data

    b.ResetTimer()
    for b.Loop() {
        tx, err := client.Tx(ctx)
        if err != nil { b.Fatal(err) }
        _, err = upsertOrganizations(ctx, tx, orgs)
        if err != nil {
            tx.Rollback()
            b.Fatal(err)
        }
        tx.Commit()
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `for i := 0; i < b.N; i++` | `for b.Loop()` | Go 1.24 introduced, Go 1.26 fixed inlining | Use `b.Loop()` exclusively. Prevents compiler from optimizing away benchmark code. |
| Content-hash ETags | Timestamp-based ETags | N/A (project design) | Much simpler middleware, no response buffering. |

**Deprecated/outdated:**
- `b.N` loop style: Still works but `b.Loop()` is preferred in Go 1.26 due to inlining fix.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go 1.26 stdlib `testing` |
| Config file | None needed (standard `go test`) |
| Quick run command | `go test -bench=. -benchtime=1s -count=1 ./internal/middleware/ ./internal/web/ ./internal/pdbcompat/ ./internal/grpcserver/ ./internal/sync/` |
| Full suite command | `go test -race ./... && go test -bench=. -benchtime=3s -count=6 ./internal/...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PERF-02 | GET responses include Cache-Control + ETag headers | unit | `go test ./internal/middleware/ -run TestCaching -v` | No -- Wave 0 |
| PERF-02 | If-None-Match returns 304 | unit | `go test ./internal/middleware/ -run TestCaching -v` | No -- Wave 0 |
| PERF-02 | POST/mutation requests have no caching headers | unit | `go test ./internal/middleware/ -run TestCaching -v` | No -- Wave 0 |
| PERF-02 | No caching before first sync | unit | `go test ./internal/middleware/ -run TestCaching -v` | No -- Wave 0 |
| PERF-04 | Search benchmark runs | benchmark | `go test -bench=BenchmarkSearch ./internal/web/ -benchtime=1s` | No -- Wave 0 |
| PERF-04 | Field projection benchmark runs | benchmark | `go test -bench=BenchmarkApplyFieldProjection ./internal/pdbcompat/ -benchtime=1s` | No -- Wave 0 |
| PERF-04 | List entities benchmark runs | benchmark | `go test -bench=BenchmarkListEntities ./internal/grpcserver/ -benchtime=1s` | No -- Wave 0 |
| PERF-04 | Upsert benchmark runs | benchmark | `go test -bench=BenchmarkUpsert ./internal/sync/ -benchtime=1s` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race ./internal/middleware/... ./internal/web/... ./internal/pdbcompat/... ./internal/grpcserver/... ./internal/sync/...`
- **Per wave merge:** `go test -race ./... && go test -bench=. -benchtime=1s -count=3 ./internal/...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/middleware/caching_test.go` -- covers PERF-02 (caching middleware unit tests)
- [ ] `internal/web/search_bench_test.go` -- covers PERF-04 (search benchmark)
- [ ] `internal/pdbcompat/projection_bench_test.go` -- covers PERF-04 (field projection benchmark)
- [ ] `internal/grpcserver/list_bench_test.go` -- covers PERF-04 (generic list benchmark)
- [ ] `internal/sync/upsert_bench_test.go` -- covers PERF-04 (upsert benchmark)

## Open Questions

1. **SyncTimeFn: DB query vs cached atomic value**
   - What we know: `GetLastSuccessfulSyncTime` is a single-row indexed query on local SQLite. Microsecond-scale latency.
   - What's unclear: Whether the overhead matters at scale (thousands of concurrent requests on edge nodes).
   - Recommendation: Start with direct DB query. If benchmarks show it matters, refactor to atomic.Value cache updated by sync worker. Simple first, optimize if needed.

2. **If-None-Match with multiple values**
   - What we know: RFC 7232 allows comma-separated ETags in If-None-Match. Browsers typically send single values for GET.
   - What's unclear: Whether any PeeringDB API clients send multiple ETags.
   - Recommendation: Implement simple single-value comparison. If needed later, add comma-split parsing. The wildcard `*` case should be handled.

## Project Constraints (from CLAUDE.md)

Actionable directives that apply to this phase:

- **CS-5/CS-6 (MUST):** CachingInput struct declared before Caching function (>2 args pattern)
- **ERR-1 (MUST):** Wrap errors with `%w` and context
- **API-1 (MUST):** Document exported items (Caching, CachingInput)
- **T-1 (MUST):** Table-driven tests for caching middleware
- **T-2 (MUST):** Run `-race` in CI; use `t.Cleanup` for teardown
- **T-3 (SHOULD):** Mark safe tests with `t.Parallel()`
- **OBS-1 (MUST):** Structured logging with slog (if the middleware logs anything)
- **PERF-1 (MUST):** Measure before optimizing -- benchmarks establish baselines
- **Middleware convention:** Response writer wrappers MUST implement `http.Flusher` and `Unwrap()`. However, the caching middleware does NOT need a response writer wrapper -- it sets headers before delegation and short-circuits on 304.
- **Go module:** `GONOSUMCHECK=* GONOSUMDB=*` may be needed for `go mod tidy`. `TMPDIR=/tmp/claude-1000` required for go commands in sandbox mode.

## Sources

### Primary (HIGH confidence)
- Project codebase: `internal/middleware/` -- existing middleware patterns (CORS, Logging, Recovery)
- Project codebase: `internal/sync/status.go` -- `GetLastSuccessfulSyncTime` function (line 122-135)
- Project codebase: `internal/sync/worker.go` -- Worker struct, sync timestamp tracking
- Project codebase: `cmd/peeringdb-plus/main.go` -- middleware chain order (line 355-361)
- Project codebase: `internal/testutil/testutil.go` -- SetupClient and SetupClientWithDB for benchmarks
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26) -- `b.Loop()` inlining fix confirmed

### Secondary (MEDIUM confidence)
- [ETag and HTTP Caching - rednafi.com](https://rednafi.com/misc/etag-and-http-caching/) -- ETag middleware implementation patterns
- [Understanding HTTP 304 with Go - furkanbaytekin.dev](https://www.furkanbaytekin.dev/blogs/software/understanding-http-304-etag-cache-control-and-last-modified-with-go) -- Cache-Control + ETag + 304 patterns
- [Go 1.26 Release Notes (Phoronix)](https://www.phoronix.com/news/Go-1.26-Released) -- Green Tea GC, benchmark improvements

### Tertiary (LOW confidence)
- None -- all findings verified against primary sources.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all stdlib, no new dependencies
- Architecture: HIGH -- middleware pattern matches existing project conventions exactly
- Pitfalls: HIGH -- well-understood HTTP caching semantics, documented in RFC 7232

**Research date:** 2026-03-26
**Valid until:** 2026-04-26 (stable patterns, no fast-moving dependencies)
