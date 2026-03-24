# Phase 19: Prometheus Metrics & Grafana Dashboard - Research

**Researched:** 2026-03-24
**Domain:** OTel metrics verification via OTLP/Grafana Cloud, Grafana dashboard JSON authoring, Go observable gauge instrumentation
**Confidence:** HIGH

## Summary

Phase 19 delivers observability through two work streams: (1) verifying that existing OTLP metrics arrive correctly in Grafana Cloud Mimir and registering new observable gauges for per-type object counts, and (2) authoring a comprehensive Grafana dashboard JSON covering five rows (sync health, HTTP RED, per-type sync, Go runtime, business metrics). A critical user decision changed the approach from the earlier research -- OTLP only, no Prometheus endpoint. This means zero fly.toml changes, zero new env vars for metrics export, and the dashboard queries PromQL against Grafana Cloud's Mimir backend where OTLP metrics are ingested.

The codebase already has 10 custom metric instruments (9 sync + 1 role transitions), otelhttp HTTP metrics, and Go runtime metrics via `runtime.Start()`. The only new Go code needed is registering observable Int64Gauges for per-type object counts using the same pattern as `InitFreshnessGauge()`. The Grafana dashboard is a hand-authored JSON file committed to `deploy/grafana/dashboards/pdbplus-overview.json`.

**Primary recommendation:** Author a single portable dashboard JSON using `__inputs` for datasource import, `$datasource` template variable of type "datasource", and PromQL queries targeting Grafana Cloud Mimir metric names (which include OTel-to-Prometheus suffixes by default).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** OTLP only -- no Prometheus endpoint needed. Metrics already flow to Grafana Cloud via existing OTLP export. No fly.toml `[metrics]` section or Prometheus env vars.
- **D-02:** OBS-01 scope is: verify OTLP metrics arrive in Grafana Cloud Prometheus (Mimir) backend correctly. Confirm the OTel-to-Prometheus name translation is as expected.
- **D-03:** Grafana Cloud Prometheus datasource -- queries use PromQL against Mimir backend where OTLP metrics land.
- **D-04:** Single dashboard with 5 collapsible rows: sync health, HTTP RED, per-type sync, Go runtime, business metrics. UID: `pdbplus-overview`, title: "PeeringDB Plus - Service Overview"
- **D-05:** Hand-author JSON directly -- no Grafana UI design step. Classic schema (pre-v12.2), `schemaVersion: 39`.
- **D-06:** Template variables: `$datasource` (datasource selector for portability), `$type` (PeeringDB type filter), `$region` (Fly.io region), `$interval` (rate interval via `$__rate_interval`).
- **D-07:** `$datasource` variable for portability -- no hardcoded datasource UIDs.
- **D-08:** Dashboard uses `__inputs` for importability. Set `id: null`, `version: null` for clean import per Pitfall #11.
- **D-09:** Each row has a documentation text panel explaining metrics and troubleshooting steps.
- **D-10:** Freshness stat panel with color thresholds: green < 3600s (1h), yellow < 7200s (2h), red >= 7200s (2h+).
- **D-11:** Dashboard JSON committed to `deploy/grafana/dashboards/pdbplus-overview.json` with provisioning YAML at `deploy/grafana/provisioning/dashboards.yaml`.
- **D-12:** Register new observable Int64Gauges for ALL 13 PeeringDB types, querying counts via ent (type-safe). Pattern matches existing `InitFreshnessGauge` with `Float64ObservableGauge` and callback.
- **D-13:** Ent queries: `client.Network.Query().Count(ctx)`, `client.Organization.Query().Count(ctx)`, etc. -- type-safe, consistent with codebase.
- **D-14:** New metric: `pdbplus.data.type.count` with `type` attribute (net, ix, fac, etc.) -- or 13 individual gauges. Claude's discretion on which pattern is cleaner.

### Claude's Discretion
- Whether to use a single `pdbplus.data.type.count` gauge with `type` attribute or 13 separate gauges
- Panel sizes and grid positions in the dashboard JSON
- Default time range and auto-refresh interval
- Whether to include a Fly.io region breakdown in the business metrics row or HTTP row

### Deferred Ideas (OUT OF SCOPE)
- Grafana alerting rules -- defer until production baselines established
- SLO/SLI tracking -- defer until 2-4 weeks of dashboard data
- Annotation markers for sync events -- requires OTel Collector or Tempo mapping
- Per-endpoint deep-dive dashboard -- add only if overview dashboard reveals problems
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| OBS-01 | Prometheus metrics export enabled via OTEL_METRICS_EXPORTER=prometheus env var and fly.toml [metrics] config | **SCOPE CHANGED by D-01:** No Prometheus endpoint. OBS-01 scope is verifying OTLP metrics arrive in Grafana Cloud Mimir correctly. Zero infrastructure changes needed -- autoexport already pushes via OTLP. |
| OBS-02 | Grafana dashboard JSON with sync health row | Dashboard authoring research complete. PromQL queries verified against OTel-to-Prometheus name mapping. |
| OBS-03 | Grafana dashboard JSON with HTTP RED metrics row | otelhttp metric names documented (OTel HTTP semantic conventions). PromQL patterns for rate/error/duration verified. |
| OBS-04 | Grafana dashboard JSON with per-type sync detail row | Per-type metrics use `type` attribute (org, net, ix, fac, etc.). 13 type names confirmed from `syncSteps()`. |
| OBS-05 | Grafana dashboard JSON with Go runtime row | OTel runtime instrumentation v0.67.0 metric names documented. Prometheus-translated names confirmed. |
| OBS-06 | Grafana dashboard JSON with business metrics row | New observable gauge needed: `pdbplus.data.type.count`. Ent client query pattern follows `InitFreshnessGauge`. |
| OBS-07 | Data freshness stat panel with color thresholds | `pdbplus_sync_freshness_seconds` gauge. Thresholds: green < 3600, yellow < 7200, red >= 7200. |
| OBS-08 | Documentation text panels in each dashboard row | Text panel JSON structure documented. One per row. |
| OBS-09 | Dashboard uses datasource template variables for portability | `__inputs` + `$datasource` variable pattern documented. No hardcoded UIDs. |
| OBS-10 | Dashboard provisioning YAML and JSON committed to deploy/grafana/ | File structure: `deploy/grafana/dashboards/pdbplus-overview.json` + `deploy/grafana/provisioning/dashboards.yaml`. |
</phase_requirements>

