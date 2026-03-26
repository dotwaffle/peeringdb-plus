package termrender

import (
	"fmt"
	"io"
	"strings"

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
