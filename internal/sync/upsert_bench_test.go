package sync

import (
	"fmt"
	"testing"
	"time"

	"entgo.io/ent/dialect"

	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// generateBenchOrgs creates n Organization structs with realistic field values.
func generateBenchOrgs(n int) []peeringdb.Organization {
	ts := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	orgs := make([]peeringdb.Organization, n)
	for i := range n {
		orgs[i] = peeringdb.Organization{
			ID:       i + 1,
			Name:     fmt.Sprintf("Organization %d", i),
			Aka:      fmt.Sprintf("Org%d", i),
			NameLong: fmt.Sprintf("Organization Number %d International", i),
			Website:  fmt.Sprintf("https://org%d.example.com", i),
			Address1: fmt.Sprintf("%d Main Street", 100+i),
			City:     "Frankfurt",
			State:    "HE",
			Country:  "DE",
			Zipcode:  "60313",
			Created:  ts,
			Updated:  ts,
			Status:   "ok",
		}
	}
	return orgs
}

// BenchmarkUpsertOrganizations measures bulk upsert performance at different
// batch sizes using in-memory SQLite.
func BenchmarkUpsertOrganizations(b *testing.B) {
	ctx := b.Context()

	b.Run("100_orgs", func(b *testing.B) {
		dsn := "file:bench_upsert_100?mode=memory&cache=shared&_pragma=foreign_keys(1)"
		client := enttest.Open(b, dialect.SQLite, dsn)
		b.Cleanup(func() { client.Close() })

		orgs := generateBenchOrgs(100)
		b.ResetTimer()
		for b.Loop() {
			tx, err := client.Tx(ctx)
			if err != nil {
				b.Fatalf("begin tx: %v", err)
			}
			if _, err := upsertOrganizations(ctx, tx, orgs); err != nil {
				_ = tx.Rollback()
				b.Fatalf("upsert 100 orgs: %v", err)
			}
			if err := tx.Commit(); err != nil {
				b.Fatalf("commit tx: %v", err)
			}
		}
	})

	b.Run("500_orgs", func(b *testing.B) {
		dsn := "file:bench_upsert_500?mode=memory&cache=shared&_pragma=foreign_keys(1)"
		client := enttest.Open(b, dialect.SQLite, dsn)
		b.Cleanup(func() { client.Close() })

		orgs := generateBenchOrgs(500)
		b.ResetTimer()
		for b.Loop() {
			tx, err := client.Tx(ctx)
			if err != nil {
				b.Fatalf("begin tx: %v", err)
			}
			if _, err := upsertOrganizations(ctx, tx, orgs); err != nil {
				_ = tx.Rollback()
				b.Fatalf("upsert 500 orgs: %v", err)
			}
			if err := tx.Commit(); err != nil {
				b.Fatalf("commit tx: %v", err)
			}
		}
	})
}
