// Package main ordering_cross_surface_e2e_test.go — Phase 67 Plan 06
// cross-surface ordering parity + entrest override + nested _set
// (D-04 clarification) end-to-end verification.
//
// This test closes out Phase 67 "default-ordering-flip" by exercising the
// three query surfaces (pdbcompat /api, entrest /rest/v1, ConnectRPC
// List*) against a shared in-memory ent client and asserting that the
// row order returned from each surface is identical under the new
// compound (-updated, -created, -id) default. Three matching surfaces
// is what the must_haves call "goal-backward verification" — the
// individual plans 03/04/05 assert per-surface contracts in isolation,
// but only this test locks in the cross-surface parity guarantee that
// the user-visible contract depends on.
//
// Four top-level tests:
//
//   - TestOrdering_CrossSurface  — compound default + tie-break parity across all 3 surfaces
//   - TestEntrestDefaultOrder    — ORDER-03 override contract (no sort = compound default; explicit ?sort= honoured)
//   - TestEntrestNestedSetOrder  — D-04 clarification: nested eager-loaded edge arrays sort by (-updated)
//
// The test boots ONLY the three list-capable surfaces (pdbcompat,
// entrest, ConnectRPC) — /ui and /graphql are out-of-scope for the
// ordering contract (ORDER-03 "Out of scope: GraphQL, Web UI" per
// CONTEXT.md). The fixture deliberately avoids buildMiddlewareChain so
// we don't pull in readiness/CSP/compression concerns that have
// nothing to do with the ordering assertion.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/ent/rest"
	pbv1 "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/internal/grpcserver"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbcompat"
)

// orderingDBCounter yields isolated in-memory SQLite DBs for parallel
// sub-tests so seeding from one sub-test doesn't leak into another.
var orderingDBCounter atomic.Int64

// orderingFixture bundles the three-surface httptest.Server + client
// so sub-tests can issue requests against the same seed data.
type orderingFixture struct {
	server *httptest.Server
	client *ent.Client
}

