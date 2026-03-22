# Requirements: PeeringDB Plus

**Defined:** 2026-03-22
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### Data

- [ ] **DATA-01**: Mirror all 13 PeeringDB object types (org, net, fac, ix, poc, ixlan, ixpfx, netixlan, netfac, carrier, carrierfac, campus)
- [ ] **DATA-02**: All fields per object match PeeringDB's actual API responses (not their buggy OpenAPI spec)
- [ ] **DATA-03**: Handle deleted/status-filtered objects correctly
- [ ] **DATA-04**: Full re-fetch sync runs hourly or on-demand

### Storage

- [ ] **STOR-01**: Data stored in SQLite using entgo ORM
- [ ] **STOR-02**: Deploy on Fly.io with LiteFS for global edge reads

### API

- [ ] **API-01**: GraphQL API exposing all PeeringDB objects via entgql
- [ ] **API-02**: Relationship traversal in single GraphQL query (e.g., networks at an IX with their facilities)
- [ ] **API-03**: Filter by any field (equality matching)
- [ ] **API-04**: Lookup by ASN
- [ ] **API-05**: Lookup by ID
- [ ] **API-06**: Pagination (limit/skip)
- [ ] **API-07**: Interactive GraphQL playground

### Operations

- [ ] **OPS-01**: OpenTelemetry tracing throughout
- [ ] **OPS-02**: OpenTelemetry metrics throughout
- [ ] **OPS-03**: OpenTelemetry structured logging (slog)
- [ ] **OPS-04**: Health/readiness endpoints with sync age check
- [ ] **OPS-05**: Expose last sync timestamp
- [ ] **OPS-06**: CORS headers for browser integrations

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Additional API Surfaces

- **APIV2-01**: REST API with OpenAPI spec via entrest
- **APIV2-02**: gRPC API via entproto
- **APIV2-03**: gRPC reflection for schema discovery

### Advanced Querying

- **QRYV2-01**: Numeric query modifiers (__lt, __lte, __gt, __gte, __in)
- **QRYV2-02**: String query modifiers (__contains, __startswith, __in)
- **QRYV2-03**: Full-text search across objects (SQLite FTS5)
- **QRYV2-04**: Cross-object queries (networks at both IX-A and IX-B)
- **QRYV2-05**: `since` parameter for downstream incremental sync
- **QRYV2-06**: Field selection for REST/gRPC responses

### Data Presentation

- **PRESV2-01**: Structured data exports (JSON, CSV bulk downloads)
- **PRESV2-02**: Generated client SDKs from OpenAPI and protobuf specs

### Web UI

- **UIV2-01**: Browse and search PeeringDB data via HTMX + Templ web interface
- **UIV2-02**: Network/facility/IX detail pages with related data
- **UIV2-03**: Visual network comparison (shared IXPs/facilities)

### Advanced Features

- **ADVV2-01**: Geographic queries (facilities within X km)
- **ADVV2-02**: Webhook/callback on data changes
- **ADVV2-03**: Query performance metrics (p50/p95/p99 per endpoint)

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Write/mutation API | Read-only mirror; PeeringDB is the authoritative source |
| User accounts and authentication | Adds complexity; value prop is fast public access |
| OAuth / social login | Unnecessary for read-only public data |
| Real-time streaming / WebSockets | Hourly sync granularity is sufficient for peering decisions |
| Drop-in PeeringDB API compatibility | Would inherit their bugs and constrain our API design |
| Mobile app | Web UI handles mobile adequately; operators work from desktops |
| Historical data / time-series | Different product; CAIDA already does this |
| Data quality validation / correction | Mirror faithfully reproduces data; PeeringDB's job to validate |
| Email notifications | Requires email infrastructure and user accounts |
| Rate limiting matching PeeringDB | Whole point is to be faster and more accessible |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| DATA-01 | Phase 1 | Pending |
| DATA-02 | Phase 1 | Pending |
| DATA-03 | Phase 1 | Pending |
| DATA-04 | Phase 1 | Pending |
| STOR-01 | Phase 1 | Pending |
| API-01 | Phase 2 | Pending |
| API-02 | Phase 2 | Pending |
| API-03 | Phase 2 | Pending |
| API-04 | Phase 2 | Pending |
| API-05 | Phase 2 | Pending |
| API-06 | Phase 2 | Pending |
| API-07 | Phase 2 | Pending |
| OPS-06 | Phase 2 | Pending |
| OPS-01 | Phase 3 | Pending |
| OPS-02 | Phase 3 | Pending |
| OPS-03 | Phase 3 | Pending |
| OPS-04 | Phase 3 | Pending |
| OPS-05 | Phase 3 | Pending |
| STOR-02 | Phase 3 | Pending |

**Coverage:**
- v1 requirements: 19 total
- Mapped to phases: 19
- Unmapped: 0

---
*Requirements defined: 2026-03-22*
*Last updated: 2026-03-22 after roadmap creation*
