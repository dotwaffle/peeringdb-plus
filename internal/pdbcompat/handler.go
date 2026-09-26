package pdbcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"

	"entgo.io/ent/dialect/sql"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// Handler serves PeeringDB-compatible API endpoints.
type Handler struct {
	client *ent.Client
	// responseMemoryLimit is the per-response byte budget consumed by
	// the pre-flight CheckBudget gate. 0 disables the
	// check entirely — documented local-dev / test escape hatch. See
	// cmd/peeringdb-plus/main.go for the Config.ResponseMemoryLimit
	// wiring (default 128 MiB).
	responseMemoryLimit int64

	// inflightBytes tracks the summed estimated bytes of list AND detail
	// responses admitted by CheckBudget and not yet finished serving.
	// The per-request budget alone admits each request in isolation, so
	// two concurrent near-budget dumps — each individually under the
	// limit — could jointly materialize ~2x the budget and OOM a 256 MB
	// replica. Admission charges the estimate here (lists: the CheckBudget
	// figure; depth>=2 details: the child-count fan-out from
	// detailInflightEstimate) and rejects with 503 + Retry-After when the
	// pool would exceed responseMemoryLimit; serveList / serveDetail
	// release the charge on return.
	inflightBytes atomic.Int64

	// listDepthChunk is the number of rows that a list at depth > 0 of
	// a type with reverse sets loads and renders at a time
	// (defaultListDepthChunk). Tests set a smaller value.
	listDepthChunk int
}

// NewHandler creates a Handler for PeeringDB-compatible API endpoints.
// responseMemoryLimit is the per-response byte budget consumed by the
// pre-flight CheckBudget gate. Pass 0 to disable the
// budget check (local dev / tests only; operators ship a non-zero
// PDBPLUS_RESPONSE_MEMORY_LIMIT in prod — default 128 MiB).
func NewHandler(client *ent.Client, responseMemoryLimit int64) *Handler {
	return &Handler{
		client:              client,
		responseMemoryLimit: responseMemoryLimit,
		listDepthChunk:      defaultListDepthChunk,
	}
}

// Register sets up PeeringDB-compatible routes on the given mux.
// Routes follow PeeringDB's URL patterns: /api/{type}, /api/{type}/{id}.
// Both with and without trailing slash variants are handled.
// The index endpoint at /api/ lists all available types.
func (h *Handler) Register(mux *http.ServeMux) {
	// Single wildcard pattern handles all /api/ sub-paths including the
	// index itself. Go 1.22+ {rest...} wildcard matches the empty string
	// for /api/ requests, so index, list, and detail are all dispatched
	// from one registration point.
	mux.HandleFunc("GET /api/{rest...}", h.dispatch)
	// Every other method reaches the method-less pattern: GET (and HEAD,
	// which the mux serves with the GET pattern) is more specific.
	mux.HandleFunc("/api/{rest...}", h.methodNotAllowed)
}

// methodNotAllowed answers a method other than GET and HEAD. The mirror
// is read-only, so every such method gets 405 with the DRF text
// (views.py:167-172, exceptions.py:194-196). Upstream lists its write
// methods in Allow and runs the write handlers (docs/API.md § Known
// Divergences). The mux sets no Allow header for this pattern, so the
// handler sets it. The as_set lookup serves only GET upstream (2.83.0
// rest.py:1399), so its paths get Allow: GET.
func (h *Handler) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	allow := "GET, HEAD"
	if typeName, _ := splitTypeID(r.PathValue("rest")); typeName == asSetPath {
		allow = "GET"
	}
	w.Header().Set("Allow", allow)
	// Concatenate: %q would escape the method a second time.
	writeError(w, r, apiError{
		Status: http.StatusMethodNotAllowed,
		Detail: "Method \"" + r.Method + "\" not allowed.",
	})
}

