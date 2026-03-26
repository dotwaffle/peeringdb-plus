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
- [ ] **v1.9 Hardening & Polish** - Phases 32-36 (in progress)

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

<details>
<summary>v1.8 Terminal CLI Interface (Phases 28-31) - SHIPPED 2026-03-26</summary>

- [x] **Phase 28: Terminal Detection & Infrastructure** - Content negotiation, User-Agent detection, rendering framework, help text, and error pages for terminal clients (completed 2026-03-25)
- [x] **Phase 29: Network Detail (Reference Implementation)** - Network entity terminal renderer with whois-style header, IX/facility tables, colored speed tiers, and cross-reference paths (completed 2026-03-26)
- [x] **Phase 30: Entity Types, Search & Formats** - Terminal renderers for remaining 5 entity types, search results, ASN comparison, plus plain text, JSON, and WHOIS output modes (completed 2026-03-26)
- [x] **Phase 31: Differentiators & Shell Integration** - One-line summary, section filtering, width control, freshness footer, and downloadable bash/zsh completions (completed 2026-03-26)

</details>

- [ ] **Phase 32: Quick Wins** - Middleware reorder and structured error logging fix across 90 call sites
- [ ] **Phase 33: gRPC Deduplication & Filter Parity** - Generic List/Stream helpers replacing 1,154 lines of duplicated handlers, plus ConnectRPC filter parity with PeeringDB compat
- [ ] **Phase 34: Query Optimization & Architecture** - Eliminate double-count queries, add indexes, fix field projection, unify error formats, refactor renderer and detail handlers
- [ ] **Phase 35: HTTP Caching & Benchmarks** - Cache-Control/ETag headers derived from sync timestamp, plus benchmark suite for hot paths
- [ ] **Phase 36: UI & Terminal Polish** - WCAG AA contrast, ARIA attributes, bookmarkable search, htmx error handling, breadcrumbs, mobile menu, terminal wrapping and error styling

## Phase Details

<details>
<summary>v1.8 Terminal CLI Interface (Phases 28-31) - SHIPPED 2026-03-26</summary>

### Phase 28: Terminal Detection & Infrastructure
**Goal**: Terminal clients (curl, wget, HTTPie) hitting any /ui/ URL receive appropriate text responses instead of HTML, with explicit format overrides available
**Depends on**: Phase 27
**Requirements**: DET-01, DET-02, DET-03, DET-04, DET-05, RND-01, RND-18, NAV-01, NAV-02, NAV-03, NAV-04
**Success Criteria** (what must be TRUE):
  1. Running `curl peeringdb-plus.fly.dev/ui/` returns CLI help text listing available endpoints, query parameters, and usage examples -- not an HTML page
  2. Running `curl peeringdb-plus.fly.dev/ui/asn/13335` returns ANSI-colored text output (not HTML), while the same URL in a browser returns the existing web UI unchanged
  3. Appending `?T` or `?format=plain` to any /ui/ URL returns plain ASCII output with no ANSI escape codes, and `?format=json` returns JSON
  4. Requesting a nonexistent path like `curl /ui/asn/99999999` returns a text-formatted 404 error (not HTML), and server errors return text-formatted 500 errors
  5. Setting `?nocolor` suppresses all ANSI escape codes in terminal output while preserving layout
**Plans:** 3/3 plans complete

Plans:
- [x] 28-01-PLAN.md -- termrender package foundation: detection logic, renderer engine, style definitions
- [x] 28-02-PLAN.md -- renderPage integration, PageContent.Data wiring, help text and error renderers
- [x] 28-03-PLAN.md -- Root handler terminal detection, error handler wiring, integration tests

### Phase 29: Network Detail (Reference Implementation)
**Goal**: Network engineers can look up any network by ASN from the terminal and see a comprehensive, well-formatted detail view with colored status indicators and navigable cross-references
**Depends on**: Phase 28
**Requirements**: RND-02, RND-12, RND-13, RND-14, RND-15, RND-16
**Success Criteria** (what must be TRUE):
  1. Running `curl /ui/asn/13335` displays a whois-style key-value header (name, ASN, type, policy, website, etc.) followed by tabular IX presences and facility lists with Unicode box drawing
  2. Port speeds in IX presence tables are color-coded matching the web UI tiers (gray sub-1G, neutral 1G, blue 10G, emerald 100G, amber 400G+) and route server peers show a colored [RS] badge
  3. Peering policy is color-coded (green for Open, yellow for Selective, red for Restrictive) in the network header
  4. Aggregate bandwidth is displayed in the network header and per-IX section headers
  5. Each entity reference (IX name, facility name) includes its ID or path (e.g., `/ui/ix/123`) so the user can follow up with another curl command
**Plans:** 2/2 plans complete

Plans:
- [x] 29-01-PLAN.md -- Data plumbing: NetworkDetail struct extension, eager IX/facility fetching, type-switch dispatch, formatting helpers
- [x] 29-02-PLAN.md -- RenderNetworkDetail full implementation with whois-style output and comprehensive tests

