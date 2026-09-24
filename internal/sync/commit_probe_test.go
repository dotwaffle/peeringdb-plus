package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	stdsync "sync"
	"sync/atomic"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// errInjectedCommit is the error of a commit that commitProbe fails. The
// text is the SQLite error that failed a startup cascade commit in prod.
// It is a plain error, not a modernc.org/sqlite error, so retryOnLock
// does not retry it. Use failCommits to inject a lock error.
var errInjectedCommit = errors.New("injected commit failure: locking protocol (15)")

// commitSeamDriver is an ent SQL driver whose transactions end with commit
// in place of a plain COMMIT. commit gets the open transaction and must
// commit it or roll it back.
type commitSeamDriver struct {
	*entsql.Driver
	commit func(dialect.Tx) error
}

// Tx starts a transaction whose Commit calls d.commit.
func (d commitSeamDriver) Tx(ctx context.Context) (dialect.Tx, error) {
	tx, err := d.Driver.Tx(ctx)
	if err != nil {
		return nil, err
	}
	sqlTx, ok := tx.(*entsql.Tx)
	if !ok {
		_ = tx.Rollback()
		return nil, fmt.Errorf("transaction is %T, want *sql.Tx", tx)
	}
	return commitSeamTx{Tx: sqlTx, commit: d.commit}, nil
}

// commitSeamTx is a transaction of commitSeamDriver.
type commitSeamTx struct {
	*entsql.Tx
	commit func(dialect.Tx) error
}

// Commit calls the commit function of the driver with the real
// transaction.
func (tx commitSeamTx) Commit() error { return tx.commit(tx.Tx) }

// commitProbe ends the transactions of a test worker (see
// commitSeamDriver). A commit first takes the next error that
// failCommits queued: it rolls back and returns that error. Else, while
// fail is set, it rolls back and returns errInjectedCommit. Otherwise it
// stores a copy of the worker log before it commits, so a test can see
// what the worker logged before its last commit.
type commitProbe struct {
	logs *logBuffer
	fail atomic.Bool

	mu       stdsync.Mutex
	atCommit *logBuffer
	queued   []error // errors of the next commits, in order
	onQueued func()  // called before a commit returns a queued error
	commits  int     // commit calls
}

// failCommits makes the next len(errs) commits roll back and return errs,
// in order.
func (p *commitProbe) failCommits(errs ...error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queued = append(p.queued, errs...)
}

// commitCalls returns the number of commit calls.
func (p *commitProbe) commitCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.commits
}

// commit ends tx as the queued errors and the probe's fail flag select.
func (p *commitProbe) commit(tx dialect.Tx) error {
	p.mu.Lock()
	p.commits++
	var queued error
	if len(p.queued) > 0 {
		queued, p.queued = p.queued[0], p.queued[1:]
	}
	hook := p.onQueued
	p.mu.Unlock()
	if queued != nil {
		if hook != nil {
			hook()
		}
		if err := tx.Rollback(); err != nil {
			return errors.Join(queued, err)
		}
		return queued
	}
	if p.fail.Load() {
		if err := tx.Rollback(); err != nil {
			return errors.Join(errInjectedCommit, err)
		}
		return errInjectedCommit
	}
	snap := p.logs.snapshot()
	p.mu.Lock()
	p.atCommit = snap
	p.mu.Unlock()
	return tx.Commit()
}

// logsAtCommit returns the worker log as it was just before the last
// successful commit, or an empty log when no commit succeeded.
func (p *commitProbe) logsAtCommit() *logBuffer {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.atCommit == nil {
		return &logBuffer{}
	}
	return p.atCommit
}

// withCommitProbe gives w an ent client over db whose transactions end
// through the returned probe, and a JSON logger at DEBUG that writes to
// the probe's log. db must hold the schema.
func withCommitProbe(w *Worker, db *sql.DB) *commitProbe {
	p := &commitProbe{logs: &logBuffer{}}
	w.logger = slog.New(slog.NewJSONHandler(p.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	w.entClient = ent.NewClient(ent.Driver(commitSeamDriver{
		Driver: entsql.OpenDB(dialect.SQLite, db),
		commit: p.commit,
	}))
	return p
}
