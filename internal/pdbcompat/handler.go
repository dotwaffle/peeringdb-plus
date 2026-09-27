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
	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
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

	// syncClock is the time of meta.generated (SetSyncClock). nil sends
	// no meta.generated.
	syncClock *SyncClock

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
// Routes follow PeeringDB's URL patterns: /api/{type}, /api/{type}/{id}
// (parseAPIPath).
// The index endpoint at /api/ lists all available types.
func (h *Handler) Register(mux *http.ServeMux) {
	// Single wildcard pattern handles all /api/ sub-paths including the
	// index itself. Go 1.22+ {rest...} wildcard matches the empty string
	// for /api/ requests, so index, list, and detail are all dispatched
	// from one registration point.
	// prettyJSON indents the body of any of them for ?pretty.
	mux.Handle("GET /api/{rest...}", prettyJSON(http.HandlerFunc(h.dispatch)))
	// Every other method reaches the method-less pattern: GET (and HEAD,
	// which the mux serves with the GET pattern) is more specific.
	mux.Handle("/api/{rest...}", prettyJSON(http.HandlerFunc(h.methodNotAllowed)))
	// The mux would send a 307 for /api, so the path has its own route.
	mux.HandleFunc("/api", redirectAPIRoot)
}

// redirectAPIRoot answers /api with the redirect of the upstream
// PDBCommonMiddleware (2.83.0 middleware.py:175-190), a Django
// CommonMiddleware with APPEND_SLASH: a 301 for every method to the
// path with a "/" and the same query string, with an empty HTML body
// (Django HttpResponsePermanentRedirect). The Location is relative, as
// upstream sends it for its www host. Django quotes the query string
// with iri_to_uri (django/http/request.py:219-230).
func redirectAPIRoot(w http.ResponseWriter, r *http.Request) {
	writeDjangoRedirect(w, "/api/", r.URL.RawQuery, http.StatusMovedPermanently)
}

// methodNotAllowed answers a method other than GET and HEAD. The mirror
// is read-only, so every such method on a path that an upstream route
// matches gets 405 with the DRF text (views.py:167-172,
// exceptions.py:194-196). Upstream lists its write methods in Allow and
// runs the write handlers (docs/API.md § Known Divergences). The mux
// sets no Allow header for this pattern, so the handler sets it. The
// paths of getOnly types get Allow: GET.
//
// A path that no route matches is a 404 for every method, as in
// dispatch. The self and organization users routes answer every
// method themselves (serveSelf, serveOrgUsers). Content negotiation comes before the method check
// (negotiate).
func (h *Handler) methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	if tag, ok := selfTag(r.PathValue("rest")); ok {
		serveSelf(w, r, tag)
		return
	}
	if route, orgID, ok := matchOrgUsers(r.PathValue("rest")); ok {
		h.serveOrgUsers(w, r, route, orgID)
		return
	}
	typeName, _, format, routed := parseAPIPath(r.PathValue("rest"))
	_, known := Registry[typeName]
	switch {
	case !routed:
		writeDetailNotFound(w, r, detailSliceNotFound)
	case !known && typeName != "" && typeName != asSetPath:
		writeUnknownType(w, r, typeName)
	case !negotiate(w, r, format):
	case getOnly(typeName):
		writeMethodNotAllowed(w, r, "GET")
	default:
		writeMethodNotAllowed(w, r, "GET, HEAD")
	}
}

// writeUnknownType writes the 404 of a path that names no type. Upstream
// has no route for it and sends its HTML 404 page (docs/API.md § Known
// Divergences).
func writeUnknownType(w http.ResponseWriter, r *http.Request, typeName string) {
	writeError(w, r, apiError{
		Status: http.StatusNotFound,
		Detail: fmt.Sprintf("unknown type %q", typeName),
	})
}

// getOnly reports whether the upstream viewset of typeName leaves HEAD
// and OPTIONS out of http_method_names: ixlan maps GET and PUT (2.83.0
// rest.py:1358), as_set maps GET (:1404). DRF answers HEAD and OPTIONS
// there with 405 (views.py:513-521). The mirror does not list the
// write method PUT in Allow.
func getOnly(typeName string) bool {
	return typeName == peeringdb.TypeIXLan || typeName == asSetPath
}

