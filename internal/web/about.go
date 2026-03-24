package web

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/sync"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// handleAbout renders the about page with live data freshness.
func (h *Handler) handleAbout(w http.ResponseWriter, r *http.Request) {
	var freshness templates.DataFreshness
	if h.db != nil {
		status, err := sync.GetLastStatus(r.Context(), h.db)
		if err != nil {
			slog.Error("get last sync status", slog.String("error", err.Error()))
		} else if status != nil && status.Status == "success" {
			freshness = templates.DataFreshness{
				Available:  true,
				LastSyncAt: status.LastSyncAt,
				Age:        time.Since(status.LastSyncAt).Truncate(time.Second),
			}
		}
	}

	page := PageContent{Title: "About", Content: templates.AboutPage(freshness)}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}
