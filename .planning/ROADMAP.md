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
- [ ] **v1.10 Code Coverage & Test Quality** - Phases 37-42 (in progress)

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

<details>
<summary>v1.9 Hardening & Polish (Phases 32-36) - SHIPPED 2026-03-26</summary>

- [x] **Phase 32: Quick Wins** - Middleware reorder and structured error logging fix across 90 call sites
- [x] **Phase 33: gRPC Deduplication & Filter Parity** - Generic List/Stream helpers replacing 1,154 lines of duplicated handlers, plus ConnectRPC filter parity with PeeringDB compat
- [x] **Phase 34: Query Optimization & Architecture** - Eliminate double-count queries, add indexes, fix field projection, unify error formats, refactor renderer and detail handlers
- [x] **Phase 35: HTTP Caching & Benchmarks** - Cache-Control/ETag headers derived from sync timestamp, plus benchmark suite for hot paths
- [x] **Phase 36: UI & Terminal Polish** - WCAG AA contrast, ARIA attributes, bookmarkable search, htmx error handling, breadcrumbs, mobile menu, terminal wrapping and error styling

</details>

- [ ] **Phase 37: Test Seed Infrastructure** - Shared deterministic entity factory package for all 13 PeeringDB types
- [ ] **Phase 38: GraphQL Resolver Coverage** - Integration tests for all 13 list resolvers and custom resolver error paths
- [ ] **Phase 39: gRPC Handler Coverage** - Filter, streaming, and branch coverage for all 13 entity types
- [ ] **Phase 40: Web Handler Coverage** - Fragment handler, multi-mode dispatch, and edge case tests
- [ ] **Phase 41: Schema & Minor Package Coverage** - Schema hook/constraint tests plus otel, health, and peeringdb error path tests
- [ ] **Phase 42: Test Quality Audit & Coverage Hygiene** - Assertion density audit, error path coverage, fuzz tests, and CI coverage filtering

## Phase Details

<details>
<summary>v1.9 Hardening & Polish (Phases 32-36) - SHIPPED 2026-03-26</summary>

### Phase 32: Quick Wins
**Goal**: Middleware ordering prevents unnecessary OTel noise from preflight requests, and all error logging preserves structured error types
**Depends on**: Phase 31
**Requirements**: ARCH-03, QUAL-02
**Success Criteria** (what must be TRUE):
  1. An OPTIONS preflight request to any endpoint returns CORS headers without creating an OTel trace span or emitting a log line
  2. Every `slog` error log call in the codebase passes the error value via `slog.Any("error", err)`, preserving error type information for structured log consumers
**Plans:** 1/1 plans complete

Plans:
- [x] 32-01-PLAN.md -- Middleware chain reorder (CORS before OTel) + slog.String->slog.Any replacement across 90 call sites

### Phase 33: gRPC Deduplication & Filter Parity
**Goal**: gRPC service handlers use shared generic helpers instead of per-type copy-paste, and ConnectRPC exposes the same filter fields as the PeeringDB compat layer
**Depends on**: Phase 32
**Requirements**: QUAL-01, QUAL-03, ARCH-02
**Success Criteria** (what must be TRUE):
  1. The `internal/grpcserver/` package contains a generic `List` and `Stream` implementation parameterized by entity type, and per-type handler files delegate to it
  2. Total line count in `internal/grpcserver/` service handler files is reduced by at least 800 lines compared to v1.8
  3. Running `go test -race ./internal/grpcserver/...` passes with 60%+ coverage, and `go test -race ./internal/middleware/...` passes with 60%+ coverage
  4. Every filterable field available on a PeeringDB compat List endpoint (e.g., `/api/net?info_type=Content`) has a corresponding optional field on the ConnectRPC List RPC request message
**Plans:** 3/3 plans complete

Plans:
- [x] 33-01-PLAN.md -- Proto filter parity (add ~96 optional fields to services.proto) + middleware test coverage
- [x] 33-02-PLAN.md -- Generic ListEntities/StreamEntities helpers + refactor all 13 handler files with full filter functions
- [x] 33-03-PLAN.md -- Comprehensive grpcserver tests for all 13 entity types + coverage validation

### Phase 34: Query Optimization & Architecture
**Goal**: Search and API queries are faster (no double-counting, proper indexes, no JSON roundtrips), errors are consistent across all surfaces, and the renderer and detail handlers are cleanly structured
**Depends on**: Phase 33
**Requirements**: PERF-01, PERF-03, PERF-05, ARCH-01, ARCH-04, QUAL-04
**Success Criteria** (what must be TRUE):
  1. The search service issues one SQL query per entity type (not separate item + count queries) -- observable via OTel trace spans or query logging
  2. Running `EXPLAIN QUERY PLAN` on filtered queries against `updated` and `created` fields shows index usage (not full table scans)
  3. Field projection in the pdbcompat layer operates on struct fields directly, not through `json.Marshal` followed by `json.Unmarshal`
  4. A malformed request to any of the 6 API surfaces (GraphQL, REST, PeeringDB compat, ConnectRPC, Web UI, Terminal) returns an error body with the same top-level structure containing `code`, `message`, and optional `details`
  5. Terminal entity renderers implement a `Renderer` interface, and each web detail handler function body is under 80 lines with query logic separated from rendering
