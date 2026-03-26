package termrender

import (
	"bytes"
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

func TestRendererWrite_RichMode(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, false)
	var buf bytes.Buffer

	styled := StyleHeading.Render("Test Heading")
	if err := r.Write(&buf, styled); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "\x1b[") {
		t.Errorf("Rich mode output should contain ANSI escape codes, got: %q", output)
	}
	if !strings.Contains(output, "Test Heading") {
		t.Errorf("Output should contain text content, got: %q", output)
	}
}

func TestRendererWrite_PlainMode(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	styled := StyleHeading.Render("Test Heading")
	if err := r.Write(&buf, styled); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "\x1b[") {
		t.Errorf("Plain mode output should NOT contain ANSI escape codes, got: %q", output)
	}
	if !strings.Contains(output, "Test Heading") {
		t.Errorf("Output should contain text content, got: %q", output)
	}
}

func TestRendererWrite_NoColor(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeRich, true)
	var buf bytes.Buffer

	styled := StyleHeading.Render("Test Heading")
	if err := r.Write(&buf, styled); err != nil {
		t.Fatalf("Write() error: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, "\x1b[") {
		t.Errorf("noColor output should NOT contain ANSI escape codes, got: %q", output)
	}
	if !strings.Contains(output, "Test Heading") {
		t.Errorf("Output should contain text content, got: %q", output)
	}
}

func TestRendererWritef(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModePlain, false)
	var buf bytes.Buffer

	if err := r.Writef(&buf, "hello %s, count: %d", "world", 42); err != nil {
		t.Fatalf("Writef() error: %v", err)
	}

	output := buf.String()
	if output != "hello world, count: 42" {
		t.Errorf("Writef() = %q, want %q", output, "hello world, count: 42")
	}
}

func TestRendererMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode RenderMode
	}{
		{name: "HTML", mode: ModeHTML},
		{name: "Rich", mode: ModeRich},
		{name: "Plain", mode: ModePlain},
		{name: "JSON", mode: ModeJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRenderer(tt.mode, false)
			if got := r.Mode(); got != tt.mode {
				t.Errorf("Mode() = %v, want %v", got, tt.mode)
			}
		})
	}
}

func TestRendererNoColor(t *testing.T) {
	t.Parallel()

	r1 := NewRenderer(ModeRich, false)
	if r1.NoColor() {
		t.Error("NoColor() should be false when created with noColor=false")
	}

	r2 := NewRenderer(ModeRich, true)
	if !r2.NoColor() {
		t.Error("NoColor() should be true when created with noColor=true")
	}
}

func TestRenderJSON(t *testing.T) {
	t.Parallel()

	type testData struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	var buf bytes.Buffer
	data := testData{Name: "Test", Value: 42}

	if err := RenderJSON(&buf, data); err != nil {
		t.Fatalf("RenderJSON() error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, `"name": "Test"`) {
		t.Errorf("JSON output should contain name field, got: %q", output)
	}
	if !strings.Contains(output, `"value": 42`) {
		t.Errorf("JSON output should contain value field, got: %q", output)
	}
	if !strings.HasSuffix(output, "\n") {
		t.Errorf("JSON output should end with newline, got: %q", output)
	}
}

// renderJSONOutput is a test helper that renders data as JSON and returns the string.
func renderJSONOutput(t *testing.T, data any) string {
	t.Helper()
	var buf bytes.Buffer
	if err := RenderJSON(&buf, data); err != nil {
		t.Fatalf("RenderJSON() error: %v", err)
	}
	return buf.String()
}

func TestRenderJSON_NetworkWithChildren(t *testing.T) {
	t.Parallel()

	data := templates.NetworkDetail{
		ID:      1,
		ASN:     13335,
		Name:    "Cloudflare",
		OrgName: "Cloudflare, Inc.",
		IXPresences: []templates.NetworkIXLanRow{
			{IXName: "DE-CIX Frankfurt", IXID: 31, Speed: 100000, IPAddr4: "80.81.192.123"},
			{IXName: "AMS-IX", IXID: 26, Speed: 100000},
		},
		FacPresences: []templates.NetworkFacRow{
			{FacName: "Equinix FR5", FacID: 42, City: "Frankfurt", Country: "DE"},
		},
	}

	out := renderJSONOutput(t, data)

	if !strings.Contains(out, `"ixPresences"`) {
		t.Error("JSON output should contain ixPresences key")
	}
	if !strings.Contains(out, `"facPresences"`) {
		t.Error("JSON output should contain facPresences key")
	}
	if !strings.Contains(out, "DE-CIX Frankfurt") {
		t.Error("JSON output should contain IX name 'DE-CIX Frankfurt'")
	}
	if !strings.Contains(out, "Equinix FR5") {
		t.Error("JSON output should contain facility name 'Equinix FR5'")
	}
}

func TestRenderJSON_IXWithChildren(t *testing.T) {
	t.Parallel()

	data := templates.IXDetail{
		ID:   31,
		Name: "DE-CIX Frankfurt",
		Participants: []templates.IXParticipantRow{
			{NetName: "Cloudflare", ASN: 13335, Speed: 100000},
		},
		Facilities: []templates.IXFacilityRow{
			{FacName: "Equinix FR5", FacID: 42, City: "Frankfurt", Country: "DE"},
		},
		Prefixes: []templates.IXPrefixRow{
			{Prefix: "80.81.192.0/22", Protocol: "IPv4", InDFZ: true},
		},
	}

	out := renderJSONOutput(t, data)

	if !strings.Contains(out, `"participants"`) {
		t.Error("JSON output should contain participants key")
	}
	if !strings.Contains(out, `"facilities"`) {
		t.Error("JSON output should contain facilities key")
	}
	if !strings.Contains(out, `"prefixes"`) {
		t.Error("JSON output should contain prefixes key")
	}
}

func TestRenderJSON_FacilityWithChildren(t *testing.T) {
	t.Parallel()

	data := templates.FacilityDetail{
		ID:   42,
		Name: "Equinix FR5",
		Networks: []templates.FacNetworkRow{
			{NetName: "Cloudflare", ASN: 13335},
		},
		IXPs: []templates.FacIXRow{
			{IXName: "DE-CIX Frankfurt", IXID: 31},
		},
		Carriers: []templates.FacCarrierRow{
			{CarrierName: "Zayo", CarrierID: 7},
		},
	}

	out := renderJSONOutput(t, data)

	if !strings.Contains(out, `"networks"`) {
		t.Error("JSON output should contain networks key")
	}
	if !strings.Contains(out, `"ixps"`) {
		t.Error("JSON output should contain ixps key")
	}
	if !strings.Contains(out, `"carriers"`) {
		t.Error("JSON output should contain carriers key")
	}
}

func TestRenderJSON_OrgWithChildren(t *testing.T) {
	t.Parallel()

	data := templates.OrgDetail{
		ID:   123,
		Name: "Cloudflare, Inc.",
		Networks: []templates.OrgNetworkRow{
			{NetName: "Cloudflare", ASN: 13335},
		},
		IXPs: []templates.OrgIXRow{
			{IXName: "Cloudflare IX", IXID: 999},
		},
		Facs: []templates.OrgFacilityRow{
			{FacName: "Equinix FR5", FacID: 42},
		},
		Campuses: []templates.OrgCampusRow{
			{CampusName: "Ashburn Campus", CampusID: 5},
		},
		Carriers: []templates.OrgCarrierRow{
			{CarrierName: "Cloudflare Carrier", CarrierID: 99},
		},
	}

	out := renderJSONOutput(t, data)

	checks := []string{
		`"networks"`,
		`"ixps"`,
		`"facilities"`,
		`"campuses"`,
		`"carriers"`,
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("JSON output should contain %s key", want)
		}
	}
}

