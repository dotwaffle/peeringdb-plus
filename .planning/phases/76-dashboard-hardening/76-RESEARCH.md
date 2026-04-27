# Phase 76: Dashboard Hardening - Research

**Researched:** 2026-04-27
**Domain:** Grafana dashboard JSON editing + Go invariant testing
**Confidence:** HIGH

## Summary

Tight, mechanical phase. Five `go_*` PromQL expressions across five panels need a `service_name="$service"` filter. Four currently have no `service_name` filter at all; one (panel 35) has the literal `"peeringdb-plus"` and must be migrated to the `$service` template var for consistency. A new `$service` template variable must be added to `templating.list`, modelled on the existing `$type` and `$process_group` query variables. One new invariant test asserting "every `go_*` expression references `service_name`" fits the existing table-driven pattern in `dashboard_test.go` cleanly. OBS-05 is a pure live-query confirmation step — no file changes unless the metric is broken.

**Primary recommendation:** Treat this as one PLAN with three tasks: (1) add `$service` template var + sweep the 5 `go_*` exprs, (2) add the `go_*`-filter invariant to `dashboard_test.go`, (3) run the OBS-05 live confirmation against Grafana Cloud Prom. No architectural research is needed; the existing dashboard already encodes the patterns.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions

- **D-01 — OBS-03: both template variable + per-panel reference.** Add a global `$service` template variable to `pdbplus-overview.json` with default value `peeringdb-plus`, sourced as a Prometheus label-values query on `service_name`. Update every `go_*` PromQL query to reference `{service_name="$service"}`. Two-edge approach:
  - **Template var:** single edit point if the service name ever changes (e.g., a fork/rebrand), and lets future operators filter to a different application within the same Grafana stack via the dropdown.
  - **Per-panel reference:** explicit `{service_name="$service"}` in every panel's PromQL is grep-able from the JSON; no hidden global filter.

  The current dashboard relies on a fragile coincidence — syncthing on the local laptop scrapes `go_goroutines` (plural) while peeringdb-plus emits `go_goroutine_count` (singular), so the namespaces don't currently collide. Any future scrape target sharing metric names would silently double-count without this fix.

- **D-02 — OBS-05: confirm only.** Verify `count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1.17.0|v1.18.*"})` returns non-zero during normal pdbcompat list traffic. No code changes expected unless flow is broken. **No documentation in panel description, no Prom drop rule for the legacy `_kib_KiB_bucket` series** — user explicitly chose "confirm only" over the recommended "confirm + document". Rationale: retention will expire the legacy series naturally; documentation in panel-text would just be noise.

### Claude's Discretion

- Exact naming/wording of the new dashboard_test.go invariant function and its error message.
- Choice of `multi: false` vs `multi: true` on the `$service` template var (recommendation below: `multi: false`, single-value dropdown).
- Whether to add `regex` filter on the label_values query to scope to peeringdb-plus only (recommendation below: leave empty — operators benefit from seeing all services in the dropdown if they ever fork the dashboard).

### Deferred Ideas (OUT OF SCOPE)

- Active cleanup of the legacy `pdbplus_response_heap_delta_kib_KiB_bucket` series via Prometheus `metric_relabel_configs` drop rules — out of scope per D-02.
- Documentation in dashboard panel descriptions about the legacy metric — out of scope per D-02.
- Adding new `go_*` panels — this phase hardens existing queries, doesn't expand coverage.
- Changing the dashboard layout, panel sizing, or row organization — purely a query-correctness pass.
- Updating the alert rules in `deploy/grafana/alerts/pdbplus-alerts.yaml` to add `service_name` filters — those are alert rules, separate concern; the alerts already have `service_name="peeringdb-plus"` literal filter (verified — see Alerts Audit below). No change needed.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| OBS-03 | All `go_*` panel queries in `deploy/grafana/dashboards/pdbplus-overview.json` filter by `service_name="peeringdb-plus"` | Current state inventory below: 5 `go_*` exprs identified, 4 unfiltered, 1 already filtered with literal (must migrate to `$service`). New invariant test asserts every future `go_*` expr keeps the filter. |
| OBS-05 | `pdbplus_response_heap_delta_bytes_bucket` flowing on v1.17.0+ binary; confirm legacy `_kib_KiB_bucket` is retention noise | Live confirmation via Grafana Cloud Prom one-liner; metric is referenced at panel "Response Heap Delta" (lines 2098-2129 of dashboard JSON) — already wired correctly. No code change unless count returns zero. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **No PII in repo:** Email addresses, hosted Grafana Cloud stack URLs, tenant IDs, API keys MUST NOT appear in `deploy/grafana/dashboards/`, `deploy/grafana/alerts/`, or any committed file. Confirmation queries that need credentials must use env-var placeholders only — exactly the pattern in `deploy/grafana/alerts/README.md`.
- **GSD workflow:** Edits to `deploy/grafana/dashboards/pdbplus-overview.json` and `deploy/grafana/dashboard_test.go` must go through a GSD command flow.
- **Generated-code drift gate:** N/A — neither file is generated.

