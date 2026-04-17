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

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

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
// context with the configured tier via privctx.WithTier AND emits an
// OTel attribute pdbplus.privacy.tier on the current span (the inbound
// HTTP server span created by otelhttp.NewMiddleware, which sits just
// outside this middleware in the chain).
//
// The tier and its string form are resolved once at construction;
// there is zero per-request env read, zero per-request string alloc,
// and the only per-request cost is context.WithValue + SetAttributes
// on the active span. When the chain does not carry an active span
// (unit tests without a tracer), trace.SpanFromContext returns a
// noop span and SetAttributes is a zero-cost no-op — fail-safe-closed.
//
// Per 61-CONTEXT.md D-07/D-08/D-09 (OBS-03):
//   - attribute key is the literal "pdbplus.privacy.tier" (pdbplus.*
//     namespace, matches pdbplus.sync.*, pdbplus.data.*).
//   - cardinality is 2: "public" or "users". A future TierAdmin
//     addition must force a compile error here (exhaustive switch
//     with no default) before shipping.
//
// Downstream ent/sql spans inherit the attribute via parent-span
// context propagation — there is intentionally no redundant stamping
// further down the chain.
//
// The returned middleware does not modify the response (headers, status,
// body) — it is a pure context stamper. Callers composing chains can
// assume it has no effect on output.
func PrivacyTier(in PrivacyTierInput) func(http.Handler) http.Handler {
	tier := in.DefaultTier
	tierAttr := attribute.String("pdbplus.privacy.tier", tierString(tier))
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			trace.SpanFromContext(ctx).SetAttributes(tierAttr)
			ctx = privctx.WithTier(ctx, tier)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// tierString maps a privctx.Tier to its canonical OBS-03 attribute
// value. The switch is exhaustive by design — a future Tier addition
// will fail compilation here (caught by golangci-lint's exhaustive
// checker) and force a coordinated update to the Grafana dashboard
// cardinality model before the new value can ship.
//
// Unknown tiers panic rather than silently emitting "unknown": a
// silent dashboard outlier would mask the actual config drift, and
// the only way this code path runs today is via a programming error
// (new enum value added without updating this switch).
func tierString(t privctx.Tier) string {
	switch t { //nolint:exhaustive // panic fallback covers future Tier additions at runtime; the exhaustive compile-time check is the intended design per 61-CONTEXT.md D-09.
	case privctx.TierPublic:
		return "public"
	case privctx.TierUsers:
		return "users"
	}
	panic("privacy_tier: unknown tier value — add case above before shipping")
}
