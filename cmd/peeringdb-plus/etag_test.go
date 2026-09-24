package main

import (
	"context"
	"database/sql"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/database"
	"github.com/dotwaffle/peeringdb-plus/internal/litefs"
	"github.com/dotwaffle/peeringdb-plus/internal/middleware"
	pdbsync "github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// fakeETagSource is a version and sync_status stand-in for etagWatcher
// unit tests. The mutex lets a test change it while run() polls it.
type fakeETagSource struct {
	mu         sync.Mutex
	version    string
	versionErr error
	synced     bool
	syncedErr  error
	syncCalls  int
}

func (f *fakeETagSource) set(fn func(*fakeETagSource)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeETagSource) readVersion(context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.version, f.versionErr
}

func (f *fakeETagSource) readSynced(context.Context) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.syncCalls++
	return f.synced, f.syncedErr
}

func (f *fakeETagSource) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.syncCalls
}

// newFakeWatcher returns a watcher over src that writes to a new
// CachingState and logs to h.
func newFakeWatcher(src *fakeETagSource, h slog.Handler) *etagWatcher {
	return &etagWatcher{
		source:  "fake",
		version: src.readVersion,
		synced:  src.readSynced,
		closeFn: func() {},
		state:   middleware.NewCachingState(time.Hour),
		logger:  slog.New(h),
	}
}

// etagGet sends a GET through the caching middleware of state. It returns
// the response and whether the request reached the handler.
func etagGet(state *middleware.CachingState, ifNoneMatch string) (*httptest.ResponseRecorder, bool) {
	called := false
	h := state.Middleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/net", nil)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, called
}

// currentETag returns the ETag that state sends now, or "" if it sends no
// caching headers.
func currentETag(t *testing.T, state *middleware.CachingState) string {
	t.Helper()
	rec, _ := etagGet(state, "")
	etag := rec.Header().Get("ETag")
	if (etag == "") != (rec.Header().Get("Cache-Control") == "") {
		t.Fatalf("ETag = %q but Cache-Control = %q; want both or neither",
			etag, rec.Header().Get("Cache-Control"))
	}
	return etag
}

// countLogs returns how many records at level with message msg h holds.
func countLogs(h *captureHandler, level slog.Level, msg string) int {
	n := 0
	for _, r := range h.snapshot() {
		if r.Level == level && r.Message == msg {
			n++
		}
	}
	return n
}

const (
	etagFailMsg      = "etag version check failed, caching headers off until it recovers"
	etagRecoveredMsg = "etag version check recovered"
)

func TestETagWatcher_UnsyncedSendsNoHeaders(t *testing.T) {
	t.Parallel()
	src := &fakeETagSource{version: "A"}
	w := newFakeWatcher(src, &captureHandler{})

	w.poll(t.Context())
	if got := currentETag(t, w.state); got != "" {
		t.Errorf("ETag = %q before the first successful sync, want none", got)
	}
}

func TestETagWatcher_SyncedSetsETag(t *testing.T) {
	t.Parallel()
	src := &fakeETagSource{version: "A", synced: true}
	w := newFakeWatcher(src, &captureHandler{})

	w.poll(t.Context())
	if got := currentETag(t, w.state); got == "" {
		t.Error("no ETag after the first poll of a synced node")
	}
}

// TestETagWatcher_RunFollowsVersionChange drives run() on a fake clock: one
// poll interval after the version changes, a client that revalidates with
// the old ETag gets the full body and the new ETag.
func TestETagWatcher_RunFollowsVersionChange(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		src := &fakeETagSource{version: "A", synced: true}
		w := newFakeWatcher(src, &captureHandler{})

		w.poll(ctx)
		oldETag := currentETag(t, w.state)
		if oldETag == "" {
			t.Fatal("no ETag after the first poll")
		}
		go w.run(ctx, etagPollInterval)

		src.set(func(f *fakeETagSource) { f.version = "B" })
		time.Sleep(etagPollInterval)
		synctest.Wait()

		rec, called := etagGet(w.state, oldETag)
		if rec.Code != http.StatusOK || !called {
			t.Fatalf("revalidation with the old ETag: status %d, handler called %v; want 200 from the handler",
				rec.Code, called)
		}
		if got := rec.Header().Get("ETag"); got == "" || got == oldETag {
			t.Errorf("ETag = %q after the version change, want a new one (old %q)", got, oldETag)
		}
	})
}

