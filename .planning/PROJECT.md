# PeeringDB Plus

## What This Is

A high-performance, globally distributed, read-only mirror of PeeringDB data with a modern web interface. Syncs all 13 PeeringDB object types via full or incremental re-fetch (hourly or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and exposes the data through five surfaces: a web UI (templ + htmx + Tailwind CSS) with live search, detail pages, and ASN comparison; GraphQL (with playground); OpenAPI REST (with auto-generated spec); a PeeringDB-compatible drop-in replacement API; and ConnectRPC/gRPC with all 13 types queryable via Get/List RPCs with typed filtering, reflection, and health checking. Supports optional PeeringDB API key authentication for higher sync rate limits. Built in Go using entgo as the ORM, with full OpenTelemetry observability.

## Core Value

Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## Requirements

### Validated

- [x] Sync all PeeringDB objects via full re-fetch (hourly or on-demand) — v1.0
- [x] Store data in SQLite using entgo ORM — v1.0
- [x] Handle PeeringDB API response format discrepancies — v1.0
- [x] Expose data via GraphQL (entgql) with filtering, pagination, relationship traversal — v1.0
- [x] Interactive GraphQL playground with example queries — v1.0
- [x] CORS headers for browser integrations — v1.0
- [x] Lookup by ASN and ID — v1.0
- [x] Deploy on Fly.io with LiteFS for global edge distribution — v1.0
- [x] OpenTelemetry tracing, metrics, and logs throughout — v1.0
- [x] Health/readiness endpoints with sync age check — v1.0
- [x] OTel trace spans on PeeringDB HTTP client — v1.1
- [x] Sync metrics reviewed, expanded, and wired to record — v1.1
- [x] Expose data via OpenAPI REST (entrest) — v1.1
- [x] Full PeeringDB-compatible REST layer (paths, response envelope, query params, field names) — v1.1
- [x] Fully public — verify no auth barriers, document public access model — v1.2
- [x] Golden file tests for PeeringDB compatibility layer — v1.2
- [x] CI pipeline (GitHub Actions) enforcing tests, linting, and vetting — v1.2
- [x] All tests pass with -race, all linters pass clean — v1.2
- [x] Optional PeeringDB API key for authenticated sync with higher rate limits — v1.3
- [x] Conformance tooling uses API key for authenticated PeeringDB access — v1.3

- [x] Live search across all PeeringDB types with instant results — v1.4
- [x] Record detail views for all 6 types with collapsible lazy-loaded sections — v1.4
- [x] ASN comparison tool showing shared IXPs, facilities, and campuses — v1.4
- [x] Linkable/shareable URLs for every page — URL is the state — v1.4
- [x] Polished design with dark mode, transitions, keyboard navigation, error pages — v1.4
- [x] Verify meta.generated field behavior for depth=0 paginated PeeringDB responses; graceful fallback if missing — v1.5
- [x] Remove unused DataLoader middleware and convert WorkerConfig.IsPrimary to dynamic LiteFS detection — v1.5
- [x] Verify all 26 deferred human verification items against live Fly.io deployment — v1.5
- [x] Grafana dashboard (JSON provisioning) covering sync health, API traffic, infrastructure, and business metrics — v1.5
- [x] App serves traffic directly without LiteFS HTTP proxy, h2c for gRPC wire protocol — v1.6
- [x] Proto definitions for all 13 PeeringDB types via entproto + buf + ConnectRPC codegen — v1.6
- [x] Get + List RPCs for all 13 types via ConnectRPC with typed filtering and pagination — v1.6
- [x] gRPC server reflection (v1 + v1alpha) for grpcurl/grpcui discovery — v1.6
- [x] gRPC health checking with sync-readiness-driven status — v1.6
- [x] otelconnect observability interceptor on all ConnectRPC handlers — v1.6
- [x] CORS updated for Connect, gRPC, and gRPC-Web content types — v1.6

- [x] Server-streaming RPCs for bulk data export (stream rows from DB, no full buffering) — v1.7
- [x] since_id stream resume and updated_since incremental filter on streaming RPCs — v1.7
- [x] IX presence UI improvements (field labels, RS badge, port speed colors, copyable text) — v1.7

- [x] Terminal client detection (User-Agent sniffing for curl, wget, HTTPie, etc.) — v1.8
- [x] Content negotiation under existing /ui/ URLs — browsers unchanged, terminals get text — v1.8
- [x] CLI help text at /ui/ for terminal clients listing available endpoints — v1.8
- [x] Text-formatted error responses (404, 500) for terminal clients — v1.8
- [x] Rich 256-color ANSI output with Unicode box drawing for all 6 entity types — v1.8
- [x] Plain text mode (?T) and JSON mode (?format=json) as alternative output formats — v1.8
- [x] WHOIS/RPSL format output (?format=whois) for all entity types — v1.8
- [x] Short format one-liner mode (?format=short) for scripting — v1.8
- [x] Data freshness timestamp footer on all terminal responses — v1.8
- [x] Section filtering (?section=ix,fac) for detail views — v1.8
- [x] Width adaptation (?w=N) with progressive column dropping — v1.8
- [x] Bash and zsh shell completion scripts — v1.8
- [x] Terminal search results and ASN comparison — v1.8

### Active

## Current Milestone: v1.9 Hardening & Polish

**Goal:** Improve performance, code quality, architecture consistency, and UI polish across the entire codebase — no new features, just making what exists better.

**Target features:**
- ~~Query optimization (eliminate double-count queries, add missing indexes, HTTP caching)~~ — completed Phase 34+35
- ~~gRPC handler deduplication (~1,154 lines of near-identical code across 13 services)~~ — completed Phase 33
- ~~Error format unification across all 6 API surfaces~~ — completed Phase 34 (RFC 9457)
- ~~ConnectRPC filter parity with PeeringDB compat layer~~ — completed Phase 33
- ~~Structured error logging fix (90 instances)~~ — completed Phase 32
- ~~Test coverage expansion (grpcserver, middleware)~~ — completed Phase 33
- ~~Benchmark suite for hot paths~~ — completed Phase 35
- ~~WCAG AA accessibility fixes (contrast, ARIA, form labels)~~ — completed Phase 36
- ~~Bookmarkable search results~~ — completed Phase 36
- ~~htmx error handling for collapsible sections~~ — completed Phase 36
- ~~Breadcrumbs, mobile menu fixes, visual polish~~ — completed Phase 36
- ~~Terminal line wrapping and error rendering~~ — completed Phase 36

### Deferred

- [ ] SyncStatus custom RPC — deferred, available via existing REST/GraphQL
- [ ] Per-ASN BGP summary from bgp.tools (prefix counts, RPKI coverage) — needs design
- [ ] IRR/AS-SET membership from WHOIS source — needs design
- [ ] IP prefix lookup with origin ASN, RPKI status — needs design

### Out of Scope

- Write-path / data modification — this is a read-only mirror
- User accounts or end-user authentication — fully public read access
- Per-user API key management or rotation — server-side config, restart to change
- Mobile app — web-first
- Real-time streaming of changes — periodic sync is sufficient

## Context

- PeeringDB is the authoritative database for network interconnection data (organizations, networks, IXPs, facilities, etc.)
- PeeringDB suffers from poor performance, single-region hosting, and an API spec that doesn't match actual API responses
- LiteFS on Fly.io enables SQLite replication to edge nodes worldwide, giving every region local read latency
- entgo provides code generation for the ORM layer, with ecosystem packages for GraphQL (entgql), gRPC (entproto), and REST (entrest)

## Constraints

- **Language**: Go 1.26
- **ORM**: entgo (non-negotiable — ecosystem drives GraphQL/gRPC/REST generation)
- **Storage**: SQLite + LiteFS (enables edge distribution without a central database)
- **Platform**: Fly.io (LiteFS dependency, global edge deployment)
- **Observability**: OpenTelemetry — mandatory for tracing, metrics, and logs
- **Data fidelity**: Must handle PeeringDB's actual API responses, not their documented spec

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Full re-fetch sync (not incremental) | Simpler implementation, guarantees data consistency | ✓ Validated Phase 1 |
| SQLite + LiteFS over PostgreSQL | Enables edge-local reads on Fly.io without central DB latency | ✓ Validated Phase 1 |
| entgo as ORM | Ecosystem packages (entgql, entproto, entrest) generate all API surfaces from schema | ✓ Validated Phase 1 |
| GraphQL as first API surface for v1 | Flexible querying, entgql is mature, good fit for network data exploration | ✓ Validated Phase 2 |
| rs/cors for CORS middleware | Well-maintained, stdlib-compatible, simple config | ✓ Validated Phase 2 |
| Autoexport for OTel exporters | Environment-driven exporter selection, no hardcoded endpoints | ✓ Validated Phase 3 |
| Dual slog handler (stdout + OTel) | Structured logs to both console and OTel pipeline simultaneously | ✓ Validated Phase 3 |
| LiteFS .primary file for leader detection | Inverted semantics (.primary exists on replicas), fallback to env var | ✓ Validated Phase 3 |
| otelhttp.NewTransport + manual parent spans | Both automatic HTTP semantics AND business-level span hierarchy for PeeringDB calls | ✓ Validated Phase 4 |
| Flat metric naming with type attribute | pdbplus.sync.type.* with type=net|ix|fac — fewer instruments, filter by type | ✓ Validated Phase 4 |
| entrest for REST API generation | Code-generated REST alongside entgql from same schemas, read-only config | ✓ Validated Phase 5 |
| PeeringDB compat layer queries ent directly | NOT wrapping entrest — different response envelopes, query parameters, and serialization requirements | ✓ Validated Phase 6 |
| Generic Django-style filter parser | One parser handles all 13 types via shared func(*sql.Selector) predicate type | ✓ Validated Phase 6 |
| Golden file tests with go-cmp for compat layer | 39 golden files (13 types x 3 scenarios) with -update flag for regeneration | ✓ Validated Phase 9 |
| Structure-only conformance comparison | CompareStructure checks field names/types/nesting, not values — handles live data changes | ✓ Validated Phase 9 |
| GitHub Actions CI with 4 parallel jobs | lint + go generate drift, test -race, build, govulncheck — coverage PR comments via gh api | ✓ Validated Phase 10 |
| Public access by design | All read endpoints unauthenticated; only POST /sync gated; root endpoint self-documents | ✓ Validated Phase 10 |
| ClientOption functional options for NewClient | Backward-compatible variadic opts; WithAPIKey injects auth header without breaking callers | ✓ Validated Phase 11 |
| 401/403 auth errors never retried | Placed between body-discard and isRetryable check; WARN log with SEC-2 compliance | ✓ Validated Phase 11 |
| CLI flag with env var fallback for API key | --api-key flag takes precedence over PDBPLUS_PEERINGDB_API_KEY env var | ✓ Validated Phase 12 |
| templ + htmx + Tailwind CDN for web UI | Type-safe server-rendered HTML, no JS build toolchain, no SPA complexity | ✓ Validated Phase 13 |
| Tailwind via CDN (no build step) | Eliminates Node.js dependency; trade-off: ~300KB download, no tree-shaking | ✓ Validated Phase 13 |
| Dual render mode (full page vs htmx fragment) | Single renderPage helper checks HX-Request, sets Vary header | ✓ Validated Phase 13 |
| errgroup fan-out for search across 6 types | Parallel LIKE queries, 10 results + count per type | ✓ Validated Phase 14 |
| Networks by ASN in URLs (/ui/asn/{asn}) | Users think in ASNs, not internal IDs | ✓ Validated Phase 15 |
| Pre-computed count fields for summary stats | ix_count, fac_count etc. from PeeringDB sync, avoid extra count queries | ✓ Validated Phase 15 |
| Map-based set intersection for ASN comparison | Load presences for both networks, compute shared IXPs/facilities/campuses in Go | ✓ Validated Phase 16 |
| Class-based dark mode with localStorage | @custom-variant dark, system preference detection, manual toggle persists | ✓ Validated Phase 17 |
| IsPrimary as func() bool, not static bool | Dynamic LiteFS primary detection on each sync cycle start | ✓ Validated Phase 18 |
| OTLP autoexport for Prometheus metrics | No /metrics endpoint needed — OTEL_METRICS_EXPORTER=prometheus with autoexport | ✓ Validated Phase 19 |
| Hand-authored Grafana dashboard JSON | Simpler than Grafonnet/Jsonnet for single dashboard; DS_PROMETHEUS template variable for portability | ✓ Validated Phase 19 |
| Single pdbplus.data.type.count gauge with type attribute | One instrument for all 13 PeeringDB types, filter by type label | ✓ Validated Phase 19 |
| ConnectRPC over google.golang.org/grpc | Standard net/http handlers, same mux as REST/GraphQL, supports Connect+gRPC+gRPC-Web on one port | ✓ Validated Phase 23 |
| Remove LiteFS proxy, app-level fly-replay | LiteFS proxy is HTTP/1.1 only, blocks h2c/gRPC; app already handles POST /sync replay | ✓ Validated Phase 21 |
| entproto for .proto generation, skip protoc-gen-entgrpc | entproto generates standard .proto files; entgrpc is hardcoded to google.golang.org/grpc interfaces | ✓ Validated Phase 22 |
| Manual services.proto over entproto service generation | entproto only generates message types, not service definitions; manual services.proto with Get/List RPCs for all 13 types | ✓ Validated Phase 22 |
| Predicate accumulation for List filtering | Nil-check optional proto fields, validate, accumulate ent predicates, apply via entity.And() | ✓ Validated Phase 24 |
| StreamNetworks naming convention (not StreamAllNetworks) | Concise, mirrors ListNetworks pattern | ✓ Validated Phase 25 |
| Hardcoded 500-row batch size for streaming | Simple, sufficient for PeeringDB data volumes (~200K max rows) | ✓ Validated Phase 25 |
| WithoutTraceEvents() globally on otelconnect | Per-message events at 200K rows is telemetry explosion; per-RPC interceptor not feasible | ✓ Validated Phase 25 |
| grpc-total-count response header for streaming | gRPC metadata convention; set before first Send() via stream.ResponseHeader() | ✓ Validated Phase 25 |
| Configurable StreamTimeout via PDBPLUS_STREAM_TIMEOUT | 60s default; prevents indefinite connection hold from slow clients | ✓ Validated Phase 25 |
| google.protobuf.Timestamp for updated_since | Standard protobuf well-known type, nanosecond precision, widely supported | ✓ Validated Phase 26 |
| since_id as both predicate and cursor | IDGT predicate affects count (grpc-total-count reflects remaining), sets initial lastID | ✓ Validated Phase 26 |
| 5-tier port speed color coding | Sub-1G gray, 1G neutral, 10G blue, 100G emerald, 400G+ amber — networking industry intuitive gradient | ✓ Validated Phase 27 |
| CopyableIP with click-to-copy + hover icon | Best discoverability — both click-on-IP and explicit clipboard icon | ✓ Validated Phase 27 |
| lipgloss v2 + colorprofile for terminal rendering | Force ANSI256 over HTTP (not TTY), NoTTY for plain text; vanity domain charm.land/lipgloss/v2 | ✓ Validated Phase 28 |
| Type-switch dispatch in RenderPage | Concrete type assertion over interface polymorphism — simpler, explicit, grep-able | ✓ Validated Phase 29 |
| Eager-load child rows unconditionally | All 6 detail handlers eager-load regardless of render mode — simplifies handler logic | ✓ Validated Phase 30 |
| RPSL aut-num class for WHOIS format | RFC 2622 aut-num for networks; custom ix:/site:/organisation:/campus:/carrier: for other types | ✓ Validated Phase 30 |
| Renderer struct fields for per-request state | Sections/Width as exported fields set before RenderPage — avoids signature explosion (CS-5) | ✓ Validated Phase 31 |
| Server-side completion search returning IDs only | Prevents shell injection from entity names containing special characters | ✓ Validated Phase 31 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

## Current State

Shipped v1.8 with 31 phases across 9 milestones (v1.0-v1.8). The terminal CLI interface is complete — all 6 PeeringDB entity types, search, and comparison are accessible via `curl` with rich ANSI output, plain text, JSON, WHOIS/RPSL, and short one-liner formats. Section filtering, width adaptation, data freshness footer, and shell completions (bash/zsh) round out the power-user experience. Go codebase using entgo ORM, modernc.org/sqlite, gqlgen GraphQL, entrest REST, custom PeeringDB compat layer, ConnectRPC/gRPC API with streaming, web UI (templ + htmx + Tailwind CSS), OpenTelemetry with Grafana dashboard. Six user-facing surfaces: Web UI at /ui/, Terminal CLI at /ui/ (curl), GraphQL at /graphql, REST at /rest/v1/, PeeringDB compat at /api/, ConnectRPC at /peeringdb.v1.*/. Application serves traffic directly (no LiteFS proxy) with h2c support. 85 files, +16K lines added in v1.8.

**Known tech debt:**
- Nyquist validation frontmatter not formally signed off across v1.8 phases (VALIDATION.md exists, tests pass)
- fly_region Grafana template variable needs verification after multi-region deployment
- Go runtime metric names need verification against live Grafana Cloud
- /ui/about terminal rendering falls through to generic stub (not in v1.8 scope)
- All 8 grpcserver/convert.go helpers confirmed in use (boolPtrVal, float64PtrVal NOT unused)
- Search service runs 12 queries per search (double-count pattern)
- JSON marshal/unmarshal roundtrip for field projection in pdbcompat
- 90 instances of slog.String("error", err.Error()) instead of slog.Any("error", err)
- 13 gRPC handlers with ~1,154 lines of near-identical code
- Error formats inconsistent across 6 API surfaces
- ConnectRPC exposes fewer filters than PeeringDB compat
- WCAG AA contrast failures in dark mode (text-neutral-600)
- Search results not bookmarkable (no URL history push)

---
*Last updated: 2026-03-26 after v1.9 milestone start*
