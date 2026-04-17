package sync_test

import (
	"context"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/ent/privacy"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// seedPocWithVisible creates a Poc via the bypass context so the privacy
// policy never sees the write, then returns the row id. The Poc schema
// requires a non-empty role, so we set one deterministically. Callers
// that want NULL visible should call seedPocAndClearVisible instead.
func seedPocWithVisible(tb testing.TB, client *ent.Client, id int, visible string) int {
	tb.Helper()
	bypass := privacy.DecisionContext(context.Background(), privacy.Allow)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := client.Poc.Create().
		SetID(id).
		SetRole("NOC").
		SetVisible(visible).
		SetCreated(ts).
		SetUpdated(ts).
		Save(bypass)
	if err != nil {
		tb.Fatalf("seed poc id=%d visible=%q: %v", id, visible, err)
	}
	return id
}

// seedPocAndClearVisible creates a Poc via bypass and then clears the
// visible column to NULL via a second Update call — bypassing the
// schema Default("Public") which fires on insert but not on explicit
// ClearVisible. Used by the NULL-defence test per RESEARCH Pitfall 2.
func seedPocAndClearVisible(tb testing.TB, client *ent.Client, id int) int {
	tb.Helper()
	bypass := privacy.DecisionContext(context.Background(), privacy.Allow)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := client.Poc.Create().
		SetID(id).
		SetRole("NOC").
		SetVisible("Public"). // initial — immediately cleared below
		SetCreated(ts).
		SetUpdated(ts).
		Save(bypass)
	if err != nil {
		tb.Fatalf("seed poc id=%d (pre-clear): %v", id, err)
	}
	if err := client.Poc.UpdateOneID(id).ClearVisible().Exec(bypass); err != nil {
		tb.Fatalf("clear visible on poc id=%d: %v", id, err)
	}
	return id
}

// TestPocPolicy_FiltersUsersTier (VIS-04 case 59-i): a Users-visibility
// row must be invisible to anonymous (TierPublic) callers.
func TestPocPolicy_FiltersUsersTier(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	id := seedPocWithVisible(t, client, 1001, "Users")

	_, err := client.Poc.Get(context.Background(), id)
	if !ent.IsNotFound(err) {
		t.Fatalf("TierPublic Get(Users-row) = %v, want NotFound", err)
	}
}

// TestPocPolicy_AdmitsPublicToTierPublic (VIS-04 case 59-j): a
// Public-visibility row must be visible to anonymous callers.
func TestPocPolicy_AdmitsPublicToTierPublic(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	id := seedPocWithVisible(t, client, 1002, "Public")

	got, err := client.Poc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("TierPublic Get(Public-row) err = %v, want nil", err)
	}
	if got.ID != id {
		t.Errorf("got id=%d, want %d", got.ID, id)
	}
}

// TestPocPolicy_AdmitsUsersToTierUsers (VIS-04 case 59-k): a
// Users-visibility row must be visible to TierUsers callers (the
// env-override and future-OAuth path).
func TestPocPolicy_AdmitsUsersToTierUsers(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	id := seedPocWithVisible(t, client, 1003, "Users")

	ctx := privctx.WithTier(context.Background(), privctx.TierUsers)
	got, err := client.Poc.Get(ctx, id)
	if err != nil {
		t.Fatalf("TierUsers Get(Users-row) err = %v, want nil", err)
	}
	if got.ID != id {
		t.Errorf("got id=%d, want %d", got.ID, id)
	}
}

// TestPocPolicy_AdmitsNullVisibleToTierPublic (VIS-04 case 59-l, Pitfall 2):
// a row with NULL visible must be admitted to TierPublic. SQL three-valued
// logic makes `visible = 'Public'` FALSE for NULL, so the policy adds
// poc.Or(VisibleEQ("Public"), VisibleIsNil()) as defence-in-depth against
// legacy rows migrated before the column's Default was applied.
func TestPocPolicy_AdmitsNullVisibleToTierPublic(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	id := seedPocAndClearVisible(t, client, 1004)

	got, err := client.Poc.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("TierPublic Get(NULL-visible row) err = %v, want nil", err)
	}
	if got.ID != id {
		t.Errorf("got id=%d, want %d", got.ID, id)
	}
	if got.Visible != "" {
		t.Errorf("Visible = %q, want empty string (NULL materialised as zero value)", got.Visible)
	}
}

// TestPocPolicy_EdgeTraversalFilters (VIS-04 case 59-m): traversing the
// Network→Pocs edge (via poc.HasNetworkWith) must apply the same policy
// — this exercises the typed PocQueryRuleFunc adapter on edge-derived
// query builders, not just root-level Get()/Query() calls.
func TestPocPolicy_EdgeTraversalFilters(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	bypass := privacy.DecisionContext(context.Background(), privacy.Allow)
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Seed a parent Network and two Pocs (one Public, one Users), all via bypass.
	org, err := client.Organization.Create().
		SetID(2000).SetName("EdgeOrg").
		SetCreated(ts).SetUpdated(ts).
		Save(bypass)
	if err != nil {
		t.Fatalf("seed org: %v", err)
	}
	net, err := client.Network.Create().
		SetID(2001).SetOrgID(org.ID).SetName("EdgeNet").SetAsn(64500).
		SetCreated(ts).SetUpdated(ts).
		Save(bypass)
	if err != nil {
		t.Fatalf("seed network: %v", err)
	}
	for _, row := range []struct {
		id      int
		visible string
	}{
		{2010, "Public"},
		{2011, "Users"},
	} {
		_, err := client.Poc.Create().
			SetID(row.id).
			SetNetID(net.ID).
			SetRole("NOC").
			SetVisible(row.visible).
			SetCreated(ts).SetUpdated(ts).
			Save(bypass)
		if err != nil {
			t.Fatalf("seed poc id=%d visible=%q: %v", row.id, row.visible, err)
		}
	}

	// Edge-traversal query with TierPublic (anonymous) context. The
	// where-predicate materialises the query as an edge-derived *ent.PocQuery,
	// which is the type the PocQueryRuleFunc adapter asserts on.
	got, err := client.Poc.Query().
		Where(poc.HasNetwork()).
		All(context.Background())
	if err != nil {
		t.Fatalf("edge-traversal All() err = %v, want nil", err)
	}
	if len(got) != 1 {
		var ids []int
		for _, p := range got {
			ids = append(ids, p.ID)
		}
		t.Fatalf("TierPublic edge-traversal returned %d rows %v, want exactly 1 (Public only)", len(got), ids)
	}
	if got[0].ID != 2010 {
		t.Errorf("edge-traversal returned id=%d, want 2010 (the Public row)", got[0].ID)
	}
}
