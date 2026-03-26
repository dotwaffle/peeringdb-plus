package termrender

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRenderHelp_RichMode(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer
	ts := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	if err := r.RenderHelp(&buf, ts); err != nil {
		t.Fatalf("RenderHelp: %v", err)
	}

	out := buf.String()
	checks := []string{
		"PeeringDB Plus",
		"Usage:",
		"curl peeringdb-plus.fly.dev/ui/asn/",
		"?format=json",
		"?format=short",
		"?format=whois",
		"?section=",
		"?w=N",
		"?T",
		"?nocolor",
		"Shell Integration:",
		"completions/bash",
		"completions/zsh",
		"pdb()",
		"Data last synced:",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Rich mode should contain ANSI escape codes.
	if !strings.Contains(out, "\x1b[") {
		t.Error("expected ANSI escape codes in rich mode output")
	}
}

func TestRenderHelp_PlainMode(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer
	ts := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	if err := r.RenderHelp(&buf, ts); err != nil {
		t.Fatalf("RenderHelp: %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "PeeringDB Plus") {
		t.Error("output missing 'PeeringDB Plus'")
	}

	// Plain mode should NOT contain ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("unexpected ANSI escape codes in plain mode output")
	}
}

func TestRenderHelp_ZeroTimestamp(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer

	if err := r.RenderHelp(&buf, time.Time{}); err != nil {
		t.Fatalf("RenderHelp: %v", err)
	}

	out := buf.String()

	if strings.Contains(out, "Data last synced") {
		t.Error("output should not contain 'Data last synced' for zero timestamp")
	}
}
