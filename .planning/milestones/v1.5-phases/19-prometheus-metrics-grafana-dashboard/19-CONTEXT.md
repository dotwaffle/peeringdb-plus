# Phase 19: Prometheus Metrics & Grafana Dashboard - Context

**Gathered:** 2026-03-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Verify existing OTLP metrics arrive in Grafana Cloud, register new observable gauges for per-type object counts, and create a comprehensive Grafana dashboard JSON (importable to Grafana Cloud) covering sync health, HTTP traffic, per-type sync detail, Go runtime, and business metrics.

</domain>

<decisions>
## Implementation Decisions

### Metrics Export (OBS-01)
- **D-01:** OTLP only — no Prometheus endpoint needed. Metrics already flow to Grafana Cloud via existing OTLP export. No fly.toml `[metrics]` section or Prometheus env vars.
- **D-02:** OBS-01 scope is: verify OTLP metrics arrive in Grafana Cloud Prometheus (Mimir) backend correctly. Confirm the OTel-to-Prometheus name translation is as expected.
- **D-03:** Grafana Cloud Prometheus datasource — queries use PromQL against Mimir backend where OTLP metrics land.

### Dashboard Design (OBS-02 through OBS-10)
- **D-04:** Single dashboard with 5 collapsible rows: sync health, HTTP RED, per-type sync, Go runtime, business metrics. UID: `pdbplus-overview`, title: "PeeringDB Plus - Service Overview"
- **D-05:** Hand-author JSON directly — no Grafana UI design step. Classic schema (pre-v12.2), `schemaVersion: 39`.
- **D-06:** Template variables: `$datasource` (datasource selector for portability), `$type` (PeeringDB type filter), `$region` (Fly.io region), `$interval` (rate interval via `$__rate_interval`).
- **D-07:** `$datasource` variable for portability — no hardcoded datasource UIDs.
- **D-08:** Dashboard uses `__inputs` for importability. Set `id: null`, `version: null` for clean import per Pitfall #11.
- **D-09:** Each row has a documentation text panel explaining metrics and troubleshooting steps.
- **D-10:** Freshness stat panel with color thresholds: green < 3600s (1h), yellow < 7200s (2h), red >= 7200s (2h+).
- **D-11:** Dashboard JSON committed to `deploy/grafana/dashboards/pdbplus-overview.json` with provisioning YAML at `deploy/grafana/provisioning/dashboards.yaml`.

### Business Metrics (OBS-06)
- **D-12:** Register new observable Int64Gauges for ALL 13 PeeringDB types, querying counts via ent (type-safe). Pattern matches existing `InitFreshnessGauge` with `Float64ObservableGauge` and callback.
- **D-13:** Ent queries: `client.Network.Query().Count(ctx)`, `client.Organization.Query().Count(ctx)`, etc. — type-safe, consistent with codebase.
- **D-14:** New metric: `pdbplus.data.type.count` with `type` attribute (net, ix, fac, etc.) — or 13 individual gauges. Claude's discretion on which pattern is cleaner.

### Claude's Discretion
- Whether to use a single `pdbplus.data.type.count` gauge with `type` attribute or 13 separate gauges
- Panel sizes and grid positions in the dashboard JSON
- Default time range and auto-refresh interval
- Whether to include a Fly.io region breakdown in the business metrics row or HTTP row

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### OTel Metrics
- `internal/otel/metrics.go` — All 10 custom metric instruments (9 sync + 1 role transitions)
- `internal/otel/provider.go` — `Setup()` with autoexport, `runtime.Start()`, resource attributes
- `.planning/research/ARCHITECTURE.md` — OTel-to-Prometheus metric name mapping table
- `.planning/research/PITFALLS.md` — Pitfall #2 (OTel name translation), Pitfall #9 (otelhttp names), Pitfall #10 (Go runtime names)

### Dashboard
- `.planning/research/FEATURES.md` — Dashboard row design, panel specifications, PromQL query examples
- `.planning/research/PITFALLS.md` — Pitfall #1 (datasource UID), Pitfall #5 (panel sprawl), Pitfall #8 (schema version), Pitfall #11 (UID/version conflicts)
- `.planning/research/STACK.md` — Grafana JSON format details, Fly.io managed Grafana context

### Deployment
- `fly.toml` — Current deployment config (DO NOT add [metrics] section)
- `deploy/` — Target directory for dashboard artifacts (may need creation)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `InitFreshnessGauge()` in `metrics.go:125` — pattern for observable gauge with callback, reuse for object count gauges
- `autoexport.NewMetricReader()` in `provider.go` — already handles OTLP export
- Resource attributes (`fly.region`, `fly.machine_id`, `fly.app_name`, `service.name`) already set

### Established Patterns
- Flat metric naming: `pdbplus.sync.*` with `type` attribute for per-type breakdown
- Observable gauge with callback: `meter.Float64ObservableGauge(name, ...WithFloat64Callback(fn))`
- otelhttp semantic convention names for HTTP metrics

### Integration Points
- `internal/otel/metrics.go` — where new gauges are registered
- `internal/otel/provider.go` — where gauge callbacks must have access to ent client
- `cmd/peeringdb-plus/main.go` — where ent client is passed to gauge init

</code_context>

<specifics>
## Specific Ideas

- OTel metric names translate to Prometheus: dots→underscores, `s` unit→`_seconds` suffix, counters→`_total` suffix
- otelhttp metrics use semantic conventions: `http.server.request.duration` → `http_server_request_duration_seconds`
- Go runtime metrics: `go.goroutine.count` → `process_runtime_go_goroutines` (OTel contrib runtime package names)
- Dashboard must verify these translations against actual Grafana Cloud metric explorer before finalizing queries

</specifics>

<deferred>
## Deferred Ideas

- Grafana alerting rules — defer until production baselines established
- SLO/SLI tracking — defer until 2-4 weeks of dashboard data
- Annotation markers for sync events — requires OTel Collector or Tempo mapping
- Per-endpoint deep-dive dashboard — add only if overview dashboard reveals problems

</deferred>

---

*Phase: 19-prometheus-metrics-grafana-dashboard*
*Context gathered: 2026-03-24*
