package parity

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

func TestParity_RelationCaseFilters(t *testing.T) {
	t.Parallel()
	// PeeringDB 2.84.0 serializers.py:718-750 keeps raw contains and
	// startswith on three-segment keys. Two-segment forms use i-lookups.
	c := seedFKKeys(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	const name = "München [IX]*%_\\West"
	c.InternetExchange.UpdateOneID(300).SetName(name).SetNameFold(unifold.Fold(name)).ExecX(t.Context())
	c.Facility.UpdateOneID(200).SetName(name).SetNameFold(unifold.Fold(name)).ExecX(t.Context())
	srv := newTestServer(t, c)
	assertKeysResolve(t, srv, []silentIgnoreCase{
		{path: "/api/net?ix__name__contains=" + url.QueryEscape("München"), want: []int{100}},
		{path: "/api/net?ix__name__contains=" + url.QueryEscape("münchen"), want: []int{}},
		{path: "/api/net?ix__name__contains=Munchen", want: []int{}},
		{path: "/api/net?ix__name__startswith=" + url.QueryEscape("Mün"), want: []int{100}},
		{path: "/api/net?ix__name__startswith=" + url.QueryEscape("mün"), want: []int{}},
		{path: "/api/net?ix__name__startswith=Mun", want: []int{}},
		{path: "/api/net?ix__name__contains=" + url.QueryEscape("[IX]*%_\\"), want: []int{100}},
		{path: "/api/net?ix__name__contains=", want: []int{100, 101}},
		// Two-segment field-replacement seeds still ignore case and accents.
		{path: "/api/netfac?name__contains=munchen", want: []int{600}},
		{path: "/api/netfac?name__anything__contains=munchen", want: []int{}},
		{path: "/api/netfac?name__anything__startswith=" + url.QueryEscape("Mün"), want: []int{600}},
		// Model filters keep their documented folding behavior.
		{path: "/api/ix?name__contains=munchen", want: []int{300}},
		{path: "/api/net/100?ix__name__contains=" + url.QueryEscape("München") + "&depth=0", want: []int{100}},
	})
	status, body := httpGet(t, srv, "/api/net/100?ix__name__contains=munchen&depth=0")
	if status != http.StatusNotFound {
		t.Errorf("detail miss: status %d, want 404: %s", status, body)
	}
}
