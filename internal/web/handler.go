package web

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// Handler serves web UI pages.
type Handler struct {
	client   *ent.Client
	db       *sql.DB
	searcher *SearchService
	comparer *CompareService
}

// NewHandler creates a web UI handler with integrated search and compare services.
func NewHandler(client *ent.Client, db *sql.DB) *Handler {
	return &Handler{
		client:   client,
		db:       db,
		searcher: NewSearchService(client),
		comparer: NewCompareService(client),
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

	// Serve favicon.ico at root for browsers that request it directly.
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "/favicon.ico"
		http.FileServerFS(StaticFS).ServeHTTP(w, r)
	})

	mux.HandleFunc("GET /ui/{rest...}", h.dispatch)
}

func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	switch {
	case rest == "" || rest == "/":
		h.handleHome(w, r)
	case rest == "search":
		h.handleSearch(w, r)
	case strings.HasPrefix(rest, "asn/"):
		h.handleNetworkDetail(w, r, strings.TrimPrefix(rest, "asn/"))
	case strings.HasPrefix(rest, "ix/"):
		h.handleIXDetail(w, r, strings.TrimPrefix(rest, "ix/"))
	case strings.HasPrefix(rest, "fac/"):
		h.handleFacilityDetail(w, r, strings.TrimPrefix(rest, "fac/"))
	case strings.HasPrefix(rest, "org/"):
		h.handleOrgDetail(w, r, strings.TrimPrefix(rest, "org/"))
	case strings.HasPrefix(rest, "campus/"):
		h.handleCampusDetail(w, r, strings.TrimPrefix(rest, "campus/"))
	case strings.HasPrefix(rest, "carrier/"):
		h.handleCarrierDetail(w, r, strings.TrimPrefix(rest, "carrier/"))
	case strings.HasPrefix(rest, "fragment/"):
		h.handleFragment(w, r, strings.TrimPrefix(rest, "fragment/"))
	case rest == "about":
		h.handleAbout(w, r)
	case rest == "compare":
		h.handleCompareForm(w, r)
	case strings.HasPrefix(rest, "compare/"):
		h.handleCompare(w, r, strings.TrimPrefix(rest, "compare/"))
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
		h.handleServerError(w, r)
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
			h.handleServerError(w, r)
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

	page := PageContent{Title: "Search", Content: templates.SearchResults(groups), Data: groups}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}

func (h *Handler) handleNotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	page := PageContent{Title: "Not Found", Content: templates.NotFoundPage()}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// handleServerError renders a styled 500 error page.
func (h *Handler) handleServerError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	page := PageContent{Title: "Server Error", Content: templates.ServerErrorPage()}
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

// handleCompareForm renders the empty comparison form, optionally pre-filling
// ASN values from query parameters.
func (h *Handler) handleCompareForm(w http.ResponseWriter, r *http.Request) {
	asn1 := r.URL.Query().Get("asn1")
	asn2 := r.URL.Query().Get("asn2")
	page := PageContent{Title: "Compare Networks", Content: templates.CompareFormPage(asn1, asn2)}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}

// handleCompare handles both /ui/compare/{asn1} (pre-fill form) and
// /ui/compare/{asn1}/{asn2} (show comparison results).
func (h *Handler) handleCompare(w http.ResponseWriter, r *http.Request, path string) {
	parts := strings.SplitN(path, "/", 2)

	asn1, err := strconv.Atoi(parts[0])
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	// /ui/compare/{asn1} -- show form with first ASN pre-filled.
	if len(parts) == 1 || parts[1] == "" {
		page := PageContent{
			Title:   "Compare Networks",
			Content: templates.CompareFormPage(parts[0], ""),
		}
		if err := renderPage(r.Context(), w, r, page); err != nil {
			h.handleServerError(w, r)
		}
		return
	}

	// /ui/compare/{asn1}/{asn2} -- show results.
	asn2, err := strconv.Atoi(parts[1])
	if err != nil {
		h.handleNotFound(w, r)
		return
	}

	viewMode := r.URL.Query().Get("view")
	if viewMode == "" {
		viewMode = "shared"
	}

	data, err := h.comparer.Compare(r.Context(), CompareInput{
		ASN1:     asn1,
		ASN2:     asn2,
		ViewMode: viewMode,
	})
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("compare networks", slog.Int("asn1", asn1), slog.Int("asn2", asn2), slog.String("error", err.Error()))
		h.handleServerError(w, r)
		return
	}

	title := fmt.Sprintf("%s vs %s", data.NetA.Name, data.NetB.Name)
	page := PageContent{Title: title, Content: templates.CompareResultsPage(*data), Data: data}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render compare", slog.Int("asn1", asn1), slog.Int("asn2", asn2), slog.String("error", err.Error()))
		h.handleServerError(w, r)
	}
}
