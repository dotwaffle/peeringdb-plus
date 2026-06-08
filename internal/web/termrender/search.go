package termrender

import (
	"fmt"
	"io"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderSearch renders search results as a grouped text list for terminal clients.
// Results are grouped by entity type with headers showing total counts.
// Each result shows the entity name, subtitle (ASN or location), and detail URL.
func (r *Renderer) RenderSearch(w io.Writer, groups []templates.SearchGroup) error {
	var buf strings.Builder

	if len(groups) == 0 {
		buf.WriteString(StyleMuted.Render("No results found."))
		buf.WriteString("\n")
		return r.Write(w, buf.String())
	}

	for i, group := range groups {
		if i > 0 {
			buf.WriteString("\n")
		}

		// Group header: TypeName (N results), where N is the exact total match
		// count (Total), consistent with the web UI's count badge and "View all"
		// link. Only the first len(Results) rows are listed below.
		header := fmt.Sprintf("%s (%s results)", group.TypeName, templates.FormatThousands(group.Total))
		buf.WriteString(StyleHeading.Render(header))
		buf.WriteString("\n")

		// Individual results
		for _, result := range group.Results {
			buf.WriteString("  ")
			buf.WriteString(styledName(result.Name))

			// Show metadata: ASN, country, city as applicable.
			if result.ASN > 0 {
				buf.WriteString("  ")
				buf.WriteString(StyleMuted.Render(fmt.Sprintf("AS%d", result.ASN)))
			}
			if result.Country != "" {
				buf.WriteString("  ")
				buf.WriteString(styledMuted(result.Country))
			}
			if result.City != "" {
				buf.WriteString("  ")
				buf.WriteString(styledMuted(result.City))
			}

			buf.WriteString("  ")
			buf.WriteString(StyleLink.Render(sanitizeUpstream(result.DetailURL)))
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}
