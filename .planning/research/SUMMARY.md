# Project Research Summary

**Project:** PeeringDB Plus
**Domain:** Globally distributed read-only data mirror with multi-protocol API surfaces
**Researched:** 2026-03-22
**Confidence:** MEDIUM-HIGH

## Executive Summary

PeeringDB Plus is a read-only mirror of PeeringDB data, deployed globally on Fly.io edge nodes using SQLite replicated via LiteFS. The core architectural decision is schema-driven code generation: all 13 PeeringDB object types are defined as entgo schemas, and three API surfaces (GraphQL, gRPC, REST) are generated from those schemas using entgql, entproto, and entrest respectively. This single-source-of-truth approach eliminates API surface drift but creates a hard dependency on the ent extension ecosystem, which varies in maturity -- entgql is production-grade, entproto is experimental, and entrest warns of breaking changes despite reaching v1.0.

The riskiest dependency is LiteFS. Fly.io discontinued LiteFS Cloud in October 2024 and placed LiteFS itself in maintenance mode with limited support. The technology works and is stable in production, but the team must budget for self-reliance on debugging and must architect a migration path (Litestream for backups, storage abstraction for potential Turso/LibSQL migration). The second major risk is PeeringDB's API itself: its OpenAPI spec does not match actual response shapes (confirmed in GitHub issues #1878, #1658, #637). No existing Go client handles this. A custom sync client built from analysis of PeeringDB's Django source code is required.

Go 1.26 with the Green Tea GC, stdlib net/http routing, and slog provides a minimal-dependency foundation. The CGo-free SQLite driver (modernc.org/sqlite) avoids cross-compilation issues on Fly.io. The dataset is small (~100MB) and read-only, meaning SQLite's performance characteristics are ideal: sub-millisecond local reads, WAL mode for concurrent reads during sync writes, and the entire dataset fits in page cache. The architecture scales horizontally by adding Fly.io regions (read replicas), not by sharding. The primary bottleneck is PeeringDB's upstream API rate limits during sync, not serving capacity.

## Key Findings

### Recommended Stack

The stack is Go-centric with minimal external dependencies, aligned with the project's coding standards (MD-1: prefer stdlib). All technologies have been version-verified.

**Core technologies:**
- **Go 1.26:** Application language. Green Tea GC (10-40% lower overhead), enhanced stdlib routing and structured logging.
- **entgo v0.14.5:** Schema-first ORM with code generation. Single schema definition drives all API surfaces.
- **entgql v0.7.0 + gqlgen:** Generates Relay-compliant GraphQL with pagination, filtering, sorting. The most mature ent extension. PRIMARY API surface.
- **entproto v0.7.0:** Generates protobuf definitions and gRPC services from ent schemas. Experimental but functional. Does NOT support M2M edges.
- **entrest v1.0.2:** Generates OpenAPI specs and REST handlers. Community-maintained by a single developer. Pin version strictly.
- **modernc.org/sqlite v1.36+:** CGo-free SQLite driver. Avoids cross-compilation issues. Works with ent via database/sql.
- **LiteFS v0.5.14:** FUSE-based SQLite replication across Fly.io regions. Maintenance mode. Pin version, add Litestream backup.
- **OpenTelemetry v1.35+:** Traces, metrics, logs. Bridge slog via otelslog. HTTP instrumentation via otelhttp.
- **templ v0.3.x + htmx 2.0.8:** Type-safe server-side HTML templating with browser interactivity. Web UI is secondary priority.
- **net/http stdlib:** HTTP server and router. Sufficient for generated handlers. Chi v5 as fallback only if middleware composition proves unwieldy.

**Version pinning:** All dependencies pinned in go.mod. Critical pins: ent v0.14.5, entrest v1.0.2, modernc.org/sqlite v1.36+, OTel v1.35+.

### Expected Features