## Current State Inventory

### Every `go_*` PromQL Expression in `pdbplus-overview.json`

Exhaustive walk across both top-level and row-nested panels (`jq` walk verified). [VERIFIED: jq query against the file]

| Panel ID | Panel Title | Current Expr | Has `service_name`? | Action |
|----------|-------------|--------------|---------------------|--------|
| 22 | Goroutines | `sum by(instance)(go_goroutine_count)` | NO | Add `{service_name="$service"}` |
| 23 | Heap Memory | `sum by(instance)(go_memory_used_bytes)` | NO | Add `{service_name="$service"}` |
| 24 | Allocation Rate | `sum by(instance)(rate(go_memory_allocated_bytes_total[$__rate_interval]))` | NO | Add `{service_name="$service"}` (inside the `rate(...)` selector) |
| 25 | GC Goal | `sum by(instance)(go_memory_gc_goal_bytes)` | NO | Add `{service_name="$service"}` |
| 35 | Live Heap by Instance | `sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name="peeringdb-plus", service_namespace=~"$process_group"})` | YES (literal) | Migrate literal `"peeringdb-plus"` → `$service` interpolation |

**Total: 5 expressions, 5 panels.** No `go_*` queries exist anywhere else in the JSON; CONTEXT.md's enumerated panel list is exact and complete. [VERIFIED]

### Existing `templating.list` Shape

Three template variables already present, all using consistent shape: [VERIFIED]

1. **`$datasource`** (`type: "datasource"`, `query: "prometheus"`) — referenced from every panel's `datasource.uid` as `${datasource}`.
2. **`$type`** (`type: "query"`, `definition: "label_values(pdbplus_sync_type_objects_total, type)"`, `multi: true`, `includeAll: true`, `allValue: ".*"`).
3. **`$process_group`** (`type: "query"`, `definition: "label_values(pdbplus_sync_peak_heap_bytes, service_namespace)"`, `multi: true`, `includeAll: true`, `allValue: ".*"`).

The new `$service` variable should follow this exact shape. **Recommendation:** model on `$type` / `$process_group` but with `multi: false` and `includeAll: false`, since collision-safety wants a single concrete service name, not an aggregate. The default value should be the literal `"peeringdb-plus"` set in `current.value` / `current.text`.

### Recommended `$service` Template Variable JSON

```json
{
  "current": {
    "selected": false,
    "text": "peeringdb-plus",
    "value": "peeringdb-plus"
  },
  "datasource": {
    "uid": "${datasource}"
  },
  "definition": "label_values(service_name)",
  "hide": 0,
  "includeAll": false,
  "label": "Service",
  "multi": false,
  "name": "service",
  "options": [],
  "query": {
    "query": "label_values(service_name)",
    "refId": "StandardVariableQuery"
  },
  "refresh": 2,
  "regex": "",
  "skipUrlSync": false,
  "sort": 1,
  "type": "query"
}
```

**Placement:** Add as the FIRST query-type variable (after `$datasource`, before `$type`). Operators expect "what application?" before "what subset of data?" in the dropdown order. [ASSUMED — Grafana UX convention; not load-bearing]

`refresh: 2` = "On Time Range Change" — matches the other query variables. [CITED: Grafana docs — `refresh` enum values]

### Existing `dashboard_test.go` Invariants

Pattern is table-driven assertions over a typed dashboard struct (`loadDashboard(t)` helper). The test that proves the strongest precedent for the new invariant is `TestDashboard_GaugesUseAggregation` (lines 409-445) — it iterates panels by title and asserts each target's `Expr` contains a required substring. [VERIFIED: read `dashboard_test.go`]

