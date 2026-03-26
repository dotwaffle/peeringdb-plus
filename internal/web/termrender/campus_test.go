package termrender

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// fullCampus is a fully-populated campus detail with facilities.
var fullCampus = templates.CampusDetail{
	ID:       10,
	Name:     "Equinix Amsterdam",
	NameLong: "Equinix Amsterdam Campus",
	AKA:      "AM Campus",
	Website:  "https://equinix.com/amsterdam",
	OrgName:  "Equinix, Inc.",
	OrgID:    20,
	City:     "Amsterdam",
	Country:  "NL",
	State:    "North Holland",
	Zipcode:  "1101",
	Status:   "ok",
	FacCount: 3,
	Facilities: []templates.CampusFacilityRow{
		{FacName: "Equinix AM3", FacID: 101, City: "Amsterdam", Country: "NL"},
		{FacName: "Equinix AM5", FacID: 102, City: "Amsterdam", Country: "NL"},
	},
}

// emptyCampus has only Name, no facilities or optional fields.
var emptyCampus = templates.CampusDetail{
	Name: "Empty Campus",
}

// renderCampusDetail is a test helper that renders a CampusDetail and returns the output string.
func renderCampusDetail(t *testing.T, mode RenderMode, noColor bool, data templates.CampusDetail) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderCampusDetail(&buf, data); err != nil {
		t.Fatalf("RenderCampusDetail() error: %v", err)
	}
	return buf.String()
}

func TestRenderCampusDetail_Header(t *testing.T) {
	t.Parallel()

	out := renderCampusDetail(t, ModeRich, false, fullCampus)

	checks := []string{
		"Equinix Amsterdam",
		"Equinix, Inc.",
		"https://equinix.com/amsterdam",
		"Amsterdam",
		"NL",
		"3", // FacCount
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderCampusDetail_Facilities(t *testing.T) {
	t.Parallel()

	out := renderCampusDetail(t, ModeRich, false, fullCampus)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Equinix AM3",
		"[/ui/fac/101]",
		"Amsterdam, NL",
		"Equinix AM5",
		"[/ui/fac/102]",
		"Facilities (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderCampusDetail_EmptyCampus(t *testing.T) {
	t.Parallel()

	out := renderCampusDetail(t, ModeRich, false, emptyCampus)
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "Empty Campus") {
		t.Error("output missing campus name")
	}

	if strings.Contains(stripped, "Facilities (") {
		t.Error("empty campus should not contain Facilities section header")
	}
}

func TestRenderCampusDetail_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderCampusDetail(t, ModePlain, false, fullCampus)

	// Plain mode should NOT contain any ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("plain mode output should not contain ANSI escape codes")
	}

	// All text content should still be present.
	checks := []string{
		"Equinix Amsterdam",
		"Equinix, Inc.",
		"Equinix AM3",
		"Equinix AM5",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("plain mode output missing text %q", want)
		}
	}
}
