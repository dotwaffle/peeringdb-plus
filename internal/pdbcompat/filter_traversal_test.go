package pdbcompat

import (
	"context"
	"net/url"
	"testing"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// TestParseFilters_Traversal_Table — syntactic / resolution table. Covers
// Path A hits, Path B hits, over-cap rejection, unknown-edge rejection,
// and unknown-field-on-known-edge rejection. Phase 70 TRAVERSAL-02..04.
func TestParseFilters_Traversal_Table(t *testing.T) {
	t.Parallel()

	netTC := Registry[peeringdb.TypeNet]
	facTC := Registry[peeringdb.TypeFac]
	ixpfxTC := Registry[peeringdb.TypeIXPfx]

	tests := []struct {
		name        string
		tc          TypeConfig
		params      url.Values
		wantPreds   int
		wantEmpty   bool
		wantUnknown []string
	}{
		{
			name:      "Path A 1-hop org__name on net",
			tc:        netTC,
			params:    url.Values{"org__name": {"Test Organization"}},
			wantPreds: 1,
		},
		{
			name:      "Path A 1-hop org__id on net (int field)",
			tc:        netTC,
			params:    url.Values{"org__id": {"1"}},
			wantPreds: 1,
		},
		{
			name:        "Path B fallback 1-hop: net?poc__name=X (not in Allowlists but via edge)",
			tc:          netTC,
			params:      url.Values{"poc__name": {"NOC"}},
			wantPreds:   1,
			wantUnknown: nil,
		},
		{
			name:        "Path A 2-hop ixlan__ix__id on ixpfx",
			tc:          ixpfxTC,
			params:      url.Values{"ixlan__ix__id": {"20"}},
			wantPreds:   1,
			wantUnknown: nil,
		},
		{
			name:        "Path A 2-hop ixlan__ix__fac_count__gt on fac",
			tc:          facTC,
			params:      url.Values{"ixlan__ix__fac_count__gt": {"0"}},
			wantPreds:   0, // fac has no "ixlan" edge, so Path B also fails
			wantUnknown: []string{"ixlan__ix__fac_count__gt"},
		},
		{
			name:        "unknown edge bogus__name on net — silent ignore",
			tc:          netTC,
			params:      url.Values{"bogus__name": {"x"}},
			wantPreds:   0,
			wantUnknown: []string{"bogus__name"},
		},
		{
			name:        "known edge, unknown target field — silent ignore",
			tc:          netTC,
			params:      url.Values{"org__notarealcolumn": {"x"}},
			wantPreds:   0,
			wantUnknown: []string{"org__notarealcolumn"},
		},
		{
			name:        "3-hop over cap — silent ignore per D-04",
			tc:          netTC,
			params:      url.Values{"a__b__c__d": {"x"}},
			wantPreds:   0,
			wantUnknown: []string{"a__b__c__d"},
		},
		{
			name:        "4-hop over cap — silent ignore per D-04",
			tc:          netTC,
			params:      url.Values{"a__b__c__d__e": {"x"}},
			wantPreds:   0,
			wantUnknown: []string{"a__b__c__d__e"},
		},
		{
			// Phase 69 IN-02 preservation: empty __in on a traversal key
			// also short-circuits (same as local field).
			name:      "empty __in on traversal key short-circuits",
			tc:        netTC,
			params:    url.Values{"org__id__in": {""}},
			wantPreds: 0,
			wantEmpty: true,
		},
		{
			// Phase 69 fold routing preservation on local field — the
			// FoldedFields map on net has "name":true.
			name:      "local _fold routing preserved",
			tc:        netTC,
			params:    url.Values{"name__contains": {"cloud"}},
			wantPreds: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := WithUnknownFields(context.Background())
			preds, empty, err := ParseFiltersCtx(ctx, tt.params, tt.tc)
			if err != nil {
				t.Fatalf("ParseFiltersCtx() unexpected error: %v", err)
			}
			if empty != tt.wantEmpty {
				t.Errorf("emptyResult = %v, want %v", empty, tt.wantEmpty)
			}
			if len(preds) != tt.wantPreds {
				t.Errorf("preds count = %d, want %d", len(preds), tt.wantPreds)
			}
			got := UnknownFieldsFromCtx(ctx)
			if len(got) != len(tt.wantUnknown) {
				t.Errorf("unknown fields = %v, want %v", got, tt.wantUnknown)
			} else {
				for i := range got {
					if got[i] != tt.wantUnknown[i] {
						t.Errorf("unknown[%d] = %q, want %q", i, got[i], tt.wantUnknown[i])
					}
				}
			}
		})
	}
}

