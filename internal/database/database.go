// Package database provides SQLite database setup for the ent ORM client.
// Uses modernc.org/sqlite (CGo-free) with WAL mode and FK constraints.
package database

import (
	"database/sql"
	"fmt"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"modernc.org/sqlite"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

// Open creates an ent client connected to the SQLite database at dbPath.
// The database is configured with WAL journal mode, foreign key enforcement,
// and a 5-second busy timeout. It returns both the ent client and the
// underlying *sql.DB for raw SQL operations (e.g., sync_status table).
func Open(dbPath string) (*ent.Client, *sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
		dbPath,
	)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("opening database %s: %w", dbPath, err)
	}

	// Configure connection pool for SQLite WAL mode.
	// Hardcoded infrastructure constants per SRVR-02.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	drv := entsql.OpenDB(dialect.SQLite, db)
	return ent.NewClient(ent.Driver(drv)), db, nil
}
