package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, already a project dep

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// scratchChunkSize is the number of raw rows drained from the scratch DB
// into memory at a time during Phase B replay. Each chunk is decoded
// into typed Go structs and then passed to the per-type upsertX
// function. upsertBatch writes the chunk with one upsert statement for
// each batchSize rows (see upsert.go). The FK pre-pass also works on one
// chunk at a time: it sends one backfill request for each parent type
// that has missing parents in the chunk.
//
// The chunk size had almost no effect on memory. In a full sync at
// production row counts, peak Go heap was 20-23 MiB above the base, and
// peak RSS at most 12 MiB above the base, with chunks of 50, 100 and 500
// rows. An earlier table here showed chunks of 250 rows and more near a
// 400 MiB heap limit. That benchmark kept all of its fixtures (about
// 110 MB) on the heap, and its heap samples included them.
const scratchChunkSize = 100

// scratchDB is a sql.DB handle to the per-sync SQLite file plus the
// absolute path so closeScratchDB can unlink it on teardown.
//
// The scratch DB stages raw JSON rows from each PeeringDB type via
// StreamAll's callback — Phase A Go heap stays bounded to one element
// per handler invocation (~5-10 KB) instead of one full []T per type
// (~35 MB for netixlan). Phase B reads the scratch rows back in chunks
// and replays them into the real ent tables inside the single ent.Tx.
//
// Lifetime: opened at the start of each sync cycle, closed and unlinked
// by defer closeScratchDB(...) at the end of the same cycle. A random
// file name prevents collisions across concurrent worker processes.
// On Fly.io, this case does not occur: only one primary runs at a time.
type scratchDB struct {
	db   *sql.DB
	path string
}

// scratchFilePrefix starts the name of each scratch database file.
// sweepStaleScratchFiles removes only files with this prefix.
const scratchFilePrefix = "pdbplus-sync-scratch-"

// openScratchDB creates an isolated SQLite database file in dir,
// pre-populates all 13 staging tables, and returns a scratchDB wrapper.
// An empty dir means os.TempDir(). A missing dir is created with mode
// 0700. The caller MUST call closeScratchDB to both close the handle and
// unlink the file.
//
// SQLite pragmas:
//   - journal_mode=MEMORY: the rollback journal is in memory, not in a
//     file. The scratch DB is transient, so crash-safety is irrelevant. A
//     crashed process leaves its file behind, and no later openScratchDB
//     call uses that name. stageType can roll back a failed transaction,
//     and the rollback needs the journal. With journal_mode=OFF, the changed
//     pages that SQLite already wrote to the file stay after a rollback.
//     The journal holds the original pages that a transaction changes: a
//     few pages when a stage fills an empty table, and more when a
//     tombstone window replaces staged rows. In a test with a copy of the
//     production data, a window that replaced every staged row raised peak
//     RSS by 95 MiB. A window of 1000 rows for each table raised it by
//     9 MiB or less. A journal file would write the same pages to dir.
//     In production, dir is on the volume of the primary, and the journal
//     writes would compete with the LiteFS writes to that volume.
//   - synchronous=OFF — skip fsyncs. Writes go straight to the OS page
//     cache; correctness is preserved because SQLite is the only writer.
//
// Path uniqueness: os.CreateTemp picks a random name, so concurrent Sync
// runs do not collide on a scratch file. This is also true for different
// processes that share a temp directory. A PID-based name is not unique
// across PID namespaces, because sandboxed or containerized processes
// often get the same small PID.
func openScratchDB(ctx context.Context, dir string) (*scratchDB, error) {
	// A configured dir can be missing, for example on a new volume.
	if dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create scratch dir %s: %w", dir, err)
		}
	}
	// Atomically create an exclusive scratch file so neither a stale file
	// nor a pre-placed symlink can be targeted. os.CreateTemp opens with
	// O_EXCL and mode 0600, and tries a new random name if one exists.
	f, err := os.CreateTemp(dir, scratchFilePrefix+"*.db")
	if err != nil {
		return nil, fmt.Errorf("create scratch db: %w", err)
	}
	path := f.Name()
	_ = f.Close()
	// sql.Open needs exclusive access; remove the empty file we just
	// created so SQLite can write its magic bytes fresh. Between here
	// and sql.Open there is still a micro-window, but any attacker who
	// can race it would have been able to pre-place the file before
	// O_EXCL too — and since we just proved the path was free, the
	// window is strictly narrower than the pre-fix implementation.
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("clear exclusive scratch placeholder at %s: %w", path, err)
	}

	// Shrink the SQLite page cache on the scratch handle: default is
	// 2000 pages ≈ 8 MB, but for bulk staging we read rows sequentially
	// in chunks and never revisit older ones, so a tiny cache is fine.
	// Negative cache_size is in KiB (positive is pages); -2048 = 2 MiB.
	// With synchronous=OFF, this keeps the non-Go-heap footprint of the
	// scratch DB small.
	//
	// EscapedPath percent-encodes path. SQLite reads '?', '#' and '%' in
	// a URI as syntax, so an unescaped dir with one of these characters
	// makes SQLite open a different file, which closeScratchDB does not
	// remove.
	dsn := "file:" + (&url.URL{Path: path}).EscapedPath() +
		"?_pragma=journal_mode(MEMORY)&_pragma=synchronous(OFF)&_pragma=cache_size(-2048)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open scratch db at %s: %w", path, err)
	}
	// Serialize writes: SQLite is a single-writer store and the modernc
	// driver cannot parallelise transactions across connections cleanly.
	db.SetMaxOpenConns(1)

	s := &scratchDB{db: db, path: path}
	if err := s.initSchema(ctx); err != nil {
		_ = db.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("init scratch schema: %w", err)
	}
	return s, nil
}

