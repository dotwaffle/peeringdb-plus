package pdbcompat

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestServeList_UnknownFilterFields_SilentlyIgnored verifies Phase 70
// D-05 / TRAVERSAL-04: unknown filter fields produce HTTP 200 with an
// unfiltered response, never 400. The slog.DebugContext call can't
// easily be asserted without a test slog handler — we rely on the
// observable HTTP behaviour and the OTel-attr test below.
func TestServeList_UnknownFilterFields_SilentlyIgnored(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	tests := []struct {
		name string
		path string
	}{
		{"unknown top-level field", "/api/net?totally_unknown_field=x"},
		{"3-hop over cap", "/api/net?a__b__c__d=y"},
		{"4-hop over cap", "/api/net?a__b__c__d__e=y"},
		{"unknown relation edge", "/api/net?bogus__name=z"},
		{"known edge, unknown target field", "/api/net?org__notarealcolumn=w"},
		{"combined: mix of unknown and valid filters", "/api/net?totally_unknown=x&org__id=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200 (TRAVERSAL-04: unknown fields must not 400): %s",
					rec.Code, rec.Body.String())
			}
			var env testEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("unmarshal response: %v", err)
			}
			if len(env.Data) == 0 {
				t.Errorf("expected non-empty data payload, got empty")
			}
		})
	}
}

// TestServeList_UnknownFilterFields_OTelAttrEmitted verifies Phase 70
// D-05 OTel span attribute emission. Uses a tracetest in-memory exporter
// to capture spans produced during the handler call.
func TestServeList_UnknownFilterFields_OTelAttrEmitted(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	exporter := tracetest.NewInMemoryExporter()
	tp := trace.NewTracerProvider(trace.WithSyncer(exporter))
	tracer := tp.Tracer("test")

	// Wrap the request with a span so SpanFromContext returns a valid
	// span inside serveList. Without this the SpanContext().IsValid()
	// guard in handler.go short-circuits the SetAttributes call.
	ctx, span := tracer.Start(context.Background(), "test-serve-list")

	req := httptest.NewRequest(http.MethodGet, "/api/net?unknown_field_one=a&bogus__field=b", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	span.End()

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	spans := exporter.GetSpans()
	if len(spans) == 0 {
		t.Fatal("expected at least one exported span")
	}
	var found bool
	for _, s := range spans {
		for _, a := range s.Attributes {
			if a.Key == attribute.Key("pdbplus.filter.unknown_fields") {
				v := a.Value.AsString()
				// Both unknowns should appear in the CSV (order not
				// guaranteed — url.Values is a map).
				if len(v) == 0 {
					t.Errorf("pdbplus.filter.unknown_fields attr present but empty")
				}
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected span attribute %q on at least one span; got %d spans",
			"pdbplus.filter.unknown_fields", len(spans))
	}
}

// TestServeList_ValidTraversalFilter_200 sanity-checks that a valid
// traversal filter (?org__id=1) returns 200 and a sensible row set —
// confirms the Phase 70 filter layer composes with the Phase 68 status
// matrix and Phase 69 preserved invariants in the full serveList path.
func TestServeList_ValidTraversalFilter_200(t *testing.T) {
	t.Parallel()
	_, mux := setupTestHandler(t)

	// setupTestHandler seeds 3 networks without organization links, so
	// ?org__id=1 returns zero rows (matches in SQL: no network has
	// org_id=1). The point of this test is that a traversal-shaped
	// filter does NOT 400 — behavioural correctness of row counts is
	// covered by the integration tests in filter_traversal_test.go.
	req := httptest.NewRequest(http.MethodGet, "/api/net?org__id=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}
