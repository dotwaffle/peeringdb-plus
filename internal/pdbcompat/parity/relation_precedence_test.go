package parity

import (
	"net/http"
	"testing"
	"time"
)

func TestParity_RelationPrecedence(t *testing.T) {
	t.Parallel()
	// PeeringDB 2.84.0 serializers.py:708-758 stores one filter per
	// translated relation and field. QueryDict keeps first key order,
	// and get_relation_filters reads the first value of the winning key.
	c := seedFKKeys(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	c.InternetExchange.UpdateOneID(300).SetNetCount(10).ExecX(t.Context())
	c.InternetExchange.UpdateOneID(301).SetNetCount(20).ExecX(t.Context())
	c.Network.UpdateOneID(100).SetFacCount(3).ExecX(t.Context())
	c.Network.UpdateOneID(101).SetFacCount(4).ExecX(t.Context())
	srv := newTestServer(t, c)
	assertKeysResolve(t, srv, []silentIgnoreCase{
		{path: "/api/ix?net=100&net__in=101", want: []int{301}},
		{path: "/api/ix?net__in=101&net=100", want: []int{300}},
		{path: "/api/ix?net_id=100&net__in=101", want: []int{301}},
		{path: "/api/ix?net__in=101&net_id=100", want: []int{300}},
		{path: "/api/ix?net=bogus&net__in=101", want: []int{301}},
		{path: "/api/ix?net=100&net__in=101&net=102", want: []int{301}},
		{path: "/api/ix?net__in=101&net=100&net=102", want: []int{300}},
		{path: "/api/ix?%6Eet=100&net__in=101&net=102", want: []int{301}},
		{path: "/api/ix?net__asn=64500&net__asn__in=64501", want: []int{301}},
		// Different fields remain independent constraints.
		{path: "/api/ix?net__asn=64500&net__name=FKNet101", want: []int{}},
		{path: "/api/ix?net_count__gte=20&net_count__lte=10", want: []int{300}},
		{path: "/api/net?fac_count__gte=4&fac_count__lte=3", want: []int{100, 102}},
		{path: "/api/net?fac_count__lte=3&fac_count__gte=4", want: []int{101}},
		{path: "/api/net?fac_count=bogus&fac_count__gte=4", want: []int{101}},
		{path: "/api/ix?net=100&net__notes_private__contains=bogus", want: []int{300}},
		{path: "/api/ix?net__name=FKNet100&net__name__notes_private=x", want: []int{300}},
		{path: "/api/ix?net__in=100,101&net=100&net__in=101&limit=1&skip=0", want: []int{300}},
		// Pages aggregate to the same IDs as the winning filter alone.
		{path: "/api/ix?net=100&net__in=100,101&page=1&per_page=1", want: []int{300}},
		{path: "/api/ix?net=100&net__in=100,101&page=2&per_page=1", want: []int{301}},
		{path: "/api/ix/301?net=100&net__in=101&depth=0", want: []int{301}},
	})
	assertFilterError(t, srv, "/api/ix?net__in=101&net=bogus", "filter net:")
	assertFilterError(t, srv, "/api/net?fac_count__gte=4&fac_count=bogus", "filter fac_count:")
	status, body := httpGet(t, srv, "/api/ix/300?net=100&net__in=101&depth=0")
	if status != http.StatusNotFound {
		t.Errorf("detail miss: status %d, want 404: %s", status, body)
	}
}
