package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestLookupIPCarriesMeta locks that lookup_ip, which returns raw ent
// netixlan rows, carries the PeeringDB 2.83.0 meta document of each row,
// and {} for a row without a stored document.
func TestLookupIPCarriesMeta(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	plan := map[string]any{
		"planned_status_change": map[string]any{"status": "deleted", "date": "2026-12-31"},
		"rfc8950":               true,
	}

	org := c.Organization.Create().SetName("Meta Org").SetStatus("ok").
		SetCreated(now).SetUpdated(now).SaveX(ctx)
	net := c.Network.Create().SetName("Meta Net").SetAsn(65001).SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	ix := c.InternetExchange.Create().SetName("Meta IX").SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	lan := c.IxLan.Create().SetInternetExchange(ix).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	c.NetworkIxLan.Create().SetNetwork(net).SetIxLan(lan).SetIxID(ix.ID).
		SetAsn(65001).SetSpeed(1000).SetIpaddr4("192.0.2.10").SetMeta(plan).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	c.NetworkIxLan.Create().SetNetwork(net).SetIxLan(lan).SetIxID(ix.ID).
		SetAsn(65001).SetSpeed(1000).SetIpaddr4("192.0.2.11").
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)

	mux := http.NewServeMux()
	mux.Handle("/mcp", New(Input{Client: c, Version: "v-test", AllowedOrigins: "*"}))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v-test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/mcp"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	lookupMeta := func(ip string) any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "lookup_ip",
			Arguments: map[string]any{"ip": ip},
		})
		require.NoError(t, err)
		require.False(t, result.IsError, "lookup_ip failed: %+v", result.Content)

		raw, err := json.Marshal(result.StructuredContent)
		require.NoError(t, err)
		var out struct {
			NetworkPresences []map[string]any `json:"network_presences"`
		}
		require.NoError(t, json.Unmarshal(raw, &out))
		require.Len(t, out.NetworkPresences, 1)
		meta, present := out.NetworkPresences[0]["meta"]
		require.True(t, present, "meta key missing for %s", ip)
		return meta
	}

	assert.Equal(t, plan, lookupMeta("192.0.2.10"))
	assert.Equal(t, map[string]any{}, lookupMeta("192.0.2.11"), "a row without a stored document must carry {}, not null")
}
