package sync

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	stdsync "sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"modernc.org/sqlite"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
)

// lockRetryMsg is the log message of a retry in retryOnLock.
const lockRetryMsg = "retrying write after sqlite lock error"

// testLockRetry is a lock retry policy with short waits for tests.
var testLockRetry = LockRetry{Attempts: 4, BaseDelay: time.Millisecond}

// sqliteErr returns a *sqlite.Error with result code code and message
// msg: the error type that modernc.org/sqlite returns. The type has no
// exported constructor, so the helper sets its unexported fields through
// reflect. When a driver upgrade renames the fields, the helper panics.
// TestIsLockError checks real driver errors next to these.
func sqliteErr(code int, msg string) error {
	e := &sqlite.Error{}
	v := reflect.ValueOf(e).Elem()
	for name, val := range map[string]any{"code": code, "msg": msg} {
		f := v.FieldByName(name)
		reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem().Set(reflect.ValueOf(val))
	}
	return e
}

// errProtocol is the SQLite error that failed a startup cascade commit on
// the LiteFS primary.
func errProtocol() error { return sqliteErr(15, "locking protocol (15)") }

// errBusy is a plain SQLITE_BUSY error.
func errBusy() error { return sqliteErr(5, "database is locked (5) (SQLITE_BUSY)") }

// openFileDB opens a file database in dir with the production pragmas,
// except that busy_timeout is 0 and journal is the journal mode.
func openFileDB(t *testing.T, path, journal string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=journal_mode("+journal+")"+
		"&_pragma=busy_timeout(0)&_pragma=synchronous(NORMAL)")
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// mustExec runs query on conn and fails the test on error.
func mustExec(t *testing.T, conn *sql.Conn, query string) {
	t.Helper()
	if _, err := conn.ExecContext(context.Background(), query); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// realLockErrors returns errors that the driver returned for real
// conflicts, by name.
func realLockErrors(t *testing.T) map[string]error {
	t.Helper()
	ctx := t.Context()
	out := map[string]error{}

	// Two connections want the WAL write lock: SQLITE_BUSY.
	path := filepath.Join(t.TempDir(), "busy.db")
	a, b := openFileDB(t, path, "WAL"), openFileDB(t, path, "WAL")
	if _, err := a.ExecContext(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	ac, err := a.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ac.Close() }()
	mustExec(t, ac, "BEGIN IMMEDIATE")
	_, out["busy"] = b.ExecContext(ctx, "INSERT INTO t (id) VALUES (1)")
	mustExec(t, ac, "ROLLBACK")

	// A read transaction that another connection wrote past cannot
	// write: SQLITE_BUSY_SNAPSHOT.
	mustExec(t, ac, "BEGIN")
	var n int
	if err := ac.QueryRowContext(ctx, "SELECT count(*) FROM t").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ExecContext(ctx, "INSERT INTO t (id) VALUES (2)"); err != nil {
		t.Fatal(err)
	}
	_, out["busy_snapshot"] = ac.ExecContext(ctx, "INSERT INTO t (id) VALUES (3)")
	mustExec(t, ac, "ROLLBACK")

	// A deferred foreign key fails the COMMIT.
	if _, err := a.ExecContext(ctx, "CREATE TABLE c (id INTEGER PRIMARY KEY, t_id INTEGER NOT NULL REFERENCES t(id))"); err != nil {
		t.Fatal(err)
	}
	tx, err := a.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "PRAGMA defer_foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO c (id, t_id) VALUES (1, 999)"); err != nil {
		t.Fatal(err)
	}
	out["fk_commit"] = tx.Commit()

	// A connection drops a table that its own open query reads:
	// SQLITE_LOCKED. (The driver waits out a shared-cache table lock
	// with sqlite3_unlock_notify, so that conflict returns no error.)
	rows, err := ac.QueryContext(ctx, "SELECT id FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatalf("no row to hold the query open: %v", rows.Err())
	}
	_, out["locked"] = ac.ExecContext(ctx, "DROP TABLE t")
	_ = rows.Close()
	return out
}

