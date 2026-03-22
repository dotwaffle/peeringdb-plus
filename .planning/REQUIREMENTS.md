# Requirements: PeeringDB Plus

**Defined:** 2026-03-22
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.1 Requirements

Requirements for v1.1 REST API & Observability milestone. Each maps to roadmap phases.

### Observability

- [ ] **OBS-01**: OTel MeterProvider is initialized alongside existing TracerProvider
- [ ] **OBS-02**: PeeringDB HTTP client calls produce OTel trace spans with semantic conventions
- [ ] **OBS-03**: Sync worker records values for all registered sync metrics (duration, operations)
- [ ] **OBS-04**: Per-type sync metrics track duration, object count, and delete count for each of the 13 PeeringDB types

### REST API

- [ ] **REST-01**: All 13 PeeringDB types are queryable via entrest-generated read-only REST endpoints at /rest/
- [ ] **REST-02**: OpenAPI specification is served at /rest/openapi.json
- [ ] **REST-03**: REST endpoints support query parameter filtering and sorting via entrest annotations
- [ ] **REST-04**: REST endpoints support relationship eager-loading via entrest annotations

### PeeringDB Compatibility

- [ ] **PDBCOMPAT-01**: PeeringDB URL paths (/api/net, /api/ix, /api/fac, etc.) return data in PeeringDB's response envelope format ({data:[], meta:{}})
- [ ] **PDBCOMPAT-02**: Django-style query filters (__contains, __startswith, __in, __lt, __gt, __lte, __gte) work on string and numeric fields
- [ ] **PDBCOMPAT-03**: Depth parameter (?depth=0|2) controls relationship expansion in responses
- [ ] **PDBCOMPAT-04**: Since parameter (?since=) returns only objects updated after the given timestamp
- [ ] **PDBCOMPAT-05**: Pagination via limit/skip query parameters matches PeeringDB behavior

## Future Requirements

### gRPC API

- **GRPC-01**: All 13 PeeringDB types queryable via gRPC (entproto)
- **GRPC-02**: Protobuf schema generated from ent schemas

### Web UI

- **UI-01**: Browse PeeringDB data via web interface (HTMX + Templ)
- **UI-02**: Search and filter across object types

## Out of Scope

| Feature | Reason |
|---------|--------|
| Write operations via REST | Read-only mirror — all data comes from PeeringDB sync |
| Authentication / API keys | Fully public service, no auth needed for v1.x |
| PeeringDB depth=1 (ID-only sets) | Uncommon usage, high complexity for low value — support depth=0 and depth=2 only |
| PeeringDB depth=3+ (deep nesting) | PeeringDB itself limits depth; depth=2 covers typical use cases |
| Rate limiting on REST API | Defer to infrastructure layer (Fly.io) rather than application |
| Mobile app | Web-first |
| Real-time streaming | Periodic sync is sufficient |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| OBS-01 | Phase 4 | Pending |
| OBS-02 | Phase 4 | Pending |
| OBS-03 | Phase 4 | Pending |
| OBS-04 | Phase 4 | Pending |
| REST-01 | Phase 5 | Pending |
| REST-02 | Phase 5 | Pending |
| REST-03 | Phase 5 | Pending |
| REST-04 | Phase 5 | Pending |
| PDBCOMPAT-01 | Phase 6 | Pending |
| PDBCOMPAT-02 | Phase 6 | Pending |
| PDBCOMPAT-03 | Phase 6 | Pending |
| PDBCOMPAT-04 | Phase 6 | Pending |
| PDBCOMPAT-05 | Phase 6 | Pending |

**Coverage:**
- v1.1 requirements: 13 total
- Mapped to phases: 13
- Unmapped: 0

---
*Requirements defined: 2026-03-22*
*Last updated: 2026-03-22 after roadmap creation*
