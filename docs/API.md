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
Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging -> Readiness ->
SecurityHeaders -> CSP -> Caching -> Gzip -> mux
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
| `limit` | List | Page size. Parsed by `ParsePaginationParams` (see `internal/pdbcompat/filter.go`) |
| `skip` | List | Offset for pagination |
| `depth` | Detail | Edge expansion depth. Accepted values: `0` (default, flat object) and `2` (embed related `_set` collections). Any other value is silently ignored |
| `fields` | Both | Comma-separated projection — only the listed JSON keys are returned after retrieval |
| `since` | List | Only return rows with `updated` greater than the given timestamp (Unix seconds). Invalid input returns `400` |
| `{field}`, `{field}__{op}` | List | Arbitrary field filter. Operator suffixes: `__contains`, `__startswith`, `__in`, `__lt`, `__lte`, `__gt`, `__gte`. Typed against the field; invalid types (e.g. `asn__contains`) return `400` |

Unknown query parameters that are not in the reserved set (`limit`, `skip`,
`depth`, `since`, `q`, `fields`) are treated as field filters and validated
against the type's schema.

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

# Networks updated since 2024-01-01 (Unix seconds)
curl "https://peeringdb-plus.fly.dev/api/net?since=1704067200"
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
| `400` | Invalid filter operator, malformed `since`, non-integer ID, filter type mismatch |
| `404` | Unknown `{type}` or missing `{id}` |
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

`Stream{Type}` RPCs use **batched keyset pagination** under the hood
(`StreamEntities` in `internal/grpcserver/generic.go`), fetching
`streamBatchSize` (500) rows per database round-trip and emitting one proto
message per row.

| Field | Semantics |
|-------|-----------|
| `since_id` | Resume cursor — only rows with `id > since_id` are emitted |
| `updated_since` | Timestamp filter — only rows with `updated > updated_since` are emitted |

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
boundaries intentionally. This section is the Phase 68 seed; Phase 72's
upstream-parity regression work will expand it into a full divergence
registry.

