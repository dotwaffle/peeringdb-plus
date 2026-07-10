package main

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// routeTagMiddleware injects http.route into the otelhttp labeler AFTER
// the mux dispatches a request. otelhttp.NewMiddleware reads the labeler
// for metric attributes inside RecordMetrics AFTER its inner
// next.ServeHTTP returns; the Labeler pointer is INSTALLED into ctx at
// otelhttp@v0.68.0/handler.go:172 (LabelerFromContext + ContextWithLabeler
// backfill) and the *Labeler.Get() READ for metric attribute emission
// happens at handler.go:202 inside the MetricAttributes literal that
// RecordMetrics consumes. A post-dispatch labeler mutation here is
// therefore visible to the metric record pass.
//
// Why a tail middleware instead of otelhttp.WithRouteTag: that option does
// not exist in v0.68.0. The Labeler is the supported escape hatch for
// adding metric attributes after the framework has dispatched.
//
// Why this middleware exists at all when otelhttp v0.68.0 ALREADY emits
// http.route natively from req.Pattern at semconv/server.go:367-368:
// production middleware between otelhttp and the mux (e.g.,
// middleware.PrivacyTier) calls r.WithContext(...) which creates a NEW
// *http.Request struct (per net/http/request.go:368-376
// `r2 := *r; r2.ctx = ctx`). The mux populates Pattern on that NEW r2,
// not on otelhttp's local r — so otelhttp's NATIVE Pattern-read returns
// empty, and the labeler-add path here is the only source of http.route
// in the metric attribute set on production-shaped chains. The shared
// *Labeler pointer in ctx (installed via context.WithValue at
// otelhttp/labeler.go:44) IS preserved across r.WithContext-derived
// requests, so this middleware's post-dispatch mutation IS visible to
// the otelhttp metric record pass even though Pattern is not.
// See the project history
// for the empirical evidence that drove this design.
//
// Empty r.Pattern (unmatched routes / NotFound) is skipped so we do not
// emit an http.route="" label that would balloon Prometheus cardinality
// for 404 traffic.
func routeTagMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Pattern == "" {
			return
		}
		// Rename the otelhttp server span from the static "peeringdb-plus"
		// operation to the matched route so every HTTP surface is
		// distinguishable in trace search and span-name TraceQL filters.
		// Same rationale as the labeler below: otelhttp's native Pattern read
		// returns empty because middleware re-derives the request, but the
		// span lives in ctx and is still recording here (routeTagMiddleware
		// runs inside the otelhttp span), so SetName after dispatch is valid.
		// r.Pattern carries the method ("GET /api/net/{id}") under method
		// routing, matching the OTel "{method} {route}" span-name convention.
		trace.SpanFromContext(r.Context()).SetName(r.Pattern)
		labeler, ok := otelhttp.LabelerFromContext(r.Context())
		if !ok {
			return
		}
		labeler.Add(attribute.String("http.route", r.Pattern))
	})
}
