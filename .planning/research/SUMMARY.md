# Project Research Summary

**Project:** PeeringDB Plus -- v1.12 Hardening & Tech Debt
**Domain:** Security hardening, operational improvements, and tech debt cleanup for a mature Go HTTP server
**Researched:** 2026-04-02
**Confidence:** HIGH

## Executive Summary

The v1.12 hardening milestone is a well-scoped collection of security, reliability, and code quality improvements for an existing production Go HTTP server with five API surfaces (Web UI, GraphQL, REST, PeeringDB compat, ConnectRPC/gRPC). The application is mature -- 46 phases and 11 milestones shipped -- and the hardening work requires **zero new direct dependencies** for most items. The single new dependency (promoting `klauspost/compress` from indirect to direct for compression middleware) is already in the module graph. Go 1.26's `slog.NewMultiHandler` eliminates 75 lines of custom handler code. All changes are additive or configurational -- no architectural rework is needed.

The recommended approach is to phase the work by dependency chain and risk profile: start with the zero-risk server configuration items (timeouts, connection pool, body limits), progress through request/response hardening (CSP, SRI, input validation), then tackle internal quality improvements (sentinel errors, metrics caching, compression), and finish with refactoring and CI improvements. This order ensures each phase has a clean rollback boundary and avoids the anti-pattern of refactoring code that is about to have new linters applied to it.

The primary risks are: (1) setting `WriteTimeout` on `http.Server` which will kill gRPC streaming RPCs -- use `ReadHeaderTimeout` only; (2) compression middleware double-encoding ConnectRPC responses -- exclude gRPC content types; (3) CSP headers breaking the web UI due to CDN-loaded assets and inline scripts -- deploy as `Report-Only` first; (4) SRI hash incompatibility with `@tailwindcss/browser@4` semver ranges -- pin exact versions. All four have clear, well-documented prevention strategies.

## Key Findings

### Recommended Stack

No new technologies are introduced. The hardening milestone is entirely about configuring, securing, and improving what already exists. See [STACK.md](STACK.md) for full details.

**Core changes:**
- **HTTP server timeouts**: `ReadHeaderTimeout: 10s`, `IdleTimeout: 120s` on `http.Server` -- stdlib, prevents Slowloris attacks without killing gRPC streams
- **SQLite connection pool**: `MaxOpenConns` + `MaxIdleConns` + `ConnMaxIdleTime` -- stdlib `database/sql`, prevents fd exhaustion under concurrent reads
- **Compression**: `klauspost/compress/gzhttp` -- already a transitive dependency, supports gzip + zstd with content negotiation, handles ETag suffixing and `http.Flusher` passthrough
- **Go 1.26**: `slog.NewMultiHandler` replaces custom `fanoutHandler` (75 lines eliminated); `io.ReadAll` is ~2x faster for free

**What NOT to add:** Brotli (Fly.io edge handles it), nonce-based CSP (premature while using Tailwind browser runtime), `bodyclose` linter (minimal value), `exhaustruct` linter (too noisy), rate limiting (read-only app behind Fly.io proxy).

### Expected Features

See [FEATURES.md](FEATURES.md) for full analysis including dependency graph.

**Must have (table stakes):**
- HTTP server timeouts (ReadHeaderTimeout, IdleTimeout) -- zero-timeout server is a known vulnerability
- SQLite connection pool limits -- unbounded connections risk fd exhaustion
- Request body size limits -- DoS vector even on read-only endpoints
- Input validation (ASN range 1-4294967295, width parameter 40-500) -- prevents edge cases and confusing errors
- Config validation expansion -- fail-fast per GO-CFG-1
- GraphQL error classification via sentinel errors -- fixes GO-ERR-2 violation (string matching)
- SRI attributes on remaining CDN assets -- completes partial supply chain protection

**Should have (differentiators):**
- Gzip/zstd compression middleware -- 60-80% bandwidth reduction on JSON/HTML
- Content-Security-Policy header -- prevents XSS on web UI
- Metrics COUNT query caching -- eliminates 13 unnecessary queries per scrape
- Test coverage for GraphQL handler and database package -- zero test files in both
- Additional linters (exhaustive, contextcheck, gosec) -- catches enum gaps, context misuse, security issues
- Docker build in CI + Dockerfile HEALTHCHECK -- catches build failures before deploy

**Defer:**
- Generic upsert refactoring -- low ROI, ent types have no shared interface
- CORS preflight caching -- already done (MaxAge=86400), verify only
- Rate limiting -- Fly.io proxy handles it
- Self-hosted CDN assets -- SRI provides equivalent security

### Architecture Approach

