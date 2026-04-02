package termrender

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// whoisAnsiRE matches ANSI escape sequences.
var whoisAnsiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// renderWHOIS is a test helper that renders data in WHOIS format and returns the output.
func renderWHOIS(t *testing.T, title string, data any) string {
	t.Helper()
	r := NewRenderer(ModeWHOIS, true)
	var buf bytes.Buffer
	if err := r.RenderWHOIS(&buf, title, data); err != nil {
		t.Fatalf("RenderWHOIS() error: %v", err)
	}
	return buf.String()
}

func TestRenderWHOIS_NetworkHeader(t *testing.T) {
	t.Parallel()

	out := renderWHOIS(t, "Network", fullNetwork)

	if !strings.HasPrefix(out, "% Source: PeeringDB-Plus\n") {
		t.Errorf("output should start with Source header, got prefix: %q", out[:min(len(out), 50)])
	}
	if !strings.Contains(out, "% Query: AS13335\n") {
		t.Error("output should contain Query line with AS13335")
	}
}

func TestRenderWHOIS_NetworkAutNum(t *testing.T) {
	t.Parallel()

	out := renderWHOIS(t, "Network", fullNetwork)

	checks := []string{
		"aut-num:",
		"as-name:",
		"descr:",
		"org:",
		"website:",
		"irr-as-set:",
		"policy:",
		"source:         PEERINGDB-PLUS",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Verify specific values.
	if !strings.Contains(out, "AS13335") {
		t.Error("output missing AS13335 value")
	}
	if !strings.Contains(out, "Cloudflare") {
		t.Error("output missing Cloudflare name")
	}
	if !strings.Contains(out, "AS-CLOUDFLARE") {
		t.Error("output missing AS-CLOUDFLARE")
	}
}

func TestRenderWHOIS_NetworkMultiValue(t *testing.T) {
	t.Parallel()

	out := renderWHOIS(t, "Network", fullNetwork)

	// Should have multiple ix: lines (one per IX presence).
	ixLines := 0
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "ix:") {
			ixLines++
		}
	}
	if ixLines != 3 {
		t.Errorf("expected 3 ix: lines, got %d", ixLines)
	}

	// Should have multiple fac: lines (one per facility presence).
	facLines := 0
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "fac:") {
			facLines++
		}
	}
	if facLines != 2 {
		t.Errorf("expected 2 fac: lines, got %d", facLines)
	}

	// Verify specific IX names appear.
	if !strings.Contains(out, "DE-CIX Frankfurt") {
		t.Error("output missing IX name 'DE-CIX Frankfurt'")
	}
	if !strings.Contains(out, "AMS-IX") {
		t.Error("output missing IX name 'AMS-IX'")
	}
}

func TestRenderWHOIS_IXClass(t *testing.T) {
	t.Parallel()

	data := templates.IXDetail{
		ID:              31,
		Name:            "DE-CIX Frankfurt",
		NameLong:        "Deutscher Commercial Internet Exchange",
		OrgName:         "DE-CIX Management GmbH",
		Website:         "https://de-cix.net",
		City:            "Frankfurt",
		Country:         "DE",
		RegionContinent: "Europe",
		Media:           "Ethernet",
		ProtoUnicast:    true,
		ProtoMulticast:  false,
		ProtoIPv6:       true,
		NetCount:        800,
		FacCount:        35,
		PrefixCount:     12,
		AggregateBW:     5000000,
	}

	out := renderWHOIS(t, "IX", data)

	checks := []string{
		"% Source: PeeringDB-Plus",
		"% Query: IX 31",
		"ix:",
		"ix-name:",
		"descr:",
		"org:",
		"website:",
		"city:",
		"country:",
		"region:",
		"media:",
		"proto:",
		"net-count:",
		"fac-count:",
		"prefix-count:",
		"bandwidth:",
		"source:         PEERINGDB-PLUS",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Proto should include unicast and IPv6 but not multicast.
	protoLine := ""
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "proto:") {
			protoLine = line
			break
		}
	}
	if !strings.Contains(protoLine, "unicast") {
		t.Error("proto line should contain 'unicast'")
	}
	if !strings.Contains(protoLine, "IPv6") {
		t.Error("proto line should contain 'IPv6'")
	}
	if strings.Contains(protoLine, "multicast") {
		t.Error("proto line should NOT contain 'multicast' (disabled)")
	}
}

