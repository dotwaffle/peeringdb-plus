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

// TestLookupIPFindsNotOperationalConnection locks that lookup_ip returns a
// netixlan whose status is not-operational. PeeringDB 2.83.0 treats that
// status as live (models.py:109-122), and its IP search indexes these
// connections (documents.py:146-158). A deleted row with the same address
// stays out.
func TestLookupIPFindsNotOperationalConnection(t *testing.T) {
	t.Parallel()
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)

	org := c.Organization.Create().SetName("Status Org").SetStatus("ok").
		SetCreated(now).SetUpdated(now).SaveX(ctx)
	net := c.Network.Create().SetName("Dark Net").SetAsn(65002).SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	ix := c.InternetExchange.Create().SetName("Dark IX").SetOrganization(org).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	lan := c.IxLan.Create().SetInternetExchange(ix).
		SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	for _, status := range []string{"not-operational", "deleted"} {
		c.NetworkIxLan.Create().SetNetwork(net).SetIxLan(lan).SetIxID(ix.ID).
			SetAsn(65002).SetSpeed(1000).SetIpaddr4("192.0.2.20").
			SetStatus(status).SetOperational(false).
			SetCreated(now).SetUpdated(now).SaveX(ctx)
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", New(Input{Client: c, Version: "v-test", AllowedOrigins: "*"}))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v-test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/mcp"}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "lookup_ip",
		Arguments: map[string]any{"ip": "192.0.2.20"},
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
	assert.Equal(t, "not-operational", out.NetworkPresences[0]["status"])
}
