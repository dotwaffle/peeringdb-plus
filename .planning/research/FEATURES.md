# Feature Landscape

**Domain:** PeeringDB data mirror/proxy with modern API surfaces
**Researched:** 2026-03-22

## Table Stakes

Features users expect from a PeeringDB mirror. Missing any of these means the product is not viable as a replacement data source.

### Data Completeness

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| All basic objects: org, net, fac, ix, poc | These are the core PeeringDB entities. Any mirror missing one is unusable. | Med | 5 object types, each with 10-30 fields |
| All derived objects: ixlan, ixpfx, netixlan, netfac | Derived objects hold the most operationally useful data (which networks are at which IXPs/facilities). | Med | 4 object types linking basic objects |
| Carrier and campus objects | Added in 2024 (PeeringDB 2.43.0). 556 campuses and 8,164 carriers as of late 2024. Any modern mirror must include them. | Low | carrier, carrierfac, campus objects |
| All fields per object with correct types | Operators rely on specific fields: ASN, peering_policy, info_type, ipaddr4/6, speed, irr_as_set, etc. | High | Must match PeeringDB's actual response shapes, not their (buggy) OpenAPI spec |
| Deleted/status-filtered objects | PeeringDB marks objects as deleted rather than removing them. Mirror must handle status correctly. | Low | Objects have a `status` field |

### Query Capabilities

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Filter by any field | PeeringDB supports `?field=value` filtering on all objects. Operators build automation around this. | Med | Must support equality matching at minimum |
| Numeric query modifiers: __lt, __lte, __gt, __gte, __in | Used for range queries (e.g., speed >= 10000). Core filtering pattern. | Med | Maps well to GraphQL/gRPC filter types |
| String query modifiers: __contains, __startswith, __in | Used for name/prefix searching. Essential for "find networks matching X". | Med | Consider full-text search as an upgrade |
| Lookup by ASN | The single most common query pattern. `GET /api/net?asn=42` or equivalent. Must work. | Low | Index on ASN field |
| Lookup by ID | Direct object retrieval by primary key. Every automation tool uses this. | Low | Primary key lookup |
| Pagination (limit/skip) | API consumers paginate through large result sets. | Low | Standard pattern in all three API surfaces |
| Field selection | PeeringDB supports `?fields=name,asn,info_type` to reduce response size. Automation tools depend on this. | Low | GraphQL gets this for free; REST/gRPC need explicit support |

### Data Freshness

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Hourly (or better) sync from upstream PeeringDB | Operators need reasonably current data. Stale data = wrong peering decisions. | Med | Full re-fetch approach per PROJECT.md |
| Expose last sync timestamp | Consumers must know data age to assess reliability. | Low | Metadata endpoint or response header |
| `since` parameter support | PeeringDB's incremental query (`?since=<unix_timestamp>`) lets downstream caches sync efficiently. Even if our sync is full re-fetch, consumers may use `since` to detect changes. | Med | Requires tracking `updated` timestamps per object |

### API Surface Quality

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| REST API returning JSON | Every PeeringDB client library, automation script, and tool speaks REST+JSON. Must be compatible. | Med | entrest generates this from entgo schema |
| Consistent response format | PeeringDB wraps responses in `{"meta": {...}, "data": [...]}`. Consumers parse this shape. Decide: match it or use a cleaner format. | Low | Recommend cleaner format since we're not a drop-in replacement |
| Proper HTTP status codes and error messages | Automation tools branch on status codes. 200, 400, 404, 429 at minimum. | Low | Standard HTTP semantics |
| CORS headers | Browser-based tools and dashboards query PeeringDB data. Without CORS, web integrations break. | Low | Middleware concern |
| HTTPS only | PeeringDB deprecated TLS < 1.2 in April 2025. Secure-by-default is expected. | Low | Fly.io handles TLS termination |

