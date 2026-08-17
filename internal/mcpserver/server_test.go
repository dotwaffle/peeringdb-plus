package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
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
	require.Equal(t, "2026-07-28", session.InitializeResult().ProtocolVersion)
	capabilities, err := json.Marshal(session.InitializeResult().Capabilities)
	require.NoError(t, err)
	assert.NotContains(t, string(capabilities), `"logging"`)
	require.NotNil(t, session.InitializeResult().Capabilities.Tools)
	assert.False(t, session.InitializeResult().Capabilities.Tools.ListChanged)

	tools, err := session.ListTools(context.Background(), nil)
	require.NoError(t, err)
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		assert.True(t, tool.Annotations.ReadOnlyHint)
		assert.NotNil(t, tool.OutputSchema, "%s has no output schema", tool.Name)
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

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "lookup_ip",
		Arguments: map[string]any{"ip": "not-an-ip"},
	})
	require.NoError(t, err)
	assert.True(t, result.IsError)
}

func TestStreamableHTTPWireCompatibility(t *testing.T) {
	t.Parallel()

	handler := New(Input{Version: "v-test", AllowedOrigins: "*"})

	t.Run("modern discovery", func(t *testing.T) {
		status, response := callRPC(t, handler, "2026-07-28", "server/discover", "server/discover", modernParams("2026-07-28"))
		require.Equal(t, http.StatusOK, status)
		result := requireObject(t, response, "result")
		assert.Contains(t, result["supportedVersions"], "2026-07-28")
		capabilities := requireObject(t, result, "capabilities")
		assert.NotContains(t, capabilities, "logging")
	})

	t.Run("header mismatch", func(t *testing.T) {
		status, response := callRPC(t, handler, "2026-07-28", "tools/list", "server/discover", modernParams("2026-07-28"))
		require.Equal(t, http.StatusBadRequest, status)
		errorObject := requireObject(t, response, "error")
		assert.Equal(t, float64(mcp.CodeHeaderMismatch), errorObject["code"])
	})

	t.Run("unsupported version", func(t *testing.T) {
		status, response := callRPC(t, handler, "2099-01-01", "server/discover", "server/discover", modernParams("2099-01-01"))
		require.Equal(t, http.StatusBadRequest, status)
		errorObject := requireObject(t, response, "error")
		assert.Equal(t, float64(mcp.CodeUnsupportedProtocolVersion), errorObject["code"])
		data := requireObject(t, errorObject, "data")
		assert.Contains(t, data["supported"], "2026-07-28")
		assert.Equal(t, "2099-01-01", data["requested"])
	})

	t.Run("legacy initialize", func(t *testing.T) {
		params := map[string]any{
			"protocolVersion": "2025-11-25",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "wire-test", "version": "v-test"},
		}
		status, response := callRPC(t, handler, "2025-11-25", "", "initialize", params)
		require.Equal(t, http.StatusOK, status)
		result := requireObject(t, response, "result")
		assert.Equal(t, "2025-11-25", result["protocolVersion"])
		capabilities := requireObject(t, result, "capabilities")
		assert.NotContains(t, capabilities, "logging")
	})
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

func modernParams(version string) map[string]any {
	return map[string]any{
		"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    version,
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "wire-test", "version": "v-test"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{},
		},
	}
}

func callRPC(
	t *testing.T,
	handler http.Handler,
	version string,
	headerMethod string,
	bodyMethod string,
	params map[string]any,
) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  bodyMethod,
		"params":  params,
	})
	require.NoError(t, err)

	request := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", version)
	if headerMethod != "" {
		request.Header.Set("Mcp-Method", headerMethod)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	var response map[string]any
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	return recorder.Code, response
}

func requireObject(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()
	value, ok := object[key].(map[string]any)
	require.True(t, ok, "%s is %T", key, object[key])
	return value
}
