---
quick_id: 260426-jke
slug: obs-cleanup
description: Observability cleanup — log noise, trace overflow, redundant metric names, dashboard repair
date: 2026-04-26
status: pending
---

# Quick Task 260426-jke — Observability cleanup

## Background

Audit of Fly logs and Grafana dashboards for `peeringdb-plus` surfaced five concrete issues. Findings:

- `/healthz` access logs are 87 % of all log volume (45,886 of 52,926 entries / 24 h).
- 2,033 per-row "dropping FK orphan" / "nulling orphan FK" WARN entries every sync cycle. They blow Tempo's 7.5 MB per-trace limit (`TRACE_TOO_LARGE 422`); we are silently losing the entire sync trace once per cycle.
- Three metric instruments embed the unit twice in the emitted Prom name (`pdbplus_sync_peak_heap_mib_MiB`, `pdbplus_sync_peak_rss_mib_MiB`, `pdbplus_response_heap_delta_kib_KiB`). Dashboard panels reference the cleaner names without the OTel-appended unit suffix and silently render "no data".
- `pdbplus_sync_*` gauges/counters carry only `job/service_name/service_version`. Dashboard panels filter `{fly_region=~"$region"}` and silently empty whenever a specific region is selected.

User explicitly approved fixing all of the above in a single quick task.

## Plan (4 tasks, 4 atomic commits)

### Task 1 — Skip `/healthz`+`/readyz` from the access log

**Files:** `internal/middleware/logging.go`, `internal/middleware/logging_test.go`.
**Action:** In `Logging`, return early without logging when `r.URL.Path` is `/healthz` or `/readyz`. These two endpoints are owned by Fly's per-15 s health-check loop and produce no operator value at INFO. Add a test that covers the suppression. Hot-path methods: GET only — but match by path regardless of method (HEAD probes, etc).
**Verify:** `go test -race ./internal/middleware/...`
**Done:** Probing `/healthz` 100× emits zero "http request" log entries.

### Task 2 — Aggregate orphan-FK warnings per sync cycle

**Files:** `internal/sync/worker.go`, `internal/otel/metrics.go`, `internal/sync/worker_test.go`.
**Action:**

1. Register a new counter `pdbplus.sync.type.orphans` with attrs `{type, parent_type, field, action=drop|null}` in `internal/otel/metrics.go` (`SyncTypeOrphans`).
2. Add `fkOrphanCounts map[fkOrphanKey]int` and `fkOrphanKey struct { ChildType, ParentType, Field, Action string }` to `Worker`. Reset in `resetFKState`.
3. Replace the per-row `slog.LevelWarn` calls at lines 1102 (nulling orphan FK) and 1260 (dropping FK orphan) with `slog.LevelDebug` + a counter increment via `Worker.recordOrphan`. The counter increment also calls `SyncTypeOrphans.Add(ctx, 1, …)`.
4. Add `Worker.emitOrphanSummary(ctx)` called once per sync cycle from `Worker.Sync` after `recordSuccess` / `rollbackAndRecord`. It emits one WARN log line if total > 0 with structured aggregate counts and one DEBUG line otherwise.

**Verify:** `go test -race ./internal/sync/... ./internal/otel/...`
**Done:** A sync cycle that hits the orphan paths emits ≤ 1 WARN log (the summary), N DEBUG logs (per-row), and N counter increments. Sync trace stays well under 7.5 MB so Tempo accepts it.

### Task 3 — Rename redundant `_mib_MiB` / `_kib_KiB` instruments to bytes

**Files:** `internal/otel/metrics.go`, `internal/otel/metrics_test.go`, `internal/sync/worker.go`, `internal/sync/worker_test.go`, `internal/pdbcompat/telemetry.go`, `internal/pdbcompat/telemetry_test.go`, `internal/pdbcompat/handler.go`, `internal/config/config.go`, `CLAUDE.md`.
**Action:** Switch the three instruments (and their span-attribute mirrors) to base bytes per OTel/Prom canonical practice:

