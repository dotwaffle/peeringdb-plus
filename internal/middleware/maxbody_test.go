package middleware_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// bodyConsumerHandler is a tiny test handler that drains r.Body and surfaces
// *http.MaxBytesError as HTTP 413. It is the minimal downstream needed to
// observe MaxBytesBody's effect without pulling in gql/connectrpc.
func bodyConsumerHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func TestMiddleware_MaxBytesReaderAppliesToAllPOST(t *testing.T) {
	t.Parallel()

	const limit = int64(1024) // 1 KB cap for the test

	mw := middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: limit})
	srv := httptest.NewServer(mw(bodyConsumerHandler()))
	t.Cleanup(srv.Close)

	tests := []struct {
		name string
		path string
	}{
		{name: "sync endpoint", path: "/sync"},
		{name: "graphql endpoint", path: "/graphql"},
		{name: "pdb compat api", path: "/api/net"},
		{name: "rest v1 networks", path: "/rest/v1/networks"},
		{name: "ui POST (unusual but capped)", path: "/ui/"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 2 KB oversized body — double the limit.
			body := bytes.Repeat([]byte("x"), int(limit*2))
			resp, err := http.Post(srv.URL+tc.path, "application/octet-stream", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("POST %s: %v", tc.path, err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })

			if resp.StatusCode != http.StatusRequestEntityTooLarge {
				t.Errorf("POST %s: got status %d, want %d (413)", tc.path, resp.StatusCode, http.StatusRequestEntityTooLarge)
			}
		})
	}
}

func TestMiddleware_MaxBytesReaderSkipsGRPC(t *testing.T) {
	t.Parallel()

	const limit = int64(1024)

	mw := middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: limit})
	srv := httptest.NewServer(mw(bodyConsumerHandler()))
	t.Cleanup(srv.Close)

	tests := []struct {
		name string
		path string
	}{
		{name: "connectrpc network service list", path: "/peeringdb.v1.NetworkService/ListNetworks"},
		{name: "grpc health check", path: "/grpc.health.v1.Health/Check"},
		{name: "grpc reflection", path: "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// 2 KB body — would trigger 413 on a capped path. Because this path
			// matches the skip list, the middleware must bypass entirely and the
			// downstream handler drains the full body with no error → 200.
			body := bytes.Repeat([]byte("x"), int(limit*2))
			resp, err := http.Post(srv.URL+tc.path, "application/octet-stream", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("POST %s: %v", tc.path, err)
			}
			t.Cleanup(func() { _ = resp.Body.Close() })

			if resp.StatusCode != http.StatusOK {
				t.Errorf("POST %s (bypass expected): got status %d, want %d (200)", tc.path, resp.StatusCode, http.StatusOK)
			}
		})
	}
}

func TestMiddleware_MaxBytesReaderAllowsSmallBodies(t *testing.T) {
	t.Parallel()

	const limit = int64(1024)

	mw := middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: limit})
	srv := httptest.NewServer(mw(bodyConsumerHandler()))
	t.Cleanup(srv.Close)

	// 512-byte body — well under the 1 KB limit.
	body := bytes.Repeat([]byte("y"), 512)
	resp, err := http.Post(srv.URL+"/graphql", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /graphql: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if resp.StatusCode != http.StatusOK {
		t.Errorf("POST /graphql with 512-byte body: got status %d, want %d (200)", resp.StatusCode, http.StatusOK)
	}
}
