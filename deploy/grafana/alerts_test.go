// Package grafana_test validates the Grafana Cloud alert rule YAML and
// dashboard JSON checked into deploy/grafana/. The alert tests live here
// alongside dashboard_test.go so a single `go test ./deploy/grafana/...`
// invocation covers both surfaces.
package grafana_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// alertFile mirrors the standard Prometheus alerting-rule YAML schema:
// https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/
type alertFile struct {
	Groups []alertGroup `yaml:"groups"`
}

type alertGroup struct {
	Name     string      `yaml:"name"`
	Interval string      `yaml:"interval"`
	Rules    []alertRule `yaml:"rules"`
}

type alertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

const (
	alertsPath       = "alerts/pdbplus-alerts.yaml"
	alertsReadmePath = "alerts/README.md"
	maxRules         = 8 // Stay under Grafana Cloud free-tier alertmanager limits.
	receiverName     = "grafana-default-email"
)

// loadAlerts reads and parses the alert YAML. testing.TB lets benchmarks
// reuse this helper if they are added later (Phase 72 parity convention).
func loadAlerts(tb testing.TB) alertFile {
	tb.Helper()
	data, err := os.ReadFile(alertsPath)
	if err != nil {
		tb.Fatalf("reading alert YAML: %v", err)
	}

	var f alertFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		tb.Fatalf("parsing alert YAML: %v", err)
	}
	return f
}

func TestAlerts_ValidYAML(t *testing.T) {
	t.Parallel()
	f := loadAlerts(t)
	if len(f.Groups) == 0 {
		t.Fatal("alert YAML has zero rule groups")
	}
}

func TestAlerts_RequiredFields(t *testing.T) {
	t.Parallel()
	f := loadAlerts(t)

	allowedSeverities := map[string]bool{"critical": true, "warning": true}

	for _, g := range f.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				t.Errorf("group %q: rule has empty alert name", g.Name)
				continue
			}
			if r.Expr == "" {
				t.Errorf("rule %q: missing expr", r.Alert)
			}
			if r.For == "" {
				t.Errorf("rule %q: missing for: duration", r.Alert)
			}
			if !allowedSeverities[r.Labels["severity"]] {
				t.Errorf("rule %q: severity=%q, want critical|warning",
					r.Alert, r.Labels["severity"])
			}
			if got := r.Labels["receiver"]; got != receiverName {
				t.Errorf("rule %q: receiver=%q, want %q",
					r.Alert, got, receiverName)
			}
			if r.Annotations["summary"] == "" {
				t.Errorf("rule %q: missing %s annotation", r.Alert, "summary")
			}
			if r.Annotations["description"] == "" {
				t.Errorf("rule %q: missing %s annotation", r.Alert, "description")
			}
		}
	}
}

func TestAlerts_RuleCountUnderCap(t *testing.T) {
	t.Parallel()
	f := loadAlerts(t)

	total := 0
	for _, g := range f.Groups {
		total += len(g.Rules)
	}
	if total > maxRules {
		t.Errorf("alert rule count %d exceeds cap of %d", total, maxRules)
	}
	if total == 0 {
		t.Error("alert rule count is 0; expected at least one rule")
	}
}

func TestAlerts_NoForbiddenContent(t *testing.T) {
	t.Parallel()

	// Literal byte tokens that MUST NOT appear in committed observability
	// config. The hosted Grafana Cloud stack URL fragment is split across
	// the literal parts so this source file does not itself trip the
	// check on a recursive grep of deploy/grafana/.
	const grafanaCloudHost = ".grafana" + ".net"
	forbidden := [][]byte{
		[]byte("@gmail.com"),
		[]byte("@anthropic.com"),
		[]byte(grafanaCloudHost),
	}

	for _, path := range []string{alertsPath, alertsReadmePath} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		for _, tok := range forbidden {
			if bytes.Contains(data, tok) {
				t.Errorf("%s contains forbidden token %q", path, tok)
			}
		}
	}
}

func TestAlerts_NamesPascalCasePdbPlusPrefix(t *testing.T) {
	t.Parallel()
	f := loadAlerts(t)

	re := regexp.MustCompile(`^PdbPlus[A-Z][A-Za-z]+$`)
	for _, g := range f.Groups {
		for _, r := range g.Rules {
			if !re.MatchString(r.Alert) {
				t.Errorf("rule name %q does not match ^PdbPlus[A-Z][A-Za-z]+$",
					r.Alert)
			}
		}
	}
}

// TestAlerts_PromtoolCheck shells out to `promtool check rules`. promtool
// is not in the project go.mod or `go tool` directives — it is an
// operator-installed external binary. Skip when absent.
func TestAlerts_PromtoolCheck(t *testing.T) {
	t.Parallel()
	bin, err := exec.LookPath("promtool")
	if err != nil {
		t.Skip("promtool not on PATH; skipping (operator-installed binary)")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, bin, "check", "rules", alertsPath).CombinedOutput()
	if err != nil {
		t.Fatalf("promtool check rules failed: %v\noutput:\n%s", err, out)
	}
}

// TestAlerts_MimirtoolCheck shells out to `mimirtool rules check`.
// Mirrors TestAlerts_PromtoolCheck — same skip-when-absent semantics.
func TestAlerts_MimirtoolCheck(t *testing.T) {
	t.Parallel()
	bin, err := exec.LookPath("mimirtool")
	if err != nil {
		t.Skip("mimirtool not on PATH; skipping (operator-installed binary)")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, bin, "rules", "check", alertsPath).CombinedOutput()
	if err != nil {
		// mimirtool may return non-zero on lint warnings; surface output for diagnosis.
		if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
			t.Fatalf("mimirtool rules check failed (exit %d):\n%s",
				exitErr.ExitCode(), out)
		}
		t.Fatalf("mimirtool rules check failed: %v\noutput:\n%s", err, out)
	}
}
