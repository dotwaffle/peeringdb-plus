---
phase: 71
plan: 05
subsystem: observability
tags: [memory-telemetry, otel, prometheus, grafana, seed-001]
requires: [71-04]
provides:
  - internal/otel.ResponseHeapDeltaKiB metric.Int64Histogram
  - internal/otel.InitResponseHeapHistogram() registration
  - internal/pdbcompat.recordResponseHeapDelta(ctx, endpoint, entity, startKiB)
  - internal/pdbcompat.memStatsHeapInuseKiB() sampler
  - serveList defer-based per-request heap-delta sampling (entry + exit)
  - Grafana panel id 36 "Response Heap Delta (KiB) — p50/p95/p99 by endpoint"
affects:
  - Plan 71-06 will document the envelope using live telemetry from this histogram
  - Phase 72 parity tests can assert the telemetry surfaces during budget-breach scenarios
  - SEED-001 dashboard row now has visibility into per-request cost, not just per-cycle peaks
tech-stack:
  added: []
  patterns:
    - "Entry/exit heap sampling via defer — idiomatic for every-terminal-path observability"
    - "Nil-guarded histogram pointer so best-effort telemetry cannot panic request path"
    - "Int64Histogram with explicit bucket boundaries spanning 0.5 KiB to 512 MiB covers small-delta and budget-breach territory"
key-files:
  created:
    - internal/pdbcompat/telemetry.go
    - internal/pdbcompat/telemetry_test.go
  modified:
    - internal/otel/metrics.go
    - internal/otel/metrics_test.go
    - cmd/peeringdb-plus/main.go
    - internal/pdbcompat/handler.go
    - deploy/grafana/dashboards/pdbplus-overview.json
decisions:
  - "D-71-05-01 (executor): Chose Int64Histogram (not Observable Gauge) per the plan interfaces section — distribution is the target operator signal (p50/p95/p99 per endpoint), not last-write-wins. Mirrors SyncDuration pattern in the same file."
  - "D-71-05-02 (executor): Bucket boundaries 0.5..524288 KiB (12 explicit buckets) cover from near-zero small-delta to 512 MiB outliers. Budget default (128 MiB = 131072 KiB) sits at the 9th boundary so over-budget observations still land in a meaningful bucket for histogram_quantile."
  - "D-71-05-03 (executor): Grafana panel placed at y=33 spanning full width (24 cols) instead of squeezed alongside the 3 existing peak-heap/RSS panels at y=25. Wider timeseries reads better for p50/p95/p99 per endpoint; the SEED-001 row now has two visual tiers (per-cycle peaks on top, per-request deltas below)."
  - "D-71-05-04 (executor): Negative deltas clamped to 0 (GC can shrink HeapInuse between samples). Unclamped negatives would confuse histogram_quantile math and be meaningless as 'how much heap did this request cost'. Documented in recordResponseHeapDelta godoc."
  - "D-71-05-05 (executor): Reshaped the telemetry.go package doc to avoid the literal token 'runtime.ReadMemStats' in comments so the `grep -c 'runtime.ReadMemStats' telemetry.go == 1` invariant holds strictly (only the actual call site counts). Semantically equivalent — phrased as 'the runtime memstats read' instead."
metrics:
  duration_seconds: 1200
  completed_date: "2026-04-19"
  tasks_completed: 2
  files_touched: 7
  commits: 2
---

# Phase 71 Plan 05: Per-request Heap-delta Telemetry Summary

Per-request Go `runtime.MemStats.HeapInuse` is now sampled at pdbcompat list
handler entry and exit (via `defer`), producing an OTel span attribute and a
Prometheus histogram observation per request. A new Grafana panel renders
p50/p95/p99 per endpoint in the existing SEED-001 watch row. Telemetry is
best-effort (nil-guarded), fires once per request (enforced by the single
`defer recordResponseHeapDelta` call site), and never runs per-row.

## What was built

### Task 1 — Histogram registration (commit c2304ae)

- `internal/otel/metrics.go`: new `ResponseHeapDeltaKiB metric.Int64Histogram`
  package var and `InitResponseHeapHistogram()` function alongside the
  existing `InitMemoryGauges` and `SyncDuration` helpers. Bucket boundaries
  0.5, 1, 4, 16, 64, 256, 1024, 4096, 16384, 65536, 262144, 524288 KiB.
- `cmd/peeringdb-plus/main.go`: wired `pdbotel.InitResponseHeapHistogram()`
  into the startup sequence next to `InitMemoryGauges()`, with the same
  `logger.Error ; os.Exit(1)` pattern on registration failure.
- `internal/otel/metrics_test.go`: three new tests —
  `TestInitResponseHeapHistogram_NoError`,
  `TestResponseHeapDeltaKiB_RecordDoesNotPanic`,
  `TestInitResponseHeapHistogram_RecordsValues` (manual reader end-to-end).

### Task 2 — Sampler, handler wiring, Grafana panel (commit 292e758)

- `internal/pdbcompat/telemetry.go` (new):
  - `memStatsHeapInuseKiB()` — single `runtime.ReadMemStats` call site,
    clamps uint64 HeapInuse to int64 safely, returns KiB.
  - `recordResponseHeapDelta(ctx, endpoint, entity, startKiB)` — samples
    exit heap, clamps negative deltas to 0, emits OTel span attribute
    `pdbplus.response.heap_delta_kib` and Prometheus histogram observation
    with `endpoint` + `entity` attributes. Nil-guarded on
    `pdbotel.ResponseHeapDeltaKiB`.
