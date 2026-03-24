package grafana_test

import (
	"encoding/json"
	"os"
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
	ID      int      `json:"id"`
	Type    string   `json:"type"`
	Title   string   `json:"title"`
	Targets []target `json:"targets"`
	Options *options `json:"options,omitempty"`
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
	Name  string `json:"name"`
	Type  string `json:"type"`
	Query string `json:"query"`
}

const dashboardPath = "dashboards/peeringdb-plus.json"

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
		if v.Name == "DS_PROMETHEUS" && v.Type == "datasource" && v.Query == "prometheus" {
			found = true
			break
		}
	}
	if !found {
		t.Error("dashboard missing DS_PROMETHEUS datasource template variable")
	}
}

func TestDashboard_NoHardcodedDatasourceUIDs(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	for _, p := range d.Panels {
		for _, tgt := range p.Targets {
			if tgt.Datasource.UID != "" && tgt.Datasource.UID != "${DS_PROMETHEUS}" {
				t.Errorf("panel %q target has hardcoded datasource UID %q (want ${DS_PROMETHEUS})",
					p.Title, tgt.Datasource.UID)
			}
		}
	}
}

func TestDashboard_EachRowHasTextPanel(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	// Track which rows have doc panels by checking text panels following rows.
	var currentRow string
	rowHasDoc := make(map[string]bool)

	for _, p := range d.Panels {
		if p.Type == "row" {
			currentRow = p.Title
			continue
		}
		if p.Type == "text" && currentRow != "" {
			rowHasDoc[currentRow] = true
		}
	}

	requiredRows := []string{
		"Sync Health",
		"HTTP RED Metrics",
		"Per-Type Sync Detail",
		"Go Runtime",
		"Business Metrics",
	}

	for _, row := range requiredRows {
		if !rowHasDoc[row] {
			t.Errorf("row %q has no documentation text panel", row)
		}
	}
}

func TestDashboard_MetricNameReferences(t *testing.T) {
	t.Parallel()
	d := loadDashboard(t)

	// Collect all PromQL expressions from the dashboard.
	var exprs []string
	for _, p := range d.Panels {
		for _, tgt := range p.Targets {
			if tgt.Expr != "" {
				exprs = append(exprs, tgt.Expr)
			}
		}
	}

	allExprs := strings.Join(exprs, " ")

	// OTel metrics are exported to Prometheus with dots converted to underscores
	// and appropriate suffixes added.
	requiredMetrics := []struct {
		prometheusName string
		description    string
	}{
		{"pdbplus_sync_freshness_seconds", "sync freshness gauge"},
		{"pdbplus_sync_duration_seconds", "sync duration histogram"},
		{"pdbplus_sync_operations_total", "sync operations counter"},
		{"pdbplus_sync_type_duration_seconds", "per-type sync duration"},
		{"pdbplus_sync_type_objects_total", "per-type object count"},
		{"pdbplus_sync_type_deleted_total", "per-type delete count"},
		{"pdbplus_sync_type_fetch_errors_total", "per-type fetch errors"},
		{"pdbplus_sync_type_upsert_errors_total", "per-type upsert errors"},
		{"pdbplus_sync_type_fallback_total", "per-type fallback events"},
		{"pdbplus_role_transitions_total", "role transitions"},
		{"http_server_request_duration_seconds", "HTTP request duration"},
		{"http_server_active_requests", "HTTP active requests"},
		{"process_runtime_go_goroutines", "Go goroutines"},
		{"process_runtime_go_mem_heap_alloc_bytes", "Go heap memory"},
		{"process_runtime_go_gc_heap_goal_bytes", "Go GC goal"},
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

	// Verify the freshness gauge has green/yellow/red thresholds at 0/3600/7200.
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
	}
	type fullDashboard struct {
		Panels []fullPanel `json:"panels"`
	}

	var fd fullDashboard
	if err := json.Unmarshal(data, &fd); err != nil {
		t.Fatalf("parsing dashboard JSON: %v", err)
	}

	for _, p := range fd.Panels {
		if p.Title != "Data Freshness" || p.Type != "gauge" {
			continue
		}

		steps := p.FieldConfig.Defaults.Thresholds.Steps
		if len(steps) < 3 {
			t.Fatalf("Data Freshness gauge has %d threshold steps, want at least 3", len(steps))
		}

		// Step 0: green (null value = base)
		if steps[0].Color != "green" {
			t.Errorf("step 0 color = %q, want green", steps[0].Color)
		}
		// Step 1: yellow at 3600 (1 hour)
		if steps[1].Color != "yellow" || steps[1].Value == nil || *steps[1].Value != 3600 {
			val := "nil"
			if steps[1].Value != nil {
				val = strings.TrimRight(strings.TrimRight(
					strings.Replace(
						strings.Replace(
							strings.Replace("%f", "%f", "", 1),
							"%f", "", 1),
						"%f", "", 1),
					"0"), ".")
			}
			_ = val
			t.Errorf("step 1: color=%q value=%v, want yellow/3600", steps[1].Color, steps[1].Value)
		}
		// Step 2: red at 7200 (2 hours)
		if steps[2].Color != "red" || steps[2].Value == nil || *steps[2].Value != 7200 {
			t.Errorf("step 2: color=%q value=%v, want red/7200", steps[2].Color, steps[2].Value)
		}
		return
	}
	t.Error("Data Freshness gauge panel not found")
}

func TestDashboard_ProvisioningYAMLExists(t *testing.T) {
	t.Parallel()
	_, err := os.ReadFile("provisioning/dashboards/default.yaml")
	if err != nil {
		t.Fatalf("provisioning YAML not found: %v", err)
	}
}
