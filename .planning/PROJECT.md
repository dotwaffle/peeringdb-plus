# PeeringDB Plus

## What This Is

A high-performance, globally distributed, read-only mirror of PeeringDB data. Syncs all 13 PeeringDB object types via full re-fetch (hourly or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and exposes the data through a GraphQL API with rich filtering, relationship traversal, and an interactive playground. Built in Go using entgo as the ORM, with full OpenTelemetry observability.

## Core Value

Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## Requirements

### Validated

- [x] Sync all PeeringDB objects via full re-fetch (hourly or on-demand) — Validated in Phase 1: Data Foundation
- [x] Store data in SQLite using entgo ORM — Validated in Phase 1: Data Foundation
- [x] Handle PeeringDB API response format discrepancies — Validated in Phase 1: Data Foundation
- [x] Expose data via GraphQL (entgql) with filtering, pagination, relationship traversal — Validated in Phase 2: GraphQL API
- [x] Interactive GraphQL playground with example queries — Validated in Phase 2: GraphQL API
- [x] CORS headers for browser integrations — Validated in Phase 2: GraphQL API
- [x] Lookup by ASN and ID — Validated in Phase 2: GraphQL API
- [x] Deploy on Fly.io with LiteFS for global edge distribution — Validated in Phase 3: Production Readiness
- [x] OpenTelemetry tracing, metrics, and logs throughout — Validated in Phase 3: Production Readiness
- [x] Health/readiness endpoints with sync age check — Validated in Phase 3: Production Readiness

### Active
- [ ] Expose data via gRPC (entproto)
- [ ] Expose data via OpenAPI REST (entrest)
- [ ] Fully public — no authentication required
- [ ] Web UI for browsing data (HTMX + Templ) — secondary priority
### Out of Scope

- Write-path / data modification — this is a read-only mirror
- User accounts or authentication — fully public
- OAuth or API key gating — not needed for v1
- Mobile app — web-first
- Real-time streaming of changes — periodic sync is sufficient

## Context

- PeeringDB (https://github.com/peeringdb/peeringdb) is the authoritative database for network interconnection data (organizations, networks, IXPs, facilities, etc.)
- PeeringDB suffers from poor performance, single-region hosting (AWS), and an API spec that doesn't match actual API responses
- The PeeringDB API response format diverges from their OpenAPI specification — the original Python source code must be analyzed to understand the actual response shapes
- LiteFS on Fly.io enables SQLite replication to edge nodes worldwide, giving every region local read latency
- entgo provides code generation for the ORM layer, with ecosystem packages for GraphQL (entgql), gRPC (entproto), and REST (entrest via https://github.com/lrstanley/entrest)

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

Shipped v1.0 with 3 phases, 14 plans, 27 tasks. Go codebase using entgo ORM, modernc.org/sqlite, gqlgen GraphQL, OpenTelemetry. Deployment artifacts ready for Fly.io with LiteFS edge replication.

**Known tech debt (from v1.0 audit):**
- Custom sync metrics registered but not recorded by sync worker
- DataLoader middleware wired but unused (entgql handles N+1 natively)
- Vestigial config.IsPrimary field (replaced by LiteFS detection)
- PeeringDB HTTP client lacks OTel trace spans
- graph/globalid.go exported functions unused (ent Noder handles it)

---
*Last updated: 2026-03-22 after v1.0 milestone complete*