**Must have (table stakes):**
- All 13 PeeringDB object types with correct field types matching actual API responses (not the buggy spec)
- Full query capabilities: filter by any field, numeric/string modifiers, ASN lookup, pagination, field selection
- Hourly (or better) sync with exposed last-sync timestamp
- REST API returning JSON with proper HTTP status codes, CORS, HTTPS
- High availability via multi-region deployment, no rate limiting (or very generous), low global latency
- `since` parameter support for downstream incremental sync

**Should have (differentiators):**
- GraphQL API with relationship traversal (flagship differentiator -- eliminates PeeringDB's N+1 API problem)
- gRPC API for high-performance programmatic access
- OpenAPI-compliant REST with a CORRECT spec (PeeringDB's own spec has bugs)
- Full-text search via SQLite FTS5
- Interactive GraphQL playground
- OpenTelemetry observability (traces, metrics, logs)
- Health/readiness endpoints with sync freshness checks

**Defer (v2+):**
- Web UI (explicitly secondary priority per project requirements)
- Geographic/geospatial queries (high complexity, niche use case)
- Webhooks/change notifications (requires change tracking infrastructure)
- Generated client SDKs (can be community-driven from published specs)
- ASN comparison features (nice-to-have, not blocking adoption)

**Anti-features (explicitly do NOT build):**
- Write/mutation API, user accounts, authentication, OAuth
- Drop-in PeeringDB API compatibility (would inherit their tech debt)
- Real-time streaming/WebSockets, historical data/time-series
- Mobile app, email notifications, data quality validation

### Architecture Approach

Single Go binary running on every Fly.io node, with two operating modes: primary (runs sync worker + serves reads) and replica (serves reads only). LiteFS handles replication transparently via FUSE filesystem interception. All API surfaces serve from every node. gRPC runs on a separate port (8082) because the LiteFS HTTP proxy only handles HTTP/1.1.

**Major components:**
1. **ent/schema/** -- 13 entgo schema definitions with entgql, entproto, entrest annotations. The single source of truth.
2. **internal/sync/** -- Custom PeeringDB API client with hand-written deserializers that handle spec-vs-reality divergences. Transaction-per-object-type sync in FK dependency order.
3. **internal/server/** -- HTTP and gRPC server setup, middleware composition (recovery, request ID, OTel, CORS, logging), health checks.
4. **internal/graphql/, internal/grpc/, internal/rest/** -- Generated API handlers from ent schemas.
5. **internal/otel/** -- OpenTelemetry SDK initialization, slog bridge, exporters, graceful shutdown.
6. **internal/litefs/** -- Primary detection via `.primary` file presence.

**Key patterns:**
- Schema-driven code generation (define once, generate three API surfaces)
- Transaction-per-object-type sync (avoids database-level write locks during entire sync)
- Separate read/write ent clients (read-only mode prevents accidental writes from API handlers)
- Primary-aware request handling (replicas forward writes via Fly-Replay header)
- Environment-based configuration with fail-fast validation (CFG-1, CFG-2)

### Critical Pitfalls

1. **LiteFS is unsupported and pre-1.0** -- Pin version, design storage abstraction layer for potential migration, implement Litestream backup to Tigris for disaster recovery. Disable autoscale stop for primary region.
2. **PeeringDB API responses don't match their OpenAPI spec** -- Analyze actual Django serializer source code, write custom deserializers, collect golden test fixtures from live API, subscribe to release notes. This is the FIRST thing to tackle.
3. **PeeringDB data has referential integrity violations** -- FK references to deleted entities exist in production. Use sync-in-dependency-order strategy. Consider soft FK approach or staging database. Log and reconcile orphaned references.
4. **Full re-fetch sync vs. rate limits and WAL growth** -- Use authenticated API access (40 req/min), per-table transactions, UPSERT strategy. Space requests 2+ seconds apart. Monitor WAL file size. Consider incremental sync via `since` after initial full fetch.
5. **entproto does not support M2M edges** -- Design schemas with intermediate "through" entities (which aligns with PeeringDB's own data model). Prioritize GraphQL as primary surface. Hand-write proto definitions for M2M-heavy parts if needed.

## Implications for Roadmap

Based on combined research, the following phase structure respects dependency ordering, groups related features, and sequences work from highest-confidence tooling to lowest.

### Phase 1: Data Foundation (Schema + SQLite + Sync)

**Rationale:** Everything depends on the data model being correct and data being present. The ent schema definitions must be validated against PeeringDB's actual API responses (not their spec) before any API surface can be generated. This is the highest-risk, highest-learning phase.
**Delivers:** All 13 PeeringDB object types modeled in entgo, stored in SQLite, populated via sync from PeeringDB API. Correct field types. FK dependency-ordered sync. UPSERT strategy for data freshness during sync.
**Addresses features:** All basic objects, all derived objects, carrier/campus objects, correct field types, deleted/status-filtered objects, hourly sync, last sync timestamp.
**Avoids pitfalls:** #2 (spec mismatch -- validate early), #3 (rate limits -- design sync strategy), #4 (FK violations -- handle during schema design), #8 (SQLite driver/PRAGMA -- configure correctly from day one).
**Stack elements:** Go 1.26, entgo v0.14.5, modernc.org/sqlite, custom PeeringDB HTTP client.

### Phase 2: GraphQL API (Primary Surface)

**Rationale:** GraphQL is the flagship differentiator and uses the most mature ent extension (entgql). It exercises the full entgo query path including pagination, filtering, and edge traversal. Building this second validates the schema design before generating secondary API surfaces.
**Delivers:** Relay-compliant GraphQL API with cursor pagination, field filtering, relationship traversal, schema introspection. Interactive GraphQL playground.
**Addresses features:** GraphQL API, relationship traversal in single query, cross-object queries, schema introspection, ASN lookup, basic filtering, field selection, pagination.
**Avoids pitfalls:** #7 (N+1 queries -- implement pagination and query complexity limits from the start).
**Stack elements:** entgql v0.7.0, gqlgen, otelhttp middleware.

### Phase 3: REST API + OpenAPI

**Rationale:** REST is the broadest compatibility surface for existing PeeringDB consumers. entrest generates both handlers and OpenAPI spec from the same schemas. Building this after GraphQL means the schema is proven and stable. entrest's WIP status is mitigated by having GraphQL as the primary fallback.
**Delivers:** REST API with JSON responses, auto-generated OpenAPI spec, filter/pagination support, CORS headers, proper HTTP status codes.
**Addresses features:** REST API returning JSON, consistent response format, proper status codes, CORS, numeric/string query modifiers, `since` parameter.
**Avoids pitfalls:** #9 (entrest breaking changes -- pin version, have GraphQL as primary if REST needs manual implementation).
**Stack elements:** entrest v1.0.2, net/http stdlib.

### Phase 4: gRPC API

**Rationale:** gRPC is the highest-performance API surface but uses the least mature extension (entproto). Building it last among API surfaces limits exposure to entproto's M2M edge limitations. The "through entity" pattern from Phase 1 schema design mitigates this, but hand-written proto definitions may be needed for some relationships.
**Delivers:** gRPC server with protobuf definitions, gRPC reflection for service discovery, typed client generation from proto files.
**Addresses features:** gRPC API, generated client SDKs (protobuf-based), high-performance programmatic access.
**Avoids pitfalls:** #6 (M2M edge limitation -- through entities already in schema, hand-write protos for gaps).
**Stack elements:** entproto v0.7.0, google.golang.org/grpc, buf CLI, protoc toolchain.

### Phase 5: Observability + Production Hardening

**Rationale:** OTel instrumentation should be woven into each phase as components are built, but a dedicated hardening phase ensures full coverage: trace correlation across services, metric dashboards, health endpoints, graceful shutdown, and abuse prevention. This must happen before multi-region deployment so issues are visible.
**Delivers:** OpenTelemetry traces/metrics/logs across all API surfaces, health/readiness endpoints with sync freshness checks, graceful shutdown, basic IP-based abuse prevention.
**Addresses features:** OpenTelemetry integration, health/readiness endpoints, query performance metrics, last sync timestamp exposure.
**Avoids pitfalls:** #11 (OpenCensus bridge -- use direct OTel instrumentation on HTTP/gRPC, bridge only for entgo internals).
**Stack elements:** OTel v1.35+, otelslog bridge, otelhttp, slog.

### Phase 6: Edge Deployment (LiteFS + Fly.io)

**Rationale:** Multi-region deployment is the availability and latency differentiator, but it adds operational complexity (FUSE filesystem, Consul leases, primary election, replication monitoring). Deploying after all API surfaces are stable reduces debugging surface area. LiteFS's maintenance-mode status means extra time must be budgeted for troubleshooting.
**Delivers:** Multi-region Fly.io deployment with LiteFS replication, Litestream backup to Tigris, primary/replica detection, Dockerfile, fly.toml, litefs.yml.
**Addresses features:** High availability, low latency from multiple regions, no rate limiting.
**Avoids pitfalls:** #1 (LiteFS unsupported -- pin version, Litestream backup, storage abstraction), #5 (primary election -- static leasing, disable autoscale stop for primary region).
**Stack elements:** LiteFS v0.5.14, Litestream, Fly.io, Consul.

### Phase 7: Web UI

**Rationale:** Explicitly secondary priority per project requirements. Depends on stable APIs and deployed system. Strict scope: read-only browsing of entities with links between related objects. No search, no comparison, no visualization in v1.
**Delivers:** HTMX + Templ browser interface for browsing PeeringDB data, entity detail pages with related data.
**Addresses features:** Browse and search web interface, entity detail pages.
**Avoids pitfalls:** #12 (scope creep -- strict scope definition, hard time-box, dogfood GraphQL API as data source).
**Stack elements:** templ v0.3.x, htmx 2.0.8.

### Phase Ordering Rationale

- **Schema before APIs:** All three API surfaces are code-generated from ent schemas. Schema changes after API generation create cascading rework. The schema must be validated against real PeeringDB data before any API surface is built.
- **GraphQL before REST/gRPC:** entgql is the most mature extension, provides the highest-value differentiator (relationship traversal), and validates the schema's query paths. If the schema works for GraphQL, it will work for the simpler REST and gRPC surfaces.
- **REST before gRPC:** REST has broader consumer adoption and entrest is simpler to deploy (HTTP handlers, no protoc toolchain). gRPC's entproto has known limitations (no M2M edges) that may require workarounds.
- **Observability before deployment:** Issues must be visible before going multi-region. Adding OTel after deployment means debugging in production without instrumentation.
- **Deployment before Web UI:** The web UI is secondary and benefits from being built against a deployed, production-like system. It also lets the team validate the API surfaces under real conditions before building a consumer.

### Research Flags

Phases likely needing deeper research during planning:
- **Phase 1:** NEEDS RESEARCH -- PeeringDB API actual response shapes require analysis of Django serializer source code. Golden test fixtures must be collected from live API. FK violation handling strategy needs validation.
- **Phase 4:** NEEDS RESEARCH -- entproto codegen workflow, protoc/buf plugin chain, M2M edge workarounds. May need hand-written proto definitions.
- **Phase 6:** NEEDS RESEARCH -- LiteFS FUSE configuration on Fly.io, Consul lease setup, Litestream backup configuration, primary election behavior under failure scenarios.

Phases with standard patterns (skip research-phase):
- **Phase 2:** Well-documented. entgql has extensive official documentation and examples.
- **Phase 3:** Straightforward. entrest documentation covers setup. OpenAPI spec generation is automated.
- **Phase 5:** Standard OTel patterns. Official Go SDK documentation is comprehensive.
- **Phase 7:** Standard templ + htmx patterns. Well-documented by both projects.

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | All technologies verified with official sources and pkg.go.dev publish dates. Version compatibility confirmed. |
| Features | HIGH | Feature landscape validated against PeeringDB official documentation, API specs, GitHub issues, and ecosystem tools. Clear table stakes vs. differentiators. |
| Architecture | MEDIUM-HIGH | Single-writer/multi-reader with LiteFS is well-documented. entgo multi-surface generation is less commonly documented in production at this scale. Port allocation and protocol separation is validated by LiteFS proxy docs. |
| Pitfalls | HIGH | Critical pitfalls confirmed via official sources: LiteFS status from Fly.io community forums, PeeringDB spec divergence from GitHub issues, SQLite WAL behavior from SQLite docs, entproto M2M limitation from ent GitHub issues. |

**Overall confidence:** MEDIUM-HIGH

The individual technologies are well-understood and well-documented. The risk lies in their combination: three ent extensions generating simultaneously from one schema, running on a maintenance-mode replication layer, syncing from an API whose spec doesn't match reality. Each risk is manageable with the documented mitigations, but the compound risk warrants careful phase sequencing and validation gates between phases.

### Gaps to Address

- **PeeringDB API actual response shapes:** Must analyze Django serializer source code and collect live API samples before writing entgo schemas. This is the critical unknown for Phase 1.
- **entproto M2M edge workarounds:** Need to validate that "through entity" pattern generates correct proto definitions. May need hand-written protos. Research needed before Phase 4.
- **LiteFS FUSE on Fly.io:** Specific configuration for FUSE mount, Consul lease, and primary election behavior under Fly.io autoscale needs validation. Research needed before Phase 6.
- **modernc.org/sqlite performance under LiteFS FUSE:** No benchmarks exist for this specific combination. Benchmark during Phase 1 to confirm sub-millisecond read performance.
- **entrest concurrent read behavior:** No production reports of entrest under high concurrent read load. Benchmark during Phase 3.
- **OpenCensus bridge compatibility with OTel v1.35+:** entgo uses OpenCensus for tracing. Bridge behavior with latest OTel SDK needs validation. Test during Phase 5.

## Sources

### Primary (HIGH confidence)
- [Go 1.26 Release Notes](https://go.dev/doc/go1.26)
- [entgo.io](https://entgo.io/) -- ORM documentation, entgql, entproto guides
- [ent/ent GitHub Releases](https://github.com/ent/ent/releases) -- v0.14.5
- [LiteFS Architecture](https://github.com/superfly/litefs/blob/main/docs/ARCHITECTURE.md)
- [LiteFS Fly.io Docs](https://fly.io/docs/litefs/)
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/)
- [PeeringDB GitHub Issues #1878, #1658, #637](https://github.com/peeringdb/peeringdb/issues/)
- [SQLite WAL Documentation](https://sqlite.org/wal.html)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)

### Secondary (MEDIUM confidence)
- [LiteFS Status Discussion](https://community.fly.io/t/what-is-the-status-of-litefs/23883) -- Maintenance mode confirmation
- [LiteFS Cloud Sunset](https://community.fly.io/t/sunsetting-litefs-cloud/20829)
- [entproto M2M Issue #2476](https://github.com/ent/ent/issues/2476)
- [ent SQLite modernc Migration Bug #2209](https://github.com/ent/ent/issues/2209)
- [entrest Documentation](https://lrstanley.github.io/entrest/)
- [django-peeringdb Sync Errors](https://github.com/peeringdb/django-peeringdb/issues/31)
- [peeringdb-py FK Constraint Errors](https://github.com/peeringdb/peeringdb-py/issues/46)

### Tertiary (LOW confidence)
- [ConnectRPC Conformance](https://buf.build/blog/grpc-conformance-deep-dive) -- gRPC vs ConnectRPC benchmarks (used for technology decision, not architecture)
- [Templ + HTMX SSR Guide](https://templ.guide/server-side-rendering/htmx/) -- Web UI patterns (deferred to Phase 7)

---
*Research completed: 2026-03-22*
*Ready for roadmap: yes*
