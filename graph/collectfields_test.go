package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/graph"
	pdbgql "github.com/dotwaffle/peeringdb-plus/internal/graphql"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestGraphQLAPI_FlatListEagerLoadsEdges guards the CollectFields wiring
// in the flat-list resolvers (2026-06-10 audit): without it, every
// nested edge selection fell into ent's per-row lazy-load path — 1+N
// queries (and N otelsql spans) for networksList(limit:N){organization}.
// CollectFields batches the requested edges; this test locks the
// behavioral half (edges resolve through the eager-load path).
func TestGraphQLAPI_FlatListEagerLoadsEdges(t *testing.T) {
	t.Parallel()
	srv := seedTestData(t)

	resp := postGraphQL(t, srv.URL, `{
		networksList(limit: 10) {
			name
			asn
			organization { name }
		}
	}`)
	if len(resp.Errors) > 0 {
		t.Fatalf("unexpected errors: %+v", resp.Errors)
	}

	var data struct {
		NetworksList []struct {
			Name         string `json:"name"`
			Asn          int    `json:"asn"`
			Organization *struct {
				Name string `json:"name"`
			} `json:"organization"`
		} `json:"networksList"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode: %v\ndata: %s", err, resp.Data)
	}
	if len(data.NetworksList) == 0 {
		t.Fatal("networksList returned no rows")
	}
	for _, n := range data.NetworksList {
		if n.Organization == nil || n.Organization.Name == "" {
			t.Errorf("network %q: organization edge not resolved (CollectFields regression)", n.Name)
		}
	}
}

// TestGraphQLAPI_NetworkByAsnExcludesTombstones locks the status filter
// on the curated by-ASN lookup: a deleted network's ASN must resolve to
// null, matching the /api/ and /ui/ surfaces.
func TestGraphQLAPI_NetworkByAsnExcludesTombstones(t *testing.T) {
	t.Parallel()
	srv := seedDeletedNetwork(t)

	resp := postGraphQL(t, srv.URL, `{ networkByAsn(asn: 64999) { name } }`)
	if len(resp.Errors) > 0 {
		t.Fatalf("unexpected errors: %+v", resp.Errors)
	}
	var data struct {
		NetworkByAsn *struct {
			Name string `json:"name"`
		} `json:"networkByAsn"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode: %v\ndata: %s", err, resp.Data)
	}
	if data.NetworkByAsn != nil {
		t.Errorf("networkByAsn resolved a tombstoned network: %+v", data.NetworkByAsn)
	}
}

// seedDeletedNetwork builds a server whose only ASN-64999 network is a
// soft-deleted tombstone.
func seedDeletedNetwork(t *testing.T) *httptest.Server {
	t.Helper()
	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	if err := pdbsync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init sync_status table: %v", err)
	}
	ts := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	org, err := client.Organization.Create().
		SetID(1).SetName("Tombstone Org").
		SetCreated(ts).SetUpdated(ts).SetStatus("ok").
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if _, err := client.Network.Create().
		SetID(1).SetName("Deleted Net").SetAsn(64999).SetOrganization(org).
		SetCreated(ts).SetUpdated(ts).SetStatus("deleted").
		Save(ctx); err != nil {
		t.Fatalf("create network: %v", err)
	}

	resolver := graph.NewResolver(client, db)
	gqlHandler := pdbgql.NewHandler(resolver)
	mux := http.NewServeMux()
	mux.Handle("/graphql", gqlHandler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
