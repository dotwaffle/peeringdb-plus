package templates

import (
	"strings"
	"testing"
)

// TestConnectionMarkers locks the marker badges and their upstream tooltips
// (PeeringDB 2.83.0 templates/site/view_exchange_side.html).
func TestConnectionMarkers(t *testing.T) {
	t.Parallel()

	render := func(t *testing.T, m ConnectionMarkers) string {
		t.Helper()
		var buf strings.Builder
		if err := connectionMarkers(m).Render(t.Context(), &buf); err != nil {
			t.Fatalf("render connectionMarkers(%+v): %v", m, err)
		}
		return buf.String()
	}

	if out := render(t, ConnectionMarkers{}); out != "" {
		t.Errorf("a connection without markers rendered %q, want nothing", out)
	}

	out := render(t, ConnectionMarkers{
		NotOperational: true, PlannedStatus: "deleted", PlannedDate: "2026-12-31", RFC8950: true,
	})
	for _, want := range []string{
		`title="Not operational">not operational</span>`,
		">planned removal 2026-12-31</span>",
		"nothing happens automatically on this date.",
		`title="The network supports RFC8950 extended next hop on this connection">RFC8950</span>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("markers missing %q:\n%s", want, out)
		}
	}

	// The plan date is user-supplied upstream text and must be escaped.
	out = render(t, ConnectionMarkers{PlannedStatus: "ok", PlannedDate: "<script>x</script>"})
	if strings.Contains(out, "<script>") || !strings.Contains(out, "planned activation &lt;script&gt;") {
		t.Errorf("plan date not escaped:\n%s", out)
	}
}
