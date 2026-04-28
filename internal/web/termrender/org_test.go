package termrender

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// fullOrg is a fully-populated org detail with diverse child entities.
var fullOrg = templates.OrgDetail{
	ID:           16,
	Name:         "Cloudflare, Inc.",
	NameLong:     "Cloudflare, Incorporated",
	AKA:          "CF",
	Website:      "https://cloudflare.com",
	Address1:     "101 Townsend St",
	Address2:     "Suite 200",
	City:         "San Francisco",
	Country:      "US",
	State:        "CA",
	Zipcode:      "94107",
	Notes:        "Major CDN and DNS provider",
	Status:       "ok",
	NetCount:     2,
	FacCount:     300,
	IXCount:      280,
	CampusCount:  1,
	CarrierCount: 1,
	Networks: []templates.OrgNetworkRow{
		{NetName: "Cloudflare", ASN: 13335},
		{NetName: "Cloudflare WARP", ASN: 202623},
	},
	IXPs: []templates.OrgIXRow{
		{IXName: "DE-CIX Frankfurt", IXID: 31},
		{IXName: "AMS-IX", IXID: 26},
	},
	Facs: []templates.OrgFacilityRow{
		{FacName: "Equinix FR5", FacID: 42, City: "Frankfurt", Country: "DE"},
		{FacName: "Equinix LD8", FacID: 55, City: "London", Country: "GB"},
	},
	Campuses: []templates.OrgCampusRow{
		{CampusName: "SFO Campus", CampusID: 10},
	},
	Carriers: []templates.OrgCarrierRow{
		{CarrierName: "Zayo Group", CarrierID: 5},
	},
}

// emptyOrg has only Name, no children or optional fields.
var emptyOrg = templates.OrgDetail{
	Name: "Empty Org",
}

// renderOrgDetail is a test helper that renders an OrgDetail and returns the output string.
func renderOrgDetail(t *testing.T, mode RenderMode, noColor bool, data templates.OrgDetail) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderOrgDetail(&buf, data); err != nil {
		t.Fatalf("RenderOrgDetail() error: %v", err)
	}
	return buf.String()
}

func TestRenderOrgDetail_Header(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModeRich, false, fullOrg)

	checks := []string{
		"Cloudflare, Inc.",
		"https://cloudflare.com",
		"101 Townsend St",
		"San Francisco",
		"US",
		"280", // IXCount
		"300", // FacCount
		"2",   // NetCount
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderOrgDetail_Networks(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModeRich, false, fullOrg)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Cloudflare",
		"[/ui/asn/13335]",
		"Cloudflare WARP",
		"[/ui/asn/202623]",
		"Networks (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderOrgDetail_IXPs(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModeRich, false, fullOrg)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"DE-CIX Frankfurt",
		"[/ui/ix/31]",
		"AMS-IX",
		"[/ui/ix/26]",
		"IXPs (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderOrgDetail_Facilities(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModeRich, false, fullOrg)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Equinix FR5",
		"[/ui/fac/42]",
		"Frankfurt, DE",
		"Equinix LD8",
		"[/ui/fac/55]",
		"London, GB",
		"Facilities (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderOrgDetail_Campuses(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModeRich, false, fullOrg)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"SFO Campus",
		"[/ui/campus/10]",
		"Campuses (1)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderOrgDetail_Carriers(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModeRich, false, fullOrg)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Zayo Group",
		"[/ui/carrier/5]",
		"Carriers (1)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderOrgDetail_EmptyOrg(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModeRich, false, emptyOrg)
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "Empty Org") {
		t.Error("output missing org name")
	}

	// Section headers should not appear for empty child lists.
	if strings.Contains(stripped, "Networks (") {
		t.Error("empty org should not contain Networks section header")
	}
	if strings.Contains(stripped, "IXPs (") {
		t.Error("empty org should not contain IXPs section header")
	}
	if strings.Contains(stripped, "Facilities (") {
		t.Error("empty org should not contain Facilities section header")
	}
	if strings.Contains(stripped, "Campuses (") {
		t.Error("empty org should not contain Campuses section header")
	}
	if strings.Contains(stripped, "Carriers (") {
		t.Error("empty org should not contain Carriers section header")
	}
}

func TestRenderOrgDetail_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderOrgDetail(t, ModePlain, false, fullOrg)

	// Plain mode should NOT contain any ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("plain mode output should not contain ANSI escape codes")
	}

	// All text content should still be present.
	checks := []string{
		"Cloudflare, Inc.",
		"https://cloudflare.com",
		"San Francisco",
		"DE-CIX Frankfurt",
		"Equinix FR5",
		"SFO Campus",
		"Zayo Group",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("plain mode output missing text %q", want)
		}
	}
}

func TestRenderOrgDetail_OmittedFields(t *testing.T) {
	t.Parallel()

	data := templates.OrgDetail{
		Name:     "Sparse Org",
		NetCount: 1,
		// Website, Address1, Address2, City, Country are all empty.
	}

	out := renderOrgDetail(t, ModeRich, false, data)
	stripped := ansiRE.ReplaceAllString(out, "")

	if strings.Contains(stripped, "Website") {
		t.Error("empty Website should be omitted from output")
	}
	if strings.Contains(stripped, "Address") {
		t.Error("empty Address should be omitted from output")
	}
	if strings.Contains(stripped, "Location") {
		t.Error("empty Location should be omitted from output")
	}

	// Name should still be present.
	if !strings.Contains(stripped, "Sparse Org") {
		t.Error("output missing org name")
	}
}
