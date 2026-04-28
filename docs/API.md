<!-- generated-by: gsd-doc-writer -->
# API Reference

PeeringDB Plus exposes the mirrored PeeringDB dataset through **five coexisting API
surfaces** served from the same process on the same port, plus a small set of
infrastructure endpoints for health, on-demand sync, and service discovery. This
document is the comprehensive reference; see `README.md` for a one-page overview
and `docs/ARCHITECTURE.md` for the rationale behind each surface.

All routes are registered in `cmd/peeringdb-plus/main.go` and pass through the
production middleware chain:

```
Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging -> PrivacyTier ->
Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip -> RouteTag -> mux
```

The server speaks HTTP/1.1 and h2c (HTTP/2 cleartext) on the same listener so
that Connect, gRPC, and gRPC-Web clients can use the same base URL as browser
and CLI clients.

## Authentication

Most endpoints are **unauthenticated and read-only** — they expose the
same public data that PeeringDB itself publishes.

| Endpoint | Authentication |
|----------|----------------|
| All `GET` endpoints (Web UI, GraphQL, REST, `/api/`, ConnectRPC) | None |
| `POST /sync` | `X-Sync-Token` header must match `PDBPLUS_SYNC_TOKEN` (constant-time compare) |
| Upstream fetch from `api.peeringdb.com` | Optional — set `PDBPLUS_PEERINGDB_API_KEY` to use an authenticated client with higher rate limits |

If `PDBPLUS_SYNC_TOKEN` is empty at startup the sync endpoint logs a warning and
rejects every request as `401 unauthorized` — there is no "accept anything"
mode. Replica instances reject the request with a `fly-replay` header that
routes the request to the primary region on Fly.io, or return `503 not primary`
when running outside Fly.io.

## Endpoints overview

| Method | Path | Surface | Description |
|--------|------|---------|-------------|
| `GET` | `/` | Root | Content-negotiated service discovery (terminal / browser / JSON) |
| `GET` | `/healthz` | Health | Liveness probe (always `200`) |
| `GET` | `/readyz` | Health | Readiness probe (checks DB and sync freshness) |
| `POST` | `/sync` | Admin | On-demand sync trigger (primary only, token-gated) |
| `GET` | `/favicon.ico` | Static | Favicon served from embedded `internal/web/static/` |
| `GET` | `/static/*` | Static | Embedded UI assets (CSS, JS, images) |
| `GET` | `/ui/` | Web UI | Home / search page |
| `GET` | `/ui/asn/{asn}` | Web UI | Network detail by ASN |
| `GET` | `/ui/ix/{id}` | Web UI | Internet exchange detail |
| `GET` | `/ui/fac/{id}` | Web UI | Facility detail |
| `GET` | `/ui/org/{id}` | Web UI | Organization detail |
| `GET` | `/ui/campus/{id}` | Web UI | Campus detail |
| `GET` | `/ui/carrier/{id}` | Web UI | Carrier detail |
| `GET` | `/ui/search` | Web UI | Search results (supports `?q=`) |
| `GET` | `/ui/about` | Web UI | Build info and sync freshness |
| `GET` | `/ui/compare` | Web UI | ASN comparison form |
| `GET` | `/ui/compare/{asn1}/{asn2}` | Web UI | ASN comparison results |
| `GET` | `/ui/completions/bash` | Web UI | Bash completion script |
| `GET` | `/ui/completions/zsh` | Web UI | Zsh completion script |
| `GET` | `/ui/completions/search` | Web UI | Name suggestions for shell completion |
| `GET` | `/graphql` | GraphQL | GraphiQL playground (HTML) |
| `POST` | `/graphql` | GraphQL | Query execution |
| `GET` | `/rest/v1/openapi.json` | REST | OpenAPI 3 specification |
| `GET` | `/rest/v1/{collection}` | REST | List resources (entrest-generated) |
| `GET` | `/rest/v1/{collection}/{id}` | REST | Get single resource (entrest-generated) |
| `GET` | `/api/` | PDB Compat | Index of available object types |
| `GET` | `/api/{type}` | PDB Compat | List endpoint with PeeringDB-compatible filters |
| `GET` | `/api/{type}/{id}` | PDB Compat | Single object (wrapped in `data: []`) |
| `POST` | `/peeringdb.v1.{Service}/{Method}` | ConnectRPC | One of 13 services × {Get, List, Stream} methods |
| `POST` | `/grpc.reflection.v1.ServerReflection/*` | ConnectRPC | gRPC reflection (v1) |
| `POST` | `/grpc.reflection.v1alpha.ServerReflection/*` | ConnectRPC | gRPC reflection (v1alpha) |
| `POST` | `/grpc.health.v1.Health/*` | ConnectRPC | gRPC health check |

The 13 entity types mirrored from PeeringDB are: `campus`, `carrier`,
`carrierfac`, `fac`, `ix`, `ixfac`, `ixlan`, `ixpfx`, `net`, `netfac`,
`netixlan`, `org`, `poc`.

There is no `/metrics` endpoint. Prometheus / Grafana metrics are exported
via OTLP to the configured collector (see `docs/CONFIGURATION.md`'s
`OTEL_*` variables); no Prometheus scrape endpoint is exposed by the
process. <!-- VERIFY: OTLP collector endpoint configured for the peeringdb-plus.fly.dev deployment -->

## 1. Web UI (`/ui/`)