### Reliability

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| High availability | PeeringDB itself suffers from single-region hosting. A mirror that is also unreliable defeats the purpose. | Med | Fly.io multi-region with LiteFS handles this |
| No rate limiting (or very generous limits) | PeeringDB's 40 req/min limit is the #1 pain point driving people to mirrors. Removing this is table stakes for a mirror. | Low | Read-only SQLite can handle very high query rates |
| Low latency from multiple regions | PeeringDB is single-region (AWS). Global operators need fast access. Edge deployment is expected from a mirror. | Med | Core architecture decision (Fly.io + LiteFS) |

## Differentiators

Features that set PeeringDB Plus apart. Not expected from a basic mirror, but create compelling reasons to switch.

### Modern API Surfaces

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| GraphQL API | PeeringDB has no GraphQL. GraphQL enables clients to request exactly the fields they need, traverse relationships in a single query (e.g., "give me all networks at IX 42 with their facilities"), and introspect the schema. Eliminates the N+1 query problem that plagues PeeringDB's REST API (e.g., netixlan returns only net_id, forcing extra lookups). | High | entgql generates from entgo schema. This is the flagship differentiator. |
| gRPC API | No PeeringDB gRPC exists. Enables strongly-typed, high-performance programmatic access. Useful for automation pipelines, service meshes, and Go/Python/Rust clients. Protobuf schemas serve as machine-readable contracts. | High | entproto generates from entgo schema |
| OpenAPI-compliant REST | PeeringDB's own OpenAPI spec has known bugs (issue #1878: duplicate parameters, invalid requestBody schemas, code generation fails). A correct, validated OpenAPI spec is itself a differentiator. | Med | entrest generates from entgo schema; validate spec in CI |

### Query Power

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Relationship traversal in single query | PeeringDB requires multiple API calls to walk relationships (net -> netixlan -> ix). GraphQL resolves this naturally. Even REST can support `?include=` or `?expand=` patterns. | Med | GraphQL handles this natively; REST needs explicit design |
| Full-text search across objects | PeeringDB's search is basic field matching. Full-text search across names, descriptions, and notes would enable "find anything mentioning 'Equinix'" type queries. | Med | SQLite FTS5 is excellent for this |
| Cross-object queries | "Find all networks present at both IX-A and IX-B" or "Find all facilities in Germany with networks that have open peering policy." PeeringDB cannot do this without multiple round trips. | High | GraphQL with proper relationship modeling makes this natural |
| ASN comparison | PeeringDB added basic ASN comparison in mid-2025. A richer version with facility/IX overlap analysis would differentiate. | Med | Query pattern over netixlan and netfac joins |

### Data Presentation

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Schema introspection | GraphQL introspection and gRPC reflection let clients discover the API shape programmatically. PeeringDB's self-describing API docs (apidocs) are rendered HTML, not machine-queryable. | Low | Built into GraphQL and gRPC by default |
| Accurate, validated API documentation | PeeringDB's OpenAPI spec does not match actual responses (GitHub issue #1658, #1878). Generating docs from the actual entgo schema guarantees accuracy. | Low | Entrest + OpenAPI spec validation in CI |
| Structured data exports (JSON, CSV) | PeeringDB added this recently but it's limited to search results. Offering bulk exports per object type is valuable for researchers (CAIDA mirrors PeeringDB daily for this reason). | Low | Endpoint that streams full object lists |
| Geographic queries | PeeringDB supports radius search from the web UI but not the API. Offering geo-queries (facilities within X km of a point) via API would serve network planning use cases. | High | Requires geospatial indexing (lat/long fields exist on fac objects) |

### Operations and Observability

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| OpenTelemetry traces/metrics/logs | No PeeringDB mirror offers observability. Users can see query performance, error rates, and data freshness. Operators can debug integration issues. | Med | Already required per PROJECT.md |
| Health/readiness endpoints | Standard Kubernetes/Fly.io health checks. Lets consumers programmatically verify the mirror is healthy and data is fresh. | Low | `/healthz`, `/readyz` with sync age checks |
| Query performance metrics | Expose p50/p95/p99 latency per endpoint. Demonstrates the performance advantage over upstream PeeringDB. | Low | OpenTelemetry histograms |