// TestIsLockError verifies that SQLITE_BUSY with its extended codes and
// SQLITE_PROTOCOL are lock errors, wrapped or not, and that SQLITE_LOCKED,
// other codes and plain errors are not. The first cases are errors that
// the driver returned for real conflicts.
func TestIsLockError(t *testing.T) {
	t.Parallel()
	driverErrs := realLockErrors(t)
	for name, wantCode := range map[string]int{
		"busy": 5, "busy_snapshot": 517, "fk_commit": 787, "locked": 6,
	} {
		var se *sqlite.Error
		if !errors.As(driverErrs[name], &se) || se.Code() != wantCode {
			t.Fatalf("real %s error = %v, want a *sqlite.Error with code %d", name, driverErrs[name], wantCode)
		}
	}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"real busy", driverErrs["busy"], true},
		{"real busy snapshot", driverErrs["busy_snapshot"], true},
		{"real deferred fk at commit", driverErrs["fk_commit"], false},
		{"real locked", driverErrs["locked"], false},
		{"protocol", errProtocol(), true},
		{"busy", errBusy(), true},
		{"busy recovery", sqliteErr(261, "database is locked (261)"), true},
		{"busy timeout", sqliteErr(773, "database is locked (773)"), true},
		{"locked", sqliteErr(6, "database table is locked (6)"), false},
		{"sql logic error", sqliteErr(1, "SQL logic error (1)"), false},
		{"wrapped protocol", fmt.Errorf("commit poc scrub transaction: %w", errProtocol()), true},
		{"joined busy", errors.Join(errors.New("scrub"), errBusy()), true},
		{"plain text", errors.New("database is locked (5)"), false},
		{"injected commit", errInjectedCommit, false},
		{"canceled", context.Canceled, false},
		{"nil", nil, false},
	} {
		if got := isLockError(tc.err); got != tc.want {
			t.Errorf("%s: isLockError(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

// newLogger returns a JSON logger at DEBUG that writes to a new buffer.
func newLogger() (*slog.Logger, *logBuffer) {
	logs := &logBuffer{}
	return slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), logs
}

// retryRecords returns the retry log records of op, and fails the test
// when a record has another op.
func retryRecords(t *testing.T, logs *logBuffer, op string) []map[string]any {
	t.Helper()
	recs := logs.records(t, lockRetryMsg)
	for _, r := range recs {
		if r["op"] != op || r["level"] != "WARN" {
			t.Errorf("retry record = %v, want a WARN with op %q", r, op)
		}
	}
	return recs
}

// TestRetryOnLock verifies the retry loop: a lock error calls fn again up
// to the attempt limit with doubling waits and one WARN per retry, and
// the result of the last call returns. Another error and a success
// return at once.
func TestRetryOnLock(t *testing.T) {
	t.Parallel()
	other := sqliteErr(787, "FOREIGN KEY constraint failed (787)")
	locked := sqliteErr(6, "database table is locked (6)")
	for _, tc := range []struct {
		name       string
		attempts   int
		errs       []error // error of each call, nil past the end
		wantCalls  int
		wantErr    error
		wantDelays []string
	}{
		{"first call succeeds", 4, nil, 1, nil, nil},
		{"succeeds after two lock errors", 4, []error{errProtocol(), errBusy()}, 3, nil, []string{"1ms", "2ms"}},
		{"gives up after the attempt limit", 3, []error{errProtocol(), errBusy(), errProtocol(), errBusy()}, 3, errProtocol(), []string{"1ms", "2ms"}},
		{"other error returns at once", 4, []error{other}, 1, other, nil},
		{"sqlite locked returns at once", 4, []error{locked}, 1, locked, nil},
		{"lock error after a lock error returns other", 4, []error{errBusy(), other}, 2, other, []string{"1ms"}},
		{"attempts below one make one call", 0, []error{errProtocol()}, 1, errProtocol(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			logger, logs := newLogger()
			calls := 0
			v, err := retryOnLock(t.Context(), logger, LockRetry{Attempts: tc.attempts, BaseDelay: time.Millisecond}, "test_op",
				func(context.Context) (int, error) {
					calls++
					if calls <= len(tc.errs) && tc.errs[calls-1] != nil {
						return -1, tc.errs[calls-1]
					}
					return calls, nil
				})
			if calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.wantErr == nil {
				if err != nil || v != tc.wantCalls {
					t.Errorf("result = %d, %v; want %d, nil", v, err, tc.wantCalls)
				}
			} else if err == nil || err.Error() != tc.wantErr.Error() {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
			recs := retryRecords(t, logs, "test_op")
			var delays []string
			for i, r := range recs {
				delays = append(delays, fmt.Sprint(r["delay"]))
				if r["attempt"] != float64(i+1) || r["error"] == nil {
					t.Errorf("retry record %d = %v, want attempt %d with the error", i, r, i+1)
				}
			}
			// The JSON handler writes a duration in nanoseconds.
			var want []string
			for _, d := range tc.wantDelays {
				pd, _ := time.ParseDuration(d)
				want = append(want, fmt.Sprint(float64(pd)))
			}
			if !slices.Equal(delays, want) {
				t.Errorf("retry delays = %v, want %v", delays, tc.wantDelays)
			}
		})
	}
}

// TestRetryOnLock_ContextStopsWait verifies that a done context ends the
// wait before a retry: the call returns at once with the lock error and
// the cause of the context, and fn is not called again.
func TestRetryOnLock_ContextStopsWait(t *testing.T) {
	t.Parallel()
	logger, logs := newLogger()
	ctx, cancel := context.WithCancelCause(t.Context())
	stop := errors.New("shutting down")
	calls := 0
	start := time.Now()
	_, err := retryOnLock(ctx, logger, LockRetry{Attempts: 4, BaseDelay: time.Hour}, "test_op",
		func(context.Context) (struct{}, error) {
			calls++
			cancel(stop)
			return struct{}{}, errProtocol()
		})
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if !errors.Is(err, stop) || !isLockError(err) {
		t.Errorf("error = %v, want the lock error and the context cause", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("returned after %v, want at once", elapsed)
	}
	if recs := retryRecords(t, logs, "test_op"); len(recs) != 1 {
		t.Errorf("retry records = %d, want 1", len(recs))
	}
}

// TestRetryOnLock_CountsRetries verifies that each retry adds 1 to
// pdbplus.sync.lock_retries{op}.
// Not parallel: rebinds the package-level metric instruments.
func TestRetryOnLock_CountsRetries(t *testing.T) {
	reader := setupMetricTest(t)
	logger, _ := newLogger()
	calls := 0
	if _, err := retryOnLock(t.Context(), logger, testLockRetry, opStartupPocScrub,
		func(context.Context) (int, error) {
			calls++
			if calls < 3 {
				return 0, errProtocol()
			}
			return 0, nil
		}); err != nil {
		t.Fatalf("retryOnLock: %v", err)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatalf("collect: %v", err)
	}
	m := findMetric(rm, "pdbplus.sync.lock_retries")
	if m == nil {
		t.Fatal("pdbplus.sync.lock_retries not found")
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("pdbplus.sync.lock_retries is %T, want Sum[int64]", m.Data)
	}
	got := map[string]int64{}
	for _, dp := range sum.DataPoints {
		op, _ := dp.Attributes.Value("op")
		got[op.AsString()] = dp.Value
	}
	if want := map[string]int64{opStartupPocScrub: 2}; !maps.Equal(got, want) {
		t.Errorf("lock retries = %v, want %v", got, want)
	}
}

// TestScrubPocContactsAtStartup_RetriesLockErrors verifies that the
// startup scrub runs its transaction again when the commit fails with a
// lock error, and gives up after the attempt limit. Another error and a
// done context end the run after one commit. Each failed run logs one
// failure WARN and keeps the contact data.
func TestScrubPocContactsAtStartup_RetriesLockErrors(t *testing.T) {
	t.Parallel()
	const failedMsg = "startup poc contact scrub failed, the next sync cycle retries it"
	jane := [4]string{"Jane Doe", "+1 555 0100", "jane@example.invalid", "https://example.invalid/jane"}
	for _, tc := range []struct {
		name        string
		errs        []error
		cancel      bool
		wantCommits int
		wantRetries int
		wantErrText string // "" when the scrub commits
	}{
		{"committed after two lock errors", []error{errProtocol(), errBusy()}, false, 3, 2, ""},
		{"gives up after the attempt limit", []error{errProtocol(), errProtocol(), errProtocol(), errProtocol()}, false, 4, 3, "locking protocol (15)"},
		{"other error is not retried", []error{sqliteErr(787, "FOREIGN KEY constraint failed (787)")}, false, 1, 0, "FOREIGN KEY constraint failed"},
		{"done context stops the retry", []error{errProtocol()}, true, 1, 1, "context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			w, db := newTestWorker(t, f)
			probe := withCommitProbe(w, db)
			w.lockRetry = testLockRetry
			seedLegacyPocs(t, w.entClient)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				w.lockRetry.BaseDelay = time.Hour
				probe.onQueued = cancel
			}
			probe.failCommits(tc.errs...)

			w.scrubPocContactsAtStartup(ctx)

			if got := probe.commitCalls(); got != tc.wantCommits {
				t.Errorf("commits = %d, want %d", got, tc.wantCommits)
			}
			if recs := retryRecords(t, probe.logs, opStartupPocScrub); len(recs) != tc.wantRetries {
				t.Errorf("retry records = %d, want %d", len(recs), tc.wantRetries)
			}
			failed := probe.logs.records(t, failedMsg)
			if tc.wantErrText == "" {
				if len(failed) != 0 {
					t.Errorf("failure log = %v, want none", failed)
				}
				if got := pocContactSQL(t, db, 10); got != [4]string{} {
					t.Errorf("poc 10 contact = %q, want blank", got)
				}
				recs := probe.logs.records(t, scrubLogMsg)
				if len(recs) != 1 || recs[0]["count"] != float64(2) {
					t.Errorf("scrub log = %v, want one record with count 2", recs)
				}
			} else {
				if len(failed) != 1 || !strings.Contains(fmt.Sprint(failed[0]["error"]), tc.wantErrText) {
					t.Errorf("failure log = %v, want one record with %q", failed, tc.wantErrText)
				}
				if got := pocContactSQL(t, db, 10); got != jane {
					t.Errorf("poc 10 contact = %q, want %q", got, jane)
				}
			}
			if w.Running() {
				t.Error("running latch still held after the startup scrub")
			}
		})
	}
}

// TestCascadeNetIxLansAtStartup_RetriesLockErrors verifies that the
// startup cascade runs its transaction again when the commit fails with a
// lock error, and gives up after the attempt limit. Verification runs
// once for all attempts.
func TestCascadeNetIxLansAtStartup_RetriesLockErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		errs        []error
		wantCommits int
		wantDone    bool
	}{
		{"committed after two lock errors", []error{errProtocol(), errProtocol()}, 3, true},
		{"gives up after the attempt limit", []error{errProtocol(), errBusy(), errProtocol(), errBusy()}, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			up := newCascadeUpstream(t)
			w, db, _ := newCascadeWorker(t, up, 20)
			probe := withCommitProbe(w, db)
			w.lockRetry = testLockRetry
			seedCascadeRows(t, w.entClient)
			want := allNetIxLans(t, w.entClient)
			if tc.wantDone {
				for _, id := range []int{9110, 9111, 9120} {
					want[id] = cascadedRow
				}
			}
			probe.failCommits(tc.errs...)

			w.cascadeNetIxLansAtStartup(t.Context())

			// The verify memo would also keep a second pass off the wire,
			// so count the passes by their log line.
			if calls := up.requestCount(); calls != 1 {
				t.Errorf("upstream requests = %d, want 1", calls)
			}
			if recs := probe.logs.records(t, "verified netixlan cascade candidates"); len(recs) != 1 {
				t.Errorf("verification passes = %d, want 1 (verification must not repeat)", len(recs))
			}
			if got := probe.commitCalls(); got != tc.wantCommits {
				t.Errorf("commits = %d, want %d", got, tc.wantCommits)
			}
			if recs := retryRecords(t, probe.logs, opStartupNetIxLanCascade); len(recs) != tc.wantCommits-1 {
				t.Errorf("retry records = %d, want %d", len(recs), tc.wantCommits-1)
			}
			if got := allNetIxLans(t, w.entClient); !maps.Equal(got, want) {
				t.Errorf("rows after the startup cascade:\n got %v\nwant %v", got, want)
			}
			failed := probe.logs.records(t, startupCascadeFailedMsg)
			if tc.wantDone {
				if len(failed) != 0 {
					t.Errorf("failure log = %v, want none", failed)
				}
				wantCascadeLog(t, probe.logs, "WARN", "startup", 3, 2, 3)
			} else {
				if len(failed) != 1 {
					t.Errorf("failure log = %v, want one record", failed)
				}
				wantNoCascadeChangeLog(t, probe.logs)
			}
		})
	}
}

