package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/ent/rest"
)

// problemBody mirrors internal/httperr.ProblemDetail for decode assertions.
type problemBody struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Status   int    `json:"status"`
	Detail   string `json:"detail"`
	Instance string `json:"instance"`
}

// TestRESTErrorMiddleware_DetailPreserved verifies that restErrorMiddleware
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
			restErrorMiddleware(inner).ServeHTTP(rec, req)

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
	restErrorMiddleware(inner).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body = %q, want untouched 2xx body", got)
	}
}

// TestRESTErrorMiddleware_EntrestIntegration drives a real entrest server
// through the middleware and asserts the out-of-bounds per_page message
// reaches the client as problem Detail — the regression that motivated the
// buffered-body rework (previously Detail was always empty).
func TestRESTErrorMiddleware_EntrestIntegration(t *testing.T) {
	t.Parallel()

	id := restDBCounter.Add(1)
	dsn := fmt.Sprintf("file:rest_err_test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { client.Close() })

	restSrv, err := rest.NewServer(client, &rest.ServerConfig{})
	if err != nil {
		t.Fatalf("create REST server: %v", err)
	}
	ts := httptest.NewServer(restErrorMiddleware(restSrv.Handler()))
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/networks?per_page=0")
	if err != nil {
		t.Fatalf("GET /networks?per_page=0: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q, want application/problem+json", ct)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var p problemBody
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatalf("decode problem body %q: %v", raw, err)
	}
	if !strings.Contains(p.Detail, "per_page") {
		t.Errorf("detail = %q, want the offending parameter named (per_page)", p.Detail)
	}
}
