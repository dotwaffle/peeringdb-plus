# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.7 — Streaming RPCs & UI Polish

**Shipped:** 2026-03-25
**Phases:** 3 | **Plans:** 6 | **Tasks:** 11

### What Was Built
- 13 server-streaming RPCs with batched keyset pagination, grpc-total-count header, configurable timeout, and graceful cancellation
- since_id stream resume and updated_since incremental filter on all 13 streaming RPCs
- IX presence UI redesign with labeled fields, 5-tier speed colors, RS pill badge, grid-aligned copyable IPs, aggregate bandwidth headers
- Consistent layout across network detail and IX detail pages via shared templ components

### What Worked
- Batched keyset pagination pattern (timeout -> predicates -> count header -> keyset batch loop) templated cleanly across all 13 entity types
- Parallel worktree execution: Plans 02 and 03 ran in parallel worktrees, each self-contained with prerequisite code
- templ script components for clipboard handled JS interop type-safely without raw inline JS
- Shared helpers (speedColorClass, CopyableIP, CollapsibleSectionWithBandwidth) reduced Plan 02 to a 2-minute execution
- UAT combined automated integration tests (8) with visual verification (5) for efficient validation

### What Was Inefficient
- STRM-02 through STRM-05 traceability status showed "Pending" in REQUIREMENTS.md despite being satisfied — requirements tracking lagged execution
- Phase directories were already archived to milestones/ before milestone completion workflow ran, causing CLI to report 0 phases/plans
- MILESTONES.md got a duplicate entry (0-stat + real-stat) from the CLI running against already-archived phases

### Patterns Established
- Streaming handler template: timeout -> predicates -> count -> header -> keyset batch loop with ctx.Err() check
- httptest HTTP/2 TLS server pattern for ConnectRPC integration tests (EnableHTTP2=true + StartTLS)
- setupStreamTestServer helper for reusable streaming test infrastructure
- templ script component for clipboard: type-safe JS interop with auto-deduplicated script injection
- 5-tier speed color coding: sub-1G gray, 1G neutral, 10G blue, 100G emerald, 400G+ amber

### Key Lessons
1. Run milestone completion before archiving phase directories — the CLI needs phases in .planning/phases/ to count stats
2. Streaming handlers are highly templatable — once one reference implementation exists, the remaining 12 are mechanical
3. templ script components are the correct pattern for any JS interop (not inline onclick strings)
4. Combined automated + visual UAT is the right approach for milestones with both backend and frontend work

### Cost Observations
- Model mix: ~85% opus, ~15% sonnet (subagents)
- Sessions: 1 session
- Notable: Entire milestone (3 phases, 6 plans) completed in a single session — fastest multi-phase milestone

---

## Milestone: v1.6 — ConnectRPC / gRPC API

**Shipped:** 2026-03-25
**Phases:** 4 | **Plans:** 9 | **Tasks:** 18

### What Was Built
- LiteFS proxy removal with fly-replay and h2c for gRPC wire protocol
- Protobuf toolchain (buf v2, entproto extension, manual SocialMedia proto)
- All 13 ent schemas annotated with entproto, compiled to Go protobuf types
- 13 ConnectRPC service handlers with Get/List RPCs, typed filtering, pagination
- gRPC reflection, health checking, otelconnect observability, Connect-aware CORS

### What Worked
- ConnectRPC over google.golang.org/grpc was the right call — http.Handler compatibility meant same mux as REST/GraphQL
- Hand-written services.proto (entproto generates messages only) gave full control over RPC signatures
- Predicate accumulation pattern for List filtering proved clean and consistent across all 13 types
- Parallel worktree execution for Plan 01 infrastructure and Plan 02 reference implementation

### What Was Inefficient
- Response writer wrappers missing http.Flusher broke gRPC streaming — caught in post-merge testing
- 9 human verification items deferred to runtime testing (grpcurl, buf curl, reflection discovery)

### Patterns Established
- Predicate accumulation: []predicate.T with entity.And() for composable filter logic
- ConnectRPC simple codegen for cleaner handler signatures (direct params, not connect.Request wrappers)
- connectcors helpers for merging Connect/gRPC/gRPC-Web content types with existing CORS config

### Key Lessons
1. Response writer wrappers MUST implement http.Flusher — gRPC streaming requires it
2. entproto generates message types only; service definitions need manual authoring
3. ConnectRPC's http.Handler compatibility eliminates the need for a separate gRPC port

