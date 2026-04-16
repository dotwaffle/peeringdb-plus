# Technology Stack: v1.12 Hardening & Tech Debt

**Project:** PeeringDB Plus
**Researched:** 2026-04-02
**Mode:** Incremental -- additions/changes for hardening milestone only

## Executive Summary

The v1.12 hardening milestone requires **zero new direct dependencies** for most items. HTTP server timeouts, SQLite connection pool, request body limits, and CSP headers are all stdlib/configuration changes. Compression middleware promotes the existing `klauspost/compress` transitive dependency to a direct one. The three new linters (`exhaustive`, `contextcheck`, `gosec`) are config-only additions to golangci-lint. Go 1.26's `slog.NewMultiHandler` replaces the project's hand-written `fanoutHandler`, eliminating custom code.

## 1. HTTP Server Timeouts

**Confidence:** HIGH (stdlib, well-documented, no Go 1.26 changes to http.Server timeout fields)

### Current State

The `http.Server` in `cmd/peeringdb-plus/main.go` (line 376) has **no timeouts configured**. This means:
- Unlimited read time (slow client can hold connections open indefinitely)
- Unlimited write time (stuck handlers never timeout)
- Unlimited idle time (default 0 means no keep-alive timeout beyond TCP)
- No header read timeout (Slowloris attack vector)

### Recommendation

```go
server := &http.Server{
    Addr:              cfg.ListenAddr,
    Handler:           handler,
    Protocols:         &protocols,
    ReadTimeout:       30 * time.Second,
    ReadHeaderTimeout: 10 * time.Second,
    IdleTimeout:       120 * time.Second,
    // WriteTimeout intentionally omitted (0 = no limit).
    // Streaming RPCs manage their own timeout via cfg.StreamTimeout.
}
```

**Rationale for values:**
- `ReadHeaderTimeout: 10s` -- Prevents Slowloris attacks. Must be set independently of `ReadTimeout` because it governs the TLS handshake deadline too (since Go 1.19).
- `ReadTimeout: 30s` -- Covers full request body read. The only POST endpoint is `/sync` (no body), so 30s is generous. GraphQL POST bodies are small queries.
- `IdleTimeout: 120s` -- Keep-alive connection reuse for htmx polling and gRPC multiplexing. Default is infinite; 120s matches common reverse proxy timeouts.
- `WriteTimeout` intentionally omitted (0 = no limit) -- Streaming RPCs have their own `StreamTimeout` (default 60s). Setting `WriteTimeout` at the server level would kill long-running gRPC streams. Per-handler timeout control via `http.TimeoutHandler` can be added later if needed for non-streaming routes.

**Go 1.26 changes:** None to `http.Server` timeout fields. The `HTTP2Config.StrictMaxConcurrentRequests` field was added but is unrelated to timeouts. The existing timeout semantics are unchanged.

**New dependencies:** None. Pure stdlib.

**Configuration:** Add `PDBPLUS_READ_TIMEOUT`, `PDBPLUS_IDLE_TIMEOUT` env vars to `internal/config/config.go` with the defaults above. `ReadHeaderTimeout` can be hardcoded (no reason to make configurable).

### Sources