func TestRenderJSON_EmptyChildren(t *testing.T) {
	t.Parallel()

	data := templates.NetworkDetail{
		ID:   1,
		ASN:  13335,
		Name: "Cloudflare",
		// IXPresences and FacPresences are nil -- should be omitted via omitempty.
	}

	out := renderJSONOutput(t, data)

	if strings.Contains(out, `"ixPresences"`) {
		t.Error("JSON output should NOT contain ixPresences when nil (omitempty)")
	}
	if strings.Contains(out, `"facPresences"`) {
		t.Error("JSON output should NOT contain facPresences when nil (omitempty)")
	}

	// Core fields should still be present.
	if !strings.Contains(out, `"Name": "Cloudflare"`) {
		t.Error("JSON output should contain network name")
	}
}

func TestRenderJSON_SearchGroups(t *testing.T) {
	t.Parallel()

	data := []templates.SearchGroup{
		{
			TypeName:    "Networks",
			TypeSlug:    "net",
			AccentColor: "emerald",
			TotalCount:  42,
			Results: []templates.SearchResult{
				{Name: "Cloudflare", Subtitle: "AS13335", DetailURL: "/ui/asn/13335"},
				{Name: "Google", Subtitle: "AS15169", DetailURL: "/ui/asn/15169"},
			},
		},
		{
			TypeName:    "IXPs",
			TypeSlug:    "ix",
			AccentColor: "sky",
			TotalCount:  3,
			Results: []templates.SearchResult{
				{Name: "DE-CIX Frankfurt", Subtitle: "Frankfurt, DE", DetailURL: "/ui/ix/31"},
			},
		},
	}

	out := renderJSONOutput(t, data)

	// Should be valid JSON array with type groups.
	if !strings.Contains(out, `"TypeName": "Networks"`) {
		t.Error("JSON output should contain TypeName 'Networks'")
	}
	if !strings.Contains(out, `"TypeName": "IXPs"`) {
		t.Error("JSON output should contain TypeName 'IXPs'")
	}
	if !strings.Contains(out, `"/ui/asn/13335"`) {
		t.Error("JSON output should contain DetailURL")
	}
	if !strings.Contains(out, `"Cloudflare"`) {
		t.Error("JSON output should contain result name 'Cloudflare'")
	}
}

func TestTableBorder_RichMode(t *testing.T) {
	t.Parallel()

	got := TableBorder(ModeRich)
	want := lipgloss.NormalBorder()
	if got != want {
		t.Errorf("TableBorder(ModeRich) = %+v, want NormalBorder %+v", got, want)
	}
}

func TestTableBorder_PlainMode(t *testing.T) {
	t.Parallel()

	got := TableBorder(ModePlain)
	want := lipgloss.ASCIIBorder()
	if got != want {
		t.Errorf("TableBorder(ModePlain) = %+v, want ASCIIBorder %+v", got, want)
	}
}
