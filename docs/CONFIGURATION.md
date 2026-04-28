<!-- generated-by: gsd-doc-writer -->
# Configuration

All PeeringDB Plus configuration is supplied via environment variables. The
application follows a fail-fast initialization model (GO-CFG-1): invalid or
out-of-range values cause startup to abort with a descriptive error, and
configuration is treated as immutable after `config.Load()` returns
(GO-CFG-2).

The authoritative loader is `internal/config/config.go` (function `Load`,
struct `Config`). Values not parsed there — most notably the OpenTelemetry
exporter selection, the `PDBPLUS_LOG_LEVEL` filter, and the LiteFS/Fly.io
attribution variables — are consumed directly by `internal/otel/provider.go`,
`internal/otel/logger.go`, `internal/litefs/primary.go`, or the `autoexport`
SDK package and are documented in their own sections below.

## Environment Variables

### Application Configuration

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_LISTEN_ADDR` | No | `:8080` | string | HTTP listen address. Must contain `:`. Overridden by `PDBPLUS_PORT` when that is set. |
| `PDBPLUS_PORT` | No | (unset) | string | Convenience override. When non-empty, the listener is forced to `:${PDBPLUS_PORT}`, ignoring `PDBPLUS_LISTEN_ADDR`. |
| `PDBPLUS_DB_PATH` | No | `./peeringdb-plus.db` | path | SQLite database file path. Empty string is rejected at startup. In production (Fly.io) this is set to `/litefs/peeringdb-plus.db` via `fly.toml`. |
| `PDBPLUS_PEERINGDB_URL` | No | `https://api.peeringdb.com` | URL | PeeringDB API base URL. Must use `https://`, or `http://` against loopback (`localhost`, `127.0.0.1`, `::1`) or RFC 1918 private ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`). Other `http://` hosts and any non-http(s) scheme are rejected at startup. |
| `PDBPLUS_PEERINGDB_API_KEY` | No | (empty) | secret | **Recommended.** Optional PeeringDB API key. Empty value means unauthenticated requests. Also read directly by the `pdbcompat-check` CLI and the live conformance / client tests. See [Privacy & Tiers](#privacy--tiers) for the operational implication. |
| `PDBPLUS_PUBLIC_TIER` | No | `public` | enum | Effective privacy tier for anonymous callers. Accepted values are case-sensitive lowercase only: `public` (default — anonymous callers see only rows with `visible="Public"`) or `users` (private-instance escape hatch — anonymous callers are treated as Users-tier and see `visible="Users"` rows too; the application logs `slog.Warn("public tier override active", …)` at startup). Any other value (including case variants like `Users` / `PUBLIC` and whitespace-padded forms) is rejected at startup per GO-CFG-1 — the strict switch is a fail-safe-closed choice so a typo cannot silently default to either tier. See [Privacy & Tiers](#privacy--tiers). |
| `PDBPLUS_CORS_ORIGINS` | No | `*` | string | Comma-separated list of allowed CORS origins. |
| `PDBPLUS_CSP_ENFORCE` | No | `false` | bool | When `true`, serve the enforcing `Content-Security-Policy` header on `/ui/` and `/graphql`. Default `false` serves `Content-Security-Policy-Report-Only` — enforcement is opt-in per deploy through v1.13 (SEC-07 rollout). |
| `PDBPLUS_DRAIN_TIMEOUT` | No | `10s` | duration | Graceful shutdown drain timeout. Must be greater than 0. |
| `PDBPLUS_RESPONSE_MEMORY_LIMIT` | No | `128MB` | byte size | Per-response memory budget (bytes). pdbcompat list handlers run a pre-flight `SELECT COUNT(*) × typical_row_bytes` heuristic; requests whose estimated response size exceeds this budget receive an RFC 9457 413 problem-detail up-front before any row data is materialised. **Unit suffix is mandatory** (`KB`/`MB`/`GB`/`TB`, base 1024; `K`/`M`/`G`/`T` are accepted as aliases). A bare number is rejected. Literal `0` disables the check (local development only — the guardrail is the reason Phase 68's `limit=0` unlimited semantic is safe to expose in prod). Default sized against the 256 MB replica cap minus an 80 MB Go runtime baseline and 48 MB slack for other in-flight requests + GC overhead (Phase 71 D-05). Must be non-negative. |
| `PDBPLUS_STREAM_TIMEOUT` | No | `60s` | duration | Maximum duration for a single streaming RPC. |

### Sync Worker

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_SYNC_TOKEN` | No | (empty) | secret | Shared secret for the `POST /sync` on-demand trigger. When empty, the endpoint rejects every request — on-demand sync is effectively disabled. Compared in constant time against the `X-Sync-Token` request header. |
| `PDBPLUS_SYNC_INTERVAL` | No | `1h` (unauthenticated) / `15m` (when `PDBPLUS_PEERINGDB_API_KEY` is set) | duration | Duration between automatic sync runs. Default is auth-conditional: `15m` when an API key is configured, `1h` otherwise. Explicit value overrides both defaults. Must be greater than 0. See [Sync cadence](#sync-cadence) for the rationale. |
| `PDBPLUS_SYNC_MODE` | No | `incremental` | enum | Sync strategy. Accepted values: `full` (complete re-fetch), `incremental` (only objects modified since last sync, using `?since=<unix-ts>`). Default flipped from `full` to `incremental` on 2026-04-26 (v1.17.0 / quick task `260426-pms`) after empirical confirmation that upstream PeeringDB emits `status="deleted"` tombstones on `?since=` responses (resolving SEED-001's prerequisite). `full` remains a supported operator override for first-sync, recovery, and as an escape-hatch. Any other value is rejected at startup. |
| `PDBPLUS_SYNC_STALE_THRESHOLD` | No | `24h` | duration | Maximum age of sync data before `/readyz` reports the service as degraded. |
| `PDBPLUS_SYNC_MEMORY_LIMIT` | No | `400MB` | byte size | Peak Go heap ceiling checked after the sync worker's Phase A fetch pass. If `runtime.ReadMemStats` reports `HeapAlloc` above this value, the sync aborts with a WARN log and returns `sync.ErrSyncMemoryLimitExceeded`; the next scheduled cycle retries normally. **Unit suffix is mandatory** (`KB`/`MB`/`GB`/`TB`, base 1024; `K`/`M`/`G`/`T` are accepted as aliases). A bare number is rejected. Literal `0` disables the guardrail (local development only). Must be non-negative. |
| `PDBPLUS_PEERINGDB_RPS` | No | `2.0` | float (req/sec) | Sustained requests-per-second cap to the upstream PeeringDB API. Burst is hardcoded at 1 in the client. Authenticated requests (`PDBPLUS_PEERINGDB_API_KEY` set) override this to 60 req/min — the upstream auth quota is fixed and cannot be exceeded by operator preference. Quick task `260428-2zl`. Values ≤ 0 are rejected at startup. The transport (`internal/peeringdb/transport.go`) records per-request wait time on the `pdbplus.peeringdb.rate_limit_wait_ms` histogram for dashboard observability. |
| `PDBPLUS_FK_BACKFILL_MAX_REQUESTS_PER_CYCLE` | No | `20` | non-negative integer | Maximum **underlying HTTP requests** issued by FK-backfill per sync cycle. v1.18.5 semantic shift: the previous cap counted rows, which became a weak circuit breaker once v1.18.4 batching collapsed N rows into 1 request via `?id__in=`. This now bounds the actual upstream surface — at 1 req/sec authenticated, 20 requests ≈ 20s of upstream pressure per cycle. With internal `FetchByIDsBatchSize=100`, each request can carry up to 100 IDs, so 20 requests cover up to 2,000 missing-parent rows per cycle. When `fkCheckParent` finds a missing parent (cache miss + DB miss), the worker attempts one batched fetch via `?since=1&id__in=<csv>` to recover rows before declaring children orphaned (the `since=1` path returns both `ok` and `deleted` rows per upstream `rest.py:694-727`). A per-cycle dedup cache prevents repeat fetches for the same `(type, id)` pair; v1.18.3 added recursive grandparent backfill so a missing parent's own missing parents are chained-in before the parent upserts. When the cap is reached, additional missing parents fall through to drop-on-miss with the `pdbplus.sync.fk_backfill{result="ratelimited"}` counter incremented. Set to `0` to disable backfill entirely (operator escape-hatch). |
| `PDBPLUS_FK_BACKFILL_TIMEOUT` | No | `5m` | Go duration | Per-cycle wall-clock budget for FK-backfill HTTP activity. Backfill calls happen inside the sync transaction; without a deadline a cascade of slow / rate-limited backfills could hold the tx open for tens of minutes, stalling LiteFS replication. After the deadline, `fkBackfillParent` short-circuits to drop-on-miss with the `pdbplus.sync.fk_backfill{result="deadline_exceeded"}` counter incremented; the rest of the sync (bulk fetches + upserts) commits cleanly and the next cycle picks up where we left off. Set to `0` (or any negative duration) to disable the deadline (only the cap applies). v1.18.3. |

#### WAF behavior

On HTTP 403 responses the transport (`internal/peeringdb/transport.go`)
sniffs the response body (first 4 KiB) for WAF signatures: `AWS WAF`,
`Request blocked`, `Access Denied`, `<title>403 Forbidden</title>`. On
match the client logs WARN with full response headers attached and
returns the `errWAFBlocked` sentinel without retrying. Retrying within
the same source IP is futile against an IP-level block. Non-WAF 403
responses fall through to the existing API-key auth-error path.

Quick task `260428-2zl`. Operators can detect WAF blocks at the
`errors.Is(..., errWAFBlocked)` boundary via `peeringdb.IsWAFBlocked(err)`.

#### Sync cadence

`PDBPLUS_SYNC_INTERVAL` defaults to **15 minutes** when `PDBPLUS_PEERINGDB_API_KEY`
is non-empty and **1 hour** otherwise. Authenticated callers have a much higher
PeeringDB rate-limit budget, so the tighter cadence keeps the mirror fresher
without risking throttling; unauthenticated deployments stay on the
conservative 1h default to avoid burning the shared anonymous ceiling.

Override precedence is explicit-wins: setting `PDBPLUS_SYNC_INTERVAL=5m`
forces 5-minute syncs regardless of whether an API key is configured. An
unset `PDBPLUS_SYNC_INTERVAL` selects the auth-conditional default; an
empty string (`PDBPLUS_SYNC_INTERVAL=`) is treated as unset. On startup
the effective interval, authentication state, and whether the operator
supplied an explicit override are announced in a single structured log
line (`sync interval configured`) — the API key itself is never logged.

### Removed in v1.16

| Variable | Removed in | Replacement | Migration |
|----------|------------|-------------|-----------|
| `PDBPLUS_INCLUDE_DELETED` | v1.16 (Phase 68, D-01) | None — deleted rows are always persisted as tombstones (soft-delete, Plan 68-02) and exposed via `?since=N` / pk lookup per upstream `rest.py:694-727` status × since matrix. | Remove from your environment. During the v1.16 → v1.17 grace period a startup WARN is emitted; v1.17 upgrades this to a fatal startup error. |

### Observability

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_OTEL_SAMPLE_RATE` | No | `1.0` | float | Trace sampling ratio passed to `sdktrace.TraceIDRatioBased`. Must be in the inclusive range `[0.0, 1.0]`. Values outside this range are rejected at startup. |
| `PDBPLUS_LOG_LEVEL` | No | `INFO` | enum | Minimum severity for log records shipped via the OTel logging pipeline (and from there to Loki). Accepted values (case-insensitive, parsed via `slog.Level.UnmarshalText`): `DEBUG`, `INFO`, `WARN`, `ERROR`. The stdout (Fly log) handler is independently gated at INFO and is not affected by this variable. Default `INFO` was chosen so DEBUG records remain local for opt-in debugging without polluting production Loki ingestion volume — Phase 77 OBS-06 audit found 52 DEBUG records / 30 min / 8 machines reaching Loki under the prior unfiltered configuration. Invalid values fall back to `INFO` with no error (logging-level config is operator-friendly; CLAUDE.md GO-CFG-1 normally prefers fail-fast, but a malformed log level should not take production down). Consumed by `internal/otel/logger.go` `otelLevelFromEnv()` — not parsed by `internal/config`. |
| `PDBPLUS_HEAP_WARN_MIB` | No | `400` | integer (MiB) | Peak Go heap (MiB) threshold checked at end of every sync cycle. When `runtime.MemStats.HeapInuse` exceeds this value, the worker emits `slog.Warn("heap threshold crossed", …)` with typed attrs (`peak_heap_bytes`, `heap_warn_bytes`, `heap_over`, etc.). The OTel span attribute `pdbplus.sync.peak_heap_bytes` (Prometheus gauge `pdbplus_sync_peak_heap_bytes`) emits on every cycle regardless. `0` disables the warn. Sustained breach across multiple cycles re-fires the SEED-001 trigger (revisit incremental-sync defaults / phase planning). Default sits comfortably under the 512 MB Fly VM cap so the failure order is `log → app crash → Fly OOM-kill`. Observed v1.17.0 baseline (2026-04-17): primary peak ~84 MiB, replicas 58–59 MiB — ~4.5× headroom. **Bare integer only** — no unit suffix (the variable name encodes the unit); `400MB` is rejected. Negative values are rejected at startup. |
| `PDBPLUS_RSS_WARN_MIB` | No | `384` | integer (MiB) | Peak OS RSS (MiB) threshold derived from `/proc/self/status` `VmHWM` (Linux only). Same warn semantics as `PDBPLUS_HEAP_WARN_MIB`. The OTel span attr `pdbplus.sync.peak_rss_bytes` (Prometheus gauge `pdbplus_sync_peak_rss_bytes`) is omitted on non-Linux platforms (RSS not available). `0` disables the warn. Bare integer only — no unit suffix. |

### LiteFS / Primary Detection

| Variable | Required | Default | Type | Description |
|----------|----------|---------|------|-------------|
| `PDBPLUS_IS_PRIMARY` | No | `true` | bool | Fallback primary-role flag. Consulted only when no LiteFS mount is present (local development). Detection order is: (1) lease file `/litefs/.primary` present → replica; (2) `/litefs/` directory present but no `.primary` file → primary; (3) otherwise parse this variable (default `true`, unparseable values default to `true` for safety). Consumed by `internal/litefs/primary.go` — not parsed by `internal/config`. |

### Fly.io Resource Attribution (read-only)

These variables are injected by the Fly.io runtime and Fly Consul. They are
read at startup by `internal/otel/provider.go` and by the `/sync`
write-forwarding handler in `cmd/peeringdb-plus/main.go`, but are never
loaded via `internal/config`. The application never sets them itself.

The OTel resource attributes emitted by `buildResourceFiltered` use OTel
semconv keys (not custom `fly.*` keys) for everything except `fly.app_name`,
because Grafana Cloud's hosted OTLP receiver only promotes a small allowlist
of resource attrs to Prometheus labels (`service.*`, `cloud.*`, `host.*`,
`k8s.*`); custom `fly.*` keys are silently dropped on the metrics path.

| Env var | Resource attr (semconv) | On metrics? | On traces/logs? | Consumer |
|---------|-------------------------|-------------|-----------------|----------|
| `FLY_REGION` | `cloud.region` (`semconv.CloudRegion`) | yes | yes | OTel resource; `POST /sync` handler — when set on a replica, returns `fly-replay: region=${PRIMARY_REGION}` HTTP 307 to forward writes to the primary region. |
| `FLY_PROCESS_GROUP` | `service.namespace` (`semconv.ServiceNamespace`) | yes | yes | OTel resource. 2-cardinality: `primary` / `replica`. Drives the dashboard's `process_group` template variable. |
| `FLY_MACHINE_ID` | `service.instance.id` (`semconv.ServiceInstanceID`) | **no** (per-VM cardinality stripped) | yes | OTel resource (traces and logs only). Deliberately omitted from the metric resource via the `includeInstanceID` gate to keep cardinality low. |
| `FLY_APP_NAME` | `fly.app_name` (custom key) | dropped by Grafana Cloud allowlist | yes (human grep) | OTel resource; `litefs.yml` substitution. |
| (constant) | `cloud.provider="fly_io"` (`semconv.CloudProviderKey`) | yes | yes | Always-on, 1-cardinality. |
| (constant) | `cloud.platform="fly_io_apps"` (`semconv.CloudPlatformKey`) | yes | yes | Always-on, 1-cardinality. |
| `PRIMARY_REGION` | (not a resource attr) | — | — | `POST /sync` handler; `litefs.yml` lease candidacy. Three-letter Fly region designated as the LiteFS primary candidate. `fly.toml` sets this to `lhr`. <!-- VERIFY: PRIMARY_REGION is fixed at lhr per fly.toml; any override must be reconciled with LiteFS lease configuration --> |
| `FLY_CONSUL_URL` | (not a resource attr) | — | — | `litefs.yml` Consul lease backend. Injected by `fly consul attach`. <!-- VERIFY: FLY_CONSUL_URL is provisioned out-of-band via fly consul attach --> |
| `HOSTNAME` | (not a resource attr) | — | — | `litefs.yml` advertise URL `http://${HOSTNAME}.vm.${FLY_APP_NAME}.internal:20202`. |

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
anonymous callers. Startup logs `slog.Warn("public tier override active", …)`
naming the override so the elevated default is never silent; the OTel attribute
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
| `PDBPLUS_PUBLIC_TIER` | Case-sensitive lowercase `public` or `users` only; any other value (including `Users`, `PUBLIC`, whitespace-padded forms) rejected | `invalid value %q for PDBPLUS_PUBLIC_TIER: must be 'public' or 'users'` |
| `PDBPLUS_SYNC_MEMORY_LIMIT` | `≥ 0`; mandatory unit suffix (`KB`/`MB`/`GB`/`TB`); bare numbers rejected (except literal `0`) | `PDBPLUS_SYNC_MEMORY_LIMIT must be non-negative (0 = disabled)`, plus several parse-level messages. |
| `PDBPLUS_RESPONSE_MEMORY_LIMIT` | `≥ 0`; mandatory unit suffix (`KB`/`MB`/`GB`/`TB`); bare numbers rejected (except literal `0`) | `PDBPLUS_RESPONSE_MEMORY_LIMIT must be non-negative (0 = disabled)`, plus several parse-level messages. |
| `PDBPLUS_HEAP_WARN_MIB` | `≥ 0`; bare non-negative integer only (no unit suffix); `400MB` rejected | `PDBPLUS_HEAP_WARN_MIB must be non-negative (0 = disabled)`, plus parse-level messages. |
| `PDBPLUS_RSS_WARN_MIB` | `≥ 0`; bare non-negative integer only (no unit suffix); `384MB` rejected | `PDBPLUS_RSS_WARN_MIB must be non-negative (0 = disabled)`, plus parse-level messages. |
| `PDBPLUS_SYNC_MODE` | Must be `full` or `incremental` | `invalid sync mode %q for PDBPLUS_SYNC_MODE: must be 'full' or 'incremental'` |

`PDBPLUS_LOG_LEVEL` is **not** in this table by design — invalid values fall
back to `INFO` rather than aborting startup, because a malformed log-level
string is operator-friendly and should not take production down (Phase 77
OBS-06 explicit deviation from GO-CFG-1).

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
- **`PDBPLUS_LOG_LEVEL`** — Parsed once at logger construction by
  `internal/otel/logger.go` `otelLevelFromEnv()`. Invalid values silently
  default to `INFO` (operator-friendly fallback, intentional deviation from
  GO-CFG-1). Changes take effect only on next startup.
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
  `FLY_REGION`, `FLY_PROCESS_GROUP`, `FLY_MACHINE_ID`, `FLY_APP_NAME`, and
  `FLY_CONSUL_URL` are injected by the Fly.io runtime. Secrets such as
  `PDBPLUS_SYNC_TOKEN` and `PDBPLUS_PEERINGDB_API_KEY` are managed with
  `fly secrets set`.
  <!-- VERIFY: Production secret names (PDBPLUS_SYNC_TOKEN, PDBPLUS_PEERINGDB_API_KEY) are configured via `fly secrets set` for app `peeringdb-plus` — this cannot be inferred from the repository alone -->
- **OpenTelemetry collector endpoint** — Set at deploy time through
  `fly secrets set OTEL_EXPORTER_OTLP_ENDPOINT=...` (or the individual
  signal endpoints). <!-- VERIFY: Actual OTLP endpoint URL is deployment-specific and not checked into the repository -->

## Related Documentation

- `internal/config/config.go` — authoritative loader and validator.
- `internal/otel/provider.go` — OTel pipeline setup, resource attribute mapping, and metric views.
- `internal/otel/logger.go` — `PDBPLUS_LOG_LEVEL` parser and OTel log handler filter.
- `internal/litefs/primary.go` — primary detection and env fallback.
- `fly.toml`, `litefs.yml`, `Dockerfile.prod` — deployment manifests.
