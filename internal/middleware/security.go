package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SecurityHeadersInput configures the SecurityHeaders middleware.
type SecurityHeadersInput struct {
	// HSTSMaxAge is the Strict-Transport-Security max-age duration.
	// Zero disables the HSTS header entirely. The v1.13 default is
	// 180 * 24 * time.Hour (conservative first-enforcement); flip to
	// 365 days in v1.14 after production bake.
	HSTSMaxAge time.Duration

	// HSTSIncludeSubDomains appends the includeSubDomains directive.
	// See the SecurityHeaders doc comment for the Fly.io shared-suffix
	// caveat on the HSTS rendering.
	HSTSIncludeSubDomains bool

	// FrameOptions is the X-Frame-Options header value, applied ONLY to
	// browser paths (/, /ui/*, /graphql). Empty disables the header.
	// Recommended: "DENY".
	FrameOptions string

	// ContentTypeOptions sets X-Content-Type-Options: nosniff on ALL
	// responses when true. Important on text/plain error responses.
	ContentTypeOptions bool
}

// SecurityHeaders returns middleware that sets HSTS, X-Frame-Options, and
// X-Content-Type-Options response headers. HSTS and XCTO apply to every
// response; XFO is scoped to browser paths because JSON APIs and gRPC do
// not render in frames.
//
// The HSTS header emits only max-age and includeSubDomains — the preload
// directive is intentionally omitted because Fly.io .fly.dev is a shared-
// suffix domain. See .planning/research/FEATURES.md §SEC-7. Negative locked
// by TestMiddleware_SecurityHeaders_NoPreload.
func SecurityHeaders(in SecurityHeadersInput) func(http.Handler) http.Handler {
	// Build the HSTS value once at factory time — the inputs are locked
	// at startup and never change per request.
	hstsValue := ""
	if in.HSTSMaxAge > 0 {
		hstsValue = fmt.Sprintf("max-age=%d", int(in.HSTSMaxAge.Seconds()))
		if in.HSTSIncludeSubDomains {
			hstsValue += "; includeSubDomains"
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			if hstsValue != "" {
				h.Set("Strict-Transport-Security", hstsValue)
			}
			if in.ContentTypeOptions {
				h.Set("X-Content-Type-Options", "nosniff")
			}
			if in.FrameOptions != "" && isBrowserPath(r.URL.Path) {
				h.Set("X-Frame-Options", in.FrameOptions)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isBrowserPath reports whether p is a path that renders as HTML in a
// browser (root discovery, /ui/, /graphql). Used by SecurityHeaders to
// scope X-Frame-Options: JSON APIs and gRPC do not render in frames, so
// XFO on those paths is noise.
func isBrowserPath(p string) bool {
	if p == "/" || p == "/ui" || p == "/graphql" {
		return true
	}
	return strings.HasPrefix(p, "/ui/") || strings.HasPrefix(p, "/graphql/")
}
