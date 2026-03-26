package termrender

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// fullFacility is a fully-populated facility detail with networks, IXPs, and carriers.
var fullFacility = templates.FacilityDetail{
	ID:              42,
	Name:            "Equinix FR5",
	NameLong:        "Equinix Frankfurt 5",
	Website:         "https://equinix.com",
	OrgName:         "Equinix, Inc.",
	OrgID:           16,
	CampusName:      "Frankfurt Campus",
	CampusID:        7,
	Address1:        "Kleyerstrasse 90",
	Address2:        "Building C",
	City:            "Frankfurt",
	State:           "Hessen",
	Country:         "DE",
	Zipcode:         "60326",
	RegionContinent: "Europe",
	CLLI:            "FRKTDECA",
	Status:          "ok",
	NetCount:        150,
	IXCount:         3,
	CarrierCount:    5,
	Networks: []templates.FacNetworkRow{
		{NetName: "Cloudflare", ASN: 13335},
		{NetName: "Google", ASN: 15169},
	},
	IXPs: []templates.FacIXRow{
		{IXName: "DE-CIX Frankfurt", IXID: 31},
		{IXName: "ECIX Frankfurt", IXID: 123},
	},
	Carriers: []templates.FacCarrierRow{
		{CarrierName: "Telia Carrier", CarrierID: 10},
		{CarrierName: "Lumen", CarrierID: 20},
	},
}

// emptyFacility has only basic header fields, no networks, IXPs, or carriers.
var emptyFacility = templates.FacilityDetail{
	Name:    "Empty Facility",
	City:    "Nowhere",
	Country: "XX",
}

// minimalFacility has no CLLI, no campus, no state - tests optional field omission.
var minimalFacility = templates.FacilityDetail{
	Name:     "Simple DC",
	OrgName:  "Simple Org",
	Address1: "123 Main St",
	City:     "London",
	Country:  "GB",
	NetCount: 10,
}

// renderFacilityDetail is a test helper that renders a FacilityDetail and returns the output string.
func renderFacilityDetail(t *testing.T, mode RenderMode, noColor bool, data templates.FacilityDetail) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderFacilityDetail(&buf, data); err != nil {
		t.Fatalf("RenderFacilityDetail() error: %v", err)
	}
	return buf.String()
}

func TestRenderFacilityDetail_Header(t *testing.T) {
	t.Parallel()

	out := renderFacilityDetail(t, ModeRich, false, fullFacility)

	checks := []string{
		"Equinix FR5",
		"Frankfurt",
		"DE",
		"Equinix, Inc.",
		"https://equinix.com",
		"Europe",
		"FRKTDECA",
		"Kleyerstrasse 90",
		"Building C",
		"Hessen",
		"60326",
		"Frankfurt Campus",
		"150",
		"3",
		"5",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderFacilityDetail_Networks(t *testing.T) {
	t.Parallel()

	out := renderFacilityDetail(t, ModeRich, false, fullFacility)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Cloudflare",
		"[/ui/asn/13335]",
		"Google",
		"[/ui/asn/15169]",
		"Networks (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderFacilityDetail_IXPs(t *testing.T) {
	t.Parallel()

	out := renderFacilityDetail(t, ModeRich, false, fullFacility)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"DE-CIX Frankfurt",
		"[/ui/ix/31]",
		"ECIX Frankfurt",
		"[/ui/ix/123]",
		"IXPs (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderFacilityDetail_Carriers(t *testing.T) {
	t.Parallel()

	out := renderFacilityDetail(t, ModeRich, false, fullFacility)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Telia Carrier",
		"[/ui/carrier/10]",
		"Lumen",
		"[/ui/carrier/20]",
		"Carriers (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderFacilityDetail_EmptyFacility(t *testing.T) {
	t.Parallel()

	out := renderFacilityDetail(t, ModeRich, false, emptyFacility)
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "Empty Facility") {
		t.Error("output missing facility name")
	}
	if strings.Contains(stripped, "Networks (") {
		t.Error("empty facility should not contain Networks section")
	}
	if strings.Contains(stripped, "IXPs (") {
		t.Error("empty facility should not contain IXPs section")
	}
	if strings.Contains(stripped, "Carriers (") {
		t.Error("empty facility should not contain Carriers section")
	}
}

func TestRenderFacilityDetail_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderFacilityDetail(t, ModePlain, false, fullFacility)

	// Plain mode should have no ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("Plain mode output should not contain ANSI escape codes")
	}

	// Should still contain the facility name and key data.
	if !strings.Contains(out, "Equinix FR5") {
		t.Error("output missing facility name in plain mode")
	}
	if !strings.Contains(out, "Cloudflare") {
		t.Error("output missing network name in plain mode")
	}
}

func TestRenderFacilityDetail_OmittedFields(t *testing.T) {
	t.Parallel()

	out := renderFacilityDetail(t, ModeRich, false, minimalFacility)
	stripped := ansiRE.ReplaceAllString(out, "")

	// CLLI is empty, should not appear in output.
	if strings.Contains(stripped, "CLLI") {
		t.Error("output should not contain CLLI label when CLLI is empty")
	}

	// Campus is empty, should not appear in output.
	if strings.Contains(stripped, "Campus") {
		t.Error("output should not contain Campus label when campus is empty")
	}

	// State is empty, should not appear.
	if strings.Contains(stripped, "State") {
		t.Error("output should not contain State label when state is empty")
	}

	// Address should still show.
	if !strings.Contains(stripped, "123 Main St") {
		t.Error("output missing address")
	}
}
