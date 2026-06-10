package pdbcompat

import (
	"encoding/json"
	"fmt"
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
	// vs 250 on the mirror). The response-memory budget
	// (PDBPLUS_RESPONSE_MEMORY_LIMIT, default 128 MiB) is the real DoS
	// safeguard — it gates the precount × TypicalRowBytes before
	// materialising any result set, returning 413 application/problem+json
	// when the would-be payload exceeds the budget. The 250 default
	// added nothing on top of that, only divergence.
	DefaultLimit = 0

	// poweredByHeader identifies this server in responses.
	poweredByHeader = "PeeringDB-Plus/1.1"
)

// envelope is the PeeringDB response wrapper: {"meta": {}, "data": [...]}.
type envelope struct {
	Meta any `json:"meta"`
	Data any `json:"data"`
}

// WriteResponse writes a successful PeeringDB-compatible JSON response with
// the standard envelope format. Data must be a slice.
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
// with a standards-based format.
func WriteProblem(w http.ResponseWriter, input httperr.WriteProblemInput) {
	w.Header().Set("X-Powered-By", poweredByHeader)
	httperr.WriteProblem(w, input)
}

// ParsePaginationParams extracts limit and skip from query parameters
// with defaults and validation.
//
// Bare URL (no `limit=`): returns DefaultLimit (0 = unlimited),
// matching upstream `rest.py:495` which defaults `limit` to 0 and
// then `rest.py:737` which slices `qset[skip:]` (no upper bound).
// All-rows responses are gated by the response-memory
// budget; if the precount × TypicalRowBytes exceeds the budget, the
// handler returns 413 application/problem+json before materialising
// anything.
//
// Explicit `limit=N`: positive N is honoured unmodified — upstream
// applies qset[skip:skip+limit] with no upper cap (rest.py:734-735),
// and the response-memory budget is the real cost bound. (An earlier
// revision clamped to 1000, which silently truncated pages for
// clients paginating with larger windows — rows past the clamp were
// permanently skipped with nothing in the envelope to signal it.)
// limit=0 is the explicit "unlimited" sentinel and is passed through
// unchanged; the list closures' `if opts.Limit > 0 { .Limit(...) }`
// gate omits the SQL LIMIT clause when limit is 0.
//
// Non-numeric values are a 400 — upstream raises RestValidationError
// "'limit' needs to be a number" (rest.py:490-497). Silently treating
// a typo'd limit as absent turned a bounded page request into a
// full-table dump. Negative values get the same 400 (upstream's
// negative slice raises server-side; a clean 400 is the sane mirror).
func ParsePaginationParams(params url.Values) (limit, skip int, err error) {
	limit = DefaultLimit
	if v := params.Get("limit"); v != "" {
		parsed, perr := strconv.Atoi(v)
		if perr != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("'limit' needs to be a non-negative number, got %q", v)
		}
		limit = parsed
	}

	if v := params.Get("skip"); v != "" {
		parsed, perr := strconv.Atoi(v)
		if perr != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("'skip' needs to be a non-negative number, got %q", v)
		}
		skip = parsed
	}
	return limit, skip, nil
}

// ParseSinceParam parses the ?since= query parameter as a Unix timestamp.
// Returns nil if the parameter is absent or empty.
//
// since<=0 is also treated as absent: upstream activates the since
// matrix only `if since > 0` (rest.py:696), so ?since=0 falls through
// to the plain status='ok' list there. Honouring a zero boundary here
// would flip the status matrix and serve the entire tombstone corpus.
func ParseSinceParam(params url.Values) (*time.Time, error) {
	v := params.Get("since")
	if v == "" {
		return nil, nil
	}
	t, err := parseTime(v)
	if err != nil {
		return nil, err
	}
	if t.Unix() <= 0 {
		return nil, nil
	}
	return &t, nil
}