### Developer Experience

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Interactive GraphQL playground | GraphiQL or similar embedded explorer. Lets operators discover and test queries without writing code. | Low | Standard middleware for GraphQL servers |
| Generated, type-safe client SDKs | From the OpenAPI spec and protobuf definitions, generate Go, Python, TypeScript clients. PeeringDB's only official client is Python (peeringdb-py). | Med | OpenAPI and protobuf code generation tooling |
| Webhook/callback on data changes | Notify subscribers when specific objects change. Eliminates polling. | High | Requires change tracking infrastructure; defer to later phase |

### Web UI

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Browse and search PeeringDB data in a web interface | PeeringDB's web UI is functional but dated. A modern, fast UI would attract casual users and demonstrate the product. | High | HTMX + Templ per PROJECT.md; secondary priority |
| Network/facility/IX detail pages | Display rich object pages with all related data (networks at a facility, IXPs a network peers at, etc.) | Med | Template rendering from entgo queries |
| Visual network comparison | Side-by-side comparison of two networks showing shared IXPs and facilities. PeeringDB added basic comparison in 2025; a better version differentiates. | Med | Query pattern + presentation |

## Anti-Features

Features to explicitly NOT build. These would increase complexity without proportional value, or would conflict with the project's read-only mirror architecture.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Write/mutation API | PeeringDB is the authoritative source. Accepting writes creates data conflicts, requires auth, and contradicts the mirror model. | Read-only mirror. Link to PeeringDB for edits. |
| User accounts and authentication | Adds enormous complexity (registration, password management, permissions, abuse prevention). The value prop is fast public access. | Fully public, no auth. Consider optional API keys later only if abuse requires it. |
| OAuth / social login | Complex, unnecessary for read-only public data. | N/A |
| Real-time streaming / WebSockets | Hourly sync granularity is sufficient for peering decisions. Real-time streaming adds complexity (connection management, backpressure, client SDK support) with marginal value. | Polling via `since` parameter. Consider webhooks in future. |
| Drop-in PeeringDB API compatibility | Matching PeeringDB's exact response format (including its bugs and inconsistencies) would constrain our API design and inherit their technical debt. | Clean, well-documented API. Provide migration guide for users moving from PeeringDB API. |
| Mobile app | Network operators work from laptops/desktops. PeeringDB's own stats show only 20% mobile visits, mostly for quick lookups. Web UI handles mobile adequately. | Responsive web UI with HTMX. |
| Historical data / time-series | Storing every version of every object over time is a different product (CAIDA already does this). Massively increases storage and query complexity. | Serve current state only. Link to CAIDA for historical data. |
| Data quality validation / correction | Flagging bad data in PeeringDB (wrong speeds, stale contacts, etc.) is PeeringDB's job. A mirror should faithfully reproduce the data. | Mirror data as-is. Do not editorialize. |
| Email notifications | Requires email infrastructure, user accounts, preference management. Overkill for a data mirror. | Expose change data via API; let consumers build their own notifications. |
| Rate limiting matching PeeringDB's restrictions | The whole point is to be faster and more accessible. Don't artificially limit. | Basic abuse prevention only (e.g., IP-based throttle at extreme levels to prevent DoS). |

## Feature Dependencies

```
entgo schema definition
  |-> entgql (GraphQL API)
  |-> entproto (gRPC API)
  |-> entrest (REST API + OpenAPI spec)
  |-> HTMX web UI (queries via entgo)

Data sync from PeeringDB
  |-> All API surfaces (need data to serve)
  |-> `since` parameter support (need updated timestamps)
  |-> Last sync timestamp exposure
  |-> Health/readiness endpoints (check sync freshness)

SQLite + LiteFS deployment
  |-> Multi-region low latency (edge reads)
  |-> High availability
  |-> Full-text search (SQLite FTS5)

GraphQL API
  |-> Interactive GraphQL playground
  |-> Relationship traversal queries
  |-> Cross-object queries
  |-> Schema introspection

gRPC API
  |-> Generated client SDKs (protobuf)
  |-> gRPC reflection

REST API (entrest)
  |-> OpenAPI spec (auto-generated)
  |-> Generated client SDKs (OpenAPI)
  |-> REST filter/pagination support

OpenTelemetry integration
  |-> Health endpoints
  |-> Query performance metrics
  |-> Distributed tracing
```

