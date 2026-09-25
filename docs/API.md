# API Reference

PeeringDB Plus exposes the mirrored PeeringDB dataset through
**six coexisting API surfaces** served from the same process on the same port,
plus a small set of infrastructure endpoints for health, on-demand sync, and
service discovery.
This document is the comprehensive reference;
see `README.md` for a one-page overview and `docs/ARCHITECTURE.md`
for the rationale behind each surface.

All routes are registered in `cmd/peeringdb-plus/main.go`
and pass through the production middleware chain:

```text
Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging -> PrivacyTier ->
Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip -> RouteTag -> mux
```

The server speaks HTTP/1.1 and h2c
(HTTP/2 cleartext)
on the same listener so that Connect, gRPC,
and gRPC-Web clients can use the same base URL as browser and CLI clients.

## Authentication

Most endpoints are **unauthenticated and read-only** —
they expose the same public data that PeeringDB itself publishes.

| Endpoint | Authentication |
|----------|----------------|
| Read endpoints (Web UI, GraphQL, REST, `/api/`, ConnectRPC, MCP) | None |
| `POST /sync` | `X-Sync-Token` header must match `PDBPLUS_SYNC_TOKEN` (constant-time compare) |
| Upstream fetch from `api.peeringdb.com` | Optional — set `PDBPLUS_PEERINGDB_API_KEY` to use an authenticated client with higher rate limits |

If `PDBPLUS_SYNC_TOKEN` is empty at startup the sync endpoint logs a warning
and rejects every request as `401 unauthorized` —
there is no "accept anything" mode.
Replica instances reject the request with a `fly-replay` header
that routes the request to the primary region on Fly.io,
or return `503 not primary` when running outside Fly.io.

## Endpoints overview

| Method | Path | Surface | Description |
|--------|------|---------|-------------|
| `GET` | `/` | Root | Content-negotiated service discovery (terminal / browser / JSON) |
| `GET` | `/healthz` | Health | Liveness probe (always `200`) |
| `GET` | `/readyz` | Health | Readiness probe (checks the DB and the age of the last successful sync) |
| `POST` | `/sync` | Admin | On-demand sync trigger (primary only, token-gated) |
| `GET` | `/favicon.ico` | Static | Favicon served from embedded `internal/web/static/` |
| `GET` | `/robots.txt` | Static | Crawler rules. Blocks `/ui/fragment/` |
| `GET` | `/static/*` | Static | Embedded UI assets (CSS, JS, images) |
| `GET` | `/ui/` | Web UI | Home / search page |
| `GET` | `/ui/asn/{asn}` | Web UI | Network detail by ASN |
| `GET` | `/ui/ix/{id}` | Web UI | Internet exchange detail |
| `GET` | `/ui/fac/{id}` | Web UI | Facility detail |
| `GET` | `/ui/org/{id}` | Web UI | Organization detail |
| `GET` | `/ui/campus/{id}` | Web UI | Campus detail |
| `GET` | `/ui/carrier/{id}` | Web UI | Carrier detail |
| `GET` | `/ui/search` | Web UI | Search results (`?q=`). With `?type=`, the results of one type with `offset` paging |
| `GET` | `/ui/about` | Web UI | Application version, optional serving region, privacy mode, and sync freshness |
| `GET` | `/ui/compare` | Web UI | ASN comparison form |
| `GET` | `/ui/compare/{asn1}` | Web UI | Comparison form with the first ASN filled in |
| `GET` | `/ui/compare/{asn1}/{asn2}` | Web UI | ASN comparison results |
| `GET` | `/ui/completions/bash` | Web UI | Bash completion script |
| `GET` | `/ui/completions/zsh` | Web UI | Zsh completion script |
| `GET` | `/ui/completions/search` | Web UI | Identifiers for shell completion |
| `GET` | `/ui/fragment/{type}/{id}/{relation}` | Web UI | htmx fragments for detail pages. Not a stable interface |
| `GET` | `/graphql` | GraphQL | GraphiQL playground (HTML) |
| `POST` | `/graphql` | GraphQL | Query execution |
| `GET` | `/rest/v1/openapi.json` | REST | OpenAPI 3 specification |
| `GET` | `/rest/v1/{collection}` | REST | List resources (entrest-generated) |
| `GET` | `/rest/v1/{collection}/{id}` | REST | Get single resource (entrest-generated) |
| `GET` | `/rest/v1/{collection}/{id}/{edge}` | REST | Related resources of one resource (entrest-generated) |
| `GET` | `/api/` | PDB Compat | Index of available object types (upstream `{"data":[{type:absolute-url}],"meta":{}}` shape) |
| `GET` | `/api/{type}` | PDB Compat | List endpoint with PeeringDB-compatible filters |
| `GET` | `/api/{type}/{id}` | PDB Compat | Single object (wrapped in `data: []`) |
| `POST` | `/peeringdb.v1.{Service}/{Method}` | ConnectRPC | One of 13 services × {Get, List, Stream} methods |
| `POST` | `/grpc.reflection.v1.ServerReflection/*` | ConnectRPC | gRPC reflection (v1) |
| `POST` | `/grpc.reflection.v1alpha.ServerReflection/*` | ConnectRPC | gRPC reflection (v1alpha) |
| `POST` | `/grpc.health.v1.Health/*` | ConnectRPC | gRPC health check |
| `POST` | `/mcp` | MCP | Stateless Streamable HTTP requests |
| `GET` / `HEAD` | `/skills/peeringdb-plus/SKILL.md` | Agent Skill | Raw, origin-neutral skill instructions |
| `GET` / `HEAD` | `/skills/peeringdb-plus.zip` | Agent Skill | Archive with origin-aware MCP dependency metadata |
| `GET` / `HEAD` | `/.well-known/mcp/server-card.json` | Agent discovery | MCP endpoint, versions, capabilities, tools, and prompts |
| `GET` / `HEAD` | `/.well-known/agent-skills/index.json` | Agent discovery | Agent Skills v0.2.0 index with a SHA-256 digest |
| `GET` / `HEAD` | `/.well-known/agent-skills/peeringdb-plus/SKILL.md` | Agent Skill | Standard well-known alias for the raw skill |
| `GET` / `HEAD` | `/llms.txt` | Agent discovery | Curated Markdown index of agent and API interfaces |

The 13 entity types mirrored from PeeringDB are: `campus`, `carrier`,
`carrierfac`, `fac`, `ix`, `ixfac`, `ixlan`, `ixpfx`, `net`, `netfac`,
`netixlan`, `org`, `poc`.

