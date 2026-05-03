//go:build bench
// +build bench

package pdbcompat

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	sqlite "modernc.org/sqlite"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// benchDBCounter gives each sub-benchmark an isolated in-memory DB.
// testutil.SetupClient takes *testing.T and therefore can't be reused
// from a *testing.B context; the benchmark path reimplements the tiny
// setup shim here to stay build-tag-scoped.
var benchDBCounter atomic.Int64

// benchDriverOnce guards a one-time "sqlite3" alias registration so we
// can share the DSN pattern testutil already uses. modernc.org/sqlite
// registers itself as "sqlite" via its blank-import init; we add
// "sqlite3" as an alias so enttest.Open(dialect.SQLite, "file:...") works
// — dialect.SQLite = "sqlite3" in ent.
var benchDriverOnce sync.Once

func registerBenchSQLiteDriver() {
	benchDriverOnce.Do(func() {
		for _, d := range sql.Drivers() {
			if d == "sqlite3" {
				return
			}
		}
		sql.Register("sqlite3", &sqlite.Driver{})
	})
}

// setupBenchClient constructs a fresh in-memory ent client for a
// benchmark. Mirrors testutil.SetupClient but accepts *testing.B.
func setupBenchClient(b *testing.B) *ent.Client {
	b.Helper()
	registerBenchSQLiteDriver()
	id := benchDBCounter.Add(1)
	dsn := fmt.Sprintf("file:bench_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(b, dialect.SQLite, dsn)
	b.Cleanup(func() { _ = client.Close() })
	return client
}

// seedBenchNetworks bulk-creates n networks with deterministic non-ASCII
// names so the benchmark exercises the LIKE %network% path against a
// realistic text column. IDs start at 100 to avoid collision with
// seed/Full. Each row populates both `name` and `name_fold` so the two
// predicate paths operate on equivalent data and return equal result
// cardinality (both match the literal "Network" substring present in
// every row, whether the diacritic is retained on `name` or folded on
// `name_fold`).
//
// Rows are chunked at 500 per CreateBulk call because the Network
// schema has >30 columns; CreateBulk binds every column of every row
// as a separate parameter, and SQLite's SQLITE_MAX_VARIABLE_NUMBER on
// modernc.org/sqlite v1.48.2 is 32_766. At ~36 cols × 500 rows = 18k
// params per statement we stay well under the cap.
func seedBenchNetworks(b *testing.B, client *ent.Client, n int) {
	b.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	const chunkSize = 500
	for start := 0; start < n; start += chunkSize {
		end := start + chunkSize
		if end > n {
			end = n
		}
		builders := make([]*ent.NetworkCreate, 0, end-start)
		for i := start; i < end; i++ {
			name := fmt.Sprintf("Network-%d-Zürich", i)
			builders = append(builders, client.Network.Create().
				SetID(100+i).
				SetName(name).
				SetNameFold(unifold.Fold(name)).
				SetAsn(65000+i).
				SetWebsite("https://example.com").
				SetStatus("ok").
				SetCreated(now).
				SetUpdated(now))
		}
		if err := client.Network.CreateBulk(builders...).Exec(ctx); err != nil {
			b.Fatalf("bulk create networks [%d,%d): %v", start, end, err)
		}
	}
}

// predicateBuilder is the shared shape of directContainsPredicate and
// shadowContainsPredicate (both in filter_bench.go). Declared here so
// benchNameContains can assign either helper to the same local variable
// without the compiler complaining about implicit conversion.
type predicateBuilder func(field, value string) func(*entsql.Selector)

// benchNameContains runs the query path for each of (direct, shadow) on
// the same HEAD commit without editing production code. The two
// predicate builders come from filter_bench.go (also //go:build bench).
// A fresh client is set up per benchmark so seed scale doesn't bleed
// between sub-benchmarks.
//
// The query value "Network" is chosen deliberately: it is an ASCII
// substring present in both the `name` column ("Network-%d-Zürich")
// and the `name_fold` column ("network-%d-zurich"), so both the
// direct and shadow paths return the same cardinality (all n rows).
// This keeps the comparison focused on column / LIKE scan cost rather
// than result-set-shape differences.
func benchNameContains(b *testing.B, rows int, shadow bool) {
	b.Helper()
	client := setupBenchClient(b)
	seedBenchNetworks(b, client, rows)

	var build predicateBuilder = directContainsPredicate
	if shadow {
		build = shadowContainsPredicate
	}
	ctx := context.Background()

	b.ResetTimer()
	for b.Loop() {
		pred := build("name", "Network")
		_, err := client.Network.Query().Where(pred).All(ctx)
		if err != nil {
			b.Fatalf("query shadow=%v rows=%d: %v", shadow, rows, err)
		}
	}
}

// BenchmarkNameContains_100_Direct measures the Phase 68 non-shadow
// path (FieldContainsFold on `name` with raw value) at 100 rows.
func BenchmarkNameContains_100_Direct(b *testing.B) { benchNameContains(b, 100, false) }

// BenchmarkNameContains_100_Shadow measures the Phase 69 shadow path
// (FieldContainsFold on `name_fold` with unifold.Fold(value)) at 100
// rows.
func BenchmarkNameContains_100_Shadow(b *testing.B) { benchNameContains(b, 100, true) }

// BenchmarkNameContains_10000_Direct measures the Phase 68 non-shadow
// path at 10k rows — this is the acceptance threshold target from
// coordination_notes ("no slower than current NOCASE LIKE").
func BenchmarkNameContains_10000_Direct(b *testing.B) { benchNameContains(b, 10_000, false) }

// BenchmarkNameContains_10000_Shadow measures the Phase 69 shadow
// path at 10k rows — compared against BenchmarkNameContains_10000_Direct
// to decide whether `_fold` columns need `@index(...)` annotations in
// the 6 ent schemas (Plan 05 Step D).
func BenchmarkNameContains_10000_Shadow(b *testing.B) { benchNameContains(b, 10_000, true) }
