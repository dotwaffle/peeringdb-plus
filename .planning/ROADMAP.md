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
- [ ] **v1.11 Web UI Density & Interactivity** - Phases 43-46 (in progress)

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

<details>
<summary>v1.10 Code Coverage & Test Quality (Phases 37-42) - SHIPPED 2026-03-26</summary>

- [x] **Phase 37: Test Seed Infrastructure** - Shared deterministic entity factory package for all 13 PeeringDB types
- [x] **Phase 38: GraphQL Resolver Coverage** - Integration tests for all 13 list resolvers and custom resolver error paths
- [x] **Phase 39: gRPC Handler Coverage** - Filter, streaming, and branch coverage for all 13 entity types
- [x] **Phase 40: Web Handler Coverage** - Fragment handler, multi-mode dispatch, and edge case tests
- [x] **Phase 41: Schema & Minor Package Coverage** - Schema hook/constraint tests plus otel, health, and peeringdb error path tests
- [x] **Phase 42: Test Quality Audit & Coverage Hygiene** - Assertion density audit, error path coverage, fuzz tests, and CI coverage filtering

</details>

- [ ] **Phase 43: Dense Tables with Sorting and Flags** - Convert all detail page child-entity lists to dense sortable tables with country flag columns
- [ ] **Phase 44: Facility Map & Map Infrastructure** - Interactive Leaflet map on facility detail pages with dark/light tiles and clickable pins
- [ ] **Phase 45: Multi-Pin Maps** - Maps on IX, network, and comparison pages with marker clustering and colored pins
- [ ] **Phase 46: Search & Compare Density** - Dense layouts for search results and ASN comparison with country flags

## Phase Details

<details>
<summary>v1.10 Code Coverage & Test Quality (Phases 37-42) - SHIPPED 2026-03-26</summary>

### Phase 37: Test Seed Infrastructure
**Goal**: Any test file in the project can create a fully-populated database with all 13 PeeringDB types by calling a single function
**Depends on**: Phase 36
**Requirements**: INFRA-01
**Success Criteria** (what must be TRUE):
  1. Calling `seed.Full(t, client)` creates at least one entity of each of the 13 PeeringDB types with realistic field values and correct FK relationships
  2. Calling `seed.Minimal(t, client)` creates the minimum entity graph needed for relationship traversal
  3. Tests in at least 3 different packages can import and use the seed package without import cycles
**Plans:** 1/1 plans complete

Plans:
- [x] 37-01-PLAN.md -- Seed package (Full/Minimal/Networks) with TDD + import-cycle validation

### Phase 38: GraphQL Resolver Coverage
**Goal**: Hand-written GraphQL resolver code is tested to 80%+ coverage
**Depends on**: Phase 37
**Requirements**: GQL-01, GQL-02, GQL-03
**Success Criteria** (what must be TRUE):
  1. All 13 offset/limit list resolvers exercised with data assertions
  2. Error paths for NetworkByAsn, SyncStatus, and validatePageSize tested
  3. 80%+ coverage on each hand-written resolver file
**Plans:** 1/1 plans complete

Plans:
- [x] 38-01-PLAN.md -- All 13 offset/limit + cursor resolvers, error paths, pagination unit tests

### Phase 39: gRPC Handler Coverage
**Goal**: Every gRPC List filter branch and Stream RPC covered by tests at 80%+
**Depends on**: Phase 37
**Requirements**: GRPC-01, GRPC-02, GRPC-03
**Success Criteria** (what must be TRUE):
  1. All 13 entity types have List tests with filter assertions
  2. All 13 entity types have Stream tests with count and field assertions
  3. 80%+ package-level coverage on grpcserver
**Plans:** 1/1 plans complete

Plans:
- [x] 39-01-PLAN.md -- List filter tests for 6 missing types + Stream tests for 4 missing types

### Phase 40: Web Handler Coverage
**Goal**: All web handler paths tested including fragments, dispatch modes, and edge cases
**Depends on**: Phase 37
**Requirements**: WEB-01, WEB-02, WEB-03
**Success Criteria** (what must be TRUE):
  1. All 6 lazy-loaded fragment handlers have integration tests
  2. renderPage dispatch tested for terminal, JSON, and WHOIS modes
  3. Edge cases for extractID, getFreshness, and error responses tested
**Plans:** 1/1 plans complete

Plans:
- [x] 40-01-PLAN.md -- renderPage dispatch modes, org fragment gaps, extractID/getFreshness edge cases

### Phase 41: Schema & Minor Package Coverage
**Goal**: Schema hooks, FK constraints, and utility packages all have error paths tested
**Depends on**: Phase 37
**Requirements**: SCHEMA-01, SCHEMA-02, SCHEMA-03, MINOR-01, MINOR-02, MINOR-03
**Success Criteria** (what must be TRUE):
  1. otelMutationHook error path tested
  2. FK edge cases (non-existent reference, nullable nil) tested
  3. internal/otel, health, peeringdb each at 90%+ coverage
  4. ent/schema/ hand-written files at 65%+ coverage
