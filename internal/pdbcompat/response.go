package pdbcompat

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
)

const (
	// DefaultLimit is the default value used when the `limit=` query
	// parameter is absent. Set to 0 ("unlimited") to mirror upstream
	// PeeringDB's `rest.py:495` behaviour: bare `/api/<type>` URLs
	// return ALL rows from the queryset, not a paginated page.
	//
	// Earlier revisions of this code defaulted to 250, treating that as
	// a defensive page-size cap. The cap turned out to be a real parity
	// bug — verified 2026-04-28 against upstream live data
	// (parity-results.txt: bare /api/org returned 33,556 rows upstream
	// vs 250 on the mirror). The Phase 71 response-memory budget
	// (PDBPLUS_RESPONSE_MEMORY_LIMIT, default 128 MiB) is the real DoS
	// safeguard — it gates the precount × TypicalRowBytes before
	// materialising any result set, returning 413 application/problem+json
	// when the would-be payload exceeds the budget. The 250 default
	// added nothing on top of that, only divergence.
	DefaultLimit = 0

	// MaxLimit is the maximum allowed page size per D-21. Applied only
	// when the caller supplies an explicit positive `limit=` (see
	// ParsePaginationParams). limit=0 / unset bypasses this clamp and
	// returns the full result set, gated by the Phase 71 budget.
	MaxLimit = 1000

	// poweredByHeader identifies this server in responses per D-25.
	poweredByHeader = "PeeringDB-Plus/1.1"
)

// envelope is the PeeringDB response wrapper: {"meta": {}, "data": [...]}.
type envelope struct {
	Meta any `json:"meta"`
	Data any `json:"data"`
}

// WriteResponse writes a successful PeeringDB-compatible JSON response with
// the standard envelope format per D-04. Data must be a slice.
func WriteResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Powered-By", poweredByHeader)

	resp := envelope{
		Meta: struct{}{},
		Data: data,
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// WriteProblem writes an RFC 9457 problem detail error response with the
// X-Powered-By header. This replaces the former PeeringDB error envelope
// with a standards-based format per ARCH-01.
func WriteProblem(w http.ResponseWriter, input httperr.WriteProblemInput) {
	w.Header().Set("X-Powered-By", poweredByHeader)
	httperr.WriteProblem(w, input)
}

// ParsePaginationParams extracts limit and skip from query parameters
// with defaults and bounds per D-16, D-21.
//
// Bare URL (no `limit=`): returns DefaultLimit (0 = unlimited),
// matching upstream `rest.py:495` which defaults `limit` to 0 and
// then `rest.py:737` which slices `qset[skip:]` (no upper bound).
// All-rows responses are gated by the Phase 71 response-memory
// budget; if the precount × TypicalRowBytes exceeds the budget, the
// handler returns 413 application/problem+json before materialising
// anything.
//
// Explicit `limit=N`: positive N is honoured, clamped to MaxLimit
// (1000) per D-21. limit=0 is the explicit "unlimited" sentinel and
// is passed through unchanged; the list closures' `if opts.Limit > 0
// { .Limit(...) }` gate omits the SQL LIMIT clause when limit is 0.
//
// Negative values are ignored (treated as missing) per upstream
// behaviour.
func ParsePaginationParams(params url.Values) (limit, skip int) {
	limit = DefaultLimit
	if v := params.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			limit = parsed
		}
	}
	if limit > 0 && limit > MaxLimit {
		limit = MaxLimit
	}

	if v := params.Get("skip"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed >= 0 {
			skip = parsed
		}
	}
	return limit, skip
}

// ParseSinceParam parses the ?since= query parameter as a Unix timestamp
// per D-15. Returns nil if the parameter is absent or empty.
func ParseSinceParam(params url.Values) (*time.Time, error) {
	v := params.Get("since")
	if v == "" {
		return nil, nil
	}
	t, err := parseTime(v)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
