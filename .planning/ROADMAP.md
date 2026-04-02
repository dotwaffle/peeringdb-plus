# Roadmap: PeeringDB Plus

## Milestones

- [x] **v1.0 MVP** - Phases 1-3 (shipped 2026-03-22)
- [x] **v1.1 REST API & Observability** - Phases 4-6 (shipped 2026-03-23)
- [x] **v1.2 Quality, Incremental Sync & CI** - Phases 7-10 (shipped 2026-03-24)
- [x] **v1.3 PeeringDB API Key Support** - Phases 11-12 (shipped 2026-03-24)
- [x] **v1.4 Web UI** - Phases 13-17 (shipped 2026-03-24)
- [x] **v1.5 Tech Debt & Observability** - Phases 18-20 (shipped 2026-03-24)
- [x] **v1.6 ConnectRPC / gRPC API** - Phases 21-24 (shipped 2026-03-25)
- [x] **v1.7 Streaming RPCs & UI Polish** - Phases 25-27 (shipped 2026-03-25)
- [x] **v1.8 Terminal CLI Interface** - Phases 28-31 (shipped 2026-03-26)
- [x] **v1.9 Hardening & Polish** - Phases 32-36 (shipped 2026-03-26)
- [x] **v1.10 Code Coverage & Test Quality** - Phases 37-42 (shipped 2026-03-26)
- [x] **v1.11 Web UI Density & Interactivity** - Phases 43-46 (shipped 2026-03-26)
- [ ] **v1.12 Hardening & Tech Debt** - Phases 47-50 (in progress)

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

<details>
<summary>v1.11 Web UI Density & Interactivity (Phases 43-46) - SHIPPED 2026-03-26</summary>

- [x] **Phase 43: Dense Tables with Sorting and Flags** - Convert all detail page child-entity lists to dense sortable tables with country flag columns
- [x] **Phase 44: Facility Map & Map Infrastructure** - Interactive Leaflet map on facility detail pages with dark/light tiles and clickable pins
- [x] **Phase 45: Multi-Pin Maps** - Maps on IX, network, and comparison pages with marker clustering and colored pins
- [x] **Phase 46: Search & Compare Density** - Dense layouts for search results and ASN comparison with country flags

</details>

### v1.12 Hardening & Tech Debt

- [ ] **Phase 47: Server & Request Hardening** - HTTP server timeouts, connection pool limits, body size limits, config validation, and input validation
- [ ] **Phase 48: Response Hardening & Internal Quality** - CSP headers, gzip compression, metrics caching, and GraphQL error classification
- [ ] **Phase 49: Refactoring & Tech Debt** - Split detail.go, extract generic upsert, write tests for graphql/database, clean up tech debt items
- [ ] **Phase 50: CI & Linting** - Additional linters (exhaustive, contextcheck, gosec) and Docker build in CI

## Phase Details

<details>
<summary>v1.11 Web UI Density & Interactivity (Phases 43-46) - SHIPPED 2026-03-26</summary>

### Phase 43: Dense Tables with Sorting and Flags
**Goal**: Users see detail page child-entity lists as information-dense sortable tables with country flags, replacing the current multi-line card layout
**Depends on**: Phase 42
**Requirements**: DENS-01, DENS-02, DENS-03, SORT-01, SORT-02, SORT-03, FLAG-01
**Success Criteria** (what must be TRUE):
  1. Every detail page child-entity list (IX participants, network facilities, IX facilities, org networks, etc.) renders as a `<table>` with columnar layout instead of stacked card divs
  2. User can click any sortable column header (name, ASN, speed, country) and the table re-sorts by that column, with a visible arrow indicating sort direction
  3. User sees parsed city and country in dedicated columns, with an SVG country flag icon (via flag-icons CSS) alongside the country code
  4. On narrow screens (< 768px), low-priority columns (city, speed, etc.) hide automatically instead of causing horizontal scroll
  5. Tables load with a sensible default sort order (IX participants by ASN, facilities by country)
**Plans**: 4/4 plans complete