**Plans:** 2/2 plans complete

Plans:
- [x] 41-01-PLAN.md -- Schema hook error path, FK constraint tests, Edges/Indexes/Annotations coverage
- [x] 41-02-PLAN.md -- internal/otel, internal/health, internal/peeringdb error path coverage to 90%+

### Phase 42: Test Quality Audit & Coverage Hygiene
**Goal**: All tests validated for meaningful assertions, error paths covered, CI reports accurate numbers
**Depends on**: Phase 38, Phase 39, Phase 40, Phase 41
**Requirements**: QUAL-01, QUAL-02, QUAL-03, INFRA-02
**Success Criteria** (what must be TRUE):
  1. No test asserts only err == nil without data property checks
  2. Every error call site has at least one test exercising the error path
  3. Fuzz testing exercises filter parser without panics
  4. CI coverage excludes generated code from denominator
**Plans:** 5/5 plans complete

Plans:
- [x] 42-01-PLAN.md -- Fuzz test for filter parser + CI coverage exclusion
- [x] 42-02-PLAN.md -- Assertion density audit and weak test strengthening
- [x] 42-03-PLAN.md -- Error path coverage cross-reference and gap closure
- [x] 42-04-PLAN.md -- Gap closure: sync status.go + web compare/search/handler DB error path tests
- [x] 42-05-PLAN.md -- Gap closure: graph resolver where.P() filter error path tests

</details>

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
**Plans**: 4 plans
**UI hint**: yes

Plans:
- [x] 43-01-PLAN.md -- Flag-icons CDN, sort JS/CSS, CountryFlag component, FacNetworkRow data enrichment
- [ ] 43-02-PLAN.md -- IX, Network, and Facility table conversions (9 templates)
- [x] 43-03-PLAN.md -- Org, Campus, and Carrier table conversions + test updates (7 templates + tests)
- [ ] 43-04-PLAN.md -- Gap closure: IX/net/fac fragment test table structure assertions

### Phase 44: Facility Map & Map Infrastructure
**Goal**: Users see an interactive map on facility detail pages showing the facility's geographic location, establishing the map component and CDN infrastructure for all subsequent map work
**Depends on**: Phase 43
**Requirements**: MAP-01, MAP-04, MAP-05
**Success Criteria** (what must be TRUE):
  1. Facility detail pages with populated latitude/longitude display an interactive Leaflet map centered on the facility location with a clickable pin
  2. Clicking the map pin shows a popup with the facility name (and a link back to the detail page when navigated from another context)
  3. The map tile layer switches between CARTO light and dark basemaps matching the current app dark mode setting
  4. Facility detail pages without lat/lng data render normally with no map section (no empty container or error)
**Plans**: TBD
**UI hint**: yes

### Phase 45: Multi-Pin Maps
**Goal**: Users see maps with multiple facility pins on IX, network, and ASN comparison pages, with clustering for dense regions and colored pins distinguishing shared vs unique facilities
**Depends on**: Phase 44
**Requirements**: MAP-02, MAP-03
**Success Criteria** (what must be TRUE):
  1. IX detail pages display a map with pins for all associated facilities, and network detail pages display a map with pins for all facility presences
  2. When many pins overlap in the same geographic area, they cluster into a numbered circle that expands on click
  3. The ASN comparison page displays a map with two pin colors distinguishing shared facilities from facilities unique to each network
  4. All multi-pin maps auto-fit bounds to show all pins, and clicking any pin shows a popup with facility name linking to its detail page
**Plans**: TBD
**UI hint**: yes

### Phase 46: Search & Compare Density
**Goal**: Users see search results and ASN comparison tables in a denser layout with country flags, completing the information density overhaul
**Depends on**: Phase 43
**Requirements**: DENS-04, DENS-05, FLAG-02
**Success Criteria** (what must be TRUE):
  1. Search results display country and city information with SVG country flag icons alongside each result entry
  2. ASN comparison results (shared IXPs, shared facilities, shared campuses) render as dense columnar tables consistent with the detail page table style from Phase 43
  3. Search result entries show key metadata (country, city, ASN where applicable) in a compact layout without expanding vertical space per result
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 43 -> 44 -> 45 -> 46
(Phase 46 can execute in parallel with Phase 44/45 after Phase 43 completes)

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 43. Dense Tables with Sorting and Flags | 2/4 | In Progress|  |
| 44. Facility Map & Map Infrastructure | 0/? | Not started | - |
| 45. Multi-Pin Maps | 0/? | Not started | - |
| 46. Search & Compare Density | 0/? | Not started | - |