**Other relevant invariants already present:**
- `TestDashboard_NoOrphanTemplateVars` (Phase 74 D-02) — walks the raw JSON tree as `any` and matches every `$name` / `${name}` reference with a regex. The new `$service` variable will trip this immediately if the per-panel expression sweep misses any `go_*` query — useful failsafe.
- `TestDashboard_MetricNameReferences` — asserts each named metric (including `go_goroutine_count`, `go_memory_used_bytes`, etc.) appears at least once. The new sweep doesn't break this because the metric NAMES are unchanged.

### New Invariant: Recommended Shape

```go
// TestDashboard_GoMetricsFilterByService asserts that every PromQL expression
// referencing a go_* metric carries a {service_name="$service"} filter. Phase 76
// OBS-03 — collision-safety against shared Prometheus tenants where another
// scrape target may emit go_* metrics with overlapping names.
func TestDashboard_GoMetricsFilterByService(t *testing.T) {
    t.Parallel()
    d := loadDashboard(t)

    goMetricRe := regexp.MustCompile(`\bgo_[a-z_]+`)
    for _, p := range allPanels(d) {
        for _, tgt := range p.Targets {
            if !goMetricRe.MatchString(tgt.Expr) {
                continue
            }
            if !strings.Contains(tgt.Expr, `service_name="$service"`) {
                t.Errorf("panel %q target references a go_* metric but lacks "+
                    `service_name="$service" filter (Phase 76 OBS-03): %s`,
                    p.Title, tgt.Expr)
            }
        }
    }
}
```

The match is intentionally on the literal string `service_name="$service"` (not just `service_name`) so that mid-rename or partial edits surface as failures. The regex `\bgo_[a-z_]+` matches `go_goroutine_count`, `go_memory_used_bytes`, etc., but won't false-positive on substrings like `lego_*` or `go_template`. [VERIFIED via mental parse against the 5 known expressions]

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Walk panel tree | Custom recursion | Existing `allPanels(d)` helper | Already handles row-nested panels; consistency with sibling tests |
| Match variable references | Regex on string | (For new test, regex is fine — but reuse the boundary-aware pattern from `TestDashboard_NoOrphanTemplateVars` if needed for variable-presence check) | Substring match is sound for fixed strings like `service_name="$service"` |
| Live Prom query | Custom HTTP client | `mimirtool` if available, or existing Grafana Cloud MCP `query_prometheus` tool | The grafana-cloud MCP server is wired in this session — use it directly for OBS-05 confirmation |

## Common Pitfalls

### Pitfall 1: Allocation Rate uses `rate()` — selector goes inside

The `Allocation Rate` panel's expr is `sum by(instance)(rate(go_memory_allocated_bytes_total[$__rate_interval]))`. The `service_name="$service"` filter must be applied to the metric selector INSIDE the `rate()` call, not outside the `sum by(instance)(...)`. Correct result: `sum by(instance)(rate(go_memory_allocated_bytes_total{service_name="$service"}[$__rate_interval]))`. Putting the matcher outside `rate()` is a syntax error in PromQL. [CITED: Prometheus docs — `rate()` requires a range vector]

### Pitfall 2: Panel 35 already has `service_namespace=~"$process_group"` filter

When migrating panel 35 from `service_name="peeringdb-plus"` to `service_name="$service"`, do NOT touch the existing `service_namespace=~"$process_group"` filter — they coexist as a comma-separated label list inside the selector braces. Correct result: `sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name="$service", service_namespace=~"$process_group"})`.

### Pitfall 3: Grafana variable interpolation in label values

When Grafana interpolates `$service` inside a PromQL string label match, it substitutes the bare value (no quoting). The PromQL must read `service_name="$service"` (with the quotes already in the dashboard JSON), and Grafana renders that as `service_name="peeringdb-plus"` at query time. Do NOT write `service_name=$service` (no quotes) — that produces invalid PromQL after interpolation. The single-value `multi: false` choice means there's no risk of multi-value `(a|b|c)` regex injection that would require the `=~` operator. [CITED: Grafana docs — variable interpolation in queries]

### Pitfall 4: `TestDashboard_NoOrphanTemplateVars` will fail mid-edit

