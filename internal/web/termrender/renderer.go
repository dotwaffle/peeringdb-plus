package termrender

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
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

// RenderPage renders a terminal page, dispatching to entity-specific renderers
// based on the data type. Falls back to a generic stub for unrecognized types.
func (r *Renderer) RenderPage(w io.Writer, title string, data any) error {
	switch d := data.(type) {
	case templates.NetworkDetail:
		return r.RenderNetworkDetail(w, d)
	case templates.IXDetail:
		return r.RenderIXDetail(w, d)
	case templates.FacilityDetail:
		return r.RenderFacilityDetail(w, d)
	case templates.OrgDetail:
		return r.RenderOrgDetail(w, d)
	case templates.CampusDetail:
		return r.RenderCampusDetail(w, d)
	case templates.CarrierDetail:
		return r.RenderCarrierDetail(w, d)
	case []templates.SearchGroup:
		return r.RenderSearch(w, d)
	case *templates.CompareData:
		return r.RenderCompare(w, d)
	default:
		return r.renderStub(w, title)
	}
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

// RenderOrgDetail renders an organization entity as terminal output. Stub pending Plan 02.
func (r *Renderer) RenderOrgDetail(w io.Writer, data templates.OrgDetail) error {
	return r.renderStub(w, data.Name)
}

// RenderCampusDetail renders a campus entity as terminal output. Stub pending Plan 02.
func (r *Renderer) RenderCampusDetail(w io.Writer, data templates.CampusDetail) error {
	return r.renderStub(w, data.Name)
}

// RenderCarrierDetail renders a carrier entity as terminal output. Stub pending Plan 02.
func (r *Renderer) RenderCarrierDetail(w io.Writer, data templates.CarrierDetail) error {
	return r.renderStub(w, data.Name)
}

// RenderSearch renders search results as terminal output. Stub pending Plan 03.
func (r *Renderer) RenderSearch(w io.Writer, groups []templates.SearchGroup) error {
	return r.renderStub(w, "Search Results")
}

// RenderCompare renders ASN comparison as terminal output. Stub pending Plan 03.
func (r *Renderer) RenderCompare(w io.Writer, data *templates.CompareData) error {
	return r.renderStub(w, "Compare")
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
