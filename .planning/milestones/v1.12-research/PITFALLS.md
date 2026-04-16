# Domain Pitfalls

**Domain:** Security hardening, operational improvements, and tech debt cleanup on an existing Go HTTP server with gRPC streaming, 5 API surfaces, and CDN-loaded frontend assets
**Researched:** 2026-04-02
**Milestone:** v1.12 Hardening & Tech Debt

## Critical Pitfalls

Mistakes that break existing functionality, cause outages, or require architectural rework.

### Pitfall 1: HTTP Server WriteTimeout Kills gRPC Streaming RPCs

**What goes wrong:** Setting `WriteTimeout` on `http.Server` applies an absolute deadline from when the response headers are written. Server-streaming RPCs (StreamNetworks, StreamFacilities, etc.) that send data over 10-60 seconds are terminated mid-stream with `RST_STREAM INTERNAL_ERROR`. Clients see `stream terminated by RST_STREAM` errors.

**Why it happens:** `WriteTimeout` is a connection-level absolute timeout, not a per-write idle timeout. There is no way to distinguish "handler is actively writing data" from "handler is hung" at the server level. gRPC streaming fundamentally requires long-lived HTTP/2 connections where the server writes over an extended period.

**Consequences:** All 13 streaming RPCs break in production. The existing `StreamTimeout` (60s configurable) already governs stream lifetime via `context.WithTimeout`, so the application-level timeout is already handled. A server-level `WriteTimeout` shorter than the stream timeout makes streams fail before their intended deadline.

**Prevention:**
- Do NOT set `WriteTimeout` on `http.Server`. The ConnectRPC deployment docs explicitly warn about this.
- Set `ReadHeaderTimeout` (e.g., 10s) to prevent slowloris attacks on header reading. This is safe because it only applies to reading request headers, not to the response lifecycle.
- Do NOT set `ReadTimeout` either, because for gRPC streaming the client may send messages over an extended period.
- Rely on per-handler timeouts: the existing `StreamTimeout` config for streaming RPCs, and `http.TimeoutHandler` or `context.WithTimeout` for unary endpoints.
- Use `http.ResponseController.SetWriteDeadline()` (Go 1.20+) for per-request write deadlines on non-streaming handlers if finer control is needed. However, the middleware response writer wrappers must implement `Unwrap()` (already done in this codebase) for `ResponseController` to find the underlying `net.Conn`.

**Detection:** Deploy with `WriteTimeout` set. Run `grpcurl` with a streaming RPC (`StreamNetworks`). If the stream is cut short with an internal error before the data is fully sent, the timeout is killing streams.

### Pitfall 2: Compression Middleware Double-Encodes gRPC/ConnectRPC Responses

**What goes wrong:** An HTTP-level gzip compression middleware wraps all responses, including ConnectRPC handlers. ConnectRPC already handles its own compression negotiation (gzip by default for responses). The response body gets compressed twice: once by ConnectRPC, then again by the HTTP middleware. Clients cannot decode the response because they expect single-layer gzip but receive double-compressed data.

**Why it happens:** ConnectRPC uses standard HTTP `Content-Encoding: gzip` and `Accept-Encoding` headers for its compression negotiation. An HTTP compression middleware that blindly compresses all responses sees the ConnectRPC response body and compresses it again. The middleware does not know the body is already gzip-compressed because ConnectRPC may set the encoding headers after the middleware has already wrapped the writer.

**Consequences:** All gRPC and Connect protocol clients fail to decode responses. gRPC-Web clients and browser-based Connect clients break silently (corrupted data). REST and GraphQL endpoints work fine, masking the problem until gRPC clients are tested.

**Prevention:**
- Exclude ConnectRPC paths from HTTP compression middleware. Match on content type: skip compression when `Content-Type` starts with `application/grpc`, `application/connect+`, or `application/proto`.
- Alternatively, exclude by path prefix: skip compression for paths matching `/peeringdb.v1.*/`, `/grpc.health.v1.Health/`, and `/grpc.reflection.v1/`.
- Apply compression only to REST (`/rest/v1/`), GraphQL (`/graphql`), PeeringDB compat (`/api/`), and web UI (`/ui/`, `/static/`) paths.
- Test with `grpcurl` (which sends `grpc-accept-encoding: gzip`) and verify the response decodes correctly.

**Detection:** `grpcurl` calls return garbled output or `ERROR: rpc error: code = Internal desc = grpc: failed to decompress the received message`. Browser DevTools show double `Content-Encoding` headers.

