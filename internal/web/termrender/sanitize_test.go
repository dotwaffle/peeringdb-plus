package termrender

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"unicode"

	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// injectionPayload bundles the terminal-escape attack vectors an upstream
// PeeringDB submitter could smuggle into a free-text field: a raw ESC, an OSC
// window-title rewrite, an OSC-8 hyperlink, and a bare BEL. Each marker word is
// chosen so its presence in the rendered output proves the escape survived.
const (
	oscTitlePayload = "\x1b]0;pwnedTitle\x07"
	osc8Payload     = "\x1b]8;;http://evil.example/pwnedLink\x1b\\clickme\x1b]8;;\x1b\\"
	rawESCPayload   = "\x1bX"
	belPayload      = "ring\x07ring"
)

// stripSGR removes the renderer's own Select Graphic Rendition (colour/style)
// sequences so that any ESC remaining in the output must have come from an
// upstream field rather than from lipgloss styling.
func stripSGR(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}

// hasInjectedControl reports whether s carries a control byte that the renderer
// never legitimately emits as structure. The renderer's own line separators
// ('\n') are intentional layout, so they are exempt; everything else (ESC, BEL,
// the C1 range, DEL, CR, TAB) could only originate from an upstream payload.
func hasInjectedControl(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return r != '\n' && unicode.IsControl(r)
	}) >= 0
}

// injectedNetwork seeds every reachable free-text network field with a distinct
// escape payload so the regression test exercises titles, KV header values,
// list-row names, and IP columns at once.
var injectedNetwork = templates.NetworkDetail{
	ID:            7,
	ASN:           64500,
	Name:          "Acme" + oscTitlePayload + "Corp",
	NameLong:      "Acme" + rawESCPayload + "Corp Inc.",
	Website:       "https://acme.example/" + osc8Payload,
	OrgName:       "Acme" + belPayload + "Org",
	IRRAsSet:      "AS-ACME" + rawESCPayload,
	InfoType:      "NSP" + oscTitlePayload,
	PolicyGeneral: "Open" + rawESCPayload,
	Status:        "ok",
	IXCount:       1,
	FacCount:      1,
	IXPresences: []templates.NetworkIXLanRow{
		{
			IXName:  "Evil" + oscTitlePayload + "IX",
			IXID:    9,
			Speed:   10000,
			IPAddr4: "192.0.2.1" + belPayload,
			IPAddr6: "2001:db8::1" + rawESCPayload,
		},
	},
	FacPresences: []templates.NetworkFacRow{
		{
			FacName: "Evil" + osc8Payload + "Fac",
			FacID:   5,
			City:    "Town" + rawESCPayload,
			Country: "ZZ" + belPayload,
		},
	},
}

// TestRenderNetworkDetail_StripsInjectedEscapes is the terminal-injection
// regression. It renders an attacker-controlled network in ModeRich (the
// default for curl/wget) and asserts that no escape sequence smuggled through
// an upstream field survives. The renderer's own SGR styling is allowed; once
// those are stripped, the remaining bytes must be free of control characters.
//
// Before the sanitizeUpstream fix this test fails: colorprofile.ANSI256 only
// downsamples SGR sequences, so the OSC title, OSC-8 hyperlink, raw ESC, and
// BEL pass through verbatim.
func TestRenderNetworkDetail_StripsInjectedEscapes(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModeRich, false, injectedNetwork)

	// BEL is never emitted by the renderer itself, so a single occurrence
	// anywhere proves an upstream payload leaked.
	if strings.ContainsRune(out, '\x07') {
		t.Errorf("rendered output contains BEL (0x07) from an upstream field")
	}

	// The ESC-prefixed payloads (OSC title, OSC-8 hyperlink, raw ESC) must not
	// survive: each begins with ESC, the lead byte of every escape sequence.
	for _, marker := range []string{oscTitlePayload, "\x1b]8;;", rawESCPayload} {
		if strings.Contains(out, marker) {
			t.Errorf("rendered output contains injected escape sequence %q", marker)
		}
	}

	// After removing the renderer's own SGR sequences, no escape-class control
	// byte (ESC 0x1b, BEL 0x07, DEL, C1, ...) may remain — only the renderer's
	// structural newlines. Anything else came from a payload.
	if residual := stripSGR(out); hasInjectedControl(residual) {
		idx := strings.IndexFunc(residual, func(r rune) bool { return r != '\n' && unicode.IsControl(r) })
		t.Errorf("rendered output carries control byte 0x%02x from an upstream field (post-SGR-strip)", residual[idx])
	}

	// The benign text bracketing each payload must survive so sanitisation is
	// not over-broad: the payloads were strippable, the surrounding name was not.
	for _, want := range []string{"Acme", "Corp", "Evil", "Fac"} {
		if !strings.Contains(stripSGR(out), want) {
			t.Errorf("rendered output dropped benign text %q", want)
		}
	}
}

