// Package testutil provides shared test helpers for ent client setup.
package testutil

import (
	"database/sql"
	"fmt"
	"sync/atomic"
	"testing"

	"entgo.io/ent/dialect"
	"modernc.org/sqlite"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
)

func init() {
	sql.Register("sqlite3", &sqlite.Driver{})
}

// dbCounter provides unique database names for parallel tests.
var dbCounter atomic.Int64

// SetupClient creates an in-memory SQLite-backed ent client for testing.
// Each call gets an isolated database so parallel tests do not conflict.
// The client is automatically closed when the test completes via t.Cleanup.
func SetupClient(t *testing.T) *ent.Client {
	t.Helper()
	client, _ := SetupClientWithDB(t)
	return client
}

// SetupClientWithDB creates an in-memory SQLite-backed ent client for testing
// and also returns the underlying *sql.DB for raw SQL operations (e.g.
// sync_status table). Each call gets an isolated database so parallel tests
// do not conflict. Both the client and the DB are automatically closed via
// t.Cleanup.
func SetupClientWithDB(t *testing.T) (*ent.Client, *sql.DB) {
	t.Helper()
	id := dbCounter.Add(1)
	dsn := fmt.Sprintf("file:test_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)", id)
	client := enttest.Open(t,
		dialect.SQLite,
		dsn,
	)
	t.Cleanup(func() { _ = client.Close() })

	// Open a second connection to the same shared in-memory database for raw SQL.
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw sql.DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return client, db
}
