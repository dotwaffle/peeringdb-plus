# Phase 2: GraphQL API - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Expose all PeeringDB data through a GraphQL API with filtering, pagination, relationship traversal, and an interactive playground. This phase builds on Phase 1's entgo schemas and data. No REST, gRPC, or web UI — those are later phases.

</domain>

<decisions>
## Implementation Decisions

### Query Design
- **D-01:** Support both Relay cursor-based pagination AND offset/limit pagination
- **D-02:** GraphQL field names use camelCase (e.g., `infoPrefixes4`, `irrAsSet`) — idiomatic GraphQL, entgql default
- **D-03:** Use entgql's generated WhereInput types for filtering — automatic, type-safe, supports all field operators
- **D-04:** Query complexity and depth limits enabled to prevent abuse
- **D-05:** Dedicated `networkByAsn(asn: Int!)` top-level query AND filter support via `networks(where: {asn: 42})`
- **D-06:** Use Relay-style opaque global IDs (base64 type:id) for Node interface compliance
- **D-07:** No GraphQL subscriptions — read-only mirror with hourly sync, subscriptions add complexity with little value
- **D-08:** Query-only schema — no mutations (writes happen via sync only)
- **D-09:** All types implement Relay-compliant Node interface — enables `node(id:)` query and Relay client compatibility
- **D-10:** No cross-type search query in v1 — per-type queries only (FTS5 search is v2)
- **D-11:** Expose `syncStatus` query returning lastSyncAt, duration, objectCounts from the sync_status table
- **D-12:** totalCount field in connections is optional — only returned when explicitly requested
- **D-13:** Use DataLoader pattern for relationship traversal batching (not entgql eager loading)
- **D-14:** Maximum page size of 1000 items per request
- **D-15:** Export GraphQL schema as `.graphql` SDL file in the repo for consumers
- **D-16:** Detailed query errors with field paths, validation details, and helpful messages

### Playground & Developer Experience
- **D-17:** GraphiQL as the embedded playground IDE
- **D-18:** Playground served at same path as API (`/graphql`) — GET serves playground, POST handles queries
- **D-19:** Ship with pre-built example queries: ASN lookup, IX network listing, facility details, relationship traversal
- **D-20:** GraphQL introspection always enabled — public data, no reason to hide schema
- **D-21:** Playground always enabled — no disable option, it's part of the public DX

### Server Setup
- **D-22:** 99designs/gqlgen as the GraphQL library — entgql integrates natively
- **D-23:** stdlib net/http for HTTP server — Go 1.22+ routing, no external router dependency
- **D-24:** Port configurable via PDBPLUS_PORT env var, default 8080
- **D-25:** Graceful shutdown on SIGTERM with configurable drain timeout — important for Fly.io rolling deploys
- **D-26:** CORS origins configurable via PDBPLUS_CORS_ORIGINS env var, default to `*`
- **D-27:** Structured request logging middleware via slog (method, path, status, duration) — feeds into OTel in Phase 3
- **D-28:** Root endpoint (GET /) returns JSON with version, links to /graphql, sync status — helpful for API discovery

### Claude's Discretion
- GraphQL complexity/depth limit values
- Exact DataLoader implementation approach
- GraphiQL configuration and example query content
- Graceful shutdown drain timeout default
- Root endpoint JSON shape

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 1 Foundation
- `.planning/phases/01-data-foundation/01-CONTEXT.md` — All schema and sync decisions that Phase 2 builds on
- `.planning/phases/01-data-foundation/01-REFERENCE-SQLITE-ENTGO.md` — SQLite + entgo integration snippets

### Research Artifacts
- `.planning/research/STACK.md` — Technology recommendations (gqlgen, entgql versions)
- `.planning/research/ARCHITECTURE.md` — System architecture and API surface component design
- `.planning/research/FEATURES.md` — Feature landscape including GraphQL differentiators

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- entgo schemas from Phase 1 with entgql annotations already included (D-42 from Phase 1)
- `internal/peeringdb` client package from Phase 1
- sync_status metadata table from Phase 1 (for syncStatus query)

### Established Patterns
- slog structured logging (Phase 1)
- OTel span creation pattern (Phase 1 basic spans)
- Environment variable configuration (Phase 1)

### Integration Points
- gqlgen resolver code connects to entgo client for queries
- DataLoader wraps entgo queries for batched relationship loading
- GraphQL handler mounts on the HTTP server alongside /sync endpoint from Phase 1
- CORS middleware wraps the HTTP handler

</code_context>

<specifics>
## Specific Ideas

- GraphiQL with pre-built example queries makes the API immediately explorable — show off relationship traversal (e.g., "all networks at DE-CIX Frankfurt with their facilities")
- Relay cursor pagination + offset/limit gives both modern GraphQL clients and simple scripts what they need
- syncStatus query lets consumers check data freshness without hitting a separate health endpoint

</specifics>

<deferred>
## Deferred Ideas

- None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-graphql-api*
*Context gathered: 2026-03-22*
