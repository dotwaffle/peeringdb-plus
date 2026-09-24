# Configuration

All PeeringDB Plus configuration is supplied via environment variables.
The application follows a fail-fast initialization model:
invalid or out-of-range values cause startup to abort with a descriptive error,
and configuration is treated as immutable after `config.Load()` returns.

The authoritative loader is `internal/config/config.go`
(function `Load`, struct `Config`).
Other variables are read directly by `internal/otel/provider.go`,
`internal/otel/logger.go`, `internal/litefs/primary.go`, `cmd/peeringdb-plus`,
or the `autoexport` SDK package.
These include the OpenTelemetry exporter selection, the `PDBPLUS_LOG_LEVEL`
filter, and the LiteFS and Fly.io variables.
Their own sections below describe them.

## Environment Variables

### Application Configuration

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_LISTEN_ADDR` | No | `:8080` | string | HTTP listen address. Must contain `:`. Overridden by `PDBPLUS_PORT` when that is set. |
| `PDBPLUS_PORT` | No | (unset) | string | Convenience override. When non-empty, the listener is forced to `:${PDBPLUS_PORT}`, ignoring `PDBPLUS_LISTEN_ADDR`. |
| `PDBPLUS_DB_PATH` | No | `./peeringdb-plus.db` | path | SQLite database file path. An empty value selects the default. In production, `fly.toml` sets `/litefs/peeringdb-plus.db`. |
| `PDBPLUS_PEERINGDB_URL` | No | `https://api.peeringdb.com` | URL | PeeringDB API base URL. Must use `https://`, or `http://` against loopback (`localhost`, `127.0.0.1`, `::1`) or RFC 1918 private ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`). Other `http://` hosts and any non-http(s) scheme are rejected at startup. An empty value selects the default. |
| `PDBPLUS_PEERINGDB_API_KEY` | No | (empty) | secret | **Recommended.** Optional PeeringDB API key. Empty value means unauthenticated requests. Also read directly by the `pdbcompat-check` CLI and the live conformance / client tests. See [Privacy & Tiers](#privacy--tiers) for the operational implication. |
| `PDBPLUS_PUBLIC_TIER` | No | `public` | enum | Effective privacy tier for anonymous callers. Accepted values are case-sensitive lowercase only: `public` (default — anonymous callers see only rows with `visible="Public"`) or `users` (private-instance escape hatch — anonymous callers are treated as Users-tier and see `visible="Users"` rows too, but never `visible="Private"` rows; the application logs `slog.Warn("public tier override active", …)` at startup). Any other value (including case variants like `Users` / `PUBLIC` and whitespace-padded forms) is rejected at startup — the strict switch is a fail-safe-closed choice so a typo cannot silently default to either tier. See [Privacy & Tiers](#privacy--tiers). |
| `PDBPLUS_PUBLIC_URL` | No | (empty) | URL | Optional external origin used in generated agent discovery documents and Agent Skill metadata. When empty, the server card, skill index, `llms.txt`, and skill archive use the request `Host` and protocol (`r.TLS`, then `X-Forwarded-Proto`). Set this only when a reverse proxy rewrites `Host`. The value must be an `http` or `https` origin with no userinfo, path, query, or fragment. |
| `PDBPLUS_CORS_ORIGINS` | No | `*` | string | Comma-separated list of allowed CORS origins. An entry can contain one `*` wildcard, for example `https://*.example.net`. The `/mcp` endpoint uses the same list. It rejects a request with `403` when the `Origin` header does not match an entry. It does not check requests that have no `Origin` header. |
| `PDBPLUS_CSP_ENFORCE` | No | `false` | bool | When `true`, serve the enforcing `Content-Security-Policy` header on `/ui/` and `/graphql`. Default `false` sends `Content-Security-Policy-Report-Only`. |
| `PDBPLUS_MAP_TILE_URL` | No | `https://tile.openstreetmap.org/{z}/{x}/{y}.png` | URL template | Browser basemap tile URL. The value must be an absolute HTTP or HTTPS URL, or a root-relative URL. It must contain the `{z}`, `{x}`, and `{y}` placeholders. A custom URL also requires `PDBPLUS_MAP_TILE_ATTRIBUTION`. |
| `PDBPLUS_MAP_TILE_ATTRIBUTION` | With a custom tile URL | `© OpenStreetMap contributors` | HTML string | Visible attribution for the configured map tile service. The browser receives this value and Leaflet shows it on each map. |
| `PDBPLUS_DRAIN_TIMEOUT` | No | `10s` | duration | Graceful shutdown timeout. The HTTP server drain uses this timeout, and the final telemetry flush then uses it again. Keep two times this value below `kill_timeout` in `fly.toml` (30s). Must be greater than 0. |
| `PDBPLUS_RESPONSE_MEMORY_LIMIT` | No | `128MB` | byte size | Per-response memory budget (bytes). pdbcompat list handlers run a pre-flight `SELECT COUNT(*) × typical_row_bytes` heuristic; requests whose estimated response size exceeds this budget receive an RFC 9457 413 problem-detail up-front before any row data is materialized. The same value also limits the total estimated bytes of all `/api` list and detail responses in progress on one machine. A request that would go over this total gets `503` with `Retry-After: 1`. The WARN log `pdbcompat: concurrent budget pool exhausted` records each rejection. **Unit suffix is mandatory** (`KB`/`MB`/`GB`/`TB`, base 1024; `K`/`M`/`G`/`T` are accepted as aliases). The unit is not case-sensitive (`128mb` is valid). A bare number is rejected. Literal `0` disables the check (local development only). The guardrail is the reason the `limit=0` unlimited semantic is safe to expose in production. Default sized against the 256 MB replica cap minus an 80 MB Go runtime baseline and 48 MB slack for other in-flight requests + GC overhead (sized from measured runtime+request overhead). Must be non-negative. |
| `PDBPLUS_STREAM_TIMEOUT` | No | `60s` | duration | Maximum duration for a single streaming RPC. Must be greater than 0 — it is the only bound on stream lifetime (`WriteTimeout` is deliberately unset for gRPC streaming); startup fails otherwise. |

#### Map tiles

The default uses the OpenStreetMap Standard tile service without an API key.
This service supports normal, interactive browser use on a best-effort basis.
It does not provide an SLA or guaranteed capacity for third-party applications.

Follow the [OpenStreetMap tile usage policy](https://operations.osmfoundation.org/policies/tiles/).
Keep the attribution visible and allow the browser to cache tiles.
Do not add bulk download, prefetch, or offline features against the default service.

Set both map variables to use a commercial or self-hosted tile service.
The application adds the validated tile origin to the UI Content Security Policy.
A root-relative URL uses the existing same-origin policy.

The browser can read the tile URL and its query string.
Treat any API key in this URL as a public browser credential.
Apply provider-supported domain and usage restrictions to that key.

### Sync Worker

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_SYNC_TOKEN` | No | (empty) | secret | Shared secret for the `POST /sync` on-demand trigger. When empty, the endpoint rejects every request — on-demand sync is effectively disabled. Compared in constant time against the `X-Sync-Token` request header. |
| `PDBPLUS_SYNC_INTERVAL` | No | `1h` (unauthenticated) / `15m` (when `PDBPLUS_PEERINGDB_API_KEY` is set) | duration | Duration between automatic sync runs. Default is auth-conditional: `15m` when an API key is configured, `1h` otherwise. Explicit value overrides both defaults. Must be greater than 0. See [Sync cadence](#sync-cadence) for the rationale. |
| `PDBPLUS_SYNC_MODE` | No | `incremental` | enum | Sync strategy. `incremental` fetches only the objects that changed since the last sync (`?since=<unix-ts>`). `full` fetches every type completely on every cycle. A first sync does not need `full`. A primary with no successful sync recorded runs a full sync at startup in both modes. In `incremental` mode, `PDBPLUS_FULL_SYNC_INTERVAL` (default `24h`) also forces a periodic full cycle. For one full sync, send `POST /sync?mode=full` (see [Force a full sync](DEPLOYMENT.md#5-force-a-full-sync)). Other values stop startup. |
| `PDBPLUS_SYNC_STALE_THRESHOLD` | No | `24h` | duration | Maximum age of the newest successful sync. When the sync is older, `/readyz` returns 503. Must be greater than 0. |
| `PDBPLUS_SYNC_MEMORY_LIMIT` | No | `400MB` | byte size | Peak Go heap ceiling checked after the sync worker's Phase A fetch pass. If `runtime.ReadMemStats` reports `HeapAlloc` above this value, the sync aborts with a WARN log and returns `sync.ErrSyncMemoryLimitExceeded`. The worker retries the sync after 30s, 2m, and 8m, and then waits for the next scheduled cycle. **Unit suffix is mandatory** (`KB`/`MB`/`GB`/`TB`, base 1024; `K`/`M`/`G`/`T` are accepted as aliases). The unit is not case-sensitive (`400mb` is valid). A bare number is rejected. Literal `0` disables the guardrail (local development only). Must be non-negative. |
| `PDBPLUS_PEERINGDB_RPS` | No | `2.0` | float (req/sec) | Sustained requests-per-second cap to the upstream PeeringDB API. Burst is hardcoded at 1 in the client. Authenticated requests (`PDBPLUS_PEERINGDB_API_KEY` set) override this to 60 req/min — the upstream auth quota is fixed and cannot be exceeded by operator preference. Values ≤ 0 are rejected at startup. The transport (`internal/peeringdb/transport.go`) records per-request wait time on the `pdbplus.peeringdb.rate_limit_wait_ms` histogram for dashboard observability. Sync, FK backfill and the netixlan cascade verification share this budget. |
| `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE` | No | `20` | non-negative integer | Maximum number of HTTP requests that FK backfill sends in one sync cycle. When a row refers to a parent that is not in the database, the worker fetches the missing parents with one `?since=1&id__in=<csv>` request for each parent type (up to 100 IDs per request). It also fetches missing grandparents. It does not fetch the same `(type, id)` two times in one cycle. After the cap, the worker drops the remaining orphan rows (or sets a nullable FK to `NULL`) and increments `pdbplus.sync.fk_backfill{result="ratelimited"}`. At 1 request per second (authenticated), the default adds at most about 20 seconds to a cycle. `0` disables backfill. The same value also caps the requests that verify netixlan cascade candidates, with a count of their own: at most the smaller of this value and 10 in each sync attempt (each retry is a new attempt) and at primary start, usually none. So at the default, one sync attempt sends at most 30 requests for backfill and verification together. `0` also turns that verification off; live connections of a network that was deleted before a sync then stay live. A network that the RIR reclaim deletes during a sync still has its live connections marked in that sync: that path sends no request, and no setting turns it off. To stop it, deploy the previous release. |
| `PDBPLUS_FK_BACKFILL_TIMEOUT` | No | `5m` | Go duration | Per-cycle wall-clock budget for FK-backfill HTTP activity. Backfill calls happen inside the sync transaction; without a deadline a cascade of slow / rate-limited backfills could hold the tx open for tens of minutes, stalling LiteFS replication. After the deadline, `fkBackfillParent` short-circuits to drop-on-miss with the `pdbplus.sync.fk_backfill{result="deadline_exceeded"}` counter incremented; the rest of the sync (bulk fetches + upserts) commits cleanly and the next cycle picks up where we left off. Set to `0` (or any negative duration) to disable the deadline (only the cap applies). |
| `PDBPLUS_SYNC_TIMEOUT` | No | `30m` | Go duration | Wall-clock bound on a single sync attempt (each retry in the 30s/2m/8m ladder gets its own budget). The upstream HTTP client deliberately carries no whole-request timeout — a full-sync body read is legitimately slow — so this watchdog is what stops an upstream body that stalls or trickles forever from wedging the cycle, and with it the worker's running latch, for the life of the process (every later trigger would return `ErrSyncAlreadyRunning` while data went silently stale fleet-wide). Cancellation propagates into in-flight HTTP body reads via the request context. Size it comfortably above the slowest observed full-sync cycle. The netixlan cascade verification adds at most 2 minutes to a cycle. Set to `0` to disable (dev/debug only). Must be non-negative. |
| `PDBPLUS_FULL_SYNC_INTERVAL` | No | `24h` | Go duration | Maximum time between full sync cycles in `incremental` mode. When the newest successful full cycle is older than this value, the next cycle fetches every type completely. It also rewrites rows whose `updated` did not advance, but keeps a stored row whose `updated` is newer than the snapshot's version and not older than the snapshot's newest row, because upstream serves the bare list from an API cache that can be stale. This repairs data that incremental sync cannot see. [Daily full reconcile](ARCHITECTURE.md#daily-full-reconcile) lists these cases. A full cycle also fetches a `?since=` window per type, from the earlier of the cursor and the newest `updated` in the snapshot. The window captures tombstones (bare lists carry only live rows) and rows that changed after upstream built its API cache. `0` disables the forced cycle. The data then stays stale until an operator sends `POST /sync?mode=full`. |

#### WAF behavior

On HTTP 403 responses the transport (`internal/peeringdb/transport.go`) sniffs
the response body (first 4 KiB) for WAF signatures: `AWS WAF`,
`Request blocked`, `Access Denied`, `<title>403 Forbidden</title>`.
On match the client logs WARN with full response headers attached
and returns the `errWAFBlocked` sentinel without retrying.
Retrying within the same source IP is futile against an IP-level block.
Non-WAF 403 responses fall through to the existing API-key auth-error path.

The WARN log line is `WAF block detected on 403; not retrying`.
The sync skips its retry ladder, and the next scheduled cycle tries again.

#### Rate limits (HTTP 429)

When `Retry-After` is 60 seconds or less,
the transport waits and tries again, up to 3 attempts.
Each retry increments `pdbplus.peeringdb.retries{cause="429"}`.
When `Retry-After` is missing or longer than 60 seconds,
the fetch stops at once and logs
`PeeringDB rate-limited, aborting (retry-after exceeds cap)`.
The sync skips its retry ladder, and the next scheduled cycle tries again.

#### Sync cadence

`PDBPLUS_SYNC_INTERVAL` defaults to **15 minutes**
when `PDBPLUS_PEERINGDB_API_KEY` is non-empty and **1 hour** otherwise.
Authenticated callers have a much higher PeeringDB rate-limit budget,
so the tighter cadence keeps the mirror fresher without risking throttling;
unauthenticated deployments stay on the conservative 1h default to avoid burning
the shared anonymous ceiling.

Override precedence is explicit-wins:
setting `PDBPLUS_SYNC_INTERVAL=5m` forces 5-minute syncs regardless of
whether an API key is configured.
An unset or empty `PDBPLUS_SYNC_INTERVAL` selects the auth-conditional default.
With an empty value (`PDBPLUS_SYNC_INTERVAL=`),
the startup log still shows `explicit_override=true`.
On startup the effective interval, authentication state,
and whether the operator supplied an explicit override are announced in a single
structured log line (`sync interval configured`) — the API key itself is never
logged.

### Removed in v1.16

| Variable | Removed in | Replacement | Migration |
|----------|------------|-------------|-----------|
| `PDBPLUS_INCLUDE_DELETED` | v1.16 | None — deleted rows are always persisted as tombstones (soft-delete) and exposed via `?since=N` per upstream 2.83.0 `rest.py:719-750` status × since matrix. | Remove from your environment. The variable is now rejected at startup if set. |

### Observability

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_OTEL_SAMPLE_RATE` | No | `1.0` | float | Trace sampling ratio for the known app surfaces (`/api/`, `/rest/v1/`, `/peeringdb.v1.`, `/graphql`). Unknown-path, scanner-bait, and health-probe ratios are hardcoded (see `docs/ARCHITECTURE.md` § Sampling Matrix). This variable does not change `/mcp`, which uses the 1% default for unknown paths, or `/ui/`, which uses 50%. Must be in the inclusive range `[0.0, 1.0]`. Values outside this range are rejected at startup. |
| `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` | No | `1.0` | float | Trace sampling ratio for scheduled sync cycles. `1.0` traces every cycle. `0` traces no scheduled cycle. A traced cycle includes its step spans (`sync-fetch-<type>`, `sync-upsert-<type>`) and no DB spans (see `PDBPLUS_OTEL_SQL`). Without DB spans, the prod traces of 2026-09-24 have about 58 spans for a full cycle and about 44 for an incremental cycle: about 4.2k spans per day at the 15m interval. This ratio does not apply to a `POST /sync` cycle. The server always traces that cycle, or never traces it with `?trace=0`. Must be in the inclusive range `[0.0, 1.0]`. Values outside this range are rejected at startup. |
| `PDBPLUS_OTEL_SQL` | No | `true` | bool | Emit a per-query OpenTelemetry DB span (via XSAM/otelsql) for every SQL statement on the shared `*sql.DB`: ent's queries and the raw `sync_status` statements alike. On by default; set `PDBPLUS_OTEL_SQL=false` to disable. DB spans are children of the active request span, so volume is bounded by the existing sampler: API-read spans inherit `PDBPLUS_OTEL_SAMPLE_RATE`. A sync cycle emits no DB spans: a full cycle runs thousands of statements, and with their spans its trace is larger than the 5 MB per-trace limit of Grafana Cloud Tempo. `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` sets which scheduled cycles are traced. The ETag watcher runs outside any request: its once-a-second `PRAGMA data_version` probe (nodes without LiteFS) emits no span, and its `sync_status` reads before the node's first successful sync have no parent, so each starts its own trace at the 1% default ratio. |
| `PDBPLUS_LOG_LEVEL` | No | `INFO` | enum | Minimum severity of the log records that go to the OTel log pipeline (and from there to Loki). Values are not case-sensitive: `DEBUG`, `INFO`, `WARN`, `ERROR` (parsed by `slog.Level.UnmarshalText`). The stdout handler (Fly logs) always drops records below `INFO`. At the default, the application drops DEBUG records. With `DEBUG`, DEBUG records go to the OTel pipeline only. An invalid value selects `INFO` and does not stop startup. `internal/otel/logger.go` reads this variable, not `internal/config`. |
| `PDBPLUS_HEAP_WARN_MIB` | No | `400` | integer (MiB) | Peak Go heap (MiB) threshold checked at end of every sync cycle. The compared value is the cycle's true `HeapInuse` high-water mark, sampled after the Phase A fetch and after each type's Phase B upsert, *before* the forced GC that follows a type of 1000 or more rows reclaims the spike. It is not the post-GC end-of-cycle floor. When that per-cycle peak exceeds this value, the worker emits `slog.Warn("heap threshold crossed", …)` with typed attrs (`peak_heap_bytes`, `heap_warn_bytes`, `heap_over`, etc.). The OTel span attribute `pdbplus.sync.peak_heap_bytes` (Prometheus gauge `pdbplus_sync_peak_heap_bytes`) emits on every cycle regardless. `0` disables the warn. Investigate a sustained breach across multiple cycles before the primary reaches its 512 MB limit. Default sits comfortably under the 512 MB Fly VM cap so the failure order is `log → app crash → Fly OOM-kill`. Observed baseline (2026-04-17): primary peak ~84 MiB, replicas 58–59 MiB, about 4.5× headroom. **Bare integer only**, with no unit suffix (the variable name encodes the unit). `400MB` is rejected. Negative values are rejected at startup. |
| `PDBPLUS_RSS_WARN_MIB` | No | `384` | integer (MiB) | Peak OS RSS (MiB) threshold derived from `/proc/self/status` `VmHWM` (Linux only). Same warn semantics as `PDBPLUS_HEAP_WARN_MIB`, but note the lifetime difference: `VmHWM` is a **process-lifetime** high-water mark that includes API-serving load and only resets on restart, whereas the heap peak is per-cycle. The OTel span attr `pdbplus.sync.peak_rss_bytes` (Prometheus gauge `pdbplus_sync_peak_rss_bytes`) is omitted on non-Linux platforms (RSS not available). `0` disables the warn. Bare integer only — no unit suffix. |

### LiteFS / Primary Detection

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_IS_PRIMARY` | No | `true` | bool | Fallback primary-role flag. Consulted only when no LiteFS mount is present (local development). Detection order is: (1) lease file `/litefs/.primary` present → replica; (2) the check of `/litefs/.primary` returns an error other than "not found" → replica; (3) `/litefs/` directory present but no `.primary` file → primary; (4) otherwise parse this variable (default `true` when unset). An unparseable value stops startup, also on Fly.io, so a typo cannot select a cluster role. Consumed by `internal/litefs/primary.go`, not parsed by `internal/config`. |
| `PDBPLUS_LITEFS_METRICS_URL` | No | empty (off) | URL | LiteFS Prometheus metrics endpoint. When it is set, the app reads the endpoint at each OTel metric collection and exports the values as `pdbplus.litefs.*` instruments (see `docs/ARCHITECTURE.md` § OpenTelemetry instrumentation). LiteFS runs only in the Fly.io deployment, so the default is empty and the app registers no LiteFS instruments. `fly.toml` sets `http://localhost:20202/metrics`. A set value must be an `http://` or `https://` URL with a host. When a read fails, the app logs a WARN and exports no LiteFS values until a read succeeds. |

### Fly.io Resource Attribution (read-only)

These variables are injected by the Fly.io runtime and Fly Consul.
`internal/otel/provider.go` reads the resource variables at startup.
`cmd/peeringdb-plus/main.go` reads `FLY_REGION` at startup
for the Web UI and the MCP server.
The `POST /sync` handler (`cmd/peeringdb-plus/sync_handler.go`)
reads `FLY_REGION` and `PRIMARY_REGION` on each request.
`internal/config` never loads them, and the application never sets them.

The OTel resource attributes emitted by `buildResourceFiltered` use OTel semconv
keys (not custom `fly.*` keys) for everything except `fly.app_name`, because
Grafana Cloud's hosted OTLP receiver only promotes a small allowlist of resource
attrs to Prometheus labels (`service.*`, `cloud.*`, `host.*`, `k8s.*`); custom
`fly.*` keys are silently dropped on the metrics path.

| Env var | Resource attr (semconv) | On metrics? | On traces/logs? | Consumer |
|---------|-------------------------|-------------|-----------------|----------|
| `FLY_REGION` | `cloud.region` (`semconv.CloudRegion`) | yes | yes | OTel resource. Region shown on `/ui/about` and in the MCP `peeringdb-plus://service` resource. `POST /sync` handler: on a replica, returns HTTP 307 with `fly-replay: region=${PRIMARY_REGION}`. |
| `FLY_PROCESS_GROUP` | `service.namespace` (`semconv.ServiceNamespace`) | yes | yes | OTel resource. 2-cardinality: `primary` / `replica`. The dashboard `process_group` variable takes its values from `pdbplus_sync_peak_heap_bytes`, which only the primary sends, so it lists only `primary`. Select `All` to include replicas. |
| `FLY_MACHINE_ID` | `service.instance.id` (`semconv.ServiceInstanceID`) | **no** (per-VM cardinality stripped) | yes | OTel resource (traces and logs only). Deliberately omitted from the metric resource via the `includeInstanceID` gate to keep cardinality low. |
| `FLY_APP_NAME` | `fly.app_name` (custom key) | dropped by Grafana Cloud allowlist | yes (human grep) | OTel resource; `litefs.yml` substitution. |
| (constant) | `cloud.provider="fly_io"` (`semconv.CloudProviderKey`) | yes | yes | Always-on, 1-cardinality. |
| (constant) | `cloud.platform="fly_io_apps"` (`semconv.CloudPlatformKey`) | yes | yes | Always-on, 1-cardinality. |
| `PRIMARY_REGION` | (not a resource attr) | n/a | n/a | `POST /sync` handler; `litefs.yml` lease candidacy. Three-letter Fly region designated as the LiteFS primary candidate. `fly.toml` sets `lhr`. If you change it, the LiteFS lease candidates move to the new region. |
| `FLY_CONSUL_URL` | (not a resource attr) | n/a | n/a | `litefs.yml` Consul lease backend. `fly consul attach` sets it as an app secret. Run the command once for each app. |
| `HOSTNAME` | (not a resource attr) | n/a | n/a | `litefs.yml` advertise URL `http://${HOSTNAME}.vm.${FLY_APP_NAME}.internal:20202`. |

### Standard OpenTelemetry Variables (autoexport)

All signals are initialized through
`go.opentelemetry.io/contrib/exporters/autoexport`, which honors the standard
`OTEL_*` environment variables.
The exporter for each signal can be selected independently
(for example, `OTEL_TRACES_EXPORTER=otlp` with
`OTEL_METRICS_EXPORTER=prometheus` and `OTEL_LOGS_EXPORTER=none`) and any signal
can be disabled by setting its `OTEL_*_EXPORTER` variable to `none`.
Supported values follow the OpenTelemetry SDK specification.

Commonly used variables:

| Variable | Purpose |
|----------|---------|
| `OTEL_SERVICE_NAME` | Has no effect. The application always sets `service.name=peeringdb-plus`, and that value replaces the value from the environment. |
| `OTEL_RESOURCE_ATTRIBUTES` | Adds resource attributes. When the application sets one of these keys, its value replaces the environment value: `service.name`, `service.version`, `service.namespace`, `service.instance.id`, `cloud.provider`, `cloud.platform`, `cloud.region`, `fly.app_name`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Base OTLP endpoint (affects traces, metrics, and logs unless overridden per-signal). |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc`, `http/protobuf`, or `http/json`. |
| `OTEL_EXPORTER_OTLP_HEADERS` | Comma-separated list of headers for OTLP requests (e.g., auth tokens). |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Per-signal trace endpoint override. |
| `OTEL_EXPORTER_OTLP_METRICS_ENDPOINT` | Per-signal metric endpoint override. |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | Per-signal log endpoint override. |
| `OTEL_TRACES_EXPORTER` | Trace exporter selection. `none` disables traces. |
| `OTEL_METRICS_EXPORTER` | Metric exporter selection. `none` disables metrics. `prometheus` enables the embedded Prometheus scrape endpoint. |
| `OTEL_LOGS_EXPORTER` | Log exporter selection. `none` disables the OTel log pipeline (slog still emits to stdout via the dual handler). |
| `OTEL_EXPORTER_PROMETHEUS_HOST` | Prometheus exporter bind host when `OTEL_METRICS_EXPORTER=prometheus`. |
| `OTEL_EXPORTER_PROMETHEUS_PORT` | Prometheus exporter port when `OTEL_METRICS_EXPORTER=prometheus`. |

The full list of variables honored by autoexport is documented in the upstream
SDK: see `go.opentelemetry.io/contrib/exporters/autoexport`.

These signal-specific details are enforced by `internal/otel/provider.go`
regardless of exporter selection:

- W3C Trace Context and Baggage are installed as the global text-map
  propagators.
- `http.server.request.body.size` and `http.server.response.body.size`
  instruments are dropped via a metric view (low debugging value, high
  cardinality).
- `http.server.request.duration` uses an explicit-bucket histogram with
  boundaries `[0.01, 0.05, 0.25, 1, 5]` seconds. It keeps only the
  `http.route`, `http.response.status_code` and `network.protocol.version`
  attributes.
- Go runtime metrics (goroutines, heap, GC) are started unconditionally via
  `runtime.Start(runtime.WithMeterProvider(mp))`.

The ConnectRPC interceptor records spans but no `rpc.server.*` metrics
(`otelconnect.WithoutMetrics()` in `cmd/peeringdb-plus/main.go`).
`http.server.request.duration` covers ConnectRPC latency,
with one `http.route` value per service.

## Privacy & Tiers

PeeringDB tags per-row visibility on some entities
(most notably `poc.visible` with values `Public`, `Users`, `Private`).
PeeringDB Plus honors this upstream visibility via an
[ent Privacy policy](./ARCHITECTURE.md#privacy-layer) on the read path.
Two environment variables control the resulting behavior.

### Default behavior: anonymous callers see Public only

With `PDBPLUS_PUBLIC_TIER=public`
(the default),
anonymous callers receive only rows whose upstream visibility is `Public`.
`Users`-tier rows are absent from the response,
not present-with-redacted-fields —
this matches upstream's own anonymous API shape.

### Authenticated sync — Users-tier rows present in DB, filtered on read

When `PDBPLUS_PEERINGDB_API_KEY` is set,
the sync worker fetches both `Public` and `Users`-tier rows from PeeringDB
and writes them into the local database.
If the key belongs to a member of an organization,
upstream also sends that organization's `Private` contacts,
and the sync worker writes them too.
The sync worker bypasses the privacy policy
(via `privacy.DecisionContext(ctx, privacy.Allow)`), so all rows land in the DB.
On the read path the policy still filters `Users`-tier rows out of anonymous
responses, so the anonymous API surface remains `Public`-only.
This is the recommended production configuration (see
[DEPLOYMENT.md](./DEPLOYMENT.md#authenticated-peeringdb-sync-recommended)).

### `PDBPLUS_PUBLIC_TIER=users` — private-instance override

Setting `PDBPLUS_PUBLIC_TIER=users` elevates anonymous callers to Users-tier
for private-instance deployments
where the mirror is not reachable from the public internet.
In this mode the privacy policy admits `Users`-tier rows for anonymous callers.
It never admits `Private` rows.
Upstream shows a `Private` contact only to members of the owning organization
(2.83.0 `permissions.py:336-339`, `signals.py:343-347`),
and the mirror has no organization membership.
A Users-tier caller thus sees the rows that an authenticated PeeringDB user
sees when that user is not a member of the owning organization.
Startup logs `slog.Warn("public tier override active", …)` naming the override
so the elevated default is never silent;
the OTel attribute `pdbplus.privacy.tier=users` also appears on read spans.

Only use this for deployments you would not want indexed by a search engine.
It does not affect the sync worker (which has always had full access).

## Configuration File Format

PeeringDB Plus does not use a configuration file for application settings —
every runtime option is an environment variable.

Two deployment-adjacent files exist in the repository:

- `fly.toml`: the Fly.io deployment manifest.
  It sets `PDBPLUS_LISTEN_ADDR` (`:8080`),
  `PDBPLUS_DB_PATH` (`/litefs/peeringdb-plus.db`), and `PRIMARY_REGION` (`lhr`).
  It defines the `primary` process group
  (`shared-cpu-2x`, 512 MB, `litefs_data` volume)
  and the `replica` process group (`shared-cpu-1x`, 256 MB, no volume).
  It also sets the rolling deploy (`max_unavailable = 0.5`),
  `kill_timeout = 30`, the `always` restart policy, and the `/readyz` HTTP check.
- `litefs.yml` — LiteFS FUSE and lease configuration.
  Uses `${FLY_REGION}`, `${PRIMARY_REGION}`, `${FLY_APP_NAME}`, `${HOSTNAME}`,
  and `${FLY_CONSUL_URL}` substitutions supplied by the Fly.io runtime.

## Required vs Optional Settings

All variables are optional,
except `PDBPLUS_MAP_TILE_ATTRIBUTION` when `PDBPLUS_MAP_TILE_URL` is not the
default.
`internal/config/config.go` holds the defaults.

These validation errors stop startup:

| Variable | Validation rule | Error message |
|----------|-----------------|---------------|
| `PDBPLUS_SYNC_INTERVAL` | `> 0` after duration parse | `PDBPLUS_SYNC_INTERVAL must be greater than 0` |
| `PDBPLUS_OTEL_SAMPLE_RATE` | `0.0 ≤ value ≤ 1.0` | `PDBPLUS_OTEL_SAMPLE_RATE must be between 0.0 and 1.0` |
| `PDBPLUS_OTEL_SYNC_SAMPLE_RATE` | `0.0 ≤ value ≤ 1.0` | `PDBPLUS_OTEL_SYNC_SAMPLE_RATE must be between 0.0 and 1.0` |
| `PDBPLUS_LISTEN_ADDR` | Contains `:` | `PDBPLUS_LISTEN_ADDR must contain ':' (e.g., ':8080' or '0.0.0.0:8080')` |
| `PDBPLUS_PEERINGDB_URL` | `https://` always allowed; `http://` only to loopback or RFC 1918; scheme must be set; host must be set. An empty value selects the default. | Multiple messages, one per rejection class (invalid URL, missing scheme, unsupported scheme, empty host, non-local `http://`). |
| `PDBPLUS_LITEFS_METRICS_URL` | Empty, or an `http://` or `https://` URL with a host | One message for each rejection class (invalid URL, other scheme, empty host). |
| `PDBPLUS_DRAIN_TIMEOUT` | `> 0` after duration parse | `PDBPLUS_DRAIN_TIMEOUT must be greater than 0` |
| `PDBPLUS_SYNC_STALE_THRESHOLD` | `> 0` after duration parse | `PDBPLUS_SYNC_STALE_THRESHOLD must be greater than 0` |
| `PDBPLUS_STREAM_TIMEOUT` | `> 0` after duration parse | `PDBPLUS_STREAM_TIMEOUT must be greater than 0 (it is the only bound on streaming RPC lifetime)` |
| `PDBPLUS_SYNC_TIMEOUT` | `≥ 0` after duration parse | `PDBPLUS_SYNC_TIMEOUT must be non-negative (0 = disabled)` |
| `PDBPLUS_PUBLIC_TIER` | Case-sensitive lowercase `public` or `users` only; any other value (including `Users`, `PUBLIC`, whitespace-padded forms) rejected | `invalid value %q for PDBPLUS_PUBLIC_TIER: must be 'public' or 'users'` |
| `PDBPLUS_SYNC_MEMORY_LIMIT` | `≥ 0`; mandatory unit suffix (`KB`/`MB`/`GB`/`TB`); bare numbers rejected (except literal `0`) | `invalid byte size "<v>" for PDBPLUS_SYNC_MEMORY_LIMIT: must be non-negative`, plus messages for a missing unit, an unknown unit, and overflow. |
| `PDBPLUS_RESPONSE_MEMORY_LIMIT` | `≥ 0`; mandatory unit suffix (`KB`/`MB`/`GB`/`TB`); bare numbers rejected (except literal `0`) | `invalid byte size "<v>" for PDBPLUS_RESPONSE_MEMORY_LIMIT: must be non-negative`, plus messages for a missing unit, an unknown unit, and overflow. |
| `PDBPLUS_HEAP_WARN_MIB` | `≥ 0`; bare non-negative integer only (no unit suffix); `400MB` rejected | `invalid MiB value "<v>" for PDBPLUS_HEAP_WARN_MIB: must be non-negative`, plus messages for a non-integer value and overflow. |
| `PDBPLUS_RSS_WARN_MIB` | `≥ 0`; bare non-negative integer only (no unit suffix); `384MB` rejected | `invalid MiB value "<v>" for PDBPLUS_RSS_WARN_MIB: must be non-negative`, plus messages for a non-integer value and overflow. |
| `PDBPLUS_SYNC_MODE` | Must be `full` or `incremental` | `invalid sync mode %q for PDBPLUS_SYNC_MODE: must be 'full' or 'incremental'` |
| `PDBPLUS_PEERINGDB_RPS` | `> 0` after float parse | `PDBPLUS_PEERINGDB_RPS must be greater than 0` |
| `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE` | `≥ 0` (bare non-negative integer; `0` disables backfill and netixlan cascade verification) | `invalid value "<v>" for PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE: must be non-negative` |
| `PDBPLUS_MAP_TILE_URL` | Root-relative, or `http://` or `https://` with a host; no user information or fragment; contains `{z}`, `{x}`, and `{y}` | `PDBPLUS_MAP_TILE_URL must contain {z}` and similar messages |
| `PDBPLUS_MAP_TILE_ATTRIBUTION` | Required when the tile URL is not the default | `PDBPLUS_MAP_TILE_ATTRIBUTION is required when PDBPLUS_MAP_TILE_URL is customized` |
| `PDBPLUS_PUBLIC_URL` | `http` or `https` origin; no user information, path, query, or fragment | `failed to create agent skill handler` with `validate public URL: ...` |
| `PDBPLUS_INCLUDE_DELETED` | Must be unset or empty | `PDBPLUS_INCLUDE_DELETED was removed in v1.16; ...` |
| `PDBPLUS_IS_PRIMARY` | Empty, or a value that `strconv.ParseBool` accepts | `PDBPLUS_IS_PRIMARY="<v>" is not a boolean (use true/false/1/0)` |
| Any duration, bool, float, or integer variable | Must parse | `invalid duration "<v>" for <VAR>`, and the same form for `bool`, `float`, and `integer` |

`PDBPLUS_LOG_LEVEL` is **not** in this table by design —
invalid values fall back to `INFO` rather than aborting startup,
because a malformed log-level string is operator-friendly
and should not take production down
(an explicit deviation from the fail-fast rule).

Duration-typed variables accept any value parseable by
[`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) (e.g., `500ms`,
`90s`, `2h30m`).
Bool-typed variables accept the values recognized by
[`strconv.ParseBool`](https://pkg.go.dev/strconv#ParseBool)
(`1`/`0`, `t`/`f`, `T`/`F`, `true`/`false`, `TRUE`/`FALSE`, `True`/`False`).

## Runtime Fallbacks

Most validation is fail-fast at startup.
The following values have runtime fallbacks rather than startup validation:

- **`PDBPLUS_IS_PRIMARY`**:
  the role check uses this variable only when `/litefs/` does not exist,
  so on Fly.io the variable does not select the role.
  At each start, also on Fly.io, `litefs.ValidateEnvFallback` stops the
  process when `strconv.ParseBool` cannot parse the value,
  so a typo cannot select a cluster role.
  `litefs.IsPrimaryWithFallback()` reads the variable at each role check:
  at startup, on each `POST /sync`, at each scheduler wakeup,
  and once per second while a sync cycle runs (scheduled or on-demand).
  If a check cannot parse the value, the node acts as a replica.
  The startup check makes this branch unreachable in practice.
- **`PDBPLUS_LOG_LEVEL`** —
  Parsed once at logger construction by `internal/otel/logger.go`
  `otelLevelFromEnv()`.
  Invalid values silently default to `INFO`
  (operator-friendly fallback, intentional deviation from the fail-fast rule).
  Changes take effect only on next startup.
- **`FLY_REGION` / `PRIMARY_REGION`** —
  Read per-request inside the `POST /sync` handler.
  Empty `FLY_REGION` indicates local development;
  an empty `PRIMARY_REGION` produces a `fly-replay: region=` header
  (behavior undefined on Fly.io, intentional for local testing).
- **OTel `autoexport` variables** —
  Changes take effect only on the next startup.
  The SDK providers are constructed once and shut down on termination.

## Per-Environment Overrides

The repository does not ship `.env.development`, `.env.production`,
or any language-level environment manager.
Environment values are supplied by:

- **Local Go execution**: the developer's shell.
  Defaults in `internal/config/config.go` are chosen
  so `./peeringdb-plus` runs with no exports set: listens on `:8080`,
  reads/writes `./peeringdb-plus.db`,
  syncs hourly from `https://api.peeringdb.com`,
  assumes the single process is the primary.
- **Local Docker**: image defaults plus `-e` or `--env-file` flags on
  `docker run`.
  `Dockerfile` sets `PDBPLUS_DB_PATH=/data/peeringdb-plus.db`.
  `Dockerfile.prod` sets no `PDBPLUS_*` variable.
- **Fly.io production**:
  the `[env]` block of `fly.toml` sets `PDBPLUS_LISTEN_ADDR`, `PDBPLUS_DB_PATH`,
  `PDBPLUS_LITEFS_METRICS_URL`, and `PRIMARY_REGION`.
  The Fly.io runtime injects `FLY_REGION`, `FLY_PROCESS_GROUP`,
  `FLY_MACHINE_ID`, and `FLY_APP_NAME`.
  `fly consul attach` sets `FLY_CONSUL_URL` as an app secret.
  Set other secrets, such as `PDBPLUS_SYNC_TOKEN`
  and `PDBPLUS_PEERINGDB_API_KEY`, with `fly secrets set`.
  To see the configured secret names,
  run `fly secrets list --app peeringdb-plus`.
- **OpenTelemetry collector endpoint**:
  set at deploy time with `fly secrets set OTEL_EXPORTER_OTLP_ENDPOINT=...`
  (or the individual signal endpoints).
  The endpoint is a deployment value and is not in the repository.

## Related Documentation

- `internal/config/config.go` — authoritative loader and validator.
- `internal/otel/provider.go` — OTel pipeline setup, resource attribute mapping,
  and metric views.
- `internal/otel/logger.go` —
  `PDBPLUS_LOG_LEVEL` parser and OTel log handler filter.
- `internal/litefs/primary.go` — primary detection and env fallback.
- `fly.toml`, `litefs.yml`, `Dockerfile.prod` — deployment manifests.

## Upstream sync threat model and mitigations

This section lists the risks of upstream fetches,
the signal for each risk, and the variable that controls it.

- **Rate-limit amplification (HTTP 429):** anonymous traffic can get long
  `Retry-After` windows from upstream.
  - Signal: `pdbplus.peeringdb.retries{cause="429"}` and the WARN log
    `PeeringDB rate-limited, aborting (retry-after exceeds cap)`.
  - Action: set `PDBPLUS_PEERINGDB_API_KEY`.
    Without a key, lower `PDBPLUS_PEERINGDB_RPS`.
    With a key, the client uses 1 request per second and ignores this
    variable.
- **Credential leakage (API key):** a leaked key can use your upstream quota,
  and upstream abuse handling then sees your identity.
  - Signal: 429 responses while a key is set,
    or the WARN log `PeeringDB rejected request: API key may be invalid`
    after upstream revokes the key.
  - Action: keep the key only in a Fly secret.
    The application does not log the key.
    The startup logs show only whether a key is set.
    To rotate the key, run `fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key>`.
    This command restarts the machines.
- **Unset sync token (fail-closed):** an empty `PDBPLUS_SYNC_TOKEN` does not
  leave `/sync` open.
  The handler rejects every request with 401, so on-demand sync is disabled.
  The scheduled sync worker is unaffected.
  The risk is availability:
  operators cannot start a recovery sync until a token is set.
  - Signal: the startup WARN log that starts with
    `PDBPLUS_SYNC_TOKEN not set`.
  - Action: set `PDBPLUS_SYNC_TOKEN` in every persistent environment
    where on-demand sync is wanted.
