package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// The history sweep fetches the tombstones that the mirror never saw.
//
// A bare list holds only live rows, and a ?since= window holds only the
// rows changed after the cursor. A mirror that bootstraps from bare lists
// therefore never gets the rows that upstream deleted before the
// bootstrap, and a pending campus that has not changed since then. Each
// incremental cycle sends at most HistoryMaxRequestsPerCycle requests of
// the form
//
//	/api/<type>?since=1&status=deleted&id__gte=A&id__lt=B&depth=0
//
// in id order, one type after the other in step order (parents first). A
// type is done after its last window, which has no upper id bound. The
// sweep of a cycle stops after the last window of a type, so the next type
// starts in a later cycle. By then the windows of its parent types are
// committed, and the normal fetch of that cycle has landed the parent rows
// that the cursor gate dropped (see sweepHistory). So a swept row finds
// each parent that upstream still returns in a ?since= list, with no FK
// backfill request, unless the parent table is empty. A row that Phase B
// still drops has a parent that upstream does not return (hard-deleted, or
// pending), which a later retry cannot fix. A type whose table is empty
// waits, and the later types go on. A required FK to an empty type means
// an empty child table, so only the nullable fac campus_id can point into
// an empty type (no live campus upstream). prefetchStagedFacCampuses
// backfills that campus for a swept fac, as for any fac, and the stored
// campus lets a later cycle sweep campus. Campus takes one request with no
// status filter, so it also returns the pending campuses. poc is skipped:
// upstream removes poc tombstones after 30 days, and the ?since= windows
// of the normal cycles already hold those. When every type is done, the
// sweep sends no more requests until POST /sync?mode=history restarts it.
//
// The windows stage into the scratch DB after the normal fetch, and Phase
// B upserts them in the same transaction as the rest of the cycle, with
// the incremental gate (a stored row changes only when the fetched row has
// a later updated). A merge never deletes: rows that only the mirror holds
// stay. The progress of each type is a row in the sync_history_sweep table,
// written in the sync transaction, so a cycle that rolls back also rolls
// back its sweep progress.
//
// Upstream limits: every window is a new URL, and the widths in
// historySpecs keep a window of tombstones below about 0.9 MB, under the
// 1 MB size above which upstream throttles identical repeats. The sweep
// never sends a URL again within historyMemoTTL (a failed cycle is retried
// after 30s, 2m and 8m), and it stops at the first 429 or WAF block.

// historySweepTableSQL creates the table that holds the progress of the
// history sweep: one row for each type that the sweep has started.
// next_id is the first id of the next window. A type without a row
// starts at id 1. InitStatusTable runs it.
const historySweepTableSQL = `CREATE TABLE IF NOT EXISTS sync_history_sweep (
	type TEXT PRIMARY KEY,
	next_id INTEGER NOT NULL,
	done INTEGER NOT NULL DEFAULT 0,
	updated_at TEXT NOT NULL
)`

// historyMemoTTL is how long the sweep does not send a URL again. It is
// longer than the retry ladder of a failed cycle (30s + 2m + 8m) and
// longer than the one-hour window of upstream's repeated-request limit.
const historyMemoTTL = 65 * time.Minute

// historySpec is the request shape of one type in the history sweep.
type historySpec struct {
	// width is the number of ids in one window. 0 fetches the whole type
	// in one request.
	width int
	// allStatuses leaves out status=deleted. Campus uses it: its pending
	// rows appear only in a ?since= list.
	allStatuses bool
	// hideIXNoFac adds hide_ix_no_fac=0. Upstream's IXFilterMixin can
	// hide rows of these types for a user who set "hide exchanges
	// without a facility" (2.83.0 rest.py:1267-1297).
	hideIXNoFac bool
}

