package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/otelconnect"
	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/internal/grpcserver"
)

// TestConnectHandlerOpts_ReadMaxBytes verifies that the shared ConnectRPC
// handler options cap inbound message size. The HTTP-level MaxBytesBody
// middleware deliberately skips /peeringdb.v1.* paths (streaming), so
// connect.WithReadMaxBytes is the ONLY body bound on these endpoints —
// without it a single oversized (or gzip-bombed) unary POST buffers fully
// in heap and can OOM a 256 MB replica.
func TestConnectHandlerOpts_ReadMaxBytes(t *testing.T) {
	t.Parallel()
	client := enttest.Open(t, dialect.SQLite,
		"file:connect_readmaxbytes?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	t.Cleanup(func() { _ = client.Close() })

	otelInterceptor, err := otelconnect.NewInterceptor(
		otelconnect.WithoutServerPeerAttributes(),
		otelconnect.WithoutTraceEvents(),
	)
	if err != nil {
		t.Fatalf("create otel interceptor: %v", err)
	}

	mux := http.NewServeMux()
	netPath, netHandler := peeringdbv1connect.NewNetworkServiceHandler(
		&grpcserver.NetworkService{Client: client, StreamTimeout: 30 * time.Second},
		connectHandlerOpts(otelInterceptor),
	)
	mux.Handle(netPath, netHandler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	listURL := srv.URL + "/peeringdb.v1.NetworkService/ListNetworks"

	t.Run("OversizedBodyRejected", func(t *testing.T) {
		t.Parallel()
		// 2 MB body — double the 1 MB cap. Size is enforced before
		// unmarshalling, so the padding content never matters.
		body := `{"pad":"` + strings.Repeat("a", 2<<20) + `"}`
		resp, err := http.Post(listURL, "application/json", bytes.NewReader([]byte(body))) //nolint:noctx // test helper
		if err != nil {
			t.Fatalf("POST oversized: %v", err)
		}
		defer resp.Body.Close()
		// connect maps CodeResourceExhausted to HTTP 429 for unary calls.
		if resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("oversized body: got HTTP %d, want %d (resource_exhausted)",
				resp.StatusCode, http.StatusTooManyRequests)
		}
	})

	t.Run("NormalBodyStillServed", func(t *testing.T) {
		t.Parallel()
		resp, err := http.Post(listURL, "application/json", strings.NewReader(`{}`)) //nolint:noctx // test helper
		if err != nil {
			t.Fatalf("POST normal: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("normal body: got HTTP %d, want 200", resp.StatusCode)
		}
	})
}
