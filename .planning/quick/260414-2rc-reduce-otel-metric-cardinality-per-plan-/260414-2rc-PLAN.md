---
phase: quick
plan: 260414-2rc
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/otel/metrics.go
  - internal/otel/metrics_test.go
  - internal/otel/provider.go
  - internal/otel/provider_test.go
  - internal/sync/worker.go
autonomous: false
requirements:
  - QUICK-260414-2rc
must_haves:
  truths:
    - "pdbplus.sync.type.duration histogram is no longer registered and no longer recorded"
    - "http.server.request.body.size and http.server.response.body.size metrics are dropped by MeterProvider views"
    - "rpc.server.duration histogram uses explicit bucket boundaries {0.01, 0.05, 0.25, 1, 5} (5 boundaries -> 6 buckets) instead of the SDK default"
    - "Metric resource attributes omit fly.machine_id; trace and log resource attributes still include it when FLY_MACHINE_ID is set"
    - "go build, go test -race ./internal/otel/... ./internal/sync/..., and golangci-lint run all pass"
  artifacts:
    - path: "internal/otel/metrics.go"
      provides: "Custom metric instruments minus pdbplus.sync.type.duration"
      contains: "pdbplus.sync.duration"
    - path: "internal/otel/provider.go"
      provides: "MeterProvider with drop/bucket-override views and metric-specific resource"
      contains: "sdkmetric.NewView"
    - path: "internal/sync/worker.go"
      provides: "Sync worker without SyncTypeDuration.Record calls"
      contains: "pdbotel.SyncTypeFetchErrors"
  key_links:
    - from: "internal/otel/provider.go"
      to: "go.opentelemetry.io/otel/sdk/metric"
      via: "sdkmetric.WithView(sdkmetric.NewView(...)) passed to NewMeterProvider"
      pattern: "sdkmetric\\.WithView"
    - from: "internal/otel/provider.go"
      to: "buildMetricResource"
      via: "sdkmetric.WithResource(buildMetricResource(...))"
      pattern: "buildMetricResource"
---

<objective>
Reduce OpenTelemetry metric cardinality by ~30-55% to curb TSDB spend and noise, per the approved plan at `/home/dotwaffle/.claude/plans/ethereal-petting-pelican.md`. Four atomic changes, all in the `internal/otel` + `internal/sync` seam, with no new dependencies:

1. Delete the per-type `pdbplus.sync.type.duration` histogram (saves ~143 series).
2. Add MeterProvider views to drop `http.server.request.body.size` and `http.server.response.body.size` (saves ~50-100 series).
3. Add a MeterProvider view to override `rpc.server.duration` with 5 explicit bucket boundaries (saves ~250-550 series).
4. Split resource attributes: metrics get a resource without `fly.machine_id`; traces and logs keep it.

Purpose: Lower cost and noise in the metrics pipeline without sacrificing per-type sync counters or any trace/log debuggability. Aggregate `pdbplus.sync.duration` already captures overall sync latency, so per-type latency is redundant.

Output: Modified `internal/otel/metrics.go`, `internal/otel/provider.go`, `internal/sync/worker.go`, and their co-located tests. Verified with `go build`, `go test -race`, `golangci-lint run`, and a manual console-exporter smoke check.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@internal/otel/provider.go
@internal/otel/metrics.go
@internal/otel/metrics_test.go
@internal/otel/provider_test.go
@internal/sync/worker.go

<interfaces>
<!-- Key OTel SDK APIs used by this plan. Extracted from installed v1.43.0. -->
<!-- Executor should use these directly -- no Context7 lookup needed for these symbols. -->

