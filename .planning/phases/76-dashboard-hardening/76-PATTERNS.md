# Phase 76: Dashboard Hardening - Pattern Map

**Mapped:** 2026-04-27
**Files analyzed:** 2 modified files (no new files)
**Analogs found:** 3 / 3 (all in-tree)

## File Classification

| Modified File | Role | Data Flow | Closest Analog | Match Quality |
|---------------|------|-----------|----------------|---------------|
| `deploy/grafana/dashboards/pdbplus-overview.json` | config (Grafana dashboard JSON) | declarative-config | Same file (existing `$type` / `$process_group` template vars; existing panel 35 expr) | exact (in-file precedent) |
| `deploy/grafana/dashboard_test.go` | test (table-driven JSON-walk invariant) | request-response (synchronous file read + assert) | Same file: `TestDashboard_GaugesUseAggregation` (lines 409-445) and `TestDashboard_NoOrphanTemplateVars` (lines 332-377) | exact (in-file precedent) |

Both files are modifications, not creations. The cleanest analogs live inside the same file you're editing — copy that idiom verbatim.

## Pattern Assignments

### `deploy/grafana/dashboards/pdbplus-overview.json` (config, declarative-config)

#### Pattern A — `$service` template var: model on `$type` / `$process_group`

**Analog:** `deploy/grafana/dashboards/pdbplus-overview.json` lines 99-144 (the two existing query-type variables).

**Existing `$type` block** (lines 99-121) — copy this structure exactly:
```json
{
  "current": {},
  "datasource": {
    "uid": "${datasource}"
  },
  "definition": "label_values(pdbplus_sync_type_objects_total, type)",
  "hide": 0,
  "includeAll": true,
  "allValue": ".*",
  "label": "Type",
  "multi": true,
  "name": "type",
  "options": [],
  "query": {
    "query": "label_values(pdbplus_sync_type_objects_total, type)",
    "refId": "StandardVariableQuery"
  },
  "refresh": 2,
  "regex": "",
  "skipUrlSync": false,
  "sort": 1,
  "type": "query"
}
```

**Field-by-field deltas for the new `$service` block** (model on the above, but D-01 / RESEARCH.md A2 force these specific changes):

| Key | `$type` (analog) | `$service` (new) | Why differ |
|-----|------------------|------------------|------------|
| `definition` | `"label_values(pdbplus_sync_type_objects_total, type)"` | `"label_values(service_name)"` | Source label, not a per-metric label |
| `query.query` | matches `definition` | matches `definition` (`"label_values(service_name)"`) | Mirror invariant from analog |
| `includeAll` | `true` | `false` | Single-value collision-safety filter (A2) |
| `allValue` | `".*"` | OMIT | Not applicable when `includeAll: false` |
| `multi` | `true` | `false` | Single concrete service name, not aggregate |
| `name` | `"type"` | `"service"` | Variable name |
| `label` | `"Type"` | `"Service"` | UI dropdown label |
| `current` | `{}` | `{"selected": false, "text": "peeringdb-plus", "value": "peeringdb-plus"}` | Pin default per D-01 |
| `refresh`, `sort`, `hide`, `skipUrlSync`, `regex`, `options`, `datasource`, `type` | (as-is) | (as-is) | Mirror invariant — keep identical to `$type` |

**Placement:** insert as a new array element in `templating.list` between the `$datasource` block (lines 85-98) and the `$type` block (line 99). Operator UX convention is "what app?" before "what subset?" (RESEARCH.md A1, low risk if reordered).

#### Pattern B — Per-panel selector edit (panels 22, 23, 24, 25)

**Analog:** None of these four panels currently filter by `service_name`. The closest analog is panel 35's existing literal filter (Pattern C below) — copy its label-selector idiom but use `$service` interpolation.

**Mechanical edit** (per RESEARCH.md "Surgical Pattern" + Pitfall 1):

| Panel | Line | Before | After |
|-------|------|--------|-------|
| 22 Goroutines | 1370 | `"expr": "sum by(instance)(go_goroutine_count)"` | `"expr": "sum by(instance)(go_goroutine_count{service_name=\"$service\"})"` |
| 23 Heap Memory | 1444 | `"expr": "sum by(instance)(go_memory_used_bytes)"` | `"expr": "sum by(instance)(go_memory_used_bytes{service_name=\"$service\"})"` |
| 24 Allocation Rate | 1518 | `"expr": "sum by(instance)(rate(go_memory_allocated_bytes_total[$__rate_interval]))"` | `"expr": "sum by(instance)(rate(go_memory_allocated_bytes_total{service_name=\"$service\"}[$__rate_interval]))"` |
| 25 GC Goal | 1592 | `"expr": "sum by(instance)(go_memory_gc_goal_bytes)"` | `"expr": "sum by(instance)(go_memory_gc_goal_bytes{service_name=\"$service\"})"` |

**Critical:** for panel 24, the selector goes INSIDE `rate(...)`, not outside the `sum by(instance)(...)`. Putting the matcher outside `rate()` is a PromQL syntax error (RESEARCH.md Pitfall 1).

#### Pattern C — Panel 35 literal-to-template migration