// historySpecs holds the types that the sweep fetches. poc is not one of
// them (see the comment at the top of this file). The widths come from
// the density of each type's ids and the size of its tombstones, measured
// on 2026-09-24.
var historySpecs = map[string]historySpec{
	"org":        {width: 2000},
	"campus":     {allStatuses: true},
	"fac":        {width: 1000},
	"carrier":    {width: 900},
	"carrierfac": {width: 5000},
	"ix":         {width: 900, hideIXNoFac: true},
	"ixlan":      {width: 3000, hideIXNoFac: true},
	"ixpfx":      {width: 5000},
	"ixfac":      {width: 4500},
	"net":        {width: 800, hideIXNoFac: true},
	"netfac":     {width: 4000},
	"netixlan":   {width: 2400, hideIXNoFac: true},
}

// historySweepTypes is the order of the sweep: the sync step order,
// without the types that historySpecs leaves out.
var historySweepTypes = func() []string {
	var out []string
	for _, name := range canonicalStepOrder {
		if _, ok := historySpecs[name]; ok {
			out = append(out, name)
		}
	}
	return out
}()

// historyProgress is the state of one type in the sweep. The zero value
// is a type that has not started.
type historyProgress struct {
	nextID int
	done   bool
}

// historyWindow is one request of the sweep. It is comparable, so it is
// also the key of Worker.historyMemo.
type historyWindow struct {
	objectType string
	fromID     int
	// toID is the exclusive upper id bound. 0 means no bound: the last
	// window of the type.
	toID int
}

// params returns the query of the window.
func (hw historyWindow) params() url.Values {
	spec := historySpecs[hw.objectType]
	q := url.Values{}
	q.Set("since", "1")
	q.Set("depth", "0")
	q.Set("id__gte", strconv.Itoa(hw.fromID))
	if hw.toID > 0 {
		q.Set("id__lt", strconv.Itoa(hw.toID))
	}
	if !spec.allStatuses {
		q.Set("status", "deleted")
	}
	if spec.hideIXNoFac {
		q.Set("hide_ix_no_fac", "0")
	}
	return q
}

// nextHistoryWindow returns the next window of objectType and the progress
// after it. maxID is the largest id of the type in the local DB. When the
// window would reach maxID, it is the last one: it has no upper bound, so
// it also covers ids above maxID, and the type is done after it.
func nextHistoryWindow(objectType string, p historyProgress, maxID int) (historyWindow, historyProgress) {
	from := max(p.nextID, 1)
	width := historySpecs[objectType].width
	if width <= 0 || from+width > maxID {
		return historyWindow{objectType: objectType, fromID: from}, historyProgress{nextID: from, done: true}
	}
	return historyWindow{objectType: objectType, fromID: from, toID: from + width},
		historyProgress{nextID: from + width}
}

// historyRestartKey is the context key of withHistoryRestart.
type historyRestartKey struct{}

// withHistoryRestart marks ctx so that the history sweep of the cycle
// starts again at the first window of the first type. Worker.Sync sets it
// for config.SyncModeHistory.
func withHistoryRestart(ctx context.Context) context.Context {
	return context.WithValue(ctx, historyRestartKey{}, true)
}

// historyRestart reports whether ctx carries withHistoryRestart.
func historyRestart(ctx context.Context) bool {
	v, _ := ctx.Value(historyRestartKey{}).(bool)
	return v
}

// historySweep is the result of the sweep of one cycle. The caller writes
// its progress in the sync transaction (writeHistoryProgress) and logs it
// after the commit (logHistoryCommitted).
type historySweep struct {
	// restart deletes the stored progress before the new progress is
	// written.
	restart bool
	// progress is the state of each type after the windows that this
	// cycle staged.
	progress map[string]historyProgress
	// changed lists the types whose progress this cycle advanced, in
	// sweep order.
	changed []string
	// requests counts the requests sent. rows counts the rows staged.
	// dropped counts the rows not staged because their updated is later
	// than the cursor of their type, or does not parse.
	requests, rows, dropped int
	// stop says why the sweep stopped: budget, type_done, complete,
	// memo, zero_cursor, rate_limited or error.
	stop string
}

// markChanged adds objectType to s.changed once.
func (s *historySweep) markChanged(objectType string) {
	if len(s.changed) == 0 || s.changed[len(s.changed)-1] != objectType {
		s.changed = append(s.changed, objectType)
	}
}

