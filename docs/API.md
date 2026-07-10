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
| `GET` | `/api/` | PDB Compat | Index of available object types (upstream `{"data":[{type:absolute-url}],"meta":{}}` shape) |
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
| `GET /ui/compare/{asn1}/{asn2}` | Comparison results. `?view=shared` (default) shows only IXPs/facilities/campuses where both networks are present; `?view=full` shows the union with shared-flag highlighting. Any other `view` value falls back to the shared view. |
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
| Query complexity | 1,000,000 (weighted) | `gqlgen.extension.FixedComplexityLimit(graph.ComplexityLimit)` — per-field costs weighted by row materialization via `graph.ComplexityLimits()`, not a raw field count |
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
| `limit` | List | Maximum rows in response. **Default unlimited** when absent — matches upstream `rest.py:504` (`limit` defaults to `0`) + `rest.py:744-748` (no slice when `limit=0`). Bare `/api/<type>` URLs return ALL rows from the filtered queryset; the response is gated only by the response memory budget (see below). Explicit `limit=N`: positive `N` is clamped to `MaxLimit=1000`; `limit=0` is the explicit "unlimited" sentinel; negative values are ignored. Constants: `DefaultLimit=0`, `MaxLimit=1000` (`internal/pdbcompat/response.go`). The `?page=N` shape is not supported — clients that want pagination set `?limit=N&skip=M` instead |
| `skip` | List | Offset for pagination. Negative values are ignored |
| `depth` | Detail | Edge expansion depth, clamped to `0`–`4` (the range upstream accepts, `serializers.py:802-826` — `max_depth` returns 3 for lists / 4 for detail, `default_depth` 0 / 2). `0` = flat row (FK fields as IDs, no `_set`); `1` = forward FK objects expanded flat with reverse `_set` fields as bare ID lists; `2` = default — `_set` collections as full objects, each first-level nested FK object carrying its own reverse sets as ID lists. The detail default is `2` (`default_depth(is_list=False)`). Non-numeric keeps the default; negatives floor to `0`. `3`/`4` render the depth-2 shape (the deeper sub-level nesting they add upstream is not reproduced). **List endpoints silently drop `?depth=`** — see § Known Divergences |
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
without running SQL. Malformed `__in` values for typed
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

### Soft-delete tombstones

Sync soft-deletes rows by setting `status='deleted'` rather than physically
removing them. The list path applies the upstream `rest.py:707-738` status
matrix as the final predicate via `applyStatusMatrix`:

| Request shape | Admitted statuses |
|---------------|-------------------|
| List, no `?since` | `status='ok'` only |
| List with `?since=N` | `status IN ('ok', 'deleted')`; `pending` additionally admitted on `/api/campus` |
| Single-object GET `/api/<type>/<id>` | `status IN ('ok', 'pending')` — tombstones return `404` |

