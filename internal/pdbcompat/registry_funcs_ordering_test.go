package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestDefaultOrdering_Pdbcompat asserts that pdbcompat list endpoints
// without ?since return rows in id order, ascending. Upstream adds no
// ORDER BY to a plain list (2.83.0 rest.py:747-748) and no model
// declares Meta.ordering, so MySQL serves primary-key order.
//
// Each seed sets the created and updated stamps of the three rows so
// that id ASC, timestamp DESC and timestamp ASC each give a different
// order. An order that used either timestamp as a sort key, in either
// direction, would fail.
func TestDefaultOrdering_Pdbcompat(t *testing.T) {
	t.Parallel()

	// Base timestamp: spread seeds by 1h so SQLite time precision is
	// never a factor.
	t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		seed func(t *testing.T, ctx *orderingTestCtx) []int // returns expected id order (asc)
		path string                                         // e.g. "/api/net"
	}{
		{"Network", seedThreeNetworks, "/api/net"},
		{"Facility", seedThreeFacilities, "/api/fac"},
		{"InternetExchange", seedThreeIXes, "/api/ix"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := testutil.SetupClient(t)
			octx := &orderingTestCtx{client: client, t0: t0}
			expected := tc.seed(t, octx)

			mux := newMuxForOrdering(client)
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			got := fetchIDOrder(t, srv.URL+tc.path)
			if !intSliceEqual(got, expected) {
				t.Fatalf("%s ordering mismatch: got %v, want %v", tc.name, got, expected)
			}
		})
	}
}

// orderingTestCtx carries shared seed state so the three representative
// cases can share a deterministic timeline and parent-org FK without
// repeating boilerplate.
type orderingTestCtx struct {
	client *ent.Client
	t0     time.Time
}

// seedThreeNetworks seeds 3 networks whose updated and created stamps
// neither rise nor fall with the id. Returns the id slice in the
// expected ascending order.
func seedThreeNetworks(t *testing.T, o *orderingTestCtx) []int {
	t.Helper()
	ctx := t.Context()

	org, err := o.client.Organization.Create().
		SetID(1).SetName("Order Org").
		SetCreated(o.t0).SetUpdated(o.t0).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}

	// id ASC = [5 10 20], timestamp DESC = [10 5 20], timestamp ASC =
	// [20 5 10].
	type row struct {
		id      int
		updated time.Time
	}
	rows := []row{
		{id: 5, updated: o.t0.Add(1 * time.Hour)},
		{id: 10, updated: o.t0.Add(2 * time.Hour)},
		{id: 20, updated: o.t0},
	}

	for i, r := range rows {
		_, err := o.client.Network.Create().
			SetID(r.id).
			SetName(fmt.Sprintf("Net-%d", r.id)).
			SetAsn(64500 + r.id).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(r.updated).SetUpdated(r.updated).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed network row %d: %v", i, err)
		}
	}

	return []int{5, 10, 20}
}

// seedThreeFacilities seeds 3 facilities whose updated and created
// stamps neither rise nor fall with the id. Returns the expected id
// order.
func seedThreeFacilities(t *testing.T, o *orderingTestCtx) []int {
	t.Helper()
	ctx := t.Context()

	org, err := o.client.Organization.Create().
		SetID(1).SetName("Order Org").
		SetCreated(o.t0).SetUpdated(o.t0).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}

	type row struct {
		id      int
		updated time.Time
	}
	rows := []row{
		{id: 30, updated: o.t0.Add(1 * time.Hour)},
		{id: 40, updated: o.t0.Add(2 * time.Hour)},
		{id: 50, updated: o.t0},
	}

	for i, r := range rows {
		_, err := o.client.Facility.Create().
			SetID(r.id).
			SetName(fmt.Sprintf("Fac-%d", r.id)).
			SetOrgID(org.ID).SetOrganization(org).
			SetCity("Frankfurt").SetCountry("DE").
			SetCreated(r.updated).SetUpdated(r.updated).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed facility row %d: %v", i, err)
		}
	}

	return []int{30, 40, 50}
}

// seedThreeIXes seeds 3 InternetExchange rows whose updated and
// created stamps neither rise nor fall with the id.
func seedThreeIXes(t *testing.T, o *orderingTestCtx) []int {
	t.Helper()
	ctx := t.Context()

	org, err := o.client.Organization.Create().
		SetID(1).SetName("Order Org").
		SetCreated(o.t0).SetUpdated(o.t0).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}

	type row struct {
		id      int
		updated time.Time
	}
	rows := []row{
		{id: 100, updated: o.t0.Add(1 * time.Hour)},
		{id: 200, updated: o.t0.Add(2 * time.Hour)},
		{id: 300, updated: o.t0},
	}

	for i, r := range rows {
		_, err := o.client.InternetExchange.Create().
			SetID(r.id).
			SetName(fmt.Sprintf("IX-%d", r.id)).
			SetOrgID(org.ID).SetOrganization(org).
			SetCity("Frankfurt").SetCountry("DE").
			SetRegionContinent("Europe").SetMedia("Ethernet").
			SetCreated(r.updated).SetUpdated(r.updated).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed ix row %d: %v", i, err)
		}
	}

	return []int{100, 200, 300}
}

// newMuxForOrdering registers a pdbcompat handler on a fresh mux for use
// with httptest.NewServer.
func newMuxForOrdering(client *ent.Client) *http.ServeMux {
	h := NewHandler(client, 0)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// fetchIDOrder GETs the given URL, decodes the PeeringDB envelope, and
// returns the id order in response.data.
func fetchIDOrder(t *testing.T, url string) []int {
	t.Helper()
	resp, err := http.Get(url) //nolint:noctx // test code, local httptest server
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: unexpected status %d", url, resp.StatusCode)
	}

	var env struct {
		Data []struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope from %s: %v", url, err)
	}

	ids := make([]int, len(env.Data))
	for i, r := range env.Data {
		ids[i] = r.ID
	}
	return ids
}

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestListOrder_OrderBy checks the ORDER BY of a list with a filter
// sort key (a fac or org distance search): the key, then id. A ?since=
// list keeps updated, id, as upstream order_by("updated") replaces the
// distance order (2.83.0 rest.py:744).
func TestListOrder_OrderBy(t *testing.T) {
	t.Parallel()
	byDist := func(s *entsql.Selector) { s.OrderExpr(entsql.Expr("dist")) }
	since := time.Unix(1, 0)
	for _, tt := range []struct {
		name string
		opts QueryOptions
		want string
	}{
		{"plain", QueryOptions{}, "ORDER BY `t`.`id` ASC"},
		{"order_by", QueryOptions{OrderBy: byDist}, "ORDER BY dist, `t`.`id` ASC"},
		{"since", QueryOptions{Since: &since}, "ORDER BY `t`.`updated` ASC, `t`.`id` ASC"},
		{"order_by_since", QueryOptions{OrderBy: byDist, Since: &since}, "ORDER BY `t`.`updated` ASC, `t`.`id` ASC"},
	} {
		s := entsql.Dialect(dialect.SQLite).Select("*").From(entsql.Table("t"))
		for _, o := range listOrder[func(*entsql.Selector)](tt.opts) {
			o(s)
		}
		q, _ := s.Query()
		if !strings.HasSuffix(q, tt.want) {
			t.Errorf("%s: query = %q, want suffix %q", tt.name, q, tt.want)
		}
	}
}
