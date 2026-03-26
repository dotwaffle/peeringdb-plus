package termrender

import (
	"fmt"
	"io"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderNetworkDetail renders a network entity detail page for terminal output.
// Placeholder -- full implementation in Plan 02.
func (r *Renderer) RenderNetworkDetail(w io.Writer, data templates.NetworkDetail) error {
	var buf strings.Builder
	buf.WriteString(StyleHeading.Render(data.Name))
	buf.WriteString("  ")
	buf.WriteString(StyleMuted.Render(fmt.Sprintf("AS%d", data.ASN)))
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
