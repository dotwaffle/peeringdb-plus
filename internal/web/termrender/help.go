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
	fmt.Fprintf(&buf, "  %s  %s\n", StyleLabel.Render("?format=json"), "JSON output")
	fmt.Fprintf(&buf, "  %s %s\n", StyleLabel.Render("?format=plain"), "Plain text (no ANSI colors)")
	fmt.Fprintf(&buf, "  %s %s\n", StyleLabel.Render("?format=short"), "One-line summary")
	fmt.Fprintf(&buf, "  %s %s\n", StyleLabel.Render("?format=whois"), "RPSL-style WHOIS output")
	fmt.Fprintf(&buf, "  %s        %s\n", StyleLabel.Render("?T"), "Shorthand for ?format=plain")
	fmt.Fprintf(&buf, "  %s   %s\n", StyleLabel.Render("?nocolor"), "Keep layout, strip colors")
	fmt.Fprintf(&buf, "  %s  %s\n", StyleLabel.Render("?section=..."), "Filter sections (ix, fac, net, carrier, campus, prefix)")
	fmt.Fprintf(&buf, "  %s       %s\n", StyleLabel.Render("?w=N"), "Adapt to terminal width (e.g. ?w=80)")
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

	// Shell integration section (D-19, SHL-03).
	buf.WriteString(StyleHeading.Render("Shell Integration:"))
	buf.WriteString("\n")
	buf.WriteString("  Quick setup (bash):\n")
	buf.WriteString("    $ eval \"$(curl -s peeringdb-plus.fly.dev/ui/completions/bash)\"\n")
	buf.WriteString("\n")
	buf.WriteString("  Quick setup (zsh):\n")
	buf.WriteString("    $ eval \"$(curl -s peeringdb-plus.fly.dev/ui/completions/zsh)\"\n")
	buf.WriteString("\n")
	buf.WriteString("  Manual alias:\n")
	buf.WriteString("    $ pdb() { curl -s \"peeringdb-plus.fly.dev/ui/$@\"; }\n")
	buf.WriteString("\n")
	buf.WriteString("  Then use:\n")
	buf.WriteString("    $ pdb asn/13335\n")
	buf.WriteString("    $ pdb ix/31\n")
	buf.WriteString("    $ pdb ?q=cloudflare\n")
	buf.WriteString("\n")

	// Data freshness footer.
	//
	// Renders only the absolute UTC timestamp; no "(N ago)" wall-clock-relative
	// phrasing. The help page is served through the sync-time-keyed HTTP caching
	// middleware, so a relative age string would freeze at cache-creation time
	// and mislead readers for up to a full sync interval. Readers who want a
	// relative age can compute it locally from the absolute timestamp.
	if !freshness.IsZero() {
		buf.WriteString(StyleMuted.Render("Data last synced: " + freshness.UTC().Format("2006-01-02 15:04:05 UTC")))
		buf.WriteString("\n")
	}

	return r.Write(w, buf.String())
}
