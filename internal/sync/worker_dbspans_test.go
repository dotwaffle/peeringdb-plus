package sync

import (
	"context"
	"database/sql/driver"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/XSAM/otelsql"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/migrate"
	"github.com/dotwaffle/peeringdb-plus/internal/config"
	pdbotel "github.com/dotwaffle/peeringdb-plus/internal/otel"
)

// TestSync_EmitsNoDBSpans locks that a sync cycle marks its context with
// pdbotel.WithoutDBSpans, so the otelsql handle emits no DB span for the
// cycle, while the step spans stay. A full cycle runs thousands of
// statements, and with their spans its trace is larger than the per-trace
// limit of the trace backend. The handle uses a span filter on
// pdbotel.DBSpansOff, like the one of internal/database. That package is
// not linked here: it registers the sqlite3 driver, as internal/testutil
// does.
//
// Not parallel: it sets the global TracerProvider, which the sync tracer and
// otelsql read.
func TestSync_EmitsNoDBSpans(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	ctx := t.Context()
	dsn := "file:" + filepath.Join(t.TempDir(), "sync.db") + "?_pragma=foreign_keys(1)"
	db, err := otelsql.Open("sqlite3", dsn,
		otelsql.WithTracerProvider(tp),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			SpanFilter: func(ctx context.Context, _ otelsql.Method, _ string, _ []driver.NamedValue) bool {
				return !pdbotel.DBSpansOff(ctx)
			},
		}),
	)
	if err != nil {
		t.Fatalf("open traced db: %v", err)
	}
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.SQLite, db)))
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx, migrate.WithDropColumn(true), migrate.WithDropIndex(true)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if err := InitStatusTable(ctx, db); err != nil {
		t.Fatalf("init status table: %v", err)
	}

	dbSpans := func(spans []sdktrace.ReadOnlySpan) int {
		n := 0
		for _, s := range spans {
			if strings.HasPrefix(s.Name(), "sql.") {
				n++
			}
		}
		return n
	}
	before := rec.Ended()
	if dbSpans(before) == 0 {
		t.Fatal("no DB span from the schema setup: the handle does not trace")
	}

	f := newFixture(t)
	f.responses["org"] = []any{makeOrg(1, "TestOrg", "ok")}
	w := NewWorker(newFastPDBClient(t, f.server.URL), client, db, WorkerConfig{}, slog.Default())
	if err := w.Sync(ctx, config.SyncModeFull); err != nil {
		t.Fatalf("sync: %v", err)
	}

	cycle := rec.Ended()[len(before):]
	var names []string
	for _, s := range cycle {
		names = append(names, s.Name())
	}
	for _, want := range []string{"sync-full", "sync-upsert-org", "sync-commit"} {
		if !slices.Contains(names, want) {
			t.Errorf("cycle has no %s span; spans = %v", want, names)
		}
	}
	if n := dbSpans(cycle); n != 0 {
		t.Errorf("cycle emitted %d DB spans, want 0; spans = %v", n, names)
	}
}
