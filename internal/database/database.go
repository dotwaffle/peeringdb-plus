// Package database provides SQLite database setup for the ent ORM client.
// Uses modernc.org/sqlite (CGo-free) with WAL mode and FK constraints.
package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel/attribute"
	"modernc.org/sqlite"

	"github.com/dotwaffle/peeringdb-plus/ent"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

// Open creates an ent client connected to the SQLite database at dbPath.
// The database is configured with WAL journal mode, foreign key enforcement,
// a 5-second busy timeout, and bulk-write-tuned pragmas. It returns both the
// ent client and the underlying *sql.DB for raw SQL operations (e.g.,
// sync_status table).
//
// DSN pragma rationale:
//   - synchronous(NORMAL): safe under LiteFS-replicated WAL — LiteFS provides
//     durability via streaming replication so per-commit local fsync is
//     redundant overhead. Halves the per-commit syscall cost on the bulk-
//     upsert tx; the Fly primary's local WAL is replayed on replicas
//     regardless of the local fsync mode.
//   - cache_size(-8000): 8 MB page cache PER CONNECTION (negative value =
//     KiB; SQLite's default is ~2 MB). The page cache multiplies by the
//     pool: with MaxOpenConns=10 the worst case is 10 x cache_size, and
//     MaxIdleConns=5 retains up to 5 x cache_size for ConnMaxLifetime
//     after a read burst. The earlier -32000 (32 MB) was sized for the
//     primary's single-connection bulk-upsert burst but capped at 320 MB
//     pooled — more than an entire 256 MB replica VM serving the
//     read-heavy full-dump traffic that actually fills caches. 8 MB
//     bounds the pool at 80 MB worst-case while still quadrupling the
//     default for the upsert tx's page reuse.
//   - temp_store(MEMORY): keeps sorter and temp tables in RAM. modernc.org/
//     sqlite's default is FILE which on Fly hits the rootfs overlay (NOT
//     tmpfs — verified via /proc/mounts).
//
// When traceSQL is true the underlying *sql.DB is opened through XSAM/otelsql,
// so every query — ent's and the raw sync_status statements that share this
// handle — emits an OpenTelemetry span beneath the active request span.
// Controlled by PDBPLUS_OTEL_SQL (default on; set false to disable). Span
// volume is bounded by the trace sampler: a DB span follows the decision of
// its parent span. A sync cycle emits no DB spans (see
// pdbotel.WithoutDBSpans).
// The low-signal sql.rows and sql.conn.reset_session span types are suppressed
// (see otelOptions): they roughly halve the span count per request
// trace and remove the orphan single-span traces that pool-lifecycle and
// boot-time schema-migration DB operations otherwise emit with no request root.
// The DataVersionProbe query emits no span either (see keepQuerySpan).
func Open(dbPath string, traceSQL bool) (*ent.Client, *sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"+
			"&_pragma=synchronous(NORMAL)&_pragma=cache_size(-8000)&_pragma=temp_store(MEMORY)",
		dbPath,
	)
	var (
		db  *sql.DB
		err error
	)
	if traceSQL {
		db, err = otelsql.Open("sqlite3", dsn, otelOptions()...)
	} else {
		db, err = sql.Open("sqlite3", dsn)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("opening database %s: %w", dbPath, err)
	}

	// Configure connection pool for SQLite WAL mode.
	// Hardcoded infrastructure constants.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	drv := entsql.OpenDB(dialect.SQLite, db)
	return ent.NewClient(ent.Driver(drv)), db, nil
}

// otelOptions returns the otelsql options of the traced handle that Open
// returns.
func otelOptions() []otelsql.Option {
	return []otelsql.Option{
		otelsql.WithAttributes(attribute.String("db.system", "sqlite")),
		// Suppress the high-volume, low-signal span types: per-row
		// iteration (sql.rows) and connection-pool session resets
		// (sql.conn.reset_session). These roughly halve the DB-span count
		// on every request trace and eliminate the orphan single-span
		// traces that pool-lifecycle and boot-time (schema migration) DB
		// operations emit outside any request context. The query spans
		// (sql.conn.query, carrying db.query.text) are retained.
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			OmitRows:             true,
			OmitConnResetSession: true,
			SpanFilter:           keepQuerySpan,
		}),
	}
}

// keepQuerySpan is the otelsql span filter of Open. It drops the span of
// the DataVersionProbe query, which a caller runs about once a second
// outside any request: each span would be the root of its own trace. It
// also drops each span under a context from pdbotel.WithoutDBSpans, which
// marks a sync cycle.
func keepQuerySpan(ctx context.Context, _ otelsql.Method, query string, _ []driver.NamedValue) bool {
	return !pdbotel.DBSpansOff(ctx) && query != dataVersionQuery
}
