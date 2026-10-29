package parity

import (
	"net/http"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestParity_HideIXNoFac(t *testing.T) {
	t.Parallel()
	// Upstream 2.84.0 rest.py:1303-1330 uses stored counts, after the
	// API cache redirect at :717-721 and before pagination at :766-772.
	c := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mustOrg(ctx, t, c, 1, "Org", now)
	for _, id := range []int{10, 11} {
		mustIX(ctx, t, c, id, "IX", 1, now)
		mustIxLan(ctx, t, c, id, "LAN", id, now)
		mustNet(ctx, t, c, id, "Net", 64500+id, 1, now)
		c.NetworkIxLan.Create().SetID(id).SetNetID(id).SetIxlanID(id).SetIxID(id).SetAsn(64500 + id).SetSpeed(1000).
			SetStatus("ok").SetCreated(now).SetUpdated(now).SaveX(ctx)
	}
	c.InternetExchange.UpdateOneID(11).SetFacCount(2).ExecX(ctx)
	c.Network.UpdateOneID(11).SetIxCount(1).ExecX(ctx)
	srv := newTestServer(t, c)
	for _, typ := range []string{"ix", "ixlan", "net", "netixlan"} {
		t.Run(typ, func(t *testing.T) {
			root := "/api/" + typ
			assertKeysResolve(t, srv, []silentIgnoreCase{
				{path: root + "?hide_ix_no_fac=1&limit=250", want: []int{11}},
				{path: root + "?hide_ix_no_fac=TRUE&id__gte=0", want: []int{11}},
				{path: root + "?hide_ix_no_fac=0&hide_ix_no_fac=true&limit=1", want: []int{11}},
				{path: root + "?hide_ix_no_fac=1&hide_ix_no_fac=false&limit=250", want: []int{10, 11}},
				{path: root + "?hide_ix_no_fac=%20true&limit=250", want: []int{10, 11}},
				{path: root + "?hide_ix_no_fac=1&id__gte=0&skip=1", want: []int{}},
				{path: root + "?hide_ix_no_fac=1&id__gte=0&page=1&per_page=1", want: []int{11}},
				{path: root + "/11?hide_ix_no_fac=1&depth=0", want: []int{11}},
				// Unfiltered lists use the upstream API cache, which bypasses the flag.
				{path: root + "?hide_ix_no_fac=1", want: []int{10, 11}},
				{path: root + "?hide_ix_no_fac=1&depth=1", want: []int{10, 11}},
			})
			status, body := httpGet(t, srv, root+"/10?hide_ix_no_fac=1")
			if status != http.StatusNotFound {
				t.Errorf("detail status=%d, want 404: %s", status, body)
			}
		})
	}
	// The join does not add a status filter to the related exchange.
	c.InternetExchange.UpdateOneID(11).SetStatus("deleted").ExecX(ctx)
	assertKeysResolve(t, srv, []silentIgnoreCase{
		{path: "/api/ixlan?hide_ix_no_fac=1&limit=250", want: []int{11}},
		{path: "/api/netixlan?hide_ix_no_fac=1&limit=250", want: []int{11}},
		{path: "/api/org?hide_ix_no_fac=1&limit=250", want: []int{1}},
	})
}