### Cost Observations
- Model mix: ~85% opus, ~15% sonnet (subagents)
- Sessions: ~2 sessions
- Notable: 4 phases shipped in one day including post-merge fixes

---

## Milestone: v1.5 — Tech Debt & Observability

**Shipped:** 2026-03-24
**Phases:** 3 | **Plans:** 9 | **Tasks:** 12

### What Was Built
- Corrected stale planning docs (PROJECT.md, Phase 7 summary) to accurately reflect DataLoader removal and IsPrimary conversion
- Flag-gated live test verifying meta.generated field behavior across three PeeringDB request patterns
- Per-type object count gauges (Int64ObservableGauge) for all 13 PeeringDB types wired into main.go
- 5-row Grafana dashboard (30 panels) with sync health, HTTP RED, per-type sync, Go runtime, and business metrics
- Verified all 26 deferred human verification items from v1.2-v1.4 against live Fly.io deployment

### What Worked
- OTLP autoexport eliminated need for dedicated Prometheus endpoint — simpler config via env vars
- Hand-authored Grafana dashboard JSON was faster than Grafonnet/Jsonnet for a single dashboard
- DS_PROMETHEUS template variable kept dashboard portable across Grafana instances
- Human verification phase (Phase 20) caught real issues and confirmed all major features work in production
- Milestone audit before completion caught stale roadmap criterion text (OBS-01 Prometheus vs OTLP)

### What Was Inefficient
- Phase directories were archived before milestone completion workflow ran, causing plan/summary counts to show 0/N
- VFY requirements (VFY-01 through VFY-09) were unchecked in REQUIREMENTS.md despite being satisfied per the audit
- Some verification items (syncing page animation, 500 error page) are inherently untestable non-destructively
- Gap closure plans (19-03, 19-04) were needed because initial execution missed InitObjectCountGauges wiring

### Patterns Established
- Observable gauge pattern: `meter.Int64ObservableGauge` with callback querying ent count
- Grafana dashboard portability: `__inputs` + `${datasource}` variable + null id/version
- Flag-gated live API tests: `-live` build tag for tests hitting external APIs
- Strikethrough in PROJECT.md for resolved tech debt items to preserve history

### Key Lessons
1. Gap closure plans are a sign of incomplete verification during initial execution — invest more in plan-check
2. OTLP autoexport is the right default for new Go services — no manual endpoint configuration
3. Human verification phases should be planned early, not deferred to final milestone
4. Milestone audit is worth running even for small milestones — it caught stale criterion text

### Cost Observations
- Model mix: ~85% opus, ~15% sonnet (subagents)
- Sessions: ~2 sessions
- Notable: Smallest milestone by plan count (9 plans), but included human verification across all prior milestones

---

## Milestone: v1.4 — Web UI

**Shipped:** 2026-03-24
**Phases:** 5 | **Plans:** 11 | **Tasks:** 22

### What Was Built
- templ + htmx + Tailwind CSS web UI with dual rendering (full page vs htmx fragment)
- Live search with errgroup fan-out across 6 entity types, grouped results with count badges
- Detail pages for all 6 entity types with lazy-loaded collapsible sections via htmx fragments
- ASN comparison tool with map-based set intersection for shared IXPs/facilities/campuses
- Dark mode with system preference detection and manual toggle, CSS transitions, loading indicators
- Styled 404/500 error pages, About page with live data freshness
- Keyboard navigation with ARIA listbox/option roles for search results

### What Worked
- templ + htmx eliminated need for JS build toolchain — no Node.js, no bundler, no SPA complexity
- Dual render mode (full page vs fragment) via HX-Request header kept templates DRY
- errgroup fan-out for search queries gave sub-300ms results across 6 types
- Tailwind CDN avoided build-step complexity at the cost of ~300KB (acceptable trade-off for dev velocity)
- Prefix-based dispatch routing handled all detail page URLs with a clean switch statement

### What Was Inefficient
- 13-02 and 17-03 plan checkboxes in ROADMAP.md weren't ticked during execution (stale state)
- 20 human verification items deferred — browser UX testing not possible in CLI sessions
- Nyquist validation skipped for Phases 16-17 (research phase omitted for speed)

### Patterns Established
- renderPage helper with HX-Request check and Vary header for dual rendering
- Fragment endpoints that bypass renderPage, write bare HTML directly to ResponseWriter
- SearchGroup/SearchResult types in templates package to avoid circular imports
- Class-based dark mode via @custom-variant with localStorage persistence
- IIFE script pattern in layout for keyboard navigation without global scope pollution