// buildOrderingFixture wires pdbcompat + entrest + a ConnectRPC
// NetworkService onto a fresh mux, wraps in httptest.NewServer, and
// returns both the server handle and the ent client for direct seeding.
// The client is isolated per-call via atomic counter so parallel sub-tests
// can seed independently.
func buildOrderingFixture(t *testing.T) *orderingFixture {
	t.Helper()

	id := orderingDBCounter.Add(1)
	dsn := fmt.Sprintf("file:ordering_e2e_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { _ = client.Close() })

	mux := http.NewServeMux()

	// pdbcompat (/api/*) — Register binds all 13 types to /api/<type>.
	compatHandler := pdbcompat.NewHandler(client)
	compatHandler.Register(mux)

	// entrest (/rest/v1/*). No middleware wrap needed here — the
	// ordering assertion doesn't depend on CORS/error-shape/field-redact
	// layers (those live above the sort, not below it).
	restSrv, err := rest.NewServer(client, &rest.ServerConfig{BasePath: "/rest/v1"})
	if err != nil {
		t.Fatalf("create REST server: %v", err)
	}
	mux.Handle("/rest/v1/", restSrv.Handler())

	// ConnectRPC — only register the services this test exercises
	// (NetworkService is sufficient for the cross-surface + override
	// assertions; nested _set is checked via entrest only per D-04
	// "pdbcompat and grpcserver nested eager-loads are unaffected").
	netPath, netHandler := peeringdbv1connect.NewNetworkServiceHandler(
		&grpcserver.NetworkService{Client: client, StreamTimeout: 30 * time.Second},
	)
	mux.Handle(netPath, netHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &orderingFixture{server: srv, client: client}
}

// seedNetworksWithUpdated creates n networks with ids 1..n, each with
// updated += i*1h starting at base. All share the same created timestamp
// so the compound ORDER BY collapses to (-updated, -id) tiebreak. The
// returned slice is the expected DESC id order ([n, n-1, ..., 1]).
func seedNetworksWithUpdated(t *testing.T, client *ent.Client, n int, base time.Time) []int {
	t.Helper()
	ctx := t.Context()

	org := client.Organization.Create().
		SetID(1).
		SetName("Ordering E2E Org").
		SetCreated(base).
		SetUpdated(base).
		SetStatus("ok").
		SaveX(ctx)

	for i := 1; i <= n; i++ {
		client.Network.Create().
			SetID(i).
			SetName(fmt.Sprintf("Net-%d", i)).
			SetAsn(64500 + i).
			SetInfoUnicast(true).
			SetInfoMulticast(false).
			SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).
			SetPolicyRatio(false).
			SetAllowIxpUpdate(false).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(base).
			SetUpdated(base.Add(time.Duration(i-1) * time.Hour)).
			SetStatus("ok").
			SaveX(ctx)
	}

	expected := make([]int, n)
	for i := range n {
		expected[i] = n - i
	}
	return expected
}

// fetchPdbcompatNetworkIDs hits /api/net and returns the id order from
// the {"data":[{"id": N}, ...]} envelope.
func fetchPdbcompatNetworkIDs(t *testing.T, baseURL string) []int {
	t.Helper()
	return fetchEnvelopeIDs(t, baseURL+"/api/net", "data")
}

// fetchEntrestNetworkIDs hits /rest/v1/networks and returns the id
// order from the {"content":[{"id": N}, ...], "total_count": N}
// envelope. Optional query appends to the URL.
func fetchEntrestNetworkIDs(t *testing.T, baseURL, query string) []int {
	t.Helper()
	url := baseURL + "/rest/v1/networks"
	if query != "" {
		url += "?" + query
	}
	return fetchEnvelopeIDs(t, url, "content")
}

// fetchEnvelopeIDs issues GET url, decodes a {envelopeKey: [{id}]}
// response, and returns the id slice.
func fetchEnvelopeIDs(t *testing.T, url, envelopeKey string) []int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s body: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status=%d body=%s", url, resp.StatusCode, body)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope from %s: %v\nbody=%s", url, err, body)
	}
	raw, ok := env[envelopeKey]
	if !ok {
		t.Fatalf("response missing %q key\nurl=%s\nbody=%s", envelopeKey, url, body)
	}
	var rows []struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatalf("decode %s[] from %s: %v\nraw=%s", envelopeKey, url, err, raw)
	}
	ids := make([]int, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

// fetchGrpcNetworkIDs uses the ConnectRPC NetworkServiceClient to issue
// ListNetworks and returns the id order from the response. The
// generated client wraps/unwraps connect.Request/Response internally so
// we pass the bare *ListNetworksRequest and receive the bare response.
func fetchGrpcNetworkIDs(t *testing.T, baseURL string, pageSize int32) []int {
	t.Helper()
	cl := peeringdbv1connect.NewNetworkServiceClient(http.DefaultClient, baseURL)
	resp, err := cl.ListNetworks(t.Context(), &pbv1.ListNetworksRequest{PageSize: pageSize})
	if err != nil {
		t.Fatalf("ListNetworks: %v", err)
	}
	nets := resp.GetNetworks()
	ids := make([]int, len(nets))
	for i, n := range nets {
		ids[i] = int(n.GetId())
	}
	return ids
}

// idSliceEqual returns true iff both slices have the same length and
// element-wise equal ids.
func idSliceEqual(a, b []int) bool {
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

// =============================================================================
// TestOrdering_CrossSurface — compound default + tie-break parity across
// pdbcompat, entrest, and ConnectRPC for Network (the representative
// entity; the underlying compound ORDER BY is identical across all 13
// per Plans 03/04/05).
// =============================================================================

func TestOrdering_CrossSurface(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	// ---------------------------------------------------------------
	// Sub-test 1: compound default — 3 rows with distinct `updated`.
	// All three surfaces must return the same (-updated) DESC order.
	// ---------------------------------------------------------------
	t.Run("Network", func(t *testing.T) {
		t.Parallel()
		fix := buildOrderingFixture(t)
		expected := seedNetworksWithUpdated(t, fix.client, 3, base)

		compatIDs := fetchPdbcompatNetworkIDs(t, fix.server.URL)
		entrestIDs := fetchEntrestNetworkIDs(t, fix.server.URL, "")
		grpcIDs := fetchGrpcNetworkIDs(t, fix.server.URL, 100)

		if !idSliceEqual(compatIDs, expected) {
			t.Errorf("pdbcompat /api/net order = %v, want %v", compatIDs, expected)
		}
		if !idSliceEqual(entrestIDs, expected) {
			t.Errorf("entrest /rest/v1/networks order = %v, want %v", entrestIDs, expected)
		}
		if !idSliceEqual(grpcIDs, expected) {
			t.Errorf("ConnectRPC ListNetworks order = %v, want %v", grpcIDs, expected)
		}

		// Cross-surface parity: the three slices must be pairwise equal.
		// This is the contract the test locks in — any surface diverging
		// from the others is a user-visible regression regardless of
		// whether its standalone ordering matches "expected".
		if !idSliceEqual(compatIDs, entrestIDs) {
			t.Errorf("cross-surface parity FAILED: pdbcompat=%v entrest=%v", compatIDs, entrestIDs)
		}
		if !idSliceEqual(compatIDs, grpcIDs) {
			t.Errorf("cross-surface parity FAILED: pdbcompat=%v grpc=%v", compatIDs, grpcIDs)
		}
	})

	// ---------------------------------------------------------------
	// Sub-test 2: TieBreakCreated — 2 rows share `updated` but differ
	// by `created`. All three surfaces must fall through to (-created).
	// ---------------------------------------------------------------
	t.Run("TieBreakCreated", func(t *testing.T) {
		t.Parallel()
		fix := buildOrderingFixture(t)
		ctx := t.Context()

		org := fix.client.Organization.Create().
			SetID(1).SetName("TieBreakCreated Org").
			SetCreated(base).SetUpdated(base).
			SetStatus("ok").SaveX(ctx)

		// Both rows have updated = base+2h, but created differs.
		// Under (-created) tie-break, the row with the LATER created
		// timestamp wins.
		sameUpdated := base.Add(2 * time.Hour)
		fix.client.Network.Create().
			SetID(10).SetName("OlderCreated").SetAsn(64510).
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).SetAllowIxpUpdate(false).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(base).SetUpdated(sameUpdated).
			SetStatus("ok").SaveX(ctx)
		fix.client.Network.Create().
			SetID(11).SetName("NewerCreated").SetAsn(64511).
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).SetAllowIxpUpdate(false).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(base.Add(1 * time.Hour)).SetUpdated(sameUpdated).
			SetStatus("ok").SaveX(ctx)

		// DESC by created => id 11 (newer created) first.
		want := []int{11, 10}

		compatIDs := fetchPdbcompatNetworkIDs(t, fix.server.URL)
		entrestIDs := fetchEntrestNetworkIDs(t, fix.server.URL, "")
		grpcIDs := fetchGrpcNetworkIDs(t, fix.server.URL, 100)

		if !idSliceEqual(compatIDs, want) {
			t.Errorf("pdbcompat TieBreakCreated = %v, want %v", compatIDs, want)
		}
		if !idSliceEqual(entrestIDs, want) {
			t.Errorf("entrest TieBreakCreated = %v, want %v", entrestIDs, want)
		}
		if !idSliceEqual(grpcIDs, want) {
			t.Errorf("grpc TieBreakCreated = %v, want %v", grpcIDs, want)
		}
	})

	// ---------------------------------------------------------------
	// Sub-test 3: TieBreakID — 2 rows share both `updated` and
	// `created`. All three surfaces must fall through to (-id).
	// ---------------------------------------------------------------
	t.Run("TieBreakID", func(t *testing.T) {
		t.Parallel()
		fix := buildOrderingFixture(t)
		ctx := t.Context()

		org := fix.client.Organization.Create().
			SetID(1).SetName("TieBreakID Org").
			SetCreated(base).SetUpdated(base).
			SetStatus("ok").SaveX(ctx)

		// Both rows have identical updated AND created timestamps.
		// Only the id DESC tiebreak differentiates them.
		sameTs := base.Add(3 * time.Hour)
		fix.client.Network.Create().
			SetID(10).SetName("LowerID").SetAsn(64510).
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).SetAllowIxpUpdate(false).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(sameTs).SetUpdated(sameTs).
			SetStatus("ok").SaveX(ctx)
		fix.client.Network.Create().
			SetID(99).SetName("HigherID").SetAsn(64599).
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).SetAllowIxpUpdate(false).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(sameTs).SetUpdated(sameTs).
			SetStatus("ok").SaveX(ctx)

		// DESC by id => 99 first.
		want := []int{99, 10}

		compatIDs := fetchPdbcompatNetworkIDs(t, fix.server.URL)
		entrestIDs := fetchEntrestNetworkIDs(t, fix.server.URL, "")
		grpcIDs := fetchGrpcNetworkIDs(t, fix.server.URL, 100)

		if !idSliceEqual(compatIDs, want) {
			t.Errorf("pdbcompat TieBreakID = %v, want %v", compatIDs, want)
		}
		if !idSliceEqual(entrestIDs, want) {
			t.Errorf("entrest TieBreakID = %v, want %v", entrestIDs, want)
		}
		if !idSliceEqual(grpcIDs, want) {
			t.Errorf("grpc TieBreakID = %v, want %v", grpcIDs, want)
		}
	})
}

