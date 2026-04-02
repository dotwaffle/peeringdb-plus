# Phase 48: Response Hardening & Internal Quality - Context

**Gathered:** 2026-04-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Add security headers, response compression, metrics caching, and type-safe error classification to the response pipeline. All changes are middleware additions or internal code quality improvements — no new user-facing features.

</domain>

<decisions>
## Implementation Decisions

### Content-Security-Policy
- Deploy as Content-Security-Policy-Report-Only (not enforcing) — first deployment, need to observe violations
- Per-route policies: tighter CSP on /ui/* routes, more permissive on /graphql (GraphiQL needs unsafe-eval)
- /ui/* CSP: default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; img-src 'self' data: https://*.basemaps.cartocdn.com; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net
- /graphql CSP: default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data:; connect-src 'self'
- Implemented as middleware in internal/middleware/ — separate CSP middleware function
- 'unsafe-inline' required for Tailwind browser runtime and inline scripts in layout.templ

### Gzip Compression
- Gzip only (no zstd) — universal browser support, simple
- Use klauspost/compress/gzhttp — already a transitive dependency, just promote to direct
- Place AFTER caching middleware in the chain: Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> Caching -> Gzip -> mux
- MUST exclude gRPC content types: application/grpc*, application/connect+proto — ConnectRPC handles its own compression
- klauspost/gzhttp handles ETag suffixing automatically (appends --gzip to ETags from caching middleware)
- Minimum size threshold: use gzhttp default (no override needed)

### Metrics COUNT Caching
- Sync worker computes entity counts after each successful sync, stores in an atomic map
- OTel observable gauge callback reads from the cached map instead of running 13 live COUNT queries
- Cache structure: sync.Map or atomic pointer to map[string]int64 in the sync worker
- Sync worker calls a callback function provided by main.go to update the cache
- On first startup (before first sync), counts are 0 — acceptable, will populate after first sync

### GraphQL Sentinel Errors
- Replace strings.Contains error matching in internal/graphql/handler.go classifyError() with:
  - ent.IsNotFound(err) -> "NOT_FOUND"
  - ent.IsValidationError(err) -> "VALIDATION_ERROR"
  - ent.IsConstraintError(err) -> "CONSTRAINT_ERROR"
  - Check for specific error types using errors.Is/errors.As
- Remove the strings.Contains("limit must") and strings.Contains("offset must") patterns — these should be caught by validation error check
- Keep "INTERNAL_ERROR" as the default fallback
- This fixes GO-ERR-2 violation (no string matching for error control flow)

### Claude's Discretion
- Exact CSP report-uri or report-to configuration (can omit for now since Report-Only)
- Whether to add a CSP header to /rest/v1/ and /api/ endpoints (JSON-only, no browser rendering)
- gzhttp.NewWrapper options beyond content-type exclusion
- Whether to log cache miss events for metrics (probably not — too noisy)

</decisions>

<code_context>
## Existing Code Insights

### Key Files to Modify
- `internal/middleware/` — new csp.go for CSP middleware, new compression.go for gzip
- `internal/middleware/caching.go` — existing ETag/Cache-Control middleware (gzip placed after this)
- `cmd/peeringdb-plus/main.go:355-369` — middleware chain assembly (add CSP and gzip)
- `cmd/peeringdb-plus/main.go:193-199` — /graphql handler (CSP needs different policy here)
- `internal/otel/metrics.go:180-194` — observable gauge callback (replace with cache read)
- `internal/sync/worker.go` — add count computation after successful sync commit
- `internal/graphql/handler.go:57-72` — classifyError function (replace string matching)

### Established Patterns
- Middleware functions in internal/middleware/ follow func(http.Handler) http.Handler pattern
- CORS middleware uses rs/cors library; CSP will be simpler (just set header)
- Caching middleware generates ETags from sync time — gzip must not break this
- Sync worker already has hooks for post-sync operations (cursor updates, etc.)

### Integration Points
- CSP middleware must come before the mux in the chain (applies to all responses)
- Gzip must come after caching (ETags computed on uncompressed content)
- Gzip must NOT wrap gRPC paths — content-type based exclusion in gzhttp handles this
- Metrics cache must be thread-safe (sync worker writes, OTel scrape reads concurrently)

</code_context>

<specifics>
## Specific Ideas

- CSP as a per-route middleware: wrap /ui/* and /graphql handlers with different policies rather than a single global middleware
- Metrics cache as a simple atomic.Pointer[map[string]int64] — swap pointer after each sync
- Use ent's built-in error type checking functions rather than reimplementing with errors.As

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
