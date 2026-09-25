package grafana_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// slo mirrors the fields of slo.v0_0.Slo in the OpenAPI specification of
// the Grafana SLO app that the files in slos/ use. The specification sets
// additionalProperties to false, so loadSLO rejects unknown fields. It
// leaves out uuid: the create example of the API documentation omits it,
// and the server assigns it.
type slo struct {
	Name                  string       `json:"name"`
	Description           string       `json:"description"`
	Folder                *sloUID      `json:"folder"`
	DestinationDatasource *sloUID      `json:"destinationDatasource"`
	Labels                []sloLabel   `json:"labels"`
	Objectives            []sloGoal    `json:"objectives"`
	Query                 sloQuery     `json:"query"`
	Alerting              *sloAlerting `json:"alerting"`
}

type sloUID struct {
	UID string `json:"uid"`
}

type sloLabel struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type sloGoal struct {
	Value  float64 `json:"value"`
	Window string  `json:"window"`
}

type sloQuery struct {
	Type     string       `json:"type"`
	Ratio    *sloRatio    `json:"ratio"`
	Freeform *sloFreeform `json:"freeform"`
}

type sloRatio struct {
	SuccessMetric sloMetric `json:"successMetric"`
	TotalMetric   sloMetric `json:"totalMetric"`
}

type sloMetric struct {
	PrometheusMetric string `json:"prometheusMetric"`
}

type sloFreeform struct {
	Query string `json:"query"`
}

type sloAlerting struct {
	AdvancedOptions *struct {
		MinFailures int64 `json:"minFailures"`
	} `json:"advancedOptions"`
	FastBurn *sloBurn `json:"fastBurn"`
	SlowBurn *sloBurn `json:"slowBurn"`
}

type sloBurn struct {
	Labels      []sloLabel `json:"labels"`
	Annotations []sloLabel `json:"annotations"`
}

const (
	slosDir                   = "slos"
	availabilitySLOPath       = "slos/availability.json"
	errorRatePanelID          = 11
	availabilityRouteMatchers = `http_route!="", http_route!~"GET /(healthz|readyz)|/grpc[.]health[.].*|POST /sync"`
)

func loadSLO(t *testing.T, path string) slo {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var s slo
	if err := dec.Decode(&s); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return s
}

func sloFiles(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(slosDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no SLO files in %s", slosDir)
	}
	return paths
}

// TestSLOs_RequiredFields checks the fields that the SLO API requires,
// and the values that the apply workflow in slos/README.md expects.
func TestSLOs_RequiredFields(t *testing.T) {
	t.Parallel()
	for _, path := range sloFiles(t) {
		s := loadSLO(t, path)
		if s.Name == "" || s.Description == "" {
			t.Errorf("%s: name and description are required", path)
		}
		if s.Folder == nil || s.Folder.UID != "peeringdb-plus" {
			t.Errorf("%s: folder.uid must be peeringdb-plus", path)
		}
		if s.DestinationDatasource == nil || s.DestinationDatasource.UID == "" {
			t.Errorf("%s: destinationDatasource.uid is required", path)
		}
		if !slices.Contains(s.Labels, sloLabel{Key: "service", Value: "peeringdb-plus"}) {
			t.Errorf("%s: label service=peeringdb-plus is required", path)
		}
		if len(s.Objectives) != 1 {
			t.Errorf("%s: %d objectives, want 1", path, len(s.Objectives))
		}
		for _, o := range s.Objectives {
			if o.Value <= 0 || o.Value >= 1 {
				t.Errorf("%s: objective value %v, want between 0 and 1", path, o.Value)
			}
			days, err := strconv.Atoi(strings.TrimSuffix(o.Window, "d"))
			if err != nil || !strings.HasSuffix(o.Window, "d") || days < 7 {
				t.Errorf("%s: objective window %q, want at least 7d in days", path, o.Window)
			}
		}
		switch s.Query.Type {
		case "ratio":
			if s.Query.Ratio == nil || s.Query.Ratio.SuccessMetric.PrometheusMetric == "" ||
				s.Query.Ratio.TotalMetric.PrometheusMetric == "" {
				t.Errorf("%s: a ratio query needs successMetric and totalMetric", path)
			}
		case "freeform":
			if s.Query.Freeform == nil || s.Query.Freeform.Query == "" {
				t.Errorf("%s: a freeform query needs query", path)
			}
		default:
			t.Errorf("%s: query type %q, want ratio or freeform", path, s.Query.Type)
		}
	}
}