// =============================================================================
// TestEntrestDefaultOrder — ORDER-03 override contract. entrest under the
// compound-default template (Plan 02) must:
//   - default (no ?sort=) -> (-updated, -created, -id)
//   - ?sort=id&order=asc  -> ascending id (override honoured)
//   - ?sort=id&order=desc -> descending id
// =============================================================================

func TestEntrestDefaultOrder(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	t.Run("default", func(t *testing.T) {
		t.Parallel()
		fix := buildOrderingFixture(t)
		expected := seedNetworksWithUpdated(t, fix.client, 3, base)

		got := fetchEntrestNetworkIDs(t, fix.server.URL, "")
		if !idSliceEqual(got, expected) {
			t.Errorf("default order: got %v, want %v", got, expected)
		}
	})

	t.Run("explicit_id_asc", func(t *testing.T) {
		t.Parallel()
		fix := buildOrderingFixture(t)
		_ = seedNetworksWithUpdated(t, fix.client, 3, base)

		got := fetchEntrestNetworkIDs(t, fix.server.URL, "sort=id&order=asc")
		want := []int{1, 2, 3}
		if !idSliceEqual(got, want) {
			t.Errorf("?sort=id&order=asc: got %v, want %v", got, want)
		}
	})

	t.Run("explicit_id_desc", func(t *testing.T) {
		t.Parallel()
		fix := buildOrderingFixture(t)
		_ = seedNetworksWithUpdated(t, fix.client, 3, base)

		got := fetchEntrestNetworkIDs(t, fix.server.URL, "sort=id&order=desc")
		want := []int{3, 2, 1}
		if !idSliceEqual(got, want) {
			t.Errorf("?sort=id&order=desc: got %v, want %v", got, want)
		}
	})
}