// TestRenderNetworkDetail_PlainStripsInjectedEscapes confirms ModePlain is also
// covered (it was the only mode that stripped escapes before the fix, via the
// NoTTY profile, but sanitisation must hold here too).
func TestRenderNetworkDetail_PlainStripsInjectedEscapes(t *testing.T) {
	t.Parallel()

	out := renderNetworkDetail(t, ModePlain, false, injectedNetwork)

	if hasInjectedControl(out) {
		idx := strings.IndexFunc(out, func(r rune) bool { return r != '\n' && unicode.IsControl(r) })
		t.Errorf("plain-mode output carries control byte 0x%02x from an upstream field", out[idx])
	}
}

// TestRenderShort_StripsInjectedEscapes covers the ?format=short surface, which
// writes raw upstream names with no colorprofile pass at all.
func TestRenderShort_StripsInjectedEscapes(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeShort, false)
	var buf bytes.Buffer
	if err := r.RenderShort(&buf, injectedNetwork); err != nil {
		t.Fatalf("RenderShort() error: %v", err)
	}
	if hasInjectedControl(buf.String()) {
		t.Errorf("short-mode output carries a control byte from an upstream field: %q", buf.String())
	}
}

// TestRenderWHOIS_StripsInjectedEscapes covers the ?format=whois surface, whose
// values reach the buffer through writeWHOISField without any styling.
func TestRenderWHOIS_StripsInjectedEscapes(t *testing.T) {
	t.Parallel()

	r := NewRenderer(ModeWHOIS, false)
	var buf bytes.Buffer
	if err := r.RenderWHOIS(&buf, "AS64500", injectedNetwork); err != nil {
		t.Fatalf("RenderWHOIS() error: %v", err)
	}
	if hasInjectedControl(buf.String()) {
		t.Errorf("whois-mode output carries a control byte from an upstream field: %q", buf.String())
	}
}

// assertNoInjectedEscapes runs the three terminal-injection checks against a
// rendered surface: no BEL, no ESC-prefixed payload marker, and — once the
// renderer's own SGR styling is stripped — no escape-class control byte. Any
// remaining control byte could only have come from an upstream field, so a
// dropped sanitizeUpstream on any RENDERED field fails here. Fields that the
// surface does not render simply never appear, so broad injection is safe.
func assertNoInjectedEscapes(t *testing.T, surface, out string) {
	t.Helper()
	if strings.ContainsRune(out, '\x07') {
		t.Errorf("%s: rendered output contains BEL (0x07) from an upstream field", surface)
	}
	for _, marker := range []string{oscTitlePayload, "\x1b]8;;", rawESCPayload} {
		if strings.Contains(out, marker) {
			t.Errorf("%s: rendered output contains injected escape sequence %q", surface, marker)
		}
	}
	if residual := stripSGR(out); hasInjectedControl(residual) {
		idx := strings.IndexFunc(residual, func(r rune) bool { return r != '\n' && unicode.IsControl(r) })
		t.Errorf("%s: rendered output carries control byte 0x%02x from an upstream field (post-SGR-strip)", surface, residual[idx])
	}
}

