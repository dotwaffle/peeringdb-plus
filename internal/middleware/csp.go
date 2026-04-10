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

	// EnforcingMode selects the CSP header name. When true, the middleware
	// sets "Content-Security-Policy" (enforcing). When false, it sets
	// "Content-Security-Policy-Report-Only" (report-only). The policy
	// strings themselves are unchanged across modes.
	EnforcingMode bool
}

// CSP returns middleware that sets a Content-Security-Policy header based
// on the request path. Web UI routes get the tighter UIPolicy; GraphQL gets
// the more permissive GraphQLPolicy (for the GraphiQL playground). Non-browser
// routes (/api/, /rest/, ConnectRPC) receive no CSP header.
//
// The header name is chosen by in.EnforcingMode: true → "Content-Security-Policy"
// (enforcing), false → "Content-Security-Policy-Report-Only" (report-only monitoring
// without blocking). The policy strings are identical in both modes.
func CSP(in CSPInput) func(http.Handler) http.Handler {
	headerName := "Content-Security-Policy-Report-Only"
	if in.EnforcingMode {
		headerName = "Content-Security-Policy"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/ui" || strings.HasPrefix(r.URL.Path, "/ui/"):
				w.Header().Set(headerName, in.UIPolicy)
			case strings.HasPrefix(r.URL.Path, "/graphql"):
				w.Header().Set(headerName, in.GraphQLPolicy)
			}

			next.ServeHTTP(w, r)
		})
	}
}
