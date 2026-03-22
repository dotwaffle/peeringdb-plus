# Roadmap: PeeringDB Plus

## Overview

PeeringDB Plus delivers fast, globally distributed, read-only access to PeeringDB data. The v1 roadmap moves through three phases: first, model and sync all PeeringDB data into SQLite using entgo; second, expose that data through a GraphQL API with filtering, pagination, and relationship traversal; third, add OpenTelemetry observability and deploy globally on Fly.io with LiteFS for edge-local reads.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: Data Foundation** - Model all PeeringDB objects in entgo, store in SQLite, sync via full re-fetch
- [ ] **Phase 2: GraphQL API** - Expose all data through GraphQL with filtering, pagination, and relationship traversal
- [ ] **Phase 3: Production Readiness** - OpenTelemetry observability, health checks, and global edge deployment on Fly.io

## Phase Details

### Phase 1: Data Foundation
**Goal**: All PeeringDB data is modeled, stored locally, and kept fresh via automated sync
**Depends on**: Nothing (first phase)
**Requirements**: DATA-01, DATA-02, DATA-03, DATA-04, STOR-01
**Success Criteria** (what must be TRUE):
  1. All 13 PeeringDB object types exist as entgo schemas with fields matching actual PeeringDB API responses (not their buggy OpenAPI spec)
  2. A full sync from PeeringDB populates a local SQLite database with all objects, handling deleted/status-filtered objects correctly
  3. Sync can be triggered on-demand and runs automatically on an hourly schedule
  4. After sync completes, querying the local database returns the same data as querying PeeringDB directly
**Plans**: TBD

Plans:
- [ ] 01-01: TBD
- [ ] 01-02: TBD

### Phase 2: GraphQL API
**Goal**: Users can query all PeeringDB data through a GraphQL API with rich filtering and relationship traversal
**Depends on**: Phase 1
**Requirements**: API-01, API-02, API-03, API-04, API-05, API-06, API-07, OPS-06
**Success Criteria** (what must be TRUE):
  1. A user can query any of the 13 PeeringDB object types via GraphQL and get correct results
  2. A user can traverse relationships in a single query (e.g., fetch an IX, its networks, and those networks' facilities)
  3. A user can filter results by any field, look up by ASN or ID, and paginate through large result sets
  4. A user can open the interactive GraphQL playground in a browser and execute queries against the API
  5. Browser-based clients can access the API without CORS errors
**Plans**: TBD

Plans:
- [ ] 02-01: TBD
- [ ] 02-02: TBD

### Phase 3: Production Readiness
**Goal**: The system is observable, health-monitored, and serving from edge nodes worldwide with low latency
**Depends on**: Phase 2
**Requirements**: OPS-01, OPS-02, OPS-03, OPS-04, OPS-05, STOR-02
**Success Criteria** (what must be TRUE):
  1. All requests produce OpenTelemetry traces, and key operations emit metrics and structured logs
  2. A health endpoint reports whether the system is ready to serve and how fresh the synced data is
  3. The application runs on Fly.io with LiteFS replicating data to multiple regions
  4. A user querying from a different continent gets responses from a nearby edge node with low latency
**Plans**: TBD

Plans:
- [ ] 03-01: TBD
- [ ] 03-02: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Data Foundation | 0/0 | Not started | - |
| 2. GraphQL API | 0/0 | Not started | - |
| 3. Production Readiness | 0/0 | Not started | - |
