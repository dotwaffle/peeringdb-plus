# PeeringDB Plus

## What This Is

A high-performance, globally distributed, read-only mirror of PeeringDB data. Syncs all 13 PeeringDB object types via full re-fetch (hourly or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and exposes the data through three API surfaces: GraphQL (with playground), OpenAPI REST (with auto-generated spec), and a PeeringDB-compatible drop-in replacement API. Built in Go using entgo as the ORM, with full OpenTelemetry observability including per-type sync metrics and HTTP client tracing.

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

### Active

- [x] Fully public — verify no auth barriers, document public access model — v1.2 Phase 10
- [x] Golden file tests for PeeringDB compatibility layer — v1.2 Phase 9
- [x] CI pipeline (GitHub Actions) enforcing tests, linting, and vetting — v1.2 Phase 10
- [x] All tests pass with -race, all linters pass clean — v1.2 Phase 7
- [ ] Expose data via gRPC (entproto) — deferred to future milestone
- [ ] Web UI for browsing data (HTMX + Templ) — deferred to future milestone

## Current Milestone: v1.2 Quality & CI

**Goal:** Harden the codebase with golden file tests for the PeeringDB compat layer, verify and document the fully-public access model, and establish CI enforcement via GitHub Actions.

**Target features:**
- Verify and document fully-public access model
- Configurable incremental sync mode with per-type delta fetches
- Golden file tests for PeeringDB compatibility layer API responses
- Fix all test and linter issues across the codebase
- GitHub Actions CI pipeline enforcing tests, linting, and vetting on every PR

### Out of Scope

- Write-path / data modification — this is a read-only mirror
- User accounts or authentication — fully public
- OAuth or API key gating — not needed for current scope
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

Shipped v1.2 with 10 phases (3 from v1.0 + 3 from v1.1 + 4 from v1.2), 31 plans, 61 tasks. Go codebase using entgo ORM, modernc.org/sqlite, gqlgen GraphQL, entrest REST, custom PeeringDB compat layer, OpenTelemetry with per-type sync metrics. Three API surfaces: GraphQL at /graphql, REST at /rest/v1/, PeeringDB compat at /api/. Codebase passes golangci-lint v2 clean. Sync supports full re-fetch and incremental delta fetch with per-type cursor tracking. 39 golden files lock down PeeringDB compat layer responses. GitHub Actions CI enforces lint, test (-race), build, and govulncheck on every PR.

**Known tech debt:**
- DataLoader middleware wired but unused (entgql handles N+1 natively)
- WorkerConfig.IsPrimary dead field (replaced by LiteFS detection, explicitly deferred)
- 3 human verification items deferred (CI execution on GitHub, coverage comment posting, comment deduplication — require actual GitHub push)
- meta.generated field behavior unverified for depth=0 paginated PeeringDB responses (fallback covers this)

---
*Last updated: 2026-03-24 after v1.2 (Quality, Incremental Sync & CI) milestone complete*
