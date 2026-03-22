# Technology Stack: v1.1 Additions

**Project:** PeeringDB Plus v1.1 (REST API & Observability)
**Researched:** 2026-03-22
**Scope:** Stack additions/changes for OTel HTTP client tracing, expanded sync metrics, entrest REST API, and PeeringDB-compatible REST layer. Does NOT re-research validated v1.0 stack.

## New Dependencies

### OTel HTTP Client Tracing

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp | v0.67.0 | HTTP client transport tracing | **Already in go.mod** (used for server-side middleware). The same package provides `otelhttp.NewTransport(base http.RoundTripper)` which wraps outgoing HTTP requests with trace spans, propagates W3C TraceContext headers, and records request duration/size metrics. Zero new dependencies needed. | HIGH |

**Integration point:** The PeeringDB `Client` struct (internal/peeringdb/client.go) currently creates a bare `&http.Client{Timeout: 30 * time.Second}`. Wrapping the transport is a one-line change:

```go
http: &http.Client{
    Timeout:   30 * time.Second,
    Transport: otelhttp.NewTransport(http.DefaultTransport),
},
```

**What it provides:**
- Automatic span creation for each outgoing HTTP request (span kind: Client)
- Default span name: `"HTTP GET"` (customizable via `WithSpanNameFormatter`)
- Semantic convention attributes: `http.request.method`, `http.response.status_code`, `server.address`, `url.full`
- Request/response size metrics
- W3C TraceContext propagation into outgoing request headers
- Span lifecycle extends until response body is closed or reaches EOF

**What it does NOT provide (must add manually):**
- PeeringDB-specific span attributes (object type, page number, retry attempt)
- These should be added as span events or attributes within `doWithRetry` and `FetchAll`

**No new `go get` required.** The package is already a direct dependency at v0.67.0.

### Expanded Sync Metrics

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| go.opentelemetry.io/otel/metric | v1.42.0 | OTel metric instruments | **Already in go.mod.** Used to create counters, histograms, and gauges. The existing `internal/otel/metrics.go` defines `SyncDuration` and `SyncOperations` but the sync worker never calls `.Record()` or `.Add()` on them. | HIGH |

**Current state:** Two metrics are registered but never recorded:
- `pdbplus.sync.duration` (Float64Histogram) -- registered, not recorded
- `pdbplus.sync.operations` (Int64Counter) -- registered, not recorded

**Recommended expansion (no new dependencies):**

| Metric | Type | Attributes | Purpose |
|--------|------|------------|---------|
| `pdbplus.sync.duration` | Float64Histogram | `status=success\|failed` | Total sync cycle time. **Exists, needs recording.** |
| `pdbplus.sync.operations` | Int64Counter | `status=success\|failed` | Sync attempt count. **Exists, needs recording.** |
| `pdbplus.sync.objects` | Int64Counter | `type=org\|net\|fac\|...`, `action=upsert\|delete` | Per-type object counts per sync. **New.** |
| `pdbplus.sync.last_success` | Float64Gauge | (none) | Unix timestamp of last successful sync. **New.** Enables "time since last sync" alerts. |
| `pdbplus.sync.step_duration` | Float64Histogram | `type=org\|net\|fac\|...` | Per-step timing within a sync cycle. **New.** Identifies slow object types. |
| `pdbplus.peeringdb.request_duration` | Float64Histogram | `object_type`, `status_code` | Per-HTTP-request timing to PeeringDB API. **New.** Note: otelhttp transport also records `http.client.request.duration` but without PeeringDB-specific attributes. |
| `pdbplus.peeringdb.retries` | Int64Counter | `object_type`, `status_code` | Retry count per object type. **New.** Surfaces API reliability issues. |

**Instrument types available (all in go.opentelemetry.io/otel/metric, already imported):**
- `Int64Counter` -- monotonically increasing (sync count, object count)
- `Int64UpDownCounter` -- can decrease (not needed here)
- `Float64Histogram` -- distribution of values (durations)
- `Float64Gauge` -- point-in-time value (last sync timestamp). **Note:** `Float64Gauge` was stabilized in OTel Go SDK v1.31.0. Our v1.42.0 includes it.

**No new `go get` required.**

### entrest REST API Generation

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| github.com/lrstanley/entrest | v1.0.2 | OpenAPI spec + HTTP handler generation from ent schemas | Generates a complete OpenAPI 3.1 specification and a fully functional HTTP handler from ent schema definitions. Supports pagination, filtering (AND/OR predicates, field-level filters), eager-loading edges, and sorting. Published 2025-08-21. Requires ent v0.14.5 (matches our version). MIT licensed. | MEDIUM |

**Why MEDIUM confidence:** Documentation warns "expect breaking changes." No official GitHub releases exist (only tagged versions on pkg.go.dev). The library is functional and v1.x semver, but the author's caveat warrants caution. The project has one primary maintainer.