// initSchema creates one staging table per PeeringDB type. Schema is
// minimal: (id INTEGER PRIMARY KEY, data BLOB NOT NULL). The BLOB holds
// the raw json.RawMessage bytes from the PeeringDB response; Phase B
// drains each table in chunks and decodes the BLOBs into typed Go
// structs only at the moment of upsert, keeping Go heap bounded.
//
// Rationale for BLOB-over-columns: the ent schema for 13 entity types
// has dozens of columns each; mirroring them in scratch would require
// maintaining a parallel schema definition that drifts under future
// schema edits. The BLOB staging avoids that trap entirely — scratch is
// schema-agnostic and the existing upsertX functions handle the typed
// mapping inside the single ent.Tx.
func (s *scratchDB) initSchema(ctx context.Context) error {
	for _, t := range scratchTypes {
		// Table name is from the closed-set constant list — safe against
		// SQL injection; no user input flows through here.
		stmt := fmt.Sprintf("CREATE TABLE %q (id INTEGER PRIMARY KEY, data BLOB NOT NULL)", t)
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create scratch.%s: %w", t, err)
		}
	}
	return nil
}

// closeScratchDB closes the underlying sql.DB handle and unlinks the
// file. Safe to call in a defer even if openScratchDB returned an error
// (nil-safe on receiver). Errors are logged at WARN level — a failure
// to close or unlink a transient scratch file is non-fatal but deserves
// operator attention. The ctx parameter is used only for log attribute
// propagation (trace context) — actual close/unlink is synchronous.
func closeScratchDB(ctx context.Context, s *scratchDB, logger *slog.Logger) {
	if s == nil {
		return
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil && logger != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "close scratch db",
				slog.String("path", s.path),
				slog.Any("error", err),
			)
		}
	}
	if s.path != "" {
		if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) && logger != nil {
			logger.LogAttrs(ctx, slog.LevelWarn, "unlink scratch db",
				slog.String("path", s.path),
				slog.Any("error", err),
			)
		}
	}
}