// dispatch routes requests under /api/ to index, list, or detail handlers
// based on the URL path structure.
func (h *Handler) dispatch(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")

	// Parse the rest path: "", "{type}", "{type}/", or "{type}/{id}".
	typeName, idStr := splitTypeID(rest)

	if typeName == "" {
		// /api/ or /api -- serve the index.
		h.serveIndex(w, r)
		return
	}

	// A route that is not a Registry type and reads the raw idStr must
	// branch here, before the Registry lookup: the Registry detail path
	// below turns an id that is not an integer into a 404. The as_set
	// lookup parses its own ASN and takes its own heap-delta sample.
	if typeName == asSetPath {
		h.serveASSet(w, r, idStr)
		return
	}

	// Validate type name against Registry.
	tc, ok := Registry[typeName]
	if !ok {
		writeError(w, r, apiError{
			Status: http.StatusNotFound,
			Detail: fmt.Sprintf("unknown type %q", typeName),
		})
		return
	}

	// Per-request heap-delta sampler — covers BOTH the list and detail
	// paths (it used to live in serveList only, leaving detail requests
	// unobserved). Samples HeapInuse once here and once at handler exit
	// (via defer); emits OTel span attribute
	// pdbplus.response.heap_delta_bytes and a Prometheus histogram
	// observation (pdbplus.response.heap_delta). runtime.ReadMemStats is
	// STW but acceptable once per request. MUST NOT be called per-row.
	//
	// Fires on EVERY terminal path (200 success, 400 bad parameter or
	// filter, 404, 413 budget-exceeded, 500 query-error, 503
	// pool-exhausted): that's the point of a defer; observing the small
	// delta of a cheap error path gives us a noise floor reference. The
	// index path above is not sampled: it has no entity label and serves
	// a constant-size body.
	startHeapBytes := memStatsHeapInuseBytes()
	defer recordResponseHeapDelta(r.Context(), r.URL.Path, tc.Name, startHeapBytes)

	if idStr == "" {
		// List endpoint: /api/{type} or /api/{type}/
		h.serveList(tc, w, r)
		return
	}

	// Detail endpoint: /api/{type}/{id}. An id with a "." or a "/" is
	// a 404 before any parameter check. Upstream routes "1.5" as pk "1"
	// with format suffix "5", and the renderer negotiation raises Http404
	// in initial(), before get_queryset (drf routers.py:143,
	// urlpatterns.py:109, negotiation.py:80-88, views.py:408-411).
	// Upstream has no GET route for a path with more segments: it sends
	// its HTML 404 page, or 405 on a POST-only action path such as
	// /api/ix/<id>/request_ixf_import (rest.py:209-228, :1033-1191).
	// serveDetail parses any other id after the parameter checks.
	if strings.ContainsAny(idStr, "./") {
		writeDetailNotFound(w, r, detailSliceNotFound)
		return
	}
	h.serveDetail(tc, idStr, w, r)
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

// serveIndex writes the API index in upstream PeeringDB's shape:
//
//	{"data": [{"<type>": "<absolute-url>", ...}], "meta": {}}
//
// a single object mapping every mirrored type, and the as_set lookup, to
// its absolute list-endpoint URL, built from the request scheme + host.
// Upstream lists as_set last in router order (rest.py:1598); the Go map
// encodes the keys in sorted order, and JSON object order has no meaning.
func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "" {
		scheme = xfp
	} else if r.TLS == nil {
		scheme = "http"
	}
	base := scheme + "://" + r.Host + "/api/"

	types := make(map[string]string, len(Registry)+1)
	for name := range Registry {
		types[name] = base + name
	}
	types[asSetPath] = base + asSetPath
	body := struct {
		Data []map[string]string `json:"data"`
		Meta map[string]any      `json:"meta"`
	}{
		Data: []map[string]string{types},
		Meta: map[string]any{},
	}
	b, _ := json.Marshal(body)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Powered-By", poweredByHeader)
	_, _ = w.Write(b)
}

