---
quick_id: 260426-jke
slug: obs-cleanup
description: Observability cleanup — log noise, trace overflow, redundant metric names, dashboard repair
date: 2026-04-26
status: complete
---

# Quick Task 260426-jke — Summary

Triggered by the Fly + Grafana audit performed 2026-04-26 over a 24 h window. The audit surfaced five concrete operability issues; this quick task addressed all of them in four atomic commits on `main`.

## What changed

| Concern | Resolution | Commit |
|---|---|---|
| `/healthz` + `/readyz` access logs were 87 % of all log volume (45,886 / 52,926 entries / 24 h) | `internal/middleware/logging.go` returns early without emitting an INFO line for these two paths. Trace context and otelhttp metrics are unaffected — they fire upstream. | `7ca08bd` |
| 2,033 per-row `dropping FK orphan` / `nulling orphan FK` WARN logs / 24 h were attaching span events to the active sync span and pushing it past Tempo's 7.5 MB per-trace cap (silent `TRACE_TOO_LARGE 422`). | Added `pdbplus.sync.type.orphans{type, parent_type, field, action}` counter. Per-row events log at DEBUG and increment the counter via `Worker.recordOrphan`. `Worker.emitOrphanSummary` fires once at the terminal Sync paths (`recordSuccess` + `recordFailure`) with one WARN summary on a dirty cycle, DEBUG with `total=0` on a clean one. | `d6bfa7b` |
| `pdbplus_sync_peak_heap_mib_MiB`, `pdbplus_sync_peak_rss_mib_MiB`, `pdbplus_response_heap_delta_kib_KiB` had the unit encoded twice in the emitted Prom name (OTel-to-Prom appends the unit suffix; the metric name already contained it). | Renamed instruments to canonical bytes per OTel/Prom convention: `pdbplus.sync.peak_heap`, `pdbplus.sync.peak_rss`, `pdbplus.response.heap_delta` — all unit `By`. Atomics now store raw `HeapInuse` / VmHWM bytes. Histogram bucket boundaries scaled ×1024. Span attrs and slog attrs flipped from `_mib` / `_kib` to `_bytes`. CLAUDE.md, `docs/ARCHITECTURE.md`, `docs/DEPLOYMENT.md` prose realigned. | `0ee9f40` |
| Four dashboard panels (Peak Heap, Peak RSS, Peak Heap by Process Group, Response Heap Delta) referenced metric names without the OTel-appended unit suffix and silently rendered "no data". `pdbplus_sync_*` and `go_*` panels filtered on `fly_region` — a label none of those instruments carry — so selecting a specific region in the templated dropdown emptied the panels. | Updated panel queries to the new bytes metric names; field unit `bytes` (auto-formats MiB / GiB); thresholds converted to bytes. Dropped `{fly_region=~"$region"}` from every `pdbplus_*` and `go_*` query; preserved on `http_server_*`. Region template-variable definition switched to `http_server_request_duration_seconds_count` so the dropdown populates. | `cef357a` |

## Verification

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test -race ./...` — clean (whole project)
- `golangci-lint run` — 0 issues
- `python3 -c "import json; json.load(open('deploy/grafana/dashboards/pdbplus-overview.json'))"` — valid JSON
- Visual check: every `pdbplus_*` query in the dashboard now references either no `{}` filter or only the labels the metric actually carries. Confirmed via:
  - `pdbplus_data_type_count` returns rows; only labels are `job/service_name/service_version/type`.
  - `pdbplus_sync_peak_heap_bytes{fly_region="lhr"}` would return empty (the label is absent) — dashboard no longer applies that filter.

## Out of scope (deferred)

- `fly.process_group` resource attribute wiring on the OTel SDK side (Phase-65 follow-up). Until that lands, the "Peak Heap by Process Group" panel still groups by a single absent series; the panel description was updated to flag this rather than removed.
- `traces export … failed: TRACE_TOO_LARGE` log line is emitted by the OTel SDK, not by our code, so its INFO level can't be changed directly. The fix was indirect: aggregating per-row span events in (2) keeps each sync trace well under the Tempo 7.5 MB cap.
- Sync failure burst (6 / 24 h on `api.peeringdb.com` org/poc fetches): not fixed by this task. Already counted in `pdbplus_sync_operations_total{status="failed"}` and visible on the dashboard. If the rate sustains, an alert rule against that metric is the next step.

## Effect after deploy

- Loki ingestion volume should drop ~87 % at INFO (the `/healthz` flood). Operator views become useful again.
- Each sync cycle will fire ≤ 1 WARN log for orphan FKs (or DEBUG with `total=0` on a clean cycle). Tempo should accept the trace.
- Dashboard panels under "Sync Memory (SEED-001 watch)" will render data in MiB / GiB via Grafana's auto-formatter once new metric data is scraped post-deploy. Existing historical data on the old metric names continues to live under those names until Prometheus retention drops it.
- Region dropdown will populate from the http_server_* series as soon as a request is observed in each region.
