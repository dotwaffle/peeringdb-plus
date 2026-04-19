package pdbcompat

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
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
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusNotFound,
			Detail:   fmt.Sprintf("unknown type %q", typeName),
			Instance: r.URL.Path,
		})
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
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("invalid id %q: not an integer", idStr),
			Instance: r.URL.Path,
		})
		return
	}
	h.serveDetail(tc, id, w, r)
}

// splitTypeID splits a rest path like "net", "net/", "net/42" into type name
// and optional ID string.
func splitTypeID(rest string) (typeName, id string) {
	rest = strings.TrimRight(rest, "/")
	if rest == "" {
		return "", ""
	}
	typeName, id, _ = strings.Cut(rest, "/")
	return typeName, id
}

// indexBody is computed once at init time since the Registry does not change.
var indexBody []byte

func init() {
	type indexEntry struct {
		ListEndpoint string `json:"list_endpoint"`
	}

	// Collect sorted type names for deterministic output.
	names := slices.Sorted(maps.Keys(Registry))

	index := make(map[string]indexEntry, len(Registry))
	for _, name := range names {
		index[name] = indexEntry{
			ListEndpoint: "/api/" + name,
		}
	}

	indexBody, _ = json.Marshal(index)
}

// serveIndex writes the API index listing all available types.
func (h *Handler) serveIndex(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Powered-By", poweredByHeader)
	_, _ = w.Write(indexBody)
}

// serveList handles list requests for the given type.
func (h *Handler) serveList(tc TypeConfig, w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	// Phase 68 LIMIT-02 guardrail (research Open Question 1, recommendation b):
	// list + ?depth= is not supported in Phase 68. Phase 71 owns the memory-safe
	// list+depth implementation. Silently ignore the param here so callers get
	// normal list behaviour rather than a 400 (matches upstream rest.py:
	// unsupported request shapes fall through to default list semantics).
	// opts.Depth is never populated on list requests, so there is no leak —
	// list closures never see a non-zero depth. The debug slog documents the
	// no-op for operators who enable DEBUG logging.
	if params.Get("depth") != "" {
		slog.DebugContext(r.Context(), "pdbcompat list: ignoring unsupported ?depth= param (Phase 68 LIMIT-02 guardrail; Phase 71 will add list+depth)",
			slog.String("path", r.URL.Path),
			slog.String("type", tc.Name),
		)
	}

	// Parse pagination per D-16.
	limit, skip := ParsePaginationParams(params)

	// Parse filters per D-09, D-11.
	filters, err := ParseFilters(params, tc.Fields)
	if err != nil {
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("filter error: %v", err),
			Instance: r.URL.Path,
		})
		return
	}

	// Parse since per D-15.
	since, err := ParseSinceParam(params)
	if err != nil {
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("invalid since parameter: %v", err),
			Instance: r.URL.Path,
		})
		return
	}

	// Parse search (?q=) per D-13.
	if q := params.Get("q"); q != "" {
		var sp func(*sql.Selector)
		if tc.Name == peeringdb.TypeNet {
			sp = buildNetworkSearchPredicate(q, tc.SearchFields)
		} else {
			sp = buildSearchPredicate(q, tc.SearchFields)
		}
		if sp != nil {
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
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusInternalServerError,
			Detail:   fmt.Sprintf("query error: %v", err),
			Instance: r.URL.Path,
		})
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
			WriteProblem(w, httperr.WriteProblemInput{
				Status:   http.StatusNotFound,
				Detail:   fmt.Sprintf("%s with id %d not found", tc.Name, id),
				Instance: r.URL.Path,
			})
			return
		}
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusInternalServerError,
			Detail:   fmt.Sprintf("query error: %v", err),
			Instance: r.URL.Path,
		})
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