// execFaults fails the Exec calls of a faultConn whose query, with its
// white space collapsed, starts with prefix. Each such call takes the
// next queued error. Past the queue, the call runs.
type execFaults struct {
	prefix string

	mu     stdsync.Mutex
	queued []error
	calls  int    // matching calls
	onFail func() // called before a matching call returns a queued error
}

// take counts query when it matches and returns its queued error, if any.
func (f *execFaults) take(query string) error {
	if !strings.HasPrefix(strings.Join(strings.Fields(query), " "), f.prefix) {
		return nil
	}
	f.mu.Lock()
	f.calls++
	var err error
	if len(f.queued) > 0 {
		err, f.queued = f.queued[0], f.queued[1:]
	}
	hook := f.onFail
	f.mu.Unlock()
	if err != nil && hook != nil {
		hook()
	}
	return err
}

// matchingCalls returns the number of Exec calls that matched prefix.
func (f *execFaults) matchingCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// sqliteConn is the set of driver interfaces that a modernc.org/sqlite
// connection implements and that faultConn passes on.
type sqliteConn interface {
	driver.Conn
	driver.ConnBeginTx
	driver.ConnPrepareContext
	driver.ExecerContext
	driver.QueryerContext
	driver.SessionResetter
	driver.Validator
}

// faultConn is a modernc.org/sqlite connection whose ExecContext first
// asks faults for an error. A queued error returns before the statement
// runs, as when SQLite rolls back a failed autocommit statement.
type faultConn struct {
	inner  sqliteConn
	faults *execFaults
}

