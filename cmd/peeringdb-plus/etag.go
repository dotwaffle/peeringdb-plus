package main

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/database"
	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

const (
	// etagPollInterval is how often each node reads its database version.
	// It bounds how long a node keeps answering 304 for an ETag after a
	// committed write becomes readable on that node.
	etagPollInterval = time.Second

	// etagCheckTimeout bounds the SQL reads of one poll: the data_version
	// probe and, until the first successful sync is seen, the sync_status
	// check. The LiteFS pos read is a local FUSE read that takes no
	// context, so it has no timeout.
	etagCheckTimeout = 2 * time.Second
)

// Values of the etagWatcher source, logged at startup.
const (
	etagSourceLiteFS = "litefs_pos"
	etagSourceSQLite = "sqlite_data_version"
)

// etagWatcher keeps the caching middleware's ETag in step with the local
// database. Served data changes only through committed writes: sync cycles
// on the primary, the LTX applies that replicate them to each replica, and
// writes outside a cycle such as the startup poc-contact scrub. The
// watcher runs on every node, reads a version that changes on each
// committed write, and swaps the ETag when the version changes.
//
// Until the node has seen a successful sync, the watcher keeps the ETag
// clear, so a node that starts before hydration sends no caching headers.
// A read that returns an error also clears the ETag, so a node never
// answers 304 for a version that it cannot confirm. A pos read that hangs
// blocks the poll, and the node keeps its last ETag until the read returns.
//
// poll and run are not safe for concurrent use; one goroutine owns the
// watcher.
type etagWatcher struct {
	source  string
	version func(context.Context) (string, error)
	synced  func(context.Context) (bool, error)
	closeFn func()
	state   *middleware.CachingState
	logger  *slog.Logger

	last     string // version of the last poll that read one; "" after a failure
	seenSync bool   // a success row was seen; the sync_status prune keeps the newest success row
	failing  bool   // a failure streak is in progress and was logged
}

// startETagWatcher starts the ETag watcher for the database at dbPath.
// main calls it on every node, whatever its role: a replica never runs the
// sync worker, so only the watcher changes its ETag. The first poll runs
// before startETagWatcher returns, so a warm restart serves cacheable
// responses at once. Later polls run every etagPollInterval until ctx is
// done.
func startETagWatcher(ctx context.Context, dbPath string, db *sql.DB, state *middleware.CachingState, logger *slog.Logger) {
	w := newETagWatcher(dbPath, db, state, logger)
	w.poll(ctx)
	go w.run(ctx, etagPollInterval)
}

// newETagWatcher returns a watcher for the database at dbPath. A LiteFS
// node reads the "<db>-pos" file. A node without LiteFS has no such file
// and uses a PRAGMA data_version probe on a pinned connection from db.
// Any stat result other than "does not exist" selects the pos file, so a
// faulty FUSE mount shows up as read failures that clear the ETag, not as
// a silent switch of source.
func newETagWatcher(dbPath string, db *sql.DB, state *middleware.CachingState, logger *slog.Logger) *etagWatcher {
	w := &etagWatcher{
		synced: func(ctx context.Context) (bool, error) {
			t, err := pdbsync.GetLastSuccessfulSyncTime(ctx, db)
			return !t.IsZero(), err
		},
		closeFn: func() {},
		state:   state,
		logger:  logger,
	}
	path := litefs.PosPath(dbPath)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		probe := database.NewDataVersionProbe(db)
		w.source = etagSourceSQLite
		w.version = probe.Version
		w.closeFn = func() { _ = probe.Close() }
		path = dbPath
	} else {
		w.source = etagSourceLiteFS
		w.version = func(context.Context) (string, error) { return litefs.ReadPos(path) }
	}
	logger.Info("etag version source", slog.String("source", w.source), slog.String("path", path))
	return w
}

// poll reads the database version once and updates the ETag if the
// version changed. In steady state it reads the version and compares one
// string.
func (w *etagWatcher) poll(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, etagCheckTimeout)
	defer cancel()

	v, err := w.version(checkCtx)
	if err != nil {
		w.fail(ctx, err)
		return
	}
	if v == w.last {
		w.recovered(ctx)
		return
	}
	if !w.seenSync {
		ok, err := w.synced(checkCtx)
		if err != nil {
			w.fail(ctx, err)
			return
		}
		if !ok {
			// Pre-sync: no caching headers. The commit of the first
			// success row changes the version, so the check runs again.
			w.state.Clear()
			w.last = v
			w.recovered(ctx)
			return
		}
		w.seenSync = true
	}
	w.state.SetVersion(v)
	w.last = v
	w.logger.LogAttrs(ctx, slog.LevelDebug, "etag updated",
		slog.String("source", w.source), slog.String("version", v))
	w.recovered(ctx)
}

// fail clears the ETag and logs the first failure of a streak. It also
// forgets the last version, so that the next successful read publishes the
// ETag again even if the version did not change. A failure caused by
// shutdown is not logged.
func (w *etagWatcher) fail(ctx context.Context, err error) {
	w.state.Clear()
	w.last = ""
	if w.failing || ctx.Err() != nil {
		return
	}
	w.failing = true
	w.logger.LogAttrs(ctx, slog.LevelWarn, "etag version check failed, caching headers off until it recovers",
		slog.String("source", w.source), slog.Any("error", err))
}

// recovered logs the end of a failure streak.
func (w *etagWatcher) recovered(ctx context.Context) {
	if !w.failing {
		return
	}
	w.failing = false
	w.logger.LogAttrs(ctx, slog.LevelInfo, "etag version check recovered",
		slog.String("source", w.source))
}

// run polls every interval until ctx is done, then releases the version
// source.
func (w *etagWatcher) run(ctx context.Context, interval time.Duration) {
	defer w.closeFn()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}
