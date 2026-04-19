<!-- generated-by: gsd-doc-writer -->
# Configuration

All PeeringDB Plus configuration is supplied via environment variables. The
application follows a fail-fast initialization model (GO-CFG-1): invalid or
out-of-range values cause startup to abort with a descriptive error, and
configuration is treated as immutable after `config.Load()` returns
(GO-CFG-2).

The authoritative loader is `internal/config/config.go` (function `Load`,
struct `Config`). Values not parsed there — most notably the OpenTelemetry
exporter selection and the LiteFS/Fly.io attribution variables — are
consumed directly by `internal/otel/provider.go`,
`internal/litefs/primary.go`, or the `autoexport` SDK package and are
documented in their own sections below.

## Environment Variables

### Application Configuration

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_LISTEN_ADDR` | No | `:8080` | string | HTTP listen address. Must contain `:`. Overridden by `PDBPLUS_PORT` when that is set. |
| `PDBPLUS_PORT` | No | (unset) | string | Convenience override. When non-empty, the listener is forced to `:${PDBPLUS_PORT}`, ignoring `PDBPLUS_LISTEN_ADDR`. |
| `PDBPLUS_DB_PATH` | No | `./peeringdb-plus.db` | path | SQLite database file path. Empty string is rejected at startup. In production (Fly.io) this is set to `/litefs/peeringdb-plus.db` via `fly.toml`. |
| `PDBPLUS_PEERINGDB_URL` | No | `https://api.peeringdb.com` | URL | PeeringDB API base URL. Must use `https://`, or `http://` against loopback (`localhost`, `127.0.0.1`, `::1`) or RFC 1918 private ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`). Other `http://` hosts and any non-http(s) scheme are rejected at startup. |
| `PDBPLUS_PEERINGDB_API_KEY` | No | (empty) | secret | **Recommended.** Optional PeeringDB API key. Empty value means unauthenticated requests. Also read directly by the `pdbcompat-check` CLI and the live conformance / client tests. See [Privacy & Tiers](#privacy--tiers) for the operational implication. |
| `PDBPLUS_PUBLIC_TIER` | No | `public` | enum | Effective privacy tier for anonymous callers. Accepted values: `public` (default — anonymous callers see only rows with `visible="Public"`), `users` (private-instance escape hatch — anonymous callers are treated as Users-tier and see `visible="Users"` rows too; logged with WARN at startup). See [Privacy & Tiers](#privacy--tiers). |
| `PDBPLUS_CORS_ORIGINS` | No | `*` | string | Comma-separated list of allowed CORS origins. |
| `PDBPLUS_CSP_ENFORCE` | No | `false` | bool | When `true`, serve the enforcing `Content-Security-Policy` header on `/ui/` and `/graphql`. Default `false` serves `Content-Security-Policy-Report-Only` — enforcement is opt-in per deploy through v1.13 (SEC-07 rollout). |
| `PDBPLUS_DRAIN_TIMEOUT` | No | `10s` | duration | Graceful shutdown drain timeout. Must be greater than 0. |
| `PDBPLUS_STREAM_TIMEOUT` | No | `60s` | duration | Maximum duration for a single streaming RPC. |

### Sync Worker

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_SYNC_TOKEN` | No | (empty) | secret | Shared secret for the `POST /sync` on-demand trigger. When empty, the endpoint rejects every request — on-demand sync is effectively disabled. Compared in constant time against the `X-Sync-Token` request header. |
| `PDBPLUS_SYNC_INTERVAL` | No | `1h` | duration | Duration between automatic sync runs. Must be greater than 0. |
| `PDBPLUS_SYNC_MODE` | No | `full` | enum | Sync strategy. Accepted values: `full` (complete re-fetch), `incremental` (only objects modified since last sync). Any other value is rejected at startup. |
| `PDBPLUS_SYNC_STALE_THRESHOLD` | No | `24h` | duration | Maximum age of sync data before `/readyz` reports the service as degraded. |
| `PDBPLUS_SYNC_MEMORY_LIMIT` | No | `400MB` | byte size | Peak Go heap ceiling checked after the sync worker's Phase A fetch pass. If `runtime.ReadMemStats` reports `HeapAlloc` above this value, the sync aborts with a WARN log and returns `sync.ErrSyncMemoryLimitExceeded`; the next scheduled cycle retries normally. **Unit suffix is mandatory** (`KB`/`MB`/`GB`/`TB`, base 1024; `K`/`M`/`G`/`T` are accepted as aliases). A bare number is rejected. Literal `0` disables the guardrail (local development only). Must be non-negative. |

### Removed in v1.16

| Variable | Removed in | Replacement | Migration |
|----------|------------|-------------|-----------|
| `PDBPLUS_INCLUDE_DELETED` | v1.16 (Phase 68, D-01) | None — deleted rows are always persisted as tombstones (soft-delete, Plan 68-02) and exposed via `?since=N` / pk lookup per upstream `rest.py:694-727` status × since matrix. | Remove from your environment. During the v1.16 → v1.17 grace period a startup WARN is emitted; v1.17 upgrades this to a fatal startup error. |

### Observability

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_OTEL_SAMPLE_RATE` | No | `1.0` | float | Trace sampling ratio passed to `sdktrace.TraceIDRatioBased`. Must be in the inclusive range `[0.0, 1.0]`. Values outside this range are rejected at startup. |

### LiteFS / Primary Detection

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_IS_PRIMARY` | No | `true` | bool | Fallback primary-role flag. Consulted only when no LiteFS mount is present (local development). Detection order is: (1) lease file `/litefs/.primary` present → replica; (2) `/litefs/` directory present but no `.primary` file → primary; (3) otherwise parse this variable (default `true`, unparseable values default to `true` for safety). Consumed by `internal/litefs/primary.go` — not parsed by `internal/config`. |

### Fly.io Resource Attribution (read-only)

These variables are injected by the Fly.io runtime and Fly Consul. They are
read at startup by `internal/otel/provider.go` and by the `/sync`
write-forwarding handler in `cmd/peeringdb-plus/main.go`, but are never
loaded via `internal/config`. The application never sets them itself.

| Variable | Consumed by | Purpose |
|----------|-------------|---------|
| `FLY_REGION` | OTel resource (`fly.region`); `POST /sync` handler | Determines whether the node is running on Fly.io. When set on a replica, `POST /sync` returns a `fly-replay: region=${PRIMARY_REGION}` header with HTTP 307 to forward writes to the primary region. |
| `FLY_MACHINE_ID` | OTel resource (`fly.machine_id`, traces and logs only) | Per-VM attribution for traces and logs. **Deliberately omitted from the metric resource** to keep cardinality low. |
| `FLY_APP_NAME` | OTel resource (`fly.app_name`); `litefs.yml` | Fly application name. |
| `PRIMARY_REGION` | `POST /sync` handler; `litefs.yml` lease candidacy | Three-letter Fly region designated as the LiteFS primary candidate. `fly.toml` sets this to `lhr`. <!-- VERIFY: PRIMARY_REGION is fixed at lhr per fly.toml; any override must be reconciled with LiteFS lease configuration --> |
| `FLY_CONSUL_URL` | `litefs.yml` Consul lease backend | Consul URL used by LiteFS for leader election. Injected by `fly consul attach`. <!-- VERIFY: FLY_CONSUL_URL is provisioned out-of-band via fly consul attach --> |
| `HOSTNAME` | `litefs.yml` advertise URL | Used to build the LiteFS advertise URL `http://${HOSTNAME}.vm.${FLY_APP_NAME}.internal:20202`. |

### Standard OpenTelemetry Variables (autoexport)

All signals are initialized through `go.opentelemetry.io/contrib/exporters/autoexport`,
which honours the standard `OTEL_*` environment variables. The exporter for
each signal can be selected independently (for example, `OTEL_TRACES_EXPORTER=otlp`
with `OTEL_METRICS_EXPORTER=prometheus` and `OTEL_LOGS_EXPORTER=none`) and
any signal can be disabled by setting its `OTEL_*_EXPORTER` variable to
`none`. Supported values follow the OpenTelemetry SDK specification.

Commonly used variables:

| Variable | Purpose |
|----------|---------|
| `OTEL_SERVICE_NAME` | Overrides the service name. The application passes `peeringdb-plus` as a default in `SetupInput.ServiceName`; the SDK merges `OTEL_SERVICE_NAME` over `resource.Default()` if set. |
| `OTEL_RESOURCE_ATTRIBUTES` | Additional resource attributes (merged with `resource.Default()`). |
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

The full list of variables honoured by autoexport is documented in the
upstream SDK: see `go.opentelemetry.io/contrib/exporters/autoexport`.

Three signal-specific details are enforced by `internal/otel/provider.go`
regardless of exporter selection:

- W3C Trace Context and Baggage are installed as the global text-map
  propagators.
- `http.server.request.body.size` and `http.server.response.body.size`
  instruments are dropped via a metric view (low debugging value, high
  cardinality).
- `rpc.server.duration` uses an explicit-bucket histogram with boundaries
  `[0.01, 0.05, 0.25, 1, 5]` seconds.
- Go runtime metrics (goroutines, heap, GC) are started unconditionally via
  `runtime.Start(runtime.WithMeterProvider(mp))`.

## Privacy & Tiers

PeeringDB tags per-row visibility on some entities (most notably `poc.visible`
with values `Public`, `Users`, `Private`). PeeringDB Plus honours this upstream
visibility via an [ent Privacy policy](./ARCHITECTURE.md#privacy-layer) on the
read path. Two environment variables control the resulting behaviour.

### Default behaviour — anonymous callers see Public only

With `PDBPLUS_PUBLIC_TIER=public` (the default), anonymous callers receive only
rows whose upstream visibility is `Public`. `Users`-tier rows are absent from
the response, not present-with-redacted-fields — this matches upstream's own
anonymous API shape.

### Authenticated sync — Users-tier rows present in DB, filtered on read

When `PDBPLUS_PEERINGDB_API_KEY` is set, the sync worker fetches both `Public`
and `Users`-tier rows from PeeringDB and writes them into the local database.
The sync worker bypasses the privacy policy (via
`privacy.DecisionContext(ctx, privacy.Allow)`), so all rows land in the DB.
On the read path the policy still filters `Users`-tier rows out of anonymous
responses, so the anonymous API surface remains `Public`-only. This is the
recommended production configuration (see
[DEPLOYMENT.md](./DEPLOYMENT.md#authenticated-peeringdb-sync-recommended)).

### `PDBPLUS_PUBLIC_TIER=users` — private-instance override

Setting `PDBPLUS_PUBLIC_TIER=users` elevates anonymous callers to Users-tier
for private-instance deployments where the mirror is not reachable from the
public internet. In this mode the privacy policy admits `Users`-tier rows for
anonymous callers. Startup logs a WARN line naming the override so the
elevated default is never silent; the OTel attribute
`pdbplus.privacy.tier=users` also appears on read spans.

Only use this for deployments you would not want indexed by a search engine.
It does not affect the sync worker (which has always had full access).

## Configuration File Format

PeeringDB Plus does not use a configuration file for application settings —
every runtime option is an environment variable.

Two deployment-adjacent files exist in the repository:

- `fly.toml` — Fly.io deployment manifest. Sets the production values for
  `PDBPLUS_LISTEN_ADDR` (`:8080`), `PDBPLUS_DB_PATH`
  (`/litefs/peeringdb-plus.db`), and `PRIMARY_REGION` (`lhr`), along with
  VM sizing (`shared-cpu-2x`, `512mb`), the rolling deploy strategy
  (`max_unavailable = 0.5`), and the `/healthz` HTTP check.
- `litefs.yml` — LiteFS FUSE and lease configuration. Uses `${FLY_REGION}`,
  `${PRIMARY_REGION}`, `${FLY_APP_NAME}`, `${HOSTNAME}`, and
  `${FLY_CONSUL_URL}` substitutions supplied by the Fly.io runtime.

## Required vs Optional Settings

Every variable is **optional** from the perspective of the loader — all have
defaults encoded in `internal/config/config.go`. There are no variables
whose absence aborts startup.

Validation errors (which do abort startup) are produced for:

| Variable | Validation rule | Error message |
|----------|-----------------|---------------|
| `PDBPLUS_DB_PATH` | Non-empty | `PDBPLUS_DB_PATH must not be empty` |
| `PDBPLUS_SYNC_INTERVAL` | `> 0` after duration parse | `PDBPLUS_SYNC_INTERVAL must be greater than 0` |
| `PDBPLUS_OTEL_SAMPLE_RATE` | `0.0 ≤ value ≤ 1.0` | `PDBPLUS_OTEL_SAMPLE_RATE must be between 0.0 and 1.0` |
| `PDBPLUS_LISTEN_ADDR` | Contains `:` | `PDBPLUS_LISTEN_ADDR must contain ':' (e.g., ':8080' or '0.0.0.0:8080')` |
| `PDBPLUS_PEERINGDB_URL` | Non-empty; `https://` always allowed; `http://` only to loopback or RFC 1918; scheme must be set; host must be set | Multiple messages, one per rejection class (empty, missing scheme, unsupported scheme, empty host, non-local `http://`). |
| `PDBPLUS_DRAIN_TIMEOUT` | `> 0` after duration parse | `PDBPLUS_DRAIN_TIMEOUT must be greater than 0` |
| `PDBPLUS_SYNC_MEMORY_LIMIT` | `≥ 0`; mandatory unit suffix (`KB`/`MB`/`GB`/`TB`); bare numbers rejected (except literal `0`) | `PDBPLUS_SYNC_MEMORY_LIMIT must be non-negative (0 = disabled)`, plus several parse-level messages. |
| `PDBPLUS_SYNC_MODE` | Must be `full` or `incremental` | `invalid sync mode %q for PDBPLUS_SYNC_MODE: must be 'full' or 'incremental'` |

Duration-typed variables accept any value parseable by
[`time.ParseDuration`](https://pkg.go.dev/time#ParseDuration) (e.g., `500ms`,
`90s`, `2h30m`). Bool-typed variables accept the values recognised by
[`strconv.ParseBool`](https://pkg.go.dev/strconv#ParseBool)
(`1`/`0`, `t`/`f`, `T`/`F`, `true`/`false`, `TRUE`/`FALSE`, `True`/`False`).

## Runtime Fallbacks

Most validation is fail-fast at startup. The following values have runtime
fallbacks rather than startup validation:

- **`PDBPLUS_IS_PRIMARY`** — Parsed lazily inside
  `litefs.IsPrimaryWithFallback()` on every call (startup sync gating, each
  `POST /sync` request, and every scheduler tick). An unparseable value is
  silently treated as `true` (primary) for safety. The variable is consulted
  only when neither `/litefs/.primary` nor the `/litefs/` directory is
  present, so on Fly.io it is effectively ignored.
- **`FLY_REGION` / `PRIMARY_REGION`** — Read per-request inside the
  `POST /sync` handler. Empty `FLY_REGION` indicates local development; an
  empty `PRIMARY_REGION` produces a `fly-replay: region=` header (behaviour
  undefined on Fly.io, intentional for local testing).
- **OTel `autoexport` variables** — Changes take effect only on the next
  startup. The SDK providers are constructed once and shut down on
  termination.

## Per-Environment Overrides

The repository does not ship `.env.development`, `.env.production`, or any
language-level environment manager. Environment values are supplied by:

- **Local Go execution** — The developer's shell. Defaults in
  `internal/config/config.go` are chosen so `./peeringdb-plus` runs with no
  exports set: listens on `:8080`, reads/writes `./peeringdb-plus.db`,
  syncs hourly from `https://api.peeringdb.com`, assumes the single process
  is the primary.
- **Local Docker** — Image defaults plus `-e` / `--env-file` flags on
  `docker run`. The Dockerfiles do not set `PDBPLUS_*` variables;
  production values come from Fly.io.
- **Fly.io production** — The `[env]` block of `fly.toml` sets
  `PDBPLUS_LISTEN_ADDR`, `PDBPLUS_DB_PATH`, and `PRIMARY_REGION`.
  `FLY_REGION`, `FLY_MACHINE_ID`, `FLY_APP_NAME`, and `FLY_CONSUL_URL` are
  injected by the Fly.io runtime. Secrets such as `PDBPLUS_SYNC_TOKEN` and
  `PDBPLUS_PEERINGDB_API_KEY` are managed with `fly secrets set`.
  <!-- VERIFY: Production secret names (PDBPLUS_SYNC_TOKEN, PDBPLUS_PEERINGDB_API_KEY) are configured via `fly secrets set` for app `peeringdb-plus` — this cannot be inferred from the repository alone -->
- **OpenTelemetry collector endpoint** — Set at deploy time through
  `fly secrets set OTEL_EXPORTER_OTLP_ENDPOINT=...` (or the individual
  signal endpoints). <!-- VERIFY: Actual OTLP endpoint URL is deployment-specific and not checked into the repository -->

## Related Documentation

- `internal/config/config.go` — authoritative loader and validator.
- `internal/otel/provider.go` — OTel pipeline setup and metric views.
- `internal/litefs/primary.go` — primary detection and env fallback.
- `fly.toml`, `litefs.yml`, `Dockerfile.prod` — deployment manifests.
