package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// Handler serves PeeringDB-compatible API endpoints.
type Handler struct {
	client *ent.Client
}

// NewHandler creates a Handler for PeeringDB-compatible API endpoints.
func NewHandler(client *ent.Client) *Handler {
	return &Handler{client: client}
}

// Register sets up PeeringDB-compatible routes on the given mux.
// Routes follow PeeringDB's URL patterns: /api/{type}, /api/{type}/{id}.
// Both with and without trailing slash variants are handled per D-02.
// The index endpoint at /api/ lists all available types per D-17.
func (h *Handler) Register(mux *http.ServeMux) {
	// Single wildcard pattern handles all /api/ sub-paths including the
	// index itself. Go 1.22+ {rest...} wildcard matches the empty string
	// for /api/ requests, so index, list, and detail are all dispatched
	// from one registration point.
	mux.HandleFunc("GET /api/{rest...}", h.dispatch)
}

// dispatch routes requests under /api/ to index, list, or detail handlers
// based on the URL path structure.
func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")

	// Parse the rest path: "", "{type}", "{type}/", or "{type}/{id}".
	typeName, idStr := splitTypeID(rest)

	if typeName == "" {
		// /api/ or /api -- serve the index.
		h.serveIndex(w)
		return
	}

	// Validate type name against Registry.
	tc, ok := Registry[typeName]
	if !ok {
		WriteError(w, http.StatusNotFound, fmt.Sprintf("unknown type %q", typeName))
		return
	}

	if idStr == "" {
		// List endpoint: /api/{type} or /api/{type}/
		h.serveList(tc, w, r)
		return
	}

	// Detail endpoint: /api/{type}/{id}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid id %q: not an integer", idStr))
		return
	}
	h.serveDetail(tc, id, w, r)
}

// splitTypeID splits a rest path like "net", "net/", "net/42" into type name
// and optional ID string.
func splitTypeID(rest string) (typeName, id string) {
	if rest == "" {
		return "", ""
	}

	// Strip trailing slash for type-only paths.
	for len(rest) > 0 && rest[len(rest)-1] == '/' {
		rest = rest[:len(rest)-1]
	}
	if rest == "" {
		return "", ""
	}

	// Find the first slash separator.
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			return rest[:i], rest[i+1:]
		}
	}
	return rest, ""
}

// indexBody is computed once at init time since the Registry does not change.
var indexBody []byte

func init() {
	type indexEntry struct {
		ListEndpoint string `json:"list_endpoint"`
	}

	// Collect sorted type names for deterministic output.
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	sort.Strings(names)

	index := make(map[string]indexEntry, len(Registry))
	for _, name := range names {
		index[name] = indexEntry{
			ListEndpoint: "/api/" + name,
		}
	}

	indexBody, _ = json.Marshal(index) //nolint:errcheck // static data
}

// serveIndex writes the API index listing all available types.
func (h *Handler) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Powered-By", poweredByHeader)
	w.Write(indexBody) //nolint:errcheck // best-effort write
}

// serveList handles list requests for the given type.
func (h *Handler) serveList(tc TypeConfig, w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	// Parse pagination per D-16.
	limit, skip := ParsePaginationParams(params)

	// Parse filters per D-09, D-11.
	filters, err := ParseFilters(params, tc.Fields)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("filter error: %v", err))
		return
	}

	// Parse since per D-15.
	since, err := ParseSinceParam(params)
	if err != nil {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid since parameter: %v", err))
		return
	}

	// Parse search (?q=) per D-13.
	if q := params.Get("q"); q != "" {
		if sp := buildSearchPredicate(q, tc.SearchFields); sp != nil {
			filters = append(filters, sp)
		}
	}

	// Parse field projection (?fields=) per D-14.
	var fields []string
	if f := params.Get("fields"); f != "" {
		fields = strings.Split(f, ",")
	}

	opts := QueryOptions{
		Filters: filters,
		Limit:   limit,
		Skip:    skip,
		Since:   since,
	}

	results, _, err := tc.List(r.Context(), h.client, opts)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("query error: %v", err))
		return
	}

	// Apply field projection after retrieval.
	if len(fields) > 0 {
		results = applyFieldProjection(results, fields)
	}

	WriteResponse(w, results)
}

// serveDetail handles detail requests for a single object by ID.
func (h *Handler) serveDetail(tc TypeConfig, id int, w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	// Parse depth per D-06, D-07.
	depth := 0
	if v := params.Get("depth"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil && (parsed == 0 || parsed == 2) {
			depth = parsed
		}
	}

	// Parse field projection (?fields=) per D-14.
	var fields []string
	if f := params.Get("fields"); f != "" {
		fields = strings.Split(f, ",")
	}

	result, err := tc.Get(r.Context(), h.client, id, depth)
	if err != nil {
		if ent.IsNotFound(err) {
			WriteError(w, http.StatusNotFound, fmt.Sprintf("%s with id %d not found", tc.Name, id))
			return
		}
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("query error: %v", err))
		return
	}

	// Pitfall 7: single object wrapped in array.
	data := []any{result}

	// Apply field projection after retrieval.
	if len(fields) > 0 {
		data = applyFieldProjection(data, fields)
	}

	WriteResponse(w, data)
}
