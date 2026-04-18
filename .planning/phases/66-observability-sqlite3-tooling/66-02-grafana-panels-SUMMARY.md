---
phase: 66-observability-sqlite3-tooling
plan: 02
subsystem: observability
tags: [grafana, dashboard, otel, seed-001]
status: complete
completed: "2026-04-18"
commit: 491dc85
---

# Phase 66 Plan 02 — Grafana Panels for Peak Heap + RSS (OBS-05)

## What shipped

Three new panels under a new row "Sync Memory (SEED-001 watch)" in `deploy/grafana/dashboards/pdbplus-overview.json`:

| ID | Title | Query | Threshold |
|----|-------|-------|-----------|
| 33 | Peak Heap (MiB) | `pdbplus_sync_peak_heap_mib{fly_region=~"$region"}` | line at 400 MiB |
| 34 | Peak RSS (MiB) | `pdbplus_sync_peak_rss_mib{fly_region=~"$region"}` | line at 384 MiB |
| 35 | Peak Heap by Process Group | same expr, legend `{{fly_process_group}} / {{fly_region}}` | no threshold |

Row ID 32, placed at gridPos y=24 (after the existing "Business Metrics" row).

## Data source

Relies on the OTel ObservableGauges added by Plan 66-01 commit `4e90cfd`:
- `pdbplus.sync.peak_heap_mib` (Int64ObservableGauge)
- `pdbplus.sync.peak_rss_mib` (Int64ObservableGauge)

These export through the existing Prometheus pipeline at `pdbplus_sync_peak_heap_mib` and `pdbplus_sync_peak_rss_mib`.

## Known limitation

Panel 35 (process-group breakdown) assumes `fly.process_group` is exported as an OTel resource attribute. This wiring is not yet present in v1.15 — Phase 65 split the fleet into process groups but `internal/otel/setup.go` does not yet forward the `FLY_PROCESS_GROUP` environment variable as a resource attribute. Until that small wiring lands, Panel 35 will render a single series without the breakdown. Documented as a follow-up item in the SUMMARY — not a blocker for Phase 66 closure since the panel renders cleanly (just without the grouping).

## Verification

- `jq empty deploy/grafana/dashboards/pdbplus-overview.json` → clean (JSON valid)
- Panel count: 16 total (was 12, +4 including the row)
- 3 anchors grep-verified: `Sync Memory (SEED-001 watch)`, `pdbplus_sync_peak_heap_mib` (2×), `pdbplus_sync_peak_rss_mib` (1×)
- jq re-normalized the file's indentation (1227 → 2095 lines) but content is identical; Grafana imports by JSON content so whitespace churn is cosmetic.

## Commit

`491dc85` — `feat(66-02): add Grafana panels for peak heap+RSS + process-group (OBS-05)`

## Unblocks

- Plan 66-03 (docs) — can cite metric names verbatim: `pdbplus_sync_peak_heap_mib`, `pdbplus_sync_peak_rss_mib`.
- Operators running `pdbplus-overview.json` pick up the SEED-001 watch panel on the next dashboard re-import.
