package termrender

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderOrgDetail renders an organization entity as terminal output with minimal layout.
// Shows compact header with identity fields and simple name-only child entity lists.
// (RND-05, D-03 minimal layout)
func (r *Renderer) RenderOrgDetail(w io.Writer, data templates.OrgDetail) error {
	var buf strings.Builder
	buf.Grow(len(data.Networks)*80 + len(data.IXPs)*60 + len(data.Facs)*80 +
		len(data.Campuses)*60 + len(data.Carriers)*60 + 500)

	// Title line: Name
	buf.WriteString(StyleHeading.Render(data.Name))
	buf.WriteString("\n")

	// Key-value header (compact, identity fields only per D-03).
	writeKV(&buf, "Website", styledVal(data.Website), labelWidth)
	writeKV(&buf, "Address", formatAddress(data.Address1, data.Address2), labelWidth)
	writeKV(&buf, "Location", styledVal(formatLocation(data.City, data.Country)), labelWidth)
	writeKV(&buf, "Networks", StyleValue.Render(strconv.Itoa(data.NetCount)), labelWidth)
	writeKV(&buf, "IXPs", StyleValue.Render(strconv.Itoa(data.IXCount)), labelWidth)
	writeKV(&buf, "Facilities", StyleValue.Render(strconv.Itoa(data.FacCount)), labelWidth)
	if data.CampusCount > 0 {
		writeKV(&buf, "Campuses", StyleValue.Render(strconv.Itoa(data.CampusCount)), labelWidth)
	}
	if data.CarrierCount > 0 {
		writeKV(&buf, "Carriers", StyleValue.Render(strconv.Itoa(data.CarrierCount)), labelWidth)
	}

	// Networks section.
	if len(data.Networks) > 0 {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Networks (%d)", len(data.Networks))))
		buf.WriteString("\n")

		for _, row := range data.Networks {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.NetName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/asn/%d", row.ASN)))
			buf.WriteString("\n")
		}
	}

	// IXPs section.
	if len(data.IXPs) > 0 {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("IXPs (%d)", len(data.IXPs))))
		buf.WriteString("\n")

		for _, row := range data.IXPs {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.IXName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/ix/%d", row.IXID)))
			buf.WriteString("\n")
		}
	}

	// Facilities section.
	if len(data.Facs) > 0 {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Facilities (%d)", len(data.Facs))))
		buf.WriteString("\n")

		for _, row := range data.Facs {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.FacName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/fac/%d", row.FacID)))

			if loc := formatLocation(row.City, row.Country); loc != "" {
				buf.WriteString("  ")
				buf.WriteString(StyleMuted.Render(loc))
			}

			buf.WriteString("\n")
		}
	}

	// Campuses section.
	if len(data.Campuses) > 0 {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Campuses (%d)", len(data.Campuses))))
		buf.WriteString("\n")

		for _, row := range data.Campuses {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.CampusName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/campus/%d", row.CampusID)))
			buf.WriteString("\n")
		}
	}

	// Carriers section.
	if len(data.Carriers) > 0 {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Carriers (%d)", len(data.Carriers))))
		buf.WriteString("\n")

		for _, row := range data.Carriers {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.CarrierName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/carrier/%d", row.CarrierID)))
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}
