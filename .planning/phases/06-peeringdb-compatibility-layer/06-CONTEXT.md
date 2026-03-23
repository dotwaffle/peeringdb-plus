# Phase 6: PeeringDB Compatibility Layer - Context

**Gathered:** 2026-03-22
**Status:** Ready for planning

<domain>
## Phase Boundary

Drop-in PeeringDB API replacement. Existing PeeringDB consumers can point at /api/ and get identical response behavior — same paths, same envelope, same query filters, same field names, same depth expansion. Queries ent directly, NOT wrapping entrest.

</domain>

<decisions>
## Implementation Decisions

### URL Path Mapping
- **D-01:** Exact PeeringDB paths: /api/net, /api/ix, /api/fac, /api/org, /api/poc, /api/ixlan, /api/ixpfx, /api/netixlan, /api/netfac, /api/ixfac, /api/carrier, /api/carrierfac, /api/campus
- **D-02:** Accept both with and without trailing slash (/api/net and /api/net/) — more forgiving, matches PeeringDB
- **D-03:** Single object by ID: /api/net/42 — exact PeeringDB path format

### Response Format
- **D-04:** Exact PeeringDB field names in snake_case — audit ent JSON tags against PeeringDB's actual response and use custom serializers where they differ
- **D-05:** Always return results sorted by ID — matches PeeringDB's default behavior, no sort parameter

### Depth Behavior
- **D-06:** depth=0 (default): flat objects with FK IDs only (e.g. org_id: 42)
- **D-07:** depth=2: replaces FK ID with full nested object AND includes reverse _set fields (e.g. net_set, fac_set on org) — exact PeeringDB behavior
- **D-08:** Use ent's With* eager-loading to fetch related objects for depth=2, then serialize the full graph

### Query Filters
- **D-09:** Core filter set: __contains, __startswith, __in, __lt, __gt, __lte, __gte, exact match (no suffix) — covers most common use cases
- **D-10:** Case-insensitive matching — use SQLite COLLATE NOCASE or LIKE to match PeeringDB's behavior
- **D-11:** Generic filter parser — one parser handles any field + operator + value, validates field exists, maps operator to ent predicate. Reusable across all 13 types.
- **D-12:** Support ?status= filter including status=deleted — matches PeeringDB behavior (though deleted objects may not be in our DB depending on sync config)

### Search & Projection
- **D-13:** Implement ?q= full-text-like search across name fields (and type-specific fields like ASN, irr_as_set)
- **D-14:** Implement ?fields= for field projection — ?fields=id,name,asn returns only those fields
- **D-15:** ?since= accepts Unix timestamps only — matches PeeringDB exactly

### Pagination
- **D-16:** ?limit= and ?skip= query parameters for pagination

### Claude's Discretion
- **D-17:** API index at /api/ — Claude decides whether to include and what format
- **D-18:** Meta field in envelope — Claude decides whether to include useful pagination info or keep empty like PeeringDB
- **D-19:** _set fields per type — Claude determines which _set fields PeeringDB returns per type and implements those
- **D-20:** Unknown filter fields — Claude decides whether to return 400 or silently ignore
- **D-21:** Page size limits — Claude decides max limit based on PeeringDB behavior and SQLite performance
- **D-22:** Single object response format — Claude checks PeeringDB's actual behavior (likely {data: [obj]} array)
- **D-23:** ?q= search fields per type — Claude matches PeeringDB's actual behavior

### Error Handling
- **D-24:** Error responses match PeeringDB's exact error format per status code
- **D-25:** X-Powered-By: PeeringDB-Plus/1.1 header on all compat responses — transparent but non-intrusive branding
- **D-26:** Match PeeringDB's HTTP response headers where applicable

### Package Structure
- **D-27:** New `internal/pdbcompat/` package — handler, filter parser, serializers, tests all in one package

### Server Integration
- **D-28:** Separate CORS middleware instance for /api/ — consistent with Phase 5 REST (same config, independently configurable)
- **D-29:** Readiness-gated like GraphQL and REST — 503 until first sync completes

### Testing
- **D-30:** Both fixture-based integration tests (using Phase 1 PeeringDB fixtures) AND golden file tests (compare against real PeeringDB API responses)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### PeeringDB API
- `internal/peeringdb/types.go` — Response[T] envelope type, type constants (TypeOrg, TypeNet, etc.), all 13 Go struct definitions with JSON tags
- `internal/peeringdb/client.go` — FetchAll with pagination pattern (limit/skip), URL format (/api/{type})
- PeeringDB API docs: https://www.peeringdb.com/apidocs/ — reference for filter behavior, depth semantics, response format

### Existing Test Fixtures
- `internal/sync/testdata/` — 13 PeeringDB fixture files from Phase 1 integration tests — reuse for compat layer testing

### Ent Schemas
- `ent/schema/*.go` — All 13 schemas with fields, edges, indexes — compat layer queries these

### Prior Phase Decisions
- `.planning/phases/04-observability-foundations/04-CONTEXT.md` — OTel tracing available for compat layer debugging
- `.planning/phases/05-entrest-rest-api/05-CONTEXT.md` — entrest at /rest/v1/, compat layer at /api/ — separate APIs, separate CORS
- `.planning/research/FEATURES.md` — PeeringDB compat feature analysis
- `.planning/research/PITFALLS.md` — Django filter parsing pitfalls, depth behavior edge cases

### Server Wiring
- `cmd/peeringdb-plus/main.go` — Mux registration, readiness middleware bypass list, CORS setup

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `peeringdb.Response[T]` struct — same envelope format we need to produce in responses
- `peeringdb.Type*` constants — maps directly to URL path names
- Phase 1 fixture files — pre-built test data for all 13 types
- `middleware.CORS()` — reusable for compat layer CORS instance
- Readiness middleware pattern — extend bypass list for /api/ endpoints

### Established Patterns
- Ent client query pattern: `client.Network.Query().Where(network.NameContains("cloud")).All(ctx)`
- Eager-loading: `client.Network.Query().WithOrganization().WithPocs().All(ctx)`
- HTTP handler mounting: `mux.HandleFunc("GET /api/net", handler)` pattern from main.go

### Integration Points
- `cmd/peeringdb-plus/main.go` — Mount compat handlers, add /api/ to readiness bypass, create CORS instance
- `ent.Client` — Injected into pdbcompat handler for database queries
- `internal/peeringdb/types.go` — Reference for field name mapping and response envelope

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches for areas marked as Claude's Discretion.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 06-peeringdb-compatibility-layer*
*Context gathered: 2026-03-22*
