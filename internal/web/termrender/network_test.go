package termrender

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// ansiRE strips ANSI escape sequences for test assertions.
var ansiRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestFormatSpeed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mbps int
		want string
	}{
		{0, "0M"},
		{500, "500M"},
		{1000, "1G"},
		{10000, "10G"},
		{100000, "100G"},
		{400000, "400G"},
		{1000000, "1T"},
		{2500000, "2T"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := FormatSpeed(tt.mbps)
			if got != tt.want {
				t.Errorf("FormatSpeed(%d) = %q, want %q", tt.mbps, got, tt.want)
			}
		})
	}
}

func TestFormatBandwidth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mbps int
		want string
	}{
		{0, "0 Mbps"},
		{500, "500 Mbps"},
		{1000, "1 Gbps"},
		{10000, "10 Gbps"},
		{100000, "100 Gbps"},
		{1000000, "1.0 Tbps"},
		{1500000, "1.5 Tbps"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			got := FormatBandwidth(tt.mbps)
			if got != tt.want {
				t.Errorf("FormatBandwidth(%d) = %q, want %q", tt.mbps, got, tt.want)
			}
		})
	}
}

func TestSpeedStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mbps int
	}{
		{"sub-1G", 500},
		{"1G", 1000},
		{"10G", 10000},
		{"100G", 100000},
		{"400G", 400000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			style := SpeedStyle(tt.mbps)
			rendered := style.Render("X")
			if !strings.Contains(rendered, "\x1b[") {
				t.Errorf("SpeedStyle(%d).Render() should contain ANSI codes, got: %q", tt.mbps, rendered)
			}
		})
	}

	// Verify 400G+ tier produces bold.
	t.Run("400G-bold", func(t *testing.T) {
		t.Parallel()
		style := SpeedStyle(400000)
		rendered := style.Render("X")
		if !strings.Contains(rendered, "\x1b[1") {
			t.Errorf("SpeedStyle(400000) should produce bold, got: %q", rendered)
		}
	})
}

func TestPolicyStyle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		policy   string
		wantANSI bool
	}{
		{"Open", "Open", true},
		{"Selective", "Selective", true},
		{"Restrictive", "Restrictive", true},
		{"open-lower", "open", true},
		{"OPEN-upper", "OPEN", true},
		{"Unknown", "Unknown", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := PolicyStyle(tt.policy)
			hasANSI := strings.Contains(got, "\x1b[")
			if tt.wantANSI && !hasANSI {
				t.Errorf("PolicyStyle(%q) should contain ANSI codes, got: %q", tt.policy, got)
			}
			// All outputs must contain the original policy text.
			if tt.policy != "" && !strings.Contains(got, tt.policy) {
				t.Errorf("PolicyStyle(%q) should contain policy text, got: %q", tt.policy, got)
			}
		})
	}

	// Verify case-insensitive matching produces colored output.
	t.Run("case-insensitive", func(t *testing.T) {
		t.Parallel()
		openLower := PolicyStyle("open")
		openTitle := PolicyStyle("Open")
		// Both should contain ANSI codes (not fall through to default).
		if !strings.Contains(openLower, "\x1b[") {
			t.Errorf("PolicyStyle(\"open\") should contain ANSI codes, got: %q", openLower)
		}
		if !strings.Contains(openTitle, "\x1b[") {
			t.Errorf("PolicyStyle(\"Open\") should contain ANSI codes, got: %q", openTitle)
		}
	})
}

func TestCrossRef(t *testing.T) {
	t.Parallel()

	got := CrossRef("/ui/ix/31")
	stripped := ansiRE.ReplaceAllString(got, "")
	if !strings.Contains(stripped, "[/ui/ix/31]") {
		t.Errorf("CrossRef(\"/ui/ix/31\") stripped text should contain \"[/ui/ix/31]\", got raw: %q stripped: %q", got, stripped)
	}
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("CrossRef() output should contain ANSI codes, got: %q", got)
	}
}

