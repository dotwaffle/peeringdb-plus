package pdbcompat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// setupTraversalHandler builds a Handler over a seed.Full-populated
// in-memory client. The E2E matrix below relies on the Phase 70 traversal
// fixture rows (IDs 8000+) that seed.Full adds on top of the pre-existing
// 13-entity baseline — assertions target specific row IDs, not smoke-level
// counts.
func setupTraversalHandler(t *testing.T) *http.ServeMux {
	t.Helper()
	client := testutil.SetupClient(t)
	_ = seed.Full(t, client)
	h := NewHandler(client)
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

// extractIDs decodes the response envelope and returns the "id" field of
// each row as an []int. Non-integer or missing ids fail the test via the
// passed-in *testing.T.
func extractIDs(t *testing.T, body []byte) []int {
	t.Helper()
	var env testEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("extractIDs: unmarshal envelope: %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(env.Data, &rows); err != nil {
		t.Fatalf("extractIDs: unmarshal data: %v", err)
	}
	ids := make([]int, 0, len(rows))
	for i, row := range rows {
		raw, ok := row["id"]
		if !ok {
			t.Fatalf("extractIDs: row[%d] missing id field: %+v", i, row)
		}
		f, ok := raw.(float64)
		if !ok {
			t.Fatalf("extractIDs: row[%d].id = %v (not number)", i, raw)
		}
		ids = append(ids, int(f))
	}
	return ids
}

// equalIntSets reports whether a and b contain the same set of ints
// (order-independent). Duplicate values are not expected but are compared
// positionally after sort so duplicates would also need to match.
func equalIntSets(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	aSorted := slices.Clone(a)
	bSorted := slices.Clone(b)
	slices.Sort(aSorted)
	slices.Sort(bSorted)
	return slices.Equal(aSorted, bSorted)
}

// TestTraversal_E2E_Matrix locks Phase 70 traversal behaviour end-to-end
// through the handler dispatch path against a seed.Full-populated client.
// Every subtest asserts specific expected row IDs (or explicit empty set)
// — no smoke-level "status==200 AND len(data) > 0" assertions. The seed
// fixtures at IDs 8000+ (org/campus/ix/ixlan/fac/3 nets) are the
// deterministic targets for each URL shape.
//
// Coverage matrix (17 subtests):
//   - Path A 1-hop: org__name on net, fac, ix, carrier (4 entity cases)
//   - Path A 1-hop: campus__name on fac (5th 1-hop case)
//   - Path A 2-hop upstream parity: fac?ixlan__ix__fac_count__gt=0
//     (pdb_api_test.py:2340) — silent-ignored (fac has no ixlan edge)
//   - Path A 2-hop upstream parity: fac?ixlan__ix__id=8001
//     (pdb_api_test.py:2348) — silent-ignored (same reason)
//   - Path A 1-hop + op upstream parity: net?ix__name__contains=TestIX
//     (pdb_api_test.py:5081) — silent-ignored (net has no ix edge;
//     `ix__name` in Allowlists["net"].Direct tries Path A, buildSinglHop
//     fails LookupEdge, falls through to unknown)
//   - Path B fallback 1-hop: net?org__city=Amsterdam (edge exists, field
//     exists, no row matches) — expected empty set
//   - Unknown-field silent-ignore (5 cases): unknown local, unknown edge,
//     known edge with unknown target field, 3-hop, 4-hop — each returns
//     the unfiltered live-row set for the type (DeletedNet 8003 excluded
//     by Phase 68 status matrix)
//   - Multi-filter composition (pdb_api_test.py:5047):
//     net?org__id=8001&ix__name=TestIX — org__id resolves (8001/8002),
//     ix__name silent-ignored
//   - Phase 69 _fold preservation: net?name__contains=Zurich — matches
//     both fold-normalised rows
//   - Phase 69 __in sentinel: net?org_id__in= — empty __in short-circuits
func TestTraversal_E2E_Matrix(t *testing.T) {
	t.Parallel()
	mux := setupTraversalHandler(t)

	// Seed.Full baseline (live, status=ok):
	//   net: 10 (Cloudflare, org=1), 11 (Hurricane Electric, org=1),
	//        8001 (TestNet1-Zurich, org=8001), 8002 (Zürich GmbH, org=8001)
	//   fac: 30 (Equinix FR5, org=1), 31 (Campus Facility, org=1 campus=40),
	//        8001 (TestFac1-Campus, org=8001 campus=8001)
	//   ix:  20 (DE-CIX Frankfurt, org=1), 8001 (TestIX, org=8001)
	//   carrier: 50 (Test Carrier, org=1)
	//   campus: 40 (Test Campus, org=1), 8001 (TestCampus1, org=8001)
	//   org: 1 (Test Organization, Frankfurt), 8001 (TestOrg1)
	//
	// Tombstone (excluded by Phase 68 status matrix on list w/o ?since):
	//   net 8003 (DeletedNet, org=8001, status=deleted)
	allLiveNets := []int{10, 11, 8001, 8002}
	allLiveFacs := []int{30, 31, 8001}

	tests := []struct {
		name        string
		url         string
		expectedIDs []int
	}{
		// Path A 1-hop.
		{
			name:        "path_a_1hop_net_org_name",
			url:         "/api/net?org__name=TestOrg1",
			expectedIDs: []int{8001, 8002},
		},
		{
			name:        "path_a_1hop_fac_org_name",
			url:         "/api/fac?org__name=TestOrg1",
			expectedIDs: []int{8001},
		},
		{
			name:        "path_a_1hop_ix_org_name",
			url:         "/api/ix?org__name=TestOrg1",
			expectedIDs: []int{8001},
		},
		{
			name:        "path_a_1hop_carrier_org_name",
			url:         "/api/carrier?org__name=TestOrg1",
			expectedIDs: []int{},
		},
		// Junction-type 1-hop via Path A Allowlist. netfac has
		// Allowlists.Direct = ["fac__country","fac__name","net__asn","net__name"].
		// seed.Full NetworkFacility 300 links net 10 (asn=13335). Filtering
		// by net__asn (int field, no _fold routing) validates parent-FK
		// column resolution on a non-org edge without depending on
		// name_fold priming — seed.Full's pre-Phase-70 rows don't populate
		// the fold shadow columns.
		// (A campus-targeting 1-hop case was deferred —
		// see .planning/phases/70-cross-entity-traversal/deferred-items.md
		// DEFER-70-06-01 for the campus inflection codegen bug.)
		{
			name:        "path_a_1hop_netfac_net_asn",
			url:         "/api/netfac?net__asn=13335",
			expectedIDs: []int{300},
		},
		// Upstream 2-hop parity cases — fac has no "ixlan" edge, so Path A
		// matches the Via allowlist but buildTwoHop fails LookupEdge;
		// silent-ignore returns all live facs.
		{
			name:        "upstream_2340_fac_ixlan_ix_fac_count_gt",
			url:         "/api/fac?ixlan__ix__fac_count__gt=0",
			expectedIDs: allLiveFacs,
		},
		{
			name:        "upstream_2348_fac_ixlan_ix_id",
			url:         "/api/fac?ixlan__ix__id=8001",
			expectedIDs: allLiveFacs,
		},
		// Upstream 5081 — net has no "ix" edge (only network_ix_lans); the
		// `ix__name` allowlist entry fails buildSinglHop lookup and the
		// key is silent-ignored.
		{
			name:        "upstream_5081_net_ix_name_contains",
			url:         "/api/net?ix__name__contains=TestIX",
			expectedIDs: allLiveNets,
		},
		// Path B fallback 1-hop: org.city is a queryable field via Path B
		// introspection (net has org edge → Registry["org"].Fields["city"]
		// = FieldString). No org has city=Amsterdam, so empty result.
		{
			name:        "path_b_1hop_net_org_city_no_match",
			url:         "/api/net?org__city=Amsterdam",
			expectedIDs: []int{},
		},
		// Unknown-field silent-ignore (5 cases).
		{
			name:        "unknown_local_field_silently_ignored",
			url:         "/api/net?totally_bogus_field=x",
			expectedIDs: allLiveNets,
		},
		{
			name:        "unknown_edge_silently_ignored",
			url:         "/api/net?bogus_edge__name=x",
			expectedIDs: allLiveNets,
		},
		{
			name:        "known_edge_unknown_target_field_silently_ignored",
			url:         "/api/net?org__bogus_field=x",
			expectedIDs: allLiveNets,
		},
		{
			name:        "over_cap_3hop_silently_ignored",
			url:         "/api/net?a__b__c__d=x",
			expectedIDs: allLiveNets,
		},
		{
			name:        "over_cap_4hop_silently_ignored",
			url:         "/api/net?a__b__c__d__e=x",
			expectedIDs: allLiveNets,
		},
		// Upstream 5047: multi-filter composition — org__id resolves
		// (matches 8001/8002), ix__name silent-ignored (no ix edge on net).
		{
			name:        "upstream_5047_multifilter_org_id_and_ix_name",
			url:         "/api/net?org__id=8001&ix__name=TestIX",
			expectedIDs: []int{8001, 8002},
		},
		// Phase 69 _fold preservation on traversal-parser path. `name` is
		// a local FoldedFields field; contains routes through the _fold
		// column. TestNet1-Zurich (name_fold="testnet1-zurich") and
		// Zürich GmbH (name_fold="zurich gmbh") both match "zurich".
		{
			name:        "phase69_fold_contains_ascii_zurich",
			url:         "/api/net?name__contains=Zurich",
			expectedIDs: []int{8001, 8002},
		},
		// Phase 69 IN-02: empty __in short-circuits before SQL executes.
		{
			name:        "phase69_empty_in_returns_empty_set",
			url:         "/api/net?org_id__in=",
			expectedIDs: []int{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: status = %d, want 200 (TRAVERSAL-04 silent-ignore contract): body=%s",
					tc.name, rec.Code, rec.Body.String())
			}
			got := extractIDs(t, rec.Body.Bytes())
			if !equalIntSets(got, tc.expectedIDs) {
				t.Errorf("%s: got IDs %v, want %v (response: %s)",
					tc.name, got, tc.expectedIDs, rec.Body.String())
			}
		})
	}
}
