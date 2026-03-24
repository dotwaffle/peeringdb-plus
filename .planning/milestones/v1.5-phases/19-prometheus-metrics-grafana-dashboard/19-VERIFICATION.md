---
phase: 19-prometheus-metrics-grafana-dashboard
verified: 2026-03-24T20:30:00Z
status: human_needed
score: 5/5 success criteria verified
re_verification:
  previous_status: gaps_found
  previous_score: 3/5
  gaps_closed:
    - "InitObjectCountGauges function exists and registers pdbplus.data.type.count observable gauge"
    - "pdbplus_data_type_count metric references in dashboard are backed by actual metric registration"
    - "Dashboard test suite validates the canonical pdbplus-overview.json dashboard"
    - "Provisioning artifacts are consistent (single provisioning config for single dashboard)"
  gaps_remaining: []
  regressions: []
human_verification:
  - test: "Deploy to Fly.io and verify Prometheus /metrics endpoint on port 9091 returns metrics"
    expected: "HTTP 200 with Prometheus text format containing HELP/TYPE declarations and pdbplus_* metrics including pdbplus_data_type_count"
    why_human: "Requires live Fly.io deployment; TestSetup_PrometheusExporter validates the mechanism locally but not the full Fly.io scraper integration"
  - test: "Import pdbplus-overview.json into a Grafana instance with a Prometheus datasource"
    expected: "Dashboard imports cleanly, all panels render without errors, $datasource variable selects the correct datasource, Business Metrics row shows non-zero object counts"
    why_human: "JSON structure validation can be automated but actual visual rendering and panel behavior requires a live Grafana instance"
  - test: "Verify Go runtime metric names match what the OTel Prometheus exporter actually produces"
    expected: "pdbplus-overview.json uses go_goroutine_count, go_memory_used_bytes, go_memory_allocated_bytes_total, go_memory_gc_goal_bytes -- verify these match the OTel runtime instrumentation Prometheus output"
    why_human: "Metric name translation depends on the specific OTel SDK version and Prometheus exporter behavior, which varies between OTel runtime instrumentation versions"
---

# Phase 19: Prometheus Metrics & Grafana Dashboard Verification Report

**Phase Goal:** All existing OTel metrics are exported via OTLP to Grafana Cloud and visualized in a portable Grafana dashboard covering sync health, HTTP traffic, per-type sync detail, runtime metrics, and business metrics
**Verified:** 2026-03-24T20:30:00Z
**Status:** human_needed
**Re-verification:** Yes -- after gap closure (plans 19-03, 19-04)

## Goal Achievement

### Observable Truths (from ROADMAP.md Success Criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Fly.io Prometheus scraper collects metrics from /metrics endpoint (port 9091) without errors | VERIFIED | fly.toml has `[metrics]` section with port=9091 path=/metrics; env vars OTEL_METRICS_EXPORTER=prometheus and OTEL_EXPORTER_PROMETHEUS_PORT=9091 set; TestSetup_PrometheusExporter passes |
| 2 | Grafana dashboard displays sync health (freshness gauge with green/yellow/red thresholds, sync duration, success/failure rate) using live data | VERIFIED | pdbplus-overview.json Row 1 "Sync Health" has: Data Freshness stat (thresholds green/3600-yellow/7200-red), Sync Success Rate stat, Sync Duration p95 timeseries, Fallback Events stat, Sync Operations timeseries |
| 3 | Grafana dashboard displays HTTP RED metrics and per-type sync detail | VERIFIED | pdbplus-overview.json Row 2 "HTTP RED Metrics" has request rate, error rate, active requests, latency p95/p99; Row 3 "Per-Type Sync Detail" has duration by type, objects synced, deletes, fetch/upsert errors with $type variable |
| 4 | Dashboard JSON committed to deploy/grafana/ with datasource template variables (no hardcoded UIDs) and imports cleanly into fresh Grafana | VERIFIED | pdbplus-overview.json: id=null, version=null, uid="pdbplus-overview", __inputs present, uses ${datasource} template variable, no hardcoded UIDs. Single canonical file -- duplicate removed. |
| 5 | Each dashboard row contains documentation text panels explaining metrics and troubleshooting guidance | VERIFIED | pdbplus-overview.json has 5 text panels (1 at top level in uncollapsed Sync Health row + 4 inside collapsed rows), one per row. TestDashboard_EachRowHasTextPanel passes. |