func TestWriteKV(t *testing.T) {
	t.Parallel()

	t.Run("normal", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		writeKV(&buf, "Label", "Value", 10)
		got := buf.String()
		if !strings.Contains(got, "Label") {
			t.Errorf("writeKV output should contain label, got: %q", got)
		}
		if !strings.Contains(got, "Value") {
			t.Errorf("writeKV output should contain value, got: %q", got)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("writeKV output should end with newline, got: %q", got)
		}
	})

	t.Run("empty-value", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		writeKV(&buf, "Label", "", 10)
		if buf.Len() != 0 {
			t.Errorf("writeKV with empty value should produce no output, got: %q", buf.String())
		}
	})

	t.Run("alignment", func(t *testing.T) {
		t.Parallel()
		var buf strings.Builder
		writeKV(&buf, "X", "val", 8)
		got := buf.String()
		// The label should be right-padded to 8 chars (contains spaces before X).
		// Output has ANSI codes around the label, so just check it contains "  " separator.
		if !strings.Contains(got, "  ") {
			t.Errorf("writeKV output should contain two-space separator, got: %q", got)
		}
	})
}

// --- Test fixtures for RenderNetworkDetail ---

// fullNetwork is a fully-populated network detail with diverse IX and facility data.
var fullNetwork = templates.NetworkDetail{
	ID:            1,
	ASN:           13335,
	Name:          "Cloudflare",
	NameLong:      "Cloudflare, Inc.",
	Website:       "https://cloudflare.com",
	OrgName:       "Cloudflare, Inc.",
	OrgID:         16,
	IRRAsSet:      "AS-CLOUDFLARE",
	InfoType:      "NSP",
	InfoScope:     "Global",
	InfoTraffic:   "100+ Gbps",
	InfoRatio:     "Mostly Outbound",
	InfoUnicast:   true,
	InfoIPv6:      true,
	InfoPrefixes4: 600,
	InfoPrefixes6: 200,
	LookingGlass:  "https://lg.cloudflare.com",
	PolicyGeneral: "Open",
	PolicyURL:     "https://cloudflare.com/peering",
	Status:        "ok",
	IXCount:       3,
	FacCount:      2,
	AggregateBW:   210000,
	IXPresences: []templates.NetworkIXLanRow{
		{
			IXName:   "DE-CIX Frankfurt",
			IXID:     31,
			Speed:    100000,
			IPAddr4:  "80.81.192.123",
			IPAddr6:  "2001:7f8::3337:0:1",
			IsRSPeer: true,
		},
		{
			IXName:   "AMS-IX",
			IXID:     26,
			Speed:    100000,
			IPAddr4:  "80.249.211.123",
			IPAddr6:  "2001:7f8:1::a500:1333:1",
			IsRSPeer: false,
		},
		{
			IXName:   "LINX LON1",
			IXID:     18,
			Speed:    10000,
			IPAddr4:  "195.66.224.123",
			IPAddr6:  "",
			IsRSPeer: false,
		},
	},
	FacPresences: []templates.NetworkFacRow{
		{
			FacName: "Equinix FR5",
			FacID:   42,
			City:    "Frankfurt",
			Country: "DE",
		},
		{
			FacName: "Unknown Facility",
			FacID:   0,
			City:    "London",
			Country: "GB",
		},
	},
}

// emptyNetwork has only Name and ASN, with no IX presences, no facilities, and zero bandwidth.
var emptyNetwork = templates.NetworkDetail{
	Name:          "Empty Network",
	ASN:           99999,
	PolicyGeneral: "Open",
}

// renderNetworkDetail is a test helper that renders a NetworkDetail and returns the output string.
func renderNetworkDetail(t *testing.T, mode RenderMode, noColor bool, data templates.NetworkDetail) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderNetworkDetail(&buf, data); err != nil {
		t.Fatalf("RenderNetworkDetail() error: %v", err)
	}
	return buf.String()
}

