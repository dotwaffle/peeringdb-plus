// Package database provides SQLite database setup for the ent ORM client.
// Uses modernc.org/sqlite (CGo-free) with WAL mode and FK constraints.
package database

import (
	"database/sql"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel/attribute"
	"modernc.org/sqlite"

	"github.com/dotwaffle/peeringdb-plus/ent"
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
//   - cache_size(-32000): 32 MB page cache (negative value = KiB). Default
//     is ~2 MB. The bulk-upsert workload reuses recently-read pages heavily
//     during the ~60s upsert burst; 32 MB fits comfortably under the Fly
//     512 MB primary VM cap (sync peak heap ~37 MB, peak RSS ~232 MB
//     observed in production).
//   - temp_store(MEMORY): keeps sorter and temp tables in RAM. modernc.org/
//     sqlite's default is FILE which on Fly hits the rootfs overlay (NOT
//     tmpfs — verified via /proc/mounts).
//
// When traceSQL is true the underlying *sql.DB is opened through XSAM/otelsql,
// so every query — ent's and the raw sync_status statements that share this
// handle — emits an OpenTelemetry span beneath the active request/sync span.
// Controlled by PDBPLUS_OTEL_SQL (default on; set false to disable). Span
// volume is bounded by the trace sampler: scheduled sync cycles are not traced,
// so the high-volume sync path produces no DB spans — see internal/otel sampler.
func Open(dbPath string, traceSQL bool) (*ent.Client, *sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"+
			"&_pragma=synchronous(NORMAL)&_pragma=cache_size(-32000)&_pragma=temp_store(MEMORY)",
		dbPath,
	)
	var (
		db  *sql.DB
		err error
	)
	if traceSQL {
		db, err = otelsql.Open("sqlite3", dsn,
			otelsql.WithAttributes(attribute.String("db.system", "sqlite")))
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