## MVP Recommendation

### Phase 1: Data Foundation
Prioritize getting all PeeringDB data synced correctly and queryable:

1. **All PeeringDB objects** (org, net, fac, ix, poc, ixlan, ixpfx, netixlan, netfac, carrier, carrierfac, campus) -- without complete data, nothing else matters
2. **GraphQL API with relationship traversal** -- the primary differentiator; enables single-query access to related data
3. **Correct field types and data fidelity** -- must match PeeringDB's actual responses (not their buggy spec)
4. **ASN lookup** -- the single most common query pattern
5. **Basic filtering** (equality, numeric operators) -- minimum viable query surface

### Phase 2: API Surfaces and Operations
6. **REST API with OpenAPI spec** -- broadest compatibility with existing tooling
7. **gRPC API** -- high-performance programmatic access
8. **OpenTelemetry integration** -- observability from day one
9. **Health/readiness endpoints** -- operational hygiene
10. **Last sync timestamp** -- data freshness transparency

### Phase 3: Query Power and DX
11. **String query modifiers** (__contains, __startswith) -- common search patterns
12. **`since` parameter** -- enables downstream incremental sync
13. **Interactive GraphQL playground** -- zero-friction API exploration
14. **Full-text search** (FTS5) -- cross-object search capability
15. **CORS support** -- enables browser-based integrations

**Defer:**
- **Web UI**: Secondary priority per PROJECT.md. Build after APIs are solid.
- **Geographic queries**: High complexity, niche use case. Add when demand is clear.
- **Webhooks**: Requires change tracking infrastructure. Revisit after core is stable.
- **Generated client SDKs**: Valuable but can be community-driven from the published specs.
- **ASN comparison feature**: Nice-to-have, not blocking adoption.

## Sources

- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) -- Official API documentation
- [PeeringDB API Docs (interactive)](https://www.peeringdb.com/apidocs/) -- Self-describing API reference
- [PeeringDB Tools](https://docs.peeringdb.com/tools/) -- Ecosystem of tools consuming PeeringDB data
- [PeeringDB Faster Queries](https://docs.peeringdb.com/blog/faster_queries/) -- Official guidance on rate limits and local caching
- [PeeringDB Query Limits HOWTO](https://docs.peeringdb.com/howto/work_within_peeringdbs_query_limits/) -- Rate limiting details (40/min authenticated)
- [PeeringDB Search HOWTO](https://docs.peeringdb.com/howto/search/) -- Search capabilities documentation
- [GitHub Issue #1658: netixlan missing keys](https://github.com/peeringdb/peeringdb/issues/1658) -- API response vs docs mismatch
- [GitHub Issue #1878: OpenAPI schema errors](https://github.com/peeringdb/peeringdb/issues/1878) -- OpenAPI spec validation failures
- [Carrier Objects Deployed](https://docs.peeringdb.com/blog/carrier_object_deployed/) -- New carrier/campus data model (2024)
- [September 2025 Product Update](https://docs.peeringdb.com/blog/sep_2025_product_update/) -- ASN comparison, web redesign, MFA mandate
- [April 2025 Product Update](https://docs.peeringdb.com/blog/april_2025_product_update/) -- Dark mode, KMZ export, API key mandate, TLS 1.2+
- [gmazoyer/peeringdb Go library](https://github.com/gmazoyer/peeringdb) -- Go client library with carrier/campus support (v0.1.0, Feb 2026)
- [CAIDA PeeringDB Dataset](https://catalog.caida.org/dataset/peeringdb) -- Historical PeeringDB data archive
- [Peering Manager](https://peering-manager.readthedocs.io/) -- BGP session management tool consuming PeeringDB
- [PeerCtl](https://www.fullctl.com/peerctl) -- BGP config generation from PeeringDB data
