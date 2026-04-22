---
status: complete
phase: quick
plan: 260414-2rc
subsystem: observability
tags: [otel, metrics, cardinality, cost, views, resource]
requires: []
provides:
  - "MeterProvider with drop views on http.server.{request,response}.body.size"
  - "MeterProvider with explicit-bucket override on rpc.server.duration ({0.01, 0.05, 0.25, 1, 5})"
  - "buildMetricResource (fly.machine_id omitted); buildResource retains fly.machine_id"
affects:
  - "internal/otel/metrics.go"
  - "internal/otel/metrics_test.go"
  - "internal/otel/provider.go"
  - "internal/otel/provider_test.go"
  - "internal/sync/worker.go"
  - "internal/sync/worker_test.go"
tech-stack:
  added: []
  patterns:
    - "sdkmetric.WithView(sdkmetric.NewView(Instrument{Name}, Stream{Aggregation})) for cardinality-reduction policies"
    - "Resource split: metric resource omits per-VM attributes, trace/log resource keeps them"
key-files:
  created: []
  modified:
    - "internal/otel/metrics.go"
    - "internal/otel/metrics_test.go"
    - "internal/otel/provider.go"
    - "internal/otel/provider_test.go"
    - "internal/sync/worker.go"
    - "internal/sync/worker_test.go"
decisions:
  - "Keep aggregate pdbplus.sync.duration; delete per-type SyncTypeDuration"
  - "Drop HTTP body-size metrics entirely (low debugging value, high cardinality)"
  - "rpc.server.duration bucket boundaries: {0.01, 0.05, 0.25, 1, 5}s (6 buckets)"
  - "fly.machine_id lives on traces/logs only, not on metrics, to prevent per-VM fan-out"
  - "buildResourceFiltered is the shared impl; buildResource and buildMetricResource are one-line wrappers"
metrics:
  duration: "~20 min"
  completed: "2026-04-14"
---

# Quick Task 260414-2rc: Reduce OTel Metric Cardinality Summary

Cut metric series cost ~30-55% by deleting the redundant per-type sync duration histogram, dropping the HTTP body-size auto-instrumented metrics, shrinking rpc.server.duration to 5 explicit bucket boundaries, and stripping fly.machine_id from the metric resource only (traces and logs keep it).

## What Changed

### `internal/otel/metrics.go`

- Deleted `var SyncTypeDuration metric.Float64Histogram` declaration and its comment.
- Deleted the `meter.Float64Histogram("pdbplus.sync.type.duration", ...)` registration block (and its error return) from `InitMetrics`.
- Remaining per-type instruments (`SyncTypeObjects`, `SyncTypeDeleted`, `SyncTypeFetchErrors`, `SyncTypeUpsertErrors`, `SyncTypeFallback`) untouched.

### `internal/sync/worker.go`

- Removed three call sites of `pdbotel.SyncTypeDuration.Record(...)`:
  - `syncFetchPass` (Phase A fetch loop).
  - `syncUpsertPass` (Phase B upsert loop).
  - `syncDeletePass` (delete loop, error branch).
- Removed the paired `stepStart := time.Now()` declaration at all three sites. Grep confirmed `stepStart` was only referenced by the deleted Record calls, so retaining it would trip `go vet` (declared and not used). The `stepSpan` tracer span, `typeAttr`, and error-attribution counters (`SyncTypeFetchErrors`, `SyncTypeUpsertErrors`, `SyncTypeFallback`, `SyncTypeObjects`, `SyncTypeDeleted`) were preserved intact.

### `internal/otel/metrics_test.go`

- Removed `{"SyncTypeDuration", SyncTypeDuration}` row from `TestInitMetrics_PerTypeInstruments`.
- Removed `SyncTypeDuration.Record(ctx, 1.5, typeAttr)` from `TestInitMetrics_PerTypeRecordDoesNotPanic`.
- Removed the `SyncTypeDuration.Record(ctx, 2.5, typeAttr)` call and the `pdbplus.sync.type.duration` assertion block from `TestInitMetrics_RecordsValues` (sibling objects/deleted assertions retained).

### `internal/sync/worker_test.go`

- Removed the `findMetric(rm, "pdbplus.sync.type.duration")` lookup + fatal from `TestSyncRecordsSuccessMetrics`. Out-of-scope-of-plan but required to avoid a compile/lookup failure once the metric no longer emits (Rule 3 — blocking issue fixed inline). See deviations below.