// TestETagWatcher_VersionErrorClearsAndRecovers checks the failure path:
// an unreadable version drops the caching headers at once, a failure
// streak logs one WARN, and a recovery that reads the SAME version
// publishes the ETag again.
func TestETagWatcher_VersionErrorClearsAndRecovers(t *testing.T) {
	t.Parallel()
	h := &captureHandler{}
	src := &fakeETagSource{version: "A", synced: true}
	w := newFakeWatcher(src, h)
	ctx := t.Context()

	w.poll(ctx)
	want := currentETag(t, w.state)
	if want == "" {
		t.Fatal("no ETag after the first poll")
	}

	src.set(func(f *fakeETagSource) { f.versionErr = errors.New("pos read failed") })
	for range 3 {
		w.poll(ctx)
	}
	rec, called := etagGet(w.state, "*")
	if !called || rec.Code != http.StatusOK {
		t.Errorf("If-None-Match: * while failing: status %d, handler called %v; want 200 from the handler",
			rec.Code, called)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Errorf("ETag = %q while failing, want none", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "" {
		t.Errorf("Cache-Control = %q while failing, want none", got)
	}
	if n := countLogs(h, slog.LevelWarn, etagFailMsg); n != 1 {
		t.Errorf("WARN count after 3 failures = %d, want 1", n)
	}

	src.set(func(f *fakeETagSource) { f.versionErr = nil })
	w.poll(ctx)
	if got := currentETag(t, w.state); got != want {
		t.Errorf("ETag after recovery = %q, want %q (same version)", got, want)
	}
	if n := countLogs(h, slog.LevelInfo, etagRecoveredMsg); n != 1 {
		t.Errorf("recovery INFO count = %d, want 1", n)
	}
	w.poll(ctx)
	if n := countLogs(h, slog.LevelInfo, etagRecoveredMsg); n != 1 {
		t.Errorf("recovery INFO count after a healthy poll = %d, want 1", n)
	}
}

// TestETagWatcher_SyncedCheckedUntilLatched checks that sync_status is read
// only on a version change and only until a successful sync is seen.
func TestETagWatcher_SyncedCheckedUntilLatched(t *testing.T) {
	t.Parallel()
	src := &fakeETagSource{version: "A"}
	w := newFakeWatcher(src, &captureHandler{})
	ctx := t.Context()

	w.poll(ctx)
	w.poll(ctx) // same version: no check
	if n := src.calls(); n != 1 {
		t.Fatalf("synced calls = %d, want 1", n)
	}

	src.set(func(f *fakeETagSource) { f.version, f.synced = "B", true })
	w.poll(ctx)
	if n := src.calls(); n != 2 {
		t.Fatalf("synced calls = %d, want 2", n)
	}
	if currentETag(t, w.state) == "" {
		t.Fatal("no ETag after the success row was seen")
	}

	src.set(func(f *fakeETagSource) { f.version = "C" })
	w.poll(ctx)
	if n := src.calls(); n != 2 {
		t.Errorf("synced calls after the latch = %d, want 2", n)
	}
}

func TestETagWatcher_SyncedErrorRetried(t *testing.T) {
	t.Parallel()
	h := &captureHandler{}
	src := &fakeETagSource{version: "A", syncedErr: errors.New("database is locked")}
	w := newFakeWatcher(src, h)
	ctx := t.Context()

	w.poll(ctx)
	if got := currentETag(t, w.state); got != "" {
		t.Errorf("ETag = %q after a sync_status error, want none", got)
	}
	if n := countLogs(h, slog.LevelWarn, etagFailMsg); n != 1 {
		t.Errorf("WARN count = %d, want 1", n)
	}

	src.set(func(f *fakeETagSource) { f.syncedErr, f.synced = nil, true })
	w.poll(ctx) // same version: the check must run again
	if n := src.calls(); n != 2 {
		t.Errorf("synced calls = %d, want 2 (retry on the next poll)", n)
	}
	if got := currentETag(t, w.state); got == "" {
		t.Error("no ETag after the sync_status check recovered")
	}
}

func TestETagWatcher_CancelStopsRun(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		src := &fakeETagSource{version: "A", synced: true}
		w := newFakeWatcher(src, &captureHandler{})
		closed := make(chan struct{})
		w.closeFn = func() { close(closed) }
		done := make(chan struct{})
		go func() {
			w.run(ctx, etagPollInterval)
			close(done)
		}()

		time.Sleep(3 * etagPollInterval)
		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("run did not return after ctx was canceled")
		}
		select {
		case <-closed:
		default:
			t.Error("run did not call closeFn")
		}
	})
}

// openETagDB opens a WAL-mode database file under dir with the sync_status
// table and one table that stands in for other served data.
func openETagDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	_, db, err := database.Open(path, false)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := t.Context()
	if err := pdbsync.InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS served (id INTEGER PRIMARY KEY, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("create served table: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO served (id, name) VALUES (1, 'contact')`); err != nil {
		t.Fatalf("seed served table: %v", err)
	}
	return db
}

