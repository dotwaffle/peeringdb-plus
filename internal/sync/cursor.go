// The cursor of each type starts from the newest `updated` of its entity
// table. PeeringDB does not include meta.generated on ?since= responses
// (see internal/peeringdb/client_live_test.go
// TestMetaGeneratedLive/paginated_incremental). An earlier design stored
// that absent value as zero time, so every other cycle ran a full
// bare-list fetch (Grafana 2026-04-28: total_objects alternated between
// about 1310 and about 270180 every 15 min).
//
// MAX(updated) is the clamp and the migration fallback of the cursor. The
// cursor itself is the watermark of the type (see watermark.go). The
// `updated` column is indexed on all 13 tables (`index.Fields("updated")`).
// Upstream usually sends the boundary row again (see GetMaxUpdated), and a
// second fetch of it writes nothing: the skip-on-unchanged predicate makes
// the OnConflict UPDATE a no-op.

package sync

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// queryer runs a query that returns rows. *sql.DB and *ent.Tx (through
// its QueryContext) both satisfy it, so a read can run inside or outside
// the sync transaction.
type queryer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// GetMaxUpdated returns the maximum `updated` timestamp across all rows in
// the given table, or zero time if the table is empty (NULL).
//
// The cursor of a type is its watermark, clamped to this value (see
// newSyncCursor). A type with no watermark row uses this value as its
// cursor. The value works as a cursor because:
//   - The `updated` column is indexed on all 13 entity tables
//     (`index.Fields("updated")` in every ent/schema/<type>.go).
//   - Upstream's `?since=N` filter is strict (`updated > N`, django-handleref
//     2.0.1 manager.py:53-55), but it only looks inclusive: N is whole
//     seconds, the wire value of `updated` is truncated to the second
//     (2.83.0 serializers.py:1920-1924), and upstream stores sub-second
//     values. So rows whose stored `updated` falls inside the cursor's own
//     second, usually including the boundary row, come back each cycle.
//     Re-fetching them is idempotent (the skip-on-unchanged predicate
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
	return maxUpdatedExcluding(ctx, db, table, nil)
}

// maxUpdatedExcluding returns the newest `updated` in table among the rows
// whose id is not in ids, or zero time when no such row exists. With no
// ids it runs the query of GetMaxUpdated. Otherwise it binds ids as one
// JSON array, so the statement has one parameter for any number of ids.
// The plan walks the `updated` index from its newest entry and skips at
// most len(ids) rows.
//
// writeSyncWatermarks calls it in the sync transaction with the ids that
// FK backfill landed in the cycle.
func maxUpdatedExcluding(ctx context.Context, q queryer, table string, ids []int) (time.Time, error) {
	// #nosec G201 — table comes from the closed-set entityTables map (caller
	// passes entityTables[step.name]); SQL injection is not possible. Same
	// justification as internal/sync/scratch.go's typed-table fmt.Sprintf.
	query := fmt.Sprintf("SELECT updated FROM %q ORDER BY updated DESC LIMIT 1", table)
	var args []any
	if len(ids) > 0 {
		// #nosec G201 -- same closed-set table name as above.
		query = fmt.Sprintf(
			"SELECT updated FROM %q WHERE id NOT IN (SELECT value FROM json_each(?)) ORDER BY updated DESC LIMIT 1",
			table)
		args = append(args, jsonIDs(ids))
	}
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return time.Time{}, fmt.Errorf("get max(updated) for %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var maxUpdated sql.NullTime
	if rows.Next() {
		if err := rows.Scan(&maxUpdated); err != nil {
			return time.Time{}, fmt.Errorf("get max(updated) for %s: %w", table, err)
		}
	}
	if err := rows.Err(); err != nil {
		return time.Time{}, fmt.Errorf("get max(updated) for %s: %w", table, err)
	}
	if !maxUpdated.Valid {
		return time.Time{}, nil
	}
	return maxUpdated.Time, nil
}
