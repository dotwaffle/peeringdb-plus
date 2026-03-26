package termrender

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// --- Test fixtures for RenderCompare ---

// fullCompareData is a complete comparison with shared IXPs, facilities, and campuses.
var fullCompareData = &templates.CompareData{
	NetA: templates.CompareNetwork{ASN: 13335, Name: "Cloudflare", ID: 1},
	NetB: templates.CompareNetwork{ASN: 15169, Name: "Google", ID: 2},
	SharedIXPs: []templates.CompareIXP{
		{
			IXID:   31,
			IXName: "DE-CIX Frankfurt",
			Shared: true,
			NetA: &templates.CompareIXPresence{
				Speed:       100000,
				IPAddr4:     "80.81.192.123",
				IPAddr6:     "2001:7f8::3337:0:1",
				IsRSPeer:    true,
				Operational: true,
			},
			NetB: &templates.CompareIXPresence{
				Speed:       100000,
				IPAddr4:     "80.81.192.200",
				IPAddr6:     "2001:7f8::3b41:0:1",
				IsRSPeer:    false,
				Operational: true,
			},
		},
		{
			IXID:   26,
			IXName: "AMS-IX",
			Shared: true,
			NetA: &templates.CompareIXPresence{
				Speed:       10000,
				IPAddr4:     "80.249.211.123",
				IPAddr6:     "",
				IsRSPeer:    false,
				Operational: true,
			},
			NetB: &templates.CompareIXPresence{
				Speed:       10000,
				IPAddr4:     "80.249.211.200",
				IPAddr6:     "2001:7f8:1::a500:1516:1",
				IsRSPeer:    true,
				Operational: true,
			},
		},
	},
	SharedFacilities: []templates.CompareFacility{
		{
			FacID:   42,
			FacName: "Equinix FR5",
			City:    "Frankfurt",
			Country: "DE",
			Shared:  true,
			NetA:    &templates.CompareFacPresence{LocalASN: 13335},
			NetB:    &templates.CompareFacPresence{LocalASN: 15169},
		},
		{
			FacID:   4,
			FacName: "Equinix AM1/AM2",
			City:    "Amsterdam",
			Country: "NL",
			Shared:  true,
			NetA:    &templates.CompareFacPresence{LocalASN: 13335},
			NetB:    &templates.CompareFacPresence{LocalASN: 15169},
		},
	},
	SharedCampuses: []templates.CompareCampus{
		{
			CampusID:   10,
			CampusName: "Equinix Amsterdam",
			SharedFacilities: []templates.CompareCampusFacility{
				{FacID: 4, FacName: "Equinix AM1/AM2"},
			},
		},
	},
	ViewMode: "shared",
}

// emptyCompareData has two networks but no shared resources.
var emptyCompareData = &templates.CompareData{
	NetA:     templates.CompareNetwork{ASN: 64512, Name: "Network A", ID: 100},
	NetB:     templates.CompareNetwork{ASN: 64513, Name: "Network B", ID: 101},
	ViewMode: "shared",
}

// renderCompare is a test helper that renders comparison data and returns the output string.
func renderCompare(t *testing.T, mode RenderMode, noColor bool, data *templates.CompareData) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderCompare(&buf, data); err != nil {
		t.Fatalf("RenderCompare() error: %v", err)
	}
	return buf.String()
}

func TestRenderCompare_Title(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModeRich, false, fullCompareData)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Title should contain both network names and ASNs.
	if !strings.Contains(stripped, "Cloudflare") {
		t.Error("title missing network A name 'Cloudflare'")
	}
	if !strings.Contains(stripped, "AS13335") {
		t.Error("title missing network A ASN 'AS13335'")
	}
	if !strings.Contains(stripped, "Google") {
		t.Error("title missing network B name 'Google'")
	}
	if !strings.Contains(stripped, "AS15169") {
		t.Error("title missing network B ASN 'AS15169'")
	}
	if !strings.Contains(stripped, "vs") {
		t.Error("title missing 'vs' separator")
	}
}

