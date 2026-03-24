# Technology Stack: v1.5 Tech Debt & Observability

**Project:** PeeringDB Plus v1.5
**Researched:** 2026-03-24
**Scope:** Stack additions for Grafana dashboard provisioning, Prometheus metrics exposure for Fly.io scraping, meta.generated field verification, and tech debt cleanup. Does NOT re-research validated backend stack.

---

## New Dependencies

**Zero new Go module dependencies required for v1.5.**

Every capability needed is already present in the dependency tree or achievable through configuration changes alone.

---

## Prometheus Metrics Exposure (for Fly.io Grafana)

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| `OTEL_METRICS_EXPORTER=prometheus` (env var) | N/A | Expose `/metrics` endpoint for Fly.io scraping | **Zero new dependencies.** The autoexport package (`v0.67.0`, already in go.mod) natively supports `prometheus` as a metrics exporter value. Setting this env var causes autoexport to start an HTTP server serving Prometheus-format metrics. The Prometheus exporter (`go.opentelemetry.io/otel/exporters/prometheus v0.64.0`) is already an indirect dependency via autoexport. All existing OTel metrics (sync duration/operations/freshness, per-type counters, otelhttp HTTP metrics, Go runtime metrics) automatically appear in Prometheus format. | HIGH |
| `OTEL_EXPORTER_PROMETHEUS_PORT` (env var) | N/A | Configure Prometheus exporter listen port | Defaults to 9464. Fly.io scrapes custom metrics from a configured port/path. Must match `[metrics]` section in fly.toml. Recommend 9091 to avoid conflicts with app (8081) and LiteFS proxy (8080). | HIGH |

**Critical integration detail:** The autoexport Prometheus exporter starts its own HTTP server on a separate port, independent of the main application HTTP server. Fly.io's metrics scraper hits this endpoint every 15 seconds.

### OTel Metric Name to Prometheus Name Mapping

The OTel-to-Prometheus specification converts names as follows: dots become underscores, counters get `_total` suffix, histograms get `_bucket`/`_sum`/`_count` suffixes, and unit `s` becomes `_seconds`.

| OTel Metric | Prometheus Name | Type | Labels |
|------------|----------------|------|--------|
| `pdbplus.sync.duration` | `pdbplus_sync_duration_seconds_bucket/sum/count` | Histogram | `status` |
| `pdbplus.sync.operations` | `pdbplus_sync_operations_total` | Counter | `status` |
| `pdbplus.sync.type.duration` | `pdbplus_sync_type_duration_seconds_bucket/sum/count` | Histogram | `type` |
| `pdbplus.sync.type.objects` | `pdbplus_sync_type_objects_total` | Counter | `type` |
| `pdbplus.sync.type.deleted` | `pdbplus_sync_type_deleted_total` | Counter | `type` |
| `pdbplus.sync.type.fetch_errors` | `pdbplus_sync_type_fetch_errors_total` | Counter | `type` |
| `pdbplus.sync.type.upsert_errors` | `pdbplus_sync_type_upsert_errors_total` | Counter | `type` |
| `pdbplus.sync.type.fallback` | `pdbplus_sync_type_fallback_total` | Counter | `type` |
| `pdbplus.sync.freshness` | `pdbplus_sync_freshness_seconds` | Gauge | (none) |
| (otelhttp) `http.server.request.duration` | `http_server_request_duration_seconds_bucket/sum/count` | Histogram | `http_route`, `http_request_method`, `http_response_status_code` |
| (runtime) `process.runtime.go.*` | `process_runtime_go_*` | Various | (varies) |

### fly.toml Configuration Addition

```toml
# Custom metrics endpoint for OTel Prometheus exporter
[metrics]
port = 9091
path = "/metrics"
```

### Environment Variable Additions

```bash
# Enable Prometheus metrics exporter (replaces OTLP metric push)
OTEL_METRICS_EXPORTER=prometheus
# Port for Prometheus metrics HTTP server (must match fly.toml [metrics] port)
OTEL_EXPORTER_PROMETHEUS_PORT=9091
```

**Important:** Setting `OTEL_METRICS_EXPORTER=prometheus` replaces OTLP metric export. If both Prometheus scraping AND OTLP push are needed (e.g., Grafana Cloud alongside Fly.io managed Grafana), use comma-separated: `OTEL_METRICS_EXPORTER=otlp,prometheus`. For Fly.io managed Grafana which reads from its own Prometheus, `prometheus` alone is sufficient.

---

## Grafana Dashboard Delivery

| Technology | Version | Purpose | Why | Confidence |
|------------|---------|---------|-----|------------|
| Hand-written JSON files | N/A | Grafana dashboard definitions | **Do NOT use the Grafana Foundation SDK.** The SDK is "public preview" (may break), adds a significant dependency tree, and is designed for teams managing hundreds of dashboards via CI/CD. This project needs 1-3 static dashboards. Design in Fly.io's managed Grafana UI at fly-metrics.net, export the JSON model, check it into the repo. JSON is directly importable into any Grafana instance. | HIGH |

