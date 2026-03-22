package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// TestCORSPreflightAllowed verifies that OPTIONS preflight from an allowed origin
// gets the correct CORS headers per OPS-06.
func TestCORSPreflightAllowed(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.CORS(middleware.CORSInput{AllowedOrigins: "http://example.com"})(inner)

	// Test preflight OPTIONS request.
	req := httptest.NewRequest("OPTIONS", "/graphql", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	acao := rec.Header().Get("Access-Control-Allow-Origin")
	if acao != "http://example.com" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", acao, "http://example.com")
	}

	// Test actual POST request with Origin header (non-preflight).
	req2 := httptest.NewRequest("POST", "/graphql", nil)
	req2.Header.Set("Origin", "http://example.com")
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)

	acao2 := rec2.Header().Get("Access-Control-Allow-Origin")
	if acao2 != "http://example.com" {
		t.Errorf("POST Access-Control-Allow-Origin = %q, want %q", acao2, "http://example.com")
	}
}

// TestCORSPreflightDisallowed verifies that OPTIONS preflight from a disallowed origin
// does not get CORS Allow-Origin header.
func TestCORSPreflightDisallowed(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.CORS(middleware.CORSInput{AllowedOrigins: "http://allowed.example.com"})(inner)

	req := httptest.NewRequest("OPTIONS", "/graphql", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	acao := rec.Header().Get("Access-Control-Allow-Origin")
	if acao != "" {
		t.Errorf("expected no Access-Control-Allow-Origin for disallowed origin, got %q", acao)
	}
}

// TestCORSWildcard verifies that "*" allows any origin.
func TestCORSWildcard(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.CORS(middleware.CORSInput{AllowedOrigins: "*"})(inner)

	req := httptest.NewRequest("POST", "/graphql", nil)
	req.Header.Set("Origin", "http://any-origin.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	acao := rec.Header().Get("Access-Control-Allow-Origin")
	if acao != "*" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", acao, "*")
	}
}

// TestCORSMultipleOrigins verifies comma-separated origins are all allowed.
func TestCORSMultipleOrigins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		origin   string
		wantACAO string
	}{
		{name: "first origin", origin: "http://alpha.example.com", wantACAO: "http://alpha.example.com"},
		{name: "second origin", origin: "http://beta.example.com", wantACAO: "http://beta.example.com"},
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.CORS(middleware.CORSInput{AllowedOrigins: "http://alpha.example.com, http://beta.example.com"})(inner)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest("POST", "/graphql", nil)
			req.Header.Set("Origin", tc.origin)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			acao := rec.Header().Get("Access-Control-Allow-Origin")
			if acao != tc.wantACAO {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", acao, tc.wantACAO)
			}
		})
	}
}
