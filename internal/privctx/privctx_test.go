package privctx_test

import (
	"context"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// TestTier_Roundtrip (VALIDATION.md 59-a) verifies that values stamped via
// WithTier are recovered verbatim by TierFrom for every defined tier.
func TestTier_Roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tier privctx.Tier
	}{
		{name: "public", tier: privctx.TierPublic},
		{name: "users", tier: privctx.TierUsers},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := privctx.WithTier(context.Background(), tc.tier)
			got := privctx.TierFrom(ctx)
			if got != tc.tier {
				t.Fatalf("TierFrom(WithTier(ctx, %v)) = %v, want %v", tc.tier, got, tc.tier)
			}
		})
	}
}

// TestTierFrom_ZeroValueIsPublic (VALIDATION.md 59-b) asserts that an
// un-stamped context defaults to TierPublic — the most restrictive tier,
// i.e. fail-safe-closed per D-04.
func TestTierFrom_ZeroValueIsPublic(t *testing.T) {
	t.Parallel()

	if got := privctx.TierFrom(context.Background()); got != privctx.TierPublic {
		t.Fatalf("TierFrom(Background) = %v, want TierPublic (%v)", got, privctx.TierPublic)
	}
}

// TestTierFrom_WrongTypeIsPublic verifies defence-in-depth: a caller
// cannot stamp the tier from outside the package (private key), but if
// someone stamps a non-Tier value under an unrelated key, TierFrom still
// returns TierPublic (no panic, no cross-contamination).
func TestTierFrom_WrongTypeIsPublic(t *testing.T) {
	t.Parallel()

	type externalKey struct{}
	ctx := context.WithValue(context.Background(), externalKey{}, "users")

	if got := privctx.TierFrom(ctx); got != privctx.TierPublic {
		t.Fatalf("TierFrom(ctx with foreign key) = %v, want TierPublic", got)
	}
}

// TestWithTier_ChildCtxInheritsValue verifies that derived contexts
// (WithTimeout, WithCancel, etc.) inherit the stamped tier — required
// because every HTTP handler wraps the request ctx with a timeout
// downstream of the privacy middleware.
func TestWithTier_ChildCtxInheritsValue(t *testing.T) {
	t.Parallel()

	parent := privctx.WithTier(context.Background(), privctx.TierUsers)
	child, cancel := context.WithTimeout(parent, time.Second)
	t.Cleanup(cancel)

	if got := privctx.TierFrom(child); got != privctx.TierUsers {
		t.Fatalf("TierFrom(derived child) = %v, want TierUsers", got)
	}
}