// writeMethodNotAllowed writes the DRF 405 for the request method with
// the Allow header allow.
func writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, allow string) {
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
	// The self and organization users routes come before the router
	// routes upstream (rest.py:2087-2122). The self route matches
	// anywhere in the path (selfRoute).
	if tag, ok := selfTag(r.PathValue("rest")); ok {
		serveSelf(w, r, tag)
		return
	}
	if route, orgID, ok := matchOrgUsers(r.PathValue("rest")); ok {
		h.serveOrgUsers(w, r, route, orgID)
		return
	}
	typeName, idStr, format, routed := parseAPIPath(r.PathValue("rest"))

	// Upstream sends its HTML 404 page for a path that no route matches,
	// for example a path with a "/" at the end, more segments, or a "."
	// that is not a format suffix (docs/API.md § Known Divergences). A
	// POST-only action path such as /api/ix/<id>/request_ixf_import
	// (rest.py:209-228, :1033-1191) gets 405 there.
	if !routed {
		writeDetailNotFound(w, r, detailSliceNotFound)
		return
	}

	if typeName == "" {
		// /api/ or /api/.json -- serve the index. /api has its own route
		// (redirectAPIRoot).
		if !negotiate(w, r, format) {
			return
		}
		h.serveIndex(w, r)
		return
	}

	// A route that is not a Registry type and reads the raw idStr must
	// branch here, before the Registry lookup: the Registry detail path
	// below turns an id that is not an integer into a 404. The as_set
	// lookup parses its own ASN and takes its own heap-delta sample.
	if typeName == asSetPath {
		if !negotiate(w, r, format) {
			return
		}
		h.serveASSet(w, r, idStr)
		return
	}

	// Validate type name against Registry.
	tc, ok := Registry[typeName]
	if !ok {
		writeUnknownType(w, r, typeName)
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

	// Content negotiation runs in initial(), before get_queryset and
	// before the method check (negotiate). serveDetail parses the id
	// after the parameter checks.
	if !negotiate(w, r, format) {
		return
	}

	// DRF compares the method with http_method_names after initial()
	// and before the handler, so no parameter is read. net/http sends
	// no body for a HEAD response.
	if r.Method == http.MethodHead && getOnly(tc.Name) {
		writeMethodNotAllowed(w, r, "GET")
		return
	}

	if idStr == "" {
		// List endpoint: /api/{type}
		h.serveList(tc, w, r, format != "")
		return
	}
	h.serveDetail(tc, idStr, w, r)
}

// parseAPIPath matches a rest path, the part after /api/, with the
// upstream routes: a DefaultRouter with no trailing slash (2.83.0
// rest.py:185-230, :1305) has the index "", the list "<type>" and the
// detail "<type>/<id>", and each of them also with the format suffix of
// cutFormatSuffix, which alone may have one "/" after it. routed is
// false for any other path, for example "net/", "net/1/" or "net/1/2".
// For a routed path, typeName is the type and id is the id without the
// suffix.
func parseAPIPath(rest string) (typeName, id, format string, routed bool) {
	if rest == "" {
		return "", "", "", true
	}
	path, slash := strings.CutSuffix(rest, "/")
	typeName, id, detail := strings.Cut(path, "/")
	last := typeName
	if detail {
		last = id
	}
	base, format, ok := cutFormatSuffix(last)
	if !ok || (slash && format == "") {
		return typeName, id, "", false
	}
	if !detail {
		return base, "", format, true
	}
	if typeName == "" || base == "" || strings.Contains(base, "/") {
		return typeName, id, "", false
	}
	return typeName, base, format, true
}

// cutFormatSuffix splits the format suffix off the last segment of an
// /api/ path. Upstream registers its viewsets on a DefaultRouter
// (2.83.0 rest.py:185, :1305), which adds a route with the suffix
// \.(?P<format>[a-z0-9]+)/?$ to each route (drf routers.py
// include_format_suffixes, urlpatterns.py format_suffix_patterns).
// The type name and the lookup value ([^/.]+) have no ".". ok is false
// when seg has a "." in another form.
func cutFormatSuffix(seg string) (base, format string, ok bool) {
	base, format, found := strings.Cut(seg, ".")
	if !found {
		return seg, "", true
	}
	if format == "" || strings.Trim(format, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
		return seg, "", false
	}
	return base, format, true
}

// errNotAcceptable is the DRF NotAcceptable text (exceptions.py:205-208).
const errNotAcceptable = "Could not satisfy the request Accept header."

// negotiate runs the checks of DRF content negotiation, which initial()
// runs before the method check and before any parameter is read
// (views.py:408-411): a format other than json is a 404
// (formatAccepted, negotiation.py:80-88), and an Accept header with no
// media range that matches application/json is a 406 (acceptsJSON,
// negotiation.py:52-78). A header that names application/problem+json
// gets no 406 (docs/API.md § Known Divergences). negotiate writes the
// error and returns false when a check fails.
func negotiate(w http.ResponseWriter, r *http.Request, suffix string) bool {
	if !formatAccepted(suffix, r.URL.Query()) {
		writeDetailNotFound(w, r, detailSliceNotFound)
		return false
	}
	if !acceptsJSON(r.Header) && !httperr.WantsProblemJSON(r.Header) {
		writeError(w, r, apiError{Status: http.StatusNotAcceptable, Detail: errNotAcceptable})
		return false
	}
	return true
}

// acceptsJSON reports whether a media range of the Accept header
// matches application/json as DRF matches it (mediatypes.py
// _MediaType.match): the header is split at each ",", the type and
// subtype ignore case, "*" matches any type or subtype, and no
// parameter is read, q=0 included. Without an Accept header, DRF reads
// */*. An empty header matches nothing.
func acceptsJSON(h http.Header) bool {
	vals, ok := h["Accept"]
	if !ok {
		return true
	}
	for part := range strings.SplitSeq(strings.Join(vals, ","), ",") {
		full, _, _ := strings.Cut(part, ";")
		typ, sub, _ := strings.Cut(strings.ToLower(strings.TrimSpace(full)), "/")
		if (typ == "*" || typ == "application") && (sub == "*" || sub == "json") {
			return true
		}
	}
	return false
}

// formatAccepted reports whether DRF content negotiation accepts the
// format of a request: the format suffix, or else the last ?format=
// value (drf negotiation.py:44-45, Django QueryDict.get). The only
// upstream renderer has the format json (settings DEFAULT_RENDERER_CLASSES,
// renderers.py:76-86). Another format raises Http404
// (negotiation.py:80-88), and an empty value selects no format.
func formatAccepted(suffix string, params url.Values) bool {
	format := suffix
	if vals := params["format"]; format == "" && len(vals) > 0 {
		format = vals[len(vals)-1]
	}
	return format == "" || format == "json"
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
	base := requestScheme(r) + "://" + r.Host + "/api/"

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

	w.Header().Set("Content-Type", httperr.MetaJSONContentType)
	w.Header().Set("X-Powered-By", poweredByHeader)
	_, _ = w.Write(b)
}

// serveList handles list requests for the given type. The per-request
// heap-delta sampler lives in dispatch (shared with serveDetail).
//
// suffixed is set for a path with a format suffix, which upstream never
// serves from its API cache (depthListIsLive).
func (h *Handler) serveList(tc TypeConfig, w http.ResponseWriter, r *http.Request, suffixed bool) {
	params := r.URL.Query()
	unique := isUniqueQuery(tc.Name, params)

	// Parse the filters with since, skip, limit and depth, in upstream
	// order (parseRequest).
	lf, rp, ok := parseRequest(w, r, params, tc)
	if !ok {
		return
	}
	limit, skip, since := rp.limit, rp.skip, rp.since
	// A list defaults to depth 0 (upstream default_depth(is_list=True),
	// serializers.py:1032-1039). The raw value decides the truncation
	// and prints in its message (rest.py:766-772).
	depth, depthText := rp.depth, rp.depthText
	var err error

	// Two errors come after the filters, in this order. A since that
	// float() accepts and int() does not is a 400 with the Python
	// message: upstream parses since again with int() when it builds its
	// API cache loader (2.83.0 rest.py:707, api_cache.py:80). A negative
	// skip is a 400 at the slice (rest.py:757-760, Django
	// query.py:403-417). So a filter error wins over both, and the
	// unique-query 404 never fires.
	// A name_search that matches no row is the exception: upstream
	// returns qset.none() before both (rest.py:550-553), so the result
	// is empty.
	if rp.sinceInt != nil || skip < 0 {
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
			detail := errNegativeSkip.Error()
			if rp.sinceInt != nil {
				detail = rp.sinceInt.Error()
			}
			writeError(w, r, apiError{
				Status: http.StatusBadRequest,
				Detail: detail,
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
	fields := fieldsParam(params)

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
	live := depth > 0 && depthListIsLive(lf, params, q, suffixed)
	// A list that upstream serves from its API cache carries
	// meta.generated (servedFromCache).
	cached := servedFromCache(lf, params, q, suffixed, depth, limit)
	baseMeta := h.listMeta(cached)
	// ?page= narrows opts to the rows of the page and applies the depth
	// cut first, so the paths below serve the page as a list that is
	// not live.
	if value, ok := pageValue(params, cached); ok {
		baseMeta, ok = h.paginateList(w, r, tc, &opts, value, live, depthText, baseMeta)
		if !ok {
			return
		}
		live = false
	}
	if span := trace.SpanFromContext(r.Context()); span.SpanContext().IsValid() && depth != 0 {
		span.SetAttributes(attribute.Int("pdbplus.list.depth", depth))
	}
	if depth > 0 && tc.ListDepth != nil {
		r = r.WithContext(withSetDateFilter(r.Context(), lf.setDateFilter))
		h.serveListDepth(tc, w, r, opts, listDepthRequest{
			depth:     min(depth, 2),
			depthText: depthText,
			live:      live,
			unique:    unique,
			fields:    fields,
			meta:      baseMeta,
		})
		return
	}

	// The 7 types without reverse sets serve the same rows at every
	// depth, as upstream (list_exclude, 2.83.0 serializers.py:1286-1290),
	// so only the truncation applies.
	meta := baseMeta
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
			if err := StreamListResponse(r.Context(), w, meta, iterFromSlice(nil)); err != nil {
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

	// Stream via StreamListResponse. meta is an empty object, or holds
	// meta.truncated, meta.generated or meta.pagination. iterFromSlice is a half-step toward
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
// org?ix_set__name=) does not count, so the mirror serves such a
// list whole (see docs/API.md § Known Divergences). ?q= is a mirror
// extension that filters, so it counts. The cache also needs no URL
// kwarg (api_cache.py:120-122), and a format suffix is one (suffixed).
func depthListIsLive(lf listFilters, params url.Values, q string, suffixed bool) bool {
	return lf.upstreamFilter || sinceIsNonZero(params) || q != "" || suffixed
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
	var truncated bool
	switch m := meta.(type) {
	case map[string]string:
		_, truncated = m["truncated"]
	case map[string]any:
		_, truncated = m["truncated"]
	}
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
	meta      any // meta before the depth cut (listMeta, paginateList)
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
	meta := req.meta
	if meta == nil {
		meta = struct{}{}
	}
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
func parseRequestFilters(r *http.Request, params url.Values, tc TypeConfig, afterPrepare func() error) (listFilters, error) {
	ctx := WithUnknownFields(r.Context())
	lf, err := parseListFilters(ctx, params, tc, afterPrepare)
	if err != nil {
		return listFilters{}, err
	}
	lf.setDateFilter = pickCTF(r.URL.RawQuery, lf.ctf)
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

// parseRequest parses the filters and the since, skip, limit and depth
// values of a list or detail request, in upstream order: the
// prepare_query keys, then since, skip, limit and depth, then
// name_search and the other keys (parseListFilters,
// parseRequestParams). On an error it writes the 400 and returns
// ok=false. A filter error has the "filter error: " prefix, and a
// parameter error is the upstream text.
func parseRequest(w http.ResponseWriter, r *http.Request, params url.Values, tc TypeConfig) (listFilters, requestParams, bool) {
	var rp requestParams
	var paramErr error
	lf, err := parseRequestFilters(r, params, tc, func() error {
		rp, paramErr = parseRequestParams(params)
		return paramErr
	})
	if err != nil {
		detail := "filter error: " + err.Error()
		if paramErr != nil {
			detail = paramErr.Error()
		}
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: detail,
		})
		return listFilters{}, requestParams{}, false
	}
	return lf, rp, true
}

// isUniqueQuery reports whether a list request names one object, so
// that an empty result is a 404 instead of an empty list. It mirrors
// upstream is_unique_query: the "id" key on every type (2.83.0
// serializers.py:962-967), and also the "asn" key on net
// (serializers.py:3815-3820). Only the key counts, whatever its value
// and whatever the other filters, skip, limit and since are.
//
// Upstream skips the 404 when the query has the page key, because the
// response data is then the pagination object and never empty
// (rest.py:799-815, pagination.py:35-50). An empty page is a 200 with
// meta.pagination (paginateList).
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
// order (parseRequest, then the negative skip), so a request with two
// bad parameters gets the same 400 on both paths.
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

	lf, rp, ok := parseRequest(w, r, params, tc)
	if !ok {
		return
	}
	sliced, negativeSkip := rp.detailSlice()
	var err error

	// Default depth = 2 for detail endpoints to match upstream's
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
	if rp.depthPresent {
		depth = min(max(rp.depth, 0), 4)
	}

	filters := lf.preds
	// A name_search that matches no row is upstream qset.none(), which
	// get_queryset returns before it slices the query (2.83.0
	// rest.py:550-553, :755-760). So it is the miss 404, also with a
	// limit or skip above 0 or a negative skip. Only a request with a
	// slice or a negative skip needs the query of nameSearchMisses:
	// without them, Match applies the search.
	miss := lf.none
	if sliced || negativeSkip || rp.sinceInt != nil {
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
	// A since that int() does not accept, then a negative skip, as on a
	// list (upstream: 500 for both, see docs/API.md § Known
	// Divergences).
	if rp.sinceInt != nil && !miss {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: rp.sinceInt.Error(),
		})
		return
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
	fields := fieldsParam(params)

	result, err := tc.Get(withSetDateFilter(r.Context(), lf.setDateFilter), h.client, id, depth)
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