**Dashboard delivery workflow:**
1. Open `fly-metrics.net`, switch to the peeringdb-plus org
2. Create dashboard panels using the Grafana UI with PromQL queries against the preconfigured Prometheus datasource
3. Once satisfied, go to Dashboard Settings > JSON Model > Copy
4. Save to `grafana/dashboards/peeringdb-plus.json` in the repo
5. To update: re-import, edit in UI, re-export, commit

**Dashboard JSON schema version:** Use classic JSON model (not v2 Resource schema). Fly.io runs Grafana v10.4 which predates the v2 schema (introduced in Grafana 12.2.0). The classic format is universally compatible across Grafana 9+.

### PromQL Query Patterns for Dashboard Panels

Fly.io's managed Grafana uses a Prometheus datasource backed by VictoriaMetrics. VictoriaMetrics uses MetricsQL, which is backwards-compatible with PromQL and adds useful extensions.

**Sync Health Row:**

```promql
# Sync duration p95 (timeseries panel)
histogram_quantile(0.95, rate(pdbplus_sync_duration_seconds_bucket[1h]))

# Sync success rate (stat panel, percentage)
sum(rate(pdbplus_sync_operations_total{status="success"}[1h])) /
sum(rate(pdbplus_sync_operations_total[1h]))

# Sync freshness (gauge panel, threshold at 7200s = 2h)
pdbplus_sync_freshness_seconds

# Per-type sync duration p95 (timeseries panel, legend={{type}})
histogram_quantile(0.95, sum by(type, le)(rate(pdbplus_sync_type_duration_seconds_bucket[1h])))

# Per-type object counts (bar gauge or table, latest value)
sum by(type)(increase(pdbplus_sync_type_objects_total[1h]))

# Per-type error rates (timeseries panel)
sum by(type)(rate(pdbplus_sync_type_fetch_errors_total[5m]))
sum by(type)(rate(pdbplus_sync_type_upsert_errors_total[5m]))
```

**API Traffic Row:**

```promql
# Request rate by route (timeseries panel, legend={{http_route}})
sum by(http_route)(rate(http_server_request_duration_seconds_count[5m]))

# Request latency p99 by route (timeseries panel)
histogram_quantile(0.99, sum by(http_route, le)(rate(http_server_request_duration_seconds_bucket[5m])))

# Error rate (stat panel, percentage, threshold at 0.01 = 1%)
sum(rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[5m])) /
sum(rate(http_server_request_duration_seconds_count[5m]))

# Request rate by status code (stacked timeseries)
sum by(http_response_status_code)(rate(http_server_request_duration_seconds_count[5m]))
```

**Infrastructure Row:**

```promql
# Goroutine count (timeseries panel)
process_runtime_go_goroutines

# Heap memory usage (timeseries panel, unit: bytes)
process_runtime_go_mem_heap_alloc_bytes

# GC pause duration (timeseries panel)
rate(process_runtime_go_gc_pause_ns_sum[5m]) / rate(process_runtime_go_gc_pause_ns_count[5m])
```

**Business Metrics Row:**

```promql
# Total objects by type (table panel)
sum by(type)(increase(pdbplus_sync_type_objects_total[24h]))

# Deleted objects by type (table panel)
sum by(type)(increase(pdbplus_sync_type_deleted_total[24h]))

# Sync fallback events (stat panel, should be 0)
sum(increase(pdbplus_sync_type_fallback_total[24h]))
```

---

## Fly.io Managed Grafana Details

| Property | Value |
|----------|-------|
| URL | `fly-metrics.net` |
| Grafana version | v10.4 (upgraded March 2024; may be newer) |
| Prometheus backend | VictoriaMetrics (MetricsQL, PromQL-compatible) |
| Query endpoint | `https://api.fly.io/prometheus/<org-slug>/` |
| Scrape interval | 15 seconds |
| Built-in dashboards | 3 (proxy metrics, instance metrics, volume metrics) |
| Custom metrics | Supported via `[metrics]` in fly.toml |
| Dashboard import | Manual via Grafana UI (Import JSON) |
| Auth | Fly.io org-scoped (auto-provisioned) |

---

## meta.generated Field Verification

No new dependencies needed. Uses existing stack:

| Existing Technology | How Used |
|---------------------|----------|
| `internal/peeringdb` client | HTTP client with API key, retry, OTel tracing |
| `encoding/json` (stdlib) | Parse response, check for `meta.generated` |
| `testing` (stdlib) | Implement as integration test |

Implementation approach: extend the existing `internal/conformance/live_test.go` pattern or the `cmd/pdbcompat-check` tool to make a depth=0 paginated request and inspect the response for `meta.generated` field presence.

---

## Tech Debt Cleanup

No new dependencies. Pure code deletion:

| Cleanup Item | Approach | New Dependencies |
|-------------|----------|-----------------|
| Remove unused DataLoader middleware | Delete dead code from `internal/` | None |
| Remove WorkerConfig.IsPrimary dead field | Delete field and references | None |
| 26 human verification items | Manual testing against live Fly.io deployment | None |

---

## What NOT to Add