// serveList handles list requests for the given type. The per-request
// heap-delta sampler lives in dispatch (shared with serveDetail).
func (h *Handler) serveList(tc TypeConfig, w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	unique := isUniqueQuery(tc.Name, params)

	// Parse skip, limit (2.83.0 rest.py:511-518), since (:505-510) and
	// depth (:520-523) before the filters, as upstream runs its filter
	// loop after them (:564-683). Upstream checks since before skip, so
	// for ?since=abc&skip=abc it names since and the mirror names skip.
	// Both are 400.
	limit, skip, err := ParsePaginationParams(params)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}
	since, err := ParseSinceParam(params)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}
	// A list defaults to depth 0 (upstream default_depth(is_list=True),
	// serializers.py:1032-1039). The raw value decides the truncation
	// and prints in its message (rest.py:766-772).
	depth, depthText, _, err := ParseDepthParam(params)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	// Parse filters. The emptyResult short-circuit handles ?field__in=.
	lf, err := parseRequestFilters(r, params, tc)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: fmt.Sprintf("filter error: %v", err),
		})
		return
	}

	// A negative skip is a 400. Upstream raises it at the slice (2.83.0
	// rest.py:757-760, Django query.py:403-417), after the filters and
	// since, and before the serializer. So a filter error wins, and the
	// unique-query 404 never fires.
	// A name_search that matches no row is the exception: upstream
	// returns qset.none() before the slice (rest.py:550-553), so the
	// result is empty.
	if skip < 0 {
		miss, err := h.nameSearchMisses(r.Context(), tc, lf)
		if err != nil {
			slog.ErrorContext(r.Context(), "pdbcompat: list name_search query failed",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.String("error", err.Error()),
			)
			writeError(w, r, apiError{
				Status: http.StatusInternalServerError,
				Detail: "failed to query matching records",
			})
			return
		}
		if !miss {
			writeError(w, r, apiError{
				Status: http.StatusBadRequest,
				Detail: errNegativeSkip.Error(),
			})
			return
		}
		lf.emptyResult = true
		lf.preds = nil
		skip = 0
	}

	// Parse search (?q=).
	q := params.Get("q")
	if q != "" {
		var sp func(*sql.Selector)
		if tc.Name == peeringdb.TypeNet {
			sp = buildNetworkSearchPredicate(q, tc.SearchFields)
		} else {
			sp = buildSearchPredicate(q, tc.SearchFields)
		}
		if sp != nil {
			lf.preds = append(lf.preds, sp)
		}
	}

	// Parse field projection (?fields=).
	var fields []string
	if f := params.Get("fields"); f != "" {
		fields = strings.Split(f, ",")
	}

	// A negative limit serves every row, as limit=0 does: upstream
	// slices only when limit > 0 (2.83.0 rest.py:757-760). The budget
	// check below prices the full count.
	// OrderBy is the sort key of a fac or org distance search.
	opts := QueryOptions{
		Filters:     lf.preds,
		Limit:       max(limit, 0),
		Skip:        skip,
		Since:       since,
		EmptyResult: lf.emptyResult,
		OrderBy:     lf.orderBy,
	}

	// A list at depth > 0 is cut to apiDepthRowLimit rows when upstream
	// would serve it from its live query, not from its API cache
	// (depthListIsLive).
	live := depth > 0 && depthListIsLive(lf, params, q)
	if span := trace.SpanFromContext(r.Context()); span.SpanContext().IsValid() && depth != 0 {
		span.SetAttributes(attribute.Int("pdbplus.list.depth", depth))
	}
	if depth > 0 && tc.ListDepth != nil {
		h.serveListDepth(tc, w, r, opts, listDepthRequest{
			depth:     min(depth, 2),
			depthText: depthText,
			live:      live,
			unique:    unique,
			fields:    fields,
		})
		return
	}

	// The 7 types without reverse sets serve the same rows at every
	// depth, as upstream (list_exclude, 2.83.0 serializers.py:1286-1290),
	// so only the truncation applies.
	meta := any(struct{}{})
	count, counted := 0, false
	if live {
		count, err = tc.Count(r.Context(), h.client, opts)
		if err != nil {
			h.writeCountError(w, r, tc, err)
			return
		}
		counted = true
		if count > apiDepthRowLimit {
			count = apiDepthRowLimit
			opts.Limit = apiDepthRowLimit
			meta = truncatedMeta(depthText)
		}
	}
	if depth > 0 {
		setListDepthAttrs(r.Context(), meta)
	}

	// Pre-flight budget check.
	//
	// Runs BEFORE tc.List so an over-budget response 413s without
	// committing to the expensive .All(ctx) + serialise path. Two
	// explicit bypass conditions:
	//
	//   - h.responseMemoryLimit <= 0: budget disabled (local dev / tests).
	//     CheckBudget already treats budget<=0 as "always fits" but we
	//     avoid the round-trip to COUNT(*) for the dev path.
	//   - emptyResult: the empty-__in short-circuit (?asn__in=). The
	//     result is known-empty; counting 0 rows and then streaming []
	//     is wasted work and would paper over a broken CountFunc.
	//
	// A list row of these types has the depth-0 size at every depth, so
	// the check bills TypicalRowBytes at depth 0.
	if h.responseMemoryLimit > 0 && !lf.emptyResult && tc.Count != nil {
		if !counted {
			count, err = tc.Count(r.Context(), h.client, opts)
			if err != nil {
				h.writeCountError(w, r, tc, err)
				return
			}
		}
		info, ok := CheckBudget(count, tc.Name, 0, h.responseMemoryLimit)
		if !ok {
			slog.WarnContext(r.Context(), "pdbcompat: response budget exceeded",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.Int("count", info.Count),
				slog.Int64("estimated_bytes", info.EstimatedBytes),
				slog.Int64("budget_bytes", info.BudgetBytes),
				slog.Int("max_rows", info.MaxRows),
			)
			writeBudgetError(w, r, info)
			return
		}

		// Global admission: the per-request check above treats each
		// request in isolation, but the budget is a process-wide memory
		// envelope — concurrent near-budget dumps must not stack past
		// it. Charge the estimate CheckBudget already computed (a single
		// pricing source — recomputing count × row size here could drift
		// from the 413 math) against the shared in-flight pool and
		// reject with 503 + Retry-After when the pool would overflow;
		// the charge is released when serveList returns (response fully
		// serialized, slices unreachable).
		estimate := info.EstimatedBytes
		if pooled := h.inflightBytes.Add(estimate); pooled > h.responseMemoryLimit {
			h.inflightBytes.Add(-estimate)
			slog.WarnContext(r.Context(), "pdbcompat: concurrent budget pool exhausted",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.Int64("estimated_bytes", estimate),
				slog.Int64("pooled_bytes", pooled),
				slog.Int64("budget_bytes", h.responseMemoryLimit),
			)
			w.Header().Set("Retry-After", "1")
			writeError(w, r, apiError{
				Status: http.StatusServiceUnavailable,
				Detail: "server is serving other large responses; retry shortly",
			})
			return
		}
		defer h.inflightBytes.Add(-estimate)
		// A budget count of 0 means no rows will be served —
		// a genuinely empty match, or a ?skip= past the end of the result
		// set. Stream the empty list now rather than calling tc.List,
		// which would run ORDER BY + OFFSET over the whole matching set
		// only to discard every row. An unbounded ?skip= otherwise turns a
		// 0-byte response into a full sort that the served-row budget
		// (servedRowCount = max(total-skip,0)) cannot see. The COUNT(*)
		// above is cheap and bounded; the sort is not.
		if count == 0 {
			if unique {
				writeEntityNotFound(w, r)
				return
			}
			if err := StreamListResponse(r.Context(), w, struct{}{}, iterFromSlice(nil)); err != nil {
				slog.ErrorContext(r.Context(), "pdbcompat: stream encode failed mid-response",
					slog.String("endpoint", r.URL.Path),
					slog.String("type", tc.Name),
					slog.String("error", err.Error()),
				)
			}
			return
		}
	}

	results, err := tc.List(r.Context(), h.client, opts)
	if err != nil {
		// Log raw error server-side only; keep the client Detail generic
		// so ent/SQL internals never reach the /api wire (SEC).
		slog.ErrorContext(r.Context(), "pdbcompat: list query failed",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
		writeError(w, r, apiError{
			Status: http.StatusInternalServerError,
			Detail: "failed to query matching records",
		})
		return
	}

	// An empty result also covers the empty-__in short-circuit, which
	// skips the budget count above.
	if len(results) == 0 && unique {
		writeEntityNotFound(w, r)
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
	// the wire, so there is no way to issue a 500 error body. Log for operator
	// visibility and drop the connection by returning (Go's net/http
	// closes the response on handler return).
	iter := iterFromSlice(results)
	if err := StreamListResponse(r.Context(), w, meta, iter); err != nil {
		slog.ErrorContext(r.Context(), "pdbcompat: stream encode failed mid-response",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
		return
	}
}

// apiDepthRowLimit is upstream API_DEPTH_ROW_LIMIT: a live list at
// depth > 0 serves at most this many rows (2.83.0 rest.py:484,
// settings/__init__.py:1512).
const apiDepthRowLimit = 250

// truncatedFormat is the upstream meta.truncated text (2.83.0
// rest.py:771). The first verb is the raw depth of the request.
const truncatedFormat = "Your search query (with depth %s) returned more than %d rows and has been truncated. Please be more specific in your filters, use the limit and skip parameters to page through the resultset or drop the depth parameter"

// truncatedMeta returns the meta object of a truncated depth list.
func truncatedMeta(depthText string) map[string]string {
	return map[string]string{"truncated": fmt.Sprintf(truncatedFormat, depthText, apiDepthRowLimit)}
}

// depthListIsLive reports whether upstream serves a list at depth > 0
// from its live query, which it cuts to apiDepthRowLimit rows, and not
// from its API cache file, which it never cuts (2.83.0 rest.py:705-709,
// :766-772, api_cache.py:90-124). The cache path needs no filter, no
// non-zero since and no query adjustment (lf.upstreamFilter). A key that
// the mirror ignores and upstream filters (for example org_flags or
// fac?ix_side_set__asn=) does not count, so the mirror serves such a
// list whole (see docs/API.md § Known Divergences). ?q= is a mirror
// extension that filters, so it counts.
func depthListIsLive(lf listFilters, params url.Values, q string) bool {
	return lf.upstreamFilter || sinceIsNonZero(params) || q != ""
}

// sinceIsNonZero reports whether ?since= holds a value other than 0.
// The upstream cache gate reads int(since), so a negative value also
// makes the list live (2.83.0 api_cache.py:80, :109-110), while the
// since matrix applies only for a value above 0 (rest.py:719).
func sinceIsNonZero(params url.Values) bool {
	n, present, err := parseSince(params)
	return present && err == nil && n != 0
}

// setListDepthAttrs records on the request span whether a list at
// depth > 0 was cut to apiDepthRowLimit rows. No-op without a span.
func setListDepthAttrs(ctx context.Context, meta any) {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return
	}
	_, truncated := meta.(map[string]string)
	span.SetAttributes(attribute.Bool("pdbplus.list.truncated", truncated))
}

// writeCountError logs a failed count query and writes a 500. The
// client text stays generic, so ent and SQL errors never reach the /api
// wire.
func (h *Handler) writeCountError(w http.ResponseWriter, r *http.Request, tc TypeConfig, err error) {
	slog.ErrorContext(r.Context(), "pdbcompat: count query failed",
		slog.String("endpoint", r.URL.Path),
		slog.String("type", tc.Name),
		slog.String("error", err.Error()),
	)
	writeError(w, r, apiError{
		Status: http.StatusInternalServerError,
		Detail: "failed to count matching records",
	})
}

// writeListQueryError logs a failed list query and writes a 500 with a
// generic text.
func (h *Handler) writeListQueryError(w http.ResponseWriter, r *http.Request, tc TypeConfig, err error) {
	slog.ErrorContext(r.Context(), "pdbcompat: list query failed",
		slog.String("endpoint", r.URL.Path),
		slog.String("type", tc.Name),
		slog.String("error", err.Error()),
	)
	writeError(w, r, apiError{
		Status: http.StatusInternalServerError,
		Detail: "failed to query matching records",
	})
}

// admitInflight charges estimate to the shared in-flight pool. When the
// pool would exceed the budget, it writes 503 with Retry-After: 1 and
// returns false. The caller releases the charge with
// h.inflightBytes.Add(-estimate) when the response is written.
func (h *Handler) admitInflight(w http.ResponseWriter, r *http.Request, tc TypeConfig, estimate int64, depth int) bool {
	pooled := h.inflightBytes.Add(estimate)
	if pooled <= h.responseMemoryLimit {
		return true
	}
	h.inflightBytes.Add(-estimate)
	slog.WarnContext(r.Context(), "pdbcompat: concurrent budget pool exhausted",
		slog.String("endpoint", r.URL.Path),
		slog.String("type", tc.Name),
		slog.Int("depth", depth),
		slog.Int64("estimated_bytes", estimate),
		slog.Int64("pooled_bytes", pooled),
		slog.Int64("budget_bytes", h.responseMemoryLimit),
	)
	w.Header().Set("Retry-After", "1")
	writeError(w, r, apiError{
		Status: http.StatusServiceUnavailable,
		Detail: "server is serving other large responses; retry shortly",
	})
	return false
}

// writeBudgetExceeded logs a 413 and writes it.
func writeBudgetExceeded(w http.ResponseWriter, r *http.Request, tc TypeConfig, info BudgetExceeded) {
	slog.WarnContext(r.Context(), "pdbcompat: response budget exceeded",
		slog.String("endpoint", r.URL.Path),
		slog.String("type", tc.Name),
		slog.Int("depth", info.Depth),
		slog.Int("count", info.Count),
		slog.Int64("estimated_bytes", info.EstimatedBytes),
		slog.Int64("budget_bytes", info.BudgetBytes),
		slog.Int("max_rows", info.MaxRows),
	)
	writeBudgetError(w, r, info)
}

// listDepthRequest holds the parsed parameters of a list at depth > 0.
type listDepthRequest struct {
	depth     int // render depth: 1, or 2 for any value of 2 or more
	depthText string
	live      bool // depthListIsLive
	unique    bool // isUniqueQuery
	fields    []string
}

// streamEmptyList writes the response of a list that serves no row: the
// unique-query 404, or an empty data array with meta.
func streamEmptyList(w http.ResponseWriter, r *http.Request, tc TypeConfig, unique bool, meta any) {
	if unique {
		writeEntityNotFound(w, r)
		return
	}
	if err := StreamListResponse(r.Context(), w, meta, iterFromSlice(nil)); err != nil {
		slog.ErrorContext(r.Context(), "pdbcompat: stream encode failed mid-response",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
	}
}

// serveListDepth serves a list at depth > 0 of a type with reverse sets
// (org, net, ix, ixlan, carrier, campus). Each row carries the sets of
// req.fields (all of them without ?fields=): at depth 1 as id lists, at
// depth 2 as flat objects, and no forward FK object (list_depth.go).
//
// Steps:
//  1. When the list is live or the budget is on, count the served rows.
//     A live list of more than apiDepthRowLimit rows is cut to that many
//     rows with meta.truncated (2.83.0 rest.py:757-772: upstream slices
//     by skip and limit first, then counts and cuts). With the budget
//     on, the flat Depth0 check runs over the count before any id is
//     read.
//  2. Read the served ids (ListIDs).
//  3. With the budget on, price the most expensive chunk of
//     h.listDepthChunk ids (listDepthEstimate). The figure feeds the 413
//     check and the in-flight pool.
//  4. Load the first chunk before the first byte, so its query errors
//     are a clean 500. Stream the rows: each row renders when the stream
//     pulls it, and the next chunk loads after the last row of a chunk.
//     A later chunk error drops the connection, as a mid-stream encode
//     error does.
//
// No read transaction spans the chunks: a long read transaction on a
// replica holds up the LiteFS apply. A row deleted after step 2 is left
// out of its chunk.
func (h *Handler) serveListDepth(tc TypeConfig, w http.ResponseWriter, r *http.Request, opts QueryOptions, req listDepthRequest) {
	ctx := r.Context()
	budget := h.responseMemoryLimit
	meta := any(struct{}{})
	if budget > 0 || req.live {
		count, err := tc.Count(ctx, h.client, opts)
		if err != nil {
			h.writeCountError(w, r, tc, err)
			return
		}
		if req.live && count > apiDepthRowLimit {
			// qset[skip:skip+limit][:250] is qset[skip:skip+250] here:
			// the count is over 250 only when limit is 0 or above 250.
			count = apiDepthRowLimit
			opts.Limit = apiDepthRowLimit
			meta = truncatedMeta(req.depthText)
		}
		if count == 0 {
			streamEmptyList(w, r, tc, req.unique, meta)
			return
		}
		if budget > 0 {
			if info, ok := CheckBudget(count, tc.Name, 0, budget); !ok {
				writeBudgetExceeded(w, r, tc, info)
				return
			}
		}
	}
	setListDepthAttrs(ctx, meta)

	sets := selectSets(tc.Name, req.fields)
	ids, err := tc.ListIDs(ctx, h.client, opts)
	if err != nil {
		h.writeListQueryError(w, r, tc, err)
		return
	}
	if len(ids) == 0 {
		streamEmptyList(w, r, tc, req.unique, meta)
		return
	}
	chunk := max(h.listDepthChunk, 1)
	if budget > 0 {
		cost, err := listDepthEstimate(ctx, h.client, tc.Name, ids, req.depth, sets, chunk)
		if err != nil {
			slog.ErrorContext(ctx, "pdbcompat: list depth estimate failed",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.String("error", err.Error()),
			)
			writeError(w, r, apiError{
				Status: http.StatusInternalServerError,
				Detail: "failed to count matching records",
			})
			return
		}
		if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
			span.SetAttributes(attribute.Int64("pdbplus.list.estimated_bytes", cost.bytes))
		}
		if cost.bytes > budget {
			writeBudgetExceeded(w, r, tc, cost.exceeded(len(ids), budget, tc.Name, req.depth))
			return
		}
		if !h.admitInflight(w, r, tc, cost.bytes, req.depth) {
			return
		}
		defer h.inflightBytes.Add(-cost.bytes)
	}

	end := min(chunk, len(ids))
	pending, err := tc.ListDepth(ctx, h.client, opts, ids[:end], req.depth, sets)
	if err != nil {
		h.writeListQueryError(w, r, tc, err)
		return
	}
	if len(pending) == 0 && end == len(ids) {
		// Every row changed or went away after ListIDs.
		streamEmptyList(w, r, tc, req.unique, meta)
		return
	}
	chunks := 1
	iter := func() (any, bool, error) {
		for len(pending) == 0 {
			if end >= len(ids) {
				return nil, false, nil
			}
			next := min(end+chunk, len(ids))
			rows, err := tc.ListDepth(ctx, h.client, opts, ids[end:next], req.depth, sets)
			if err != nil {
				return nil, false, err
			}
			end = next
			pending = rows
			chunks++
		}
		render := pending[0]
		pending[0] = nil
		pending = pending[1:]
		row := render()
		if len(req.fields) > 0 {
			row = applyFieldProjection([]any{row}, req.fields)[0]
		}
		return row, true, nil
	}
	err = StreamListResponse(ctx, w, meta, iter)
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		span.SetAttributes(attribute.Int("pdbplus.list.chunks", chunks))
	}
	if err != nil {
		slog.ErrorContext(ctx, "pdbcompat: stream encode failed mid-response",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
	}
}

// parseRequestFilters parses the filter keys of a list or detail
// request and records the keys that it ignores. The list and the detail
// share it, so a detail applies the same filters as a list (upstream
// get_queryset, 2.83.0 rest.py:477-703). TypeConfig is threaded so that
// shadow-column routing can consult tc.FoldedFields. The error is not
// wrapped: each caller writes it as a 400 "filter error: ...".
//
// The ctx carries an unknown-field accumulator, so operators can observe
// the silently ignored filter keys via slog DEBUG and an OTel span
// attribute. ParseFiltersCtx writes to the accumulator, and the
// diagnostics are emitted AFTER it returns, so the response does not
// change (HTTP 200, no 400).
//
// A detail request uses only the predicates and the empty-result flag
// of the result, not its sort key.
func parseRequestFilters(r *http.Request, params url.Values, tc TypeConfig) (listFilters, error) {
	ctx := WithUnknownFields(r.Context())
	lf, err := parseListFilters(ctx, params, tc)
	if err != nil {
		return listFilters{}, err
	}
	if unknown := UnknownFieldsFromCtx(ctx); len(unknown) > 0 {
		csv := strings.Join(unknown, ",")
		slog.DebugContext(ctx, "pdbcompat: unknown filter fields silently ignored",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("unknown_fields", csv),
		)
		// OTel span attribute: no-op when no active span (OTel not
		// configured in tests) because SpanFromContext returns a noop
		// span whose SetAttributes is safe and SpanContext().IsValid()
		// is false.
		if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
			span.SetAttributes(attribute.String("pdbplus.filter.unknown_fields", csv))
		}
	}
	return lf, nil
}

