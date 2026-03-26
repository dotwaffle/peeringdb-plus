package termrender

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// labelWidth is the column width for right-aligned header labels.
// "Aggregate Bandwidth" is the longest label at 19 characters.
const labelWidth = 19

// rsBadge is the styled [RS] badge for route server peers.
var rsBadge = lipgloss.NewStyle().Foreground(ColorSuccess).Render("[RS]")

// RenderNetworkDetail renders a network entity as whois-style terminal output
// with colored speed tiers, policy badges, RS indicators, and navigable
// cross-references. (RND-02, RND-14, RND-16, D-01 through D-09)
func (r *Renderer) RenderNetworkDetail(w io.Writer, data templates.NetworkDetail) error {
	var buf strings.Builder
	buf.Grow(len(data.IXPresences)*120 + len(data.FacPresences)*80 + 500)

	// Title line: Name  AS{ASN}
	buf.WriteString(StyleHeading.Render(data.Name))
	buf.WriteString("  ")
	buf.WriteString(StyleMuted.Render(fmt.Sprintf("AS%d", data.ASN)))
	buf.WriteString("\n")

	// Key-value header with right-aligned labels.
	// Use styledVal to ensure empty fields pass "" to writeKV (which skips them).
	writeKV(&buf, "Type", styledVal(data.InfoType), labelWidth)
	writeKV(&buf, "Peering Policy", PolicyStyle(data.PolicyGeneral), labelWidth)
	writeKV(&buf, "Website", styledVal(data.Website), labelWidth)
	writeKV(&buf, "Organization", styledVal(data.OrgName), labelWidth)
	writeKV(&buf, "IRR AS-SET", styledVal(data.IRRAsSet), labelWidth)
	writeKV(&buf, "Looking Glass", styledVal(data.LookingGlass), labelWidth)
	writeKV(&buf, "Route Server", styledVal(data.RouteServer), labelWidth)
	writeKV(&buf, "Traffic", styledVal(data.InfoTraffic), labelWidth)
	writeKV(&buf, "Ratio", styledVal(data.InfoRatio), labelWidth)
	writeKV(&buf, "Scope", styledVal(data.InfoScope), labelWidth)
	writeKV(&buf, "IX Presences", StyleValue.Render(strconv.Itoa(data.IXCount)), labelWidth)
	writeKV(&buf, "Facilities", StyleValue.Render(strconv.Itoa(data.FacCount)), labelWidth)
	if data.InfoPrefixes4 > 0 {
		writeKV(&buf, "Prefixes v4", StyleValue.Render(strconv.Itoa(data.InfoPrefixes4)), labelWidth)
	}
	if data.InfoPrefixes6 > 0 {
		writeKV(&buf, "Prefixes v6", StyleValue.Render(strconv.Itoa(data.InfoPrefixes6)), labelWidth)
	}
	if data.AggregateBW > 0 {
		writeKV(&buf, "Aggregate Bandwidth", StyleValue.Render(FormatBandwidth(data.AggregateBW)), labelWidth)
	}

	// IX Presences section.
	if len(data.IXPresences) > 0 {
		sectionBW := 0
		for _, row := range data.IXPresences {
			sectionBW += row.Speed
		}

		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("IX Presences (%d)", len(data.IXPresences))))
		if sectionBW > 0 {
			buf.WriteString("  ")
			buf.WriteString(StyleMuted.Render(FormatBandwidth(sectionBW)))
		}
		buf.WriteString("\n")

		for _, row := range data.IXPresences {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.IXName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/ix/%d", row.IXID)))

			if row.IsRSPeer {
				buf.WriteString("  ")
				buf.WriteString(rsBadge)
			}

			if row.Speed > 0 {
				buf.WriteString("  ")
				buf.WriteString(SpeedStyle(row.Speed).Render(FormatSpeed(row.Speed)))
			}

			if row.IPAddr4 != "" {
				buf.WriteString("  ")
				buf.WriteString(row.IPAddr4)
				if row.IPAddr6 != "" {
					buf.WriteString(" / ")
					buf.WriteString(row.IPAddr6)
				}
			} else if row.IPAddr6 != "" {
				buf.WriteString("  ")
				buf.WriteString(row.IPAddr6)
			}

			buf.WriteString("\n")
		}
	}

	// Facilities section.
	if len(data.FacPresences) > 0 {
		buf.WriteString("\n")
		buf.WriteString(StyleHeading.Render(fmt.Sprintf("Facilities (%d)", len(data.FacPresences))))
		buf.WriteString("\n")

		for _, row := range data.FacPresences {
			buf.WriteString("  ")
			buf.WriteString(StyleValue.Render(row.FacName))
			if row.FacID != 0 {
				buf.WriteString(" ")
				buf.WriteString(CrossRef(fmt.Sprintf("/ui/fac/%d", row.FacID)))
			}

			if row.City != "" || row.Country != "" {
				buf.WriteString("  ")
				loc := row.City
				if loc != "" && row.Country != "" {
					loc += ", " + row.Country
				} else if row.Country != "" {
					loc = row.Country
				}
				buf.WriteString(StyleMuted.Render(loc))
			}

			buf.WriteString("\n")
		}
	}

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}

