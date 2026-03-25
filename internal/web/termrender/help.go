package termrender

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// RenderHelp writes terminal help text listing available endpoints, query parameters,
// format options, and usage examples. Style inspired by wttr.in (D-13).
func (r *Renderer) RenderHelp(w io.Writer, freshness time.Time) error {
	var buf strings.Builder

	// Title.
	buf.WriteString(StyleHeading.Render("PeeringDB Plus"))
	buf.WriteString(" - Terminal Interface\n\n")

	// Usage section.
	buf.WriteString(StyleHeading.Render("Usage:"))
	buf.WriteString("\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/asn/<asn>\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/ix/<id>\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/fac/<id>\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/org/<id>\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/campus/<id>\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/carrier/<id>\n")
	buf.WriteString("\n")

	// Search section.
	buf.WriteString(StyleHeading.Render("Search:"))
	buf.WriteString("\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/?q=cloudflare\n")
	buf.WriteString("\n")

	// Compare section.
	buf.WriteString(StyleHeading.Render("Compare Networks:"))
	buf.WriteString("\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/compare/13335/15169\n")
	buf.WriteString("\n")

	// Format options section.
	buf.WriteString(StyleHeading.Render("Format Options:"))
	buf.WriteString("\n")
	buf.WriteString(fmt.Sprintf("  %s  %s\n", StyleLabel.Render("?format=json"), "JSON output"))
	buf.WriteString(fmt.Sprintf("  %s  %s\n", StyleLabel.Render("?format=plain"), "Plain text (no ANSI colors)"))
	buf.WriteString(fmt.Sprintf("  %s        %s\n", StyleLabel.Render("?T"), "Shorthand for ?format=plain"))
	buf.WriteString(fmt.Sprintf("  %s   %s\n", StyleLabel.Render("?nocolor"), "Keep layout, strip colors"))
	buf.WriteString("\n")

	// Examples section.
	buf.WriteString(StyleHeading.Render("Examples:"))
	buf.WriteString("\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/asn/13335           # Cloudflare\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/asn/15169           # Google\n")
	buf.WriteString("  $ curl peeringdb-plus.fly.dev/ui/ix/31               # DE-CIX Frankfurt\n")
	buf.WriteString("  $ curl \"peeringdb-plus.fly.dev/ui/asn/13335?T\"       # Plain text\n")
	buf.WriteString("  $ curl \"peeringdb-plus.fly.dev/ui/asn/13335?format=json\" | jq .\n")
	buf.WriteString("\n")

	// Data freshness footer.
	if !freshness.IsZero() {
		age := time.Since(freshness).Truncate(time.Second)
		buf.WriteString(StyleMuted.Render(fmt.Sprintf("Data last synced: %s (%s ago)", freshness.Format("2006-01-02 15:04:05 UTC"), age)))
		buf.WriteString("\n")
	}

	return r.Write(w, buf.String())
}
