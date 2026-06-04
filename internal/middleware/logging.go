package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before delegating to the wrapped writer.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Flush delegates to the underlying writer if it implements http.Flusher.
// Required for gRPC streaming (server reflection, health watch).
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter, enabling middleware-aware
// interface checks via httputil.ResponseControllerFor.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// accessLogSkipPaths lists paths that bypass the per-request access log.
// Fly Proxy hits /healthz / /readyz once per ~15s on every machine; emitting
// an INFO line per probe drowned the access log (87% of all log volume in
// the 24h sample taken on 2026-04-26). Skipping here keeps the rest of the
// access-log surface (real traffic + 404 scanner probes) intact.
var accessLogSkipPaths = map[string]struct{}{
	"/healthz": {},
	"/readyz":  {},
}

// Logging returns middleware that logs each HTTP request with method, path,
// query, status, duration, and trace context (trace_id, span_id) when
// available. It also attaches the query string to the active server span as
// url.query. url.path alone left /api traffic uninspectable on both surfaces —
// the caller's actual intent (filters, since, depth, pagination) lives in the
// query string — so it is recorded on the access log and the trace alike,
// covering every API surface from one place. Uses structured slog with
// LogAttrs for an attribute-based API.
func Logging(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Capture the query before next.ServeHTTP (while the otelhttp
			// server span is unambiguously live and before any handler can
			// rewrite r.URL) and stamp it onto that span. SpanFromContext
			// returns a no-op span when tracing is unconfigured, so the
			// IsValid guard keeps this safe in tests.
			rawQuery := r.URL.RawQuery
			if rawQuery != "" {
				if span := trace.SpanFromContext(r.Context()); span.SpanContext().IsValid() {
					span.SetAttributes(attribute.String("url.query", rawQuery))
				}
			}

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			next.ServeHTTP(wrapped, r)

			if _, skip := accessLogSkipPaths[r.URL.Path]; skip {
				return
			}

			spanCtx := trace.SpanContextFromContext(r.Context())
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", wrapped.statusCode),
				slog.Duration("duration", time.Since(start)),
			}
			if rawQuery != "" {
				attrs = append(attrs, slog.String("query", rawQuery))
			}
			if spanCtx.HasTraceID() {
				attrs = append(attrs,
					slog.String("trace_id", spanCtx.TraceID().String()),
					slog.String("span_id", spanCtx.SpanID().String()),
				)
			}
			logger.LogAttrs(r.Context(), slog.LevelInfo, "http request", attrs...)
		})
	}
}
