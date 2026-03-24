# Architecture Patterns

**Domain:** v1.5 Tech Debt & Observability -- Grafana dashboards, meta.generated verification, dead code removal
**Researched:** 2026-03-24
**Focus:** How Grafana dashboard JSON integrates with the existing OTel metric pipeline; where dashboard provisioning files live in the repo; DataLoader middleware removal impact; meta.generated field verification in the sync pipeline

## Existing Architecture (Context)

The v1.4 architecture is a single Go binary with this structure:

```
cmd/peeringdb-plus/main.go     (wiring, HTTP server, graceful shutdown)
  |
  +-- internal/config/          Config from env vars (immutable after load)
  +-- internal/database/        SQLite open (modernc.org, WAL, FK)
  +-- internal/otel/            OTel pipeline: Tracer, Meter, Logger providers (autoexport)
  +-- internal/peeringdb/       PeeringDB API client (HTTP, retry, rate limit, OTel tracing)
  +-- internal/sync/            Sync worker (fetch -> filter -> upsert -> delete, per-type metrics)
  +-- internal/health/          /healthz (liveness), /readyz (readiness + sync freshness)
  +-- internal/middleware/       Logging, Recovery, CORS
  +-- internal/litefs/          LiteFS primary detection
  +-- internal/graphql/         GraphQL handler factory (gqlgen server config)
  +-- internal/pdbcompat/       PeeringDB-compatible REST layer (13 types, Django-style filters)
  +-- internal/web/             Web UI (templ + htmx + Tailwind CSS)
  +-- graph/                    gqlgen resolvers, generated code
  +-- ent/                      entgo ORM (13 schemas), generated code
  +-- ent/schema/               Schema definitions with entgql + entrest annotations
  +-- ent/rest/                 entrest-generated REST handlers
```

**Existing OTel metrics (internal/otel/metrics.go):**

| Metric Name | Type | Attributes | Purpose |
|-------------|------|------------|---------|
| `pdbplus.sync.duration` | Float64Histogram | `status` | Total sync duration in seconds |
| `pdbplus.sync.operations` | Int64Counter | `status` | Count of sync operations (success/failed) |
| `pdbplus.sync.type.duration` | Float64Histogram | `type` | Per-type sync step duration |
| `pdbplus.sync.type.objects` | Int64Counter | `type` | Objects synced per type |
| `pdbplus.sync.type.deleted` | Int64Counter | `type` | Objects deleted per type |
| `pdbplus.sync.type.fetch_errors` | Int64Counter | `type` | PeeringDB API fetch errors per type |
| `pdbplus.sync.type.upsert_errors` | Int64Counter | `type` | Database upsert errors per type |
| `pdbplus.sync.type.fallback` | Int64Counter | `type` | Incremental-to-full fallback events |
| `pdbplus.sync.freshness` | Float64ObservableGauge | (none) | Seconds since last successful sync |

**Automatic metrics (from otelhttp and runtime):**

| Source | Metric Names (Prometheus format) | Purpose |
|--------|----------------------------------|---------|
| `otelhttp` middleware | `http_server_request_duration_seconds`, `http_server_active_requests`, `http_server_request_body_size_bytes`, `http_server_response_body_size_bytes` | HTTP RED metrics |
| `otelhttp` transport | `http_client_request_duration_seconds`, `http_client_active_requests` | Outbound PeeringDB API call metrics |
| `runtime.Start()` | `process_runtime_go_goroutines`, `process_runtime_go_mem_alloc_bytes`, `process_runtime_go_gc_*` | Go runtime metrics |

**OTel resource attributes (set in provider.go):**
- `service.name: peeringdb-plus`
- `service.version: <from build info>`
- `fly.region: <FLY_REGION>`
- `fly.machine_id: <FLY_MACHINE_ID>`
- `fly.app_name: <FLY_APP_NAME>`

## Recommended Architecture

### Component 1: Prometheus Metrics Endpoint (New -- Zero Code Changes)

**Critical finding:** The existing `autoexport.NewMetricReader(ctx)` in `internal/otel/provider.go` already supports Prometheus exposition via environment variables. No Go code changes are required to expose a Prometheus scrape endpoint.

**Configuration (fly.toml changes only):**

```toml
[env]
  OTEL_METRICS_EXPORTER = "prometheus"
  OTEL_EXPORTER_PROMETHEUS_HOST = "0.0.0.0"
  OTEL_EXPORTER_PROMETHEUS_PORT = "9091"

[metrics]
  port = 9091
  path = "/metrics"
```

**How it works:**
1. `autoexport.NewMetricReader(ctx)` reads `OTEL_METRICS_EXPORTER=prometheus`
2. Creates a `go.opentelemetry.io/otel/exporters/prometheus` exporter
3. Starts an HTTP server on `0.0.0.0:9091/metrics`
4. Fly.io scrapes this endpoint every 15 seconds automatically
5. All custom and automatic metrics are exposed
6. Metrics become available in Fly.io's managed Grafana at fly-metrics.net

**OTel-to-Prometheus metric name conversion (automatic):**

