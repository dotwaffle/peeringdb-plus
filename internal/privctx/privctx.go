// Package privctx propagates the visibility tier of the caller through a
// Go context. The ent privacy policy (see ent/schema/poc.go) reads the
// tier via TierFrom and admits Users-visibility rows when the tier is
// TierUsers.
//
// Tier is set by the HTTP middleware (internal/middleware.PrivacyTier)
// at the edge of every HTTP request, and — starting v1.15 — by the OAuth
// callback. The sync worker does NOT use this package; it bypasses the
// policy via privacy.DecisionContext at worker entry (D-07, D-08).
package privctx

import "context"

// Tier identifies the visibility scope of the caller. TierPublic is the
// zero value so any un-stamped context defaults to the most restrictive
// (safest) tier — fail-safe-closed per CONTEXT.md D-04.
type Tier int

const (
	// TierPublic is the zero-value tier. Anonymous callers see rows whose
	// upstream visibility is "Public" only.
	TierPublic Tier = iota

	// TierUsers is the signed-in / env-elevated tier. The ent privacy
	// policy admits every row (Public + Users) when the request context
	// carries this tier.
	TierUsers
)

// tierCtxKey is the unexported context key under which the tier is stored.
// The type identity (package path + type name) guarantees no other package
// can collide with this key. Same shape as ent's own decisionCtxKey
// (vendored at entgo.io/ent/privacy/privacy.go:208).
type tierCtxKey struct{}

// WithTier returns a derived context carrying tier. Safe for concurrent
// use; the returned context is a standard context.WithValue wrapper and
// inherits every cancellation/deadline from parent.
func WithTier(ctx context.Context, tier Tier) context.Context {
	return context.WithValue(ctx, tierCtxKey{}, tier)
}

// TierFrom returns the tier stored in ctx, or TierPublic if none was set
// or the value under the key is not a Tier. Never panics; the fallback is
// the most restrictive tier, so mis-wired callers default to locked-down
// behaviour rather than leaking Users-visibility rows.
func TierFrom(ctx context.Context) Tier {
	if t, ok := ctx.Value(tierCtxKey{}).(Tier); ok {
		return t
	}
	return TierPublic
}
