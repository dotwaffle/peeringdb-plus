// PrivacyTier stamps every inbound HTTP request context with the tier
// configured at startup via PDBPLUS_PUBLIC_TIER (parsed by
// internal/config.parsePublicTier and surfaced as Config.PublicTier).
// The ent privacy policy on visibility-bearing entities (see
// ent/schema/poc.go) reads the tier via internal/privctx and admits
// Users-visibility rows when the context carries TierUsers.
//
// Placement: After Logging and before Readiness/SecurityHeaders in the
// request flow (D-05, RESEARCH.md Pattern 4). The stamp is therefore
// visible to every downstream middleware and handler — including the
// Readiness 503 syncing page, so any future OTel attribution on the
// pre-ready path still sees the tier — without polluting the outer
// Recovery/Logging layers that handle the process-level error envelope.
//
// Per D-07 this middleware does NOT call the ent privacy-decision
// bypass — that path is reserved for the sync worker (internal/sync/
// worker.go). The typed-tier abstraction lets v1.15 OAuth replace this
// middleware with an identity-aware version that sets TierUsers only
// when a valid session is present, without touching the ent policy.
//
// Fail-safe-closed: privctx.TierFrom returns TierPublic on a missing or
// wrong-typed value, so a misconfigured or omitted middleware hides
// Users-tier rows rather than leaking them.

package middleware

import (
	"net/http"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// PrivacyTierInput configures the PrivacyTier middleware. DefaultTier is
// the startup-resolved value from PDBPLUS_PUBLIC_TIER (parsed by
// internal/config.parsePublicTier).
type PrivacyTierInput struct {
	// DefaultTier is the visibility tier stamped onto every request
	// context. It is captured by PrivacyTier at construction and never
	// re-read per request; callers mutating the input struct afterwards
	// have no effect.
	DefaultTier privctx.Tier
}

// PrivacyTier returns middleware that stamps every inbound request
// context with the configured tier via privctx.WithTier. The tier is
// resolved once at construction — there is zero per-request env read
// and the only per-request allocation is context.WithValue's envelope.
//
// The returned middleware does not modify the response (headers, status,
// body) — it is a pure context stamper. Callers composing chains can
// assume it has no effect on output.
func PrivacyTier(in PrivacyTierInput) func(http.Handler) http.Handler {
	tier := in.DefaultTier
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := privctx.WithTier(r.Context(), tier)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
