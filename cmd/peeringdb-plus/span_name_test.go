package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestRouteTagMiddleware_RenamesSpan verifies routeTagMiddleware renames the
// HTTP server span from the static otelhttp operation to the matched route, so
// every surface is distinguishable in trace search instead of all showing as
// "peeringdb-plus". A SpanRecorder captures a span started before dispatch
// (mimicking otelhttp's static-named span); after the middleware runs the
// recorded name must be the route pattern.
func TestRouteTagMiddleware_RenamesSpan(t *testing.T) {
	t.Parallel()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := tp.Tracer("test")

	mux := http.NewServeMux()
	mux.HandleFunc("GET /test/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// startSpan mimics otelhttp.NewMiddleware: it starts a span with the
	// static operation name before the inner chain runs and ends it after.
	startSpan := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := tracer.Start(r.Context(), "peeringdb-plus")
			defer span.End()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	handler := startSpan(routeTagMiddleware(mux))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test/42", nil))

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("recorded %d spans, want 1", len(spans))
	}
	if got, want := spans[0].Name(), "GET /test/{id}"; got != want {
		t.Errorf("span name = %q, want %q (routeTagMiddleware should rename from the static operation)", got, want)
	}
}
