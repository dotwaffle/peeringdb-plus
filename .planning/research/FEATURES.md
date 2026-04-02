# Feature Landscape

**Domain:** Go HTTP server hardening & operational improvements for an existing production application
**Researched:** 2026-04-02
**Milestone:** v1.12 Hardening & Tech Debt

## Table Stakes

Features expected in any production Go HTTP server. Missing = operational risk or known vulnerability.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| HTTP server timeouts (ReadHeaderTimeout, IdleTimeout) | Default `http.Server` has zero timeouts -- any client can hold connections indefinitely (slowloris attack vector) | Low | `ReadHeaderTimeout: 5s`, `IdleTimeout: 120s` -- two lines in main.go. Do NOT set `ReadTimeout` or `WriteTimeout` due to gRPC streaming (ConnectRPC server-streaming RPCs can run up to StreamTimeout=60s on HTTP/2 streams). ReadHeaderTimeout alone blocks the slowloris vector without affecting streaming. |
| SQLite connection pool limits | Default `database/sql` pool is unbounded open conns with 2 idle -- under sync bursts + concurrent reads, can exhaust file descriptors | Low | `db.SetMaxOpenConns(25)`, `db.SetMaxIdleConns(5)`, `db.SetConnMaxLifetime(30*time.Minute)` in `database.Open()`. Read-only app under WAL mode has no write contention concerns (sync runs on primary only). Bounding prevents fd exhaustion. |
| Request body size limits | POST /sync is the only write endpoint and has no body limit -- unbounded reads are a DoS vector even when body is ignored | Low | `http.MaxBytesReader` wrapping `r.Body` in the sync handler. 1MB is generous (sync body is ignored anyway; the handler only reads the sync token header and mode query param). |
| Input validation: ASN range | `strconv.Atoi` in `handleNetworkDetail` accepts any integer including negatives and >32-bit values; ASNs are 1--4294967295 (32-bit unsigned per RFC 6793) | Low | Bounds check after Atoi. Return 404 for out-of-range ASNs. Prevents confusing "not found" queries to the database with obviously invalid input. |
| Config validation expansion | `validate()` only checks DBPath, SyncInterval, SampleRate -- misses DrainTimeout<=0, StreamTimeout<=0, empty ListenAddr, negative thresholds | Low | Add bounds checks for remaining fields in existing `validate()` method. Fail-fast on invalid config per GO-CFG-1. |
| GraphQL error classification via sentinel errors | Current `classifyError` uses `strings.Contains(msg, "not found")` -- brittle string matching that violates GO-ERR-2 (MUST use errors.Is/errors.As) | Medium | Define sentinel errors (ErrNotFound, ErrValidation, ErrBadRequest) in the graphql package. Update resolver error returns to wrap sentinels. Rewrite classifier to use `errors.Is`. Test the new classifier with table-driven tests. |
| SRI attributes on remaining CDN assets | Leaflet CSS/JS and all GraphiQL assets have SRI; Tailwind Browser, flag-icons CSS, and both MarkerCluster CSS/JS files do not -- incomplete supply chain protection | Low | Compute sha256 hashes for 4 missing assets in layout.templ, add `integrity` + `crossorigin="anonymous"` attributes. Note: Tailwind Browser CDN `@tailwindcss/browser@4` may not support SRI since it dynamically loads resources -- verify and document if SRI is incompatible. |

## Differentiators

