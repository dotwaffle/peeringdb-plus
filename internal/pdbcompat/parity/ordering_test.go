package parity

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// TestParity_Ordering locks the v1.16 default ordering semantics
// (`-updated, -created, -id`) under upstream parity per
// django-handleref/models.py:95-101 (Phase 67 D-02).
//
// Subtests use inline-built clean rows because the ported
// OrderingFixtures slice carries Python-source artefacts in `name`
// and `status` (e.g. embedded commas / kwargs) that the harness
// rejects as unseedable. The subtests still cite their upstream
// source line so a future fixture-port refresh can re-attach to the
// canonical assertion path. See SUMMARY.md for the fixture-coverage
// gap rationale.
//
// upstream: pdb_api_test.py (multiple sites; default ordering is
// implicit in every list assertion that doesn't pass an explicit
// `?sort=` override).
func TestParity_Ordering(t *testing.T) {
	t.Parallel()

	// Base timestamp; spread updated by 1h so SQLite time precision is
	// never the deciding factor on the (-updated, -created) rank.
	t0 := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)

	t.Run("default_list_order_updated_desc", func(t *testing.T) {
		t.Parallel()
		// upstream: django-handleref/models.py:95-101 (Meta.ordering)
		// upstream: pdb_api_test.py:1604 (one of many default-list calls)
		c := testutil.SetupClient(t)
		ctx := t.Context()

		org, err := c.Organization.Create().
			SetID(1).SetName("OrderProbe").SetNameFold(unifold.Fold("OrderProbe")).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed org: %v", err)
		}
		// 5 (oldest), 10, 20 (newest). Expected DESC: [20, 10, 5].
		for _, r := range []struct {
			id      int
			updated time.Time
		}{
			{5, t0},
			{10, t0.Add(1 * time.Hour)},
			{20, t0.Add(2 * time.Hour)},
		} {
			if _, err := c.Network.Create().
				SetID(r.id).SetName("Net").SetNameFold(unifold.Fold("Net")).
				SetAsn(64500 + r.id).SetStatus("ok").
				SetOrgID(org.ID).
				SetCreated(t0).SetUpdated(r.updated).
				Save(ctx); err != nil {
				t.Fatalf("seed net %d: %v", r.id, err)
			}
		}

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", status, string(body))
		}
		got := extractIDs(t, body)
		want := []int{20, 10, 5}
		if !slices.Equal(got, want) {
			t.Errorf("default order: got %v, want %v (ORDER-01)", got, want)
		}
	})

	t.Run("tiebreak_by_created_desc", func(t *testing.T) {
		t.Parallel()
		// upstream: django-handleref/models.py:95-101 (`-updated, -created`)
		// upstream: pdb_api_test.py:1242 (sibling fixture pair with
		// identical updated and distinct created)
		c := testutil.SetupClient(t)
		ctx := t.Context()
		org, err := c.Organization.Create().
			SetID(1).SetName("TieOrg").SetNameFold(unifold.Fold("TieOrg")).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed org: %v", err)
		}
		sameUpdated := t0.Add(2 * time.Hour)
		// id=10 created=t0; id=11 created=t0+1h. Both updated=sameUpdated.
		// (-updated, -created) → 11 first, 10 second.
		if _, err := c.Network.Create().
			SetID(10).SetName("Older").SetNameFold(unifold.Fold("Older")).
			SetAsn(64510).SetStatus("ok").SetOrgID(org.ID).
			SetCreated(t0).SetUpdated(sameUpdated).
			Save(ctx); err != nil {
			t.Fatalf("seed net 10: %v", err)
		}
		if _, err := c.Network.Create().
			SetID(11).SetName("Newer").SetNameFold(unifold.Fold("Newer")).
			SetAsn(64511).SetStatus("ok").SetOrgID(org.ID).
			SetCreated(t0.Add(1*time.Hour)).SetUpdated(sameUpdated).
			Save(ctx); err != nil {
			t.Fatalf("seed net 11: %v", err)
		}

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		got := extractIDs(t, body)
		want := []int{11, 10}
		if !slices.Equal(got, want) {
			t.Errorf("tiebreak by created DESC: got %v, want %v (ORDER-02)", got, want)
		}
	})

	t.Run("tiebreak_by_id_desc", func(t *testing.T) {
		t.Parallel()
		// upstream: django-handleref/models.py:95-101 — `id` DESC is the
		// implicit final tiebreak in Meta.ordering (Phase 67 D-05).
		// synthesised: phase67-plan-03 — upstream test corpus has no
		// fixture pair with identical updated AND created; the tertiary
		// id-DESC behaviour is documented but not asserted in
		// pdb_api_test.py.
		c := testutil.SetupClient(t)
		ctx := t.Context()
		org, err := c.Organization.Create().
			SetID(1).SetName("IDTieOrg").SetNameFold(unifold.Fold("IDTieOrg")).
			SetStatus("ok").SetCreated(t0).SetUpdated(t0).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed org: %v", err)
		}
		sameTs := t0.Add(3 * time.Hour)
		if _, err := c.Network.Create().
			SetID(50).SetName("LowID").SetNameFold(unifold.Fold("LowID")).
			SetAsn(64550).SetStatus("ok").SetOrgID(org.ID).
			SetCreated(sameTs).SetUpdated(sameTs).
			Save(ctx); err != nil {
			t.Fatalf("seed net 50: %v", err)
		}
		if _, err := c.Network.Create().
			SetID(99).SetName("HighID").SetNameFold(unifold.Fold("HighID")).
			SetAsn(64599).SetStatus("ok").SetOrgID(org.ID).
			SetCreated(sameTs).SetUpdated(sameTs).
			Save(ctx); err != nil {
			t.Fatalf("seed net 99: %v", err)
		}

		srv := newTestServer(t, c)
		status, body := httpGet(t, srv, "/api/net")
		if status != http.StatusOK {
			t.Fatalf("status = %d; body=%s", status, string(body))
		}
		got := extractIDs(t, body)
		want := []int{99, 50}
		if !slices.Equal(got, want) {
			t.Errorf("tiebreak by id DESC: got %v, want %v (ORDER-03)", got, want)
		}
	})
}