### Pitfall 3: CSP Header Blocks CDN-Loaded Assets and GraphiQL Playground

**What goes wrong:** A restrictive Content-Security-Policy header blocks scripts, styles, and fonts loaded from external CDNs. The web UI goes blank (Tailwind CSS from jsdelivr blocked), maps disappear (Leaflet from unpkg blocked), and the GraphiQL playground stops working (React from jsdelivr blocked, inline scripts blocked).

**Why it happens:** The application loads assets from multiple external sources:
- `cdn.jsdelivr.net` -- Tailwind CSS browser runtime, flag-icons CSS, React, GraphiQL
- `unpkg.com` -- Leaflet JS/CSS, MarkerCluster JS/CSS
- Inline `<script>` blocks in `layout.templ` -- dark mode detection, keyboard navigation, table sorting, spotlight search
- Inline `<style>` blocks -- Tailwind custom variants, marker cluster theme, animations
- GraphiQL requires the CSP `script-src` directive to include the keyword for allowing dynamic code generation (the CSP keyword commonly known as "unsafe" + "eval"), because GraphiQL's query editor uses dynamic code generation internally

A CSP that omits any of these sources breaks the corresponding functionality silently (blocked resources logged only in browser console).

**Consequences:** Web UI appears broken for users. Tailwind not loading means the page is unstyled raw HTML. Leaflet not loading means maps are empty divs. GraphiQL not loading means the playground shows "Loading..." forever. All of these fail silently from the server's perspective.

