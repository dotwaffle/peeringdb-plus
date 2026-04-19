package pdbcompat

import (
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

// TestEntLimitZeroProbe empirically locks in the behaviour of ent v0.14.6's
// typed query builder when called with .Limit(0).
//
// Phase 68 Plan 68-03 research Assumption A1 hypothesised that .Limit(0)
// would emit `LIMIT 0` (SQL-standard "zero rows") and that the 13 list
// closures would therefore need to OMIT .Limit() when opts.Limit == 0.
// This probe proves otherwise: ent-generated code at
// `dialect/sql/sqlgraph/graph.go:1086` guards the Limit clause with
// `if q.Limit != 0`, so .Limit(0) is equivalent to not calling .Limit()
// at all — both return ALL rows.
//
// Consequence: the 13 list closures in registry_funcs.go can either pass
// opts.Limit unconditionally OR gate on `opts.Limit > 0` (defensive).
// Plan 68-03 Task 2 chose the explicit `if opts.Limit > 0` form for
// grep-ability and to insulate us against a future ent behaviour change.
// If that behaviour ever flips, this probe RED-trips and the closures
// must be revisited.
func TestEntLimitZeroProbe(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)

	// Network.asn is unique+positive and name is required; spread asn/id
	// to satisfy both constraints. No Organization FK needed — org_id is
	// Optional+Nillable on the Network schema.
	now := time.Now()
	for i := 1; i <= 3; i++ {
		_, err := client.Network.Create().
			SetID(i).
			SetName("probe").
			SetAsn(64500 + i).
			SetStatus("ok").
			SetCreated(now).
			SetUpdated(now).
			Save(ctx)
		if err != nil {
			t.Fatalf("seed network %d: %v", i, err)
		}
	}

	// Empirically: ent typed builder treats Limit(0) as "unlimited".
	// This diverges from the SQL-standard `LIMIT 0` semantics and is
	// the result that drives Plan 68-03's `if opts.Limit > 0 { .Limit(...) }`
	// gate (both branches produce the correct "return all rows" behaviour,
	// but the explicit gate documents intent and guards against future
	// ent changes).
	t.Run("Limit_0_returns_all_rows", func(t *testing.T) {
		rows, err := client.Network.Query().Limit(0).All(ctx)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("ent .Limit(0) returned %d rows; want 3 (ent treats Limit(0) as unlimited via sqlgraph graph.go:1086 `if q.Limit != 0` gate). If this flips, re-examine Plan 68-03 Task 2 list closures.", len(rows))
		}
	})

	t.Run("no_Limit_returns_all_rows", func(t *testing.T) {
		rows, err := client.Network.Query().All(ctx)
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		if len(rows) != 3 {
			t.Fatalf("omitting .Limit() returned %d rows; want 3.", len(rows))
		}
	})
}