The application server has no `/metrics` route.
By default, the process sends metrics through OTLP to the configured collector.
If you set `OTEL_METRICS_EXPORTER=prometheus`,
the OpenTelemetry autoexport library starts a separate listener
that serves `/metrics` on
`OTEL_EXPORTER_PROMETHEUS_HOST`:`OTEL_EXPORTER_PROMETHEUS_PORT`
(default `localhost:9464`).
See [CONFIGURATION.md](CONFIGURATION.md#standard-opentelemetry-variables-autoexport).

### Before the first sync

Until the first sync completes, every route returns
`503 Service Unavailable`, except `/`, `/healthz`, `/readyz`, `/sync`,
`/favicon.ico`, `/static/*` and `/grpc.health.v1.Health/*`.
This includes `/mcp` and the agent discovery files that `GET /` lists.
A browser gets a syncing page.
A terminal client gets text.
Other clients get `{"error":"sync not yet completed"}` as `application/json`.
ConnectRPC and gRPC clients get `UNAVAILABLE`.
To know when the server is ready, poll `/readyz` or the gRPC health service.

## Row status on each surface

The mirror stores rows of every status that upstream sends:
`ok`, `pending`, `deleted`, and `not-operational` on netixlan.

- `/api/` applies the upstream status matrix.
  See § Soft-delete tombstones.
- GraphQL, REST and ConnectRPC apply no status filter.
  Their lists, streams and single-object lookups also return
  `deleted` and `pending` rows.
- To get only live rows from a list, filter on `status`:
  GraphQL `where: {status: "ok"}`, REST `?status.eq=ok`,
  or ConnectRPC `"status": "ok"`.
  On netixlan, use GraphQL `where: {statusIn: ["ok", "not-operational"]}`
  or REST `?status.in=ok&status.in=not-operational`.
  The ConnectRPC `status` filter takes one value,
  so send one request for each status.
- Single-object lookups take no status filter:
  GraphQL `node`, REST `/rest/v1/{collection}/{id}` and ConnectRPC `Get{Type}`.
  Read `status` in the response.
- GraphQL `networkByAsn` returns only `ok` and `pending` networks.
- The Web UI and the MCP tools do not show `deleted` rows.

### netixlan `not-operational` status

PeeringDB 2.83.0 adds the netixlan status `not-operational`.
A connection that was `ok` with `operational=false` now has this status.
Upstream derives `operational` from the status (`status == 'ok'`),
and it treats `ok` and `not-operational` as live statuses
(`models.py:109-122`).

- The Web UI, the ASN comparison and the MCP tools list a
  `not-operational` connection the same as an `ok` one.
  Its speed counts toward the aggregate bandwidth of the network or exchange.
  The Web UI marks it `not operational`; see § IX connection markers.
- GraphQL, REST and ConnectRPC apply no status filter
  (see § Row status on each surface).
  They return the stored `status` and `operational` values unchanged.
  A `status` filter for `ok` does not return these connections.
- For `/api/`, see § Soft-delete tombstones.

## 1. Web UI (`/ui/`)

The Web UI is implemented in `internal/web/` using [templ](https://templ.guide)
for type-safe HTML templates and [htmx](https://htmx.org)
for interactive behavior without a JavaScript build pipeline.
The same URL space can render HTML, ANSI-colored terminal text, plain text,
or JSON depending on client characteristics.

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

Because `curl` and `wget` are detected by User-Agent,
running `curl https://peeringdb-plus.fly.dev/ui/asn/15169` returns
**ANSI-colored text intended for a terminal**.
If you pipe this output to a file, a logger,
or a tool that does not render ANSI codes,
you will see escape sequences like `\x1b[38;5;...`.

Four ways to get clean output:

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
| `GET /ui/search?q=&type=&offset=` | Results of one type: `net`, `ix`, `fac`, `org`, `campus` or `carrier`. Returns 50 rows for each page. A request with `offset` above 0 returns only the next rows as an htmx fragment. An unknown type returns the 404 page |
| `GET /ui/asn/{asn}` | Network detail by ASN (1 .. 2³²−1; values outside the range return `400 Problem+JSON`) |
| `GET /ui/ix/{id}` | Internet exchange detail by numeric ID |
| `GET /ui/fac/{id}` | Facility detail |
| `GET /ui/org/{id}` | Organization detail |
| `GET /ui/campus/{id}` | Campus detail |
| `GET /ui/carrier/{id}` | Carrier detail |
| `GET /ui/about` | Application version, optional serving region, privacy mode, and sync freshness. The region row is omitted outside environments that provide one. Opted out of response caching in `middleware.NewCachingState` because it renders relative time (e.g. "5 minutes ago") |
| `GET /ui/compare` | ASN comparison form. `?asn1=` and `?asn2=` pre-fill the form |
| `GET /ui/compare/{asn1}` | Pre-fills the form with `asn1`, awaits `asn2` |
| `GET /ui/compare/{asn1}/{asn2}` | Comparison results. `?view=shared` (default) shows only IXPs/facilities/campuses where both networks are present; `?view=full` shows the union with shared-flag highlighting. Any other `view` value falls back to the shared view. |
| `GET /ui/completions/bash` | Installable bash completion script |
| `GET /ui/completions/zsh` | Installable zsh completion script |
| `GET /ui/completions/search?q=&type=` | Newline-separated identifiers for the shell completion scripts: the ASN for `net`, and the numeric ID for `ix`, `fac`, `org`, `campus` and `carrier`. Up to 10 for each type. `type` is optional. A `q` shorter than 2 characters returns an empty body |
| `GET /ui/fragment/{type}/{id}/{relation}` | htmx fragments that the detail pages load, for example `/ui/fragment/net/{id}/ixlans`. Not a stable interface. `robots.txt` blocks them |

Unknown `/ui/*` paths render the themed 404 page via `handleNotFound`.

### IX connection markers

The network IX list, the exchange participant list and the ASN comparison
mark a connection as the upstream PeeringDB 2.83.0 network and exchange views do:

| Marker | Shown when |
|--------|------------|
| `not operational` | `status` is `not-operational`, or `status` is `ok` and `operational` is `false` (a row that upstream has not migrated yet) |
| `planned removal <date>` | `meta.planned_status_change.status` is `deleted` |
| `planned activation <date>` | `meta.planned_status_change` is set with any other status |
| `RFC8950` | `meta.rfc8950` is `true` |

HTML shows each marker as a badge with the upstream tooltip.
In the ASN comparison, the badges are in the speed column of each network.
Terminal output shows each marker in brackets, for example `[not operational]`,
at every width.
WHOIS output lists only exchange names and shows no markers.
JSON output (`?format=json`) and the MCP relation and comparison rows
carry the same data as a `Markers` object
(`NotOperational`, `PlannedStatus`, `PlannedDate`, `RFC8950`).
The object is left out when no marker is set.

## 2. GraphQL (`/graphql`)

GraphQL is served by [gqlgen](https://gqlgen.com) wired through
[entgql](https://entgo.io/docs/graphql/).
The handler lives in `internal/graphql/handler.go`.

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
| Page size | 1000 (default 100) | `validatePageSize` and `ValidateOffsetLimit` in `graph/pagination.go` |

### Queries and pagination

- Relay connections, for example
  `networks(first: 10, after: $cursor, where: {...})`.
  Without `first` or `last`, a connection returns the first 100 rows.
  A `first` or `last` value above 1000 returns an error.
  The default order is `id` ascending.
  `campuses`, `carriers`, `facilities`, `internetExchanges`, `networks`
  and `organizations` also accept `orderBy: {field: NAME}`.
- Offset lists, for example
  `networksList(offset: 0, limit: 100, where: {...})`.
  `limit` defaults to 100 and must be 1 to 1000.
  `offset` must be 0 or more.
  These queries set no order, so use a connection for stable paging.
- `networkByAsn(asn: Int!)` returns one network with status `ok` or `pending`,
  or `null`.
- `syncStatus` returns the latest sync record, which can be a running sync:
  `status`, `lastSyncAt`, `durationMs`, `objectCounts` and `errorMessage`.

### Error envelope

Errors are returned in standard GraphQL format with an `extensions.code` field
populated by `classifyError` in `internal/graphql/handler.go`:

| Code | Trigger |
|------|---------|
| `NOT_FOUND` | `ent.IsNotFound(err)` |
| `VALIDATION_ERROR` | `ent.IsValidationError(err)` |
| `CONSTRAINT_ERROR` | `ent.IsConstraintError(err)` |
| `INTERNAL_ERROR` | Any other error. This includes a query that does not parse or validate, a query over the complexity or depth limit, and a page-size argument out of range (for example `first: 5000` or `limit: 0`) |

An error from a resolver includes `path`, which points to the field.
An error for the full request, for example a parse error or a complexity-limit
error, has no `path`.

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

Schema browsing is easiest through the GraphiQL playground.
The schema comes from `ent/schema/` (`graph/schema.graphqls`)
and the hand-written `graph/custom.graphql`.

`Network.meta` and `NetworkIxLan.meta` carry the PeeringDB metadata
document (added upstream in 2.83.0) as the `Map` scalar.
The key set is open: upstream can add keys,
and the mirror stores the document as is, with no schema change.
The field is `null` when no document is stored for the row.

## 3. REST (`/rest/v1/`)

The REST surface is generated by [entrest](https://github.com/lrstanley/entrest)
directly from the ent schemas with read-only operations only (`OperationRead` +
`OperationList`).

| Path | Description |
|------|-------------|
| `GET /rest/v1/openapi.json` | OpenAPI 3 specification for the full REST surface. Regenerated as part of `go generate ./...` |
| `GET /rest/v1/{collection}` | List resources |
| `GET /rest/v1/{collection}/{id}` | Get one resource |
| `GET /rest/v1/{collection}/{id}/{edge}` | List or get the related resources, for example `/rest/v1/networks/{id}/network-ix-lans` |

A list takes `page` (default 1) and `per_page` (default 10, maximum 100).
A `per_page` value above 100 returns `400`.
`sort` names the field (default `updated`),
and `order` is `asc` or `desc` (default `desc`).
The default sort adds `created` and `id` as tiebreakers.
A filter has the form `<field>.<op>=<value>`,
for example `asn.eq=13335`, `name.ihas=cloud` or `status.eq=ok`.
For an operator that takes a list, repeat the key:
`status.in=ok&status.in=pending`.
The response is `{"page", "total_count", "last_page", "is_last_page", "content"}`.
Each item includes an `edges` object with its related rows.
For each edge that holds a list, the server loads at most 1000 rows
for the full response, newest `updated` first.
In a list response, all items share this limit,
so an item can show only part of its related rows.
To get every related row, use the edge route, which pages like a list.
The collection paths and the filters come from entrest annotations
in `ent/schema/`.
The OpenAPI spec lists every filter.

`meta` on `networks` and `network-ix-lans` is the PeeringDB metadata
document, an open JSON object (added upstream in 2.83.0).
A row without a stored document returns `{}`, the same as upstream.
`meta` is not filterable on this surface.

### Error format

Non-2xx responses are rewritten to
[RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html) by
`RESTError` in `internal/middleware/rest_error.go`.
The response `Content-Type` is `application/problem+json`.
The body always has `type` (`about:blank`), `title`, `status` and `instance`.
For a `4xx` response, `detail` gives the entrest error message,
for example `bad request: per_page 0 is out of bounds, must be >= 1`.
A `5xx` response has no `detail`,
so database error text does not reach the client.

## 4. PeeringDB Compatibility API (`/api/`)

This surface, in `internal/pdbcompat/`,
serves the read operations of the PeeringDB REST API.
It serves only `GET` and `HEAD` requests.
Other methods get `405`.
The URL structure, the success envelope, the filter operators
and the single object in a `data` array match upstream.
A client that only reads can switch to PeeringDB Plus
with a change of base URL.
Error bodies and some filters differ.
See § Known Divergences.

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
| `q` | List | Case-insensitive substring search across the type's search fields. For `/api/net`, an ASN literal (e.g. `8075` or `AS8075`) also matches `net.asn` exactly in addition to the text fields. This is a peeringdb-plus **extension**. Upstream ignores `?q=` on `/api` list endpoints. Unlike the field filters, `?q=` does not ignore diacritics. See § Known Divergences |
| `limit` | List | Maximum rows in response. **Default unlimited** when absent — matches upstream 2.83.0 `rest.py:516` (`limit` defaults to `0`) + `rest.py:757-760` (no slice when `limit=0`). Bare `/api/<type>` URLs return ALL rows from the filtered queryset; the response is gated only by the response memory budget (see below). Explicit `limit=N`: positive `N` is honored with no upper cap, as upstream (`rest.py:757-758`); `limit=0` is the explicit "unlimited" sentinel. A non-numeric `limit` returns `400`, as upstream (`rest.py:515-518`). A negative `limit` also returns `400`; upstream serves it as unlimited (see § Known Divergences). Constant: `DefaultLimit=0` (`internal/pdbcompat/response.go`). The `?page=N` shape is not supported — clients that want pagination set `?limit=N&skip=M` instead |
| `skip` | List | Offset for pagination. A non-numeric or negative `skip` returns `400`, as upstream: 2.83.0 `rest.py:511-514` rejects a non-numeric value, and Django rejects a negative slice with `ValueError`, which `list()` turns into a `400` (`rest.py:824-827`) |
| `depth` | Detail | Edge expansion depth, clamped to `0`–`4` (the range upstream accepts, 2.83.0 `serializers.py:1016-1039`: `max_depth` returns 3 for lists / 4 for detail, `default_depth` 0 / 2). `0` = flat row (FK fields as IDs, no `_set`); `1` = forward FK objects expanded flat with reverse `_set` fields as bare ID lists; `2` = default, with `_set` collections as full objects, each first-level nested FK object carrying its own reverse sets as ID lists. The detail default is `2` (`default_depth(is_list=False)`). Non-numeric keeps the default; negatives floor to `0`. `3`/`4` render the depth-2 shape (the deeper sub-level nesting they add upstream is not reproduced). `_set` fields list only live children (see "Soft-delete tombstones" below). The sets are in ascending id order, with these exceptions: `net.netfac_set`, `ix.fac_set` and `carrier.carrierfac_set` are in facility-id order, as upstream sends them, and `ixlan.net_set` keeps the order of the netixlan rows. **List endpoints silently drop `?depth=`**. See § Known Divergences |
| `fields` | Both | Comma-separated list of keys to keep. The response always keeps `id`. It also keeps every key that ends in `_set` (on `net`, this includes `irr_as_set`) and every nested object that has an `id`, so a detail response at the default depth still includes its sets. Unknown names are ignored |
| `since` | List | The value is Unix seconds as an integer. The list holds the rows with `updated` at or after that second, in `updated` order, then `id` order (see § List order). It also admits `deleted` rows, and `pending` rows on `/api/campus` (see § Soft-delete tombstones). `since=0` or a negative value is ignored. A value that is not an integer returns `400`. Upstream stores `updated` with microseconds and compares it with `N.000000` (2.83.0 `rest.py:736-744`). It shows the value truncated to the second (`serializers.py:1920-1924`), so it also returns almost every row shown as `updated=N`. The mirror stores only the second and includes it |
| `{field}`, `{field}__{op}` | List | Arbitrary field filter. Operator suffixes: `__contains`, `__icontains`, `__startswith`, `__istartswith`, `__iexact`, `__in`, `__lt`, `__lte`, `__gt`, `__gte`. `contains` and `startswith` are coerced to their case-insensitive variants per upstream 2.83.0 `rest.py:657-662`. Upstream ignores a key with the `__iexact`, `__icontains` or `__istartswith` suffix; the mirror applies them (see § Known Divergences). Typed against the field; invalid types (e.g. `asn__contains`) return `400`. A key that names a forward FK by its upstream model name filters the FK column: `?org=1` is the same filter as `?org_id=1`. `net` and `network` are names for `net_id`, and `fac` and `facility` are names for `fac_id` (`?network__in=1,2`, `?facility_id=2`). The operators compare the FK id, and `__contains` or `__startswith` on a FK name returns `400`, as upstream (2.83.0 `rest.py:608-631`, `:670-677`, `serializers.py:403-441`). If a request gives one FK in two spellings, the mirror applies both filters. Upstream keeps only the last one |

The server reads every query parameter outside `limit`, `skip`, `depth`,
`since`, `q` and `fields` as a filter key.
A key that names no field, or that has an unknown operator suffix,
is ignored, and the response is `200` without that filter.
For example, `/api/net?name__foo=x` returns the unfiltered list.
Some relation keys are an exception.
For example, `/api/netfac?name__foo=x` filters on the facility name
(see § Relation filters).
A known key with a value that does not parse for the field type returns `400`.
The server records each ignored key (see § Unknown-field diagnostics).
See § Cross-entity traversal for the 2-hop cap and § Validation Notes
for the rationale.

Filter values follow these rules:

- An exact match on a string field ignores case.
- A bare `address1`, `city` or `state` filter matches a substring,
  as upstream does (2.83.0 `rest.py:583-595`).
  For example, `?city=Frankfurt` also matches `Frankfurt am Main`.
  With an operator suffix or a relation prefix, the key uses the normal
  match rules.
- A bare `country` filter with a 2-letter value is an exact match.
  A longer value matches a substring.
- If a query repeats a filter key, the last value applies.
  A relation key of a `prepare_query` uses the first value
  (see § Relation filters).
- A time field, for example `created` or `updated`,
  accepts Unix seconds or ISO 8601:
  `2024-01-01`, `2024-01-01T12:00:00`, `2024-01-01 12:00:00`,
  or RFC 3339 with an offset.
  A value without an offset is UTC.
- A date without a time applies to the full day.
  `?updated=2024-01-01` matches every row updated on that day,
  `__gt` means after that day, and `__lte` includes that day.
  In an `__in` list, a date means the start of that day
  (00:00:00 UTC).
  `since` accepts only Unix seconds.

Some keys name a column that the mirror stores but upstream does not filter.
pdbcompat ignores these keys the same way, for every operator:

- Keys that `queryable_field_xl` renames to a name that matches no field
  (2.83.0 `serializers.py:428-438`).
  `net_side` and `net_side_id` on `netixlan` become `network_side`.
  `fac_count` on `carrier` becomes `facility_count`.
  The `fac_count` and `net_count` keys of `fac`, `net` and `ix` still filter,
  because their `prepare_query` handles them.
- Serializer fields and model properties that no `prepare_query` handles:
  `org_name` on `carrier` and `campus`,
  `city`, `country`, `state` and `zipcode` on `campus`,
  `name` on `carrierfac`, and `local_asn` on `netfac`
  (`rest.py:525-528`, `:633`, `:670`).
- Relation keys whose last segment is a FK column,
  for example `netixlan?net__org_id=`.
  Upstream strips `_id` from the key and gets `network__org`,
  and `queryable_relations()` leaves FK fields out
  (`serializers.py:991-995`).
  `<fk>__id` keeps its suffix and filters.
  A `prepare_query` relation key also filters,
  for example `netixlan?ix__org_id=` (`serializers.py:643-654`).

`__in` accepts a CSV value and binds
as a single JSON array via SQLite's `json_each()`,
sidestepping the variable-binding limit.
An empty `__in` (`?asn__in=`) short-circuits the request to an empty `data: []`
envelope without running SQL
(a `404` if the request is a lookup by `id` or `asn`, see § Lookup by `id` or `asn`).
The `net` keys `info_type__in` and `info_types__in` are an exception:
an empty value returns all networks, as upstream
(see § Multi-value choice filters).
Malformed `__in` values for typed fields
(e.g. non-integer in `asn__in=`) return `400`.

### Multi-value choice filters

Two fields hold a list of choices:
`info_types` on `net` and `available_voltage_services` on `fac`.
Upstream stores such a field as one string:
the choices in the order of the upstream choice list, joined with commas
(django-peeringdb `fields.py:61-71`, `const.py:114-125` and `:203-208`).
For example, a network with the types `Content` and `NSP` stores `NSP,Content`.
The API returns the list in no fixed order.
The filters compare the stored string,
and the mirror builds the same string from the list that it stores.

| Key | Match |
|-----|-------|
| `info_types=NSP,Content`, `available_voltage_services=No Power,48 VDC` | The stored string, without case. The value must use the choice-list order: `info_types=Content,NSP` matches nothing |
| `<field>__contains=`, `<field>__startswith=` | A substring or a prefix of the stored string |
| `<field>__in=` (except `net` `info_types__in`) | Each item is converted to the stored form: the choices that occur in the item, in choice-list order. A row matches when its stored string is equal to one of the items. An item that holds no choice matches the rows without a value |
| `<field>__lt=`, `__lte=`, `__gt=`, `__gte=` | The value is converted to the stored form, and the two strings are compared |

`net` also accepts the legacy `info_type` keys.
Upstream `NetworkSerializer.finalize_query_params` rewrites these keys,
and two `info_types` keys, onto `info_types`
(2.83.0 `serializers.py:3765-3813`):

| Key | Match |
|-----|-------|
| `info_type=X` | The stored string starts with `X`, contains `,X,`, or ends with `,X`. `info_type=NS` matches a network whose first type is `NSP` |
| `info_type__contains=X` | The same as `info_types__contains=X` |
| `info_type__in=`, `info_types__in=` | An item is a substring of the stored string. Spaces at the ends of an item are removed. An empty item matches every network |
| `info_type__startswith=X`, `info_types__startswith=X` | The stored string starts with `X` or contains `,X` |

An `__in` list costs one `LIKE` test per item on each network,
as the `OR` of `icontains` terms that upstream runs.
`BenchmarkMultiChoice_InfoTypesIn`
(`internal/pdbcompat/multichoice_bench_test.go`, `go test -tags=bench`)
measures the cost per item.

Upstream ignores every other `info_type` key,
because `info_type` is a model property (`models.py:5812-5816`).
This includes relation keys such as `netixlan?net__info_type=`.
A relation key on a multi-value field,
for example `netixlan?net__info_types=`, uses the rules of the first table.
A `prepare_query` relation key without an operator,
for example `ix?fac__available_voltage_services=`,
converts the value to the stored form first (`models.py:221-234`).
The comparison operators compare lower case text in byte order.
Upstream compares under the MySQL collation,
which can put punctuation in a different order.

### List order

A list without `?since` returns rows in `id` order, ascending.
Upstream adds no `ORDER BY` to this query (2.83.0 `rest.py:747-748`),
and none of the 13 models declares a default ordering,
so MySQL returns the rows in primary-key order.
A `?since` list returns rows in `updated` order, ascending,
as upstream orders it (`rest.py:744`).
Rows with the same `updated` value come back in `id` order.
Upstream leaves the order of these rows to the database.
`skip` and `limit` apply after the sort, so each page continues the same order.
An upstream netixlan list can return its rows in a different order
(see § Known Divergences).

### Lookup by `id` or `asn`

A list request with the `id` key on any type,
or with the `asn` key on `/api/net`, is a lookup of one object.
If the list is empty, the response is `404` with the detail `Entity not found`,
as upstream (2.83.0 `rest.py:809-815`, `serializers.py:962-967` and `:3815-3820`).
Only the key counts.
Any other filter, `skip`, `limit` or `since` that empties the list also causes the `404`.
`id__in`, `asn__in`, and `asn` on other types are ordinary filters,
and an empty result is `200` with an empty `data` array.
A request with `?page=` does not get the `404`, as upstream.
The body is problem+json, like every other error (see § Known Divergences).
A value that is not an integer, for example `?id=abc`, returns `400` (see § Known Divergences).

### Diacritic-insensitive matching

On the 16 fields below, these operators ignore diacritics and case:
exact match, `__iexact`, `__contains`, `__icontains`, `__startswith`,
`__istartswith` and `__in`.
For example, `?name=Koln` and `?name__contains=koln` both match `Köln`.
`__lt`, `__lte`, `__gt` and `__gte` compare the stored value
and do not ignore diacritics.
`?q=` does not ignore diacritics (see § Known Divergences).

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
ligature map).
Filter routing happens in `internal/pdbcompat/filter.go`.
When `tc.FoldedFields[<field>]` is `true`, `buildExact`, `buildContains`,
`buildStartsWith` and `buildIn` run the predicate against `<field>_fold`
with `unifold.Fold(value)` on the right-hand side.
The shadow columns carry `entgql.Skip(SkipAll)` and `entrest.WithSkip(true)`
so they are invisible to GraphQL, REST, and proto wire surfaces —
they exist only to power pdbcompat folding.
See § Known Divergences for the upstream-parity comparison
and § Validation Notes for why MySQL collation is *not* the upstream mechanism.

### Soft-delete tombstones

Sync never deletes a stored row.
When upstream deletes an object, the next `?since=` fetch returns the row
with `status='deleted'`, and sync stores that status.
Sync does not mark a row as deleted when the row is missing from a list
response.
The one exception is a connection of a deleted network (see below).
The list path applies the upstream PeeringDB 2.83.0 `rest.py:719-750`
status matrix as the final predicate via `applyStatusMatrix`.
The matrix starts from the live statuses of the type
(upstream `live_statuses()`, `models.py:109-122`):

- `netixlan`: `ok` and `not-operational`.
- All other types: `ok`.

| Request shape | Admitted statuses |
|---------------|-------------------|
| List, no `?since` | live statuses only |
| List with `?since=N` | live statuses and `deleted`; `pending` additionally admitted on `/api/campus` |
| Single-object GET `/api/<type>/<id>` | live statuses and `pending` — tombstones return `404` |
| Nested `_set` lists, `?depth=1` and higher | live statuses only |

Most relation keys of a `prepare_query` also require status `ok`
on one row of their path (see § Relation filters).
For `ixpfx?ix=`, `netfac?name=`, `ixfac?city=` and `campus?facility=`,
that row is the listed row.
A `?since=N` list with one of these keys does not return the deleted rows,
or the pending campuses.

The nested sets follow the upstream nested prefetch
(`serializers.py:1140-1148`), which admits only the live statuses of the
child type.
A pending child is fetchable by its own ID,
but it does not appear in the `_set` lists of its parent.
In practice this affects only campuses:
a campus is pending while it has fewer than two facilities,
and campus is the only type whose pending rows reach the mirror.
A pending campus reaches the mirror in a `?since=` window when it changes,
or through the FK backfill of a facility that points to it.
For `ix.fac_set` and `ixlan.net_set`, the status of the ixfac or netixlan
join row decides membership.
The facility or network that the row points to is not filtered.

A network that upstream deletes because the RIR reclaimed its ASN loses its
live connections without a tombstone
(2.83.0 `management/commands/pdb_rir_status.py:440-443`).
The mirror marks such a connection `deleted`, sets `operational` to `false`,
and keeps its `updated` value.
It does this when the network was deleted in that sync,
or when upstream no longer returns the connection live for an uncached
`?since=1&id__in=` request.
A connection that upstream still serves, or that it changed after it deleted
the network, stays live.
Lists without `?since`, the depth sets, relation keys, `/api/netixlan/<id>`
and `?id=<id>` leave the marked connections out, as upstream does.
A `?since=N` list, with N not later than their `updated` value,
returns them as tombstones, and so does `?id=<id>&since=N`.
Upstream returns nothing, or `404` for the `id` query
(see § Known Divergences).
Because `updated` does not change, a client that syncs by `updated`
does not see the change on any API.

A deleted `poc` is served with `name`, `phone`, `email` and `url` set to `""`.
Upstream applies this rule when `status` is among the rendered fields
(2.83.0 `serializers.py:2941-2954`, `pdb_api_test.py:3120-3127`),
so a tombstone shows the deletion but not the contact details.
The mirror also applies it when `?fields=` leaves out `status`
(see § Known Divergences).
`role`, `visible`, `net_id` and the timestamps are not changed.
Sync stores each deleted `poc` with these fields blank.
The primary also blanks them on older stored tombstones,
when it starts and in each sync cycle.
GraphQL, REST and ConnectRPC serve the stored values,
so they do not serve these fields of a deleted `poc` either.

A `not-operational` netixlan is a published connection that its network
declares not operational.
Upstream 2.83.0 moved every row with `status='ok'` and `operational=false`
to this status, and now derives `operational` as `status == 'ok'`.
The row is served like an `ok` row: on lists, in `?since` windows,
on direct GETs, and in the `netixlan_set` and `net_set` depth sets.
`?operational=false` returns it, and `?status=ok` does not.
To select every live connection, use `?status__in=ok,not-operational`
or leave out the status filter.

`?status=<value>` is an ordinary filter on all 13 types.
Exact match is case-insensitive, and `__in`, `__contains` and `__startswith`
also work.
The filter ANDs with the matrix, so it can only narrow the result:

- `/api/net?status=deleted` returns `[]`, because the list without `?since`
  admits only `ok` rows.
- `/api/net?since=N&status=deleted` returns only the tombstones in the window.
- `/api/campus?since=N&status=pending` returns only the pending campuses.

This matches upstream PeeringDB 2.83.0.
`rest.py:683` turns `?status=` into `status__iexact`,
and the matrix filter at `rest.py:745-750` is applied after it.
Upstream tests lock the result
(`pdb_api_test.py:4022-4028` and `:4032-4044`).

### Metadata document (`meta`)

Every `net` and `netixlan` object carries `meta`,
the PeeringDB metadata document that upstream added in 2.83.0
(migration 0159; `serializers.py:3117` and `:3684`).
The key follows `logo` on `net` and `ix_side_id` on `netixlan`.

- `meta` is an open JSON object.
  Upstream registers its keys in server code and can add keys
  without a new release of its client model library
  (`docs/api/object_metadata.md:22-23`).
  The mirror stores the document as is, so a new key needs no change here.
  At 2.83.0 the keys are `preferred_ip_mtu` and `rtbh_community` on `net`,
  and `planned_status_change` (`status`, `date`) and `rfc8950` on `netixlan`.
- Sync stores the document that upstream sends,
  and pdbcompat serves it unchanged.
  Key order inside the document can differ from upstream.
- A row without a stored document returns `{}`, the same as upstream.
- `meta` appears in every shape: lists, detail responses, depth `_set` objects,
  and nested `net` objects.
- Upstream applies no read restriction to `meta`,
  so the mirror applies no privacy filter to it.

A document that upstream set before the mirror stored `meta` arrives with the
next full sync.
With the default `PDBPLUS_FULL_SYNC_INTERVAL` of `24h`, this is within a day.
Incremental sync cannot repair such a row,
because it does not rewrite a row whose `updated` value it already has.
If the interval is `0`, run one full sync (`POST /sync?mode=full`).

#### Metadata filters

`/api/netixlan` filters on the metadata keys that upstream marks as filterable
(`meta_registry.py:277-313`, `docs/api/object_metadata.md:165-182`):

| Filter key | Upstream column name | Type |
|------------|----------------------|------|
| `meta__planned_status_change__status` | `meta_planned_status_change_status` | text |
| `meta__planned_status_change__date` | `meta_planned_status_change_date` | date |
| `meta__rfc8950` | `meta_rfc8950` | boolean |

- Each key takes the usual operator suffixes,
  for example `?meta__planned_status_change__date__lt=2026-10-15`.
- The upstream column name is also a filter key, the same as upstream.
- Text: exact match, `__contains` and `__startswith` ignore case.
  `__in`, `__lt`, `__lte`, `__gt` and `__gte` also work.
- Date: the document stores the date as `YYYY-MM-DD`.
  Exact match is a prefix match, the same as upstream,
  so `2026-10` matches every day in October 2026.
  `__lt`, `__lte`, `__gt` and `__gte` compare dates.
  `__in` matches whole dates.
  Upstream returns an error for `__in` on a date (see § Known Divergences).
  `__contains` and `__startswith` return `400`.
- Boolean: `true` (in any case) or `1` selects `true`.
  Any other value selects `false`, the same as upstream.
  `__in` parses each value like the other boolean filters.
  Other operators return `400`.
- A row without the key never matches, for any operator.
  So `?meta__rfc8950=false` returns only the rows that declare `false`,
  not the rows that never set the key.
- On these keys, upstream knows only the operators
  `__lt`, `__lte`, `__gt`, `__gte`, `__contains`, `__startswith` and `__in`.
  pdbcompat ignores a key with any other suffix, as upstream does,
  for example `meta__rfc8950__foo` or `meta__rfc8950__iexact`.
  The `__iexact`, `__icontains` and `__istartswith` names
  that the ordinary filters accept do not apply to these keys.
- `net` has no filterable metadata keys.
  Upstream ignores `?meta__rtbh_community=` and `?meta__preferred_ip_mtu=`
  and returns the full list. The mirror does the same.

Upstream rewrites these keys onto typed, indexed columns
before it applies the filters (`serializers.py:3129-3149`).
pdbcompat resolves them before it splits a key for traversal,
so the 2-hop cap does not apply to them.
The mirror has no such columns.
It reads the key from the stored document with SQLite `json_extract`
(`json_type` for the boolean).
A filter on a metadata key alone therefore scans the netixlan table.
The budget count and the served list use the same predicates.

### Cross-entity traversal

pdbcompat resolves `<fk>__<field>`
and `<fk>__<fk>__<field>` filter paths through two mechanisms,
both driven by codegen from ent schema annotations at `go generate` time:

- **Path A: per-serializer allowlists.**
  Derived from the upstream `peeringdb_server/serializers.py`
  `prepare_query(...)` / `get_relation_filters(...)` seed lists and from
  `queryable_relations()`.
  The keys are the mirror's choice of aliases, not a copy of an upstream
  list: some resolve keys that upstream ignores, and some do not resolve
  here.
  The relation keys that a `prepare_query` handles are not in Path A:
  they resolve first, as relation filters (see § Relation filters).
  Generated from ent schema `pdbcompat.WithPrepareQueryAllow(...)` annotations
  via `cmd/pdb-compat-allowlist`; emitted into
  `internal/pdbcompat/allowlist_gen.go`.
  This is the "explicitly blessed" set of filter keys.
  Every entry carries a `// serializers.py:<line>` comment that anchors it to
  upstream 2.83.0.
- **Path B: ent edge introspection.**
  When a filter key does not match Path A,
  the parser consults the generated `Edges` map
  (also emitted into `allowlist_gen.go`).
  Every non-excluded FK edge auto-exposes `<fk>__<field>`
  for any filterable field on the target entity
  that is a model field upstream.
  A serializer field or a model property that the mirror stores,
  such as `org_name` on `fac` or `city` on `campus`,
  is not a target (`TypeConfig.NonModelFields`):
  `queryable_relations()` offers only model fields,
  so upstream ignores `netfac?fac__org_name=` and `fac?campus__city=`,
  and so does pdbcompat.
  A forward edge also accepts the upstream model name of its FK
  as the first segment (`network__asn`, `facility__name`).
  This is close to upstream `queryable_relations()`, which exposes
  `<fk>__<field>` for the forward FKs and `<related_name>__<field>`
  for the reverse relations.
  The mirror also follows reverse edges and second hops,
  names a reverse edge by its traversal key (`org?ix__name=`)
  instead of the upstream `_set` name (`org?ix_set__name=`),
  and has no field-level exclusions.
  See § Known Divergences.
  A relation key filters `status` only through a forward edge
  one hop away (`net?org__status=`).
  Upstream ignores `status` on the mirror's reverse and 2-hop keys too.
  Resolution uses a static map that codegen emits.
  There is no runtime ent-client introspection, `sync.Once` or init-order
  coupling.

The resolution order is implemented in `internal/pdbcompat/filter.go`
`ParseFiltersCtx` and `buildTraversalPredicate`:
relation filters first, then Path A; on a soft miss (allowlist hit but
downstream introspection unavailable) the parser falls through to Path B rather
than suppressing the key.
`parseFieldOp` returns the 3-tuple `(relationSegments, finalField, op)`
so the same machinery serves 1-hop and 2-hop paths with a single split.

#### Relation filters

Upstream handles some relation keys in the `prepare_query` method of a
serializer, apart from its model-field filters
(2.83.0 `serializers.py:614-656`).
pdbcompat resolves these keys before Path A and Path B,
with the same paths and status rules
(`relationSeeds` in `internal/pdbcompat/relation_filter.go`).

| Type | Relation keys | Path from the listed row | Row that must have status `ok` |
|------|---------------|--------------------------|--------------------------------|
| `fac` | `net`, `ix` | netfac → net, ixfac → ix | the netfac or ixfac row |
| `fac` | `org_name` | org, field `name` | none |
| `ix` | `ixlan`, `ixfac` | ixlan, ixfac | that row |
| `ix` | `fac` | ixfac → fac | the ixfac row |
| `ix` | `net` | ixlan → netixlan → net | the netixlan row (not the ixlan) |
| `net` | `ix` | netixlan → ixlan → ix | the netixlan row |
| `net` | `ixlan`, `fac` | netixlan → ixlan, netfac → fac | the netixlan or netfac row |
| `net` | `netixlan`, `netfac` | netixlan, netfac | that row |
| `netixlan` | `ix`, `name` | ixlan → ix | the ixlan row |
| `netixlan` | `name__iexact`, `name__icontains`, `name__istartswith` | ixlan, field `name` | the ixlan row |
| `ixpfx` | `ix` | ixlan → ix | the listed prefix |
| `netfac`, `ixfac` | `name`, `country`, `city` | fac, field of the same name | the listed row |
| `campus` | `facility` | fac | the listed campus |
| `org` | `asn` | net, field `asn` | the net row |
| `carrier` | `carrierfac_set__facility_id` | carrierfac, field `fac_id` | none |

The relation keys of `fac`, `ix`, `net`, `netixlan` and `ixpfx`
also accept the spelling `<rel>_id`, except `org_name` and `name`.
The key forms follow `get_relation_filters`:

- `<rel>=V` compares the id of the related row, and `<rel>__<op>=V`
  compares that id with the operator.
- `<rel>__<field>=V` filters a field of the related row,
  and `<rel>__<field>__<op>=V` adds an operator.
  A field that ends in `_id` names a FK of that row
  (`ix?ixfac__fac_id=`).
  A field that is not a model field upstream is ignored,
  for example the serializer field in `net?netfac__name=`
  (upstream returns `400`, see § Known Divergences).
- `<rel>__<field>__<other>=V` drops the third segment:
  `net?netfac__fac__name=X` compares the facility id with `X`,
  which returns `400` for a non-numeric `X`.
- A key with four or more segments is ignored.
- `netixlan?name=V` compares the exchange name,
  not the stored `name` of the netixlan
  (`serializers.py:3166-3167`).
  `get_relation_filters` does not parse the suffixes `__iexact`,
  `__icontains` and `__istartswith` (`serializers.py:614-656`),
  so `related_to_name` applies them to the name of the ixlan:
  `netixlan?name__iexact=V` compares the ixlan name, not the exchange name.
- `fac?org_name=V` is a substring match on the name of the organization
  (`serializers.py:2115-2117`),
  not an exact match on the stored `org_name` of the facility.
  It accepts only an operator that `get_relation_filters` parses
  after the key.
  Upstream ignores `fac?org_name__iexact=`, `__icontains=` and
  `__istartswith=`, and so does pdbcompat.
- The `netfac` and `ixfac` keys `name`, `country` and `city`
  filter the field of the same name on the facility
  (`serializers.py:3417-3424`).
  One or two segments after the key name have no effect,
  but an operator at the end applies.
  `city` and `country` are exact matches:
  the substring rewrite of the location keys applies only to model fields
  (`rest.py:583-595`).
- `org?asn=V` and `carrier?carrierfac_set__facility_id=V`
  accept no operator.
  Upstream ignores `org?asn__in=` and the operator forms of the carrier key,
  and so does pdbcompat.

The status rules follow `make_relation_filter` (`models.py:221-234`):

- The row in the table must have status `ok`.
  The other rows have no status check.
  For example, `net?ix=` does not check the ixlan.
- If the key filters `status` of that row without an operator,
  upstream replaces the value with `ok`:
  `net?netixlan__status=deleted` returns the nets that have an `ok` netixlan.
  With an operator, both filters apply.
- A repeated relation key uses its first value, not the last
  (`serializers.py:618-619`).

#### Supported shapes per entity (1-hop + 2-hop)

All 13 entity types support Path A 1-hop shapes via `?<fk>__<field>=X`.
Path A and Path B both resolve 1-hop and 2-hop keys:

| Query | Hops | Path | Upstream citation |
|-------|------|------|-------------------|
| `?org__name=X` (net, fac, ix, carrier, campus) | 1 | A | 2.83.0 `serializers.py:970-996` (`queryable_relations()` adds `org__<field>` from the `org` FK) |
| `?net__asn=X` (for example netfac, netixlan, poc) | 1 | A | (same allowlist block) |
| `?ix__name=X` (for example ixfac, ixlan) | 1 | A | (same allowlist block) |
| `?<rel>=N`, `?<rel>__<field>=X` for the `prepare_query` relation keys, for example `fac?net=N`, `net?ix__name=X` and `netixlan?ix_id=N` | 1 (up to 3 tables) | Relation filter (`relationSeeds`), before Path A and B. See § Relation filters | 2.83.0 `serializers.py:614-656` (`get_relation_filters`) and the `related_to_<x>` methods, which pin one row to status `ok` (`models.py:221-234`) |
| `?fac__name=X` (for example netfac, ixfac, carrierfac) | 1 | A | (same allowlist block) |
| `?network__<field>=X` (poc, netfac, netixlan) and `?facility__<field>=X` (netfac, ixfac, carrierfac) | 1 | A or B, through the `net` or `fac` edge | 2.83.0 `serializers.py:416-438` (`queryable_field_xl` renames `net` and `fac` to `network` and `facility`) and `:970-996` |
| `?org=N`, `?network=N`, `?facility_id__in=N,M` and the other upstream FK names | 0 | Local FK column (`TypeConfig.ForeignKeys`) | 2.83.0 `rest.py:608-631` and `:670-677` (a ForeignKey key filters `<fk>_id`) |
| `?ixlan__ix__id=N`, `?ixlan__ix__name=X` (ixpfx) | 2 | A | Mirror extension. Upstream ignores 2-hop keys (see § Known Divergences) |
| `?<fk>__<fk>__<field>=X` through any two non-excluded edges, for example `netixlan?net__org__name=X` | 2 | B | Mirror extension (see § Known Divergences) |
| `?<fk>__<field>=X` for any non-excluded edge | 1 | B | 2.83.0 `serializers.py:970` (`queryable_relations()`) |

1-hop Path B fallthrough means the explicit Path A allowlists are
**additive, not restrictive**: a key that is not in Path A but is a valid ent FK
edge still resolves via Path B. The exclusion list (below) is the only way to
block a Path B key.

An allowlisted key can resolve nothing.
The server then ignores it.
For example, the `fac` allowlist has `ixlan__ix__fac_count`,
but `fac` has no `ixlan` edge, so `fac?ixlan__ix__fac_count__gt=0`
returns the unfiltered list.
Upstream also ignores this key, because it is not a `fac` filter
(2.83.0 `rest.py:525-528`, `serializers.py:970-996`).

#### FILTER_EXCLUDE list

The `pdbcompat.WithFilterExcludeFromTraversal()` ent edge annotation hides
specific edges from Path B traversal.
It is the edge-level counterpart of upstream 2.83.0 `FILTER_EXCLUDE`
(`serializers.py:136-166`).
The upstream entries that name one field of a relation have no
counterpart.
The private-field entries need none: the mirror does not filter on those
fields.
Of the unused-field entries, `org__latitude`, `org__longitude` and
`ixlan__descr` resolve on the mirror.
Upstream ignores them, except where a `prepare_query` handles the key
(for example `ix?ixlan__descr=`).
See § Known Divergences.
Upstream 2.83.0 adds the `ixf_import_request_user` relation to the list
(`serializers.py:147`), so upstream now ignores
`ix?ixf_import_request_user__<field>=`.
The mirror does not model that relation and always ignored these keys,
so the new entry needs no annotation.

| Entity | Edge | Reason |
|--------|------|--------|
| (none) | | No edge has the annotation. Field-level privacy applies in the serializers, not on edges. |

#### 2-hop cap

Filter keys with more than 2 `__`-separated relation segments are silently
ignored.
Examples:

- `?org__name=X`: 1 hop, resolves via Path A (every primary entity).
- `?ixlan__ix__id=N` on `ixpfx`: 2 hops, resolves via Path A
  (`TestParity_Traversal/DIVERGENCE_path_a_2hop_ixpfx_via_ixlan_ix_id`).
  Upstream ignores this key: it is a mirror extension
  (see § Known Divergences).
- `?ixlan__ix__org__name=X`: 3 hops, SILENTLY IGNORED (HTTP 200,
  result set is unfiltered).

A relation key of a `prepare_query` also has at most two segments
before its operator, but its path can reach three tables:
`net?ix__name=` walks netixlan, ixlan and ix
(see § Relation filters).

The netixlan metadata filter keys, such as
`meta__planned_status_change__date__lt`, are not relation paths.
pdbcompat resolves them before the split.
See § Metadata filters.

Upstream resolves at most one relation hop.
`queryable_relations()` adds `<fk>__<field>` for each FK of the model
(2.83.0 `serializers.py:970-996`).
`get_relation_filters` passes a serializer's `prepare_query` only the keys
whose first segment is in its seed list (`serializers.py:614-656`).
Upstream ignores every other multi-hop key, so it ignores 3+-hop keys too.
The mirror's 2-hop keys go one hop further than upstream
(see § Known Divergences).
The cap keeps a predictable cost ceiling of `<50ms/op @ 10k rows`,
checked locally via the build-tagged gate
`internal/pdbcompat/bench_traversal_test.go` (`go test -tags=bench`, without
`-race`); CI does not run it.
If a legitimate 3-hop use case emerges,
raise the cap together with a fresh benchstat run and a docs update here.

#### Unknown-field diagnostics

When the server ignores one or more filter keys of a list request,
it records them in two places:

- A DEBUG log record `pdbcompat: unknown filter fields silently ignored`
  with `endpoint`, `type` and `unknown_fields` (a comma-separated list).
- The span attribute `pdbplus.filter.unknown_fields`, with the same list.

This applies to every ignored key, not only to traversal keys.
INFO logs do not include these keys,
so clients that test field names do not fill the logs.
To see them, set `PDBPLUS_LOG_LEVEL=DEBUG`
or query the span attribute in Grafana Tempo.

### Response memory budget

Before the server runs a list query, it counts the matching rows
with `SELECT COUNT(*)` (`serveList` in `internal/pdbcompat/handler.go`).
It multiplies the count by a typical row size for the type.
If the result is larger than `PDBPLUS_RESPONSE_MEMORY_LIMIT` (default `128MiB`),
the server returns `413` and does not run the list query.
`0` turns the check off.
Use `0` only for local development.

A budget-exceeded request returns:

- `413 Request Entity Too Large`
- `Content-Type: application/problem+json`
- `type: https://peeringdb-plus.fly.dev/errors/response-too-large`
- Body extension fields `max_rows` (the largest result set that *would* fit)
  and `budget_bytes` (the configured ceiling)

The estimate depends on the request and the stored rows,
not on the server load,
so a retry of the same request gets the same `413`.
A client that gets `413` must add filters,
or read the list in pages with `limit` and `skip`.

The server also limits the total estimated size of the responses in progress
to `PDBPLUS_RESPONSE_MEMORY_LIMIT`.
If a new response does not fit, the server returns `503 Service Unavailable`
with `Retry-After: 1`.
A retry can succeed.
Detail requests go through both checks.
For the second check, a detail request at depth 2 or more also counts
the rows in its `_set` lists.
The budget applies only to `/api/`.
For the other surfaces, see
[ARCHITECTURE.md § Response Memory Envelope](ARCHITECTURE.md#response-memory-envelope).

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

# The 50 IXPs in country DE with the lowest IDs
curl "https://peeringdb-plus.fly.dev/api/ix?country=DE&limit=50"

# Only return id and name
curl "https://peeringdb-plus.fly.dev/api/ix?country=DE&fields=id,name"

# Networks updated since 2024-01-01 (Unix seconds) — admits tombstones
curl "https://peeringdb-plus.fly.dev/api/net?since=1704067200"

# Diacritic-insensitive substring match against organization names
curl "https://peeringdb-plus.fly.dev/api/org?name__contains=koln"

# 1-hop traversal: exchanges whose organization name contains "DE-CIX"
curl "https://peeringdb-plus.fly.dev/api/ix?org__name__contains=DE-CIX"

# 2-hop traversal (mirror extension): prefixes of exchanges whose name
# contains "DE-CIX"
curl "https://peeringdb-plus.fly.dev/api/ixpfx?ixlan__ix__name__contains=DE-CIX"
```

### Response envelope

Every successful response uses the PeeringDB envelope:

```json
{
  "meta": {},
  "data": [ { "...": "..." } ]
}
```

Detail endpoints return a single-element `data` array, not a bare object,
to preserve parity with upstream PeeringDB clients.

### Errors

Errors use
[RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html) with
`Content-Type: application/problem+json`.
Typical status codes:

| Status | Cause |
|--------|-------|
| `400` | An operator that the field type does not support (for example `asn__contains`), a value that does not parse for the field type, a malformed `__in` value, a `since` or an ID that is not an integer, or a `limit` or `skip` that is not a non-negative integer |
| `404` | Unknown `{type}`, missing `{id}`, detail GET on a tombstoned row, or an empty list for a lookup by `id` (any type) or `asn` (`net`), see § Lookup by `id` or `asn` |
| `413` | The estimated response is larger than the response memory budget (see § Response memory budget) |
| `500` | Database error (details redacted from response body, full error logged) |
| `503` | The in-flight response pool is full (transient, `Retry-After: 1`), or the first sync has not completed (see § Before the first sync) |

Responses include an `X-Powered-By` header identifying the server
as PeeringDB Plus.

## 5. ConnectRPC / gRPC (`/peeringdb.v1.*`)

Implemented in `internal/grpcserver/` using
[ConnectRPC](https://connectrpc.com/) — a gRPC-compatible framework that speaks
three protocols on the same endpoint:

| Protocol | Typical client | Content types |
|----------|----------------|---------------|
| Connect (HTTP/1.1 or HTTP/2) | `connect-go`, browser fetch | `application/proto`, `application/json` |
| gRPC (HTTP/2) | `grpc-go`, `grpcurl`, any gRPC stub | `application/grpc`, `application/grpc+proto`, `application/grpc+json` |
| gRPC-Web | Browser gRPC-Web clients | `application/grpc-web`, `application/grpc-web-text` |

The server listens on a single port with h2c enabled
(`buildServer` in `cmd/peeringdb-plus/main.go`),
so there is no separate port for gRPC.

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

### Messages

The messages in `proto/peeringdb/v1/v1.proto` are hand-maintained.
entproto generated them at v1.6.
A later ent field reaches this surface only when it is added by hand,
so some fields (for example `Network.ixp_update_exclude`) are not present.

`Network.meta` (field 41) and `NetworkIxLan.meta` (field 19) carry the
PeeringDB metadata document (added upstream in 2.83.0)
as a `google.protobuf.Struct`:

- A row without a stored document gets an empty `Struct`,
  the same as upstream's `{}`. The field is always present.
- If the server cannot convert a stored document to a `Struct`,
  it logs a warning and omits the field for that row.
  The RPC does not fail.
- Connect and gRPC JSON clients see `meta` as a plain JSON object.
  All numbers in a `Struct` are doubles.

### Filtering

List and Stream requests accept type-specific optional filter fields
(see `proto/peeringdb/v1/services.proto`).
All filters AND together.
The `name`, `aka`, `name_long` and `city` filters match a substring
and ignore case (`ContainsFold`).
They do not ignore diacritics.
The other string filters, `status` included, must match the full value,
and they are case-sensitive.
Integer filters such as `org_id` must be positive.
The `asn` filter of `ListNetworks`, `StreamNetworks`,
`ListNetworkIxLans` and `StreamNetworkIxLans` must not be negative:
upstream keeps tombstones with ASN 0.
Invalid values return `INVALID_ARGUMENT`.

### Pagination (List)

| Field | Semantics |
|-------|-----------|
| `page_size` | Requested page size. Defaults to `100`, clamped to `1000`. See `normalizePageSize` in `internal/grpcserver/pagination.go` |
| `page_token` | The `next_page_token` from the previous response. The token holds a row offset. If a sync runs between two pages, rows can move, and a page can skip or repeat rows. A token that does not decode returns `INVALID_ARGUMENT` |

List RPCs return rows in `(-updated, -created, -id)` order:
the newest `updated` value first.

### Streaming semantics

`Stream{Type}` RPCs use **batched compound keyset pagination** under the hood
(`StreamEntities` in `internal/grpcserver/generic.go`),
fetching `streamBatchSize` (`500`) rows per database round-trip
and emitting one proto message per row.
The cursor is the compound `(updated, created, id)` triple;
under the default `(-updated, -created, -id)` order each batch resumes via:

```sql
WHERE (updated < cursor.updated)
   OR (updated = cursor.updated AND created < cursor.created)
   OR (updated = cursor.updated AND created = cursor.created AND id < cursor.id)
```

The keyset carries every sort key, so it matches the three-key ordering exactly:
progress stays monotonic and no row is skipped or repeated even
when many rows share an `updated` timestamp (or an `updated`+`created` pair).

| Field | Semantics |
|-------|-----------|
| `since_id` | Filter — emits only rows with `id > since_id`. Applied as a `WHERE` predicate; **does not seed the keyset cursor** |
| `updated_since` | Filter — emits only rows with `updated > updated_since`. Applied as a `WHERE` predicate; **does not seed the keyset cursor** |

Every stream is capped by `PDBPLUS_STREAM_TIMEOUT`
(default `60s`) enforced via `context.WithTimeout` at the handler.
Exceeding the timeout closes the stream with `DEADLINE_EXCEEDED`.

### `pdbplus-total-count` response header

On **full streams** (both `since_id` and `updated_since` unset),
the handler runs a `SELECT COUNT(*)` preflight
and sets the `pdbplus-total-count` response header
to the total matching row count.
On **delta streams** (either `since_id` or `updated_since` set),
the COUNT preflight is skipped entirely
and the `pdbplus-total-count` header is **absent** —
not "present with 0" and not "present with -1".
Clients of delta streams have no use for a full-table total
and the skip avoids a needless full-table scan.

The header was named `grpc-total-count` before v1.23;
the `Grpc-` prefix is reserved for protocol metadata by connect-go/gRPC,
so the application header moved to the `pdbplus-` prefix.
The legacy `grpc-total-count` name is still dual-emitted
for a deprecation window and will be removed in a future release —
migrate clients to `pdbplus-total-count`.

### Errors

| Code | Cause |
|------|-------|
| `INVALID_ARGUMENT` | Invalid filter value or malformed `page_token`. The message names the problem |
| `NOT_FOUND` | `Get{Type}` found no row with that ID that the caller can see |
| `CANCELED` | The client canceled the call |
| `DEADLINE_EXCEEDED` | The client deadline passed, or a stream ran longer than `PDBPLUS_STREAM_TIMEOUT` |
| `RESOURCE_EXHAUSTED` | The request message is larger than 1 MB |
| `UNAVAILABLE` | The server has not completed its first sync. Poll `grpc.health.v1.Health` until it reports `SERVING` |
| `INTERNAL` | Server-side database failure. The message is only `internal error`; the full error is logged and recorded on the request trace |

### Reflection and health

| Handler | Path |
|---------|------|
| gRPC reflection v1 | `/grpc.reflection.v1.ServerReflection/*` |
| gRPC reflection v1alpha | `/grpc.reflection.v1alpha.ServerReflection/*` |
| gRPC health check | `/grpc.health.v1.Health/*` |

Reflection serves all 13 service descriptors,
enabling `grpcurl` and `grpcui` to discover the API with no additional wiring.
The health checker reports `NOT_SERVING` until the first sync completes
(`HasCompletedSync` in the sync worker),
then flips to `SERVING` for the empty service name and
for every `peeringdb.v1.*` service.
The health handler bypasses the readiness middleware so
that health checks can poll the service during sync-in-progress state without
being intercepted by the 503 syncing page.

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

## 6. MCP (`/mcp`)

`POST /mcp` serves the
[Model Context Protocol](https://modelcontextprotocol.io/)
over stateless Streamable HTTP.
Responses use JSON rather than server-sent events,
so requests can be handled by any healthy replica without session affinity.
MCP 2026-07-28 requests use `server/discover` and include the protocol version
with every request.
Clients on an older revision use the `initialize` handshake.
The server also supports the revisions 2025-11-25, 2025-06-18, 2025-03-26
and 2024-11-05.
All tools are read-only and query the same local ent client as the other
surfaces.
Each tool declares input and output schemas, read-only and idempotent hints,
and closed-corpus behavior.

| Tool | Purpose |
|------|---------|
| `search_peeringdb` | Grouped search across all entity types, or cursor-paginated typed search |
| `get_network` | Network detail by ASN, including bounded IX and facility relations |
| `get_exchange` | Exchange detail and bounded participant, facility, and prefix relations |
| `get_facility` | Facility detail and bounded network, exchange, and carrier relations |
| `get_organization` | Organization detail and bounded child-entity relations |
| `get_campus` | Campus detail and bounded facilities |
| `get_carrier` | Carrier detail and bounded facilities |
| `compare_networks` | Compare two ASNs across exchanges, facilities, and campuses |
| `lookup_ip` | Find exact peering-address records and the exchange prefixes that contain the address. Each record includes `meta` (`{}` when the row has no document) |
| `get_sync_status` | Return the latest mirror synchronization status and freshness |

Related collections default to 20 rows and are capped at 100 rows.
Opaque cursors are bound to the entity, relation, and successful-sync
watermark.
A cursor is rejected after the mirror synchronizes,
preventing rows from being skipped or repeated across snapshots.
Free-text queries are capped at 4 KiB.

The server also exposes:

- `peeringdb-plus://service` and `peeringdb-plus://guide` resources.
- `research_network` and `compare_networks` prompts.
- `GET /skills/peeringdb-plus/SKILL.md` for the origin-neutral raw skill.
- `GET /skills/peeringdb-plus.zip` for an installable skill archive.
- `GET /.well-known/mcp/server-card.json` for MCP discovery metadata.
- `GET /.well-known/agent-skills/index.json` for the Agent Skills index.
- `GET /.well-known/agent-skills/peeringdb-plus/SKILL.md` for the standard
  skill location.
- `GET /llms.txt` for a curated Markdown index.

The archive and origin-specific discovery files are generated on demand.
Their URLs point to the request's own origin,
or to the operator's `PDBPLUS_PUBLIC_URL` override.
This keeps self-hosted deployments local and avoids embedding a production
hostname in the binary.

Browser clients use `PDBPLUS_CORS_ORIGINS`.
The `/mcp` handler also checks the `Origin` header against
`PDBPLUS_CORS_ORIGINS`.
It rejects a malformed origin, or an origin that is not in the list,
with `403`.
With the default `*`, it accepts every well-formed origin,
so this check gives no DNS-rebinding defense.
To get that defense, set `PDBPLUS_CORS_ORIGINS` to the exact browser origins.
Separately, the MCP SDK rejects a request that arrives on a loopback address
with a `Host` header that is not a loopback name.
Clients that are not browsers send no `Origin`,
so the origin check does not apply to them.

## Field-level privacy

PeeringDB Plus mirrors upstream PeeringDB's per-field visibility marker
for the IX-F member list URL:
`ixlan.ixf_ixp_member_list_url` is gated by the sibling string field
`ixlan.ixf_ixp_member_list_url_visible`, which carries one of `Public` / `Users`
/ `Private` (the schema default is `Private`, and a NULL/empty or unknown value
fails closed to redacted).
Anonymous callers (the default `PDBPLUS_PUBLIC_TIER=public` deployment) receive
the value only when `_visible = Public`; for `Users` or `Private` the value is
omitted across all six surfaces while the `_visible` companion field is
**still emitted** (upstream parity).

On `/api/`, the permission decides the key, not the value.
A caller that may see the URL gets the key, also when the stored value is empty.
Upstream does the same: it deletes the key only when the caller
does not have the permission (2.83.0 `permissions.py:344-353`).
An empty `Users` value is the exception.
An anonymous sync does not get the URL of a `Users` row, and it stores `""`.
Thus `/api/` omits the key for an empty `Users` value at every tier
(see § Known Divergences).

The single source of truth is `internal/privfield.Redact(ctx, visible, value)`.
Every serializer calls it,
and `internal/middleware.PrivacyTier` stamps the resolved tier on the request
context — unstamped contexts fail-closed to `TierPublic`.

| Surface | Mechanism |
|---------|-----------|
| `/api/` (pdbcompat) | `internal/pdbcompat/serializer.go` `ixLanFromEnt(ctx, l)`. When `Redact` returns `omit=true`, or for an empty non-Public value (see above), the URL is a nil `*string`, and the JSON struct tag `,omitempty` removes the key |
| `/rest/v1/*` (entrest) | `RESTFieldRedact` (`internal/middleware/rest_redact.go`) reads every JSON response under `/rest/v1/` except `openapi.json`. When `Redact` returns `omit=true`, it deletes the key from each object that has the `_visible` companion, including ixlan objects under `edges`. It runs inside `middleware.RESTError`, so error bodies pass through unchanged |
| `/peeringdb.v1.IxLanService/*` (ConnectRPC) | `internal/grpcserver/ixlan.go` `ixLanToProto(ctx, il)` returns `nil *wrapperspb.StringValue` — wire absence under proto3 optional |
| `/graphql` | `graph/schema.resolvers.go` `ixLanResolver.IxfIxpMemberListURL` returns Go `nil` → GraphQL `null` |
| `/ui/` | No render path renders the URL today; future templates must call `privfield.Redact` in the data-prep step |
| `/mcp` | No tool returns the URL |

Operators who run a private deployment can flip `PDBPLUS_PUBLIC_TIER=users` to
make anonymous callers behave as authenticated users — the startup logger emits
a `WARN` with `public_tier=users` so the override is visible in deploy logs.
The Users tier gets what upstream gives an authenticated user who is not a
member of the owning organization:
`Public` and `Users` values and `poc` rows, but not `Private` ones.
The mirror has no organization membership, so no tier sees `Private` data.

### Contact visibility

Each `poc` row has `visible`: `Public`, `Users` or `Private`.
With the default `PDBPLUS_PUBLIC_TIER=public`,
a caller sees only `Public` contacts on every surface.
A row with no value counts as `Public`.
With `PDBPLUS_PUBLIC_TIER=users`, callers also see `Users` contacts.
No caller sees `Private` contacts.
Filters through a relation apply the same rule,
for example `/api/net?poc__email__contains=`.

The GraphQL `NetworkWhereInput` has no `hasPocs` or `hasPocsWith` predicate.
Such a predicate tests the `poc` rows in an SQL subquery,
and the privacy policy does not apply to it.
A caller could thus match networks on the `name`, `phone` or `email`
of a contact that the tier hides, and read the value one prefix at a time.
To filter on contact data, query `pocs` or `pocsList` with a `PocWhereInput`.
The policy applies to that query.

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
  "mcp": "/mcp",
  "mcp_server_card": "/.well-known/mcp/server-card.json",
  "skill": "/skills/peeringdb-plus/SKILL.md",
  "skill_well_known": "/.well-known/agent-skills/peeringdb-plus/SKILL.md",
  "skill_index": "/.well-known/agent-skills/index.json",
  "skill_archive": "/skills/peeringdb-plus.zip",
  "llms": "/llms.txt",
  "ui": "/ui/",
  "healthz": "/healthz",
  "readyz": "/readyz"
}
```

`GET /` and `HEAD /` include `Link` headers for `llms.txt`, the MCP server
card, and the Agent Skills index.
The root bypasses the readiness middleware so service discovery still works
while the first sync is in progress.

### `GET /healthz`

Liveness probe.
Always returns `200 OK` with a fixed JSON body as long
as the process can serve HTTP.
It does **not** check database connectivity or sync state —
a failing `/healthz` means the process itself is wedged and should be restarted.

Bypasses the readiness middleware.

### `GET /readyz`

Readiness probe.
Returns `200 OK` only when all of these conditions are true:

1. The database answers a ping in 2 seconds or less.
2. At least one sync has succeeded.
3. The latest sync did not fail.
   While a sync runs, the server checks the last successful sync instead.
4. The last successful sync is not older than `PDBPLUS_SYNC_STALE_THRESHOLD`
   (default `24h`).

Otherwise it returns `503 Service Unavailable`.
After a failed sync, `/readyz` returns `503` until the next sync attempt
starts: a retry or the next scheduled cycle.
Replicas read the same replicated `sync_status` table,
so every machine returns `503` during that time.
The Fly.io health check uses `/readyz`,
so Fly Proxy stops routing to these machines until then.
The response body is the opaque shape `{"status":"ok"}`
or `{"status":"unhealthy"}` —
detailed error strings are written to structured logs only
(security hardening: the wire body does not leak internal failure detail).

Bypasses the readiness-gate middleware itself
(so a probe can observe the unready state rather than being redirected to the
syncing page).

### `POST /sync`

On-demand sync trigger.
Only served by the LiteFS primary.

| Header / Param | Purpose |
|----------------|---------|
| `X-Sync-Token` request header | Must match `PDBPLUS_SYNC_TOKEN` using `subtle.ConstantTimeCompare`. Empty token on either side = always reject |
| `?mode=full` or `?mode=incremental` | Overrides `PDBPLUS_SYNC_MODE` for this run. Any other value returns `400` |
| `?mode=history` | Deletes the progress of the history sweep and runs an incremental cycle (never a full one) whose sweep starts again at the first window. See [ARCHITECTURE.md § History sweep](ARCHITECTURE.md#history-sweep) |
| `?trace=0` | Do not trace this run, whatever `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` is. Without it, the server traces every manual sync. `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` applies only to scheduled cycles |

| Status | Meaning |
|--------|---------|
| `202 Accepted` | Request authenticated; sync started in the background (fire-and-forget, using the application root context so request cancellation does not abort it). The body is `{"status":"accepted"}` |
| `307 Temporary Redirect` + `fly-replay: region=<primary>` | On Fly.io replicas, the request is replayed to the primary region. Clients following the header will land on the primary and complete the call |
| `401 Unauthorized` | Missing, wrong, or mismatched-length `X-Sync-Token` |
| `400 Bad Request` | Invalid `mode` value |
| `409 Conflict` | A sync cycle is already running. The server does not start a second one. The body is `{"status":"conflict","detail":"a sync cycle is already running"}` |
| `503 Service Unavailable` | Replica outside Fly.io (no `FLY_REGION`) cannot forward the request |

Request body is capped at 1 MB.

Bypasses the readiness middleware so an operator can kick off the first sync
before any sync has completed.

## Limits

PeeringDB Plus has no request rate limit.
It enforces these limits:

| Limit | Scope | Default | Configured by |
|-------|-------|---------|---------------|
| Request body size | Every HTTP request body. For ConnectRPC, each message | `1 MB` | `maxRequestBodySize` in `main.go` (hardcoded) |
| Read header timeout | Every connection | `10s` | `buildServer` (hardcoded) |
| Read timeout | Every connection | `30s` | `buildServer` (hardcoded) |
| Idle timeout | Every keep-alive connection | `120s` | `buildServer` (hardcoded) |
| Stream timeout | ConnectRPC `Stream{Type}` | `60s` | `PDBPLUS_STREAM_TIMEOUT` |
| Graceful drain timeout | Shutdown | `10s` | `PDBPLUS_DRAIN_TIMEOUT` |
| Sync memory ceiling | Sync worker heap | `400MB` | `PDBPLUS_SYNC_MEMORY_LIMIT` |
| Response memory budget | pdbcompat `/api/` | `128MiB` | `PDBPLUS_RESPONSE_MEMORY_LIMIT` |
| GraphQL query complexity | `POST /graphql` | `1,000,000` (weighted) | `FixedComplexityLimit(graph.ComplexityLimit)` |
| GraphQL query depth | `POST /graphql` | `15` | `FixedDepthLimit` (hardcoded) |
| GraphQL page size | `first`, `last`, `limit` | `100`, maximum `1000` (a larger value is an error) | `graph/pagination.go` (hardcoded) |
| REST `per_page` | `/rest/v1/` lists | `10`, maximum `100` (a larger value returns `400`) | entrest (generated) |
| ConnectRPC `page_size` | List RPCs | `100`, maximum `1000` (a larger value is clamped) | `internal/grpcserver/pagination.go` (hardcoded) |
| MCP `page_size` | Search and relation pages | `20`, maximum `100` | `internal/mcpserver` (hardcoded) |

Upstream PeeringDB rate limits apply to the sync worker's outbound requests —
setting `PDBPLUS_PEERINGDB_API_KEY` raises that ceiling.

Deployment-level rate limiting
(e.g., Fly.io edge, Cloudflare, or a load balancer)
is not configured in this repository.

## CORS

All surfaces pass through the shared `middleware.CORS` configured by
`PDBPLUS_CORS_ORIGINS` (default `*`).
The middleware allows the full set of headers required by Connect / gRPC /
gRPC-Web and MCP Streamable HTTP in addition to standard application headers;
see
`internal/middleware/cors.go`.
The MCP handler also checks browser `Origin` values against
`PDBPLUS_CORS_ORIGINS`.
With the default `*`, it rejects only malformed values (see § 6. MCP).
The REST subtree relies on this same outer middleware;
it is not wrapped a second time.

## Known Divergences

PeeringDB Plus strives for behavioural parity with the upstream PeeringDB API
(`peeringdb/peeringdb`) at the `/api/` surface.
The remaining divergences are listed below,
each with an upstream citation and a guarding test.
The shape of detail responses at `?depth=0`, `1` and `2` matches upstream,
including `_set` ID lists, `net_set`, back-reference removal and `campus: null`.
`internal/pdbcompat/depth_test.go` locks this.

| Request | Upstream behaviour | peeringdb-plus behaviour | Rationale | Since |
|---------|-------------------|-------------------------|-----------|-------|
| `?depth=` on list endpoints; `?depth=3`/`4` on detail | Lists accept `?depth=` capped at `API_DEPTH_ROW_LIMIT=250` (2.83.0 `rest.py:484` default, enforced `rest.py:766-771`); detail expands a third sub-level at depth 3–4. | List `?depth=` is silently dropped (`slog.DebugContext` paper trail; `opts.Depth` never threaded into list closures). On detail, depths `0`/`1`/`2` match upstream exactly; `3`/`4` are accepted and clamped, rendering the depth-2 shape — the third sub-level is not reproduced. | The 256 MB replica response budget cannot absorb depth-expanded list rows; depth>2 sub-nesting is data almost no client reads three levels deep. Locked by `TestParity_Limit/depth_on_list_silently_dropped_DIVERGENCE` and `TestDepth_DepthOne/depth_clamped_to_0_4`. | v1.16 (list) · v1.20.5 (detail 3–4) |
| `?depth=1` (and the nested `net.poc_set` at depth=2) `poc_set` ID lists | Upstream lists every POC id in the ID list regardless of visibility, filtering non-`Public` POCs only when they are expanded to objects at depth=2. | The row-level `poc.visible` privacy policy applies uniformly, so a `poc_set` ID list (and the objects at depth=2) never contains a POC id that the caller's tier cannot read: non-`Public` ids for anonymous callers, `Private` ids for the Users tier (`PDBPLUS_PUBLIC_TIER=users`, since v1.28.0). The mirror is **stricter** than upstream here. | Leaking the ids/existence of hidden contacts would contradict the load-bearing `poc.visible` policy (see § Field-level privacy). Intentional. Locked by `TestDepth_PocSetPrivacy_DIVERGENCE`. | v1.20.5 |
| `/api/poc/<id>` for a contact that the caller's tier cannot read: a `Users` contact for an anonymous caller, or a `Private` contact for any tier | Returns `403`. `retrieve` (2.83.0 `rest.py:849-865`) returns `HTTP_403_FORBIDDEN` when the permission applicator denies the object. The upstream tests expect it for a guest and a `Users` contact (`pdb_api_test.py:3855-3856`) and for a user who is not a member of the owning organization and a `Private` contact (`:1413-1414`), through `assert_get_forbidden` (`:575-577`). | Returns `404`, the same as for an id that does not exist. REST, ConnectRPC and GraphQL also use their not-found forms. The list content matches upstream: both leave the contact out. For the status code of `/api/poc?id=<id>`, see the `/api/poc?id=` row. | A `403` shows that a hidden contact with this id exists. The mirror does not show it. Locked by `TestParity_Status/DIVERGENCE_hidden_poc_detail_404`. | v1.14 (anonymous) · v1.28.0 (Users tier, `Private` contact) |
| `ixlan.ixf_ixp_member_list_url` with no stored URL, for a caller that may see it | Emits the key with the stored value. The column is nullable (django-peeringdb `abstract.py:819-824`), so the value is `null` or `""`. The key is deleted only when the caller does not have the permission (2.83.0 `permissions.py:344-353`). | Emits `""` for a `Public` row, because sync stores `null` and `""` as `""`. Omits the key for an empty `Users` row at every tier. | An anonymous sync does not get the URL of a `Users` row (the key is absent for it), and it stores `""`. For such a row, `""` does not mean that the URL is empty. A column that keeps `null` apart from `""` needs a sync that keeps an absent key apart from a `null` value. Locked by `TestParity_Serializer/DIVERGENCE_ixf_url_empty_value`. | v1.28.0 |
| `/api/netixlan?meta__planned_status_change__date__in=<dates>` | Fails. Upstream parses the whole comma-separated value as one datetime (2.83.0 `rest.py:649`), which raises, and its error handler then fails on `inst[0]` (`:651`), so the response is `400`. For a single date, the parse succeeds and `v.split(",")` on the datetime (`:666`) raises an unhandled error (`500`). | Returns the rows whose date is in the list. | A list of whole dates has one clear meaning, and failing it has no value for a client. Locked by `TestParity_Meta/DIVERGENCE_date_in_filters`. | v1.28.0 (registered 2026-09-23) |
| `?status=deleted&since=N` for a row that upstream deleted before the mirror's first sync, or that sync hard-deleted before v1.16 | Returns the tombstone with its deletion timestamp. | Returns empty until the history sweep has fetched the id window of the row. After that, returns the tombstone. A `poc` tombstone older than 30 days does not come back: the sweep skips `poc`, and upstream removes those tombstones. | A bare list holds only live rows, so the first sync gets no tombstones. The history sweep fetches them over many cycles (see [ARCHITECTURE.md § History sweep](ARCHITECTURE.md#history-sweep)). Locked by `TestParity_Status/list_since_admits_deleted_excludes_pending_noncampus` and `TestSync_HistorySweepLandsTombstones`. | v1.16; history sweep added (Unreleased) |
| `?limit=<negative>`, for example `?limit=-1` | Serves the full list. `int('-1')` parses (2.83.0 `rest.py:515-518`), and the `if limit > 0` gate (`rest.py:757-760`) then applies no slice, as for `limit=0`. | Returns `400` (`'limit' needs to be a non-negative number`), the same as a non-numeric `limit`. | A negative page size is a client bug. Serving the full table for it turns a paging loop into repeated full-table dumps. `limit=0` stays the explicit request for every row. Locked by `TestParity_Limit/DIVERGENCE_negative_limit_returns_400`. | v1.21.0 (registered 2026-09-23) |
| `?<field>__contains=`/`__startswith=` with a non-ASCII value, in the ≤1 sync interval after a fresh deploy | Folds via `unidecode.unidecode(v)` at query time (2.83.0 `rest.py:597`), so it works immediately. | Folding uses a sync-populated `<field>_fold` shadow column, so a one-time ASCII-only window exists between deploy and the first sync (≤1h default) during which non-ASCII queries return no match. ASCII queries work throughout; the next sync's `OnConflict().UpdateNewValues()` closes the window with no backfill. | Shadow columns give SQLite one indexable comparison path (benchstat within ±1% of the direct path) and stay off the GraphQL/REST/proto wire (`entgql.Skip` / `entrest.WithSkip`). Locked by `TestParity_Unicode_FoldWindow_DIVERGENCE`. | v1.16 |
| `GET /api/as_set` (and its entry in the `/api/` index) | Upstream exposes a network-derived AS-SET lookup — `{"data":[{"<asn>":"<irr_as_set>",...}],"meta":{}}` via `ASSetSerializer(NetworkSerializer)` — and lists `as_set` as a 14th endpoint in the `/api/` index. | Not mirrored: `GET /api/as_set` is unrouted, and the `/api/` index lists only the 13 served types (its envelope + absolute-URL shape otherwise matches upstream as of v1.20.6). | The endpoint is an unbounded bulk `asn → irr_as_set` dump across every network, outside the 13-type mirror scope; the same `irr_as_set` is available per-network via `GET /api/net`. Locked by `TestIndex`. | v1.20.6 |
| Any `/api/` error response (4xx/5xx) | `{"meta":{"error":"<detail>"},"data":[]}` with `Content-Type: application/json` — 2.83.0 `renderers.py:134-146` pops `detail` into `meta["error"]`. | RFC 9457 `application/problem+json` (`type`/`title`/`status`/`detail`); no top-level `meta` key at all (`internal/pdbcompat/response.go` `WriteProblem`). Clients parsing `meta.error` on errors must adapt. | A standards-based, machine-readable error shape shared with the other API surfaces (entrest emits problem+json too) beats the bespoke upstream envelope; success responses keep full envelope parity. Locked by `TestParity_Limit/DIVERGENCE_error_envelope_problem_json_not_meta_error`. | v1.1 (registered 2026-06-10) |
| `?q=<term>` on list endpoints | Ignored. The db-field filter loop skips `q` (2.83.0 `rest.py:566`) and no other consumer exists on the `/api` surface (the separate `name_search` parameter triggers the Elasticsearch-backed `search_v2`, `rest.py:532-553`), so any `?q=` value returns the unfiltered list. | Convenience **extension**: case-insensitive substring search (`ContainsFold`) across the type's SearchFields, plus exact-ASN match on `/api/net`. NOT diacritic-folded: `?q=munchen` does not match "München" although `?name__contains=munchen` does (the `_fold` shadow routing applies to the field filters only). | A search that narrows results is strictly more useful than upstream's silent no-op, and clients relying on upstream behaviour (unfiltered dump) should not be sending `?q=` at all. Fold-routing `?q=` would multiply every SearchFields OR-branch across the `_fold` shadows for marginal benefit; revisit if a real client asks. Locked by `TestParity_Unicode_QSearchExtension_DIVERGENCE`. | v1.1 (registered 2026-07-10) |
| Filters on upstream model columns that the API does not serialize: `org_flags` on `org`; `geocode_status` and `geocode_date` on `org` and `fac`; `location_method` and `location_place_id` on `fac` (new in 2.83.0); `ixf_import_request_user` on `ix`; and their `<fk>__<field>` forms, for example `net?org__org_flags=`, `netfac?fac__location_method=` and `netixlan?ix_side__location_method=` | Filters on them. The filter loop accepts every model field (2.83.0 `rest.py:525-528`), and `queryable_relations` adds `<fk>__<field>` for each FK (`serializers.py:970-996`). Columns: `models.py:1259-1264` (`org_flags`), `:591-599` (`geocode_*`), `:2207-2217` (`location_*`), `:2612` (`ixf_import_request_user`). | Silently ignored, like any unknown key: HTTP 200 with the unfiltered list. | The mirror stores only the fields that the API serializes. It never receives these values, so it cannot filter on them. Locked by `TestParity_Traversal/DIVERGENCE_unserialized_model_columns_silent_ignore`. | v1.1 (registered 2026-09-23) |
| Query keys that upstream handles in Python and that are not model fields: the `prepare_query` keys (`fac`: `asn_overlap`, `org_present`, `org_not_present`, `all_net`, `not_net`, `distance`; `net`: `not_ix`, `not_fac`; `ix`: `ipblock`, `asn_overlap`, `all_net`, `not_net`, `org_present`, `org_not_present`, `capacity`; `org`: `distance`; `ixpfx`: `whereis`); `hide_ix_no_fac` on `ix`, `ixlan`, `net` and `netixlan`; and `name_search` | Filters in Python before the model-field filters: `prepare_query` (2.83.0 `serializers.py:2092-2210` for `fac`, `:3708-3762` for `net`, `:4503-4631` for `ix`, `:4970-4992` for `org`, `:4154-4168` for `ixpfx`), the `hide_ix_no_fac` mixin (`rest.py:1267-1297`), and the search index for `name_search` (`rest.py:532-553`). `asn_overlap` returns `400` for one ASN or for more than 25 (`models.py:2457-2461` and `:2867-2871`). `distance` returns `400` without a location (`serializers.py:1856-1865`). | Silently ignored, like any unknown key: HTTP 200 with the unfiltered list. `?asn_overlap=` and `?distance=` never return `400`. | Each key needs its own query logic: presence through join tables, IP prefix containment, spatial distance, or a search index. The relation keys of a `prepare_query` are not in this row: they filter as upstream does (see § Relation filters). The model-field filters that pdbcompat mirrors do not cover them. Implement a key when a client needs it. Locked by `TestParity_Traversal/DIVERGENCE_prepare_query_keys_silent_ignore`. | v1.1 (registered 2026-09-23) |
| `/api/poc?since=N` when the window covers a contact that was deleted more than 30 days ago | Hard deletes a soft-deleted contact when its `updated` value is `POC_DELETION_PERIOD` old (default 30 days: 2.83.0 `management/commands/pdb_delete_pocs.py:34-38,58`, `mainsite/settings/__init__.py:684`). After that, the window does not return its tombstone. Upstream documents this as its only hard delete (`docs/api/obj_poc.md:14-17`, `docs/api/api_description.md:11-13`), but `pdb_rir_status` also hard-deletes connections (next row). | Keeps every `poc` tombstone, so the window still returns it. The tombstone shows `name`, `phone`, `email` and `url` blanked, as upstream shows every deleted contact (2.83.0 `serializers.py:2941-2954`). | Sync never infers a delete from a missing row (see § Soft-delete tombstones), and tombstone GC is dormant. A client that applies the tombstone deletes a contact that upstream also no longer has. Locked by `TestParity_Status/DIVERGENCE_poc_tombstone_outlives_upstream_retention`. | v1.16 (registered 2026-09-23) |
| `/api/netixlan?since=N` when N is not later than the `updated` value of a connection that upstream removed with its deleted network | A `?since` list returns nothing for the connection, and `?id=<id>&since=N` returns `404` `Entity not found` (2.83.0 `rest.py:809-815`). `pdb_rir_status` removes the live connections (`ok`, `not-operational`) of a network whose ASN the RIR reclaimed with an SQL delete, then soft-deletes the network (`management/commands/pdb_rir_status.py:440-443`, `models.py:5720-5725`). The connection gets no tombstone, although the API docs say that only contacts are hard-deleted (`docs/api/api_description.md:11-13`). | Returns the connection with `status` `deleted`, `operational` `false` and its last `updated` value, and `200` for the `id` query. Sync marks a live connection of a deleted network `deleted` when the RIR reclaim deleted the network in this sync, or when an uncached `?since=1&id__in=` request no longer returns it live. It does not change `updated`, and it leaves live a connection that upstream changed after the delete. Lists without `?since`, depth sets, relation keys, `/api/netixlan/<id>` and `?id=<id>` leave it out, as upstream does. | Without a tombstone, the mirror would serve the connection as live forever. A client that applies the tombstone deletes a connection that upstream no longer has. A window that starts after the row's `updated` value does not return it. Locked by `TestParity_Status/DIVERGENCE_deleted_net_netixlan_tombstone_in_since_window` and `TestSync_CascadesDeletedNetIxLans`. | v1.28.2 |
| `/api/poc?since=N&fields=<list>` for a deleted contact when `<list>` leaves out `status`, for example `fields=id,name,email` | Returns the stored `name`, `phone`, `email` and `url`. The serializer removes the fields that `?fields=` does not name (2.83.0 `serializers.py:942-950`) and blanks the contact fields only when the rendered `status` is `deleted` (`:2941-2954`). A soft delete does not blank the stored values. | Returns `""` for these fields. pdbcompat blanks them before it applies `?fields=`, and sync stores a deleted contact without them. The mirror is **stricter** than upstream here. | The contact data of a deleted contact must not reach a caller, whatever fields the caller asks for. Locked by `TestParity_Status/DIVERGENCE_deleted_poc_blanked_without_status_field`. | v1.16 (registered 2026-09-23) |
| `netixlan?ix_side__<field>=`, for example `?ix_side__name=` | Filters on the facility. `ix_side` is a FK to `Facility` (2.83.0 `models.py:6095-6101`), so `queryable_relations` adds its `ix_side__<field>` keys (`serializers.py:970-996`). | Silently ignored, like any unknown key: HTTP 200 with the unfiltered list. `?ix_side=` and `?ix_side_id=` filter as usual. | The mirror stores `ix_side_id` but models no edge from netixlan to the facility, so traversal has nothing to walk. Add the edge when a client needs these keys. The `net_side` keys are parity: upstream renames them to `network_side`, which matches no field (`serializers.py:428-438`), and ignores them. Locked by `TestParity_Traversal/DIVERGENCE_netixlan_ix_side_facility_keys_silent_ignore` and `net_fac_renamed_keys_ignored_like_upstream`. | v1.1 (registered 2026-09-23) |
| Relation keys that upstream ignores: 2-hop keys, for example `ixpfx?ixlan__ix__id=` and `netixlan?net__org__name=`; reverse keys named by the mirror's traversal key, for example `org?net__name=`; and the field-level `FILTER_EXCLUDE` entries `org__latitude`, `org__longitude` and `ixlan__descr`, for example `fac?org__latitude__gt=` | Resolves one relation hop: `queryable_relations()` adds `<fk>__<field>` and `<related_name>__<field>` (2.83.0 `serializers.py:970-996`), and a `prepare_query` handles the keys whose first segment is in its seed list (`:614-656`). Other keys are ignored, and the list is unfiltered: `org?net__name=` becomes `network__name` (`serializers.py:403-441`), which is not a filter key (`rest.py:525-528`, `:670`). A key whose first segment a `prepare_query` handles is a relation filter (see § Relation filters). | Resolves each key through the Path A allowlist or the Path B edges and filters on the related rows as given, without a status check on them. The exception is `status`: a 2-hop key or a reverse key never filters it, so `org?net__status=` and `netixlan?net__org__status=` are ignored, as upstream. A field that is not a model field upstream is never a target either (see § Cross-entity traversal). | A 2-hop key and a reverse key have one clear meaning on the mirror's edges, and the extra filters cost one subquery each. Locked by `TestParity_Traversal/DIVERGENCE_relation_keys_upstream_ignores_resolve` and `TestParity_Traversal/DIVERGENCE_path_a_2hop_ixpfx_via_ixlan_ix_id`. The `status` keys are parity, locked by `TestParity_Traversal/status_on_reverse_and_2hop_keys_ignored_like_upstream`. | v1.16 (registered 2026-09-23) |
| Reverse keys by upstream's `<related_name>`, for example `ix?ixlan_set__status=`, `org?ix_set__name=`, `org?ix_set__in=` and `org?ix_set=` | Filters on the related rows. A reverse relation reports the `ForeignKey` type, so `field_names` holds the bare related name, and `queryable_relations()` adds `<related_name>__<field>` (2.83.0 `serializers.py:970-996`; related names at `models.py:3308` and `:2621-2622`). `<related_name>__<field>` is an exact match on the related rows, with the match rules of the field type (`rest.py:670-683`). `<related_name>__in`, `__lt`, `__lte`, `__gt` and `__gte` compare the related row ids (`rest.py:633-669`). A bare `<related_name>` returns `400`: `rest.py:676-677` builds `<related_name>_id`, which Django cannot resolve (`:702-703`). Upstream ignores the related names that start with `net_` or `fac_` (`net_set` and `fac_set` on `org`, `fac_set` on `campus`, `net_side_set` on `fac`): `queryable_field_xl` renames them to `network_*` and `facility_*` (`serializers.py:428-438`), and these names match no relation. | Silently ignored: HTTP 200 with the unfiltered list. The mirror names a reverse edge by its traversal key instead (`org?ix__name=`). | The traversal keys cover the same relations. Accepting both names would double the key surface for no new query. Locked by `TestParity_Traversal/DIVERGENCE_reverse_set_keys_silent_ignore`. The `net_set` and `fac_set` keys are parity, locked by `TestParity_Traversal/reverse_net_fac_set_keys_ignored_like_upstream`. | v1.16 (registered 2026-09-23) |
| `?<field>__iexact=`, `__icontains=` and `__istartswith=`, for example `netixlan?status__iexact=OK`, `net?org__iexact=1` and `net?ix__name__icontains=x` | Ignored. The operator regex (2.83.0 `rest.py:616`) knows only `lt`, `lte`, `gt`, `gte`, `contains`, `startswith` and `in`, so a key with another suffix is not a filter key (`:628-630`, `:670`), and the list is unfiltered. On a relation key of a `prepare_query` (§ Relation filters), `get_relation_filters` does not parse the suffix (`serializers.py:614-656`). A 3-segment key, for example `net?ix__name__icontains=`, drops the suffix and runs an exact match, and so do the `netfac` and `ixfac` keys `name`, `country` and `city`. A status of the pinned row is then replaced with `ok`. A 2-segment key, for example `net?ixlan__iexact=`, keeps the suffix as a Django lookup on the relation, which raises `FieldError`, so upstream returns `400` (`rest.py:488-500`). `netixlan?name__iexact=` filters the name of the ixlan with the lookup (`serializers.py:3161-3169`), and `fac?org_name__iexact=` is ignored (`:2108-2117`). | Applies the suffix: an exact, substring or prefix match that ignores case. On a field that is not a string, and on a key that names a FK, `__iexact` is an exact match, and `__icontains` and `__istartswith` return `400`. The `netixlan` `name` keys and the `fac` `org_name` keys with these suffixes match upstream (see § Relation filters). | These suffixes have one clear meaning, and `contains` and `startswith` already ignore case. The metadata keys accept only the upstream operators (see § Metadata filters). Locked by `TestParity_Status/DIVERGENCE_i_operator_suffixes_filter`. | v1.16 (registered 2026-09-23) |
| `<rel>__<field>__contains=` and `<rel>__<field>__startswith=` on a relation key of a `prepare_query`, for example `netixlan?ix__name__contains=` and `net?ix__name__startswith=` | Case-sensitive. `get_relation_filters` keeps the raw operator for a 3-segment key (2.83.0 `serializers.py:643-654`), and Django runs `contains` and `startswith` as `LIKE BINARY` on MySQL. Only the 2-segment form becomes `icontains` or `istartswith` (`:622-626`). | Ignores case, and folds diacritics on the fields that have a `_fold` column, as for every other `contains` and `startswith` key. | One rule for every `contains` and `startswith` key. SQLite has no `LIKE BINARY` on the fold columns. Locked by `TestParity_Traversal/DIVERGENCE_relation_field_contains_ignores_case`. | v1.28.0 (registered 2026-09-23) |
| `campus?facility__<field>=`, `campus?facility__in=` and the other `campus?facility` keys that match more than one facility of a campus | Returns the campus once for each matching facility. `related_to_facility` filters the reverse `fac_set` join (2.83.0 `models.py:2101-2111`). `_filters_may_produce_duplicates` looks up `facility`, which is not a `Campus` field, so upstream does not call `distinct()` (`rest.py:115-141`, `:715-716`). Found by reading the code, not checked against a live upstream. | Returns each campus once. | A list with repeated rows has no use to a client, and `distinct()` is what upstream runs for the other reverse joins. Locked by `TestParity_Traversal/DIVERGENCE_campus_facility_keys_no_duplicates`. | v1.28.0 (registered 2026-09-23) |
| A relation key of a `prepare_query` whose field the related row does not have, for example `fac?net__bogus=1` or `net?ix__fac_count=1`. This includes a serializer field or a model property that the mirror stores, for example `net?netfac__name=`, `net?netfac__city=`, `ix?ixfac__name=` and `net?netixlan__name=` | Returns `400` (`Invalid query`): the Django filter raises `FieldError` inside `prepare_query` (2.83.0 `rest.py:488-500`). `net?netfac__name=` runs `NetworkFacility.filter(name=...)`, and `name` is a serializer field (`serializers.py:3372-3380`). | Silently ignored, like any unknown key: HTTP 200 with the unfiltered list. | Every other unknown filter key is ignored, and one rule is easier to use. Upstream returns `400` for a bad value on a known field, and so does the mirror (`net?ix=abc`). Locked by `TestParity_Traversal/DIVERGENCE_relation_key_unknown_field_silent_ignore`. | v1.28.0 (registered 2026-09-23) |
| Filters on a single-object GET, for example `/api/netixlan/<id>?status=ok` for a `not-operational` row or `/api/net/<id>?name=<other>` | Applies them. `retrieve` (2.83.0 `rest.py:849-855`) calls DRF `get_object`, which filters `get_queryset()`: the same query-parameter filters as a list (`rest.py:565-703`) plus the live-or-pending PK status set (`:750`). A filter that excludes the object returns `404`. | Reads only `?depth=` and `?fields=` on a detail request. Every other key is ignored, and the object is returned. | A detail request stays a plain PK lookup. To test one object against a filter, list with `?id=<id>&<filter>=`, which returns `404` when the filter excludes the object. Locked by `TestParity_Status/DIVERGENCE_detail_ignores_filters`. | v1.1 (registered 2026-09-23) |
| `/api/netixlan` without `?since` | Returns the rows in the order of the MySQL access path, because the query has no `ORDER BY` (2.83.0 `rest.py:747-748`). MySQL can read the `status IN ('ok', 'not-operational')` filter through the `netixlan_status` index (`models.py:6111`), which returns the `not-operational` rows first. A live capture on 2026-09-23 of `/api/netixlan?limit=2` returned two `not-operational` rows ahead of `ok` rows with lower ids. Other filters can also change the upstream order when MySQL reads the rows through a different index. | Always returns `id` order, ascending (see § List order). | An order that depends on the query plan cannot be reproduced, and it changes when the upstream data or indexes change. A fixed order keeps `skip`/`limit` pages stable. Locked by `TestParity_Ordering/DIVERGENCE_netixlan_list_order_is_id_asc`. | v1.28.0 (registered 2026-09-23) |
| `/api/poc?id=<id>` for a contact that the caller's tier cannot read: a `Users` contact for an anonymous caller, or a `Private` contact for any tier | Returns `200` with an empty `data` array. The unique-query `404` check runs before `APIPermissionsApplicator` removes the contact (2.83.0 `rest.py:809-821`), so the status shows that the contact exists: a missing id gets `404`. The same applies to a user who is not a member of the organization that owns a `Private` contact. | Returns `404` (`Entity not found`), the same as for a missing id. | The `poc.visible` privacy policy removes the contact in the query, before the check, so the status does not show whether a hidden contact exists. The mirror is stricter than upstream here. Locked by `TestParity_Status/DIVERGENCE_poc_hidden_id_returns_404`. | v1.28.0 (registered 2026-09-23) |
| `?id=` on any type, or `?asn=` on `/api/net`, with a value that is not an integer, for example `?id=abc` or `?asn=` | Returns `404` (`Entity not found`). A plain key on a field that is not a relation, a date or a boolean becomes an `__iexact` filter (2.83.0 `rest.py:670-683`). Django does not convert an `iexact` value to an integer, so the query matches no row, and the key makes the request a unique query that gets the `404` (`rest.py:809-815`). | Returns `400` (`convert "abc" to int`), as for every other integer filter with a value that is not an integer. | The mirror checks the type of each integer filter value before it runs the query, and reports a bad value as a client error. Locked by `TestParity_Status/DIVERGENCE_unique_key_non_integer_returns_400`. | v1.1 (registered 2026-09-23) |

## Validation Notes

Future conformance auditors reading third-party gotchas documentation
(notably pdbfe's upstream-behaviour claims)
against the PeeringDB Plus codebase may encounter assertions about upstream
behaviour that turn out to be wrong.
This section documents 4 such invalid claims from the v1.16 audit,
each with a pinned `peeringdb/peeringdb@<sha>` reference
so the authoritative upstream source can be re-read without re-research.
All 4 were re-confirmed against commit
`peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516` (PeeringDB
2.83.0), the parity anchor as of 2026-09-23.

| Claim | Verdict | Upstream truth | Our implementation |
|-------|---------|----------------|--------------------|
| `net?country=NL` is a valid filter key | **WRONG** | `country` lives on `org`, not `net`. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/serializers.py:3708` — `NetworkSerializer.prepare_query` has no `country` key, and `django-peeringdb/src/django_peeringdb/models/abstract.py`'s Network model has no country field. Callers who want `net` filtered by country must traverse through `org` (e.g. `net?org__country=NL`). | Filter key silently ignored via the unknown-field silent-ignore mechanism — no row-level match, response unfiltered. OTel span attribute `pdbplus.filter.unknown_fields` records the dropped key. Parity-locked by `TestParity_Traversal/unknown_field_silently_ignored_with_otel_attr`. |
| `?limit=0` returns a count-only envelope | **WRONG** | `limit=0` means unlimited. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/rest.py:516` (`limit` defaults to `0`) + `:757-760` (`if limit > 0: qset[skip:skip+limit] else: qset[skip:]`: a non-positive `limit` applies no SQL `LIMIT`). There is no count-only semantic upstream. Callers wanting a count read the length of the returned `data` array (`meta` is the empty `{}` envelope, with no top-level count field). | Unbounded response: when `limit` is `0`, the list query has no SQL `LIMIT` (`wireEntity` in `internal/pdbcompat/registry_funcs.go`). The memory budget (`PDBPLUS_RESPONSE_MEMORY_LIMIT`, default 128 MiB) still applies and returns RFC 9457 `413` when the response is too large. Parity-locked by `TestParity_Limit/bare_url_and_zero_both_return_all_rows` and `TestParity_Limit/zero_over_budget_returns_413_problem_json`. |
| Unicode folding uses MySQL collation (`utf8_general_ci` or similar) | **WRONG** | Folding is Python-side via `unidecode.unidecode(v)` at query time. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/rest.py:597` — the call happens in the Python filter construction layer before any SQL is emitted, so the database collation is irrelevant. | peeringdb-plus uses shadow `<field>_fold` columns populated at sync time via `internal/unifold.Fold` (`golang.org/x/text/unicode/norm` NFKD decomposition + a hand-rolled ligature map for `ß`/`æ`/`œ`/`ø`/`ł`/`þ`/`đ`/`ð`/dotless `ı`). `__contains` / `__startswith` route to `<field>_fold LIKE ?` with `unifold.Fold(query)` on the RHS. Not byte-compatible with Python `unidecode` for every input (e.g. the two libraries handle rare CJK edge cases differently); any specific gap that surfaces will be logged as a new § Known Divergences row. Parity-locked by `TestParity_Unicode/net_name_contains_diacritic_matches_ascii`, `fac_city_cjk_roundtrip`, and `combining_mark_NFKD_equivalent`. |
| Filter surface is a DRF `filterset_class` per ViewSet | **WRONG** | Filter surface is a per-serializer `prepare_query(...)` method plus an auto-`queryable_relations()` mechanism with a `FILTER_EXCLUDE` denylist. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/serializers.py:970` (`queryable_relations()`) and `:136-166` (`FILTER_EXCLUDE`). No `django_filters.FilterSet` subclass exists anywhere in the upstream codebase. | Path A = `pdbcompat.WithPrepareQueryAllow` ent-schema annotations → `allowlist_gen.go` `Allowlists` map (13 entries derived from upstream `prepare_query` seed lists and `queryable_relations()`); Path B = ent edge introspection via the generated `Edges` map. The `WithFilterExcludeFromTraversal` edge annotation is the edge-level counterpart of upstream's `FILTER_EXCLUDE` — currently empty across all 13 schemas (every FK edge exposed in v1.16). Upstream's field-level entries have no counterpart (see § Known Divergences). Parity-locked by `TestParity_Traversal/path_a_1hop_org_name` and `path_b_1hop_org_city`. |

An earlier revision of this section also listed one claim from the
2026-05-30 audit as wrong: that `org_flags` is a valid filter on `/api/org`.
The claim is correct.
`org_flags` is an upstream model column (2.83.0 `models.py:1259-1264`).
Upstream filters on it but does not serialize it,
so the mirror ignores the key.
See § Known Divergences.

An earlier revision also listed the claim that the default list order is
`id ASC` as wrong.
It cited the `django-handleref` base `Meta.ordering = ('-updated', '-created')`.
The claim is correct.
The django-peeringdb abstract bases declare their own `class Meta`
without subclassing the handleref `Meta`,
so the 13 models do not inherit that ordering.
The upstream migrations record no `ordering` option for them
(2.83.0 `migrations/0001_initial.py`).
A list without `?since` has no `ORDER BY` (`rest.py:747-748`),
and MySQL returns primary-key order.
A live capture on 2026-09-23 returned the lowest ids first on every type
except netixlan.
See § List order and § Known Divergences.

Quarterly re-validation against upstream is a manual review against the pinned
commit above — it does not block merges.
Drift that invalidates a Validation Note row should be surfaced
as a GitHub issue and reviewed against the parity test suite;
if upstream has changed semantics,
update the row here and flip or retain the matching parity assertion
as a new § Known Divergences row.
