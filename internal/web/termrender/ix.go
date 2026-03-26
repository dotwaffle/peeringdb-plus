package termrender

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderIXDetail renders an IX entity as terminal output with rich layout.
// Shows key-value header, participant table with speed/RS/IPs, facility list,
// and prefix list. (RND-03, D-02 rich layout)
func (r *Renderer) RenderIXDetail(w io.Writer, data templates.IXDetail) error {
	var buf strings.Builder
	buf.Grow(len(data.Participants)*120 + len(data.Facilities)*80 + len(data.Prefixes)*60 + 500)

	// Title line: Name  City, Country
	buf.WriteString(StyleHeading.Render(data.Name))
	if loc := formatLocation(data.City, data.Country); loc != "" {
		buf.WriteString("  ")
		buf.WriteString(StyleMuted.Render(loc))
	}
	buf.WriteString("\n")

	// Key-value header with right-aligned labels.
	writeKV(&buf, "Organization", styledVal(data.OrgName), labelWidth)
	writeKV(&buf, "Website", styledVal(data.Website), labelWidth)
	writeKV(&buf, "Region", styledVal(data.RegionContinent), labelWidth)
	writeKV(&buf, "Media", styledVal(data.Media), labelWidth)
	if protos := formatProtocols(data.ProtoUnicast, data.ProtoMulticast, data.ProtoIPv6); protos != "" {
		writeKV(&buf, "Protocols", styledVal(protos), labelWidth)
	}
	writeKV(&buf, "Participants", StyleValue.Render(strconv.Itoa(data.NetCount)), labelWidth)
	writeKV(&buf, "Facilities", StyleValue.Render(strconv.Itoa(data.FacCount)), labelWidth)
	writeKV(&buf, "Prefixes", StyleValue.Render(strconv.Itoa(data.PrefixCount)), labelWidth)
	if data.AggregateBW > 0 {
		writeKV(&buf, "Aggregate Bandwidth", StyleValue.Render(FormatBandwidth(data.AggregateBW)), labelWidth)
	}

	// Participants section (rich layout with speed, RS badge, IPs).
	if len(data.Participants) > 0 && ShouldShowSection(r.Sections, "net") {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Participants (%d)", len(data.Participants))))
		buf.WriteString("\n")

		for _, row := range data.Participants {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.NetName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/asn/%d", row.ASN)))

			if ShouldShowField("ix-participants", "rs", r.Width) && row.IsRSPeer {
				buf.WriteString("  ")
				buf.WriteString(rsBadge)
			}

			if ShouldShowField("ix-participants", "speed", r.Width) && row.Speed > 0 {
				buf.WriteString("  ")
				buf.WriteString(SpeedStyle(row.Speed).Render(FormatSpeed(row.Speed)))
			}

			if ShouldShowField("ix-participants", "ipv4", r.Width) && row.IPAddr4 != "" {
				buf.WriteString("  ")
				buf.WriteString(row.IPAddr4)
				if ShouldShowField("ix-participants", "ipv6", r.Width) && row.IPAddr6 != "" {
					buf.WriteString(" / ")
					buf.WriteString(row.IPAddr6)
				}
			} else if ShouldShowField("ix-participants", "ipv6", r.Width) && row.IPAddr6 != "" {
				buf.WriteString("  ")
				buf.WriteString(row.IPAddr6)
			}

			buf.WriteString("\n")
		}
	}

	// Facilities section.
	if len(data.Facilities) > 0 && ShouldShowSection(r.Sections, "fac") {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Facilities (%d)", len(data.Facilities))))
		buf.WriteString("\n")

		for _, row := range data.Facilities {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.FacName))
			if ShouldShowField("ix-facilities", "crossref", r.Width) {
				buf.WriteString(" ")
				buf.WriteString(CrossRef(fmt.Sprintf("/ui/fac/%d", row.FacID)))
			}

			if ShouldShowField("ix-facilities", "location", r.Width) {
				if loc := formatLocation(row.City, row.Country); loc != "" {
					buf.WriteString("  ")
					buf.WriteString(StyleMuted.Render(loc))
				}
			}

			buf.WriteString("\n")
		}
	}

	// Prefixes section.
	if len(data.Prefixes) > 0 && ShouldShowSection(r.Sections, "prefix") {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Prefixes (%d)", len(data.Prefixes))))
		buf.WriteString("\n")

		for _, row := range data.Prefixes {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.Prefix))
			buf.WriteString("  ")
			buf.WriteString(StyleMuted.Render(row.Protocol))
			if ShouldShowField("ix-prefixes", "dfz", r.Width) {
				buf.WriteString("  ")
				if row.InDFZ {
					buf.WriteString(StyleHeading.Render("[DFZ]"))
				} else {
					buf.WriteString(StyleMuted.Render("[not in DFZ]"))
				}
			}
			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}

// formatProtocols builds a comma-separated string from boolean protocol flags.
func formatProtocols(unicast, multicast, ipv6 bool) string {
	var parts []string
	if unicast {
		parts = append(parts, "unicast")
	}
	if multicast {
		parts = append(parts, "multicast")
	}
	if ipv6 {
		parts = append(parts, "IPv6")
	}
	return strings.Join(parts, ", ")
}

// formatLocation builds a "City, Country" string, handling empty components.
func formatLocation(city, country string) string {
	switch {
	case city != "" && country != "":
		return city + ", " + country
	case city != "":
		return city
	case country != "":
		return country
	default:
		return ""
	}
}