// TestSLOs_AvailabilitySelector locks the request selector of the
// availability SLI, and that the Error Rate (5xx) dashboard panel uses
// the same one.
func TestSLOs_AvailabilitySelector(t *testing.T) {
	t.Parallel()
	s := loadSLO(t, availabilitySLOPath)
	if s.Query.Ratio == nil {
		t.Fatalf("%s: query is not a ratio", availabilitySLOPath)
	}
	total := s.Query.Ratio.TotalMetric.PrometheusMetric
	success := s.Query.Ratio.SuccessMetric.PrometheusMetric
	wantTotal := `http_server_request_duration_seconds_count{service_name="peeringdb-plus", ` +
		availabilityRouteMatchers + `}`
	if total != wantTotal {
		t.Errorf("totalMetric = %q, want %q", total, wantTotal)
	}
	wantSuccess := strings.TrimSuffix(wantTotal, "}") + `, http_response_status_code!~"5.."}`
	if success != wantSuccess {
		t.Errorf("successMetric = %q, want %q", success, wantSuccess)
	}

	for _, p := range allPanels(loadDashboard(t)) {
		if p.ID != errorRatePanelID {
			continue
		}
		if len(p.Targets) != 1 {
			t.Fatalf("panel %d has %d targets, want 1", p.ID, len(p.Targets))
		}
		if got := strings.Count(p.Targets[0].Expr, availabilityRouteMatchers); got != 2 {
			t.Errorf("panel %d expr has the SLO route matchers %d times, want 2 (errors and total): %s",
				p.ID, got, p.Targets[0].Expr)
		}
		return
	}
	t.Errorf("panel %d not found", errorRatePanelID)
}

// TestSLOs_BurnRateAlerts checks that the availability burn-rate alerts
// use the severity tiers of the alert rules and have a failure floor.
func TestSLOs_BurnRateAlerts(t *testing.T) {
	t.Parallel()
	a := loadSLO(t, availabilitySLOPath).Alerting
	if a == nil || a.FastBurn == nil || a.SlowBurn == nil {
		t.Fatal("availability SLO needs fastBurn and slowBurn alerting")
	}
	if a.AdvancedOptions == nil || a.AdvancedOptions.MinFailures < 1 {
		t.Error("availability SLO needs alerting.advancedOptions.minFailures >= 1")
	}
	for _, tt := range []struct {
		name     string
		burn     *sloBurn
		severity string
	}{
		{"fastBurn", a.FastBurn, "critical"},
		{"slowBurn", a.SlowBurn, "warning"},
	} {
		if !slices.Contains(tt.burn.Labels, sloLabel{Key: "severity", Value: tt.severity}) {
			t.Errorf("%s: label severity=%s is required", tt.name, tt.severity)
		}
		keys := make([]string, 0, len(tt.burn.Annotations))
		for _, l := range tt.burn.Annotations {
			keys = append(keys, l.Key)
		}
		for _, want := range []string{"summary", "description"} {
			if !slices.Contains(keys, want) {
				t.Errorf("%s: annotation %q is required", tt.name, want)
			}
		}
	}

	for _, path := range sloFiles(t) {
		if path == filepath.FromSlash(availabilitySLOPath) {
			continue
		}
		// The SLO app makes no alert rules for time-based SLIs.
		if loadSLO(t, path).Alerting != nil {
			t.Errorf("%s: alerting is set, but only the availability SLO alerts", path)
		}
	}
}

func TestSLOs_NoForbiddenContent(t *testing.T) {
	t.Parallel()
	assertNoForbiddenContent(t, append(sloFiles(t), filepath.Join(slosDir, "README.md")))
}

func TestSynthetics_NoForbiddenContent(t *testing.T) {
	t.Parallel()
	assertNoForbiddenContent(t, []string{"synthetics/README.md"})
}

// assertNoForbiddenContent fails the test when a file contains an email
// address domain or the hosted Grafana Cloud stack host.
func assertNoForbiddenContent(t *testing.T, paths []string) {
	t.Helper()
	// Split so that this file does not match the check.
	const grafanaCloudHost = ".grafana" + ".net"
	forbidden := []string{"@gmail.com", "@anthropic.com", grafanaCloudHost}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, tok := range forbidden {
			if bytes.Contains(data, []byte(tok)) {
				t.Errorf("%s contains forbidden token %q", path, tok)
			}
		}
	}
}
