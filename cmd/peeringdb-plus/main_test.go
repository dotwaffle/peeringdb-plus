package main

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/http2"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

func TestSyncReplay_FlyReplica(t *testing.T) {
	// When isPrimaryFn returns false AND FLY_REGION is set,
	// POST /sync should return 307 with fly-replay: region=lhr header.
	t.Setenv("FLY_REGION", "iad")
	t.Setenv("PRIMARY_REGION", "lhr")

	handler := newSyncHandler(t.Context(), SyncHandlerInput{
		IsPrimaryFn: func() bool { return false },
		SyncToken:   "test-token",
		DefaultMode: config.SyncModeFull,
		SyncFn:      func(_ context.Context, _ config.SyncMode) {},
	})

	req := httptest.NewRequest("POST", "/sync", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("want status %d, got %d", http.StatusTemporaryRedirect, rec.Code)
	}
	replay := rec.Header().Get("fly-replay")
	if replay != "region=lhr" {
		t.Fatalf("want fly-replay %q, got %q", "region=lhr", replay)
	}
}

func TestSyncReplay_LocalNonPrimary(t *testing.T) {
	// When isPrimaryFn returns false AND FLY_REGION is empty (local dev),
	// POST /sync should return 503 "not primary".
	// Ensure FLY_REGION is explicitly unset.
	t.Setenv("FLY_REGION", "")

	handler := newSyncHandler(t.Context(), SyncHandlerInput{
		IsPrimaryFn: func() bool { return false },
		SyncToken:   "test-token",
		DefaultMode: config.SyncModeFull,
		SyncFn:      func(_ context.Context, _ config.SyncMode) {},
	})

	req := httptest.NewRequest("POST", "/sync", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "not primary") {
		t.Fatalf("want body containing %q, got %q", "not primary", body)
	}
}

func TestSyncReplay_PrimaryDirect(t *testing.T) {
	// Table-driven tests for primary behavior (T-1).
	tests := []struct {
		name       string
		token      string
		header     string
		queryMode  string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "valid token returns 202",
			token:      "test-token",
			header:     "test-token",
			wantStatus: http.StatusAccepted,
			wantBody:   `{"status":"accepted"}`,
		},
		{
			name:       "missing token returns 401",
			token:      "test-token",
			header:     "",
			wantStatus: http.StatusUnauthorized,
			wantBody:   "unauthorized",
		},
		{
			name:       "invalid token returns 401",
			token:      "test-token",
			header:     "wrong-token",
			wantStatus: http.StatusUnauthorized,
			wantBody:   "unauthorized",
		},
		{
			name:       "mode override full returns 202",
			token:      "test-token",
			header:     "test-token",
			queryMode:  "full",
			wantStatus: http.StatusAccepted,
			wantBody:   `{"status":"accepted"}`,
		},
		{
			name:       "mode override incremental returns 202",
			token:      "test-token",
			header:     "test-token",
			queryMode:  "incremental",
			wantStatus: http.StatusAccepted,
			wantBody:   `{"status":"accepted"}`,
		},
		{
			name:       "invalid mode returns 400",
			token:      "test-token",
			header:     "test-token",
			queryMode:  "invalid",
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid mode",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newSyncHandler(t.Context(), SyncHandlerInput{
				IsPrimaryFn: func() bool { return true },
				SyncToken:   tt.token,
				DefaultMode: config.SyncModeFull,
				SyncFn:      func(_ context.Context, _ config.SyncMode) {},
			})

			path := "/sync"
			if tt.queryMode != "" {
				path += "?mode=" + tt.queryMode
			}
			req := httptest.NewRequest("POST", path, nil)
			if tt.header != "" {
				req.Header.Set("X-Sync-Token", tt.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("want status %d, got %d", tt.wantStatus, rec.Code)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantBody) {
				t.Fatalf("want body containing %q, got %q", tt.wantBody, body)
			}
		})
	}
}

func TestServerProtocols_H2C(t *testing.T) {
	// Verify h2c configuration: HTTP/1.1 + UnencryptedHTTP2 on same port.
	var protocols http.Protocols
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	if !protocols.HTTP1() {
		t.Fatal("want HTTP1 enabled")
	}
	if !protocols.UnencryptedHTTP2() {
		t.Fatal("want UnencryptedHTTP2 enabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Proto", r.Proto)
		w.WriteHeader(http.StatusOK)
	})

	srv := &http.Server{
		Handler:   mux,
		Protocols: &protocols,
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go srv.Serve(ln) //nolint:errcheck // test server
	t.Cleanup(func() { srv.Close() })

	// Make HTTP/2 prior-knowledge (h2c) request.
	h2Transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}
	h2Client := &http.Client{Transport: h2Transport}
	t.Cleanup(func() { h2Client.CloseIdleConnections() })

	resp, err := h2Client.Get("http://" + ln.Addr().String() + "/test")
	if err != nil {
		t.Fatalf("h2c request: %v", err)
	}
	defer resp.Body.Close()

	if resp.Proto != "HTTP/2.0" {
		t.Fatalf("want proto %q, got %q", "HTTP/2.0", resp.Proto)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want status %d, got %d", http.StatusOK, resp.StatusCode)
	}
}
