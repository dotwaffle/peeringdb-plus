package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/graph"
	pdbgql "github.com/dotwaffle/peeringdb-plus/internal/graphql"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestGraphQLAPI_ConnectionDefaultPageSize verifies that a Relay connection
// query supplying neither first nor last is bounded to DefaultLimit instead of
// materializing the whole table. Before the defaultFirst fix the entgql
// paginator applied no LIMIT, so the parameterless query below returned all
// 105 networks — an unbounded full-table scan and a connection of every row.
func TestGraphQLAPI_ConnectionDefaultPageSize(t *testing.T) {
	t.Parallel()
	const total = 105

	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	if err := pdbsync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init sync_status table: %v", err)
	}
	org, err := client.Organization.Create().
		SetID(1).SetName("PageSize Org").
		SetCreated(testTimestamp).SetUpdated(testTimestamp).
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	for i := range total {
		if _, err := client.Network.Create().
			SetID(1000 + i).
			SetName("Net").
			SetAsn(65000 + i).
			SetOrganization(org).
			SetCreated(testTimestamp).SetUpdated(testTimestamp).
			Save(ctx); err != nil {
			t.Fatalf("create network %d: %v", i, err)
		}
	}

	resolver := graph.NewResolver(client, db)
	mux := http.NewServeMux()
	mux.Handle("/graphql", pdbgql.NewHandler(resolver))
	srv := httptest.NewServer(middleware.CORS(middleware.CORSInput{AllowedOrigins: "*"})(mux))
	t.Cleanup(srv.Close)

	edgeCount := func(t *testing.T, query string) int {
		t.Helper()
		res := postGraphQL(t, srv.URL, query)
		if len(res.Errors) > 0 {
			t.Fatalf("query %q: errors %v", query, res.Errors)
		}
		var data struct {
			Networks struct {
				Edges []json.RawMessage `json:"edges"`
			} `json:"networks"`
		}
		if err := json.Unmarshal(res.Data, &data); err != nil {
			t.Fatalf("unmarshal %q: %v", query, err)
		}
		return len(data.Networks.Edges)
	}

	// No first/last: bounded to DefaultLimit, not all `total` rows.
	if got := edgeCount(t, `{ networks { edges { node { id } } } }`); got != graph.DefaultLimit {
		t.Errorf("parameterless networks connection returned %d edges, want DefaultLimit=%d (unbounded query leaked the whole %d-row table)", got, graph.DefaultLimit, total)
	}

	// An explicit page size is still honoured.
	if got := edgeCount(t, `{ networks(first: 5) { edges { node { id } } } }`); got != 5 {
		t.Errorf("networks(first: 5) returned %d edges, want 5", got)
	}
}
