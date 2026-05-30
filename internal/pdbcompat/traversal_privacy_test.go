package pdbcompat

import (
	"context"
	"net/url"
	"slices"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// TestTraversal_PocVisibilityGate verifies that a cross-entity traversal
// subquery reproduces the poc row-level privacy policy (poc.visible). The
// traversal builds a raw SQL subquery against the pocs table that does NOT
// pass through ent's PocQuery, so the Phase 59 Privacy policy would never
// fire — letting an anonymous caller use net?pocs__<field>=... as a boolean
// oracle to read or enumerate Users-tier (hidden) contact PII that the policy
// hides on the direct /api/poc surface. applyVisibilityGate closes that leak.
//
// seed.Full provides the fixtures: r.UsersPoc (visible="Users",
// email "users-noc@example.invalid") and the Public r.Poc (name "NOC
// Contact"), both attached to r.Network; r.UsersPoc2 (visible="Users") on
// r.Network2.
func TestTraversal_PocVisibilityGate(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	r := seed.Full(t, client)
	netTC := Registry[peeringdb.TypeNet]

	// matchedNetIDs resolves a net traversal filter under the given tier and
	// returns the ids of the networks it matches.
	matchedNetIDs := func(t *testing.T, tier privctx.Tier, key, value string) []int {
		t.Helper()
		ctx := privctx.WithTier(context.Background(), tier)
		preds, empty, err := ParseFiltersCtx(ctx, url.Values{key: {value}}, netTC)
		if err != nil {
			t.Fatalf("ParseFiltersCtx(%s=%q): %v", key, value, err)
		}
		if empty {
			return nil
		}
		ids, err := client.Network.Query().
			Where(castPredicates[predicate.Network](preds)...).
			IDs(ctx)
		if err != nil {
			t.Fatalf("network query: %v", err)
		}
		return ids
	}

	// Anonymous callers must not reach a hidden poc's PII through the
	// traversal subquery. Before the gate, the hidden poc's email matches
	// and leaks r.Network.
	t.Run("hidden_poc_email_not_leaked_to_anon", func(t *testing.T) {
		got := matchedNetIDs(t, privctx.TierPublic, "poc__email", "users-noc@example.invalid")
		if slices.Contains(got, r.Network.ID) {
			t.Fatalf("anon traversal leaked network %d via hidden poc email; got %v", r.Network.ID, got)
		}
	})

	// Users-tier callers still reach the hidden poc — proving the filter
	// resolves and that the gate (not a silent-ignore) is what excludes it
	// for anonymous callers above.
	t.Run("hidden_poc_email_visible_to_users", func(t *testing.T) {
		got := matchedNetIDs(t, privctx.TierUsers, "poc__email", "users-noc@example.invalid")
		if !slices.Contains(got, r.Network.ID) {
			t.Fatalf("TierUsers traversal should match network %d via hidden poc email; got %v", r.Network.ID, got)
		}
	})

	// The gate must not over-filter: a Public poc remains traversal-matchable
	// by anonymous callers.
	t.Run("public_poc_still_filterable_by_anon", func(t *testing.T) {
		got := matchedNetIDs(t, privctx.TierPublic, "poc__name", "NOC Contact")
		if !slices.Contains(got, r.Network.ID) {
			t.Fatalf("anon traversal should still match network %d via its Public poc; got %v", r.Network.ID, got)
		}
	})

	// Anonymous callers must not be able to enumerate which networks have
	// hidden contacts via ?pocs__visible=Users.
	t.Run("hidden_poc_enumeration_blocked_for_anon", func(t *testing.T) {
		got := matchedNetIDs(t, privctx.TierPublic, "poc__visible", "Users")
		if len(got) != 0 {
			t.Fatalf("anon traversal enumerated networks with hidden pocs: got %v, want none", got)
		}
	})

	t.Run("hidden_poc_enumeration_allowed_for_users", func(t *testing.T) {
		got := matchedNetIDs(t, privctx.TierUsers, "poc__visible", "Users")
		for _, want := range []int{r.Network.ID, r.Network2.ID} {
			if !slices.Contains(got, want) {
				t.Fatalf("TierUsers enumeration should include network %d; got %v", want, got)
			}
		}
	})
}
