package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

func TestCSP(t *testing.T) {
	t.Parallel()

	uiPolicy := "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; img-src 'self' data: https://*.basemaps.cartocdn.com; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net"
	graphQLPolicy := "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.jsdelivr.net; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net; img-src 'self' data:; connect-src 'self'"

	cspMW := middleware.CSP(middleware.CSPInput{
		UIPolicy:      uiPolicy,
		GraphQLPolicy: graphQLPolicy,
	})

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := cspMW(inner)

	headerName := "Content-Security-Policy-Report-Only"

	tests := []struct {
		name       string
		path       string
		wantCSP    bool
		wantPolicy string
	}{
		{
			name:       "UI search path gets UI CSP policy",
			path:       "/ui/search",
			wantCSP:    true,
			wantPolicy: uiPolicy,
		},
		{
			name:       "UI exact path gets UI CSP policy",
			path:       "/ui",
			wantCSP:    true,
			wantPolicy: uiPolicy,
		},
		{
			name:       "UI trailing slash gets UI CSP policy",
			path:       "/ui/",
			wantCSP:    true,
			wantPolicy: uiPolicy,
		},
		{
			name:       "GraphQL path gets GraphQL CSP policy",
			path:       "/graphql",
			wantCSP:    true,
			wantPolicy: graphQLPolicy,
		},
		{
			name:    "API path gets no CSP header",
			path:    "/api/net",
			wantCSP: false,
		},
		{
			name:    "REST path gets no CSP header",
			path:    "/rest/v1/networks",
			wantCSP: false,
		},
		{
			name:    "gRPC path gets no CSP header",
			path:    "/peeringdb.v1.NetworkService/GetNetwork",
			wantCSP: false,
		},
		{
			name:    "root path gets no CSP header",
			path:    "/",
			wantCSP: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			got := rec.Header().Get(headerName)

			if tc.wantCSP {
				if got == "" {
					t.Fatalf("%s header missing, want policy set", headerName)
				}
				if got != tc.wantPolicy {
					t.Errorf("%s = %q, want %q", headerName, got, tc.wantPolicy)
				}
			} else {
				if got != "" {
					t.Errorf("%s = %q, want empty (no CSP on this path)", headerName, got)
				}
			}

			// Verify it's Report-Only, not enforcing.
			enforcing := rec.Header().Get("Content-Security-Policy")
			if enforcing != "" {
				t.Errorf("Content-Security-Policy (enforcing) = %q, want empty (should be Report-Only)", enforcing)
			}
		})
	}
}

func TestCSPUIDirectives(t *testing.T) {
	t.Parallel()

	uiPolicy := "default-src 'self'; script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; img-src 'self' data: https://*.basemaps.cartocdn.com; connect-src 'self'; font-src 'self' https://cdn.jsdelivr.net"

	cspMW := middleware.CSP(middleware.CSPInput{
		UIPolicy:      uiPolicy,
		GraphQLPolicy: "default-src 'self'",
	})
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := cspMW(inner)

	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Content-Security-Policy-Report-Only")

	// Verify specific directives from the plan.
	wantContains := []string{
		"img-src 'self' data: https://*.basemaps.cartocdn.com",
		"font-src 'self' https://cdn.jsdelivr.net",
		"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com",
	}

	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Errorf("CSP header missing directive %q\ngot: %s", want, got)
		}
	}
}
