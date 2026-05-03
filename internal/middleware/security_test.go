package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// TestMiddleware_SecurityHeaders is a table-driven assertion of HSTS, XFO,
// and XCTO header presence and values across every path class the server
// handles. HSTS and XCTO must appear on every response; XFO must appear
// only on browser paths (/, /ui/*, /graphql, /graphql/*).
func TestMiddleware_SecurityHeaders(t *testing.T) {
	t.Parallel()

	const expectedHSTS = "max-age=31536000; includeSubDomains"

	mw := middleware.SecurityHeaders(middleware.SecurityHeadersInput{
		HSTSMaxAge:                365 * 24 * time.Hour,
		HSTSIncludeSubDomains:     true,
		FrameOptions:              "DENY",
		ContentTypeOptions:        true,
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
	})
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(inner)

	tests := []struct {
		name     string
		path     string
		wantHSTS string
		wantXFO  string // empty string => header must be absent
		wantXCTO string
		wantRP   string
	}{
		{name: "ui root", path: "/ui/", wantHSTS: expectedHSTS, wantXFO: "DENY", wantXCTO: "nosniff", wantRP: "strict-origin-when-cross-origin"},
		{name: "ui deep path", path: "/ui/asn/13335", wantHSTS: expectedHSTS, wantXFO: "DENY", wantXCTO: "nosniff"},
		{name: "graphql exact", path: "/graphql", wantHSTS: expectedHSTS, wantXFO: "DENY", wantXCTO: "nosniff"},
		{name: "graphql subpath", path: "/graphql/playground", wantHSTS: expectedHSTS, wantXFO: "DENY", wantXCTO: "nosniff"},
		{name: "root discovery", path: "/", wantHSTS: expectedHSTS, wantXFO: "DENY", wantXCTO: "nosniff"},
		{name: "rest API", path: "/rest/v1/networks", wantHSTS: expectedHSTS, wantXFO: "", wantXCTO: "nosniff"},
		{name: "peeringdb-compat API", path: "/api/net", wantHSTS: expectedHSTS, wantXFO: "", wantXCTO: "nosniff"},
		{name: "connectrpc service", path: "/peeringdb.v1.NetworkService/ListNetworks", wantHSTS: expectedHSTS, wantXFO: "", wantXCTO: "nosniff"},
		{name: "grpc health", path: "/grpc.health.v1.Health/Check", wantHSTS: expectedHSTS, wantXFO: "", wantXCTO: "nosniff"},
		{name: "readyz", path: "/readyz", wantHSTS: expectedHSTS, wantXFO: "", wantXCTO: "nosniff"},
		{name: "healthz", path: "/healthz", wantHSTS: expectedHSTS, wantXFO: "", wantXCTO: "nosniff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if got := rec.Header().Get("Strict-Transport-Security"); got != tt.wantHSTS {
				t.Errorf("HSTS header mismatch: got %q, want %q", got, tt.wantHSTS)
			}
			if got := rec.Header().Get("X-Frame-Options"); got != tt.wantXFO {
				t.Errorf("X-Frame-Options header mismatch: got %q, want %q", got, tt.wantXFO)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != tt.wantXCTO {
				t.Errorf("X-Content-Type-Options header mismatch: got %q, want %q", got, tt.wantXCTO)
			}
			if got := rec.Header().Get("Referrer-Policy"); got != tt.wantRP && tt.wantRP != "" {
				t.Errorf("Referrer-Policy header mismatch: got %q, want %q", got, tt.wantRP)
			}
			if got := rec.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
				t.Errorf("Cross-Origin-Opener-Policy header mismatch: got %q, want %q", got, "same-origin")
			}
			if got := rec.Header().Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
				t.Errorf("Cross-Origin-Resource-Policy header mismatch: got %q, want %q", got, "same-origin")
			}
		})
	}
}

// TestMiddleware_SecurityHeaders_NoPreload regression-locks the decision
// to omit the preload directive from HSTS. Fly.io .fly.dev is a shared-
// suffix domain incompatible with HSTS preloading — any future developer
// "improving" the header by appending "; preload" fails this test
// immediately. The assertion is a negative substring match on a sample
// of paths covering both the browser and API families.
func TestMiddleware_SecurityHeaders_NoPreload(t *testing.T) {
	t.Parallel()

	mw := middleware.SecurityHeaders(middleware.SecurityHeadersInput{
		// Use a huge max-age to ensure any future "improvement" cannot
		// hide behind "only apply preload at long durations".
		HSTSMaxAge:            10 * 365 * 24 * time.Hour,
		HSTSIncludeSubDomains: true,
		FrameOptions:          "DENY",
		ContentTypeOptions:    true,
	})
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(inner)

	paths := []string{
		"/ui/",
		"/ui/asn/13335",
		"/graphql",
		"/rest/v1/networks",
		"/peeringdb.v1.NetworkService/ListNetworks",
		"/readyz",
	}
	for _, p := range paths {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		hsts := rec.Header().Get("Strict-Transport-Security")
		if hsts == "" {
			t.Errorf("path %q: HSTS header absent, expected max-age directive", p)
			continue
		}
		if strings.Contains(hsts, "preload") {
			t.Errorf("path %q: HSTS header contains forbidden 'preload' directive: %q", p, hsts)
		}
	}
}

// TestMiddleware_SecurityHeaders_HSTSMaxAgeZero verifies the opt-out case:
// when HSTSMaxAge is zero, the Strict-Transport-Security header must NOT
// be emitted at all (not "max-age=0", which browsers interpret as "clear
// cached HSTS immediately").
func TestMiddleware_SecurityHeaders_HSTSMaxAgeZero(t *testing.T) {
	t.Parallel()

	mw := middleware.SecurityHeaders(middleware.SecurityHeadersInput{
		HSTSMaxAge:            0,
		HSTSIncludeSubDomains: true,
		FrameOptions:          "DENY",
		ContentTypeOptions:    true,
	})
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := mw(inner)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS header should be absent when HSTSMaxAge=0, got %q", got)
	}
	// XFO and XCTO should still fire — they are independent of HSTS opt-out.
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options should still be DENY when HSTSMaxAge=0, got %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options should still be nosniff when HSTSMaxAge=0, got %q", got)
	}
}
