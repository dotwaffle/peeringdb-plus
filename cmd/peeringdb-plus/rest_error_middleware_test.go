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
	ts := httptest.NewServer(middleware.RESTError(restSrv.Handler()))
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