// p builds a field value that bundles all four escape payloads around a benign
// marker, so any single rendered field proves on its own whether the surface
// sanitises. Varying nothing per field keeps the fixtures terse; the payload
// markers (not field identity) are what the assertions key on.
func injected(benign string) string {
	return benign + oscTitlePayload + rawESCPayload + osc8Payload + belPayload
}

// The detail/compare/search renderers below were previously unguarded by any
// terminal-injection regression — only NetworkDetail was. Each call
// sanitizeUpstream on attacker-controlled upstream strings across ~23 sites;
// these fixtures inject every rendered free-text field (top-level + nested
// rows) so a dropped sanitizeUpstream in any of the seven renderers fails.
var (
	injectedOrg = templates.OrgDetail{
		ID: 1, Name: injected("Org"), NameLong: injected("OrgLong"), AKA: injected("OrgAKA"),
		Website: injected("https://org/"), Notes: injected("OrgNotes"),
		City: injected("OrgCity"), State: injected("OrgState"), Country: injected("ZZ"), Status: "ok",
		Networks: []templates.OrgNetworkRow{{NetName: injected("OrgNet"), ASN: 64500}},
		IXPs:     []templates.OrgIXRow{{IXName: injected("OrgIX"), IXID: 9}},
		Facs:     []templates.OrgFacilityRow{{FacName: injected("OrgFac"), FacID: 5, City: injected("FacCity"), Country: injected("ZZ")}},
		Campuses: []templates.OrgCampusRow{{CampusName: injected("OrgCampus"), CampusID: 3}},
		Carriers: []templates.OrgCarrierRow{{CarrierName: injected("OrgCarrier"), CarrierID: 7}},
	}
	injectedIX = templates.IXDetail{
		ID: 1, Name: injected("IX"), NameLong: injected("IXLong"), AKA: injected("IXAKA"),
		Website: injected("https://ix/"), OrgName: injected("IXOrg"), City: injected("IXCity"),
		Country: injected("ZZ"), RegionContinent: injected("Europe"), Media: injected("Ethernet"),
		Notes: injected("IXNotes"), Status: "ok",
		Participants: []templates.IXParticipantRow{{NetName: injected("IXPart"), ASN: 64500, IPAddr4: injected("192.0.2.1"), IPAddr6: injected("2001:db8::1")}},
		Facilities:   []templates.IXFacilityRow{{FacName: injected("IXFac"), FacID: 2, City: injected("FacCity"), Country: injected("ZZ")}},
		Prefixes:     []templates.IXPrefixRow{{Prefix: injected("10.0.0.0/24"), Protocol: "IPv4"}},
	}
	injectedFacility = templates.FacilityDetail{
		ID: 1, Name: injected("Fac"), NameLong: injected("FacLong"), AKA: injected("FacAKA"),
		Website: injected("https://fac/"), OrgName: injected("FacOrg"), CampusName: injected("FacCampus"),
		Address1: injected("Addr1"), Address2: injected("Addr2"), City: injected("FacCity"),
		State: injected("ST"), Country: injected("ZZ"), Zipcode: injected("00000"),
		RegionContinent: injected("Europe"), CLLI: injected("CLLI"), Notes: injected("FacNotes"), Status: "ok",
		Networks: []templates.FacNetworkRow{{NetName: injected("FacNet"), ASN: 64500, City: injected("C"), Country: injected("ZZ")}},
		IXPs:     []templates.FacIXRow{{IXName: injected("FacIX"), IXID: 9}},
		Carriers: []templates.FacCarrierRow{{CarrierName: injected("FacCarrier"), CarrierID: 7}},
	}
	injectedCarrier = templates.CarrierDetail{
		ID: 1, Name: injected("Carrier"), NameLong: injected("CarrierLong"), AKA: injected("CarrierAKA"),
		Website: injected("https://carrier/"), OrgName: injected("CarrierOrg"), Notes: injected("CarrierNotes"), Status: "ok",
		Facilities: []templates.CarrierFacilityRow{{FacName: injected("CarrierFac"), FacID: 5}},
	}
	injectedCampus = templates.CampusDetail{
		ID: 1, Name: injected("Campus"), NameLong: injected("CampusLong"), AKA: injected("CampusAKA"),
		Website: injected("https://campus/"), OrgName: injected("CampusOrg"), City: injected("CampusCity"),
		State: injected("ST"), Country: injected("ZZ"), Zipcode: injected("00000"), Notes: injected("CampusNotes"), Status: "ok",
		Facilities: []templates.CampusFacilityRow{{FacName: injected("CampusFac"), FacID: 5, City: injected("C"), Country: injected("ZZ")}},
	}
	injectedCompare = &templates.CompareData{
		NetA: templates.CompareNetwork{ASN: 64500, Name: injected("NetA"), ID: 1},
		NetB: templates.CompareNetwork{ASN: 64501, Name: injected("NetB"), ID: 2},
		SharedIXPs: []templates.CompareIXP{{IXID: 9, IXName: injected("CmpIX"), Shared: true,
			NetA: &templates.CompareIXPresence{Speed: 10000, IPAddr4: injected("192.0.2.1"), IPAddr6: injected("2001:db8::1")},
			NetB: &templates.CompareIXPresence{Speed: 10000}}},
		SharedFacilities: []templates.CompareFacility{{FacID: 5, FacName: injected("CmpFac"), City: injected("C"), Country: injected("ZZ"), Shared: true,
			NetA: &templates.CompareFacPresence{LocalASN: 64500}}},
		SharedCampuses: []templates.CompareCampus{{CampusID: 3, CampusName: injected("CmpCampus"),
			SharedFacilities: []templates.CompareCampusFacility{{FacID: 6, FacName: injected("CmpCampusFac")}}}},
		ViewMode: "full",
	}
	injectedSearch = []templates.SearchGroup{{
		TypeName: "Networks", TypeSlug: "net", AccentColor: "emerald",
		Results: []templates.SearchResult{{Name: injected("Hit"), Country: injected("ZZ"), City: injected("HitCity"), ASN: 64500, DetailURL: "/ui/asn/64500"}},
	}}
)

