package database

import (
	"context"
	"path/filepath"
	"testing"
)

// TestDataVersionProbe drives a probe against a real WAL-mode database
// file and checks which writes change the key.
func TestDataVersionProbe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	client, db, err := Open(filepath.Join(t.TempDir(), "dv.db"), false)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})
	if _, err := db.ExecContext(ctx, `CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	p := NewDataVersionProbe(db)
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	version := func() string {
		t.Helper()
		v, err := p.Version(ctx)
		if err != nil {
			t.Fatalf("Version: %v", err)
		}
		return v
	}

	k1 := version()
	if k2 := version(); k2 != k1 {
		t.Fatalf("key changed with no writes: %q -> %q", k1, k2)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO t (v) VALUES ('a')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	k3 := version()
	if k3 == k1 {
		t.Fatalf("key %q did not change after a committed INSERT", k3)
	}

	res, err := db.ExecContext(ctx, `UPDATE t SET v = 'b' WHERE id = -1`)
	if err != nil {
		t.Fatalf("no-op update: %v", err)
	}
	if n, _ := res.RowsAffected(); n != 0 {
		t.Fatalf("no-op update affected %d rows, want 0", n)
	}
	if k4 := version(); k4 != k3 {
		t.Errorf("key changed after an UPDATE that matched no rows: %q -> %q", k3, k4)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	k5 := version()
	for _, earlier := range []string{k1, k3} {
		if k5 == earlier {
			t.Errorf("key after Close = %q, same as an earlier key", k5)
		}
	}
}
