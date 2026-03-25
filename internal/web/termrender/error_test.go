package termrender

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderError_404(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer

	if err := r.RenderError(&buf, 404, "Not Found", "The page you're looking for doesn't exist."); err != nil {
		t.Fatalf("RenderError: %v", err)
	}

	out := buf.String()
	checks := []string{
		"404 Not Found",
		"doesn't exist",
		"Try: curl",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderError_500(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer

	if err := r.RenderError(&buf, 500, "Internal Server Error", "An unexpected error occurred."); err != nil {
		t.Fatalf("RenderError: %v", err)
	}

	out := buf.String()
	checks := []string{
		"500 Internal Server Error",
		"unexpected error",
		"Try: curl",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderError_PlainMode(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	if err := r.RenderError(&buf, 404, "Not Found", "The page you're looking for doesn't exist."); err != nil {
		t.Fatalf("RenderError: %v", err)
	}

	out := buf.String()

	if !strings.Contains(out, "404 Not Found") {
		t.Error("output missing '404 Not Found'")
	}

	// Plain mode should NOT contain ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("unexpected ANSI escape codes in plain mode output")
	}
}