// next returns the first type that is not done and its next id, or ""
// when every type is done.
func (s *historySweep) next() (string, int) {
	for _, name := range historySweepTypes {
		if p := s.progress[name]; !p.done {
			return name, max(p.nextID, 1)
		}
	}
	return "", 0
}

// sweepHistory stages the next windows of the history sweep into scratch.
// It returns nil when the sweep does not run: in a full cycle (the bare
// lists and the reconcile gate are a different merge), when
// HistoryMaxRequestsPerCycle is 0, or when the progress table cannot be
// read.
//
// The sweep stops at the request budget, after the last window of a type
// (type_done: the next type starts in a later cycle, see the comment at
// the top of this file), at a window sent less than historyMemoTTL ago,
// and at the first failed window. It skips a type whose table is empty:
// its cursor is zero, so the cursor gate would drop every row. The type
// is not done, and the sweep goes on with the next type. In the first
// cycle of a new install every table is empty, so the sweep sends
// nothing (zero_cursor). A failed window keeps none of its rows, and
// the windows before it keep their rows and progress. A 429 or WAF block
// logs a WARN. No window error fails the cycle, except a fault of the
// scratch DB (errScratchDB).
//
// A window stages only the rows whose updated is not later than the
// cursor of their type (MAX(updated) before the cycle). A later row may
// have changed after the normal ?since= fetch of this cycle. Committing it
// would move the cursor past the other rows that changed in that gap.
//
// now stamps the memo entries (see Worker.historyMemo).
func (w *Worker) sweepHistory(ctx context.Context, scratch *scratchDB, mode config.SyncMode, now time.Time) (*historySweep, error) {
	restart := historyRestart(ctx)
	if w.config.HistoryMaxRequestsPerCycle <= 0 || mode != config.SyncModeIncremental {
		if restart {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "history sweep is off, ignoring restart",
				slog.String("mode", string(mode)))
		}
		return nil, nil
	}
	ctx, span := otel.Tracer("sync").Start(ctx, "sync-history-sweep")
	defer span.End()

	progress := make(map[string]historyProgress, len(historySweepTypes))
	if !restart {
		stored, err := readHistoryProgress(ctx, w.db)
		if err != nil {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "failed to read history sweep progress, skipping sweep",
				slog.Any("error", err))
			return nil, nil
		}
		progress = stored
	}
	s := &historySweep{restart: restart, progress: progress}
	for k, sent := range w.historyMemo {
		if now.Sub(sent) >= historyMemoTTL {
			delete(w.historyMemo, k)
		}
	}
	if err := w.stageHistoryWindows(ctx, scratch, s, now); err != nil {
		span.RecordError(err)
		return nil, err
	}
	span.SetAttributes(
		attribute.Bool("pdbplus.sync.history.restart", s.restart),
		attribute.Int("pdbplus.sync.history.requests", s.requests),
		attribute.Int("pdbplus.sync.history.rows", s.rows),
		attribute.Int("pdbplus.sync.history.dropped", s.dropped),
		attribute.String("pdbplus.sync.history.stop", s.stop),
	)
	return s, nil
}

