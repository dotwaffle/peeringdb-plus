# Milestones

## v1.9 Hardening & Polish (Shipped: 2026-03-26)

**Phases completed:** 5 phases, 12 plans, 24 tasks

**Key accomplishments:**

- CORS middleware reordered before OTel tracing to eliminate preflight trace noise, plus 90 slog.String error calls replaced with slog.Any to preserve structured error type information
- Added ~96 optional filter fields across 26 ConnectRPC request messages for full pdbcompat parity, plus middleware tests reaching 96.7% coverage
- Created generic ListEntities/StreamEntities helpers and refactored all 13 gRPC handler files to delegate pagination and streaming logic, with per-type filter functions covering all pdbcompat Registry fields
- 61.8% grpcserver test coverage with all 13 entity types covered by Get/List/Stream tests, generic helper unit tests, and filter parity field verification
- Halved search queries (12 to 6) via limit+1 pattern, added updated/created indexes to all 13 schemas, replaced JSON roundtrip field projection with reflect-based accessor map
- Shared httperr package with RFC 9457 Problem Details integrated into pdbcompat, web JSON mode, and REST error middleware; ConnectRPC and GraphQL unchanged
- Generic Register function replaces type-switch dispatch in terminal RenderPage; all 6 detail handlers refactored to 29-line bodies with extracted queryXxx methods
- HTTP caching middleware with Cache-Control/ETag headers and 304 Not Modified for conditional GET/HEAD requests
- Four benchmark files covering search (3 query patterns, 125 entities), field projection (3 field counts), gRPC list pagination (100/1000 items), and sync upsert (100/500 orgs) with Go 1.26 b.Loop() for benchstat-compatible output
- WCAG AA dark-mode contrast fixes across 5 templates, ARIA nav attributes with mobile menu close, and breadcrumb navigation on all 6 entity detail pages
- Bookmarkable search URLs via HX-Push-Url with browser history, plus htmx error retry on failed collapsible section loads
- TruncateName helper with ellipsis truncation in all 6 entity renderers, plus styled terminal sync-not-ready detection in readinessMiddleware

---

## v1.8 Terminal CLI Interface (Shipped: 2026-03-26)

**Phases completed:** 4 phases, 12 plans, 24 tasks

**Key accomplishments:**

- Terminal client detection priority chain with lipgloss v2 ANSI rendering engine and Tailwind-to-ANSI256 color tier mapping
- Terminal detection wired into renderPage with Rich/Plain/JSON branching, wttr.in-style help text, and styled error pages
- Root handler terminal detection (NAV-04) with 14 integration tests covering curl/wget/HTTPie UA detection, Accept header negotiation, query param overrides, ANSI color control, and text error pages
- Eager IX/facility data fetching in handleNetworkDetail, type-switch dispatch in RenderPage, and 6 tested terminal formatting helpers (speed tiers, policy colors, bandwidth, cross-references)
- Whois-style RenderNetworkDetail with 15-field aligned header, color-coded policy/speed, RS badges, IX/facility sections with cross-references, and 12 comprehensive tests
- IX and Facility rich terminal renderers with full data plumbing across all 5 entity types, handler eager-loading, type-switch dispatch, and search/WHOIS mode detection
- Org, Campus, Carrier terminal renderers with D-03 compact layout, cross-referenced child entity lists, and 17 table-driven tests
- Search grouped-text-list and ASN comparison renderers with per-network IX presence details, cross-references, and TDD test coverage
- RPSL-like WHOIS format for all 6 entity types with 16-char key alignment and JSON child entity completeness verification
- Pipe-delimited ?format=short mode for scripting and FormatFreshness sync timestamp footer on all terminal responses
- Section filtering (?section=ix,fac) and width-adaptive column dropping (?w=N) for terminal detail views
- Bash/zsh completion scripts with server-side search endpoint and help text with shell integration setup instructions

---

## v1.7 Streaming RPCs & UI Polish (Shipped: 2026-03-25)

**Phases completed:** 3 phases, 6 plans, 11 tasks
**Timeline:** 2026-03-25 (single day)
**Git range:** v1.6..HEAD (38 commits, 73 files changed, +10,708/-1,938 lines)
**UAT:** 13/13 passed (8 automated, 5 visual)

**Key accomplishments:**

- 13 server-streaming RPCs with batched keyset pagination (500-row chunks), grpc-total-count header, configurable stream timeout, and graceful cancellation
- since_id stream resume and updated_since incremental filter on all 13 streaming RPCs with filter composition via AND
- IX presence UI redesign: labeled Speed/IPv4/IPv6 fields, 5-tier port speed colors, inline RS pill badge, grid-aligned copyable IPs, aggregate bandwidth headers
- Consistent layout across network detail and IX detail pages with shared templ components
- 20 integration tests for streaming RPCs covering filters, total count, cancellation, resume, and incremental fetch

---

## v1.6 ConnectRPC / gRPC API (Shipped: 2026-03-25)