// The methods below pass the call on to the modernc.org/sqlite
// connection. Only ExecContext adds behavior.

func (c *faultConn) Prepare(query string) (driver.Stmt, error) { return c.inner.Prepare(query) }
func (c *faultConn) Close() error                              { return c.inner.Close() }
func (c *faultConn) Begin() (driver.Tx, error)                 { return c.inner.Begin() }

func (c *faultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.inner.BeginTx(ctx, opts)
}

func (c *faultConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	return c.inner.PrepareContext(ctx, query)
}

func (c *faultConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := c.faults.take(query); err != nil {
		return nil, err
	}
	return c.inner.ExecContext(ctx, query, args)
}

func (c *faultConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.inner.QueryContext(ctx, query, args)
}

func (c *faultConn) ResetSession(ctx context.Context) error { return c.inner.ResetSession(ctx) }
func (c *faultConn) IsValid() bool                          { return c.inner.IsValid() }

// faultConnector opens faultConns on dsn.
type faultConnector struct {
	dsn    string
	faults *execFaults
}

func (c faultConnector) Connect(context.Context) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(c.dsn)
	if err != nil {
		return nil, err
	}
	sc, ok := conn.(sqliteConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("sqlite connection is %T, missing a driver interface", conn)
	}
	return &faultConn{inner: sc, faults: c.faults}, nil
}