### Key Lessons
1. Server-rendered HTML with htmx handles complex UI interactions (live search, lazy loading, comparison) without SPA complexity
2. CDN-delivered CSS (Tailwind) is the right trade-off for projects without a frontend build pipeline
3. Human verification items accumulate fast in UI milestones — plan for manual testing sessions
4. Map-based set intersection is the cleanest pattern for "where can we peer?" comparisons

### Cost Observations
- Model mix: ~85% opus, ~15% sonnet (subagents)
- Sessions: ~3 sessions
- Notable: 5 phases (11 plans) completed in a single day — largest milestone by plan count

---

## Milestone: v1.3 — PeeringDB API Key Support

**Shipped:** 2026-03-24
**Phases:** 2 | **Plans:** 3 | **Tasks:** 4

### What Was Built
- WithAPIKey functional option on NewClient with Authorization header injection and 60 req/min authenticated rate limit
- 401/403 auth error handling with WARN logging, SEC-2 compliant startup logging
- pdbcompat-check CLI --api-key flag with env var fallback for authenticated conformance testing
- Live conformance test with conditional 1s/3s sleep based on authentication status

### What Worked
- Functional options pattern (ClientOption) made API key addition backward-compatible with zero caller changes
- TDD caught t.Setenv/t.Parallel conflict early — resolved by extracting resolveAPIKey helper
- Small milestone scope (2 phases, 3 plans) made execution fast and focused
- Reusing existing patterns (SEC-2 logging, header injection) from earlier milestones kept implementation clean

### What Was Inefficient
- Nothing significant — this was a well-scoped, surgical milestone

### Patterns Established
- ClientOption functional options for NewClient (variadic, backward-compatible)
- CLI flag with env var fallback: flag.StringVar then os.Getenv after flag.Parse()
- Auth error early-exit between body-discard and isRetryable check in retry loop

### Key Lessons
1. Small milestones with clear scope execute fastest — 2 phases completed in under 30 minutes
2. Functional options pattern is the right choice for optional configuration on existing constructors
3. SEC-2 compliance (never log secrets) should be a pattern, not an afterthought

### Cost Observations
- Model mix: ~90% opus, ~10% sonnet (subagents)
- Sessions: 1 session
- Notable: Entire milestone completed in ~30 minutes wall time

---

## Milestone: v1.1 — REST API & Observability

**Shipped:** 2026-03-23
**Phases:** 3 | **Plans:** 8 | **Tasks:** 16

### What Was Built
- OpenTelemetry HTTP client tracing with span hierarchy for PeeringDB sync calls
- Per-type sync metrics (duration, object count, delete count) with freshness observable gauge
- Auto-generated REST API at /rest/v1/ via entrest with OpenAPI spec, filtering, sorting, pagination, eager-loading
- PeeringDB-compatible drop-in API at /api/ with Django-style filters, depth expansion, search, and field projection

### What Worked
- entrest integration was straightforward — dual codegen with entgql worked first try
- TDD approach (failing tests first) caught integration issues early, especially REST mount overwrite by Phase 6
- Milestone audit caught the REST handler mount regression before shipping
- Phase 6 compat layer design decision (querying ent directly, not wrapping entrest) avoided complex adapter layer
- Generic Django-style filter parser handled all 13 types without per-type switch statements

### What Was Inefficient
- Phase branch merges created merge conflicts in STATE.md and REQUIREMENTS.md that needed manual resolution
- ROADMAP.md plan checkboxes weren't updated during execution, creating stale state
- REST handler mount was overwritten by Phase 6 commit — caught only during audit, should have been caught by integration tests
- Phase 4/5 velocity metrics not tracked in STATE.md (only phases 1-3 and 6 recorded)

### Patterns Established
- `func(*sql.Selector)` as universal predicate type for cross-entity filtering
- Type registry pattern for PeeringDB entity metadata (fields, edges, serializers)
- JSON round-trip for dynamic field injection (depth expansion _set fields)
- Wildcard route pattern `GET /api/{rest...}` for unified PeeringDB path handling

### Key Lessons
1. Cross-phase integration tests are essential — Phase 6 silently broke Phase 5's REST mount
2. Milestone audit is worth the cost — it caught a real regression
3. Phase branches with merge-back create friction — consider milestone branches or trunk-based development
4. Code-generated APIs (entrest, entgql) pay for themselves in consistency across 13 types

