# Architecture Patterns

**Domain:** v1.1 REST API & Observability integration into existing PeeringDB Plus
**Researched:** 2026-03-22
**Focus:** OTel HTTP client tracing, expanded sync metrics, entrest REST API, PeeringDB-compatible REST layer

## Existing Architecture (Context)

The v1.0 architecture is a single Go binary with the following structure:

```
cmd/peeringdb-plus/main.go (wiring, HTTP server, graceful shutdown)
  |
  +-- internal/config/         Config from env vars (immutable after load)
  +-- internal/database/       SQLite open (modernc.org, WAL, FK)
  +-- internal/otel/           OTel pipeline: TracerProvider, MeterProvider, LoggerProvider
  |     provider.go            autoexport-driven setup
  |     metrics.go             SyncDuration (histogram) + SyncOperations (counter) -- REGISTERED BUT NOT RECORDED
  |     logger.go              Dual slog handler (stdout + OTel)
  +-- internal/peeringdb/      PeeringDB API client (HTTP, pagination, retry, rate limit)
  +-- internal/sync/           Sync worker (fetch -> filter -> upsert -> delete, single tx)
  +-- internal/health/         /healthz (liveness), /readyz (readiness + sync freshness)
  +-- internal/middleware/      Logging, Recovery, CORS
  +-- internal/litefs/         LiteFS primary detection
  +-- internal/graphql/        GraphQL handler factory (gqlgen server config)
  +-- graph/                   gqlgen resolvers, generated code
  +-- ent/                     entgo ORM (13 schemas), generated code
  +-- ent/schema/              Schema definitions with entgql annotations
```

**HTTP middleware stack** (outermost to innermost):
```
Recovery -> otelhttp.NewMiddleware -> Logging -> CORS -> readinessMiddleware -> mux
```

**Routes:**
```
GET  /           Root discovery (JSON)
GET  /healthz    Liveness probe
GET  /readyz     Readiness probe
POST /sync       On-demand sync trigger (auth via X-Sync-Token)
GET  /graphql    GraphiQL playground
POST /graphql    GraphQL queries
```

**Code generation pipeline:**
```
ent/entc.go (entgql extension) -> go generate ./ent -> ent/ generated code
                                                    -> graph/schema.graphqls
                                                    -> gqlgen generates graph/*.go
```

## Recommended Architecture for v1.1

Four new capabilities integrate into the existing architecture with minimal disruption. Each touches specific files and layers.

### 1. OTel HTTP Client Tracing

**What changes:** The PeeringDB HTTP client (`internal/peeringdb/client.go`) gains automatic trace span creation for every outbound HTTP request.

**Integration point:** Wrap the `http.Client.Transport` with `otelhttp.NewTransport` during client construction.

**Component boundary:** This is a change to `internal/peeringdb/client.go` only. No new packages needed.

```
Before:
  Client.http = &http.Client{Timeout: 30s}

After:
  Client.http = &http.Client{
      Timeout:   30s,
      Transport: otelhttp.NewTransport(http.DefaultTransport),
  }
```

**Span details produced automatically by otelhttp.Transport:**
- Span name: `HTTP GET` (default format: `HTTP {method}`)
- Span kind: `SpanKindClient`
- Attributes: `http.method`, `http.url`, `http.status_code`, `http.request.content_length`, `http.response.content_length`
- W3C TraceContext propagation injected into outgoing headers
- Metrics: request duration, request size

**What this enables:**
- Every PeeringDB API call appears as a child span under the `full-sync` span
- Trace waterfall shows: `full-sync` -> `sync-org` -> `HTTP GET /api/org?limit=250&skip=0`
- Network latency, retries, and rate limiting become visible in traces
- PeeringDB upstream issues become diagnosable from OTel backend

**Why otelhttp.NewTransport and not manual spans:**
- The project already depends on `otelhttp v0.67.0` (used for server middleware)
- Transport wrapping is the canonical pattern (2,691 importers per pkg.go.dev)
- Automatic semantic convention compliance (no manual attribute tagging)
- Automatic context propagation (W3C TraceContext injected into request headers)
- Handles response body close lifecycle correctly

**Confidence:** HIGH (otelhttp is already a project dependency, pattern is well-established)

**New/modified files:**

