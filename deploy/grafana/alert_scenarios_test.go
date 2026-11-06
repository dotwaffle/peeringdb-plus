package grafana_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"go.yaml.in/yaml/v4"
)

// TestAlerts_IncidentScenarios checks both Prometheus alert presence and the
// Grafana threshold conversion. A matching zero is an alert in Prometheus,
// but Grafana's A > 0 condition rejects it.
func TestAlerts_IncidentScenarios(t *testing.T) {
	t.Parallel()
	bin, err := exec.LookPath("promtool")
	if err != nil {
		t.Skip("promtool not on PATH; skipping incident scenarios")
	}
	for _, grafana := range []bool{false, true} {
		name := "prometheus"
		if grafana {
			name = "grafana"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			rules := loadAlerts(t)
			for i := range rules.Groups {
				for j := range rules.Groups[i].Rules {
					rule := &rules.Groups[i].Rules[j]
					// Scenarios check conditions and timing, not annotation text.
					rule.Annotations = nil
					if grafana {
						rule.Expr = "(" + rule.Expr + ") > 0"
					}
				}
			}
			dir := t.TempDir()
			data, err := yaml.Marshal(rules)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "rules.yaml"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			scenarios, err := os.ReadFile("testdata/alert-scenarios.yaml")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "scenarios.yaml"), scenarios, 0o600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, bin, "test", "rules", "scenarios.yaml")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("incident scenarios: %v\n%s", err, out)
			}
		})
	}
}