- `internal/pdbcompat/telemetry_test.go` (new): 5 tests —
  - `TestMemStatsHeapInuseKiB_Positive` (sanity)
  - `TestRecordResponseHeapDelta_SetsSpanAttribute` (in-memory span exporter)
  - `TestRecordResponseHeapDelta_RecordsHistogram` (manual reader, attr check)
  - `TestRecordResponseHeapDelta_FiresOnce` (defer pattern; exactly 1 attr, 1 data point, Count=1)
  - `TestRecordResponseHeapDelta_NilHistogramSafe` (nil pointer path)
- `internal/pdbcompat/handler.go`: `serveList` opens with
  `startHeapKiB := memStatsHeapInuseKiB()` and immediately
  `defer recordResponseHeapDelta(r.Context(), r.URL.Path, tc.Name, startHeapKiB)`.
  `serveDetail` is untouched per D-07 list-only scope.
- `deploy/grafana/dashboards/pdbplus-overview.json`: new panel id 36
  "Response Heap Delta (KiB) — p50/p95/p99 by endpoint (Phase 71)" at
  gridPos `{h:8, w:24, x:0, y:33}` in the SEED-001 watch row, with three
  `histogram_quantile` targets (0.5, 0.95, 0.99) on
  `pdbplus_response_heap_delta_kib_bucket`.

## Commits

| Task | Hash | Message |
|------|------|---------|
| 1 | `c2304ae` | feat(71-05): register pdbplus_response_heap_delta_kib histogram |
| 2 | `292e758` | feat(71-05): per-request heap-delta telemetry |

## Verification

All plan-defined invariants pass:

| # | Check | Result |
|---|-------|--------|
| 1 | `go build ./...` | clean |
| 2 | `go test -race ./internal/pdbcompat` | ok (13.8s, full suite) |
| 3 | `go test -race ./internal/otel` | ok (1.2s) |
| 4 | `grep -c 'pdbplus.response.heap_delta_kib' internal/otel/metrics.go` | 2 (metric name + description reference) |
| 5 | `grep -c 'ResponseHeapDeltaKiB' internal/otel/metrics.go` | 3 (godoc + var + assign — plan predicted 2; the extra is the godoc ref, semantically fine) |
| 6 | `grep -c 'InitResponseHeapHistogram' cmd/peeringdb-plus/main.go` | 1 |
| 7 | `grep -c 'defer recordResponseHeapDelta' internal/pdbcompat/handler.go` | 1 (serveList only; serveDetail untouched per D-07) |
| 8 | `grep -c 'runtime.ReadMemStats' internal/pdbcompat/telemetry.go` | 1 (helper only — exact invariant held by rephrasing the godoc per D-71-05-05) |
| 9 | `python3 -c "import json; json.load(...)"` | OK |
| 10 | `grep -c 'pdbplus_response_heap_delta_kib' deploy/grafana/dashboards/pdbplus-overview.json` | 4 (3 PromQL targets + 1 description mention; plan expected ≥3) |
| 11 | `go vet ./...` | clean |
| 12 | `golangci-lint run ./internal/pdbcompat/... ./internal/otel/... ./cmd/peeringdb-plus/...` | 0 issues |
| 13 | Full suite `go test -race ./...` | all packages ok |

## Deviations from Plan

### Documentation-only clarifications

**1. [Doc clarity] Rephrased telemetry.go package comment to avoid literal `runtime.ReadMemStats` token**

- **Found during:** Task 2 grep verification
- **Issue:** The plan's verification step 8 required `grep -c 'runtime.ReadMemStats' internal/pdbcompat/telemetry.go == 1` (helper only), but the initial godoc used the literal token in an explanatory comment, producing count 2.
- **Fix:** Rewrote the comment to say "The runtime memstats read is stop-the-world (STW)…" — semantically identical, keeps the invariant strictly satisfied.
- **Files modified:** `internal/pdbcompat/telemetry.go`
- **Commit:** folded into 292e758 pre-commit (no separate commit)

### No functional deviations from D-06 / plan

- Telemetry fires on every serveList terminal path as required (200, 413, 400 filter-error, 500 query-error) — the defer pattern guarantees this by construction and the `TestRecordResponseHeapDelta_FiresOnce` test pins single-invocation per request.
- serveDetail remains untouched per D-07 list-only scope.
- Bucket boundaries, metric attributes (endpoint + entity), histogram name, span attribute name all match the plan exactly.
- No new Go module dependencies — the OTel SDK packages used were already pulled in by existing tests (`metrics_test.go`, `privacy_tier_test.go`).

## ReadMemStats call-site invariant

Grep proof of the "once per request" discipline (zero per-row calls):

```
$ grep -rn 'runtime\.ReadMemStats' internal/pdbcompat/
internal/pdbcompat/telemetry.go:35:	runtime.ReadMemStats(&ms)
```

Single call site in `memStatsHeapInuseKiB`. Invoked exactly twice per request:
1. Inline at the top of `serveList` (entry sample)
2. Inside `recordResponseHeapDelta` via the deferred call (exit sample)

No row iterators, no serialiser paths, no per-object hooks call it.

## Known Stubs

None. All wiring is live and exercised by unit tests.

## Self-Check: PASSED

- `internal/pdbcompat/telemetry.go` — FOUND
- `internal/pdbcompat/telemetry_test.go` — FOUND
- `internal/otel/metrics.go` — modified, verified
- `internal/otel/metrics_test.go` — modified, verified
- `cmd/peeringdb-plus/main.go` — modified, verified
- `internal/pdbcompat/handler.go` — modified, verified
- `deploy/grafana/dashboards/pdbplus-overview.json` — modified, JSON valid
- Commit `c2304ae` — FOUND
- Commit `292e758` — FOUND
