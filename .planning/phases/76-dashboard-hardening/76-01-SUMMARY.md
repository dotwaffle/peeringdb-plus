---
phase: 76
plan: 01
status: complete
completed: 2026-04-27
requirements_addressed: [OBS-03, OBS-05]
---

# Plan 76-01 Summary — Dashboard Hardening

## One-liner

OBS-03 dashboard hardening shipped clean (Wave 0 RED → Wave 1 GREEN, 5 panels filtered, `$service` template var wired, invariant test locks future regressions). OBS-05 confirmed during inline investigation — the post-bytes-canonicalisation `pdbplus_response_heap_delta_bytes_*` series flows correctly; the apparent absence was a triple-cause artifact (zero pdbcompat list traffic on prod for 5 days + an anchored-regex assumption mismatch + ambient confusion about Prom's metric-name catalog retention).

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

## OBS-05 — CONFIRMED (after inline investigation + regex fix)

### Final query result

```promql
count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1\.1[78]\..*"})
```

**Result: 13** (le-buckets × instances) — non-zero, OBS-05 acceptance criterion met. The confirmation flushed after a single synthetic `curl https://peeringdb-plus.fly.dev/api/net?limit=1`.

### What the apparent failure looked like, and why it wasn't real

First-pass live query (using the original `service_version=~"v1.17.0|v1.18.*"` regex from CONTEXT.md / ROADMAP) returned empty. Three independent causes contributed:

1. **No pdbcompat list traffic on prod for the prior 5 days.** The histogram only `Record()`s on terminal pdbcompat list paths via `defer recordResponseHeapDelta` in `internal/pdbcompat/handler.go:152`. A 24h Loki scan (`{service_name="peeringdb-plus"} |~ "/api/"` returns zero matches) confirmed nothing was hitting `/api/*`. Sync continues to run in the background, but reads against the data are entirely absent. Fleet alive: `count(go_goroutine_count{service_name="peeringdb-plus"}) = 8`.
2. **The success-criterion regex was anchored against the wrong label format.** Prom evaluates `=~` as a full-string match (`^...$`). Original `v1.17.0|v1.18.*` would only match `v1.17.0` exactly OR `v1.18.<anything>` exactly. The current prod build labels itself `v1.17.0-64-g565b762` (git-describe — `v<tag>-<commits-since-tag>-g<sha>`), which fails both branches. Corrected regex: `v1\.1[78]\..*`.
3. **The legacy `pdbplus_response_heap_delta_kib_KiB_*` series confused the diagnosis.** Both legacy and new metric names appear in `list_prometheus_metric_names` because Prom's metric-name catalog retains historical entries for the retention window (~30d default). The legacy `_kib_KiB_*` series stopped emitting around April 22 when v1.17.0 deployed (commit `0ee9f40 refactor(metrics): switch peak heap/RSS and response heap-delta to bytes` renamed the OTel instrument from `pdbplus.response.heap_delta_kib`/`KiB` → `pdbplus.response.heap_delta`/`By`). The legacy series is correctly retired — it just hangs around in the catalog as a retention carcass. Per CONTEXT.md D-02 we deliberately chose to let retention expire it naturally rather than adding a Prom drop rule.

### Synthetic-traffic remediation

Sent 6 read-only `/api/net?limit=N` requests against prod to flush the histogram. Single triggering request was enough to surface the metric; subsequent ones added bucket samples. The dashboard's response-heap-delta panels now have data to render.

### Files updated to encode the regex fix going forward

- `.planning/REQUIREMENTS.md` — OBS-05 line: regex corrected, item flipped to `[x]`
- `.planning/ROADMAP.md` — phase 76 success criterion #2 regex corrected; criterion #3 (panel-description doc requirement) explicitly marked as overridden by CONTEXT.md D-02; plan list flipped to `[x]`
- `.planning/phases/76-dashboard-hardening/CONTEXT.md` — D-02 regex corrected with parenthetical explanation
- `.planning/phases/76-dashboard-hardening/76-RESEARCH.md` — 3 occurrences corrected (D-02 reference + MCP query example + curl fallback example)
- `.planning/phases/76-dashboard-hardening/76-VALIDATION.md` — manual-verifications table regex corrected; pre-existing operator-Grafana-stack URL leak in the visual-confirmation row scrubbed (no-PII rule)
- `.planning/phases/76-dashboard-hardening/76-01-PLAN.md` — 6 occurrences corrected (must_haves.truths YAML + MCP query + curl fallback + acceptance criterion + verification block + success criterion)
- `.planning/STATE.md` — pre-existing operator-Grafana-stack URL leak in production-state paragraph scrubbed (no-PII rule)

## Out-of-scope audit (per CONTEXT.md)

`grep -cE 'go_[a-z_]+\{service_name="peeringdb-plus"' deploy/grafana/alerts/pdbplus-alerts.yaml` → **1** (the `PdbPlusFleetMachineCountLow` rule already uses literal `service_name="peeringdb-plus"`).

`git diff main -- deploy/grafana/alerts/pdbplus-alerts.yaml` → empty (alerts file byte-unchanged).

No fix needed in this phase. **Future operators forking the dashboard** with the new `$service` template variable may want to migrate the alert rules to a similar mechanism, but Prometheus alert rules don't have Grafana template variables — the migration would require operator-runtime substitution at apply time (e.g., `mimirtool rules sync` with templating). Recommend defer to a separate phase if/when the dashboard fork happens.

## Patterns established

- **`\bgo_[a-z_]+` regex idiom** for invariant-style filter tests: word-boundary anchored, lowercase + underscore character class, anchored to the leading `go_` namespace. Reusable for any future "every `<namespace>_*` metric must carry filter X" assertion.
- **Single-commit ordering for templating + selector edits** is now contract-locked by `TestDashboard_NoOrphanTemplateVars`. Any future template-var introduction MUST land its first reference in the same commit, or CI breaks.
- **Anchored-regex pitfall for prod label-format assumptions.** PromQL `=~` is full-string anchored. When writing a regex against `service_version`, always assume git-describe format (`<tag>-<commits>-g<sha>` with possible `-dirty`) — never assume bare semver. Default pattern: `v1\.<minor>\..*`.
- **Prom metric-name catalog ≠ active emission.** `list_prometheus_metric_names` returns names within the retention window, including retired/renamed ones that have stopped emitting. Always cross-check with a `count()` instant query at `now` to confirm a series is actively flowing before treating its presence in the catalog as evidence of emission.

## Deviations from plan

None on the OBS-03 path — Tasks 1 and 2 executed verbatim per plan with exact commit messages.

On OBS-05 (Task 3) — the plan's `<resume-signal>` blocked-path was hit on first-pass query, then resolved inline per user direction ("investigate the metric-flow gap inline"). The remediation added 7 file edits to encode the corrected regex assumption + 1 PII scrub on STATE.md. No code changes; all edits are in `.planning/` artifacts.

## Commits

- `c2ff758` — Wave 0 RED test
- `c2abc93` — Wave 1 GREEN JSON edits
- `d514b1c` — first-pass SUMMARY (superseded by this rewrite + remediation commit)
- (this commit) — SUMMARY rewrite + regex-assumption fixes across REQUIREMENTS / ROADMAP / CONTEXT / RESEARCH / VALIDATION / PLAN / STATE
