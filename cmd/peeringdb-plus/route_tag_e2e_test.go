package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
)

// privacyTierLikeMW mimics the production middleware.PrivacyTier wrap by
// installing a sentinel value into ctx and calling next.ServeHTTP with
// r.WithContext(ctx). This forces a NEW *http.Request struct between
// otelhttp's local r and the mux's dispatch, mirroring the production
// runtime flow:
//
//	otelhttp -> Logging -> PrivacyTier (r.WithContext) -> ...
//	-> routeTagMiddleware -> mux
//
// The empirical OBS-04 investigation (see
// .planning/phases/75-code-side-observability/OBS-04-INVESTIGATION.md
// § Direct metric-record verification) proves that this WithContext
// hop hides r.Pattern from otelhttp's local r AFTER mux dispatch
// returns — but the *Labeler pointer is preserved in the propagated
// ctx, so routeTagMiddleware's tail mutation IS visible to the
// labeler.Get() call inside otelhttp's MetricAttributes literal.
//
// This wrap is the load-bearing piece of the E2E test: without it,
// otelhttp's NATIVE http.route emission (semconv/server.go:367-368)
// fires for every request, masking any latent bug in routeTagMiddleware.
// With it, the labeler-add path is the SOLE source of http.route, and
// the test exercises exactly the production-shaped failure-mode
// investigated in OBS-04.
func privacyTierLikeMW(next http.Handler) http.Handler {
	type tierKey struct{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), tierKey{}, "users")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// TestRouteTag_E2E_AllRouteFamilies asserts the production
// routeTagMiddleware populates the http.route label on the otelhttp
// labeler for ALL major route families, not just /healthz. Mirrors
// the production wrap shape: otelhttp wraps the chain from outside,
// then a privacyTierLikeMW sits between otelhttp and routeTagMiddleware
// (modelling middleware.PrivacyTier's r.WithContext call), and
// routeTagMiddleware sits closest to the mux.
//
// The captureLabelerMW helper from route_tag_test.go installs a
// synthetic *otelhttp.Labeler into ctx BEFORE the inner handler runs
// so the test can inspect the attributes routeTagMiddleware added
// during dispatch, without spinning up a real otelhttp middleware
// (which would also natively emit http.route via semconv/server.go:367
// and mask any bug in our middleware).
//
// Phase 75 OBS-04 regression guard: if the http.route label stops
// populating for any of the 4 route families, this test fails. The
// failure mode is the production-only-/healthz observation that
// triggered OBS-04 in the first place — locking it via E2E means a
// future middleware-chain reshuffle or otelhttp upgrade that breaks
// the labeler-add path is caught at CI time rather than in production
// dashboard regression.
func TestRouteTag_E2E_AllRouteFamilies(t *testing.T) {
	t.Parallel()

	type tc struct {
		name        string
		method      string
		target      string
		wantPattern string
	}
	cases := []tc{
		{"healthz_works", http.MethodGet, "/healthz", "GET /healthz"},
		{"api_family", http.MethodGet, "/api/networks", "GET /api/{rest...}"},
		{"rest_v1_family", http.MethodGet, "/rest/v1/networks", "/rest/v1/"},
		{"graphql", http.MethodPost, "/graphql", "/graphql"},
		{"ui_family", http.MethodGet, "/ui/asn/13335", "GET /ui/{rest...}"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			mux := http.NewServeMux()
			// Mirror the production registration shapes from main.go.
			mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			mux.HandleFunc("GET /api/{rest...}", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			mux.Handle("/rest/v1/", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			mux.HandleFunc("/graphql", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			mux.HandleFunc("GET /ui/{rest...}", func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			var captured *otelhttp.Labeler
			// Wire: captureLabelerMW (installs synthetic labeler) ->
			// privacyTierLikeMW (forces r.WithContext between labeler
			// install and mux) -> routeTagMiddleware -> mux.
			// captureLabelerMW is defined in route_tag_test.go (same
			// package main).
			handler := captureLabelerMW(&captured)(privacyTierLikeMW(routeTagMiddleware(mux)))

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(c.method, c.target, nil)
			handler.ServeHTTP(rec, req)

			if captured == nil {
				t.Fatal("labeler not captured")
			}

			var got string
			var found bool
			for _, a := range captured.Get() {
				if a.Key == attribute.Key("http.route") {
					got = a.Value.AsString()
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("http.route attribute missing for %s %s; got attrs=%v",
					c.method, c.target, captured.Get())
			}
			if got != c.wantPattern {
				t.Errorf("http.route = %q, want %q (target=%s %s)",
					got, c.wantPattern, c.method, c.target)
			}
		})
	}
}

// TestRouteTag_E2E_HealthzStillWorks is a regression guard ensuring
// the OBS-04 fix did not accidentally break the one route family that
// already worked in production prior to the fix. /healthz must
// continue to produce http.route="GET /healthz" through the same
// chain shape as the multi-family test.
func TestRouteTag_E2E_HealthzStillWorks(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var captured *otelhttp.Labeler
	handler := captureLabelerMW(&captured)(privacyTierLikeMW(routeTagMiddleware(mux)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	handler.ServeHTTP(rec, req)

	if captured == nil {
		t.Fatal("labeler not captured")
	}
	var got string
	for _, a := range captured.Get() {
		if a.Key == attribute.Key("http.route") {
			got = a.Value.AsString()
			break
		}
	}
	if got != "GET /healthz" {
		t.Errorf("/healthz http.route = %q, want %q", got, "GET /healthz")
	}
}

// TestRouteTag_E2E_UnmatchedOmitsLabel is a regression guard ensuring
// an unmatched route still produces NO http.route label. This matches
// the existing unit-test invariant in route_tag_test.go's
// "unmatched_route_omits_label" sub-case but exercised through the
// production-shaped chain (with the privacyTierLikeMW wrap that the
// E2E test models).
//
// Without this guard, a future change to routeTagMiddleware that
// inadvertently set http.route="" on unmatched routes would balloon
// Prometheus cardinality for 404 traffic — directly contradicting
// the existing main.go:917-919 doc-comment invariant.
func TestRouteTag_E2E_UnmatchedOmitsLabel(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /test", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var captured *otelhttp.Labeler
	handler := captureLabelerMW(&captured)(privacyTierLikeMW(routeTagMiddleware(mux)))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/no-such-path", nil)
	handler.ServeHTTP(rec, req)

	if captured == nil {
		t.Fatal("labeler not captured")
	}
	for _, a := range captured.Get() {
		if a.Key == attribute.Key("http.route") {
			t.Errorf("unmatched route must not produce http.route; got %q",
				a.Value.AsString())
		}
	}
}