- [The complete guide to Go net/http timeouts (Cloudflare)](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26) -- no http.Server timeout changes
- [Exposing Go on the Internet (Gopher Academy)](https://blog.gopheracademy.com/advent-2016/exposing-go-on-the-internet/)

---

## 2. SQLite Connection Pool Configuration

**Confidence:** HIGH (database/sql pool is well-documented, modernc.org/sqlite behavior verified)

### Current State

`internal/database/database.go` opens the database with no pool configuration:
```go
db, err := sql.Open("sqlite3", dsn)
```

The default `sql.DB` pool has unlimited open connections and 2 idle connections. With SQLite's single-writer constraint, concurrent writes cause `SQLITE_BUSY` errors (mitigated by the 5000ms `busy_timeout` pragma but not eliminated).

### Recommendation

```go
db.SetMaxOpenConns(4)
db.SetMaxIdleConns(4)
db.SetConnMaxLifetime(0) // connections reused indefinitely (SQLite is local)
db.SetConnMaxIdleTime(5 * time.Minute)
```

**Rationale:**
- `MaxOpenConns(4)` -- This is a **read-heavy** application. Multiple concurrent readers improve query throughput on the 5 API surfaces. Only the sync worker writes. 4 connections balances read parallelism against SQLite's file-level locking.
- `MaxIdleConns(4)` -- Match `MaxOpenConns` to avoid connection churn. SQLite connections are cheap (local file), so keeping them idle is fine.
- `ConnMaxLifetime(0)` -- No reason to expire connections to a local file. Unlike network databases, there is no connection staleness concern.
- `ConnMaxIdleTime(5m)` -- Reclaim truly unused connections during idle periods, but generous enough to survive traffic bursts.

**Important:** Each `sql.DB` connection in modernc.org/sqlite holds a separate SQLite connection to the same file. WAL mode allows concurrent readers with one writer. The `busy_timeout(5000)` pragma (already set) handles write contention by retrying for 5 seconds before returning SQLITE_BUSY.

**New dependencies:** None. Pure `database/sql` stdlib configuration.

**Configuration:** Hardcode these values. They are internal implementation details, not user-facing config. The pool sizes could be made configurable later if needed.

### Sources

- [Managing connections (Go docs)](https://go.dev/doc/database/manage-connections)
- [Configuring sql.DB for Better Performance (Alex Edwards)](https://www.alexedwards.net/blog/configuring-sqldb)
- [SQLite with Go best practices (modernc.org)](https://theitsolutions.io/blog/modernc.org-sqlite-with-go)

---

## 3. Request Body Size Limits

**Confidence:** HIGH (stdlib `http.MaxBytesReader`)

### Current State

No request body size limits. The GraphQL endpoint accepts POST bodies of arbitrary size.

### Recommendation

Use `http.MaxBytesReader` in a middleware wrapper for POST/PUT/PATCH routes:

```go
func MaxBodySize(maxBytes int64) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
                r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
            }
            next.ServeHTTP(w, r)
        })
    }
}
```

**Recommended limit: 1 MB (1048576 bytes)**. GraphQL queries are typically < 10KB. The largest legitimate PeeringDB-related GraphQL query would be well under 100KB. 1MB provides generous headroom while preventing abuse.

**Why `http.MaxBytesReader` over middleware alternatives:**
- Stdlib, zero dependencies
- Automatically returns 413 Request Entity Too Large when exceeded
- Properly terminates the read, preventing memory exhaustion
- Works with any handler (GraphQL, REST, ConnectRPC)

**New dependencies:** None.

**Configuration:** Add `PDBPLUS_MAX_BODY_SIZE` env var with default `1048576` (1MB).

### Sources

- [Go stdlib http.MaxBytesReader](https://pkg.go.dev/net/http#MaxBytesReader)

---

## 4. Compression Middleware (Gzip + Zstd)

**Confidence:** HIGH (klauspost/compress already a transitive dependency)

### Current State

No response compression. All responses sent uncompressed.

### Recommendation

Use `github.com/klauspost/compress/gzhttp` -- the `gzhttp` subpackage of the already-transitive `klauspost/compress` (v1.18.4 in go.mod as indirect). Promotes to direct dependency.

```go
import "github.com/klauspost/compress/gzhttp"

// In middleware chain:
handler = gzhttp.GzipHandler(handler)
```

**Why klauspost/compress/gzhttp:**
- Already in the dependency tree (transitive via buf/protobuf tooling) -- zero new modules downloaded
- Supports gzip + zstd by default with content negotiation
- Benchmarks: 214 MB/s gzip, 548 MB/s zstd (vs stdlib 169 MB/s)
- Minimum body size 200 bytes by default (avoids compressing tiny responses)
- Properly handles `Vary: Accept-Encoding`, skips already-compressed content types
- Implements `http.Flusher` passthrough (required for gRPC streaming)

**Why NOT brotli:**
- `gzhttp` does not include brotli. Adding brotli requires `CAFxX/httpcompression` which is a new dependency.
- Gzip + zstd covers all modern clients. Zstd is better than brotli for real-time compression (faster encode, similar ratio).
- Fly.io's edge proxy already handles brotli for TLS-terminated connections. Adding it at the app layer is redundant for production.

**Why NOT stdlib `compress/gzip`:**
- No middleware wrapper, no content negotiation, no zstd support
- klauspost/compress is faster (214 MB/s vs 169 MB/s single-core)

**Middleware chain placement:**
Compression must wrap **after** CORS (CORS headers must not be compressed) and **before** OTel HTTP (trace the compressed response).

**gRPC/ConnectRPC compatibility:** The gzhttp middleware skips compression for `application/grpc` content types by default. ConnectRPC's `application/connect+proto` will be compressed, which is desirable.

**New dependencies:** None new in go.sum. Promotes `github.com/klauspost/compress` from indirect to direct.

### Sources

- [klauspost/compress/gzhttp package](https://pkg.go.dev/github.com/klauspost/compress/gzhttp)
- [klauspost/compress GitHub](https://github.com/klauspost/compress)

---

## 5. Content-Security-Policy Headers

**Confidence:** HIGH (standard HTTP header, well-documented)

### Current State

No CSP header set. The layout template loads these CDN resources:

| Resource | Source | SRI |
|----------|--------|-----|
| @tailwindcss/browser@4 | cdn.jsdelivr.net | No |
| flag-icons@7.5.0 | cdn.jsdelivr.net | No |
| leaflet@1.9.4 CSS | unpkg.com | Yes |
| leaflet@1.9.4 JS | unpkg.com | Yes |
| leaflet.markercluster@1.5.3 CSS (x2) | unpkg.com | No |
| leaflet.markercluster@1.5.3 JS | unpkg.com | No |
| htmx.min.js | /static/ (self-hosted) | N/A |
| Tile images | basemaps.cartocdn.com | N/A (img) |

Additionally, the syncing page loads `@tailwindcss/browser@4` from jsdelivr.

### Recommendation

**Phase 1: Add SRI hashes to all CDN assets missing them.** This is the highest-value security improvement. SRI prevents CDN compromise from serving malicious code.

Missing SRI: `@tailwindcss/browser@4`, `flag-icons@7.5.0`, `leaflet.markercluster@1.5.3` (CSS x2 + JS).

Generate hashes with:
```bash
curl -sL https://cdn.jsdelivr.net/npm/@tailwindcss/browser@4 | openssl dgst -sha256 -binary | openssl base64 -A
```

**Phase 2: Add CSP header via middleware.**

```
Content-Security-Policy:
  default-src 'none';
  script-src 'self' https://cdn.jsdelivr.net https://unpkg.com 'unsafe-inline';
  style-src 'self' https://cdn.jsdelivr.net https://unpkg.com 'unsafe-inline';
  img-src 'self' https://*.basemaps.cartocdn.com data:;
  font-src https://cdn.jsdelivr.net;
  connect-src 'self';
  frame-src 'none';
  base-uri 'self';
  form-action 'self';
```

**Key decisions:**
- `'unsafe-inline'` for `script-src` is **required** because: (a) Tailwind CSS browser runtime generates inline styles, (b) layout.templ has inline `<script>` blocks for dark mode detection, keyboard navigation, table sorting, and spotlight search. Nonce-based CSP would require per-request nonce generation and templ modifications -- deferred to a future milestone if Tailwind is self-hosted.
- `'unsafe-inline'` for `style-src` is required because Tailwind browser runtime injects `<style>` elements. Also, inline `<style>` blocks exist in layout.templ for marker-cluster theming and animations.
- `connect-src 'self'` covers htmx AJAX requests.
- `img-src` includes `data:` for potential base64 inline images and `*.basemaps.cartocdn.com` for Leaflet tile layers.
- `font-src` for flag-icons web font from jsdelivr.

**Implementation:** New middleware function in `internal/middleware/` that sets the header on all responses.

**New dependencies:** None.

**Caveat:** The `'unsafe-inline'` directives weaken CSP for XSS protection. This is an inherent limitation of using Tailwind CSS browser runtime and inline scripts. The primary value of this CSP is preventing unauthorized resource loading (exfiltration, cryptominers). SRI hashes on CDN assets provide the actual defense layer for third-party code.

### Sources

- [Content Security Policy (MDN)](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP)
- [CSP OWASP Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html)
- [Tailwind CSS CSP discussion](https://github.com/tailwindlabs/tailwindcss/discussions/13326)

---

## 6. Additional golangci-lint Linters

**Confidence:** HIGH (verified against golangci-lint v2 linter catalog)

### Current State

`.golangci.yml` enables: standard preset + `gocritic`, `misspell`, `nolintlint`, `revive`. Has exclusion for `gosec` in test files.

### Recommendation

```yaml
version: "2"

linters:
  default: standard
  enable:
    - exhaustive
    - contextcheck
    - gosec
    - gocritic
    - misspell
    - nolintlint
    - revive
  exclusions:
    generated: strict
    presets:
      - comments
      - std-error-handling
    rules:
      - path: _test\.go
        linters:
          - gosec
```

**Linter details:**

| Linter | Purpose | Why Add | Noise Risk |
|--------|---------|---------|------------|
| `exhaustive` | Checks enum switch completeness | Catches missing cases in `SyncMode`, `termrender.Mode` type switches. Prevents silent bugs when new enum values are added. | LOW -- only fires on actual switch statements with enum types |
| `contextcheck` | Detects non-inherited context usage | Catches `context.Background()` where a request context should be propagated. Critical for OTel trace continuity. | LOW-MEDIUM -- may flag intentional `context.Background()` uses (e.g., the sync goroutine uses `appCtx` deliberately). Add `//nolint:contextcheck` with rationale for legitimate cases. |
| `gosec` | Security issue detection | Catches hardcoded credentials, weak crypto, SQL injection, path traversal, etc. Already excluded in test files. Recent v2 updates added rules G113 (HTTP body closure), G118-G123 (various). | MEDIUM -- may flag false positives on modernc.org/sqlite DSN strings. Tune with `gosec.excludes` if needed. |

**Linters considered but NOT recommended:**

| Linter | Why Not |
|--------|---------|
| `bodyclose` | Only relevant for HTTP client code, limited to `internal/peeringdb/client.go`. Already handles body closure. Low value. |
| `exhaustruct` | Too aggressive for a codebase using struct literals extensively. Massive noise. |
| `wrapcheck` | The project already wraps errors consistently per GO-ERR-1. Would flag generated code patterns. |

**New dependencies:** None. Linters are built into golangci-lint.

**Configuration additions for gosec (if needed after initial run):**

```yaml
linters:
  settings:
    gosec:
      excludes:
        - G304  # File path from variable (used for DB path from config, validated)
```

### Sources

- [golangci-lint v2 Linters catalog](https://golangci-lint.run/docs/linters/)
- [gosec rules](https://github.com/securego/gosec#available-rules)

---

## 7. Docker HEALTHCHECK and Docker Build in CI

**Confidence:** HIGH (Docker and GitHub Actions are well-documented)

### Current State

- Dockerfile has no `HEALTHCHECK` instruction
- CI does not build the Docker image (only `go build ./...`)
- Runtime image is `cgr.dev/chainguard/glibc-dynamic:latest-dev` which has no `curl`/`wget`

### Recommendation: HEALTHCHECK

Since the Chainguard glibc-dynamic image contains no shell utilities, build a tiny Go healthcheck binary:

```go
// cmd/healthcheck/main.go
package main

import (
    "net/http"
    "os"
)

func main() {
    resp, err := http.Get("http://localhost:8080/healthz")
    if err != nil || resp.StatusCode != http.StatusOK {
        os.Exit(1)
    }
}
```

Add to Dockerfile:

```dockerfile
# Build stage
FROM cgr.dev/chainguard/go AS build
WORKDIR /app
COPY . .
RUN \
    --mount=type=cache,target=/root/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/peeringdb-plus ./cmd/peeringdb-plus && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/healthcheck ./cmd/healthcheck

# Runtime stage
FROM cgr.dev/chainguard/glibc-dynamic:latest-dev
COPY --from=build /bin/peeringdb-plus /usr/local/bin/peeringdb-plus
COPY --from=build /bin/healthcheck /usr/local/bin/healthcheck
RUN mkdir -p /data
ENV PDBPLUS_DB_PATH=/data/peeringdb-plus.db
HEALTHCHECK --interval=30s --timeout=5s --start-period=120s --retries=3 \
    CMD ["/usr/local/bin/healthcheck"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/peeringdb-plus"]
```

**Timing rationale:**
- `--start-period=120s` -- Generous startup window for initial PeeringDB sync (can take 60-90s on first boot)
- `--interval=30s` -- Frequent enough to detect issues quickly
- `--timeout=5s` -- /healthz is a trivial handler, should respond in < 1ms
- `--retries=3` -- Allows transient failures during sync

### Recommendation: Docker Build in CI

Add a new job to `.github/workflows/ci.yml`:

```yaml
  docker:
    name: Docker Build
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v6

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3

      - name: Build Docker image
        uses: docker/build-push-action@v7
        with:
          context: .
          push: false
          load: true
          tags: peeringdb-plus:ci
          cache-from: type=gha
          cache-to: type=gha,mode=max

      - name: Verify HEALTHCHECK is configured
        run: |
          docker inspect peeringdb-plus:ci --format='{{json .Config.Healthcheck}}' | grep -q healthcheck
```

**Why `docker/build-push-action@v7`:**
- Official Docker action, well-maintained
- `push: false` + `load: true` -- builds and loads into local Docker daemon without pushing
- GHA cache support reduces build times on subsequent runs
- Catches Dockerfile syntax errors, build failures, and missing files before deploy

**New dependencies:** GitHub Actions only (`docker/setup-buildx-action@v3`, `docker/build-push-action@v7`).

### Sources

- [Docker test before push (Docker docs)](https://docs.docker.com/build/ci/github-actions/test-before-push/)
- [docker/build-push-action](https://github.com/docker/build-push-action)
- [Chainguard glibc-dynamic image](https://images.chainguard.dev/directory/image/glibc-dynamic/overview)

---

## 8. Go 1.26-Specific Improvements

**Confidence:** HIGH (verified against Go 1.26 release notes)

### slog.NewMultiHandler (replaces custom fanoutHandler)

Go 1.26 adds `slog.NewMultiHandler` to the stdlib, which is a drop-in replacement for the project's hand-written `fanoutHandler` in `internal/otel/logger.go` (75 lines of custom code).

**Current code:**
```go
type fanoutHandler struct {
    handlers []slog.Handler
}
// ... Enabled, Handle, WithAttrs, WithGroup methods (75 lines)
```

**Replacement:**
```go
func NewDualLogger(stdout io.Writer, logProvider *sdklog.LoggerProvider) *slog.Logger {
    otelHandler := otelslog.NewHandler("peeringdb-plus",
        otelslog.WithLoggerProvider(logProvider),
    )
    stdoutHandler := slog.NewJSONHandler(stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo,
    })
    return slog.New(slog.NewMultiHandler(stdoutHandler, otelHandler))
}
```

Eliminates the entire `fanoutHandler` type and its 4 methods. The stdlib implementation is tested and handles edge cases that the custom implementation may not.

### io.ReadAll performance (free win)

Go 1.26 makes `io.ReadAll` approximately 2x faster with half the memory. The sync client uses `io.ReadAll` to read PeeringDB API responses. This is a free performance win with zero code changes -- just building with Go 1.26.

### errors.AsType -- NOT applicable

Go 1.26 adds the generic `errors.AsType[E]`. However, all `errors.As` calls in the codebase are in **generated code** (`ent/ent.go`, `ent/rest/server.go`). No hand-written code uses `errors.As`. Not relevant for v1.12.

### Other Go 1.26 features NOT relevant to v1.12
- `crypto/hpke` -- not applicable (no custom crypto)
- Green Tea GC -- automatic, no code changes needed
- `net.Dialer.DialTCP/DialUDP` -- not applicable
- `http.HTTP2Config.StrictMaxConcurrentRequests` -- not needed
- Goroutine leak profiler (experimental) -- nice for testing but not a hardening item
- `bytes.Buffer.Peek` -- potentially useful for terminal renderer but not a priority

---

## Stack Changes Summary

### New Direct Dependencies

| Dependency | Version | Purpose | Already in go.sum |
|------------|---------|---------|-------------------|
| `github.com/klauspost/compress/gzhttp` | v1.18.4 | HTTP compression middleware | Yes (promote indirect to direct) |

### New Go Files

| File | Purpose |
|------|---------|
| `cmd/healthcheck/main.go` | Docker HEALTHCHECK binary (~15 lines) |
| `internal/middleware/compression.go` | Compression middleware wrapper |
| `internal/middleware/csp.go` | Content-Security-Policy header middleware |
| `internal/middleware/bodylimit.go` | Request body size limit middleware |

### Modified Files

| File | Change |
|------|--------|
| `cmd/peeringdb-plus/main.go` | Add server timeouts, body limit middleware, compression middleware, CSP middleware to chain |
| `internal/config/config.go` | Add timeout config fields, body size limit |
| `internal/database/database.go` | Add connection pool configuration |
| `internal/otel/logger.go` | Replace fanoutHandler with slog.NewMultiHandler |
| `.golangci.yml` | Add exhaustive, contextcheck, gosec linters |
| `Dockerfile` | Add healthcheck binary build, HEALTHCHECK instruction |
| `.github/workflows/ci.yml` | Add Docker build job |
| `internal/web/templates/layout.templ` | Add SRI hashes to CDN assets missing them |
| `internal/web/templates/syncing.templ` | Add SRI hash to Tailwind CDN script |

### What NOT to Add

| Technology | Why Not |
|------------|---------|
| Brotli middleware (CAFxX/httpcompression) | New dependency. Gzip + zstd covers all clients. Fly.io edge handles brotli for TLS. |
| Nonce-based CSP | Requires per-request nonce generation and templ changes. Premature while using Tailwind browser runtime. |
| `bodyclose` linter | Minimal value -- only one HTTP client in the project, already handles body closure. |
| `exhaustruct` linter | Too aggressive, massive noise from struct literals throughout codebase. |
| Custom error types for Go 1.26 `errors.AsType` | All `errors.As` usage is in generated code. |
| `http.TimeoutHandler` per-route wrappers | Over-engineering for v1.12. Server-level timeouts + existing StreamTimeout cover the attack surface. |
| Rate limiting middleware | Out of scope for v1.12. Read-only app behind Fly.io's proxy. |

---

## Middleware Chain Order (After Hardening)

```
Recovery -> CORS -> Compression -> OTel HTTP -> Logging -> CSP -> BodyLimit -> Readiness -> Caching -> mux
```

**Order rationale:**
1. **Recovery** -- outermost to catch panics from any layer
2. **CORS** -- before compression (CORS headers must be present in responses)
3. **Compression** -- wraps response writer, must be early to compress all downstream output
4. **OTel HTTP** -- traces the compressed response
5. **Logging** -- logs after OTel has added trace context
6. **CSP** -- sets security headers on all responses
7. **BodyLimit** -- limits request bodies before handlers read them
8. **Readiness** -- gates routes until sync complete
9. **Caching** -- sets cache headers, closest to handlers

---

## Sources

### Official Documentation
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26)
- [Go database/sql managing connections](https://go.dev/doc/database/manage-connections)
- [Go net/http package](https://pkg.go.dev/net/http)
- [golangci-lint v2 Linters](https://golangci-lint.run/docs/linters/)

### Libraries
- [klauspost/compress/gzhttp](https://pkg.go.dev/github.com/klauspost/compress/gzhttp)
- [klauspost/compress GitHub](https://github.com/klauspost/compress)

### Best Practices
- [Go net/http timeouts (Cloudflare)](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/)
- [Configuring sql.DB (Alex Edwards)](https://www.alexedwards.net/blog/configuring-sqldb)
- [CSP OWASP Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html)
- [MDN CSP Guide](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP)
- [Docker HEALTHCHECK test before push](https://docs.docker.com/build/ci/github-actions/test-before-push/)
- [docker/build-push-action](https://github.com/docker/build-push-action)
