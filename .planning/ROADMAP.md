# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 PeeringDB Plus** — Phases 1-3 (shipped 2026-03-22)
- ✅ **v1.1 REST API & Observability** — Phases 4-6 (shipped 2026-03-23)
- ✅ **v1.2 Quality, Incremental Sync & CI** — Phases 7-10 (shipped 2026-03-24)
- ✅ **v1.3 PeeringDB API Key Support** — Phases 11-12 (shipped 2026-03-24)
- ✅ **v1.4 Web UI** — Phases 13-17 (shipped 2026-03-24)
- ✅ **v1.5 Tech Debt & Observability** — Phases 18-20 (shipped 2026-03-24)
- 🚧 **v1.6 ConnectRPC / gRPC API** — Phases 21-24 (in progress)

## Phases

<details>
<summary>✅ v1.0 PeeringDB Plus (Phases 1-3) — SHIPPED 2026-03-22</summary>

- [x] Phase 1: Data Foundation (7/7 plans) — completed 2026-03-22
- [x] Phase 2: GraphQL API (4/4 plans) — completed 2026-03-22
- [x] Phase 3: Production Readiness (3/3 plans) — completed 2026-03-22

See: `.planning/milestones/v1.0-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.1 REST API & Observability (Phases 4-6) — SHIPPED 2026-03-23</summary>

- [x] Phase 4: Observability Foundations (2/2 plans) — completed 2026-03-22
- [x] Phase 5: entrest REST API (3/3 plans) — completed 2026-03-22
- [x] Phase 6: PeeringDB Compatibility Layer (3/3 plans) — completed 2026-03-22

See: `.planning/milestones/v1.1-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.2 Quality, Incremental Sync & CI (Phases 7-10) — SHIPPED 2026-03-24</summary>

- [x] Phase 7: Lint & Code Quality (2/2 plans) — completed 2026-03-23
- [x] Phase 8: Incremental Sync (3/3 plans) — completed 2026-03-23
- [x] Phase 9: Golden File Tests & Conformance (2/2 plans) — completed 2026-03-23
- [x] Phase 10: CI Pipeline & Public Access (2/2 plans) — completed 2026-03-24

See: `.planning/milestones/v1.2-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.3 PeeringDB API Key Support (Phases 11-12) — SHIPPED 2026-03-24</summary>

- [x] Phase 11: API Key & Rate Limiting (2/2 plans) — completed 2026-03-24
- [x] Phase 12: Conformance Tooling Integration (1/1 plan) — completed 2026-03-24

See: `.planning/milestones/v1.3-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.4 Web UI (Phases 13-17) — SHIPPED 2026-03-24</summary>

- [x] Phase 13: Foundation (2/2 plans) — completed 2026-03-24
- [x] Phase 14: Live Search (2/2 plans) — completed 2026-03-24
- [x] Phase 15: Record Detail Pages (2/2 plans) — completed 2026-03-24
- [x] Phase 16: ASN Comparison (2/2 plans) — completed 2026-03-24
- [x] Phase 17: Polish & Accessibility (3/3 plans) — completed 2026-03-24

See: `.planning/milestones/v1.4-ROADMAP.md` for full details.

</details>

<details>
<summary>✅ v1.5 Tech Debt & Observability (Phases 18-20) — SHIPPED 2026-03-24</summary>

- [x] Phase 18: Tech Debt & Data Integrity (2/2 plans) — completed 2026-03-24
- [x] Phase 19: Prometheus Metrics & Grafana Dashboard (4/4 plans) — completed 2026-03-24
- [x] Phase 20: Deferred Human Verification (3/3 plans) — completed 2026-03-24

See: `.planning/milestones/v1.5-ROADMAP.md` for full details.

</details>

### 🚧 v1.6 ConnectRPC / gRPC API (In Progress)

**Milestone Goal:** Expose all PeeringDB data via ConnectRPC, providing gRPC, gRPC-Web, and Connect protocol access with reflection, health checking, and typed filtering.

- [ ] **Phase 21: Infrastructure** - Remove LiteFS proxy, implement fly-replay write forwarding, enable h2c
- [ ] **Phase 22: Proto Generation Pipeline** - Annotate 13 ent schemas, configure buf toolchain, generate protos and ConnectRPC interfaces
- [ ] **Phase 23: ConnectRPC Services** - Get/List RPCs for all 13 types with observability, reflection, and health checking
- [ ] **Phase 24: List Filtering** - Typed filter fields on List RPCs for querying across all 13 types

## Phase Details

### Phase 21: Infrastructure
**Goal**: Application serves traffic directly without LiteFS HTTP proxy, supporting HTTP/2 cleartext for native gRPC wire protocol
**Depends on**: Phase 20
**Requirements**: INFRA-01, INFRA-02, INFRA-03, INFRA-04, INFRA-05
**Success Criteria** (what must be TRUE):
  1. Application listens on Fly.io internal port directly -- LiteFS proxy is no longer in the request path
  2. POST /sync on a replica returns a fly-replay response header routing the request to the primary node (when running on Fly.io)
  3. POST /sync works directly without replay when not running on Fly.io (local dev, tests)
  4. Server accepts both HTTP/1.1 and HTTP/2 cleartext (h2c) connections on the same port
  5. fly.toml is configured with h2_backend so Fly.io edge sends HTTP/2 to the application