### Cost Observations
- Model mix: ~90% opus, ~10% sonnet (subagents)
- Sessions: ~4 sessions across v1.1
- Notable: Entire milestone completed in a single day (12 hours wall time)

---

## Milestone: v1.0 — PeeringDB Plus

**Shipped:** 2026-03-22
**Phases:** 3 | **Plans:** 14 | **Tasks:** 27

### What Was Built
- Full PeeringDB sync pipeline: HTTP client with rate limiting, pagination, retry for all 13 types
- entgo ORM with all 13 PeeringDB schemas, FK edges, mutation hooks
- Bulk upsert in FK dependency order with hard delete and sync status tracking
- Relay-compliant GraphQL API with playground, pagination, filtering, custom resolvers
- OpenTelemetry observability (traces, metrics, logs) with autoexport
- Health/readiness endpoints with LiteFS primary detection
- Fly.io deployment artifacts (Dockerfile.prod, litefs.yml, fly.toml)

### What Worked
- Schema extraction pipeline (Python Django source -> JSON -> entgo schemas) avoided manual schema transcription errors
- entgo code generation eliminated boilerplate across 13 types
- Fixture-based integration tests caught real data handling issues
- OTel autoexport made observability configuration-free

### What Was Inefficient
- Initial OTel setup registered metrics but didn't wire recording (caught in v1.1 planning)
- DataLoader middleware was wired but unused (entgql handles N+1 natively)
- globalid.go exports were created but ent Noder handles ID resolution

### Patterns Established
- FK dependency order for bulk operations (upsert, delete)
- Fixture files for integration testing PeeringDB data
- entgo annotation-driven API generation (entgql annotations -> GraphQL schema)

### Key Lessons
1. Read the actual API responses, not the documentation — PeeringDB's spec diverges from reality
2. Code generation from a single source of truth (ent schemas) prevents drift across API surfaces
3. Start with observability infrastructure early — retrofitting is harder (v1.1 proved this)

### Cost Observations
- Model mix: ~85% opus, ~15% sonnet
- Sessions: ~3 sessions across v1.0
- Notable: Both milestones completed in same day — total project time ~1 day

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | ~3 | 3 | Initial patterns: TDD, fixture tests, code generation |
| v1.1 | ~4 | 3 | Milestone audit added, caught integration regression |
| v1.2 | ~3 | 4 | golangci-lint enforcement, golden file tests, CI pipeline |
| v1.3 | 1 | 2 | Smallest milestone — focused scope, fastest execution |
| v1.4 | ~3 | 5 | Largest milestone — full web UI, 11 plans in one day |
| v1.5 | ~2 | 3 | Final cleanup — tech debt, observability, human verification |
| v1.6 | ~2 | 4 | ConnectRPC/gRPC API — 13 services with typed filtering |
| v1.7 | 1 | 3 | Streaming RPCs + UI polish — fastest multi-phase milestone |

### Cumulative Quality

| Milestone | Plans | Tasks | Integration Tests |
|-----------|-------|-------|-------------------|
| v1.0 | 14 | 27 | 16 GraphQL + 4 sync |
| v1.1 | 8 | 16 | 15 REST + 25 filter + 7 compat |
| v1.2 | 9 | 18 | 39 golden file + conformance CLI |
| v1.3 | 3 | 4 | 7 client auth + 4 CLI auth |
| v1.4 | 11 | 22 | 80+ detail page tests, search + compare integration |
| v1.5 | 9 | 12 | 8 dashboard tests, flag-gated live API test, 26 human verification items |
| v1.6 | 9 | 18 | 13 ConnectRPC Get/List + filter tests |
| v1.7 | 6 | 11 | 20 streaming tests (filters, count, cancel, resume, incremental) |

### Top Lessons (Verified Across Milestones)

1. Code generation from entgo schemas is the right bet — scales to 13 types without per-type maintenance
2. Integration tests catch real issues that unit tests miss — all milestones benefited
3. Small, focused milestones (v1.3) execute fastest and with fewest issues
4. Functional options pattern enables backward-compatible extension of constructors
5. SEC-2 compliance as a pattern (never log secrets) prevents security issues at every integration point
6. Human verification items accumulate across milestones — plan dedicated verification phases early rather than deferring to the end
7. Streaming handlers are highly templatable — one reference implementation enables mechanical replication across N entity types
8. Run milestone completion before archiving phase directories — CLI needs phases in place to count stats
