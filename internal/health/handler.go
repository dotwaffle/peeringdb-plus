// Package health provides HTTP handlers for liveness and readiness checks.
// The liveness endpoint confirms the process is alive. The readiness endpoint
// checks database connectivity and sync data freshness before reporting the
// service as ready to receive traffic.
package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// Response is the JSON body for health endpoint responses.
type Response struct {
	Status     string               `json:"status"`
	Components map[string]Component `json:"components"`
}

// Component is the status of an individual health check component.
type Component struct {
	Status  string `json:"status"`            // "ok", "degraded", "failed"
	Message string `json:"message,omitempty"`
}

// LivenessHandler returns an http.HandlerFunc that always responds 200 OK.
// It confirms the process is running; no dependency checks are performed.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Status:     "ok",
			Components: map[string]Component{},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}

// ReadinessInput holds dependencies for the readiness check handler.
type ReadinessInput struct {
	DB             *sql.DB
	StaleThreshold time.Duration
}

// ReadinessHandler returns an http.HandlerFunc that checks database
// connectivity and sync data freshness. It returns 200 when all checks
// pass, or 503 when any component is unhealthy.
func ReadinessHandler(in ReadinessInput) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			Status:     "ready",
			Components: map[string]Component{},
		}

		// DB check with 2-second timeout.
		dbCtx, dbCancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer dbCancel()

		if err := in.DB.PingContext(dbCtx); err != nil {
			resp.Components["db"] = Component{
				Status:  "failed",
				Message: err.Error(),
			}
			resp.Status = "not_ready"
		} else {
			resp.Components["db"] = Component{Status: "ok"}
		}

		// Sync freshness check.
		if resp.Components["db"].Status == "ok" {
			checkSync(r.Context(), in.DB, in.StaleThreshold, &resp)
		}

		// Write response.
		w.Header().Set("Content-Type", "application/json")
		if resp.Status == "ready" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(resp)
	}
}

// checkSync evaluates sync data freshness and updates the response.
func checkSync(ctx context.Context, db *sql.DB, staleThreshold time.Duration, resp *Response) {
	status, err := sync.GetLastSyncStatus(ctx, db)
	if err != nil {
		resp.Components["sync"] = Component{
			Status:  "failed",
			Message: err.Error(),
		}
		resp.Status = "not_ready"
		return
	}

	if status == nil {
		resp.Components["sync"] = Component{
			Status:  "failed",
			Message: "no sync completed",
		}
		resp.Status = "not_ready"
		return
	}

	switch status.Status {
	case "success":
		evaluateSyncAge(status.LastSyncAt, staleThreshold, resp)

	case "running":
		// A currently-running sync; check the most recent completed sync.
		lastCompleted, lookupErr := getLastCompletedSync(ctx, db)
		if lookupErr != nil || lastCompleted == nil {
			resp.Components["sync"] = Component{
				Status:  "failed",
				Message: "no sync completed",
			}
			resp.Status = "not_ready"
			return
		}
		evaluateSyncAge(lastCompleted.LastSyncAt, staleThreshold, resp)

	case "failed":
		age := time.Since(status.LastSyncAt).Truncate(time.Second)
		msg := fmt.Sprintf("last sync failed: %s", status.ErrorMessage)
		resp.Components["sync"] = Component{
			Status:  "degraded",
			Message: fmt.Sprintf("%s, last at %s, age %s", msg, status.LastSyncAt.UTC().Format(time.RFC3339), age),
		}
		resp.Status = "not_ready"

	default:
		resp.Components["sync"] = Component{
			Status:  "failed",
			Message: fmt.Sprintf("unknown sync status: %s", status.Status),
		}
		resp.Status = "not_ready"
	}
}

// evaluateSyncAge checks whether the sync timestamp is within the staleness
// threshold and updates the response accordingly.
func evaluateSyncAge(lastSyncAt time.Time, staleThreshold time.Duration, resp *Response) {
	age := time.Since(lastSyncAt).Truncate(time.Second)
	ts := lastSyncAt.UTC().Format(time.RFC3339)

	if age > staleThreshold {
		resp.Components["sync"] = Component{
			Status:  "degraded",
			Message: fmt.Sprintf("sync data stale, last sync at %s, age %s", ts, age),
		}
		resp.Status = "not_ready"
		return
	}

	resp.Components["sync"] = Component{
		Status:  "ok",
		Message: fmt.Sprintf("last sync at %s, age %s", ts, age),
	}
}

// getLastCompletedSync returns the most recent non-running sync status.
// This is used when the latest row is "running" to find the previous sync result.
func getLastCompletedSync(ctx context.Context, db *sql.DB) (*sync.SyncStatus, error) {
	row := db.QueryRowContext(ctx,
		`SELECT started_at, completed_at, duration_ms, object_counts, status, error_message
		 FROM sync_status
		 WHERE status != 'running'
		 ORDER BY id DESC LIMIT 1`,
	)

	var (
		startedAt    time.Time
		completedAt  sql.NullTime
		durationMs   sql.NullInt64
		countsStr    sql.NullString
		statusStr    string
		errorMessage sql.NullString
	)
	err := row.Scan(&startedAt, &completedAt, &durationMs, &countsStr, &statusStr, &errorMessage)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get last completed sync: %w", err)
	}

	s := &sync.SyncStatus{
		LastSyncAt: startedAt,
		Status:     statusStr,
	}
	if completedAt.Valid {
		s.LastSyncAt = completedAt.Time
	}
	if durationMs.Valid {
		s.Duration = time.Duration(durationMs.Int64) * time.Millisecond
	}
	if errorMessage.Valid {
		s.ErrorMessage = errorMessage.String
	}
	return s, nil
}