Features that improve operational quality beyond baseline. Not expected, but valued.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Gzip compression middleware | Reduces response bandwidth 60-80% for JSON/HTML responses; impactful for large GraphQL responses, PeeringDB compat API listing all 200K+ netixlan rows, and REST API responses | Medium | Use `klauspost/compress/gzhttp` -- the standard Go compression middleware (15K+ stars, used widely). Must NOT compress gRPC/ConnectRPC responses (they handle their own encoding via Content-Encoding). Apply selectively: wrap the mux but exclude `/peeringdb.v1.*`, `/grpc.*` path prefixes, or apply only to specific handlers. Check CVE-2025-61728 status before adoption. |
| Content-Security-Policy header | Prevents XSS injection on web UI pages; entrest already sets CSP for Scalar docs but the main web UI under /ui/ has none | Medium | Policy must allow: `cdn.jsdelivr.net` (Tailwind, flag-icons), `unpkg.com` (Leaflet, markercluster), `basemaps.cartocdn.com` and subdomains (map tiles via img-src), `'unsafe-inline'` (Tailwind Browser requires inline style evaluation), `'self'` for /static/ assets. Deploy as `Content-Security-Policy-Report-Only` first to catch violations before enforcing. Apply only to /ui/ routes (API endpoints serve JSON, not HTML). |
| Metrics COUNT query caching | `InitObjectCountGauges` runs 13 `SELECT COUNT(*)` queries on every OTel scrape interval (default 60s for Prometheus, configurable for OTLP) -- unnecessary SQLite read load that scales with scrape frequency | Medium | Cache counts in memory after each sync completion. Sync worker already signals completion (HasCompletedSync). Add a post-sync hook that queries all 13 counts once, stores in `sync.Map` or `atomic.Int64` values. Observable gauge callback reads from cache. Counts only change after sync, so staleness between syncs is expected and correct. |
| Input validation: width parameter bounds | Terminal `?w=N` parameter for column width adaptation has no bounds check -- extremely large values could cause excessive string padding/allocation | Low | Clamp to 40--500 range in width detection. Below 40 is unusable for any table layout; above 500 exceeds any reasonable terminal. |
| Docker build in CI | CI runs `go build ./...` but never exercises `docker build` -- Dockerfile syntax errors, missing COPY sources, or Chainguard base image issues only caught at deploy time | Low | Add `docker build --file Dockerfile --target build .` step to CI build job. Tests compilation in Docker context. Do NOT add Dockerfile.prod (requires LiteFS binary from flyio/litefs which may not be available in CI). |
| Additional linters (exhaustive, contextcheck, gosec) | Current config has gocritic, misspell, nolintlint, revive. Missing: exhaustive (enum switch completeness -- catches when new enum values are added to SyncMode etc.), contextcheck (detects context misuse in goroutines), gosec (security-focused static analysis) | Low | Add to `.golangci.yml` linters.enable. Run locally first: `golangci-lint run --enable exhaustive,contextcheck,gosec` to assess how many existing violations need fixing or annotating. gosec already has test exclusion rule in config. |
| Test coverage: GraphQL handler package | `internal/graphql/` has zero test files despite containing error classification logic, handler construction with complexity/depth limits, and playground rendering | Medium | Table-driven tests for `classifyError` (test all branches: nil, "not found", "validation", "limit must", default). Handler construction test (verify complexity and depth limits are set). Playground handler test (verify HTML output contains expected elements). Error classifier tests are particularly high-value since the function is being rewritten to use sentinel errors. |
| Test coverage: database package | `internal/database/` has zero test files despite constructing the DSN with pragmas and returning both ent client + raw DB | Low | Test that `Open` returns a working client/DB pair on a temp file. Verify WAL mode is active (PRAGMA journal_mode query). Verify foreign keys are enabled (PRAGMA foreign_keys query). Test error path with invalid path. |
| Dockerfile HEALTHCHECK instruction | Dev Dockerfile lacks HEALTHCHECK; container orchestrators (Docker Compose, Swarm, Kubernetes with exec probes) depend on it for liveness detection. Fly.io uses its own HTTP check (already configured in fly.toml) so Dockerfile.prod is fine. | Low | Chainguard images lack curl/wget. Options: (a) add a `--healthcheck` flag to the Go binary that does an HTTP GET to /healthz, or (b) use a simple Go-based healthcheck binary compiled in the build stage. Option (a) is cleaner. |
| Refactor detail.go | 1422 lines, 6 entity types each with handleXDetail + queryX + helper functions. Largest non-generated file in the codebase. | High | Extract per-entity query functions into separate files (detail_network.go, detail_ix.go, etc.) or use a shared query builder pattern. Each entity detail handler follows the same structure: parse ID, query with eager-load, build template data, render. Risk: touching 6 working handlers in a refactor-only change. |
| Refactor sync upsert duplication | 13 upsert functions in 613-line upsert.go, each following identical batchSize-loop + builder pattern with type-specific field mappings | Medium | Extract shared batching loop into a generic helper (Go 1.26 generics): `func batchUpsert[T any, B any](ctx, tx, items []T, batchSize int, builderFn func(T) B, execFn func([]B) error)`. BUT: ent's generated Create types share no interface, so the generic would need function parameters for builder creation and batch execution. Evaluate whether the abstraction actually clarifies or just adds indirection. |

