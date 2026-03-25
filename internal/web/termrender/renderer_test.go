package termrender

import (
	"bytes"
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
)

func TestRendererWrite_RichMode(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer

	styled := StyleHeading.Render("Test Heading")
	if err := r.Write(&buf, styled); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("Rich mode output should contain ANSI escape codes, got: %q", output)
	}
	if !strings.Contains(output, "Test Heading") {
		t.Errorf("Output should contain text content, got: %q", output)
	}
}

func TestRendererWrite_PlainMode(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	styled := StyleHeading.Render("Test Heading")
	if err := r.Write(&buf, styled); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "\x1b[") {
		t.Errorf("Plain mode output should NOT contain ANSI escape codes, got: %q", output)
	}
	if !strings.Contains(output, "Test Heading") {
		t.Errorf("Output should contain text content, got: %q", output)
	}
}

func TestRendererWrite_NoColor(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, true)
	var buf bytes.Buffer

	styled := StyleHeading.Render("Test Heading")
	if err := r.Write(&buf, styled); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "\x1b[") {
		t.Errorf("noColor output should NOT contain ANSI escape codes, got: %q", output)
	}
	if !strings.Contains(output, "Test Heading") {
		t.Errorf("Output should contain text content, got: %q", output)
	}
}

func TestRendererWritef(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	if err := r.Writef(&buf, "hello %s, count: %d", "world", 42); err != nil {
		t.Fatalf("Writef() error: %v", err)
	}

	output := buf.String()
	if output != "hello world, count: 42" {
		t.Errorf("Writef() = %q, want %q", output, "hello world, count: 42")
	}
}

func TestRendererMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode RenderMode
	}{
		{name: "HTML", mode: ModeHTML},
		{name: "Rich", mode: ModeRich},
		{name: "Plain", mode: ModePlain},
		{name: "JSON", mode: ModeJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRenderer(tt.mode, false)
			if got := r.Mode(); got != tt.mode {
				t.Errorf("Mode() = %v, want %v", got, tt.mode)
			}
		})
	}
}

func TestRendererNoColor(t *testing.T) {
	t.Parallel()

	r1 := NewRenderer(ModeRich, false)
	if r1.NoColor() {
		t.Error("NoColor() should be false when created with noColor=false")
	}

	r2 := NewRenderer(ModeRich, true)
	if !r2.NoColor() {
		t.Error("NoColor() should be true when created with noColor=true")
	}
}

func TestRenderJSON(t *testing.T) {
	t.Parallel()

	type testData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	var buf bytes.Buffer
	data := testData{Name: "Test", Value: 42}

	if err := RenderJSON(&buf, data); err != nil {
		t.Fatalf("RenderJSON() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name": "Test"`) {
		t.Errorf("JSON output should contain name field, got: %q", output)
	}
	if !strings.Contains(output, `"value": 42`) {
		t.Errorf("JSON output should contain value field, got: %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("JSON output should end with newline, got: %q", output)
	}
}

func TestTableBorder_RichMode(t *testing.T) {
	t.Parallel()

	got := TableBorder(ModeRich)
	want := lipgloss.NormalBorder()
	if got != want {
		t.Errorf("TableBorder(ModeRich) = %+v, want NormalBorder %+v", got, want)
	}
}

func TestTableBorder_PlainMode(t *testing.T) {
	t.Parallel()

	got := TableBorder(ModePlain)
	want := lipgloss.ASCIIBorder()
	if got != want {
		t.Errorf("TableBorder(ModePlain) = %+v, want ASCIIBorder %+v", got, want)
	}
}