## Standard Stack

### Core (No New Dependencies)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `go.opentelemetry.io/otel` | v1.35+ | OTel API for metric instruments | Already in go.mod. Used for new observable gauge registration. |
| `go.opentelemetry.io/otel/metric` | v1.35+ | Metric instrument types (Int64ObservableGauge) | Already in go.mod. Provides observable gauge with callback. |
| `go.opentelemetry.io/contrib/exporters/autoexport` | v0.67.0 | OTLP export (already configured) | Already in go.mod. Reads OTEL_EXPORTER_OTLP_ENDPOINT from env. No changes needed. |
| `go.opentelemetry.io/contrib/instrumentation/runtime` | v0.67.0 | Go runtime metrics | Already in go.mod. Already initialized in `provider.go`. |
| `entgo.io/ent` | v0.14.5 | ORM for type-safe count queries | Already in go.mod. Used by new gauge callbacks for object counts. |

**Zero new Go module dependencies required.** This phase adds Go code for new observable gauges and creates JSON/YAML files for the dashboard.

### What NOT to Add

| Technology | Why Not |
|------------|---------|
| `go.opentelemetry.io/otel/exporters/prometheus` | D-01 says OTLP only -- no Prometheus endpoint |
| `prometheus/client_golang` | No direct Prometheus instrumentation |
| Grafana Foundation SDK | Overkill for 1 dashboard |
| Grafonnet/Jsonnet | Out of scope per REQUIREMENTS.md |

## Architecture Patterns

### Recommended Project Structure

```
deploy/
  grafana/
    dashboards/
      pdbplus-overview.json      # Single dashboard, 5 collapsible rows
    provisioning/
      dashboards.yaml            # Grafana file provisioning config

internal/otel/
  metrics.go                     # EXISTING: 10 instruments + InitFreshnessGauge
                                 # ADD: InitObjectCountGauges (new function)
```

### Pattern 1: Observable Gauge with Callback (Existing Pattern)