// TestBuildTraversal_SingleHop_Integration executes a 1-hop predicate
// against a real seeded database and asserts only the expected rows match.
// Phase 70 TRAVERSAL-02 integration gate — the Path A/B resolution +
// subquery construction must actually produce correct SQL end-to-end.
func TestBuildTraversal_SingleHop_Integration(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	_ = seed.Full(t, client)

	// Only one Org exists in seed.Full ("Test Organization", id=1), and
	// both Network (id=10) and Network2 (id=11) hang off it. A traversal
	// filter ?org__id=1 on net should return both. Using org__id (an int
	// field) rather than org__name avoids the FoldedFields shadow-column
	// path — seed.Full does not populate Organization.name_fold, so a
	// name-based filter would depend on sync-path column priming that is
	// intentionally out of scope for this unit test. The dedicated
	// TestBuildTraversal_FoldRouting_Preserved test seeds name_fold
	// explicitly and covers that branch.
	preds, empty, err := ParseFiltersCtx(WithUnknownFields(ctx), url.Values{
		"org__id": {"1"},
	}, Registry[peeringdb.TypeNet])
	if err != nil {
		t.Fatalf("ParseFiltersCtx: %v", err)
	}
	if empty {
		t.Fatal("unexpected emptyResult=true")
	}
	if len(preds) != 1 {
		t.Fatalf("preds count = %d, want 1", len(preds))
	}

	// Cast the generic selector predicate back to a typed Network
	// predicate (same shape registry_funcs.go uses).
	nwPreds := make([]func(*sql.Selector), len(preds))
	copy(nwPreds, preds)
	q := client.Network.Query().Where(func(s *sql.Selector) {
		for _, p := range nwPreds {
			p(s)
		}
	}).Where(network.StatusEQ("ok"))
	nets, err := q.All(ctx)
	if err != nil {
		t.Fatalf("Network.Query: %v", err)
	}
	if len(nets) != 2 {
		t.Errorf("expected 2 networks matching org__id=1, got %d", len(nets))
	}

	// Negative: non-matching org id filter should return 0.
	preds2, _, err := ParseFiltersCtx(WithUnknownFields(ctx), url.Values{
		"org__id": {"9999"},
	}, Registry[peeringdb.TypeNet])
	if err != nil {
		t.Fatalf("ParseFiltersCtx: %v", err)
	}
	q2 := client.Network.Query().Where(func(s *sql.Selector) {
		for _, p := range preds2 {
			p(s)
		}
	}).Where(network.StatusEQ("ok"))
	nets2, err := q2.All(ctx)
	if err != nil {
		t.Fatalf("Network.Query: %v", err)
	}
	if len(nets2) != 0 {
		t.Errorf("expected 0 networks with nonexistent org, got %d", len(nets2))
	}

	// Sanity: unfiltered returns the full seed set (both networks).
	all, err := client.Network.Query().Where(network.StatusEQ("ok")).All(ctx)
	if err != nil {
		t.Fatalf("unfiltered: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("unfiltered network count = %d, want 2 (seed invariant)", len(all))
	}
}

// TestBuildTraversal_TwoHop_Integration executes a 2-hop predicate against
// seed.Full and asserts at least one row matches. Phase 70 TRAVERSAL-03 —
// the nested-subquery SQL shape must compose correctly across two edges.
func TestBuildTraversal_TwoHop_Integration(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()
	_ = seed.Full(t, client)

	// 2-hop: ixpfx → ixlan → ix. Allowlists["ixpfx"].Via["ixlan"] = ["ix__id", "ix__name"].
	// The single seed IxPrefix lives under IxLan (id=100) which belongs
	// to IX (id=20). ?ixlan__ix__id=20 must match one row.
	preds, empty, err := ParseFiltersCtx(WithUnknownFields(ctx), url.Values{
		"ixlan__ix__id": {"20"},
	}, Registry[peeringdb.TypeIXPfx])
	if err != nil {
		t.Fatalf("ParseFiltersCtx: %v", err)
	}
	if empty {
		t.Fatal("unexpected emptyResult=true")
	}
	if len(preds) != 1 {
		t.Fatalf("preds count = %d, want 1", len(preds))
	}

	q := client.IxPrefix.Query().Where(func(s *sql.Selector) {
		for _, p := range preds {
			p(s)
		}
	})
	pfxs, err := q.All(ctx)
	if err != nil {
		t.Fatalf("IxPrefix.Query: %v", err)
	}
	if len(pfxs) == 0 {
		t.Errorf("expected at least one ixpfx matching 2-hop traversal, got 0")
	}

	// Negative: different ix id → zero rows.
	predsNeg, _, err := ParseFiltersCtx(WithUnknownFields(ctx), url.Values{
		"ixlan__ix__id": {"9999"},
	}, Registry[peeringdb.TypeIXPfx])
	if err != nil {
		t.Fatalf("ParseFiltersCtx: %v", err)
	}
	qNeg := client.IxPrefix.Query().Where(func(s *sql.Selector) {
		for _, p := range predsNeg {
			p(s)
		}
	})
	pfxsNeg, err := qNeg.All(ctx)
	if err != nil {
		t.Fatalf("IxPrefix.Query: %v", err)
	}
	if len(pfxsNeg) != 0 {
		t.Errorf("expected 0 ixpfx matching nonexistent ix id, got %d", len(pfxsNeg))
	}
}

// TestBuildTraversal_FoldRouting_Preserved asserts Phase 69 FoldedFields
// routing still applies at the target entity of a 1-hop traversal. The
// predicate should use the <field>_fold shadow column, not the raw column,
// when the target TypeConfig declares FoldedFields[field]=true.
func TestBuildTraversal_FoldRouting_Preserved(t *testing.T) {
	t.Parallel()
	client := testutil.SetupClient(t)
	ctx := t.Context()

	// Seed an Org with a diacritic-bearing name; manually set org.name_fold
	// so the fold column is populated (mirrors production sync path).
	now := time.Now().UTC()
	org, err := client.Organization.Create().
		SetID(999).
		SetName("Zürich Peers").
		SetNameFold("zurich peers").
		SetStatus("ok").
		SetCreated(now).
		SetUpdated(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	_, err = client.Network.Create().
		SetID(9999).
		SetName("Test Net").
		SetNameFold("test net").
		SetAsn(12345).
		SetStatus("ok").
		SetOrganizationID(org.ID).
		SetCreated(now).
		SetUpdated(now).
		Save(ctx)
	if err != nil {
		t.Fatalf("create net: %v", err)
	}

	// Traversal with a diacritic-free query should match via _fold column.
	preds, _, err := ParseFiltersCtx(WithUnknownFields(ctx), url.Values{
		"org__name__contains": {"Zurich"},
	}, Registry[peeringdb.TypeNet])
	if err != nil {
		t.Fatalf("ParseFiltersCtx: %v", err)
	}
	q := client.Network.Query().Where(func(s *sql.Selector) {
		for _, p := range preds {
			p(s)
		}
	}).Where(network.StatusEQ("ok"))
	nets, err := q.All(ctx)
	if err != nil {
		t.Fatalf("Network.Query: %v", err)
	}
	if len(nets) != 1 {
		t.Errorf("_fold routing broken: expected 1 network matching Zurich (via fold), got %d", len(nets))
	}

	// Sanity: the underlying Org.name_fold carries "zurich peers".
	count, err := client.Organization.Query().Where(organization.NameFoldContains("zurich")).Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 org with name_fold contains 'zurich', got %d", count)
	}
}
