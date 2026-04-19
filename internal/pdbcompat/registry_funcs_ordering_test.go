package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestDefaultOrdering_Pdbcompat asserts that pdbcompat list endpoints return
// rows in compound (-updated, -created, -id) order — matching upstream
// django-handleref Meta.ordering = ("-updated", "-created") with `id DESC`
// as the tertiary tiebreak.
//
// Phase 67 Plan 03: flips all 13 .Order(ent.Asc("id")) calls in
// registry_funcs.go to .Order(ent.Desc("updated"), ent.Desc("created"),
// ent.Desc("id")). This test is RED before Task 2 of Plan 03 lands and
// GREEN after. See .planning/phases/67-default-ordering-flip/CONTEXT.md
// D-02, D-05, D-07 and 67-RESEARCH.md §1.1 / §6.
func TestDefaultOrdering_Pdbcompat(t *testing.T) {
	t.Parallel()

	// Base timestamp: spread seeds by 1h so SQLite time precision is
	// never the tiebreaker between the (-updated, -created) ranks.
	t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		seed func(t *testing.T, ctx *orderingTestCtx) []int // returns expected id order (desc)
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

	// Tie-break tests live under a Network subtree because Network has
	// the most flexible field set (no FK beyond Organization) and the
	// assertions are entity-agnostic (same SQL ORDER BY applies to all
	// 13 types).

	t.Run("TieBreakCreated", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		ctx := t.Context()

		org, err := client.Organization.Create().
			SetID(1).SetName("Tiebreak Org").
			SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("create org: %v", err)
		}

		// Same updated timestamp, different created. DESC by created
		// expected => row with later created wins.
		sameUpdated := t0.Add(2 * time.Hour)

		n1, err := client.Network.Create().
			SetID(10).SetName("SameUpdated-OlderCreated").SetAsn(64510).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(t0).SetUpdated(sameUpdated).
			Save(ctx)
		if err != nil {
			t.Fatalf("create net1: %v", err)
		}
		n2, err := client.Network.Create().
			SetID(11).SetName("SameUpdated-NewerCreated").SetAsn(64511).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(t0.Add(1 * time.Hour)).SetUpdated(sameUpdated).
			Save(ctx)
		if err != nil {
			t.Fatalf("create net2: %v", err)
		}

		mux := newMuxForOrdering(client)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		got := fetchIDOrder(t, srv.URL+"/api/net")
		want := []int{n2.ID, n1.ID}
		if !intSliceEqual(got, want) {
			t.Fatalf("tie-break by created DESC failed: got %v, want %v (n1.created=%s n2.created=%s)",
				got, want, n1.Created, n2.Created)
		}
	})

	t.Run("TieBreakID", func(t *testing.T) {
		t.Parallel()
		client := testutil.SetupClient(t)
		ctx := t.Context()

		org, err := client.Organization.Create().
			SetID(1).SetName("Tiebreak Org").
			SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("create org: %v", err)
		}

		// Same updated AND same created, different id. DESC by id
		// expected => higher id wins.
		sameTs := t0.Add(3 * time.Hour)

		nLow, err := client.Network.Create().
			SetID(10).SetName("LowerID").SetAsn(64510).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(sameTs).SetUpdated(sameTs).
			Save(ctx)
		if err != nil {
			t.Fatalf("create nLow: %v", err)
		}
		nHigh, err := client.Network.Create().
			SetID(99).SetName("HigherID").SetAsn(64599).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(sameTs).SetUpdated(sameTs).
			Save(ctx)
		if err != nil {
			t.Fatalf("create nHigh: %v", err)
		}

		mux := newMuxForOrdering(client)
		srv := httptest.NewServer(mux)
		t.Cleanup(srv.Close)

		got := fetchIDOrder(t, srv.URL+"/api/net")
		want := []int{nHigh.ID, nLow.ID}
		if !intSliceEqual(got, want) {
			t.Fatalf("tie-break by id DESC failed: got %v, want %v", got, want)
		}
	})
}

// orderingTestCtx carries shared seed state so the three representative
// cases can share a deterministic timeline and parent-org FK without
// repeating boilerplate.
type orderingTestCtx struct {
	client *ent.Client
	t0     time.Time
}

// seedThreeNetworks seeds 3 networks with distinct updated timestamps
// (t0, t0+1h, t0+2h) and identical created = t0. Returns the id slice
// in the expected (-updated) DESC order: newest first.
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

	// Insert id 20 at t0+2h (newest), id 10 at t0+1h, id 5 at t0 (oldest).
	// Expected DESC-by-updated: 20, 10, 5.
	type row struct {
		id      int
		updated time.Time
	}
	rows := []row{
		{id: 5, updated: o.t0},
		{id: 10, updated: o.t0.Add(1 * time.Hour)},
		{id: 20, updated: o.t0.Add(2 * time.Hour)},
	}

	for i, r := range rows {
		_, err := o.client.Network.Create().
			SetID(r.id).
			SetName(fmt.Sprintf("Net-%d", r.id)).
			SetAsn(64500 + r.id).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(o.t0).SetUpdated(r.updated).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed network row %d: %v", i, err)
		}
	}

	// Expected DESC order: 20 (newest updated), 10, 5.
	return []int{20, 10, 5}
}

// seedThreeFacilities seeds 3 facilities with distinct updated
// timestamps and identical created. Returns the expected id order.
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
		{id: 30, updated: o.t0},
		{id: 40, updated: o.t0.Add(1 * time.Hour)},
		{id: 50, updated: o.t0.Add(2 * time.Hour)},
	}

	for i, r := range rows {
		_, err := o.client.Facility.Create().
			SetID(r.id).
			SetName(fmt.Sprintf("Fac-%d", r.id)).
			SetOrgID(org.ID).SetOrganization(org).
			SetCity("Frankfurt").SetCountry("DE").
			SetCreated(o.t0).SetUpdated(r.updated).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed facility row %d: %v", i, err)
		}
	}

	return []int{50, 40, 30}
}

// seedThreeIXes seeds 3 InternetExchange rows with distinct updated
// timestamps and identical created.
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
		{id: 100, updated: o.t0},
		{id: 200, updated: o.t0.Add(1 * time.Hour)},
		{id: 300, updated: o.t0.Add(2 * time.Hour)},
	}

	for i, r := range rows {
		_, err := o.client.InternetExchange.Create().
			SetID(r.id).
			SetName(fmt.Sprintf("IX-%d", r.id)).
			SetOrgID(org.ID).SetOrganization(org).
			SetCity("Frankfurt").SetCountry("DE").
			SetRegionContinent("Europe").SetMedia("Ethernet").
			SetCreated(o.t0).SetUpdated(r.updated).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed ix row %d: %v", i, err)
		}
	}

	return []int{300, 200, 100}
}

// newMuxForOrdering registers a pdbcompat handler on a fresh mux for use
// with httptest.NewServer.
func newMuxForOrdering(client *ent.Client) *http.ServeMux {
	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// fetchIDOrder GETs the given URL, decodes the PeeringDB envelope, and
// returns the id order in response.data.
func fetchIDOrder(t *testing.T, url string) []int {
	t.Helper()
	resp, err := http.Get(url) //nolint:gosec,noctx // test code, local httptest server
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