The existing architecture is clean and modular. Hardening changes fit naturally into the existing component boundaries. The middleware chain gains three new layers (body limit, compression, CSP), the server gains timeout configuration, the database gains pool configuration, and internal packages get quality improvements. No cross-cutting architectural changes are needed. See [ARCHITECTURE.md](ARCHITECTURE.md) for component-by-component integration analysis.

**Key integration points:**
1. **`cmd/peeringdb-plus/main.go`** -- server timeouts, middleware chain additions, healthcheck mode
2. **`internal/middleware/`** -- three new files: `bodylimit.go`, `compression.go`, `csp.go`
3. **`internal/config/config.go`** -- new timeout fields, expanded validation
4. **`internal/database/database.go`** -- connection pool configuration (3 lines)
5. **`internal/otel/`** -- `slog.NewMultiHandler` replacement, metrics count caching
6. **`internal/graphql/handler.go`** -- sentinel error classification
7. **Templates** -- SRI hashes on 5 CDN assets
8. **CI/Docker** -- Docker build job, HEALTHCHECK instruction with Go-based health binary

**Updated middleware chain order:**
```
Recovery -> CORS -> Compression -> OTel HTTP -> Logging -> CSP -> BodyLimit -> Readiness -> Caching -> mux
```

### Critical Pitfalls

See [PITFALLS.md](PITFALLS.md) for full analysis with 13 pitfalls (4 critical, 5 moderate, 4 minor).

1. **WriteTimeout kills gRPC streaming** -- Do NOT set `WriteTimeout` or `ReadTimeout` on `http.Server`. Use `ReadHeaderTimeout` + `IdleTimeout` only. ConnectRPC streaming RPCs require long-lived HTTP/2 connections. The existing per-stream `StreamTimeout` handles application-level timeout.

2. **Compression double-encodes gRPC responses** -- Exclude `application/grpc*` and `application/connect+proto` content types from HTTP compression. ConnectRPC handles its own compression negotiation. Test with `grpcurl` after adding middleware.

3. **CSP blocks CDN assets and GraphiQL** -- The app loads from `cdn.jsdelivr.net`, `unpkg.com`, and uses inline scripts/styles. GraphiQL needs dynamic code generation CSP permissions. Deploy with `Content-Security-Policy-Report-Only` first. Use per-route CSP (more permissive for GraphiQL).

4. **SRI incompatible with @tailwindcss/browser semver ranges** -- `@tailwindcss/browser@4` resolves to different files as versions publish, breaking static hashes. Pin exact versions (`@4.1.3`) before adding SRI. Consider that SRI on Tailwind browser is lower value than on React/GraphiQL since it only generates CSS.

5. **Adding linters produces hundreds of findings** -- Enable one at a time. Use `--new-from-merge-base=main` in CI for incremental enforcement. Configure `exclusions.generated: strict` (already done). Run locally first to assess finding count.

## Implications for Roadmap

Based on research, the milestone should be structured in 7 phases ordered by dependency chain, risk profile, and logical grouping.

### Phase 1: Server Foundation
**Rationale:** Highest-impact security items with zero risk and no dependencies. Three small changes to three existing files. Enables the new config fields that later phases reference.
**Delivers:** HTTP server timeout protection (Slowloris mitigation), SQLite connection pool bounds (fd exhaustion prevention), expanded config validation (fail-fast on invalid config).
**Addresses:** Server timeouts, connection pool, config validation (table stakes items 1, 2, 5 from FEATURES.md).
**Avoids:** Pitfall 1 (WriteTimeout -- use ReadHeaderTimeout only), Pitfall 5 (SQLITE_BUSY -- set pool limits, preserve busy_timeout), Pitfall 11 (config rejects deployments -- defaults match current behavior).

### Phase 2: Request/Response Hardening
**Rationale:** Input validation and body limits are independent, zero-risk changes that harden the request path. SRI completion is a prerequisite for CSP (Phase 3). Group these because they share a theme (request-path security) and have no cross-dependencies.
**Delivers:** Request body size limits (DoS prevention), input validation (ASN range, width bounds, ID positivity), SRI hashes on all 5 missing CDN assets.
**Addresses:** Body limits, input validation, SRI attributes (table stakes items 3, 4, 7 from FEATURES.md).
**Avoids:** Pitfall 6 (body limit breaks GraphQL -- use 1MB global limit, generous for any legitimate query), Pitfall 4 (SRI + Tailwind -- pin exact CDN versions).

