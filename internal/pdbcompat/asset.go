package pdbcompat

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
)

// The as_set lookup (upstream 2.83.0 rest.py:1396-1423, registered at
// :1598) maps the ASN of a network to its irr_as_set value. It is not a
// Registry type: it has no filters, no pagination and no depth, and it
// reads only three network columns.

// asSetPath is the /api/ path segment of the as_set lookup.
const asSetPath = "as_set"

// asSetEntryBytes is the response budget charge for each entry of the
// as_set list. Measured over 20,000 rows: the ent Scan allocates about
// 300 bytes for each row and the body is about 33 bytes for each row.
// The rowsize rule (double the measured bytes, round up to 64) gives
// 640.
const asSetEntryBytes = 640

// Heap-delta endpoint labels. They are fixed so that an ASN never
// becomes a metric attribute value.
const (
	asSetListEndpoint   = "/api/as_set"
	asSetDetailEndpoint = "/api/as_set/{asn}"
)

// asSetRow is one entry of the lookup. ent's sql/scan matches a column
// to the json tag of a field, so the tags are necessary.
type asSetRow struct {
	Asn      int    `json:"asn"`
	IrrAsSet string `json:"irr_as_set"`
}

// asSetPredicates selects the networks of the as_set list: status ok and
// an irr_as_set that is not empty (upstream get_queryset,
// rest.py:1405-1406). The count and the select use the same predicates,
// so the budget check prices the rows that the list serves. The SQL
// comparison with the empty string also leaves out a NULL value.
func asSetPredicates() []predicate.Network {
	return []predicate.Network{network.StatusEQ("ok"), network.IrrAsSetNEQ("")}
}

// serveASSet serves /api/as_set and /api/as_set/<asn>. idStr is the raw
// path segment after as_set. dispatch routes here before the Registry
// lookup, so this function takes the heap-delta sample of the request.
func (h *Handler) serveASSet(w http.ResponseWriter, r *http.Request, idStr string) {
	endpoint := asSetListEndpoint
	if idStr != "" {
		endpoint = asSetDetailEndpoint
	}
	startHeapBytes := memStatsHeapInuseBytes()
	defer recordResponseHeapDelta(r.Context(), endpoint, asSetPath, startHeapBytes)

	// Upstream has no route for a path with more segments. For a "."
	// the format-suffix route raises Http404 in content negotiation,
	// before the method check (drf urlpatterns.py:109,
	// negotiation.py:80-88, views.py:408-411).
	if strings.ContainsAny(idStr, "./") {
		writeDetailNotFound(w, r, detailSliceNotFound)
		return
	}

	// The viewset sets http_method_names = ["get"] (rest.py:1399), so
	// DRF answers HEAD with 405 (views.py:517-521). net/http sends no
	// body for a HEAD response.
	if r.Method == http.MethodHead {
		w.Header().Set("Allow", "GET")
		writeError(w, r, apiError{
			Status: http.StatusMethodNotAllowed,
			Detail: "Method \"" + r.Method + "\" not allowed.",
		})
		return
	}

	if idStr == "" {
		h.serveASSetList(w, r)
		return
	}
	h.serveASSetDetail(w, r, idStr)
}

