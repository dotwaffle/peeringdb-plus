package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

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

// installInMemoryTracer installs a TracerProvider with an InMemoryExporter
// (sync-exported) as the global provider for the duration of the test.
// Returns the exporter so the test body can inspect captured spans.
//
// Matches the established in-tree pattern used by
// internal/peeringdb/client_test.go (setupTraceTest). WithSyncer is used
// (not the default BatchSpanProcessor) so captured spans are available
// synchronously on End — batching would race against the test assertion.
func installInMemoryTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return exporter
}

// findStringAttr returns the string value of the named attribute on
// the captured span, or ("", false) if not set. The scan is linear;
// attribute lists on HTTP server spans are small and unindexed in
// tracetest.SpanStub.
func findStringAttr(span tracetest.SpanStub, key string) (string, bool) {
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			return a.Value.AsString(), true
		}
	}
	return "", false
}

// TestPrivacyTier_SetsOTelAttribute covers OBS-03 / D-07 / D-09.
// The PrivacyTier middleware must stamp pdbplus.privacy.tier on the
// active HTTP server span with the canonical string form of the
// resolved tier. Values are strictly "public" or "users" — a third
// value indicates cardinality drift and fails the test.
//
// The test does NOT import otelhttp. A thin in-test span wrapper
// starts a span via otel.Tracer("test").Start(...) around PrivacyTier,
// simulating the placement of otelhttp.NewMiddleware one layer out in
// the real chain. This keeps the test focused on PrivacyTier's
// stamping behaviour, not on otelhttp's internals.
func TestPrivacyTier_SetsOTelAttribute(t *testing.T) {
	// Not parallel at the top level: subtests share the global
	// TracerProvider and each subtest re-installs its own exporter.
	cases := []struct {
		name    string
		tier    privctx.Tier
		wantStr string
	}{
		{"public", privctx.TierPublic, "public"},
		{"users", privctx.TierUsers, "users"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// No t.Parallel: installInMemoryTracer mutates the global
			// OTel TracerProvider and concurrent subtests would race.
			exporter := installInMemoryTracer(t)

			var observedTier privctx.Tier
			var observedTierSet bool
			inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				observedTier = privctx.TierFrom(r.Context())
				observedTierSet = true
			})

			privacyMW := middleware.PrivacyTier(middleware.PrivacyTierInput{
				DefaultTier: tc.tier,
			})

			// Simulate the otelhttp-created HTTP server span around
			// PrivacyTier so trace.SpanFromContext inside the middleware
			// has a live, recording span to stamp attributes on.
			spanWrapper := func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctx, span := otel.Tracer("test").Start(r.Context(), "http.server")
					defer span.End()
					next.ServeHTTP(w, r.WithContext(ctx))
				})
			}

			handler := spanWrapper(privacyMW(inner))
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if !observedTierSet {
				t.Fatal("inner handler did not run")
			}
			if observedTier != tc.tier {
				t.Errorf("privctx.TierFrom = %v, want %v", observedTier, tc.tier)
			}

			spans := exporter.GetSpans()
			if len(spans) != 1 {
				t.Fatalf("got %d spans, want 1; spans=%+v", len(spans), spans)
			}
			got, ok := findStringAttr(spans[0], "pdbplus.privacy.tier")
			if !ok {
				t.Fatalf("span missing attribute pdbplus.privacy.tier; attrs=%+v", spans[0].Attributes)
			}
			if got != tc.wantStr {
				t.Errorf("pdbplus.privacy.tier = %q, want %q", got, tc.wantStr)
			}

			// Defence-in-depth: assert the attribute appears exactly
			// once — a stray duplicate stamping from a future refactor
			// would bloat cardinality-per-span even though the value
			// matches, and should fail the regression.
			var count int
			for _, a := range spans[0].Attributes {
				if string(a.Key) == "pdbplus.privacy.tier" {
					count++
				}
			}
			if count != 1 {
				t.Errorf("pdbplus.privacy.tier attribute count = %d, want 1", count)
			}
		})
	}
}

// TestPrivacyTier_NoSpanSafe covers the fail-safe-closed behaviour when
// no active span is in ctx (e.g. unit tests that don't install a
// tracer, or a middleware wired before otelhttp in a misconfigured
// chain). The middleware must not panic and must still stamp the
// privctx.Tier so downstream privacy filtering remains correct.
func TestPrivacyTier_NoSpanSafe(t *testing.T) {
	// Not parallel: mutates the global TracerProvider.

	// Install a default (no-op-by-default) tracer provider so that
	// trace.SpanFromContext on a bare ctx returns the noop span. Any
	// previous subtest's provider is replaced here; t.Cleanup restores
	// a clean provider after the test returns.
	tp := sdktrace.NewTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	var observedTier privctx.Tier
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		observedTier = privctx.TierFrom(r.Context())
	})

	handler := middleware.PrivacyTier(middleware.PrivacyTierInput{
		DefaultTier: privctx.TierUsers,
	})(inner)

	req := httptest.NewRequest(http.MethodGet, "/nospan", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if observedTier != privctx.TierUsers {
		t.Errorf("privctx.TierFrom = %v, want TierUsers even without an active span", observedTier)
	}
}
