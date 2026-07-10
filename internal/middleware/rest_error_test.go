package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// problemBody mirrors internal/httperr.ProblemDetail for decode assertions.
type problemBody struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

// TestRESTErrorMiddleware_DetailPreserved verifies that RESTError
// rewrites entrest error bodies to RFC 9457 problem+json while preserving
// the client-actionable `error` message as Detail for 4xx responses, and
// dropping it for 5xx responses (potential SQL/driver internals).
func TestRESTErrorMiddleware_DetailPreserved(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		body       string
		wantDetail string
	}{
		{
			name:       "4xx detail copied from entrest error field",
			status:     http.StatusBadRequest,
			body:       `{"error":"per_page 0 is out of bounds, must be >= 1","type":"Bad Request","code":400}`,
			wantDetail: "per_page 0 is out of bounds, must be >= 1",
		},
		{
			name:       "5xx detail dropped",
			status:     http.StatusInternalServerError,
			body:       `{"error":"sqlite3: database is locked","type":"Internal Server Error","code":500}`,
			wantDetail: "",
		},
		{
			name:       "malformed error body yields empty detail",
			status:     http.StatusBadRequest,
			body:       `not json`,
			wantDetail: "",
		},
		{
			name:       "empty error body yields empty detail",
			status:     http.StatusNotFound,
			body:       "",
			wantDetail: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/rest/v1/networks", nil)
			middleware.RESTError(inner).ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("Content-Type = %q, want application/problem+json", ct)
			}
			var p problemBody
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("decode problem body: %v", err)
			}
			if p.Detail != tt.wantDetail {
				t.Errorf("detail = %q, want %q", p.Detail, tt.wantDetail)
			}
			if p.Status != tt.status {
				t.Errorf("problem status = %d, want %d", p.Status, tt.status)
			}
			if p.Instance != "/rest/v1/networks" {
				t.Errorf("instance = %q, want /rest/v1/networks", p.Instance)
			}
		})
	}
}

// TestRESTErrorMiddleware_PassThrough2xx verifies that successful responses
// flow through the middleware untouched.
func TestRESTErrorMiddleware_PassThrough2xx(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/rest/v1/networks", nil)
	middleware.RESTError(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q, want untouched 2xx body", got)
	}
}