### Phase 3: Security Headers and Compression
**Rationale:** CSP depends on SRI completion (Phase 2). Compression interacts with the caching middleware's ETag generation. Both are new middleware additions that modify the response path. Group them because they both modify the middleware chain and need careful testing.
**Delivers:** Content-Security-Policy header (XSS prevention), gzip/zstd compression (60-80% bandwidth reduction).
**Addresses:** CSP header, compression middleware (differentiator items 1, 2 from FEATURES.md).
**Avoids:** Pitfall 2 (compression double-encodes gRPC -- exclude content types), Pitfall 3 (CSP blocks CDN -- deploy Report-Only first), Pitfall 7 (per-route CSP -- separate GraphiQL policy).

### Phase 4: Internal Quality
**Rationale:** Self-contained code quality improvements with no external-facing changes. GraphQL sentinel errors fix a GO-ERR-2 violation. Metrics caching eliminates unnecessary DB load. Go 1.26 cleanup removes custom code.
**Delivers:** GraphQL error classification via sentinel errors, metrics COUNT query caching (zero-query scrapes), `slog.NewMultiHandler` replacement of custom fanoutHandler.
**Addresses:** GraphQL sentinel errors, metrics caching, Go 1.26 improvements (table stakes item 6, differentiator item 3 from FEATURES.md).
**Avoids:** Pitfall 9 (stale metrics -- invalidate on sync completion, not timer-based TTL).

### Phase 5: Test Coverage
**Rationale:** Tests should be written after the code they test is stable. GraphQL handler tests validate the sentinel error migration from Phase 4. Database tests verify pragmas are actually applied (WAL, FK, busy_timeout). Tests before refactoring (Phase 6) provide a safety net.
**Delivers:** Test files for `internal/graphql/` and `internal/database/` (both currently at zero coverage).
**Addresses:** Test coverage (differentiator items 7, 8 from FEATURES.md).
**Avoids:** Pitfall 10 (refactoring breaks routes -- tests provide safety net for Phase 6).

### Phase 6: Refactoring
**Rationale:** File reorganization is safest after tests exist (Phase 5) and before linters are added (Phase 7). detail.go (1422 LOC) splits into 7+ files with no logic changes. Upsert batch loop extraction saves ~100 lines.
**Delivers:** `detail.go` split into per-entity files, upsert batch loop extracted to generic helper.
**Addresses:** Refactor detail.go, refactor sync upsert (differentiator items 11, 12 from FEATURES.md).
**Avoids:** Pitfall 10 (fragment routes break -- extract one entity at a time, run tests after each), Pitfall 13 (insert order changes -- test against empty database).

### Phase 7: CI and Linting
**Rationale:** Linters go last to avoid lint churn on code being refactored. Docker build catches Dockerfile errors before deploy. HEALTHCHECK enables local Docker health monitoring. Group all CI/tooling changes together.
**Delivers:** Three new linters (exhaustive, contextcheck, gosec), Docker build in CI, Dockerfile HEALTHCHECK with Go-based health binary.
**Addresses:** Linter additions, Docker CI build, Dockerfile HEALTHCHECK (differentiator items 5, 6, 9 from FEATURES.md).
**Avoids:** Pitfall 8 (linter flood -- enable one at a time, use --new-from-merge-base), Pitfall 12 (Docker/Fly.io health conflict -- use /healthz liveness endpoint, generous start-period).

### Phase Ordering Rationale

- **Dependency chain drives order:** Server config (Phase 1) must precede middleware additions (Phase 3) because config fields feed timeout values. SRI (Phase 2) must precede CSP (Phase 3) because CSP policy depends on knowing which assets have SRI. Sentinel errors (Phase 4) should precede GraphQL tests (Phase 5). Tests (Phase 5) should precede refactoring (Phase 6). Refactoring (Phase 6) should precede linters (Phase 7).
- **Risk isolation:** Each phase has a clean rollback boundary. Phases 1-2 are zero-risk additive changes. Phase 3 introduces the highest-risk items (CSP, compression) but with documented mitigation (Report-Only, content-type exclusion). Phase 6 is the riskiest code change (1422 LOC split) but arrives after test coverage exists.
- **Pitfall avoidance:** The ordering avoids the anti-pattern of refactoring + linting simultaneously (Pitfall 8, Architecture anti-pattern 4), and ensures security fundamentals land before the more complex middleware additions.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 3 (Security Headers and Compression):** CSP policy must be tested against all 5 API surfaces. Compression middleware ETag interaction with caching middleware needs careful verification. GraphiQL's CSP requirements (dynamic code generation keyword) need per-route handling.
- **Phase 6 (Refactoring):** detail.go split requires auditing all fragment route registrations. Upsert refactoring needs empty-database integration testing.

