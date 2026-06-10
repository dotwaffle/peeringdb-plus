package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// TestCaching_ErrorResponsesNotCacheable locks the 2026-06-10 audit fix:
// the caching middleware sets Cache-Control/ETag BEFORE dispatching, so
// without the cacheStripWriter every 400/404/413/500 inherited the
// public, hour-plus Cache-Control and the sync ETag — a shared cache
// replaying a transient error for a full sync interval.
func TestCaching_ErrorResponsesNotCacheable(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour)
	state.UpdateETag(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	mw := state.Middleware()

	for _, status := range []int{http.StatusBadRequest, http.StatusNotFound, http.StatusRequestEntityTooLarge, http.StatusInternalServerError} {
		rec := httptest.NewRecorder()
		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/net", nil))

		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("status %d: Cache-Control = %q, want no-store", status, got)
		}
		if got := rec.Header().Get("ETag"); got != "" {
			t.Errorf("status %d: ETag = %q, want absent", status, got)
		}
	}

	// Control: a 200 keeps the public caching headers.
	rec := httptest.NewRecorder()
	mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/net", nil))
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=3720" {
		t.Errorf("200: Cache-Control = %q, want public, max-age=3720", got)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("200: ETag missing")
	}
}

// TestCaching_HealthEndpointsSkipped asserts /healthz and /readyz are on
// the no-store skip list in production wiring (mirrored here): a cached
// health verdict — or a 304 short-circuit that skips the probes entirely
// — defeats the endpoint's purpose.
func TestCaching_HealthEndpointsSkipped(t *testing.T) {
	t.Parallel()

	state := middleware.NewCachingState(time.Hour, "/healthz", "/readyz")
	syncTime := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	state.UpdateETag(syncTime)
	mw := state.Middleware()

	for _, path := range []string{"/healthz", "/readyz"} {
		var handlerRan bool
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		// A conditional request matching the current ETag must STILL
		// reach the handler — no 304 short-circuit for health checks.
		req.Header.Set("If-None-Match", "*")
		mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			handlerRan = true
			w.WriteHeader(http.StatusOK)
		})).ServeHTTP(rec, req)

		if handlerRan == false {
			t.Errorf("%s: handler skipped by 304 short-circuit; health probes never ran", path)
		}
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", path, got)
		}
		if got := rec.Header().Get("ETag"); got != "" {
			t.Errorf("%s: ETag = %q, want absent", path, got)
		}
	}
}
