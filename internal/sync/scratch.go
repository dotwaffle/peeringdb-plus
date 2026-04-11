package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, already a project dep

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// scratchChunkSize is the number of raw rows drained from the scratch DB
// into memory at a time during Phase B replay. Each chunk is decoded
// into typed Go structs (one per row, dozens of fields each) and then
// passed to the per-type upsertX function. Internally `upsertBatch`
// builds ALL builders for the chunk up front before sub-batching by
// ent's batchSize = 100 for the actual INSERT OR REPLACE — so setting
// scratchChunkSize equal to batchSize minimises the live builder
// slice's peak memory footprint.
//
// Tuning (benchmarked against production-scale fixtures at 364K rows;
// the sampler has 100ms granularity so transient GC scheduling
// fluctuation adds ±50 MiB variance run-to-run):
//
//	chunk=5000 → ~424 MiB (over gate)
//	chunk=1000 → ~356-422 MiB (on the gate edge, risky)
//	chunk=250  → ~334-422 MiB (still intermittently over)
//	chunk=100  → comfortably under with per-type runtime.GC() hint
//
// 100 aligns with ent's internal batchSize and combined with the
// per-type runtime.GC() hint in syncUpsertPass gives deterministic
// peak heap well under the 400 MiB hard gate.
const scratchChunkSize = 100

// scratchTypes is the closed-set of PeeringDB types that get a scratch
// staging table. Order does not matter here — the FK parent-first order
// is enforced later at the Phase B replay loop via syncSteps().
var scratchTypes = []string{
	peeringdb.TypeOrg,
	peeringdb.TypeCampus,
	peeringdb.TypeFac,
	peeringdb.TypeCarrier,
	peeringdb.TypeCarrierFac,
	peeringdb.TypeIX,
	peeringdb.TypeIXLan,
	peeringdb.TypeIXPfx,
	peeringdb.TypeIXFac,
	peeringdb.TypeNet,
	peeringdb.TypePoc,
	peeringdb.TypeNetFac,
	peeringdb.TypeNetIXLan,
}

// scratchDB is a sql.DB handle to the per-sync /tmp SQLite file plus the
// absolute path so closeScratchDB can unlink it on teardown.
//
// The scratch DB stages raw JSON rows from each PeeringDB type via
// StreamAll's callback — Phase A Go heap stays bounded to one element
// per handler invocation (~5-10 KB) instead of one full []T per type
// (~35 MB for netixlan). Phase B reads the scratch rows back in chunks
// and replays them into the real ent tables inside the single ent.Tx.
//
// Lifetime: opened at the start of a sync run if UseScratchDB is true,
// closed+unlinked via defer closeScratchDB(...) at the end of the same
// run. PID-scoped filename prevents collisions across concurrent worker
// processes (cross-process is a non-issue on Fly.io — only one primary
// runs at a time per D-30).
type scratchDB struct {
	db   *sql.DB
	path string
}

