# API Reference

PeeringDB Plus exposes the mirrored PeeringDB dataset through **six coexisting API surfaces** served from the same process on the same port, plus a small set of infrastructure endpoints for health, on-demand sync, and service discovery.
This document is the comprehensive reference; see `README.md` for a one-page overview and `docs/ARCHITECTURE.md` for the rationale behind each surface.

All routes are registered in `cmd/peeringdb-plus/main.go` and pass through the production middleware chain:

```text
Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Recovery -> Logging ->
PrivacyTier -> Readiness -> SecurityHeaders -> CSP -> Caching -> Gzip ->
RouteTag -> mux
```

The server speaks HTTP/1.1 and h2c (HTTP/2 cleartext) on the same listener so that Connect, gRPC, and gRPC-Web clients can use the same base URL as browser and CLI clients.

## Authentication

Most endpoints are **unauthenticated and read-only** — they expose the same public data that PeeringDB itself publishes.

| Endpoint | Authentication |
|----------|----------------|
| Read endpoints (Web UI, GraphQL, REST, `/api/`, ConnectRPC, MCP) | None |
| `POST /sync` | `X-Sync-Token` header must match `PDBPLUS_SYNC_TOKEN` (constant-time compare) |
| Upstream fetch from `api.peeringdb.com` | Optional — set `PDBPLUS_PEERINGDB_API_KEY` to use an authenticated client with higher rate limits |

If `PDBPLUS_SYNC_TOKEN` is empty at startup the sync endpoint logs a warning and rejects every request as `401 unauthorized` — there is no "accept anything" mode.
Replica instances reject the request with a `fly-replay` header that routes the request to the primary region on Fly.io, or return `503 not primary` when running outside Fly.io.

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
| `GET` | `/api/` | PDB Compat | Index of the object types and the as_set lookup (upstream `{"data":[{name:absolute-url}],"meta":{}}` shape) |
| `GET` | `/api/{type}` | PDB Compat | List endpoint with PeeringDB-compatible filters |
| `GET` | `/api/{type}/{id}` | PDB Compat | Single object (wrapped in `data: []`) |
| `GET` | `/api/as_set` | PDB Compat | Map of the ASN of each `ok` network to its `irr_as_set`, for the networks that have one |
| `GET` | `/api/as_set/{asn}` | PDB Compat | The `irr_as_set` of the network with this ASN, in any status |
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

The 13 entity types mirrored from PeeringDB are: `campus`, `carrier`, `carrierfac`, `fac`, `ix`, `ixfac`, `ixlan`, `ixpfx`, `net`, `netfac`, `netixlan`, `org`, `poc`.

