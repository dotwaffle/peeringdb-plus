# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 PeeringDB Plus** — Phases 1-3 (shipped 2026-03-22)
- 🚧 **v1.1 REST API & Observability** — Phases 4-6 (in progress)

## Phases

<details>
<summary>✅ v1.0 PeeringDB Plus (Phases 1-3) — SHIPPED 2026-03-22</summary>

- [x] Phase 1: Data Foundation (7/7 plans) — completed 2026-03-22
- [x] Phase 2: GraphQL API (4/4 plans) — completed 2026-03-22
- [x] Phase 3: Production Readiness (3/3 plans) — completed 2026-03-22

See: `.planning/milestones/v1.0-ROADMAP.md` for full details.

</details>

### 🚧 v1.1 REST API & Observability (In Progress)

**Milestone Goal:** Fix observability gaps from v1.0 and add OpenAPI REST API with full PeeringDB compatibility.

- [ ] **Phase 4: Observability Foundations** - Complete OTel instrumentation with HTTP client tracing and sync metrics
- [ ] **Phase 5: entrest REST API** - Code-generated read-only REST API with OpenAPI spec for all 13 PeeringDB types
- [ ] **Phase 6: PeeringDB Compatibility Layer** - Drop-in PeeringDB API replacement with exact paths, envelope, and query filters

## Phase Details

### Phase 4: Observability Foundations
**Goal**: Operators can observe PeeringDB sync behavior through traces and metrics -- every HTTP call is traced, every sync step is measured
**Depends on**: Phase 3 (v1.0 complete)
**Requirements**: OBS-01, OBS-02, OBS-03, OBS-04
**Success Criteria** (what must be TRUE):
  1. OTel MeterProvider is initialized and metric recordings produce real values (not silently dropped to no-op)
  2. Every outbound HTTP request to PeeringDB produces an OTel trace span with object type and page attributes
  3. After a sync completes, per-type duration, object count, and delete count metrics are recorded for each of the 13 PeeringDB types
  4. Sync-level duration and operation count metrics are recorded and visible in any OTel-compatible metrics backend
**Plans**: TBD

### Phase 5: entrest REST API
**Goal**: All PeeringDB data is queryable through a modern, auto-documented REST API with filtering, sorting, and relationship loading
**Depends on**: Phase 4
**Requirements**: REST-01, REST-02, REST-03, REST-04
**Success Criteria** (what must be TRUE):
  1. A GET request to /rest/{type} returns paginated JSON for any of the 13 PeeringDB object types
  2. A GET request to /rest/openapi.json returns a valid OpenAPI specification describing all endpoints
  3. Query parameters on REST endpoints filter and sort results (e.g., /rest/networks?name=Cloudflare)
  4. Relationship edges can be eager-loaded via query parameters (e.g., include an organization's networks in a single response)
**Plans**: TBD

### Phase 6: PeeringDB Compatibility Layer
**Goal**: Existing PeeringDB API consumers can point at this service and get identical response behavior -- same paths, same envelope, same query filters
**Depends on**: Phase 5
**Requirements**: PDBCOMPAT-01, PDBCOMPAT-02, PDBCOMPAT-03, PDBCOMPAT-04, PDBCOMPAT-05
**Success Criteria** (what must be TRUE):
  1. A GET to /api/net returns a JSON response with PeeringDB's exact envelope format ({"meta": {}, "data": [...]}) containing network objects
  2. Django-style query filters (__contains, __startswith, __in, __lt, __gt, __lte, __gte) filter results on string and numeric fields
  3. The depth parameter controls relationship expansion: depth=0 returns flat objects, depth=2 returns objects with nested related data
  4. The since parameter returns only objects updated after the given Unix timestamp
  5. Pagination via limit and skip query parameters controls result windows, matching PeeringDB's behavior

**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 4 -> 5 -> 6

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Data Foundation | v1.0 | 7/7 | Complete | 2026-03-22 |
| 2. GraphQL API | v1.0 | 4/4 | Complete | 2026-03-22 |
| 3. Production Readiness | v1.0 | 3/3 | Complete | 2026-03-22 |
| 4. Observability Foundations | v1.1 | 0/? | Not started | - |
| 5. entrest REST API | v1.1 | 0/? | Not started | - |
| 6. PeeringDB Compatibility Layer | v1.1 | 0/? | Not started | - |
