package termrender

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// fullIX is a fully-populated IX detail with participants, facilities, and prefixes.
var fullIX = templates.IXDetail{
	ID:              31,
	Name:            "DE-CIX Frankfurt",
	NameLong:        "Deutscher Commercial Internet Exchange",
	Website:         "https://de-cix.net",
	OrgName:         "DE-CIX Management GmbH",
	OrgID:           42,
	City:            "Frankfurt",
	Country:         "DE",
	RegionContinent: "Europe",
	Media:           "Ethernet",
	ProtoUnicast:    true,
	ProtoMulticast:  false,
	ProtoIPv6:       true,
	Status:          "ok",
	NetCount:        800,
	FacCount:        5,
	PrefixCount:     4,
	AggregateBW:     310000,
	Participants: []templates.IXParticipantRow{
		{
			NetName:  "Cloudflare",
			ASN:      13335,
			Speed:    100000,
			IPAddr4:  "80.81.192.123",
			IPAddr6:  "2001:7f8::3337:0:1",
			IsRSPeer: true,
		},
		{
			NetName:  "Google",
			ASN:      15169,
			Speed:    100000,
			IPAddr4:  "80.81.192.200",
			IPAddr6:  "2001:7f8::3b41:0:1",
			IsRSPeer: false,
		},
		{
			NetName:  "Small Network",
			ASN:      64512,
			Speed:    10000,
			IPAddr4:  "80.81.193.50",
			IPAddr6:  "",
			IsRSPeer: true,
		},
	},
	Facilities: []templates.IXFacilityRow{
		{FacName: "Equinix FR5", FacID: 42, City: "Frankfurt", Country: "DE"},
		{FacName: "Interxion FRA1", FacID: 99, City: "Frankfurt", Country: "DE"},
	},
	Prefixes: []templates.IXPrefixRow{
		{Prefix: "80.81.192.0/22", Protocol: "IPv4", InDFZ: true},
		{Prefix: "2001:7f8::/64", Protocol: "IPv6", InDFZ: true},
		{Prefix: "185.1.47.0/24", Protocol: "IPv4", InDFZ: false},
		{Prefix: "2001:7f8:1::/48", Protocol: "IPv6", InDFZ: false},
	},
}

// emptyIX has only basic header fields, no participants, facilities, or prefixes.
var emptyIX = templates.IXDetail{
	Name:    "Empty IX",
	City:    "Nowhere",
	Country: "XX",
}

// renderIXDetail is a test helper that renders an IXDetail and returns the output string.
func renderIXDetail(t *testing.T, mode RenderMode, noColor bool, data templates.IXDetail) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderIXDetail(&buf, data); err != nil {
		t.Fatalf("RenderIXDetail() error: %v", err)
	}
	return buf.String()
}

func TestRenderIXDetail_Header(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, fullIX)

	checks := []string{
		"DE-CIX Frankfurt",
		"Frankfurt",
		"DE",
		"DE-CIX Management GmbH",
		"https://de-cix.net",
		"Europe",
		"Ethernet",
		"800",
		"5",
		"4",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderIXDetail_Protocols(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, fullIX)
	stripped := ansiRE.ReplaceAllString(out, "")

	// ProtoUnicast=true, ProtoMulticast=false, ProtoIPv6=true
	if !strings.Contains(stripped, "unicast") {
		t.Error("output missing unicast protocol")
	}
	if !strings.Contains(stripped, "IPv6") {
		t.Error("output missing IPv6 protocol")
	}
	if strings.Contains(stripped, "multicast") {
		t.Error("output should not contain multicast (ProtoMulticast=false)")
	}
}

func TestRenderIXDetail_Participants(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, fullIX)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Cloudflare",
		"[/ui/asn/13335]",
		"100G",
		"80.81.192.123",
		"2001:7f8::3337:0:1",
		"Google",
		"[/ui/asn/15169]",
		"Small Network",
		"[/ui/asn/64512]",
		"10G",
		"Participants (3)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderIXDetail_RSBadge(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, fullIX)
	stripped := ansiRE.ReplaceAllString(out, "")

	// Cloudflare has IsRSPeer=true.
	lines := strings.Split(stripped, "\n")
	foundRS := false
	for _, line := range lines {
		if strings.Contains(line, "Cloudflare") && strings.Contains(line, "[RS]") {
			foundRS = true
			break
		}
	}
	if !foundRS {
		t.Error("[RS] badge not found on Cloudflare line")
	}

	// Google should NOT have [RS] (IsRSPeer=false).
	for _, line := range lines {
		if strings.Contains(line, "Google") && strings.Contains(line, "[RS]") {
			t.Error("Google should not have [RS] badge")
		}
	}
}

func TestRenderIXDetail_Facilities(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, fullIX)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Equinix FR5",
		"[/ui/fac/42]",
		"Interxion FRA1",
		"[/ui/fac/99]",
		"Facilities (2)",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderIXDetail_Prefixes(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, fullIX)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"80.81.192.0/22",
		"2001:7f8::/64",
		"Prefixes (4)",
		"[DFZ]",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Verify non-DFZ prefixes have a not-in-DFZ indicator.
	if !strings.Contains(stripped, "not in DFZ") {
		t.Error("output missing 'not in DFZ' indicator for non-DFZ prefixes")
	}
}

func TestRenderIXDetail_EmptyIX(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, emptyIX)
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "Empty IX") {
		t.Error("output missing IX name")
	}
	if strings.Contains(stripped, "Participants (") {
		t.Error("empty IX should not contain Participants section")
	}
	if strings.Contains(stripped, "Facilities (") {
		t.Error("empty IX should not contain Facilities section")
	}
	if strings.Contains(stripped, "Prefixes (") {
		t.Error("empty IX should not contain Prefixes section")
	}
}

func TestRenderIXDetail_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModePlain, false, fullIX)

	// Plain mode should have no ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("Plain mode output should not contain ANSI escape codes")
	}

	// Should still contain the IX name and key data.
	if !strings.Contains(out, "DE-CIX Frankfurt") {
		t.Error("output missing IX name in plain mode")
	}
	if !strings.Contains(out, "Cloudflare") {
		t.Error("output missing participant name in plain mode")
	}
}

func TestRenderIXDetail_AggregateBandwidth(t *testing.T) {
	t.Parallel()

	out := renderIXDetail(t, ModeRich, false, fullIX)

	if !strings.Contains(out, "310 Gbps") {
		t.Errorf("output missing aggregate bandwidth '310 Gbps', got:\n%s", out)
	}
}

func TestFormatProtocols(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		unicast   bool
		multicast bool
		ipv6      bool
		want      string
	}{
		{"all", true, true, true, "unicast, multicast, IPv6"},
		{"unicast+ipv6", true, false, true, "unicast, IPv6"},
		{"unicast-only", true, false, false, "unicast"},
		{"none", false, false, false, ""},
		{"multicast-only", false, true, false, "multicast"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := formatProtocols(tt.unicast, tt.multicast, tt.ipv6)
			if got != tt.want {
				t.Errorf("formatProtocols(%v, %v, %v) = %q, want %q",
					tt.unicast, tt.multicast, tt.ipv6, got, tt.want)
			}
		})
	}
}