`?status=<value>` is dropped at the filter layer (the key is absent from
every type's `Fields` map in `internal/pdbcompat/registry.go`) — the
observable outcome is identical to upstream's effective behaviour, where a
caller-supplied `?status=` is overridden by a final unconditional
`filter(status='ok')` (`rest.py:733-738`). This is parity, not a divergence.

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
budget is request-shape, so retrying the identical request returns the same
413. Separately, a process-wide in-flight byte pool admission-controls
concurrent near-budget responses: when simultaneous large dumps would
overflow it, the request is rejected with `503 Service Unavailable` and
`Retry-After: 1` — that one *is* transient, and a retry can succeed. The
budget is enforced only on the
pdbcompat list path; entrest, GraphQL, ConnectRPC, and Web UI have their own
memory stories (see `docs/ARCHITECTURE.md § Response Memory Envelope`).

### Examples

```bash
# All networks with ASN 15169 (Google)
curl "https://peeringdb-plus.fly.dev/api/net?asn=15169"

# Single network by internal ID, with edges expanded (default depth=2)
curl "https://peeringdb-plus.fly.dev/api/net/20/?depth=2"

# Same network at depth=1: org expands to a flat object, and poc_set /
# netfac_set / netixlan_set come back as bare ID lists rather than objects
curl "https://peeringdb-plus.fly.dev/api/net/20/?depth=1"

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
| `503` | Concurrent in-flight response pool exhausted (transient; `Retry-After: 1`) |

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
message per row. The cursor is the compound `(updated, created, id)` triple;
under the default `(-updated, -created, -id)` order each batch resumes via:

```sql
WHERE (updated < cursor.updated)
   OR (updated = cursor.updated AND created < cursor.created)
   OR (updated = cursor.updated AND created = cursor.created AND id < cursor.id)
```

The keyset carries every sort key, so it matches the three-key ordering
exactly: progress stays monotonic and no row is skipped or repeated even when
many rows share an `updated` timestamp (or an `updated`+`created` pair).

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

## Field-level privacy

PeeringDB Plus mirrors upstream PeeringDB's per-field visibility marker for
the IX-F member list URL: `ixlan.ixf_ixp_member_list_url` is gated by the
sibling string field `ixlan.ixf_ixp_member_list_url_visible`, which carries one
of `Public` / `Users` / `Private` (the schema default is `Private`, and a
NULL/empty or unknown value fails closed to redacted). Anonymous callers (the default `PDBPLUS_PUBLIC_TIER=public`
deployment) receive the value only when `_visible = Public`; for `Users` or
`Private` the value is omitted across all five surfaces while the `_visible`
companion field is **still emitted** (upstream parity).

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
  "version": "<injected build version, e.g. v1.22.0>",
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
| GraphQL query complexity | `POST /graphql` | `1,000,000` (weighted) | `FixedComplexityLimit(graph.ComplexityLimit)` |
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
(`peeringdb/peeringdb`) at the `/api/` surface. The remaining divergences are
listed below, each with an upstream citation and a guarding test. The
single-object `?depth=0/1/2` response shape — including reverse `_set` ID
lists, `net_set` through-relations, back-reference stripping, second-level
nested ID-list sets, and `campus:null` — was brought to full parity in v1.20.5
and verified against live `www.peeringdb.com/api` payloads (2026-06-08); see
`internal/pdbcompat/depth_test.go`.

| Request | Upstream behaviour | peeringdb-plus behaviour | Rationale | Since |
|---------|-------------------|-------------------------|-----------|-------|
| `?depth=` on list endpoints; `?depth=3`/`4` on detail | Lists accept `?depth=` capped at `API_DEPTH_ROW_LIMIT=250` (`rest.py:472` default, enforced `rest.py:755-758`); detail expands a third sub-level at depth 3–4. | List `?depth=` is silently dropped (`slog.DebugContext` paper trail; `opts.Depth` never threaded into list closures). On detail, depths `0`/`1`/`2` match upstream exactly; `3`/`4` are accepted and clamped, rendering the depth-2 shape — the third sub-level is not reproduced. | The 256 MB replica response budget cannot absorb depth-expanded list rows; depth>2 sub-nesting is data almost no client reads three levels deep. Locked by `TestParity_Limit/depth_on_list_silently_dropped_DIVERGENCE` and `TestDepth_DepthOne/depth_clamped_to_0_4`. | v1.16 (list) · v1.20.5 (detail 3–4) |
| `?depth=1` (and the nested `net.poc_set` at depth=2) `poc_set` ID lists | Upstream lists every POC id in the ID list regardless of visibility, filtering non-`Public` POCs only when they are expanded to objects at depth=2. | The row-level `poc.visible` privacy policy applies uniformly, so non-`Public` POC ids never appear in an anonymous `poc_set` ID list (nor as objects). The mirror is **stricter** than upstream here. | Leaking the ids/existence of non-`Public` contacts to anonymous callers would contradict the load-bearing `poc.visible` policy (see § Field-level privacy). Intentional. Locked by `TestDepth_PocSetPrivacy_DIVERGENCE`. | v1.20.5 |
| `?status=deleted&since=N` for a row hard-deleted by sync before v1.16 | Returns the tombstone with its deletion timestamp. | Returns empty; tombstone population began at the first post-v1.16 sync, so anything hard-deleted earlier is gone. Rows deleted from v1.16 on are visible via the `?since=N` window. | No retroactive reconstruction is possible — the public API exposes no historical state and we did not persist pre-v1.16 deletions. Locked by `TestParity_Status/list_since_admits_deleted_excludes_pending_noncampus`. | v1.16 |
| `?a__b__c__d=X` — 3+ `__`-separated relation segments (incl. `fac?ixlan__ix__fac_count__gt=0`) | Upstream walks arbitrary Django ORM relation chains, bounded only by the query planner. | Silently ignored (HTTP 200, unfiltered) with one aggregated `slog.DebugContext` + a `pdbplus.filter.unknown_fields` span attribute. Keys with 1 or 2 segments resolve via the allowlist / ent-edge introspection. | DoS ceiling: 3+-hop joins in SQLite trigger super-linear scans the 256 MB replica envelope cannot absorb; the 2-hop cap trades limitless traversal for a predictable cost. Locked by `TestParity_Traversal/DIVERGENCE_fac_ixlan_ix_fac_count_silent_ignore`. | v1.16 |
| `?<field>__contains=`/`__startswith=` with a non-ASCII value, in the ≤1 sync interval after a fresh deploy | Folds via `unidecode.unidecode(v)` at query time (`rest.py:585`), so it works immediately. | Folding uses a sync-populated `<field>_fold` shadow column, so a one-time ASCII-only window exists between deploy and the first sync (≤1h default) during which non-ASCII queries return no match. ASCII queries work throughout; the next sync's `OnConflict().UpdateNewValues()` closes the window with no backfill. | Shadow columns give SQLite one indexable comparison path (benchstat within ±1% of the direct path) and stay off the GraphQL/REST/proto wire (`entgql.Skip` / `entrest.WithSkip`). Locked by `TestParity_Unicode_FoldWindow_DIVERGENCE`. | v1.16 |
| `GET /api/as_set` (and its entry in the `/api/` index) | Upstream exposes a network-derived AS-SET lookup — `{"data":[{"<asn>":"<irr_as_set>",...}],"meta":{}}` via `ASSetSerializer(NetworkSerializer)` — and lists `as_set` as a 14th endpoint in the `/api/` index. | Not mirrored: `GET /api/as_set` is unrouted, and the `/api/` index lists only the 13 served types (its envelope + absolute-URL shape otherwise matches upstream as of v1.20.6). | The endpoint is an unbounded bulk `asn → irr_as_set` dump across every network, outside the 13-type mirror scope; the same `irr_as_set` is available per-network via `GET /api/net`. Locked by `TestIndex`. | v1.20.6 |
| Any `/api/` error response (4xx/5xx) | `{"meta":{"error":"<detail>"},"data":[]}` with `Content-Type: application/json` — `renderers.py:107-113` pops `detail` into `meta["error"]`. | RFC 9457 `application/problem+json` (`type`/`title`/`status`/`detail`); no top-level `meta` key at all (`internal/pdbcompat/response.go` `WriteProblem`). Clients parsing `meta.error` on errors must adapt. | A standards-based, machine-readable error shape shared with the other API surfaces (entrest emits problem+json too) beats the bespoke upstream envelope; success responses keep full envelope parity. Locked by `TestParity_Limit/DIVERGENCE_error_envelope_problem_json_not_meta_error`. | v1.1 (registered 2026-06-10) |

## Validation Notes

Future conformance auditors reading third-party gotchas documentation
(notably pdbfe's upstream-behaviour claims) against the PeeringDB Plus
codebase may encounter assertions about upstream behaviour that turn
out to be wrong. This section documents 6 such invalid claims — 5 from
the v1.16 audit and one (`org_flags`) from the
2026-05-30 audit — each with a pinned `peeringdb/peeringdb@<sha>`
reference so the authoritative upstream source can be re-read without
re-research. All 6 were re-confirmed against commit
`peeringdb/peeringdb@545c58a44af5273e9ddc844b6389e130a023e5ab`
(PeeringDB 2.80.1), the parity anchor as of 2026-06-30.

| Claim | Verdict | Upstream truth | Our implementation |
|-------|---------|----------------|--------------------|
| `net?country=NL` is a valid filter key | **WRONG** | `country` lives on `org`, not `net`. See `peeringdb/peeringdb@545c58a44af5273e9ddc844b6389e130a023e5ab:src/peeringdb_server/serializers.py:2954` — `NetworkSerializer.prepare_query` has no `country` key, and `django-peeringdb/src/django_peeringdb/models/abstract.py`'s Network model has no country field. Callers who want `net` filtered by country must traverse through `org` (e.g. `net?org__country=NL`). | Filter key silently ignored via the unknown-field silent-ignore mechanism — no row-level match, response unfiltered. OTel span attribute `pdbplus.filter.unknown_fields` records the dropped key. Parity-locked by `TestParity_Traversal/unknown_field_silently_ignored_with_otel_attr`. |
| `?limit=0` returns a count-only envelope | **WRONG** | `limit=0` means unlimited. See `peeringdb/peeringdb@545c58a44af5273e9ddc844b6389e130a023e5ab:src/peeringdb_server/rest.py:504` (`limit` defaults to `0`) + `:744-748` (`if limit > 0: qset[skip:skip+limit] else: qset[skip:]` — a non-positive `limit` applies no SQL `LIMIT`). There is no count-only semantic upstream. Callers wanting a count read the length of the returned `data` array (`meta` is the empty `{}` envelope — there is no top-level count field). | Unbounded response (ent v0.14.6 `.Limit(0)` = unlimited, gated by sqlgraph `graph.go:1086 if q.Limit != 0`) plus the memory budget (`PDBPLUS_RESPONSE_MEMORY_LIMIT`, default 128 MiB) with RFC 9457 413 on over-budget. Parity-locked by `TestParity_Limit/bare_url_and_zero_both_return_all_rows` and `TestParity_Limit/zero_over_budget_returns_413_problem_json`. |
| Default list ordering is `id ASC` | **WRONG** | Default ordering is `(-updated, -created)` via the `django-handleref` base Meta. See `peeringdb/peeringdb@545c58a44af5273e9ddc844b6389e130a023e5ab` + upstream dep `django-handleref:src/django_handleref/models.py:95-101` (`class Meta: ordering = ('-updated', '-created')`). Every PeeringDB model inherits from this base, so the default applies across all 13 entity types. | pdbcompat `/api/*`, entrest `/rest/v1/*`, ConnectRPC list RPCs, and GraphQL list queries all use the compound `(-updated, -created, -id)` order (the trailing `-id` is a tie-breaker to ensure stable cursor pagination). Single-object lookups and nested `_set` fields retain their default ordering. Parity-locked by `TestParity_Ordering/default_list_order_updated_desc`, `tiebreak_by_created_desc`, and `tiebreak_by_id_desc`. |
| Unicode folding uses MySQL collation (`utf8_general_ci` or similar) | **WRONG** | Folding is Python-side via `unidecode.unidecode(v)` at query time. See `peeringdb/peeringdb@545c58a44af5273e9ddc844b6389e130a023e5ab:src/peeringdb_server/rest.py:585` — the call happens in the Python filter construction layer before any SQL is emitted, so the database collation is irrelevant. | peeringdb-plus uses shadow `<field>_fold` columns populated at sync time via `internal/unifold.Fold` (`golang.org/x/text/unicode/norm` NFKD decomposition + a hand-rolled ligature map for `ß`/`æ`/`œ`/`ø`/`ł`/`þ`/`đ`/`ð`/dotless `ı`). `__contains` / `__startswith` route to `<field>_fold LIKE ?` with `unifold.Fold(query)` on the RHS. Not byte-compatible with Python `unidecode` for every input (e.g. the two libraries handle rare CJK edge cases differently); any specific gap that surfaces will be logged as a new § Known Divergences row. Parity-locked by `TestParity_Unicode/net_name_contains_diacritic_matches_ascii`, `fac_city_cjk_roundtrip`, and `combining_mark_NFKD_equivalent`. |
| Filter surface is a DRF `filterset_class` per ViewSet | **WRONG** | Filter surface is a per-serializer `prepare_query(...)` method plus an auto-`queryable_relations()` mechanism with a `FILTER_EXCLUDE` denylist. See `peeringdb/peeringdb@545c58a44af5273e9ddc844b6389e130a023e5ab:src/peeringdb_server/serializers.py:756` (`queryable_relations()`) and `:130-159` (`FILTER_EXCLUDE`). No `django_filters.FilterSet` subclass exists anywhere in the upstream codebase. | Path A = `pdbcompat.WithPrepareQueryAllow` ent-schema annotations → `allowlist_gen.go` `Allowlists` map (13 entries verbatim from upstream); Path B = ent edge introspection via the generated `Edges` map. The `WithFilterExcludeFromTraversal` edge annotation mirrors upstream's `FILTER_EXCLUDE` — currently empty across all 13 schemas (every FK edge exposed in v1.16). Parity-locked by `TestParity_Traversal/path_a_1hop_org_name` and `path_b_1hop_org_city`. |
| `org_flags` is a valid filter param on `/api/org` | **WRONG** | No `org_flags` (or `flags`) filter key or column exists upstream. Grepping `peeringdb/peeringdb@545c58a44af5273e9ddc844b6389e130a023e5ab:src/peeringdb_server/{rest.py,serializers.py,models.py}` returns zero matches, and the `django-peeringdb` Organization model has no `flags` field — `OrganizationSerializer.prepare_query` exposes no such key. This is not a real upstream parameter. | Filter key silently ignored via the unknown-field silent-ignore mechanism — response unfiltered, `pdbplus.filter.unknown_fields` records the dropped key. NOT a § Known Divergences entry: there is no upstream behaviour to diverge from. Covered by the generic `TestParity_Traversal/unknown_field_silently_ignored_with_otel_attr`. |

Quarterly re-validation against upstream is a manual review against
the pinned commit above — it does not block merges. Drift that
invalidates a Validation Note row should be surfaced as a GitHub
issue and reviewed against the parity test suite; if upstream has
changed semantics, update the row here and flip or retain the
matching parity assertion as a new § Known Divergences row.

## Cross-entity traversal

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
  `sync.Once`, no init-order coupling.

The resolution order is implemented in
`internal/pdbcompat/filter.go` `buildTraversalPredicate`: Path A
first; on a soft miss (allowlist hit but downstream introspection
unavailable) the parser falls through to Path B rather than
suppressing the key. `parseFieldOp` returns
the 3-tuple `(relationSegments, finalField, op)` so the same machinery
serves 1-hop and 2-hop paths with a single split.

### Supported shapes per entity (1-hop + 2-hop)

All 13 entity types support Path A 1-hop shapes via
`?<fk>__<field>=X`. The 2-hop subset tracks upstream
`pdb_api_test.py`:

| Query | Hops | Path | Upstream citation |
|-------|------|------|-------------------|
| `?org__name=X` (net, fac, ix, carrier, campus) | 1 | A | each serializer's `prepare_query` classmethod (e.g. `serializers.py:1823` `OrganizationSerializer`, `:2954` `NetworkSerializer`) |
| `?net__asn=X` (netfac, netixlan, poc) | 1 | A | (same allowlist block) |
| `?ix__name=X` (ixfac, ixlan, ixpfx) | 1 | A | (same allowlist block) |
| `?fac__name=X` (netfac, ixfac, carrierfac) | 1 | A | (same allowlist block) |
| `?ixlan__ix__fac_count__gt=0` (fac) | 2 | A | `pdb_api_test.py:2340, 2348` |
| `?ixlan__ix__id=N` (fac) | 2 | A | `pdb_api_test.py:5081` |
| `?<fk>__<field>=X` for any non-excluded edge | 1 | B | `serializers.py:756` (`queryable_relations()`) |

1-hop Path B fallthrough means the explicit Path A allowlists are
**additive, not restrictive**: a key that is not in Path A but is a
valid ent FK edge still resolves via Path B. The exclusion list
(below) is the only way to block a Path B key.

### FILTER_EXCLUDE list

The `pdbcompat.WithFilterExcludeFromTraversal()` ent edge annotation
hides specific edges from Path B traversal. Mirrors
upstream `serializers.py:130-159`.

| Entity | Edge | Reason |
|--------|------|--------|
| _(none — all FK edges exposed in v1.16)_ | _—_ | Initial release. Field-level privacy (`ixlan.ixf_ixp_member_list_url_visible`) operates at the **serializer layer**, not the edge layer, so no edge exclusion is required for the v1.16 surface. Future OAuth-gated relations will populate this table. |

### 2-hop cap

Filter keys with more than 2 `__`-separated relation segments are
silently ignored. Examples:

- `?org__name=X` — 1 hop, resolves via Path A (every primary entity).
- `?ixlan__ix__fac_count__gt=0` — 2 hops, resolves via Path A
  (upstream `pdb_api_test.py:2340` assertion).
- `?ixlan__ix__org__name=X` — 3 hops, SILENTLY IGNORED (HTTP 200,
  result set is unfiltered).

Upstream PeeringDB has no hard cap but is bound by Django ORM's query
planner. We trade limitless traversal for a predictable cost ceiling
of `<50ms/op @ 10k rows`, checked locally via the build-tagged
gate `internal/pdbcompat/bench_traversal_test.go` (`go test -tags=bench`,
without `-race`); CI does not run it. If a legitimate 3-hop use case
emerges, raise the cap together with a fresh benchstat run and a docs
update here.

### Unknown-field diagnostics

When a filter key fails Path A, Path B, and the 2-hop cap check,
the following observability signals fire:

- `slog.DebugContext(ctx, "pdbcompat: unknown filter fields silently
  ignored", slog.String("endpoint", ...),
  slog.String("type", ...), slog.Any("unknown_fields", ...))`
- OTel span attribute `pdbplus.filter.unknown_fields` (CSV of all
  unknown keys in the request)

Both are DEBUG-level; INFO and higher are untouched so that naive
clients probing field names don't flood structured logs. To surface
these in production, set `PDBPLUS_LOG_LEVEL=DEBUG` or query the span
attribute in Grafana/Tempo.