func (c faultConnector) Driver() driver.Driver { return &sqlite.Driver{} }

// faultDBCounter names the in-memory databases of newFaultDB.
var faultDBCounter atomic.Int64

// newFaultDB returns an ent client and a *sql.DB over one new in-memory
// database with the sync_status table. Exec calls on the *sql.DB go
// through faults.
func newFaultDB(t *testing.T, faults *execFaults) (*ent.Client, *sql.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:lockfault_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", faultDBCounter.Add(1))
	client := enttest.Open(t, dialect.SQLite, dsn)
	t.Cleanup(func() { _ = client.Close() })
	db := sql.OpenDB(faultConnector{dsn: dsn, faults: faults})
	t.Cleanup(func() { _ = db.Close() })
	if err := InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init status table: %v", err)
	}
	return client, db
}

// lastSyncStatus returns the status of the newest sync_status row, or ""
// when the table is empty.
func lastSyncStatus(t *testing.T, db *sql.DB) string {
	t.Helper()
	var status string
	err := db.QueryRowContext(t.Context(), "SELECT status FROM sync_status ORDER BY id DESC LIMIT 1").Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatalf("read sync_status: %v", err)
	}
	return status
}

// TestSync_StatusWritesRetryLockErrors verifies that a sync cycle retries
// its sync_status INSERT and its completion UPDATE, on success and on
// failure, when they fail with a lock error. After the attempt limit, or
// on another error, the cycle logs the existing ERROR and goes on.
func TestSync_StatusWritesRetryLockErrors(t *testing.T) {
	t.Parallel()
	const (
		insertPrefix   = "INSERT INTO sync_status"
		completePrefix = "UPDATE sync_status SET completed_at"
	)
	for _, tc := range []struct {
		name        string
		prefix      string
		errs        []error
		failFetch   bool
		wantOp      string
		wantCalls   int
		wantStatus  string // of the newest row, "" for no row
		wantFailLog string // final-failure ERROR message, "" for none
	}{
		{"start retried", insertPrefix, []error{errProtocol(), errBusy()}, false,
			opRecordSyncStart, 3, "success", ""},
		{"start gives up", insertPrefix, []error{errProtocol(), errProtocol(), errProtocol(), errProtocol()}, false,
			opRecordSyncStart, 4, "", "failed to record sync start"},
		{"start other error", insertPrefix, []error{sqliteErr(1, "SQL logic error (1)")}, false,
			opRecordSyncStart, 1, "", "failed to record sync start"},
		{"completion retried", completePrefix, []error{errBusy()}, false,
			opRecordSyncComplete, 2, "success", ""},
		{"completion gives up", completePrefix, []error{errProtocol(), errProtocol(), errProtocol(), errProtocol()}, false,
			opRecordSyncComplete, 4, "running", "failed to record sync completion"},
		{"failure completion retried", completePrefix, []error{errProtocol(), errBusy()}, true,
			opRecordSyncComplete, 3, "failed", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			f.responses["org"] = []any{makeOrg(1, "Org1", "ok")}
			if tc.failFetch {
				f.failTypes["org"] = true
			}
			faults := &execFaults{prefix: tc.prefix, queued: tc.errs}
			client, db := newFaultDB(t, faults)
			logger, logs := newLogger()
			w := NewWorker(newFastPDBClient(t, f.server.URL), client, db, WorkerConfig{}, logger)
			w.lockRetry = testLockRetry

			err := w.Sync(t.Context(), config.SyncModeFull)
			if (err != nil) != tc.failFetch {
				t.Fatalf("sync error = %v, want failure %v", err, tc.failFetch)
			}

			if got := faults.matchingCalls(); got != tc.wantCalls {
				t.Errorf("matching Exec calls = %d, want %d", got, tc.wantCalls)
			}
			wantRetries := tc.wantCalls - 1
			if tc.wantFailLog != "" && tc.wantCalls == 1 {
				wantRetries = 0
			}
			if recs := retryRecords(t, logs, tc.wantOp); len(recs) != wantRetries {
				t.Errorf("retry records = %d, want %d", len(recs), wantRetries)
			}
			if got := lastSyncStatus(t, db); got != tc.wantStatus {
				t.Errorf("newest sync_status = %q, want %q", got, tc.wantStatus)
			}
			for _, msg := range []string{"failed to record sync start", "failed to record sync completion"} {
				want := 0
				if msg == tc.wantFailLog {
					want = 1
				}
				if recs := logs.records(t, msg); len(recs) != want {
					t.Errorf("%q records = %v, want %d", msg, recs, want)
				}
			}
		})
	}
}

