package pdbcompat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"entgo.io/ent/dialect"
	sqlite "modernc.org/sqlite"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil/seed"
)

// BenchmarkRowSize measures serialized bytes per row for each entity type
// at depth=0 (list shape) and depth=2 (detail expansion). Drives D-03
// calibration of typicalRowBytes — run with:
//
//	go test -run=NONE -bench=BenchmarkRowSize ./internal/pdbcompat -benchtime=20x -count=3
//
// seed.Full is deterministic so a small iteration count gives stable
// numbers; the metric of record is bytes/op via b.ReportMetric.
//
// The benchmark is skip-guarded under -short so regular
// `go test -short` is unaffected.

// rowSizeDBCounter gives each sub-benchmark an isolated in-memory DB
// without colliding with other benches in the package.
var rowSizeDBCounter atomic.Int64

// rowSizeDriverOnce registers modernc.org/sqlite under the "sqlite3"
// alias once per process so enttest.Open(dialect.SQLite, ...) resolves
// on systems where testutil.SetupClient's init has not run yet (bench
// suites sometimes import only this file).
var rowSizeDriverOnce sync.Once

func registerRowSizeSQLiteDriver() {
	rowSizeDriverOnce.Do(func() {
		for _, d := range sql.Drivers() {
			if d == "sqlite3" {
				return
			}
		}
		sql.Register("sqlite3", &sqlite.Driver{})
	})
}

// setupRowSizeClient mirrors testutil.SetupClient but accepts *testing.B.
// testutil.SetupClient takes *testing.T so it cannot be reused here.
func setupRowSizeClient(b *testing.B) *ent.Client {
	b.Helper()
	registerRowSizeSQLiteDriver()
	id := rowSizeDBCounter.Add(1)
	dsn := fmt.Sprintf("file:rowsize_bench_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(b, dialect.SQLite, dsn)
	b.Cleanup(func() { _ = client.Close() })
	return client
}

// benchEntities lists every type name the harness must cover. Keeping
// this explicit (rather than iterating Registry) guarantees the test
// fails loudly if somebody drops a type without updating the map.
func benchEntities() []string {
	return []string{
		peeringdb.TypeOrg, peeringdb.TypeNet, peeringdb.TypeFac,
		peeringdb.TypeIX, peeringdb.TypePoc, peeringdb.TypeIXLan,
		peeringdb.TypeIXPfx, peeringdb.TypeNetIXLan, peeringdb.TypeNetFac,
		peeringdb.TypeIXFac, peeringdb.TypeCarrier, peeringdb.TypeCarrierFac,
		peeringdb.TypeCampus,
	}
}

func BenchmarkRowSize(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping calibration bench under -short")
	}
	client := setupRowSizeClient(b)
	seed.Full(b, client)
	ctx := context.Background()

	for _, name := range benchEntities() {
		for _, depth := range []int{0, 2} {
			tc, ok := Registry[name]
			if !ok {
				b.Fatalf("registry missing %q", name)
			}
			b.Run(fmt.Sprintf("%s_Depth%d", name, depth), func(b *testing.B) {
				rows, _, err := tc.List(ctx, client, QueryOptions{Limit: 100})
				if err != nil {
					b.Fatalf("list %s: %v", name, err)
				}
				if len(rows) == 0 {
					b.Skipf("seed produced 0 rows for %s — extend seed.Full", name)
				}
				// For depth=2 we call Get on the first id (list does not honour depth).
				var sample any
				if depth == 2 {
					id := extractID(b, rows[0])
					got, err := tc.Get(ctx, client, id, 2)
					if err != nil {
						b.Fatalf("get %s id=%d depth=2: %v", name, id, err)
					}
					sample = got
				} else {
					sample = rows[0]
				}
				var totalBytes int
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					buf, err := json.Marshal(sample)
					if err != nil {
						b.Fatalf("marshal: %v", err)
					}
					totalBytes += len(buf)
				}
				b.ReportMetric(float64(totalBytes)/float64(b.N), "bytes/op")
			})
		}
	}
}

// extractID reads the integer "id" field from a serialized pdbcompat row.
// All 13 types expose id as an int per peeringdb-python conventions.
func extractID(tb testing.TB, row any) int {
	tb.Helper()
	buf, err := json.Marshal(row)
	if err != nil {
		tb.Fatalf("marshal row for id: %v", err)
	}
	var probe struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(buf, &probe); err != nil {
		tb.Fatalf("unmarshal id probe: %v", err)
	}
	if probe.ID == 0 {
		tb.Fatalf("row id=0 — seed must assign non-zero ids")
	}
	return probe.ID
}