// sweepStaleScratchFiles removes the scratch files that an earlier
// process left in dir. It returns the number of files and bytes that it
// removed. closeScratchDB removes the file of each cycle, but a process
// that crashes or is killed during a cycle leaves its file. On a
// persistent volume, these files stay after a restart and can fill the
// volume.
//
// The sweep removes only regular files whose name starts with
// scratchFilePrefix. It keeps directories, symbolic links and all other
// files. It cannot tell the file of a live process from a stale file, so
// dir must belong to one process. An empty dir means os.TempDir(), which
// other processes share, so the sweep does nothing. A missing dir has no
// files to remove.
//
// It logs a WARN with the counts when it removed files, and a DEBUG when
// it removed none. It logs an error at WARN and does not return it,
// because a failed sweep must not stop the scheduler.
func sweepStaleScratchFiles(ctx context.Context, dir string, logger *slog.Logger) (files int, size int64) {
	if dir == "" {
		return 0, 0
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.LogAttrs(ctx, slog.LevelWarn, "failed to sweep stale scratch files",
			slog.String("dir", dir),
			slog.Any("error", err),
		)
		return 0, 0
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasPrefix(e.Name(), scratchFilePrefix) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		var n int64
		if info, err := e.Info(); err == nil {
			n = info.Size()
		}
		if err := os.Remove(path); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				logger.LogAttrs(ctx, slog.LevelWarn, "failed to remove stale scratch file",
					slog.String("path", path),
					slog.Any("error", err),
				)
			}
			continue
		}
		files++
		size += n
	}
	level := slog.LevelDebug
	if files > 0 {
		level = slog.LevelWarn
	}
	logger.LogAttrs(ctx, level, "removed stale scratch files",
		slog.String("dir", dir),
		slog.Int("count", files),
		slog.Int64("bytes", size),
	)
	return files, size
}

// sweepScratchDirAtStartup runs sweepStaleScratchFiles on the configured
// scratch directory. StartScheduler calls it on the primary before it
// waits for the first sync cycle. It does nothing when ScratchDir is
// empty.
//
// The run holds the running latch, so it cannot remove the live file of
// a cycle that POST /sync started. When a cycle holds the latch, the
// sweep is skipped, and the stale files stay until the next start.
func (w *Worker) sweepScratchDirAtStartup(ctx context.Context) {
	if w.config.ScratchDir == "" {
		return
	}
	if !w.running.CompareAndSwap(false, true) {
		w.logger.LogAttrs(ctx, slog.LevelDebug, "sync cycle running, skipping startup scratch sweep")
		return
	}
	defer w.running.Store(false)
	sweepStaleScratchFiles(ctx, w.config.ScratchDir, w.logger)
}

