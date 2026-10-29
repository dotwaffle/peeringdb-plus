package parity

import (
	"net/http"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

func TestParity_NestedReverseFilters(t *testing.T) {
	t.Parallel()
	// PeeringDB 2.84.0 serializers.py:708-758 passes reverse related
	// names to make_relation_filter (models.py:223-236). Django compares
	// the related row ID. Only the original path's pinned row needs status ok.
	c := seedFKKeys(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	srv := newTestServer(t, c)
	assertKeysResolve(t, srv, []silentIgnoreCase{
		{path: "/api/net?ix__ixlan_set=3000", want: []int{100}},
		{path: "/api/net?ix__ixlan_set__in=3000,3001", want: []int{100, 101}},
		{path: "/api/net?ixlan__netixlan_set=5000", want: []int{100}},
		{path: "/api/fac?net__poc_set=800", want: []int{200}},
		{path: "/api/ix?fac__netfac_set=600", want: []int{300}},
		{path: "/api/net?ix__ixfac_set=700", want: []int{100}},
		{path: "/api/ix?net__netfac_set=600", want: []int{300}},
		{path: "/api/campus?facility__ix_side_set=5000", want: []int{50}},
		{path: "/api/net?fac__carrierfac_set=950", want: []int{100}},
		{path: "/api/net?ixlan__ixpfx_set=4000", want: []int{100}},
		{path: "/api/net?ix__ixlan_set__lt=3001", want: []int{100}},
		{path: "/api/net?ix__ixlan_set__in=3000,3001&limit=1&skip=0", want: []int{100}},
		{path: "/api/net?ix__ixlan_set__in=3000,3001&limit=1&skip=1", want: []int{101}},
		{path: "/api/net?ix__ixlan_set__in=3000,3001&limit=1&skip=2", want: []int{}},
		{path: "/api/net/100?ix__ixlan_set=3000&depth=0", want: []int{100}},
	})
	status, body := httpGet(t, srv, "/api/net/101?ix__ixlan_set=3000&depth=0")
	if status != http.StatusNotFound {
		t.Errorf("detail miss: status %d, want 404: %s", status, body)
	}
	assertFilterError(t, srv, "/api/net?ix__ixlan_set__contains=3000", "Invalid query")
	assertFilterError(t, srv, "/api/net?ix__ixlan_set=bogus", "filter ix__ixlan_set:")
	// Multiple matching child rows do not duplicate the listed network.
	mustIxLan(t.Context(), t, c, 3002, "Deleted child", 300, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c.IxLan.UpdateOneID(3002).SetStatus("deleted").ExecX(t.Context())
	assertKeysResolve(t, srv, []silentIgnoreCase{
		{path: "/api/net?ix__ixlan_set__in=3000,3002", want: []int{100}},
		{path: "/api/net?ix__ixlan_set=3002", want: []int{100}},
	})
	// Contact visibility applies to this extra hop at both caller tiers.
	c.Poc.UpdateOneID(800).SetVisible("Users").ExecX(t.Context())
	users := newTestServerWithTier(t, c, privctx.TierUsers)
	assertKeysResolve(t, srv, []silentIgnoreCase{{path: "/api/fac?net__poc_set=800", want: []int{}}})
	assertKeysResolve(t, users, []silentIgnoreCase{{path: "/api/fac?net__poc_set=800", want: []int{200}}})
	c.Poc.UpdateOneID(800).SetStatus("deleted").ExecX(t.Context())
	assertKeysResolve(t, users, []silentIgnoreCase{{path: "/api/fac?net__poc_set=800", want: []int{200}}})
	c.Poc.UpdateOneID(800).SetVisible("Private").ExecX(t.Context())
	assertKeysResolve(t, users, []silentIgnoreCase{{path: "/api/fac?net__poc_set=800", want: []int{}}})
	c.Poc.UpdateOneID(800).SetVisible("Public").ExecX(t.Context())
	c.NetworkFacility.UpdateOneID(600).SetStatus("deleted").ExecX(t.Context())
	assertKeysResolve(t, srv, []silentIgnoreCase{{path: "/api/fac?net__poc_set=800", want: []int{}}})
}
