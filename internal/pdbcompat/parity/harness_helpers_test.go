package parity

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
)

// newTestServer wraps the canonical pdbcompat handler in an
// httptest.Server. It mirrors cmd/peeringdb-plus/main.go's wiring
// (NewHandler + Register) without the production middleware chain so
// parity failures are localised to the pdbcompat layer rather than to
// CSP/CORS/gzip/recovery surfaces. The middleware chain is exercised
// elsewhere (cmd/peeringdb-plus/*_test.go); duplicating it here would
// only obscure regression signals.
//
// budget=0 disables the Phase 71 pre-flight CheckBudget gate; tests
// that exercise the 413 path pass a non-zero budget explicitly via
// newTestServerWithBudget.
//
// Accepts testing.TB so bench_test.go can reuse the same server setup;
// *testing.T and *testing.B both satisfy the interface.
func newTestServer(t testing.TB, c *ent.Client) *httptest.Server {
	t.Helper()
	return newTestServerWithBudget(t, c, 0)
}

// newTestServerWithBudget mirrors newTestServer but exposes the
// per-response memory budget knob. Used by limit_test.go to drive the
// CheckBudget 413 path with a deliberately tiny budget.
func newTestServerWithBudget(t testing.TB, c *ent.Client, budget int64) *httptest.Server {
	t.Helper()
	h := pdbcompat.NewHandler(c, budget)
	mux := http.NewServeMux()
	h.Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// httpGet does a GET against srv and returns (status, body). Transport
// errors fail the test via t.Fatal — they indicate harness/server
// breakage, not behavioural regression.
func httpGet(t testing.TB, srv *httptest.Server, path string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("httpGet: build request for %s: %v", path, err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("httpGet: GET %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("httpGet: read body from %s: %v", path, err)
	}
	return resp.StatusCode, body
}

// envelope is the on-the-wire {"meta": ..., "data": [...]} shape.
type envelope struct {
	Meta json.RawMessage   `json:"meta"`
	Data []json.RawMessage `json:"data"`
}

// decodeDataArray decodes the {"data":[...]} envelope into a slice of
// objects. Each element is a generic map so individual subtests can
// pluck out the field they care about (id, name, asn, ...) without
// per-test struct definitions.
func decodeDataArray(t testing.TB, body []byte) []map[string]any {
	t.Helper()
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decodeDataArray: unmarshal envelope: %v\nbody=%s", err, string(body))
	}
	out := make([]map[string]any, 0, len(env.Data))
	for i, raw := range env.Data {
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			t.Fatalf("decodeDataArray: row[%d]: %v\nraw=%s", i, err, string(raw))
		}
		out = append(out, row)
	}
	return out
}

// extractIDs decodes data[].id as ints. Missing or non-numeric ids
// fail the test — they indicate a broken serializer, which is itself
// a regression worth catching.
func extractIDs(t testing.TB, body []byte) []int {
	t.Helper()
	rows := decodeDataArray(t, body)
	ids := make([]int, 0, len(rows))
	for i, row := range rows {
		raw, ok := row["id"]
		if !ok {
			t.Fatalf("extractIDs: row[%d] missing id: %+v", i, row)
		}
		f, ok := raw.(float64)
		if !ok {
			t.Fatalf("extractIDs: row[%d].id = %T(%v), want number", i, raw, raw)
		}
		ids = append(ids, int(f))
	}
	return ids
}

// problem is the budget-exceeded RFC 9457 problem-detail body shape
// (mirrors pdbcompat.budgetProblemBody / WriteBudgetProblem). Only the
// fields parity tests assert on are surfaced here.
type problem struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Status      int    `json:"status"`
	Detail      string `json:"detail"`
	Instance    string `json:"instance,omitempty"`
	MaxRows     int    `json:"max_rows"`
	BudgetBytes int64  `json:"budget_bytes"`
}

// mustDecodeProblem decodes an application/problem+json body. Used by
// limit_test's 413 case. Failure to decode is a hard error — it
// signals a serializer regression, not a behavioural one.
func mustDecodeProblem(t testing.TB, body []byte) problem {
	t.Helper()
	var p problem
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatalf("mustDecodeProblem: %v\nbody=%s", err, string(body))
	}
	return p
}
