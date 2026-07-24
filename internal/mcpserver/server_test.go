package mcpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamableHTTPDiscovery(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("/mcp", New(Input{Version: "v-test", AllowedOrigins: "*"}))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v-test"}, nil)
	session, err := client.Connect(
		context.Background(),
		&mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/mcp"},
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	tools, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint)
	}
	assert.ElementsMatch(t, []string{
		"search_peeringdb",
		"get_network",
		"get_exchange",
		"get_facility",
		"get_organization",
		"get_campus",
		"get_carrier",
		"compare_networks",
		"lookup_ip",
		"get_sync_status",
	}, names)

	resources, err := session.ListResources(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, resources.Resources, 2)

	prompts, err := session.ListPrompts(context.Background(), nil)
	require.NoError(t, err)
	require.Len(t, prompts.Prompts, 2)
}

func TestStreamableHTTPConfiguredCrossOrigin(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("/mcp", New(Input{
		Version:        "v-test",
		AllowedOrigins: "https://agent.example",
	}))
	httpServer := httptest.NewServer(mux)
	t.Cleanup(httpServer.Close)

	httpClient := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		request.Header.Set("Origin", "https://agent.example")
		return http.DefaultTransport.RoundTrip(request)
	})}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v-test"}, nil)
	session, err := client.Connect(
		context.Background(),
		&mcp.StreamableClientTransport{Endpoint: httpServer.URL + "/mcp", HTTPClient: httpClient},
		nil,
	)
	require.NoError(t, err)
	require.NoError(t, session.Close())
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
