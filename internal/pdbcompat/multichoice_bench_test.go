//go:build bench

package pdbcompat

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"testing"

	entsql "entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// BenchmarkMultiChoice_InfoTypesIn measures net?info_types__in= on 10k
// networks, with lists of 1 to 1000 items that match no network, so
// each row tests every item.
//
// The predicate builds the upstream string once per row
// (multiChoiceLikeAny), but it tests each item with LIKE. The cost is
// rows x items, as for the OR of icontains terms that upstream runs
// (2.83.0 serializers.py:3794-3801). It is not the flat cost per row of
// an __in filter on a plain column (buildIn). For a short list, the
// fixed cost of building the string dominates. For a long list, ns/op
// grows in proportion to the item count, and the ns/item metric comes
// near the cost of one item on the whole table.
//
// Invocation:
//
//	go test -tags=bench -run='^$' -bench=BenchmarkMultiChoice_ ./internal/pdbcompat/
func BenchmarkMultiChoice_InfoTypesIn(b *testing.B) {
	client := setupBenchClient(b)
	seedBenchNetworks(b, client, 10_000)
	ctx := context.Background()
	client.Network.Update().SetInfoTypes([]string{"Content", "NSP"}).ExecX(ctx)

	for _, items := range []int{1, 10, 100, 1000} {
		list := make([]string, items)
		for i := range list {
			list[i] = fmt.Sprintf("zz%d", i)
		}
		params := url.Values{"info_types__in": {strings.Join(list, ",")}}
		b.Run(fmt.Sprintf("items=%d", items), func(b *testing.B) {
			for b.Loop() {
				preds, _, err := ParseFiltersCtx(ctx, params, Registry[peeringdb.TypeNet])
				if err != nil {
					b.Fatalf("ParseFiltersCtx: %v", err)
				}
				n, err := client.Network.Query().Where(func(s *entsql.Selector) {
					for _, p := range preds {
						p(s)
					}
				}).Count(ctx)
				if err != nil {
					b.Fatalf("count: %v", err)
				}
				if n != 0 {
					b.Fatalf("count = %d, want 0", n)
				}
			}
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(items), "ns/item")
		})
	}
}