| OTel Name | Prometheus Name | Type |
|-----------|----------------|------|
| `pdbplus.sync.duration` | `pdbplus_sync_duration_seconds_bucket/sum/count` | histogram |
| `pdbplus.sync.operations` | `pdbplus_sync_operations_total` | counter |
| `pdbplus.sync.type.duration` | `pdbplus_sync_type_duration_seconds_bucket/sum/count` | histogram |
| `pdbplus.sync.type.objects` | `pdbplus_sync_type_objects_total` | counter |
| `pdbplus.sync.type.deleted` | `pdbplus_sync_type_deleted_total` | counter |
| `pdbplus.sync.type.fetch_errors` | `pdbplus_sync_type_fetch_errors_total` | counter |
| `pdbplus.sync.type.upsert_errors` | `pdbplus_sync_type_upsert_errors_total` | counter |
| `pdbplus.sync.type.fallback` | `pdbplus_sync_type_fallback_total` | counter |
| `pdbplus.sync.freshness` | `pdbplus_sync_freshness_seconds` | gauge |
| `http.server.request.duration` | `http_server_request_duration_seconds_*` | histogram |

**Trade-off:** `OTEL_METRICS_EXPORTER=prometheus` disables the OTLP metric exporter. For v1.5, Fly.io's built-in Prometheus scraping is sufficient.

### Component 2: Grafana Dashboard JSON Files (New)

**File location:** `deploy/grafana/dashboards/`

Rationale: `deploy/` is conventional for deployment artifacts in Go projects. Dashboard JSON is deployment config, not application code.

**Directory structure:**

```
deploy/
  grafana/
    dashboards/
      sync-health.json         # Sync operations, duration, freshness, per-type breakdown
      api-traffic.json         # HTTP request rate, latency, error rate by API surface
      infrastructure.json      # Go runtime metrics, memory, goroutines
      business-metrics.json    # Object counts, growth trends, coverage stats
    provisioning/
      dashboards.yaml          # Grafana dashboard provider config (for self-hosted)
```

**Dashboard JSON format:** Classic schema (pre-v12.2). Compatible with Grafana 9+. Use `${DS_PROMETHEUS}` variable datasource references for portability.

**PromQL query examples:**

Sync Health:
- Success rate: `rate(pdbplus_sync_operations_total{status="success"}[1h])`
- Duration p99: `histogram_quantile(0.99, rate(pdbplus_sync_duration_seconds_bucket[1h]))`
- Freshness: `pdbplus_sync_freshness_seconds`
- Per-type objects: `rate(pdbplus_sync_type_objects_total[1h])`
- Fetch errors: `rate(pdbplus_sync_type_fetch_errors_total[5m])`

API Traffic:
- Request rate: `rate(http_server_request_duration_seconds_count[5m])`
- Latency p95: `histogram_quantile(0.95, rate(http_server_request_duration_seconds_bucket[5m]))`
- Error rate: `rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[5m])`

Infrastructure:
- Goroutines: `process_runtime_go_goroutines`
- Memory: `process_runtime_go_mem_alloc_bytes`
- GC pause: `process_runtime_go_gc_pause_total_seconds`

### Component 3: WorkerConfig.IsPrimary Removal (Modification)

The `IsPrimary bool` field in `internal/sync/worker.go` `WorkerConfig` struct is dead code -- replaced by LiteFS detection via `internal/litefs/`. Safe to remove along with any references in config setup.

### Component 4: DataLoader Middleware Removal (Modification)

DataLoader middleware was wired in v1.0 but entgql handles N+1 prevention natively via eager-loading annotations. The DataLoader package was already deleted in v1.2 lint cleanup. Any remaining references in main.go or middleware wiring should be removed.

### Component 5: meta.generated Field Verification (Investigation + Code Change)

**Approach:** Test the live PeeringDB API at depth=0 with pagination to verify whether `meta.generated` is present in responses. If absent for paginated depth=0 responses:
1. Add graceful fallback in `internal/peeringdb/client.go` FetchAll
2. Use `time.Now()` or skip the field when not present
3. Log at DEBUG level when falling back (not WARN -- expected behavior)

### Suggested Build Order

1. **Tech debt cleanup** (dead code removal) -- no external dependencies
2. **meta.generated verification** -- investigate then fix, foundation for correct sync
3. **Prometheus metrics exposure** -- fly.toml config change, deploy
4. **Verify deferred items** -- requires live deployment, can happen after deploy
5. **Grafana dashboards** -- requires live metrics data in Prometheus, must come last

## Confidence Assessment

| Component | Confidence | Notes |
|-----------|------------|-------|
| Prometheus via autoexport | HIGH | Documented, dependencies already present |
| Dashboard file location | HIGH | Convention-based, no technical constraints |
| PromQL queries | MEDIUM | Standard patterns, need verification against live scrape |
| DataLoader removal | HIGH | Package already deleted, just wiring cleanup |
| WorkerConfig.IsPrimary | HIGH | Dead field confirmed by code search |
| meta.generated behavior | LOW | Must test against live API to determine |