## Anti-Features

Features to explicitly NOT build in this hardening milestone.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Rate limiting middleware | Read-only mirror behind Fly.io proxy which provides its own concurrency limits (soft_limit=10 per machine in fly.toml). Adding application-level rate limiting adds per-request overhead and state management for a public service with no user accounts. | Rely on Fly.io proxy concurrency limits and OTel metrics for traffic monitoring. Revisit if abuse patterns emerge in Grafana dashboards. |
| WriteTimeout / ReadTimeout on http.Server | WriteTimeout kills gRPC streaming connections (ConnectRPC server-streaming RPCs run up to 60s). ReadTimeout overlaps with body reading and interferes with h2c. Both are server-global and cannot be set per-handler. | Use ReadHeaderTimeout + IdleTimeout only. If per-route timeouts are needed later, use `http.TimeoutHandler` wrapping specific non-streaming handlers. |
| Full WAF / OWASP security header set | Over-engineering for a read-only data mirror. X-Frame-Options redundant with CSP frame-ancestors. Permissions-Policy, X-Content-Type-Options, Referrer-Policy add config complexity with minimal benefit for an app that only serves data, not user-submitted content. | CSP on /ui/ routes. HSTS is handled by Fly.io proxy (force_https=true in fly.toml). |
| Brotli compression | Marginal improvement over gzip (5-10% smaller) with significantly higher CPU cost. Fly.io edge proxy can add brotli at the CDN layer if needed. | Start with gzip only via klauspost/compress. |
| Self-hosted CDN assets | Eliminates CDN dependency but adds ~2MB of static files to every edge node, versioning burden, and go:embed bloat in the binary. | Keep CDN with SRI integrity attributes for supply chain protection. SRI + crossorigin provides equivalent security to self-hosting. |
| Generic upsert refactoring | 13 repetitive upsert functions are verbose but each handles type-specific field mappings. ent's generated Create types have no shared interface suitable for generics. | Keep per-type functions. Extract only obviously shared patterns (batchSize loop wrapper, social media conversion) if other changes already touch the file. |
| CORS preflight caching improvements | Already implemented: `MaxAge: 86400` (24 hours) in cors.go. This is the maximum practical value. | Verify in testing and document as complete. No further work needed. |
| CI coverage enforcement threshold | Coverage numbers vary across packages (87-98% on utilities, 80%+ on handlers). A global threshold creates noise when generated code is included. | Continue using octocov for PR coverage comments. Address specific untested packages (graphql, database) individually. |

## Feature Dependencies

```
SRI attributes on CDN assets  --> CSP header (policy must include hashes or 'unsafe-inline' for assets without SRI)
CSP header                     --> SRI attributes (complete SRI first to tighten CSP)

Server timeouts                --> (independent, main.go server config)
Connection pool limits         --> (independent, database.go)
Body size limits               --> (independent, sync handler in main.go)
ASN range validation           --> (independent, detail.go)
Width param bounds             --> (independent, termrender or handler)
Config validation              --> (independent, config.go)

GraphQL sentinel errors        --> GraphQL handler tests (write tests for old behavior first, then migrate)
GraphQL handler tests          --> GraphQL sentinel errors (tests validate the migration)

Metrics caching                --> (independent, otel/metrics.go + sync worker completion signal)
Compression middleware         --> (independent, main.go middleware chain; must handle gRPC exclusion)

Linter additions               --> (independent, .golangci.yml; may generate fix work across codebase)
Docker CI build                --> (independent, .github/workflows/ci.yml)
Dockerfile HEALTHCHECK         --> (independent, Dockerfile + possibly main.go for --healthcheck flag)

detail.go refactor             --> (independent but high risk; existing web handler tests provide safety net)
upsert.go refactor             --> (independent but low ROI; defer unless other sync changes touch it)
```

