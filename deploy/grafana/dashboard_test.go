package grafana_test

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// dashboard is a minimal representation of a Grafana dashboard JSON.
type dashboard struct {
	Title      string     `json:"title"`
	Panels     []panel    `json:"panels"`
	Templating templating `json:"templating"`
}

type panel struct {
	ID          int      `json:"id"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Targets     []target `json:"targets"`
	Options     *options `json:"options,omitempty"`
	Panels      []panel  `json:"panels,omitempty"`
}

type target struct {
	Expr         string     `json:"expr"`
	Datasource   datasource `json:"datasource"`
	LegendFormat string     `json:"legendFormat"`
}

type datasource struct {
	Type string `json:"type"`
	UID  string `json:"uid"`
}

type options struct {
	Mode    string `json:"mode"`
	Content string `json:"content"`
}

type templating struct {
	List []templateVar `json:"list"`
}

type templateVar struct {
	Name  string          `json:"name"`
	Type  string          `json:"type"`
	Query json.RawMessage `json:"query"`
}

const dashboardPath = "dashboards/pdbplus-overview.json"

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

func TestDashboard_ValidJSON(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("reading dashboard JSON: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("dashboard JSON is not valid: %v", err)
	}
}

func TestDashboard_HasRequiredRows(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	requiredRows := []string{
		"Sync Health",
		"HTTP RED Metrics",
		"Per-Type Sync Detail",
		"Go Runtime",
		"Business Metrics",
	}

	rowTitles := make(map[string]bool)
	for _, p := range d.Panels {
		if p.Type == "row" {
			rowTitles[p.Title] = true
		}
	}

	for _, required := range requiredRows {
		if !rowTitles[required] {
			t.Errorf("missing required row %q; found rows: %v", required, rowTitles)
		}
	}
}

func TestDashboard_DatasourceTemplateVariable(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	found := false
	for _, v := range d.Templating.List {
		if v.Name == "datasource" && v.Type == "datasource" && string(v.Query) == `"prometheus"` {
			found = true
			break
		}
	}
	if !found {
		t.Error("dashboard missing datasource template variable")
	}
}

func TestDashboard_NoHardcodedDatasourceUIDs(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	for _, p := range allPanels(d) {
		for _, tgt := range p.Targets {
			if tgt.Datasource.UID != "" && tgt.Datasource.UID != "${datasource}" {
				t.Errorf("panel %q target has hardcoded datasource UID %q (want ${datasource})",
					p.Title, tgt.Datasource.UID)
			}
		}
	}
}

// TestDashboard_EachMetricPanelHasDescription enforces that every
// metric panel (timeseries, stat, bargauge, gauge) carries a
// non-empty description field. Documentation moved out of standalone
// "Guide" text panels into panel-level descriptions during the
// dashboard cleanup (commit a3a3acf) — operators get help via the
// (?) hover tooltip rather than guide-row screen real-estate.
func TestDashboard_EachMetricPanelHasDescription(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	metricPanelTypes := map[string]bool{
		"timeseries": true,
		"stat":       true,
		"bargauge":   true,
		"gauge":      true,
	}

	for _, p := range allPanels(d) {
		if !metricPanelTypes[p.Type] {
			continue
		}
		if strings.TrimSpace(p.Description) == "" {
			t.Errorf("panel id=%d title=%q (type=%s) has empty description",
				p.ID, p.Title, p.Type)
		}
	}
}

func TestDashboard_MetricNameReferences(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	// Collect all PromQL expressions from the dashboard, including nested panels.
	var exprs []string
	for _, p := range allPanels(d) {
		for _, tgt := range p.Targets {
			if tgt.Expr != "" {
				exprs = append(exprs, tgt.Expr)
			}
		}
	}

	allExprs := strings.Join(exprs, " ")

	// OTel metrics are exported to Prometheus with dots converted to underscores
	// and appropriate suffixes added. Go runtime metrics use the new OTel naming
	// convention (go_* instead of process_runtime_go_*).
	requiredMetrics := []struct {
		prometheusName string
		description    string
	}{
		{"pdbplus_sync_freshness_seconds", "sync freshness gauge"},
		{"pdbplus_sync_duration_seconds", "sync duration histogram"},
		{"pdbplus_sync_operations_total", "sync operations counter"},
		{"pdbplus_sync_type_objects_total", "per-type object count (also drives Sync Throughput)"},
		{"pdbplus_response_heap_delta_bytes", "per-request pdbcompat heap delta histogram"},
		{"pdbplus_sync_type_deleted_total", "per-type delete count"},
		{"pdbplus_sync_type_fetch_errors_total", "per-type fetch errors"},
		{"pdbplus_sync_type_upsert_errors_total", "per-type upsert errors"},
		{"pdbplus_sync_type_fallback_total", "per-type fallback events"},
		{"http_server_request_duration_seconds", "HTTP request duration"},
		{"http_server_active_requests", "HTTP active requests"},
		{"go_goroutine_count", "Go goroutines"},
		{"go_memory_used_bytes", "Go heap memory"},
		{"go_memory_gc_goal_bytes", "Go GC goal"},
		{"go_memory_allocated_bytes_total", "Go allocation rate"},
		{"pdbplus_data_type_count", "business metrics object count"},
		{"pdbplus_role_transitions_total", "role transition events"},
	}

	for _, m := range requiredMetrics {
		if !strings.Contains(allExprs, m.prometheusName) {
			t.Errorf("dashboard missing metric %q (%s) in PromQL expressions",
				m.prometheusName, m.description)
		}
	}
}

func TestDashboard_FreshnessGaugeThresholds(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("reading dashboard JSON: %v", err)
	}

	// Verify the freshness stat panel has green/yellow/red thresholds at 0/3600/7200.
	type thresholdStep struct {
		Color string   `json:"color"`
		Value *float64 `json:"value"`
	}
	type thresholds struct {
		Steps []thresholdStep `json:"steps"`
	}
	type fieldConfig struct {
		Defaults struct {
			Thresholds thresholds `json:"thresholds"`
		} `json:"defaults"`
	}
	type fullPanel struct {
		ID          int         `json:"id"`
		Title       string      `json:"title"`
		Type        string      `json:"type"`
		FieldConfig fieldConfig `json:"fieldConfig"`
		Panels      []fullPanel `json:"panels,omitempty"`
	}
	type fullDashboard struct {
		Panels []fullPanel `json:"panels"`
	}

	var fd fullDashboard
	if err := json.Unmarshal(data, &fd); err != nil {
		t.Fatalf("parsing dashboard JSON: %v", err)
	}

	// Collect all panels including nested ones from collapsed rows.
	var all []fullPanel
	for _, p := range fd.Panels {
		all = append(all, p)
		all = append(all, p.Panels...)
	}

	for _, p := range all {
		if p.Title != "Data Freshness" || p.Type != "stat" {
			continue
		}

		steps := p.FieldConfig.Defaults.Thresholds.Steps
		if len(steps) < 3 {
			t.Fatalf("Data Freshness stat panel has %d threshold steps, want at least 3", len(steps))
		}

		// Step 0: green (null value = base)
		if steps[0].Color != "green" {
			t.Errorf("step 0 color = %q, want green", steps[0].Color)
		}
		// Step 1: yellow at 3600 (1 hour)
		if steps[1].Color != "yellow" || steps[1].Value == nil || *steps[1].Value != 3600 {
			t.Errorf("step 1: color=%q value=%v, want yellow/3600", steps[1].Color, steps[1].Value)
		}
		// Step 2: red at 7200 (2 hours)
		if steps[2].Color != "red" || steps[2].Value == nil || *steps[2].Value != 7200 {
			t.Errorf("step 2: color=%q value=%v, want red/7200", steps[2].Color, steps[2].Value)
		}
		return
	}
	t.Error("Data Freshness stat panel not found")
}

// TestDashboard_NoOrphanTemplateVars asserts that every template variable
// declared in templating.list is referenced somewhere in the dashboard JSON.
//
// Phase 74 D-02 — replaces the prior TestDashboard_RegionVariableUsed which
// checked only the cloud_region Prometheus label and silently passed even
// after the $region template variable became dead UI cruft. This positive
// structural invariant catches the whole class of orphan-template-var
// accumulation.
//
// Implementation note: the haystack is built by walking the raw JSON tree
// (map[string]any) and concatenating every string leaf. This catches every
// surface where a variable is valid — Expr, Datasource.UID, LegendFormat,
// panel Title/Description, alert annotations, links, options.content, etc.
// — without per-field plumbing that would inevitably drift behind Grafana
// schema additions (the prior typed-struct walk missed five surfaces).
func TestDashboard_NoOrphanTemplateVars(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	if len(d.Templating.List) == 0 {
		t.Fatal("templating.list is empty — dashboard has no template variables to validate")
	}

	// Re-parse the dashboard JSON as an untyped tree so the haystack covers
	// every string value, not just the fields modelled in the typed structs.
	raw, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("reading dashboard JSON: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("re-parsing dashboard JSON: %v", err)
	}

	// Collect every string leaf in the dashboard JSON tree, except the
	// templating.list subtree itself — a variable referencing its own
	// definition (in `query`, `current.text`, etc.) does not count as a
	// panel-side reference and would defeat the orphan check.
	var hay strings.Builder
	collectStringLeaves(tree, "", &hay)
	haystack := hay.String()

	for _, v := range d.Templating.List {
		// Variable references in dashboard JSON: either $name or ${name} (with
		// optional format suffix such as ${name:csv}). The match must be
		// boundary-aware: a future variable named "type" must not be falsely
		// flagged as referenced by a string that contains "$type_total" or
		// "${type_count}". Substring matching is unsound.
		//
		// Pattern: literal "$", then either
		//   - "{" + name + ("}" or ":")  — brace form, possibly formatted
		//   - name + (non-identifier byte or end-of-string) — bare form
		// Identifier continuation chars per Grafana variable naming: [A-Za-z0-9_].
		name := regexp.QuoteMeta(v.Name)
		pat := regexp.MustCompile(`\$(?:\{` + name + `[}:]|` + name + `(?:[^A-Za-z0-9_]|$))`)

		if !pat.MatchString(haystack) {
			t.Errorf("template variable %q (type=%q) is declared but no dashboard string references it (looked for $%s / ${%s} with boundary across all JSON string leaves) — orphan UI cruft per Phase 74 D-02", v.Name, v.Type, v.Name, v.Name)
		}
	}
}

// collectStringLeaves walks the JSON tree v and writes every string leaf to
// out (one per line). The path argument carries the dotted JSON path of the
// current node and is used to skip the templating.list subtree — a variable
// definition naturally contains its own name in `query`, `current.text`,
// etc., and counting those as references would defeat the orphan check.
func collectStringLeaves(v any, path string, out *strings.Builder) {
	if strings.HasPrefix(path, "templating.list") {
		return
	}
	switch t := v.(type) {
	case string:
		out.WriteString(t)
		out.WriteByte('\n')
	case map[string]any:
		for k, child := range t {
			next := k
			if path != "" {
				next = path + "." + k
			}
			collectStringLeaves(child, next, out)
		}
	case []any:
		for _, child := range t {
			// Index suffix is omitted from path; templating.list[i] still
			// matches the prefix check above via "templating.list".
			collectStringLeaves(child, path, out)
		}
	}
}

func TestDashboard_GaugesUseAggregation(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	panels := allPanels(d)

	cases := []struct {
		title    string
		contains string
		desc     string
	}{
		{"Data Freshness", "max(", "should use max() for worst-case freshness"},
		{"Total Objects", "max by", "should use max by(type) to deduplicate replicas"},
		{"Goroutines", "sum by(instance)", "should show per-instance lines"},
		{"Heap Memory", "sum by(instance)", "should show per-instance lines"},
		{"Allocation Rate", "sum by(instance)", "should show per-instance lines"},
		{"GC Goal", "sum by(instance)", "should show per-instance lines"},
	}

	for _, tc := range cases {
		for _, p := range panels {
			if p.Title != tc.title {
				continue
			}
			found := false
			for _, tgt := range p.Targets {
				if strings.Contains(tgt.Expr, tc.contains) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("panel %q: %s (expected %q in PromQL)", tc.title, tc.desc, tc.contains)
			}
		}
	}
}

func TestDashboard_ProvisioningYAMLExists(t *testing.T) {
	t.Parallel()
	_, err := os.ReadFile("provisioning/dashboards.yaml")
	if err != nil {
		t.Fatalf("provisioning YAML not found: %v", err)
	}
}

// TestDashboard_GoMetricsFilterByService asserts that every PromQL expression
// referencing a go_* metric carries a {service_name="$service"} filter. Phase 76
// OBS-03 (per D-01) — collision-safety against shared Prometheus tenants where
// another scrape target may emit go_* metrics with overlapping names.
//
// The literal substring match on `service_name="$service"` (not just
// `service_name`) is intentional: it catches mid-rename / partial edits as well
// as complete omissions. The `\bgo_[a-z_]+` regex won't false-positive on
// substrings like `lego_*` or `go_template`.
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