// recordSuccessfulSync writes a sync_status success row the way the sync
// worker does.
func recordSuccessfulSync(t *testing.T, db *sql.DB) {
	t.Helper()
	ctx := t.Context()
	now := time.Now()
	id, err := pdbsync.RecordSyncStart(ctx, db, now, "incremental")
	if err != nil {
		t.Fatalf("record sync start: %v", err)
	}
	if err := pdbsync.RecordSyncComplete(ctx, db, id, pdbsync.Status{
		LastSyncAt: now,
		Status:     "success",
	}); err != nil {
		t.Fatalf("record sync complete: %v", err)
	}
}

// newDBWatcher builds a production watcher for the database at path. It
// fails the test if the watcher did not select wantSource.
func newDBWatcher(t *testing.T, path string, db *sql.DB, wantSource string) *etagWatcher {
	t.Helper()
	w := newETagWatcher(path, db, middleware.NewCachingState(time.Hour), discardLogger())
	t.Cleanup(w.closeFn)
	if w.source != wantSource {
		t.Fatalf("source = %q, want %q", w.source, wantSource)
	}
	return w
}

// TestETagWatcher_FollowsOutOfCycleWrite covers a write that no sync cycle
// makes, such as the startup poc-contact scrub: the ETag must change, and
// a client that revalidates with the old ETag must get the new body.
func TestETagWatcher_FollowsOutOfCycleWrite(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "etag.db")
	db := openETagDB(t, path)
	w := newDBWatcher(t, path, db, etagSourceSQLite)

	recordSuccessfulSync(t, db)
	w.poll(ctx)
	e1 := currentETag(t, w.state)
	if e1 == "" {
		t.Fatal("no ETag after a successful sync")
	}

	if _, err := db.ExecContext(ctx, `UPDATE served SET name = '' WHERE id = 1`); err != nil {
		t.Fatalf("out-of-cycle update: %v", err)
	}
	w.poll(ctx)
	e2 := currentETag(t, w.state)
	if e2 == "" || e2 == e1 {
		t.Fatalf("ETag after an out-of-cycle write = %q, want a new one (was %q)", e2, e1)
	}
	rec, called := etagGet(w.state, e1)
	if rec.Code != http.StatusOK || !called {
		t.Errorf("revalidation with the old ETag: status %d, handler called %v; want 200 from the handler",
			rec.Code, called)
	}

	// An UPDATE that matches no rows commits no pages, so the ETag stays.
	if _, err := db.ExecContext(ctx, `UPDATE served SET name = 'x' WHERE id = -1`); err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	w.poll(ctx)
	if got := currentETag(t, w.state); got != e2 {
		t.Errorf("ETag after a no-op UPDATE = %q, want %q", got, e2)
	}
	if rec, _ := etagGet(w.state, e2); rec.Code != http.StatusNotModified {
		t.Errorf("revalidation with the current ETag: status %d, want 304", rec.Code)
	}
}

// TestETagWatcher_ReplicaFollowsReplicatedSync models a replica: its
// watcher reads through its own handle and never writes, and it started
// before the first sync. Another handle on the same file stands in for
// the primary whose commits LiteFS replicates. The replica must pick up
// the first sync and each later one with no restart.
func TestETagWatcher_ReplicaFollowsReplicatedSync(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "etag.db")
	primary := openETagDB(t, path)
	_, replica, err := database.Open(path, false)
	if err != nil {
		t.Fatalf("open replica handle: %v", err)
	}
	t.Cleanup(func() { _ = replica.Close() })
	w := newDBWatcher(t, path, replica, etagSourceSQLite)

	w.poll(ctx)
	if got := currentETag(t, w.state); got != "" {
		t.Fatalf("ETag = %q before hydration, want none", got)
	}

	recordSuccessfulSync(t, primary)
	w.poll(ctx)
	e1 := currentETag(t, w.state)
	if e1 == "" {
		t.Fatal("replica has no ETag after the first replicated sync")
	}

	recordSuccessfulSync(t, primary)
	w.poll(ctx)
	e2 := currentETag(t, w.state)
	if e2 == "" || e2 == e1 {
		t.Fatalf("replica ETag after a second sync = %q, want a new one (was %q)", e2, e1)
	}
	if rec, called := etagGet(w.state, e1); rec.Code != http.StatusOK || !called {
		t.Errorf("replica revalidation with the old ETag: status %d, handler called %v; want 200 from the handler",
			rec.Code, called)
	}
}

