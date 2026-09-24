package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// The op values of the short writes that retryOnLock retries. They label
// the retry log and the pdbplus.sync.lock_retries counter.
const (
	opRecordSyncStart        = "record_sync_start"
	opRecordSyncComplete     = "record_sync_complete"
	opReapStaleRunningRows   = "reap_stale_running_rows"
	opStartupPocScrub        = "startup_poc_scrub"
	opStartupNetIxLanCascade = "startup_netixlan_cascade"
)

// LockRetry is the retry policy of a short write that can fail on a
// transient SQLite lock error (see isLockError).
type LockRetry struct {
	// Attempts is the maximum number of attempts, the first one
	// included. A value below 1 means 1.
	Attempts int
	// BaseDelay is the wait before the second attempt. Each later wait
	// is double the one before it.
	BaseDelay time.Duration
}

// DefaultLockRetry returns the policy of the primary's short writes: 4
// attempts, with waits of 250ms, 500ms and 1s between them.
func DefaultLockRetry() LockRetry {
	return LockRetry{Attempts: 4, BaseDelay: 250 * time.Millisecond}
}

// isLockError reports whether err holds a modernc.org/sqlite error whose
// primary result code is SQLITE_BUSY (5) or SQLITE_PROTOCOL (15). The
// driver enables extended result codes, so the check masks the code with
// 0xff: SQLITE_BUSY_SNAPSHOT (517), SQLITE_BUSY_RECOVERY (261) and
// SQLITE_BUSY_TIMEOUT (773) are SQLITE_BUSY.
//
// Both codes report a lock that SQLite could not get at that moment.
// SQLite returns SQLITE_PROTOCOL when it cannot complete its file locking
// protocol, for example after it loses many WAL lock races. On the LiteFS
// primary a COMMIT failed with it. busy_timeout does not wait for
// SQLITE_PROTOCOL or for SQLITE_BUSY_SNAPSHOT.
//
// SQLITE_LOCKED (6) is not a lock error here. It reports a conflict in the
// same connection, or between connections that share a cache. Production
// opens the database without a shared cache, so a conflict with another
// connection is SQLITE_BUSY. A conflict in the same connection is a
// program error, and a retry gets the same error.
func isLockError(err error) bool {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return false
	}
	switch se.Code() & 0xff {
	case sqlite3.SQLITE_BUSY, sqlite3.SQLITE_PROTOCOL:
		return true
	}
	return false
}

// retryOnLock calls fn and calls it again when it fails with a lock error
// (see isLockError), up to p.Attempts calls in total. Before each retry it
// logs one WARN with op, the attempt that failed and its error, adds 1 to
// pdbplus.sync.lock_retries{op}, and waits: p.BaseDelay before the second
// call, and double the previous wait before each later call. It returns
// the result of the last call. Another error returns at once, and so does
// the last attempt, so the caller logs the final failure.
//
// When ctx is done during a wait, retryOnLock stops and returns the last
// lock error with the cause of ctx.
//
// fn must be safe to call again after it fails: it must write in one
// transaction, or in one autocommit statement, that it starts itself. A
// failed attempt then leaves no open transaction on its pooled
// connection. After a failed statement in a transaction, fn rolls back.
// When a COMMIT fails and SQLite keeps the transaction open,
// modernc.org/sqlite rolls it back before database/sql returns the
// connection to the pool. SQLite rolls back a failed autocommit statement
// itself. So a retry starts on a clean connection.
//
// Use it only for short writes. The sync transaction does not retry: its
// fetch pass is expensive, and the next cycle does the work again. The
// sync_status prune does not retry either: the next cycle runs it again.
func retryOnLock[T any](ctx context.Context, logger *slog.Logger, p LockRetry, op string, fn func(context.Context) (T, error)) (T, error) {
	attempts := max(p.Attempts, 1)
	delay := p.BaseDelay
	for attempt := 1; ; attempt++ {
		v, err := fn(ctx)
		if err == nil || attempt >= attempts || !isLockError(err) {
			return v, err
		}
		logger.LogAttrs(ctx, slog.LevelWarn, "retrying write after sqlite lock error",
			slog.String("op", op),
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", attempts),
			slog.Duration("delay", delay),
			slog.Any("error", err),
		)
		pdbotel.SyncLockRetries.Add(ctx, 1, metric.WithAttributes(attribute.String("op", op)))
		if waitErr := sleepCtx(ctx, delay); waitErr != nil {
			var zero T
			return zero, fmt.Errorf("%w (retry stopped: %w)", err, waitErr)
		}
		delay *= 2
	}
}

// sleepCtx waits for d, or until ctx is done. It returns the cause of ctx
// when ctx is done first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-t.C:
		return nil
	}
}
