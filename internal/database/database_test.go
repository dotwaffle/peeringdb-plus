package database

import (
	"path/filepath"
	"testing"
)

func TestOpen_Success(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	client, db, err := Open(dbPath)
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
	client, db, err := Open(dbPath)
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

func TestOpen_PoolConfig(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	client, db, err := Open(dbPath)
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
