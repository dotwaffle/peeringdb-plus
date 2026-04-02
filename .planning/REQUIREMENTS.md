# Requirements: PeeringDB Plus

**Defined:** 2026-04-02
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.12 Requirements

Requirements for the Hardening & Tech Debt milestone. Each maps to roadmap phases.

### Server Hardening

- [ ] **SRVR-01**: Server sets ReadHeaderTimeout (10s) and IdleTimeout (120s) to prevent connection exhaustion
- [ ] **SRVR-02**: SQLite connection pool configured with MaxOpenConns, MaxIdleConns, and ConnMaxLifetime
- [ ] **SRVR-03**: Config validates ListenAddr format, PeeringDBBaseURL as valid URL, and DrainTimeout > 0 at startup
- [ ] **SRVR-04**: POST endpoints enforce request body size limits via http.MaxBytesReader

### Security

- [ ] **SEC-01**: ASN input validated to 0 < ASN < 4294967296 range in all web handlers
- [ ] **SEC-02**: Width query parameter (?w=) bounded to a reasonable maximum
- [ ] **SEC-03**: Content-Security-Policy-Report-Only header served with CDN allowlist on web UI responses

### Performance

- [ ] **PERF-01**: HTTP responses compressed via gzip middleware, excluding gRPC content types
- [ ] **PERF-02**: Metrics type count gauge reads cached values computed at sync time, not per-scrape COUNT queries
- [ ] **PERF-03**: GraphQL error presenter classifies errors via ent.IsNotFound / errors.Is instead of string matching

### Quality

- [ ] **QUAL-01**: internal/graphql/handler.go has test coverage for error classification and complexity limits
- [ ] **QUAL-02**: internal/database/database.go has test coverage for Open() pragmas and error paths
- [ ] **QUAL-03**: golangci-lint config enables exhaustive, contextcheck, and gosec linters with clean pass
- [ ] **QUAL-04**: CI pipeline builds both Dockerfiles and fails on build errors

### Refactoring

- [ ] **REFAC-01**: internal/web/detail.go split into focused per-entity query helpers
- [ ] **REFAC-02**: internal/sync/upsert.go duplication reduced via generic bulk upsert pattern

### Tech Debt

- [ ] **DEBT-01**: /ui/about renders properly for terminal clients (no stub fallthrough)
- [ ] **DEBT-02**: seed.Minimal and seed.Networks consolidated (unused exports removed)

## Future Requirements

Deferred to future milestones.

### Server-Side Sorting

- **SSORT-01**: Server-side table sorting via htmx for paginated datasets exceeding client-side limits

### Map Enhancements

- **MAPE-01**: Map on search results page showing locations of results
- **MAPE-02**: Drawing tools / measurement on map for distance calculation

### Cross-Surface Consistency

- **XSURF-01**: Same entity returns identical data across GraphQL, REST, PeeringDB compat, and gRPC surfaces
- **XSURF-02**: Golden file tests for gRPC responses (after filter coverage is complete)

### CDN Security

- **CDNS-01**: Subresource Integrity (SRI) attributes on all CDN-loaded assets with pinned versions

### Operational Verification

- **OPVR-01**: fly_region Grafana template variable verified against live multi-region deployment
- **OPVR-02**: Go runtime metric names verified against live Grafana Cloud
- **OPVR-03**: CI coverage pipeline verified on actual GitHub Actions run

## Out of Scope

| Feature | Reason |
|---------|--------|
| SRI on CDN assets | Tailwind browser CDN may not support SRI; pin versions first in future milestone |
| Dockerfile HEALTHCHECK | Fly.io handles health checks via fly.toml; Docker-only adds minimal value |
| Grafana variable verification | Requires live multi-region deployment; defer to operational validation |
| CI coverage pipeline verification | Requires live GitHub Actions run; not automatable in this milestone |
| WriteTimeout on http.Server | Kills streaming RPCs; ReadHeaderTimeout + IdleTimeout sufficient |
| CORS preflight caching | Already implemented (MaxAge: 86400) |
| Rate limiting middleware | Fly.io proxy provides connection-level limits; app-level adds complexity without clear need |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| SRVR-01 | — | Pending |
| SRVR-02 | — | Pending |
| SRVR-03 | — | Pending |
| SRVR-04 | — | Pending |
| SEC-01 | — | Pending |
| SEC-02 | — | Pending |
| SEC-03 | — | Pending |
| PERF-01 | — | Pending |
| PERF-02 | — | Pending |
| PERF-03 | — | Pending |
| QUAL-01 | — | Pending |
| QUAL-02 | — | Pending |
| QUAL-03 | — | Pending |
| QUAL-04 | — | Pending |
| REFAC-01 | — | Pending |
| REFAC-02 | — | Pending |
| DEBT-01 | — | Pending |
| DEBT-02 | — | Pending |

**Coverage:**
- v1.12 requirements: 18 total
- Mapped to phases: 0
- Unmapped: 18 (pending roadmap creation)

---
*Requirements defined: 2026-04-02*
*Last updated: 2026-04-02 after initial definition*
