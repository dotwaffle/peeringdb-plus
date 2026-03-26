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
// (RND-08, D-05 through D-07)
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

		// Group header: TypeName (N results)
		header := fmt.Sprintf("%s (%d results)", group.TypeName, group.TotalCount)
		buf.WriteString(StyleHeading.Render(header))
		buf.WriteString("\n")

		// Individual results
		for _, result := range group.Results {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(result.Name))

			if result.Subtitle != "" {
				buf.WriteString("  ")
				buf.WriteString(StyleMuted.Render(result.Subtitle))
			}

			buf.WriteString("  ")
			buf.WriteString(StyleLink.Render(result.DetailURL))
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}