// TestETagWatcher_LiteFSPos checks that a node with a "<db>-pos" file keys
// the ETag on it and follows a TXID change.
func TestETagWatcher_LiteFSPos(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "etag.db")
	db := openETagDB(t, path)
	recordSuccessfulSync(t, db)
	writePos := func(pos string) {
		t.Helper()
		if err := os.WriteFile(litefs.PosPath(path), []byte(pos), 0o600); err != nil {
			t.Fatalf("write pos file: %v", err)
		}
	}

	writePos("0000000000000001/00000000deadbeef\n")
	w := newDBWatcher(t, path, db, etagSourceLiteFS)
	w.poll(ctx)
	e1 := currentETag(t, w.state)
	if e1 == "" {
		t.Fatal("no ETag from the pos file")
	}

	w.poll(ctx)
	if got := currentETag(t, w.state); got != e1 {
		t.Errorf("ETag changed with no pos change: %q -> %q", e1, got)
	}

	writePos("0000000000000002/00000000cafef00d\n")
	w.poll(ctx)
	if got := currentETag(t, w.state); got == "" || got == e1 {
		t.Errorf("ETag after TXID 1 -> 2 = %q, want a new one (was %q)", got, e1)
	}
}

func TestETagWatcher_SelectsDataVersionWithoutPos(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "etag.db")
	db := openETagDB(t, path)
	h := &captureHandler{}
	w := newETagWatcher(path, db, middleware.NewCachingState(time.Hour), slog.New(h))
	t.Cleanup(w.closeFn)
	if w.source != etagSourceSQLite {
		t.Errorf("source = %q, want %q", w.source, etagSourceSQLite)
	}
	if n := countLogs(h, slog.LevelInfo, "etag version source"); n != 1 {
		t.Errorf("startup source log count = %d, want 1", n)
	}
}

// TestStartETagWatcher_WarmStartAndFollow checks the helper that main calls
// on every node. It takes no role input: the ETag must be set when it
// returns, and one poll interval after a committed write a client that
// revalidates with the old ETag must get the full response.
func TestStartETagWatcher_WarmStartAndFollow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		path := filepath.Join(t.TempDir(), "etag.db")
		db := openETagDB(t, path)
		recordSuccessfulSync(t, db)
		state := middleware.NewCachingState(time.Hour)

		startETagWatcher(ctx, path, db, state, discardLogger())
		e1 := currentETag(t, state)
		if e1 == "" {
			t.Fatal("no ETag when startETagWatcher returned; the first poll must run before it returns")
		}

		if _, err := db.ExecContext(ctx, `UPDATE served SET name = '' WHERE id = 1`); err != nil {
			t.Fatalf("update: %v", err)
		}
		time.Sleep(etagPollInterval)
		synctest.Wait()
		rec, called := etagGet(state, e1)
		if rec.Code != http.StatusOK || !called {
			t.Errorf("revalidation with the old ETag: status %d, handler called %v; want 200 from the handler",
				rec.Code, called)
		}
		if got := rec.Header().Get("ETag"); got == "" || got == e1 {
			t.Errorf("ETag = %q after a write, want a new one (old %q)", got, e1)
		}

		// Stop the watcher so that it releases its connection before the
		// database closes.
		cancel()
		synctest.Wait()
	})
}

// TestMain_StartsETagWatcherUnconditionally is a source-scan regression
// lock for the replica ETag fix. main must call startETagWatcher exactly
// once, as a statement of its own body, so that no role check (such as
// isPrimary or a startup policy flag) can gate it and the first poll stays
// synchronous. A replica never runs the sync worker, so without the
// watcher its ETag never changes.
func TestMain_StartsETagWatcherUnconditionally(t *testing.T) {
	t.Parallel()
	f, err := parser.ParseFile(token.NewFileSet(), "main.go", nil, 0)
	if err != nil {
		t.Fatalf("parse main.go: %v", err)
	}
	isStart := func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := call.Fun.(*ast.Ident)
		return ok && id.Name == "startETagWatcher"
	}

	calls := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if isStart(n) {
			calls++
		}
		return true
	})
	if calls != 1 {
		t.Fatalf("main.go calls startETagWatcher %d times, want 1", calls)
	}

	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != "main" {
			continue
		}
		for _, stmt := range fn.Body.List {
			if es, ok := stmt.(*ast.ExprStmt); ok && isStart(es.X) {
				return
			}
		}
		t.Fatal("startETagWatcher is not a statement of main's body; a call inside a condition, closure, go or defer statement can leave a node with a frozen ETag")
	}
	t.Fatal("func main not found in main.go")
}
