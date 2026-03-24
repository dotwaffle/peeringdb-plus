# Phase 19: Prometheus Metrics & Grafana Dashboard - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-24
**Phase:** 19-prometheus-metrics-grafana-dashboard
**Areas discussed:** Metrics export, dashboard design, dashboard organization, business metrics, template variables, datasource reference

---

## Metrics Export Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Prometheus only | Set OTEL_METRICS_EXPORTER=prometheus, Fly.io scrapes natively | |
| Both Prometheus + OTLP | Custom MeterProvider with dual readers | |
| OTLP only to Grafana Cloud | Existing OTLP export, dashboard queries Grafana Cloud Prometheus (Mimir) | ✓ |

**User's choice:** OTLP only — changed mind from initial "Both" selection. All OTLP data goes to Grafana Cloud. No Prometheus endpoint needed.
**Notes:** Significant pivot from research recommendation. Simplifies infrastructure (no fly.toml changes), dashboard uses PromQL against Grafana Cloud Prometheus/Mimir backend.

---

## Grafana Cloud Datasource

| Option | Description | Selected |
|--------|-------------|----------|
| Grafana Cloud Prometheus | OTLP metrics land in Mimir, PromQL queries | ✓ |
| Grafana Cloud Tempo | Traces only, derive metrics from traces | |
| Not sure | Need to check | |

**User's choice:** Grafana Cloud Prometheus
**Notes:** PromQL queries against Mimir. OTel-to-Prometheus name translation applies.

---

## Dashboard Development Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Hand-author JSON | Write JSON directly using documented schemas | ✓ |
| Design in Grafana, export | Build visually, export, clean up UIDs | |
| Hybrid | Hand-author structure, verify by importing | |

**User's choice:** Hand-author JSON
**Notes:** Fully reproducible, no Grafana instance needed for authoring.

---

## Dashboard Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Single with rows | One dashboard, 5 collapsible rows | ✓ |
| Multiple dashboards | Separate JSON per concern | |
| Single + deep-dive | Overview plus per-type deep-dive | |

**User's choice:** Single with rows

---

## Business Metrics Implementation

| Option | Description | Selected |
|--------|-------------|----------|
| New gauges in Phase 19 | Observable Int64Gauges per type via ent queries | ✓ |
| Derive from counters | Use cumulative sync counters | |
| Defer entirely | Skip business metrics row | |

**User's choice:** New gauges — all 13 PeeringDB types, queried via ent (type-safe)

---

## Template Variables

| Variable | Description | Selected |
|----------|-------------|----------|
| $datasource | Datasource selector for portability | ✓ |
| $type | PeeringDB type filter | ✓ |
| $region | Fly.io region filter | ✓ |
| $interval | Rate interval ($__rate_interval) | ✓ |

**User's choice:** All four variables

---

## Datasource Reference

| Option | Description | Selected |
|--------|-------------|----------|
| $datasource variable | Portable, user selects on import | ✓ |
| Hardcode Grafana Cloud | Simpler but breaks on other instances | |

**User's choice:** $datasource variable

---

## Object Count Query Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Ent queries | Type-safe, consistent with codebase | ✓ |
| Raw SQL | Simpler but bypasses ent | |
| You decide | Claude's discretion | |

**User's choice:** Ent queries

---

## Claude's Discretion

- Single gauge with type attribute vs. 13 separate gauges
- Panel sizes and grid positions
- Default time range and auto-refresh interval
- Region breakdown placement

## Deferred Ideas

- Grafana alerting rules
- SLO/SLI tracking
- Annotation markers for sync events
- Per-endpoint deep-dive dashboard