// TestReapStaleRunningRows_RetriesLockErrors verifies that the startup
// reap retries its UPDATE on a lock error, gives up after the attempt
// limit, does not retry another error, and stops when its context is
// done.
func TestReapStaleRunningRows_RetriesLockErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		errs       []error
		cancel     bool
		wantCalls  int
		wantReaped int
		wantErr    string // "" for success
	}{
		{"reaped after two lock errors", []error{errBusy(), errProtocol()}, false, 3, 1, ""},
		{"gives up after the attempt limit", []error{errBusy(), errBusy(), errBusy(), errBusy()}, false, 4, 0, "database is locked"},
		{"other error is not retried", []error{sqliteErr(1, "SQL logic error (1)")}, false, 1, 0, "SQL logic error"},
		{"done context stops the retry", []error{errProtocol()}, true, 1, 0, "context canceled"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			faults := &execFaults{prefix: "UPDATE sync_status SET status = 'failed'", queued: tc.errs}
			_, db := newFaultDB(t, faults)
			if _, err := db.ExecContext(t.Context(),
				`INSERT INTO sync_status (started_at, status) VALUES (?, 'running')`, time.Now()); err != nil {
				t.Fatalf("seed running row: %v", err)
			}
			logger, logs := newLogger()
			retry := testLockRetry
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancel {
				retry.BaseDelay = time.Hour
				faults.onFail = cancel
			}

			reaped, err := ReapStaleRunningRows(ctx, db, logger, retry)

			if tc.wantErr == "" && err != nil {
				t.Fatalf("ReapStaleRunningRows: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			if reaped != tc.wantReaped {
				t.Errorf("reaped = %d, want %d", reaped, tc.wantReaped)
			}
			if got := faults.matchingCalls(); got != tc.wantCalls {
				t.Errorf("matching Exec calls = %d, want %d", got, tc.wantCalls)
			}
			wantRetries := tc.wantCalls - 1
			if tc.cancel {
				wantRetries = 1
			}
			if recs := retryRecords(t, logs, opReapStaleRunningRows); len(recs) != wantRetries {
				t.Errorf("retry records = %d, want %d", len(recs), wantRetries)
			}
			wantStatus := "failed"
			if tc.wantReaped == 0 {
				wantStatus = "running"
			}
			if got := lastSyncStatus(t, db); got != wantStatus {
				t.Errorf("row status = %q, want %q", got, wantStatus)
			}
		})
	}
}

