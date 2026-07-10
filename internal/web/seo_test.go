package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRobotsTxt verifies the crawl policy endpoint: fragments are
// excluded, everything else is allowed.
func TestRobotsTxt(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{"User-agent: *", "Allow: /", "Disallow: /ui/fragment/"} {
		if !strings.Contains(body, want) {
			t.Errorf("robots.txt missing %q, got %q", want, body)
		}
	}
}

// TestSEOHead verifies detail pages emit the meta description,
// canonical link, and OpenGraph tags derived from entity data.
func TestSEOHead(t *testing.T) {
	t.Parallel()
	mux := setupAllTestMux(t)

	req := httptest.NewRequest(http.MethodGet, "/ui/asn/13335", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Host = "example.test"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	checks := []string{
		`<meta name="description" content="Cloudflare (AS13335)`,
		`<link rel="canonical" href="https://example.test/ui/asn/13335"`,
		`<meta property="og:title"`,
		`<meta property="og:description"`,
		`<meta property="og:url" content="https://example.test/ui/asn/13335"`,
	}
	for _, want := range checks {
		if !strings.Contains(body, want) {
			t.Errorf("detail page missing %q", want)
		}
	}
}
