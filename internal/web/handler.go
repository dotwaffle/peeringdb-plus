package web

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// Handler serves web UI pages.
type Handler struct {
	client   *ent.Client
	searcher *SearchService
}

// NewHandler creates a web UI handler with an integrated search service.
func NewHandler(client *ent.Client) *Handler {
	return &Handler{
		client:   client,
		searcher: NewSearchService(client),
	}
}

// Register mounts web UI routes on the given mux.
// Static assets are served from embedded files at /static/.
// UI pages are served under the /ui/ prefix.
// A single wildcard pattern dispatches all /ui/ paths internally,
// following the pdbcompat handler pattern.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.StripPrefix("/static/",
		http.FileServerFS(StaticFS)))

	mux.HandleFunc("GET /ui/{rest...}", h.dispatch)
}

func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	switch rest {
	case "", "/":
		h.handleHome(w, r)
	case "search":
		h.handleSearch(w, r)
	default:
		h.handleNotFound(w, r)
	}
}

// handleHome renders the homepage. Supports ?q= for bookmarked URL search
// results (Pitfall 3: pre-render results on page load for shared URLs).
func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var groups []templates.SearchGroup

	if len(strings.TrimSpace(query)) >= 2 {
		results, err := h.searcher.Search(r.Context(), query)
		if err == nil {
			groups = convertToSearchGroups(results)
		}
	}

	page := PageContent{Title: "Home", Content: templates.Home(query, groups)}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// handleSearch returns search results as an HTML fragment for htmx partial updates.
// Sets HX-Replace-Url header to keep the browser URL in sync with the search query.
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var groups []templates.SearchGroup

	if len(strings.TrimSpace(query)) >= 2 {
		results, err := h.searcher.Search(r.Context(), query)
		if err != nil {
			http.Error(w, "search error", http.StatusInternalServerError)
			return
		}
		groups = convertToSearchGroups(results)
	}

	// Set HX-Replace-Url so the browser URL updates to /ui/?q=value
	// without creating a new browser history entry (Pitfall 4).
	if query != "" {
		w.Header().Set("HX-Replace-Url", "/ui/?q="+url.QueryEscape(query))
	} else {
		w.Header().Set("HX-Replace-Url", "/ui/")
	}

	page := PageContent{Title: "Search", Content: templates.SearchResults(groups)}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (h *Handler) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	page := PageContent{Title: "Not Found", Content: templates.Home("", nil)}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// convertToSearchGroups converts SearchService results to template-friendly types.
func convertToSearchGroups(results []TypeResult) []templates.SearchGroup {
	groups := make([]templates.SearchGroup, len(results))
	for i, r := range results {
		hits := make([]templates.SearchResult, len(r.Results))
		for j, h := range r.Results {
			hits[j] = templates.SearchResult{
				Name:      h.Name,
				Subtitle:  h.Subtitle,
				DetailURL: h.DetailURL,
			}
		}
		groups[i] = templates.SearchGroup{
			TypeName:    r.TypeName,
			TypeSlug:    r.TypeSlug,
			AccentColor: r.AccentColor,
			Results:     hits,
			TotalCount:  r.TotalCount,
		}
	}
	return groups
}
