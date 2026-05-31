package templates

import (
	"strings"
	"testing"
)

// TestDetailLink_SanitizesUpstreamURL verifies DetailLink routes its
// upstream-controlled url through templ.URL — which strips dangerous schemes
// such as javascript: and data: — rather than templ.SafeURL, which bypasses
// sanitization and would emit a mirrored upstream value verbatim as an
// executable href (stored XSS).
func TestDetailLink_SanitizesUpstreamURL(t *testing.T) {
	t.Parallel()

	render := func(t *testing.T, url string) string {
		t.Helper()
		var buf strings.Builder
		if err := DetailLink("Website", url).Render(t.Context(), &buf); err != nil {
			t.Fatalf("render DetailLink(%q): %v", url, err)
		}
		return buf.String()
	}

	// A javascript: scheme must not survive into the href attribute.
	out := render(t, "javascript:alert(document.domain)")
	if strings.Contains(out, `href="javascript:`) || strings.Contains(out, "href='javascript:") {
		t.Errorf("DetailLink emitted an executable javascript: href:\n%s", out)
	}
	if !strings.Contains(out, "about:invalid") {
		t.Errorf("expected the sanitized about:invalid placeholder in the href, got:\n%s", out)
	}

	// A legitimate https URL passes through unchanged.
	out = render(t, "https://lg.example.net/")
	if !strings.Contains(out, `href="https://lg.example.net/"`) {
		t.Errorf("a legitimate https URL should pass through unchanged, got:\n%s", out)
	}
}