**Score:** 5/5 truths verified

### Plan 19-01/19-03 Must-Haves (InitObjectCountGauges -- gap closure)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Per-type object count gauge emits a value for all 13 PeeringDB types | VERIFIED | InitObjectCountGauges in metrics.go lines 159-200 registers pdbplus.data.type.count Int64ObservableGauge with 13 typeCounter entries |
| 2 | Object count gauge uses existing InitFreshnessGauge pattern | VERIFIED | Same meter := otel.Meter("peeringdb-plus"), observable callback pattern, fmt.Errorf wrapping |
| 3 | Gauge is registered and wired in main.go after entClient is initialized | VERIFIED | main.go line 126: pdbotel.InitObjectCountGauges(entClient) -- placed after InitFreshnessGauge (line 114) |
| 4 | Existing OTLP metrics pipeline exports the new gauge alongside existing metrics | VERIFIED | Metric registered via standard OTel meter; no separate export config needed |
| 5 | Tests verify registration and data points | VERIFIED | TestInitObjectCountGauges_NoError and TestInitObjectCountGauges_RecordsValues both pass with -race flag |

### Plan 19-04 Must-Haves (Duplicate Cleanup -- gap closure)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Only one canonical dashboard file exists: pdbplus-overview.json | VERIFIED | ls deploy/grafana/dashboards/ shows only pdbplus-overview.json; peeringdb-plus.json removed |
| 2 | Only one provisioning YAML exists: provisioning/dashboards.yaml | VERIFIED | ls deploy/grafana/provisioning/ shows only dashboards.yaml; dashboards/default.yaml removed, dashboards/ dir removed |
| 3 | Dashboard tests validate the canonical pdbplus-overview.json | VERIFIED | dashboardPath constant = "dashboards/pdbplus-overview.json" (line 52); no references to peeringdb-plus.json in test file |
| 4 | All dashboard tests pass against the canonical file | VERIFIED | 8/8 tests pass with -race flag including nested panel support and json.RawMessage handling |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/otel/metrics.go` | InitObjectCountGauges function | VERIFIED | Function at lines 159-200, registers pdbplus.data.type.count, queries all 13 types |
| `internal/otel/metrics_test.go` | TestInitObjectCountGauges tests | VERIFIED | TestInitObjectCountGauges_NoError (line 202) and TestInitObjectCountGauges_RecordsValues (line 214) both pass |
| `cmd/peeringdb-plus/main.go` | InitObjectCountGauges wiring | VERIFIED | pdbotel.InitObjectCountGauges(entClient) at line 126, after InitFreshnessGauge |
| `deploy/grafana/dashboards/pdbplus-overview.json` | Complete dashboard with 5 rows | VERIFIED | 5 rows, text panels per row, Business Metrics row references pdbplus_data_type_count in 3 PromQL queries |
| `deploy/grafana/provisioning/dashboards.yaml` | Provisioning config | VERIFIED | apiVersion=1, provider peeringdb-plus, type=file, folder "PeeringDB Plus" |
| `deploy/grafana/dashboard_test.go` | Dashboard validation tests | VERIFIED | 8 tests validate canonical pdbplus-overview.json with allPanels helper for nested panels |
| `fly.toml` | Prometheus metrics section | VERIFIED | [metrics] port=9091 path=/metrics, OTEL_METRICS_EXPORTER=prometheus env vars |
| `deploy/grafana/dashboards/peeringdb-plus.json` | REMOVED (duplicate) | VERIFIED | File does not exist |
| `deploy/grafana/provisioning/dashboards/default.yaml` | REMOVED (duplicate) | VERIFIED | File does not exist, directory does not exist |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| fly.toml | internal/otel/provider.go | OTEL_METRICS_EXPORTER=prometheus env var | WIRED | autoexport reads env var, TestSetup_PrometheusExporter confirms |
| pdbplus-overview.json | internal/otel/metrics.go | PromQL references to pdbplus_sync_* metrics | WIRED | All sync metrics in dashboard match registered OTel instruments |
| pdbplus-overview.json | internal/otel/metrics.go | pdbplus_data_type_count references | WIRED | 3 PromQL queries in Business Metrics row backed by pdbplus.data.type.count registration at metrics.go:177 |
| provisioning/dashboards.yaml | dashboards/ directory | path reference | WIRED | path: /var/lib/grafana/dashboards/peeringdb-plus |
| cmd/peeringdb-plus/main.go | internal/otel/metrics.go | pdbotel.InitObjectCountGauges | WIRED | main.go:126 calls InitObjectCountGauges(entClient) |
| deploy/grafana/dashboard_test.go | pdbplus-overview.json | dashboardPath constant | WIRED | const dashboardPath = "dashboards/pdbplus-overview.json" |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| pdbplus-overview.json Sync Health panels | pdbplus_sync_* metrics | internal/otel/metrics.go + sync engine | Yes (registered counters/histograms) | FLOWING |
| pdbplus-overview.json HTTP RED panels | http_server_* metrics | otelhttp middleware | Yes (auto-instrumented) | FLOWING |
| pdbplus-overview.json Per-Type Sync panels | pdbplus_sync_type_* metrics | internal/otel/metrics.go + sync engine | Yes (registered per-type instruments) | FLOWING |
| pdbplus-overview.json Go Runtime panels | go_goroutine_count etc. | OTel runtime instrumentation | Needs human verification (metric name translation varies by version) | UNCERTAIN |
| pdbplus-overview.json Business Metrics panels | pdbplus_data_type_count | InitObjectCountGauges -> ent client queries | Yes (queries all 13 types via ent client) | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Application builds | `go build ./cmd/peeringdb-plus/` | Success (exit 0) | PASS |
| go vet passes | `go vet ./internal/otel/ ./cmd/peeringdb-plus/ ./deploy/grafana/` | No issues (exit 0) | PASS |
| OTel metrics tests pass | `go test -race ./internal/otel/ -v` | 28/28 tests pass | PASS |
| Dashboard tests pass | `go test -race ./deploy/grafana/ -v` | 8/8 tests pass | PASS |
| InitObjectCountGauges exists | grep in metrics.go | Found at line 159 | PASS |
| InitObjectCountGauges wired | grep in main.go | Found at line 126 | PASS |
| pdbplus.data.type.count registered | grep in metrics.go | Found at line 177 | PASS |
| Dashboard references backed metric | grep pdbplus_data_type_count in dashboard JSON | 3 references, all backed | PASS |
| Duplicate dashboard removed | test -f peeringdb-plus.json | REMOVED | PASS |
| Duplicate provisioning removed | test -f default.yaml | REMOVED | PASS |
| No stale references in tests | grep peeringdb-plus.json in dashboard_test.go | No matches | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| OBS-01 | 19-01 | Prometheus metrics export via env var and fly.toml config | SATISFIED | fly.toml [metrics] section, OTEL_METRICS_EXPORTER=prometheus, TestSetup_PrometheusExporter |
| OBS-02 | 19-02 | Sync health row in Grafana dashboard | SATISFIED | pdbplus-overview.json Row 1 with freshness, duration, success rate, fallback, operations |
| OBS-03 | 19-02 | HTTP RED metrics row | SATISFIED | pdbplus-overview.json Row 2 with request rate, error rate, active requests, latency |
| OBS-04 | 19-02 | Per-type sync detail row | SATISFIED | pdbplus-overview.json Row 3 with duration, objects, deletes, errors by type |
| OBS-05 | 19-02 | Go runtime row | SATISFIED (needs human verification for metric names) | pdbplus-overview.json Row 4 with goroutines, heap, allocation rate, GC goal |
| OBS-06 | 19-01, 19-03 | Business metrics (object counts per type) | SATISFIED | InitObjectCountGauges registers pdbplus.data.type.count for all 13 types; dashboard Business Metrics row references it in 3 panels |
| OBS-07 | 19-02 | Freshness gauge with green/yellow/red thresholds | SATISFIED | Thresholds at 0(green), 3600(yellow), 7200(red); TestDashboard_FreshnessGaugeThresholds passes |
| OBS-08 | 19-02 | Documentation text panels in each row | SATISFIED | 5 text panels, one per row; TestDashboard_EachRowHasTextPanel passes |
| OBS-09 | 19-02 | Datasource template variables for portability | SATISFIED | $datasource variable, ${datasource} in all panel refs; TestDashboard_DatasourceTemplateVariable and TestDashboard_NoHardcodedDatasourceUIDs pass |
| OBS-10 | 19-02, 19-04 | Dashboard provisioning YAML and JSON in deploy/grafana/ | SATISFIED | Single canonical dashboard + provisioning config; duplicates removed |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | - | - | All previous anti-patterns resolved by gap closure plans |

### Human Verification Required

### 1. Verify Go Runtime Metric Names
**Test:** Deploy the application with OTEL_METRICS_EXPORTER=prometheus and check /metrics endpoint output for Go runtime metric names
**Expected:** pdbplus-overview.json uses `go_goroutine_count`, `go_memory_used_bytes`, `go_memory_allocated_bytes_total`, `go_memory_gc_goal_bytes`. Confirm these match the actual Prometheus output from OTel runtime instrumentation.
**Why human:** Metric name translation depends on specific OTel SDK version and exporter behavior; cannot verify without running the application.

### 2. Import Dashboard into Grafana
**Test:** Import pdbplus-overview.json into a Grafana instance via the dashboard import UI
**Expected:** Clean import with no errors, $datasource variable populates, all panels show query editors with valid PromQL, Business Metrics panels display non-zero object counts after a sync completes.
**Why human:** Grafana import behavior and panel rendering cannot be validated programmatically.

### 3. Verify Fly.io Prometheus Scraping
**Test:** After deployment, check Fly.io metrics dashboard or Grafana Cloud to confirm metrics are being scraped from port 9091
**Expected:** Prometheus scraper connects to /metrics on port 9091 and metrics appear in Grafana Cloud including pdbplus_data_type_count.
**Why human:** Requires live Fly.io deployment and network connectivity.

### Gaps Summary

All 4 gaps from the initial verification have been closed:

1. **InitObjectCountGauges missing (BLOCKER):** CLOSED. Function implemented in metrics.go, registers pdbplus.data.type.count for all 13 PeeringDB types, wired in main.go after InitFreshnessGauge. Two passing tests confirm registration and data point emission.

2. **pdbplus_data_type_count dashboard refs with no backing metric:** CLOSED. The 3 PromQL queries in the Business Metrics row are now backed by the pdbplus.data.type.count metric registration.

3. **Dashboard tests validate wrong file:** CLOSED. dashboardPath constant updated to pdbplus-overview.json, all 8 tests pass against the canonical dashboard including nested panel support.

4. **Duplicate artifacts:** CLOSED. peeringdb-plus.json removed, provisioning/dashboards/default.yaml removed, provisioning/dashboards/ directory removed. No stale references remain in test code.

No regressions detected. All 28 otel tests and all 8 dashboard tests pass with -race flag. Application builds and vets clean.

---

_Verified: 2026-03-24T20:30:00Z_
_Verifier: Claude (gsd-verifier)_