| File | Change Type | What |
|------|-------------|------|
| `internal/peeringdb/client.go` | MODIFY | Wrap transport with `otelhttp.NewTransport` in `NewClient` |
| `internal/peeringdb/client_test.go` | MODIFY | Verify span creation in tests (use `sdktrace/tracetest`) |

### 2. Expanded Sync Metrics

**What changes:** The existing `SyncDuration` and `SyncOperations` metrics (registered in `internal/otel/metrics.go` but never recorded) get wired into the sync worker, and new per-type metrics are added.

**Current state problem:**
- `otel.InitMetrics()` registers `pdbplus.sync.duration` and `pdbplus.sync.operations`
- `sync.Worker.Sync()` never calls `.Record()` or `.Add()` on these instruments
- This is listed as known tech debt in PROJECT.md

**Integration approach:** The sync worker should record metrics at two levels:

1. **Full-sync level** (existing instruments, just wire them):
   - `pdbplus.sync.duration` -- record total sync wall-clock time after commit/failure
   - `pdbplus.sync.operations` -- increment with `status=success` or `status=failed`

2. **Per-type level** (new instruments):
   - `pdbplus.sync.type.duration` -- histogram per object type
   - `pdbplus.sync.type.objects` -- gauge of objects synced per type
   - `pdbplus.sync.type.deleted` -- counter of objects deleted per type

**Metric attribute conventions (OTel semantic conventions):**
- `pdbplus.sync.type` -- object type name (`org`, `net`, `fac`, etc.)
- `pdbplus.sync.status` -- `success` or `failed`

**Component boundary:** Changes span `internal/otel/metrics.go` (instrument registration) and `internal/sync/worker.go` (recording calls).

**Dependency flow:**
```
internal/otel/metrics.go  -- defines and exports metric instruments
     |
     v
internal/sync/worker.go   -- imports otel package, calls .Record() / .Add()
```

**Why pass instruments via package-level vars (current pattern) vs. injection:**
The existing pattern uses package-level vars (`otel.SyncDuration`, `otel.SyncOperations`). This is acceptable for a single-binary application where `InitMetrics()` runs once at startup before any goroutines use the instruments. Dependency injection would be cleaner but would change the `Worker` constructor signature, which is out of scope for a metrics fix.

**Confidence:** HIGH (straightforward instrument wiring, no external research needed)

**New/modified files:**

| File | Change Type | What |
|------|-------------|------|
| `internal/otel/metrics.go` | MODIFY | Add per-type metric instruments, review bucket boundaries |
| `internal/sync/worker.go` | MODIFY | Add `.Record()` and `.Add()` calls at sync completion points |
| `internal/sync/worker_test.go` | MODIFY | Verify metric recording (use `sdkmetric/metricdata` reader) |
| `internal/otel/metrics_test.go` | MODIFY | Test new instrument registration |

### 3. entrest-Generated OpenAPI REST API

**What changes:** The ent code generation pipeline gains entrest as a second extension (alongside entgql), producing REST HTTP handlers and an OpenAPI specification.

**Integration with code generation pipeline:**

```
Before (entc.go):
  Extensions: [entgql]
  Output: ent/ generated code, graph/schema.graphqls

After (entc.go):
  Extensions: [entgql, entrest]
  Output: ent/ generated code, graph/schema.graphqls, ent/rest/ generated REST handlers
```

**entrest configuration for this project:**

```go
restExt, err := entrest.NewExtension(&entrest.Config{
    Handler: entrest.HandlerStdlib,   // Use net/http, not chi -- matches existing router
    DefaultOperations: []entrest.Operation{
        entrest.OperationRead,
        entrest.OperationList,
    },                                 // Read-only mirror: no create/update/delete
    StrictMutate:      false,          // Irrelevant for read-only
    ItemsPerPage:      20,             // Sensible default page size
    MaxItemsPerPage:   250,            // Match PeeringDB's max
    DefaultEagerLoad:  false,          // Don't eager-load by default (depth=0 behavior)
})
```

**Key configuration decisions:**

| Decision | Value | Rationale |
|----------|-------|-----------|
| Handler type | `HandlerStdlib` | Project uses net/http mux, not chi. Stdlib handler uses Go 1.22+ path matching. |
| Operations | Read + List only | This is a read-only mirror. No mutations exposed via REST. |
| Items per page | 20 (default), 250 (max) | PeeringDB API uses limit=250 max. Match that ceiling. |
| Eager loading | Off by default | PeeringDB's depth=0 returns no nested objects. Match that. |

