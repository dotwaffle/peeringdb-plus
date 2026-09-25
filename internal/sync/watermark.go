package sync

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// The watermark is the stored cursor of each type.
//
// Before the watermark, the cursor of a type was the newest updated of its
// table (MAX(updated)). FK backfill breaks that rule. A cycle fetches type
// T with ?since=<cursor> and gets the changes up to the time of that
// request. A later step of the same cycle can backfill a row of T that
// upstream changed after that request. MAX(updated) then moves past the
// changes of T between the request and the backfilled row, and no later
// ?since= fetch returns them. A tombstone in that gap was lost, so the
// mirror kept the row live.
//
// The rules, for each type T, in whole seconds of upstream updated:
//
//   - M is MAX(updated) before the first request of the cycle.
//   - K is the stored watermark (the sync_watermark row).
//   - E, the cursor of the cycle, is 0 for an empty table, M when T has no
//     watermark row (the migration fallback), and min(K, M) otherwise. The
//     cursor is held when K < M.
//   - S is the newest updated at the end of the sync transaction, without
//     the rows that FK backfill upserted in the cycle.
//   - The next watermark is E when the cursor is held and the tombstone
//     window of T was discarded (see stageOneTypeToScratch). Otherwise it
//     is max(E, S).
//   - When E and S are both zero (the table was empty before the cycle
//     and holds only backfilled rows at the end), the next watermark is
//     the oldest updated of the backfilled rows. Each backfill request
//     returns the rows as upstream had them at that time, so a later
//     change of a row has a later updated than the row. Two requests can
//     land rows of T at different times: the newest row can then be later
//     than a delete of an older row. A zero value writes no row.
//
// The fetch of T covered the changes from E up to the time of its
// request. Rows stored before the cycle came from earlier fetches and
// backfills, before that request. So the next watermark is never later
// than the end of the fetch of T. The next cycle fetches T again from E:
// that fetch returns the gap rows and the backfilled rows again (the
// upsert gate makes those a no-op), and the watermark then moves past
// them. The rule does not depend on the step order.
//
// Without FK backfill, S is MAX(updated) at the end of the cycle, and S is
// not below E. So the next cursor is MAX(updated), as before, and the
// requests and the data writes do not change.
//
// The watermarks are written in the sync transaction (writeSyncWatermarks),
// so they commit and roll back with the data. The table is not named
// sync_cursors: an older release used a table with that name for a
// different cursor and removed its DDL with no DROP, so a production
// database can still hold it with stale values.

// syncWatermarkTableSQL creates the table of the watermarks: one row for
// each type, max_updated in Unix seconds, and updated_at the RFC 3339 UTC
// start of the cycle that wrote the value. InitStatusTable runs it, and
// writeSyncWatermarks runs it again in the sync transaction.
const syncWatermarkTableSQL = `CREATE TABLE IF NOT EXISTS sync_watermark (
	type TEXT PRIMARY KEY,
	max_updated INTEGER NOT NULL,
	updated_at TEXT NOT NULL
)`

// upsertWatermarkSQL writes one watermark. It changes no row when the
// value is the same, so an unchanged watermark adds no page to the LiteFS
// transaction.
const upsertWatermarkSQL = `INSERT INTO sync_watermark (type, max_updated, updated_at) VALUES (?, ?, ?)
ON CONFLICT(type) DO UPDATE SET max_updated = excluded.max_updated, updated_at = excluded.updated_at
WHERE max_updated IS NOT excluded.max_updated`

// The values of the pdbplus.sync.cursor.source span attribute.
const (
	// cursorSourceWatermark: the type has a watermark row.
	cursorSourceWatermark = "watermark"
	// cursorSourceMaxUpdated: the type has rows and no watermark row.
	cursorSourceMaxUpdated = "max_updated"
	// cursorSourceEmpty: the table of the type is empty.
	cursorSourceEmpty = "empty"
)

