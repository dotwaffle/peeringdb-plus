package parity

import (
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

func TestParity_ReverseSets(t *testing.T) {
	t.Parallel()
	// Upstream 2.84.0 serializers.py:970-996 exposes reverse related
	// names and non-FK model fields. No related status filter is added.
	c := seedFKKeys(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	srv := newTestServer(t, c)
	assertKeysResolve(t, srv, []silentIgnoreCase{
		{path: "/api/org?carrier_set__in=900", want: []int{1}},
		{path: "/api/org?campus_set__name=FKCampus50", want: []int{1}},
		{path: "/api/ix?ixfac_set__in=700", want: []int{300}},
		{path: "/api/ixlan?netixlan_set__in=5000", want: []int{3000}},
		{path: "/api/ixlan?ixpfx_set__prefix=10.0.0.0/24", want: []int{3000}},
		{path: "/api/net?netfac_set__in=600", want: []int{100}},
		{path: "/api/net?netixlan_set__in=5000", want: []int{100}},
		{path: "/api/net?poc_set__in=800", want: []int{100}},
		{path: "/api/fac?netfac_set__in=600", want: []int{200}},
		{path: "/api/fac?ixfac_set__in=700", want: []int{200}},
		{path: "/api/fac?carrierfac_set__in=950", want: []int{200}},
		{path: "/api/carrier?carrierfac_set__in=950", want: []int{900}},
	})
	c.Poc.UpdateOneID(800).SetVisible("Users").SetName("Hidden").ExecX(t.Context())
	for _, tc := range []struct {
		tier privctx.Tier
		want []int
	}{
		{privctx.TierPublic, []int{}}, {privctx.TierUsers, []int{100}},
	} {
		ts := newTestServerWithTier(t, c, tc.tier)
		assertKeysResolve(t, ts, []silentIgnoreCase{{path: "/api/net?poc_set__in=800", want: tc.want}})
		// Contact PII remains excluded at every tier, including reverse keys.
		assertKeysResolve(t, ts, []silentIgnoreCase{{path: "/api/net?poc_set__name=Hidden", want: []int{100, 101, 102}}})
	}
	c.Poc.UpdateOneID(800).SetVisible("Private").ExecX(t.Context())
	users := newTestServerWithTier(t, c, privctx.TierUsers)
	assertKeysResolve(t, users, []silentIgnoreCase{{path: "/api/net?poc_set__in=800", want: []int{}}})
	c.NetworkFacility.UpdateOneID(600).SetStatus("deleted").ExecX(t.Context())
	assertKeysResolve(t, srv, []silentIgnoreCase{{path: "/api/net?netfac_set__status=deleted", want: []int{100}}})
}
