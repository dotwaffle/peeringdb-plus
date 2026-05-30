package termrender

import (
	"bytes"
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
