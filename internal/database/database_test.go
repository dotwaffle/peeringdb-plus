package database

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/XSAM/otelsql"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
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

// TestOtelOptions_ProbeQueryHasNoSpan checks that a handle opened with
// Open's otelsql options emits no span for the DataVersionProbe query and
// still emits one for other queries. It passes its own TracerProvider, so
// it does not touch the global one.
func TestOtelOptions_ProbeQueryHasNoSpan(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	dsn := "file:" + filepath.Join(t.TempDir(), "probe.db")
	db, err := otelsql.Open("sqlite3", dsn, append(otelOptions(), otelsql.WithTracerProvider(tp))...)
	if err != nil {
		t.Fatalf("open traced db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	querySpans := func() int {
		n := 0
		for _, s := range rec.Ended() {
			if s.Name() == string(otelsql.MethodConnQuery) {
				n++
			}
		}
		return n
	}

	p := NewDataVersionProbe(db)
	t.Cleanup(func() { _ = p.Close() })
	for range 3 {
		if _, err := p.Version(ctx); err != nil {
			t.Fatalf("Version: %v", err)
		}
	}
	if n := querySpans(); n != 0 {
		t.Errorf("query spans after 3 probe reads = %d, want 0", n)
	}

	var one int
	if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if n := querySpans(); n != 1 {
		t.Errorf("query spans after SELECT 1 = %d, want 1", n)
	}
}

// TestOtelOptions_WithoutDBSpans checks that a handle opened with Open's
// otelsql options emits no span for a statement under a context from
// pdbotel.WithoutDBSpans: a query, an exec, and a transaction with its begin
// and commit. The same statements under a plain context emit spans.
func TestOtelOptions_WithoutDBSpans(t *testing.T) {
	t.Parallel()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	dsn := "file:" + filepath.Join(t.TempDir(), "nospans.db")
	db, err := otelsql.Open("sqlite3", dsn, append(otelOptions(), otelsql.WithTracerProvider(tp))...)
	if err != nil {
		t.Fatalf("open traced db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	run := func(ctx context.Context) {
		t.Helper()
		var one int
		if err := db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
			t.Fatalf("SELECT 1: %v", err)
		}
		if _, err := db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS t (v INTEGER)"); err != nil {
			t.Fatalf("create table: %v", err)
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO t (v) VALUES (1)"); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert: %v", err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}

	run(pdbotel.WithoutDBSpans(t.Context()))
	if n := len(rec.Ended()); n != 0 {
		names := make([]string, 0, n)
		for _, s := range rec.Ended() {
			names = append(names, s.Name())
		}
		t.Errorf("spans under WithoutDBSpans = %v, want none", names)
	}

	run(t.Context())
	if n := len(rec.Ended()); n == 0 {
		t.Error("no spans under a plain context: the handle does not trace")
	}
}
