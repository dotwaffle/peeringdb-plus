package termrender

import (
	"io"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderAboutPage renders the About page as terminal output with project info,
// data freshness, the Phase 61 OBS-02 Privacy & Sync section, and a list of
// API endpoints. The Privacy & Sync section is placed between Data Age and
// API Endpoints per D-04 so operators see the auth posture adjacent to the
// freshness signal.
func (r *Renderer) RenderAboutPage(w io.Writer, data templates.DataFreshness, privacy templates.PrivacySync) error {
	var buf strings.Builder
	buf.Grow(700)

	buf.WriteString(StyleHeading.Render("PeeringDB Plus"))
	buf.WriteString("\n\n")

	writeKV(&buf, "Description", styledVal("Read-only PeeringDB mirror with GraphQL, gRPC, and REST APIs"), labelWidth)

	if data.Available {
		writeKV(&buf, "Last Sync", styledVal(data.LastSyncAt.Format("2006-01-02 15:04:05 UTC")), labelWidth)
		writeKV(&buf, "Data Age", styledVal(data.Age.String()), labelWidth)
	} else {
		writeKV(&buf, "Last Sync", StyleMuted.Render("No sync data available"), labelWidth)
	}

	// Phase 61 OBS-02 Privacy & Sync section (D-04/D-05/D-06).
	buf.WriteString("\n")
	buf.WriteString(StyleHeading.Render("Privacy & Sync"))
	buf.WriteString("\n")
	writeKV(&buf, "Sync Mode", styledVal(privacy.AuthMode), labelWidth)

	tierValue := privacy.PublicTier
	if privacy.OverrideActive {
		// D-06 override indicator. The "! " prefix is part of the value
		// string (not a styled glyph) so PlainMode and ANSI-stripped
		// output still carry the signal — no silent escalation.
		tierValue = "! " + tierValue
	}
	writeKV(&buf, "Public Tier", styledVal(tierValue), labelWidth)
	if privacy.PublicTierExplanation != "" {
		buf.WriteString(StyleMuted.Render(privacy.PublicTierExplanation))
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
	buf.WriteString(StyleHeading.Render("API Endpoints"))
	buf.WriteString("\n")
	writeKV(&buf, "Web UI", styledVal("/ui/"), labelWidth)
	writeKV(&buf, "GraphQL", styledVal("/graphql"), labelWidth)
	writeKV(&buf, "REST", styledVal("/rest/v1/"), labelWidth)
	writeKV(&buf, "PeeringDB API", styledVal("/api/"), labelWidth)
	writeKV(&buf, "ConnectRPC", styledVal("/peeringdb.v1.*/"), labelWidth)

	buf.WriteString("\n")
	buf.WriteString(StyleMuted.Render("Use ?format=json for structured data."))
	buf.WriteString("\n")

	return r.Write(w, buf.String())
}
