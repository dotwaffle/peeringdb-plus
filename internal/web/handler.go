package web

import (
	"cmp"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/web/templates"
)

// parseASN parses and validates an ASN string. Returns the ASN value and true
// if valid (1 <= asn <= 2^32-1). Returns 0 and false for non-numeric, zero,
// negative, or out-of-range values. The 32-bit bit size of ParseUint is the
// single source of truth for the upper bound.
func parseASN(s string) (uint32, bool) {
	asn, err := strconv.ParseUint(s, 10, 32)
	if err != nil || asn < 1 {
		return 0, false
	}
	return uint32(asn), true
}

// Handler serves web UI pages.
type Handler struct {
	client     *ent.Client
	db         *sql.DB
	searcher   *SearchService
	comparer   *CompareService
	authMode   string       // "authenticated" | "anonymous"
	publicTier privctx.Tier // captured from config.Config.PublicTier at startup
}

// NewHandlerInput configures a web Handler. Client is required; DB may be
// nil for tests that do not exercise the sync-status table. AuthMode and
// PublicTier feed the /ui/about Privacy & Sync section. When AuthMode is
// empty, the renderer falls back to "anonymous"; when PublicTier is the
// zero value (TierPublic), no override flag is shown.
type NewHandlerInput struct {
	Client     *ent.Client
	DB         *sql.DB
	AuthMode   string
	PublicTier privctx.Tier
}

// NewHandler creates a web UI handler with integrated search and compare
// services. AuthMode and PublicTier are captured at construction and
// surface on /ui/about. The input uses named fields so callers cannot
// transpose them.
func NewHandler(in NewHandlerInput) *Handler {
	return &Handler{
		client:     in.Client,
		db:         in.DB,
		searcher:   NewSearchService(in.Client),
		comparer:   NewCompareService(in.Client),
		authMode:   in.AuthMode,
		publicTier: in.PublicTier,
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
	case rest == "completions/bash":
		h.handleCompletionBash(w, r)
	case rest == "completions/zsh":
		h.handleCompletionZsh(w, r)
	case rest == "completions/search":
		h.handleCompletionSearch(w, r)
	default:
		h.handleNotFound(w, r)
	}
}