func TestRenderNetworkDetail_Header(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, fullNetwork)

	checks := []string{
		"Cloudflare",
		"AS13335",
		"NSP",
		"Open",
		"https://cloudflare.com",
		"Cloudflare, Inc.",
		"AS-CLOUDFLARE",
		"100+ Gbps",
		"Mostly Outbound",
		"Global",
		"600",
		"200",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderNetworkDetail_PolicyColors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		policy string
	}{
		{"Open", "Open"},
		{"Selective", "Selective"},
		{"Restrictive", "Restrictive"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			data := templates.NetworkDetail{
				Name:          "Test Net",
				ASN:           64512,
				PolicyGeneral: tt.policy,
			}
			out := renderNetworkDetail(t, ModeRich, false, data)
			if !strings.Contains(out, tt.policy) {
				t.Errorf("output missing policy text %q", tt.policy)
			}
			if !strings.Contains(out, "\x1b[") {
				t.Errorf("output missing ANSI codes for policy %q", tt.policy)
			}
		})
	}
}

func TestRenderNetworkDetail_IXPresences(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, fullNetwork)

	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"DE-CIX Frankfurt",
		"[/ui/ix/31]",
		"100G",
		"80.81.192.123",
		"2001:7f8::3337:0:1",
		"AMS-IX",
		"[/ui/ix/26]",
		"LINX LON1",
		"[/ui/ix/18]",
		"10G",
		"195.66.224.123",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderNetworkDetail_RSBadge(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, fullNetwork)

	// The [RS] badge should appear in the output (DE-CIX Frankfurt has IsRSPeer=true).
	stripped := ansiRE.ReplaceAllString(out, "")
	if !strings.Contains(stripped, "[RS]") {
		t.Error("output missing [RS] badge for route server peer")
	}

	// Split output into lines and verify [RS] appears on the DE-CIX line.
	lines := strings.Split(stripped, "\n")
	foundRS := false
	for _, line := range lines {
		if strings.Contains(line, "DE-CIX Frankfurt") && strings.Contains(line, "[RS]") {
			foundRS = true
			break
		}
	}
	if !foundRS {
		t.Error("[RS] badge not found on DE-CIX Frankfurt line")
	}

	// AMS-IX should NOT have [RS].
	for _, line := range lines {
		if strings.Contains(line, "AMS-IX") && strings.Contains(line, "[RS]") {
			t.Error("AMS-IX should not have [RS] badge")
		}
	}
}

func TestRenderNetworkDetail_SpeedColors(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, fullNetwork)

	// In Rich mode, speed values should have ANSI codes.
	if !strings.Contains(out, "\x1b[") {
		t.Error("Rich mode output should contain ANSI codes")
	}

	// 100G and 10G should both appear.
	if !strings.Contains(out, "100G") {
		t.Error("output missing 100G speed")
	}
	if !strings.Contains(out, "10G") {
		t.Error("output missing 10G speed")
	}
}

func TestRenderNetworkDetail_CrossRefs(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, fullNetwork)
	stripped := ansiRE.ReplaceAllString(out, "")

	if !strings.Contains(stripped, "/ui/ix/") {
		t.Error("output missing IX cross-reference path /ui/ix/")
	}
	if !strings.Contains(stripped, "/ui/fac/") {
		t.Error("output missing facility cross-reference path /ui/fac/")
	}

	// Verify specific IDs.
	if !strings.Contains(stripped, "[/ui/ix/31]") {
		t.Error("output missing cross-reference [/ui/ix/31]")
	}
	if !strings.Contains(stripped, "[/ui/fac/42]") {
		t.Error("output missing cross-reference [/ui/fac/42]")
	}
}

func TestRenderNetworkDetail_Facilities(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, fullNetwork)
	stripped := ansiRE.ReplaceAllString(out, "")

	checks := []string{
		"Equinix FR5",
		"[/ui/fac/42]",
		"Frankfurt",
		"DE",
		"Unknown Facility",
		"London",
		"GB",
	}

	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderNetworkDetail_EmptyNetwork(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, emptyNetwork)
	stripped := ansiRE.ReplaceAllString(out, "")

	// Section headers include count in parentheses: "IX Presences (N)".
	// These sections should not be rendered for empty presences.
	if strings.Contains(stripped, "IX Presences (") {
		t.Error("empty network should not contain IX Presences section header")
	}
	if strings.Contains(stripped, "Facilities (") {
		t.Error("empty network should not contain Facilities section header")
	}

	// Should still have name and ASN.
	if !strings.Contains(out, "Empty Network") {
		t.Error("output missing network name")
	}
	if !strings.Contains(out, "AS99999") {
		t.Error("output missing ASN")
	}
}

