# Phase 5: entrest REST API - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Code-generated read-only REST API for all 13 PeeringDB types using entrest. Includes OpenAPI spec generation, filtering, sorting, eager-loading, and integration tests. Mounted alongside existing GraphQL API.

</domain>

<decisions>
## Implementation Decisions

### URL Structure
- **D-01:** REST API mounted at `/rest/v1/` prefix — gives both namespace separation from GraphQL (/graphql) and PeeringDB compat (/api/), plus versioning for future API changes
- **D-02:** Use entrest default resource naming from ent schema names (e.g. Network → /networks, InternetExchange → /internet-exchanges) — no custom overrides
- **D-03:** Do NOT update root discovery endpoint (GET /) to advertise REST API — REST is self-discoverable via OpenAPI spec

### Read-Only Enforcement
- **D-04:** Config-level only — set DefaultOperations to Read+List in entrest extension config. No write handlers generated at all. No additional middleware defense.

### Schema Annotations
- **D-05:** Minimal schema-level annotations only — add entrest annotations in each schema's Annotations() method. Let entrest auto-discover filterable/sortable fields from ent field types. No per-field annotation granularity.
- **D-06:** All edges eager-loadable by default — maximum flexibility for API consumers
- **D-07:** Use entrest default pagination — no custom page size limits

### Response Format
- **D-08:** JSON fields (social_media, info_types) included as-is in REST responses — raw JSON arrays/objects
- **D-09:** JSON only — no content negotiation. Always return application/json
- **D-10:** Nullable fields always present in responses (serialized as null), not omitted — predictable schema for consumers
- **D-11:** OpenAPI spec includes field descriptions pulled from ent field Comment() annotations
- **D-12:** Error responses follow RFC 7807 Problem Details format

### Server Integration
- **D-13:** REST handler mounted on the shared mux alongside /graphql, /healthz etc.
- **D-14:** REST endpoints gated by readiness middleware — returns 503 until first sync completes, consistent with GraphQL behavior
- **D-15:** Separate CORS middleware instance for REST (same config as GraphQL, but independently configurable for future changes)

### Testing
- **D-16:** Explicit codegen coexistence test — verify entrest + entgql both produce valid output when running go generate
- **D-17:** Full integration tests — test list, read by ID, pagination, filtering, sorting, eager-loading, and error cases for REST endpoints

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### entrest
- Research: `.planning/research/STACK.md` — entrest v1.0.2 integration details, config API, compatibility notes
- Research: `.planning/research/ARCHITECTURE.md` — entrest + entgql coexistence architecture

### Existing Codegen
- `ent/entc.go` — Current entgql extension config, will need entrest extension added alongside
- `ent/schema/network.go` — Example schema with entgql annotations, Comment() fields — pattern for adding entrest annotations

### Server Wiring
- `cmd/peeringdb-plus/main.go` — HTTP server setup, middleware stack, mux registration, readiness middleware

### Prior Phase Context
- `.planning/phases/04-observability-foundations/04-CONTEXT.md` — OTel decisions that affect REST (otelhttp middleware already in stack)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- Middleware stack (Recovery → OTel → Logging → CORS → Readiness) — REST handler plugs into existing mux behind this stack
- `middleware.CORS()` — already implemented, can create second instance with same config
- `readinessMiddleware()` — already gates /graphql, just needs /rest/ paths excluded from bypass list

### Established Patterns
- entgql extension in entc.go — entrest follows identical pattern (extension option to entc.Generate)
- Schema annotations via Annotations() method — each schema already has entgql.RelayConnection(), entgql.QueryField()
- HTTP handler mounting via mux.HandleFunc/Handle — consistent with existing patterns

### Integration Points
- `ent/entc.go` — Add entrest extension alongside entgql
- `ent/schema/*.go` — Add entrest annotations to all 13 schemas' Annotations() methods
- `cmd/peeringdb-plus/main.go` — Mount generated REST handler, add /rest/ to readiness bypass
- `go.mod` — Add entrest dependency

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 05-entrest-rest-api*
*Context gathered: 2026-03-22*