// TestRenderDetails_StripInjectedEscapes generalises the NetworkDetail
// terminal-injection regression to the other seven render entrypoints. Each is
// rendered in ModeRich (the curl/wget default, where colorprofile only
// downsamples SGR and so passes OSC/BEL/raw-ESC through verbatim absent
// sanitisation). A dropped sanitizeUpstream in any renderer leaks a payload and
// fails its sub-test.
func TestRenderDetails_StripInjectedEscapes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		render func(r *Renderer, w io.Writer) error
	}{
		{"org", func(r *Renderer, w io.Writer) error { return r.RenderOrgDetail(w, injectedOrg) }},
		{"ix", func(r *Renderer, w io.Writer) error { return r.RenderIXDetail(w, injectedIX) }},
		{"facility", func(r *Renderer, w io.Writer) error { return r.RenderFacilityDetail(w, injectedFacility) }},
		{"carrier", func(r *Renderer, w io.Writer) error { return r.RenderCarrierDetail(w, injectedCarrier) }},
		{"campus", func(r *Renderer, w io.Writer) error { return r.RenderCampusDetail(w, injectedCampus) }},
		{"compare", func(r *Renderer, w io.Writer) error { return r.RenderCompare(w, injectedCompare) }},
		{"search", func(r *Renderer, w io.Writer) error { return r.RenderSearch(w, injectedSearch) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := NewRenderer(ModeRich, false)
			var buf bytes.Buffer
			if err := tt.render(r, &buf); err != nil {
				t.Fatalf("render %s: %v", tt.name, err)
			}
			assertNoInjectedEscapes(t, tt.name, buf.String())
		})
	}
}
