# Architecture Patterns

**Domain:** Hardening & tech debt integration for existing Go HTTP server
**Researched:** 2026-04-02

## Existing Architecture Summary

The application is a mature Go 1.26 HTTP server with five API surfaces (Web UI, GraphQL, REST, PeeringDB compat, ConnectRPC) served from a single `http.ServeMux`. The middleware chain runs outermost-first:

```
Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> Caching -> mux
```

Key integration points for hardening:
- **Server config**: `cmd/peeringdb-plus/main.go` lines 376-381 -- `http.Server` struct with only `Addr`, `Handler`, `Protocols`
- **Database**: `internal/database/database.go` -- bare `sql.Open` with no pool config
- **Config**: `internal/config/config.go` -- env var parsing, `validate()` method, 12 existing fields
- **Middleware**: `internal/middleware/` -- 4 files (CORS, logging, recovery, caching)
- **Templates**: `internal/web/templates/layout.templ` -- CDN script/style tags, some with SRI, some without
- **Metrics**: `internal/otel/metrics.go` -- 13 COUNT queries per scrape in `InitObjectCountGauges`
- **GraphQL**: `internal/graphql/handler.go` -- string-matching error classification
- **Detail handlers**: `internal/web/detail.go` -- 1422 LOC, 30 functions, repeated patterns
- **Upsert functions**: `internal/sync/upsert.go` -- 613 LOC, 13 near-identical functions
- **CI**: `.github/workflows/ci.yml` -- 4 jobs (lint, test, build, govulncheck)
- **Docker**: `Dockerfile` + `Dockerfile.prod` -- Chainguard-based multi-stage, no HEALTHCHECK
- **Linters**: `.golangci.yml` -- standard + gocritic, misspell, nolintlint, revive

## Integration Analysis: Each Hardening Item

### 1. HTTP Server Timeouts

**What changes:** `cmd/peeringdb-plus/main.go` -- add fields to `http.Server` struct (lines 376-381).

**Current state:**
```go
server := &http.Server{
    Addr:      cfg.ListenAddr,
    Handler:   handler,
    Protocols: &protocols,
}
```

**Target state:**
```go
server := &http.Server{
    Addr:              cfg.ListenAddr,
    Handler:           handler,
    Protocols:         &protocols,
    ReadHeaderTimeout: cfg.ReadHeaderTimeout,
    IdleTimeout:       cfg.IdleTimeout,
}
```

**Integration type:** Modify existing (3 lines in main.go + 2 config fields).

**Why NOT ReadTimeout/WriteTimeout:** This server runs ConnectRPC streaming RPCs (StreamNetworks etc.) with a 60s configurable `StreamTimeout`. Setting a global `WriteTimeout` would kill long-running streams. `ReadHeaderTimeout` protects against Slowloris without affecting streaming. `IdleTimeout` protects against connection exhaustion from idle keep-alives.

**New config fields:**
- `PDBPLUS_READ_HEADER_TIMEOUT` (default: 5s)
- `PDBPLUS_IDLE_TIMEOUT` (default: 120s)

**Dependencies:** Config validation (item 11). No other dependencies.

**Risk:** LOW -- additive fields on existing struct.

### 2. SQLite Connection Pool Limits

**What changes:** `internal/database/database.go` -- add `SetMaxOpenConns`/`SetMaxIdleConns` after `sql.Open`.

**Current state:** No pool configuration. Go's `database/sql` defaults to unlimited open connections, 2 idle connections.

**Target state:**
```go
db.SetMaxOpenConns(maxOpen)
db.SetMaxIdleConns(maxIdle)
db.SetConnMaxIdleTime(connMaxIdleTime)
```

**Integration type:** Modify existing (3 lines in database.go).

**Rationale:** SQLite in WAL mode allows concurrent readers but only one writer. Unbounded connections waste file descriptors and can hit SQLite's internal limits. For this read-heavy workload:
- `MaxOpenConns`: 10 (allows parallel reads across API surfaces)
- `MaxIdleConns`: 5 (keep warm connections for bursty traffic)
- `ConnMaxIdleTime`: 5m (release stale connections)

