package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// TestPrivacyTier_StampsDefault verifies that a middleware constructed with
// DefaultTier: TierPublic stamps every inbound request's context so the
// inner handler observes TierPublic via privctx.TierFrom. This is the
// baseline behaviour for anonymous callers when PDBPLUS_PUBLIC_TIER is
// unset or "public" (D-11). Acceptance criterion 59-f.
func TestPrivacyTier_StampsDefault(t *testing.T) {
	t.Parallel()

	var observed privctx.Tier
	var called bool
	h := middleware.PrivacyTier(middleware.PrivacyTierInput{
		DefaultTier: privctx.TierPublic,
	})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		observed = privctx.TierFrom(r.Context())
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatalf("inner handler not invoked")
	}
	if observed != privctx.TierPublic {
		t.Fatalf("observed tier = %v, want TierPublic (%v)", observed, privctx.TierPublic)
	}
}

// TestPrivacyTier_StampsUsers verifies that a middleware constructed with
// DefaultTier: TierUsers stamps every inbound request's context so the
// inner handler observes TierUsers. This is the internal-deployment
// override path (PDBPLUS_PUBLIC_TIER=users) per D-11. Acceptance
// criterion 59-g.
func TestPrivacyTier_StampsUsers(t *testing.T) {
	t.Parallel()

	var observed privctx.Tier
	var called bool
	h := middleware.PrivacyTier(middleware.PrivacyTierInput{
		DefaultTier: privctx.TierUsers,
	})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		observed = privctx.TierFrom(r.Context())
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	if !called {
		t.Fatalf("inner handler not invoked")
	}
	if observed != privctx.TierUsers {
		t.Fatalf("observed tier = %v, want TierUsers (%v)", observed, privctx.TierUsers)
	}
}

// TestPrivacyTier_DoesNotModifyResponse is defence-in-depth against
// accidental header/body writes. The middleware is a pure context
// stamper — it must not set Vary, Set-Cookie, or any other response
// header, must not change the status code, and must not write a body.
func TestPrivacyTier_DoesNotModifyResponse(t *testing.T) {
	t.Parallel()

	h := middleware.PrivacyTier(middleware.PrivacyTierInput{
		DefaultTier: privctx.TierPublic,
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "" {
		t.Errorf("body = %q, want empty", body)
	}
	if got := len(rec.Header()); got != 0 {
		t.Errorf("response headers = %v, want none added by middleware", rec.Header())
	}
}

// upstreamKey is a custom context key used by
// TestPrivacyTier_UpstreamCtxValuesPreserved to simulate a prior
// middleware stamping an unrelated value.
type upstreamKey struct{}

// TestPrivacyTier_UpstreamCtxValuesPreserved verifies that when an
// upstream layer (e.g. a future Logging or OTel enrichment) has already
// stamped the request context, PrivacyTier layers the tier on top
// without clobbering that value.
func TestPrivacyTier_UpstreamCtxValuesPreserved(t *testing.T) {
	t.Parallel()

	const sentinel = "upstream-value-123"

	var (
		observedTier     privctx.Tier
		observedSentinel any
	)
	h := middleware.PrivacyTier(middleware.PrivacyTierInput{
		DefaultTier: privctx.TierUsers,
	})(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observedTier = privctx.TierFrom(r.Context())
		observedSentinel = r.Context().Value(upstreamKey{})
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), upstreamKey{}, sentinel))

	h.ServeHTTP(httptest.NewRecorder(), req)

	if observedTier != privctx.TierUsers {
		t.Errorf("tier = %v, want TierUsers (%v)", observedTier, privctx.TierUsers)
	}
	if observedSentinel != sentinel {
		t.Errorf("upstream value = %v, want %q (middleware clobbered parent ctx)", observedSentinel, sentinel)
	}
}