| Request | Upstream behaviour | peeringdb-plus behaviour | Rationale | Since |
|---------|-------------------|-------------------------|-----------|-------|
| `GET /api/<type>?status=<value>` (list, no `?since`) | Upstream `rest.py:700-712` builds `allowed_status` from the caller's `?status=` parameter (or a default set), then applies a final unconditional `filter(status='ok')` on `rest.py:725`. The caller-supplied value is effectively overridden by the final filter — only `status=ok` rows ever reach the response. | pdbcompat drops `?status=` at the filter layer (the `status` entry is removed from all 13 `Fields` maps in `internal/pdbcompat/registry.go`) so the predicate never reaches ent. The observable outcome is identical to upstream; the implementation is explicit rather than implicit. | Upstream's double-filter is a no-op by design. We model the intent rather than the mechanism, which makes the D-07 semantic greppable in one place. | v1.16 (Phase 68) |
| `GET /api/<type>?status=deleted&since=N` for a row hard-deleted by sync cycles before v1.16 | Upstream returns the tombstone row with its deletion timestamp (upstream has always soft-deleted). | peeringdb-plus returns empty for such rows. Tombstone population begins at the first post-upgrade sync cycle; anything hard-deleted before v1.16 shipped is gone forever. Rows deleted from v1.16 onwards are visible via both `?since=N` windows and pk lookup (pk admits `status IN (ok, pending)`; tombstones are reachable only via the `since` window). | No retroactive reconstruction is possible — PeeringDB's public API does not expose historical state, and we did not persist deleted rows prior to the Phase 68 soft-delete flip (D-03). Documented intentional one-time gap. | v1.16 (Phase 68) |
| `?<field>__contains=<non-ASCII>` / `?<field>__startswith=<non-ASCII>` against searchable text fields on `network`, `facility`, `ix`, `organization`, `campus`, `carrier` (16 fields total — `name`, `aka`, `name_long`, `city` per entity) | Upstream applies `unidecode.unidecode(v)` to BOTH the query value and the column at query time (`rest.py:576`), producing diacritic-insensitive matches in a single SQL pass. | peeringdb-plus precomputes the folded value into a sibling `<field>_fold` shadow column at sync time (via `internal/unifold.Fold` — NFKD normalisation + a small ligature map for `ß`/`æ`/`ø`/`ł`/`þ`/`đ`), then routes `__contains` / `__startswith` to `<field>_fold LIKE ?` with `unifold.Fold(query)` on the RHS. The end-state semantic match is identical, but it is staged differently: a brief one-time ASCII-only window exists between v1.16 deploy and the first post-deploy sync cycle (≤1h with default `PDBPLUS_SYNC_INTERVAL=1h`) during which rows have `<field>_fold = ''` and return no match for non-ASCII queries. ASCII queries continue to work via the non-folded columns throughout the window. No manual backfill is required — the next sync cycle's `OnConflict().UpdateNewValues()` path rewrites every row's `_fold` columns. | Shadow columns let SQLite use a single indexable comparison path (no per-query `unidecode` call), and Phase 69 benchstat (n=6, 10k rows) shows the shadow path within ±1% of the direct path so the trade-off is invisible at production scale. The folded columns carry `entgql.Skip(SkipAll)` + `entrest.WithSkip(true)` annotations and are never exposed on the GraphQL / REST / proto wire surfaces — they are server-side plumbing only. | v1.16 (Phase 69) |
| `GET /api/<type>?a__b__c__d=X` (3+ `__`-separated relation segments) | Upstream walks arbitrary Django ORM chains — no hard depth cap (bounded only by the Django ORM query planner). | peeringdb-plus silently ignores the filter (HTTP 200, unfiltered) per Phase 70 D-04. One aggregated `slog.DebugContext("pdbcompat: unknown filter fields silently ignored (Phase 70 TRAVERSAL-04)", ...)` plus OTel span attribute `pdbplus.filter.unknown_fields` record the dropped key. Keys with exactly 1 or 2 relation segments resolve normally via Path A (explicit allowlist) or Path B (ent edge introspection). | DoS protection: 3+-hop joins in SQLite can trigger super-linear query plan scans at scale, and the replica memory envelope (256 MB post-Phase-65) cannot absorb unbounded Cartesian-product row counts. The 2-hop cap trades limitless traversal for a predictable cost ceiling gated in CI by `internal/pdbcompat/bench_traversal_test.go` (`<50ms/op @ 10k rows`). | v1.16 (Phase 70) |
| `GET /api/fac?campus__name=X` and any `<entity>?campus__<field>=X` via Path A or Path B (DEFER-70-06-01) | Upstream resolves through the `campus` FK edge and returns matching rows. | peeringdb-plus returns `500 SQL logic error: no such table: campus (1)` because `cmd/pdb-compat-allowlist` emits `TargetTable: "campus"` instead of `"campuses"` when `entc.LoadGraph` walks the Campus ent type — the inflection patch that the ent runtime codegen applies via `fixCampusInflection` is not applied on the codegen-tool path. Outgoing edges FROM Campus are unaffected (the table name is resolved on the non-Campus peer). | Known one-time gap documented in `.planning/phases/70-cross-entity-traversal/deferred-items.md`. Fix is scheduled as a follow-up plan — preferred approach is `entsql.Annotation{Table: "campuses"}` on `ent/schema/campus.go` so the pluralised name is explicit and survives every codegen path. | v1.16 (Phase 70) |

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

### DEFER-70-06-01 — campus edge codegen bug (v1.16 gap)

`cmd/pdb-compat-allowlist` emits `TargetTable: "campus"` instead of
`"campuses"` for every edge targeting the Campus entity, because
`entc.LoadGraph` does not apply the `fixCampusInflection` patch used
by the ent runtime codegen. Any `<entity>?campus__<field>=X` query
hitting Path A or Path B returns
`500 SQL logic error: no such table: campus (1)`. Outgoing edges FROM
Campus (`?org__campuses__name=X` would walk Org→Campuses) are correct
because the `.Table()` call resolves on the non-Campus peer.

**Workaround:** Use the direct Campus list endpoint
(`GET /api/campus?name=X`) — single-entity queries are unaffected.

**Fix (scheduled as a follow-up plan):** Add
`entsql.Annotation{Table: "campuses"}` to `ent/schema/campus.go` so
the pluralised name is explicit in the schema and survives every
codegen path. This mirrors v1.15 Phase 63's preference for explicit
schema annotations over inflection heuristics.

