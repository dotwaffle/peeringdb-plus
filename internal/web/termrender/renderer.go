package termrender

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/charmbracelet/colorprofile"
)

// Renderer produces styled terminal text output with ANSI color control.
// It uses colorprofile.Writer to force the appropriate color profile for
// HTTP responses, since HTTP response writers are not TTYs and would
// otherwise auto-detect as NoTTY.
type Renderer struct {
	mode    RenderMode
	noColor bool
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
