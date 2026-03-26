package termrender

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderFacilityDetail renders a facility entity as terminal output with rich layout.
// Shows key-value header with address, network list, IX list, and carrier list.
// (RND-04, D-02 rich layout)
func (r *Renderer) RenderFacilityDetail(w io.Writer, data templates.FacilityDetail) error {
	var buf strings.Builder
	buf.Grow(len(data.Networks)*80 + len(data.IXPs)*60 + len(data.Carriers)*60 + 500)

	// Title line: Name  City, Country
	buf.WriteString(StyleHeading.Render(data.Name))
	if loc := formatLocation(data.City, data.Country); loc != "" {
		buf.WriteString("  ")
		buf.WriteString(StyleMuted.Render(loc))
	}
	buf.WriteString("\n")

	// Key-value header with right-aligned labels.
	writeKV(&buf, "Organization", styledVal(data.OrgName), labelWidth)
	writeKV(&buf, "Campus", styledVal(data.CampusName), labelWidth)
	writeKV(&buf, "Address", formatAddress(data.Address1, data.Address2), labelWidth)
	writeKV(&buf, "City", styledVal(data.City), labelWidth)
	writeKV(&buf, "State", styledVal(data.State), labelWidth)
	writeKV(&buf, "Country", styledVal(data.Country), labelWidth)
	writeKV(&buf, "Zipcode", styledVal(data.Zipcode), labelWidth)
	writeKV(&buf, "Region", styledVal(data.RegionContinent), labelWidth)
	writeKV(&buf, "CLLI", styledVal(data.CLLI), labelWidth)
	writeKV(&buf, "Website", styledVal(data.Website), labelWidth)
	writeKV(&buf, "Networks", StyleValue.Render(strconv.Itoa(data.NetCount)), labelWidth)
	writeKV(&buf, "IXPs", StyleValue.Render(strconv.Itoa(data.IXCount)), labelWidth)
	writeKV(&buf, "Carriers", StyleValue.Render(strconv.Itoa(data.CarrierCount)), labelWidth)

	// Networks section.
	if len(data.Networks) > 0 && ShouldShowSection(r.Sections, "net") {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Networks (%d)", len(data.Networks))))
		buf.WriteString("\n")

		for _, row := range data.Networks {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.NetName))
			if ShouldShowField("fac-networks", "crossref", r.Width) {
				buf.WriteString(" ")
				buf.WriteString(CrossRef(fmt.Sprintf("/ui/asn/%d", row.ASN)))
			}
			buf.WriteString("\n")
		}
	}

	// IXPs section.
	if len(data.IXPs) > 0 && ShouldShowSection(r.Sections, "ix") {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("IXPs (%d)", len(data.IXPs))))
		buf.WriteString("\n")

		for _, row := range data.IXPs {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.IXName))
			if ShouldShowField("fac-ixps", "crossref", r.Width) {
				buf.WriteString(" ")
				buf.WriteString(CrossRef(fmt.Sprintf("/ui/ix/%d", row.IXID)))
			}
			buf.WriteString("\n")
		}
	}

	// Carriers section.
	if len(data.Carriers) > 0 && ShouldShowSection(r.Sections, "carrier") {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Carriers (%d)", len(data.Carriers))))
		buf.WriteString("\n")

		for _, row := range data.Carriers {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.CarrierName))
			if ShouldShowField("fac-carriers", "crossref", r.Width) {
				buf.WriteString(" ")
				buf.WriteString(CrossRef(fmt.Sprintf("/ui/carrier/%d", row.CarrierID)))
			}
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}

// formatAddress builds a display address from address lines.
// Returns "" if both lines are empty. Omits Address2 if empty.
func formatAddress(addr1, addr2 string) string {
	switch {
	case addr1 != "" && addr2 != "":
		return styledVal(addr1 + ", " + addr2)
	case addr1 != "":
		return styledVal(addr1)
	case addr2 != "":
		return styledVal(addr2)
	default:
		return ""
	}
}