### `internal/otel/provider.go`

- Introduced `buildResourceFiltered(ctx, serviceName, includeMachineID bool) *resource.Resource` holding the shared implementation.
- `buildResource` is now a one-liner: `return buildResourceFiltered(ctx, serviceName, true)`.
- Added `buildMetricResource(ctx, serviceName) *resource.Resource { return buildResourceFiltered(ctx, serviceName, false) }` with a GoDoc stating its intent.
- `Setup` now computes `metricRes := buildMetricResource(ctx, in.ServiceName)` and passes it to `sdkmetric.NewMeterProvider(sdkmetric.WithResource(metricRes), ...)`. `TracerProvider` and `LoggerProvider` continue to use the unfiltered `res`.
- `sdkmetric.NewMeterProvider` now receives three additional options:
  1. `sdkmetric.WithView(sdkmetric.NewView(sdkmetric.Instrument{Name: "http.server.request.body.size"}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}}))`
  2. Same pattern for `http.server.response.body.size`.
  3. `sdkmetric.WithView(sdkmetric.NewView(sdkmetric.Instrument{Name: "rpc.server.duration"}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{0.01, 0.05, 0.25, 1, 5}, NoMinMax: false}}))`

### `internal/otel/provider_test.go`

- Added `TestBuildMetricResource_OmitsFlyMachineID`: sets `FLY_MACHINE_ID=abc123`, asserts `buildMetricResource` produces a resource whose attributes do NOT contain `fly.machine_id`, and sanity-checks that `service.name=test-service` is still present.
- Added `TestBuildResource_IncludesFlyMachineID`: sets `FLY_MACHINE_ID=abc123`, asserts `buildResource` retains `fly.machine_id=abc123` (locks in the trace/log behaviour).

## Must-Have Truths — Verification

| Truth | Status |
|-------|--------|
| `pdbplus.sync.type.duration` histogram is no longer registered and no longer recorded | PASS — `grep -rn "SyncTypeDuration\|pdbplus\.sync\.type\.duration" internal/ cmd/` returns no matches |
| HTTP server body-size metrics are dropped by MeterProvider views | PASS — two `sdkmetric.NewView` entries with `AggregationDrop{}` present in `Setup` |
| `rpc.server.duration` histogram uses explicit bucket boundaries `{0.01, 0.05, 0.25, 1, 5}` (6 buckets) | PASS — `sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{0.01, 0.05, 0.25, 1, 5}, NoMinMax: false}` present |
| Metric resource attributes omit `fly.machine_id`; trace/log resource attributes include it when `FLY_MACHINE_ID` is set | PASS — `TestBuildMetricResource_OmitsFlyMachineID` + `TestBuildResource_IncludesFlyMachineID` both green |
| `go build`, `go test -race ./internal/otel/... ./internal/sync/...`, `golangci-lint run` all pass | PASS — see automated verification output below |

## Automated Verification

```
go build ./...                                                -> exit 0
go vet ./...                                                  -> exit 0
go test -race ./internal/otel/... ./internal/sync/...         -> ok otel (1.147s), ok sync (5.7-7.9s)
golangci-lint run                                             -> 0 issues
grep -rn "SyncTypeDuration|pdbplus.sync.type.duration" internal/ cmd/  -> no matches
```

## Deferred Human Verification — Task 3 Console Exporter Smoke Test

Plan Task 3 is a `checkpoint:human-verify` gate. I ran everything Claude can automate; the exporter smoke check must be run by a human on a terminal that can keep a server process alive and execute `curl`/`grpcurl` side-by-side. The exact recipe is in `260414-2rc-PLAN.md` Task 3 `<how-to-verify>`. Summary:

1. Build: `go build -o /tmp/pdbplus ./cmd/peeringdb-plus`.
2. Run with `OTEL_METRICS_EXPORTER=console OTEL_METRIC_EXPORT_INTERVAL=5000 FLY_MACHINE_ID=test-vm-001 PDBPLUS_LISTEN_ADDR=:8765 PDBPLUS_SYNC_INTERVAL=24h` and redirect stderr to `/tmp/pdbplus-metrics.log`.
3. Hit `/ui/`, `/rest/v1/net?limit=1`, and `peeringdb.v1.NetworkService/List` via grpcurl to generate traffic.
4. Inspect `/tmp/pdbplus-metrics.log` for absence of `"Name":"pdbplus.sync.type.duration"`, `"Name":"http.server.request.body.size"`, `"Name":"http.server.response.body.size"`, and `"Key":"fly.machine_id"` inside `"Resource"` blocks for metric exports. Confirm `rpc.server.duration` histogram shows `"Bounds":[0.01, 0.05, 0.25, 1, 5]` (6 buckets incl. +Inf tail).