The Web UI is implemented in `internal/web/` using [templ](https://templ.guide)
for type-safe HTML templates and [htmx](https://htmx.org) for interactive
behavior without a JavaScript build pipeline. The same URL space can render
HTML, ANSI-colored terminal text, plain text, or JSON depending on client
characteristics.

### Content negotiation

The server inspects each request through `internal/web/termrender.Detect`
(`internal/web/termrender/detect.go`) and picks a render mode based on:

1. `?T` or `?format=plain|json|whois|short` query parameter (highest priority)
2. `Accept` header (`text/plain` → rich terminal, `application/json` → JSON)
3. `User-Agent` prefix — `curl/`, `Wget/`, `HTTPie/`, `xh/`, `PowerShell/`,
   `fetch` are treated as terminal clients and receive ANSI-colored output
4. `HX-Request` header — htmx fragments are returned without the page shell
5. Default: HTML

`?nocolor` suppresses all ANSI escape codes regardless of mode.

### The curl gotcha

Because `curl` and `wget` are detected by User-Agent, running
`curl https://peeringdb-plus.fly.dev/ui/asn/15169` returns **ANSI-colored text
intended for a terminal**. If you pipe this output to a file, a logger, or a
tool that does not render ANSI codes, you will see escape sequences like
`\x1b[38;5;...`.

Three ways to get clean output:

```bash
# Option 1: request plain ASCII
curl "https://peeringdb-plus.fly.dev/ui/asn/15169?format=plain"

# Option 2: disable color codes only
curl "https://peeringdb-plus.fly.dev/ui/asn/15169?nocolor"

# Option 3: pretend to be a browser and get HTML
curl -H "User-Agent: Mozilla/5.0" https://peeringdb-plus.fly.dev/ui/asn/15169

# Option 4: strip ANSI codes after the fact
curl https://peeringdb-plus.fly.dev/ui/asn/15169 | sed 's/\x1b\[[0-9;]*m//g'
```

For machine-readable output, use one of the structured API surfaces
(`/api/`, `/rest/v1/`, `/graphql`, or ConnectRPC) instead of scraping `/ui/`.

### Routes

| Route | Description |
|-------|-------------|
| `GET /ui/` | Home page. Accepts `?q=` for pre-rendered search results (shareable URLs) |
| `GET /ui/search?q=` | Search results. Returns a full page, an htmx fragment, or a terminal render depending on headers. Sets `HX-Push-Url` for browser history |
| `GET /ui/asn/{asn}` | Network detail by ASN (1 .. 2³²−1; values outside the range return `400 Problem+JSON`) |
| `GET /ui/ix/{id}` | Internet exchange detail by numeric ID |
| `GET /ui/fac/{id}` | Facility detail |
| `GET /ui/org/{id}` | Organization detail |
| `GET /ui/campus/{id}` | Campus detail |
| `GET /ui/carrier/{id}` | Carrier detail |
| `GET /ui/about` | Build info, sync freshness, environment summary. Opted out of response caching in `middleware.NewCachingState` because it renders relative time (e.g. "5 minutes ago") |
| `GET /ui/compare` | ASN comparison form. `?asn1=` and `?asn2=` pre-fill the form |
| `GET /ui/compare/{asn1}` | Pre-fills the form with `asn1`, awaits `asn2` |
| `GET /ui/compare/{asn1}/{asn2}` | Comparison results. `?view=shared` (default) / `all` / `differences` toggles the panes |
| `GET /ui/completions/bash` | Installable bash completion script |
| `GET /ui/completions/zsh` | Installable zsh completion script |
| `GET /ui/completions/search?q=&type=` | Newline-separated name suggestions used by the shell completion scripts (`type=net|ix|fac|org`) |

Unknown `/ui/*` paths render the themed 404 page via `handleNotFound`.

## 2. GraphQL (`/graphql`)

GraphQL is served by [gqlgen](https://gqlgen.com) wired through
[entgql](https://entgo.io/docs/graphql-integration/). The handler lives in
`internal/graphql/handler.go`.

| Method | Behavior |
|--------|----------|
| `GET /graphql` | Serves the GraphiQL playground (HTML page, CDN-hosted JS, pre-populated with example queries). Introspection is always enabled |
| `POST /graphql` | Executes a GraphQL query. Request body capped at 1 MB |

### Operational limits

| Limit | Value | Enforcement |
|-------|-------|-------------|
| Request body | 1 MB | `http.MaxBytesReader` wrap at the route + global `MaxBytesBody` middleware |
| Query complexity | 500 | `gqlgen.extension.FixedComplexityLimit(500)` |
| Query depth | 15 | `gqlgen-depth-limit-extension` |

### Error envelope

Errors are returned in standard GraphQL format with an `extensions.code` field
populated by `classifyError` in `internal/graphql/handler.go`:

| Code | Trigger |
|------|---------|
| `NOT_FOUND` | `ent.IsNotFound(err)` |
| `VALIDATION_ERROR` | `ent.IsValidationError(err)` |
| `CONSTRAINT_ERROR` | `ent.IsConstraintError(err)` |
| `INTERNAL_ERROR` | Anything else |

Every error also includes a populated `path` pointing at the offending field.

### Example

```graphql
{
  networkByAsn(asn: 13335) {
    name
    asn
    infoType
    website
    organization { name }
  }
}
```

Schema browsing is easiest through the GraphiQL playground. The schema is
generated from `ent/schema/` and committed to `graph/schema.graphqls`.

## 3. REST (`/rest/v1/`)

The REST surface is generated by
[entrest](https://github.com/lrstanley/entrest) directly from the ent schemas
with read-only operations only (`OperationRead` + `OperationList`).

| Path | Description |
|------|-------------|
| `GET /rest/v1/openapi.json` | OpenAPI 3 specification for the full REST surface. Regenerated as part of `go generate ./...` |
| `GET /rest/v1/{collection}` | List resources with filtering and pagination per the OpenAPI spec |
| `GET /rest/v1/{collection}/{id}` | Get a single resource |

The collection paths and filter parameters are defined by entrest annotations
in `ent/schema/`; consult the OpenAPI spec for the canonical list.

### Error format

Non-2xx responses are rewritten to [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html)
by `restErrorMiddleware` in `cmd/peeringdb-plus/main.go`. The response
`Content-Type` is `application/problem+json` and the body includes at minimum
`status`, `title`, `detail`, and `instance` fields.

## 4. PeeringDB Compatibility API (`/api/`)

Implemented in `internal/pdbcompat/`, this surface is a **drop-in replacement
for the PeeringDB REST API** — URL structure, response envelope, filter
operators, and the single-object-wrapped-in-array quirk all match the upstream
API so existing clients can switch to a PeeringDB Plus instance with only a
base-URL change.

### Routes

| Route | Description |
|-------|-------------|
| `GET /api/` | JSON index mapping each of the 13 type names to its list endpoint |
| `GET /api/{type}` | List endpoint |
| `GET /api/{type}/{id}` | Single object by numeric ID, wrapped in `data: [ ... ]` (intentional parity with upstream) |

Valid `{type}` values are the same 13 constants defined in
`internal/peeringdb/types.go`: `org`, `net`, `fac`, `ix`, `poc`, `ixlan`,
`ixpfx`, `netixlan`, `netfac`, `ixfac`, `carrier`, `carrierfac`, `campus`.

### Query parameters

| Parameter | Applies to | Description |
|-----------|------------|-------------|
| `q` | List | Case-insensitive substring search across the type's search fields. For `/api/net`, an ASN literal (e.g. `8075` or `AS8075`) also matches `net.asn` exactly in addition to the text fields |
| `limit` | List | Maximum rows in response. **Default unlimited** when absent — matches upstream `rest.py:495` (`limit` defaults to `0`) + `rest.py:737` (no slice when `limit=0`). Bare `/api/<type>` URLs return ALL rows from the filtered queryset; the response is gated only by the response memory budget (see below). Explicit `limit=N`: positive `N` is clamped to `MaxLimit=1000`; `limit=0` is the explicit "unlimited" sentinel; negative values are ignored. Constants: `DefaultLimit=0`, `MaxLimit=1000` (`internal/pdbcompat/response.go`). The `?page=N` shape is not supported — clients that want pagination set `?limit=N&skip=M` instead |
| `skip` | List | Offset for pagination. Negative values are ignored |
| `depth` | Detail | Edge expansion depth. Accepted values: `0` (default, flat object) and `2` (embed related `_set` collections). Any other value is silently ignored. **List endpoints silently drop `?depth=`** — see § Known Divergences |
| `fields` | Both | Comma-separated projection — only the listed JSON keys are returned after retrieval |
| `since` | List | Only return rows with `updated` greater than the given timestamp (Unix seconds). Invalid input returns `400`. Activates the upstream "since matrix" — see "Soft-delete tombstones" below |
| `{field}`, `{field}__{op}` | List | Arbitrary field filter. Operator suffixes: `__contains`, `__icontains`, `__startswith`, `__istartswith`, `__iexact`, `__in`, `__lt`, `__lte`, `__gt`, `__gte`. `contains` and `startswith` are coerced to their case-insensitive variants per upstream `rest.py:638-641`. Typed against the field; invalid types (e.g. `asn__contains`) return `400` |

Unknown query parameters that are not in the reserved set (`limit`, `skip`,
`depth`, `since`, `q`, `fields`) are treated as field filters and validated
against the type's schema. Unknown filter keys (including over-cap traversal
keys) are silently ignored — they do not cause `400` — and a debug-level slog
record plus an `pdbplus.filter.unknown_fields` OTel span attribute are
emitted so operators can observe them. See § Cross-entity traversal for the
2-hop cap and § Validation Notes for the rationale.

`__in` accepts a CSV value and binds as a single JSON array via SQLite's
`json_each()`, sidestepping the variable-binding limit. An empty `__in`
(`?asn__in=`) short-circuits the request to an empty `data: []` envelope
without running SQL (Phase 69 IN-02). Malformed `__in` values for typed
fields (e.g. non-integer in `asn__in=`) return `400`.

### Diacritic-insensitive substring / prefix search

`?<field>__contains=` and `?<field>__startswith=` (and their `__icontains` /
`__istartswith` aliases) on the following 16 fields are diacritic-insensitive
— they match `Köln` and `koln` interchangeably:

| Entity | Folded fields |
|--------|---------------|
| `org` | `name`, `aka`, `city` |
| `net` | `name`, `aka`, `name_long` |
| `fac` | `name`, `aka`, `city` |
| `ix` | `name`, `aka`, `name_long`, `city` |
| `carrier` | `name`, `aka` |
| `campus` | `name` |

Implementation: each row carries a sibling `<field>_fold` shadow column
populated at sync time via `internal/unifold.Fold` (NFKD decomposition + a
ligature map). Filter routing happens in `internal/pdbcompat/filter.go`
`buildContains` / `buildStartsWith` — when `tc.FoldedFields[<field>]` is
`true` the predicate runs against `<field>_fold` with `unifold.Fold(value)`
on the RHS. The shadow columns carry `entgql.Skip(SkipAll)` and
`entrest.WithSkip(true)` so they are invisible to GraphQL, REST, and proto
wire surfaces — they exist only to power pdbcompat folding. See § Known
Divergences for the upstream-parity comparison and § Validation Notes for
why MySQL collation is *not* the upstream mechanism.

### Soft-delete tombstones (Phase 68)

Sync soft-deletes rows by setting `status='deleted'` rather than physically
removing them. The list path applies the upstream `rest.py:694-727` status
matrix as the final predicate via `applyStatusMatrix`:

| Request shape | Admitted statuses |
|---------------|-------------------|
| List, no `?since` | `status='ok'` only |
| List with `?since=N` | `status IN ('ok', 'deleted')`; `pending` additionally admitted on `/api/campus` |
| Single-object GET `/api/<type>/<id>` | `status IN ('ok', 'pending')` — tombstones return `404` |

`?status=<value>` is dropped at the filter layer (the key is absent from
every type's `Fields` map in `internal/pdbcompat/registry.go`) — the
observable outcome is identical to upstream's effective behaviour. See
§ Known Divergences for the explicit comparison.

### Response memory budget

Every list response is gated by a pre-flight 413 budget check before any
SQL is executed. `serveList` in `internal/pdbcompat/handler.go` runs a
`SELECT COUNT(*)` against the filtered query, multiplies by the per-entity
typical row size, and refuses up-front if the projected response exceeds
`PDBPLUS_RESPONSE_MEMORY_LIMIT` (default `128MiB`). `0` disables the check
(local-dev escape hatch only).

A budget-exceeded request returns:

- `413 Request Entity Too Large`
- `Content-Type: application/problem+json`
- `type: https://peeringdb-plus.fly.dev/errors/response-too-large`
- Body extension fields `max_rows` (the largest result set that *would* fit)
  and `budget_bytes` (the configured ceiling)

Operators receiving a 413 should narrow their filters or page smaller — the
budget is request-shape, not transient resource pressure, so retrying the
identical request returns the same 413. The budget is enforced only on the
pdbcompat list path; entrest, GraphQL, ConnectRPC, and Web UI have their own
memory stories (see `docs/ARCHITECTURE.md § Response Memory Envelope`).

### Examples

```bash
# All networks with ASN 15169 (Google)
curl "https://peeringdb-plus.fly.dev/api/net?asn=15169"

# Single network by internal ID, with edges expanded
curl "https://peeringdb-plus.fly.dev/api/net/20/?depth=2"

# Full-text search against networks, matching name, aka, name_long, irr_as_set
# and the asn column for numeric queries
curl "https://peeringdb-plus.fly.dev/api/net?q=AS8075"

# First 50 IXPs in country DE
curl "https://peeringdb-plus.fly.dev/api/ix?country=DE&limit=50"

# Only return id and name
curl "https://peeringdb-plus.fly.dev/api/ix?country=DE&fields=id,name"

# Networks updated since 2024-01-01 (Unix seconds) — admits tombstones
curl "https://peeringdb-plus.fly.dev/api/net?since=1704067200"

# Diacritic-insensitive substring match against organization names
curl "https://peeringdb-plus.fly.dev/api/org?name__contains=koln"

# 2-hop traversal: facilities whose parent organization is named "DE-CIX"
curl "https://peeringdb-plus.fly.dev/api/fac?org__name=DE-CIX"
```

### Response envelope

All responses follow the PeeringDB shape:

```json
{
  "meta": {},
  "data": [ { "...": "..." } ]
}
```

Detail endpoints return a single-element `data` array, not a bare object, to
preserve parity with upstream PeeringDB clients.

### Errors

Errors use [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html)
with `Content-Type: application/problem+json`. Typical status codes:

| Status | Cause |
|--------|-------|
| `400` | Invalid filter operator, malformed `since`, non-integer ID, filter type mismatch, malformed `__in` value |
| `404` | Unknown `{type}`, missing `{id}`, or detail GET on a tombstoned row |
| `413` | Pre-flight response memory budget exceeded — see "Response memory budget" above |
| `500` | Database error (details redacted from response body, full error logged) |

Responses include an `X-Powered-By` header identifying the server as
PeeringDB Plus.

## 5. ConnectRPC / gRPC (`/peeringdb.v1.*`)

Implemented in `internal/grpcserver/` using
[ConnectRPC](https://connectrpc.com/) — a gRPC-compatible framework that
speaks three protocols on the same endpoint:

| Protocol | Typical client | Content types |
|----------|----------------|---------------|
| Connect (HTTP/1.1 or HTTP/2) | `connect-go`, browser fetch | `application/proto`, `application/json` |
| gRPC (HTTP/2) | `grpc-go`, `grpcurl`, any gRPC stub | `application/grpc`, `application/grpc+proto`, `application/grpc+json` |
| gRPC-Web | Browser gRPC-Web clients | `application/grpc-web`, `application/grpc-web-text` |

The server listens on a single port with h2c enabled
(`buildServer` in `cmd/peeringdb-plus/main.go`), so there is no separate
port for gRPC.

### Services

All 13 entity types expose the same three RPCs
(`proto/peeringdb/v1/services.proto`):

| Service | Get | List | Stream |
|---------|-----|------|--------|
| `peeringdb.v1.CampusService` | `GetCampus` | `ListCampuses` | `StreamCampuses` |
| `peeringdb.v1.CarrierService` | `GetCarrier` | `ListCarriers` | `StreamCarriers` |
| `peeringdb.v1.CarrierFacilityService` | `GetCarrierFacility` | `ListCarrierFacilities` | `StreamCarrierFacilities` |
| `peeringdb.v1.FacilityService` | `GetFacility` | `ListFacilities` | `StreamFacilities` |
| `peeringdb.v1.InternetExchangeService` | `GetInternetExchange` | `ListInternetExchanges` | `StreamInternetExchanges` |
| `peeringdb.v1.IxFacilityService` | `GetIxFacility` | `ListIxFacilities` | `StreamIxFacilities` |
| `peeringdb.v1.IxLanService` | `GetIxLan` | `ListIxLans` | `StreamIxLans` |
| `peeringdb.v1.IxPrefixService` | `GetIxPrefix` | `ListIxPrefixes` | `StreamIxPrefixes` |
| `peeringdb.v1.NetworkService` | `GetNetwork` | `ListNetworks` | `StreamNetworks` |
| `peeringdb.v1.NetworkFacilityService` | `GetNetworkFacility` | `ListNetworkFacilities` | `StreamNetworkFacilities` |
| `peeringdb.v1.NetworkIxLanService` | `GetNetworkIxLan` | `ListNetworkIxLans` | `StreamNetworkIxLans` |
| `peeringdb.v1.OrganizationService` | `GetOrganization` | `ListOrganizations` | `StreamOrganizations` |
| `peeringdb.v1.PocService` | `GetPoc` | `ListPocs` | `StreamPocs` |

The URL path for every RPC is `/{fully.qualified.ServiceName}/{MethodName}` —
e.g. `/peeringdb.v1.NetworkService/GetNetwork`.

### Filtering

List and Stream requests accept type-specific optional filter fields (see
`proto/peeringdb/v1/services.proto`). All filters AND together. String filters
for free-text columns (name, aka, name_long) use case-insensitive substring
match (`ContainsFold`); other strings are matched exactly. Integer filters
such as `asn` and `org_id` are validated to be positive; invalid values return
`INVALID_ARGUMENT`.

### Pagination (List)

| Field | Semantics |
|-------|-----------|
| `page_size` | Requested page size. Defaults to `100`, clamped to `1000`. See `normalizePageSize` in `internal/grpcserver/pagination.go` |
| `page_token` | Opaque cursor. Clients pass back the `next_page_token` from the previous response to fetch the next page. Invalid tokens return `INVALID_ARGUMENT` |

### Streaming semantics

`Stream{Type}` RPCs use **batched compound keyset pagination** under the
hood (`StreamEntities` in `internal/grpcserver/generic.go`), fetching
`streamBatchSize` (`500`) rows per database round-trip and emitting one proto
message per row. The cursor is the compound `(updated, id)` pair; under the
default `(-updated, -created, -id)` order each batch resumes via:

```sql
WHERE (updated < cursor.updated)
   OR (updated = cursor.updated AND id < cursor.id)
```

The `id` tiebreaker keeps progress monotonic when multiple rows share a
timestamp.

| Field | Semantics |
|-------|-----------|
| `since_id` | Filter — emits only rows with `id > since_id`. Applied as a `WHERE` predicate; **does not seed the keyset cursor** |
| `updated_since` | Filter — emits only rows with `updated > updated_since`. Applied as a `WHERE` predicate; **does not seed the keyset cursor** |

Every stream is capped by `PDBPLUS_STREAM_TIMEOUT` (default `60s`) enforced via
`context.WithTimeout` at the handler. Exceeding the timeout closes the stream
with a cancellation error.

### `grpc-total-count` response header

On **full streams** (both `since_id` and `updated_since` unset), the handler
runs a `SELECT COUNT(*)` preflight and sets the `grpc-total-count` response
header to the total matching row count. On **delta streams** (either `since_id`
or `updated_since` set), the COUNT preflight is skipped entirely and the
`grpc-total-count` header is **absent** — not "present with 0" and not
"present with -1". Clients of delta streams have no use for a full-table total
and the skip avoids a needless full-table scan.

### Reflection and health

| Handler | Path |
|---------|------|
| gRPC reflection v1 | `/grpc.reflection.v1.ServerReflection/*` |
| gRPC reflection v1alpha | `/grpc.reflection.v1alpha.ServerReflection/*` |
| gRPC health check | `/grpc.health.v1.Health/*` |

Reflection serves all 13 service descriptors, enabling `grpcurl` and `grpcui`
to discover the API with no additional wiring. The health checker reports
`NOT_SERVING` until the first sync completes (`HasCompletedSync` in the sync
worker), then flips to `SERVING` for the empty service name and for every
`peeringdb.v1.*` service. The health handler bypasses the readiness middleware
so that health checks can poll the service during sync-in-progress state
without being intercepted by the 503 syncing page.

### Example clients

```bash
# Get one network by ID using grpcurl (reflection-driven)
grpcurl -d '{"id": 20}' peeringdb-plus.fly.dev:443 \
  peeringdb.v1.NetworkService/GetNetwork

# List first 10 ASNs registered under organization 10
grpcurl -d '{"page_size": 10, "org_id": 10}' peeringdb-plus.fly.dev:443 \
  peeringdb.v1.NetworkService/ListNetworks

# Stream all organizations updated since an RFC3339 timestamp
grpcurl -d '{"updated_since": "2025-01-01T00:00:00Z"}' peeringdb-plus.fly.dev:443 \
  peeringdb.v1.OrganizationService/StreamOrganizations

# Connect-over-HTTP JSON — works with plain curl
curl -X POST https://peeringdb-plus.fly.dev/peeringdb.v1.NetworkService/GetNetwork \
  -H "Content-Type: application/json" \
  -d '{"id": 20}'
```

## Field-level privacy (Phase 64)

PeeringDB Plus mirrors upstream PeeringDB's per-field visibility marker for
the IX-F member list URL: `ixlan.ixf_ixp_member_list_url` is gated by the
sibling `ixlan.ixf_ixp_member_list_url_visible` enum (`Public` / `Users` /
`Private`). Anonymous callers (the default `PDBPLUS_PUBLIC_TIER=public`
deployment) receive the value only when `_visible = Public`; for `Users` or
`Private` the value is omitted across all five surfaces while the `_visible`
companion field is **still emitted** (upstream parity, Phase 64 D-05).

The single source of truth is `internal/privfield.Redact(ctx, visible,
value)`. Every serializer calls it, and `internal/middleware.PrivacyTier`
stamps the resolved tier on the request context — unstamped contexts
fail-closed to `TierPublic`.

| Surface | Mechanism |
|---------|-----------|
| `/api/` (pdbcompat) | `internal/pdbcompat/serializer.go` `ixLanFromEnt(ctx, l)`; the JSON struct tag `,omitempty` produces wire absence |
| `/rest/v1/ix-lans*` (entrest) | `restFieldRedactMiddleware` in `cmd/peeringdb-plus/main.go` buffers the JSON response and deletes the key in-place when `Redact` returns `omit=true`. Wraps INSIDE `restErrorMiddleware` so error bodies pass through |
| `/peeringdb.v1.IxLanService/*` (ConnectRPC) | `internal/grpcserver/ixlan.go` `ixLanToProto(ctx, il)` returns `nil *wrapperspb.StringValue` — wire absence under proto3 optional |
| `/graphql` | `graph/schema.resolvers.go` `ixLanResolver.IxfIxpMemberListURL` returns Go `nil` → GraphQL `null` |
| `/ui/` | No render path renders the URL today; future templates must call `privfield.Redact` in the data-prep step |

Operators who run a private deployment can flip `PDBPLUS_PUBLIC_TIER=users`
to make anonymous callers behave as authenticated users — the startup logger
emits a `WARN` with `public_tier=users` so the override is visible in deploy
logs.

## Infrastructure endpoints

### `GET /`

Root endpoint with content negotiation (`main.go` `GET /{$}`):

| Client | Response |
|--------|----------|
| Terminal (curl, wget, HTTPie, …) | `text/plain` help screen rendered by `termrender.NewRenderer` with optional ANSI colors |
| Browser (`Accept: text/html`) | `302 Found` redirect to `/ui/` |
| JSON client (`Accept: application/json`) | Static service discovery document (see below) |
| Default | Same JSON service discovery document |

Service discovery JSON body:

```json
{
  "name": "peeringdb-plus",
  "version": "0.1.0",
  "graphql": "/graphql",
  "rest": "/rest/v1/",
  "api": "/api/",
  "connectrpc": "/peeringdb.v1.",
  "ui": "/ui/",
  "healthz": "/healthz",
  "readyz": "/readyz"
}
```

The root bypasses the readiness middleware so service discovery still works
while the first sync is in progress.

### `GET /healthz`

Liveness probe. Always returns `200 OK` with a fixed JSON body as long as the
process can serve HTTP. It does **not** check database connectivity or sync
state — a failing `/healthz` means the process itself is wedged and should be
restarted.

Bypasses the readiness middleware.

### `GET /readyz`

Readiness probe. Returns `200 OK` only when:

1. The database is reachable (`db.PingContext`).
2. The most recent sync is more recent than `PDBPLUS_SYNC_STALE_THRESHOLD`
   (default `24h`).

Returns `503 Service Unavailable` otherwise. The response body is the opaque
shape `{"status":"ok"}` or `{"status":"unhealthy"}` — detailed error strings
are written to structured logs only (security hardening: the wire body does
not leak internal failure detail).

Bypasses the readiness-gate middleware itself (so a probe can observe the
unready state rather than being redirected to the syncing page).

### `POST /sync`

On-demand sync trigger. Only served by the LiteFS primary.

| Header / Param | Purpose |
|----------------|---------|
| `X-Sync-Token` request header | Must match `PDBPLUS_SYNC_TOKEN` using `subtle.ConstantTimeCompare`. Empty token on either side = always reject |
| `?mode=full` or `?mode=incremental` | Overrides `PDBPLUS_SYNC_MODE` for this run. Any other value returns `400` |

| Status | Meaning |
|--------|---------|
| `202 Accepted` | Request authenticated; sync started in the background (fire-and-forget, using the application root context so request cancellation does not abort it) |
| `307 Temporary Redirect` + `fly-replay: region=<primary>` | On Fly.io replicas, the request is replayed to the primary region. Clients following the header will land on the primary and complete the call |
| `401 Unauthorized` | Missing, wrong, or mismatched-length `X-Sync-Token` |
| `400 Bad Request` | Invalid `mode` value |
| `503 Service Unavailable` | Replica outside Fly.io (no `FLY_REGION`) cannot forward the request |

Request body is capped at 1 MB.

Bypasses the readiness middleware so an operator can kick off the first sync
before any sync has completed.

## Rate limits

PeeringDB Plus does not itself enforce request rate limits at any edge. The
only operational limits are:

| Limit | Scope | Default | Configured by |
|-------|-------|---------|---------------|
| Request body size | Every non-gRPC request | `1 MB` | `maxRequestBodySize` in `main.go` (hardcoded) |
| Read header timeout | Every connection | `10s` | `buildServer` (hardcoded) |
| Read timeout | Every connection | `30s` | `buildServer` (hardcoded) |
| Idle timeout | Every keep-alive connection | `120s` | `buildServer` (hardcoded) |
| Stream timeout | ConnectRPC `Stream{Type}` | `60s` | `PDBPLUS_STREAM_TIMEOUT` |
| Graceful drain timeout | Shutdown | `10s` | `PDBPLUS_DRAIN_TIMEOUT` |
| Sync memory ceiling | Sync worker heap | `400MB` | `PDBPLUS_SYNC_MEMORY_LIMIT` |
| Response memory budget | pdbcompat `/api/` list | `128MiB` | `PDBPLUS_RESPONSE_MEMORY_LIMIT` |
| GraphQL query complexity | `POST /graphql` | `500` | `FixedComplexityLimit` (hardcoded) |
| GraphQL query depth | `POST /graphql` | `15` | `FixedDepthLimit` (hardcoded) |

Upstream PeeringDB rate limits apply to the sync worker's outbound requests —
setting `PDBPLUS_PEERINGDB_API_KEY` raises that ceiling.

Deployment-level rate limiting (e.g., Fly.io edge, Cloudflare, or a load
balancer) is not configured in this repository. <!-- VERIFY: rate limiting policy on the peeringdb-plus.fly.dev deployment -->

## CORS

All surfaces pass through the shared `middleware.CORS` configured by
`PDBPLUS_CORS_ORIGINS` (default `*`). The middleware allows the full set of
headers required by Connect / gRPC / gRPC-Web in addition to standard
application headers; see `internal/middleware/cors.go`. The REST handler
additionally wraps its subtree in its own CORS middleware so that preflight
requests for `/rest/v1/*` are handled even if another middleware short-circuits.

## Known Divergences

PeeringDB Plus strives for behavioural parity with the upstream PeeringDB API
(`peeringdb/peeringdb`) at the `/api/` surface. Divergences are documented
here with upstream source-line citations so operators can audit the
boundaries intentionally. Each row cross-references a parity test under
`internal/pdbcompat/parity/*_test.go` — typically a `DIVERGENCE_<…>` sub-test.

| Request | Upstream behaviour | peeringdb-plus behaviour | Rationale | Since |
|---------|-------------------|-------------------------|-----------|-------|
| `GET /api/<type>?status=<value>` (list, no `?since`) | Upstream `rest.py:700-712` builds `allowed_status` from the caller's `?status=` parameter (or a default set), then applies a final unconditional `filter(status='ok')` on `rest.py:725`. The caller-supplied value is effectively overridden by the final filter — only `status=ok` rows ever reach the response. | pdbcompat drops `?status=` at the filter layer (the `status` entry is removed from all 13 `Fields` maps in `internal/pdbcompat/registry.go`) so the predicate never reaches ent. The observable outcome is identical to upstream; the implementation is explicit rather than implicit. | Upstream's double-filter is a no-op by design. We model the intent rather than the mechanism, which makes the D-07 semantic greppable in one place. | v1.16 (Phase 68) |
| `GET /api/<type>?status=deleted&since=N` for a row hard-deleted by sync cycles before v1.16 | Upstream returns the tombstone row with its deletion timestamp (upstream has always soft-deleted). | peeringdb-plus returns empty for such rows. Tombstone population begins at the first post-upgrade sync cycle; anything hard-deleted before v1.16 shipped is gone forever. Rows deleted from v1.16 onwards are visible via both `?since=N` windows and pk lookup (pk admits `status IN (ok, pending)`; tombstones are reachable only via the `since` window). | No retroactive reconstruction is possible — PeeringDB's public API does not expose historical state, and we did not persist deleted rows prior to the Phase 68 soft-delete flip (D-03). Documented intentional one-time gap. See `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63:src/peeringdb_server/rest.py:694-727` for upstream status matrix. Parity-locked by `TestParity_Status/STATUS-04_list_since_admits_deleted_excludes_pending_noncampus`. | v1.16 (Phase 68; locked in Phase 72) |
| `?limit=0` interpreted as "return a count envelope only" (pdbfe claim) | Upstream `rest.py:494-497` treats `limit=0` as **unlimited** (`if limit == 0: limit = None` — Python `None` means no SQL `LIMIT`). There is no count-only semantic upstream — pdbfe's gotchas doc is simply wrong on this point. | peeringdb-plus matches upstream: `limit=0` returns all matching rows unbounded, gated only by the Phase 71 `PDBPLUS_RESPONSE_MEMORY_LIMIT` budget (default 128 MiB). Callers wanting a count should use the `meta.count` field on a depth=0 list response, not `limit=0`. | We match upstream semantics verbatim rather than codifying an invalid-pdbfe-claim as a behavioural divergence. See § Validation Notes entry 2. Parity-locked by `TestParity_Limit/LIMIT-01_zero_returns_all_rows_unbounded` and `TestParity_Limit/LIMIT-01b_zero_over_budget_returns_413_problem_json`. | v1.16 (Phase 68; locked in Phase 72) |
| `?depth=N` on list endpoints (any `/api/<type>` without a pk) | Upstream `rest.py:744-748` accepts `?depth=` on list requests and caps row count at `API_DEPTH_ROW_LIMIT=250`. | peeringdb-plus silently drops `?depth=` on list endpoints with a `slog.DebugContext` paper trail (Phase 68 LIMIT-02 guardrail — `opts.Depth` is never threaded into list closures). `?depth=` on single-object GET (`/api/<type>/<id>`) works as upstream specifies. Functional list+depth is deferred indefinitely — the Phase 71 `budget.go` memory envelope would refuse the resulting response sizes on 256 MB replicas anyway. | Memory envelope on 256 MB replicas (Phase 71 D-06 — 13-entity × 2-depth worst case exceeds the 128 MiB budget for any realistic row count). `docs/ARCHITECTURE.md § Response Memory Envelope` documents the per-entity ceiling. Parity-locked by `TestParity_Limit/LIMIT-02_depth_on_list_silently_dropped_DIVERGENCE`. | v1.16 (Phase 68 guardrail; locked in Phase 72) |
| `?<field>__contains=<non-ASCII>` / `?<field>__startswith=<non-ASCII>` against searchable text fields on `network`, `facility`, `ix`, `organization`, `campus`, `carrier` (16 fields total — see "Diacritic-insensitive substring / prefix search" above) | Upstream applies `unidecode.unidecode(v)` to BOTH the query value and the column at query time (`rest.py:576`), producing diacritic-insensitive matches in a single SQL pass. | peeringdb-plus precomputes the folded value into a sibling `<field>_fold` shadow column at sync time (via `internal/unifold.Fold` — NFKD normalisation + a small ligature map for `ß`/`æ`/`ø`/`ł`/`þ`/`đ`), then routes `__contains` / `__startswith` to `<field>_fold LIKE ?` with `unifold.Fold(query)` on the RHS. The end-state semantic match is identical, but it is staged differently: a brief one-time ASCII-only window exists between v1.16 deploy and the first post-deploy sync cycle (≤1h with default `PDBPLUS_SYNC_INTERVAL=1h`) during which rows have `<field>_fold = ''` and return no match for non-ASCII queries. ASCII queries continue to work via the non-folded columns throughout the window. No manual backfill is required — the next sync cycle's `OnConflict().UpdateNewValues()` path rewrites every row's `_fold` columns. | Shadow columns let SQLite use a single indexable comparison path (no per-query `unidecode` call), and Phase 69 benchstat (n=6, 10k rows) shows the shadow path within ±1% of the direct path so the trade-off is invisible at production scale. The folded columns carry `entgql.Skip(SkipAll)` + `entrest.WithSkip(true)` annotations and are never exposed on the GraphQL / REST / proto wire surfaces — they are server-side plumbing only. | v1.16 (Phase 69) |
| `GET /api/<type>?a__b__c__d=X` (3+ `__`-separated relation segments) | Upstream walks arbitrary Django ORM chains — no hard depth cap (bounded only by the Django ORM query planner). | peeringdb-plus silently ignores the filter (HTTP 200, unfiltered) per Phase 70 D-04. One aggregated `slog.DebugContext("pdbcompat: unknown filter fields silently ignored (Phase 70 TRAVERSAL-04)", ...)` plus OTel span attribute `pdbplus.filter.unknown_fields` record the dropped key. Keys with exactly 1 or 2 relation segments resolve normally via Path A (explicit allowlist) or Path B (ent edge introspection). | DoS protection: 3+-hop joins in SQLite can trigger super-linear query plan scans at scale, and the replica memory envelope (256 MB post-Phase-65) cannot absorb unbounded Cartesian-product row counts. The 2-hop cap trades limitless traversal for a predictable cost ceiling gated in CI by `internal/pdbcompat/bench_traversal_test.go` (`<50ms/op @ 10k rows`). | v1.16 (Phase 70) |
| `GET /api/fac?ixlan__ix__fac_count__gt=0` (DEFER-70-verifier-01; upstream citation `pdb_api_test.py:2340, 2348`) | Upstream resolves via a per-serializer `prepare_query` that joins `fac → ixfac → ix → fac_count` (3-hop bespoke SQL). | peeringdb-plus silently ignores the filter (HTTP 200, unfiltered result) because `fac` has no direct `ixlan` edge in the ent schema — `ixlan` belongs to `ix`, not to `fac` — and the 3-hop walk via `ixfac` exceeds the hard 2-hop cap (Phase 70 D-04). The generic 2-hop mechanism continues to work for entity pairs with direct edges (e.g. `/api/ixpfx?ixlan__ix__id=20` resolves correctly). | Relaxing the 2-hop cap re-opens cost-ceiling concerns that D-04 was designed to contain (unbounded Cartesian-product joins in SQLite under 256 MB replica memory envelope). Adding a bespoke per-serializer hook for this single upstream citation case doesn't fit the generic D-01/D-04 model cleanly. See `.planning/milestones/v1.16-phases/70-cross-entity-traversal/deferred-items.md` § DEFER-70-verifier-01. | v1.16 (Phase 70; locked by `TestParity_Traversal/DIVERGENCE_fac_ixlan_ix_fac_count_silent_ignore` in Phase 72) |

## Validation Notes

Future conformance auditors reading third-party gotchas documentation
(notably pdbfe's upstream-behaviour claims) against the PeeringDB Plus
codebase may encounter assertions about upstream behaviour that turn
out to be wrong. This section documents 5 such invalid claims
identified during the v1.16 audit (Phases 67-72), each with a pinned
`peeringdb/peeringdb@<sha>` reference so the authoritative upstream
source can be re-read without re-research. All 5 were confirmed
against commit
`peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63` (the
same SHA pinned in `internal/testutil/parity/fixtures.go`).

| Claim | Verdict | Upstream truth | Our implementation |
|-------|---------|----------------|--------------------|
| `net?country=NL` is a valid filter key | **WRONG** | `country` lives on `org`, not `net`. See `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63:src/peeringdb_server/serializers.py:2938-2992` — `NetworkSerializer.prepare_query` has no `country` key, and `django-peeringdb/src/django_peeringdb/models/abstract.py`'s Network model has no country field. Callers who want `net` filtered by country must traverse through `org` (e.g. `net?org__country=NL`). | Filter key silently ignored via the TRAVERSAL-04 silent-ignore mechanism (Phase 70 D-04) — no row-level match, response unfiltered. OTel span attribute `pdbplus.filter.unknown_fields` records the dropped key. Parity-locked by `TestParity_Traversal/TRAVERSAL-04_unknown_field_silently_ignored_with_otel_attr`. |
| `?limit=0` returns a count-only envelope | **WRONG** | `limit=0` means unlimited. See `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63:src/peeringdb_server/rest.py:494-497` — `if limit == 0: limit = None` (Python `None` = no SQL `LIMIT` clause). There is no count-only semantic upstream. Callers wanting a count should use the `meta.count` field on a standard depth=0 list response. | Unbounded response via Phase 68 LIMIT-01 (ent v0.14.6 `.Limit(0)` = unlimited, gated by sqlgraph `graph.go:1086 if q.Limit != 0`) plus Phase 71 memory budget (`PDBPLUS_RESPONSE_MEMORY_LIMIT`, default 128 MiB) with RFC 9457 413 on over-budget. Parity-locked by `TestParity_Limit/LIMIT-01_zero_returns_all_rows_unbounded` and `TestParity_Limit/LIMIT-01b_zero_over_budget_returns_413_problem_json`. |
| Default list ordering is `id ASC` | **WRONG** | Default ordering is `(-updated, -created)` via the `django-handleref` base Meta. See `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63` + upstream dep `django-handleref:src/django_handleref/models.py:95-101` (`class Meta: ordering = ('-updated', '-created')`). Every PeeringDB model inherits from this base, so the default applies across all 13 entity types. | Phase 67 flipped pdbcompat `/api/*`, entrest `/rest/v1/*`, ConnectRPC list RPCs, and GraphQL list queries to the compound `(-updated, -created, -id)` order (the trailing `-id` is a tie-breaker to ensure stable cursor pagination). Single-object lookups and nested `_set` fields are unchanged per D-04. Parity-locked by `TestParity_Ordering/default_list_order_updated_desc`, `tiebreak_by_created_desc`, and `tiebreak_by_id_desc`. |
| Unicode folding uses MySQL collation (`utf8_general_ci` or similar) | **WRONG** | Folding is Python-side via `unidecode.unidecode(v)` at query time. See `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63:src/peeringdb_server/rest.py:576` — the call happens in the Python filter construction layer before any SQL is emitted, so the database collation is irrelevant. | Phase 69 uses shadow `<field>_fold` columns populated at sync time via `internal/unifold.Fold` (`golang.org/x/text/unicode/norm` NFKD decomposition + a hand-rolled ligature map for `ß`/`æ`/`ø`/`ł`/`þ`/`đ`). `__contains` / `__startswith` route to `<field>_fold LIKE ?` with `unifold.Fold(query)` on the RHS. Not byte-compatible with Python `unidecode` for every input (e.g. the two libraries handle rare CJK edge cases differently); any specific gap that surfaces will be logged as a new § Known Divergences row. Parity-locked by `TestParity_Unicode/UNICODE-01_net_name_contains_diacritic_matches_ascii`, `UNICODE-01_fac_city_cjk_roundtrip`, and `UNICODE-01_combining_mark_NFKD_equivalent`. |
| Filter surface is a DRF `filterset_class` per ViewSet | **WRONG** | Filter surface is a per-serializer `prepare_query(...)` method plus an auto-`queryable_relations()` mechanism with a `FILTER_EXCLUDE` denylist. See `peeringdb/peeringdb@99e92c726172ead7d224ce34c344eff0bccb3e63:src/peeringdb_server/serializers.py:754-780` (`queryable_relations()`) and `:128-157` (`FILTER_EXCLUDE`). No `django_filters.FilterSet` subclass exists anywhere in the upstream codebase. | Phase 70 Path A = `pdbcompat.WithPrepareQueryAllow` ent-schema annotations → `allowlist_gen.go` `Allowlists` map (13 entries verbatim from upstream); Path B = ent edge introspection via the generated `Edges` map. The Phase 70 D-03 `WithFilterExcludeFromTraversal` edge annotation mirrors upstream's `FILTER_EXCLUDE` — currently empty across all 13 schemas (every FK edge exposed in v1.16). Parity-locked by `TestParity_Traversal/TRAVERSAL-01_path_a_1hop_org_name` and `TRAVERSAL-03_path_b_1hop_org_city`. |

Quarterly re-validation against upstream (manually or via
`cmd/pdb-fixture-port/ --check`) is advisory only per Phase 72
CONTEXT.md D-03 — a drift alert does not block merges. Drift that
invalidates a Validation Note row should be surfaced as a GitHub
issue and reviewed against the parity test suite; if upstream has
changed semantics, update the row here and flip or retain the
matching parity assertion as a new § Known Divergences row.

## Cross-entity traversal (Phase 70)

pdbcompat resolves `<fk>__<field>` and `<fk>__<fk>__<field>` filter
paths through two mechanisms, both driven by codegen from ent schema
annotations at `go generate` time:

- **Path A — per-serializer allowlists.** Mirrors upstream
  `peeringdb_server/serializers.py` `prepare_query(...)` /
  `get_relation_filters(...)` lists verbatim. Generated from ent schema
  `pdbcompat.WithPrepareQueryAllow(...)` annotations via
  `cmd/pdb-compat-allowlist`; emitted into
  `internal/pdbcompat/allowlist_gen.go`. This is the "explicitly
  blessed" set of filter keys — every entry carries a
  `// serializers.py:<line>` comment anchoring it to upstream.
- **Path B — ent edge introspection.** When a filter key does not
  match Path A, the parser consults the generated `Edges` map (also
  emitted into `allowlist_gen.go`). Every non-excluded FK edge
  auto-exposes `<fk>__<field>` for any filterable field on the target
  entity. Mirrors upstream `queryable_relations()`. Resolution uses a
  codegen-time static map — no runtime ent-client introspection, no
  `sync.Once`, no init-order coupling (Phase 70 D-02 as amended
  2026-04-19).

The resolution order is implemented in
`internal/pdbcompat/filter.go` `buildTraversalPredicate`: Path A
first; on a soft miss (allowlist hit but downstream introspection
unavailable) the parser falls through to Path B rather than
suppressing the key (Phase 70 REVIEW WR-03). `parseFieldOp` returns
the 3-tuple `(relationSegments, finalField, op)` so the same machinery
serves 1-hop and 2-hop paths with a single split (Phase 70 D-06).

### Supported shapes per entity (1-hop + 2-hop)

All 13 entity types support Path A 1-hop shapes via
`?<fk>__<field>=X`. The 2-hop subset tracks upstream
`pdb_api_test.py`:

| Query | Hops | Path | Upstream citation |
|-------|------|------|-------------------|
| `?org__name=X` (net, fac, ix, carrier, campus) | 1 | A | `serializers.py:1823, 2244, 2361, 2423, 2573, 2732, 2947, 2995, 3315, 3451, 3622, 3925, 4041` |
| `?net__asn=X` (netfac, netixlan, poc) | 1 | A | (same allowlist block) |
| `?ix__name=X` (ixfac, ixlan, ixpfx) | 1 | A | (same allowlist block) |
| `?fac__name=X` (netfac, ixfac, carrierfac) | 1 | A | (same allowlist block) |
| `?ixlan__ix__fac_count__gt=0` (fac) | 2 | A | `pdb_api_test.py:2340, 2348` |
| `?ixlan__ix__id=N` (fac) | 2 | A | `pdb_api_test.py:5081` |
| `?<fk>__<field>=X` for any non-excluded edge | 1 | B | `serializers.py:754-780` (`queryable_relations()`) |

1-hop Path B fallthrough means the explicit Path A allowlists are
**additive, not restrictive**: a key that is not in Path A but is a
valid ent FK edge still resolves via Path B. The exclusion list
(below) is the only way to block a Path B key.

### FILTER_EXCLUDE list (TRAVERSAL success criterion #5)

The `pdbcompat.WithFilterExcludeFromTraversal()` ent edge annotation
(Phase 70 D-03) hides specific edges from Path B traversal. Mirrors
upstream `serializers.py:128-157`.

| Entity | Edge | Reason |
|--------|------|--------|
| _(none — all FK edges exposed in v1.16)_ | _—_ | Initial release. Phase 64's field-level privacy (`ixlan.ixf_ixp_member_list_url_visible`) operates at the **serializer layer**, not the edge layer, so no edge exclusion is required for the v1.16 surface. Future OAuth-gated relations (post-VIS-08 OAuth work) will populate this table. |

### 2-hop cap (Phase 70 D-04)

Filter keys with more than 2 `__`-separated relation segments are
silently ignored. Examples:

- `?org__name=X` — 1 hop, resolves via Path A (every primary entity).
- `?ixlan__ix__fac_count__gt=0` — 2 hops, resolves via Path A
  (upstream `pdb_api_test.py:2340` assertion).
- `?ixlan__ix__org__name=X` — 3 hops, SILENTLY IGNORED (HTTP 200,
  result set is unfiltered).

Upstream PeeringDB has no hard cap but is bound by Django ORM's query
planner. We trade limitless traversal for a predictable cost ceiling
gated in CI at `<50ms/op @ 10k rows` via
`internal/pdbcompat/bench_traversal_test.go` (Phase 70 D-07). If a
legitimate 3-hop use case emerges, raise the cap together with a
fresh benchstat run and a docs update here.

### Unknown-field diagnostics (Phase 70 D-05)

When a filter key fails Path A, Path B, and the 2-hop cap check,
the following observability signals fire:

- `slog.DebugContext(ctx, "pdbcompat: unknown filter fields silently
  ignored (Phase 70 TRAVERSAL-04)", slog.String("endpoint", ...),
  slog.String("type", ...), slog.Any("unknown_fields", ...))`
- OTel span attribute `pdbplus.filter.unknown_fields` (CSV of all
  unknown keys in the request)

Both are DEBUG-level; INFO and higher are untouched so that naive
clients probing field names don't flood structured logs. To surface
these in production, set `OTEL_LOG_LEVEL=debug` or query the span
attribute in Grafana/Tempo.