**Prevention:**
- Build the CSP incrementally by auditing every external resource in `layout.templ`, `syncing.templ`, and `graphql/handler.go` (playground template).
- Required `script-src` sources: `'self'`, `https://cdn.jsdelivr.net`, `https://unpkg.com`, `'unsafe-inline'` (for inline scripts in layout.templ). GraphiQL additionally needs the dynamic code generation keyword.
- Required `style-src` sources: `'self'`, `https://cdn.jsdelivr.net`, `https://unpkg.com`, `'unsafe-inline'` (for inline styles in layout.templ and Tailwind's runtime-generated styles).
- Required `img-src` sources: `'self'`, `data:` (for Leaflet markers), tile server domains (`*.basemaps.cartocdn.com` or equivalent).
- Required `connect-src`: `'self'` (for htmx fetch requests, GraphQL queries).
- Consider using `Content-Security-Policy-Report-Only` header first to discover violations without breaking functionality, then switch to enforcing mode after validation.
- The GraphiQL playground path (`GET /graphql`) could use a different, more permissive CSP than the rest of the application. Set the CSP header per-route in the handler rather than as a global middleware.

**Detection:** Open browser DevTools Console after deploying CSP. Every blocked resource generates a `Refused to load...` console error with the exact violated directive.

### Pitfall 4: SRI (Subresource Integrity) Cannot Protect @tailwindcss/browser

**What goes wrong:** Adding `integrity="sha384-..."` attributes to the `@tailwindcss/browser` CDN script tag causes the browser to refuse to load it because the Tailwind browser runtime's content may change between versions served by the CDN, or the integrity hash computed at development time does not match what the CDN serves.

**Why it happens:** The `@tailwindcss/browser@4` import (without pinned patch version) fetches the latest v4.x release. Even with a pinned version, the Tailwind browser runtime generates CSS dynamically at runtime based on the HTML content it observes -- while the script itself is static, CDN edge caching and version resolution can cause hash mismatches. More importantly, `@tailwindcss/browser@4` (a semver range) resolves to different files as new versions are published, making a static hash invalid immediately.

**Consequences:** If a CDN update changes the resolved file, the integrity check fails and Tailwind CSS stops loading entirely. The page renders as unstyled HTML.

**Prevention:**
- Pin exact versions for all CDN scripts when adding SRI: `@tailwindcss/browser@4.1.3` not `@tailwindcss/browser@4`.
- Compute and update SRI hashes as part of a version bump process, not set-and-forget.
- Leaflet and React already use pinned versions with integrity attributes in the existing code -- extend this pattern to Tailwind and flag-icons.
- Consider that SRI on the Tailwind browser script is lower value than SRI on React/GraphiQL, because `@tailwindcss/browser` only generates CSS (no code execution risk), while React and GraphiQL execute arbitrary JavaScript.
- Alternatively, self-host Tailwind browser as a static asset (same pattern as `htmx.min.js` in `/static/`) to eliminate CDN dependency entirely.

**Detection:** SRI failure manifests as a console error: `Failed to find a valid digest in the 'integrity' attribute for resource...`. The page loads without any styling.

## Moderate Pitfalls

### Pitfall 5: SQLite Connection Pool Misconfiguration Causes SQLITE_BUSY Under Load

**What goes wrong:** Setting `MaxOpenConns` too high on the `database/sql.DB` allows multiple goroutines to open concurrent write transactions. SQLite WAL mode permits only one writer at a time. Concurrent writes contend on the WAL lock, producing `SQLITE_BUSY` errors despite having `busy_timeout` set.

**Why it happens:** The current `database.Open()` does not set `MaxOpenConns`, `MaxIdleConns`, or `ConnMaxIdleTime`. Go's `database/sql` defaults to unlimited open connections. While the application is read-heavy (read-only mirror), the sync worker writes data, and ent ORM migrations write. Multiple open connections all attempting writes at the same instant exceed the single-writer limit.

**Consequences:** Sync failures during high read load, or read queries seeing `database is locked` errors if the busy timeout (5000ms, already configured) is exceeded. On Fly.io edge nodes with LiteFS, only the primary writes, but concurrent API reads during sync could still exhaust the connection pool.

**Prevention:**
- Separate read and write pools by creating two `*sql.DB` instances: one for reads (higher `MaxOpenConns`, e.g., 10-20) and one for writes (`MaxOpenConns` = 1).
- OR, since the application architecture already separates reads (API handlers) from writes (sync worker on primary only), set a reasonable `MaxOpenConns` (e.g., 10) and `MaxIdleConns` (e.g., 5) on the single pool. The sync worker uses transactions that serialize writes internally.
- Set `ConnMaxIdleTime` (e.g., 5 minutes) to prevent idle connections from holding locks or going stale after LiteFS failover.
- The existing `busy_timeout(5000)` pragma is correct and should be preserved.
- Do NOT set `MaxOpenConns = 1` globally -- this would serialize all reads and destroy API latency.

**Detection:** `SQLITE_BUSY` or `database is locked` errors in logs during sync. OTel traces showing long `db.query` spans (>5s) during sync cycles.

### Pitfall 6: Request Body Size Limit Breaks GraphQL Introspection and Complex Queries

**What goes wrong:** Applying a global `http.MaxBytesReader` with a small limit (e.g., 64KB) to all POST requests causes GraphQL introspection queries and deeply nested queries to be rejected with `413 Request Entity Too Large`. PeeringDB compat API queries with many filter parameters may also exceed the limit.

**Why it happens:** GraphQL introspection responses require the client to send the full introspection query, which can be 2-5KB. Complex user queries with multiple nested fields and fragments can be 10-50KB. A global limit treats all endpoints the same, but GraphQL payloads are legitimately larger than typical REST bodies.

**Consequences:** GraphiQL playground sends introspection query on load -- if this fails, the playground shows no schema documentation and autocomplete breaks. Power users with complex queries get silent failures.

**Prevention:**
- Apply body size limits per-route, not globally:
  - GraphQL (`/graphql`): 256KB-1MB (queries can be large, but complexity/depth limits already prevent abuse).
  - REST (`/rest/v1/`): Read-only, so request bodies should be minimal. 64KB is sufficient.
  - PeeringDB compat (`/api/`): Read-only GET requests. Body limit does not apply (no POST).
  - ConnectRPC: ConnectRPC handles its own message size limits via `connect.WithReadMaxBytes()`. Do not layer an HTTP-level limit on top.
  - POST `/sync`: Small JSON body. 4KB is sufficient.
- Use `http.MaxBytesReader` in per-route middleware or within the handler itself, not as a global middleware.

**Detection:** GraphiQL playground loads but shows "Loading schema..." indefinitely. The browser Network tab shows the introspection POST returning 413.

### Pitfall 7: CORS Preflight Caching Interacts with Per-Route CSP Headers

**What goes wrong:** If CSP headers vary by route (e.g., more permissive for `/graphql` playground) but CORS preflight responses are cached with a long `Access-Control-Max-Age`, the browser may cache a preflight response from one route and reuse it for another route. This does not directly break functionality (CORS and CSP are independent mechanisms), but if the CSP header is set in the CORS middleware rather than per-route, all routes get the same CSP.

**Why it happens:** CORS and CSP are often conflated because both are security headers set in middleware. Developers add CSP to the CORS middleware for convenience, then every preflight and actual response gets the same policy.

**Consequences:** Either the GraphiQL playground CSP is too restrictive (breaks playground) or the general CSP is too permissive (allows dynamic code generation on all pages when only GraphiQL needs it).

**Prevention:**
- Keep CSP and CORS as separate middleware. CORS is already handled by `rs/cors` -- do not add CSP there.
- Implement CSP as a separate middleware or per-handler header setting.
- For the GraphiQL playground handler, set the CSP header directly in `PlaygroundHandler()` before writing the response.
- For the web UI, set CSP in the `Layout` templ component or in the web handler middleware.
- For API endpoints (REST, compat, ConnectRPC), CSP is irrelevant (no HTML rendering), so omit it or use a minimal policy.

**Detection:** Inspect response headers for different routes. If `/graphql` and `/ui/` have identical CSP headers, the separation is not working.

### Pitfall 8: Adding Linters to Existing Codebase Produces Hundreds of Findings

**What goes wrong:** Enabling `exhaustive`, `contextcheck`, and `gosec` in `.golangci.yml` on a codebase with ~15K lines of hand-written Go produces dozens to hundreds of findings. CI immediately fails. Developers either disable the linters (wasted effort) or spend days fixing findings that may be false positives.

**Why it happens:** `exhaustive` flags every switch statement on a type without a default case or missing enum values -- the ent-generated code and protobuf-generated enums have many values. `gosec` flags every `fmt.Sprintf` with user input (even when it is safe), every unvalidated integer conversion, and every use of `math/rand` (even in non-crypto contexts). `contextcheck` may flag legitimate patterns where context is intentionally not propagated (like the fire-and-forget sync in `newSyncHandler`).

**Consequences:** CI pipeline blocks all PRs. Team morale drops. Developers add `//nolint` comments everywhere, which defeats the purpose of the linter.

**Prevention:**
- Use `--new-from-rev=HEAD~1` or `--new-from-merge-base=main` in CI to only flag issues in changed code, not the entire codebase.
- Enable one linter at a time, fix the legitimate findings, and commit before enabling the next one.
- Configure exclusions in `.golangci.yml` for known false positives:
  - `exhaustive`: Exclude generated code paths (already handled by `exclusions.generated: strict`).
  - `gosec`: Exclude test files (already done). Consider excluding `G104` (unhandled errors on `fmt.Fprint` to `http.ResponseWriter` -- idiomatic in Go HTTP handlers).
  - `contextcheck`: May not be worth enabling if the codebase has legitimate fire-and-forget patterns.
- The current `.golangci.yml` already has `exclusions.generated: strict` which skips generated files. Verify this covers `ent/`, `gen/`, `graph/`, and `*_templ.go` patterns.

**Detection:** Run `golangci-lint run` locally with the new linters enabled before pushing to CI. Count the findings. If >20, the linter needs configuration tuning before enabling in CI.

### Pitfall 9: Metrics COUNT Query Caching Returns Stale Data After Sync

**What goes wrong:** Caching the result of `SELECT COUNT(*) FROM networks` (or the ent equivalent) for metrics/dashboard causes the Grafana dashboard to show stale object counts. After a sync adds 50 new networks, the cached count is still the old value until the cache expires.

**Why it happens:** The sync runs hourly (or on-demand). If the metrics cache TTL is longer than the sync interval (e.g., cache for 2 hours, sync every 1 hour), the counts are always one sync behind. Even with a 1-hour cache TTL, there is a window where the dashboard shows pre-sync counts.

**Consequences:** Grafana dashboard shows incorrect object counts. Operators may think the sync failed when it actually succeeded. The existing `pdbplus.data.type.count` gauge (registered via `InitObjectCountGauges`) already uses an observable gauge with a callback -- if this callback is cached, it returns stale data.

**Prevention:**
- Invalidate the metrics cache after each successful sync, not on a time-based TTL.
- The current `InitObjectCountGauges` uses an OTel observable gauge callback that queries the database on each metrics collection cycle. If the OTel metrics collection interval is shorter than the sync interval, this is already correct (no caching needed).
- If adding a caching layer, use a sync-completion signal (channel or callback) to invalidate, not a timer.
- For the GraphQL `syncStatus` query (which also returns counts), ensure the cache is keyed on the last sync timestamp, so a new sync automatically invalidates.

**Detection:** Compare Grafana dashboard counts with actual `SELECT COUNT(*) FROM networks` query immediately after a sync completes. If they differ, caching is stale.

## Minor Pitfalls

### Pitfall 10: Refactoring detail.go Breaks htmx Fragment Endpoints

**What goes wrong:** `detail.go` (1422 lines) handles all 6 entity detail pages plus their htmx fragment sub-routes (collapsible sections, map data endpoints). Extracting handlers into separate files changes the import structure or accidentally removes a route registration, causing htmx lazy-load sections to 404.

**Why it happens:** The file contains both the full-page handlers and the fragment handlers. Fragment routes are registered inside `Register()` or handler setup methods. Moving code between files can miss a route registration or break a closure that captures shared state.

**Prevention:**
- Before refactoring, write a test that hits every registered route and asserts 200 status. The existing `detail_test.go` likely covers most routes -- verify coverage.
- Extract one entity type at a time (e.g., `detail_network.go`, `detail_ix.go`), not all at once.
- Keep the `Register()` method in a single file that calls into the per-entity files, so route registration remains centralized and auditable.
- Run `go test -race ./internal/web/...` after each extraction step.

**Detection:** After refactoring, expand every collapsible section on every detail page type. Any section showing "Failed to load" means a fragment route was lost.

### Pitfall 11: Config Validation Rejects Existing Deployments

**What goes wrong:** Adding new validation rules to `config.Load()` (e.g., requiring `PDBPLUS_SYNC_TOKEN` to be non-empty, or enforcing minimum `DrainTimeout`) causes existing Fly.io deployments to fail on restart because their environment variables do not satisfy the new constraints.

**Why it happens:** Configuration validation runs at startup. New validation is tested locally with full env vars set but deployed to machines that have legacy env var configurations.

**Consequences:** Rolling deploy on Fly.io starts new machines that crash immediately. If all machines are updated simultaneously (unlikely but possible), the service goes down.

**Prevention:**
- New validation rules should have sane defaults that match current behavior, not new stricter defaults.
- Test config validation against the actual Fly.io env vars (check `fly secrets list` output) before deploying.
- Add validation for new config fields only, not retroactive validation on existing fields that were previously unchecked.
- Use `WARN` log messages for soft validation (e.g., "PDBPLUS_SYNC_TOKEN is empty, sync endpoint is unprotected") rather than `os.Exit(1)` for non-critical constraints.

**Detection:** `fly deploy` shows machines failing health checks and rolling back.

### Pitfall 12: Docker HEALTHCHECK and Fly.io Health Checks Conflict

**What goes wrong:** Adding a `HEALTHCHECK` instruction to the Dockerfile creates a Docker-level health check that runs inside the container. Fly.io also runs its own HTTP health checks against the configured health check path. If the Docker healthcheck has different timing or a different endpoint than Fly.io's, the container can be marked unhealthy by Docker (restarted) while Fly.io thinks it is healthy, or vice versa.

**Why it happens:** Fly.io ignores Docker's `HEALTHCHECK` instruction -- it uses its own health check mechanism configured in `fly.toml`. The Docker `HEALTHCHECK` is only meaningful if someone runs the image outside Fly.io (e.g., local Docker, CI).

**Consequences:** In CI Docker builds, the health check may fail if the container does not have a database or PeeringDB access, causing the build step to report an unhealthy image. On Fly.io, it is harmless but adds confusion.

**Prevention:**
- Set the Docker `HEALTHCHECK` to use the liveness endpoint (`/healthz`) which always returns 200, not the readiness endpoint (`/readyz`) which requires a completed sync.
- Use a generous interval and start-period in the Dockerfile `HEALTHCHECK` to account for startup time: `HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 CMD wget -q --spider http://localhost:8080/healthz || exit 1`
- Document that Fly.io ignores this and uses its own health check configuration.

**Detection:** `docker run` the image locally. Check `docker inspect` for health status. If it shows "unhealthy" despite the server running, the healthcheck endpoint or timing is wrong.

### Pitfall 13: Sync Upsert Deduplication Refactor Changes Insert Order

**What goes wrong:** Refactoring duplicated upsert code in `sync/upsert.go` (613 lines) changes the order in which entities are inserted or updated. If the new order violates foreign key constraints (e.g., inserting a `NetworkFacility` before the referenced `Network` or `Facility`), the sync fails with constraint violations.

**Why it happens:** PeeringDB entities have foreign key relationships. The sync must insert organizations before networks, networks before network_facility, etc. Duplicated code often has the correct order baked in implicitly. Refactoring into a generic function may lose the implicit ordering.

**Consequences:** Sync fails on fresh database (no existing data). May appear to work on incremental sync (entities already exist from prior sync) but fail on a cold start.

**Prevention:**
- Document the required insert order as a comment in the sync package.
- Test the refactored upsert code against a fresh (empty) database, not just an existing one.
- The existing `sync/integration_test.go` (619 lines) should cover this -- verify it tests from an empty database state.

**Detection:** Deploy to a new Fly.io region (empty database). If the first sync fails with `FOREIGN KEY constraint failed`, the insert order is wrong.

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| HTTP server timeouts | Pitfall 1 (WriteTimeout kills streaming) | Use ReadHeaderTimeout only. No WriteTimeout. Per-handler timeouts for unary RPCs. |
| SQLite connection pool | Pitfall 5 (SQLITE_BUSY) | Set MaxOpenConns=10, MaxIdleConns=5, ConnMaxIdleTime=5m. Preserve busy_timeout. |
| Request body size limits | Pitfall 6 (breaks GraphQL introspection) | Per-route limits. 256KB for GraphQL, 64KB for REST, skip ConnectRPC. |
| Compression middleware | Pitfall 2 (double-encodes gRPC) | Exclude gRPC/ConnectRPC content types and paths from compression. |
| CSP headers | Pitfall 3 (blocks CDN assets), Pitfall 4 (SRI + Tailwind), Pitfall 7 (per-route CSP) | Start with Report-Only. Separate CSP from CORS. Per-route CSP for GraphiQL. |
| Adding linters | Pitfall 8 (hundreds of findings) | Use --new-from-merge-base=main in CI. Enable one linter at a time. |
| Refactoring detail.go | Pitfall 10 (breaks fragment routes) | Extract one entity at a time. Run tests after each step. |
| Refactoring sync upsert | Pitfall 13 (insert order changes) | Test against empty database. Document required entity order. |
| Metrics caching | Pitfall 9 (stale counts) | Invalidate on sync completion, not timer-based TTL. |
| Config validation | Pitfall 11 (rejects existing deployments) | Defaults match current behavior. WARN, do not crash, for non-critical checks. |
| Docker HEALTHCHECK | Pitfall 12 (conflicts with Fly.io) | Use /healthz (liveness), not /readyz. Generous start-period. |

## Sources

- [ConnectRPC Deployment docs](https://connectrpc.com/docs/go/deployment/) -- timeout warnings for streaming, h2c configuration
- [ConnectRPC Serialization and Compression](https://connectrpc.com/docs/go/serialization-and-compression/) -- built-in gzip handling, Accept-Encoding negotiation
- [Cloudflare: Complete Guide to Go net/http Timeouts](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) -- ReadTimeout vs ReadHeaderTimeout vs WriteTimeout
- [grpc-go #3884: Server streams never close with HTTP mux timeout](https://github.com/grpc/grpc-go/issues/3884) -- WriteTimeout kills streaming
- [Alex Edwards: http.ResponseController](https://www.alexedwards.net/blog/how-to-use-the-http-responsecontroller-type) -- per-request SetWriteDeadline
- [SQLite WAL Mode](https://www.sqlite.org/wal.html) -- single-writer limitation
- [High-Performance SQLite Reads in Go](https://dev.to/lovestaco/high-performance-sqlite-reads-in-a-go-server-4on3) -- separate read/write pools
- [Bert Hubert: SQLITE_BUSY despite timeout](https://berthub.eu/articles/posts/a-brief-post-on-sqlite3-database-locked-despite-timeout/) -- busy_timeout is not enough
- [MDN: Content Security Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP) -- CSP directives reference
- [graphql-playground #1283: CSP blocks CDN](https://github.com/graphql/graphql-playground/issues/1283) -- GraphiQL requires dynamic code generation CSP keyword
- [CSP Bypass via unpkg.com](https://aszx87410.github.io/beyond-xss/en/ch2/csp-bypass/) -- public CDN CSP risks
- [golangci-lint FAQ: new-from-rev](https://golangci-lint.run/docs/welcome/faq/) -- incremental linting on existing codebases
- [golangci-lint Configuration](https://golangci-lint.run/docs/configuration/file/) -- exclusion rules, generated code handling
- [MDN: Subresource Integrity](https://developer.mozilla.org/en-US/docs/Web/Security/Defenses/Subresource_Integrity) -- SRI requirements for CDN assets
- Current codebase: `cmd/peeringdb-plus/main.go` (server config, middleware chain), `internal/middleware/` (CORS, logging, recovery, caching), `internal/config/config.go` (env vars), `internal/database/database.go` (SQLite setup), `internal/web/templates/layout.templ` (CDN assets), `internal/graphql/handler.go` (GraphiQL playground)