The automated assertions above cover construction-level correctness; the smoke test is exporter-pipeline correctness at runtime. Both should succeed — flag any surprises against this SUMMARY and the approved plan.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Removed per-type duration assertion in worker_test.go**

- **Found during:** Task 1 verification (`grep -rn "SyncTypeDuration\|pdbplus\.sync\.type\.duration"` after initial edits).
- **Issue:** `internal/sync/worker_test.go:782-785` referenced `pdbplus.sync.type.duration` via `findMetric(rm, "pdbplus.sync.type.duration")` and would `t.Fatal` once the metric no longer emits. Plan `<context>` enumerated `metrics_test.go` test call sites but missed this sibling file.
- **Fix:** Removed the five-line assertion block. Sibling `pdbplus.sync.type.objects` check retained as the per-type signal.
- **Files modified:** `internal/sync/worker_test.go`
- **Commit:** `2be9d95` (folded into Task 1 commit since it was the direct consequence of the Task 1 deletion)

**2. [Rule 3 - Blocking] Deleted `stepStart := time.Now()` declarations alongside Record calls**

- **Found during:** Task 1 verification.
- **Issue:** Plan action point 2 said "At lines 520 and 615 it is used elsewhere". Grep showed `stepStart` was only referenced by the three `SyncTypeDuration.Record` call sites I was deleting. Leaving the declarations would trip `go vet` (declared-and-not-used).
- **Fix:** Removed `stepStart := time.Now()` at lines 505, 608, and 1091 (the three sites where the paired `.Record` call was deleted). `stepSpan`, `typeAttr`, and error-attribution counters preserved per plan intent.
- **Files modified:** `internal/sync/worker.go`
- **Commit:** `2be9d95`

No architectural deviations; no Rule 4 escalations.

## Authentication Gates

None — purely in-repo telemetry plumbing changes.

## Metric-Series Impact (Estimated)

Per the approved plan at `/home/dotwaffle/.claude/plans/ethereal-petting-pelican.md`:

| Change | Estimated series reduction |
|--------|----------------------------|
| Delete `pdbplus.sync.type.duration` | ~143 series (7 buckets x 13 types + counts + sum) |
| Drop `http.server.request.body.size` + `http.server.response.body.size` | ~50-100 series |
| Reduce `rpc.server.duration` buckets from SDK default (14) to 5 explicit | ~250-550 series |
| Omit `fly.machine_id` from metrics resource | Multiplicative: eliminates per-VM fan-out across all retained metrics |

Aggregate target: 30-55% reduction. Actual ratio depends on traffic mix and fleet size; the console-exporter smoke test above is the empirical confirmation.

## Commits

| Task | Type | Hash | Message |
|------|------|------|---------|
| 1 | refactor | `2be9d95` | delete `pdbplus.sync.type.duration` histogram |
| 2 | feat | `fc7e366` | add OTel metric views and split metric resource |

## Self-Check: PASSED

- `internal/otel/metrics.go` FOUND; no `SyncTypeDuration` symbol (`grep` exit 1).
- `internal/otel/metrics_test.go` FOUND; table + two test bodies updated.
- `internal/otel/provider.go` FOUND; contains `buildMetricResource`, `buildResourceFiltered`, three `sdkmetric.WithView` entries, `sdkmetric.AggregationDrop{}`, and `sdkmetric.AggregationExplicitBucketHistogram`.
- `internal/otel/provider_test.go` FOUND; contains both new resource-attribute tests.
- `internal/sync/worker.go` FOUND; no `SyncTypeDuration.Record` or bare `stepStart` at the three former sites.
- `internal/sync/worker_test.go` FOUND; no `pdbplus.sync.type.duration` reference.
- Commit `2be9d95` FOUND on `main`.
- Commit `fc7e366` FOUND on `main`.
