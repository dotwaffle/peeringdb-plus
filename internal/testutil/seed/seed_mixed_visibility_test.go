// Package seed_test — phase 60-01 regression tests for seed.Full's
// mixed-visibility contract.
//
// These tests lock in the mixed-visibility fixture that downstream
// phase 60 plans (02-05) depend on. Any future refactor of seed.go that
// drops the Users-tier rows, changes their IDs, or alters the
// pre-existing Public POC breaks the build here rather than cause
// silent drift in every per-surface test.
//
// Lives in the external `seed_test` package (rather than the existing
// internal-package seed_test.go) so the assertions exercise the same
// exported surface that downstream callers see, and so the privctx +
// ent/privacy imports don't leak into package seed's API.
//
// TestFull_PrivacyFilterShapes double-asserts the ent Privacy policy
// installed in phase 59. Phase 59's own tests live in
// internal/sync/policy_test.go and target freshly-seeded rows; this
// version exercises the policy against the canonical seed.Full fixture,
// so it will catch a regression where the seed mutates in a way the
// policy no longer filters (e.g., row with visible="users" lowercase).
package seed_test

import (
	"context"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/privacy"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// TestFull_HasUsersPocs asserts seed.Full creates exactly 2 visible="Users"
// POCs (one per seeded network) with stable IDs 9000 and 9001, and that
// Result exposes them as typed handles. Phase 60 Plan 02 depends on this
// contract.
func TestFull_HasUsersPocs(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	r := seed.Full(t, client)

	if r.UsersPoc == nil || r.UsersPoc.ID != 9000 || r.UsersPoc.Visible != "Users" {
		t.Fatalf("UsersPoc: got %+v, want {ID:9000 Visible:Users}", r.UsersPoc)
	}
	if r.UsersPoc2 == nil || r.UsersPoc2.ID != 9001 || r.UsersPoc2.Visible != "Users" {
		t.Fatalf("UsersPoc2: got %+v, want {ID:9001 Visible:Users}", r.UsersPoc2)
	}
	if len(r.AllPocs) != 3 {
		t.Fatalf("AllPocs len: got %d, want 3", len(r.AllPocs))
	}
	// Deterministic order: r.Poc (Public, ID 500) first so list-count
	// consumers can rely on AllPocs[0] being the Public row.
	if r.AllPocs[0] != r.Poc || r.AllPocs[1] != r.UsersPoc || r.AllPocs[2] != r.UsersPoc2 {
		t.Fatalf("AllPocs order: got [%d %d %d], want [500 9000 9001]",
			r.AllPocs[0].ID, r.AllPocs[1].ID, r.AllPocs[2].ID)
	}
}

// TestFull_PublicCountsUnchanged locks in D-02: the Public POC in Result.Poc
// keeps its existing ID (500), name, and role. Any change here is a
// breaking regression for every existing seed.Full consumer (graph/
// resolver_test.go etc).
func TestFull_PublicCountsUnchanged(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	r := seed.Full(t, client)

	if r.Poc == nil || r.Poc.ID != 500 || r.Poc.Visible != "Public" ||
		r.Poc.Name != "NOC Contact" || r.Poc.Role != "NOC" {
		t.Fatalf("Result.Poc regressed: got %+v, want {ID:500 Name:NOC Contact Role:NOC Visible:Public}", r.Poc)
	}
}

// TestFull_PrivacyFilterShapes asserts Plan 02's core contract at the
// ent-query layer: an anonymous-tier context sees 1 POC; a Users-tier
// context (and the sync bypass) sees all 3. This is effectively a
// privacy-policy smoke test scoped to the seed fixture — if this fails,
// Plans 02-05's per-surface assertions will fail en masse.
func TestFull_PrivacyFilterShapes(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	_ = seed.Full(t, client)

	cases := []struct {
		name     string
		ctx      context.Context
		wantRows int
	}{
		{"anonymous TierPublic", privctx.WithTier(context.Background(), privctx.TierPublic), 1},
		{"stamped TierUsers", privctx.WithTier(context.Background(), privctx.TierUsers), 3},
		{"sync bypass", privacy.DecisionContext(context.Background(), privacy.Allow), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := client.Poc.Query().Where(poc.IDIn(500, 9000, 9001)).Count(tc.ctx)
			if err != nil {
				t.Fatalf("Poc.Query.Count: %v", err)
			}
			if got != tc.wantRows {
				t.Fatalf("row count: got %d, want %d", got, tc.wantRows)
			}
		})
	}
}