**Analog:** `deploy/grafana/dashboards/pdbplus-overview.json` line 2046 (existing literal selector, the closest precedent for selector composition with multiple labels).

**Before** (line 2046):
```
"expr": "sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name=\"peeringdb-plus\", service_namespace=~\"$process_group\"})"
```

**After:**
```
"expr": "sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name=\"$service\", service_namespace=~\"$process_group\"})"
```

Single substitution: `service_name=\"peeringdb-plus\"` → `service_name=\"$service\"`. Do NOT touch the comma-adjacent `service_namespace=~\"$process_group\"` filter — both labels coexist in the selector braces (RESEARCH.md Pitfall 2).

---

### `deploy/grafana/dashboard_test.go` (test, request-response)

**Analog:** `TestDashboard_GaugesUseAggregation` (lines 409-445) — closest precedent for "walk all panels, assert each `target.Expr` contains a required substring."

**Function-signature pattern** (line 409):
```go
func TestDashboard_GaugesUseAggregation(t *testing.T) {
    t.Parallel()
    d := loadDashboard(t)

    panels := allPanels(d)
    // ... iterate, assert expr contents
}
```

**Helper-discovery pattern** (lines 56-77) — already in scope:
```go
// allPanels returns all panels including those nested inside collapsed row panels.
func allPanels(d dashboard) []panel {
    var out []panel
    for _, p := range d.Panels {
        out = append(out, p)
        out = append(out, p.Panels...)
    }
    return out
}

func loadDashboard(t *testing.T) dashboard {
    t.Helper()
    data, err := os.ReadFile(dashboardPath)
    if err != nil {
        t.Fatalf("reading dashboard JSON: %v", err)
    }
    var d dashboard
    if err := json.Unmarshal(data, &d); err != nil {
        t.Fatalf("parsing dashboard JSON: %v", err)
    }
    return d
}
```

**Reuse, don't redefine.** The new test must call `loadDashboard(t)` and `allPanels(d)` — not introduce a parallel walker. The `dashboard`/`panel`/`target` typed structs (lines 12-31) already give `target.Expr` as a string.

**Assertion-loop pattern** (from `TestDashboard_GaugesUseAggregation`, lines 428-444) — adapt for go_*-filter invariant:

The existing test iterates a table of `(title, contains, desc)` cases and walks panels by title. The new invariant is structurally simpler: it walks ALL panels (no title filter), uses a regex to detect `go_*` exprs, and asserts a literal substring is present. The error-message pattern (line 441) is the model for the new test's failure message — short title, expected substring, the offending expr.

**Regex precedent** (from `TestDashboard_NoOrphanTemplateVars`, line 371) — file-level `regexp` import already in place (line 6); boundary-aware patterns are the established convention. The new test's `\bgo_[a-z_]+` regex follows that idiom (won't false-positive on `lego_*` or `go_template`).

**Recommended new function** (paste into the file alongside `TestDashboard_GaugesUseAggregation`, idiomatic shape per RESEARCH.md):

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

The literal-substring match on `service_name="$service"` (not just `service_name`) is intentional — it catches mid-rename / partial edits as well as complete omissions.

---

## Shared Patterns

### Test-file imports
**Source:** `deploy/grafana/dashboard_test.go` lines 1-9
**Apply to:** New invariant function (no new imports needed).
```go
import (
    "encoding/json"
    "os"
    "regexp"
    "strings"
    "testing"
)
```

`regexp` and `strings` are both already imported — the new test reuses them.

### Test-file naming and structure
**Source:** `deploy/grafana/dashboard_test.go` (every test in the file)
**Apply to:** New `TestDashboard_GoMetricsFilterByService` function.

- Prefix: `TestDashboard_` (every public test in the file uses this prefix)
- Always call `t.Parallel()` first (every test in the file does this — go-guidelines GO-T-3)
- Always start body with `d := loadDashboard(t)` for any panel-walk test
- `t.Errorf` on per-panel failures (collect, don't fail-fast); use `t.Fatalf` only for setup errors. Established by `TestDashboard_GaugesUseAggregation` line 441 vs `loadDashboard` lines 69, 73.

### JSON edit hygiene (dashboard_test.go ↔ dashboard.json coupling)
**Source:** `TestDashboard_NoOrphanTemplateVars` (lines 332-377)
**Apply to:** Edit ordering of the dashboard JSON.

Per RESEARCH.md Pitfall 4: if you add `$service` to `templating.list` BEFORE updating any panel expression, `TestDashboard_NoOrphanTemplateVars` fails (variable declared, never referenced). If you update panel expressions BEFORE adding the variable, the new `TestDashboard_GoMetricsFilterByService` test passes but the runtime dashboard breaks. **Order both edits in the same commit; never partial-stage.**

## No Analog Found

None. Both files have strong in-file precedent for every change in this phase.

## Metadata

**Analog search scope:** `deploy/grafana/` (single-package phase — no need to scan further)
**Files scanned:** 3 (`pdbplus-overview.json`, `dashboard_test.go`, `alerts/pdbplus-alerts.yaml` for cross-check)
**Pattern extraction date:** 2026-04-27
