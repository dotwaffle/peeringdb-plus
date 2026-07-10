package main

import (
	"net/http"
	"testing"
)

// TestRESTVaryOriginSingle locks the CORS layering fix: CORS is applied
// once by the outer middleware chain, and the former inner restCORS wrap
// on the /rest/v1/ mount double-appended Vary: Origin on every REST
// response, needlessly fragmenting shared HTTP caches.
func TestRESTVaryOriginSingle(t *testing.T) {
	t.Parallel()
	fix := buildPrivacySurfacesFixture(t)

	req, err := http.NewRequest(http.MethodGet, fix.server.URL+"/rest/v1/organizations", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Origin", "https://example.test")
	resp, err := fix.server.Client().Do(req)
	if err != nil {
		t.Fatalf("GET /rest/v1/organizations: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var origins int
	for _, v := range resp.Header.Values("Vary") {
		if v == "Origin" {
			origins++
		}
	}
	if origins != 1 {
		t.Errorf("Vary: Origin appears %d times, want exactly 1 (values=%v)", origins, resp.Header.Values("Vary"))
	}
}
