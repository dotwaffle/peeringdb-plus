// Quick task 260428-mu0: replaces the meta.generated-based cursor with a
// SELECT MAX(updated) per type. PeeringDB does not include meta.generated on
// ?since= responses (see internal/peeringdb/client_live_test.go
// TestMetaGeneratedLive/paginated_incremental); the prior worker.go path
// stored the absent zero-time, which then alternated every cycle into a full
// bare-list re-fetch on the next cycle (Grafana 2026-04-28: total_objects
// oscillating 1310/1315/1317 ↔ 270176/270184/270190 every 15 min).
//
// New design: derive each per-type cursor from MAX(updated) on the
// corresponding entity table. The `updated` column is indexed on all 13
// tables (`index.Fields("updated")`), and PeeringDB's ?since=N is inclusive
// (`updated >= since`), so re-fetching the boundary row each cycle is
// idempotent — the existing OnConflict UPDATE is a no-op via the Phase 75
// skip-on-unchanged predicate.

package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// entityTables maps a PeeringDB type name (the keys produced by syncSteps())
// to the underlying ent table name. Stays in lock-step with
// initialcounts.go's UNION ALL — both consumers read raw SQLite tables
// outside the ent client. Adding a 14th type requires updating BOTH maps
// (TestEntityTablesMatchSchema introspects sqlite_master to enforce that
// every value here is a real table).
var entityTables = map[string]string{
	"org":        "organizations",
	"campus":     "campuses",
	"fac":        "facilities",
	"carrier":    "carriers",
	"carrierfac": "carrier_facilities",
	"ix":         "internet_exchanges",
	"ixlan":      "ix_lans",
	"ixpfx":      "ix_prefixes",
	"ixfac":      "ix_facilities",
	"net":        "networks",
	"poc":        "pocs",
	"netfac":     "network_facilities",
	"netixlan":   "network_ix_lans",
}

// GetMaxUpdated returns the maximum `updated` timestamp across all rows in
// the given table, or zero time if the table is empty (NULL).
//
// The cursor is derived from MAX(updated) on each sync cycle rather than
// persisted in a sync_cursors table. This works because:
//   - The `updated` column is indexed on all 13 entity tables
//     (`index.Fields("updated")` in every ent/schema/<type>.go).
//   - PeeringDB's `?since=N` is inclusive (`updated >= since` per
//     internal/pdbcompat/filter.go applySince), so re-fetching the boundary
//     row each cycle is idempotent (the Phase 75 skip-on-unchanged predicate
//     turns the OnConflict UPDATE into a no-op).
//   - Empty table → NULL → zero time → caller falls through to the full
//     bare-list path (existing stageOneTypeToScratch behaviour preserved).
//   - Tombstone rows (status='deleted') still count toward MAX(updated)
//     because their `updated` reflects the upstream deletion event.
//
// Implementation note: the query uses `ORDER BY updated DESC LIMIT 1`
// instead of `MAX(updated)` because modernc.org/sqlite only auto-parses
// TEXT → time.Time when the result column has a declared type of DATE /
// DATETIME / TIMESTAMP (see modernc.org/sqlite/rows.go:171-176). Aggregate
// expressions like MAX(...) drop the decltype, so the driver returns the
// raw stored string ("2026-04-28 12:00:00 +0000 UTC" — Go time.String()
// format, since the DSN does not pin _time_format). The
// ORDER-BY-LIMIT-1 form is index-backed (every entity has
// `index.Fields("updated")`) so the plan is identical: a single index seek.
//
// Pathological-cross-row-inconsistency caveat: if upstream serves a response
// where row R' (updated=M) is present but row R (updated < M) is missing, R
// is permanently missed under any since-based design. The
// PDBPLUS_FULL_SYNC_INTERVAL escape hatch (Task 2) defends against this.
func GetMaxUpdated(ctx context.Context, db *sql.DB, table string) (time.Time, error) {
	// #nosec G201 — table comes from the closed-set entityTables map (caller
	// passes entityTables[step.name]); SQL injection is not possible. Same
	// justification as internal/sync/scratch.go's typed-table fmt.Sprintf.
	query := fmt.Sprintf("SELECT updated FROM %q ORDER BY updated DESC LIMIT 1", table)
	var maxUpdated sql.NullTime
	if err := db.QueryRowContext(ctx, query).Scan(&maxUpdated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("get max(updated) for %s: %w", table, err)
	}
	if !maxUpdated.Valid {
		return time.Time{}, nil
	}
	return maxUpdated.Time, nil
}