**Approach:** Hardcode sensible defaults in `database.Open` -- these are infrastructure constants, not user-tunable. The app is read-only with one writer (sync), so hardcoded defaults are appropriate.

**Dependencies:** None. Pure additive.

**Risk:** LOW -- but test under load to verify 10 is sufficient for parallel GraphQL + REST + gRPC + web.

### 3. Request Body Size Limits

**What changes:** New middleware in `internal/middleware/bodylimit.go`.

**Current state:** No body size limits. Only POST endpoint is `/sync` (fire-and-forget, no body parsing) and `/graphql` (JSON body). REST/compat/web are all GET.

**Target state:** Middleware wraps request bodies with `http.MaxBytesReader` for POST/PUT/PATCH requests.

**Integration type:** New file + modify middleware chain in main.go.

**Middleware position:** After Recovery, before CORS (or after CORS -- order doesn't matter since CORS only adds headers).

```
Recovery -> CORS -> OTel HTTP -> BodyLimit -> Logging -> Readiness -> Caching -> mux
```

**Default limit:** 1MB. GraphQL queries are typically < 10KB. The `/sync` endpoint doesn't read the body at all.

**Dependencies:** None. Pure additive.

**Risk:** LOW -- but must NOT apply to ConnectRPC streaming (client-streaming not used, so this is moot for current API).

### 4. Compression Middleware

**What changes:** New middleware in `internal/middleware/compress.go` or use `klauspost/compress/gzhttp`.

**Current state:** No response compression. All responses served uncompressed.

**Target state:** Gzip/zstd compression for responses based on `Accept-Encoding`.

**Integration type:** New file (or new dependency) + modify middleware chain in main.go.

**Library choice:** `klauspost/compress/gzhttp` -- battle-tested, wraps `http.Handler`, handles ETag interaction correctly (critical since caching middleware sets ETags). The `SuffixETag()` option prevents ETag collisions on compressed responses. Alternatively, write a minimal gzip middleware using stdlib `compress/gzip` (fewer features but zero new deps).

**Recommendation:** Use `klauspost/compress/gzhttp` because it handles the ETag suffix correctly, which interacts with the existing caching middleware's ETag generation. Rolling your own would require reimplementing this.

**Middleware position:** Between Caching and mux (compress the response after cache headers are set):

```
Recovery -> CORS -> OTel HTTP -> BodyLimit -> Logging -> Readiness -> Caching -> Compress -> mux
```

**Interaction with gRPC:** ConnectRPC handles its own compression via the Connect protocol. The compression middleware should skip paths with `application/grpc` content type or `application/connect+proto`. The `gzhttp` middleware's `ContentTypes` option handles this.

**Dependencies:** Caching middleware (ETag interaction). Should be implemented after or alongside caching review.

**Risk:** MEDIUM -- ETag interaction with caching middleware needs careful testing. gRPC/ConnectRPC content-type exclusion is critical.

### 5. CSP Header and Subresource Integrity

**What changes:** Two locations: new middleware for CSP header + template modifications for SRI.

**CSP middleware:** New file `internal/middleware/csp.go` or inline in existing middleware. Sets `Content-Security-Policy` header on HTML responses only.

**Current CDN assets (from layout.templ):**
| Asset | SRI Present? |
|-------|-------------|
| `@tailwindcss/browser@4` (cdn.jsdelivr.net) | NO |
| `flag-icons@7.5.0` (cdn.jsdelivr.net) | NO |
| `leaflet@1.9.4` CSS (unpkg.com) | YES |
| `leaflet@1.9.4` JS (unpkg.com) | YES |
| `leaflet.markercluster@1.5.3` CSS x2 (unpkg.com) | NO |
| `leaflet.markercluster@1.5.3` JS (unpkg.com) | NO |
| `htmx.min.js` (self-hosted /static/) | N/A (self) |

**GraphiQL playground CDN assets (graphql/handler.go):**
| Asset | SRI Present? |
|-------|-------------|
| `react@18.2.0` (cdn.jsdelivr.net) | YES |
| `react-dom@18.2.0` (cdn.jsdelivr.net) | YES |
| `graphiql@3.7.0` CSS (cdn.jsdelivr.net) | YES |
| `graphiql@3.7.0` JS (cdn.jsdelivr.net) | YES |

**Syncing page (syncing.templ):**
| Asset | SRI Present? |
|-------|-------------|
| `@tailwindcss/browser@4` (cdn.jsdelivr.net) | NO |

**SRI additions needed:** 4 assets in layout.templ + 1 in syncing.templ. Generate hashes with `openssl dgst -sha256 -binary <file> | openssl base64 -A`.

**CSP policy:**
```
default-src 'self';
script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com;
style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com;
img-src 'self' data: https://*.basemaps.cartocdn.com;
connect-src 'self';
font-src 'self' https://cdn.jsdelivr.net;
```

Note: `'unsafe-inline'` is required because:
- Tailwind CSS browser runtime uses inline styles
- Layout template has inline `<script>` blocks (dark mode, Leaflet config)
- templ components may generate inline event handlers

**Integration type:** New middleware file + modify 5 template lines for SRI hashes.

**Middleware position:** Only applies to HTML responses. Could be a dedicated middleware or added to the dispatch in web handler. Middleware is cleaner -- set on all responses, browsers ignore it for non-HTML.

**Dependencies:** Must catalog all CDN assets first (done above). Template changes are independent of middleware.

**Risk:** MEDIUM -- CSP violations can break functionality silently. Deploy with `Content-Security-Policy-Report-Only` first, then switch to enforcing after validation.

### 6. Input Validation (ASN Range, Width Parameter)

**What changes:** Modify existing handler code in `internal/web/handler.go` and `internal/web/detail.go`.

**Current state:** ASN parsing uses `strconv.Atoi()` with no range check. Width parameter `?w=N` parsed without bounds.

**Target validation:**
- ASN: Must be 1-4294967295 (32-bit unsigned, per RFC 6793)
- Width `?w=N`: Must be 40-500 (reasonable terminal width range)
- ID parameters: Must be positive integers

**Integration type:** Modify existing functions. Add validation helper functions (new file `internal/web/validate.go` or inline).

**Where:**
- `handleNetworkDetail` (line 45-49): Add ASN range check after `strconv.Atoi`
- `handleFragment` (line 889): Add positive ID check after `strconv.Atoi`
- Width parameter parsing in `termrender` package: Add bounds clamping
- All `handleXXXDetail` functions (6 total): Add positive ID check

**Dependencies:** None. Pure logic change.

**Risk:** LOW -- straightforward validation. Edge case: ASN 0 and ASN 4294967295 (reserved ranges) may or may not exist in PeeringDB.

### 7. GraphQL Error Classification via Sentinel Errors

**What changes:** Modify `internal/graphql/handler.go` -- replace string matching with `errors.Is`/`errors.As`.

**Current state:**
```go
func classifyError(err error) string {
    msg := err.Error()
    switch {
    case strings.Contains(msg, "not found"):
        return "NOT_FOUND"
    case strings.Contains(msg, "validation"):
        return "VALIDATION_ERROR"
    ...
    }
}
```

**Target state:** Define sentinel errors, use `errors.Is`/`errors.As` per GO-ERR-2.

**Integration type:** Modify existing file + potentially new sentinel error definitions in a shared package.

**Dependencies:** None. Self-contained refactor.

**Risk:** LOW -- but must verify that ent errors wrap properly with `errors.Is` (ent's `IsNotFound` already does this).

### 8. Metrics COUNT Query Caching

**What changes:** Modify `internal/otel/metrics.go` `InitObjectCountGauges` function.

**Current state:** 13 `COUNT(*)` queries execute on every OTel scrape (typically every 15-60s). Each is a full table scan.

**Target state:** Cache counts with a TTL (e.g., 5 minutes). Since data only changes on sync (hourly), counts are stable between syncs.

**Approach options:**
1. **Sync-time cache:** Compute counts after each sync, store in memory. Observable gauge reads cached values. Zero scrape-time queries.
2. **TTL cache:** Cache counts for N minutes, refresh on miss. Simple but still hits DB periodically.
3. **Sync-triggered invalidation:** Cache counts, invalidate when sync completes. Re-query lazily on next scrape.

**Recommendation:** Option 1 (sync-time cache). The sync worker already runs after data changes. Add a `RefreshCounts()` call at sync completion. The observable gauge callback reads from a `sync.Map` or atomic struct. Zero DB queries during normal scrape cycles.

**Integration type:** Modify `InitObjectCountGauges` to accept a cache instead of querying directly. Add count refresh to sync worker completion path.

**Files changed:**
- `internal/otel/metrics.go` -- change observable gauge to read from cache
- `internal/sync/worker.go` -- add count refresh after successful sync
- New: cached count storage (could be in otel package or a shared struct)

**Dependencies:** Requires understanding sync worker completion hook. No hard blockers.

**Risk:** LOW -- worst case is stale counts for one sync cycle, which is fine for metrics.

### 9. CORS Preflight Caching

**What changes:** Already partially done. Check `internal/middleware/cors.go`.

**Current state:** `MaxAge: 86400` is already set in CORS options. This tells browsers to cache preflight responses for 24 hours.

**Assessment:** This may already be sufficient. Verify that the `MaxAge` value is being sent as `Access-Control-Max-Age` header in preflight responses. The `rs/cors` library handles this.

**Integration type:** Verify existing (may be a no-op).

**Risk:** NONE.

### 10. Test Coverage: GraphQL Handler and Database Package

**What changes:** New test files.

**Current state:**
- `internal/graphql/` -- NO test files
- `internal/database/` -- NO test files
- `internal/config/` -- has `config_test.go` (comprehensive)

**GraphQL handler tests (`internal/graphql/handler_test.go`):**
- Test `NewHandler` returns non-nil handler
- Test complexity limit enforcement (query exceeding 500 complexity)
- Test depth limit enforcement (query exceeding 15 depth)
- Test error presenter populates path and extensions
- Test `classifyError` function with various error types
- Test playground handler returns HTML with correct template data

**Database package tests (`internal/database/database_test.go`):**
- Test `Open` with valid path returns working client and db
- Test `Open` with invalid path returns error
- Test WAL mode is enabled (query `PRAGMA journal_mode`)
- Test foreign keys are enabled (query `PRAGMA foreign_keys`)
- Test busy timeout is set (query `PRAGMA busy_timeout`)

**Integration type:** New test files only. No production code changes.

**Dependencies:** GraphQL tests need the `graph` package schema. Database tests are self-contained (SQLite in-memory or temp dir).

**Risk:** LOW.

### 11. Config Validation Enhancements

**What changes:** Modify `internal/config/config.go` `validate()` method.

**Current state:** Validates only `DBPath` non-empty, `SyncInterval > 0`, and `OTelSampleRate` range.

**New validations:**
- `ListenAddr` format (must parse as host:port or :port)
- `DrainTimeout > 0`
- `SyncStaleThreshold > SyncInterval` (stale threshold should exceed sync interval)
- `StreamTimeout > 0`
- New timeout fields: `ReadHeaderTimeout > 0`, `IdleTimeout > 0`

**Integration type:** Modify existing `validate()` method + add new config fields (if adding timeout configs).

**Dependencies:** New config fields from item 1 (server timeouts) should be added first or simultaneously.

**Risk:** LOW -- but breaking change if existing deployments have invalid-but-working configs. Use sensible defaults.

### 12. Additional Linters (exhaustive, contextcheck, gosec)

**What changes:** Modify `.golangci.yml`.

**Current enabled linters:** `standard` preset + gocritic, misspell, nolintlint, revive.

**Target additions:**
- `exhaustive` -- checks exhaustiveness of enum switch statements. Catches missing cases in type switches (relevant for detail.go dispatch, termrender dispatch).
- `contextcheck` -- detects nested contexts in loops. Catches context misuse.
- `gosec` -- security linter (already in exclusions for test files, but not in enable list; the `standard` preset may include it).

**Current gosec status:** Listed in exclusions (`path: _test\.go, linters: [gosec]`) suggesting it's already active via the `standard` preset. Verify.

**Integration type:** Modify `.golangci.yml` (add to enable list) + fix any new findings.

**Potential findings to expect:**
- `exhaustive`: The `handleFragment` switch in detail.go has default cases, so exhaustive may be satisfied. Terminal render dispatch type-switch may trigger.
- `contextcheck`: Verify sync worker goroutine context usage.
- `gosec`: May flag the missing `ReadHeaderTimeout` (G112 rule) which is addressed by item 1.

**Dependencies:** Should be done AFTER refactoring (items 15-16) to avoid fixing lint issues in code that's about to be restructured.

**Risk:** LOW-MEDIUM -- may surface issues that need fixing across the codebase.

### 13. Docker Build in CI

**What changes:** Modify `.github/workflows/ci.yml` -- add Docker build job.

**Current CI jobs:** lint, test, build, govulncheck.

**New job:**
```yaml
docker:
  name: Docker Build
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v6
    - name: Build Docker image
      run: docker build -f Dockerfile -t peeringdb-plus:ci .
    - name: Build prod Docker image
      run: docker build -f Dockerfile.prod -t peeringdb-plus-prod:ci .
```

**Integration type:** Additive CI job. No production code changes.

**Dependencies:** Dockerfile changes (item 14) should be done first so CI validates the updated Dockerfiles.

**Risk:** LOW -- but adds ~2-3 minutes to CI.

### 14. Dockerfile HEALTHCHECK

**What changes:** Modify `Dockerfile` and `Dockerfile.prod`.

**Current state:** No HEALTHCHECK instruction. Fly.io uses its own health checks (`/healthz`, `/readyz`) configured in `fly.toml`, but Docker-level health checks are useful for local development and non-Fly deployments.

**Addition to both Dockerfiles:**
```dockerfile
HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD ["/usr/local/bin/peeringdb-plus", "--healthcheck"] || exit 1
```

**Problem:** The app binary doesn't have a `--healthcheck` flag. Options:
1. Add a `--healthcheck` mode that GETs `http://localhost:8080/healthz` and exits 0/1
2. Use `wget` or `curl` -- neither is in Chainguard images
3. Install a minimal HTTP client in the image
4. Use a Go-based healthcheck binary

**Recommendation:** Option 1 is cleanest -- add a `--healthcheck` flag to the existing binary that performs a simple HTTP GET to `localhost:$PDBPLUS_PORT/healthz`. This keeps the image minimal and avoids adding tools to the Chainguard base.

**Integration type:** Modify both Dockerfiles + add healthcheck mode to main.go (or separate small cmd).

**Dependencies:** None, but pairs well with config validation (uses same listen addr/port).

**Risk:** LOW.

### 15. Refactor detail.go

**What changes:** Split `internal/web/detail.go` (1422 LOC, 30 functions) into smaller files.

**Current structure:**
- 6 `handleXXXDetail` functions (network, IX, facility, org, campus, carrier)
- 6 `queryXXX` functions (corresponding queries)
- 1 `handleFragment` dispatcher
- 17 `handleXXXFragment` functions (lazy-loaded child sections)

**Recommended split:**
```
internal/web/
  detail_network.go    -- handleNetworkDetail, queryNetwork
  detail_ix.go         -- handleIXDetail, queryIX
  detail_facility.go   -- handleFacilityDetail, queryFacility
  detail_org.go        -- handleOrgDetail, queryOrg
  detail_campus.go     -- handleCampusDetail, queryCampus
  detail_carrier.go    -- handleCarrierDetail, queryCarrier
  fragment.go          -- handleFragment dispatcher + all fragment handlers
  freshness.go         -- getFreshness helper
```

**Integration type:** File reorganization only. No logic changes. All functions are methods on `*Handler`, same package.

**Pattern to extract:** Each detail type follows identical structure:
1. Parse ID/ASN from URL
2. Query with eager-loaded relations
3. Map to template struct
4. Render via `renderPage`

The fragment handlers also follow a uniform pattern:
1. Query child entities with parent filter
2. Map to template row structs
3. Render template component

**Dependencies:** Should be done BEFORE adding linters (item 12) to avoid lint churn on code being moved.

**Risk:** LOW -- pure file split, no logic changes. Git will track moves correctly with `git diff -M`.

### 16. Refactor sync/upsert.go Duplication

**What changes:** Reduce 13 near-identical `upsertXXX` functions in `internal/sync/upsert.go` (613 LOC).

**Current pattern (repeated 13 times):**
```go
func upsertXXX(ctx context.Context, tx *ent.Tx, items []peeringdb.XXX) ([]int, error) {
    builders := make([]*ent.XXXCreate, 0, len(items))
    ids := make([]int, 0, len(items))
    for _, item := range items {
        ids = append(ids, item.ID)
        b := tx.XXX.Create().SetID(item.ID).SetField1(item.Field1)...
        builders = append(builders, b)
    }
    for i := 0; i < len(builders); i += batchSize {
        end := min(i+batchSize, len(builders))
        err := tx.XXX.CreateBulk(builders[i:end]...).
            OnConflictColumns(xxx.FieldID).UpdateNewValues().Exec(ctx)
        if err != nil {
            return nil, fmt.Errorf("upsert xxx batch %d: %w", i/batchSize, err)
        }
    }
    return ids, nil
}
```

**Refactoring approach:** Extract the batched upsert loop into a generic helper:

```go
func batchUpsert[B any](ctx context.Context, builders []B, execFn func([]B) error, typeName string) error {
    for i := 0; i < len(builders); i += batchSize {
        end := min(i+batchSize, len(builders))
        if err := execFn(builders[i:end]); err != nil {
            return fmt.Errorf("upsert %s batch %d: %w", typeName, i/batchSize, err)
        }
    }
    return nil
}
```

Each `upsertXXX` keeps its field-mapping logic (which is inherently type-specific) but delegates the batching loop. This eliminates ~13 copies of the batch loop (~8 lines each = ~100 lines saved).

**Important constraint:** The field-mapping portion CANNOT be generified because each PeeringDB type has different fields and different ent builder types. Only the batch loop and error wrapping can be extracted.

**Integration type:** Modify existing file. Extract helper, simplify each function.

**Dependencies:** Should be done BEFORE adding linters to avoid lint churn.

**Risk:** LOW -- the batch loop is identical across all 13 functions. The refactoring is mechanical.

## Component Boundaries

| Component | Responsibility | Changes For Hardening |
|-----------|---------------|----------------------|
| `cmd/peeringdb-plus/main.go` | Server setup, handler registration, middleware chain | Add server timeouts, body limit middleware, compression middleware, healthcheck mode |
| `internal/config/config.go` | Environment variable parsing and validation | New timeout fields, enhanced validate() |
| `internal/database/database.go` | SQLite connection setup | Add pool configuration |
| `internal/middleware/` | HTTP middleware chain | New: bodylimit.go, compress.go, csp.go |
| `internal/graphql/handler.go` | GraphQL handler factory | Sentinel error classification |
| `internal/otel/metrics.go` | OTel metric instruments | Cached count gauges |
| `internal/sync/worker.go` | Sync orchestration | Trigger count cache refresh |
| `internal/sync/upsert.go` | Bulk upsert logic | Extract batch helper |
| `internal/web/detail.go` | Detail page handlers | Split into 7+ files, add input validation |
| `internal/web/templates/layout.templ` | HTML layout | Add SRI hashes |
| `internal/web/templates/syncing.templ` | Syncing page | Add SRI hash |
| `.golangci.yml` | Linter config | Add exhaustive, contextcheck |
| `.github/workflows/ci.yml` | CI pipeline | Add Docker build job |
| `Dockerfile` / `Dockerfile.prod` | Container build | Add HEALTHCHECK |

## Data Flow Changes

### Metrics Count Caching (New Data Flow)

```
Before:
  OTel scrape -> InitObjectCountGauges callback -> 13 COUNT queries -> observe values

After:
  Sync completes -> RefreshCounts() -> 13 COUNT queries -> store in cache
  OTel scrape -> callback -> read cached values -> observe values (0 queries)
```

### Compression (New Data Flow)

```
Before:
  Request -> middleware chain -> handler -> raw response body -> client

After:
  Request -> middleware chain -> handler -> compress middleware checks Accept-Encoding
    -> if gzip/zstd: compress body, suffix ETag, set Content-Encoding
    -> if grpc/connect content-type: skip compression (protocol handles it)
    -> client
```

### Body Size Limit (New Data Flow)

```
Before:
  POST request -> handler reads body without limit

After:
  POST request -> MaxBytesReader wraps body (1MB limit) -> handler reads body
    -> if exceeds: http.StatusRequestEntityTooLarge
```

## Optimal Build Order

The items have the following dependency graph:

```
Independent (no deps):
  [2] SQLite pool limits
  [6] Input validation
  [7] GraphQL sentinel errors
  [9] CORS preflight (verify only)

Config-dependent:
  [11] Config validation  <->  [1] Server timeouts (co-develop)

Caching-aware:
  [4] Compression middleware  -- interacts with caching ETags

Refactor-before-lint:
  [15] Refactor detail.go  \
  [16] Refactor upsert.go  /->  [12] Additional linters

Docker-related:
  [14] Dockerfile HEALTHCHECK  ->  [13] Docker build in CI

Template + middleware:
  [5] CSP + SRI  (template changes + new middleware)

Metrics:
  [8] Metrics count caching  (crosses otel + sync packages)

Tests:
  [10] Test coverage  -- after [7] GraphQL sentinel errors
```

### Recommended Phase Ordering

**Phase 1: Foundation (no dependencies, enables later phases)**
Items: [1] Server timeouts, [2] SQLite pool, [11] Config validation

Rationale: Server timeouts and pool limits are the highest-impact security items. Config validation supports the new timeout fields. All are small, self-contained changes to existing files.

**Phase 2: Request/Response Hardening**
Items: [3] Body size limits, [5] CSP + SRI, [6] Input validation

Rationale: These are the request-path security items. Body limits and CSP are new middleware. Input validation modifies existing handlers. All are independent of each other.

**Phase 3: Middleware Additions**
Items: [4] Compression, [9] CORS preflight verification

Rationale: Compression interacts with caching middleware ETags and must be tested carefully. CORS preflight is a verify-only task. Group these because compression is the trickiest middleware addition.

**Phase 4: Internal Quality**
Items: [7] GraphQL sentinel errors, [8] Metrics caching

Rationale: These are internal code quality improvements. Sentinel errors are a self-contained refactor. Metrics caching crosses package boundaries (otel + sync) but is well-defined.

**Phase 5: Refactoring**
Items: [15] Refactor detail.go, [16] Refactor upsert.go

Rationale: File reorganization is safest when done before adding new linters (which would create noise on old code). These are mechanical refactors with no logic changes.

**Phase 6: Test Coverage**
Items: [10] GraphQL + database tests

Rationale: Tests are best written after the code they test is stable. GraphQL sentinel errors (Phase 4) change the error classification, so tests should come after.

**Phase 7: CI and Linting**
Items: [12] Additional linters, [13] Docker build in CI, [14] Dockerfile HEALTHCHECK

Rationale: Linters go last because they may surface issues in refactored code. Docker changes are independent but grouping them with CI makes sense for a single "CI hardening" phase.

**Phase 8: Tech Debt Items**
Items: Grafana verification, /ui/about terminal stub, seed consolidation, CI coverage pipeline verification

Rationale: These are the deferred tech debt items listed in PROJECT.md. They are independent of hardening and can be done in any order.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Global WriteTimeout with Streaming RPCs
**What:** Setting `http.Server.WriteTimeout` globally.
**Why bad:** Kills ConnectRPC streaming RPCs (StreamNetworks etc.) after the timeout. The server currently has a per-stream `StreamTimeout` (configurable, default 60s) that handles this correctly at the application level.
**Instead:** Use `ReadHeaderTimeout` + `IdleTimeout` for server-level protection. Stream timeouts remain application-level.

### Anti-Pattern 2: Compression on gRPC Paths
**What:** Applying HTTP-level gzip compression to ConnectRPC/gRPC responses.
**Why bad:** gRPC has its own compression negotiation. Double-compressing wastes CPU and can break protocol framing.
**Instead:** Configure compression middleware to skip `application/grpc*` and `application/connect+proto` content types.

### Anti-Pattern 3: Enforcing CSP Before Testing
**What:** Deploying `Content-Security-Policy` header directly.
**Why bad:** Tailwind CSS browser runtime and inline scripts may trigger violations that break the UI.
**Instead:** Deploy with `Content-Security-Policy-Report-Only` first, monitor for violations, then switch to enforcing.

### Anti-Pattern 4: Refactoring and Adding Linters Simultaneously
**What:** Adding exhaustive/contextcheck linters in the same phase as refactoring detail.go/upsert.go.
**Why bad:** Linter findings in code that's about to be restructured creates wasted effort. Fix findings in old locations, then move code, then fix again.
**Instead:** Refactor first (Phase 5), then add linters (Phase 7).

### Anti-Pattern 5: Over-Parameterizing Infrastructure Constants
**What:** Making SQLite pool sizes configurable via env vars.
**Why bad:** Users don't know what good values are. Bad values (MaxOpenConns=1) would kill performance. These are implementation details, not user-facing config.
**Instead:** Hardcode sensible defaults in `database.Open`. Make configurable only if there's a demonstrated need.

## Scalability Considerations

| Concern | Current (5 machines) | At 20 machines | At 100 machines |
|---------|---------------------|----------------|-----------------|
| Metrics COUNT queries | 65 queries/scrape (13 types x 5 machines) | 260/scrape | 1300/scrape -- caching essential |
| SQLite connections | Unbounded, works by luck | May hit FD limits under load | Pool limits prevent resource exhaustion |
| Body size limit | No limit, low risk (read-only) | Same | Same -- attack surface doesn't scale with replicas |
| Compression | Bandwidth cost linear with machines | Meaningful bandwidth savings | Significant bandwidth reduction for REST/GraphQL JSON |

## Sources

- [Cloudflare: Go net/http timeouts](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) - Timeout field semantics
- [Go docs: Managing connections](https://go.dev/doc/database/manage-connections) - Connection pool configuration
- [klauspost/compress gzhttp](https://pkg.go.dev/github.com/klauspost/compress/gzhttp) - Compression middleware with ETag handling
- [MDN: Content Security Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP) - CSP header reference
- [OWASP: CSP Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html) - CSP deployment strategy
- [golangci-lint: Linters](https://golangci-lint.run/docs/linters/) - Linter documentation
- [Alex Edwards: Configuring sql.DB](https://www.alexedwards.net/blog/configuring-sqldb) - Pool tuning guidance