// serveASSetList writes the map of every listed network. Upstream reads
// no query parameter here (rest.py:1411-1412): limit, skip, since,
// depth, fields and filters have no effect, so none is parsed.
func (h *Handler) serveASSetList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.responseMemoryLimit > 0 {
		count, err := h.client.Network.Query().Where(asSetPredicates()...).Count(ctx)
		if err != nil {
			slog.ErrorContext(ctx, "pdbcompat: as_set count query failed",
				slog.String("error", err.Error()),
			)
			writeError(w, r, apiError{
				Status: http.StatusInternalServerError,
				Detail: "failed to count matching records",
			})
			return
		}
		info, ok := checkBudgetBytes(count, asSetEntryBytes, asSetPath, 0, h.responseMemoryLimit)
		if !ok {
			slog.WarnContext(ctx, "pdbcompat: response budget exceeded",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", asSetPath),
				slog.Int("count", info.Count),
				slog.Int64("estimated_bytes", info.EstimatedBytes),
				slog.Int64("budget_bytes", info.BudgetBytes),
				slog.Int("max_rows", info.MaxRows),
			)
			writeBudgetError(w, r, info)
			return
		}
		estimate := info.EstimatedBytes
		if pooled := h.inflightBytes.Add(estimate); pooled > h.responseMemoryLimit {
			h.inflightBytes.Add(-estimate)
			slog.WarnContext(ctx, "pdbcompat: concurrent budget pool exhausted",
				slog.String("endpoint", r.URL.Path),
				slog.String("type", asSetPath),
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
		if count == 0 {
			writeASSetList(ctx, w, nil)
			return
		}
	}

	// Upstream sends the rows in database order (as_set_map sorts by asn
	// only for its own queryset, models.py:5699-5705). The mirror sorts
	// by asn. The order of JSON object keys has no meaning.
	var rows []asSetRow
	if err := h.client.Network.Query().
		Where(asSetPredicates()...).
		Order(ent.Asc(network.FieldAsn)).
		Select(network.FieldAsn, network.FieldIrrAsSet).
		Scan(ctx, &rows); err != nil {
		slog.ErrorContext(ctx, "pdbcompat: as_set query failed",
			slog.String("error", err.Error()),
		)
		writeError(w, r, apiError{
			Status: http.StatusInternalServerError,
			Detail: "failed to query matching records",
		})
		return
	}
	writeASSetList(ctx, w, rows)
}

// serveASSetDetail writes the pair of the network with the ASN in
// idStr. Upstream reads the ASN with int() and looks it up with the
// plain manager, so a network in any status matches (rest.py:1414-1423,
// handleref models.py:92-93). This is a lookup by ASN, not the PK detail
// path, so the detail status set does not apply.
func (h *Handler) serveASSetDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	ctx := r.Context()

	// A value that int() rejects is a 400 (rest.py:1417-1420). A value
	// out of range saturates, finds no row and is a 404, as upstream.
	asn, _, err := pyInt(idStr)
	if err != nil {
		writeError(w, r, apiError{
			Status: http.StatusBadRequest,
			Detail: "Invalid ASN",
		})
		return
	}

	var rows []asSetRow
	err = h.client.Network.Query().
		Where(network.Asn(asn)).
		Select(network.FieldAsn, network.FieldIrrAsSet).
		Scan(ctx, &rows)
	if err != nil {
		slog.ErrorContext(ctx, "pdbcompat: as_set detail query failed",
			slog.String("error", err.Error()),
		)
		writeError(w, r, apiError{
			Status: http.StatusInternalServerError,
			Detail: "failed to query record",
		})
		return
	}
	if len(rows) == 0 {
		// Upstream answers Response(status=404) with no data. The
		// renderer writes an empty body and DRF drops Content-Type
		// (renderers.py:106-107, drf response.py:82-83).
		w.Header().Set("X-Powered-By", poweredByHeader)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	// asn is unique (networks_asn_key), so there is at most one row.
	writeASSetList(ctx, w, rows[:1])
}

// writeASSetList writes {"meta":{},"data":[{"<asn>":"<irr_as_set>",...}]}
// with the keys in the order of rows. With no rows, data is an empty
// array, as upstream renders an empty dict (renderers.py:131-132).
func writeASSetList(ctx context.Context, w http.ResponseWriter, rows []asSetRow) {
	var b strings.Builder
	b.WriteString(`{"meta":{},"data":[`)
	if len(rows) > 0 {
		b.WriteByte('{')
		for i, row := range rows {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteByte('"')
			b.WriteString(strconv.Itoa(row.Asn))
			b.WriteString(`":`)
			v, _ := json.Marshal(row.IrrAsSet) // a string always encodes
			b.Write(v)
		}
		b.WriteByte('}')
	}
	b.WriteString("]}")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Powered-By", poweredByHeader)
	if _, err := io.WriteString(w, b.String()); err != nil {
		slog.ErrorContext(ctx, "pdbcompat: stream encode failed mid-response",
			slog.String("type", asSetPath),
			slog.String("error", err.Error()),
		)
	}
}
