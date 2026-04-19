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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// Handler serves PeeringDB-compatible API endpoints.
type Handler struct {
	client *ent.Client
	// responseMemoryLimit is the per-response byte budget consumed by
	// the pre-flight CheckBudget gate (Phase 71 D-02). 0 disables the
	// check entirely — documented local-dev / test escape hatch. See
	// cmd/peeringdb-plus/main.go for the Config.ResponseMemoryLimit
	// wiring (default 128 MiB per D-05).
	responseMemoryLimit int64
}

// NewHandler creates a Handler for PeeringDB-compatible API endpoints.
// responseMemoryLimit is the per-response byte budget consumed by the
// pre-flight CheckBudget gate (Phase 71 D-02). Pass 0 to disable the
// budget check (local dev / tests only; operators ship a non-zero
// PDBPLUS_RESPONSE_MEMORY_LIMIT in prod — default 128 MiB per D-05).
func NewHandler(client *ent.Client, responseMemoryLimit int64) *Handler {
	return &Handler{client: client, responseMemoryLimit: responseMemoryLimit}
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
	// Phase 71 Plan 05 (MEMORY-03, D-06): per-request heap-delta sampler.
	// Samples HeapInuse once at entry and once at handler exit (via defer);
	// emits OTel span attribute pdbplus.response.heap_delta_kib and a
	// Prometheus histogram observation. runtime.ReadMemStats is STW but
	// acceptable once per request. MUST NOT be called per-row.
	//
	// Fires on EVERY terminal path (200 success, 413 budget-exceeded, 400
	// filter-error, 500 query-error) — that's the point of a defer; a
	// budget-exceeded 413 is still a useful data point for the
	// distribution, and observing the small delta of a cheap error path
	// gives us a noise floor reference.
	startHeapKiB := memStatsHeapInuseKiB()
	defer recordResponseHeapDelta(r.Context(), r.URL.Path, tc.Name, startHeapKiB)

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

	// Parse filters per D-09, D-11. Phase 69 Plan 04 adds the emptyResult
	// short-circuit for ?field__in= (IN-02) and threads TypeConfig so that
	// shadow-column routing (UNICODE-01) can consult tc.FoldedFields.
	//
	// Phase 70 D-05 / TRAVERSAL-04: thread the ctx through an unknown-field
	// accumulator so operators can observe silently-ignored filter keys via
	// slog DEBUG + OTel span attribute. ParseFiltersCtx writes to the
	// accumulator; we emit the diagnostics AFTER it returns so the request
	// path sees zero behavioural change (HTTP 200, no 400).
	ctx := WithUnknownFields(r.Context())
	filters, emptyResult, err := ParseFiltersCtx(ctx, params, tc)
	if err != nil {
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("filter error: %v", err),
			Instance: r.URL.Path,
		})
		return
	}
	if unknown := UnknownFieldsFromCtx(ctx); len(unknown) > 0 {
		csv := strings.Join(unknown, ",")
		slog.DebugContext(ctx, "pdbcompat: unknown filter fields silently ignored (Phase 70 TRAVERSAL-04)",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("unknown_fields", csv),
		)
		// OTel span attribute — no-op when no active span (OTel not
		// configured in tests) because SpanFromContext returns a noop
		// span whose SetAttributes is safe and SpanContext().IsValid()
		// is false.
		if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
			span.SetAttributes(attribute.String("pdbplus.filter.unknown_fields", csv))
		}
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
		Filters:     filters,
		Limit:       limit,
		Skip:        skip,
		Since:       since,
		EmptyResult: emptyResult,
	}

	// Phase 71 pre-flight budget check (D-02).
	//
	// Runs BEFORE tc.List so an over-budget response 413s without
	// committing to the expensive .All(ctx) + serialise path. Two
	// explicit bypass conditions:
	//
	//   - h.responseMemoryLimit <= 0: budget disabled (local dev / tests).
	//     CheckBudget already treats budget<=0 as "always fits" but we
	//     avoid the round-trip to COUNT(*) for the dev path.
	//   - emptyResult: Phase 69 IN-02 short-circuit (?asn__in=). The
	//     result is known-empty; counting 0 rows and then streaming []
	//     is wasted work and would paper over a broken CountFunc.
	//
	// List depth is always 0 per Phase 68 LIMIT-02 guardrail (the
	// ?depth= param is ignored on list endpoints; opts.Depth is never
	// populated by ParsePaginationParams).
	if h.responseMemoryLimit > 0 && !emptyResult && tc.Count != nil {
		count, err := tc.Count(r.Context(), h.client, opts)
		if err != nil {
			WriteProblem(w, httperr.WriteProblemInput{
				Status:   http.StatusInternalServerError,
				Detail:   fmt.Sprintf("count error: %v", err),
				Instance: r.URL.Path,
			})
			return
		}
		if info, ok := CheckBudget(count, tc.Name, 0 /*list depth=0 per Phase 68 LIMIT-02*/, h.responseMemoryLimit); !ok {
			slog.WarnContext(r.Context(), "pdbcompat: response budget exceeded",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.Int("count", info.Count),
				slog.Int64("estimated_bytes", info.EstimatedBytes),
				slog.Int64("budget_bytes", info.BudgetBytes),
				slog.Int("max_rows", info.MaxRows),
			)
			WriteBudgetProblem(w, r.URL.Path, info)
			return
		}
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

	// Stream via Plan 01's StreamListResponse (replaces legacy
	// WriteResponse). Meta envelope stays as struct{}{} for on-the-wire
	// parity with the legacy path. iterFromSlice is a half-step toward
	// true cursor-based streaming: a future plan flips tc.List to a
	// pull-iterator and serveList is unaffected.
	//
	// If streaming fails mid-response, bytes are already committed to
	// the wire — no way to issue a 500/problem-detail. Log for operator
	// visibility and drop the connection by returning (Go's net/http
	// closes the response on handler return).
	iter := iterFromSlice(results)
	if err := StreamListResponse(r.Context(), w, struct{}{}, iter); err != nil {
		slog.ErrorContext(r.Context(), "pdbcompat: stream encode failed mid-response",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
		return
	}
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
