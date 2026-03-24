# PeeringDB Plus

## What This Is

A high-performance, globally distributed, read-only mirror of PeeringDB data with a modern web interface. Syncs all 13 PeeringDB object types via full or incremental re-fetch (hourly or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and exposes the data through four surfaces: a web UI (templ + htmx + Tailwind CSS) with live search, detail pages, and ASN comparison; GraphQL (with playground); OpenAPI REST (with auto-generated spec); and a PeeringDB-compatible drop-in replacement API. Supports optional PeeringDB API key authentication for higher sync rate limits. Built in Go using entgo as the ORM, with full OpenTelemetry observability.

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

### Active

- [x] Verify meta.generated field behavior for depth=0 paginated PeeringDB responses; graceful fallback if missing — Validated in Phase 18: Tech Debt & Data Integrity
- [x] Remove unused DataLoader middleware (removed v1.2 Phase 7) and convert WorkerConfig.IsPrimary to dynamic LiteFS detection (quick task 260324-lc5) — Validated in Phase 18: Tech Debt & Data Integrity
- [x] Verify all 26 deferred human verification items against live Fly.io deployment — Validated in Phase 20: Deferred Human Verification
- [x] Grafana dashboard (JSON provisioning) covering sync health, API traffic, infrastructure, and business metrics — Validated in Phase 19: Prometheus Metrics & Grafana Dashboard

### Deferred

- [ ] Expose data via gRPC (entproto) — deferred to future milestone

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

## Current Milestone: v1.5 Tech Debt & Observability

**Goal:** Clean up accumulated tech debt, verify all deferred items against live Fly.io deployment, and build comprehensive Grafana dashboards for production monitoring.

**Target features:**
- Verify meta.generated field behavior for depth=0 paginated responses; graceful fallback if missing
- ~~Remove unused DataLoader middleware and WorkerConfig.IsPrimary dead field~~ Both resolved: DataLoader removed in v1.2 Phase 7, IsPrimary converted to live `func() bool` by quick task 260324-lc5
- Verify all 26 deferred human verification items against live deployment
- Grafana dashboard (JSON provisioning) with sync health, API traffic, infrastructure, and business metrics

## Current State

Shipped v1.4 with 17 phases across 4 milestones (v1.0-v1.4), 45 plans, 87 tasks. Go codebase using entgo ORM, modernc.org/sqlite, gqlgen GraphQL, entrest REST, custom PeeringDB compat layer, web UI (templ + htmx + Tailwind CSS), OpenTelemetry. Four user-facing surfaces: Web UI at /ui/ (search, detail pages, ASN comparison), GraphQL at /graphql, REST at /rest/v1/, PeeringDB compat at /api/. Codebase passes golangci-lint v2 clean. Live deployment running on Fly.io.

**Known tech debt (being addressed in v1.5):**
- ~~DataLoader middleware~~ Removed in v1.2 Phase 7
- ~~WorkerConfig.IsPrimary dead field~~ Converted from dead `bool` to live `func() bool` by quick task 260324-lc5; now wired to `litefs.IsPrimaryWithFallback()` for dynamic primary detection
- 6 human verification items deferred from v1.2/v1.3 (CI execution, coverage comments, API key live testing)
- 20 human verification items from v1.4 (visual/browser UX — dark mode, keyboard nav, responsive layout, transitions)
- meta.generated field behavior unverified for depth=0 paginated PeeringDB responses
- Nyquist validation incomplete for Phases 16-17 (research skipped)

---
*Last updated: 2026-03-24 after v1.5 milestone started*