The application server has no `/metrics` route.
By default, the process sends metrics through OTLP to the configured collector.
If you set `OTEL_METRICS_EXPORTER=prometheus`, the OpenTelemetry autoexport library starts a separate listener that serves `/metrics` on `OTEL_EXPORTER_PROMETHEUS_HOST`:`OTEL_EXPORTER_PROMETHEUS_PORT` (default `localhost:9464`).
See [CONFIGURATION.md](CONFIGURATION.md#standard-opentelemetry-variables-autoexport).

### Before the first sync

Until the first sync completes, every route returns `503 Service Unavailable`, except `/`, `/healthz`, `/readyz`, `/sync`, `/favicon.ico`, `/static/*` and `/grpc.health.v1.Health/*`.
This includes `/mcp` and the agent discovery files that `GET /` lists.
A browser gets a syncing page.
A terminal client gets text.
Other clients get `{"error":"sync not yet completed"}` as `application/json`.
On `/api/`, every client gets `{"meta":{"error":"sync not yet completed"}}` as `application/json`, the error form of that API (see § Errors in section 4).
ConnectRPC and gRPC clients get `UNAVAILABLE`.
To know when the server is ready, poll `/readyz` or the gRPC health service.

## Row status on each surface

The mirror stores rows of every status that upstream sends: `ok`, `pending`, `deleted`, and `not-operational` on netixlan.

- `/api/` applies the upstream status matrix.
  See § Soft-delete tombstones.
- GraphQL, REST and ConnectRPC apply no status filter.
  Their lists, streams and single-object lookups also return `deleted` and `pending` rows.
- To get only live rows from a list, filter on `status`: GraphQL `where: {status: "ok"}`, REST `?status.eq=ok`, or ConnectRPC `"status": "ok"`.
  On netixlan, use GraphQL `where: {statusIn: ["ok", "not-operational"]}` or REST `?status.in=ok&status.in=not-operational`.
  The ConnectRPC `status` filter takes one value, so send one request for each status.
- Single-object lookups take no status filter: GraphQL `node`, REST `/rest/v1/{collection}/{id}` and ConnectRPC `Get{Type}`.
  Read `status` in the response.
- GraphQL `networkByAsn` returns only `ok` and `pending` networks.
- The Web UI and the MCP tools do not show `deleted` rows.

### netixlan `not-operational` status

PeeringDB 2.83.0 adds the netixlan status `not-operational`.
A connection that was `ok` with `operational=false` now has this status.
Upstream derives `operational` from the status (`status == 'ok'`), and it treats `ok` and `not-operational` as live statuses (`models.py:109-122`).

- The Web UI, the ASN comparison and the MCP tools list a `not-operational` connection the same as an `ok` one.
  Its speed counts toward the aggregate bandwidth of the network or exchange.
  The Web UI marks it `not operational`; see § IX connection markers.
- GraphQL, REST and ConnectRPC apply no status filter (see § Row status on each surface).
  They return the stored `status` and `operational` values unchanged.
  A `status` filter for `ok` does not return these connections.
- For `/api/`, see § Soft-delete tombstones.

## 1. Web UI (`/ui/`)

The Web UI is implemented in `internal/web/` using [templ](https://templ.guide) for type-safe HTML templates and [htmx](https://htmx.org) for interactive behavior without a JavaScript build pipeline.
The same URL space can render HTML, ANSI-colored terminal text, plain text, or JSON depending on client characteristics.

### Content negotiation

The server inspects each request through `internal/web/termrender.Detect` (`internal/web/termrender/detect.go`) and picks a render mode based on:

1. `?T` or `?format=plain|json|whois|short` query parameter (highest priority)
2. `Accept` header (`text/plain` → rich terminal, `application/json` → JSON)
3. `User-Agent` prefix — `curl/`, `Wget/`, `HTTPie/`, `xh/`, `PowerShell/`,
   `fetch` are treated as terminal clients and receive ANSI-colored output
4. `HX-Request` header — htmx fragments are returned without the page shell
5. Default: HTML

`?nocolor` suppresses all ANSI escape codes regardless of mode.

### The curl gotcha

Because `curl` and `wget` are detected by User-Agent, running `curl https://peeringdb-plus.fly.dev/ui/asn/15169` returns **ANSI-colored text intended for a terminal**.
If you pipe this output to a file, a logger, or a tool that does not render ANSI codes, you will see escape sequences like `\x1b[38;5;...`.

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

For machine-readable output, use one of the structured API surfaces (`/api/`, `/rest/v1/`, `/graphql`, or ConnectRPC) instead of scraping `/ui/`.

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

The network IX list, the exchange participant list and the ASN comparison mark a connection as the upstream PeeringDB 2.83.0 network and exchange views do:

| Marker | Shown when |
|--------|------------|
| `not operational` | `status` is `not-operational`, or `status` is `ok` and `operational` is `false` (a row that upstream has not migrated yet) |
| `planned removal <date>` | `meta.planned_status_change.status` is `deleted` |
| `planned activation <date>` | `meta.planned_status_change` is set with any other status |
| `RFC8950` | `meta.rfc8950` is `true` |

HTML shows each marker as a badge with the upstream tooltip.
In the ASN comparison, the badges are in the speed column of each network.
Terminal output shows each marker in brackets, for example `[not operational]`, at every width.
WHOIS output lists only exchange names and shows no markers.
JSON output (`?format=json`) and the MCP relation and comparison rows carry the same data as a `Markers` object (`NotOperational`, `PlannedStatus`, `PlannedDate`, `RFC8950`).
The object is left out when no marker is set.

## 2. GraphQL (`/graphql`)

GraphQL is served by [gqlgen](https://gqlgen.com) wired through [entgql](https://entgo.io/docs/graphql/).
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

- Relay connections, for example `networks(first: 10, after: $cursor, where: {...})`.
  Without `first` or `last`, a connection returns the first 100 rows.
  A `first` or `last` value above 1000 returns an error.
  The default order is `id` ascending.
  `campuses`, `carriers`, `facilities`, `internetExchanges`, `networks` and `organizations` also accept `orderBy: {field: NAME}`.
- Offset lists, for example `networksList(offset: 0, limit: 100, where: {...})`.
  `limit` defaults to 100 and must be 1 to 1000.
  `offset` must be 0 or more.
  These queries set no order, so use a connection for stable paging.
- `networkByAsn(asn: Int!)` returns one network with status `ok` or `pending`,
  or `null`.
- `syncStatus` returns the latest sync record, which can be a running sync:
  `status`, `lastSyncAt`, `durationMs`, `objectCounts` and `errorMessage`.

### Error envelope

Errors are returned in standard GraphQL format with an `extensions.code` field populated by `classifyError` in `internal/graphql/handler.go`:

| Code | Trigger |
|------|---------|
| `NOT_FOUND` | `ent.IsNotFound(err)` |
| `VALIDATION_ERROR` | `ent.IsValidationError(err)` |
| `CONSTRAINT_ERROR` | `ent.IsConstraintError(err)` |
| `INTERNAL_ERROR` | Any other error. This includes a query that does not parse or validate, a query over the complexity or depth limit, and a page-size argument out of range (for example `first: 5000` or `limit: 0`) |

An error from a resolver includes `path`, which points to the field.
An error for the full request, for example a parse error or a complexity-limit error, has no `path`.

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
The schema comes from `ent/schema/` (`graph/schema.graphqls`) and the hand-written `graph/custom.graphql`.

`Network.meta` and `NetworkIxLan.meta` carry the PeeringDB metadata document (added upstream in 2.83.0) as the `Map` scalar.
The key set is open: upstream can add keys, and the mirror stores the document as is, with no schema change.
The field is `null` when no document is stored for the row.

## 3. REST (`/rest/v1/`)

The REST surface is generated by [entrest](https://github.com/lrstanley/entrest) directly from the ent schemas with read-only operations only (`OperationRead` + `OperationList`).

| Path | Description |
|------|-------------|
| `GET /rest/v1/openapi.json` | OpenAPI 3 specification for the full REST surface. Regenerated as part of `go generate ./...` |
| `GET /rest/v1/{collection}` | List resources |
| `GET /rest/v1/{collection}/{id}` | Get one resource |
| `GET /rest/v1/{collection}/{id}/{edge}` | List or get the related resources, for example `/rest/v1/networks/{id}/network-ix-lans` |

A list takes `page` (default 1) and `per_page` (default 10, maximum 100).
A `per_page` value above 100 returns `400`.
`sort` names the field (default `updated`), and `order` is `asc` or `desc` (default `desc`).
The default sort adds `created` and `id` as tiebreakers.
A filter has the form `<field>.<op>=<value>`, for example `asn.eq=13335`, `name.ihas=cloud` or `status.eq=ok`.
For an operator that takes a list, repeat the key: `status.in=ok&status.in=pending`.
The response is `{"page", "total_count", "last_page", "is_last_page", "content"}`.
Each item includes an `edges` object with its related rows.
For each edge that holds a list, the server loads at most 1000 rows for the full response, newest `updated` first.
In a list response, all items share this limit, so an item can show only part of its related rows.
To get every related row, use the edge route, which pages like a list.
The collection paths and the filters come from entrest annotations in `ent/schema/`.
The OpenAPI spec lists every filter.

`meta` on `networks` and `network-ix-lans` is the PeeringDB metadata document, an open JSON object (added upstream in 2.83.0).
A row without a stored document returns `{}`, the same as upstream.
`meta` is not filterable on this surface.

### Error format

Non-2xx responses are rewritten to [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html) by `RESTError` in `internal/middleware/rest_error.go`.
The response `Content-Type` is `application/problem+json`.
The body always has `type` (`about:blank`), `title`, `status` and `instance`.
For a `4xx` response, `detail` gives the entrest error message, for example `bad request: per_page 0 is out of bounds, must be >= 1`.
A `5xx` response has no `detail`, so database error text does not reach the client.

## 4. PeeringDB Compatibility API (`/api/`)

This surface, in `internal/pdbcompat/`, serves the read operations of the PeeringDB REST API.
It serves only `GET` and `HEAD` requests.
`/api/as_set` serves only `GET`, and `HEAD` gets `405`, as upstream.
Other methods get `405`.
The URL structure, the success envelope, the filter operators and the single object in a `data` array match upstream.
A client that only reads can switch to PeeringDB Plus with a change of base URL.
Some filters and some error responses differ.
See § Known Divergences.

### Routes

| Route | Description |
|-------|-------------|
| `GET /api/` | JSON index mapping each of the 13 type names, and `as_set`, to its list endpoint |
| `GET /api/{type}` | List endpoint |
| `GET /api/{type}/{id}` | Single object by numeric ID, wrapped in `data: [ ... ]` (intentional parity with upstream). An `{id}` that is not an integer returns `404`, see § Filters on a single-object GET |
| `GET /api/as_set` | Map of ASN to `irr_as_set` (see § AS-SET lookup) |
| `GET /api/as_set/{asn}` | One ASN to `irr_as_set` pair (see § AS-SET lookup) |

Valid `{type}` values are the same 13 constants defined in `internal/peeringdb/types.go`: `org`, `net`, `fac`, `ix`, `poc`, `ixlan`, `ixpfx`, `netixlan`, `netfac`, `ixfac`, `carrier`, `carrierfac`, `campus`.

### Query parameters

| Parameter | Applies to | Description |
|-----------|------------|-------------|
| `q` | List | Case-insensitive substring search across the type's search fields. For `/api/net`, an ASN literal (e.g. `8075` or `AS8075`) also matches `net.asn` exactly in addition to the text fields. This is a peeringdb-plus **extension**. Upstream ignores `?q=` on `/api` list endpoints. Unlike the field filters, `?q=` does not ignore diacritics. See § Known Divergences |
| `name_search` | Both | Search by name on `org`, `fac`, `ix`, `net`, `campus` and `carrier`, as upstream's search index does. On the other 7 types a non-empty value returns no rows. On a single-object GET, an object that the search does not match returns `404`. See § Name search |
| `limit` | Both | Maximum rows in response. **Default unlimited** when absent, as upstream 2.83.0 `rest.py:516` (`limit` defaults to `0`) + `rest.py:757-760` (no slice when `limit=0`). Bare `/api/<type>` URLs return ALL rows from the filtered queryset; the response is gated only by the response memory budget (see below). Explicit `limit=N`: positive `N` is honored with no upper cap, as upstream (`rest.py:757-758`); `limit=0` is the explicit "unlimited" sentinel. A `limit` that is not an integer, or an empty `limit`, returns `400` (`'limit' needs to be a number`), as upstream (`rest.py:515-518`). A negative `limit` serves every row, as `limit=0` does: upstream slices only when `limit > 0` (`rest.py:757-760`). At `depth` greater than `0`, a list without a filter is an exception, see § Known Divergences. The value is parsed as Python `int()` parses it: spaces at the ends, a sign, Unicode decimal digits and single underscores between digits are accepted. A value too large for a 64-bit integer serves every row. With a repeated `limit`, the last value applies. Constant: `DefaultLimit=0` (`internal/pdbcompat/response.go`). The `?page=N` shape is not supported: clients that want pagination set `?limit=N&skip=M` instead. On a single-object GET, a value above `0` returns `404` (see § Filters on a single-object GET) |
| `skip` | Both | Offset for pagination. The value is parsed as for `limit`. A `skip` that is not an integer, or an empty `skip`, returns `400` (`'skip' needs to be a number`, 2.83.0 `rest.py:511-514`). A negative `skip` returns `400` (`Negative indexing is not supported.`): Django rejects the negative slice (`django/db/models/query.py:403-417`), and `list()` returns `400` (`rest.py:824-827`). A list with no filter key, a `since` that is absent or `0`, and a `limit` of `0` or more than `250` (any `limit` at `depth` greater than `0`) is the exception, see § Known Divergences. A `name_search` that matches no row is also an exception: the result is empty (see § Name search). On a single-object GET, a value above `0` returns `404` (see § Filters on a single-object GET) |
| `depth` | Both | Edge expansion depth. A `depth` that is not an integer, or an empty `depth`, returns `400` (`'depth' needs to be a number`), as upstream (2.83.0 `rest.py:520-523`). The value is parsed as for `limit`, and with a repeated `depth` the last value applies. **Detail:** clamped to `0`–`4` (the range upstream accepts, 2.83.0 `serializers.py:1016-1039`: `max_depth` returns 3 for lists / 4 for detail, `default_depth` 0 / 2). `0` = flat row (FK fields as IDs, no `_set`); `1` = forward FK objects expanded flat with reverse `_set` fields as bare ID lists; `2` = default, with `_set` collections as full objects, each first-level nested FK object carrying its own reverse sets as ID lists. The detail default is `2` (`default_depth(is_list=False)`). Negatives floor to `0`. `3`/`4` render the depth-2 shape (the deeper sub-level nesting they add upstream is not reproduced). **List:** default `0`; `0` or a negative value gives flat rows. `1` adds the reverse `_set` fields of each row as ID lists, `2` or higher as full objects. A list row never carries the forward FK object (`org`, `net`, `ixlan` and so on), as upstream (`serializers.py:1166-1167`, `:1286-1290`), so `fac`, `poc`, `ixpfx`, `netixlan`, `netfac`, `ixfac` and `carrierfac` return the same rows at every depth. `3` or higher renders the depth-2 shape. See § List depth. `_set` fields list only live children (see "Soft-delete tombstones" below). The sets are in ascending id order, with these exceptions: `net.netfac_set`, `ix.fac_set` and `carrier.carrierfac_set` are in facility-id order, as upstream sends them, and `ixlan.net_set` keeps the order of the netixlan rows. See § Known Divergences |
| `fields` | Both | Comma-separated list of keys to keep. The response always keeps `id`. It also keeps every key that ends in `_set` (on `net`, this includes `irr_as_set`) and every nested object that has an `id`, so a detail response at the default depth still includes its sets. Unknown names are ignored. On a `?depth=` list, only the `_set` fields that `fields` names are returned. See § Known Divergences |
| `since` | Both | The value is Unix seconds as an integer. The list holds the rows with `updated` at or after that second, in `updated` order, then `id` order (see § List order). It also admits `deleted` rows, and `pending` rows on `/api/campus` (see § Soft-delete tombstones). `since=0` or a negative value is ignored. The value is parsed as for `limit`, and with a repeated `since`, the last value applies. An empty value or a value that is not an integer returns `400` (`'since' needs to be a unix timestamp (epoch seconds)`, 2.83.0 `rest.py:505-510`). Upstream stores `updated` with microseconds and compares it with `N.000000` (2.83.0 `rest.py:736-744`). It shows the value truncated to the second (`serializers.py:1920-1924`), so it also returns almost every row shown as `updated=N`. The mirror stores only the second and includes it. On a single-object GET, the value is checked and then ignored |
| `distance`, with `latitude` and `longitude` | Both (`fac`, `org`) | Keeps the rows within that many kilometers of the point, nearest first. On a single-object GET, an object that is farther returns `404`. See § Distance filter |
| `{field}`, `{field}__{op}` | Both | Arbitrary field filter. Operator suffixes: `__contains`, `__icontains`, `__startswith`, `__istartswith`, `__iexact`, `__in`, `__lt`, `__lte`, `__gt`, `__gte`. `contains` and `startswith` are coerced to their case-insensitive variants per upstream 2.83.0 `rest.py:657-662`. Upstream ignores a key with the `__iexact`, `__icontains` or `__istartswith` suffix; the mirror applies them (see § Known Divergences). Typed against the field: an operator that the type does not support (e.g. `asn__contains`) returns `400`, and so does a value that does not convert for an operator (`asn__lt=abc`, `asn__in=1,x`). An integer value is parsed as Python `int()` parses it (spaces at the ends, a sign, Unicode digits, underscores between digits). A key without an operator on an integer field matches the decimal text of the stored value, as upstream (`__iexact`, 2.83.0 `rest.py:670-683`): `?asn=abc`, `?asn=042` and `?asn=` match no row. A key that names a FK (`?org=abc`) and the count keys (`net_count` on `fac`, `fac_count` on `net`, `net_count` and `fac_count` on `ix`) still return `400` for a value that is not an integer, as upstream. A key that names a forward FK by its upstream model name filters the FK column: `?org=1` is the same filter as `?org_id=1`. `net` and `network` are names for `net_id`, and `fac` and `facility` are names for `fac_id` (`?network__in=1,2`, `?facility_id=2`). The operators compare the FK id, and `__contains` or `__startswith` on a FK name returns `400`, as upstream (2.83.0 `rest.py:608-631`, `:670-677`, `serializers.py:403-441`). If a request gives one FK in two spellings, the mirror applies both filters. Upstream keeps only the last one |

The server reads every query parameter outside `limit`, `skip`, `depth`, `since`, `q` and `fields` as a filter key.
A key that names no field, or that has an unknown operator suffix, is ignored, and the response is `200` without that filter.
For example, `/api/net?name__foo=x` returns the unfiltered list.
Some relation keys are an exception.
For example, `/api/netfac?name__foo=x` filters on the facility name, and `/api/fac?net__foo=x` returns `400` (see § Relation filters).
A known key with a value that does not parse for the field type returns `400`.
The server records each ignored key (see § Unknown-field diagnostics).
See § Cross-entity traversal for the 2-hop cap and § Validation Notes for the rationale.

Filter values follow these rules:

- An exact match on a string field ignores case.
- A bare `address1`, `city` or `state` filter matches a substring, as upstream does when the query is not a distance search (2.83.0 `rest.py:582-595`).
  For example, `?city=Frankfurt` also matches `Frankfurt am Main`.
  With an operator suffix or a relation prefix, the key uses the normal match rules.
  On `fac` and `org`, upstream can turn a bare `city` into a distance search around the city; the mirror does not (see § Known Divergences).
- A bare `country` filter with a 2-letter value is an exact match.
  A longer value matches a substring.
  In a distance search, a bare `country` is an exact match for a value of any length (see § Distance filter).
- A bare `ipaddr6` filter on `netixlan` compares the canonical text of the address, as upstream does (2.83.0 `rest.py:605-606`, `util.py:61-73`).
  For example, `?ipaddr6=2001:7F8:0:0::1` matches `2001:7f8::1`.
  A value that is not an IP address matches no row, so a list returns `200` with no rows.
  The value of `ipaddr6` with an operator suffix, for example `__in` or `__startswith`, and of `ipaddr4` is not canonicalized; the normal match rules apply.
- If a query repeats a filter key, the last value applies.
  A relation key of a `prepare_query` and the `distance` key use the first value, as upstream (see § Relation filters and § Distance filter).
  In a distance search, `latitude` and `longitude` also use the first value; upstream passes every value to the database (see § Known Divergences).
- A time field, for example `created` or `updated`, accepts Unix seconds or ISO 8601: `2024-01-01`, `2024-01-01T12:00:00`, `2024-01-01 12:00:00`, or RFC 3339 with an offset.
  A value without an offset is UTC.
- A date without a time applies to the full day.
  `?updated=2024-01-01` matches every row updated on that day, `__gt` means after that day, and `__lte` includes that day.
  In an `__in` list, a date means the start of that day (00:00:00 UTC).
  `since` accepts only Unix seconds.

Some keys name a column that the mirror stores but upstream does not filter. pdbcompat ignores these keys the same way, for every operator:

- Keys that `queryable_field_xl` renames to a name that matches no field (2.83.0 `serializers.py:428-438`).
  `net_side` and `net_side_id` on `netixlan` become `network_side`.
  `fac_count` on `carrier` becomes `facility_count`.
  The count keys `net_count` on `fac`, `fac_count` on `net`, and `net_count` and `fac_count` on `ix` still filter, because their `prepare_query` handles them.
- Serializer fields and model properties that no `prepare_query` handles:
  `org_name` on `carrier` and `campus`,
  `city`, `country`, `state` and `zipcode` on `campus`,
  `name` on `carrierfac`, and `local_asn` on `netfac`
  (`rest.py:525-528`, `:633`, `:670`).
- Relation keys whose last segment is a FK column, for example `netixlan?net__org_id=`.
  Upstream strips `_id` from the key and gets `network__org`, and `queryable_relations()` leaves FK fields out (`serializers.py:991-995`).
  `<fk>__id` keeps its suffix and filters.
  A `prepare_query` relation key also filters, for example `netixlan?ix__org_id=` (`serializers.py:643-654`).

`__in` accepts a CSV value and binds as a single JSON array via SQLite's `json_each()`, sidestepping the variable-binding limit.
An empty `__in` (`?asn__in=`) short-circuits the request to an empty `data: []` envelope without running SQL (a `404` if the request is a lookup by `id` or `asn`, see § Lookup by `id` or `asn`).
A filter error in any other key of the request returns `400` first.
The `net` keys `info_type__in` and `info_types__in` are an exception: an empty value returns all networks, as upstream (see § Multi-value choice filters).
Malformed `__in` values for typed fields (e.g. non-integer in `asn__in=`) return `400`.

### List depth

A list request with `?depth=` greater than `0` expands the reverse `_set` fields of each row (see the `depth` row in § Query parameters).

A list with a filter, a `?since` other than `0`, or `?q`, that has more than 250 rows after `skip` and `limit`, returns the first 250 rows.
A filter key counts even when it matches every row, as upstream counts it (for example `net?info_type__in=,x`).
A key that the mirror ignores does not count (for example `org?org_flags=1` or `fac?ix_side_set__asn=64500`, see § Known Divergences).
The `meta` object then carries the upstream message (2.83.0 `rest.py:766-772`, `API_DEPTH_ROW_LIMIT` = 250):

```json
"meta": {"truncated": "Your search query (with depth 1) returned more than 250 rows and has been truncated. Please be more specific in your filters, use the limit and skip parameters to page through the resultset or drop the depth parameter"}
```

The message shows the `depth` value of the request, for example `with depth 7`.
To get more rows, use `limit` and `skip`, or add filters.

A list without a filter, `?since` or `?q` is not truncated, because upstream serves it from its API cache (`api_cache.py:90-124`).
The server loads a `?depth=` list of `org`, `net`, `ix`, `ixlan`, `carrier` or `campus` in groups of 250 rows.
The response memory budget applies to the most expensive group: if one group does not fit, the response is `413` (see [ARCHITECTURE.md § Response Memory Envelope](./ARCHITECTURE.md#response-memory-envelope)).

### Multi-value choice filters

Two fields hold a list of choices: `info_types` on `net` and `available_voltage_services` on `fac`.
Upstream stores such a field as one string: the choices in the order of the upstream choice list, joined with commas (django-peeringdb `fields.py:61-71`, `const.py:114-125` and `:203-208`).
For example, a network with the types `Content` and `NSP` stores `NSP,Content`.
The API returns the list in no fixed order.
The filters compare the stored string, and the mirror builds the same string from the list that it stores.

| Key | Match |
|-----|-------|
| `info_types=NSP,Content`, `available_voltage_services=No Power,48 VDC` | The stored string, without case. The value must use the choice-list order: `info_types=Content,NSP` matches nothing |
| `<field>__contains=`, `<field>__startswith=` | A substring or a prefix of the stored string |
| `<field>__in=` (except `net` `info_types__in`) | Each item is converted to the stored form: the choices that occur in the item, in choice-list order. A row matches when its stored string is equal to one of the items. An item that holds no choice matches the rows without a value |
| `<field>__lt=`, `__lte=`, `__gt=`, `__gte=` | The value is converted to the stored form, and the two strings are compared |

`net` also accepts the legacy `info_type` keys.
Upstream `NetworkSerializer.finalize_query_params` rewrites these keys, and two `info_types` keys, onto `info_types` (2.83.0 `serializers.py:3765-3813`):

| Key | Match |
|-----|-------|
| `info_type=X` | The stored string starts with `X`, contains `,X,`, or ends with `,X`. `info_type=NS` matches a network whose first type is `NSP` |
| `info_type__contains=X` | The same as `info_types__contains=X` |
| `info_type__in=`, `info_types__in=` | An item is a substring of the stored string. Spaces at the ends of an item are removed. An empty item matches every network |
| `info_type__startswith=X`, `info_types__startswith=X` | The stored string starts with `X` or contains `,X` |

An `__in` list costs one `LIKE` test per item on each network, as the `OR` of `icontains` terms that upstream runs.
`BenchmarkMultiChoice_InfoTypesIn` (`internal/pdbcompat/multichoice_bench_test.go`, `go test -tags=bench`) measures the cost per item.

Upstream ignores every other `info_type` key, because `info_type` is a model property (`models.py:5812-5816`).
This includes relation keys such as `netixlan?net__info_type=`.
A relation key on a multi-value field, for example `netixlan?net__info_types=`, uses the rules of the first table.
A `prepare_query` relation key without an operator, for example `ix?fac__available_voltage_services=`, converts the value to the stored form first (`models.py:221-234`).
The comparison operators compare lower case text in byte order.
Upstream compares under the MySQL collation, which can put punctuation in a different order.

### Name search

Upstream sends `?name_search=` to its Elasticsearch index (2.83.0 `rest.py:532-553`, `search_v2.py:861-978`).
The mirror runs the same kinds of match in SQL (`internal/pdbcompat/name_search.go`).

- Only the exact key applies.
  A repeated key uses its last value.
  An empty value does nothing.
- On `poc`, `ixlan`, `ixpfx`, `netixlan`, `netfac`, `ixfac` and `carrierfac`, any other value returns no rows, as upstream.
  The same applies to a value that can match nothing, for example `AND` or only spaces.
  Then a bad value in another filter of the request does not return `400`, as upstream.
  The `prepare_query` keys are still read, so a bad value of one of them returns `400`: the relation and presence keys, `whereis`, `capacity`, `distance` and the `fac_count` and `net_count` keys.
  So does a `since`, `limit`, `skip` or `depth` that is not an integer.
- A negative `skip` does not return `400` when the search matches no row, on a list or a single-object GET: upstream returns the empty result before it slices the query (`rest.py:550-553`, `:755-760`).
  A list is then `[]`, or `404` (`Entity not found`) for a unique query.
  With a match, a negative `skip` returns `400` as usual.
- A match is always a row with status `ok`, also with `?since=`: the upstream index holds only `ok` rows (`documents.py:97-136`).
- A single-object GET applies the key too: `/api/fac/<id>?name_search=` returns `404` (`No Facility matches the given query.`) when the search does not match the object.
  A search with no match returns this `404` also with a `limit` or `skip` above `0` or a negative `skip`, as upstream, because upstream returns the empty result before it slices the query.
  On the 6 types with a search index, the mirror runs one extra query to find out if the search has a match.
- The value selects one of these matches:

| Value | Example | Rows that match |
|-------|---------|-----------------|
| A partial IPv4 address: 1 to 4 dotted numbers, each at most 255 | `80.81.192` | `net`: the networks with an `ok` or `not-operational` netixlan whose `ipaddr4` starts with the value. `ix`: the exchanges with such a netixlan on an `ok` ixlan. Other types: none |
| A partial IPv6 address: hex groups and colons, with a colon after the first group | `2001:7f8:` | The same, on `ipaddr6`. One trailing `:` is removed, and case is ignored |
| Only digits, or 1 to 4 digits and one trailing `:` | `64500`, `6939:` | `net`: the networks whose ASN, as a decimal number, starts with the number of the value (leading zeros are ignored). All 6 types: the rows whose `name`, `aka` or `name_long` contains the digits. Each `AND` in the value is removed first, so `64500 AND` and `645AND00` are also digit values. Decimal digits of other scripts count as digits, as in Python |
| Other text | `equinix fr5` | The rows where each word is in at least one of the fields of the type, case ignored. Words can be in different fields. The words `AND` and `OR` are not searched |

| Type | Fields |
|------|--------|
| `org`, `fac`, `ix`, `campus` | `name`, `aka`, `name_long`, `city` |
| `net` | `name`, `aka`, `name_long`, `irr_as_set` |
| `carrier` | `name`, `aka`, `name_long` |

- On these 6 types, a digit value that Python `int()` rejects returns `400`, as upstream: a character such as `²` or `①`, or more than 4300 digits.
  A key that upstream handles in `prepare_query`, for example `not_ix` or `asn_overlap`, is checked first, as upstream (`rest.py:488-500`), so its error message is returned when both values are wrong.
- The other filters of the request also apply.
  The exception is `id__in`: as upstream (`rest.py:685-690`), the result is the matches plus the listed ids.
  When no row of the type matches, the result is empty, also with `id__in`.
- Diacritics are not ignored, as upstream: `Koln` does not match `Köln`.
- The list order does not change (see § List order).
  Upstream also loses the search rank.
- The match is not the same as upstream's search engine in all cases (see § Known Divergences).

### List order

A list without `?since` and without a distance search returns rows in `id` order, ascending.
Upstream adds no `ORDER BY` to this query (2.83.0 `rest.py:747-748`), and none of the 13 models declares a default ordering, so MySQL returns the rows in primary-key order.
A `?since` list returns rows in `updated` order, ascending, as upstream orders it (`rest.py:744`).
Rows with the same `updated` value come back in `id` order.
Upstream leaves the order of these rows to the database.
A distance search on `fac` or `org` (see § Distance filter) returns the rows in distance order, nearest first, then in `id` order.
With `?since`, the list keeps the `updated` order: upstream sorts it by `updated` after the distance sort (2.83.0 `rest.py:744`), and Django replaces the earlier order.
`skip` and `limit` apply after the sort, so each page continues the same order.
An upstream netixlan list can return its rows in a different order (see § Known Divergences).

### Lookup by `id` or `asn`

A list request with the `id` key on any type, or with the `asn` key on `/api/net`, is a lookup of one object.
If the list is empty, the response is `404` with the detail `Entity not found`, as upstream (2.83.0 `rest.py:809-815`, `serializers.py:962-967` and `:3815-3820`).
Only the key counts.
Any other filter, `skip`, `limit` or `since` that empties the list also causes the `404`.
`id__in`, `asn__in`, and `asn` on other types are ordinary filters, and an empty result is `200` with an empty `data` array.
A request with `?page=` does not get the `404`, as upstream.
The body is `{"data": [], "meta": {"error": "Entity not found"}}`, as upstream.
A value that is not the decimal text of an integer, for example `?id=abc`, `?id=` or `?asn=042`, matches no row, so the response is `404`, as upstream.

### AS-SET lookup (`as_set`)

`GET /api/as_set` returns one object that maps the ASN of each network to its `irr_as_set` value, as upstream (2.83.0 `rest.py:1396-1423`, `models.py:5699-5705`).
The object holds only the networks with status `ok` and a value that is not empty.
The keys are ASNs as strings, in ascending ASN order.
Upstream sends them in database order; the order of the keys in a JSON object has no meaning.
If no network has a value, `data` is an empty array.

`GET /api/as_set/{asn}` returns the pair of one network, in any status, also when the value is empty.
The ASN is read with the rules of Python `int()`: spaces around the value, a `+` sign, leading zeros, `_` between digits and decimal digits of other scripts are accepted.
A value that is not an integer returns `400` with the error `Invalid ASN`.
An ASN that no network has returns `404` with an empty body, as upstream.

Both routes ignore every query parameter: `limit`, `skip`, `fields`, `depth`, `since` and filters have no effect.
A path with one more segment, or with a `.` in the ASN, returns `404`.
`HEAD` returns `405` with `Allow: GET`, as upstream.

Example:

```bash
curl "https://peeringdb-plus.fly.dev/api/as_set/64500"
```

### Filters on a single-object GET

A single-object GET applies the filter keys of a list, for example `/api/net/<id>?name=<name>` or `/api/netixlan/<id>?status=ok`.
The keys, operators and relation keys are the same as on a list, and so are the Known Divergences rows about a filter.
If a filter excludes the object, the response is the same `404` as for an id that does not exist (`No Network matches the given query.`), as upstream (2.83.0 `rest.py:849-855`, `:477-703`).
The status set does not change: a single-object GET returns the live statuses and `pending`, and `?since=` does not add `deleted` (`rest.py:718-750`).
A relation key that checks the status of the listed row, for example `/api/campus/<id>?facility=<id>`, returns `404` for a `pending` object, as upstream.
`?q=` is ignored.
`?since=`, `?limit=` and `?skip=` must be integers, and `skip` must not be negative, or the response is `400`, as on a list (for a negative `skip`, a `name_search` that matches no row is the exception, see below).
`?depth=` must also be an integer, or the response is `400`, as upstream.
The mirror checks `skip`, `limit`, `since` and `depth` in that order, and then the filters.
The `{id}` is parsed as `limit` is, so `/api/net/%D9%A1` and `/api/net/+1` return net `1`, as upstream.
An `{id}` that is not an integer returns `404` (`Not found.`) after these checks, also when a filter excludes every object, as upstream: `get_object_or_404` converts the id after `get_queryset` has checked the parameters and filters (DRF `generics.py:13-21`, `:87-100`).
An `{id}` with a `.` or a `/` returns the `404` before these checks.
Upstream reads the part after the `.` as a format suffix, so `/api/net/1.5?since=abc` is also a `404` there: it rejects the format before the parameter checks (DRF `negotiation.py:80-88`, `views.py:408-411`).
The `json` suffix is the exception: upstream serves `/api/net/1.json`, and the mirror returns `404`.
A `limit` or `skip` above `0` returns `404` (`Not found.`), as upstream: upstream slices the query before it looks up the object.
A `name_search` that matches no row is the exception: it returns the `404` of a filter that excludes the object, also with a negative `skip`, as upstream returns the empty result before the slice (see § Name search).
A negative `limit` is accepted and returns the object, as upstream.
A filter that the caller's tier cannot see does not change the result: a hidden contact is `404` whatever the filters are.

### Diacritic-insensitive matching

On the 16 fields below, these operators ignore diacritics and case: exact match, `__iexact`, `__contains`, `__icontains`, `__startswith`, `__istartswith` and `__in`.
For example, `?name=Koln` and `?name__contains=koln` both match `Köln`.
`__lt`, `__lte`, `__gt` and `__gte` compare the stored value and do not ignore diacritics.
`?q=` does not ignore diacritics (see § Known Divergences).

| Entity | Folded fields |
|--------|---------------|
| `org` | `name`, `aka`, `city` |
| `net` | `name`, `aka`, `name_long` |
| `fac` | `name`, `aka`, `city` |
| `ix` | `name`, `aka`, `name_long`, `city` |
| `carrier` | `name`, `aka` |
| `campus` | `name` |

Implementation: each row carries a sibling `<field>_fold` shadow column populated at sync time via `internal/unifold.Fold` (NFKD decomposition + a ligature map).
Filter routing happens in `internal/pdbcompat/filter.go`.
When `tc.FoldedFields[<field>]` is `true`, `buildExact`, `buildContains`, `buildStartsWith` and `buildIn` run the predicate against `<field>_fold` with `unifold.Fold(value)` on the right-hand side.
The shadow columns carry `entgql.Skip(SkipAll)` and `entrest.WithSkip(true)` so they are invisible to GraphQL, REST, and proto wire surfaces — they exist only to power pdbcompat folding.
See § Known Divergences for the upstream-parity comparison and § Validation Notes for why MySQL collation is *not* the upstream mechanism.

### Soft-delete tombstones

Sync never deletes a stored row.
When upstream deletes an object, the next `?since=` fetch returns the row with `status='deleted'`, and sync stores that status.
Sync does not mark a row as deleted when the row is missing from a list response.
The one exception is a connection of a deleted network (see below).
The list path applies the upstream PeeringDB 2.83.0 `rest.py:719-750` status matrix as the final predicate via `applyStatusMatrix`.
The matrix starts from the live statuses of the type (upstream `live_statuses()`, `models.py:109-122`):

- `netixlan`: `ok` and `not-operational`.
- All other types: `ok`.

| Request shape | Admitted statuses |
|---------------|-------------------|
| List, no `?since` | live statuses only |
| List with `?since=N` | live statuses and `deleted`; `pending` additionally admitted on `/api/campus` |
| Single-object GET `/api/<type>/<id>` | live statuses and `pending` — tombstones return `404` |
| Nested `_set` lists, `?depth=1` and higher | live statuses only |

Most relation keys of a `prepare_query` also require status `ok` on one row of their path (see § Relation filters).
For `ixpfx?ix=`, `netfac?name=`, `ixfac?city=` and `campus?facility=`, that row is the listed row.
A `?since=N` list with one of these keys does not return the deleted rows, or the pending campuses.

The nested sets follow the upstream nested prefetch (`serializers.py:1140-1148`), which admits only the live statuses of the child type.
A pending child is fetchable by its own ID, but it does not appear in the `_set` lists of its parent.
In practice this affects only campuses: a campus is pending while it has fewer than two facilities, and campus is the only type whose pending rows reach the mirror.
A pending campus reaches the mirror in a `?since=` window when it changes, or through the FK backfill of a facility that points to it.
For `ix.fac_set` and `ixlan.net_set`, the status of the ixfac or netixlan join row decides membership.
The facility or network that the row points to is not filtered.

A network that upstream deletes because the RIR reclaimed its ASN loses its live connections without a tombstone (2.83.0 `management/commands/pdb_rir_status.py:440-443`).
The mirror marks such a connection `deleted`, sets `operational` to `false`, and keeps its `updated` value.
It does this when the network was deleted in that sync, or when upstream no longer returns the connection live for an uncached `?since=1&id__in=` request.
A connection that upstream still serves, or that it changed after it deleted the network, stays live.
Lists without `?since`, the depth sets, relation keys, `/api/netixlan/<id>` and `?id=<id>` leave the marked connections out, as upstream does.
A `?since=N` list, with N not later than their `updated` value, returns them as tombstones, and so does `?id=<id>&since=N`.
Upstream returns nothing, or `404` for the `id` query (see § Known Divergences).
Because `updated` does not change, a client that syncs by `updated` does not see the change on any API.

A deleted `poc` is served with `name`, `phone`, `email` and `url` set to `""`.
Upstream applies this rule when `status` is among the rendered fields (2.83.0 `serializers.py:2941-2954`, `pdb_api_test.py:3120-3127`), so a tombstone shows the deletion but not the contact details.
The mirror also applies it when `?fields=` leaves out `status` (see § Known Divergences).
`role`, `visible`, `net_id` and the timestamps are not changed.
Sync stores each deleted `poc` with these fields blank.
The primary also blanks them on older stored tombstones, when it starts and in each sync cycle.
GraphQL, REST and ConnectRPC serve the stored values, so they do not serve these fields of a deleted `poc` either.

A `not-operational` netixlan is a published connection that its network declares not operational.
Upstream 2.83.0 moved every row with `status='ok'` and `operational=false` to this status, and now derives `operational` as `status == 'ok'`.
The row is served like an `ok` row: on lists, in `?since` windows, on direct GETs, and in the `netixlan_set` and `net_set` depth sets.
`?operational=false` returns it, and `?status=ok` does not.
To select every live connection, use `?status__in=ok,not-operational` or leave out the status filter.

`?status=<value>` is an ordinary filter on all 13 types.
Exact match is case-insensitive, and `__in`, `__contains` and `__startswith` also work.
The filter ANDs with the matrix, so it can only narrow the result:

- `/api/net?status=deleted` returns `[]`, because the list without `?since`
  admits only `ok` rows.
- `/api/net?since=N&status=deleted` returns only the tombstones in the window.
- `/api/campus?since=N&status=pending` returns only the pending campuses.

This matches upstream PeeringDB 2.83.0.
`rest.py:683` turns `?status=` into `status__iexact`, and the matrix filter at `rest.py:745-750` is applied after it.
Upstream tests lock the result (`pdb_api_test.py:4022-4028` and `:4032-4044`).

### Metadata document (`meta`)

Every `net` and `netixlan` object carries `meta`, the PeeringDB metadata document that upstream added in 2.83.0 (migration 0159; `serializers.py:3117` and `:3684`).
The key follows `logo` on `net` and `ix_side_id` on `netixlan`.

- `meta` is an open JSON object.
  Upstream registers its keys in server code and can add keys without a new release of its client model library (`docs/api/object_metadata.md:22-23`).
  The mirror stores the document as is, so a new key needs no change here.
  At 2.83.0 the keys are `preferred_ip_mtu` and `rtbh_community` on `net`, and `planned_status_change` (`status`, `date`) and `rfc8950` on `netixlan`.
- Sync stores the document that upstream sends, and pdbcompat serves it unchanged.
  Key order inside the document can differ from upstream.
- A row without a stored document returns `{}`, the same as upstream.
- `meta` appears in every shape: lists, detail responses, depth `_set` objects,
  and nested `net` objects.
- Upstream applies no read restriction to `meta`,
  so the mirror applies no privacy filter to it.

A document that upstream set before the mirror stored `meta` arrives with the next full sync.
With the default `PDBPLUS_FULL_SYNC_INTERVAL` of `24h`, this is within a day.
Incremental sync cannot repair such a row, because it does not rewrite a row whose `updated` value it already has.
If the interval is `0`, run one full sync (`POST /sync?mode=full`).

#### Metadata filters

`/api/netixlan` filters on the metadata keys that upstream marks as filterable (`meta_registry.py:277-313`, `docs/api/object_metadata.md:165-182`):

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
  Exact match is a prefix match, the same as upstream, so `2026-10` matches every day in October 2026.
  `__lt`, `__lte`, `__gt` and `__gte` compare dates.
  `__in` matches whole dates.
  Upstream returns an error for `__in` on a date (see § Known Divergences).
  `__contains` and `__startswith` return `400`.
- Boolean: `true` (in any case) or `1` selects `true`.
  Any other value selects `false`, the same as upstream.
  `__in` parses each value like the other boolean filters.
  Other operators return `400`.
- A row without the key never matches, for any operator.
  So `?meta__rfc8950=false` returns only the rows that declare `false`, not the rows that never set the key.
- On these keys, upstream knows only the operators `__lt`, `__lte`, `__gt`, `__gte`, `__contains`, `__startswith` and `__in`. pdbcompat ignores a key with any other suffix, as upstream does, for example `meta__rfc8950__foo` or `meta__rfc8950__iexact`.
  The `__iexact`, `__icontains` and `__istartswith` names that the ordinary filters accept do not apply to these keys.
- `net` has no filterable metadata keys.
  Upstream ignores `?meta__rtbh_community=` and `?meta__preferred_ip_mtu=` and returns the full list.
  The mirror does the same.

Upstream rewrites these keys onto typed, indexed columns before it applies the filters (`serializers.py:3129-3149`). pdbcompat resolves them before it splits a key for traversal, so the 2-hop cap does not apply to them.
The mirror has no such columns.
It reads the key from the stored document with SQLite `json_extract` (`json_type` for the boolean).
A filter on a metadata key alone therefore scans the netixlan table.
The budget count and the served list use the same predicates.

### Cross-entity traversal

pdbcompat resolves `<fk>__<field>` and `<fk>__<fk>__<field>` filter paths through two mechanisms, both driven by codegen from ent schema annotations at `go generate` time:

- **Path A: per-serializer allowlists.**
  Derived from the upstream `peeringdb_server/serializers.py` `prepare_query(...)` / `get_relation_filters(...)` seed lists and from `queryable_relations()`.
  The keys are the mirror's choice of aliases, not a copy of an upstream list: some resolve keys that upstream ignores, and some do not resolve here.
  The relation keys that a `prepare_query` handles are not in Path A: they resolve first, as relation filters (see § Relation filters).
  Generated from ent schema `pdbcompat.WithPrepareQueryAllow(...)` annotations via `cmd/pdb-compat-allowlist`; emitted into `internal/pdbcompat/allowlist_gen.go`.
  This is the "explicitly blessed" set of filter keys.
  Every entry carries a `// serializers.py:<line>` comment that anchors it to upstream 2.83.0.
- **Path B: ent edge introspection.**
  When a filter key does not match Path A, the parser consults the generated `Edges` map (also emitted into `allowlist_gen.go`).
  Every non-excluded FK edge auto-exposes `<fk>__<field>` for any filterable field on the target entity that is a model field upstream.
  A serializer field or a model property that the mirror stores, such as `org_name` on `fac` or `city` on `campus`, is not a target (`TypeConfig.NonModelFields`): `queryable_relations()` offers only model fields, so upstream ignores `netfac?fac__org_name=` and `fac?campus__city=`, and so does pdbcompat.
  A forward edge also accepts the upstream model name of its FK as the first segment (`network__asn`, `facility__name`).
  `netixlan` also has a declared column edge, `ix_side`, over its `ix_side_id` column to the facility.
  `netixlan?ix_side__<field>=` filters on the facility of the exchange side, as upstream (2.83.0 `models.py:6095-6101`, `serializers.py:970-996`).
  The edge has no ent edge, so the other API surfaces do not change.
  The `net_side` keys stay ignored, as upstream ignores them (`serializers.py:428-432`).
  This is close to upstream `queryable_relations()`, which exposes `<fk>__<field>` for the forward FKs and `<related_name>__<field>` for the reverse relations.
  The mirror also follows reverse edges and second hops, names a reverse edge by its traversal key (`org?ix__name=`) instead of the upstream `_set` name (`org?ix_set__name=`), and has no field-level exclusions.
  See § Known Divergences.
  A relation key filters `status` only through a forward edge one hop away (`net?org__status=`).
  Upstream ignores `status` on the mirror's reverse and 2-hop keys too.
  Resolution uses a static map that codegen emits.
  There is no runtime ent-client introspection, `sync.Once` or init-order coupling.

The resolution order is implemented in `internal/pdbcompat/filter.go` `ParseFiltersCtx` and `buildTraversalPredicate`: relation filters first, then Path A; on a soft miss (allowlist hit but downstream introspection unavailable) the parser falls through to Path B rather than suppressing the key.
`parseFieldOp` returns the 3-tuple `(relationSegments, finalField, op)` so the same machinery serves 1-hop and 2-hop paths with a single split.

#### Relation filters

Upstream handles some relation keys in the `prepare_query` method of a serializer, apart from its model-field filters (2.83.0 `serializers.py:614-656`). pdbcompat resolves these keys before Path A and Path B, with the same paths and status rules (`relationSeeds` in `internal/pdbcompat/relation_filter.go`).

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

The relation keys of `fac`, `ix`, `net`, `netixlan` and `ixpfx` also accept the spelling `<rel>_id`, except `org_name` and `name`.
The key forms follow `get_relation_filters`:

- `<rel>=V` compares the id of the related row, and `<rel>__<op>=V`
  compares that id with the operator.
- `<rel>__<field>=V` filters a field of the related row, and `<rel>__<field>__<op>=V` adds an operator.
  A field that ends in `_id` names a FK of that row (`ix?ixfac__fac_id=`).
  A field that the related model does not have returns `400` (`Invalid query`), as upstream (`rest.py:499-500`).
  This includes a serializer field or a property that the mirror stores, for example `net?netfac__name=`, and a name that `queryable_field_xl` renames to nothing, for example `net?ix__fac_count=` (`facility_count`).
  On `ix?ixlan`, `ix?ixfac`, `net?netfac` and `net?netixlan`, the field can repeat the relation name, as upstream (`models.py:223-227`): `ix?ixlan__ixlan_mtu=` filters `mtu`, and `ix?ixlan__ixlan_id=` filters the id.
  `pk` names the id.
  On a relation through a FK, `exact`, `lt`, `lte`, `gt` and `gte` as the field compare the id, as the key without a field does.
  `isnull` as the field returns `400`.
  A model field that the mirror does not store, for example `fac?net__notes_private=`, is ignored (see § Known Divergences).
- `<rel>__<field>__<other>=V` drops the third segment:
  `net?netfac__fac__name=X` compares the facility id with `X`,
  which returns `400` for a non-numeric `X`.
- A key with four or more segments is ignored.
- `netixlan?name=V` compares the exchange name, not the stored `name` of the netixlan (`serializers.py:3166-3167`).
  `get_relation_filters` does not parse the suffixes `__iexact`, `__icontains` and `__istartswith` (`serializers.py:614-656`), so `related_to_name` applies them to the name of the ixlan: `netixlan?name__iexact=V` compares the ixlan name, not the exchange name.
- `fac?org_name=V` is a substring match on the name of the organization (`serializers.py:2115-2117`), not an exact match on the stored `org_name` of the facility.
  It accepts only an operator that `get_relation_filters` parses after the key.
  Upstream ignores `fac?org_name__iexact=`, `__icontains=` and `__istartswith=`, and so does pdbcompat.
- The `netfac` and `ixfac` keys `name`, `country` and `city` filter the field of the same name on the facility (`serializers.py:3417-3424`).
  One or two segments after the key name have no effect, but an operator at the end applies.
  `city` and `country` are exact matches: the substring rewrite of the location keys applies only to model fields (`rest.py:583-595`).
- `org?asn=V` and `carrier?carrierfac_set__facility_id=V` accept no operator.
  Upstream ignores `org?asn__in=` and the operator forms of the carrier key, and so does pdbcompat.
- If a request gives one key in two or more forms, for example `ix?net=1&net__in=2`, pdbcompat applies all of them.
  Upstream uses only one form (see § Known Divergences).

The status rules follow `make_relation_filter` (`models.py:221-234`):

- The row in the table must have status `ok`.
  The other rows have no status check.
  For example, `net?ix=` does not check the ixlan.
- If the key filters `status` of that row without an operator, upstream replaces the value with `ok`: `net?netixlan__status=deleted` returns the nets that have an `ok` netixlan.
  With an operator, both filters apply.
- A repeated relation key uses its first value, not the last
  (`serializers.py:618-619`).

#### Presence filters

Upstream `prepare_query` also handles keys that keep or exclude the listed rows by their links to the networks, exchanges or organizations whose ids (for `asn_overlap`: ASNs) the value gives (2.83.0 `serializers.py:2126-2201` for `fac`, `:3750-3760` for `net`, `:4554-4629` for `ix`).
The mirror resolves them before the relation keys (`presenceKeys` in `internal/pdbcompat/presence_filter.go`).

| Type | Key | Value | Keeps |
|------|-----|-------|-------|
| `net` | `not_ix` | one ix id | the nets that have no `ok` netixlan on the exchange |
| `net` | `not_fac` | one fac id | the nets that have no `ok` netfac at the facility |
| `fac`, `ix` | `not_net` | net ids | the rows that have no `ok` netfac (fac) or netixlan (ix) of any listed net |
| `fac`, `ix` | `all_net` | net ids | the rows that have an `ok` netfac (fac) or netixlan (ix) of every listed net |
| `fac`, `ix` | `asn_overlap` | 2 to 25 ASNs | the rows that have an `ok` netfac (fac), or an `ok` or `not-operational` netixlan (ix), of the network of every listed ASN |
| `fac` | `org_present` | org ids | the facs with a netfac of a net of a listed org, or an ixfac of an exchange of a listed org |
| `ix` | `org_present` | org ids | the exchanges with a netixlan of a net of a listed org, or an ixfac of a facility of a listed org |
| `fac`, `ix` | `org_not_present` | org ids | the rows that `org_present` does not keep |

- The `ok` status applies to the link row only, as in `make_relation_filter` (`models.py:221-234`), except for `asn_overlap` on `ix` (see below).
  The ixlan between a netixlan and its exchange has no status check.
- `org_present` and `org_not_present` check no status on any row of the path: upstream reads the links with the `objects` manager, so deleted links, networks, exchanges and facilities count.
- On `ix`, `org_present` and `org_not_present` take the `ixlan_id` of a netixlan as the exchange id (`serializers.py:4580-4587`, `:4608-4615`), as upstream does.
  Every ixlan has the id of its exchange.
- A list is comma-separated.
  Every item must be an integer, else the response is `400` (`asn_overlap` checks the item count first): upstream raises `ValueError`, which `rest.py:488-500` returns as `400`.
  `not_ix` and `not_fac` take one id, so a list is a `400` too.
- Only the exact key is a presence key: `net?not_ix__in=` is an unknown key and is ignored, as upstream ignores it.
- A repeated key uses its first value.
- The list length has no limit, except for `asn_overlap` (at most 25 items).
  The ids bind as one JSON array, so a long list adds no SQL term per id.
- `asn_overlap` (upstream `overlapping_asns`, 2.83.0 `models.py:2436-2483` for `fac`, `:2846-2893` for `ix`) compares the ASN of the network (`net.asn`), not the netfac `local_asn` or the netixlan `asn`.
  The network has no status check, so the ASN of a deleted network counts.
  On `ix`, the link status is the live status set of netixlan, `ok` and `not-operational`, and the path goes through the ixlan of each netixlan and its `ix_id`.
- `asn_overlap` needs 2 to 25 items.
  One item, an empty value, or more than 25 items returns `400`, before the items are parsed.
  Upstream counts the items of the value as sent, so an item that occurs two times matches no row: `asn_overlap=64500,64500` returns an empty list.
  Two different items for the same ASN, for example `64500` and `064500`, count as one ASN.
  An ASN that no network has, also one above the integer range, matches no row.

#### IP block filter

`ix?ipblock=<text>` keeps the exchanges that have an ixpfx whose prefix text starts with the value (2.83.0 `serializers.py:4548-4552`, `models.py:2830-2843`, `prefix__startswith`).
The mirror resolves the key after the presence keys and before the relation keys (`lookupIPBlockKey` in `internal/pdbcompat/ipblock_filter.go`).
Only the exact key applies (`ipblock__in` is an unknown key), and a repeated key uses its first value.

- The filter compares text, not addresses.
  `ipblock=10.0.0.0` and `ipblock=10.0.0.0/2` match `10.0.0.0/24`; `ipblock=10.0.0.5` does not.
- The match is case-sensitive, and `%` and `_` are ordinary characters.
  Prefixes are stored in lowercase, so `ipblock=2001:DB8` matches nothing.
- The exchange is the `ix_id` of the ixlan of the prefix, as upstream reads `ixlan__ix_id`.
- The ixpfx and its ixlan have no status check: a deleted prefix still selects its exchange.
  The mirror holds a deleted prefix only when a `?since` window or the history sweep brought it in, so until then the result can be narrower than upstream.
  The status rules of the list apply to the exchange.
- An empty value selects every exchange that has a prefix, including a deleted prefix with an empty value.
- A value is never a `400`.
- A single-object GET applies the key too: `/api/ix/<id>?ipblock=` returns `404` (`No InternetExchange matches the given query.`) when no prefix of the exchange matches.
- To find the prefix that holds an address, use `ixpfx?whereis=` (see § IP address lookup).

#### IP address lookup

`ixpfx?whereis=<address>` keeps the prefixes that contain the address (upstream `IXLanPrefixSerializer.prepare_query` and `IXLanPrefix.whereis_ip`, 2.83.0 `serializers.py:4154-4168`, `models.py:5179-5197`).
The mirror resolves the key after the ix ipblock key (`lookupWhereisKey` in `internal/pdbcompat/whereis_filter.go`).

- The value is one IPv4 or IPv6 address.
  A value that is not an address returns `400`, as upstream: `ipaddress.ip_address` raises `ValueError`, which `rest.py:488-500` returns as `400`.
  This includes an empty value, a prefix such as `10.0.0.0/24`, an integer, spaces at the ends, and an IPv4 octet with a leading zero.
- An IPv6 address can have a zone, for example `fe80::1%25eth0` in the URL.
  The zone has no effect.
  An empty zone, or a zone that contains `%` or `/`, returns `400`, as upstream.
- An IPv4-mapped address, for example `::ffff:192.0.2.1`, is an IPv6 address.
  It does not match an IPv4 prefix.
- The lookup reads prefixes of every status.
  The status rules then apply to the list (see § Soft-delete tombstones), so with `?since=` the list also holds the deleted prefixes that contain the address.
- `protocol` and `in_dfz` do not change the lookup.
  As filters, they apply as usual.
- `whereis__lt`, `__lte`, `__gt`, `__gte`, `__contains` and `__startswith` do the same lookup as `whereis`: `get_relation_filters` parses the operator, and `whereis_ip` does not use it (`serializers.py:614-656`).
- `whereis__in` returns `400` for every value: upstream splits the value into a list, and `ip_address` does not accept a list.
- Upstream ignores the other forms, for example `whereis__iexact`, and so does pdbcompat.
- A repeated key uses its first value.
- If a request gives more than one form, for example `whereis=` and `whereis__contains=`, pdbcompat applies each form, and each value must be an address.
  Upstream uses only one form (see § Known Divergences).
- A single-object GET applies the key too: `/api/ixpfx/<id>?whereis=` returns `404` when the prefix does not contain the address.
- A row with an empty prefix never matches (see § Known Divergences).
- Only `/api/ixpfx` has the key.
- To match the text of a prefix, use `ix?ipblock=` (see § IP block filter).

Upstream stores each prefix in its canonical form: django-inet builds an `ip_network`, which does not accept host bits.
The mirror stores the string that the API sends.
So the mirror makes, in Go, the list of the prefixes of each length (0 to 32, or 0 to 128) that contain the address, and compares the stored `prefix` with that list.
The list binds as one JSON array.

#### Capacity filter

`ix?capacity=N` keeps the exchanges whose capacity is N (upstream `InternetExchangeSerializer.prepare_query` and `InternetExchange.filter_capacity`, 2.83.0 `serializers.py:4503-4546`, `models.py:2895-2942`).
The capacity of an exchange is the sum of the `speed` values (Mbit/s) of its netixlans that do not have status `deleted`.
The mirror resolves the key after the ixpfx whereis key (`lookupCapacityFilter` in `internal/pdbcompat/capacity_filter.go`).

| Key | Keeps the exchanges whose capacity |
|-----|------------------------------------|
| `capacity=N` | is N |
| `capacity__lt=N`, `__lte`, `__gt`, `__gte` | is less than, at most, more than, or at least N |
| `capacity__in=N1,N2` | is one of the listed values |
| `capacity__contains=S` | contains S, as a decimal string |
| `capacity__startswith=S` | starts with S, as a decimal string |

- A netixlan with status `ok`, `pending` or `not-operational` counts (`handleref.undeleted()`).
  The ixlan and the network have no status check.
  The status rules of the list apply to the exchange, so `?since=N` can return a deleted exchange.
- An exchange with no such netixlan has no capacity and never matches, not even `capacity=0` or `capacity__lt=1`.
- Upstream takes the `ixlan_id` of each netixlan as the exchange id (`models.py:2933-2941`), and so does pdbcompat.
  Every ixlan has the id of its exchange.
- N must be an integer, else the response is `400`: upstream raises `ValueError`, which `rest.py:488-500` returns as `400`.
  This includes an empty value and each item of `__in`, so `capacity__in=` is a `400`, not the empty result of the other `__in` filters.
  The value is read with Python `int()` rules: spaces at the ends, a sign, `_` between digits and non-ASCII decimal digits are allowed.
  A number outside the 64-bit range, up to the 4300 digits that Python `int()` accepts, is not an error: pdbcompat compares it as the largest or smallest 64-bit value, which gives the same rows as upstream for every realistic capacity.
- `__contains` and `__startswith` do not convert the value, so `capacity__contains=abc` returns an empty list, not `400`.
  An empty value matches every exchange that has a capacity.
- Upstream ignores the other forms, for example `capacity__iexact=`, `capacity__exact=` and `capacity__foo__gte=`, and so does pdbcompat.
- A repeated key uses its first value.
- If a request gives more than one form, for example `capacity__gte=400&capacity__lte=600`, pdbcompat applies each form.
  Upstream uses only one form (see § Known Divergences).
- A single-object GET applies the key too: `/api/ix/<id>?capacity=` returns `404` when the capacity of the exchange does not match.
- Only `/api/ix` has the key.
- The upstream advanced search sends `capacity__gte` with a value in Mbit/s.

#### Distance filter

On `fac` and `org`, `?distance=<km>&latitude=<lat>&longitude=<lng>` keeps the rows within that distance of the point and returns them nearest first, as upstream `prepare_spatial_search` does (2.83.0 `serializers.py:1837-1905`, called at `:2203-2208` for `fac` and `:4985-4990` for `org`).
The mirror resolves the key before the other keys (`parseDistanceSearch` in `internal/pdbcompat/distance_filter.go`).

- The distance is the great-circle distance in kilometers on a sphere of radius 6371 km, the upstream formula.
  A row is kept when its distance is less than or equal to the value.
- A row with no latitude or no longitude is never kept.
- The list is in distance order, then `id` order.
  With `?since`, the list is in `updated` order (see § List order).
- While the filter applies, the keys `latitude`, `longitude`, `address1`, `city`, `city__in`, `state` and `zipcode` do not filter (`rest.py:569-581`).
  Other forms, for example `city__contains` or `state__in`, still filter.
  A bare `country` is an exact match for a value of any length.
- A value of 0 or less has no effect, and the location keys then filter as usual.
- The value is a number as Python `float()` reads it, for example `50`, `1e3` or `inf`.
  A value that `float()` rejects returns `400` (`Invalid value`), as upstream (`serializers.py:443-460`).
  The mirror also returns `400` for `nan`, for a value in non-ASCII digits, and for a `latitude` or `longitude` that is not a finite number (see § Known Divergences).
- Without both `latitude` and `longitude`, the request returns `400`.
  If `country` (or `country__in`) or `city` is missing, the error message names the missing keys, country first, as upstream (`serializers.py:1856-1865`).
  Upstream sends these keys as keys of the error body, with `meta.error` set to `Bad Request` (`renderers.py:134-141`); the mirror names them in `meta.error`.
  If both are present, upstream finds the coordinates of the address; the mirror cannot, and returns `400` (see § Known Divergences).
- A repeated `distance` key uses its first value, as upstream.
  A repeated `latitude` or `longitude` key also uses its first value (see § Known Divergences).
- A single-object GET applies the filter too: `/api/fac/<id>?distance=` returns `404` when the object is farther than the distance or has no coordinates.
- The other 11 types ignore `distance`, as upstream does for a user who may use the filter.
- Upstream allows the filter only to verified users; the mirror allows it to all callers (see § Known Divergences).
- No index serves the filter: each request reads the `fac` or `org` rows and sorts the kept rows.
  The budget count and the served list use the same predicates.

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
| `?ix_side__<field>=X` (netixlan) | 1 | B, through the declared column edge `ix_side` | 2.83.0 `models.py:6095-6101` (`ix_side` FK to `Facility`) and `serializers.py:970-996` (`queryable_relations()`) |
| `?org=N`, `?network=N`, `?facility_id__in=N,M` and the other upstream FK names | 0 | Local FK column (`TypeConfig.ForeignKeys`) | 2.83.0 `rest.py:608-631` and `:670-677` (a ForeignKey key filters `<fk>_id`) |
| `?ixlan__ix__id=N`, `?ixlan__ix__name=X` (ixpfx) | 2 | A | Mirror extension. Upstream ignores 2-hop keys (see § Known Divergences) |
| `?<fk>__<fk>__<field>=X` through any two non-excluded edges, for example `netixlan?net__org__name=X` | 2 | B | Mirror extension (see § Known Divergences) |
| `?<fk>__<field>=X` for any non-excluded edge | 1 | B | 2.83.0 `serializers.py:970` (`queryable_relations()`) |

1-hop Path B fallthrough means the explicit Path A allowlists are **additive, not restrictive**: a key that is not in Path A but is a valid ent FK edge still resolves via Path B. The exclusion list (below) is the only way to block a Path B key.

An allowlisted key can resolve nothing.
The server then ignores it.
For example, the `fac` allowlist has `ixlan__ix__fac_count`, but `fac` has no `ixlan` edge, so `fac?ixlan__ix__fac_count__gt=0` returns the unfiltered list.
Upstream also ignores this key, because it is not a `fac` filter (2.83.0 `rest.py:525-528`, `serializers.py:970-996`).

#### FILTER_EXCLUDE list

The `pdbcompat.WithFilterExcludeFromTraversal()` ent edge annotation hides specific edges from Path B traversal.
It is the edge-level counterpart of upstream 2.83.0 `FILTER_EXCLUDE` (`serializers.py:136-166`).
The upstream entries that name one field of a relation have no counterpart.
The private-field entries need none: the mirror does not filter on those fields.
Of the unused-field entries, `org__latitude`, `org__longitude` and `ixlan__descr` resolve on the mirror.
Upstream ignores them, except where a `prepare_query` handles the key (for example `ix?ixlan__descr=`).
See § Known Divergences.
Upstream 2.83.0 adds the `ixf_import_request_user` relation to the list (`serializers.py:147`), so upstream now ignores `ix?ixf_import_request_user__<field>=`.
The mirror does not model that relation and always ignored these keys, so the new entry needs no annotation.

| Entity | Edge | Reason |
|--------|------|--------|
| (none) | | No edge has the annotation. Field-level privacy applies in the serializers, not on edges. |

#### 2-hop cap

Filter keys with more than 2 `__`-separated relation segments are silently ignored.
Examples:

- `?org__name=X`: 1 hop, resolves via Path A (every primary entity).
- `?ixlan__ix__id=N` on `ixpfx`: 2 hops, resolves via Path A (`TestParity_Traversal/DIVERGENCE_path_a_2hop_ixpfx_via_ixlan_ix_id`).
  Upstream ignores this key: it is a mirror extension (see § Known Divergences).
- `?ixlan__ix__org__name=X`: 3 hops, SILENTLY IGNORED (HTTP 200,
  result set is unfiltered).

A relation key of a `prepare_query` also has at most two segments before its operator, but its path can reach three tables: `net?ix__name=` walks netixlan, ixlan and ix (see § Relation filters).

The netixlan metadata filter keys, such as `meta__planned_status_change__date__lt`, are not relation paths. pdbcompat resolves them before the split.
See § Metadata filters.

Upstream resolves at most one relation hop.
`queryable_relations()` adds `<fk>__<field>` for each FK of the model (2.83.0 `serializers.py:970-996`).
`get_relation_filters` passes a serializer's `prepare_query` only the keys whose first segment is in its seed list (`serializers.py:614-656`).
Upstream ignores every other multi-hop key, so it ignores 3+-hop keys too.
The mirror's 2-hop keys go one hop further than upstream (see § Known Divergences).
The cap keeps a predictable cost ceiling of `<50ms/op @ 10k rows`, checked locally via the build-tagged gate `internal/pdbcompat/bench_traversal_test.go` (`go test -tags=bench`, without `-race`); CI does not run it.
If a legitimate 3-hop use case emerges, raise the cap together with a fresh benchstat run and a docs update here.

#### Unknown-field diagnostics

When the server ignores one or more filter keys of a list or single-object request, it records them in two places:

- A DEBUG log record `pdbcompat: unknown filter fields silently ignored`
  with `endpoint`, `type` and `unknown_fields` (a comma-separated list).
- The span attribute `pdbplus.filter.unknown_fields`, with the same list.

This applies to every ignored key, not only to traversal keys.
INFO logs do not include these keys, so clients that test field names do not fill the logs.
To see them, set `PDBPLUS_LOG_LEVEL=DEBUG` or query the span attribute in Grafana Tempo.

### Response memory budget

Before the server runs a list query, it counts the matching rows with `SELECT COUNT(*)` (`serveList` in `internal/pdbcompat/handler.go`).
It multiplies the count by a typical row size for the type.
If the result is larger than `PDBPLUS_RESPONSE_MEMORY_LIMIT` (default `128MiB`), the server returns `413` and does not run the list query.
`0` turns the check off.
Use `0` only for local development.

`/api/as_set` counts the networks that it returns and bills 640 bytes for each, so the default budget passes up to 209,715 entries.
It ignores `limit` and `skip`, so a `413` on it comes only from a `PDBPLUS_RESPONSE_MEMORY_LIMIT` below that size, and a client cannot page around it.
`/api/as_set/{asn}` returns one pair and has no budget check.

A budget-exceeded request returns:

- `413 Request Entity Too Large`
- `Content-Type: application/json`
- A body in the error form of § Errors, with two more `meta` keys: `max_rows` (the largest result set that fits) and `budget_bytes` (the configured limit).
  For example: `{"meta":{"error":"Request would return ~50 rows totaling ~80000 bytes; limit is 100 bytes","max_rows":0,"budget_bytes":100}}`.

A client that sends `Accept: application/problem+json` gets an RFC 9457 body instead, with `type: https://peeringdb-plus.fly.dev/errors/response-too-large` and the same two fields.

The estimate depends on the request and the stored rows, not on the server load, so a retry of the same request gets the same `413`.
A client that gets `413` must add filters, or read the list in pages with `limit` and `skip`.

The server also limits the total estimated size of the responses in progress to `PDBPLUS_RESPONSE_MEMORY_LIMIT`.
If a new response does not fit, the server returns `503 Service Unavailable` with `Retry-After: 1`.
A retry can succeed.
Detail requests go through both checks.
For the second check, a detail request at depth 2 or more also counts the rows in its `_set` lists.
The budget applies only to `/api/`.
For the other surfaces, see [ARCHITECTURE.md § Response Memory Envelope](ARCHITECTURE.md#response-memory-envelope).

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

Detail endpoints return a single-element `data` array, not a bare object, to preserve parity with upstream PeeringDB clients.
A `?depth=` list that is cut to 250 rows carries `meta.truncated` (see § List depth).

### Errors

Errors use the upstream form: `{"meta": {"error": "<message>"}}` with `Content-Type: application/json`, as 2.83.0 `renderers.py:134-148` writes them.
The body has no `data` key, except for the `404` of a lookup by `id` or `asn` (see § Lookup by `id` or `asn`).
An error from the `/api/` handler, and the `503` before the first sync, has `Vary: Accept`.

A request whose `Accept` header names `application/problem+json`, with a `q` value above 0, gets an [RFC 9457 Problem Details](https://www.rfc-editor.org/rfc/rfc9457.html) body with `Content-Type: application/problem+json`.
`*/*` and `application/*` do not select it (see § Known Divergences).

Typical status codes:

| Status | Cause |
|--------|-------|
| `400` | An operator that the field type does not support (for example `asn__contains`), a value that does not parse for the field type, a malformed `__in` value, a `since` or a FK id that is not an integer, a `limit`, `skip` or `depth` that is not an integer, a negative `skip`, a relation key of a `prepare_query` whose field the related model does not have (`Invalid query`, see § Relation filters), or an `as_set` ASN that is not an integer (`Invalid ASN`) |
| `404` | Unknown `{type}`, missing `{id}`, an `{id}` that is not an integer, detail GET on a tombstoned row, an empty list for a lookup by `id` (any type) or `asn` (`net`), see § Lookup by `id` or `asn`, a single-object GET whose filters exclude the object or that has a `limit` or `skip` above `0` (see § Filters on a single-object GET), or an `as_set` ASN that no network has (empty body) |
| `405` | `HEAD` on `/api/as_set` or `/api/as_set/{asn}`, or a method other than `GET` and `HEAD`. The `Allow` header is `GET` on the `as_set` paths and `GET, HEAD` on every other path |
| `413` | The estimated response is larger than the response memory budget (see § Response memory budget) |
| `500` | Database error (details redacted from response body, full error logged) |
| `503` | The in-flight response pool is full (transient, `Retry-After: 1`), or the first sync has not completed (see § Before the first sync) |

The message is the upstream text for these errors:

- A lookup by `id` or `asn` with no match: `Entity not found`.
- A single-object GET for an object that does not exist, that the detail status set or the caller's tier excludes, or that a filter excludes: `No <Model> matches the given query.`, with the upstream model name, for example `No Network matches the given query.` or `No NetworkContact matches the given query.`.
- A single-object GET with a `limit` or `skip` above `0`, or with an `{id}` that is not an integer: `Not found.`.
- A method that upstream does not map to a handler: `Method "PUT" not allowed.`, or `Method "HEAD" not allowed.` on the `as_set` paths.
- A `limit`, `skip` or `since` that is empty or not an integer: `'limit' needs to be a number`, `'skip' needs to be a number` or `'since' needs to be a unix timestamp (epoch seconds)`.
  Upstream checks `since` before `skip` and `limit`, and the mirror checks `skip` and `limit` first, so `?since=abc&skip=abc` gets the `skip` message.
- A `depth` that is empty or not an integer: `'depth' needs to be a number`.
  The mirror checks it after `since`, `skip` and `limit`, and before the filters.
- A negative `skip`: `Negative indexing is not supported.`.

Upstream checks the keys that its `prepare_query` handles (the relation, presence and count keys, for example `fac?net_count=`) before `since`, `skip`, `limit` and `depth` (2.83.0 `rest.py:488-523`), and the other filter keys after them.
The mirror checks every filter key after `depth`.
So a request with two bad values can report a different one of them than upstream.

Other messages are the mirror's own.
A panic on the server returns an RFC 9457 `500` body on every path.
Upstream returns an HTML page for this error.

Responses include an `X-Powered-By` header identifying the server as PeeringDB Plus.

## 5. ConnectRPC / gRPC (`/peeringdb.v1.*`)

Implemented in `internal/grpcserver/` using [ConnectRPC](https://connectrpc.com/) — a gRPC-compatible framework that speaks three protocols on the same endpoint:

| Protocol | Typical client | Content types |
|----------|----------------|---------------|
| Connect (HTTP/1.1 or HTTP/2) | `connect-go`, browser fetch | `application/proto`, `application/json` |
| gRPC (HTTP/2) | `grpc-go`, `grpcurl`, any gRPC stub | `application/grpc`, `application/grpc+proto`, `application/grpc+json` |
| gRPC-Web | Browser gRPC-Web clients | `application/grpc-web`, `application/grpc-web-text` |

The server listens on a single port with h2c enabled (`buildServer` in `cmd/peeringdb-plus/main.go`), so there is no separate port for gRPC.

### Services

All 13 entity types expose the same three RPCs (`proto/peeringdb/v1/services.proto`):

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

The URL path for every RPC is `/{fully.qualified.ServiceName}/{MethodName}` — e.g. `/peeringdb.v1.NetworkService/GetNetwork`.

### Messages

The messages in `proto/peeringdb/v1/v1.proto` are hand-maintained. entproto generated them at v1.6.
A later ent field reaches this surface only when it is added by hand, so some fields (for example `Network.ixp_update_exclude`) are not present.

`Network.meta` (field 41) and `NetworkIxLan.meta` (field 19) carry the PeeringDB metadata document (added upstream in 2.83.0) as a `google.protobuf.Struct`:

- A row without a stored document gets an empty `Struct`, the same as upstream's `{}`.
  The field is always present.
- If the server cannot convert a stored document to a `Struct`, it logs a warning and omits the field for that row.
  The RPC does not fail.
- Connect and gRPC JSON clients see `meta` as a plain JSON object.
  All numbers in a `Struct` are doubles.

### Filtering

List and Stream requests accept type-specific optional filter fields (see `proto/peeringdb/v1/services.proto`).
All filters AND together.
The `name`, `aka`, `name_long` and `city` filters match a substring and ignore case (`ContainsFold`).
They do not ignore diacritics.
The other string filters, `status` included, must match the full value, and they are case-sensitive.
Integer filters such as `org_id` must be positive.
The `asn` filter of `ListNetworks`, `StreamNetworks`, `ListNetworkIxLans` and `StreamNetworkIxLans` must not be negative: upstream keeps tombstones with ASN 0.
Invalid values return `INVALID_ARGUMENT`.

### Pagination (List)

| Field | Semantics |
|-------|-----------|
| `page_size` | Requested page size. Defaults to `100`, clamped to `1000`. See `normalizePageSize` in `internal/grpcserver/pagination.go` |
| `page_token` | The `next_page_token` from the previous response. The token holds a row offset. If a sync runs between two pages, rows can move, and a page can skip or repeat rows. A token that does not decode returns `INVALID_ARGUMENT` |

List RPCs return rows in `(-updated, -created, -id)` order: the newest `updated` value first.

### Streaming semantics

`Stream{Type}` RPCs use **batched compound keyset pagination** under the hood (`StreamEntities` in `internal/grpcserver/generic.go`), fetching `streamBatchSize` (`500`) rows per database round-trip and emitting one proto message per row.
The cursor is the compound `(updated, created, id)` triple; under the default `(-updated, -created, -id)` order each batch resumes via:

```sql
WHERE (updated < cursor.updated)
   OR (updated = cursor.updated AND created < cursor.created)
   OR (updated = cursor.updated AND created = cursor.created AND id < cursor.id)
```

The keyset carries every sort key, so it matches the three-key ordering exactly: progress stays monotonic and no row is skipped or repeated even when many rows share an `updated` timestamp (or an `updated`+`created` pair).

| Field | Semantics |
|-------|-----------|
| `since_id` | Filter — emits only rows with `id > since_id`. Applied as a `WHERE` predicate; **does not seed the keyset cursor** |
| `updated_since` | Filter — emits only rows with `updated > updated_since`. Applied as a `WHERE` predicate; **does not seed the keyset cursor** |

Every stream is capped by `PDBPLUS_STREAM_TIMEOUT` (default `60s`) enforced via `context.WithTimeout` at the handler.
Exceeding the timeout closes the stream with `DEADLINE_EXCEEDED`.

### `pdbplus-total-count` response header

On **full streams** (both `since_id` and `updated_since` unset), the handler runs a `SELECT COUNT(*)` preflight and sets the `pdbplus-total-count` response header to the total matching row count.
On **delta streams** (either `since_id` or `updated_since` set), the COUNT preflight is skipped entirely and the `pdbplus-total-count` header is **absent** — not "present with 0" and not "present with -1".
Clients of delta streams have no use for a full-table total and the skip avoids a needless full-table scan.

The header was named `grpc-total-count` before v1.23; the `Grpc-` prefix is reserved for protocol metadata by connect-go/gRPC, so the application header moved to the `pdbplus-` prefix.
The legacy `grpc-total-count` name is still dual-emitted for a deprecation window and will be removed in a future release — migrate clients to `pdbplus-total-count`.

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

Reflection serves all 13 service descriptors, enabling `grpcurl` and `grpcui` to discover the API with no additional wiring.
The health checker reports `NOT_SERVING` until the first sync completes (`HasCompletedSync` in the sync worker), then flips to `SERVING` for the empty service name and for every `peeringdb.v1.*` service.
The health handler bypasses the readiness middleware so that health checks can poll the service during sync-in-progress state without being intercepted by the 503 syncing page.

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

`POST /mcp` serves the [Model Context Protocol](https://modelcontextprotocol.io/) over stateless Streamable HTTP.
Responses use JSON rather than server-sent events, so requests can be handled by any healthy replica without session affinity.
MCP 2026-07-28 requests use `server/discover` and include the protocol version with every request.
Clients on an older revision use the `initialize` handshake.
The server also supports the revisions 2025-11-25, 2025-06-18, 2025-03-26 and 2024-11-05.
All tools are read-only and query the same local ent client as the other surfaces.
Each tool declares input and output schemas, read-only and idempotent hints, and closed-corpus behavior.

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
Opaque cursors are bound to the entity, relation, and successful-sync watermark.
A cursor is rejected after the mirror synchronizes, preventing rows from being skipped or repeated across snapshots.
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
Their URLs point to the request's own origin, or to the operator's `PDBPLUS_PUBLIC_URL` override.
This keeps self-hosted deployments local and avoids embedding a production hostname in the binary.

Browser clients use `PDBPLUS_CORS_ORIGINS`.
The `/mcp` handler also checks the `Origin` header against `PDBPLUS_CORS_ORIGINS`.
It rejects a malformed origin, or an origin that is not in the list, with `403`.
With the default `*`, it accepts every well-formed origin, so this check gives no DNS-rebinding defense.
To get that defense, set `PDBPLUS_CORS_ORIGINS` to the exact browser origins.
Separately, the MCP SDK rejects a request that arrives on a loopback address with a `Host` header that is not a loopback name.
Clients that are not browsers send no `Origin`, so the origin check does not apply to them.

## Field-level privacy

PeeringDB Plus mirrors upstream PeeringDB's per-field visibility marker for the IX-F member list URL: `ixlan.ixf_ixp_member_list_url` is gated by the sibling string field `ixlan.ixf_ixp_member_list_url_visible`, which carries one of `Public` / `Users` / `Private` (the schema default is `Private`, and a NULL/empty or unknown value fails closed to redacted).
Anonymous callers (the default `PDBPLUS_PUBLIC_TIER=public` deployment) receive the value only when `_visible = Public`; for `Users` or `Private` the value is omitted across all six surfaces while the `_visible` companion field is **still emitted** (upstream parity).

On `/api/`, the permission decides the key, not the value.
A caller that may see the URL gets the key with the stored value: the URL, `""`, or `null` when upstream stores no value.
Upstream does the same: it deletes the key only when the caller does not have the permission (2.83.0 `permissions.py:344-353`), and it renders a NULL column as `null`.
Sync stores `null` and `""` as they arrive.
When the key is absent from the sync input (the sync caller may not see the row), sync stores NULL.
Thus a mirror that syncs without an API key has no value for `Users` rows (see § Known Divergences).
`/rest/v1/` and GraphQL also send `null` for a NULL value.
In GraphQL, `null` also means redacted: read `ixfIxpMemberListURLVisible` to tell them apart.
ConnectRPC sends no wrapper for a NULL or empty value.

The single source of truth is `internal/privfield.Redact(ctx, visible, value)`, generic over the stored type of the value.
Every serializer calls it, and `internal/middleware.PrivacyTier` stamps the resolved tier on the request context — unstamped contexts fail-closed to `TierPublic`.

| Surface | Mechanism |
|---------|-----------|
| `/api/` (pdbcompat) | `internal/pdbcompat/serializer.go` `ixLanFromEnt(ctx, l)`. When `Redact` returns `omit=true`, the URL is a nil `**string`, and the JSON struct tag `,omitempty` removes the key. When the caller may see it, the key carries the stored value, and a NULL value renders as `null` |
| `/rest/v1/*` (entrest) | `RESTFieldRedact` (`internal/middleware/rest_redact.go`) reads every JSON response under `/rest/v1/` except `openapi.json`. When `Redact` returns `omit=true`, it deletes the key from each object that has the `_visible` companion, including ixlan objects under `edges`. It runs inside `middleware.RESTError`, so error bodies pass through unchanged |
| `/peeringdb.v1.IxLanService/*` (ConnectRPC) | `internal/grpcserver/ixlan.go` `ixLanToProto(ctx, il)` returns `nil *wrapperspb.StringValue` — wire absence under proto3 optional |
| `/graphql` | `graph/schema.resolvers.go` `ixLanResolver.IxfIxpMemberListURL` returns Go `nil` → GraphQL `null` |
| `/ui/` | No render path renders the URL today; future templates must call `privfield.Redact` in the data-prep step |
| `/mcp` | No tool returns the URL |

Operators who run a private deployment can flip `PDBPLUS_PUBLIC_TIER=users` to make anonymous callers behave as authenticated users — the startup logger emits a `WARN` with `public_tier=users` so the override is visible in deploy logs.
The Users tier gets what upstream gives an authenticated user who is not a member of the owning organization: `Public` and `Users` values and `poc` rows, but not `Private` ones.
The mirror has no organization membership, so no tier sees `Private` data.

### Contact visibility

Each `poc` row has `visible`: `Public`, `Users` or `Private`.
With the default `PDBPLUS_PUBLIC_TIER=public`, a caller sees only `Public` contacts on every surface.
A row with no value counts as `Public`.
With `PDBPLUS_PUBLIC_TIER=users`, callers also see `Users` contacts.
No caller sees `Private` contacts.
Filters through a relation apply the same rule, for example `/api/net?poc__email__contains=`.

The GraphQL `NetworkWhereInput` has no `hasPocs` or `hasPocsWith` predicate.
Such a predicate tests the `poc` rows in an SQL subquery, and the privacy policy does not apply to it.
A caller could thus match networks on the `name`, `phone` or `email` of a contact that the tier hides, and read the value one prefix at a time.
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

`GET /` and `HEAD /` include `Link` headers for `llms.txt`, the MCP server card, and the Agent Skills index.
The root bypasses the readiness middleware so service discovery still works while the first sync is in progress.

### `GET /healthz`

Liveness probe.
Always returns `200 OK` with a fixed JSON body as long as the process can serve HTTP.
It does **not** check database connectivity or sync state — a failing `/healthz` means the process itself is wedged and should be restarted.

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
After a failed sync, `/readyz` returns `503` until the next sync attempt starts: a retry or the next scheduled cycle.
Replicas read the same replicated `sync_status` table, so every machine returns `503` during that time.
The Fly.io health check uses `/readyz`, so Fly Proxy stops routing to these machines until then.
The response body is the opaque shape `{"status":"ok"}` or `{"status":"unhealthy"}` — detailed error strings are written to structured logs only (security hardening: the wire body does not leak internal failure detail).

Bypasses the readiness-gate middleware itself (so a probe can observe the unready state rather than being redirected to the syncing page).

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

Bypasses the readiness middleware so an operator can kick off the first sync before any sync has completed.

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

Upstream PeeringDB rate limits apply to the sync worker's outbound requests — setting `PDBPLUS_PEERINGDB_API_KEY` raises that ceiling.

Deployment-level rate limiting (e.g., Fly.io edge, Cloudflare, or a load balancer) is not configured in this repository.

## CORS

All surfaces pass through the shared `middleware.CORS` configured by `PDBPLUS_CORS_ORIGINS` (default `*`).
The middleware allows the full set of headers required by Connect / gRPC / gRPC-Web and MCP Streamable HTTP in addition to standard application headers; see `internal/middleware/cors.go`.
The MCP handler also checks browser `Origin` values against `PDBPLUS_CORS_ORIGINS`.
With the default `*`, it rejects only malformed values (see § 6.
MCP).
The REST subtree relies on this same outer middleware; it is not wrapped a second time.

## Known Divergences

PeeringDB Plus strives for behavioural parity with the upstream PeeringDB API (`peeringdb/peeringdb`) at the `/api/` surface.
The remaining divergences are listed below, each with an upstream citation and a guarding test.
The shape of detail responses at `?depth=0`, `1` and `2` matches upstream, including `_set` ID lists, `net_set`, back-reference removal and `campus: null`.
The shape of list responses at `?depth=1` and `2` also matches upstream.
`internal/pdbcompat/depth_test.go` locks this.

| Request | Upstream behaviour | peeringdb-plus behaviour | Rationale | Since |
|---------|-------------------|-------------------------|-----------|-------|
| `?depth=3` on list endpoints; `?depth=3`/`4` on detail | A list at depth 3 gives each expanded set element its own `_set` fields as ID lists, for the set elements that have reverse sets (`org`: `net_set`, `ix_set`, `carrier_set`, `campus_set`; `ix`: `ixlan_set`; `ixlan`: `net_set`) (2.83.0 `serializers.py:1240-1309`, `pdb_api_test.py:4140-4152`). Detail expands a third sub-level at depth 3-4. | Both render the depth-2 shape. List depths `1` and `2` and detail depths `0`/`1`/`2` match upstream. | Depth>2 sub-nesting is data almost no client reads three levels deep. Locked by `TestParity_Serializer/DIVERGENCE_list_depth3_renders_depth2_shape` and `TestDepth_DepthOne/depth_clamped_to_0_4`. | v1.16 (list) · v1.20.5 (detail 3-4) |
| `?_ctf` with a date filter and `?depth=` on a list | The last date filter with an operator (for example `updated__lte=...`) is also applied to every nested `_set` (2.83.0 `rest.py:654-655`, `serializers.py:998-1002`). The API cache generator sends it (`pdb_api_cache.py:195-199`). | `_ctf` is an unknown key and is ignored: the `_set` fields list every live child. The date filter still filters the listed rows, and it counts as a filter for the 250-row truncation (see § List depth). | `_ctf` is an undocumented flag of the upstream cache generator. The mirror cannot know which date filter is last: Go `url.Values` does not keep the key order. Locked by `TestParity_Serializer/DIVERGENCE_list_depth_ctf_ignored`. | v1.37.0 (registered 2026-09-26) |
| `poc_set` ID lists at `?depth=1` on detail and list responses, and the nested `net.poc_set` at detail depth 2 | Upstream lists every POC id in the ID list regardless of visibility, filtering non-`Public` POCs only when they are expanded to objects at depth=2. | The row-level `poc.visible` privacy policy applies uniformly, so a `poc_set` ID list (and the objects at depth=2) never contains a POC id that the caller's tier cannot read: non-`Public` ids for anonymous callers, `Private` ids for the Users tier (`PDBPLUS_PUBLIC_TIER=users`, since v1.28.0). The mirror is **stricter** than upstream here. | Leaking the ids/existence of hidden contacts would contradict the load-bearing `poc.visible` policy (see § Field-level privacy). Intentional. Locked by `TestDepth_PocSetPrivacy_DIVERGENCE`, whose list cases also lock `poc_set` on a `?depth=` list. | v1.20.5 |
| `/api/poc/<id>` for a contact that the caller's tier cannot read: a `Users` contact for an anonymous caller, or a `Private` contact for any tier | Returns `403`. `retrieve` (2.83.0 `rest.py:849-865`) returns `HTTP_403_FORBIDDEN` when the permission applicator denies the object. The upstream tests expect it for a guest and a `Users` contact (`pdb_api_test.py:3855-3856`) and for a user who is not a member of the owning organization and a `Private` contact (`:1413-1414`), through `assert_get_forbidden` (`:575-577`). | Returns `404`, the same as for an id that does not exist. REST, ConnectRPC and GraphQL also use their not-found forms. The list content matches upstream: both leave the contact out. For the status code of `/api/poc?id=<id>`, see the `/api/poc?id=` row. | A `403` shows that a hidden contact with this id exists. The mirror does not show it. Locked by `TestParity_Status/DIVERGENCE_hidden_poc_detail_404`. | v1.14 (anonymous) · v1.28.0 (Users tier, `Private` contact) |
| `ixlan.ixf_ixp_member_list_url` of a `Users` row, for a `Users`-tier caller (`PDBPLUS_PUBLIC_TIER=users`), when the mirror syncs without an API key | Emits the stored value: the URL, `null` or `""` (2.83.0 `permissions.py:344-353`). | Emits `null`. | An anonymous sync does not get the key for a `Users` row, because upstream deletes it (`permissions.py:350-353`). Sync stores NULL for an absent key. With an API key, sync gets the value, and the mirror emits it as upstream does. The default `public` tier omits the key, as upstream does for an anonymous caller. Locked by `TestParity_Serializer/DIVERGENCE_ixf_url_users_row_null_after_anonymous_sync`. | v1.28.0 |
| `/api/netixlan?meta__planned_status_change__date__in=<dates>` | Fails. Upstream parses the whole comma-separated value as one datetime (2.83.0 `rest.py:649`), which raises, and its error handler then fails on `inst[0]` (`:651`), so the response is `400`. For a single date, the parse succeeds and `v.split(",")` on the datetime (`:666`) raises an unhandled error (`500`). | Returns the rows whose date is in the list. | A list of whole dates has one clear meaning, and failing it has no value for a client. Locked by `TestParity_Meta/DIVERGENCE_date_in_filters`. | v1.28.0 (registered 2026-09-23) |
| `?status=deleted&since=N` for a row that upstream deleted before the mirror's first sync, or that sync hard-deleted before v1.16 | Returns the tombstone with its deletion timestamp. | Returns empty until the history sweep has fetched the id window of the row. After that, returns the tombstone. A `poc` tombstone older than 30 days does not come back: the sweep skips `poc`, and upstream removes those tombstones. | A bare list holds only live rows, so the first sync gets no tombstones. The history sweep fetches them over many cycles (see [ARCHITECTURE.md § History sweep](ARCHITECTURE.md#history-sweep)). Locked by `TestParity_Status/list_since_admits_deleted_excludes_pending_noncampus` and `TestSync_HistorySweepLandsTombstones`. | v1.16; history sweep added in v1.32.0 |
| A negative `skip`, or a negative `limit` at `depth` greater than `0`, on a list that upstream serves from its API cache: no filter key, a `since` that is absent or `0`, and a `limit` of `0` or more than `250` (any `limit` at `depth` greater than `0`), for example `/api/net?skip=-5` or `/api/net?depth=1&limit=-1` | Returns a slice of the list for a negative `skip`. Upstream serves such a list from its API cache file (`API_CACHE_ENABLED`, 2.83.0 `settings/__init__.py:399`, `api_cache.py:90-124`) and slices the rows with the Python slice rules (`api_cache.py:136-142`): `data[skip:]` without a `limit`, and `data[skip:skip+limit]` with one. So `skip=-5` returns the last 5 rows, `depth=1&skip=-2&limit=10` returns no rows on a list of 10 rows or more, and `depth=1&limit=-1` returns every row but the last (`data[:-1]`). Every other list returns `400` (`Negative indexing is not supported.`) for a negative `skip`, except a list with a `name_search` that matches no row (see § Name search), and serves every row for a negative `limit`. | Returns `400` (`Negative indexing is not supported.`) for every list with a negative `skip`, except a list with a `name_search` that matches no row, as upstream. Serves every row for a negative `limit` at every depth; a list at `depth` greater than `0` with a filter, a `since` other than `0` or `?q` is still cut to 250 rows (see § List depth). | The result depends on an upstream cache setting, not on the API rules. A negative offset is a client bug. Found by reading the code, not checked against a live upstream. Locked by `TestParity_Limit/DIVERGENCE_negative_skip_on_cacheable_list_returns_400`. | v1.21.0 (registered 2026-09-26) |
| A string filter (`=`, `__contains`, `__startswith`, `__in`) on a folded field, for a row stored before the deploy that added the `_fold` column of the field | Folds via `unidecode.unidecode(v)` at query time (2.83.0 `rest.py:597`), so it works immediately. | The filter reads the `<field>_fold` shadow column, which sync writes. A deploy that adds a `_fold` column leaves it empty on the stored rows. An incremental cycle writes it only on the rows that upstream changed. The next full cycle (`PDBPLUS_FULL_SYNC_INTERVAL`, default 24h, or `POST /sync?mode=full`) writes it on all other rows. Until then, the filter does not match those rows, for ASCII and non-ASCII values. A new database has no window: the first sync writes every `_fold` column. | Shadow columns give SQLite one indexable comparison path (benchstat within ±1% of the direct path) and stay off the GraphQL/REST/proto wire (`entgql.Skip` / `entrest.WithSkip`). Locked by `TestParity_Unicode_FoldWindow_DIVERGENCE`. | v1.16 |
| `?q=<term>` on list endpoints | Ignored. The db-field filter loop skips `q` (2.83.0 `rest.py:566`) and no other consumer exists on the `/api` surface (the separate `name_search` parameter triggers the Elasticsearch-backed `search_v2`, `rest.py:532-553`), so any `?q=` value returns the unfiltered list. | Convenience **extension**: case-insensitive substring search (`ContainsFold`) across the type's SearchFields, plus exact-ASN match on `/api/net`. NOT diacritic-folded: `?q=munchen` does not match "München" although `?name__contains=munchen` does (the `_fold` shadow routing applies to the field filters only). With `?depth=` greater than `0`, the mirror counts `?q=` as a filter, so a list of more than 250 rows is cut to 250 with `meta.truncated` (`TestParity_Limit/list_depth_q_truncates`). Upstream serves the unfiltered list from its API cache, without truncation. | A search that narrows results is strictly more useful than upstream's silent no-op, and clients relying on upstream behaviour (unfiltered dump) should not be sending `?q=` at all. Fold-routing `?q=` would multiply every SearchFields OR-branch across the `_fold` shadows for marginal benefit; revisit if a real client asks. Locked by `TestParity_Unicode_QSearchExtension_DIVERGENCE`. | v1.1 (registered 2026-07-10) |
| Filters on upstream model columns that the API does not serialize: `org_flags` on `org`; `geocode_status` and `geocode_date` on `org` and `fac`; `location_method` and `location_place_id` on `fac` (new in 2.83.0); `ixf_import_request_user` on `ix`; `version` on every type; `notified_for_geocoords` on `fac`; their `<fk>__<field>` forms, for example `net?org__org_flags=`, `netfac?fac__location_method=`, `netixlan?ix_side__location_method=` and `netixlan?ix_side__version=`; and the same columns plus `version`, `social_media`, `meta`, `notes_private`, `ixp_update_exclude`, the `irr_as_set_*` and `rir_status_notified` columns, `notified_for_geocoords`, `vlan`, `ixf_ixp_member_list_url`, the `ixf_ixp_import_*` columns and `avail_sonet`/`avail_ethernet`/`avail_atm` as the field of a `prepare_query` relation key, for example `fac?net__notes_private=` (the full list is `unservedModelNames` in `internal/pdbcompat/relation_filter.go`) | Filters on them. The filter loop accepts every model field (2.83.0 `rest.py:525-528`), and `queryable_relations` adds `<fk>__<field>` for each FK (`serializers.py:970-996`). A `prepare_query` relation key filters them too (`models.py:223-234`). Columns: `models.py:1259-1264` (`org_flags`), `:591-599` (`geocode_*`), `:2207-2217` (`location_*`), `:2612` (`ixf_import_request_user`); `notified_for_geocoords`: `models.py:2257-2260`; `version`: django-handleref `models.py:90`, which `HandleRefSerializer` leaves out (`rest/serializers.py:12`). | Silently ignored, like any unknown key: HTTP 200 with the unfiltered list. | The mirror stores only the fields that the API serializes. It never receives these values, so it cannot filter on them. `meta` is stored, but it has no plain filter key (see § Metadata filters), so a relation key ignores it too. `ixp_update_exclude` is stored and served, but pdbcompat has no filter key for it, so a relation key ignores it too. Locked by `TestParity_Traversal/DIVERGENCE_unserialized_model_columns_silent_ignore`. | v1.1 (registered 2026-09-23) |
| `hide_ix_no_fac` on `ix`, `ixlan`, `net` and `netixlan`, for example `ix?hide_ix_no_fac=1` | Filters in Python after the status filter (2.83.0 `rest.py:752-753`): the `IXFilterMixin` of these four views (`rest.py:1267-1297`, used at `:1354-1364`). A value of `1` or `true`, in any case, keeps the exchanges with a facility (`fac_count > 0`), the ixlans and netixlans of those exchanges, and the networks with an exchange (`ix_count > 0`) (`:1273-1275`, `:1287-1295`). Without the key, a signed-in user gets the setting of the account (`:1276-1280`). | Silently ignored, like any unknown key: HTTP 200 with the unfiltered list. A single-object GET ignores it too; upstream returns `404` when the key excludes the object (`rest.py:752-753`). | The mirror has no user accounts, and upstream changes the count fields that the key reads without a new `updated`, so the mirror has current counts only after a full sync. Implement the key when a client needs it. The `prepare_query` keys and `name_search` filter as upstream does or have their own rows (see § Relation filters, § Presence filters, § IP block filter, § IP address lookup, § Capacity filter, § Distance filter and § Name search). Locked by `TestParity_Traversal/DIVERGENCE_hide_ix_no_fac_silent_ignore`. | v1.1 (registered 2026-09-23) |
| `/api/netixlan?since=N` when N is not later than the `updated` value of a connection that upstream removed with its deleted network | A `?since` list returns nothing for the connection, and `?id=<id>&since=N` returns `404` `Entity not found` (2.83.0 `rest.py:809-815`). `pdb_rir_status` removes the live connections (`ok`, `not-operational`) of a network whose ASN the RIR reclaimed with an SQL delete, then soft-deletes the network (`management/commands/pdb_rir_status.py:440-443`, `models.py:5720-5725`). The connection gets no tombstone, although the API docs say that only contacts are hard-deleted (`docs/api/api_description.md:11-13`). | Returns the connection with `status` `deleted`, `operational` `false` and its last `updated` value, and `200` for the `id` query. Sync marks a live connection of a deleted network `deleted` when the RIR reclaim deleted the network in this sync, or when an uncached `?since=1&id__in=` request no longer returns it live. It does not change `updated`, and it leaves live a connection that upstream changed after the delete. Lists without `?since`, depth sets, relation keys, `/api/netixlan/<id>` and `?id=<id>` leave it out, as upstream does. | Without a tombstone, the mirror would serve the connection as live forever. A client that applies the tombstone deletes a connection that upstream no longer has. A window that starts after the row's `updated` value does not return it. Locked by `TestParity_Status/DIVERGENCE_deleted_net_netixlan_tombstone_in_since_window` and `TestSync_CascadesDeletedNetIxLans`. | v1.28.2 |
| `/api/poc?since=N&fields=<list>` for a deleted contact when `<list>` leaves out `status`, for example `fields=id,name,email` | Returns the stored `name`, `phone`, `email` and `url`. The serializer removes the fields that `?fields=` does not name (2.83.0 `serializers.py:942-950`) and blanks the contact fields only when the rendered `status` is `deleted` (`:2941-2954`). A soft delete does not blank the stored values. | Returns `""` for these fields. pdbcompat blanks them before it applies `?fields=`, and sync stores a deleted contact without them. The mirror is **stricter** than upstream here. | The contact data of a deleted contact must not reach a caller, whatever fields the caller asks for. Locked by `TestParity_Status/DIVERGENCE_deleted_poc_blanked_without_status_field`. | v1.16 (registered 2026-09-23) |
| `?fields=` | The serializer removes every field that `fields` does not name, including `id` and the `_set` fields (2.83.0 `serializers.py:942-950`); the API cache path removes every key not named (`api_cache.py:170-177`). `/api/net/1?fields=name` returns only `name`. For an ixlan, the serializer adds `ixf_ixp_member_list_url_visible` back when `fields` names the URL (`serializers.py:4319-4337`). The permission check removes it again only when the URL and `_visible` are the only keys left (`permissions.py:355-372`). Thus `ixlan?fields=id,ixf_ixp_member_list_url` also returns `_visible`, and for a caller that may not see the URL, `ixlan?fields=ixf_ixp_member_list_url` returns only `_visible`. | The response always keeps `id`. A detail response also keeps every key that ends in `_set` and every nested object. A list response keeps net `irr_as_set` (its name ends in `_set`). A `?depth=` list loads only the `_set` fields that `fields` names. The mirror never adds `ixf_ixp_member_list_url_visible` back: `ixlan?fields=ixf_ixp_member_list_url` returns `id` and the URL, or only `id` for a caller that may not see the URL. | A client that selects fields still gets the row identity and the detail expansion it asked for with `depth`. The ixlan `_visible` rule is a side effect of the upstream permission check, and the mirror does not copy it. Locked by `TestParity_Serializer/DIVERGENCE_fields_keeps_id_and_detail_sets`. | v1.1 (registered 2026-09-26) |
| Relation keys that upstream ignores: 2-hop keys, for example `ixpfx?ixlan__ix__id=` and `netixlan?net__org__name=`; reverse keys named by the mirror's traversal key, for example `org?net__name=`; and the field-level `FILTER_EXCLUDE` entries `org__latitude`, `org__longitude` and `ixlan__descr`, for example `fac?org__latitude__gt=` | Resolves one relation hop: `queryable_relations()` adds `<fk>__<field>` and `<related_name>__<field>` (2.83.0 `serializers.py:970-996`), and a `prepare_query` handles the keys whose first segment is in its seed list (`:614-656`). Other keys are ignored, and the list is unfiltered: `org?net__name=` becomes `network__name` (`serializers.py:403-441`), which is not a filter key (`rest.py:525-528`, `:670`). A key whose first segment a `prepare_query` handles is a relation filter (see § Relation filters). | Resolves each key through the Path A allowlist or the Path B edges and filters on the related rows as given, without a status check on them. The exception is `status`: a 2-hop key or a reverse key never filters it, so `org?net__status=` and `netixlan?net__org__status=` are ignored, as upstream. A field that is not a model field upstream is never a target either (see § Cross-entity traversal). | A 2-hop key and a reverse key have one clear meaning on the mirror's edges, and the extra filters cost one subquery each. Locked by `TestParity_Traversal/DIVERGENCE_relation_keys_upstream_ignores_resolve` and `TestParity_Traversal/DIVERGENCE_path_a_2hop_ixpfx_via_ixlan_ix_id`. The `status` keys are parity, locked by `TestParity_Traversal/status_on_reverse_and_2hop_keys_ignored_like_upstream`. | v1.16 (registered 2026-09-23) |
| Reverse keys by upstream's `<related_name>`, for example `ix?ixlan_set__status=`, `org?ix_set__name=`, `org?ix_set__in=`, `org?ix_set=` and `fac?ix_side_set__asn=`, and a related name as the field of a `prepare_query` relation key, for example `net?ix__ixlan_set=` and `fac?net__poc_set=` | Filters on the related rows. A reverse relation reports the `ForeignKey` type, so `field_names` holds the bare related name, and `queryable_relations()` adds `<related_name>__<field>` (2.83.0 `serializers.py:970-996`; related names at `models.py:3308` and `:2621-2622`). `<related_name>__<field>` is an exact match on the related rows, with the match rules of the field type (`rest.py:670-683`). `<related_name>__in`, `__lt`, `__lte`, `__gt` and `__gte` compare the related row ids (`rest.py:633-669`). A bare `<related_name>` returns `400`: `rest.py:676-677` builds `<related_name>_id`, which Django cannot resolve (`:702-703`). Upstream ignores the related names that start with `net_` or `fac_` (`net_set` and `fac_set` on `org`, `fac_set` on `campus`, `net_side_set` on `fac`): `queryable_field_xl` renames them to `network_*` and `facility_*` (`serializers.py:428-438`), and these names match no relation. A `prepare_query` relation key also filters on a related name of the related model (`models.py:223-234`). | Silently ignored: HTTP 200 with the unfiltered list. The mirror names a reverse edge by its traversal key instead (`org?ix__name=`). `fac?ix_side_set__<field>=` has no mirror equivalent: the mirror has no key that reaches the connections of a facility through `ix_side`. | The traversal keys cover the same relations, except `fac?ix_side_set__`. Accepting both names would double the key surface for no new query. Locked by `TestParity_Traversal/DIVERGENCE_reverse_set_keys_silent_ignore`, which also covers `net?ix__ixlan_set=`, `fac?net__poc_set=`, `ix?fac__netfac_set=` and `fac?ix_side_set__asn=`. The `net_set` and `fac_set` keys are parity, locked by `TestParity_Traversal/reverse_net_fac_set_keys_ignored_like_upstream`. | v1.16 (registered 2026-09-23) |
| `?<field>__iexact=`, `__icontains=` and `__istartswith=`, for example `netixlan?status__iexact=OK`, `net?org__iexact=1` and `net?ix__name__icontains=x` | Ignored. The operator regex (2.83.0 `rest.py:616`) knows only `lt`, `lte`, `gt`, `gte`, `contains`, `startswith` and `in`, so a key with another suffix is not a filter key (`:628-630`, `:670`), and the list is unfiltered. On a relation key of a `prepare_query` (§ Relation filters), `get_relation_filters` does not parse the suffix (`serializers.py:614-656`). A 3-segment key, for example `net?ix__name__icontains=`, drops the suffix and runs an exact match, and so do the `netfac` and `ixfac` keys `name`, `country` and `city`. A status of the pinned row is then replaced with `ok`. After a lookup name, for example `fac?net__exact__iexact=`, upstream drops the suffix and runs the lookup, and the mirror does the same. A 2-segment key, for example `net?ixlan__iexact=`, keeps the suffix as a Django lookup on the relation, which raises `FieldError`, so upstream returns `400` (`rest.py:488-500`). `netixlan?name__iexact=` filters the name of the ixlan with the lookup (`serializers.py:3161-3169`), and `fac?org_name__iexact=` is ignored (`:2108-2117`). | Applies the suffix: an exact, substring or prefix match that ignores case. On an integer field, `__iexact` matches the decimal text of the value, as a key without an operator does, so a value that is not an integer matches no row. On another field that is not a string, on a count key (`fac?net_count__iexact=`, `net?fac_count__iexact=`, `ix?net_count__iexact=`, `ix?fac_count__iexact=`) and on a key that names a FK, `__iexact` is an exact match, so a value that is not an integer returns `400`. On a field that is not a string, `__icontains` and `__istartswith` return `400`. The `netixlan` `name` keys and the `fac` `org_name` keys with these suffixes match upstream (see § Relation filters). | These suffixes have one clear meaning, and `contains` and `startswith` already ignore case. The metadata keys accept only the upstream operators (see § Metadata filters). Locked by `TestParity_Status/DIVERGENCE_i_operator_suffixes_filter`. The lookup-name case is parity, locked by `TestParity_Traversal/relation_key_pk_and_lookup_names_like_upstream`. | v1.16 (registered 2026-09-23) |
| `<rel>__<field>__contains=` and `<rel>__<field>__startswith=` on a relation key of a `prepare_query`, for example `netixlan?ix__name__contains=` and `net?ix__name__startswith=` | Case-sensitive. `get_relation_filters` keeps the raw operator for a 3-segment key (2.83.0 `serializers.py:643-654`), and Django runs `contains` and `startswith` as `LIKE BINARY` on MySQL. Only the 2-segment form becomes `icontains` or `istartswith` (`:622-626`). | Ignores case, and folds diacritics on the fields that have a `_fold` column, as for every other `contains` and `startswith` key. | One rule for every `contains` and `startswith` key. SQLite has no `LIKE BINARY` on the fold columns. Locked by `TestParity_Traversal/DIVERGENCE_relation_field_contains_ignores_case`. | v1.28.0 (registered 2026-09-23) |
| `campus?facility__<field>=`, `campus?facility__in=` and the other `campus?facility` keys that match more than one facility of a campus | Returns the campus once for each matching facility. `related_to_facility` filters the reverse `fac_set` join (2.83.0 `models.py:2101-2111`). `_filters_may_produce_duplicates` looks up `facility`, which is not a `Campus` field, so upstream does not call `distinct()` (`rest.py:115-141`, `:715-716`). Found by reading the code, not checked against a live upstream. | Returns each campus once. | A list with repeated rows has no use to a client, and `distinct()` is what upstream runs for the other reverse joins. Locked by `TestParity_Traversal/DIVERGENCE_campus_facility_keys_no_duplicates`. | v1.28.0 (registered 2026-09-23) |
| A request that gives one relation key of a `prepare_query` in two or more forms, for example `ix?net=1&net__in=2`, or two or more forms of `ixpfx?whereis=` (`whereis`, `whereis__lt`, `__lte`, `__gt`, `__gte`, `__contains`, `__startswith` and `__in`), for example `ixpfx?whereis=10.1.0.1&whereis__contains=10.0.0.5`, or two or more forms of `ix?capacity=` (`capacity`, `capacity__lt`, `__lte`, `__gt`, `__gte`, `__in`, `__contains` and `__startswith`), for example `ix?capacity__gte=400&capacity__lte=600` | Uses one form: the one whose first occurrence is last in the query string. `get_relation_filters` stores every form of a key under one entry, and a later key replaces an earlier one (2.83.0 `serializers.py:614-656`). Only the value of that form is parsed. | Applies every form (AND), and every value must parse. | The parsed query parameters do not keep the order of the keys, and a client that sends two forms expects both to apply. Locked by `TestParity_Traversal/DIVERGENCE_relation_filter_forms_all_apply`. | v1.37.0 (registered 2026-09-26) |
| `/api/ixpfx?whereis=<address>` or `/api/ixpfx/<id>?whereis=<address>` while the table holds a prefix row with an empty prefix, for example the upstream tombstone ixpfx 4185 (`prefix: null`) | Returns `400`. `whereis_ip` tests the address against every row of every status (2.83.0 `rest.py:482`, `models.py:5193-5195`). django-inet loads an empty prefix as `None` (django-inet 1.1.1 `models.py:195-200`), `ip in None` raises `TypeError`, and `rest.py:497-498` returns it as `400`. An `ix` key that comes before `whereis` in the query string first keeps only the `ok` rows of that exchange (`models.py:221-234`, `:5166-5177`), and then the lookup works. This is derived from the source and was not verified on the live API. | Ignores rows with an empty prefix and returns the prefixes that contain the address. A single-object GET returns the prefix, or `404` when it does not contain the address. | The lookup has one clear meaning, and a `400` for every valid address has no value for a client. Sync stores the tombstone, because upstream sends it. Locked by `TestParity_Traversal/DIVERGENCE_whereis_ignores_empty_prefix_row`. | v1.37.0 (registered 2026-09-26) |
| `?distance=` from a caller without a verified user account, on any `/api/` list or detail request | Returns `403`: "Please authenticate to use the `distance` filter", or "Please verify your account to use the `distance` filter" for a user who is not verified. `FilterDistanceThrottle` (2.83.0 `rest_throttles.py:275-331`, `:345-350`) is a default throttle class of every API view (`mainsite/settings/__init__.py:1467-1473`), and the settings require an authenticated, verified user (`:526-529`). DRF runs it before the query is built, so it comes before any `400`. An organization API key is not checked. A verified user gets the filter on `fac` and `org` at most 10 times per minute (`:459`); the other types ignore the key. | Serves the filter to every caller on `fac` and `org`, with no rate limit. The other 11 types ignore the key. | The mirror has no user accounts, so it cannot tell a verified user from an anonymous caller. The filter reads one table, about the cost of a full list. Locked by `TestParity_Traversal/DIVERGENCE_distance_served_without_auth`. | v1.37.0 (registered 2026-09-26) |
| `fac?distance=<km>` or `org?distance=<km>` without `latitude` and `longitude`, with `city` and `country` (or `country__in`); and `fac?distance=<km>&name_search=<text>` without both `city` and `country` | Finds the coordinates of the address through the Google geocoding API and runs the distance search (2.83.0 `serializers.py:1866-1880`, `models.py:719-790`). On `fac`, `name_search` first finds a location in the search index (`serializers.py:2213-2280`). If no coordinates are found, the list is empty. | Returns `400`: the error message asks for `latitude` and `longitude`. | The mirror has no geocoder and no search index. An empty list would tell the client that no rows are near the address, which can be false. Locked by `TestParity_Traversal/DIVERGENCE_distance_without_coordinates_returns_400`. | v1.37.0 (registered 2026-09-26) |
| A bare `city` filter on `fac` or `org` with one city, for example `fac?city=Frankfurt` or `org?city__in=Frankfurt&country=DE` | Becomes a distance search: `convert_to_spatial_search` (2.83.0 `serializers.py:1709-1834`, called at `:2203` and `:4985`) finds the city through the Google geocoding API, uses the diagonal of its bounding box as the distance (`geo.py:184-189`), and keeps the rows near the city, nearest first. The `city` key then does not filter (`rest.py:569-581`). This does not add `distance` to the query parameters, so the `403` for `distance` does not apply. A request with `distance` is not converted: a value of 0 or less keeps the substring match, and a positive value is a distance search. A request with two or more `country__in` values, or with no geocoding result, keeps the substring match. A geocoding result without a bounding box returns `400` (`serializers.py:1830-1832`, `geo.py:184-185`, `rest.py:497-498`). Found by reading the code; the live effect depends on the production API key and was not checked. | A substring match on the city, in `id` order (`rest.py:582-595`). A row with no coordinates can match. | The mirror has no geocoder. Locked by `TestParity_Traversal/DIVERGENCE_city_filter_is_substring_not_geocoded_radius`. | v1.21.0 (registered 2026-09-26) |
| `fac` or `org` distance search with `?distance=nan`, `inf`, an overflowing value such as `1e999`, or a value in non-ASCII digits; or with a repeated `latitude` or `longitude`, or a `latitude` or `longitude` that is not a number | Python `float()` accepts `nan`, `inf`, `1e999` and non-ASCII digits, so `single_url_param` raises no error (2.83.0 `serializers.py:443-460`: only a `ValueError` is `400`), and `nan` and `inf` pass the `distance <= 0` test (`:1839`). `latitude` and `longitude` go to the SQL as the raw lists of URL values, not parsed (`rest.py:488-491`, `serializers.py:1881-1898`). The result then depends on the MySQL driver, which is not in the source tree; not checked against a live upstream. | `nan` and non-ASCII digits return `400`. `inf` and `1e999` keep every row with coordinates, nearest first. A repeated `latitude` or `longitude` uses its first value. A `latitude` or `longitude` that is not a finite number, or is empty, returns `400`. | SQLite binds NaN as NULL, so `nan` would match no row with no error. An infinite distance has one clear meaning. A number check on the coordinates gives a clear error in place of a database-specific cast of the text. Locked by `TestParity_Traversal/DIVERGENCE_distance_value_handling`. | v1.37.0 (registered 2026-09-26) |
| `?name_search=` on `org`, `fac`, `ix`, `net`, `campus` and `carrier` | Searches an Elasticsearch index (2.83.0 `search_v2.py:861-978`). Text: a wildcard `*word*` for each word, all words required, over `name` (one lower-case token, `documents.py:52-56`) and the word tokens of `aka`, `name_long`, `city` and `irr_as_set`, plus a phrase and a phrase-prefix clause on the word tokens (`search_v2.py:525-596`), so words joined by punctuation (`gamma-net`) match `Gamma Net`. A bare `OR` between two words makes either word enough. `clean_term` removes `AND` also inside a word, so `RAND` also matches every name that starts with `r` (`:618`). Digits: ASN exact or prefix, and a phrase or phrase-prefix match on `name`, `aka` and `name_long`, so `5` matches a name that starts with `5` (`:359-424`). IPv6: also a phrase match on address tokens (`:472-522`). Case folds with the ES `lowercase` filter. One request returns at most 1000 hits in score order, shared by the 6 types (`settings/__init__.py:1516`, `search_v2.py:962-967`). | Text: each word is a case-insensitive substring of one of the fields. Words that the value joins with punctuation must appear with the same punctuation (`gamma-net` does not match `Gamma Net`). `OR` is not an operator, and in a text match `AND` is removed only as a whole word. Digits: the ASN prefix, and a substring of `name`, `aka` or `name_long`. IPv6: a prefix of the stored address only. `lower()` in SQLite folds ASCII letters only, so a stored `MÜNCHEN` does not match `münchen`. Every matching row is returned. | SQL cannot run the tokens, scores and query parser of the search engine. A substring match over the same fields returns the rows that a client expects, and a 1000-hit cap that depends on scores in other types has no use in a filter. Locked by `TestParity_NameSearch/DIVERGENCE_name_search_substring_approximation`. | v1.37.0 (registered 2026-09-26) |
| `?name_search=` with a partial IPv4 address that has a number above 255, for example `net?name_search=999.1.1.1`; and `?name_search=` with no match together with a bad value in a model-field filter, `id__in` included, for example `net?name_search=nomatch&asn__lt=abc` or `net?name_search=nomatch&id__in=abc` | The first returns `500`: the IPv4 branch leaves the search term a list, and `construct_query_body` fails on it (2.83.0 `search_v2.py:888-891`, `:618`). The second returns `200` with no rows: `get_queryset` returns before the filter loop when the search has no hit (`rest.py:550-553`). Found by reading the code, not checked against a live upstream. | The first searches the value as text (`200`). The second returns `400`, as for any bad filter value. | A `500` is an upstream bug. The mirror checks every filter value before it runs the query, and whether the search has a hit is known only when the query runs. On the 7 types without a search index, and for a value that can match nothing, the result is known before the query, and the mirror matches upstream. Locked by `TestParity_NameSearch/DIVERGENCE_name_search_error_status`. | v1.37.0 (registered 2026-09-26) |
| A single-object GET that upstream answers with a server error: a negative `skip`, unless the request has a `name_search` that matches no row, for example `/api/net/<id>?skip=-1`; a `name_search` value on `org`, `fac`, `ix`, `net`, `campus` or `carrier` that Python `int()` rejects after the digit test, for example `/api/net/<id>?name_search=%C2%B2`; a filter value that does not parse for the field type, for example `?asn__lt=abc` or `?created__lt=x`, or an empty `__in` on an integer field (`?asn__in=`); and `/api/campus/<id>?facility__in=<ids>` (or another `campus?facility` key) that matches two or more facilities of the campus | Returns `500`. `retrieve` (2.83.0 `rest.py:849-865`) has no error handler, and DRF runs `get_queryset` outside its `404` wrapper. A negative `skip` makes Django raise `ValueError` at the slice (`rest.py:755-760`). `get_queryset` calls `search_v2` outside the `prepare_query` handler (`rest.py:546`), and `int()` raises `ValueError` for a digit such as `²` (`search_v2.py:385`, `:619`; a list returns `400`, `rest.py:824-827`). A filter value error goes to a handler that reads `inst[0]` (`rest.py:647-651`, `:693-701`), which raises a new `TypeError` (a list catches it and returns `400`, `rest.py:828-831`). For the campus key, the join on the facilities (`serializers.py:4852-4869`) returns the campus once for each facility, upstream does not call `distinct()` (`rest.py:115-141`, `:711-716`), and the lookup finds more than one row. Found by reading the code, not checked against a live upstream. | Returns `400` for a negative `skip`, a `name_search` value that `int()` rejects or a filter value that does not parse, as on a list. Returns `404` for an empty integer `__in`, the same as for any filter that matches nothing. Returns the campus for the `campus?facility` key. | A server error has no use to a client. The mirror answers as a list request does. Locked by `TestParity_Status/DIVERGENCE_detail_upstream_server_errors`. | v1.37.0 (registered 2026-09-26) |
| `/api/netixlan` without `?since` | Returns the rows in the order of the MySQL access path, because the query has no `ORDER BY` (2.83.0 `rest.py:747-748`). MySQL can read the `status IN ('ok', 'not-operational')` filter through the `netixlan_status` index (`models.py:6111`), which returns the `not-operational` rows first. A live capture on 2026-09-23 of `/api/netixlan?limit=2` returned two `not-operational` rows ahead of `ok` rows with lower ids. Other filters can also change the upstream order when MySQL reads the rows through a different index. | Always returns `id` order, ascending (see § List order). | An order that depends on the query plan cannot be reproduced, and it changes when the upstream data or indexes change. A fixed order keeps `skip`/`limit` pages stable. Locked by `TestParity_Ordering/DIVERGENCE_netixlan_list_order_is_id_asc`. | v1.28.0 (registered 2026-09-23) |
| `/api/poc?id=<id>` for a contact that the caller's tier cannot read: a `Users` contact for an anonymous caller, or a `Private` contact for any tier | Returns `200` with an empty `data` array. The unique-query `404` check runs before `APIPermissionsApplicator` removes the contact (2.83.0 `rest.py:809-821`), so the status shows that the contact exists: a missing id gets `404`. The same applies to a user who is not a member of the organization that owns a `Private` contact. | Returns `404` (`Entity not found`), the same as for a missing id. | The `poc.visible` privacy policy removes the contact in the query, before the check, so the status does not show whether a hidden contact exists. The mirror is stricter than upstream here. Locked by `TestParity_Status/DIVERGENCE_poc_hidden_id_returns_404`. | v1.28.0 (registered 2026-09-23) |
| An `/api/` request whose `Accept` header names `application/problem+json` with a `q` value above 0, for example `Accept: application/problem+json` | Returns `406` with `{"meta": {"error": "Could not satisfy the request Accept header."}}` when no range in the header matches `application/json` (DRF content negotiation, `rest_framework/negotiation.py:78`). When the header also has `*/*` or `application/json`, an error has the `meta.error` form. | An error has an RFC 9457 `application/problem+json` body. A success response is the normal JSON envelope. The mirror sends no `406`. | Clients that ask for RFC 9457 errors get the same form as from the REST API of the mirror. Upstream clients do not send this media type. Locked by `TestParity_Limit/DIVERGENCE_problem_json_when_accept_names_it`. | v1.37.0 (registered 2026-09-26) |
| A path under `/api/` that names no type, for example `/api/foo` | Returns the `404` HTML page of the web site (`mainsite/urls.py:111`, `views.py:336-340`). | Returns `404` with `{"meta": {"error": "unknown type \"foo\""}}`. | A JSON client can read the error. The status code is the same. Locked by `TestParity_Status/DIVERGENCE_unknown_path_json_404`. | v1.37.0 (registered 2026-09-26) |
| A write method that upstream maps (`POST` on a list, `PUT`, `PATCH` or `DELETE` on an object), `OPTIONS`, and the `Allow` header | Runs the handler. For an anonymous caller: `POST` or `PUT` with no body returns `400` (`No data was supplied with the POST request`, 2.83.0 `rest.py:867-924`), `PATCH` returns `403` (`:970-974`), `DELETE` returns `204`, `403` or `400` (`:978-1020`), and `OPTIONS` returns `200` with DRF metadata. Every response has `Allow` with the methods of the route: `GET, POST, HEAD, OPTIONS` on a list, `GET, PUT, PATCH, DELETE, HEAD, OPTIONS` on an object (`GET` and `GET, PUT` for `ixlan`). A method that the route does not map returns `405` with `Method "<METHOD>" not allowed.`. `as_set` answers an anonymous write with `401` (`Authentication credentials were not provided.`, DRF `views.py:174-180`, `permissions.py:191-199`) and `OPTIONS` with `405`, and sends `Allow: GET` (`rest.py:1396-1399`). | Every method other than `GET` and `HEAD` returns `405` with `{"meta": {"error": "Method \"<METHOD>\" not allowed."}}` and `Allow: GET, HEAD`, and `Allow: GET` on `/api/as_set`. Success responses have no `Allow` header. | The mirror is read-only. Writes go to PeeringDB. Locked by `TestParity_Status/DIVERGENCE_non_get_method_405_read_only`. | v1.37.0 (registered 2026-09-26) |

## Validation Notes

Future conformance auditors reading third-party gotchas documentation (notably pdbfe's upstream-behaviour claims) against the PeeringDB Plus codebase may encounter assertions about upstream behaviour that turn out to be wrong.
This section documents 4 such invalid claims from the v1.16 audit, each with a pinned `peeringdb/peeringdb@<sha>` reference so the authoritative upstream source can be re-read without re-research.
All 4 were re-confirmed against commit `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516` (PeeringDB 2.83.0), the parity anchor as of 2026-09-23.

| Claim | Verdict | Upstream truth | Our implementation |
|-------|---------|----------------|--------------------|
| `net?country=NL` is a valid filter key | **WRONG** | `country` lives on `org`, not `net`. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/serializers.py:3708` — `NetworkSerializer.prepare_query` has no `country` key, and `django-peeringdb/src/django_peeringdb/models/abstract.py`'s Network model has no country field. Callers who want `net` filtered by country must traverse through `org` (e.g. `net?org__country=NL`). | Filter key silently ignored via the unknown-field silent-ignore mechanism — no row-level match, response unfiltered. OTel span attribute `pdbplus.filter.unknown_fields` records the dropped key. Parity-locked by `TestParity_Traversal/unknown_field_silently_ignored_with_otel_attr`. |
| `?limit=0` returns a count-only envelope | **WRONG** | `limit=0` means unlimited. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/rest.py:516` (`limit` defaults to `0`) + `:757-760` (`if limit > 0: qset[skip:skip+limit] else: qset[skip:]`: a non-positive `limit` applies no SQL `LIMIT`). There is no count-only semantic upstream. Callers wanting a count read the length of the returned `data` array (`meta` is the empty `{}` envelope, with no top-level count field). | Unbounded response: when `limit` is `0`, the list query has no SQL `LIMIT` (`wireEntity` in `internal/pdbcompat/registry_funcs.go`). The memory budget (`PDBPLUS_RESPONSE_MEMORY_LIMIT`, default 128 MiB) still applies and returns `413` when the response is too large. Parity-locked by `TestParity_Limit/bare_url_and_zero_both_return_all_rows` and `TestParity_Limit/zero_over_budget_returns_413`. |
| Unicode folding uses MySQL collation (`utf8_general_ci` or similar) | **WRONG** | Folding is Python-side via `unidecode.unidecode(v)` at query time. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/rest.py:597` — the call happens in the Python filter construction layer before any SQL is emitted, so the database collation is irrelevant. | peeringdb-plus uses shadow `<field>_fold` columns populated at sync time via `internal/unifold.Fold` (`golang.org/x/text/unicode/norm` NFKD decomposition + a hand-rolled ligature map for `ß`/`æ`/`œ`/`ø`/`ł`/`þ`/`đ`/`ð`/dotless `ı`). `__contains` / `__startswith` route to `<field>_fold LIKE ?` with `unifold.Fold(query)` on the RHS. Not byte-compatible with Python `unidecode` for every input (e.g. the two libraries handle rare CJK edge cases differently); any specific gap that surfaces will be logged as a new § Known Divergences row. Parity-locked by `TestParity_Unicode/net_name_contains_diacritic_matches_ascii`, `fac_city_cjk_roundtrip`, and `combining_mark_NFKD_equivalent`. |
| Filter surface is a DRF `filterset_class` per ViewSet | **WRONG** | Filter surface is a per-serializer `prepare_query(...)` method plus an auto-`queryable_relations()` mechanism with a `FILTER_EXCLUDE` denylist. See `peeringdb/peeringdb@465931c0c03df32c5c956699eff4c5308a064516:src/peeringdb_server/serializers.py:970` (`queryable_relations()`) and `:136-166` (`FILTER_EXCLUDE`). No `django_filters.FilterSet` subclass exists anywhere in the upstream codebase. | Path A = `pdbcompat.WithPrepareQueryAllow` ent-schema annotations → `allowlist_gen.go` `Allowlists` map (13 entries derived from upstream `prepare_query` seed lists and `queryable_relations()`); Path B = ent edge introspection via the generated `Edges` map, plus the column edges declared in `schema.ColumnEdges` (today netixlan `ix_side`). The `WithFilterExcludeFromTraversal` edge annotation is the edge-level counterpart of upstream's `FILTER_EXCLUDE`, currently empty across all 13 schemas (every FK edge exposed in v1.16). Upstream's field-level entries have no counterpart (see § Known Divergences). Parity-locked by `TestParity_Traversal/path_a_1hop_org_name` and `path_b_1hop_org_city`. |

An earlier revision of this section also listed one claim from the 2026-05-30 audit as wrong: that `org_flags` is a valid filter on `/api/org`.
The claim is correct.
`org_flags` is an upstream model column (2.83.0 `models.py:1259-1264`).
Upstream filters on it but does not serialize it, so the mirror ignores the key.
See § Known Divergences.

An earlier revision also listed the claim that the default list order is `id ASC` as wrong.
It cited the `django-handleref` base `Meta.ordering = ('-updated', '-created')`.
The claim is correct.
The django-peeringdb abstract bases declare their own `class Meta` without subclassing the handleref `Meta`, so the 13 models do not inherit that ordering.
The upstream migrations record no `ordering` option for them (2.83.0 `migrations/0001_initial.py`).
A list without `?since` has no `ORDER BY` (`rest.py:747-748`), and MySQL returns primary-key order.
A live capture on 2026-09-23 returned the lowest ids first on every type except netixlan.
See § List order and § Known Divergences.

Quarterly re-validation against upstream is a manual review against the pinned commit above — it does not block merges.
Drift that invalidates a Validation Note row should be surfaced as a GitHub issue and reviewed against the parity test suite; if upstream has changed semantics, update the row here and flip or retain the matching parity assertion as a new § Known Divergences row.
