---
phase: 66-observability-sqlite3-tooling
plan: 02
type: execute
wave: 1
depends_on: []
files_modified:
  - deploy/grafana/dashboards/pdbplus-overview.json
autonomous: true
requirements: [OBS-05]
tags: [observability, grafana, dashboard]

must_haves:
  truths:
    - "deploy/grafana/dashboards/pdbplus-overview.json has a timeseries panel plotting pdbplus.sync.peak_heap_mib with a threshold line at 400"
    - "The same file has a timeseries panel plotting pdbplus.sync.peak_rss_mib with a threshold line at 384"
    - "The same file has a panel that breaks down either series by fly_process_group (primary vs replica) so post-Phase-65 asymmetric fleet is visible"
    - "The file is still valid JSON (jq parses cleanly)"
    - "Panel IDs are unique within the dashboard"
    - "Panels live in an appropriately named row so they do not clobber existing layout"
  artifacts:
    - path: "deploy/grafana/dashboards/pdbplus-overview.json"
      provides: "Three new panels (heap, RSS, process-group breakdown) under a new 'Sync Memory (SEED-001 watch)' row"
      contains: "pdbplus.sync.peak_heap_mib"
  key_links:
    - from: "deploy/grafana/dashboards/pdbplus-overview.json new panels"
      to: "OTel span attrs emitted by internal/sync/worker.go emitMemoryTelemetry (Plan 66-01)"
      via: "Prometheus metric names derived from span attr keys via the OTel collector's spanmetrics exporter OR histogram/gauge instruments"
      pattern: "pdbplus_sync_peak_heap_mib"
---

<objective>
Add three Grafana panels visualising the new sync peak heap + RSS telemetry from Plan 66-01. Per Phase 66 CONTEXT D-05 + D-06.

Purpose: SEED-001's dashboard-side story. Ops can see heap + RSS curves and threshold lines at a glance; post-Phase-65 asymmetric fleet gets a per-process-group breakdown panel.

Output: Single JSON diff to deploy/grafana/dashboards/pdbplus-overview.json.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/phases/66-observability-sqlite3-tooling/66-CONTEXT.md
@.planning/seeds/SEED-001-incremental-sync-evaluation.md

<interfaces>
<!-- Existing dashboard structure (deploy/grafana/dashboards/pdbplus-overview.json, 1227 lines). -->

Top-level schema (already present):
```json
{
  "uid": "pdbplus-overview",
  "title": "PeeringDB Plus - Service Overview",
  "schemaVersion": 39,
  "panels": [ ... ],
  "templating": { "list": [...] },
  "time": { "from": "now-6h", "to": "now" }
}
```

Existing rows (from grep of "title":):
- "Sync Health" (row)
- "HTTP RED Metrics" (row)
- "Per-Type Sync Detail" (row)
- "Go Runtime" (row) — already contains a "Heap Memory" panel for runtime/memstats
- "Business Metrics" (row)

New row location: INSERT between "Go Runtime" and "Business Metrics" so the SEED-001 watch sits adjacent to general runtime metrics.

Existing timeseries panel shape (from "Sync Duration (p95)" at line 224):
```json
{
  "type": "timeseries",
  "title": "...",
  "gridPos": { "h": 8, "w": 12, "x": 0, "y": <row Y> },
  "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
  "targets": [
    { "expr": "<promql>", "legendFormat": "...", "refId": "A" }
  ],
  "fieldConfig": {
    "defaults": { "unit": "mbytes", "thresholds": { ... } }
  }
}
```

Prometheus metric name derivation: OTel span attrs become Prometheus metrics via the spanmetrics connector in the collector, OR the existing autoexport Prometheus exporter exposes span attrs as histograms. The attribute `pdbplus.sync.peak_heap_mib` is exported as a Prometheus gauge under the name `pdbplus_sync_peak_heap_mib` (dots → underscores is standard OTel → Prom mapping). If the metrics aren't exposed as gauges by the existing autoexport config, fall back to the trace-exemplar query OR the Go runtime metric `go_memstats_heap_inuse_bytes / 1024 / 1024` as a secondary series on the heap panel.

Dashboard template var: `${DS_PROMETHEUS}` is the datasource placeholder already declared at the top of the file; new panels MUST use it.