**Plans:** 3/3 plans complete

Plans:
- [x] 34-01-PLAN.md -- Search limit+1 optimization, database indexes on updated/created, reflect-based field projection
- [x] 34-02-PLAN.md -- RFC 9457 error format (httperr package) integrated into pdbcompat, web JSON mode, and REST middleware
- [x] 34-03-PLAN.md -- Registered function map renderer dispatch, detail handler refactor with queryXxx methods

### Phase 35: HTTP Caching & Benchmarks
**Goal**: Browsers and HTTP clients can cache API responses between sync cycles, and a benchmark suite establishes performance baselines on the optimized code
**Depends on**: Phase 34
**Requirements**: PERF-02, PERF-04
**Success Criteria** (what must be TRUE):
  1. API responses include `Cache-Control` and `ETag` headers derived from the last sync timestamp, and a conditional `If-None-Match` request returns 304 Not Modified when data has not changed
  2. Running `go test -bench ./...` exercises benchmarks for search queries, pdbcompat field projection, gRPC streaming entity conversion, and sync upsert operations
  3. Benchmark results are stable across runs (no flaky timing from external I/O) and can be compared via `benchstat`
**Plans:** 2/2 plans complete

Plans:
- [x] 35-01-PLAN.md -- Caching middleware (Cache-Control, ETag, 304 Not Modified) + main.go middleware chain wiring
- [x] 35-02-PLAN.md -- Benchmark suite: search, field projection, generic list, sync upsert

### Phase 36: UI & Terminal Polish
**Goal**: The web UI meets WCAG AA accessibility standards, search results are shareable, collapsible sections handle errors gracefully, and terminal output wraps cleanly
**Depends on**: Phase 34 (ARCH-04 renderer interface)
**Requirements**: UI-01, UI-02, UI-03, UI-04, UI-05, UI-06, UI-07, TUI-01, TUI-02
**Success Criteria** (what must be TRUE):
  1. All text in dark mode passes WCAG AA contrast ratio (4.5:1 minimum) when checked with a contrast analyzer tool
  2. Screen reader navigation identifies the main nav, mobile menu toggle state (aria-expanded), and search input (label) correctly
  3. Typing a search query updates the browser URL (e.g., `/ui/?q=equinix`) so bookmarking or sharing the URL reproduces the search results
  4. When an htmx collapsible section fetch fails, the section displays an error message with a clickable retry button instead of showing "Loading..." indefinitely
  5. Detail pages show breadcrumb navigation (Home > Type > Entity), the mobile nav menu closes after link selection, and the Compare button on network pages is visually distinct from the background
  6. Long entity names in terminal tables wrap to the next line instead of being truncated, and terminal error responses (404, 500, sync-not-ready) use the same styled formatting as normal terminal output
**Plans:** 3/3 plans complete

Plans:
- [x] 36-01-PLAN.md -- WCAG AA contrast fixes, ARIA attributes, breadcrumbs, mobile menu close, compare button styling
- [x] 36-02-PLAN.md -- Bookmarkable search (HX-Push-Url), htmx error handling with retry on collapsible sections
- [x] 36-03-PLAN.md -- Terminal name wrapping (TruncateName), styled error responses, sync-not-ready terminal detection

</details>

### Phase 37: Test Seed Infrastructure
**Goal**: Any test file in the project can create a fully-populated database with all 13 PeeringDB entity types by calling a single function, eliminating duplicated entity creation code
**Depends on**: Phase 36
**Requirements**: INFRA-01
**Success Criteria** (what must be TRUE):
  1. Calling `seed.Full(t, client)` creates at least one entity of each of the 13 PeeringDB types with realistic field values and correct FK relationships, and the returned `Result` struct provides typed references to every created entity
  2. Calling `seed.Minimal(t, client)` creates the minimum entity graph needed for relationship traversal (Org + Network + IX + Facility), and `seed.Networks(t, client, 2)` creates exactly 2 networks with their dependencies
  3. Tests in at least 3 different packages (graph, grpcserver, web) can import and use the seed package without import cycles or package-level setup conflicts
**Plans:** 1/1 plans complete

Plans:
- [x] 37-01-PLAN.md -- Seed package (Full/Minimal/Networks) with TDD + import-cycle validation

### Phase 38: GraphQL Resolver Coverage
**Goal**: Hand-written GraphQL resolver code is tested to 80%+ coverage, with every custom resolver error path exercised
**Depends on**: Phase 37
**Requirements**: GQL-01, GQL-02, GQL-03
**Success Criteria** (what must be TRUE):
  1. Running `go test -race ./graph/...` exercises all 13 offset/limit list resolvers, and each test asserts that returned data matches seeded entities (not just status code or nil error)
  2. Tests exercise the NetworkByAsn not-found path (returns GraphQL error, not panic), the SyncStatus-missing path (returns null, not error), and validatePageSize rejection (returns error for limit > max)
  3. Running `go tool cover -func` filtered to `custom.resolvers.go`, `schema.resolvers.go`, and `pagination.go` shows 80%+ coverage on each file