**Schema annotations needed (per-schema):**

Each ent schema needs entrest annotations alongside existing entgql ones:

```go
// Annotations of the Network.
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.RelayConnection(),
        entgql.QueryField(),
        // entrest annotations:
        entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
    }
}
```

**Field-level annotations for filtering and sorting:**

Fields that should support REST API filtering need `entrest.WithFilter()`:

```go
field.String("name").
    Annotations(
        entgql.OrderField("NAME"),
        entrest.WithSortable(true),
        entrest.WithFilter(entrest.FilterGroupContains | entrest.FilterEQ),
    ),
field.Int("asn").
    Annotations(
        entrest.WithSortable(true),
        entrest.WithFilter(entrest.FilterEQ | entrest.FilterIn),
    ),
field.String("country").
    Annotations(
        entrest.WithFilter(entrest.FilterEQ | entrest.FilterIn),
    ),
```

JSON fields (social_media, info_types, available_voltage_services) need `entrest.WithSchema()` to provide OpenAPI schema definitions since entrest cannot infer them from Go types:

```go
field.JSON("social_media", []SocialMedia{}).
    Annotations(
        entrest.WithSchema(&ogen.Schema{
            Type: "array",
            Items: &ogen.SchemaOrRef{Schema: &ogen.Schema{
                Type: "object",
                Properties: []ogen.Property{
                    {Name: "service", Schema: &ogen.SchemaOrRef{Schema: &ogen.Schema{Type: "string"}}},
                    {Name: "identifier", Schema: &ogen.SchemaOrRef{Schema: &ogen.Schema{Type: "string"}}},
                },
            }},
        }),
    ),
```

**Handler mounting in main.go:**

```go
// Create REST server from ent client.
restSrv, err := rest.NewServer(entClient, &rest.ServerConfig{})
// Mount under /rest/ prefix
mux.Handle("/rest/", http.StripPrefix("/rest", restSrv.Handler()))
// OpenAPI spec auto-served at /rest/openapi.json by entrest
```

**Generated REST endpoints (entrest default URL pattern):**

entrest generates URLs based on ent type names (pluralized, kebab-case):

| entrest Path | HTTP Methods | Ent Type |
|-------------|--------------|----------|
| `/networks` | GET (list) | Network |
| `/networks/{id}` | GET (read) | Network |
| `/organizations` | GET (list) | Organization |
| `/organizations/{id}` | GET (read) | Organization |
| `/facilities` | GET (list) | Facility |
| `/facilities/{id}` | GET (read) | Facility |
| etc. | | |

These paths differ from PeeringDB's paths (which use `/api/net`, `/api/org`, `/api/fac`). The PeeringDB compat layer (section 4) resolves this.

**Confidence:** MEDIUM (entrest v1.0.2 is relatively new, "expect breaking changes" warning in docs, but functional)

**New/modified files:**

| File | Change Type | What |
|------|-------------|------|
| `ent/entc.go` | MODIFY | Add entrest extension alongside entgql |
| `ent/schema/*.go` (13 files) | MODIFY | Add entrest annotations (operations, filters, sorts, JSON schemas) |
| `ent/rest/` | NEW (generated) | Generated REST handlers, server, OpenAPI spec |
| `cmd/peeringdb-plus/main.go` | MODIFY | Create rest.NewServer, mount handler on mux |
| `go.mod` | MODIFY | Add `github.com/lrstanley/entrest` dependency |

### 4. PeeringDB-Compatible REST Layer

**What changes:** A hand-written compatibility layer maps PeeringDB's exact REST API paths, query parameter format, and response envelope to the underlying ent queries.

**Why not use entrest alone:**

entrest generates a modern RESTful API. PeeringDB's API has specific conventions that entrest cannot reproduce:

| Aspect | PeeringDB API | entrest API |
|--------|---------------|-------------|
| Path format | `/api/net`, `/api/fac/{id}` | `/networks`, `/facilities/{id}` |
| Response envelope | `{"meta": {}, "data": [...]}` | Direct JSON array/object |
| Field names | `snake_case` from Python Django | `snake_case` (ent fields match, but JSON keys may differ) |
| Query modifiers | `?name__contains=X`, `?asn__in=1,2` | `?name.contains=X` or similar |
| Depth parameter | `?depth=0` through `?depth=4` | Eager-load edge endpoints |
| Pagination | `?limit=N&skip=N` | Cursor-based or page-based |

