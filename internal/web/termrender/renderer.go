package termrender

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/charmbracelet/colorprofile"
)

// Renderer produces styled terminal text output with ANSI color control.
// It uses colorprofile.Writer to force the appropriate color profile for
// HTTP responses, since HTTP response writers are not TTYs and would
// otherwise auto-detect as NoTTY.
type Renderer struct {
	mode    RenderMode
	noColor bool
	// Sections is a per-request section filter (nil = show all). Set by the
	// caller after NewRenderer, before calling RenderPage.
	Sections map[string]bool
	// Width is a per-request width adaptation hint (0 = no restriction). Set
	// by the caller after NewRenderer, before calling RenderPage.
	Width int
}

// NewRenderer creates a terminal renderer. mode controls ANSI/ASCII/JSON selection.
// noColor suppresses all ANSI codes regardless of mode.
func NewRenderer(mode RenderMode, noColor bool) *Renderer {
	return &Renderer{
		mode:    mode,
		noColor: noColor,
	}
}

// Mode returns the renderer's output mode.
func (r *Renderer) Mode() RenderMode {
	return r.mode
}

// NoColor reports whether ANSI codes are suppressed.
func (r *Renderer) NoColor() bool {
	return r.noColor
}

// Write renders styled text to w, applying the correct color profile based
// on mode and noColor settings. In Rich mode, ANSI 256-color codes pass
// through. In Plain mode or when noColor is true, all ANSI codes are stripped.
func (r *Renderer) Write(w io.Writer, content string) error {
	cw := &colorprofile.Writer{Forward: w}
	switch {
	case r.noColor || r.mode == ModePlain:
		cw.Profile = colorprofile.NoTTY
	case r.mode == ModeRich:
		cw.Profile = colorprofile.ANSI256
	default:
		cw.Profile = colorprofile.NoTTY
	}
	_, err := cw.WriteString(content)
	return err
}

// Writef renders a formatted string to w using the same color profile logic
// as Write.
func (r *Renderer) Writef(w io.Writer, format string, args ...any) error {
	return r.Write(w, fmt.Sprintf(format, args...))
}

// RenderPage renders a terminal page, dispatching to entity-specific renderers
// via the registered function map. Falls back to a generic stub for unrecognized types.
func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
	if data != nil {
		if fn, ok := renderers[reflect.TypeOf(data)]; ok {
			return fn(data, w, r)
		}
	}
	return r.renderStub(w, title)
}

// renderStub renders a placeholder for entity types not yet fully implemented.
func (r *Renderer) renderStub(w io.Writer, name string) error {
	var buf strings.Builder
	buf.WriteString(StyleHeading.Render(name))
	buf.WriteString("\n\n")
	buf.WriteString(StyleMuted.Render("Detailed terminal view coming in a future update."))
	buf.WriteString("\n")
	buf.WriteString(StyleMuted.Render("Use ?format=json for structured data."))
	buf.WriteString("\n")
	return r.Write(w, buf.String())
}

// RenderSearch is implemented in search.go.
// RenderCompare is implemented in compare.go.

// RenderJSON writes data as indented JSON to w.
// Used for ?format=json responses. The caller is responsible for setting
// appropriate Content-Type headers.
func RenderJSON(w io.Writer, data any) error {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	_, err = w.Write(out)
	if err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	_, err = w.Write([]byte("\n"))
	if err != nil {
		return fmt.Errorf("write json newline: %w", err)
	}
	return nil
}