## MVP Recommendation

Priority order based on risk/reward ratio and dependency chains:

1. **Server timeouts + connection pool + body limits** -- three zero-risk security wins, 3 files touched, no behavioral change for any client
2. **ASN range validation + width param bounds + config validation** -- input hardening, prevents edge cases and crashes, all independent changes
3. **SRI attributes on remaining CDN assets** -- completes the supply chain protection already partially done (Leaflet, GraphiQL have SRI; Tailwind, flag-icons, markercluster do not)
4. **GraphQL sentinel errors + error classifier tests** -- fixes GO-ERR-2 violation, adds the first test file to internal/graphql/. Write tests against old string-matching behavior first, then refactor to sentinel errors.
5. **Database package tests** -- low-effort high-value: verify pragmas (WAL, FK) are actually applied, not just requested in DSN
6. **Metrics COUNT query caching** -- removes 13 unnecessary COUNT queries per scrape interval
7. **Compression middleware** -- bandwidth reduction, medium complexity due to gRPC path exclusion
8. **CSP header** -- requires SRI completion first; deploy as report-only first
9. **Linter additions + Docker CI build + Dockerfile HEALTHCHECK** -- CI quality improvements, low risk
10. **detail.go refactor** -- largest file (1422 lines), highest risk, but existing tests provide safety net. Defer to end of milestone.

Defer to later milestone or skip:
- **Upsert refactoring**: Low ROI. ent's generated types make generics awkward. Only touch if sync code changes for other reasons.
- **CORS preflight caching**: Already done (MaxAge=86400). Verify only.

## Sources

- [Cloudflare: Complete guide to Go net/http timeouts](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) -- HIGH confidence, authoritative reference on timeout semantics and interactions
- [Gopher Academy: Exposing Go on the Internet](https://blog.gopheracademy.com/advent-2016/exposing-go-on-the-internet/) -- HIGH confidence, production hardening checklist (still relevant)
- [Simon Frey: Standard net/http config will break your production environment](https://simon-frey.com/blog/go-as-in-golang-standard-net-http-config-will-break-your-production/) -- MEDIUM confidence, practical timeout warnings
- [Alex Edwards: Configuring sql.DB for Better Performance](https://www.alexedwards.net/blog/configuring-sqldb) -- HIGH confidence, Go connection pool tuning guide
- [Go official: Managing database connections](https://go.dev/doc/database/manage-connections) -- HIGH confidence, stdlib documentation
- [Making SQLite faster in Go](https://turriate.com/articles/making-sqlite-faster-in-go) -- MEDIUM confidence, SQLite-specific pool tuning
- [klauspost/compress gzhttp package](https://pkg.go.dev/github.com/klauspost/compress/gzhttp) -- HIGH confidence, pkg.go.dev documentation
- [MDN: Content Security Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP) -- HIGH confidence, authoritative CSP reference
- [golangci-lint: Linters list](https://golangci-lint.run/docs/linters/) -- HIGH confidence, official documentation
- Codebase analysis of: main.go (server config, middleware chain, handler registration), database.go (pool settings, DSN), config.go (validation), handler.go (GraphQL error classification, playground), layout.templ (CDN assets, SRI state), cors.go (MaxAge already set), metrics.go (COUNT queries), detail.go (1422 lines, refactor candidate), upsert.go (13 repetitive functions), .golangci.yml (current linters), Dockerfile/Dockerfile.prod (HEALTHCHECK absence), fly.toml (concurrency limits, health checks), ci.yml (missing Docker build)