// TestScrubPocContactsAtStartup_RetriesRealLock verifies the retry
// against real SQLite locks on a file database whose pool holds one
// connection, so each attempt reuses the connection of the failed one.
// In WAL mode another connection holds the write lock. The UPDATE fails
// with SQLITE_BUSY, and the scrub rolls back. In rollback-journal mode a
// reader holds a shared lock. The COMMIT fails with SQLITE_BUSY, and the
// driver rolls back. The test releases the lock after the first retry. The next attempt can only succeed when the failed attempt left
// no open transaction on the connection.
func TestScrubPocContactsAtStartup_RetriesRealLock(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		journal string
		hold    []string // statements that take the lock on the holder
	}{
		{"WAL", []string{"BEGIN IMMEDIATE"}},
		{"DELETE", []string{"BEGIN", "SELECT count(*) FROM pocs"}},
	} {
		t.Run(tc.journal, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			path := filepath.Join(t.TempDir(), "pdb.db")
			db := openFileDB(t, path, tc.journal)
			db.SetMaxOpenConns(1)
			// enttest migrates a copy of the schema tables, so parallel
			// tests do not race on the package-level tables.
			client := enttest.NewClient(t, enttest.WithOptions(ent.Driver(entsql.OpenDB(dialect.SQLite, db))))
			seedLegacyPocs(t, client)
			logger, logs := newLogger()
			w := NewWorker(nil, client, db, WorkerConfig{}, logger)
			w.lockRetry = LockRetry{Attempts: 10, BaseDelay: 10 * time.Millisecond}

			holder, err := openFileDB(t, path, tc.journal).Conn(ctx)
			if err != nil {
				t.Fatalf("holder conn: %v", err)
			}
			defer func() { _ = holder.Close() }()
			for _, q := range tc.hold {
				if _, err := holder.ExecContext(ctx, q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}

			done := make(chan struct{})
			go func() {
				defer close(done)
				w.scrubPocContactsAtStartup(ctx)
			}()
			deadline := time.Now().Add(10 * time.Second)
			for len(logs.records(t, lockRetryMsg)) == 0 {
				if time.Now().After(deadline) {
					t.Fatal("no retry before the deadline")
				}
				time.Sleep(time.Millisecond)
			}
			if _, err := holder.ExecContext(ctx, "ROLLBACK"); err != nil {
				t.Fatalf("release the lock: %v", err)
			}
			<-done

			recs := retryRecords(t, logs, opStartupPocScrub)
			for _, r := range recs {
				if !strings.Contains(fmt.Sprint(r["error"]), "database is locked (5)") {
					t.Errorf("retry record = %v, want SQLITE_BUSY", r)
				}
			}
			if failed := logs.records(t, "startup poc contact scrub failed, the next sync cycle retries it"); len(failed) != 0 {
				t.Errorf("failure log = %v, want none", failed)
			}
			if got := pocContactSQL(t, db, 10); got != [4]string{} {
				t.Errorf("poc 10 contact = %q, want blank", got)
			}
		})
	}
}