**Phases completed:** 4 phases, 9 plans, 18 tasks

**Key accomplishments:**

- LiteFS proxy removed, fly-replay fixed to region=PRIMARY_REGION with Fly.io detection, h2c enabled via stdlib http.Protocols
- Protobuf toolchain with buf v2 config, protoc-gen-go + protoc-gen-connect-go tool deps, entproto extension in entc.go, and manual SocialMedia proto message
- All 13 ent schemas annotated with entproto producing v1.proto with 227 typed fields, compiled to Go protobuf types via buf generate
- 13 proto service definitions with Get/List RPCs producing ConnectRPC handler interfaces via buf generate for Phase 23 implementation
- Ent-to-proto conversion helpers, offset pagination with base64 cursors, and NetworkService Get/List RPCs as template for all 13 entity types
- 12 ConnectRPC service handlers implementing Get/List RPCs for all PeeringDB types with correct ent-to-proto wrapper type mappings
- All 13 ConnectRPC services wired into HTTP mux with otelconnect interceptor, gRPC reflection, health checking, and Connect-aware CORS
- Optional filter fields on all 13 List RPC request messages with predicate accumulation pattern proven on NetworkService
- Predicate accumulation filter logic applied to all 12 remaining ConnectRPC List handlers with tests covering geographic, FK, name, role, protocol, and ASN filter categories

---

## v1.5 Tech Debt & Observability (Shipped: 2026-03-24)

**Phases completed:** 3 phases, 9 plans, 12 tasks

**Key accomplishments:**

- Corrected PROJECT.md and Phase 7 summary to accurately reflect DataLoader removal (v1.2) and WorkerConfig.IsPrimary conversion to live func() bool (quick task 260324-lc5)
- Flag-gated live test verifying meta.generated presence on full fetch and absence on paginated/incremental PeeringDB responses, with empirical documentation of all three request patterns
- Prometheus metrics endpoint via OTel autoexport env config and 5-row Grafana dashboard with freshness gauge, HTTP RED, per-type sync detail, Go runtime, and business metrics
- Portable Grafana dashboard JSON with 5 collapsible rows (30 panels) covering sync health, HTTP RED, per-type sync, Go runtime, and business metrics with PromQL queries against OTLP-ingested Prometheus metrics
- Observable Int64Gauge reporting per-type object counts for all 13 PeeringDB types, wired into main.go to power Grafana Business Metrics panels
- Removed duplicate dashboard/provisioning artifacts and fixed all 8 dashboard tests to validate canonical pdbplus-overview.json with nested panel support

---

## v1.4 Web UI (Shipped: 2026-03-24)

**Phases completed:** 5 phases, 11 plans, 22 tasks

**Key accomplishments:**

- Templ + Tailwind CDN + htmx web UI skeleton with dual rendering, responsive layout, and 10 passing tests
- Content negotiation on GET / with browser redirect to /ui/, HTML syncing page in readiness middleware, and templ drift detection in CI
- SearchService with errgroup fan-out querying 6 PeeringDB entity types in parallel, returning grouped results with count badges
- Homepage search form with htmx live-as-you-type, grouped results template with colored type badges, and search endpoint with bookmarked URL support
- Network detail page at /ui/asn/{asn} with lazy-loaded collapsible sections for IX presences, facilities, and contacts via htmx fragment endpoints
- Detail pages for IX, Facility, Org, Campus, and Carrier with 11 lazy-loaded fragment endpoints, cross-links between all entity types, and 80+ test cases
- CompareService with errgroup-parallel ent queries computing shared IXPs, facilities, and campuses via map-based set intersection
- Comparison page templates with form/results views, view toggle, shareable URLs, and Compare button on network detail pages
- Dark mode with system preference detection and manual toggle, fadeIn animations on search results, and global htmx loading indicator bar
- Styled 404/500 error pages with search box fallback and About page with live data freshness from sync_status
- ARIA listbox/option roles and keyboard navigation (ArrowDown/Up/Enter/Escape) for search results with visual focus ring and htmx reset

---

## v1.3 PeeringDB API Key Support (Shipped: 2026-03-24)

**Phases completed:** 2 phases, 3 plans, 4 tasks

**Key accomplishments:**

- PeeringDB API key support via WithAPIKey functional option with 60 req/min authenticated rate limit, Authorization header injection, and 401/403 immediate-fail handling
- Conditional WithAPIKey wiring in main.go with SEC-2 compliant startup logging indicating key presence
- API key auth wired into pdbcompat-check CLI (--api-key flag + env fallback) and live conformance test (conditional 1s/3s sleep, auth header injection)

---

## v1.2 Quality, Incremental Sync & CI (Shipped: 2026-03-24)

**Phases completed:** 4 phases, 9 plans, 18 tasks

**Key accomplishments:**

