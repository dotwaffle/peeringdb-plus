---
phase: 76
plan: 01
status: partial
completed: 2026-04-27
requirements_addressed: [OBS-03]
requirements_blocked: [OBS-05]
---

# Plan 76-01 Summary — Dashboard Hardening

## One-liner

OBS-03 dashboard hardening landed clean (Wave 0 RED → Wave 1 GREEN, 5 panels filtered, `$service` template var wired, invariant test locks future regressions). OBS-05 live confirmation **failed**: neither the canonicalised `pdbplus_response_heap_delta_bytes_*` series nor the legacy `_kib_KiB_*` series is currently flowing on prod — the metric stopped emitting ~5 days ago. Per CONTEXT.md D-02 confirm-only protocol, no auto-fix attempted; surfaced as blocker for separate investigation.

## OBS-03 — DONE

### Wave 0 RED — `c2ff758`

Added `TestDashboard_GoMetricsFilterByService` to `deploy/grafana/dashboard_test.go` (+28 lines, 1 new function):

- Reuses `loadDashboard(t)` + `allPanels(d)` (no new walkers)
- Regex `\bgo_[a-z_]+` (word-boundary anchored — won't false-positive on `lego_*` or `go_template`)
- Literal substring assertion: `service_name="$service"`
- `t.Parallel()` (GO-T-3)

Against the unmodified dashboard JSON: exit 1, exactly 5 `t.Errorf` lines (panels 22 Goroutines, 23 Heap Memory, 24 Allocation Rate, 25 GC Goal, 35 Live Heap by Instance). RED state proven.

### Wave 1 GREEN — `c2abc93`

Two-part edit, single commit (Pitfall 4 — partial-staging would trip `TestDashboard_NoOrphanTemplateVars`):

**Part A — `$service` template variable** inserted into `templating.list` between `$datasource` and `$type`:
- `definition: "label_values(service_name)"`
- `multi: false`, `includeAll: false`, `refresh: 2`, `label: "Service"`
- Default `current.value: "peeringdb-plus"`

**Part B — 5 surgical expression edits**:

| Panel | Expression (after edit) |
|-------|-------------------------|
| 22 Goroutines | `sum by(instance)(go_goroutine_count{service_name="$service"})` |
| 23 Heap Memory | `sum by(instance)(go_memory_used_bytes{service_name="$service"})` |
| 24 Allocation Rate | `sum by(instance)(rate(go_memory_allocated_bytes_total{service_name="$service"}[$__rate_interval]))` |
| 25 GC Goal | `sum by(instance)(go_memory_gc_goal_bytes{service_name="$service"})` |
| 35 Live Heap | `sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name="$service", service_namespace=~"$process_group"})` |

Pitfall 1 satisfied: panel 24's selector inside `rate(...)`. Pitfall 2 satisfied: panel 35's `service_namespace=~"$process_group"` co-filter preserved.

### Verification (post-Wave-1)

- `go test -race ./deploy/grafana/...` — exit 0
- `TestDashboard_GoMetricsFilterByService` — PASS (RED → GREEN proven)
- `TestDashboard_NoOrphanTemplateVars` — PASS (`$service` is referenced from at least one panel)
- `TestDashboard_ValidJSON` — PASS
- `TestDashboard_MetricNameReferences` — PASS
- jq inventory: 5/5 `go_*` exprs carry `service_name="$service"`; 0/5 carry stale literal `service_name="peeringdb-plus"`

## OBS-05 — BLOCKED (`blocked: OBS-05 zero — investigate`)

### Live Prom result

Query (per CONTEXT.md D-02 acceptance criterion):

```promql
count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1.17.0|v1.18.*"})
```

**Result: empty (no data)** — fails OBS-05 acceptance criterion.

### Investigation findings (read-only diagnostics, no fix attempted)

| Finding | Evidence |
|---------|----------|
| The canonicalised `pdbplus_response_heap_delta_bytes_*` series **does not exist in Prom under any service_version**. | `list_prometheus_metric_names` regex `pdbplus_response_heap.*` returns only the 3 legacy `_kib_KiB_*` names. |
| The legacy `pdbplus_response_heap_delta_kib_KiB_*` series **stopped flowing ~5 days ago**. | Range query `count(pdbplus_response_heap_delta_kib_KiB_count)` over `now-30d → now` shows last data point ~April 22; 2-day window returns empty. |
| The fleet is alive and emitting basic Go metrics. | `count(go_goroutine_count{service_name="peeringdb-plus"})` returns 8 at "now". |
| The bytes-rename instrument is registered correctly in code. | `internal/otel/metrics.go:227` `InitResponseHeapHistogram` registers `Int64Histogram("pdbplus.response.heap_delta", WithUnit("By"), ...)`. After OTel→Prom translation: `pdbplus_response_heap_delta_bytes_*`. Wired into `cmd/peeringdb-plus/main.go:110`. |
| The bytes rename commit is in the v1.17.0 tag. | `0ee9f40 refactor(metrics): switch peak heap/RSS and response heap-delta to bytes` is tagged `v1.17.0`. |
| The success-criterion regex `service_version=~"v1.17.0\|v1.18.*"` would never match prod's version label format **even if the metric were flowing**. | Live `service_version` values are Go pseudo-versions: `v0.0.0-20260426163533-634c96a07d54+dirty`, etc. No semver tags surface. The regex assumption from CONTEXT.md / ROADMAP success criterion #2 doesn't reflect prod's labelling convention. |
| No `pdbplus_pdbcompat_*` or `pdbcompat_*` metrics exist in Prom at all. | `list_prometheus_metric_names` regex `pdbplus_pdbcompat.*\|pdbcompat_.*` returns `[]`. |

### Possible root causes (for the follow-up phase to investigate)

1. **No pdbcompat list traffic on prod** — the histogram only `Record()`s on terminal pdbcompat list paths via `defer recordResponseHeapDelta` in `internal/pdbcompat/handler.go:152`. If `/api/*` is receiving zero list requests, the histogram never emits a sample. Combined with there being no pdbcompat counters in Prom either, this is the most likely root cause.
2. **Histogram instrument failed to register on prod** — `InitResponseHeapHistogram()` returns an error path that's wrapped in `fmt.Errorf` at `cmd/peeringdb-plus/main.go:110`. Worth checking startup logs for the error string.
3. **OTel Prometheus exporter is dropping the histogram** — less likely given the legacy series flowed cleanly until ~April 22, but worth ruling out.

The `service_version` regex problem (#6 above) is independent of the metric-flow problem and needs its own fix regardless.

## Out-of-scope audit (per CONTEXT.md)

`grep -cE 'go_[a-z_]+\{service_name="peeringdb-plus"' deploy/grafana/alerts/pdbplus-alerts.yaml` → **1** (the `PdbPlusFleetMachineCountLow` rule already uses literal `service_name="peeringdb-plus"`).

`git diff main -- deploy/grafana/alerts/pdbplus-alerts.yaml` → empty (alerts file byte-unchanged).

No fix needed in this phase. **Future operators forking the dashboard** with the new `$service` template variable may want to migrate the alert rules to a similar mechanism, but Prometheus alert rules don't have Grafana template variables — the migration would require operator-runtime substitution at apply time (e.g., `mimirtool rules sync` with templating). Recommend defer to a separate phase if/when the dashboard fork happens.

## Patterns established

- **`\bgo_[a-z_]+` regex idiom** for invariant-style filter tests: word-boundary anchored, lowercase + underscore character class, anchored to the leading `go_` namespace. Reusable for any future "every `<namespace>_*` metric must carry filter X" assertion.
- **Single-commit ordering for templating + selector edits** is now contract-locked by `TestDashboard_NoOrphanTemplateVars`. Any future template-var introduction MUST land its first reference in the same commit, or CI breaks.

## Deviations from plan

None on the OBS-03 path — Tasks 1 and 2 executed verbatim per plan with exact commit messages.

On OBS-05 (Task 3) — surfaced as blocker per the plan's `<resume-signal>` "blocked: OBS-05 zero — investigate" path. This is the planned outcome for empty-result, not a deviation.

## Next steps

User decision required:

1. **Open a quick-task / new phase** to investigate the OBS-05 metric-flow gap (driver: pdbcompat list traffic ground-truth + histogram registration confirmation + service_version regex correction).
2. **Update OBS-05 acceptance criterion** in CONTEXT.md / ROADMAP to use the actual prod version label format (e.g., `service_version=~"v0\.0\.0-.*"`) before re-running the confirm.
3. **Mark Phase 76 as partially complete** with OBS-05 deferred — the OBS-03 hardening work is independently shippable.