// isUniqueQuery reports whether a list request names one object, so
// that an empty result is a 404 instead of an empty list. It mirrors
// upstream is_unique_query: the "id" key on every type (2.83.0
// serializers.py:962-967), and also the "asn" key on net
// (serializers.py:3815-3820). Only the key counts, whatever its value
// and whatever the other filters, skip, limit and since are.
//
// Upstream skips the 404 when ?page= applies, because the response
// data is then the pagination object and never empty (rest.py:799-815,
// pagination.py:35-50). The mirror does not implement ?page=, but it
// keeps this exception so that a request with ?page= gets the same
// status as upstream.
func isUniqueQuery(typeName string, params url.Values) bool {
	if params.Has("page") {
		return false
	}
	return params.Has("id") || (typeName == peeringdb.TypeNet && params.Has("asn"))
}

// writeEntityNotFound writes the 404 for a unique list query that
// matched no row: {"data": [], "meta": {"error": "Entity not found"}},
// as upstream (2.83.0 rest.py:809-815 puts "data": [] in the response
// data, and renderers.py:134-148 keeps it next to meta.error). It is the
// only /api/ error body with a "data" key.
func writeEntityNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, apiError{
		Status:    http.StatusNotFound,
		Detail:    "Entity not found",
		EmptyData: true,
	})
}

