package health_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/health"
	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	_ "modernc.org/sqlite"
)

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

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
}

func TestReadinessHandler(t *testing.T) {
	t.Parallel()

	staleThreshold := 24 * time.Hour

	tests := []struct {
		name           string
		setupDB        func(t *testing.T) *sql.DB
		wantHTTPStatus int
		wantStatus     string
		wantDBStatus   string
		wantSyncStatus string
		wantSyncMsg    string // substring check for sync component message
	}{
		{
			name: "healthy: db ok and recent sync",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertSync(t, db, time.Now().Add(-1*time.Hour), "success", "")
				return db
			},
			wantHTTPStatus: http.StatusOK,
			wantStatus:     "ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "ok",
		},
		{
			name: "stale sync: last sync older than threshold",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertSync(t, db, time.Now().Add(-48*time.Hour), "success", "")
				return db
			},
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "not_ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "degraded",
			wantSyncMsg:    "stale",
		},
		{
			name: "no sync: no rows in sync_status",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				return db
			},
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "not_ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "failed",
			wantSyncMsg:    "no sync completed",
		},
		{
			name: "db down: closed database",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				db.Close()
				return db
			},
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "not_ready",
			wantDBStatus:   "failed",
		},
		{
			name: "running sync with previous success within threshold",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				// Insert a successful sync from 1 hour ago
				insertSync(t, db, time.Now().Add(-1*time.Hour), "success", "")
				// Insert a currently running sync (no completed_at)
				insertRunningSync(t, db)
				return db
			},
			wantHTTPStatus: http.StatusOK,
			wantStatus:     "ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "ok",
		},
		{
			name: "failed sync within threshold still shows degraded",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertSync(t, db, time.Now().Add(-1*time.Hour), "failed", "connection timeout")
				return db
			},
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "not_ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "degraded",
			wantSyncMsg:    "last sync failed",
		},
		{
			name: "running sync with no previous completed sync",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertRunningSync(t, db) // Only a "running" row, no prior completed
				return db
			},
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "not_ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "failed",
			wantSyncMsg:    "no sync completed",
		},
		{
			name: "unknown sync status",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				initSyncTable(t, db)
				insertSync(t, db, time.Now().Add(-1*time.Hour), "bogus_status", "")
				return db
			},
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "not_ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "failed",
			wantSyncMsg:    "unknown sync status",
		},
		{
			name: "sync table missing causes GetLastStatus error",
			setupDB: func(t *testing.T) *sql.DB {
				db := openTestDB(t)
				// Do NOT create the sync_status table -- GetLastStatus will fail
				return db
			},
			wantHTTPStatus: http.StatusServiceUnavailable,
			wantStatus:     "not_ready",
			wantDBStatus:   "ok",
			wantSyncStatus: "failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			db := tt.setupDB(t)

			handler := health.ReadinessHandler(health.ReadinessInput{
				DB:             db,
				StaleThreshold: staleThreshold,
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.wantHTTPStatus {
				t.Errorf("HTTP status = %d, want %d", rec.Code, tt.wantHTTPStatus)
			}

			ct := rec.Header().Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}

			var resp health.Response
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decoding response: %v", err)
			}
			if resp.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q", resp.Status, tt.wantStatus)
			}

			if tt.wantDBStatus != "" {
				db, ok := resp.Components["db"]
				if !ok {
					t.Fatal("missing 'db' component in response")
				}
				if db.Status != tt.wantDBStatus {
					t.Errorf("db.status = %q, want %q", db.Status, tt.wantDBStatus)
				}
			}

			if tt.wantSyncStatus != "" {
				sc, ok := resp.Components["sync"]
				if !ok {
					t.Fatal("missing 'sync' component in response")
				}
				if sc.Status != tt.wantSyncStatus {
					t.Errorf("sync.status = %q, want %q", sc.Status, tt.wantSyncStatus)
				}
				if tt.wantSyncMsg != "" && !strings.Contains(sc.Message, tt.wantSyncMsg) {
					t.Errorf("sync.message = %q, want substring %q", sc.Message, tt.wantSyncMsg)
				}
			}
		})
	}
}

func TestReadinessHandlerSyncTimestamp(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	initSyncTable(t, db)
	insertSync(t, db, time.Now().Add(-1*time.Hour), "success", "")

	handler := health.ReadinessHandler(health.ReadinessInput{
		DB:             db,
		StaleThreshold: 24 * time.Hour,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	handler.ServeHTTP(rec, req)

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	sc, ok := resp.Components["sync"]
	if !ok {
		t.Fatal("missing 'sync' component")
	}

	// Verify the message contains an RFC3339 timestamp
	if !strings.Contains(sc.Message, "T") || !strings.Contains(sc.Message, "Z") {
		t.Errorf("sync.message = %q, expected RFC3339 timestamp", sc.Message)
	}

	// Verify the message contains an age duration
	if !strings.Contains(sc.Message, "age") {
		t.Errorf("sync.message = %q, expected age duration", sc.Message)
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
	if err := sync.InitStatusTable(context.Background(), db); err != nil {
		t.Fatalf("init sync_status table: %v", err)
	}
}

// insertSync inserts a completed sync status row.
func insertSync(t *testing.T, db *sql.DB, completedAt time.Time, status string, errMsg string) {
	t.Helper()
	startedAt := completedAt.Add(-5 * time.Minute)
	durationMs := completedAt.Sub(startedAt).Milliseconds()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO sync_status (started_at, completed_at, duration_ms, object_counts, status, error_message) VALUES (?, ?, ?, ?, ?, ?)`,
		startedAt, completedAt, durationMs, `{"network":100}`, status, errMsg,
	)
	if err != nil {
		t.Fatalf("inserting sync status: %v", err)
	}
}

// insertRunningSync inserts a currently-running sync row (no completed_at).
func insertRunningSync(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(context.Background(),
		`INSERT INTO sync_status (started_at, status) VALUES (?, 'running')`,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("inserting running sync: %v", err)
	}
}