// handleHome renders the homepage. Supports ?q= for bookmarked URL search
// results (pre-render results on page load for shared URLs).
func (h *Handler) handleHome(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	var groups []templates.SearchGroup

	if len(strings.TrimSpace(query)) >= 2 {
		results, err := h.searcher.Search(r.Context(), query)
		if err == nil {
			groups = convertToSearchGroups(results)
		}
	}

	page := PageContent{
		Title:     "Home",
		Content:   templates.Home(query, groups),
		Freshness: h.getFreshness(r.Context()),
	}
	if len(groups) > 0 {
		page.Title = "Search"
		page.Data = groups
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}

// handleSearch returns search results as an HTML fragment for htmx partial updates.
// Sets HX-Push-Url header to keep the browser URL in sync with the search query
// and create history entries for back/forward navigation. When a ?type= slug is
// present it dispatches to the per-type "view all" page instead.
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("type") != "" {
		h.handleSearchType(w, r)
		return
	}

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

	// Set HX-Push-Url so the browser URL updates to /ui/?q=value
	// and creates a history entry for back/forward navigation.
	// Skip for spotlight overlay requests — they shouldn't modify browser history.
	if r.URL.Query().Get("spotlight") != "1" {
		if query != "" {
			w.Header().Set("HX-Push-Url", "/ui/?q="+url.QueryEscape(query))
		} else {
			w.Header().Set("HX-Push-Url", "/ui/")
		}
	}

	page := PageContent{
		Title:     "Search",
		Content:   templates.SearchResults(groups, query),
		Data:      groups,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}

// handleSearchType serves the per-type "view all" results page and its "Load
// more" pages. An offset > 0 returns a bare htmx fragment (next rows + button);
// offset 0 (or absent) returns the full page. An unknown type slug is a 404.
func (h *Handler) handleSearchType(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	query := q.Get("q")
	slug := q.Get("type")

	// Non-numeric/negative offsets clamp to 0 (the initial page).
	offset, err := strconv.Atoi(q.Get("offset"))
	if err != nil || offset < 0 {
		offset = 0
	}

	res, err := h.searcher.SearchType(r.Context(), SearchTypeInput{
		Query:    query,
		TypeSlug: slug,
		Offset:   offset,
		Limit:    pageSize,
	})
	if err != nil {
		if errors.Is(err, ErrUnknownSearchType) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("search type", slog.String("type", slug), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	view := templates.SearchTypeView{
		TypeName:    res.TypeName,
		TypeSlug:    res.TypeSlug,
		AccentColor: res.AccentColor,
		Query:       query,
		Total:       res.Total,
		Hits:        searchHitsToResults(res.Hits),
		HasMore:     res.HasMore,
		NextOffset:  offset + len(res.Hits),
	}

	// A "Load more" request (offset > 0) is always an htmx call and wants only
	// the next rows + button fragment, never the full page chrome.
	if offset > 0 {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := templates.SearchTypeMore(view).Render(r.Context(), w); err != nil {
			slog.Error("render search type more", slog.String("type", slug), slog.Any("error", err))
		}
		return
	}

	// Initial page: full layout for browsers, single-group data for terminal/JSON.
	page := PageContent{
		Title:     "Search",
		Content:   templates.SearchTypePage(view),
		Data:      searchTypeViewData(res),
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		h.handleServerError(w, r)
	}
}

// searchHitsToResults maps service-layer hits to template result rows.
func searchHitsToResults(hits []SearchHit) []templates.SearchResult {
	out := make([]templates.SearchResult, len(hits))
	for i, h := range hits {
		out[i] = templates.SearchResult{
			Name:      h.Name,
			Country:   h.Country,
			City:      h.City,
			ASN:       h.ASN,
			DetailURL: h.DetailURL,
		}
	}
	return out
}

// searchTypeViewData wraps a single-type result as a one-element SearchGroup
// slice so terminal and JSON clients reuse the existing grouped-search renderer.
func searchTypeViewData(res SearchTypeResult) []templates.SearchGroup {
	if len(res.Hits) == 0 {
		return nil
	}
	return []templates.SearchGroup{{
		TypeName:    res.TypeName,
		TypeSlug:    res.TypeSlug,
		AccentColor: res.AccentColor,
		Results:     searchHitsToResults(res.Hits),
		HasMore:     res.HasMore,
		Total:       res.Total,
	}}
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
				Country:   h.Country,
				City:      h.City,
				ASN:       h.ASN,
				DetailURL: h.DetailURL,
			}
		}
		groups[i] = templates.SearchGroup{
			TypeName:    r.TypeName,
			TypeSlug:    r.TypeSlug,
			AccentColor: r.AccentColor,
			Results:     hits,
			HasMore:     r.HasMore,
			Total:       r.Total,
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

	asn1, ok := parseASN(parts[0])
	if !ok {
		httperr.WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("invalid ASN %q: must be between 1 and 4294967295", parts[0]),
			Instance: r.URL.Path,
		})
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
	asn2, ok := parseASN(parts[1])
	if !ok {
		httperr.WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("invalid ASN %q: must be between 1 and 4294967295", parts[1]),
			Instance: r.URL.Path,
		})
		return
	}

	viewMode := cmp.Or(r.URL.Query().Get("view"), "shared")

	data, err := h.comparer.Compare(r.Context(), CompareInput{
		ASN1:     int(asn1),
		ASN2:     int(asn2),
		ViewMode: viewMode,
	})
	if err != nil {
		if ent.IsNotFound(err) {
			h.handleNotFound(w, r)
			return
		}
		slog.Error("compare networks", slog.Int("asn1", int(asn1)), slog.Int("asn2", int(asn2)), slog.Any("error", err))
		h.handleServerError(w, r)
		return
	}

	title := fmt.Sprintf("%s vs %s", data.NetA.Name, data.NetB.Name)
	page := PageContent{
		Title:     title,
		Content:   templates.CompareResultsPage(*data),
		Data:      data,
		Freshness: h.getFreshness(r.Context()),
	}
	if err := renderPage(r.Context(), w, r, page); err != nil {
		slog.Error("render compare", slog.Int("asn1", int(asn1)), slog.Int("asn2", int(asn2)), slog.Any("error", err))
		h.handleServerError(w, r)
	}
}
