package pdbcompat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// seedDetailNet creates net id with the given name and status.
func seedDetailNet(t *testing.T, c *ent.Client, id int, name, status string) {
	t.Helper()
	ts := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if _, err := c.Network.Create().
		SetID(id).SetName(name).SetNameFold(unifold.Fold(name)).
		SetAsn(64500 + id).SetStatus(status).
		SetCreated(ts).SetUpdated(ts).
		Save(t.Context()); err != nil {
		t.Fatalf("seed net id=%d: %v", id, err)
	}
}

// mustParseFilters returns the filter predicates of the query string q
// for typ.
func mustParseFilters(t *testing.T, typ, q string) []func(*entsql.Selector) {
	t.Helper()
	params, err := url.ParseQuery(q)
	if err != nil {
		t.Fatalf("parse query %q: %v", q, err)
	}
	preds, empty, err := ParseFiltersCtx(t.Context(), params, Registry[typ])
	if err != nil || empty {
		t.Fatalf("ParseFiltersCtx(%s?%s): empty=%v err=%v", typ, q, empty, err)
	}
	return preds
}

// TestRegistry_MatchWired checks that every Registry entry has a Match
// and that Match checks the id and the filters.
func TestRegistry_MatchWired(t *testing.T) {
	t.Parallel()
	for name, tc := range Registry {
		if tc.Match == nil {
			t.Errorf("Registry[%q].Match is nil", name)
		}
	}

	c := testutil.SetupClient(t)
	seedDetailNet(t, c, 1, "MatchNet", "ok")
	match := Registry[peeringdb.TypeNet].Match
	for _, tc := range []struct {
		name    string
		id      int
		filters []func(*entsql.Selector)
		want    bool
	}{
		{"stored id, no filter", 1, nil, true},
		{"missing id, no filter", 2, nil, false},
		{"stored id, matching filter", 1, mustParseFilters(t, peeringdb.TypeNet, "name=MatchNet"), true},
		{"stored id, excluding filter", 1, mustParseFilters(t, peeringdb.TypeNet, "name=Other"), false},
	} {
		got, err := match(t.Context(), c, tc.id, tc.filters)
		if err != nil {
			t.Fatalf("%s: Match: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: Match = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestMatch_AddsNoStatus checks that Match adds no status of its own:
// the detail status set belongs to the PK lookup of Get. A relation
// seed that pins the listed row to status ok still excludes a pending
// row, as upstream make_relation_filter does (2.83.0 models.py:221-234).
func TestMatch_AddsNoStatus(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	ts := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	seedDetailNet(t, c, 1, "DeletedNet", "deleted")

	got, err := Registry[peeringdb.TypeNet].Match(ctx, c, 1, mustParseFilters(t, peeringdb.TypeNet, "name=DeletedNet"))
	if err != nil {
		t.Fatalf("Match net: %v", err)
	}
	if !got {
		t.Error("Match(deleted net, name=DeletedNet) = false, want true (Get owns the status set)")
	}

	c.Organization.Create().SetID(1).SetName("Org").SetNameFold("org").
		SetStatus("ok").SetCreated(ts).SetUpdated(ts).SaveX(ctx)
	c.Campus.Create().SetID(50).SetName("Campus").SetNameFold("campus").SetOrgID(1).
		SetStatus("pending").SetCreated(ts).SetUpdated(ts).SaveX(ctx)
	c.Facility.Create().SetID(400).SetName("Fac").SetNameFold("fac").SetOrgID(1).
		SetCampusID(50).SetStatus("ok").SetCreated(ts).SetUpdated(ts).SaveX(ctx)

	got, err = Registry[peeringdb.TypeCampus].Match(ctx, c, 50, mustParseFilters(t, peeringdb.TypeCampus, "facility=400"))
	if err != nil {
		t.Fatalf("Match campus: %v", err)
	}
	if got {
		t.Error("Match(pending campus, facility=400) = true, want false (the relation seed pins ok)")
	}
}

// TestParseDetailSlice checks the limit and skip rules of a detail
// request and that the error texts are the list texts.
func TestParseDetailSlice(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		query       string
		sliced, neg bool
		wantErr     bool
	}{
		{query: ""},
		{query: "limit=0&skip=0"},
		{query: "limit=-1"},
		{query: "limit=1", sliced: true},
		{query: "skip=1", sliced: true},
		{query: "limit=-1&skip=1", sliced: true},
		{query: "skip=-1", neg: true},
		{query: "limit=", wantErr: true},
		{query: "skip=", wantErr: true},
		{query: "limit=abc", wantErr: true},
		{query: "skip=abc", wantErr: true},
		{query: "limit=abc&skip=abc", wantErr: true},
	} {
		params, _ := url.ParseQuery(tc.query)
		sliced, neg, err := parseDetailSlice(params)
		if (err != nil) != tc.wantErr {
			t.Errorf("%q: err = %v, want error %v", tc.query, err, tc.wantErr)
			continue
		}
		if err != nil {
			if _, _, listErr := ParsePaginationParams(params); listErr == nil || listErr.Error() != err.Error() {
				t.Errorf("%q: err = %q, list err = %v", tc.query, err, listErr)
			}
			continue
		}
		if sliced != tc.sliced || neg != tc.neg {
			t.Errorf("%q: sliced, negativeSkip = %v, %v; want %v, %v", tc.query, sliced, neg, tc.sliced, tc.neg)
		}
	}
}

// TestServeDetail_FilterMissBeforeBudget checks that a filter miss is a
// 404 before the 413 check, and that no request leaves a charge in the
// in-flight pool.
func TestServeDetail_FilterMissBeforeBudget(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	seedDetailNet(t, c, 1, "BudgetNet", "ok")
	h := NewHandler(c, 100)
	mux := http.NewServeMux()
	h.Register(mux)

	for path, want := range map[string]int{
		"/api/net/1":                http.StatusRequestEntityTooLarge,
		"/api/net/1?name=nomatch":   http.StatusNotFound,
		"/api/net/1?name=BudgetNet": http.StatusRequestEntityTooLarge,
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s: status = %d, want %d; body=%s", path, rec.Code, want, rec.Body.String())
		}
	}
	if got := h.inflightBytes.Load(); got != 0 {
		t.Errorf("inflightBytes = %d after the requests, want 0", got)
	}
}

// limitOne matches the id query of Match (ent Exist: LIMIT 1). The PK
// lookup of Get uses Only (LIMIT 2).
var limitOne = regexp.MustCompile(`LIMIT 1\b`)

// TestServeDetail_NoFilterNoMatchQuery checks that a detail request
// without filter keys sends no Match query, and that one with a filter
// key sends exactly one.
func TestServeDetail_NoFilterNoMatchQuery(t *testing.T) {
	t.Parallel()
	seedClient, db := testutil.SetupClientWithDB(t)
	seedDetailNet(t, seedClient, 1, "QueryNet", "ok")

	for path, want := range map[string]int{
		"/api/net/1": 0,
		"/api/net/1?depth=0&fields=id&q=x&since=5&limit=0": 0,
		"/api/net/1?name=QueryNet":                         1,
	} {
		rec := &recordingDriver{Driver: entsql.OpenDB(dialect.SQLite, db)}
		mux := http.NewServeMux()
		NewHandler(ent.NewClient(ent.Driver(rec)), 0).Register(mux)
		resp := httptest.NewRecorder()
		mux.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		if resp.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200; body=%s", path, resp.Code, resp.Body.String())
		}
		var got int
		for _, q := range rec.queries() {
			if limitOne.MatchString(q.q) {
				got++
			}
		}
		if got != want {
			t.Errorf("GET %s: %d Match queries, want %d", path, got, want)
		}
	}
}

// TestParseRequestFilters_UnknownFieldsOnDetail checks that a detail
// request records the filter keys that it ignores, as a list does.
func TestParseRequestFilters_UnknownFieldsOnDetail(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	ctx, span := tp.Tracer("test").Start(context.Background(), "test-serve-detail")
	req := httptest.NewRequest(http.MethodGet, "/api/net/1?bogus=1", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	span.End()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var found bool
	for _, s := range exporter.GetSpans() {
		for _, a := range s.Attributes {
			if a.Key == attribute.Key("pdbplus.filter.unknown_fields") && strings.Contains(a.Value.AsString(), "bogus") {
				found = true
			}
		}
	}
	if !found {
		t.Error("span attribute pdbplus.filter.unknown_fields with bogus not found")
	}
}