func TestRenderNetworkDetail_ZeroBandwidth(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, emptyNetwork)

	if strings.Contains(out, "Aggregate Bandwidth") {
		t.Error("zero aggregate bandwidth should omit 'Aggregate Bandwidth' from output")
	}
}

func TestRenderNetworkDetail_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModePlain, false, fullNetwork)

	// Plain mode should NOT contain any ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("plain mode output should not contain ANSI escape codes")
	}

	// But all text content should still be present.
	checks := []string{
		"Cloudflare",
		"AS13335",
		"NSP",
		"Open",
		"DE-CIX Frankfurt",
		"AMS-IX",
		"Equinix FR5",
		"100G",
		"10G",
	}

	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("plain mode output missing text %q", want)
		}
	}
}

func TestRenderNetworkDetail_OmitEmptyFields(t *testing.T) {
	t.Parallel()

	data := templates.NetworkDetail{
		Name:          "Sparse Network",
		ASN:           64513,
		InfoType:      "NSP",
		PolicyGeneral: "Open",
		// Website, IRRAsSet, LookingGlass, RouteServer are all empty.
	}

	out := renderNetworkDetail(t, ModeRich, false, data)
	stripped := ansiRE.ReplaceAllString(out, "")

	if strings.Contains(stripped, "Website") {
		t.Error("empty Website should be omitted from output")
	}
	if strings.Contains(stripped, "IRR AS-SET") {
		t.Error("empty IRR AS-SET should be omitted from output")
	}
	if strings.Contains(stripped, "Looking Glass") {
		t.Error("empty Looking Glass should be omitted from output")
	}
	if strings.Contains(stripped, "Route Server") {
		t.Error("empty Route Server should be omitted from output")
	}

	// Type should still be present.
	if !strings.Contains(stripped, "NSP") {
		t.Error("output missing InfoType 'NSP'")
	}
}

func TestRenderNetworkDetail_FacNoCrossRef(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, fullNetwork)
	stripped := ansiRE.ReplaceAllString(out, "")

	// Unknown Facility has FacID=0, should not have a cross-ref.
	lines := strings.Split(stripped, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Unknown Facility") {
			if strings.Contains(line, "[/ui/fac/0]") {
				t.Error("facility with FacID=0 should not have cross-reference [/ui/fac/0]")
			}
			if strings.Contains(line, "/ui/fac/") {
				t.Error("facility with FacID=0 should not have any /ui/fac/ path")
			}
			break
		}
	}

	// Equinix FR5 (FacID=42) should have cross-ref.
	for _, line := range lines {
		if strings.Contains(line, "Equinix FR5") {
			if !strings.Contains(line, "[/ui/fac/42]") {
				t.Error("Equinix FR5 should have cross-reference [/ui/fac/42]")
			}
			break
		}
	}
}

func BenchmarkRenderNetworkDetail_LargeIX(b *testing.B) {
	data := templates.NetworkDetail{Name: "Large Network", ASN: 99999}
	for i := range 1000 {
		data.IXPresences = append(data.IXPresences, templates.NetworkIXLanRow{
			IXName:   fmt.Sprintf("IX-%d", i),
			IXID:     i,
			Speed:    100000,
			IPAddr4:  "192.0.2.1",
			IPAddr6:  "2001:db8::1",
			IsRSPeer: i%3 == 0,
		})
	}
	r := NewRenderer(ModeRich, false)
	b.ResetTimer()
	for range b.N {
		var buf bytes.Buffer
		_ = r.RenderNetworkDetail(&buf, data)
	}
}