**Architecture decision: Two separate REST APIs, not one.**

The PeeringDB compat layer is a separate handler tree (`internal/pdbcompat/`) that directly queries the ent client and produces PeeringDB-format responses. It does NOT proxy to entrest. Reasons:

1. **Response envelope:** PeeringDB wraps everything in `{"meta": {...}, "data": [...]}`. Intercepting and rewrapping entrest responses is fragile.
2. **Query parameter translation:** PeeringDB uses Django-style `__contains`, `__startswith` modifiers. Translating these to entrest's query format is more work than querying ent directly.
3. **Field name mapping:** ent field names are already snake_case matching PeeringDB (designed this way in v1.0). Direct ent -> JSON serialization preserves field names.
4. **Depth semantics:** PeeringDB's depth parameter controls edge expansion in specific ways that don't map to entrest's eager-load semantics.

**Component design:**

```
internal/pdbcompat/
    handler.go     -- Router: maps /api/{type} and /api/{type}/{id} to handlers
    response.go    -- Response envelope serialization: {"meta": {...}, "data": [...]}
    filter.go      -- Query parameter parser: __contains, __startswith, __lt, etc.
    depth.go       -- Depth parameter handling: edge expansion logic
    types.go       -- Per-type handler registry: maps "net" -> Network queries
```

**Handler mounting in main.go:**

```go
// Create PeeringDB-compatible REST handler.
pdbHandler := pdbcompat.NewHandler(entClient)
mux.Handle("/api/", pdbHandler)
```

**URL routing within pdbcompat:**

```
/api/{type}        -> List handler  (e.g., /api/net, /api/fac)
/api/{type}/{id}   -> Read handler  (e.g., /api/net/42, /api/fac/1)
```

The handler uses a registry map to dispatch by type:

```go
var typeRegistry = map[string]TypeHandler{
    "org":        orgHandler{},
    "net":        netHandler{},
    "fac":        facHandler{},
    "ix":         ixHandler{},
    "ixlan":      ixlanHandler{},
    "ixpfx":      ixpfxHandler{},
    "netixlan":   netixlanHandler{},
    "netfac":     netfacHandler{},
    "ixfac":      ixfacHandler{},
    "carrier":    carrierHandler{},
    "carrierfac": carrierfacHandler{},
    "campus":     campusHandler{},
    "poc":        pocHandler{},
}
```

**Response envelope format (matching PeeringDB exactly):**

```go
type Envelope struct {
    Meta Meta            `json:"meta"`
    Data json.RawMessage `json:"data"`
}

type Meta struct {
    // PeeringDB returns empty meta on success; status/message on error.
    // We mirror this behavior.
}
```

The `data` field is always a JSON array, even for single-object responses (PeeringDB wraps single objects in an array too).

**Query parameter translation:**

PeeringDB filter modifiers map to ent predicates:

| PeeringDB Modifier | ent Predicate | Example |
|---------------------|---------------|---------|
| `name__contains=X` | `network.NameContains("X")` | Substring match |
| `name__startswith=X` | `network.NameHasPrefix("X")` | Prefix match |
| `asn__in=1,2,3` | `network.AsnIn(1, 2, 3)` | Set membership |
| `asn__lt=100` | `network.AsnLT(100)` | Less than |
| `asn__gte=100` | `network.AsnGTE(100)` | Greater or equal |
| `country=US` | `network.Country("US")` | Exact match |
| `limit=N` | `.Limit(N)` | Row limit |
| `skip=N` | `.Offset(N)` | Row offset |
| `since=T` | `.Where(network.UpdatedGT(time.Unix(T, 0)))` | Updated since |
| `depth=N` | Edge eager-loading logic | Expand relationships |

**Depth parameter semantics (must match PeeringDB behavior):**

For list endpoints (`/api/net`):
- `depth=0` (default): No edge expansion, return flat objects
- `depth=1`: `_set` fields contain arrays of IDs (e.g., `net_set: [1, 2, 3]`)
- `depth=2`: `_set` fields contain full objects

For single-object endpoints (`/api/net/42`):
- `depth=0`: No edge expansion
- `depth=1-4`: Both sets and FK relationships are expanded

**Depth implementation approach:** The depth parameter controls which ent `.With*()` eager-loading calls are added to queries. A depth map per type defines which edges to load at which depth level.

