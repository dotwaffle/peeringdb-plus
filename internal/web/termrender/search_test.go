package termrender

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// --- Test fixtures for RenderSearch ---

// fullSearchGroups is a multi-type search result set with diverse entity types.
var fullSearchGroups = []templates.SearchGroup{
	{
		TypeName:    "Networks",
		TypeSlug:    "net",
		AccentColor: "emerald",
		HasMore:     true,
		Results: []templates.SearchResult{
			{Name: "Equinix (WAN)", ASN: 47541, DetailURL: "/ui/asn/47541"},
			{Name: "Equinix LLC", ASN: 21928, DetailURL: "/ui/asn/21928"},
		},
	},
	{
		TypeName:    "IXPs",
		TypeSlug:    "ix",
		AccentColor: "sky",
		HasMore:     true,
		Results: []templates.SearchResult{
			{Name: "Equinix Chicago", Country: "US", City: "Chicago", DetailURL: "/ui/ix/81"},
		},
	},
	{
		TypeName:    "Facilities",
		TypeSlug:    "fac",
		AccentColor: "orange",
		HasMore:     true,
		Results: []templates.SearchResult{
			{Name: "Equinix AM1/AM2", Country: "NL", City: "Amsterdam", DetailURL: "/ui/fac/4"},
		},
	},
}

// emptySearchGroups is an empty result set.
var emptySearchGroups []templates.SearchGroup

// singleSearchGroup is a single-type result set.
var singleSearchGroup = []templates.SearchGroup{
	{
		TypeName: "Networks",
		TypeSlug: "net",
		HasMore:  false,
		Results: []templates.SearchResult{
			{Name: "Test Network", ASN: 64512, DetailURL: "/ui/asn/64512"},
		},
	},
}

// renderSearch is a test helper that renders search results and returns the output string.
func renderSearch(t *testing.T, mode RenderMode, noColor bool, groups []templates.SearchGroup) string {
	t.Helper()
	r := NewRenderer(mode, noColor)
	var buf bytes.Buffer
	if err := r.RenderSearch(&buf, groups); err != nil {
		t.Fatalf("RenderSearch() error: %v", err)
	}
	return buf.String()
}

func TestRenderSearch_GroupedOutput(t *testing.T) {
	t.Parallel()

	out := renderSearch(t, ModeRich, false, fullSearchGroups)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// All type names should appear as group headers.
	checks := []string{
		"Networks",
		"IXPs",
		"Facilities",
	}
	for _, want := range checks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing group header %q", want)
		}
	}

	// All result names should appear.
	resultChecks := []string{
		"Equinix (WAN)",
		"Equinix LLC",
		"Equinix Chicago",
		"Equinix AM1/AM2",
	}
	for _, want := range resultChecks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing result name %q", want)
		}
	}

	// All metadata (ASN, country, city) should appear.
	metadataChecks := []string{
		"AS47541",
		"AS21928",
		"US",
		"Chicago",
		"NL",
		"Amsterdam",
	}
	for _, want := range metadataChecks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing metadata %q", want)
		}
	}

	// All detail URLs should appear.
	urlChecks := []string{
		"/ui/asn/47541",
		"/ui/asn/21928",
		"/ui/ix/81",
		"/ui/fac/4",
	}
	for _, want := range urlChecks {
		if !strings.Contains(stripped, want) {
			t.Errorf("output missing detail URL %q", want)
		}
	}
}

func TestRenderSearch_HasMoreInHeader(t *testing.T) {
	t.Parallel()

	out := renderSearch(t, ModeRich, false, fullSearchGroups)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Headers should show len(Results) with "+" suffix when HasMore is true.
	// Networks has 2 results and HasMore=true -> "2+ results".
	if !strings.Contains(stripped, "2+ results") {
		t.Error("Networks header should show '2+ results' (HasMore=true)")
	}
	if !strings.Contains(stripped, "1+ results") {
		t.Error("IXPs/Facilities header should show '1+ results' (HasMore=true)")
	}
}

func TestRenderSearch_EmptyResults(t *testing.T) {
	t.Parallel()

	out := renderSearch(t, ModeRich, false, emptySearchGroups)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	if !strings.Contains(stripped, "No results found") {
		t.Error("empty search should show 'No results found' message")
	}
}

func TestRenderSearch_SingleGroup(t *testing.T) {
	t.Parallel()

	out := renderSearch(t, ModeRich, false, singleSearchGroup)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	if !strings.Contains(stripped, "Networks") {
		t.Error("output missing group header 'Networks'")
	}
	if !strings.Contains(stripped, "Test Network") {
		t.Error("output missing result name 'Test Network'")
	}
	if !strings.Contains(stripped, "AS64512") {
		t.Error("output missing ASN 'AS64512'")
	}
	if !strings.Contains(stripped, "/ui/asn/64512") {
		t.Error("output missing detail URL '/ui/asn/64512'")
	}
}

func TestRenderSearch_PlainMode(t *testing.T) {
	t.Parallel()

	out := renderSearch(t, ModePlain, false, fullSearchGroups)

	// Plain mode should NOT contain any ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("plain mode output should not contain ANSI escape codes")
	}

	// But all text content should still be present.
	checks := []string{
		"Networks",
		"IXPs",
		"Facilities",
		"Equinix (WAN)",
		"Equinix LLC",
		"Equinix Chicago",
		"Equinix AM1/AM2",
		"AS47541",
		"/ui/asn/47541",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("plain mode output missing text %q", want)
		}
	}
}

func TestRenderSearch_NoColorMode(t *testing.T) {
	t.Parallel()

	out := renderSearch(t, ModeRich, true, fullSearchGroups)

	// noColor should suppress all ANSI escape codes.
	if strings.Contains(out, "\x1b[") {
		t.Error("noColor output should not contain ANSI escape codes")
	}

	// Content should still be present.
	if !strings.Contains(out, "Equinix (WAN)") {
		t.Error("noColor output missing result name")
	}
}

func TestRenderSearch_ResultLineFormat(t *testing.T) {
	t.Parallel()

	out := renderSearch(t, ModeRich, false, fullSearchGroups)
	stripped := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(out, "")

	// Each result line should have name, metadata, and URL on the same line.
	lines := strings.Split(stripped, "\n")
	foundEquinixWAN := false
	for _, line := range lines {
		if strings.Contains(line, "Equinix (WAN)") {
			foundEquinixWAN = true
			if !strings.Contains(line, "AS47541") {
				t.Error("Equinix (WAN) line should contain ASN AS47541")
			}
			if !strings.Contains(line, "/ui/asn/47541") {
				t.Error("Equinix (WAN) line should contain detail URL")
			}
			break
		}
	}
	if !foundEquinixWAN {
		t.Error("output should contain an Equinix (WAN) result line")
	}
}
