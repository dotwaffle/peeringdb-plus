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

	// ReportPath, when set, is the same-origin path that receives
	// violation reports. Both policies then name it in report-uri and,
	// through the "csp" reporting endpoint, in report-to, and responses
	// that carry a policy also send Reporting-Endpoints.
	ReportPath string

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
	uiPolicy, graphQLPolicy, reportingEndpoints := in.UIPolicy, in.GraphQLPolicy, ""
	if in.ReportPath != "" {
		// report-uri is for browsers without the Reporting API; a
		// browser that supports report-to ignores report-uri.
		reporting := "; report-uri " + in.ReportPath + "; report-to csp"
		uiPolicy += reporting
		graphQLPolicy += reporting
		reportingEndpoints = `csp="` + in.ReportPath + `"`
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			policy := ""
			switch {
			case r.URL.Path == "/ui" || strings.HasPrefix(r.URL.Path, "/ui/"):
				policy = uiPolicy
			case strings.HasPrefix(r.URL.Path, "/graphql"):
				policy = graphQLPolicy
			}
			if policy != "" {
				w.Header().Set(headerName, policy)
				if reportingEndpoints != "" {
					w.Header().Set("Reporting-Endpoints", reportingEndpoints)
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
