//go:build bench

package pdbcompat

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// BenchmarkDistanceSearch_Org measures an org distance search, the List
// and the Count query that serveList runs, on 33,000 orgs of which 80%
// have coordinates, for a 700 km search around Frankfurt. No index
// serves the filter or the order, so each request reads every ok org.
// Run it with go test -tags=bench -run '^$' -bench DistanceSearch.
func BenchmarkDistanceSearch_Org(b *testing.B) {
	c := testutil.SetupClient(b)
	ctx := b.Context()
	now := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	const n = 33000
	for i := 1; i <= n; i++ {
		q := c.Organization.Create().SetID(i).SetName(fmt.Sprintf("Org %d", i)).
			SetNameFold(fmt.Sprintf("org %d", i)).
			SetStatus("ok").SetCreated(now).SetUpdated(now)
		if i%5 != 0 {
			// Spread the points over -60..60 latitude and -180..180
			// longitude.
			q.SetLatitude(float64(i%120) - 60).SetLongitude(float64(i%360) - 180)
		}
		q.SaveX(ctx)
	}
	params := url.Values{"distance": {"700"}, "latitude": {"50.1109"}, "longitude": {"8.6821"}}
	tc := Registry["org"]
	lf, err := parseListFilters(ctx, params, tc)
	if err != nil {
		b.Fatal(err)
	}
	opts := QueryOptions{Filters: lf.preds, OrderBy: lf.orderBy}
	for b.Loop() {
		if _, err := tc.Count(ctx, c, opts); err != nil {
			b.Fatal(err)
		}
		if _, err := tc.List(ctx, c, opts); err != nil {
			b.Fatal(err)
		}
	}
}
