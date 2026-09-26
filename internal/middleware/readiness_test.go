package middleware_test

import (
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

type fakeReadiness bool

func (f fakeReadiness) HasCompletedSync() bool { return bool(f) }

// TestReadiness_APIErrorForm verifies that before the first sync every
// /api/ request gets the error form of that API, whatever the client
// type, and that other paths keep their own forms.
func TestReadiness_APIErrorForm(t *testing.T) {
	t.Parallel()

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	tests := []struct {
		name       string
		synced     bool
		path       string
		header     http.Header
		wantStatus int
		wantCT     string
		wantBody   func(t *testing.T, body []byte)
	}{
		{
			name:       "api curl gets meta envelope",
			path:       "/api/net",
			header:     http.Header{"User-Agent": {"curl/8.5.0"}},
			wantStatus: http.StatusServiceUnavailable,
			wantCT:     "application/json",
			wantBody:   assertMetaError,
		},
		{
			name:       "api browser gets meta envelope",
			path:       "/api/net",
			header:     http.Header{"Accept": {"text/html,application/xhtml+xml"}, "User-Agent": {"Mozilla/5.0"}},
			wantStatus: http.StatusServiceUnavailable,
			wantCT:     "application/json",
			wantBody:   assertMetaError,
		},
		{
			name:       "api no headers gets meta envelope",
			path:       "/api/net",
			wantStatus: http.StatusServiceUnavailable,
			wantCT:     "application/json",
			wantBody:   assertMetaError,
		},
		{
			name:       "api exact path gets meta envelope",
			path:       "/api",
			wantStatus: http.StatusServiceUnavailable,
			wantCT:     "application/json",
			wantBody:   assertMetaError,
		},
		{
			name:       "api problem opt-in",
			path:       "/api/net",
			header:     http.Header{"Accept": {"application/problem+json"}},
			wantStatus: http.StatusServiceUnavailable,
			wantCT:     "application/problem+json",
			wantBody: func(t *testing.T, body []byte) {
				t.Helper()
				var p struct {
					Status int    `json:"status"`
					Detail string `json:"detail"`
				}
				if err := json.Unmarshal(body, &p); err != nil {
					t.Fatalf("decode problem: %v; body=%s", err, body)
				}
				if p.Status != http.StatusServiceUnavailable || p.Detail != "sync not yet completed" {
					t.Errorf("problem = %+v, want status 503 and detail %q", p, "sync not yet completed")
				}
			},
		},
		{
			name:       "apifoo keeps the generic JSON form",
			path:       "/apifoo",
			wantStatus: http.StatusServiceUnavailable,
			wantCT:     "application/json",
			wantBody: func(t *testing.T, body []byte) {
				t.Helper()
				if got := strings.TrimSpace(string(body)); got != `{"error":"sync not yet completed"}` {
					t.Errorf("body = %s, want the generic JSON form", got)
				}
			},
		},
		{
			name:       "ui browser keeps the syncing page",
			path:       "/ui/",
			header:     http.Header{"Accept": {"text/html"}},
			wantStatus: http.StatusServiceUnavailable,
			wantCT:     "text/html; charset=utf-8",
		},
		{
			name:       "synced api reaches next handler",
			synced:     true,
			path:       "/api/net",
			wantStatus: http.StatusTeapot,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := middleware.Readiness(fakeReadiness(tt.synced), next)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			maps.Copy(req.Header, tt.header)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantCT != "" {
				if ct := rec.Header().Get("Content-Type"); ct != tt.wantCT {
					t.Errorf("Content-Type = %q, want %q", ct, tt.wantCT)
				}
			}
			isAPI := tt.path == "/api" || strings.HasPrefix(tt.path, "/api/")
			if !tt.synced && isAPI && !slices.Contains(rec.Header().Values("Vary"), "Accept") {
				t.Errorf("Vary = %q, want it to contain Accept", rec.Header().Values("Vary"))
			}
			if tt.wantBody != nil {
				tt.wantBody(t, rec.Body.Bytes())
			}
		})
	}
}

// assertMetaError checks for exactly {"meta":{"error":"sync not yet
// completed"}}.
func assertMetaError(t *testing.T, body []byte) {
	t.Helper()
	var got map[string]map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode body: %v; body=%s", err, body)
	}
	if len(got) != 1 || len(got["meta"]) != 1 || got["meta"]["error"] != "sync not yet completed" {
		t.Errorf("body = %s, want {\"meta\":{\"error\":\"sync not yet completed\"}}", body)
	}
}
