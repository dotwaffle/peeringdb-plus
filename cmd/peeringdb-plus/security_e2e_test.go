package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// Phase 78 UAT-02 — in-process regression lock for v1.13 Phase 53 security
// controls. Mirrors the live curl-driven assertions captured in
// .planning/phases/78-uat-closeout/UAT-RESULTS.md so any drift fails CI
// rather than the next year's UAT.
//
// Tests exercise SecurityHeaders + MaxBytesBody middleware directly rather
// than spinning up the full middleware chain — full-chain coverage lives in
// middleware_chain_test.go's structural source-scan.

const uatHSTSMaxAge = 180 * 24 * time.Hour
const uatBodyCap int64 = 1 << 20 // 1 MB — matches main.go maxRequestBodySize

func uatSecurityChain() http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	})
	mb := middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: uatBodyCap})(inner)
	return middleware.SecurityHeaders(middleware.SecurityHeadersInput{
		HSTSMaxAge:            uatHSTSMaxAge,
		HSTSIncludeSubDomains: true,
		FrameOptions:          "DENY",
		ContentTypeOptions:    true,
	})(mb)
}

// TestSecurity_HeadersOnUIPath asserts that browser paths receive all four
// headers: HSTS, X-Frame-Options, X-Content-Type-Options. Mirrors the live
// curl evidence captured in UAT-RESULTS.md § UAT-02 for /ui/.
func TestSecurity_HeadersOnUIPath(t *testing.T) {
	t.Parallel()

	h := uatSecurityChain()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	h.ServeHTTP(rec, req)

	wantHSTS := "max-age=15552000; includeSubDomains"
	if got := rec.Header().Get("Strict-Transport-Security"); got != wantHSTS {
		t.Errorf("Strict-Transport-Security = %q, want %q", got, wantHSTS)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want %q", got, "DENY")
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
	}
}

// TestSecurity_HeadersOnAPIPath asserts that JSON API paths receive HSTS
// and X-Content-Type-Options but NOT X-Frame-Options. The X-Frame-Options
// scoping is intentional per v1.13 Phase 53 — clickjacking protection only
// matters for HTML responses, not JSON. Mirrors live curl on /api/net.
func TestSecurity_HeadersOnAPIPath(t *testing.T) {
	t.Parallel()

	h := uatSecurityChain()
	for _, path := range []string{"/api/net", "/rest/v1/networks"} {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			h.ServeHTTP(rec, req)

			if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
				t.Errorf("%s missing Strict-Transport-Security header", path)
			}
			if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("%s X-Content-Type-Options = %q, want nosniff", path, got)
			}
			if got := rec.Header().Get("X-Frame-Options"); got != "" {
				t.Errorf("%s X-Frame-Options = %q, want empty (browser-only header)", path, got)
			}
		})
	}
}

// TestSecurity_BodyCapEnforced asserts that a request body exceeding
// maxRequestBodySize (1 MB) triggers http.MaxBytesError when the inner
// handler tries to drain the body. The MaxBytesBody middleware wraps r.Body
// with http.MaxBytesReader; consumers see the error when they Read past the
// cap.
func TestSecurity_BodyCapEnforced(t *testing.T) {
	t.Parallel()

	// Inner handler reads the entire body and reports any error back via
	// the response body so the test can assert the cap fired.
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, "read failure: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: uatBodyCap})(inner)

	cases := []struct {
		name     string
		bodySize int
		wantCode int
	}{
		{"under_cap_900k", 900 * 1024, http.StatusOK},
		{"over_cap_1100k", 1100 * 1024, http.StatusRequestEntityTooLarge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(make([]byte, tc.bodySize)))
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Errorf("%s: got status %d, want %d", tc.name, rec.Code, tc.wantCode)
			}
		})
	}
}

// TestSecurity_GRPCBypassesBodyCap asserts that ConnectRPC and gRPC paths
// bypass the body cap entirely. Streaming RPCs produce effectively unbounded
// bodies; truncating them would break the protocol. Verified against the
// live deployment by sending a 1.1 MB POST to /peeringdb.v1.NetworkService/
// ListNetworks — the upload completed in full (curl reported size_upload =
// 1126400 bytes) and the server returned a protocol-level 400 rather than
// a body-cap 413.
func TestSecurity_GRPCBypassesBodyCap(t *testing.T) {
	t.Parallel()

	// Inner handler captures the bytes it received so the test can verify
	// the full payload reached the inner handler unchanged.
	var received int
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failure: "+err.Error(), http.StatusInternalServerError)
			return
		}
		received = len(buf)
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: uatBodyCap})(inner)

	bypassPaths := []string{
		"/peeringdb.v1.NetworkService/ListNetworks",
		"/grpc.health.v1.Health/Check",
		"/grpc.foo.bar/Baz",
	}
	for _, path := range bypassPaths {
		t.Run(path, func(t *testing.T) {
			received = 0
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(make([]byte, 1100*1024)))
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: got status %d, want %d (bypass should reach inner handler)", path, rec.Code, http.StatusOK)
			}
			if received != 1100*1024 {
				t.Errorf("%s: received %d bytes, want %d (full body should pass through)", path, received, 1100*1024)
			}
		})
	}
}

// TestSecurity_BodyCapPathsNotInSkipList covers the negative case — paths
// that are NOT in maxBytesSkipPrefixes still get capped. Belt-and-suspenders
// for /graphql, /sync, /rest/v1/, /api/, /ui/.
func TestSecurity_BodyCapPathsNotInSkipList(t *testing.T) {
	t.Parallel()

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
				http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: uatBodyCap})(inner)

	// Paths intentionally NOT prefixed with /peeringdb.v1./ or /grpc. — must be capped.
	cappedPaths := []string{"/graphql", "/sync", "/rest/v1/networks", "/api/net", "/ui/"}
	for _, path := range cappedPaths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(make([]byte, 1100*1024)))
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("%s with 1.1 MB body: got status %d, want %d (not in skip-list, must be capped)",
					path, rec.Code, http.StatusRequestEntityTooLarge)
			}
		})
	}
}

// TestSecurity_HSTSValueShape asserts the HSTS header includes the
// includeSubDomains directive when configured. The 180-day max-age matches
// the production setting (cmd/peeringdb-plus/main.go HSTSMaxAge: 180 * 24h).
func TestSecurity_HSTSValueShape(t *testing.T) {
	t.Parallel()

	h := uatSecurityChain()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	h.ServeHTTP(rec, req)

	got := rec.Header().Get("Strict-Transport-Security")
	if !strings.HasPrefix(got, "max-age=") {
		t.Errorf("HSTS = %q, want max-age= prefix", got)
	}
	if !strings.Contains(got, "includeSubDomains") {
		t.Errorf("HSTS = %q, want includeSubDomains directive", got)
	}
}
