package middleware

import (
	"fmt"
	"net/http"
	"strings"

	webtemplates "github.com/dotwaffle/peeringdb-plus/internal/web/templates"
	"github.com/dotwaffle/peeringdb-plus/internal/web/termrender"
)

// SyncReadiness reports whether at least one sync has completed.
type SyncReadiness interface {
	HasCompletedSync() bool
}

// Readiness returns 503 for all routes except infrastructure paths
// until the first sync has completed.
// Browser requests receive a styled HTML syncing page instead of JSON.
func Readiness(sr SyncReadiness, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Infrastructure, static, and gRPC health paths bypass readiness.
		// Static assets must be served for the syncing page to render correctly.
		// gRPC health check manages its own NOT_SERVING/SERVING state.
		if r.URL.Path == "/sync" || r.URL.Path == "/healthz" ||
			r.URL.Path == "/readyz" || r.URL.Path == "/" ||
			r.URL.Path == "/favicon.ico" ||
			strings.HasPrefix(r.URL.Path, "/static/") ||
			strings.HasPrefix(r.URL.Path, "/grpc.health.v1.Health/") {
			next.ServeHTTP(w, r)
			return
		}
		if !sr.HasCompletedSync() {
			accept := r.Header.Get("Accept")
			if strings.Contains(accept, "text/html") {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				webtemplates.SyncingPage().Render(r.Context(), w) //nolint:errcheck,gosec // best-effort render
				return
			}

			// Terminal clients (curl, wget, HTTPie) get styled text output.
			mode := termrender.Detect(termrender.DetectInput{
				UserAgent: r.UserAgent(),
				Accept:    accept,
				Query:     r.URL.Query(),
			})
			if mode == termrender.ModeRich || mode == termrender.ModePlain {
				noColor := termrender.HasNoColor(termrender.DetectInput{Query: r.URL.Query()})
				renderer := termrender.NewRenderer(mode, noColor)
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusServiceUnavailable)
				renderer.RenderError(w, http.StatusServiceUnavailable, "Service Unavailable", "PeeringDB data sync has not yet completed.\nPlease try again in a few moments.") //nolint:errcheck,gosec // best-effort render
				return
			}

			// API/JSON fallback for non-terminal, non-browser clients.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":"sync not yet completed"}`)
			return
		}
		next.ServeHTTP(w, r)
	})
}