// syncCursor is the cursor of one type in one cycle.
type syncCursor struct {
	name string
	// maxUpdated is M, the newest updated of the table before the cycle.
	maxUpdated time.Time
	// mark is K, the stored watermark in Unix seconds, when hasMark.
	mark    int64
	hasMark bool
	// effective is E, the ?since= start of the cycle. It is zero for an
	// empty table.
	effective time.Time
	// held reports that the watermark is below maxUpdated.
	held bool
}

// newSyncCursor returns the cursor of type name from the newest updated
// of its table and its stored watermark, if any.
func newSyncCursor(name string, maxUpdated time.Time, mark int64, hasMark bool) syncCursor {
	c := syncCursor{name: name, maxUpdated: maxUpdated, mark: mark, hasMark: hasMark}
	switch {
	case maxUpdated.IsZero():
		// An empty table starts from a bare list, whatever the row holds.
	case !hasMark:
		c.effective = maxUpdated
	default:
		m := maxUpdated.Unix()
		c.held = mark < m
		c.effective = time.Unix(min(mark, m), 0).UTC()
	}
	return c
}

// source returns the pdbplus.sync.cursor.source value of c.
func (c syncCursor) source() string {
	switch {
	case c.maxUpdated.IsZero():
		return cursorSourceEmpty
	case !c.hasMark:
		return cursorSourceMaxUpdated
	default:
		return cursorSourceWatermark
	}
}

// behindSeconds returns M - E in seconds for a held cursor, and 0
// otherwise.
func (c syncCursor) behindSeconds() int64 {
	if !c.held {
		return 0
	}
	return c.maxUpdated.Unix() - c.mark
}

// attributes returns the span attributes of c for its fetch span.
func (c syncCursor) attributes() []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String("pdbplus.sync.cursor.source", c.source())}
	if !c.effective.IsZero() {
		attrs = append(attrs, attribute.String("pdbplus.sync.cursor", c.effective.UTC().Format(time.RFC3339)))
	}
	if s := c.behindSeconds(); s > 0 {
		attrs = append(attrs, attribute.Int64("pdbplus.sync.cursor.behind_seconds", s))
	}
	return attrs
}

// badWatermarkError reports a sync_watermark row whose max_updated is not
// a positive integer.
type badWatermarkError struct {
	objectType string
	value      any
}

func (e *badWatermarkError) Error() string {
	return fmt.Sprintf("sync watermark of %s is %v (%T), want a positive integer", e.objectType, e.value, e.value)
}