Plans:
- [x] 43-01-PLAN.md -- Flag-icons CDN, sort JS/CSS, CountryFlag component, FacNetworkRow data enrichment
- [x] 43-02-PLAN.md -- IX, Network, and Facility table conversions (9 templates)
- [x] 43-03-PLAN.md -- Org, Campus, and Carrier table conversions + test updates (7 templates + tests)
- [x] 43-04-PLAN.md -- Gap closure: IX/net/fac fragment test table structure assertions

### Phase 44: Facility Map & Map Infrastructure
**Goal**: Users see an interactive map on facility detail pages showing the facility's geographic location, establishing the map component and CDN infrastructure for all subsequent map work
**Depends on**: Phase 43
**Requirements**: MAP-01, MAP-04, MAP-05
**Success Criteria** (what must be TRUE):
  1. Facility detail pages with populated latitude/longitude display an interactive Leaflet map centered on the facility location with a clickable pin
  2. Clicking the map pin shows a popup with the facility name (and a link back to the detail page when navigated from another context)
  3. The map tile layer switches between CARTO light and dark basemaps matching the current app dark mode setting
  4. Facility detail pages without lat/lng data render normally with no map section (no empty container or error)
**Plans**: 2/2 plans complete

Plans:
- [x] 44-01-PLAN.md -- Leaflet CDN, data plumbing, MapContainer component, facility page integration
- [x] 44-02-PLAN.md -- Map rendering integration tests and popup HTML unit tests

### Phase 45: Multi-Pin Maps
**Goal**: Users see maps with multiple facility pins on IX, network, and ASN comparison pages, with clustering for dense regions and colored pins distinguishing shared vs unique facilities
**Depends on**: Phase 44
**Requirements**: MAP-02, MAP-03
**Success Criteria** (what must be TRUE):
  1. IX detail pages display a map with pins for all associated facilities, and network detail pages display a map with pins for all facility presences
  2. When many pins overlap in the same geographic area, they cluster into a numbered circle that expands on click
  3. The ASN comparison page displays a map with two pin colors distinguishing shared facilities from facilities unique to each network
  4. All multi-pin maps auto-fit bounds to show all pins, and clicking any pin shows a popup with facility name linking to its detail page
**Plans**: 2/2 plans complete

Plans:
- [x] 45-01-PLAN.md -- Markercluster CDN, row struct enrichment, MultiPinMapContainer component, popup helpers
- [x] 45-02-PLAN.md -- Handler query enrichment, IX/network/compare page integration, multi-pin map tests

### Phase 46: Search & Compare Density
**Goal**: Users see search results and ASN comparison tables in a denser layout with country flags, completing the information density overhaul
**Depends on**: Phase 43
**Requirements**: DENS-04, DENS-05, FLAG-02
**Success Criteria** (what must be TRUE):
  1. Search results display country and city information with SVG country flag icons alongside each result entry
  2. ASN comparison results (shared IXPs, shared facilities, shared campuses) render as dense columnar tables consistent with the detail page table style from Phase 43
  3. Search result entries show key metadata (country, city, ASN where applicable) in a compact layout without expanding vertical space per result
**Plans**: 2/2 plans complete

Plans:
- [x] 46-01-PLAN.md -- Search struct enrichment, compact row template with flags and metadata badges
- [x] 46-02-PLAN.md -- Compare IXP/Facility/Campus table conversions with sorting and flags

</details>

#### v1.12 Hardening & Tech Debt

### Phase 47: Server & Request Hardening
**Goal**: The application rejects malformed, oversized, and slow-loris requests at the server level and validates all user-facing inputs before processing
**Depends on**: Phase 46
**Requirements**: SRVR-01, SRVR-02, SRVR-03, SRVR-04, SEC-01, SEC-02
**Success Criteria** (what must be TRUE):
  1. A client that opens a connection but sends headers slowly is disconnected after 10 seconds (ReadHeaderTimeout), and idle connections are reaped after 120 seconds (IdleTimeout)
  2. The SQLite connection pool has explicit MaxOpenConns, MaxIdleConns, and ConnMaxLifetime values visible in startup logs or config output
  3. The application exits with a clear error message at startup if ListenAddr format is invalid, PeeringDBBaseURL is not a valid URL, or DrainTimeout is zero or negative
  4. POST requests with bodies exceeding the configured limit receive a 413 response
  5. Requesting /ui/asn/99999999999 (out of range) or ?w=99999 (out of bounds) returns a user-facing error instead of unexpected behavior
