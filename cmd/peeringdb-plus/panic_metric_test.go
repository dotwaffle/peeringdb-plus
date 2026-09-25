package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
)

// TestRecoveredPanicRecordsRequestMetric locks that a handler panic
// reaches http.server.request.duration as a 500 with its http.route. It
// wraps in the order of buildMiddlewareChain (otelhttp -> Recovery ->
// r.WithContext hop -> routeTagMiddleware -> mux);
// TestMiddlewareChain_Order locks that order in server.go. With Recovery
// outside otelhttp, the panic unwound past the metric record, and the
// 5xx error rate did not count the 500.
func TestRecoveredPanicRecordsRequestMetric(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/{rest...}", func(http.ResponseWriter, *http.Request) {
		panic("test panic")
	})
	h := routeTagMiddleware(mux)
	h = privacyTierLikeMW(h)
	h = middleware.Recovery(slog.New(slog.DiscardHandler))(h)
	h = otelhttp.NewMiddleware("peeringdb-plus", otelhttp.WithMeterProvider(mp))(h)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/net", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	var count uint64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "http.server.request.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("%s data is %T, want a float64 histogram", m.Name, m.Data)
			}
			for _, dp := range hist.DataPoints {
				status, _ := dp.Attributes.Value(attribute.Key("http.response.status_code"))
				route, _ := dp.Attributes.Value(attribute.Key("http.route"))
				if status.AsInt64() != http.StatusInternalServerError || route.AsString() != "GET /api/{rest...}" {
					t.Errorf("data point attributes = %v, want status 500 and route %q",
						dp.Attributes.ToSlice(), "GET /api/{rest...}")
					continue
				}
				count += dp.Count
			}
		}
	}
	if count != 1 {
		t.Errorf("recorded %d requests with status 500 and the route, want 1", count)
	}
}
