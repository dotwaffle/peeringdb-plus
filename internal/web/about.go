package web

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// handleAbout renders the about page with live data freshness.
//
// Freshness lookup strategy: inspect the most recent sync_status row first,
// and only use it if the sync succeeded. If the latest row is "running" (a
// sync is in flight, or a previous sync crashed before completion) or
// "failed", fall back to the most recent non-running row via
// GetLastCompletedStatus. This mirrors the /readyz handler's fallback logic
// (see internal/health/handler.go checkSync) so both surfaces agree on what
// the last "known-good" sync was. Prevents a naive "Sync status unavailable"
// display whenever a sync happens to be mid-flight.
func (h *Handler) handleAbout(w http.ResponseWriter, r *http.Request) {
	var freshness templates.DataFreshness
	if h.db != nil {
		status, err := sync.GetLastStatus(r.Context(), h.db)
		if err != nil {
			slog.Error("get last sync status", slog.Any("error", err))
		} else {
			// If the latest row is "running" or "failed", try the last
			// completed row so the UI can still show freshness for the
			// most recent success even though a newer sync is mid-flight
			// or errored.
			if status != nil && status.Status != "success" {
				completed, cerr := sync.GetLastCompletedStatus(r.Context(), h.db)
				if cerr != nil {
					slog.Error("get last completed sync status", slog.Any("error", cerr))
				} else if completed != nil && completed.Status == "success" {
					status = completed
				}
			}
			if status != nil && status.Status == "success" {
				freshness = templates.DataFreshness{
					Available:  true,
					LastSyncAt: status.LastSyncAt,
					Age:        time.Since(status.LastSyncAt).Truncate(time.Second),
				}
			}
		}
	}

	page := PageContent{Title: "About", Content: templates.AboutPage(freshness), Data: freshness}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}