// stageHistoryWindows runs the window loop of sweepHistory and records in
// s why it stopped. It returns an error only for a scratch DB fault.
func (w *Worker) stageHistoryWindows(ctx context.Context, scratch *scratchDB, s *historySweep, now time.Time) error {
	budget := w.config.HistoryMaxRequestsPerCycle
	skipped := false
	for _, name := range historySweepTypes {
		p := s.progress[name]
		if p.done {
			continue
		}
		table := entityTables[name]
		cursor, err := GetMaxUpdated(ctx, w.db, table)
		if err != nil {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "history sweep stopped: cannot read cursor",
				slog.String("type", name), slog.Any("error", err))
			s.stop = "error"
			return nil
		}
		if cursor.IsZero() {
			skipped = true
			continue
		}
		maxID, err := maxStoredID(ctx, w.db, table)
		if err != nil {
			w.logger.LogAttrs(ctx, slog.LevelWarn, "history sweep stopped: cannot read max id",
				slog.String("type", name), slog.Any("error", err))
			s.stop = "error"
			return nil
		}
		for !p.done {
			if s.requests >= budget {
				s.stop = "budget"
				return nil
			}
			win, next := nextHistoryWindow(name, p, maxID)
			if sent, ok := w.historyMemo[win]; ok && now.Sub(sent) < historyMemoTTL {
				s.stop = "memo"
				return nil
			}
			w.historyMemo[win] = now
			s.requests++
			stats, err := scratch.stageHistoryWindow(ctx, w.pdbClient, win, cursor)
			if err != nil {
				if errors.Is(err, errScratchDB) {
					return err
				}
				s.stop = w.historyWindowFailed(ctx, win, err)
				return nil
			}
			pdbotel.SyncHistoryRequests.Add(ctx, 1, metric.WithAttributes(
				attribute.String("type", name), attribute.String("result", "ok")))
			s.rows += stats.rows
			s.dropped += stats.dropped
			p = next
			s.progress[name] = p
			s.markChanged(name)
		}
		s.stop = "type_done"
		if nextType, _ := s.next(); nextType == "" {
			s.stop = "complete"
		}
		return nil
	}
	s.stop = "complete"
	if skipped {
		s.stop = "zero_cursor"
	}
	return nil
}

// historyWindowFailed counts and logs a failed window and returns the
// stop reason: rate_limited for a 429 or WAF block, error otherwise.
func (w *Worker) historyWindowFailed(ctx context.Context, win historyWindow, err error) string {
	result, msg := "error", "history sweep stopped: window failed"
	if rateLimited(err) || peeringdb.IsWAFBlocked(err) {
		result, msg = "rate_limited", "history sweep stopped by upstream rate limit"
	}
	pdbotel.SyncHistoryRequests.Add(ctx, 1, metric.WithAttributes(
		attribute.String("type", win.objectType), attribute.String("result", result)))
	w.logger.LogAttrs(ctx, slog.LevelWarn, msg,
		slog.String("type", win.objectType),
		slog.Int("from_id", win.fromID),
		slog.Int("to_id", win.toID),
		slog.Any("error", err))
	return result
}

// historyStageStats counts the rows of one history window.
type historyStageStats struct {
	// rows is the number of rows staged.
	rows int
	// dropped is the number of rows not staged: their updated is later
	// than the cursor, or does not parse.
	dropped int
}

