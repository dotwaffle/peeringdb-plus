package main

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// buildServer constructs the production http.Server with all timeouts
// deliberately set. WriteTimeout is explicitly 0 because StreamEntities in
// internal/grpcserver/generic.go already enforces cfg.StreamTimeout per
// stream via context.WithTimeout; a server-wide WriteTimeout would race
// with it and silently truncate streams.
//
// ReadHeaderTimeout=10s mitigates slowloris header-stall attacks;
// ReadTimeout=30s mitigates slowloris body-stall attacks;
// IdleTimeout=120s caps keep-alive idle connections.
// Go 1.26 net/http godoc: "A zero or negative value means there will be
// no timeout" — WriteTimeout:0 is safe for long-lived h2c streams.
//
// TestServer_NoWriteTimeoutOnStreamingPaths regression-locks every field;
// any drift fails CI.
func buildServer(addr string, handler http.Handler, protocols *http.Protocols) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		// WriteTimeout intentionally 0 — see buildServer doc comment.
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
}

// chainConfig bundles the inputs for buildMiddlewareChain. It is a plain
// data struct rather than a fluent builder — the chain is locked and
// every field is required at startup.
type chainConfig struct {
	Logger       *slog.Logger
	CORSOrigins  string // comma-separated list of allowed CORS origins
	CSPInput     middleware.CSPInput
	CachingState *middleware.CachingState
	SyncWorker   middleware.SyncReadiness
	MaxBodyBytes int64
	HSTSMaxAge   time.Duration
	// DefaultTier is the resolved PDBPLUS_PUBLIC_TIER value stamped on
	// every inbound request context by middleware.PrivacyTier. Consumed
	// downstream by the ent privacy policies on visibility-bearing
	// entities (see ent/schema/poc.go). Zero value is TierPublic — the
	// safest default if unset.
	DefaultTier privctx.Tier
}

// buildMiddlewareChain wraps the innermost handler in the full production
// middleware stack, returning the outermost handler. The chain order is:
//
//	Recovery -> MaxBytesBody -> CORS -> OTel HTTP -> Logging ->
//	PrivacyTier -> Readiness -> SecurityHeaders -> CSP -> Caching ->
//	Gzip -> RouteTag -> innermost
//
// The code below wraps innermost-first (RouteTag is wrapped first so it
// sits closest to the handler; Recovery is wrapped last so it sits
// outermost). This ordering is regression-locked by
// TestMiddlewareChain_Order, which source-scans this function body and
// asserts the literal wrap order.
//
// RouteTag must be the innermost wrap so its `next.ServeHTTP(mux)` is the
// real mux dispatch — only then does r.Pattern get populated, which
// routeTagMiddleware reads to set the http.route metric label.
//
// SecurityHeaders sits between Readiness and CSP so HSTS/XCTO fire
// on every response, including the Readiness 503 syncing page, and XFO
// stays scoped to browser paths via middleware.isBrowserPath. HSTSMaxAge is
// passed through chainConfig so the deployment default can be managed in one
// place without touching this helper.
//
// PrivacyTier sits between Logging and Readiness in
// request flow so every request ctx — including the Readiness 503 path —
// carries the resolved PDBPLUS_PUBLIC_TIER before any handler or ent
// query reads it. Placing it inside Logging (rather than outside) keeps
// Recovery/Logging free of tier coupling while still stamping the ctx
// before any downstream observation of the request.
func buildMiddlewareChain(inner http.Handler, cc chainConfig) http.Handler {
	h := routeTagMiddleware(inner)
	h = middleware.Compression()(h)
	h = cc.CachingState.Middleware()(h)
	h = middleware.CSP(cc.CSPInput)(h)
	h = middleware.SecurityHeaders(middleware.SecurityHeadersInput{
		HSTSMaxAge:                cc.HSTSMaxAge,
		HSTSIncludeSubDomains:     true,
		FrameOptions:              "DENY",
		ContentTypeOptions:        true,
		ReferrerPolicy:            "strict-origin-when-cross-origin",
		CrossOriginOpenerPolicy:   "same-origin",
		CrossOriginResourcePolicy: "same-origin",
	})(h)
	h = middleware.Readiness(cc.SyncWorker, h)
	h = middleware.PrivacyTier(middleware.PrivacyTierInput{DefaultTier: cc.DefaultTier})(h)
	h = middleware.Logging(cc.Logger)(h)
	// Public endpoint: start a fresh root span per request instead of joining
	// any client-supplied traceparent. We serve arbitrary external callers, so
	// inheriting their trace context would let them pick our trace-ids and —
	// through the ParentBased sampler — dictate our sampling decision (a
	// sampled traceparent overriding the per-route rate is a trace-volume/cost
	// vector). The inbound span context is preserved as a link, not a parent;
	// the global propagator (outbound propagation, baggage) is unaffected.
	h = otelhttp.NewMiddleware("peeringdb-plus",
		otelhttp.WithPublicEndpointFn(func(*http.Request) bool { return true }))(h)
	h = middleware.CORS(middleware.CORSInput{AllowedOrigins: cc.CORSOrigins})(h)
	h = middleware.MaxBytesBody(middleware.MaxBytesBodyInput{MaxBytes: cc.MaxBodyBytes})(h)
	h = middleware.Recovery(cc.Logger)(h)
	return h
}