**Integration with existing entc.go:**

The existing `ent/entc.go` uses entgql. entrest adds as a second extension:

```go
import (
    "github.com/lrstanley/entrest"
)

restExt, err := entrest.NewExtension(&entrest.Config{
    Handler:           entrest.HandlerStdlib,
    DefaultOperations: []entrest.Operation{
        entrest.OperationRead,
        entrest.OperationList,
    },
})

opts := []entc.Option{
    entc.Extensions(gqlExt, restExt),
    entc.FeatureNames("sql/upsert"),
}
```

**Key Config options for this project:**

| Config Field | Value | Why |
|-------------|-------|-----|
| `Handler` | `entrest.HandlerStdlib` | Uses Go 1.22+ stdlib ServeMux with path parameters via `http.Request.PathValue`. Matches our existing routing approach. No chi dependency needed. |
| `DefaultOperations` | `[OperationRead, OperationList]` | Read-only mirror. Excludes Create, Update, Delete globally. |
| `ItemsPerPage` | 250 | Match PeeringDB's default page size. |
| `MaxItemsPerPage` | 1000 | Reasonable upper bound for API clients. |
| `MinItemsPerPage` | 1 | Allow single-object fetches. |
| `DefaultEagerLoad` | false | Opt-in via query parameter, not default (matches PeeringDB `depth=0` default). |
| `DisablePatchJSONTag` | true | We use snake_case JSON tags on ent fields already. Do not want entrest to modify them. |
| `StrictMutate` | false | No mutations in read-only mode; irrelevant. |

**Generated output:**
- `ent/rest.go` -- HTTP handler implementation (one handler per entity type for List and Read operations)
- `ent/openapi.json` -- OpenAPI 3.1 specification
- Handler mounts on stdlib mux via generated `NewHandler()` function

**entrest vs. hand-rolled REST:**

entrest handles the mechanical work (pagination, filtering, sorting, eager-loading, OpenAPI spec). The PeeringDB compatibility layer sits in front of (or alongside) the entrest handler, translating PeeringDB's envelope format and query parameter conventions. entrest does NOT produce PeeringDB-compatible output directly -- it produces standard REST conventions. The compat layer is a separate concern.

**New `go get` required:**
```bash
go get github.com/lrstanley/entrest@v1.0.2
```

### PeeringDB-Compatible REST Layer

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| net/http (stdlib) | Go 1.26 | Custom HTTP handlers for PeeringDB-compatible REST | Hand-written handlers that wrap entrest-generated queries but transform the response into PeeringDB's envelope format. No external library needed. | HIGH |

**No new dependencies needed.** This is an application-layer concern, not a library concern.

**What makes PeeringDB REST different from standard REST (what entrest generates):**

| Aspect | PeeringDB Format | entrest Format | Compat Layer Responsibility |
|--------|-----------------|----------------|----------------------------|
| Response envelope | `{"meta": {}, "data": [...]}` | Standard JSON array/object with pagination metadata | Wrap entrest results in `meta`/`data` envelope |
| URL paths | `/api/net`, `/api/org`, `/api/ix` | `/organizations`, `/networks` (pluralized entity names) | Route `/api/{type}` to correct entrest entity handler |
| Single object | `/api/net/42` returns `{"data": [{...}]}` (array with one element) | `/networks/42` returns `{...}` (single object) | Wrap single object in array within `data` key |
| Query params | `?limit=N&skip=N&depth=N&since=N&fields=f1,f2` | `?page=N&per_page=N` (or similar pagination) | Translate PeeringDB params to entrest query format |
| Filter syntax | `?name__contains=foo&asn__gt=100` | entrest's own filter syntax | Parse `__contains`, `__startswith`, `__lt/lte/gt/gte/in` suffixes and translate to ent predicates |
| Field names | snake_case (matches ent schema JSON tags) | snake_case (ent field names) | Likely pass-through; verify during implementation |
| Depth expansion | `?depth=0..4` controls nested set expansion | `?eager_load=edge1,edge2` | Map depth levels to specific edge eager-loads |
| `_set` fields | Related objects as `net_set`, `fac_set` arrays | Edges as named relationships | Rename edge fields to `{type}_set` format |
| `since` parameter | Unix timestamp, returns objects updated after | Not natively supported | Custom ent predicate on `updated` field |

**Architecture decision:** The PeeringDB compat layer should be a thin HTTP handler that:
1. Accepts PeeringDB-style requests (`/api/net?asn=42&limit=10`)
2. Translates to ent queries (using the ent client directly, not going through entrest)
3. Serializes responses in PeeringDB envelope format

Using the ent client directly (rather than wrapping entrest HTTP handlers) is cleaner because the translation from PeeringDB query params to ent predicates is easier at the Go API level than at the HTTP level. The entrest-generated handler serves a separate standard REST API endpoint.