**What:** Register an observable gauge that computes its value on scrape/collect via a callback function.
**When to use:** For metrics that represent current state (object counts, freshness), not cumulative totals.
**Recommendation (Claude's discretion):** Use a single `pdbplus.data.type.count` gauge with a `type` attribute. This is cleaner than 13 separate gauges because:
- Consistent with `pdbplus.sync.type.*` pattern already using `type` attribute
- Single registration call with one callback iterating all types
- Grafana queries use `{type="net"}` label selector -- same pattern as sync metrics
- Less code, fewer metrics in the registry, easier to add new types

**Example (based on existing InitFreshnessGauge pattern):**
```go
// InitObjectCountGauges registers observable gauges for per-type object counts.
// The entClient is used to query row counts from the database.
func InitObjectCountGauges(entClient *ent.Client) error {
    meter := otel.Meter("peeringdb-plus")

    type typeCount struct {
        name    string
        countFn func(ctx context.Context) (int, error)
    }

    types := []typeCount{
        {"org", func(ctx context.Context) (int, error) { return entClient.Organization.Query().Count(ctx) }},
        {"campus", func(ctx context.Context) (int, error) { return entClient.Campus.Query().Count(ctx) }},
        {"fac", func(ctx context.Context) (int, error) { return entClient.Facility.Query().Count(ctx) }},
        {"carrier", func(ctx context.Context) (int, error) { return entClient.Carrier.Query().Count(ctx) }},
        {"carrierfac", func(ctx context.Context) (int, error) { return entClient.CarrierFacility.Query().Count(ctx) }},
        {"ix", func(ctx context.Context) (int, error) { return entClient.InternetExchange.Query().Count(ctx) }},
        {"ixlan", func(ctx context.Context) (int, error) { return entClient.IxLan.Query().Count(ctx) }},
        {"ixpfx", func(ctx context.Context) (int, error) { return entClient.IxPrefix.Query().Count(ctx) }},
        {"ixfac", func(ctx context.Context) (int, error) { return entClient.IxFacility.Query().Count(ctx) }},
        {"net", func(ctx context.Context) (int, error) { return entClient.Network.Query().Count(ctx) }},
        {"poc", func(ctx context.Context) (int, error) { return entClient.Poc.Query().Count(ctx) }},
        {"netfac", func(ctx context.Context) (int, error) { return entClient.NetworkFacility.Query().Count(ctx) }},
        {"netixlan", func(ctx context.Context) (int, error) { return entClient.NetworkIxLan.Query().Count(ctx) }},
    }

    _, err := meter.Int64ObservableGauge("pdbplus.data.type.count",
        metric.WithDescription("Number of objects stored per PeeringDB type"),
        metric.WithUnit("{object}"),
        metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
            ctx := context.Background()
            for _, tc := range types {
                count, err := tc.countFn(ctx)
                if err != nil {
                    continue // Skip on error, report what we can.
                }
                o.Observe(int64(count), metric.WithAttributes(
                    attribute.String("type", tc.name),
                ))
            }
            return nil
        }),
    )
    if err != nil {
        return fmt.Errorf("registering pdbplus.data.type.count gauge: %w", err)
    }
    return nil
}
```

### Pattern 2: Grafana Dashboard JSON with __inputs for Import Portability

**What:** Dashboard JSON with `__inputs` array at top level and `$datasource` template variable. Panels reference `"datasource": {"uid": "${datasource}"}`.
**When to use:** For any dashboard JSON committed to version control that must be importable into any Grafana instance.

**Dashboard JSON skeleton:**
```json
{
  "__inputs": [
    {
      "name": "DS_PROMETHEUS",
      "label": "Prometheus",
      "description": "Grafana Cloud Prometheus (Mimir) datasource",
      "type": "datasource",
      "pluginId": "prometheus",
      "pluginName": "Prometheus"
    }
  ],
  "__requires": [
    { "type": "grafana", "id": "grafana", "name": "Grafana", "version": "9.0.0" },
    { "type": "datasource", "id": "prometheus", "name": "Prometheus", "version": "1.0.0" },
    { "type": "panel", "id": "timeseries", "name": "Time series", "version": "" },
    { "type": "panel", "id": "stat", "name": "Stat", "version": "" },
    { "type": "panel", "id": "gauge", "name": "Gauge", "version": "" },
    { "type": "panel", "id": "table", "name": "Table", "version": "" },
    { "type": "panel", "id": "text", "name": "Text", "version": "" },
    { "type": "panel", "id": "bargauge", "name": "Bar gauge", "version": "" }
  ],
  "id": null,
  "uid": "pdbplus-overview",
  "title": "PeeringDB Plus - Service Overview",
  "version": null,
  "schemaVersion": 39,
  "editable": true,
  "graphTooltip": 1,
  "time": { "from": "now-6h", "to": "now" },
  "refresh": "30s",
  "timezone": "browser",
  "tags": ["peeringdb-plus", "auto-generated"],
  "templating": {
    "list": [
      {
        "name": "datasource",
        "type": "datasource",
        "query": "prometheus",
        "current": {},
        "hide": 0,
        "label": "Datasource"
      },
      {
        "name": "type",
        "type": "query",
        "datasource": { "uid": "${datasource}" },
        "query": "label_values(pdbplus_sync_type_objects_total, type)",
        "refresh": 2,
        "includeAll": true,
        "allValue": ".*",
        "multi": true,
        "current": {},
        "hide": 0,
        "label": "Type"
      },
      {
        "name": "region",
        "type": "query",
        "datasource": { "uid": "${datasource}" },
        "query": "label_values(pdbplus_sync_freshness_seconds, fly_region)",
        "refresh": 2,
        "includeAll": true,
        "allValue": ".*",
        "multi": true,
        "current": {},
        "hide": 0,
        "label": "Region"
      }
    ]
  },
  "panels": []
}
```

### Pattern 3: Collapsible Row with Child Panels

**What:** Row panel type with `collapsed: true` containing child panels.
**Structure:**
```json
{
  "type": "row",
  "title": "Sync Health",
  "collapsed": false,
  "gridPos": { "h": 1, "w": 24, "x": 0, "y": 0 },
  "panels": []
}
```

Child panels follow immediately after the row panel in the `panels` array with `y` values starting from the row's `y + 1`.

### Anti-Patterns to Avoid
- **Hardcoded datasource UIDs:** Use `"datasource": {"uid": "${datasource}"}` everywhere. Never use a specific UID.
- **Non-null id/version:** Set `"id": null` and `"version": null` to prevent import conflicts.
- **More than 20 panels:** Keep to 15-20 panels maximum. Use template variables for drill-down instead of duplicating panels per type.
- **Raw OTel metric names in PromQL:** Dots become underscores. Counters get `_total`. Histograms get `_seconds_bucket/sum/count`. Use Prometheus-translated names.

## OTLP-to-Prometheus Metric Name Translation

**Critical finding:** Grafana Cloud's OTLP endpoint applies OTel-to-Prometheus name translation **by default** (suffix generation is enabled). This means metric names in Grafana Cloud Mimir follow the same translation rules as a local Prometheus exporter.

### Translation Rules (Applied by Grafana Cloud Mimir)

1. **Dots to underscores:** `pdbplus.sync.duration` becomes `pdbplus_sync_duration`
2. **Unit suffix:** Unit `s` adds `_seconds`, unit `By` adds `_bytes`, unit `%` adds `_percent`. Custom units like `{operation}` add no suffix (brackets dropped).
3. **Counter suffix:** Monotonic sum metrics get `_total` suffix.
4. **Histogram suffixes:** Histograms create three metric families: `_bucket`, `_sum`, `_count`.
5. **Attribute dots to underscores:** `http.route` becomes `http_route`, `fly.region` becomes `fly_region`.

### Complete Metric Name Map (PromQL-ready)

| OTel Metric Name | OTel Type | OTel Unit | Prometheus/Mimir Name | Labels |
|---|---|---|---|---|
| `pdbplus.sync.duration` | Histogram | `s` | `pdbplus_sync_duration_seconds` (+bucket/sum/count) | `status` |
| `pdbplus.sync.operations` | Counter | `{operation}` | `pdbplus_sync_operations_total` | `status` |
| `pdbplus.sync.freshness` | Gauge | `s` | `pdbplus_sync_freshness_seconds` | (none) |
| `pdbplus.sync.type.duration` | Histogram | `s` | `pdbplus_sync_type_duration_seconds` (+bucket/sum/count) | `type` |
| `pdbplus.sync.type.objects` | Counter | `{object}` | `pdbplus_sync_type_objects_total` | `type` |
| `pdbplus.sync.type.deleted` | Counter | `{object}` | `pdbplus_sync_type_deleted_total` | `type` |
| `pdbplus.sync.type.fetch_errors` | Counter | `{error}` | `pdbplus_sync_type_fetch_errors_total` | `type` |
| `pdbplus.sync.type.upsert_errors` | Counter | `{error}` | `pdbplus_sync_type_upsert_errors_total` | `type` |
| `pdbplus.sync.type.fallback` | Counter | `{event}` | `pdbplus_sync_type_fallback_total` | `type` |
| `pdbplus.role.transitions` | Counter | `{event}` | `pdbplus_role_transitions_total` | (none) |
| `pdbplus.data.type.count` (NEW) | Gauge | `{object}` | `pdbplus_data_type_count` | `type` |
| `http.server.request.duration` | Histogram | `s` | `http_server_request_duration_seconds` (+bucket/sum/count) | `http_request_method`, `http_route`, `http_response_status_code` |
| `http.server.active_requests` | UpDownCounter | | `http_server_active_requests` | `http_request_method` |
| `http.client.request.duration` | Histogram | `s` | `http_client_request_duration_seconds` (+bucket/sum/count) | `http_request_method`, `server_address`, `http_response_status_code` |
| `go.goroutine.count` | UpDownCounter | `{goroutine}` | `go_goroutine_count` | (none) |
| `go.memory.used` | UpDownCounter | `By` | `go_memory_used_bytes` | |
| `go.memory.allocated` | Counter | `By` | `go_memory_allocated_bytes_total` | |
| `go.memory.allocations` | Counter | `{allocation}` | `go_memory_allocations_total` | |
| `go.memory.gc.goal` | UpDownCounter | `By` | `go_memory_gc_goal_bytes` | |
| `go.processor.limit` | UpDownCounter | `{thread}` | `go_processor_limit` | |
| `go.config.gogc` | UpDownCounter | `%` | `go_config_gogc_percent` | |
| `go.schedule.duration` | Histogram | `s` | `go_schedule_duration_seconds` (+bucket/sum/count) | |

**Confidence:** HIGH for custom metrics (we define the names and units). MEDIUM for Go runtime metrics (OTel runtime library has changed metric names between versions -- verify against actual Grafana Cloud metric explorer after deployment).

### Resource Attributes as Labels

Grafana Cloud automatically promotes certain OTel resource attributes to labels. The default promoted list includes:
- `service.name` -> `service_name` (also becomes `job` label)
- `service.version` -> `service_version`
- `service.instance.id` -> `instance`
- `deployment.environment.name` -> `deployment_environment_name`
- `cloud.region`, `cloud.availability_zone`
- Various `k8s.*` attributes

**Custom attributes NOT auto-promoted:** `fly.region`, `fly.machine_id`, `fly.app_name` are NOT in the default promoted list. They go to the `target_info` metric only.

**Impact on $region variable:** The `$region` template variable may not work as expected. Two options:
1. Contact Grafana Cloud support to add `fly.region` to the promoted list
2. Use a `target_info` join in PromQL: `pdbplus_sync_freshness_seconds * on(instance) group_left(fly_region) target_info{}`

**Recommendation:** Flag this as an open question. The dashboard should be authored with the `$region` variable, but the label_values query may need adjustment depending on whether `fly_region` appears directly on metrics or only in `target_info`. This can be verified after deployment.

## PromQL Query Reference

### Row 1: Sync Health

```promql
# Freshness (stat panel with thresholds)
pdbplus_sync_freshness_seconds

# Sync duration p95 (timeseries)
histogram_quantile(0.95, rate(pdbplus_sync_duration_seconds_bucket[$__rate_interval]))

# Sync success rate (stat panel, percentage)
sum(rate(pdbplus_sync_operations_total{status="success"}[$__rate_interval])) /
sum(rate(pdbplus_sync_operations_total[$__rate_interval]))

# Sync operations over time (timeseries, by status)
sum by(status)(rate(pdbplus_sync_operations_total[$__rate_interval]))

# Fallback events (stat panel, should be 0)
sum(increase(pdbplus_sync_type_fallback_total[$__rate_interval]))
```

### Row 2: HTTP RED Metrics

```promql
# Request rate by route (timeseries)
sum by(http_route)(rate(http_server_request_duration_seconds_count[$__rate_interval]))

# Error rate (stat panel, percentage)
sum(rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[$__rate_interval])) /
sum(rate(http_server_request_duration_seconds_count[$__rate_interval]))

# Latency p95 (timeseries)
histogram_quantile(0.95, sum by(le)(rate(http_server_request_duration_seconds_bucket[$__rate_interval])))

# Latency p99 by route (timeseries)
histogram_quantile(0.99, sum by(http_route, le)(rate(http_server_request_duration_seconds_bucket[$__rate_interval])))

# Active requests (timeseries)
http_server_active_requests
```

### Row 3: Per-Type Sync Detail

```promql
# Duration by type (timeseries, legend={{type}})
histogram_quantile(0.95, sum by(type, le)(rate(pdbplus_sync_type_duration_seconds_bucket{type=~"$type"}[$__rate_interval])))

# Objects synced per type (bar gauge or table)
sum by(type)(increase(pdbplus_sync_type_objects_total{type=~"$type"}[$__rate_interval]))

# Deletes per type (bar gauge)
sum by(type)(increase(pdbplus_sync_type_deleted_total{type=~"$type"}[$__rate_interval]))

# Fetch errors by type (timeseries)
sum by(type)(rate(pdbplus_sync_type_fetch_errors_total{type=~"$type"}[$__rate_interval]))

# Upsert errors by type (timeseries)
sum by(type)(rate(pdbplus_sync_type_upsert_errors_total{type=~"$type"}[$__rate_interval]))
```

### Row 4: Go Runtime

```promql
# Goroutines (timeseries)
go_goroutine_count

# Heap memory (timeseries, unit: bytes)
go_memory_used_bytes

# Allocation rate (timeseries)
rate(go_memory_allocated_bytes_total[$__rate_interval])

# GC goal (timeseries, unit: bytes)
go_memory_gc_goal_bytes
```

### Row 5: Business Metrics

```promql
# Object counts per type (bar gauge or table, from new gauge)
pdbplus_data_type_count{type=~"$type"}

# Total objects (stat panel)
sum(pdbplus_data_type_count)

# Object counts over time (timeseries, by type)
pdbplus_data_type_count{type=~"$type"}
```

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Prometheus endpoint | Custom HTTP handler serving metrics | autoexport + OTEL_METRICS_EXPORTER env var | Already configured for OTLP; Prometheus exporter also built into autoexport if ever needed |
| Dashboard generation | Go code to produce JSON | Hand-author JSON directly | One dashboard, static structure, easier to review |
| Grafana SDK | grafana-foundation-sdk or grabana | Raw JSON | Public preview / abandoned respectively; JSON is portable |
| Metric name mapping | Manual string translation | Let Grafana Cloud Mimir handle OTel-to-Prometheus translation | Built into the OTLP ingestion pipeline |

## Common Pitfalls

### Pitfall 1: OTel Metric Names vs Prometheus Names in PromQL Queries

**What goes wrong:** Dashboard author writes PromQL using OTel names (`pdbplus.sync.duration`) instead of translated Prometheus names (`pdbplus_sync_duration_seconds`). Queries return empty results.
**Why it happens:** Code defines metrics with OTel names (dots, no suffixes). Prometheus uses underscores + type/unit suffixes.
**How to avoid:** Use the complete metric name map in this document. Every PromQL query MUST use the Prometheus-translated name.
**Warning signs:** All panels show "No data" despite metrics being emitted.

### Pitfall 2: Hardcoded Datasource UIDs Break Import

**What goes wrong:** Panel datasource references contain a specific Grafana instance UID. Import into another instance shows "No data" or "Datasource not found".
**Why it happens:** Each Grafana instance assigns unique datasource UIDs.
**How to avoid:** Use `"datasource": {"uid": "${datasource}"}` in every panel and target definition. Include `__inputs` section for import-time datasource selection.
**Warning signs:** Panels show "Datasource not found" after import.

### Pitfall 3: Non-null id/version Cause Import Conflicts

**What goes wrong:** Dashboard import creates duplicate instead of updating, or rejects with version conflict.
**Why it happens:** `id` is instance-specific (auto-assigned). `version` is auto-incremented by Grafana.
**How to avoid:** Set `"id": null` and `"version": null`. Keep `"uid": "pdbplus-overview"` stable for idempotent updates.
**Warning signs:** Multiple copies of dashboard appear after re-import.

### Pitfall 4: fly_region Label Not Available on Metrics

**What goes wrong:** `$region` template variable returns empty because `fly_region` is not a label on any metric.
**Why it happens:** Grafana Cloud only auto-promotes a default set of resource attributes to metric labels. Custom attributes like `fly.region` go to `target_info` metric only.
**How to avoid:** Two options: (a) contact Grafana Cloud support to add `fly.region` to promoted list, or (b) use `target_info` join in PromQL. Author dashboard with fallback approach.
**Warning signs:** `label_values(pdbplus_sync_freshness_seconds, fly_region)` returns empty.

### Pitfall 5: Panel Sprawl Kills Dashboard Load Time

**What goes wrong:** Dashboard has 50+ panels, each firing a PromQL query. Grafana Cloud rate-limits or times out.
**Why it happens:** Creating a panel for every metric x dimension combination.
**How to avoid:** Limit to 15-20 panels across 5 rows. Use `$type` variable for drill-down instead of 13 separate per-type panels.
**Warning signs:** Dashboard takes > 5s to load. Panels show "Query timeout" errors.

### Pitfall 6: Observable Gauge Callback Database Queries Under Load

**What goes wrong:** The object count gauge callback runs 13 COUNT queries against SQLite on every metrics collection cycle (every 15-60s depending on scrape interval). Under high read load, this adds latency to the metrics collection.
**Why it happens:** Observable gauge callbacks are invoked synchronously during metric collection.
**How to avoid:** These are simple `SELECT COUNT(*) FROM table` queries on SQLite, which are fast even under load (typically < 1ms each). The callback runs at most once per OTLP push interval (typically 30s). 13 count queries at < 1ms each = < 13ms total. This is acceptable. If performance becomes an issue, cache counts with a TTL.
**Warning signs:** Metric export latency increases. OTel SDK logs "callback exceeded deadline" warnings.

## Code Examples

### New Function: InitObjectCountGauges

Add to `internal/otel/metrics.go`:

```go
// InitObjectCountGauges registers observable gauges for per-type object counts.
// Each type emits an observation with the "type" attribute matching the sync
// step names (org, campus, fac, carrier, carrierfac, ix, ixlan, ixpfx, ixfac,
// net, poc, netfac, netixlan).
func InitObjectCountGauges(entClient *ent.Client) error {
    meter := otel.Meter("peeringdb-plus")

    type typeCounter struct {
        name    string
        countFn func(ctx context.Context) (int, error)
    }

    types := []typeCounter{
        {"org", func(ctx context.Context) (int, error) { return entClient.Organization.Query().Count(ctx) }},
        {"campus", func(ctx context.Context) (int, error) { return entClient.Campus.Query().Count(ctx) }},
        {"fac", func(ctx context.Context) (int, error) { return entClient.Facility.Query().Count(ctx) }},
        {"carrier", func(ctx context.Context) (int, error) { return entClient.Carrier.Query().Count(ctx) }},
        {"carrierfac", func(ctx context.Context) (int, error) { return entClient.CarrierFacility.Query().Count(ctx) }},
        {"ix", func(ctx context.Context) (int, error) { return entClient.InternetExchange.Query().Count(ctx) }},
        {"ixlan", func(ctx context.Context) (int, error) { return entClient.IxLan.Query().Count(ctx) }},
        {"ixpfx", func(ctx context.Context) (int, error) { return entClient.IxPrefix.Query().Count(ctx) }},
        {"ixfac", func(ctx context.Context) (int, error) { return entClient.IxFacility.Query().Count(ctx) }},
        {"net", func(ctx context.Context) (int, error) { return entClient.Network.Query().Count(ctx) }},
        {"poc", func(ctx context.Context) (int, error) { return entClient.Poc.Query().Count(ctx) }},
        {"netfac", func(ctx context.Context) (int, error) { return entClient.NetworkFacility.Query().Count(ctx) }},
        {"netixlan", func(ctx context.Context) (int, error) { return entClient.NetworkIxLan.Query().Count(ctx) }},
    }

    _, err := meter.Int64ObservableGauge("pdbplus.data.type.count",
        metric.WithDescription("Number of objects stored per PeeringDB type"),
        metric.WithUnit("{object}"),
        metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
            ctx := context.Background()
            for _, tc := range types {
                count, err := tc.countFn(ctx)
                if err != nil {
                    continue // Skip on error, report what we can.
                }
                o.Observe(int64(count), metric.WithAttributes(
                    attribute.String("type", tc.name),
                ))
            }
            return nil
        }),
    )
    if err != nil {
        return fmt.Errorf("registering pdbplus.data.type.count gauge: %w", err)
    }
    return nil
}
```

**Wiring in main.go** (after entClient and db init, near InitFreshnessGauge call):
```go
if err := pdbotel.InitObjectCountGauges(entClient); err != nil {
    logger.Error("failed to init object count gauges", slog.String("error", err.Error()))
    os.Exit(1)
}
```

### Grafana Provisioning YAML

File: `deploy/grafana/provisioning/dashboards.yaml`

```yaml
apiVersion: 1

providers:
  - name: peeringdb-plus
    orgId: 1
    folder: PeeringDB Plus
    type: file
    disableDeletion: false
    editable: true
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /var/lib/grafana/dashboards
      foldersFromFilesStructure: false
```

### Test Pattern for Observable Gauge

Follow existing `TestInitFreshnessGauge_RecordsValue` pattern:

```go
func TestInitObjectCountGauges_RecordsValues(t *testing.T) {
    reader := sdkmetric.NewManualReader()
    mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
    otel.SetMeterProvider(mp)
    t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

    // Need an entClient with test data.
    // Use enttest.Open to create an in-memory SQLite database.
    entClient := enttest.Open(t, "sqlite3", "file:ent?mode=memory&_fk=1")
    defer entClient.Close()

    // Create some test data.
    entClient.Organization.Create().SetName("Test Org").SaveX(context.Background())

    if err := InitObjectCountGauges(entClient); err != nil {
        t.Fatalf("InitObjectCountGauges: %v", err)
    }

    var rm metricdata.ResourceMetrics
    if err := reader.Collect(context.Background(), &rm); err != nil {
        t.Fatalf("Collect: %v", err)
    }

    found := findMetric(rm, "pdbplus.data.type.count")
    if found == nil {
        t.Fatal("expected pdbplus.data.type.count metric, not found")
    }
}
```

## Dashboard Panel Layout (Recommended)

### Row 1: Sync Health (y=0)
| Panel | Type | Width | Description |
|-------|------|-------|-------------|
| Doc text | text | 24 | Explains sync metrics and troubleshooting |
| Data Freshness | stat | 4 | `pdbplus_sync_freshness_seconds` with color thresholds |
| Sync Success Rate | stat | 4 | Percentage of successful syncs |
| Sync Duration | timeseries | 8 | p50/p95/p99 over time |
| Sync Operations | timeseries | 8 | Success vs failed rate |

### Row 2: HTTP RED Metrics (y=10)
| Panel | Type | Width | Description |
|-------|------|-------|-------------|
| Doc text | text | 24 | Explains HTTP metrics and troubleshooting |
| Request Rate | timeseries | 8 | By route |
| Error Rate | stat | 4 | 5xx percentage |
| Latency p95/p99 | timeseries | 8 | By route |
| Active Requests | stat | 4 | Current gauge |

### Row 3: Per-Type Sync Detail (y=20)
| Panel | Type | Width | Description |
|-------|------|-------|-------------|
| Doc text | text | 24 | Explains per-type metrics |
| Duration by Type | timeseries | 12 | p95 by type, filtered by $type |
| Objects Synced | bargauge | 6 | Per type |
| Errors by Type | timeseries | 6 | Fetch + upsert errors |

### Row 4: Go Runtime (y=30)
| Panel | Type | Width | Description |
|-------|------|-------|-------------|
| Doc text | text | 24 | Explains runtime metrics |
| Goroutines | timeseries | 6 | `go_goroutine_count` |
| Heap Memory | timeseries | 6 | `go_memory_used_bytes` |
| Allocation Rate | timeseries | 6 | `rate(go_memory_allocated_bytes_total)` |
| GC Goal | timeseries | 6 | `go_memory_gc_goal_bytes` |

### Row 5: Business Metrics (y=40)
| Panel | Type | Width | Description |
|-------|------|-------|-------------|
| Doc text | text | 24 | Explains business metrics |
| Total Objects | stat | 4 | `sum(pdbplus_data_type_count)` |
| Objects by Type | bargauge | 12 | `pdbplus_data_type_count` per type |
| Object Counts Table | table | 8 | Detailed per-type breakdown |

**Total panels: 25** (including 5 text panels). Within the recommended < 25-30 limit. Text panels are lightweight (no queries).

## PeeringDB Type Names (for $type variable)

These are the `type` attribute values used in sync metrics and the new object count gauge:

| Type Name | Ent Client | Description |
|-----------|------------|-------------|
| `org` | Organization | Organizations |
| `campus` | Campus | Campuses |
| `fac` | Facility | Facilities |
| `carrier` | Carrier | Carriers |
| `carrierfac` | CarrierFacility | Carrier-Facility links |
| `ix` | InternetExchange | Internet Exchanges |
| `ixlan` | IxLan | IX LANs |
| `ixpfx` | IxPrefix | IX Prefixes |
| `ixfac` | IxFacility | IX-Facility links |
| `net` | Network | Networks |
| `poc` | Poc | Points of Contact |
| `netfac` | NetworkFacility | Network-Facility links |
| `netixlan` | NetworkIxLan | Network-IX LAN links |

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Prometheus endpoint via fly.toml `[metrics]` | OTLP direct to Grafana Cloud | D-01 (this phase) | No fly.toml changes needed. Simpler infrastructure. |
| Fly.io managed Grafana at fly-metrics.net | Grafana Cloud with OTLP | D-01 (this phase) | Dashboard uses Grafana Cloud Prometheus datasource, not Fly.io VictoriaMetrics. |
| Design in Grafana UI, export JSON | Hand-author JSON directly | D-05 (this phase) | No dependency on running Grafana instance for authoring. |
| `process_runtime_go_goroutines` (old OTel naming) | `go_goroutine_count` (semconv naming) | OTel runtime instrumentation v0.56+ | Dashboard queries use new names from current semantic conventions. |

## Open Questions

1. **fly_region label availability on metrics**
   - What we know: Grafana Cloud's default promoted attributes list does NOT include `fly.region`. Custom resource attributes go to `target_info` only.
   - What's unclear: Whether the user has already contacted Grafana Cloud support to promote `fly.region`, or if a `target_info` join is needed.
   - Recommendation: Author the `$region` variable but use a fallback approach. If `label_values()` returns empty, document the `target_info` join pattern. This is a verification item, not a blocker.

2. **Exact Go runtime metric names in Grafana Cloud**
   - What we know: OTel semantic conventions define names like `go.goroutine.count`, `go.memory.used` (unit: By). Prometheus translation should produce `go_goroutine_count`, `go_memory_used_bytes`.
   - What's unclear: The OTel Go runtime instrumentation package has changed metric names between versions (e.g., older versions used `process.runtime.go.*` prefix). The FEATURES.md research uses older names.
   - Recommendation: Use the semantic convention names documented here (v0.67.0 aligns with semconv). Verify against actual Grafana Cloud metric explorer after first deployment. Flag as LOW confidence until verified.

3. **ent import cycle in metrics.go**
   - What we know: `internal/otel/metrics.go` currently has no dependency on `ent`. Adding `InitObjectCountGauges` that takes `*ent.Client` would create a dependency from `otel` -> `ent`.
   - What's unclear: Whether this creates an import cycle (if `ent` or its generated code imports `internal/otel`).
   - Recommendation: Check import graph. If cycle exists, accept the `*ent.Client` parameter in `metrics.go` or move the function to a new file (e.g., `internal/otel/gauges.go`). The ent generated code does NOT import `internal/otel`, so this should be safe.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` |
| Config file | none (Go convention) |
| Quick run command | `go test ./internal/otel/ -race -run TestInit` |
| Full suite command | `go test -race ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| OBS-01 | OTLP metrics verified in Grafana Cloud | manual-only | N/A -- requires live Grafana Cloud | N/A |
| OBS-02 | Sync health row panels exist | unit | `go test ./deploy/... -run TestDashboardJSON` | Wave 0 |
| OBS-03 | HTTP RED row panels exist | unit | (same as OBS-02) | Wave 0 |
| OBS-04 | Per-type sync row panels exist | unit | (same as OBS-02) | Wave 0 |
| OBS-05 | Go runtime row panels exist | unit | (same as OBS-02) | Wave 0 |
| OBS-06 | Business metrics row + new gauge | unit | `go test ./internal/otel/ -race -run TestInitObjectCountGauges` | Wave 0 |
| OBS-07 | Freshness stat with thresholds | unit | (JSON validation in OBS-02 test) | Wave 0 |
| OBS-08 | Doc text panels in each row | unit | (JSON validation in OBS-02 test) | Wave 0 |
| OBS-09 | Datasource template variable | unit | (JSON validation in OBS-02 test) | Wave 0 |
| OBS-10 | Files committed to deploy/grafana/ | unit | (file existence check in OBS-02 test) | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/otel/ -race`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/otel/metrics_test.go` -- add `TestInitObjectCountGauges_NoError` and `TestInitObjectCountGauges_RecordsValues`
- [ ] Dashboard JSON validation test (optional) -- could validate JSON structure, panel count, datasource refs
- [ ] No new framework install needed

## Environment Availability

Step 2.6: SKIPPED (no external dependencies identified). This phase creates Go code and JSON files only. The Grafana Cloud endpoint and OTLP export are already configured in production.

## Project Constraints (from CLAUDE.md)

| Directive | Impact on Phase |
|-----------|----------------|
| CS-0: Modern Go | Use Go 1.26 features. |
| CS-2: No stutter | New function in `otel` package: `InitObjectCountGauges` not `OtelInitObjectCountGauges`. |
| CS-5: Input structs for >2 args | `InitObjectCountGauges` takes 1 arg (`*ent.Client`), no struct needed. |
| ERR-1: Wrap with %w | All errors wrapped with context. |
| T-1: Table-driven tests | Observable gauge test should verify multiple type observations. |
| T-2: -race in CI | All tests must pass with `-race`. |
| OBS-1: Structured slog | Logging in gauge callback should use slog if needed. |
| API-1: Document exported items | New exported function needs godoc comment. |
| MD-1: Prefer stdlib | No new deps -- using existing OTel SDK + ent. |

## Sources

### Primary (HIGH confidence)
- `internal/otel/metrics.go` -- existing 10 metric instruments, InitFreshnessGauge pattern
- `internal/otel/provider.go` -- autoexport setup, runtime.Start(), resource attributes
- `internal/sync/worker.go` lines 77-94 -- syncSteps() with 13 type names
- `ent/client.go` -- 13 entity client types (Organization, Campus, Facility, etc.)
- `fly.toml` -- current deployment config (no [metrics] section, confirming OTLP-only)
- [OTel Prometheus Compatibility Spec](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/) -- naming translation rules
- [OTel Go Runtime Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/runtime/go-metrics/) -- metric names for go.goroutine.count, go.memory.*, etc.
- [OTel HTTP Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/http/http-metrics/) -- otelhttp metric names
- [Grafana Cloud OTLP format considerations](https://grafana.com/docs/grafana-cloud/send-data/otlp/otlp-format-considerations/) -- suffix generation enabled by default, attribute translation rules
- [Grafana Dashboard JSON Model](https://grafana.com/docs/grafana/latest/dashboards/json-model/) -- JSON structure reference
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/dashboards/build-dashboards/best-practices/) -- panel count limits

### Secondary (MEDIUM confidence)
- [Grafana Cloud promoted resource attributes](https://grafana.com/blog/2025/05/20/opentelemetry-with-prometheus-better-integration-through-resource-attribute-promotion/) -- default promoted list (fly.region NOT included)
- [Grafana Community: datasource UID in provisioned dashboards](https://community.grafana.com/t/should-provisioned-dashboards-have-datasource-uids/65463) -- __inputs pattern
- [OTel Go runtime instrumentation pkg.go.dev](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/runtime) -- v0.67.0 metric names
- [Mimir OTLP otel_metric_suffixes_enabled](https://github.com/grafana/mimir/discussions/10492) -- default behavior in Grafana Cloud

### Tertiary (LOW confidence)
- Go runtime metric Prometheus names -- need verification against actual Grafana Cloud metric explorer after deployment
- `$region` variable behavior -- depends on whether fly_region is promoted or requires target_info join

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- zero new dependencies, existing patterns
- Architecture (Go code): HIGH -- follows established InitFreshnessGauge pattern
- Architecture (Dashboard JSON): HIGH -- well-documented Grafana JSON model
- OTel-to-Prometheus naming: HIGH for custom metrics, MEDIUM for Go runtime metrics
- Resource attribute promotion (fly_region): LOW -- needs verification
- Pitfalls: HIGH -- well-documented in earlier research + verified with official docs

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (stable domain, naming specs change slowly)