// readSyncWatermarks returns the stored watermark of each type, in Unix
// seconds. A missing table gives no watermarks: no cycle committed one,
// so the MAX(updated) fallback applies. A value that is not a positive
// integer is a *badWatermarkError. A row of an unknown type is ignored.
//
// The caller fails the cycle on an error. It must not fall back to
// MAX(updated): that cursor can be past rows that the mirror never
// fetched, which is the defect that the watermark fixes. A zero or
// negative value would send ?since=0, and upstream reads that as no
// filter: a paged fetch of the whole type with no tombstones.
func readSyncWatermarks(ctx context.Context, db *sql.DB) (map[string]int64, error) {
	var tables int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sync_watermark'`,
	).Scan(&tables); err != nil {
		return nil, fmt.Errorf("read sync watermarks: %w", err)
	}
	marks := make(map[string]int64, len(canonicalStepOrder))
	if tables == 0 {
		return marks, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT type, max_updated FROM sync_watermark`)
	if err != nil {
		return nil, fmt.Errorf("read sync watermarks: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			name  string
			value any
		)
		if err := rows.Scan(&name, &value); err != nil {
			return nil, fmt.Errorf("scan sync watermark: %w", err)
		}
		if _, ok := entityTables[name]; !ok {
			continue
		}
		mark, ok := value.(int64)
		if !ok || mark <= 0 {
			return nil, &badWatermarkError{objectType: name, value: value}
		}
		marks[name] = mark
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read sync watermarks: %w", err)
	}
	return marks, nil
}

// logSyncCursors logs the cursors of a cycle before its first request:
// one INFO for each held cursor, and one INFO that lists the types with
// rows and no watermark. The held INFO pairs with the "fk backfill:
// parent inserted" log of an earlier cycle. The missing INFO is expected
// once, in the first cycle after the upgrade.
func logSyncCursors(ctx context.Context, logger *slog.Logger, cursors []syncCursor) {
	var missing []string
	for _, c := range cursors {
		if c.held {
			logger.LogAttrs(ctx, slog.LevelInfo, "sync cursor held behind newest row",
				slog.String("type", c.name),
				slog.Time("cursor", c.effective),
				slog.Time("max_updated", c.maxUpdated.UTC()),
				slog.Int64("behind_seconds", c.behindSeconds()),
			)
		}
		if c.source() == cursorSourceMaxUpdated {
			missing = append(missing, c.name)
		}
	}
	if len(missing) > 0 {
		logger.LogAttrs(ctx, slog.LevelInfo, "sync watermark missing, using MAX(updated)",
			slog.Any("types", missing))
	}
}

// nextWatermark returns the next watermark of a type from its cursor e
// and S, the newest updated of its table without the rows that FK
// backfill upserted in the cycle (see the comment at the top of this
// file). A held cursor whose tombstone window was discarded keeps e: the
// window did not cover the rows after e, so the next cycle fetches them
// again. The result is never below e, also when a backfilled row was the
// newest row before the cycle.
func nextWatermark(e, s time.Time, held, discarded bool) time.Time {
	if held && discarded {
		return e
	}
	if s.After(e) {
		return s
	}
	return e
}

// watermarkInput is the state of the cycle that writeSyncWatermarks
// needs.
type watermarkInput struct {
	// cursors holds the cursor of each type, in step order.
	cursors []syncCursor
	// discarded holds the types whose tombstone window
	// stageOneTypeToScratch discarded.
	discarded map[string]bool
	// backfilled holds the ids that FK backfill upserted in the cycle, by
	// type (Worker.fkBackfilled).
	backfilled map[string][]int
}

// watermarkBehind is a type whose next cursor stays below the newest row
// of its table.
type watermarkBehind struct {
	objectType string
	watermark  time.Time
	// newest is the newest updated of the table at the end of the
	// transaction.
	newest time.Time
	// kept reports that the cursor was held and the tombstone window was
	// discarded, so the watermark stayed at the cursor.
	kept bool
}

// watermarkResult is the outcome of writeSyncWatermarks.
type watermarkResult struct {
	// written counts the rows that the transaction inserted or changed.
	written int
	// behind lists the types whose next cursor stays below the newest row
	// of their table, in step order.
	behind []watermarkBehind
}

// writeSyncWatermarks computes the next watermark of each type and writes
// it in tx (see nextWatermark). It runs after the cascade, when the rows
// of the cycle are in tx. It creates the table first, for a primary that
// did not run InitStatusTable. It writes no row for a zero watermark, and
// the conditional UPSERT changes no row when the value is the same.
//
// now is the start of the cycle, stored as updated_at. The counts go on
// the span in ctx as pdbplus.sync.watermarks_written and
// pdbplus.sync.watermarks_held. The caller logs the result after the
// commit (logWatermarksCommitted).
func writeSyncWatermarks(ctx context.Context, tx *ent.Tx, in watermarkInput, now time.Time) (watermarkResult, error) {
	if _, err := tx.ExecContext(ctx, syncWatermarkTableSQL); err != nil {
		return watermarkResult{}, fmt.Errorf("create sync_watermark table: %w", err)
	}
	stamp := now.UTC().Format(time.RFC3339)
	var res watermarkResult
	for _, c := range in.cursors {
		table := entityTables[c.name]
		ids := in.backfilled[c.name]
		fetched, err := maxUpdatedExcluding(ctx, tx, table, ids)
		if err != nil {
			return watermarkResult{}, fmt.Errorf("read watermark of %s: %w", c.name, err)
		}
		discarded := in.discarded[c.name]
		next := nextWatermark(c.effective, fetched, c.held, discarded)
		if next.IsZero() && len(ids) > 0 {
			if next, err = oldestUpdatedOf(ctx, tx, table, ids); err != nil {
				return watermarkResult{}, fmt.Errorf("read watermark of %s: %w", c.name, err)
			}
		}
		mark := next.Unix()
		if next.IsZero() || mark <= 0 {
			continue
		}
		newest := fetched
		if len(ids) > 0 {
			if newest, err = maxUpdatedExcluding(ctx, tx, table, nil); err != nil {
				return watermarkResult{}, fmt.Errorf("read watermark of %s: %w", c.name, err)
			}
		}
		if newest.Unix() > mark {
			res.behind = append(res.behind, watermarkBehind{
				objectType: c.name,
				watermark:  time.Unix(mark, 0).UTC(),
				newest:     newest.UTC(),
				kept:       c.held && discarded,
			})
		}
		r, err := tx.ExecContext(ctx, upsertWatermarkSQL, c.name, mark, stamp)
		if err != nil {
			return watermarkResult{}, fmt.Errorf("write watermark of %s: %w", c.name, err)
		}
		if n, err := r.RowsAffected(); err == nil && n > 0 {
			res.written++
		}
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.Int("pdbplus.sync.watermarks_written", res.written),
		attribute.Int("pdbplus.sync.watermarks_held", len(res.behind)),
	)
	return res, nil
}

// oldestUpdatedOf returns the oldest `updated` among the rows of table
// whose id is in ids, or zero time when none of them is stored. It binds
// ids as one JSON array, as maxUpdatedExcluding does.
func oldestUpdatedOf(ctx context.Context, q queryer, table string, ids []int) (time.Time, error) {
	// #nosec G201 -- table comes from the closed-set entityTables map.
	query := fmt.Sprintf(
		"SELECT updated FROM %q WHERE id IN (SELECT value FROM json_each(?)) ORDER BY updated ASC LIMIT 1",
		table)
	rows, err := q.QueryContext(ctx, query, jsonIDs(ids))
	if err != nil {
		return time.Time{}, fmt.Errorf("get oldest updated for %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	var oldest sql.NullTime
	if rows.Next() {
		if err := rows.Scan(&oldest); err != nil {
			return time.Time{}, fmt.Errorf("get oldest updated for %s: %w", table, err)
		}
	}
	if err := rows.Err(); err != nil {
		return time.Time{}, fmt.Errorf("get oldest updated for %s: %w", table, err)
	}
	if !oldest.Valid {
		return time.Time{}, nil
	}
	return oldest.Time, nil
}

// logWatermarksCommitted logs the watermarks of a cycle after its
// transaction commits. A type behind backfilled rows logs INFO: the next
// cycle fetches it from the watermark. A type that kept its watermark
// because its tombstone window was discarded logs WARN. When this WARN
// repeats for a type, the ?since= requests of that type fail.
func logWatermarksCommitted(ctx context.Context, logger *slog.Logger, res watermarkResult) {
	for _, b := range res.behind {
		if b.kept {
			logger.LogAttrs(ctx, slog.LevelWarn, "sync watermark kept, tombstone window discarded",
				slog.String("type", b.objectType),
				slog.Time("watermark", b.watermark),
			)
			continue
		}
		logger.LogAttrs(ctx, slog.LevelInfo, "sync watermark behind backfilled rows",
			slog.String("type", b.objectType),
			slog.Time("watermark", b.watermark),
			slog.Time("max_updated", b.newest),
		)
	}
}
