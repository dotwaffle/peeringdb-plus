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
// Freshness lookup strategy: the about page wants to show "when was the
// last known-good data?", so we go straight to GetLastSuccessfulStatus
// which queries WHERE status = 'success'. This is robust against:
//   - an in-flight sync (latest row is "running")
//   - a failed latest sync (latest row is "failed", e.g. upstream 429)
//   - both at once (latest is "running", one before is "failed", etc.)
//
// If GetLastSuccessfulStatus returns nil, no successful sync has ever been
// recorded and the page truthfully shows "Sync status unavailable". This
// is the right answer for a fresh database or a broken-from-the-start sync.
//
// Note: /readyz uses GetLastCompletedStatus instead because a health check
// wants to report "most recent outcome, success or failure" — the two
// surfaces have different semantics by design.
func (h *Handler) handleAbout(w http.ResponseWriter, r *http.Request) {
	var freshness templates.DataFreshness
	if h.db != nil {
		status, err := sync.GetLastSuccessfulStatus(r.Context(), h.db)
		if err != nil {
			slog.Error("get last successful sync status", slog.Any("error", err))
		} else if status != nil {
			freshness = templates.DataFreshness{
				Available:  true,
				LastSyncAt: status.LastSyncAt,
				Age:        time.Since(status.LastSyncAt).Truncate(time.Second),
			}
		}
	}

	page := PageContent{Title: "About", Content: templates.AboutPage(freshness), Data: freshness}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}
