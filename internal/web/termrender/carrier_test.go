package termrender

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// fullCarrier is a fully-populated carrier detail with facilities.
var fullCarrier = templates.CarrierDetail{
	ID:       5,
	Name:     "Zayo Group",
	NameLong: "Zayo Group Holdings, Inc.",
	AKA:      "Zayo",
	Website:  "https://zayo.com",
	OrgName:  "Zayo Group Holdings",
	OrgID:    30,
	Status:   "ok",
	FacCount: 5,
	Facilities: []templates.CarrierFacilityRow{
		{FacName: "Equinix DC2", FacID: 200},
		{FacName: "CoreSite LA1", FacID: 201},
	},
}

// emptyCarrier has only Name, no facilities or optional fields.
var emptyCarrier = templates.CarrierDetail{
	Name: "Empty Carrier",
}

// renderCarrierDetail is a test helper that renders a CarrierDetail and returns the output string.
func renderCarrierDetail(t *testing.T, mode RenderMode, noColor bool, data templates.CarrierDetail) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderCarrierDetail(&buf, data); err != nil {
		t.Fatalf("RenderCarrierDetail() error: %v", err)
	}
	return buf.String()
}

func TestRenderCarrierDetail_Header(t *testing.T) {
	t.Parallel()

	out := renderCarrierDetail(t, ModeRich, false, fullCarrier)

	checks := []string{
		"Zayo Group",
		"Zayo Group Holdings",
		"https://zayo.com",
		"5", // FacCount
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderCarrierDetail_Facilities(t *testing.T) {
	t.Parallel()

	out := renderCarrierDetail(t, ModeRich, false, fullCarrier)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Equinix DC2",
		"[/ui/fac/200]",
		"CoreSite LA1",
		"[/ui/fac/201]",
		"Facilities (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderCarrierDetail_EmptyCarrier(t *testing.T) {
	t.Parallel()

	out := renderCarrierDetail(t, ModeRich, false, emptyCarrier)
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "Empty Carrier") {
		t.Error("output missing carrier name")
	}

	if strings.Contains(stripped, "Facilities (") {
		t.Error("empty carrier should not contain Facilities section header")
	}
}

func TestRenderCarrierDetail_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderCarrierDetail(t, ModePlain, false, fullCarrier)

	// Plain mode should NOT contain any ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("plain mode output should not contain ANSI escape codes")
	}

	// All text content should still be present.
	checks := []string{
		"Zayo Group",
		"Zayo Group Holdings",
		"Equinix DC2",
		"CoreSite LA1",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("plain mode output missing text %q", want)
		}
	}
}
