package web

import (
	"context"
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
)

// seedBenchSearchData creates 120+ entities across all 6 searchable types and
// returns a SearchService ready for benchmarking.
func seedBenchSearchData(b *testing.B) *SearchService {
	b.Helper()

	// enttest.Open accepts testing.TB, which *testing.B satisfies.
	dsn := "file:bench_search?mode=memory&cache=shared&_pragma=foreign_keys(1)"
	client := enttest.Open(b, dialect.SQLite, dsn)
	b.Cleanup(func() { client.Close() })

	ctx := context.Background()
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// Seed 20 organizations.
	for i := range 20 {
		_, err := client.Organization.Create().
			SetID(1000 + i).
			SetName(fmt.Sprintf("Cloud Org %d", i)).
			SetCreated(ts).
			SetUpdated(ts).
			Save(ctx)
		if err != nil {
			b.Fatalf("creating org %d: %v", i, err)
		}
	}

	// Seed 25 networks (some with "Cloud" in the name).
	for i := range 25 {
		name := fmt.Sprintf("Network Provider %d", i)
		if i%3 == 0 {
			name = fmt.Sprintf("Cloud Network %d", i)
		}
		_, err := client.Network.Create().
			SetID(2000 + i).
			SetName(name).
			SetAsn(64500 + i).
			SetOrgID(1000).
			SetCreated(ts).
			SetUpdated(ts).
			Save(ctx)
		if err != nil {
			b.Fatalf("creating network %d: %v", i, err)
		}
	}

	// Seed 20 IXPs (some with "DE-CIX" in the name).
	for i := range 20 {
		name := fmt.Sprintf("Exchange Point %d", i)
		if i%5 == 0 {
			name = fmt.Sprintf("DE-CIX Location %d", i)
		}
		_, err := client.InternetExchange.Create().
			SetID(3000 + i).
			SetName(name).
			SetCity("Frankfurt").
			SetCountry("DE").
			SetOrgID(1000).
			SetRegionContinent("Europe").
			SetMedia("Ethernet").
			SetCreated(ts).
			SetUpdated(ts).
			Save(ctx)
		if err != nil {
			b.Fatalf("creating ix %d: %v", i, err)
		}
	}

	// Seed 20 facilities (some with "Equinix" in the name).
	for i := range 20 {
		name := fmt.Sprintf("Datacenter %d", i)
		if i%4 == 0 {
			name = fmt.Sprintf("Equinix DC%d", i)
		}
		_, err := client.Facility.Create().
			SetID(4000 + i).
			SetName(name).
			SetCity("Ashburn").
			SetCountry("US").
			SetOrgID(1000).
			SetClli(fmt.Sprintf("ASHB%02d", i)).
			SetCreated(ts).
			SetUpdated(ts).
			Save(ctx)
		if err != nil {
			b.Fatalf("creating facility %d: %v", i, err)
		}
	}

	// Seed 20 campuses.
	for i := range 20 {
		_, err := client.Campus.Create().
			SetID(5000 + i).
			SetName(fmt.Sprintf("Campus Site %d", i)).
			SetCity("London").
			SetCountry("GB").
			SetOrgID(1000).
			SetCreated(ts).
			SetUpdated(ts).
			Save(ctx)
		if err != nil {
			b.Fatalf("creating campus %d: %v", i, err)
		}
	}

	// Seed 20 carriers.
	for i := range 20 {
		_, err := client.Carrier.Create().
			SetID(6000 + i).
			SetName(fmt.Sprintf("Carrier Transit %d", i)).
			SetOrgID(1000).
			SetCreated(ts).
			SetUpdated(ts).
			Save(ctx)
		if err != nil {
			b.Fatalf("creating carrier %d: %v", i, err)
		}
	}

	return NewSearchService(client)
}

// BenchmarkSearch measures search performance across different query patterns
// with 125 seeded entities.
func BenchmarkSearch(b *testing.B) {
	svc := seedBenchSearchData(b)
	ctx := context.Background()

	b.Run("broad_match", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			_, err := svc.Search(ctx, "Cloud")
			if err != nil {
				b.Fatalf("search Cloud: %v", err)
			}
		}
	})

	b.Run("narrow_match", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			_, err := svc.Search(ctx, "DE-CIX")
			if err != nil {
				b.Fatalf("search DE-CIX: %v", err)
			}
		}
	})

	b.Run("no_match", func(b *testing.B) {
		b.ResetTimer()
		for b.Loop() {
			_, err := svc.Search(ctx, "zzzznonexistent")
			if err != nil {
				b.Fatalf("search nonexistent: %v", err)
			}
		}
	})
}
