package termrender

import (
	"io"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderAboutPage renders the About page as terminal output with project info,
// data freshness, and a list of API endpoints.
func (r *Renderer) RenderAboutPage(w io.Writer, data templates.DataFreshness) error {
	var buf strings.Builder
	buf.Grow(500)

	buf.WriteString(StyleHeading.Render("PeeringDB Plus"))
	buf.WriteString("\n\n")

	writeKV(&buf, "Description", styledVal("Read-only PeeringDB mirror with GraphQL, gRPC, and REST APIs"), labelWidth)

	if data.Available {
		writeKV(&buf, "Last Sync", styledVal(data.LastSyncAt.Format("2006-01-02 15:04:05 UTC")), labelWidth)
		writeKV(&buf, "Data Age", styledVal(data.Age.String()), labelWidth)
	} else {
		writeKV(&buf, "Last Sync", StyleMuted.Render("No sync data available"), labelWidth)
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