// writeDetailNotFound writes the 404 of a detail request: no row with
// the id, a row outside the detail status set, a row that the caller's
// tier cannot read, a row that a filter excludes, or a sliced request.
// Upstream sends a 404 for all of them (Django get_object_or_404 and
// DRF get_object_or_404, 2.83.0 rest.py:849-855). The body has no
// "data" key.
func writeDetailNotFound(w http.ResponseWriter, r *http.Request, detail string) {
	writeError(w, r, apiError{
		Status: http.StatusNotFound,
		Detail: detail,
	})
}

// missMessage is the 404 detail of a PK miss or a filter miss on a
// detail request: the Django text with the upstream model name
// (django/shortcuts.py:90-93), for example "No Network matches the
// given query.". A hidden poc gets the same text as a missing one.
func missMessage(tc TypeConfig) string {
	model, ok := pdbtypes.DjangoModelOf(tc.Name)
	if !ok {
		model = tc.Name
	}
	return "No " + model + " matches the given query."
}

// detailSliceNotFound is the 404 detail of a detail request with a
// limit or skip above 0, or with an id that is not an integer. DRF turns
// the TypeError of the sliced get(), and the ValueError of int() on the
// pk (django/db/models/fields/__init__.py:2123-2131), into a bare Http404
// (generics.py:13-21), which renders the NotFound default text
// (exceptions.py:188-191). The format-suffix route of an id with a "."
// raises the same Http404 (negotiation.py:80-88).
const detailSliceNotFound = "Not found."