**Confidence:** HIGH for the architectural pattern (hand-written compat layer is the right approach). MEDIUM for implementation details (PeeringDB's undocumented quirks may require iteration).

**New/modified files:**

| File | Change Type | What |
|------|-------------|------|
| `internal/pdbcompat/handler.go` | NEW | Router, type registry, dispatch |
| `internal/pdbcompat/response.go` | NEW | Envelope serialization |
| `internal/pdbcompat/filter.go` | NEW | Query parameter parsing and ent predicate building |
| `internal/pdbcompat/depth.go` | NEW | Depth parameter handling |
| `internal/pdbcompat/types.go` | NEW | Per-type handler implementations |
| `internal/pdbcompat/*_test.go` | NEW | Tests for each component |
| `cmd/peeringdb-plus/main.go` | MODIFY | Mount compat handler on `/api/` |

## Data Flow

### Current Data Flow (v1.0)

```
PeeringDB API --[HTTP/JSON]--> peeringdb.Client --[Go structs]--> sync.Worker
    --> ent.Tx (upsert/delete) --> SQLite (via modernc.org)
    --> LiteFS replicates to edge nodes

User --[HTTP POST]--> /graphql --> gqlgen --> ent.Client --> SQLite --> JSON response
```

### New Data Flow (v1.1 additions)

```
PeeringDB API --[HTTP/JSON w/ OTel spans]--> peeringdb.Client --[metrics recorded]--> sync.Worker
    --> ent.Tx (upsert/delete) --> SQLite
    --> Sync metrics recorded to OTel MeterProvider

User --[HTTP GET]--> /rest/networks --> entrest handler --> ent.Client --> SQLite --> JSON response
User --[HTTP GET]--> /api/net --> pdbcompat handler --> ent.Client --> SQLite --> PDB envelope response
```

### Request Flow Through Middleware

All new REST routes go through the same middleware stack:

```
Recovery -> otelhttp.NewMiddleware -> Logging -> CORS -> readinessMiddleware -> mux
                                                                                |
                                                          +--------------------+--------------------+
                                                          |                    |                    |
                                                     /graphql            /rest/*               /api/*
                                                     (gqlgen)           (entrest)           (pdbcompat)
                                                          |                    |                    |
                                                     ent.Client          ent.Client          ent.Client
                                                          |                    |                    |
                                                        SQLite             SQLite              SQLite
```

The `readinessMiddleware` already gates all non-infrastructure paths with 503 until first sync completes. Both `/rest/*` and `/api/*` are non-infrastructure paths, so they automatically get this gating.

**Infrastructure paths to exempt:** The existing exemption list (`/sync`, `/healthz`, `/readyz`, `/`) does NOT need to change. The new REST endpoints should be gated by readiness -- you should not serve data that has not been synced yet.

## Patterns to Follow

### Pattern 1: Transport Wrapping for Client Tracing

**What:** Wrap `http.DefaultTransport` with `otelhttp.NewTransport` to automatically create spans for outbound HTTP requests.

**When:** Any HTTP client that makes outbound requests where tracing visibility is needed.

**Example:**

```go
func NewClient(baseURL string, logger *slog.Logger) *Client {
    return &Client{
        http: &http.Client{
            Timeout:   30 * time.Second,
            Transport: otelhttp.NewTransport(http.DefaultTransport),
        },
        // ... rest unchanged
    }
}
```

**Why this pattern:** otelhttp.NewTransport is the standard approach (HIGH confidence per OpenTelemetry Go docs). It automatically handles span lifecycle, context propagation, and semantic convention attributes. The alternative (manual spans in `doWithRetry`) would duplicate what otelhttp does and miss low-level HTTP connection attributes.

### Pattern 2: Metric Recording at Operation Boundaries

**What:** Record OTel metrics at the natural start/end boundaries of operations, using attributes to distinguish success/failure and type.

**When:** Any measured operation (sync cycles, API requests, etc.).

**Example:**

```go
// After sync completes (success or failure):
pdbotel.SyncDuration.Record(ctx, elapsed.Seconds(),
    metric.WithAttributes(
        attribute.String("pdbplus.sync.status", "success"),
    ),
)
pdbotel.SyncOperations.Add(ctx, 1,
    metric.WithAttributes(
        attribute.String("pdbplus.sync.status", "success"),
    ),
)
```

**Why this pattern:** OTel metrics should be recorded at the point where the measured event concludes, not before (to get accurate duration/status). The worker.Sync() method already has clean success and failure paths that are ideal recording points.

### Pattern 3: Dual API Surfaces from Single Schema

**What:** Use ent as the single source of truth for data access, generating multiple API surfaces (GraphQL, REST) from the same schema definitions.

**When:** The project needs to expose the same data through different protocols or response formats.

**Example of the ent schema acting as schema-of-record:**

```go
func (Network) Annotations() []schema.Annotation {
    return []schema.Annotation{
        // GraphQL: Relay connection + query field
        entgql.RelayConnection(),
        entgql.QueryField(),
        // REST: Read + List operations only
        entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
    }
}
```

Both entgql-generated GraphQL and entrest-generated REST hit the same `ent.Client` which queries the same SQLite database.

### Pattern 4: Compatibility Layer as Adapter

**What:** Build a thin adapter layer that translates between an external API contract (PeeringDB REST) and internal data access (ent queries).

**When:** You need to reproduce an existing API's exact behavior without contaminating your internal data model.

**Key principles:**
- The adapter does NOT import or proxy through entrest. It queries ent directly.
- Field names in ent schemas already match PeeringDB (designed this way in v1.0).
- The adapter only handles: URL routing, query parameter parsing, response envelope wrapping, depth expansion.
- No business logic in the adapter -- it is purely a translation layer.

## Anti-Patterns to Avoid

### Anti-Pattern 1: Proxying Through entrest for PDB Compat

**What:** Building the PeeringDB compat layer as a proxy/middleware on top of entrest responses.

**Why bad:** entrest produces its own response format (pagination cursors, JSON structure). Intercepting, parsing, and rewrapping these responses is fragile, slower (double serialization), and couples two independent API surfaces. When entrest changes its response format (it warns about breaking changes), the compat layer breaks too.

**Instead:** The compat layer queries ent.Client directly and produces PeeringDB-format responses. Two independent code paths from the same data source.

### Anti-Pattern 2: Manual Spans Where Transport Wrapping Suffices

**What:** Adding `otel.Tracer("peeringdb").Start(ctx, "http-get")` inside `doWithRetry` when otelhttp.NewTransport handles it.

**Why bad:** Duplicates span creation (two spans per request), misses low-level HTTP attributes (connection reuse, DNS, TLS), and does not correctly handle response body lifecycle. The otelhttp Transport is specifically designed for this.

**Instead:** Wrap the transport once at client construction. The existing `ctx` in `http.NewRequestWithContext` propagates correctly to the transport's span.

### Anti-Pattern 3: Recording Metrics Inside Tight Loops

**What:** Recording per-object metrics inside the upsert loop (e.g., one metric call per organization).

**Why bad:** For 80K+ objects, this generates excessive metric cardinality and overhead. OTel metric recording has nontrivial overhead per call.

**Instead:** Aggregate at the sync-step level. Record once per type per sync cycle: total count, total duration, deleted count.

### Anti-Pattern 4: Mixing entrest Annotations with PDB Compat Logic

**What:** Configuring entrest to try to match PeeringDB's URL patterns, response format, or query parameters.

**Why bad:** entrest has its own opinionated URL structure and response format. Fighting the framework to match PeeringDB's conventions defeats the purpose of using a code generator. You end up with fragile overrides that break on entrest upgrades.

**Instead:** Let entrest be entrest (modern REST API at `/rest/`). Let pdbcompat be PeeringDB-compatible (at `/api/`). Two clean, independent surfaces.

## Integration Summary

### Components Changed vs. New

| Component | Status | Scope |
|-----------|--------|-------|
| `internal/peeringdb/client.go` | MODIFIED | Add otelhttp.NewTransport to HTTP client |
| `internal/otel/metrics.go` | MODIFIED | Add per-type sync metric instruments |
| `internal/sync/worker.go` | MODIFIED | Wire metric recording calls |
| `ent/entc.go` | MODIFIED | Add entrest extension to code generation |
| `ent/schema/*.go` (13 files) | MODIFIED | Add entrest annotations |
| `cmd/peeringdb-plus/main.go` | MODIFIED | Mount REST and compat handlers, update root discovery |
| `ent/rest/` (generated) | NEW | entrest-generated REST handlers |
| `internal/pdbcompat/` | NEW | PeeringDB compatibility layer |

### Dependency Changes

| Dependency | Type | Purpose |
|------------|------|---------|
| `github.com/lrstanley/entrest` | NEW (direct) | REST code generation extension |
| `github.com/ogen-go/ogen` | NEW (indirect via entrest) | OpenAPI spec types for JSON field schemas |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | EXISTING | Already imported; now also used for client transport |

## Suggested Build Order

Based on dependency analysis, the four features should be built in this order:

### Phase 1: OTel HTTP Client Tracing + Sync Metrics (no dependencies between them)

These two can be built in parallel or sequence -- they do not depend on each other.

**OTel HTTP Client Tracing:**
1. Modify `NewClient` to wrap transport
2. Add test verifying span creation
3. Verify trace waterfall in dev (run with `OTEL_TRACES_EXPORTER=console`)

**Sync Metrics:**
1. Add new metric instruments to `internal/otel/metrics.go`
2. Add recording calls to `internal/sync/worker.go`
3. Add tests verifying metric values
4. Verify metrics in dev (run with `OTEL_METRICS_EXPORTER=console`)

**Rationale for first:** Smallest changes, fix known tech debt, provide observability for everything that follows. No schema changes, no code generation, no new packages.

### Phase 2: entrest REST API

**Depends on:** Nothing from Phase 1, but logically comes after because Phase 1 fixes tech debt first.

1. Add entrest dependency to go.mod
2. Modify `ent/entc.go` to add entrest extension
3. Add entrest annotations to all 13 ent schemas
4. Run code generation
5. Mount REST handler in main.go
6. Test generated endpoints
7. Verify OpenAPI spec at `/rest/openapi.json`

**Rationale for second:** Schema changes and code generation are a prerequisite for Phase 3 since the code must compile. The entrest API is also independently useful as a modern REST surface.

### Phase 3: PeeringDB-Compatible REST Layer

**Depends on:** Phases 1 and 2 complete (project must build with new schema annotations). Does NOT depend on entrest handlers at runtime -- queries ent directly.

1. Create `internal/pdbcompat/` package
2. Build response envelope (`response.go`)
3. Build query parameter parser (`filter.go`)
4. Build depth handling (`depth.go`)
5. Build per-type handler registry (`types.go`, `handler.go`)
6. Mount on `/api/` in main.go
7. Integration tests against real PeeringDB responses for format validation

**Rationale for last:** Most complex feature, highest risk of PeeringDB API quirks. Benefits from observability (Phase 1) being in place to debug issues. Needs compiled project (Phase 2).

## Scalability Considerations

| Concern | Current (v1.0) | With v1.1 additions |
|---------|----------------|---------------------|
| Trace volume | Server spans only (~100/min at low traffic) | +13 types x ~4 pages = ~52 client spans per sync cycle. Negligible. |
| Metric cardinality | 2 instruments, 0 recorded | ~8 instruments, ~15 attribute combinations. Well within OTel recommendations (<2000 time series). |
| REST endpoint latency | N/A | SQLite read queries; p99 should be <10ms for single-object, <50ms for filtered lists. Same as GraphQL. |
| Memory overhead | Baseline Go + ent + SQLite | entrest adds generated handler code (compiled, not runtime). pdbcompat is lightweight. <1MB additional RSS. |
| PDB compat depth queries | N/A | depth=2+ triggers eager-loading which can be expensive for types with many edges. Cap at depth=2 for list, depth=4 for single (match PeeringDB). |

## Sources

- [otelhttp package docs](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) - Transport wrapping pattern, v0.67.0 (published 2026-03-06)
- [otelhttp transport.go source](https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/net/http/otelhttp/transport.go) - Implementation details for span attributes and context propagation
- [entrest pkg.go.dev](https://pkg.go.dev/github.com/lrstanley/entrest) - Config struct, annotation functions, v1.0.2 (published 2025-08-21)
- [entrest GitHub](https://github.com/lrstanley/entrest) - Getting started guide, example code
- [entrest getting started](https://lrstanley.github.io/entrest/guides/getting-started/) - Extension setup, handler mounting
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) - URL patterns, response envelope, query parameters, depth behavior
- [OpenTelemetry Go otelhttp instrumentation guide](https://oneuptime.com/blog/post/2026-02-06-instrument-go-net-http-otelhttp-opentelemetry/view) - Client transport tracing patterns (2026)
