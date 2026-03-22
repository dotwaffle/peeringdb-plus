# PeeringDB Plus

## What This Is

A high-performance, globally distributed, read-only mirror of PeeringDB data. It syncs all PeeringDB objects via full re-fetch on a regular schedule (hourly or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and presents the data through modern API surfaces: GraphQL, gRPC, and OpenAPI-compliant REST. Built in Go using entgo as the ORM.

## Core Value

Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] Sync all PeeringDB objects via full re-fetch (hourly or on-demand)
- [ ] Store data in SQLite using entgo ORM
- [ ] Deploy on Fly.io with LiteFS for global edge distribution
- [ ] Expose data via GraphQL (entgql)
- [ ] Expose data via gRPC (entproto)
- [ ] Expose data via OpenAPI REST (entrest)
- [ ] OpenTelemetry tracing, metrics, and logs throughout
- [ ] Fully public — no authentication required
- [ ] Web UI for browsing data (HTMX + Templ) — secondary priority
- [ ] Handle PeeringDB API response format discrepancies (responses don't match their OpenAPI spec — Python source analysis required)

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
| Full re-fetch sync (not incremental) | Simpler implementation, guarantees data consistency | — Pending |
| SQLite + LiteFS over PostgreSQL | Enables edge-local reads on Fly.io without central DB latency | — Pending |
| entgo as ORM | Ecosystem packages (entgql, entproto, entrest) generate all API surfaces from schema | — Pending |
| GraphQL as first API surface for v1 | Flexible querying, entgql is mature, good fit for network data exploration | — Pending |

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

---
*Last updated: 2026-03-22 after initialization*