Phases with standard patterns (skip research-phase):
- **Phase 1 (Server Foundation):** stdlib configuration, well-documented patterns, zero ambiguity.
- **Phase 2 (Request/Response Hardening):** `http.MaxBytesReader`, input validation, SRI hashes -- all straightforward.
- **Phase 4 (Internal Quality):** sentinel errors and cache invalidation are standard Go patterns.
- **Phase 5 (Test Coverage):** table-driven tests per GO-T-1, no research needed.
- **Phase 7 (CI and Linting):** golangci-lint config and Docker GitHub Actions are well-documented.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All technologies are stdlib or already in the dependency graph. Go 1.26 features verified against release notes. No new dependencies introduced except promoting one indirect to direct. |
| Features | HIGH | Feature set derived from codebase audit of actual missing capabilities. Priority ordering based on risk/reward analysis with dependency chains validated against code. |
| Architecture | HIGH | Integration points verified against actual file locations and line numbers. Middleware chain ordering rationale grounded in HTTP semantics (CORS before compression, compression before OTel). |
| Pitfalls | HIGH | Critical pitfalls (WriteTimeout, compression double-encoding, CSP blocking) backed by ConnectRPC deployment docs, gRPC GitHub issues, and MDN CSP reference. Prevention strategies are specific and actionable. |

**Overall confidence:** HIGH

All four research files cite authoritative sources (Go stdlib docs, Cloudflare engineering blog, ConnectRPC deployment docs, MDN, OWASP). The codebase is mature and well-understood -- research was able to reference specific file paths, line numbers, and existing patterns. No speculative recommendations were made.

### Gaps to Address

- **Compression + ETag interaction:** STACK.md recommends compression after CORS and before OTel; ARCHITECTURE.md recommends compression after caching and before mux. The correct position depends on whether ETags should reflect compressed or uncompressed content. `gzhttp.SuffixETag()` handles this, but needs validation during Phase 3 planning.
- **SQLite pool size disagreement:** STACK.md recommends `MaxOpenConns(4)`, FEATURES.md recommends 25, ARCHITECTURE.md recommends 10. The correct value depends on actual concurrent query load. Recommendation: start with 10, instrument connection pool metrics via `db.Stats()`, and tune based on production data.
- **Body size limit scope:** STACK.md recommends a global 1MB middleware. PITFALLS.md recommends per-route limits (256KB for GraphQL, 64KB for REST, skip ConnectRPC). The global approach is simpler and 1MB is generous for all legitimate payloads. Recommendation: start global at 1MB; add per-route limits only if specific endpoint abuse emerges.
- **Tailwind SRI feasibility:** Pitfall 4 identifies that `@tailwindcss/browser@4` semver range makes SRI unreliable. Need to verify whether pinning an exact version (e.g., `@4.1.3`) produces a stable hash. If not, self-hosting may be required for that one asset.
- **CORS preflight caching:** Already implemented (`MaxAge: 86400`). Both FEATURES.md and ARCHITECTURE.md confirm this is a verify-only task. No work needed beyond a quick confirmation.

## Sources

### Primary (HIGH confidence)
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26) -- slog.NewMultiHandler, io.ReadAll performance
- [Go database/sql: Managing connections](https://go.dev/doc/database/manage-connections) -- pool configuration
- [Cloudflare: Complete Guide to Go net/http Timeouts](https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/) -- timeout semantics
- [ConnectRPC Deployment docs](https://connectrpc.com/docs/go/deployment/) -- streaming timeout warnings
- [MDN: Content Security Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/Guides/CSP) -- CSP directives
- [OWASP: CSP Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Content_Security_Policy_Cheat_Sheet.html) -- CSP deployment strategy
- [golangci-lint v2 Linters](https://golangci-lint.run/docs/linters/) -- linter configuration
- [klauspost/compress/gzhttp](https://pkg.go.dev/github.com/klauspost/compress/gzhttp) -- compression middleware

### Secondary (MEDIUM confidence)
- [Alex Edwards: Configuring sql.DB](https://www.alexedwards.net/blog/configuring-sqldb) -- pool tuning
- [Gopher Academy: Exposing Go on the Internet](https://blog.gopheracademy.com/advent-2016/exposing-go-on-the-internet/) -- production hardening
- [Alex Edwards: http.ResponseController](https://www.alexedwards.net/blog/how-to-use-the-http-responsecontroller-type) -- per-request deadlines
- [Docker: Test before push](https://docs.docker.com/build/ci/github-actions/test-before-push/) -- CI Docker build
- [grpc-go #3884: Server streams never close with HTTP mux timeout](https://github.com/grpc/grpc-go/issues/3884) -- WriteTimeout kills streaming

### Tertiary (LOW confidence)
- None. All recommendations are backed by multiple sources or official documentation.

---
*Research completed: 2026-04-02*
*Ready for roadmap: yes*