**Plans:** 1/1 plans complete

Plans:
- [x] 38-01-PLAN.md -- All 13 offset/limit + cursor resolvers, error paths, pagination unit tests, 80%+ per-file coverage

### Phase 39: gRPC Handler Coverage
**Goal**: Every gRPC List filter branch and every Stream RPC is covered by tests, reaching 80%+ coverage on grpcserver handler code
**Depends on**: Phase 37
**Requirements**: GRPC-01, GRPC-02, GRPC-03
**Success Criteria** (what must be TRUE):
  1. All 13 entity types have at least one List test that sets an optional proto filter field to a non-nil value and asserts the response contains only matching entities (not just "no error")
  2. All 13 entity types have Stream tests (closing the gap for CarrierFacility, IxPrefix, NetworkIxLan, and Poc), and each stream test asserts the streamed entity count and at least one field value
  3. Running `go test -race -cover ./internal/grpcserver/...` reports 80%+ package-level coverage
**Plans**: TBD

### Phase 40: Web Handler Coverage
**Goal**: All web handler paths -- fragment endpoints, terminal/JSON/WHOIS dispatch, and utility functions -- are tested
**Depends on**: Phase 37
**Requirements**: WEB-01, WEB-02, WEB-03
**Success Criteria** (what must be TRUE):
  1. All 6 lazy-loaded fragment handlers (network IX presences, network facilities, IX networks, IX facilities, facility networks, org networks) have integration tests that seed data, request the fragment endpoint, and assert the response contains expected entity names or IDs
  2. Tests exercise renderPage dispatch for terminal (User-Agent: curl), JSON (?format=json), and WHOIS (?format=whois) modes, asserting each produces the correct content type and contains mode-specific markers (ANSI codes, JSON braces, RPSL keys respectively)
  3. Edge cases for extractID (invalid input, zero, negative), getFreshness (no sync status, stale data), and error response paths (404 entity not found, 500 database error) each have at least one test case
**Plans**: TBD

### Phase 41: Schema & Minor Package Coverage
**Goal**: Schema validation hooks, relationship constraints, and three minor utility packages all have their error paths and edge cases tested
**Depends on**: Phase 37
**Requirements**: SCHEMA-01, SCHEMA-02, SCHEMA-03, MINOR-01, MINOR-02, MINOR-03
**Success Criteria** (what must be TRUE):
  1. The otelMutationHook error path (OTel span records error when mutation fails) has a test that triggers a mutation failure and asserts the hook does not panic and the error propagates correctly
  2. FK edge cases (creating an entity with a non-existent FK reference, nullable FK set to nil) have tests that verify the correct ent error is returned or the entity is created successfully
  3. Running `go test -race -cover` on `internal/otel`, `internal/health`, and `internal/peeringdb` each reports 90%+ coverage, with new tests specifically targeting error returns (not just happy paths)
  4. Running `go tool cover -func` on `ent/schema/` hand-written files shows 65%+ coverage
**Plans**: TBD

### Phase 42: Test Quality Audit & Coverage Hygiene
**Goal**: Existing tests are validated for meaningful assertions, every error code path has test coverage, and CI reports accurate coverage numbers excluding generated code
**Depends on**: Phase 38, Phase 39, Phase 40, Phase 41
**Requirements**: QUAL-01, QUAL-02, QUAL-03, INFRA-02
**Success Criteria** (what must be TRUE):
  1. An audit pass through all test files confirms no test function asserts only `err == nil` or `status == 200` without also checking at least one data property -- any such tests found are updated with data assertions
  2. Every `fmt.Errorf` and `connect.NewError` call site in hand-written code has at least one test that exercises the error path (verified by grepping error sites and cross-referencing with coverage output)
  3. Running `go test -fuzz=FuzzFilterParser -fuzztime=30s` exercises the filter parser with random inputs without panicking or returning incorrect parse results
  4. CI coverage reporting (GitHub Actions) excludes `ent/*`, `gen/*`, `*generated.go`, and `*_templ.go` from the coverage denominator, and the reported percentage reflects hand-written code only
**Plans**: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 37 -> 38 -> 39 -> 40 -> 41 -> 42
(Phases 38 and 39 can execute in parallel after 37 completes)

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 37. Test Seed Infrastructure | 1/1 | Complete    | 2026-03-26 |
| 38. GraphQL Resolver Coverage | 1/1 | Complete   | 2026-03-26 |
| 39. gRPC Handler Coverage | 0/? | Not started | - |
| 40. Web Handler Coverage | 0/? | Not started | - |
| 41. Schema & Minor Package Coverage | 0/? | Not started | - |
| 42. Test Quality Audit & Coverage Hygiene | 0/? | Not started | - |
