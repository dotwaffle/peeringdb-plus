# Feature Landscape: v1.1 REST API & Observability

**Domain:** PeeringDB data mirror -- REST API surface and observability enhancements
**Researched:** 2026-03-22
**Scope:** Features for v1.1 milestone ONLY (OTel HTTP client tracing, sync metrics, entrest REST API, PeeringDB-compatible REST layer)
**Existing:** entgo ORM with 13 schemas, SQLite + LiteFS, GraphQL API, OTel setup, sync worker, health/readiness endpoints, Fly.io deployment

## Table Stakes

Features the v1.1 milestone must deliver. Without these, the milestone is incomplete.

### OTel HTTP Client Tracing

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|-------------|-------|
| Trace spans on outbound PeeringDB API calls | Known tech debt from v1.0. The PeeringDB HTTP client (`internal/peeringdb/client.go`) makes paginated API calls with retries but produces no trace spans. Without client spans, the "full-sync" parent span has no visibility into where time is spent (which API call? which retry?). Operators cannot diagnose slow syncs. | Low | Existing OTel provider, existing `http.Client` in peeringdb.Client | Use `otelhttp.NewTransport(http.DefaultTransport)` to wrap the client transport. This is a one-line change to `NewClient()` plus import. Creates spans for each outbound HTTP request with method, URL, status code, and duration as semantic attributes. |
| Span attributes for PeeringDB object type and page | Beyond basic HTTP spans, sync debugging needs to know which object type and page number each request corresponds to. Without these attributes, spans show raw URLs but not semantic context. | Low | OTel HTTP client tracing (above) | Add `attribute.String("peeringdb.object_type", objectType)` and `attribute.Int("peeringdb.page", page)` to each request span. Set via context or span events within `FetchAll()`. |
| Parent-child span hierarchy: full-sync -> sync-{type} -> HTTP request | The sync worker already creates `full-sync` and `sync-{type}` spans via `otel.Tracer("sync").Start()`. The HTTP client spans must nest under these as children. This happens automatically when context propagation is correct (the ctx flows from `Sync()` -> `syncOrganizations()` -> `FetchAll()` -> `doWithRetry()` -> `http.NewRequestWithContext(ctx, ...)`). | Low | Correct context propagation (already implemented) | Verify that the existing ctx flow is unbroken. The current code passes ctx correctly through all layers. |
| Retry attempt recorded in spans | The client retries on 429/5xx with backoff. Each retry attempt should be visible in traces so operators can see transient upstream failures. | Low | OTel HTTP client tracing | otelhttp transport creates a span per HTTP request, so retries naturally appear as sibling child spans under the same parent. Add span events for retry decisions. |

### Sync Metrics (Expanded and Wired)

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|-------------|-------|
| `peeringdb.sync.duration` histogram | Known tech debt from v1.0: custom sync metrics were registered but never recorded. Sync duration is the fundamental operational metric -- how long does a full sync take? Track as a histogram to see p50/p95/p99 over time. | Low | OTel MeterProvider (must be initialized alongside TracerProvider in `internal/otel/provider.go`) | Use `meter.Float64Histogram("peeringdb.sync.duration", metric.WithUnit("s"), metric.WithDescription("Duration of full PeeringDB sync"))`. Record `time.Since(start).Seconds()` at sync completion. |
| `peeringdb.sync.objects` counter per type | How many objects were synced in each run, broken down by type. Essential for detecting upstream data anomalies (sudden drop in network count = PeeringDB issue). | Low | OTel Meter | `meter.Int64Counter("peeringdb.sync.objects")` with `attribute.String("peeringdb.type", step.name)`. Record after each step in `Sync()`. |
| `peeringdb.sync.errors` counter | Count of failed syncs, broken down by error type/phase. Alerts should fire when this increments. | Low | OTel Meter | `meter.Int64Counter("peeringdb.sync.errors")` with type attribute. Record in `recordFailure()`. |
| `peeringdb.sync.status` gauge (0=idle, 1=running) | Is the sync currently running? Useful for dashboards and correlation with latency spikes. | Low | OTel Meter | `meter.Int64Gauge("peeringdb.sync.status")`. Record 1 on sync start, 0 on completion/failure. |
| `peeringdb.sync.last_success` gauge (unix timestamp) | When did the last successful sync complete? The most direct indicator of data freshness. Pair with an alert when age exceeds threshold. | Low | OTel Meter | `meter.Float64Gauge("peeringdb.sync.last_success")`. Record `float64(time.Now().Unix())` on success. |
| `peeringdb.sync.deletes` counter per type | How many stale objects were deleted per type per sync. Useful for detecting mass deletions (potential upstream issue). | Low | OTel Meter | Same pattern as objects counter, recorded in each sync step alongside the delete count. |
| MeterProvider initialization | The existing `internal/otel/provider.go` only sets up a TracerProvider with stdout exporter. Must add a MeterProvider with a periodic reader. For development, use stdout metric exporter. For production, autoexport should pick up OTLP. | Med | OTel SDK (`go.opentelemetry.io/otel/sdk/metric`) | Follow the same pattern as the trace provider. Consider using `go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp` for production or `autoexport` for environment-driven selection. Return both shutdown functions. |