### Phase 30: Entity Types, Search & Formats
**Goal**: All six PeeringDB entity types, search, and comparison are accessible from the terminal, with plain text, JSON, and WHOIS as alternative output formats
**Depends on**: Phase 29
**Requirements**: RND-03, RND-04, RND-05, RND-06, RND-07, RND-08, RND-09, RND-10, RND-11, RND-17
**Success Criteria** (what must be TRUE):
  1. Running `curl /ui/ix/{id}`, `/ui/fac/{id}`, `/ui/org/{id}`, `/ui/campus/{id}`, and `/ui/carrier/{id}` each returns a formatted terminal detail view appropriate to that entity type
  2. Running `curl /ui/?q=equinix` returns search results grouped by entity type as a text list, matching the web UI search behavior
  3. Running `curl /ui/compare/13335/15169` renders a terminal comparison of two networks showing shared IXPs, facilities, and campuses
  4. Appending `?format=whois` to any detail URL returns RPSL-like key-value output suitable for parsing by network automation scripts
  5. All alternative format modes (?T, ?format=json, ?format=whois) produce consistent output across all entity types -- not just networks
**Plans:** 4/4 plans complete

Plans:
- [x] 30-01-PLAN.md -- Data plumbing (struct fields, handler eager-loading, type-switch) + IX and Facility rich renderers
- [x] 30-02-PLAN.md -- Org, Campus, Carrier minimal renderers
- [x] 30-03-PLAN.md -- Search results and ASN comparison renderers
- [x] 30-04-PLAN.md -- WHOIS format for all entity types + JSON completeness verification

### Phase 31: Differentiators & Shell Integration
**Goal**: Power users can customize terminal output (summary mode, section filtering, width control) and install shell completions for a native CLI feel
**Depends on**: Phase 30
**Requirements**: DIF-01, DIF-02, DIF-03, DIF-04, SHL-01, SHL-02, SHL-03
**Success Criteria** (what must be TRUE):
  1. Running `curl /ui/asn/13335?format=short` returns a single-line summary suitable for scripting (e.g., `AS13335 | Cloudflare, Inc. | Open | 2847 prefixes`)
  2. Every terminal response includes a data freshness timestamp footer showing when PeeringDB data was last synced
  3. Appending `?section=ix,fac` to a detail URL renders only the IX presences and facilities sections, omitting other sections
  4. Appending `?w=120` adapts table rendering to 120-column width, and `?w=80` produces narrower tables that fit standard terminals
  5. Running `curl /ui/completions/bash` and `curl /ui/completions/zsh` downloads shell completion scripts, and the help text includes alias/function setup instructions
**Plans:** 3/3 plans complete

Plans:
- [x] 31-01-PLAN.md -- Short format mode (?format=short) + data freshness footer on all terminal responses
- [x] 31-02-PLAN.md -- Section filtering (?section=) + width adaptation (?w=N) for detail views
- [x] 31-03-PLAN.md -- Shell completion scripts (bash, zsh) + search endpoint + help text update

</details>

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
- [ ] 34-01-PLAN.md -- Search limit+1 optimization, database indexes on updated/created, reflect-based field projection
- [ ] 34-02-PLAN.md -- RFC 9457 error format (httperr package) integrated into pdbcompat, web JSON mode, and REST middleware
- [x] 34-03-PLAN.md -- Registered function map renderer dispatch, detail handler refactor with queryXxx methods

### Phase 35: HTTP Caching & Benchmarks
**Goal**: Browsers and HTTP clients can cache API responses between sync cycles, and a benchmark suite establishes performance baselines on the optimized code
**Depends on**: Phase 34
**Requirements**: PERF-02, PERF-04
**Success Criteria** (what must be TRUE):
  1. API responses include `Cache-Control` and `ETag` headers derived from the last sync timestamp, and a conditional `If-None-Match` request returns 304 Not Modified when data has not changed
  2. Running `go test -bench ./...` exercises benchmarks for search queries, pdbcompat field projection, gRPC streaming entity conversion, and sync upsert operations
  3. Benchmark results are stable across runs (no flaky timing from external I/O) and can be compared via `benchstat`
**Plans:** 2 plans

Plans:
- [ ] 35-01-PLAN.md -- Caching middleware (Cache-Control, ETag, 304 Not Modified) + main.go middleware chain wiring
- [ ] 35-02-PLAN.md -- Benchmark suite: search, field projection, generic list, sync upsert

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
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 32 -> 33 -> 34 -> 35 -> 36

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 32. Quick Wins | 1/1 | Complete    | 2026-03-26 |
| 33. gRPC Deduplication & Filter Parity | 3/3 | Complete    | 2026-03-26 |
| 34. Query Optimization & Architecture | 0/3 | Complete    | 2026-03-26 |
| 35. HTTP Caching & Benchmarks | 0/2 | Not started | - |
| 36. UI & Terminal Polish | 0/0 | Not started | - |
