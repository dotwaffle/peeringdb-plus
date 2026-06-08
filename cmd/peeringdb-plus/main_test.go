package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"

	"github.com/dotwaffle/peeringdb-plus/internal/config"
	"github.com/dotwaffle/peeringdb-plus/internal/database"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// TestDiscoveryBody locks the GET / service-discovery payload to a dynamic
// version (sourced from buildinfo via -ldflags injection) rather than the old
// hardcoded "0.1.0" banner, and guards that it stays valid JSON.
func TestDiscoveryBody(t *testing.T) {
	t.Parallel()
	body := discoveryBody("v9.9.9-test")
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("discovery body is not valid JSON: %v\nbody: %s", err, body)
	}
	if m["version"] != "v9.9.9-test" {
		t.Errorf("version = %v, want the passed-in v9.9.9-test (regression: must not be a hardcoded banner)", m["version"])
	}
	if m["name"] != "peeringdb-plus" {
		t.Errorf("name = %v, want peeringdb-plus", m["name"])
	}
	// The version must interpolate dynamically — distinct inputs yield distinct
	// bodies — so the old static "0.1.0" banner can't silently creep back.
	if strings.Contains(discoveryBody("vAAA"), "vBBB") || !strings.Contains(discoveryBody("vBBB"), "vBBB") {
		t.Error("discoveryBody must interpolate the version argument, not hardcode it")
	}
}

// TestRedactIxlanJSON_FastPath verifies the redact writer skips the
// re-marshal when nothing is gated out, returning the original bytes
// (audit P6). The input uses a non-alphabetical key order; json.Marshal
// sorts keys, so byte-equality to the input proves no marshal happened.
func TestRedactIxlanJSON_FastPath(t *testing.T) {
	t.Parallel()
	publicCtx := privctx.WithTier(context.Background(), privctx.TierPublic)
	usersCtx := privctx.WithTier(context.Background(), privctx.TierUsers)

	mk := func(visible string) []byte {
		return []byte(`{"ixf_ixp_member_list_url":"http://x/","ixf_ixp_member_list_url_visible":"` + visible + `","id":1}`)
	}

	t.Run("public field returns original bytes", func(t *testing.T) {
		t.Parallel()
		in := mk("Public")
		out, err := redactIxlanJSON(publicCtx, in)
		if err != nil {
			t.Fatalf("redact: %v", err)
		}
		if !bytes.Equal(out, in) {
			t.Errorf("expected original bytes (fast-path); got re-marshaled %s", out)
		}
	})

	t.Run("users field at users tier returns original bytes", func(t *testing.T) {
		t.Parallel()
		in := mk("Users")
		out, err := redactIxlanJSON(usersCtx, in)
		if err != nil {
			t.Fatalf("redact: %v", err)
		}
		if !bytes.Equal(out, in) {
			t.Errorf("expected original bytes (admitted for users tier); got %s", out)
		}
	})

	t.Run("users field for anonymous is redacted and re-marshaled", func(t *testing.T) {
		t.Parallel()
		in := mk("Users")
		out, err := redactIxlanJSON(publicCtx, in)
		if err != nil {
			t.Fatalf("redact: %v", err)
		}
		if bytes.Equal(out, in) {
			t.Fatal("expected redaction (re-marshal), got original bytes")
		}
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := m["ixf_ixp_member_list_url"]; ok {
			t.Error("gated url should be removed for anonymous caller")
		}
		if _, ok := m["ixf_ixp_member_list_url_visible"]; !ok {
			t.Error("_visible companion must still be emitted")
		}
	})
}

// flushRecorder is an http.ResponseWriter that records whether Flush was
// called, for the REST writer Flusher-contract test.
type flushRecorder struct {
	http.ResponseWriter
	flushed bool
}

func (f *flushRecorder) Flush() { f.flushed = true }

// plainWriter implements http.ResponseWriter but NOT http.Flusher.
type plainWriter struct{ h http.Header }

func (p plainWriter) Header() http.Header         { return p.h }
func (p plainWriter) Write(b []byte) (int, error) { return len(b), nil }
func (p plainWriter) WriteHeader(int)             {}