- golangci-lint v2 configured with generated:strict exclusion, dead code removed (globalid.go, dataloader package, config.IsPrimary), ~40 violations baselined for Plan 02
- All 40 golangci-lint violations fixed across 22 hand-written files: 21 errcheck, 8 revive, 6 staticcheck, 3 unused, 1 gocritic, 1 nolintlint
- SyncMode config field with env var parsing plus FetchAll functional options returning FetchResult with meta.generated tracking
- Per-type sync cursor persistence via sync_cursors table with success-filtered reads, plus SyncTypeFallback OTel counter for incremental-to-full fallback tracking
- Mode-aware sync worker with per-type incremental fetch via WithSince, automatic fallback to full on failure, and cursor persistence only after successful commit
- Golden file test infrastructure with 39 committed reference files locking down PeeringDB compat JSON output for all 13 types across list, detail, and depth-expanded scenarios
- Structural JSON comparison library with CLI tool and flag-gated live integration test against beta.peeringdb.com
- GitHub Actions CI with parallel lint (golangci-lint + generate drift), test (race + coverage comment), build, and govulncheck jobs
- Verified all read API endpoints are publicly accessible without authentication; root endpoint JSON self-documents the API surface

---

## v1.1 REST API & Observability (Shipped: 2026-03-23)

**Phases completed:** 3 phases, 8 plans, 16 tasks

**Key accomplishments:**

- OTel trace spans on PeeringDB HTTP client with otelhttp transport wrapping and manual span hierarchy (FetchAll parent, per-attempt children, page events, rate limiter wait events)
- 5 per-type sync metric instruments registered and wired with SyncDuration/SyncOperations recording and freshness observable gauge computing seconds-since-last-sync on demand
- entrest v1.0.2 dual codegen with entgql producing read-only REST handlers and OpenAPI spec for all 13 PeeringDB types
- REST API mounted at /rest/v1/ with CORS and 7 integration tests covering all 13 entity types, OpenAPI spec, sorting, pagination, eager-loading, readiness gate, and write rejection
- Per-field filtering via entrest.WithFilter annotations on all 13 PeeringDB schemas with 8 integration test cases
- Django-style filter parser with 8 operators, type registry for all 13 PeeringDB types with field metadata, entity serializers with correct field mapping, and response envelope producing PeeringDB-identical JSON output
- HTTP handlers for all 13 PeeringDB types with list/detail/index endpoints, pagination, since filter, Django-style query filters, and trailing slash handling
- Depth expansion with _set field serialization for all 13 types, text search via ?q=, and field projection via ?fields=

---

## v1.0 PeeringDB Plus (Shipped: 2026-03-22)

**Phases completed:** 3 phases, 14 plans, 27 tasks

**Key accomplishments:**

- Go module bootstrapped with entgo code generation pipeline, Organization schema with full PeeringDB field coverage, SQLite/WAL database setup via modernc.org/sqlite, and OTel trace provider -- all validated end-to-end with in-memory CRUD test
- All 13 PeeringDB object types modeled as entgo schemas with complete field inventories, FK edges, OTel mutation hooks, and CRUD tests validating creation, nullable FKs, and edge traversal
- Rate-limited HTTP client with pagination and retry for all 13 PeeringDB object types, using golang.org/x/time/rate at 20 req/min with exponential backoff on transient errors
- Full PeeringDB sync worker with 13-type bulk upsert in FK dependency order, single-transaction atomicity, hard delete, mutex, 30s/2m/8m exponential backoff retry, and sync_status tracking
- Go-based schema extraction pipeline: regex parser for Django Python source producing intermediate JSON with all 13 PeeringDB types, plus entgo code generator with formatted output, edges, indexes, and entgql annotations
- Application entry point wiring config, database, OTel, and sync worker with HTTP endpoints and multi-stage Docker build
- 13 PeeringDB fixture files and 4 integration tests verifying full sync pipeline: upsert, delete, status filtering, edge traversal, and idempotency
- Relay-compliant GraphQL schema with 13 connection types, offset/limit list queries, and gqlgen resolver scaffold for read-only PeeringDB API
- All 13 PeeringDB type queries with cursor and offset/limit pagination, custom resolvers (syncStatus, networkByAsn), DataLoader middleware for N+1 prevention, and Relay global ID encoding
- HTTP middleware (CORS, logging, recovery), GraphQL handler factory with complexity/depth limits and D-16 error presenter, and config extensions for port, CORS origins, and drain timeout
- Full GraphQL API wired into main.go with middleware stack, playground with example queries, exported SDL, and 16 integration tests covering all 8 requirements
- Autoexport-driven OTel initialization with all three signals (traces, metrics, logs), dual slog handler for stdout+OTel output, and custom sync metrics with configurable sampling
- Liveness/readiness HTTP handlers with sync freshness checking and LiteFS primary/replica detection via .primary file semantics
- Full application wiring with OTel observability, health endpoints, LiteFS primary detection, otelhttp middleware, Fly-Replay write forwarding, and Fly.io deployment artifacts (Dockerfile.prod, litefs.yml, fly.toml)

---
