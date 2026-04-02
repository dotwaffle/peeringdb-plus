package termrender

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

func TestRenderAboutPage_WithFreshness(t *testing.T) {
	t.Parallel()

	data := templates.DataFreshness{
		Available:  true,
		LastSyncAt: time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC),
		Age:        5 * time.Minute,
	}

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer
	if err := r.RenderAboutPage(&buf, data); err != nil {
		t.Fatalf("RenderAboutPage() error: %v", err)
	}
	out := buf.String()
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"PeeringDB Plus",
		"Last Sync",
		"2025-06-15 12:30:00 UTC",
		"Data Age",
		"5m0s",
		"API Endpoints",
		"/graphql",
		"/rest/v1/",
		"/api/",
		"/ui/",
		"/peeringdb.v1.*/",
		"?format=json",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderAboutPage_NoFreshness(t *testing.T) {
	t.Parallel()

	data := templates.DataFreshness{Available: false}

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer
	if err := r.RenderAboutPage(&buf, data); err != nil {
		t.Fatalf("RenderAboutPage() error: %v", err)
	}
	out := buf.String()
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "PeeringDB Plus") {
		t.Error("output missing project name")
	}
	if !strings.Contains(stripped, "No sync data available") {
		t.Error("output missing 'No sync data available'")
	}
	if strings.Contains(stripped, "Data Age") {
		t.Error("output should not contain 'Data Age' when freshness unavailable")
	}
	if !strings.Contains(stripped, "/graphql") {
		t.Error("output missing API endpoints")
	}
}

func TestRenderAboutPage_PlainMode(t *testing.T) {
	t.Parallel()

	data := templates.DataFreshness{
		Available:  true,
		LastSyncAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Age:        time.Hour,
	}

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer
	if err := r.RenderAboutPage(&buf, data); err != nil {
		t.Fatalf("RenderAboutPage() error: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "\x1b[") {
		t.Error("Plain mode output should not contain ANSI escape codes")
	}
	if !strings.Contains(out, "PeeringDB Plus") {
		t.Error("output missing project name in plain mode")
	}
}