// TestRESTWriters_FlushContract verifies the http.Flusher contract on the
// two REST response-writer wrappers (audit M5/M7): the pass-through
// restErrorWriter delegates Flush to the underlying writer, while the
// buffering restFieldRedactWriter must NOT delegate — flushing mid-response
// would commit headers before its deferred body rewrite.
func TestRESTWriters_FlushContract(t *testing.T) {
	t.Parallel()

	t.Run("restErrorWriter delegates", func(t *testing.T) {
		t.Parallel()
		rec := &flushRecorder{ResponseWriter: httptest.NewRecorder()}
		w := &restErrorWriter{ResponseWriter: rec, r: httptest.NewRequest(http.MethodGet, "/rest/v1/x", nil)}
		w.Flush()
		if !rec.flushed {
			t.Error("restErrorWriter.Flush should delegate to the underlying Flusher")
		}
	})

	t.Run("restErrorWriter tolerates non-Flusher underlying", func(t *testing.T) {
		t.Parallel()
		w := &restErrorWriter{ResponseWriter: plainWriter{h: http.Header{}}, r: httptest.NewRequest(http.MethodGet, "/", nil)}
		w.Flush() // must not panic
	})

	t.Run("restFieldRedactWriter does not delegate", func(t *testing.T) {
		t.Parallel()
		rec := &flushRecorder{ResponseWriter: httptest.NewRecorder()}
		w := &restFieldRedactWriter{ResponseWriter: rec, ctx: context.Background()}
		w.Flush()
		if rec.flushed {
			t.Error("restFieldRedactWriter.Flush must be a no-op (it buffers; delegating would commit headers early)")
		}
	})
}

// TestFreshnessFromDB verifies the freshness gauge reads sync_status live:
// no successful sync reports no observation, a failed sync is ignored, and a
// successful sync reports its completion time. Backed by a real SQLite DB so
// the read path is exercised end-to-end without a metric reader.
func TestFreshnessFromDB(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	_, db, err := database.Open(filepath.Join(t.TempDir(), "freshness.db"), false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := pdbsync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	// No rows yet → no observation.
	if _, ok := freshnessFromDB(ctx, db); ok {
		t.Error("empty sync_status should report no observation")
	}

	// Most recent sync failed → still no observation (freshness tracks the
	// last *successful* sync only).
	failID, err := pdbsync.RecordSyncStart(ctx, db, time.Now(), "incremental")
	if err != nil {
		t.Fatalf("record failed start: %v", err)
	}
	if err := pdbsync.RecordSyncComplete(ctx, db, failID, pdbsync.Status{
		LastSyncAt: time.Now(),
		Status:     "failed",
	}); err != nil {
		t.Fatalf("record failed complete: %v", err)
	}
	if _, ok := freshnessFromDB(ctx, db); ok {
		t.Error("failed sync should report no observation")
	}

	// A successful sync → reports its completion time.
	want := time.Now().Add(-2 * time.Minute).UTC().Truncate(time.Second)
	okID, err := pdbsync.RecordSyncStart(ctx, db, want, "incremental")
	if err != nil {
		t.Fatalf("record success start: %v", err)
	}
	if err := pdbsync.RecordSyncComplete(ctx, db, okID, pdbsync.Status{
		LastSyncAt: want,
		Status:     "success",
	}); err != nil {
		t.Fatalf("record success complete: %v", err)
	}
	got, ok := freshnessFromDB(ctx, db)
	if !ok {
		t.Fatal("successful sync should report an observation")
	}
	if !got.Equal(want) {
		t.Errorf("freshness time = %v, want %v", got, want)
	}
}

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

func TestNewStartupPolicy(t *testing.T) {
	p := newStartupPolicy(true)
	if !p.ShouldMigrateSchema || !p.ShouldInitSyncStatus {
		t.Fatalf("primary policy should enable migration + sync status init, got %+v", p)
	}

	p = newStartupPolicy(false)
	if p.ShouldMigrateSchema || p.ShouldInitSyncStatus {
		t.Fatalf("replica policy should disable migration + sync status init, got %+v", p)
	}
}
