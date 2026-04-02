package grpcserver

import (
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// seedBenchNetworks creates n networks in an in-memory SQLite database and
// returns a NetworkService ready for benchmarking.
func seedBenchNetworks(b *testing.B, n int) *NetworkService {
	b.Helper()

	dsn := fmt.Sprintf("file:bench_list_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", n)
	client := enttest.Open(b, dialect.SQLite, dsn)
	b.Cleanup(func() { client.Close() })

	ctx := b.Context()
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := range n {
		_, err := client.Network.Create().
			SetID(i + 1).
			SetName(fmt.Sprintf("Network %d", i)).
			SetAsn(64500 + i).
			SetCreated(ts).
			SetUpdated(ts).
			Save(ctx)
		if err != nil {
			b.Fatalf("creating network %d: %v", i, err)
		}
	}

	return &NetworkService{Client: client}
}

// BenchmarkListNetworks measures paginated list performance at different data
// sizes and page sizes. This benchmarks the actual hot path used by all 13
// gRPC List RPCs (query + convert + paginate).
func BenchmarkListNetworks(b *testing.B) {
	ctx := b.Context()

	b.Run("100_items_page50", func(b *testing.B) {
		svc := seedBenchNetworks(b, 100)
		req := &pb.ListNetworksRequest{PageSize: 50}
		b.ResetTimer()
		for b.Loop() {
			_, err := svc.ListNetworks(ctx, req)
			if err != nil {
				b.Fatalf("ListNetworks: %v", err)
			}
		}
	})

	b.Run("1000_items_page100", func(b *testing.B) {
		svc := seedBenchNetworks(b, 1000)
		req := &pb.ListNetworksRequest{PageSize: 100}
		b.ResetTimer()
		for b.Loop() {
			_, err := svc.ListNetworks(ctx, req)
			if err != nil {
				b.Fatalf("ListNetworks: %v", err)
			}
		}
	})

	b.Run("empty_result", func(b *testing.B) {
		svc := seedBenchNetworks(b, 0)
		req := &pb.ListNetworksRequest{}
		b.ResetTimer()
		for b.Loop() {
			_, err := svc.ListNetworks(ctx, req)
			if err != nil {
				b.Fatalf("ListNetworks: %v", err)
			}
		}
	})
}