// stageHistoryWindow streams one window into the scratch table of its
// type, in one transaction. A failed window rolls its rows back.
//
// A row that the normal fetch of this cycle staged stays, unless the
// window row has a later updated: the scratch table keeps the newest
// version of each row. The updated values compare as strings, which is
// correct because upstream always sends them as RFC 3339 UTC seconds
// (2006-01-02T15:04:05Z).
func (s *scratchDB) stageHistoryWindow(ctx context.Context, pdbClient *peeringdb.Client, win historyWindow, cursor time.Time) (historyStageStats, error) {
	// #nosec G201 -- objectType is a key of historySpecs, a closed set.
	insertSQL := fmt.Sprintf(`INSERT INTO %q (id, data) VALUES (?, ?)
ON CONFLICT(id) DO UPDATE SET data = excluded.data
WHERE json_extract(CAST(excluded.data AS TEXT), '$.updated') > json_extract(CAST(data AS TEXT), '$.updated')`,
		win.objectType)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return historyStageStats{}, fmt.Errorf("begin history stage %s: %w: %w", win.objectType, errScratchDB, err)
	}
	// Rolls back a failed window. After Commit it does nothing.
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return historyStageStats{}, fmt.Errorf("prepare history insert %s: %w: %w", win.objectType, errScratchDB, err)
	}
	defer func() { _ = stmt.Close() }()

	var stats historyStageStats
	handler := func(raw json.RawMessage) error {
		var row struct {
			ID      int    `json:"id"`
			Updated string `json:"updated"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return fmt.Errorf("decode id from %s element: %w", win.objectType, err)
		}
		updated, err := time.Parse(time.RFC3339, row.Updated)
		if err != nil || updated.After(cursor) {
			stats.dropped++
			return nil
		}
		if _, err := stmt.ExecContext(ctx, row.ID, []byte(raw)); err != nil {
			return fmt.Errorf("insert history %s id=%d: %w: %w", win.objectType, row.ID, errScratchDB, err)
		}
		stats.rows++
		return nil
	}
	if err := pdbClient.StreamWithParams(ctx, win.objectType, win.params(), handler); err != nil {
		return historyStageStats{}, fmt.Errorf("stage history %s [%d, %d): %w", win.objectType, win.fromID, win.toID, err)
	}
	if err := tx.Commit(); err != nil {
		return historyStageStats{}, fmt.Errorf("commit history stage %s: %w: %w", win.objectType, errScratchDB, err)
	}
	return stats, nil
}

// maxStoredID returns the largest id in table, or 0 for an empty table.
func maxStoredID(ctx context.Context, db *sql.DB, table string) (int, error) {
	var id sql.NullInt64
	// #nosec G201 -- table comes from entityTables, a closed set.
	q := fmt.Sprintf("SELECT MAX(id) FROM %q", table)
	if err := db.QueryRowContext(ctx, q).Scan(&id); err != nil {
		return 0, fmt.Errorf("read max id of %s: %w", table, err)
	}
	return int(id.Int64), nil
}

// readHistoryProgress returns the stored progress of each type that the
// sweep has started.
func readHistoryProgress(ctx context.Context, db *sql.DB) (map[string]historyProgress, error) {
	rows, err := db.QueryContext(ctx, `SELECT type, next_id, done FROM sync_history_sweep`)
	if err != nil {
		return nil, fmt.Errorf("read history sweep progress: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]historyProgress, len(historySweepTypes))
	for rows.Next() {
		var (
			name string
			p    historyProgress
		)
		if err := rows.Scan(&name, &p.nextID, &p.done); err != nil {
			return nil, fmt.Errorf("scan history sweep progress: %w", err)
		}
		out[name] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history sweep progress: %w", err)
	}
	return out, nil
}

// writeHistoryProgress stores the progress of s in tx. For a restart it
// first deletes all stored progress. It writes nothing for a nil s, or
// when the cycle changed no progress and did not restart, so a cycle of a
// finished sweep adds no page to the LiteFS transaction.
func writeHistoryProgress(ctx context.Context, tx *ent.Tx, s *historySweep, now time.Time) error {
	if s == nil {
		return nil
	}
	if s.restart {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sync_history_sweep`); err != nil {
			return fmt.Errorf("restart history sweep: %w", err)
		}
	}
	for _, name := range s.changed {
		p := s.progress[name]
		if _, err := tx.ExecContext(ctx, `INSERT INTO sync_history_sweep (type, next_id, done, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(type) DO UPDATE SET next_id = excluded.next_id, done = excluded.done, updated_at = excluded.updated_at`,
			name, p.nextID, p.done, now.UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("write history sweep progress of %s: %w", name, err)
		}
	}
	return nil
}

// logHistoryCommitted logs the sweep of a cycle after its transaction
// commits. A cycle that rolled back changed no progress, and its caller
// logs the failure.
func logHistoryCommitted(ctx context.Context, logger *slog.Logger, s *historySweep) {
	if s == nil {
		return
	}
	if s.restart {
		// stop=memo: the first window was sent less than historyMemoTTL
		// ago. The progress is deleted, and the next cycle after the
		// memo expires starts at the first window.
		logger.LogAttrs(ctx, slog.LevelInfo, "history sweep restarted",
			slog.String("stop", s.stop))
	}
	if s.requests == 0 {
		return
	}
	nextType, nextID := s.next()
	logger.LogAttrs(ctx, slog.LevelInfo, "history sweep progress",
		slog.Int("requests", s.requests),
		slog.Int("rows", s.rows),
		slog.Int("dropped", s.dropped),
		slog.String("stop", s.stop),
		slog.String("next_type", nextType),
		slog.Int("next_id", nextID),
	)
	if nextType == "" {
		logger.LogAttrs(ctx, slog.LevelInfo, "history sweep complete")
	}
}