// FormatSpeed converts a speed in Mbps to a human-readable string.
// Matches the web UI's formatSpeed: >= 1,000,000 as terabits, >= 1000 as gigabits, else megabits.
func FormatSpeed(mbps int) string {
	switch {
	case mbps >= 1_000_000:
		return fmt.Sprintf("%dT", mbps/1_000_000)
	case mbps >= 1000:
		return fmt.Sprintf("%dG", mbps/1000)
	default:
		return fmt.Sprintf("%dM", mbps)
	}
}

// FormatBandwidth formats aggregate bandwidth in Mbps as a human-readable string.
// Returns "1.5 Tbps" for >= 1M, "10 Gbps" for >= 1000, "500 Mbps" otherwise.
// Matches the web UI's formatAggregateBW. (RND-15)
func FormatBandwidth(mbps int) string {
	switch {
	case mbps >= 1_000_000:
		return fmt.Sprintf("%.1f Tbps", float64(mbps)/1_000_000)
	case mbps >= 1000:
		return fmt.Sprintf("%d Gbps", mbps/1000)
	default:
		return fmt.Sprintf("%d Mbps", mbps)
	}
}

// SpeedStyle returns a lipgloss style colored by port speed tier. (RND-12)
// Matches web UI tiers: sub-1G gray, 1G neutral, 10G blue, 100G emerald, 400G+ amber.
func SpeedStyle(mbps int) lipgloss.Style {
	switch {
	case mbps >= 400_000:
		return lipgloss.NewStyle().Foreground(ColorSpeed400G).Bold(true)
	case mbps >= 100_000:
		return lipgloss.NewStyle().Foreground(ColorSpeed100G)
	case mbps > 1000:
		return lipgloss.NewStyle().Foreground(ColorSpeed10G)
	case mbps == 1000:
		return lipgloss.NewStyle().Foreground(ColorSpeed1G)
	default:
		return lipgloss.NewStyle().Foreground(ColorSpeedSub1G)
	}
}

// PolicyStyle returns styled text for a peering policy value. (RND-13, D-03)
// Open=green (ColorPolicyOpen), Selective=yellow (ColorPolicySelective),
// Restrictive=red (ColorPolicyRestrictive), others=default value style.
func PolicyStyle(policy string) string {
	switch strings.ToLower(policy) {
	case "open":
		return lipgloss.NewStyle().Foreground(ColorPolicyOpen).Render(policy)
	case "selective":
		return lipgloss.NewStyle().Foreground(ColorPolicySelective).Render(policy)
	case "restrictive":
		return lipgloss.NewStyle().Foreground(ColorPolicyRestrictive).Render(policy)
	default:
		return StyleValue.Render(policy)
	}
}

// CrossRef formats an inline cross-reference path styled with StyleLink. (RND-16, D-08)
// Example output: "[/ui/ix/31]" with link styling.
func CrossRef(path string) string {
	return StyleLink.Render("[" + path + "]")
}

// styledVal returns StyleValue-rendered text for non-empty strings.
// Returns "" for empty input so writeKV can skip the field.
func styledVal(s string) string {
	if s == "" {
		return ""
	}
	return StyleValue.Render(s)
}

// writeKV writes a labeled key-value pair with right-aligned label. (D-01)
// Labels are right-padded to labelWidth for column alignment.
// Empty values are omitted.
func writeKV(buf *strings.Builder, label, value string, labelWidth int) {
	if value == "" {
		return
	}
	padded := fmt.Sprintf("%*s", labelWidth, label)
	buf.WriteString(StyleLabel.Render(padded))
	buf.WriteString("  ")
	buf.WriteString(value)
	buf.WriteString("\n")
}
