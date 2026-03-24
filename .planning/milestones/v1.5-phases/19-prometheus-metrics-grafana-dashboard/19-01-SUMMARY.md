---
phase: 19-prometheus-metrics-grafana-dashboard
plan: "01"
subsystem: infra
tags: [prometheus, grafana, otel, metrics, observability, fly-io]

# Dependency graph
requires:
  - phase: 04-observability-foundations
    provides: OTel autoexport setup with runtime metrics and custom sync instruments
provides:
  - Prometheus metrics endpoint on port 9091 via OTEL_METRICS_EXPORTER=prometheus
  - Grafana dashboard JSON with 5 rows covering sync, HTTP, per-type, runtime, business metrics
  - Dashboard provisioning YAML for automated Grafana setup
  - Dashboard validation test suite (8 tests)
affects: [20-deferred-human-verification]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - OTel autoexport prometheus reader for metrics export (no new Go dependencies)
    - Grafana dashboard JSON with datasource template variables for portability
    - Fly.io [metrics] section for managed Prometheus scraping

key-files:
  created:
    - deploy/grafana/dashboards/peeringdb-plus.json
    - deploy/grafana/provisioning/dashboards/default.yaml
    - deploy/grafana/dashboard_test.go
  modified:
    - fly.toml
    - internal/otel/provider_test.go

key-decisions:
  - "No new Go dependencies: autoexport already supports prometheus exporter via env var"
  - "Port 9091 for Prometheus metrics: standard OTel prometheus exporter default, separate from app port"
  - "Hand-authored dashboard JSON: simpler than Grafonnet/Jsonnet for a single dashboard"

patterns-established:
  - "deploy/grafana/ directory structure for dashboard and provisioning artifacts"
  - "Dashboard test pattern: Go tests validate JSON structure, metric references, and thresholds"

requirements-completed: [OBS-01, OBS-02, OBS-03, OBS-04, OBS-05, OBS-06, OBS-07, OBS-08, OBS-09, OBS-10]

# Metrics
duration: 6min
completed: 2026-03-24
---

# Phase 19 Plan 01: Prometheus Export & Grafana Dashboard Summary

**Prometheus metrics endpoint via OTel autoexport env config and 5-row Grafana dashboard with freshness gauge, HTTP RED, per-type sync detail, Go runtime, and business metrics**

## Performance

- **Duration:** 6 min
- **Started:** 2026-03-24T17:16:41Z
- **Completed:** 2026-03-24T17:23:08Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments
- Enabled Prometheus metrics export via OTEL_METRICS_EXPORTER=prometheus with zero new Go dependencies
- Created comprehensive Grafana dashboard with 5 metric rows, 29 panels, and documentation text panels
- Data freshness gauge with green (<1h) / yellow (<2h) / red (>2h) color thresholds
- All dashboard datasource references use ${DS_PROMETHEUS} template variable for portability
- 8 dashboard validation tests verify JSON structure, metric names, and thresholds

## Task Commits

Each task was committed atomically:

1. **Task 1: Enable Prometheus metrics endpoint** - `0335bcc` (feat)
2. **Task 2: Create Grafana dashboard JSON with all rows** - `8445a0d` (feat)
3. **Task 3: Verify dashboard JSON validity and metric references** - `f8a6ac9` (test)

## Files Created/Modified
- `fly.toml` - Added [metrics] section and OTEL_METRICS_EXPORTER/PORT env vars
- `deploy/grafana/dashboards/peeringdb-plus.json` - Comprehensive Grafana dashboard with 5 rows
- `deploy/grafana/provisioning/dashboards/default.yaml` - Grafana provisioning config
- `deploy/grafana/dashboard_test.go` - 8 tests validating dashboard structure and metric references
- `internal/otel/provider_test.go` - Added TestSetup_PrometheusExporter test

## Decisions Made
- No new Go dependencies needed: autoexport already supports prometheus exporter via OTEL_METRICS_EXPORTER env var
- Port 9091 chosen for Prometheus metrics: matches OTel prometheus exporter default, separate from app port 8081
- Hand-authored dashboard JSON rather than Grafonnet/Jsonnet: simpler for a single dashboard, easier to review

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required. Prometheus scraping is configured via fly.toml [metrics] section and will activate on next deployment.

## Next Phase Readiness
- Prometheus endpoint configured and tested, ready for Fly.io deployment
- Grafana dashboard JSON ready for import into Fly.io managed Grafana
- Phase 20 (Deferred Human Verification) can proceed independently

---
*Phase: 19-prometheus-metrics-grafana-dashboard*
*Completed: 2026-03-24*