From go.opentelemetry.io/otel/sdk/metric (v1.43.0):
```go
// Register a view on the MeterProvider.
func WithView(views ...View) Option

// Build a view from an instrument selector + stream override.
func NewView(criteria Instrument, mask Stream) View

// Selector: match by instrument Name (glob supported with "*").
type Instrument struct {
    Name        string
    Description string
    Kind        InstrumentKind
    Unit        string
    Scope       instrumentation.Scope
}

// Stream override: Aggregation=Drop drops all data points.
type Stream struct {
    Name            string
    Description     string
    Unit            string
    Aggregation     Aggregation
    AttributeFilter attribute.Filter
}

// Drop aggregator: discards all measurements for matching instruments.
type AggregationDrop struct{}

// Explicit-bucket histogram override.
type AggregationExplicitBucketHistogram struct {
    Boundaries []float64
    NoMinMax   bool
}
```

Current call sites of `SyncTypeDuration` (all three must be deleted; grep confirms):
- `internal/sync/worker.go:520` — in fetch/stage loop, after `stepSpan.End()`.
- `internal/sync/worker.go:615` — in upsert loop, after `stepSpan.End()`.
- `internal/sync/worker.go:1101` — in delete loop, inside the error branch.

Current test references to `SyncTypeDuration` (must be removed from tests):
- `internal/otel/metrics_test.go:82` — inside `TestInitMetrics_PerTypeInstruments` nil-check table.
- `internal/otel/metrics_test.go:105` — inside `TestInitMetrics_PerTypeRecordDoesNotPanic`.
- `internal/otel/metrics_test.go:171,180-183` — inside `TestInitMetrics_RecordsValues`; the `pdbplus.sync.type.duration` assertion must be replaced with a sibling metric (objects/deleted are already asserted — drop just the duration assertion).
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Delete pdbplus.sync.type.duration instrument and all call sites</name>
  <files>
    internal/otel/metrics.go
    internal/otel/metrics_test.go
    internal/sync/worker.go
  </files>
  <behavior>
    - `internal/otel/metrics.go` no longer declares `SyncTypeDuration` (var at line 20) and no longer registers `pdbplus.sync.type.duration` (block at lines 63-70).
    - `internal/sync/worker.go` no longer calls `pdbotel.SyncTypeDuration.Record(...)` at lines 520, 615, or 1101. The surrounding `stepStart := time.Now()`, `typeAttr`, and `stepSpan` setups MUST remain — they are still used by other metrics (`SyncTypeFetchErrors`, `SyncTypeFallback`, `SyncTypeUpsertErrors`, `SyncTypeObjects`, `SyncTypeDeleted`) and by the trace span annotation, respectively. Do NOT remove `stepStart` — only the `.Record()` line.
    - `internal/otel/metrics_test.go` compiles without `SyncTypeDuration` references:
      - Remove the `{"SyncTypeDuration", SyncTypeDuration}` row from the table in `TestInitMetrics_PerTypeInstruments` (line 82).
      - Remove the `SyncTypeDuration.Record(ctx, 1.5, typeAttr)` line in `TestInitMetrics_PerTypeRecordDoesNotPanic` (line 105).
      - In `TestInitMetrics_RecordsValues`, remove the `SyncTypeDuration.Record(ctx, 2.5, typeAttr)` call (line 171) and the assertion block for `pdbplus.sync.type.duration` (lines 180-183). Leave the objects/deleted assertions intact.
    - `go build ./...` and `go test -race ./internal/otel/... ./internal/sync/...` pass after the deletions.
  </behavior>
  <action>
    1. Edit `internal/otel/metrics.go`:
       - Delete lines 19-20 (`// SyncTypeDuration records per-type sync step duration in seconds.` and `var SyncTypeDuration metric.Float64Histogram`).
       - Delete the registration block at lines 63-70 (`SyncTypeDuration, err = meter.Float64Histogram(...)` through the `return fmt.Errorf("registering pdbplus.sync.type.duration histogram: %w", err)` closing brace).
       - Verify `InitMetrics` still returns the first error encountered and no dangling `err` assignment is left.

    2. Edit `internal/sync/worker.go`:
       - Line 520: delete only `pdbotel.SyncTypeDuration.Record(ctx, time.Since(stepStart).Seconds(), typeAttr)`. Keep `stepStart`, `typeAttr`, and the surrounding error-attribution calls.
       - Line 615: same — delete only the `.Record` line. Keep `stepStart` and `typeAttr`.
       - Line 1101: delete only the `.Record` line inside the `if stepErr != nil` branch. Keep the `return fmt.Errorf(...)` immediately after. `stepStart` at line 1091 is now unused at this call site; scan the function — if `stepStart` is only referenced by the deleted line, remove the `stepStart := time.Now()` declaration too. (At lines 520 and 615 it is used elsewhere; at line 1101 confirm via grep before removing.)
       - Verify `go vet ./internal/sync/...` passes — an unused `stepStart` will fail compile, which tells you whether to keep or delete it.

    3. Edit `internal/otel/metrics_test.go`:
       - Remove the table row `{"SyncTypeDuration", SyncTypeDuration},` in `TestInitMetrics_PerTypeInstruments`.
       - Remove the line `SyncTypeDuration.Record(ctx, 1.5, typeAttr)` in `TestInitMetrics_PerTypeRecordDoesNotPanic`.
       - In `TestInitMetrics_RecordsValues`: remove `SyncTypeDuration.Record(ctx, 2.5, typeAttr)` and the `found := findMetric(rm, "pdbplus.sync.type.duration")` / `if found == nil { ... }` block. Leave the `foundObjs`/`foundDel` assertions untouched.

    Per CLAUDE.md: run `gofmt` / `go vet` / `go test -race` after edits. Use `errors.Is`/`%w` idioms already present — no new error handling introduced.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go build ./... &amp;&amp; go vet ./... &amp;&amp; go test -race ./internal/otel/... ./internal/sync/...</automated>
  </verify>
  <done>
    - `grep -rn "SyncTypeDuration\|pdbplus.sync.type.duration" internal/ cmd/` returns no matches.
    - `go build ./...` passes.
    - `go vet ./...` passes.
    - `go test -race ./internal/otel/... ./internal/sync/...` passes.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Add MeterProvider views + split metric resource (omit fly.machine_id)</name>
  <files>
    internal/otel/provider.go
    internal/otel/provider_test.go
  </files>
  <behavior>
    - New helper `buildMetricResource(ctx context.Context, serviceName string) *resource.Resource` returns a resource identical to `buildResource` except it omits the `fly.machine_id` attribute. Trace and log providers continue to use `buildResource(...)` (keep `fly.machine_id`). Implement by either (a) extracting a shared internal helper that takes a `includeMachineID bool` flag and wrapping it with two public helpers, or (b) having `buildMetricResource` call `buildResource` and strip the attribute — option (a) is cleaner because `resource.Resource.Attributes()` returns a read-only set and rebuilding from a filtered set is awkward. Prefer option (a): introduce an unexported `buildResourceFiltered(ctx, serviceName, includeMachineID bool)` and have both public helpers call it.
    - `Setup` constructs the MeterProvider with three views via `sdkmetric.WithView(...)`:
      1. `sdkmetric.NewView(sdkmetric.Instrument{Name: "http.server.request.body.size"}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}})`
      2. `sdkmetric.NewView(sdkmetric.Instrument{Name: "http.server.response.body.size"}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}})`
      3. `sdkmetric.NewView(sdkmetric.Instrument{Name: "rpc.server.duration"}, sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{0.01, 0.05, 0.25, 1, 5}, NoMinMax: false}})`
    - MeterProvider receives `sdkmetric.WithResource(buildMetricResource(ctx, in.ServiceName))` — NOT the shared `res`. TracerProvider and LoggerProvider continue to use the shared `res` (keep `fly.machine_id` on traces/logs).
    - Order of `NewMeterProvider` options: `WithResource`, `WithReader`, then three `WithView(...)` calls.
    - New tests added to `internal/otel/provider_test.go`:
      1. `TestBuildMetricResource_OmitsFlyMachineID` — sets `FLY_MACHINE_ID=abc123` via `t.Setenv`, calls `buildMetricResource`, iterates attributes, asserts no key equals `fly.machine_id`. Also asserts `service.name` is present (sanity).
      2. `TestBuildResource_IncludesFlyMachineID` — sets `FLY_MACHINE_ID=abc123`, calls `buildResource`, asserts attributes DO include `fly.machine_id=abc123`. This locks in the trace/log behavior so a future refactor can't accidentally strip it everywhere.
      3. `TestSetup_MetricViewsRegistered` (optional but recommended) — with `OTEL_METRICS_EXPORTER=none`, call `Setup`, then use a `sdkmetric.ManualReader` pattern won't work because `Setup` creates its own reader. Skip this test unless a simple observable seam exists — just rely on the unit-level view-construction tests implicitly via the build check. (If time-constrained, omit this third test; tasks 1+2 still give solid coverage.)
  </behavior>
  <action>
    1. Edit `internal/otel/provider.go`:
       - Introduce `buildResourceFiltered(ctx context.Context, serviceName string, includeMachineID bool) *resource.Resource`. Move the existing body of `buildResource` into it. In the Fly env loop, skip `FLY_MACHINE_ID`/`fly.machine_id` when `includeMachineID` is false.
       - Keep the existing `buildResource` signature by making it a one-line wrapper: `return buildResourceFiltered(ctx, serviceName, true)`.
       - Add `buildMetricResource(ctx context.Context, serviceName string) *resource.Resource { return buildResourceFiltered(ctx, serviceName, false) }` with a GoDoc comment: `// buildMetricResource is like buildResource but omits fly.machine_id to prevent per-VM metric fan-out. Use for MeterProvider only; TracerProvider/LoggerProvider keep the full resource for per-VM debugging.`
       - In `Setup`, compute the metric resource once: `metricRes := buildMetricResource(ctx, in.ServiceName)`. Leave the existing `res := buildResource(ctx, in.ServiceName)` untouched — it still feeds traces and logs.
       - Replace the `sdkmetric.NewMeterProvider(...)` call (lines 66-69) with:
         ```go
         mp := sdkmetric.NewMeterProvider(
             sdkmetric.WithResource(metricRes),
             sdkmetric.WithReader(metricReader),
             sdkmetric.WithView(
                 sdkmetric.NewView(
                     sdkmetric.Instrument{Name: "http.server.request.body.size"},
                     sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
                 ),
             ),
             sdkmetric.WithView(
                 sdkmetric.NewView(
                     sdkmetric.Instrument{Name: "http.server.response.body.size"},
                     sdkmetric.Stream{Aggregation: sdkmetric.AggregationDrop{}},
                 ),
             ),
             sdkmetric.WithView(
                 sdkmetric.NewView(
                     sdkmetric.Instrument{Name: "rpc.server.duration"},
                     sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
                         Boundaries: []float64{0.01, 0.05, 0.25, 1, 5},
                         NoMinMax:   false,
                     }},
                 ),
             ),
         )
         ```
       - Leave `sdktrace.NewTracerProvider(..., sdktrace.WithResource(res), ...)` and `sdklog.NewLoggerProvider(..., sdklog.WithResource(res), ...)` untouched.

    2. Edit `internal/otel/provider_test.go` — append:
       ```go
       func TestBuildMetricResource_OmitsFlyMachineID(t *testing.T) {
           t.Setenv("FLY_MACHINE_ID", "abc123")

           ctx := t.Context()
           res := buildMetricResource(ctx, "test-service")

           for _, attr := range res.Attributes() {
               if string(attr.Key) == "fly.machine_id" {
                   t.Errorf("metric resource must not contain fly.machine_id; found %v", attr.Value.AsString())
               }
           }
       }

       func TestBuildResource_IncludesFlyMachineID(t *testing.T) {
           t.Setenv("FLY_MACHINE_ID", "abc123")

           ctx := t.Context()
           res := buildResource(ctx, "test-service")

           found := false
           for _, attr := range res.Attributes() {
               if string(attr.Key) == "fly.machine_id" && attr.Value.AsString() == "abc123" {
                   found = true
                   break
               }
           }
           if !found {
               t.Errorf("trace/log resource attributes %v must contain fly.machine_id=abc123", res.Attributes())
           }
       }
       ```

    3. Run `gofmt -w internal/otel/` and verify imports haven't changed (still only need `go.opentelemetry.io/otel/sdk/metric`, already imported as `sdkmetric`).

    Notes:
    - Do NOT introduce new imports. `sdkmetric` already aliases `go.opentelemetry.io/otel/sdk/metric` per CLAUDE.md's GO-MD-1 (stdlib/existing deps only).
    - Do NOT modify `runtime.Start(runtime.WithMeterProvider(mp))` — runtime metrics continue to use the views-applied MeterProvider, which is desired (they don't match any of the three view selectors).
    - Do NOT modify `cmd/peeringdb-plus/main.go:298-301` — the otelconnect interceptor options are already minimized per the plan's "already applied" list.
    - Per GO-CS-5: all three `sdkmetric.NewView` calls use struct literals with one/two fields — no input-struct refactor needed.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go build ./... &amp;&amp; go test -race ./internal/otel/... &amp;&amp; golangci-lint run ./internal/otel/...</automated>
  </verify>
  <done>
    - `internal/otel/provider.go` contains `buildMetricResource` and three `sdkmetric.WithView(...)` calls in `Setup`.
    - `go build ./...` passes.
    - `go test -race ./internal/otel/...` passes (including the two new tests).
    - `golangci-lint run ./internal/otel/...` passes with no new warnings (if a view constructor triggers gocritic noise, investigate before adding `//nolint` — per CLAUDE.md only suppress with a justified comment).
  </done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 3: Manual console-exporter smoke test</name>
  <what-built>
    Metric cardinality reductions: `pdbplus.sync.type.duration` deleted, HTTP body-size metrics dropped, `rpc.server.duration` histogram buckets reduced to 5 explicit boundaries, and `fly.machine_id` removed from metric resource attributes only.
  </what-built>
  <how-to-verify>
    1. Build and run locally with console exporter and short export interval:
       ```
       cd /home/dotwaffle/Code/pdb/peeringdb-plus
       go build -o /tmp/pdbplus ./cmd/peeringdb-plus
       FLY_MACHINE_ID=test-vm-001 FLY_REGION=lhr \
         OTEL_TRACES_EXPORTER=none \
         OTEL_LOGS_EXPORTER=none \
         OTEL_METRICS_EXPORTER=console \
         OTEL_METRIC_EXPORT_INTERVAL=5000 \
         PDBPLUS_DB_PATH=/tmp/pdbplus-smoke.db \
         PDBPLUS_LISTEN_ADDR=:8765 \
         PDBPLUS_SYNC_INTERVAL=24h \
         /tmp/pdbplus 2>&1 | tee /tmp/pdbplus-metrics.log
       ```
    2. In a second terminal, generate traffic:
       ```
       curl -s http://127.0.0.1:8765/ui/ > /dev/null
       curl -s http://127.0.0.1:8765/rest/v1/net?limit=1 > /dev/null
       grpcurl -plaintext -d '{"pageSize":1}' 127.0.0.1:8765 peeringdb.v1.NetworkService/List || true
       ```
    3. Wait ~10 seconds, then Ctrl-C the server and inspect `/tmp/pdbplus-metrics.log`:
       - Confirm NO occurrence of `"Name":"pdbplus.sync.type.duration"`.
       - Confirm NO occurrence of `"Name":"http.server.request.body.size"` or `"Name":"http.server.response.body.size"`.
       - Find `"Name":"rpc.server.duration"` (or similar). Count `"Bounds"` entries — should be 5 values matching `[0.01, 0.05, 0.25, 1, 5]` (6 buckets including the +Inf tail).
       - In the `"Resource"` block for metric exports, confirm NO `"Key":"fly.machine_id"` entry.
       - (Optional) If you can enable traces temporarily (`OTEL_TRACES_EXPORTER=console`) run again and confirm traces DO include `fly.machine_id` in the resource — this proves the split worked as intended.
    4. Other sync counters (objects, deleted, fetch_errors, upsert_errors, fallback) must still be present if a sync ran. If you want to exercise them, either wait for the 24h sync (no) or trigger one manually via `PDBPLUS_SYNC_TOKEN=test curl -X POST -H "Authorization: Bearer test" http://127.0.0.1:8765/admin/sync` (if that endpoint exists in this tree — otherwise skip; their preservation is covered by Task 1 being scoped to remove only `SyncTypeDuration`).
  </how-to-verify>
  <resume-signal>Type "approved" once the log confirms all four absence checks + the bucket-boundary count, or describe what you saw instead.</resume-signal>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| None added | This change only modifies telemetry plumbing. No new input, no new network egress beyond existing autoexport. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-quick-260414-2rc-01 | I (Information Disclosure) | MeterProvider resource | accept | `fly.machine_id` is a low-sensitivity Fly.io-assigned identifier; removing it from metrics (while keeping on traces/logs) reduces fan-out cost, not attack surface. No PII involved. |
| T-quick-260414-2rc-02 | D (Denial of Service) | MeterProvider views | mitigate | Views reduce metric series volume (memory + egress), strictly improving DoS posture versus the unbounded-cardinality baseline. No new code path that can be abused. |
| T-quick-260414-2rc-03 | T (Tampering) | Histogram bucket override | accept | Explicit bucket boundaries are compile-time constants; not attacker-controllable. |
</threat_model>

<verification>
1. `go build ./...` — all packages compile.
2. `go vet ./...` — passes.
3. `go test -race ./internal/otel/... ./internal/sync/...` — passes, including new resource-attribute tests.
4. `golangci-lint run` — passes with no new findings.
5. Manual console-exporter smoke test (Task 3) — confirms metric absence of dropped instruments, histogram bucket count, and resource-attribute split.
6. `grep -rn "SyncTypeDuration\|pdbplus\.sync\.type\.duration" internal/ cmd/` returns no matches (dead-reference check).
</verification>

<success_criteria>
- `pdbplus.sync.type.duration` instrument, field, and all three `.Record` call sites are gone from `internal/` and `cmd/`; tests updated accordingly.
- `Setup` in `internal/otel/provider.go` registers three `sdkmetric.NewView` entries: two `AggregationDrop{}` on HTTP body-size instruments and one `AggregationExplicitBucketHistogram{Boundaries: [0.01, 0.05, 0.25, 1, 5]}` on `rpc.server.duration`.
- `MeterProvider` uses `buildMetricResource(ctx, in.ServiceName)` (no `fly.machine_id`); `TracerProvider` and `LoggerProvider` continue to use `buildResource(ctx, in.ServiceName)` (with `fly.machine_id`).
- New unit tests in `internal/otel/provider_test.go` assert the resource-attribute split in both directions.
- All automated checks (build, vet, test -race, lint) pass.
- Human smoke check in Task 3 confirms runtime behavior of all four changes.
- No new Go module dependencies added.
</success_criteria>

<output>
After completion, create `.planning/quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/260414-2rc-SUMMARY.md` covering:
- What changed (file-by-file diff summary).
- Observed metric-series reduction from console smoke check (before/after series count estimate, or at minimum the absence confirmations).
- Any deviations from the plan and why.
- Commit SHA of the merged change.
</output>
