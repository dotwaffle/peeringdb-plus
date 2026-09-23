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
// pass through ent's PocQuery, so the poc Privacy policy would never
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

	// No tier may read a Private poc (upstream shows it only to members
	// of the owning organization), so the traversal must not reach its
	// PII or reveal which network has one, not even for TierUsers.
	// upstream: 2.83.0 permissions.py:336-339, signals.py:343-347
	client.Poc.Create().
		SetID(9500).SetNetID(r.Network2.ID).SetRole("NOC").SetVisible("Private").
		SetName("Private NOC").SetEmail("private-noc@example.invalid").
		SetCreated(r.Network2.Created).SetUpdated(r.Network2.Updated).SetStatus("ok").
		SaveX(t.Context())
	for name, tier := range map[string]privctx.Tier{"anon": privctx.TierPublic, "users": privctx.TierUsers} {
		t.Run("private_poc_hidden_from_"+name, func(t *testing.T) {
			if got := matchedNetIDs(t, tier, "poc__email", "private-noc@example.invalid"); len(got) != 0 {
				t.Errorf("traversal leaked networks %v via a Private poc email", got)
			}
			if got := matchedNetIDs(t, tier, "poc__visible", "Private"); len(got) != 0 {
				t.Errorf("traversal enumerated networks %v with a Private poc", got)
			}
		})
	}

	// A 2-hop key with pocs as the middle hop (net -> poc -> net) must
	// gate the poc rows as well. Otherwise poc__net__id__gt=0 lists every
	// network that has a contact, including the contacts the tier cannot
	// read. privateOnly has only a Private poc. r.Network2 has only
	// hidden pocs for an anonymous caller (Users and Private).
	privateOnly := client.Network.Create().
		SetID(9510).SetName("Private Contact Net").SetAsn(64512).
		SetOrgID(r.Org.ID).
		SetCreated(r.Network2.Created).SetUpdated(r.Network2.Updated).SetStatus("ok").
		SaveX(t.Context())
	client.Poc.Create().
		SetID(9511).SetNetID(privateOnly.ID).SetRole("NOC").SetVisible("Private").
		SetName("Private Only NOC").SetEmail("private-only-noc@example.invalid").
		SetCreated(r.Network2.Created).SetUpdated(r.Network2.Updated).SetStatus("ok").
		SaveX(t.Context())
	for name, tc := range map[string]struct {
		tier   privctx.Tier
		hidden []int
	}{
		"anon":  {privctx.TierPublic, []int{r.Network2.ID, privateOnly.ID}},
		"users": {privctx.TierUsers, []int{privateOnly.ID}},
	} {
		t.Run("two_hop_middle_poc_gated_for_"+name, func(t *testing.T) {
			got := matchedNetIDs(t, tc.tier, "poc__net__id__gt", "0")
			if !slices.Contains(got, r.Network.ID) {
				t.Errorf("poc__net__id__gt=0 = %v, want network %d (Public poc)", got, r.Network.ID)
			}
			for _, id := range tc.hidden {
				if slices.Contains(got, id) {
					t.Errorf("poc__net__id__gt=0 = %v, matched network %d through a hidden poc", got, id)
				}
			}
			if got := matchedNetIDs(t, tc.tier, "poc__net__asn", "64512"); len(got) != 0 {
				t.Errorf("poc__net__asn=64512 = %v, want none (only a Private poc)", got)
			}
		})
	}
}