Process-group label: Fly.io injects FLY_PROCESS_GROUP into container env; OTel resource attributes pick it up via `OTEL_RESOURCE_ATTRIBUTES=fly.process_group=...` if set in fly.toml. If not set, the label will be absent; the breakdown panel should still render (empty or just one series). NOTE for executor: verify whether fly.toml or cmd/peeringdb-plus/main.go wires this resource attribute. If it does NOT, the breakdown panel still ships but is deferred-but-documented — add an inline dashboard-text panel or a "Requires fly.process_group resource attr (see Phase 65)" note in the panel description.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add three new panels under a "Sync Memory (SEED-001 watch)" row in the Grafana dashboard JSON</name>
  <files>deploy/grafana/dashboards/pdbplus-overview.json</files>
  <read_first>
    - deploy/grafana/dashboards/pdbplus-overview.json lines 1-50 (top-level frame, datasource var)
    - deploy/grafana/dashboards/pdbplus-overview.json lines 863-1026 (Go Runtime row — contains "Heap Memory" panel: copy its target/fieldConfig shape verbatim for consistency)
    - deploy/grafana/dashboards/pdbplus-overview.json lines 1076-end (Business Metrics — insertion point for new row between Go Runtime and this)
    - Run `jq '.panels[] | select(.type == "row") | {title, id, "gridPos.y": .gridPos.y}' deploy/grafana/dashboards/pdbplus-overview.json` before editing to understand current row ID and Y coordinates
    - Run `jq '[.panels[].id] | max' deploy/grafana/dashboards/pdbplus-overview.json` to find next free panel ID
  </read_first>
  <acceptance_criteria>
    - `jq empty deploy/grafana/dashboards/pdbplus-overview.json` exits 0 (valid JSON)
    - `jq '[.panels[] | select(.title == "Sync Memory (SEED-001 watch)")] | length' deploy/grafana/dashboards/pdbplus-overview.json` returns 1 (new row)
    - `jq '[.panels[] | select(.title | test("Peak Heap.*MiB"))] | length' deploy/grafana/dashboards/pdbplus-overview.json` returns 1
    - `jq '[.panels[] | select(.title | test("Peak RSS.*MiB"))] | length' deploy/grafana/dashboards/pdbplus-overview.json` returns 1
    - `jq '[.panels[] | select(.title | test("Process Group"))] | length' deploy/grafana/dashboards/pdbplus-overview.json` returns 1
    - `jq -r '.panels[] | select(.title == "Peak Heap (MiB)") | .fieldConfig.defaults.thresholds.steps[] | .value' deploy/grafana/dashboards/pdbplus-overview.json` includes 400
    - `jq -r '.panels[] | select(.title == "Peak RSS (MiB)") | .fieldConfig.defaults.thresholds.steps[] | .value' deploy/grafana/dashboards/pdbplus-overview.json` includes 384
    - `jq '[.panels[].id] | unique | length' deploy/grafana/dashboards/pdbplus-overview.json` equals `jq '[.panels[].id] | length' deploy/grafana/dashboards/pdbplus-overview.json` (all IDs unique)
    - `grep -c "pdbplus_sync_peak_heap_mib\|pdbplus.sync.peak_heap_mib" deploy/grafana/dashboards/pdbplus-overview.json` returns at least 2 (heap panel + process-group panel)
    - `grep -c "pdbplus_sync_peak_rss_mib\|pdbplus.sync.peak_rss_mib" deploy/grafana/dashboards/pdbplus-overview.json` returns at least 1
  </acceptance_criteria>
  <action>
    1. Read the full dashboard JSON. Identify the highest existing panel ID and the Y coordinate of the "Business Metrics" row. The new row will insert at `y = business_metrics_y` and shift all subsequent panels down by the row height + 3 panels × 8 units. In practice Grafana re-flows if the row is collapsed, so prefer collapsed=false and let the downstream rows keep their absolute Y — this is how the existing rows are structured (verify by reading the file; if all rows share discrete Y increments, stick to that).

    2. Add a new row panel AFTER the "Go Runtime" row's last panel and BEFORE the "Business Metrics" row:

       ```json
       {
         "type": "row",
         "id": <nextID>,
         "title": "Sync Memory (SEED-001 watch)",
         "collapsed": false,
         "gridPos": { "h": 1, "w": 24, "x": 0, "y": <insertY> },
         "panels": []
       }
       ```

       Note: Grafana row panels keep children as sibling panels in the top-level `panels[]` array with a higher Y coordinate; the `"panels": []` field on the row itself is only populated when the row is collapsed=true.

    3. Add the three child panels AFTER the row, each with appropriate Y. Use a 12-wide layout (two per row stacked 8 high; third wraps to a new Y):

       **Panel A — Peak Heap (MiB):**
       ```json
       {
         "type": "timeseries",
         "id": <nextID+1>,
         "title": "Peak Heap (MiB)",
         "description": "Peak Go heap (runtime.MemStats.HeapInuse) sampled at end of each sync cycle. Threshold line at 400 MiB is the PDBPLUS_HEAP_WARN_MIB default. Sustained breach = SEED-001 trigger fired — see .planning/seeds/SEED-001-incremental-sync-evaluation.md.",
         "gridPos": { "h": 8, "w": 12, "x": 0, "y": <insertY+1> },
         "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
         "targets": [
           {
             "expr": "pdbplus_sync_peak_heap_mib",
             "legendFormat": "{{fly_region}} {{fly_process_group}}",
             "refId": "A"
           }
         ],
         "fieldConfig": {
           "defaults": {
             "unit": "mbytes",
             "thresholds": {
               "mode": "absolute",
               "steps": [
                 { "color": "green", "value": null },
                 { "color": "orange", "value": 300 },
                 { "color": "red", "value": 400 }
               ]
             },
             "custom": {
               "thresholdsStyle": { "mode": "line" }
             }
           }
         },
         "options": { "legend": { "displayMode": "table", "placement": "bottom" } }
       }
       ```

       **Panel B — Peak RSS (MiB):** Identical shape with:
       - title "Peak RSS (MiB)"
       - description mentioning "/proc/self/status VmHWM" + "Linux only"
       - expr `pdbplus_sync_peak_rss_mib`
       - thresholds orange@288 (75% of 384), red@384
       - gridPos x=12, y=<insertY+1>, w=12, h=8

       **Panel C — Peak Heap by Process Group:** Identical shape with:
       - title "Peak Heap by Process Group"
       - description "Per-process-group breakdown (primary vs replica, post-Phase-65 asymmetric fleet). Requires fly.process_group OTel resource attribute; if absent this panel shows a single aggregate series."
       - expr `avg by (fly_process_group) (pdbplus_sync_peak_heap_mib)`
       - legendFormat `{{fly_process_group}}`
       - gridPos x=0, y=<insertY+1+8>, w=24, h=8
       - Same threshold lines at 300 / 400

    4. Assign sequential IDs starting from nextID. Do NOT renumber existing panels — ID collisions break Grafana URL state (permalinks).

    5. Preserve JSON formatting: 2-space indent, same trailing-newline convention as the rest of the file. Do NOT jq-reformat the entire file — only insert. Verify with `git diff --stat` that exactly one file changed and line count grew by ~80-120.

    6. Validate: `jq empty deploy/grafana/dashboards/pdbplus-overview.json` MUST exit 0. Run before committing.

    Traceability: implements OBS-05 dashboard surface per D-05/D-06.
  </action>
  <verify>
    <automated>jq empty deploy/grafana/dashboards/pdbplus-overview.json && jq -e '[.panels[] | select(.title == "Sync Memory (SEED-001 watch)")] | length == 1' deploy/grafana/dashboards/pdbplus-overview.json && jq -e '[.panels[] | select(.title == "Peak Heap (MiB)")] | length == 1' deploy/grafana/dashboards/pdbplus-overview.json && jq -e '[.panels[] | select(.title == "Peak RSS (MiB)")] | length == 1' deploy/grafana/dashboards/pdbplus-overview.json && jq -e '[.panels[] | select(.title == "Peak Heap by Process Group")] | length == 1' deploy/grafana/dashboards/pdbplus-overview.json && jq -e '[.panels[].id] | (length == (unique | length))' deploy/grafana/dashboards/pdbplus-overview.json</automated>
  </verify>
  <done>Three new panels + one new row exist in the dashboard JSON; heap panel has a threshold at 400, RSS panel at 384; process-group breakdown panel queries avg by fly_process_group; all panel IDs unique; jq validates the file; existing panels untouched (verifiable via git diff).</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| dashboard JSON → Grafana | Checked-in config, not a runtime attack surface |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-66-05 | Tampering | Malformed JSON breaks dashboard provisioning | mitigate | jq empty check in <verify> before commit; CI loads dashboards in self-hosted setups via provisioning.yaml, failed parse surfaces immediately. |
</threat_model>

<verification>
- `jq empty deploy/grafana/dashboards/pdbplus-overview.json` exits 0
- New panels render when the dashboard is re-imported into Grafana (manual verification — not automated, documented in Plan 66-03 SUMMARY)
</verification>

<success_criteria>
- OBS-05 dashboard requirement satisfied: heap panel, RSS panel, process-group panel present with threshold lines
- No existing panel IDs collide; no existing panel titles changed
- JSON is valid and parseable by Grafana schemaVersion 39
</success_criteria>

<output>
After completion, create `.planning/phases/66-observability-sqlite3-tooling/66-02-SUMMARY.md`.
</output>
