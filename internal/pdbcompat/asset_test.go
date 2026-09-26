package pdbcompat

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// seedASSetNet creates one network for the as_set tests.
func seedASSetNet(t *testing.T, c *ent.Client, id, asn int, status, irrAsSet string) {
	t.Helper()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if _, err := c.Network.Create().
		SetID(id).SetName("ASSetNet").SetNameFold(unifold.Fold("ASSetNet")).
		SetAsn(asn).SetIrrAsSet(irrAsSet).SetStatus(status).
		SetCreated(now).SetUpdated(now).
		Save(t.Context()); err != nil {
		t.Fatalf("seed net id=%d: %v", id, err)
	}
}

// asSetMux returns a mux with the pdbcompat routes over c.
func asSetMux(c *ent.Client, budget int64) (*Handler, *http.ServeMux) {
	h := NewHandler(c, budget)
	mux := http.NewServeMux()
	h.Register(mux)
	return h, mux
}

// TestASSet_Methods locks the method rules and the path shapes of the
// as_set routes. The dispatch branch sees the raw id, so a "." or a
// second segment is a 404, also for a later change that handles a
// format suffix on the Registry paths.
func TestASSet_Methods(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	seedASSetNet(t, c, 1, 42, "ok", "AS-FORTYTWO")
	_, mux := asSetMux(c, 0)

	for _, tc := range []struct {
		method, path string
		want         int
		wantAllow    string
	}{
		// upstream: rest.py:1399 (http_method_names = ["get"]), drf
		// views.py:517-521.
		{http.MethodHead, "/api/as_set", http.StatusMethodNotAllowed, "GET"},
		{http.MethodHead, "/api/as_set/42", http.StatusMethodNotAllowed, "GET"},
		{http.MethodOptions, "/api/as_set", http.StatusMethodNotAllowed, "GET"},
		{http.MethodPost, "/api/as_set", http.StatusMethodNotAllowed, "GET"},
		{http.MethodDelete, "/api/as_set/42", http.StatusMethodNotAllowed, "GET"},
		// A type that starts with as_set is not the lookup.
		{http.MethodPost, "/api/as_setx", http.StatusMethodNotAllowed, "GET, HEAD"},
		{http.MethodGet, "/api/as_set/42/extra", http.StatusNotFound, ""},
		{http.MethodGet, "/api/as_set/4.2", http.StatusNotFound, ""},
		{http.MethodGet, "/api/as_set/42.json", http.StatusNotFound, ""},
		{http.MethodGet, "/api/as_set/42.json/", http.StatusNotFound, ""},
		{http.MethodHead, "/api/as_set/4.2", http.StatusNotFound, ""},
		// The mirror trims a trailing slash on every /api/ path.
		{http.MethodGet, "/api/as_set/", http.StatusOK, ""},
		{http.MethodGet, "/api/as_set/42/", http.StatusOK, ""},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("%s %s: status = %d, want %d; body=%s", tc.method, tc.path, rec.Code, tc.want, rec.Body.String())
			continue
		}
		if got := rec.Header().Get("Allow"); got != tc.wantAllow {
			t.Errorf("%s %s: Allow = %q, want %q", tc.method, tc.path, got, tc.wantAllow)
		}
	}
}

// TestASSet_PoolExhausted locks the in-flight pool check of the as_set
// list: 503 with Retry-After while another response holds the pool, and
// no charge left behind.
func TestASSet_PoolExhausted(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	seedASSetNet(t, c, 1, 64500, "ok", "AS-ONE")
	seedASSetNet(t, c, 2, 64501, "ok", "AS-TWO")

	estimate := int64(2 * asSetEntryBytes)
	h, mux := asSetMux(c, estimate+estimate/2)
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/as_set", nil))
		return rec
	}

	if rec := get(); rec.Code != http.StatusOK {
		t.Fatalf("baseline: status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	h.inflightBytes.Add(estimate)
	rec := get()
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("pool nearly full: status = %d, want 503", rec.Code)
	}
	if got := rec.Header().Get("Retry-After"); got != "1" {
		t.Errorf("pool nearly full: Retry-After = %q, want 1", got)
	}
	h.inflightBytes.Add(-estimate)
	if rec := get(); rec.Code != http.StatusOK {
		t.Errorf("after release: status = %d, want 200", rec.Code)
	}
	if got := h.inflightBytes.Load(); got != 0 {
		t.Errorf("in-flight pool = %d after all requests, want 0", got)
	}
}

// TestASSet_NullIrrAsSet locks the NULL handling: upstream never stores
// NULL, the mirror column is nullable. The list leaves a NULL value out
// and the lookup reads it as "".
func TestASSet_NullIrrAsSet(t *testing.T) {
	t.Parallel()
	c, db := testutil.SetupClientWithDB(t)
	seedASSetNet(t, c, 1, 64500, "ok", "AS-ONE")
	seedASSetNet(t, c, 2, 64501, "ok", "AS-TWO")
	if _, err := db.ExecContext(t.Context(), "UPDATE networks SET irr_as_set = NULL WHERE asn = 64501"); err != nil {
		t.Fatalf("set NULL: %v", err)
	}
	_, mux := asSetMux(c, 128<<20)

	for _, tc := range []struct{ path, want string }{
		{"/api/as_set", `{"meta":{},"data":[{"64500":"AS-ONE"}]}`},
		{"/api/as_set/64501", `{"meta":{},"data":[{"64501":""}]}`},
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200; body=%s", tc.path, rec.Code, rec.Body.String())
			continue
		}
		if got := rec.Body.String(); got != tc.want {
			t.Errorf("GET %s: body = %s, want %s", tc.path, got, tc.want)
		}
	}
}
