package termrender

import (
	"fmt"
	"io"
	"strings"
)

// RenderError writes a terminal-formatted error page with status code, title,
// message, and a hint pointing users to the help page.
func (r *Renderer) RenderError(w io.Writer, statusCode int, title string, message string) error {
	var buf strings.Builder

	buf.WriteString(StyleError.Render(fmt.Sprintf("%d %s", statusCode, title)))
	buf.WriteString("\n\n")
	buf.WriteString(message)
	buf.WriteString("\n\n")
	buf.WriteString(StyleMuted.Render("Try: curl peeringdb-plus.fly.dev/ui/"))
	buf.WriteString("\n")

	return r.Write(w, buf.String())
}
