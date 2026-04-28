package health_test

import (
	"bytes"
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/health"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	_ "modernc.org/sqlite"
)

// testLogHandler is a slog.Handler test double that records every log record
// passed to Handle. It is concurrency-safe for parallel test execution.
type testLogHandler struct {
	mu      stdsync.Mutex
	records []slog.Record
}

func (h *testLogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *testLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *testLogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *testLogHandler) WithGroup(_ string) slog.Handler      { return h }

// snapshot returns a copy of the records captured so far.
func (h *testLogHandler) snapshot() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(h.records))
	copy(out, h.records)
	return out
}

// collectReadinessResponse invokes the readiness handler against in and returns
// the observed HTTP status, the wire body (trimmed), and the log records that
// the handler wrote to the injected logger.
func collectReadinessResponse(t *testing.T, in health.ReadinessInput) (int, string, []slog.Record) {
	t.Helper()

	logCap := &testLogHandler{}
	in.Logger = slog.New(logCap)

	handler := health.ReadinessHandler(in)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	handler.ServeHTTP(rec, req)

	body := strings.TrimSpace(rec.Body.String())
	return rec.Code, body, logCap.snapshot()
}

// findAttr walks a slog.Record looking for an attribute with key k. It returns
// the matching attr and true if found. Used by the log-shape assertions below.
func findAttr(r slog.Record, k string) (slog.Attr, bool) {
	var found slog.Attr
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == k {
			found = a
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func TestLivenessHandler(t *testing.T) {
	t.Parallel()

	handler := health.LivenessHandler()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	got := strings.TrimSpace(rec.Body.String())
	if got != `{"status":"ok"}` {
		t.Errorf("body = %q, want %q", got, `{"status":"ok"}`)
	}
}

// TestHealth_GenericResponse is the SEC-08 regression lock. It asserts that
// the /readyz wire body is a fixed generic shape and that all detail flows to
// the structured logger via slog attrs. Consumers MUST NOT see err.Error(),
// sync.Status.ErrorMessage, or any internal file paths/driver messages.
func TestHealth_GenericResponse(t *testing.T) {
	t.Parallel()

	staleThreshold := 24 * time.Hour

	tests := []struct {
		name       string
		setupDB    func(t *testing.T) *sql.DB
		wantStatus int
		wantBody   string
		// logAssert runs against the captured records. Returning a non-empty
		// string signals a failure with the returned message.
		logAssert func(t *testing.T, records []slog.Record) string
	}{
		{
			name: "healthy_all_ok",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertSync(t, db, time.Now().Add(-1*time.Hour), "success", "")
				return db
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
			logAssert: func(_ *testing.T, records []slog.Record) string {
				// Healthy path MUST NOT emit any error-level records.
				for _, r := range records {
					if r.Level >= slog.LevelError {
						return "unexpected error log on healthy path"
					}
				}
				return ""
			},
		},
		{
			name: "db_ping_fails",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				db.Close() // force PingContext to fail
				return db
			},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unhealthy"}`,
			logAssert: func(_ *testing.T, records []slog.Record) string {
				// Exactly one error record with component=db and a non-empty
				// error attr.
				var hits int
				for _, r := range records {
					if r.Level != slog.LevelError {
						continue
					}
					comp, ok := findAttr(r, "component")
					if !ok || comp.Value.String() != "db" {
						continue
					}
					errAttr, ok := findAttr(r, "error")
					if !ok || errAttr.Value.String() == "" {
						return "db error record missing non-empty error attr"
					}
					hits++
				}
				if hits != 1 {
					return "expected exactly 1 component=db error record"
				}
				return ""
			},
		},
		{
			name: "sync_lookup_fails",
			// DB is open but sync_status table does not exist, so
			// GetLastStatus will return a non-nil error.
			setupDB:    openTestDB,
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unhealthy"}`,
			logAssert: func(_ *testing.T, records []slog.Record) string {
				for _, r := range records {
					if r.Level != slog.LevelError {
						continue
					}
					comp, ok := findAttr(r, "component")
					if !ok || comp.Value.String() != "sync" {
						continue
					}
					errAttr, ok := findAttr(r, "error")
					if !ok || errAttr.Value.String() == "" {
						return "sync lookup error record missing non-empty error attr"
					}
					return ""
				}
				return "expected a component=sync error record from sync lookup failure"
			},
		},
		{
			name: "sync_marked_failed",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertSync(t, db, time.Now().Add(-1*time.Hour), "failed", "connection timeout to upstream")
				return db
			},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unhealthy"}`,
			logAssert: func(_ *testing.T, records []slog.Record) string {
				for _, r := range records {
					if r.Level != slog.LevelWarn {
						continue
					}
					comp, ok := findAttr(r, "component")
					if !ok || comp.Value.String() != "sync" {
						continue
					}
					errAttr, ok := findAttr(r, "error")
					if !ok || errAttr.Value.String() != "connection timeout to upstream" {
						return "sync failed record must carry ErrorMessage in error attr"
					}
					if _, ok := findAttr(r, "last_sync_at"); !ok {
						return "sync failed record missing last_sync_at attr"
					}
					return ""
				}
				return "expected a component=sync warn record for failed sync"
			},
		},
		{
			name: "sync_stale",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertSync(t, db, time.Now().Add(-48*time.Hour), "success", "")
				return db
			},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unhealthy"}`,
			logAssert: func(_ *testing.T, records []slog.Record) string {
				for _, r := range records {
					if r.Level != slog.LevelWarn {
						continue
					}
					comp, ok := findAttr(r, "component")
					if !ok || comp.Value.String() != "sync" {
						continue
					}
					if _, ok := findAttr(r, "last_sync_at"); !ok {
						return "stale sync record missing last_sync_at attr"
					}
					ageAttr, ok := findAttr(r, "age")
					if !ok {
						return "stale sync record missing age attr"
					}
					// The age attr carries a duration; its value must be
					// strictly greater than the 24h stale threshold.
					if d, ok := ageAttr.Value.Any().(time.Duration); ok {
						if d <= staleThreshold {
							return "stale sync age must exceed threshold"
						}
					}
					return ""
				}
				return "expected a component=sync warn record for stale sync"
			},
		},
		{
			name: "no_sync_yet",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				return db
			},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unhealthy"}`,
			logAssert: func(_ *testing.T, records []slog.Record) string {
				// Phase 77 OBS-06: "readyz no sync completed" demoted
				// from WARN to DEBUG — fires on every Fly health probe
				// during the 5-15min pre-first-sync window and is not
				// operator-actionable (the 503 already drives proxy
				// failover). See AUDIT.md.
				for _, r := range records {
					if r.Level != slog.LevelDebug {
						continue
					}
					comp, ok := findAttr(r, "component")
					if !ok || comp.Value.String() != "sync" {
						continue
					}
					return ""
				}
				return "expected a component=sync debug record for no-sync-yet"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := tt.setupDB(t)

			status, body, records := collectReadinessResponse(t, health.ReadinessInput{
				DB:             db,
				StaleThreshold: staleThreshold,
			})

			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d", status, tt.wantStatus)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			// The body must be EXACTLY one of the two generic shapes — no
			// surprise fields, no "components" map.
			if body != `{"status":"ok"}` && body != `{"status":"unhealthy"}` {
				t.Errorf("body %q is not a recognized generic shape", body)
			}
			if bytes.Contains([]byte(body), []byte("components")) {
				t.Errorf("body leaks components map: %q", body)
			}
			if msg := tt.logAssert(t, records); msg != "" {
				t.Errorf("log assertion failed: %s", msg)
			}
		})
	}
}

// openTestDB creates a fresh in-memory SQLite database for testing.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// initSyncTable creates the sync_status table.
func initSyncTable(t *testing.T, db *sql.DB) {
	t.Helper()
	if err := sync.InitStatusTable(t.Context(), db); err != nil {
		t.Fatalf("init sync_status table: %v", err)
	}
}

// insertSync inserts a completed sync status row.
func insertSync(t *testing.T, db *sql.DB, completedAt time.Time, status string, errMsg string) {
	t.Helper()
	startedAt := completedAt.Add(-5 * time.Minute)
	durationMs := completedAt.Sub(startedAt).Milliseconds()
	_, err := db.ExecContext(t.Context(),
		`INSERT INTO sync_status (started_at, completed_at, duration_ms, object_counts, status, error_message) VALUES (?, ?, ?, ?, ?, ?)`,
		startedAt, completedAt, durationMs, `{"network":100}`, status, errMsg,
	)
	if err != nil {
		t.Fatalf("inserting sync status: %v", err)
	}
}
