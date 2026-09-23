package graph_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/graph"
	pdbgql "github.com/dotwaffle/peeringdb-plus/internal/graphql"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestGraphQLAPI_Meta checks that Network.meta and NetworkIxLan.meta
// resolve through the Map scalar: the stored document when set, null
// when the column is NULL.
func TestGraphQLAPI_Meta(t *testing.T) {
	t.Parallel()
	client, db := testutil.SetupClientWithDB(t)
	ctx := t.Context()
	if err := pdbsync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init sync_status table: %v", err)
	}

	netMeta := map[string]any{"preferred_ip_mtu": float64(9000), "rtbh_community": "65101:666"}
	nixlMeta := map[string]any{
		"planned_status_change": map[string]any{"status": "ok", "date": "2026-11-01"},
		"rfc8950":               true,
	}
	client.Network.Create().
		SetID(101).SetName("Meta Net").SetAsn(65101).
		SetMeta(netMeta).
		SetCreated(testTimestamp).SetUpdated(testTimestamp).
		SaveX(ctx)
	client.Network.Create().
		SetID(102).SetName("Plain Net").SetAsn(65102).
		SetCreated(testTimestamp).SetUpdated(testTimestamp).
		SaveX(ctx)
	client.NetworkIxLan.Create().
		SetID(201).SetAsn(65101).SetSpeed(10000).SetNetworkID(101).
		SetMeta(nixlMeta).
		SetCreated(testTimestamp).SetUpdated(testTimestamp).
		SaveX(ctx)

	mux := http.NewServeMux()
	mux.Handle("/graphql", pdbgql.NewHandler(graph.NewResolver(client, db)))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	result := postGraphQL(t, srv.URL, `{
		meta: networkByAsn(asn: 65101) { meta networkIxLans { meta } }
		plain: networkByAsn(asn: 65102) { meta }
	}`)
	if len(result.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", result.Errors)
	}

	type netixlan struct {
		Meta map[string]any `json:"meta"`
	}
	var data struct {
		Meta struct {
			Meta          map[string]any `json:"meta"`
			NetworkIxLans []netixlan     `json:"networkIxLans"`
		} `json:"meta"`
		Plain struct {
			Meta map[string]any `json:"meta"`
		} `json:"plain"`
	}
	if err := json.Unmarshal(result.Data, &data); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if !reflect.DeepEqual(data.Meta.Meta, netMeta) {
		t.Errorf("network meta = %v, want %v", data.Meta.Meta, netMeta)
	}
	if len(data.Meta.NetworkIxLans) != 1 {
		t.Fatalf("got %d netixlans, want 1", len(data.Meta.NetworkIxLans))
	}
	if got := data.Meta.NetworkIxLans[0].Meta; !reflect.DeepEqual(got, nixlMeta) {
		t.Errorf("netixlan meta = %v, want %v", got, nixlMeta)
	}
	if data.Plain.Meta != nil {
		t.Errorf("meta of a network without a document = %v, want null", data.Plain.Meta)
	}
}
