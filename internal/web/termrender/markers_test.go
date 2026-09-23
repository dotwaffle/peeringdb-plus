package termrender

import (
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// allMarkers sets every connection marker.
var allMarkers = templates.ConnectionMarkers{
	NotOperational: true, PlannedStatus: "deleted", PlannedDate: "2026-12-31", RFC8950: true,
}

// lineWith returns the first line of s that contains substr, or "".
func lineWith(s, substr string) string {
	for line := range strings.SplitSeq(s, "\n") {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

// assertMarkers checks that the line holding marked carries every marker
// and that the line holding plain carries none.
func assertMarkers(t *testing.T, out, marked, plain string) {
	t.Helper()
	stripped := ansiRE.ReplaceAllString(out, "")
	line := lineWith(stripped, marked)
	for _, want := range []string{"[not operational]", "[planned removal 2026-12-31]", "[RFC8950]"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q lacks marker %q", line, want)
		}
	}
	if line := lineWith(stripped, plain); line == "" || strings.Contains(line, "[not operational]") ||
		strings.Contains(line, "[planned") || strings.Contains(line, "[RFC8950]") {
		t.Errorf("line %q for a connection without markers is missing or carries a marker", line)
	}
}

func TestRenderNetworkDetail_ConnectionMarkers(t *testing.T) {
	t.Parallel()
	data := templates.NetworkDetail{
		Name: "Dark Net", ASN: 65002,
		IXPresences: []templates.NetworkIXLanRow{
			{IXName: "Dark IX", IXID: 20, Speed: 10000, Markers: allMarkers},
			{IXName: "Live IX", IXID: 21, Speed: 10000},
		},
	}
	assertMarkers(t, renderNetworkDetail(t, ModeRich, false, data), "Dark IX", "Live IX")

	// The markers ignore the width thresholds.
	var buf strings.Builder
	r := NewRenderer(ModePlain, true)
	r.Width = 40
	if err := r.RenderNetworkDetail(&buf, data); err != nil {
		t.Fatalf("RenderNetworkDetail: %v", err)
	}
	if line := lineWith(buf.String(), "Dark IX"); !strings.Contains(line, "[not operational]") {
		t.Errorf("narrow plain line %q lacks [not operational]", line)
	}
}

func TestRenderIXDetail_ConnectionMarkers(t *testing.T) {
	t.Parallel()
	data := templates.IXDetail{
		Name: "Dark IX",
		Participants: []templates.IXParticipantRow{
			{NetName: "Dark Net", ASN: 65002, Speed: 10000, Markers: allMarkers},
			{NetName: "Live Net", ASN: 65003, Speed: 10000},
		},
	}
	assertMarkers(t, renderIXDetail(t, ModeRich, false, data), "Dark Net", "Live Net")
}

func TestRenderCompare_ConnectionMarkers(t *testing.T) {
	t.Parallel()
	data := &templates.CompareData{
		NetA: templates.CompareNetwork{ASN: 65002, Name: "Dark Net"},
		NetB: templates.CompareNetwork{ASN: 65003, Name: "Live Net"},
		SharedIXPs: []templates.CompareIXP{{
			IXID: 20, IXName: "Dark IX", Shared: true,
			NetA: &templates.CompareIXPresence{Speed: 10000, Markers: allMarkers},
			NetB: &templates.CompareIXPresence{Speed: 10000, Operational: true},
		}},
	}
	assertMarkers(t, renderCompare(t, ModeRich, false, data), "AS65002:", "AS65003:")
}