| Old name | New name | Old emitted Prom name | New emitted Prom name |
|---|---|---|---|
| `pdbplus.sync.peak_heap_mib` | `pdbplus.sync.peak_heap` | `pdbplus_sync_peak_heap_mib_MiB` | `pdbplus_sync_peak_heap_bytes` |
| `pdbplus.sync.peak_rss_mib` | `pdbplus.sync.peak_rss` | `pdbplus_sync_peak_rss_mib_MiB` | `pdbplus_sync_peak_rss_bytes` |
| `pdbplus.response.heap_delta_kib` | `pdbplus.response.heap_delta` | `pdbplus_response_heap_delta_kib_KiB_bucket` | `pdbplus_response_heap_delta_bytes_bucket` |

Atomic store types and value units flip to bytes (multiply old MiB int by 1024×1024 — but easier: store the raw `runtime.MemStats.HeapInuse` / `VmHWM` directly). Histogram bucket boundaries multiply by 1024. Variable names: `SyncPeakHeapMiB`→`SyncPeakHeapBytes`, `SyncPeakRSSMiB`→`SyncPeakRSSBytes`, `ResponseHeapDeltaKiB`→`ResponseHeapDeltaBytes`. Span attribute keys updated to `pdbplus.sync.peak_heap_bytes`, `pdbplus.sync.peak_rss_bytes`, `pdbplus.response.heap_delta_bytes`. The `slog` log attrs in `emitMemoryTelemetry` also flip to `peak_heap_bytes` / `peak_rss_bytes` / `heap_warn_bytes` / `rss_warn_bytes` so the human and machine views agree. CLAUDE.md and DEPLOYMENT.md prose updated to match.

**Verify:** `go build ./...`, `go test -race ./internal/otel/... ./internal/sync/... ./internal/pdbcompat/... ./internal/middleware/... ./cmd/...`, `golangci-lint run`.
**Done:** The Prom scrape exposes the new names; old names are no longer emitted; all test references aligned.

### Task 4 — Repair the Grafana dashboard

**Files:** `deploy/grafana/dashboards/pdbplus-overview.json`.
**Action:**

1. Rewrite Peak Heap, Peak RSS, Peak Heap by Process Group, and Response Heap Delta panel queries to reference the new bytes metric names (`pdbplus_sync_peak_heap_bytes`, `pdbplus_sync_peak_rss_bytes`, `pdbplus_response_heap_delta_bytes_bucket`).
2. Set those panels' field unit to `bytes` so Grafana auto-formats MiB/GiB at render time. Update threshold values (defaults 400 MiB / 384 MiB) to bytes (`400*1024*1024` / `384*1024*1024`).
3. Drop the `{fly_region=~"$region"}` filter from every `pdbplus_sync_*` series — the metrics carry only `job/service_name/service_version`.
4. Update panel descriptions to mention the new metric names so future operators searching by name find them.

**Verify:** `cat deploy/grafana/dashboards/pdbplus-overview.json | jq -e .` (parses), then visual inspection in Grafana Cloud after deploy. (No CI assertion for dashboard JSON.)
**Done:** All four broken panels render data when scoped to "All" regions; per-region dropdown still works for HTTP RED panels (which DO carry `fly_region`).

## Out of scope

- Adding `fly.process_group` resource attribute (Phase-65 follow-up; tracked elsewhere).
- Recording the otlptrace exporter's "TRACE_TOO_LARGE" log at WARN — that originates inside the OTel SDK, not our code. Task 2's event-volume reduction is the actual fix.
- Touching Fallback Events / Role Transitions / Upsert Errors panels — those instruments exist; the underlying events haven't fired in the queried window. Optionally enabling "null as zero" panel option is left for a future tweak.
- Deploying. User will deploy after merge.

## must_haves

- `truths`:
  - `MUST emit orphan-FK details at DEBUG, not WARN, on the per-row hot path.`
  - `MUST emit at most one WARN log per sync cycle summarising orphan-FK actions.`
  - `MUST drop the redundant unit suffix (_mib_MiB / _kib_KiB) on the three identified Prom-exposed metrics.`
  - `MUST suppress access-log entries for /healthz and /readyz.`
  - `MUST re-point the dashboard panels to the new metric names so they render data again.`
- `artifacts`:
  - `internal/middleware/logging.go (modified)`
  - `internal/sync/worker.go (modified)`
  - `internal/otel/metrics.go (modified)`
  - `internal/pdbcompat/telemetry.go (modified)`
  - `deploy/grafana/dashboards/pdbplus-overview.json (modified)`
  - tests updated to match
- `key_links`:
  - `CLAUDE.md § Sync observability` — operator-visible names live here, must stay accurate.
