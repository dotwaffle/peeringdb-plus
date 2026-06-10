package pdbcompat

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

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
	// the pre-flight CheckBudget gate. 0 disables the
	// check entirely — documented local-dev / test escape hatch. See
	// cmd/peeringdb-plus/main.go for the Config.ResponseMemoryLimit
	// wiring (default 128 MiB).
	responseMemoryLimit int64

	// inflightBytes tracks the summed estimated bytes of list responses
	// admitted by CheckBudget and not yet finished serving. The per-
	// request budget alone admits each request in isolation, so two
	// concurrent near-budget dumps — each individually under the limit
	// — could jointly materialize ~2x the budget and OOM a 256 MB
	// replica. Admission charges the estimate here and rejects with 503
	// + Retry-After when the pool would exceed responseMemoryLimit;
	// serveList releases the charge on return.
	inflightBytes atomic.Int64
}

// NewHandler creates a Handler for PeeringDB-compatible API endpoints.
// responseMemoryLimit is the per-response byte budget consumed by the
// pre-flight CheckBudget gate. Pass 0 to disable the
// budget check (local dev / tests only; operators ship a non-zero
// PDBPLUS_RESPONSE_MEMORY_LIMIT in prod — default 128 MiB).
func NewHandler(client *ent.Client, responseMemoryLimit int64) *Handler {
	return &Handler{client: client, responseMemoryLimit: responseMemoryLimit}
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

// serveIndex writes the API index in upstream PeeringDB's shape:
//
//	{"data": [{"<type>": "<absolute-url>", ...}], "meta": {}}
//
// a single object mapping every mirrored type to its absolute list-endpoint
// URL, built from the request scheme + host. Upstream additionally lists
// `as_set`, a network-derived AS-SET lookup this mirror does not serve (see
// docs/API.md § Known Divergences); listing only the 13 served types keeps the
// index from advertising a dead link.
func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request) {
	scheme := "https"
	if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "" {
		scheme = xfp
	} else if r.TLS == nil {
		scheme = "http"
	}
	base := scheme + "://" + r.Host + "/api/"

	types := make(map[string]string, len(Registry))
	for name := range Registry {
		types[name] = base + name
	}
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

// serveList handles list requests for the given type.
func (h *Handler) serveList(tc TypeConfig, w http.ResponseWriter, r *http.Request) {
	// Per-request heap-delta sampler.
	// Samples HeapInuse once at entry and once at handler exit (via defer);
	// emits OTel span attribute pdbplus.response.heap_delta_bytes and a
	// Prometheus histogram observation (pdbplus.response.heap_delta).
	// runtime.ReadMemStats is STW but acceptable once per request. MUST
	// NOT be called per-row.
	//
	// Fires on EVERY terminal path (200 success, 413 budget-exceeded, 400
	// filter-error, 500 query-error) — that's the point of a defer; a
	// budget-exceeded 413 is still a useful data point for the
	// distribution, and observing the small delta of a cheap error path
	// gives us a noise floor reference.
	startHeapBytes := memStatsHeapInuseBytes()
	defer recordResponseHeapDelta(r.Context(), r.URL.Path, tc.Name, startHeapBytes)

	params := r.URL.Query()

	// List-depth guardrail: list + ?depth= is not supported. Silently
	// ignore the param here so callers get normal list behaviour rather
	// than a 400 (matches upstream rest.py: unsupported request shapes
	// fall through to default list semantics). opts.Depth is never
	// populated on list requests, so there is no leak — list closures
	// never see a non-zero depth. The debug slog documents the no-op for
	// operators who enable DEBUG logging.
	if params.Get("depth") != "" {
		slog.DebugContext(r.Context(), "pdbcompat list: ignoring unsupported ?depth= param (list-depth guardrail)",
			slog.String("path", r.URL.Path),
			slog.String("type", tc.Name),
		)
	}

	// Parse pagination.
	limit, skip, err := ParsePaginationParams(params)
	if err != nil {
		// upstream rest.py:490-497 raises a 400 for non-numeric
		// limit/skip; silently ignoring a typo'd limit would turn a
		// bounded page request into a full-table dump.
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   err.Error(),
			Instance: r.URL.Path,
		})
		return
	}

	// Parse filters. The emptyResult short-circuit handles ?field__in=
	// and TypeConfig is threaded so that shadow-column routing can
	// consult tc.FoldedFields.
	//
	// Thread the ctx through an unknown-field
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
		slog.DebugContext(ctx, "pdbcompat: unknown filter fields silently ignored",
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

	// Parse since.
	since, err := ParseSinceParam(params)
	if err != nil {
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusBadRequest,
			Detail:   fmt.Sprintf("invalid since parameter: %v", err),
			Instance: r.URL.Path,
		})
		return
	}

	// Parse search (?q=).
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

	// Parse field projection (?fields=).
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
	// List depth is always 0 per the list-depth guardrail (the
	// ?depth= param is ignored on list endpoints; opts.Depth is never
	// populated by ParsePaginationParams).
	if h.responseMemoryLimit > 0 && !emptyResult && tc.Count != nil {
		count, err := tc.Count(r.Context(), h.client, opts)
		if err != nil {
			// Log the raw error for operators; never echo ent/SQL error
			// strings into the client-facing problem Detail (SEC: avoid
			// leaking schema/driver internals on the /api surface).
			slog.ErrorContext(r.Context(), "pdbcompat: count query failed",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.String("error", err.Error()),
			)
			WriteProblem(w, httperr.WriteProblemInput{
				Status:   http.StatusInternalServerError,
				Detail:   "failed to count matching records",
				Instance: r.URL.Path,
			})
			return
		}
		if info, ok := CheckBudget(count, tc.Name, 0 /*list depth=0 per the list-depth guardrail*/, h.responseMemoryLimit); !ok {
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

		// Global admission: the per-request check above treats each
		// request in isolation, but the budget is a process-wide memory
		// envelope — concurrent near-budget dumps must not stack past
		// it. Charge this request's estimate against the shared
		// in-flight pool and reject with 503 + Retry-After when the
		// pool would overflow; the charge is released when serveList
		// returns (response fully serialized, slices unreachable).
		estimate := int64(count) * int64(TypicalRowBytes(tc.Name, 0))
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
			WriteProblem(w, httperr.WriteProblemInput{
				Status:   http.StatusServiceUnavailable,
				Detail:   "server is serving other large responses; retry shortly",
				Instance: r.URL.Path,
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

	results, _, err := tc.List(r.Context(), h.client, opts)
	if err != nil {
		// Log raw error server-side only; keep the client Detail generic
		// so ent/SQL internals never reach the /api wire (SEC).
		slog.ErrorContext(r.Context(), "pdbcompat: list query failed",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusInternalServerError,
			Detail:   "failed to query matching records",
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
//
// Default detail depth is 2 (matches upstream PeeringDB
// peeringdb_server/serializers.py:817-823 — `default_depth(is_list=False)`
// returns 2 for single-object GETs versus 0 for lists). This causes
// `prefetch_related` (rest.py:750) to fire on every bare detail URL,
// embedding the per-type `_set` collections + parent FK objects (`org`,
// `campus`, etc.) that upstream always returns on `/api/<type>/<id>`.
// Explicit `?depth=0` short-circuits the prefetch (rest.py:852 returns
// the qset early when depth<=0) and yields a bare row, matching
// upstream's behaviour for that explicit override.
//
// Generalises commit 0d39654 (which fixed the IX `fac_set` shape at
// `?depth=2`) across all detail endpoints AND extends it to fire at
// the depth=0 default (which was previously skipping the prefetch
// chain entirely on bare detail URLs).
func (h *Handler) serveDetail(tc TypeConfig, id int, w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()

	// Parse depth. Default = 2 for detail endpoints to match upstream's
	// `default_depth(is_list=False)` (serializers.py:817-823). Upstream parses
	// `?depth=` as a raw int clamped to [0, max_depth] with max_depth=4 for
	// single GETs (serializers.py:789-814), so we honour 0/1/2/3/4: 0 is the
	// bare-row escape hatch (rest.py:852), 1 expands forward FKs flat with
	// reverse sets as ID lists, 2 fully expands. Depths >2 render the depth=2
	// shape (the deeper sub-level nesting they add is not reproduced). A
	// non-numeric value keeps the default; negatives floor to 0.
	depth := 2
	if v := params.Get("depth"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			depth = min(max(parsed, 0), 4)
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
	// only caller that exercises the depth=2 row-size estimate (lists are
	// pinned to depth 0 by the list-depth guardrail). budget<=0 disables the
	// check (dev/test) exactly as on the list path.
	if h.responseMemoryLimit > 0 {
		if info, ok := CheckBudget(1, tc.Name, depth, h.responseMemoryLimit); !ok {
			slog.WarnContext(r.Context(), "pdbcompat: detail response budget exceeded",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", tc.Name),
				slog.Int("depth", depth),
				slog.Int64("estimated_bytes", info.EstimatedBytes),
				slog.Int64("budget_bytes", info.BudgetBytes),
			)
			WriteBudgetProblem(w, r.URL.Path, info)
			return
		}
	}

	// Parse field projection (?fields=).
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
		// Log raw error server-side only; the client Detail stays generic
		// so ent/SQL internals never reach the /api wire (SEC).
		slog.ErrorContext(r.Context(), "pdbcompat: detail query failed",
			slog.String("endpoint", r.URL.Path),
			slog.String("type", tc.Name),
			slog.String("error", err.Error()),
		)
		WriteProblem(w, httperr.WriteProblemInput{
			Status:   http.StatusInternalServerError,
			Detail:   "failed to query record",
			Instance: r.URL.Path,
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