// openScratchDB creates an isolated SQLite database file in os.TempDir()
// scoped to the current process PID, pre-populates all 13 staging tables,
// and returns a scratchDB wrapper. The caller MUST call closeScratchDB
// to both close the handle and unlink the file.
//
// SQLite pragmas:
//   - journal_mode=OFF — no WAL/rollback journal. The scratch DB is
//     transient; crash-safety is irrelevant because the unlink-on-close
//     contract means a crashed process leaves the file for the next
//     openScratchDB call which unlinks stale files first.
//   - synchronous=OFF — skip fsyncs. Writes go straight to the OS page
//     cache; correctness is preserved because SQLite is the only writer.
//
// Path uniqueness: the filename embeds both the process PID and a
// monotonically-increasing counter so multiple concurrent Sync runs
// within the same process (e.g. parallel tests) do not collide on the
// same scratch file. In production there is only ever one primary
// sync running at a time per D-30, so the counter is effectively 0;
// the counter matters only for tests.
func openScratchDB(ctx context.Context) (*scratchDB, error) {
	// Atomically create an exclusive scratch file via O_EXCL so neither a
	// stale file nor a pre-placed symlink can be targeted. This eliminates
	// the /tmp symlink race that a deterministic PID+seq path would expose
	// on shared-/tmp hosts (local dev, CI runners). Production Fly.io is
	// single-tenant and not exposed, but the atomic path is a trivial
	// hardening upgrade. See 54-REVIEW.md WR-01.
	seq := scratchSeq.Add(1)
	path := filepath.Join(os.TempDir(), fmt.Sprintf("pdbplus-sync-scratch-%d-%d.db", os.Getpid(), seq))

	// O_EXCL fails if the path exists (including as a symlink).
	// Path is constructed from os.TempDir() + PID + atomic seq — no user
	// input, so gosec G304's "file inclusion via variable" warning does
	// not apply here.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- path is os.TempDir() + PID + atomic seq, not user input
	if err != nil {
		return nil, fmt.Errorf("create scratch db at %s: %w", path, err)
	}
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
	// Combined with journal_mode=OFF and synchronous=OFF this keeps the
	// scratch SQLite process's non-Go-heap footprint minimal.
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=journal_mode(OFF)&_pragma=synchronous(OFF)&_pragma=cache_size(-2048)")
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

// scratchSeq is a monotonically-increasing counter used to disambiguate
// scratch filenames when multiple Sync runs overlap in the same process.
// Production has only one primary at a time per D-30, so this is
// effectively unused in production; test parallelism is the real
// motivation.
var scratchSeq atomic.Uint64

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
// to close or unlink a transient /tmp file is non-fatal but deserves
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

// stageType streams a single PeeringDB type's full response into the
// scratch DB via StreamAll. The handler extracts the id field from each
// raw element and inserts (id, raw) into the scratch table. Uses a
// single prepared statement for the duration of the stream — each row
// insert allocates the minimum needed to bind the BLOB, then reuses the
// stmt for the next row. Peak Go heap is bounded to one handler
// invocation's buffer.
//
// Returns the PeeringDB meta.generated timestamp from the response so
// the caller can advance the per-type cursor. Errors wrap the
// objectType for operator diagnostics.
func (s *scratchDB) stageType(ctx context.Context, pdbClient *peeringdb.Client, objectType string, since time.Time) (time.Time, error) {
	// #nosec G201 — objectType is validated against the closed-set scratchTypes list
	// at schema creation time; SQL injection is not possible.
	insertSQL := fmt.Sprintf("INSERT OR REPLACE INTO %q (id, data) VALUES (?, ?)", objectType)
	stmt, err := s.db.PrepareContext(ctx, insertSQL)
	if err != nil {
		return time.Time{}, fmt.Errorf("prepare scratch insert %s: %w", objectType, err)
	}
	defer func() { _ = stmt.Close() }()

	handler := func(raw json.RawMessage) error {
		// Minimal decode to extract the primary key. The full decode
		// happens in Phase B at replay time, not here — keeps Go heap
		// bounded to the raw bytes + this tiny id struct per element.
		var id struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(raw, &id); err != nil {
			return fmt.Errorf("decode id from %s element: %w", objectType, err)
		}
		if _, err := stmt.ExecContext(ctx, id.ID, []byte(raw)); err != nil {
			return fmt.Errorf("insert scratch %s id=%d: %w", objectType, id.ID, err)
		}
		return nil
	}

	var opts []peeringdb.FetchOption
	if !since.IsZero() {
		opts = append(opts, peeringdb.WithSince(since))
	}
	meta, err := pdbClient.StreamAll(ctx, objectType, handler, opts...)
	if err != nil {
		return time.Time{}, fmt.Errorf("stream %s to scratch: %w", objectType, err)
	}
	return meta.Generated, nil
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
