package database

import (
	"path/filepath"
	"testing"
)

func TestOpen_Success(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	client, db, err := Open(dbPath, false)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", dbPath, err)
	}
	defer client.Close()
	defer db.Close()

	if client == nil {
		t.Error("client is nil")
	}
	if db == nil {
		t.Error("db is nil")
	}
}

func TestOpen_Pragmas(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	client, db, err := Open(dbPath, false)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", dbPath, err)
	}
	defer client.Close()
	defer db.Close()

	tests := []struct {
		name   string
		pragma string
		want   string
	}{
		{"journal_mode", "PRAGMA journal_mode", "wal"},
		{"foreign_keys", "PRAGMA foreign_keys", "1"},
		{"busy_timeout", "PRAGMA busy_timeout", "5000"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			if err := db.QueryRow(tt.pragma).Scan(&got); err != nil {
				t.Fatalf("QueryRow(%q): %v", tt.pragma, err)
			}
			if got != tt.want {
				t.Errorf("%s = %q, want %q", tt.pragma, got, tt.want)
			}
		})
	}
}

// TestOpen_TracedSQL verifies the otelsql-wrapped path (traceSQL=true) opens a
// working handle — the instrumentation is transparent to query execution.
// (Span emission is exercised live with PDBPLUS_OTEL_SQL=1, not here, to avoid
// mutating the global TracerProvider from a parallel test.)
func TestOpen_TracedSQL(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "traced.db")
	client, db, err := Open(dbPath, true)
	if err != nil {
		t.Fatalf("Open(%q, true) error: %v", dbPath, err)
	}
	defer client.Close()
	defer db.Close()

	var n int
	if err := db.QueryRow("SELECT 1").Scan(&n); err != nil {
		t.Fatalf("query on otelsql-wrapped DB: %v", err)
	}
	if n != 1 {
		t.Errorf("SELECT 1 = %d, want 1", n)
	}
}

func TestOpen_PoolConfig(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	client, db, err := Open(dbPath, false)
	if err != nil {
		t.Fatalf("Open(%q) error: %v", dbPath, err)
	}
	defer client.Close()
	defer db.Close()

	stats := db.Stats()
	if stats.MaxOpenConnections != 10 {
		t.Errorf("MaxOpenConnections = %d, want 10", stats.MaxOpenConnections)
	}
}