| Technology | Why Not |
|------------|---------|
| Grafana Foundation SDK (`github.com/grafana/grafana-foundation-sdk`) | "Public preview" status. Large dependency for 1-3 static dashboards. SDK targets teams managing hundreds of dashboards programmatically. Hand-written JSON exported from Grafana UI is simpler, more portable, zero dependency. |
| `grafana-tools/sdk` | Older community library. Same reasoning -- unnecessary abstraction for static dashboards. |
| `K-Phoen/grabana` | Abandoned. Author recommends Grafana Foundation SDK. |
| `prometheus/client_golang` (as direct dep) | Already indirect via OTel Prometheus exporter. Do not create Prometheus metrics directly -- use OTel instruments and let the exporter translate. |
| Grafana Alloy / OTel Collector sidecar | Unnecessary. The OTel Prometheus exporter serves metrics directly to Fly.io's scraper. No collector needed. |
| Dashboard JSON generation code (Go) | Do not generate dashboard JSON programmatically. Design in Grafana UI, export, version control. The JSON model is the source of truth, not Go code. |
| Any new `go get` additions | v1.5 requires zero new Go module dependencies. |

---

## Version Compatibility Matrix

| Component | Current Version | Change Needed | Notes |
|-----------|----------------|--------------|-------|
| `go.opentelemetry.io/contrib/exporters/autoexport` | v0.67.0 | None | Already supports `prometheus` exporter |
| `go.opentelemetry.io/otel/exporters/prometheus` | v0.64.0 (indirect) | None | Already present as transitive dep |
| `prometheus/client_golang` | v1.23.2 (indirect) | None | Already present as transitive dep |
| `go.opentelemetry.io/contrib/instrumentation/runtime` | v0.67.0 | None | Already emitting Go runtime metrics |
| `go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` | v0.67.0 | None | Already emitting HTTP metrics |
| fly.toml | current | Add `[metrics]` section | Minimal config change |
| Fly.io env vars | current | Add 2 env vars | OTEL_METRICS_EXPORTER, OTEL_EXPORTER_PROMETHEUS_PORT |
| Go | 1.26.1 | None | |

---

## Risk Register (v1.5 Specific)

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| Fly.io Grafana version too old for dashboard features | LOW | LOW | Use classic JSON model (Grafana 9+ compatible). Avoid v2 schema. Stick to timeseries, stat, gauge, table, bar gauge panels (all built-in). |
| `OTEL_METRICS_EXPORTER=prometheus` disables OTLP metrics push | MEDIUM | HIGH | This IS the expected behavior. Use comma-separated `otlp,prometheus` if OTLP push is also needed. For Fly.io managed Grafana, `prometheus` alone is correct. |
| Prometheus metric names drift between OTel SDK versions | LOW | LOW | Mapping is defined by the OTel specification. Names are stable. Versions are pinned. |
| Dashboard JSON breaks on Grafana upgrade | LOW | LOW | Classic JSON model is stable across versions. Avoid panel plugins not bundled with Grafana. |
| meta.generated field behavior changes in PeeringDB API | MEDIUM | LOW | This is why we verify. Build graceful fallback regardless of result. |
| Fly.io metrics scraping misses some metrics | LOW | MEDIUM | Scrape interval is 15s. OTel Prometheus exporter holds all metrics in memory. Verify in managed Grafana that all expected metrics appear after deployment. |

---

## Sources

- [Fly.io Metrics Documentation](https://fly.io/docs/monitoring/metrics/) -- Custom metrics setup, managed Grafana, Prometheus access
- [Fly.io Grafana v10.4 Upgrade](https://community.fly.io/t/fly-metrics-grafana-upgraded-to-v10-4/18823) -- Managed Grafana version
- [OTel Autoexport Package](https://pkg.go.dev/go.opentelemetry.io/contrib/exporters/autoexport) -- OTEL_METRICS_EXPORTER=prometheus support
- [OTel Prometheus Exporter](https://pkg.go.dev/go.opentelemetry.io/otel/exporters/prometheus) -- Metric naming, configuration
- [OTel-Prometheus Compatibility Spec](https://opentelemetry.io/docs/specs/otel/compatibility/prometheus_and_openmetrics/) -- Metric name translation rules
- [Grafana Dashboard JSON Model](https://grafana.com/docs/grafana/latest/visualizations/dashboards/build-dashboards/view-dashboard-json-model/) -- Classic JSON format
- [Grafana Foundation SDK](https://grafana.github.io/grafana-foundation-sdk/) -- Evaluated and rejected for this project
- [Grafana Foundation SDK GitHub](https://github.com/grafana/grafana-foundation-sdk) -- "Public preview" status confirmed
- [Grafana Dashboard Schema v2](https://grafana.com/docs/grafana/latest/as-code/observability-as-code/schema-v2/) -- Requires Grafana 12.2+, not available on Fly.io
- [Prometheus Histograms in Grafana](https://grafana.com/blog/2020/06/23/how-to-visualize-prometheus-histograms-in-grafana/) -- PromQL histogram_quantile patterns