func TestRenderCompare_SharedIXPs(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModeRich, false, fullCompareData)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Section header with count.
	if !strings.Contains(stripped, "Shared IXPs (2)") {
		t.Error("output missing 'Shared IXPs (2)' section header")
	}

	// IX names and cross-references.
	checks := []string{
		"DE-CIX Frankfurt",
		"[/ui/ix/31]",
		"AMS-IX",
		"[/ui/ix/26]",
	}
	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}

	// Per-network presence details.
	if !strings.Contains(stripped, "80.81.192.123") {
		t.Error("output missing NetA IPv4 at DE-CIX")
	}
	if !strings.Contains(stripped, "2001:7f8::3337:0:1") {
		t.Error("output missing NetA IPv6 at DE-CIX")
	}
	if !strings.Contains(stripped, "80.81.192.200") {
		t.Error("output missing NetB IPv4 at DE-CIX")
	}

	// Speed should appear.
	if !strings.Contains(stripped, "100G") {
		t.Error("output missing 100G speed")
	}
	if !strings.Contains(stripped, "10G") {
		t.Error("output missing 10G speed")
	}

	// RS badge should appear for DE-CIX NetA (IsRSPeer=true).
	if !strings.Contains(stripped, "[RS]") {
		t.Error("output missing [RS] badge")
	}
}

func TestRenderCompare_SharedFacilities(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModeRich, false, fullCompareData)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Section header.
	if !strings.Contains(stripped, "Shared Facilities (2)") {
		t.Error("output missing 'Shared Facilities (2)' section header")
	}

	// Facility names, cross-refs, and locations.
	checks := []string{
		"Equinix FR5",
		"[/ui/fac/42]",
		"Frankfurt, DE",
		"Equinix AM1/AM2",
		"[/ui/fac/4]",
		"Amsterdam, NL",
	}
	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderCompare_SharedCampuses(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModeRich, false, fullCompareData)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Section header.
	if !strings.Contains(stripped, "Shared Campuses (1)") {
		t.Error("output missing 'Shared Campuses (1)' section header")
	}

	// Campus name and cross-ref.
	if !strings.Contains(stripped, "Equinix Amsterdam") {
		t.Error("output missing campus name 'Equinix Amsterdam'")
	}
	if !strings.Contains(stripped, "[/ui/campus/10]") {
		t.Error("output missing campus cross-ref '[/ui/campus/10]'")
	}

	// Nested facility.
	if !strings.Contains(stripped, "Equinix AM1/AM2") {
		t.Error("output missing nested facility 'Equinix AM1/AM2'")
	}
	if !strings.Contains(stripped, "[/ui/fac/4]") {
		t.Error("output missing nested facility cross-ref '[/ui/fac/4]'")
	}
}

func TestRenderCompare_EmptyComparison(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModeRich, false, emptyCompareData)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Should show "no shared" messages.
	if !strings.Contains(stripped, "No shared IXPs") {
		t.Error("empty comparison should show 'No shared IXPs'")
	}
	if !strings.Contains(stripped, "No shared facilities") {
		t.Error("empty comparison should show 'No shared facilities'")
	}
	if !strings.Contains(stripped, "No shared campuses") {
		t.Error("empty comparison should show 'No shared campuses'")
	}

	// Title should still show both networks.
	if !strings.Contains(stripped, "Network A") {
		t.Error("empty comparison should show Network A name")
	}
	if !strings.Contains(stripped, "Network B") {
		t.Error("empty comparison should show Network B name")
	}
}

func TestRenderCompare_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModePlain, false, fullCompareData)

	// Plain mode should NOT contain any ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("plain mode output should not contain ANSI escape codes")
	}

	// But all text content should still be present.
	checks := []string{
		"Cloudflare",
		"Google",
		"DE-CIX Frankfurt",
		"AMS-IX",
		"Equinix FR5",
		"Equinix Amsterdam",
		"100G",
		"10G",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("plain mode output missing text %q", want)
		}
	}
}

func TestRenderCompare_NilData(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModeRich, false, nil)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	if !strings.Contains(stripped, "No comparison data available") {
		t.Error("nil data should show 'No comparison data available' message")
	}
}

func TestRenderCompare_IXPresencePerNetwork(t *testing.T) {
	t.Parallel()

	out := renderCompare(t, ModeRich, false, fullCompareData)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Both networks should have labeled presence lines.
	if !strings.Contains(stripped, "AS13335:") {
		t.Error("output missing per-network label 'AS13335:'")
	}
	if !strings.Contains(stripped, "AS15169:") {
		t.Error("output missing per-network label 'AS15169:'")
	}
}