// stageType streams a single PeeringDB type's full response into the
// scratch DB via StreamAll. The handler extracts the id field from each
// raw element and inserts (id, raw) into the scratch table. Uses a
// single prepared statement for the duration of the stream — each row
// insert allocates the minimum needed to bind the BLOB, then reuses the
// stmt for the next row. Peak Go heap is bounded to one handler
// invocation's buffer.
//
// All rows of one call go in one transaction. With one autocommit
// transaction for each row, SQLite took a file lock, checked for a hot
// journal and committed for every row: about 8 s of CPU in syscalls for
// the two stages of each type in a full cycle at production row counts.
// onFailure sets what a failed call does with the rows that it streamed
// before the error (see stageFailure).
//
// Errors wrap the objectType for operator diagnostics. Cursors derive
// from MAX(updated) per entity table (see cursor.go); the returned
// stageStats bound the follow-up window fetch after a full snapshot.
func (s *scratchDB) stageType(ctx context.Context, pdbClient *peeringdb.Client, objectType string, since time.Time, onFailure stageFailure) (stageStats, error) {
	// #nosec G201 — objectType is validated against the closed-set scratchTypes list
	// at schema creation time; SQL injection is not possible.
	insertSQL := fmt.Sprintf("INSERT OR REPLACE INTO %q (id, data) VALUES (?, ?)", objectType)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return stageStats{}, fmt.Errorf("begin scratch stage %s: %w: %w", objectType, errScratchDB, err)
	}
	// Rolls back a failed discardRows stage. After Commit it does nothing.
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return stageStats{}, fmt.Errorf("prepare scratch insert %s: %w: %w", objectType, errScratchDB, err)
	}
	defer func() { _ = stmt.Close() }()

	var stats stageStats
	handler := func(raw json.RawMessage) error {
		// Minimal decode to extract the primary key and the updated
		// timestamp. The full decode happens in Phase B at replay time,
		// not here, which keeps Go heap bounded to the raw bytes + this tiny
		// struct per element.
		var row struct {
			ID      int    `json:"id"`
			Updated string `json:"updated"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return fmt.Errorf("decode id from %s element: %w", objectType, err)
		}
		// Phase B owns updated validation; an unparseable value only
		// drops out of the maximum here.
		if updated, err := time.Parse(time.RFC3339, row.Updated); err == nil && updated.After(stats.maxUpdated) {
			stats.maxUpdated = updated
		}
		if _, err := stmt.ExecContext(ctx, row.ID, []byte(raw)); err != nil {
			return fmt.Errorf("insert scratch %s id=%d: %w: %w", objectType, row.ID, errScratchDB, err)
		}
		return nil
	}

	var opts []peeringdb.FetchOption
	if !since.IsZero() {
		opts = append(opts, peeringdb.WithSince(since))
	}
	meta, err := pdbClient.StreamAll(ctx, objectType, handler, opts...)
	if err != nil {
		err = fmt.Errorf("stream %s to scratch: %w", objectType, err)
		if onFailure == keepRows {
			if cerr := tx.Commit(); cerr != nil {
				err = errors.Join(err, fmt.Errorf("commit partial scratch stage %s: %w: %w", objectType, errScratchDB, cerr))
			}
		}
		return stageStats{}, err
	}
	if err := tx.Commit(); err != nil {
		return stageStats{}, fmt.Errorf("commit scratch stage %s: %w: %w", objectType, errScratchDB, err)
	}
	stats.generated = meta.Generated
	return stats, nil
}

// errScratchDB marks a stageType error from the scratch DB itself (begin,
// prepare, insert or commit), not from the upstream response. A caller
// that tolerates a failed upstream fetch must not tolerate this error: the
// rows that the failed write lost can include tombstones.
var errScratchDB = errors.New("scratch db")

// stageFailure sets what a failed stageType call does with the rows that
// it streamed before the error.
type stageFailure int

const (
	// discardRows rolls the rows back. The table keeps only the rows that
	// it had before the call.
	discardRows stageFailure = iota
	// keepRows commits the rows. Use it for a ?since= window on top of a
	// snapshot: each window row is a newer upstream version of its row,
	// so each one that is staged is an improvement.
	keepRows
)

// stageStats describes the response that one stageType call staged.
type stageStats struct {
	// generated is the response meta.generated time. Upstream sets it
	// only on responses served from its API cache, where it is the
	// cache file's write time. Zero when absent.
	generated time.Time
	// maxUpdated is the newest parseable updated timestamp among the
	// staged rows. Zero when no row carried one.
	maxUpdated time.Time
}

// scratchRow is a single (id, data) tuple drained from a scratch table
// during Phase B replay. Kept as a package-internal type rather than
// returning raw row scans so the replay loop reads cleanly.
type scratchRow struct {
	id  int
	raw json.RawMessage
}

// drainChunk reads up to chunkSize rows from the given scratch type,
// starting after the row id cursor afterID (exclusive). Returns the
// drained rows and the highest id seen in this chunk, which the caller
// passes back as afterID for the next call. Ordering by id ensures
// deterministic pagination and matches the PeeringDB ordering contract.
//
// Callers iterate until len(rows) < chunkSize, at which point the scratch
// table is fully drained. The chunk size bounds Go heap to approximately
// chunkSize × avg row size (~10 MB for chunkSize=5000 and avg row 2 KB).
func (s *scratchDB) drainChunk(ctx context.Context, objectType string, afterID int, chunkSize int) ([]scratchRow, int, error) {
	// #nosec G201 — objectType comes from the closed-set scratchTypes constant
	// (syncSteps()); no user input reaches the query builder.
	querySQL := fmt.Sprintf("SELECT id, data FROM %q WHERE id > ? ORDER BY id LIMIT ?", objectType)
	rows, err := s.db.QueryContext(ctx, querySQL, afterID, chunkSize)
	if err != nil {
		return nil, afterID, fmt.Errorf("query scratch %s: %w", objectType, err)
	}
	defer func() { _ = rows.Close() }()

	out := make([]scratchRow, 0, chunkSize)
	lastID := afterID
	for rows.Next() {
		var r scratchRow
		var blob []byte
		if err := rows.Scan(&r.id, &blob); err != nil {
			return nil, afterID, fmt.Errorf("scan scratch %s: %w", objectType, err)
		}
		r.raw = json.RawMessage(blob)
		out = append(out, r)
		lastID = r.id
	}
	if err := rows.Err(); err != nil {
		return nil, afterID, fmt.Errorf("iterate scratch %s: %w", objectType, err)
	}
	return out, lastID, nil
}