### entrest-Generated REST API

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|-------------|-------|
| Read-only REST endpoints for all 13 object types | The entrest extension generates CRUD endpoints from ent schemas. Since this is a read-only mirror, configure entrest with `DefaultOperations: []Operation{OperationRead, OperationList}` to suppress create/update/delete. This produces GET /org, GET /org/{id}, GET /net, GET /net/{id}, etc. for all 13 types. | Med | entrest extension added to `ent/entc.go`, ent codegen re-run | entrest v1.0.2 supports `HandlerStdlib` which generates handlers for Go 1.22+ ServeMux. Must add `entrest.NewExtension()` alongside the existing `entgql.NewExtension()` in entc.go. |
| OpenAPI spec auto-generation | entrest produces an `openapi.json` file alongside the generated handler code. This spec is generated from the actual ent schema, so it accurately reflects the real data model (unlike PeeringDB's buggy spec). Serve at `/openapi.json`. | Low | entrest codegen | Generated automatically. Optionally validate in CI with `go-swagger` or `oapi-codegen validate`. |
| Pagination support | entrest generates paginated list endpoints with `page` and `per_page` query parameters. Configurable min/max/default per page via `MinItemsPerPage`, `MaxItemsPerPage`, `ItemsPerPage` in Config. | Low | entrest codegen | Set `ItemsPerPage: 20`, `MaxItemsPerPage: 250` to match PeeringDB's page sizes. |
| Filtering by field values | entrest generates query parameter filtering when fields are annotated with `entrest.WithFilter()`. Support for equality, inequality, contains, startswith, in, lt, lte, gt, gte operators. | Med | entrest annotations on schema fields | Must add `entrest.WithFilter(entrest.FilterGroupEqual \| entrest.FilterGroupArray)` annotations to each field that should be filterable. This is per-field annotation work across 13 schemas. |
| Sorting support | entrest generates `sort` query parameter when fields are annotated with `entrest.WithSortable(true)`. | Low | entrest annotations on schema fields | Add sortable annotations to key fields (name, created, updated, asn). |
| Edge eager-loading | entrest supports `entrest.WithEagerLoad(true)` on edges, allowing related objects to be included in responses without extra API calls. | Low | entrest edge annotations | Selective eager-loading: set on commonly-traversed edges (organization->networks, network->network_ix_lans), not all edges. |
| REST handler mounting on HTTP server | The generated `rest.NewServer(db, &rest.ServerConfig{})` returns a handler to mount via `srv.Handler()`. Mount under a path prefix (e.g., `/rest/` or `/v1/`). | Low | Generated rest package, HTTP mux setup in main.go | Mount alongside existing GraphQL handler. Use different path prefixes to avoid conflicts. |
| CORS headers on REST endpoints | REST API consumers (browser apps, dashboards) need CORS. Already implemented for GraphQL; extend to REST routes. | Low | Existing CORS middleware (rs/cors) | Wrap REST handler with same CORS middleware used for GraphQL. |

### PeeringDB-Compatible REST Layer

| Feature | Why Expected | Complexity | Dependencies | Notes |
|---------|--------------|------------|-------------|-------|
| PeeringDB URL paths: `/api/{type}` and `/api/{type}/{id}` | PeeringDB's API uses paths like `/api/net`, `/api/net/42`, `/api/org/1`. Every PeeringDB client library, automation script, and Peering Manager integration hard-codes these paths. To serve as a drop-in replacement, paths must match exactly. The 13 types are: org, net, fac, ix, poc, ixlan, ixpfx, netixlan, netfac, ixfac, carrier, carrierfac, campus. | Med | Depends on either entrest (map its paths) or custom handlers | entrest generates its own path scheme (likely `/organizations/{id}`). The PeeringDB compat layer must map PeeringDB paths to ent queries or proxy to entrest. A separate hand-written handler set under `/api/` is more predictable. |
| PeeringDB response envelope: `{"meta": {}, "data": [...]}` | PeeringDB wraps all responses in `{"meta": {"status": ..., "message": ...}, "data": [...]}`. The data field is always an array, even for single-object lookups (which return an array of one). Client libraries parse this envelope. | Med | Custom JSON response serialization | entrest generates its own response format (not PeeringDB's envelope). The compat layer must serialize responses into PeeringDB's envelope format. Write a `writeResponse(w, data)` helper. |
| PeeringDB query params: `limit`, `skip`, `depth`, `fields`, `since` | PeeringDB uses `limit` and `skip` (not `page`/`per_page`), `depth` for nested relationship expansion, `fields` for field selection, and `since` for incremental queries. These are different from entrest's generated query parameters. | High | Custom query parameter parsing, ent queries | `depth=0` is default (flat objects). `depth=1` expands relationship sets to IDs. `depth=2` expands to full objects. This maps to ent's `.WithEdges()` pattern but requires careful implementation per type. Start with `depth=0` (flat) and `depth=2` (eager-loaded edges). |
| PeeringDB field filter params: `?asn=42`, `?name__contains=Equinix` | PeeringDB supports filtering by any field via query parameters: exact match (`?asn=42`), `__contains`, `__startswith`, `__in` (comma-separated), `__lt`, `__lte`, `__gt`, `__gte`. These translate to ent Where predicates. | High | Custom query parameter parsing, ent predicate construction | Must parse query params, identify field names, strip Django-style suffixes (`__contains`), and build ent Where predicates dynamically. Consider a generic "PeeringDB filter to ent predicate" translator function. |
| PeeringDB field names in JSON responses | PeeringDB uses snake_case field names matching Python conventions: `org_id`, `info_type`, `irr_as_set`, `ipaddr4`, `ixf_ixp_member_list_url_visible`. entrest/ent may generate different JSON field names (e.g., camelCase or different naming). The compat layer must output PeeringDB-matching field names. | Med | Custom JSON serialization or field mapping | The ent schema fields already use snake_case names (matching PeeringDB) because the sync client maps them that way. Verify that the serializer preserves these names. If ent/entrest transforms them, add custom JSON tags or a response transformer. |
| Single-object GET returns array of one | PeeringDB `GET /api/net/42` returns `{"data": [{ ...network... }]}` -- an array containing one object, not the object directly. This is non-standard but every PeeringDB client depends on it. | Low | Custom JSON serialization | Always wrap in array. |

## Differentiators

Features that go beyond baseline expectations for these v1.1 additions.

### Observability Excellence

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|-------------|-------|
| Correlated traces + metrics across sync lifecycle | Link a specific sync trace (with HTTP client spans) to the metric data points recorded during that sync. Achievable via exemplars or shared trace/span IDs in metric attributes. No PeeringDB mirror offers this. | Med | Both tracing and metrics implemented | Use `metric.WithAttributes(attribute.String("trace_id", span.SpanContext().TraceID().String()))` on metric recordings within sync. |
| HTTP server request tracing with otelhttp | Wrap the HTTP server handler with `otelhttp.NewHandler()` for automatic incoming request spans. Already listed in STACK.md but not yet implemented. Covers all API surfaces (GraphQL, REST, compat layer). | Low | otelhttp package (already in go.mod) | One-line wrapper: `handler = otelhttp.NewHandler(handler, "peeringdb-plus")`. |

### Dual REST API Surfaces

| Feature | Value Proposition | Complexity | Dependencies | Notes |
|---------|-------------------|------------|-------------|-------|
| Modern REST at `/v1/` (entrest) + compat at `/api/` (PeeringDB paths) | Two REST surfaces serving different audiences. `/v1/` with clean pagination, OpenAPI spec, and modern conventions. `/api/` with PeeringDB-compatible paths and envelope for drop-in replacement. Users can migrate from `/api/` to `/v1/` at their own pace. | Low (architecture) | Both REST surfaces implemented | Mount both on the same HTTP server with different path prefixes. Document the differences. |
| Generated OpenAPI spec at `/openapi.json` | Machine-readable API contract for the modern REST surface. PeeringDB's own OpenAPI spec is broken (GitHub #1878). A correct spec enables client code generation in any language. | Low | entrest codegen | Serve the generated `openapi.json`. Validate in CI. |

## Anti-Features

Features to explicitly NOT build in v1.1.

| Anti-Feature | Why Avoid | What to Do Instead |
|--------------|-----------|-------------------|
| Full `depth` support (depths 1, 3, 4) | PeeringDB supports depth 0-4 for single objects and 0-2 for lists. Depth 1 (IDs only) and depth 3-4 (deep nesting) are rarely used and add significant complexity. | Implement depth=0 (flat, default) and depth=2 (eager-load first-level edges). Log requests for depth 1/3/4 to gauge demand. Return 400 with message for unsupported depth values. |
| Write operations on REST endpoints | This is a read-only mirror. entrest can generate create/update/delete handlers but they must be disabled. | Configure `DefaultOperations: []Operation{OperationRead, OperationList}` globally. |
| PeeringDB authentication emulation | PeeringDB uses Basic auth and API keys. Some response fields (e.g., POC contact details) are visibility-gated by auth. This mirror is fully public. | Serve all data as the public (unauthenticated) PeeringDB view. POC visibility filtering based on PeeringDB's `visible` field rules. |
| Rate limiting on REST endpoints | The mirror's value proposition is removing PeeringDB's rate limits. Adding them back defeats the purpose. | Basic abuse prevention only (consider Fly.io request limits or very generous IP-based throttling if needed later). |
| `_set` field expansion | PeeringDB returns related objects in fields like `net_set`, `fac_set` when depth > 0. These are dynamic nested arrays. Implementing them requires assembling cross-table joins and serializing nested objects with all their fields. | Use edge eager-loading for the entrest surface. For the compat layer, support `depth=0` (flat) and consider `depth=2` as a stretch goal where edges are included as nested arrays matching PeeringDB's `_set` naming. |
| Custom entrest templates | entrest supports custom templates for overriding generated code. Unnecessary complexity when default generation + a separate compat handler is cleaner. | Use default entrest generation. Build compat layer as separate hand-written handlers. |

## Feature Dependencies

```
MeterProvider initialization (internal/otel/provider.go)
  |-> All sync metrics
  |-> HTTP server metrics (if enabled)

OTel HTTP client tracing (peeringdb.Client transport wrapping)
  |-> Span hierarchy verification
  |-> Retry visibility in traces
  (Depends on: existing TracerProvider)

Sync metrics registration and recording (internal/sync/worker.go)
  |-> Metric attribute design (type names, error categories)
  (Depends on: MeterProvider initialization)

entrest extension in entc.go
  |-> entrest code generation (go generate)
  |-> OpenAPI spec generation
  |-> REST handler generation
  (Depends on: existing ent schemas with annotations added)

entrest schema annotations (per-field filter/sort/eager-load)
  |-> Filtering support in generated REST
  |-> Sorting support in generated REST
  |-> Edge eager-loading in generated REST
  (Depends on: entrest extension configured)

REST handler mounting in main.go
  |-> CORS middleware wrapping
  |-> otelhttp wrapping for request tracing
  (Depends on: entrest code generation complete)

PeeringDB compat layer (internal/compat/ or internal/pdbrest/)
  |-> PeeringDB URL path routing (/api/{type})
  |-> Response envelope serialization
  |-> Query parameter parsing
  |-> Field name mapping verification
  (Depends on: existing ent client, NOT on entrest -- queries ent directly)
```

## Milestone Sequencing Recommendation

### Phase 1: Observability Fixes (fix tech debt first)
1. **MeterProvider initialization** -- prerequisite for all metrics
2. **OTel HTTP client tracing** -- one-line transport wrap + attribute additions
3. **Sync metrics registration and recording** -- wire the metrics that were registered but never recorded

Rationale: Smallest scope, highest confidence, fixes known v1.0 debt. Gives immediate operational visibility before adding new features.

### Phase 2: entrest REST API
4. **entrest extension + annotations** -- add to entc.go, annotate schemas
5. **Code generation** -- re-run go generate, verify output
6. **REST handler mounting + CORS** -- wire into main.go
7. **otelhttp server wrapping** -- automatic tracing for all incoming requests

Rationale: entrest does most of the work via code generation. Medium complexity but well-understood pattern (same approach as entgql in v1.0). Must complete before compat layer so we understand what entrest generates.

### Phase 3: PeeringDB Compatibility Layer
8. **PeeringDB path routing** -- `/api/{type}` and `/api/{type}/{id}` handlers
9. **Response envelope serialization** -- `{"meta": {}, "data": [...]}`
10. **Query parameter parsing** -- limit, skip, fields, since
11. **Field filter translation** -- `?asn=42`, `?name__contains=X` to ent predicates
12. **Depth parameter** -- depth=0 (flat) at minimum

Rationale: Highest complexity feature. Must be built separately from entrest (different response format, query params, and paths). Benefits from having entrest working first to understand the ent query patterns. The PeeringDB filter-to-predicate translation is the hardest piece.

**Defer to v1.2+:**
- Depth 1/3/4 support (gauge demand first)
- `_set` field expansion matching PeeringDB's nested format
- `since` parameter in compat layer (requires understanding PeeringDB's incremental sync semantics)

## Sources

- [otelhttp package](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) -- HTTP client/server instrumentation
- [otelhttp client example](https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/net/http/otelhttp/example/client/client.go) -- Transport wrapping pattern
- [OTel Go metric API](https://pkg.go.dev/go.opentelemetry.io/otel/metric) -- Meter, Counter, Histogram, Gauge instrument creation
- [OTel Go getting started](https://opentelemetry.io/docs/languages/go/getting-started/) -- MeterProvider setup, custom metrics examples
- [entrest documentation](https://lrstanley.github.io/entrest/) -- Extension overview and feature list
- [entrest getting started](https://lrstanley.github.io/entrest/guides/getting-started/) -- entc.go configuration, handler mounting
- [entrest annotation reference](https://lrstanley.github.io/entrest/openapi-specs/annotation-reference/) -- Schema/field/edge annotations for filtering, sorting, eager-loading
- [entrest pkg.go.dev](https://pkg.go.dev/github.com/lrstanley/entrest) -- Config struct, Operation constants, ServerConfig
- [entrest GitHub](https://github.com/lrstanley/entrest) -- Source code, examples, issues
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) -- URL structure, query params, depth parameter, response envelope
- [PeeringDB API Docs](https://www.peeringdb.com/apidocs/) -- Interactive API documentation
- [PeeringDB Search HOWTO](https://docs.peeringdb.com/howto/search/) -- Filter parameter syntax (__contains, __startswith, __in, __lt, etc.)