// =============================================================================
// TestEntrestNestedSetOrder — CONTEXT.md D-04 clarification: entrest's
// auto-eager-load calls applySorting<Type> on nested edge arrays, so
// /rest/v1/networks responses MUST carry nested `edges.network_ix_lans`
// arrays in (-updated) DESC order. This locks in the Phase 67 Plan 01
// side-effect on `ent/rest/eagerload.go` (every WithNetworkIxLans
// closure now calls `applySortingNetworkIxLan(e, "updated", "desc")`).
//
// Note on URL shape: entrest auto-eagerloads via schema annotations
// (entrest.WithEagerLoad(true) on the Network.network_ix_lans edge at
// ent/schema/network.go:202). There's no `?depth=N` param — nested
// arrays are always present on /rest/v1/networks responses.
// =============================================================================

func TestEntrestNestedSetOrder(t *testing.T) {
	t.Parallel()

	t.Run("depth2", func(t *testing.T) {
		t.Parallel()
		fix := buildOrderingFixture(t)
		ctx := t.Context()
		base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

		// Seed hierarchy: Organization -> Network (parent)
		//                 Organization -> InternetExchange -> IxLan
		//                 Network + IxLan -> 3 NetworkIxLan children
		// Each NetworkIxLan gets a distinct `updated` so we can observe
		// the DESC-by-updated sort on the nested array.
		org := fix.client.Organization.Create().
			SetID(1).SetName("Nested Order Org").
			SetCreated(base).SetUpdated(base).
			SetStatus("ok").SaveX(ctx)

		net := fix.client.Network.Create().
			SetID(1).SetName("Parent Net").SetAsn(64500).
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).SetAllowIxpUpdate(false).
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(base).SetUpdated(base).
			SetStatus("ok").SaveX(ctx)

		ix := fix.client.InternetExchange.Create().
			SetID(1).SetName("Parent IX").
			SetOrgID(org.ID).SetOrganization(org).
			SetCreated(base).SetUpdated(base).
			SetStatus("ok").SaveX(ctx)

		ixlan := fix.client.IxLan.Create().
			SetID(1).
			SetInternetExchange(ix).
			SetCreated(base).SetUpdated(base).
			SetStatus("ok").SaveX(ctx)

		// 3 NetworkIxLan children, ids 10/20/30, updated spread 1h apart
		// so the expected DESC-by-updated order is [30, 20, 10].
		type nixlRow struct {
			id      int
			updated time.Time
		}
		rows := []nixlRow{
			{id: 10, updated: base},
			{id: 20, updated: base.Add(1 * time.Hour)},
			{id: 30, updated: base.Add(2 * time.Hour)},
		}
		for _, r := range rows {
			fix.client.NetworkIxLan.Create().
				SetID(r.id).
				SetAsn(64500).
				SetSpeed(10000).
				SetNetID(net.ID).SetNetwork(net).
				SetIxlanID(ixlan.ID).SetIxLan(ixlan).
				SetCreated(base).SetUpdated(r.updated).
				SetStatus("ok").
				SaveX(ctx)
		}

		// Request the Network list; entrest will auto-eagerload
		// network_ix_lans under edges per the schema annotation. Path:
		// content[0].edges.network_ix_lans[].id must be [30, 20, 10].
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
			fix.server.URL+"/rest/v1/networks", nil)
		if err != nil {
			t.Fatalf("build GET: %v", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /rest/v1/networks: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status=%d body=%s", resp.StatusCode, body)
		}

		var env struct {
			Content []struct {
				ID    int `json:"id"`
				Edges struct {
					NetworkIxLans []struct {
						ID      int       `json:"id"`
						Updated time.Time `json:"updated"`
					} `json:"network_ix_lans"`
				} `json:"edges"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatalf("decode: %v\nbody=%s", err, body)
		}
		if len(env.Content) != 1 {
			t.Fatalf("expected 1 network, got %d\nbody=%s", len(env.Content), body)
		}
		got := env.Content[0].Edges.NetworkIxLans
		if len(got) != 3 {
			t.Fatalf("expected 3 nested network_ix_lans, got %d\nbody=%s", len(got), body)
		}

		wantIDs := []int{30, 20, 10}
		gotIDs := make([]int, len(got))
		for i, r := range got {
			gotIDs[i] = r.ID
		}
		if !idSliceEqual(gotIDs, wantIDs) {
			t.Errorf("nested network_ix_lans ids = %v, want %v (D-04: eager-loaded edges must sort by -updated)", gotIDs, wantIDs)
		}

		// Timestamp monotonicity check: updated must strictly descend.
		// This is the invariant asserted at the contract level — ids
		// happen to correlate because of how we seeded, but the real
		// contract is "DESC by updated". Guard against a future seed
		// refactor that breaks the id/updated correlation.
		for i := 1; i < len(got); i++ {
			if !got[i-1].Updated.After(got[i].Updated) {
				t.Errorf("nested network_ix_lans[%d].updated=%s not after [%d].updated=%s",
					i-1, got[i-1].Updated, i, got[i].Updated)
			}
		}
	})
}