**Plans**: TBD

Plans:
- [ ] 21-01: TBD
- [ ] 21-02: TBD

### Phase 22: Proto Generation Pipeline
**Goal**: All 13 PeeringDB types have proto definitions and generated ConnectRPC handler interfaces ready for service implementation
**Depends on**: Phase 21
**Requirements**: PROTO-01, PROTO-02, PROTO-03, PROTO-04
**Success Criteria** (what must be TRUE):
  1. Running `go generate ./ent/...` produces .proto files for all 13 PeeringDB entity types with correct field mappings
  2. Running `buf generate` produces compilable Go types (*.pb.go) and ConnectRPC handler interfaces (*connect/*.go)
  3. `buf lint` passes on all generated proto files
  4. JSON fields (social_media, info_types) that entproto cannot handle have manual proto definitions that compile cleanly
**Plans**: TBD

Plans:
- [ ] 22-01: TBD
- [ ] 22-02: TBD

### Phase 23: ConnectRPC Services
**Goal**: Users can query all 13 PeeringDB types via ConnectRPC with Get and List RPCs, observable via OTel, discoverable via reflection, and monitored via health checks
**Depends on**: Phase 22
**Requirements**: API-01, API-02, API-04, OBS-01, OBS-02, OBS-03, OBS-04
**Success Criteria** (what must be TRUE):
  1. A client can retrieve any single PeeringDB entity by ID using a Get RPC (e.g., `buf curl .../GetNetwork` returns a network)
  2. A client can list entities with pagination using List RPCs (page_size + page_token produce sequential pages)
  3. gRPC server reflection allows grpcurl/grpcui to discover all 13 services and their methods without prior knowledge of the schema
  4. gRPC health check service reports serving status that reflects sync readiness
  5. ConnectRPC requests produce OTel trace spans with RPC-level attributes (rpc.system, rpc.service, rpc.method)
**Plans**: TBD

Plans:
- [ ] 23-01: TBD
- [ ] 23-02: TBD
- [ ] 23-03: TBD

### Phase 24: List Filtering
**Goal**: Users can filter List RPC results using typed fields (ASN, country, name, org_id, status) instead of fetching all records and filtering client-side
**Depends on**: Phase 23
**Requirements**: API-03
**Success Criteria** (what must be TRUE):
  1. A client can filter List results by typed fields (e.g., ListNetworks with asn=15169 returns only that network)
  2. Multiple filter fields can be combined in a single List request (e.g., country=US AND status=ok)
  3. Invalid filter field names or values return a clear INVALID_ARGUMENT error with the offending field identified
**Plans**: TBD

Plans:
- [ ] 24-01: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 21 → 22 → 23 → 24

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. Data Foundation | v1.0 | 7/7 | Complete | 2026-03-22 |
| 2. GraphQL API | v1.0 | 4/4 | Complete | 2026-03-22 |
| 3. Production Readiness | v1.0 | 3/3 | Complete | 2026-03-22 |
| 4. Observability Foundations | v1.1 | 2/2 | Complete | 2026-03-22 |
| 5. entrest REST API | v1.1 | 3/3 | Complete | 2026-03-22 |
| 6. PeeringDB Compatibility Layer | v1.1 | 3/3 | Complete | 2026-03-22 |
| 7. Lint & Code Quality | v1.2 | 2/2 | Complete | 2026-03-23 |
| 8. Incremental Sync | v1.2 | 3/3 | Complete | 2026-03-23 |
| 9. Golden File Tests & Conformance | v1.2 | 2/2 | Complete | 2026-03-23 |
| 10. CI Pipeline & Public Access | v1.2 | 2/2 | Complete | 2026-03-24 |
| 11. API Key & Rate Limiting | v1.3 | 2/2 | Complete | 2026-03-24 |
| 12. Conformance Tooling Integration | v1.3 | 1/1 | Complete | 2026-03-24 |
| 13. Foundation | v1.4 | 2/2 | Complete | 2026-03-24 |
| 14. Live Search | v1.4 | 2/2 | Complete | 2026-03-24 |
| 15. Record Detail Pages | v1.4 | 2/2 | Complete | 2026-03-24 |
| 16. ASN Comparison | v1.4 | 2/2 | Complete | 2026-03-24 |
| 17. Polish & Accessibility | v1.4 | 3/3 | Complete | 2026-03-24 |
| 18. Tech Debt & Data Integrity | v1.5 | 2/2 | Complete | 2026-03-24 |
| 19. Prometheus Metrics & Grafana Dashboard | v1.5 | 4/4 | Complete | 2026-03-24 |
| 20. Deferred Human Verification | v1.5 | 3/3 | Complete | 2026-03-24 |
| 21. Infrastructure | v1.6 | 0/? | Not started | - |
| 22. Proto Generation Pipeline | v1.6 | 0/? | Not started | - |
| 23. ConnectRPC Services | v1.6 | 0/? | Not started | - |
| 24. List Filtering | v1.6 | 0/? | Not started | - |
