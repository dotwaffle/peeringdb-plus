package web

import (
	"net/http"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// Handler serves web UI pages.
type Handler struct {
	client *ent.Client
}

// NewHandler creates a web UI handler.
func NewHandler(client *ent.Client) *Handler {
	return &Handler{client: client}
}

// Register mounts web UI routes on the given mux.
// Static assets are served from embedded files at /static/.
// UI pages are served under the /ui/ prefix.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.StripPrefix("/static/",
		http.FileServerFS(StaticFS)))

	mux.HandleFunc("GET /ui/", h.handleHome)
	mux.HandleFunc("GET /ui/{rest...}", h.handleNotFound)
}

func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	page := PageContent{Title: "Home", Content: templates.Home()}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	page := PageContent{Title: "Not Found", Content: templates.Home()} // placeholder until 404 page exists
	if err := renderPage(r.Context(), w, r, page); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