func TestRenderWHOIS_FacilityClass(t *testing.T) {
	t.Parallel()

	data := templates.FacilityDetail{
		ID:              42,
		Name:            "Equinix FR5",
		NameLong:        "Equinix Frankfurt 5",
		OrgName:         "Equinix",
		Address1:        "Kleyerstrasse 76",
		Address2:        "Building A",
		City:            "Frankfurt",
		Country:         "DE",
		RegionContinent: "Europe",
		CLLI:            "FRKTDECE",
		Website:         "https://equinix.com",
		NetCount:        200,
		IXCount:         5,
		CarrierCount:    10,
	}

	out := renderWHOIS(t, "Facility", data)

	checks := []string{
		"% Source: PeeringDB-Plus",
		"% Query: FAC 42",
		"site:",
		"site-name:",
		"descr:",
		"org:",
		"address:",
		"city:",
		"country:",
		"region:",
		"clli:",
		"website:",
		"net-count:",
		"ix-count:",
		"carrier-count:",
		"source:         PEERINGDB-PLUS",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Address should appear as multi-value (two address: lines).
	addrLines := 0
	for line := range strings.SplitSeq(out, "\n") {
		if strings.HasPrefix(line, "address:") {
			addrLines++
		}
	}
	if addrLines != 2 {
		t.Errorf("expected 2 address: lines, got %d", addrLines)
	}
}

func TestRenderWHOIS_OrgClass(t *testing.T) {
	t.Parallel()

	data := templates.OrgDetail{
		ID:       123,
		Name:     "Cloudflare, Inc.",
		Address1: "101 Townsend Street",
		City:     "San Francisco",
		Country:  "US",
		Website:  "https://cloudflare.com",
		NetCount: 1,
		FacCount: 5,
		IXCount:  10,
	}

	out := renderWHOIS(t, "Org", data)

	checks := []string{
		"% Source: PeeringDB-Plus",
		"% Query: ORG 123",
		"organisation:",
		"org-name:",
		"address:",
		"city:",
		"country:",
		"website:",
		"net-count:",
		"fac-count:",
		"ix-count:",
		"source:         PEERINGDB-PLUS",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderWHOIS_CampusClass(t *testing.T) {
	t.Parallel()

	data := templates.CampusDetail{
		ID:       5,
		Name:     "Ashburn Campus",
		OrgName:  "Equinix",
		City:     "Ashburn",
		Country:  "US",
		Website:  "https://equinix.com",
		FacCount: 8,
	}

	out := renderWHOIS(t, "Campus", data)

	checks := []string{
		"% Source: PeeringDB-Plus",
		"% Query: CAMPUS 5",
		"campus:",
		"campus-name:",
		"org:",
		"city:",
		"country:",
		"website:",
		"fac-count:",
		"source:         PEERINGDB-PLUS",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderWHOIS_CarrierClass(t *testing.T) {
	t.Parallel()

	data := templates.CarrierDetail{
		ID:       7,
		Name:     "Zayo",
		OrgName:  "Zayo Group",
		Website:  "https://zayo.com",
		FacCount: 50,
	}

	out := renderWHOIS(t, "Carrier", data)

	checks := []string{
		"% Source: PeeringDB-Plus",
		"% Query: CARRIER 7",
		"carrier:",
		"carrier-name:",
		"org:",
		"website:",
		"fac-count:",
		"source:         PEERINGDB-PLUS",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderWHOIS_KeyAlignment(t *testing.T) {
	t.Parallel()

	out := renderWHOIS(t, "Network", fullNetwork)

	// Every non-comment, non-blank line should have the key+colon padded to 16 characters.
	// Format: "key:            value" where key+colon is left-aligned within 16 chars.
	keyLineRE := regexp.MustCompile(`^([a-z][a-z0-9-]*:\s+)(\S.*)$`)

	for line := range strings.SplitSeq(out, "\n") {
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}
		m := keyLineRE.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("line does not match WHOIS format: %q", line)
			continue
		}
		keyPart := m[1]
		if len(keyPart) != whoisKeyWidth {
			t.Errorf("key part %q has length %d, want %d", keyPart, len(keyPart), whoisKeyWidth)
		}
	}
}

func TestRenderWHOIS_EmptyFieldsOmitted(t *testing.T) {
	t.Parallel()

	data := templates.NetworkDetail{
		ASN:  64512,
		Name: "Minimal Network",
		// All other fields empty/zero.
	}

	out := renderWHOIS(t, "Network", data)

	// Should NOT contain lines for empty fields.
	if strings.Contains(out, "website:") {
		t.Error("empty website should be omitted")
	}
	if strings.Contains(out, "irr-as-set:") {
		t.Error("empty irr-as-set should be omitted")
	}
	if strings.Contains(out, "org:") {
		t.Error("empty org should be omitted")
	}

	// Should still contain the mandatory fields.
	if !strings.Contains(out, "aut-num:") {
		t.Error("output should contain aut-num even for minimal network")
	}
	if !strings.Contains(out, "as-name:") {
		t.Error("output should contain as-name even for minimal network")
	}
	if !strings.Contains(out, "source:") {
		t.Error("output should contain source")
	}
}

func TestRenderWHOIS_UnsupportedView(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data any
	}{
		{
			name: "SearchGroups",
			data: []templates.SearchGroup{
				{TypeName: "Networks", Results: []templates.SearchResult{{Name: "Test"}}},
			},
		},
		{
			name: "CompareData",
			data: &templates.CompareData{
				NetA: templates.CompareNetwork{ASN: 13335, Name: "Cloudflare"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := renderWHOIS(t, tt.name, tt.data)
			if !strings.Contains(out, "WHOIS format is not available") {
				t.Errorf("unsupported view should contain 'WHOIS format is not available', got: %q", out)
			}
		})
	}
}

func TestRenderWHOIS_NoANSICodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data any
	}{
		{"Network", fullNetwork},
		{"IX", templates.IXDetail{ID: 1, Name: "Test IX", NetCount: 10}},
		{"Facility", templates.FacilityDetail{ID: 1, Name: "Test Fac"}},
		{"Org", templates.OrgDetail{ID: 1, Name: "Test Org"}},
		{"Campus", templates.CampusDetail{ID: 1, Name: "Test Campus"}},
		{"Carrier", templates.CarrierDetail{ID: 1, Name: "Test Carrier"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			out := renderWHOIS(t, tt.name, tt.data)
			if whoisAnsiRE.MatchString(out) {
				t.Errorf("WHOIS output should not contain ANSI escape codes, got: %q", out[:min(len(out), 200)])
			}
		})
	}
}