If you add the `$service` template variable BEFORE updating any panel expressions, `TestDashboard_NoOrphanTemplateVars` will fail (variable declared but no panel references it). If you update panel expressions BEFORE adding the variable, the new test will pass but the dashboard at runtime will show no data (Grafana fails the interpolation silently). Order the edits so both go in the same commit; never partial-stage.

## Code Examples

### Live OBS-05 Confirmation Query (via grafana-cloud MCP)

The grafana-cloud MCP server is available in-session. The OBS-05 confirmation is one PromQL query against the production datasource:

```
query_prometheus(
  datasource_uid: <prom datasource uid>,
  expr: 'count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1.17.0|v1.18.*"})',
  query_type: "instant"
)
```

Acceptance: returns non-zero during normal pdbcompat list traffic. The metric is histogram-bucket-shaped, so a single instant query against `*_bucket` returns the per-`le`-bucket point count — any non-zero value across any bucket confirms the flow.

If the MCP server is unavailable, fall back to a curl one-liner — but credentials must come from env vars per the alerts/README.md "Forbidden content" policy:

```bash
export GRAFANA_CLOUD_PROM_URL="$YOUR_PROM_URL"
export GRAFANA_CLOUD_API_KEY="$YOUR_API_KEY"

curl -sG "$GRAFANA_CLOUD_PROM_URL/api/v1/query" \
  -H "Authorization: Bearer $GRAFANA_CLOUD_API_KEY" \
  --data-urlencode 'query=count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1.17.0|v1.18.*"})' \
  | jq '.data.result[0].value[1]'
```

### Dashboard JSON Edit — Surgical Pattern

For the four currently-unfiltered panels (22, 23, 24, 25), the edit is mechanical: insert `{service_name="$service"}` immediately after the metric name. For panel 35, the edit is `s/service_name="peeringdb-plus"/service_name="$service"/`. Use `Edit` tool surgical replacements rather than rewriting the whole 2178-line JSON.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `process_runtime_go_*` metric naming | `go_*` (semconv v1.36.0 `goconv`) | post-260426-lod | Already migrated; this phase is hardening the existing names |
| `service_name="peeringdb-plus"` literal in expressions | `service_name="$service"` template-var interpolation | This phase (OBS-03) | Single edit point if service name ever changes; operator can flip dropdown |

## Alerts Audit (Out-of-scope follow-up check)

Per CONTEXT.md "out of scope but check": `deploy/grafana/alerts/pdbplus-alerts.yaml` has TWO `go_*` references on the same selector pair (lines 51-52 and 62 of the YAML), both already filtered with the literal `service_name="peeringdb-plus"`. The alert is `PdbPlusFleetMachineCountLow`:

```yaml
expr: |
  count(
    count by (service_namespace, cloud_region) (
      go_memory_used_bytes{service_name="peeringdb-plus"}
    )
  ) < 6
```

**No action required** — the alert is already collision-safe via the literal filter. CONTEXT.md explicitly defers any alerts-file change to a follow-up. Migrating alerts to `$service` doesn't apply (Prometheus alert rules don't have Grafana template variables).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Placement of `$service` as the FIRST query-type variable matches operator UX expectation | Recommended `$service` Template Variable JSON | LOW — purely cosmetic; if wrong, dropdown order shifts but functionality identical |
| A2 | `multi: false` is the right choice (single-value dropdown) for the collision-safety filter | Recommended `$service` Template Variable JSON | LOW — `multi: true` would require `=~` operator and `(peeringdb-plus)` regex form; current panels expect `=` exact match |
| A3 | The grafana-cloud MCP server is available in this session for the OBS-05 live confirmation | Code Examples | NONE — fallback curl one-liner provided |
| A4 | `regex: ""` (no filter) on `label_values(service_name)` is desirable so operators see all services if they fork the dashboard | Claude's Discretion | LOW — if operators want it scoped, they can add `regex: "peeringdb-plus"` later as a 1-line edit |

## Open Questions

None — phase scope is closed by CONTEXT.md.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `jq` | Verification command in CONTEXT.md | ✓ | confirmed via earlier run | — |
| `go test` (Go 1.26+) | `go test ./deploy/grafana/...` | ✓ | per `go.mod` | — |
| grafana-cloud MCP server | OBS-05 live confirmation | ✓ | per session init | curl + Grafana Cloud Prom API key (env vars) |
| `mimirtool` | Optional alerts lint | unknown | — | promtool, or skip (alerts file not edited) |