## What NOT to Add

| Technology | Why Not |
|-----------|---------|
| chi/v5 router | Not needed. Stdlib ServeMux handles entrest HandlerStdlib paths. entrest explicitly supports Go 1.22+ stdlib path matching. |
| ConnectRPC | gRPC is deferred to a future milestone. |
| entproto | gRPC is deferred to a future milestone. |
| encoding/xml | PeeringDB API is JSON-only. |
| github.com/gorilla/mux | Dead project (archived). Stdlib ServeMux supersedes it with Go 1.22+. |
| Any JSON serialization library (jsoniter, easyjson) | encoding/json is sufficient for response serialization. PeeringDB responses are already deserialized during sync. REST API responses are ent-generated structs serialized once per request. |
| Any query string parsing library | PeeringDB filter params (`__contains`, `__gt`, etc.) are simple enough to parse with `net/url` and string splitting. No library needed. |

## Dependency Impact Analysis

**New direct dependencies for v1.1:**

| Dependency | New? | Transitive Impact |
|-----------|------|-------------------|
| `github.com/lrstanley/entrest` v1.0.2 | YES | Adds `github.com/ogen-go/ogen` (OpenAPI spec generation), `github.com/stoewer/go-strcase` (case conversion). Both are build-time only (used during `go generate`, not at runtime). |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` v0.67.0 | NO | Already in go.mod. Zero new dependencies. |
| `go.opentelemetry.io/otel/metric` v1.42.0 | NO | Already in go.mod. Zero new dependencies. |

**Total new runtime dependencies: 0** -- entrest generates code at build time; its dependencies are `go generate`-time only and do not appear in the compiled binary.

## Version Compatibility Matrix

| Component | Our Version | entrest Requires | Compatible? |
|-----------|-------------|-----------------|-------------|
| Go | 1.26.1 | >= 1.24.0 | YES |
| entgo.io/ent | v0.14.5 | v0.14.5 | YES (exact match) |
| entgo.io/contrib | v0.7.0 | N/A (no dependency) | N/A |
| net/http ServeMux | Go 1.26 | Go 1.22+ path matching | YES |

## Installation (v1.1 additions only)

```bash
# entrest (the only new dependency)
go get github.com/lrstanley/entrest@v1.0.2

# Everything else is already in go.mod
go mod tidy
```

## Risk Register (v1.1 specific)

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| entrest generated code incompatible with existing entgql code | MEDIUM | LOW | Both extensions operate on the same ent graph but generate independent files. entgql generates `gql_*.go`, entrest generates `rest*.go`. Run `go generate` and compile to verify before any logic work. |
| entrest DefaultOperations does not fully suppress mutation endpoints | MEDIUM | LOW | Verify generated code contains no Create/Update/Delete handlers. If it does, add `WithExcludeOperations` annotations on each schema as backup. |
| PeeringDB filter syntax complexity (`__contains`, `__startswith`, etc.) | MEDIUM | MEDIUM | Implement filters incrementally. Start with exact match and `__in`, add string/numeric filters in a follow-up. Most PeeringDB API consumers use exact match. |
| entrest pagination format differs from PeeringDB `limit/skip` | LOW | HIGH | Expected difference. The PeeringDB compat layer translates `limit`/`skip` to ent `.Limit(n).Offset(n)` calls directly. |
| otelhttp transport adds latency to PeeringDB requests | LOW | LOW | otelhttp transport overhead is <1ms per request. PeeringDB API latency is 100-500ms per request. Negligible. |
| Sync metrics cardinality explosion from per-type attributes | LOW | LOW | 13 object types x 2 actions = 26 unique attribute combinations. Well within safe cardinality bounds. |

## Sources

- [otelhttp Transport source](https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/net/http/otelhttp/transport.go) - NewTransport API and capabilities
- [otelhttp pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.67.0) - v0.67.0 published 2026-03-06
- [otelhttp client example](https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/net/http/otelhttp/example/client/client.go) - Official client transport example
- [OTel Go metric API](https://pkg.go.dev/go.opentelemetry.io/otel/metric) - Instrument types and recording API
- [entrest GitHub](https://github.com/lrstanley/entrest) - Source repository
- [entrest pkg.go.dev](https://pkg.go.dev/github.com/lrstanley/entrest@v1.0.2) - v1.0.2 published 2025-08-21
- [entrest Getting Started](https://lrstanley.github.io/entrest/guides/getting-started/) - Integration guide
- [entrest Annotations](https://lrstanley.github.io/entrest/openapi-specs/annotation-reference/) - Operation control, read-only mode
- [entrest config.go](https://github.com/lrstanley/entrest/blob/master/config.go) - Full Config struct definition
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) - Response envelope, query params, filter syntax
- [PeeringDB Search Guide](https://docs.peeringdb.com/howto/search/) - Filter parameter documentation