**Plans**: 2 plans

Plans:
- [x] 47-01-PLAN.md -- Server timeouts, SQLite pool config, config validation, POST body limits
- [x] 47-02-PLAN.md -- ASN range validation and width parameter capping

### Phase 48: Response Hardening & Internal Quality
**Goal**: The application serves responses with security headers and compression, and internal error handling uses type-safe patterns instead of string matching
**Depends on**: Phase 47
**Requirements**: SEC-03, PERF-01, PERF-02, PERF-03
**Success Criteria** (what must be TRUE):
  1. Web UI responses include a Content-Security-Policy-Report-Only header with directives allowing the known CDN origins (jsdelivr, unpkg) while reporting violations
  2. HTML and JSON responses are gzip-compressed (verified via Content-Encoding header), while gRPC content types (application/grpc*, application/connect+proto) are NOT compressed by the HTTP middleware
  3. The metrics type count gauge returns cached values computed at sync completion time, not live COUNT queries on each scrape
  4. GraphQL errors for not-found entities return a structured error with appropriate classification, using sentinel error checks (errors.Is, ent.IsNotFound) instead of string matching
**Plans**: 2 plans

Plans:
- [ ] 48-01-PLAN.md -- CSP headers middleware, gzip compression middleware, main.go middleware chain wiring
- [ ] 48-02-PLAN.md -- GraphQL sentinel error classification, metrics count caching with sync worker callback

### Phase 49: Refactoring & Tech Debt
**Goal**: Large files are split for maintainability, duplicated patterns are extracted, untested packages gain coverage, and known tech debt items are resolved
**Depends on**: Phase 48
**Requirements**: REFAC-01, REFAC-02, QUAL-01, QUAL-02, DEBT-01, DEBT-02
**Success Criteria** (what must be TRUE):
  1. internal/web/detail.go is split into focused per-entity files with no single file exceeding 300 lines, and all existing detail page routes continue to work
  2. The sync upsert logic uses a shared generic pattern instead of per-type copy-pasted loops, reducing total line count
  3. internal/graphql/handler.go has test coverage for error classification paths and complexity limit enforcement
  4. internal/database/database.go has test coverage for Open() pragma application and error paths (invalid DSN, pragma failure)
  5. curl /ui/about returns properly formatted terminal output (not a generic stub), and seed.Minimal/seed.Networks exports are consolidated
**Plans**: 4 plans

Plans:
- [ ] 49-01-PLAN.md -- Split detail.go into per-entity query files (query_network.go, query_ix.go, etc.)
- [ ] 49-02-PLAN.md -- Extract generic upsertBatch function from 13 copy-pasted upsert functions
- [ ] 49-03-PLAN.md -- Add test coverage for GraphQL handler limits and database.Open pragmas
- [ ] 49-04-PLAN.md -- About page terminal renderer and seed export consolidation

### Phase 50: CI & Linting
**Goal**: The CI pipeline catches more defect classes via additional linters and validates that Docker images build successfully
**Depends on**: Phase 49
**Requirements**: QUAL-03, QUAL-04
**Success Criteria** (what must be TRUE):
  1. golangci-lint config includes exhaustive, contextcheck, and gosec linters, and `golangci-lint run` passes cleanly on the full codebase
  2. The CI pipeline builds both Dockerfiles (production and dev) and fails the build if either Dockerfile produces an error
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 47 -> 48 -> 49 -> 50

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 47. Server & Request Hardening | 2/2 | Complete    | 2026-04-02 |
| 48. Response Hardening & Internal Quality | 0/2 | Complete    | 2026-04-02 |
| 49. Refactoring & Tech Debt | 0/4 | Not started | - |
| 50. CI & Linting | 0/? | Not started | - |