**Missing dependencies with no fallback:** None.

**Missing dependencies with fallback:** `mimirtool` lint is optional — alerts file is not being edited in this phase per CONTEXT.md.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` |
| Config file | none (Go convention) |
| Quick run command | `go test -race ./deploy/grafana/...` |
| Full suite command | `go test -race ./...` |

### Phase Requirements → Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| OBS-03 | Every `go_*` panel expr filters by `service_name="$service"` | unit (JSON-walk invariant) | `go test -race -run TestDashboard_GoMetricsFilterByService ./deploy/grafana/...` | New test — Wave 0 add to existing `dashboard_test.go` |
| OBS-03 | `$service` template var declared and referenced (orphan-free) | unit (existing) | `go test -race -run TestDashboard_NoOrphanTemplateVars ./deploy/grafana/...` | ✓ exists |
| OBS-03 | Dashboard JSON parses | unit (existing) | `go test -race -run TestDashboard_ValidJSON ./deploy/grafana/...` | ✓ exists |
| OBS-05 | `pdbplus_response_heap_delta_bytes_bucket` flowing | manual (live Prom query) | `query_prometheus(...)` via MCP, or curl one-liner | Manual — automated CI confirmation impossible (production-data-dependent) |

### Sampling Rate
- **Per task commit:** `go test -race ./deploy/grafana/...` (~1s)
- **Per wave merge:** `go test -race ./deploy/grafana/...` (same — only one package affected)
- **Phase gate:** `go test -race ./...` clean + OBS-05 live query returns non-zero count

### Wave 0 Gaps
- [ ] `deploy/grafana/dashboard_test.go` — add `TestDashboard_GoMetricsFilterByService` invariant. No new file, no shared fixtures, no framework install needed.

## Security Domain

> Required because `security_enforcement` is enabled by default.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | — (no auth surface changed) |
| V3 Session Management | no | — |
| V4 Access Control | no | — (dashboard is operator-internal) |
| V5 Input Validation | yes (low) | Grafana variable interpolation is the validator — `multi: false` + label-values source means no user-supplied free-text reaches PromQL |
| V6 Cryptography | no | — |

### Known Threat Patterns for Dashboard JSON

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Tenant credentials committed to repo | Information Disclosure | `deploy/grafana/alerts/README.md` "Forbidden content" policy — env vars only at apply time |
| PromQL injection via variable interpolation | Tampering | `multi: false` + Prometheus `label_values()` source restricts `$service` to known label values; no free-text user input |
| Cross-tenant metric collision (the actual driver of this phase) | Information Disclosure (display tampering) | OBS-03 filter sweep — `service_name="$service"` ensures one application's panels can't be polluted by another |

## Sources

### Primary (HIGH confidence)
- `deploy/grafana/dashboards/pdbplus-overview.json` — read directly; 5 `go_*` expressions enumerated via `jq` walk.
- `deploy/grafana/dashboard_test.go` — read in full; existing patterns identified (`TestDashboard_GaugesUseAggregation`, `TestDashboard_NoOrphanTemplateVars`).
- `deploy/grafana/alerts/pdbplus-alerts.yaml` — read in full; alerts already use literal `service_name="peeringdb-plus"`.
- `deploy/grafana/alerts/README.md` — credential-handling convention (env vars only).
- `.planning/phases/76-dashboard-hardening/CONTEXT.md` — locked decisions D-01, D-02.
- `.planning/REQUIREMENTS.md` — OBS-03 / OBS-05 acceptance criteria.

### Secondary (MEDIUM confidence)
- Grafana docs (templating variables, `refresh` enum, label_values query) — referenced from training; not fetched live this session because the existing `$type` / `$process_group` variables are a reliable mirror.

### Tertiary (LOW confidence)
- None.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — no new tooling; pure JSON edit + existing Go test pattern.
- Architecture: HIGH — the 5-panel inventory is exhaustive (jq walk verified) and CONTEXT.md's enumerated panel list matches.
- Pitfalls: HIGH — the four pitfalls are mechanical (PromQL syntax, edit ordering); not speculative.

**Research date:** 2026-04-27
**Valid until:** 2026-05-27 (Grafana JSON schema is stable; the dashboard file changes only via planned edits)
