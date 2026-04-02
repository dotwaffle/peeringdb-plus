package middleware

import (
	"net/http"
	"strings"
)

// CSPInput holds configuration for the Content-Security-Policy middleware.
type CSPInput struct {
	// UIPolicy is the CSP directive string applied to /ui/ routes.
	UIPolicy string

	// GraphQLPolicy is the CSP directive string applied to /graphql routes.
	// Typically more permissive than UIPolicy (e.g. allows unsafe-eval for GraphiQL).
	GraphQLPolicy string
}

// CSP returns middleware that sets a Content-Security-Policy-Report-Only header
// based on the request path. Web UI routes get a tighter policy; GraphQL gets a
// more permissive policy for the GraphiQL playground. Non-browser routes (/api/,
// /rest/, ConnectRPC) receive no CSP header.
//
// Report-Only mode reports violations without blocking resources, allowing safe
// rollout and monitoring before switching to enforcing mode.
func CSP(in CSPInput) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/ui" || strings.HasPrefix(r.URL.Path, "/ui/"):
				w.Header().Set("Content-Security-Policy-Report-Only", in.UIPolicy)
			case strings.HasPrefix(r.URL.Path, "/graphql"):
				w.Header().Set("Content-Security-Policy-Report-Only", in.GraphQLPolicy)
			}

			next.ServeHTTP(w, r)
		})
	}
}
