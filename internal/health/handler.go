// Package health provides HTTP handlers for liveness and readiness checks.
// The liveness endpoint confirms the process is alive. The readiness endpoint
// checks database connectivity and sync data freshness before reporting the
// service as ready to receive traffic.
//
// SEC-08: the wire body for both endpoints is a fixed generic shape —
// {"status":"ok"} or {"status":"unhealthy"}. Detailed errors (DB driver
// messages, sync.Status.ErrorMessage, upstream error paths) are routed to a
// structured slog.Logger via LogAttrs with component/error/last_sync_at/age
// attrs so operators retain visibility without leaking details to anonymous
// callers.
package health

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/sync"
)

// Generic response bodies. Hardcoded strings avoid any risk of json.Encode
// surprising us with extra fields from struct tag drift.
const (
	bodyOK        = `{"status":"ok"}`
	bodyUnhealthy = `{"status":"unhealthy"}`
)

// Response is retained for backwards compatibility with any downstream
// consumer that imports the type symbol. The health handlers themselves no
// longer write this shape to the wire — see bodyOK/bodyUnhealthy.
type Response struct {
	Status     string               `json:"status"`
	Components map[string]Component `json:"components"`
}

// Component mirrors Response: retained as an exported symbol for
// backwards-source-compat only. Not written to the wire by this package.
type Component struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// writeGeneric emits a fixed generic body with the given HTTP status. The
// body is one of bodyOK or bodyUnhealthy; callers MUST NOT pass other
// strings.
func writeGeneric(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

// LivenessHandler returns an http.HandlerFunc that always responds 200 OK
// with the generic body. It confirms the process is running; no dependency
// checks are performed.
func LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeGeneric(w, http.StatusOK, bodyOK)
	}
}

// ReadinessInput holds dependencies for the readiness check handler.
type ReadinessInput struct {
	// DB is the database handle probed by ReadinessHandler.
	DB *sql.DB
	// StaleThreshold is the maximum allowed age of the latest successful sync
	// before the handler reports unhealthy.
	StaleThreshold time.Duration
	// Logger receives structured detail about unhealthy components. MUST be
	// non-nil — the caller MUST pass a logger (use slog.Default() if no
	// project-wide logger is available). A nil Logger is a programming error
	// and will panic on first LogAttrs call.
	Logger *slog.Logger
}

// ReadinessHandler returns an http.HandlerFunc that checks database
// connectivity and sync data freshness. It returns 200 with bodyOK when all
// checks pass, or 503 with bodyUnhealthy when any component is unhealthy.
// All failure detail is written to in.Logger — the wire body carries no
// component or error information.
func ReadinessHandler(in ReadinessInput) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// DB probe with 2-second timeout.
		dbCtx, dbCancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer dbCancel()

		if err := in.DB.PingContext(dbCtx); err != nil {
			in.Logger.LogAttrs(r.Context(), slog.LevelError,
				"readyz db probe failed",
				slog.String("component", "db"),
				slog.Any("error", err),
			)
			writeGeneric(w, http.StatusServiceUnavailable, bodyUnhealthy)
			return
		}

		if !checkSync(r.Context(), in.DB, in.StaleThreshold, in.Logger) {
			writeGeneric(w, http.StatusServiceUnavailable, bodyUnhealthy)
			return
		}

		writeGeneric(w, http.StatusOK, bodyOK)
	}
}

// checkSync evaluates sync data freshness and returns true when the sync
// subsystem is healthy. On unhealthy status, it logs the detail via logger
// and returns false. The caller writes the wire response.
func checkSync(ctx context.Context, db *sql.DB, staleThreshold time.Duration, logger *slog.Logger) bool {
	status, err := sync.GetLastStatus(ctx, db)
	if err != nil {
		logger.LogAttrs(ctx, slog.LevelError,
			"readyz sync lookup failed",
			slog.String("component", "sync"),
			slog.Any("error", err),
		)
		return false
	}

	if status == nil {
		logger.LogAttrs(ctx, slog.LevelDebug,
			"readyz no sync completed",
			slog.String("component", "sync"),
		)
		return false
	}

	switch status.Status {
	case "success":
		return evaluateSyncAge(ctx, status.LastSyncAt, staleThreshold, logger)

	case "running":
		// A currently-running sync; fall back to the most recent completed
		// sync via the shared internal/sync helper. If there isn't one,
		// report unhealthy.
		lastCompleted, lookupErr := sync.GetLastCompletedStatus(ctx, db)
		if lookupErr != nil {
			logger.LogAttrs(ctx, slog.LevelError,
				"readyz sync lookup failed",
				slog.String("component", "sync"),
				slog.Any("error", lookupErr),
			)
			return false
		}
		if lastCompleted == nil {
			logger.LogAttrs(ctx, slog.LevelDebug,
				"readyz no sync completed",
				slog.String("component", "sync"),
			)
			return false
		}
		return evaluateSyncAge(ctx, lastCompleted.LastSyncAt, staleThreshold, logger)

	case "failed":
		logger.LogAttrs(ctx, slog.LevelWarn,
			"readyz sync marked failed",
			slog.String("component", "sync"),
			slog.String("error", status.ErrorMessage),
			slog.Time("last_sync_at", status.LastSyncAt),
		)
		return false

	default:
		logger.LogAttrs(ctx, slog.LevelWarn,
			"readyz unknown sync status",
			slog.String("component", "sync"),
			slog.String("status", status.Status),
		)
		return false
	}
}

// evaluateSyncAge returns true when lastSyncAt is within staleThreshold.
// When stale, it logs a warn-level record with last_sync_at and age attrs
// and returns false.
func evaluateSyncAge(ctx context.Context, lastSyncAt time.Time, staleThreshold time.Duration, logger *slog.Logger) bool {
	age := time.Since(lastSyncAt).Truncate(time.Second)
	if age > staleThreshold {
		logger.LogAttrs(ctx, slog.LevelWarn,
			"readyz sync stale",
			slog.String("component", "sync"),
			slog.Time("last_sync_at", lastSyncAt),
			slog.Duration("age", age),
		)
		return false
	}
	return true
}
