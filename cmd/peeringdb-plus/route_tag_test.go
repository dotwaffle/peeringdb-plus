package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

// captureLabelerMW installs a fresh *otelhttp.Labeler in the request
// context BEFORE the inner handler runs, then exposes the same pointer
// post-call so the test can inspect the attributes routeTagMiddleware
// added during dispatch. This mirrors otelhttp.NewMiddleware's own
// labeler installation but lets the test bypass the production wiring
// and assert the ordering invariant directly.
func captureLabelerMW(captured **otelhttp.Labeler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lbl := &otelhttp.Labeler{}
			*captured = lbl
			ctx := otelhttp.ContextWithLabeler(r.Context(), lbl)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func TestRouteTagMiddleware(t *testing.T) {
	t.Parallel()

	type tc struct {
		name        string
		method      string
		target      string
		wantPattern string // empty = expect no http.route attribute
	}
	cases := []tc{
		{
			name:        "populates_labeler_with_pattern",
			method:      http.MethodGet,
			target:      "/test/42",
			wantPattern: "GET /test/{id}",
		},
		{
			name:        "unmatched_route_omits_label",
			method:      http.MethodGet,
			target:      "/no-such-route",
			wantPattern: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			mux.HandleFunc("GET /test/{id}", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			var captured *otelhttp.Labeler
			handler := captureLabelerMW(&captured)(routeTagMiddleware(mux))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(c.method, c.target, nil)
			handler.ServeHTTP(rec, req)

			if captured == nil {
				t.Fatal("labeler not captured by setup middleware")
			}
			attrs := captured.Get()

			var got string
			var found bool
			for _, a := range attrs {
				if a.Key == attribute.Key("http.route") {
					got = a.Value.AsString()
					found = true
					break
				}
			}

			if c.wantPattern == "" {
				if found {
					t.Errorf("unmatched route must not produce http.route label; got %q", got)
				}
				return
			}
			if !found {
				t.Fatalf("http.route attribute missing; got attrs=%v", attrs)
			}
			if got != c.wantPattern {
				t.Errorf("http.route = %q, want %q", got, c.wantPattern)
			}
		})
	}
}
