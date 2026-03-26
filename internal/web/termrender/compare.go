package termrender

import (
	"fmt"
	"io"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// RenderCompare renders a network comparison as terminal output showing shared
// IXPs, facilities, and campuses with per-network presence details.
// (RND-09)
func (r *Renderer) RenderCompare(w io.Writer, data *templates.CompareData) error {
	if data == nil {
		var buf strings.Builder
		buf.WriteString(StyleMuted.Render("No comparison data available."))
		buf.WriteString("\n")
		return r.Write(w, buf.String())
	}

	var buf strings.Builder
	buf.Grow(len(data.SharedIXPs)*200 + len(data.SharedFacilities)*100 + len(data.SharedCampuses)*100 + 500)

	// Title line: NetA (AS{asn}) vs NetB (AS{asn})
	title := fmt.Sprintf("%s (AS%d) vs %s (AS%d)",
		data.NetA.Name, data.NetA.ASN,
		data.NetB.Name, data.NetB.ASN)
	buf.WriteString(StyleHeading.Render(title))
	buf.WriteString("\n")

	// Shared IXPs section.
	writeSharedIXPs(&buf, data)

	// Shared Facilities section.
	writeSharedFacilities(&buf, data)

	// Shared Campuses section.
	writeSharedCampuses(&buf, data)

	buf.WriteString("\n")
	return r.Write(w, buf.String())
}

// writeSharedIXPs renders the shared IXPs section with per-network presence details.
func writeSharedIXPs(buf *strings.Builder, data *templates.CompareData) {
	buf.WriteString("\n")
	header := fmt.Sprintf("Shared IXPs (%d)", len(data.SharedIXPs))
	buf.WriteString(StyleHeading.Render(header))
	buf.WriteString("\n")

	if len(data.SharedIXPs) == 0 {
		buf.WriteString("  ")
		buf.WriteString(StyleMuted.Render("No shared IXPs"))
		buf.WriteString("\n")
		return
	}

	for _, ixp := range data.SharedIXPs {
		// IX name and cross-reference.
		buf.WriteString("  ")
		buf.WriteString(StyleValue.Render(ixp.IXName))
		buf.WriteString(" ")
		buf.WriteString(CrossRef(fmt.Sprintf("/ui/ix/%d", ixp.IXID)))
		buf.WriteString("\n")

		// Per-network presence lines.
		writeIXPresence(buf, fmt.Sprintf("AS%d:", data.NetA.ASN), ixp.NetA)
		writeIXPresence(buf, fmt.Sprintf("AS%d:", data.NetB.ASN), ixp.NetB)
	}
}

// writeIXPresence renders one network's presence details at an IXP.
func writeIXPresence(buf *strings.Builder, label string, presence *templates.CompareIXPresence) {
	if presence == nil {
		return
	}

	buf.WriteString("    ")
	buf.WriteString(StyleMuted.Render(label))

	if presence.Speed > 0 {
		buf.WriteString("  ")
		buf.WriteString(SpeedStyle(presence.Speed).Render(FormatSpeed(presence.Speed)))
	}

	if presence.IsRSPeer {
		buf.WriteString("  ")
		buf.WriteString(rsBadge)
	}

	if presence.IPAddr4 != "" {
		buf.WriteString("  ")
		buf.WriteString(presence.IPAddr4)
		if presence.IPAddr6 != "" {
			buf.WriteString(" / ")
			buf.WriteString(presence.IPAddr6)
		}
	} else if presence.IPAddr6 != "" {
		buf.WriteString("  ")
		buf.WriteString(presence.IPAddr6)
	}

	buf.WriteString("\n")
}

// writeSharedFacilities renders the shared facilities section with location info.
func writeSharedFacilities(buf *strings.Builder, data *templates.CompareData) {
	buf.WriteString("\n")
	header := fmt.Sprintf("Shared Facilities (%d)", len(data.SharedFacilities))
	buf.WriteString(StyleHeading.Render(header))
	buf.WriteString("\n")

	if len(data.SharedFacilities) == 0 {
		buf.WriteString("  ")
		buf.WriteString(StyleMuted.Render("No shared facilities"))
		buf.WriteString("\n")
		return
	}

	for _, fac := range data.SharedFacilities {
		buf.WriteString("  ")
		buf.WriteString(StyleValue.Render(fac.FacName))
		buf.WriteString(" ")
		buf.WriteString(CrossRef(fmt.Sprintf("/ui/fac/%d", fac.FacID)))

		if loc := formatLocation(fac.City, fac.Country); loc != "" {
			buf.WriteString("  ")
			buf.WriteString(StyleMuted.Render(loc))
		}

		buf.WriteString("\n")
	}
}

// writeSharedCampuses renders the shared campuses section with nested facilities.
func writeSharedCampuses(buf *strings.Builder, data *templates.CompareData) {
	buf.WriteString("\n")
	header := fmt.Sprintf("Shared Campuses (%d)", len(data.SharedCampuses))
	buf.WriteString(StyleHeading.Render(header))
	buf.WriteString("\n")

	if len(data.SharedCampuses) == 0 {
		buf.WriteString("  ")
		buf.WriteString(StyleMuted.Render("No shared campuses"))
		buf.WriteString("\n")
		return
	}

	for _, campus := range data.SharedCampuses {
		buf.WriteString("  ")
		buf.WriteString(StyleValue.Render(campus.CampusName))
		buf.WriteString(" ")
		buf.WriteString(CrossRef(fmt.Sprintf("/ui/campus/%d", campus.CampusID)))
		buf.WriteString("\n")

		for _, fac := range campus.SharedFacilities {
			buf.WriteString("    ")
			buf.WriteString(StyleMuted.Render(fac.FacName))
			buf.WriteString(" ")
			buf.WriteString(CrossRef(fmt.Sprintf("/ui/fac/%d", fac.FacID)))
			buf.WriteString("\n")
		}
	}
}
