package termrender

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderCarrierDetail renders a carrier entity as terminal output with minimal layout.
// Shows compact header with identity fields and facility list.
// (RND-07, D-03 minimal layout)
func (r *Renderer) RenderCarrierDetail(w io.Writer, data templates.CarrierDetail) error {
	var buf strings.Builder
	buf.Grow(len(data.Facilities)*60 + 500)

	// Title line: Name
	buf.WriteString(StyleHeading.Render(data.Name))
	buf.WriteString("\n")

	// Key-value header (compact per D-03).
	writeKV(&buf, "Organization", styledVal(data.OrgName), labelWidth)
	writeKV(&buf, "Website", styledVal(data.Website), labelWidth)
	writeKV(&buf, "Facilities", StyleValue.Render(strconv.Itoa(data.FacCount)), labelWidth)

	// Facilities section.
	if len(data.Facilities) > 0 {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Facilities (%d)", len(data.Facilities))))
		buf.WriteString("\n")

		for _, row := range data.Facilities {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.FacName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/fac/%d", row.FacID)))
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}