// nameSearchMisses reports whether the name_search of a request
// matches no row: upstream qset.none(), which get_queryset returns
// before it slices the query (2.83.0 rest.py:550-553, :755-760). So it
// decides the response of a request with a slice or a negative skip.
// lf.none is known without a query. A search that runs sends one query
// (LIMIT 1) for the ok rows that it matches. Without a name_search the
// result is false.
func (h *Handler) nameSearchMisses(ctx context.Context, tc TypeConfig, lf listFilters) (bool, error) {
	if lf.none {
		return true, nil
	}
	if lf.searchHit == nil {
		return false, nil
	}
	rows, err := tc.List(ctx, h.client, QueryOptions{
		Filters: []func(*sql.Selector){lf.searchHit},
		Limit:   1,
	})
	if err != nil {
		return false, err
	}
	return len(rows) == 0, nil
}

// serveDetail handles detail requests for a single object by ID.
//
// Default detail depth is 2 (matches upstream PeeringDB 2.83.0
// peeringdb_server/serializers.py:1032-1039 — `default_depth(is_list=False)`
// returns 2 for single-object GETs versus 0 for lists). This causes
// `prefetch_related` (rest.py:774-777) to fire on every bare detail URL,
// embedding the per-type `_set` collections + parent FK objects (`org`,
// `campus`, etc.) that upstream always returns on `/api/<type>/<id>`.
// Explicit `?depth=0` short-circuits the prefetch (serializers.py:1068-1069
// returns the qset early when depth<=0) and yields a bare row, matching
// upstream's behaviour for that explicit override.
//
// Generalises commit 0d39654 (which fixed the IX `fac_set` shape at
// `?depth=2`) across all detail endpoints AND extends it to fire at
// the depth=0 default (which was previously skipping the prefetch
// chain entirely on bare detail URLs).
//
// A detail request applies the filter keys of a list, as upstream:
// retrieve calls DRF get_object, which filters get_queryset() (2.83.0
// rest.py:849-855, :477-703). The parameters are parsed in the list
// order (skip, limit, since, depth, filters, negative skip), so a
// request with two bad parameters gets the same 400 on both paths.
// rawID is parsed after them, as upstream: get_object builds the
// filtered queryset before get_object_or_404 converts the pk with int()
// (drf generics.py:87-100). An id that int() rejects is a 404 (Not
// found.), also when a filter already emptied the result: Django
// converts the pk when it builds the lookup, on a none() queryset too.
// A name_search that matches no row is the miss 404 before the limit
// and skip checks, the negative skip 400 included, as upstream returns
// qset.none() before the slice (rest.py:550-553). On a type with a
// search index, a request with a limit or skip that is not 0 runs one
// query to find out if the search has a hit (nameSearchMisses).
// since is checked and then ignored (rest.py:718), and ?q= is ignored
// (rest.py:566). The filter check runs before the budget admission, so
// a filter miss costs one primary-key query and charges nothing.
func (h *Handler) serveDetail(tc TypeConfig, rawID string, w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	sliced, negativeSkip, err := parseDetailSlice(params)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}
	if _, err := ParseSinceParam(params); err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}

	// Parse depth. Default = 2 for detail endpoints to match upstream's
	// `default_depth(is_list=False)` (2.83.0 serializers.py:1032-1039).
	// Upstream parses `?depth=` as a raw int clamped to [0, max_depth] with
	// max_depth=4 for single GETs (serializers.py:1004-1030), so we honour
	// 0/1/2/3/4: 0 is the bare-row escape hatch (serializers.py:1068-1069),
	// 1 expands forward FKs flat with
	// reverse sets as ID lists, 2 fully expands. Depths >2 render the depth=2
	// shape (the deeper sub-level nesting they add is not reproduced). A
	// value that is not an integer is a 400, as upstream (rest.py:520-523);
	// negatives floor to 0.
	depth := 2
	rawDepth, _, depthPresent, err := ParseDepthParam(params)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: err.Error(),
		})
		return
	}
	if depthPresent {
		depth = min(max(rawDepth, 0), 4)
	}

	lf, err := parseRequestFilters(r, params, tc)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: fmt.Sprintf("filter error: %v", err),
		})
		return
	}
	filters := lf.preds
	// A name_search that matches no row is upstream qset.none(), which
	// get_queryset returns before it slices the query (2.83.0
	// rest.py:550-553, :755-760). So it is the miss 404, also with a
	// limit or skip above 0 or a negative skip. Only a request with a
	// slice or a negative skip needs the query of nameSearchMisses:
	// without them, Match applies the search.
	miss := lf.none
	if sliced || negativeSkip {
		miss, err = h.nameSearchMisses(r.Context(), tc, lf)
		if err != nil {
			slog.ErrorContext(r.Context(), "pdbcompat: detail name_search query failed",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.String("error", err.Error()),
			)
			writeError(w, r, apiError{
				Status: http.StatusInternalServerError,
				Detail: "failed to query record",
			})
			return
		}
	}
	if negativeSkip && !miss {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: errNegativeSkip.Error(),
		})
		return
	}
	// A saturated id is a PK miss, as upstream: Django answers an
	// out-of-range integer lookup with an empty result
	// (django/db/models/lookups.py:461-494).
	id, _, err := pyInt(rawID)
	if err != nil {
		writeDetailNotFound(w, r, detailSliceNotFound)
		return
	}
	if miss {
		writeDetailNotFound(w, r, missMessage(tc))
		return
	}
	if sliced {
		writeDetailNotFound(w, r, detailSliceNotFound)
		return
	}
	if lf.emptyResult {
		writeDetailNotFound(w, r, missMessage(tc))
		return
	}
	// A request without filter keys sends no extra query.
	if len(filters) > 0 {
		ok, err := tc.Match(r.Context(), h.client, id, filters)
		if err != nil {
			// Log raw error server-side only; the client Detail stays
			// generic so ent/SQL internals never reach the /api wire.
			slog.ErrorContext(r.Context(), "pdbcompat: detail filter query failed",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.String("error", err.Error()),
			)
			writeError(w, r, apiError{
				Status: http.StatusInternalServerError,
				Detail: "failed to query record",
			})
			return
		}
		if !ok {
			writeDetailNotFound(w, r, missMessage(tc))
			return
		}
	}

	// Gate the detail response against the same per-response
	// memory budget as the list path. A single object is one row, but at
	// depth=2 it embeds the per-type _set collections + parent FK objects,
	// so its size tracks TypicalRowBytes(<type>, depth), not the bare
	// Depth0 figure. This is a coarse floor — it bills the typical
	// expanded row, not the actual _set cardinality — but it keeps the
	// detail path symmetric with serveList and trips a clean 413 rather
	// than serving under a degenerately small budget. This is also the
	// only caller that uses the depth=2 row-size estimate: a list at
	// depth > 0 prices its rows with listDepthEstimate, from the child
	// counts and the Depth0 figures. budget<=0 disables the check
	// (dev/test) exactly as on the list path.
	if h.responseMemoryLimit > 0 {
		if info, ok := CheckBudget(1, tc.Name, depth, h.responseMemoryLimit); !ok {
			slog.WarnContext(r.Context(), "pdbcompat: detail response budget exceeded",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.Int("depth", depth),
				slog.Int64("estimated_bytes", info.EstimatedBytes),
				slog.Int64("budget_bytes", info.BudgetBytes),
			)
			writeBudgetError(w, r, info)
			return
		}

		// Global admission, mirroring serveList: the flat check above
		// treats the request in isolation AND bills only the typical
		// expanded row, but a depth>=2 detail embeds unbounded _set
		// collections — a hub org expands thousands of full network
		// objects, easily rivalling a large list response.
		// detailInflightEstimate prices that fan-out (child COUNT(*) ×
		// child Depth0 per embedded set) so concurrent hub-object dumps
		// cannot stack past the process-wide envelope. 503 + Retry-After
		// on overflow; the charge releases when serveDetail returns. The
		// deliberate 413-vs-actual-response-size deferral is untouched:
		// the fan-out estimate feeds only the pool, never the 413.
		estimate := detailInflightEstimate(r.Context(), h.client, tc.Name, id, depth)
		if pooled := h.inflightBytes.Add(estimate); pooled > h.responseMemoryLimit {
			h.inflightBytes.Add(-estimate)
			slog.WarnContext(r.Context(), "pdbcompat: concurrent budget pool exhausted",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.Int64("estimated_bytes", estimate),
				slog.Int64("pooled_bytes", pooled),
				slog.Int64("budget_bytes", h.responseMemoryLimit),
			)
			w.Header().Set("Retry-After", "1")
			writeError(w, r, apiError{
				Status: http.StatusServiceUnavailable,
				Detail: "server is serving other large responses; retry shortly",
			})
			return
		}
		defer h.inflightBytes.Add(-estimate)
	}

	// Parse field projection (?fields=).
	var fields []string
	if f := params.Get("fields"); f != "" {
		fields = strings.Split(f, ",")
	}

	result, err := tc.Get(r.Context(), h.client, id, depth)
	if err != nil {
		if ent.IsNotFound(err) {
			writeDetailNotFound(w, r, missMessage(tc))
			return
		}
		// Log raw error server-side only; the client Detail stays generic
		// so ent/SQL internals never reach the /api wire (SEC).
		slog.ErrorContext(r.Context(), "pdbcompat: detail query failed",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
		writeError(w, r, apiError{
			Status: http.StatusInternalServerError,
			Detail: "failed to query record",
		})
		return
	}

	// Single object wrapped in array.
	data := []any{result}

	// Apply field projection after retrieval.
	if len(fields) > 0 {
		data = applyFieldProjection(data, fields)
	}

	WriteResponse(w, data)
}
