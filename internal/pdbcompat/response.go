package pdbcompat

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/dotwaffle/peeringdb-plus/internal/httperr"
)

const (
	// DefaultLimit is the default value used when the `limit=` query
	// parameter is absent. Set to 0 ("unlimited") to mirror upstream
	// PeeringDB's behaviour (2.83.0 `rest.py:516`): bare `/api/<type>` URLs
	// return ALL rows from the queryset, not a paginated page.
	//
	// Earlier revisions of this code defaulted to 250, treating that as
	// a defensive page-size cap. The cap turned out to be a real parity
	// bug — verified 2026-04-28 against upstream live data
	// (parity-results.txt: bare /api/org returned 33,556 rows upstream
	// vs 250 on the mirror). The response-memory budget
	// (PDBPLUS_RESPONSE_MEMORY_LIMIT, default 128 MiB) is the real DoS
	// safeguard — it gates the precount × TypicalRowBytes before
	// materialising any result set, returning 413 when the would-be
	// payload exceeds the budget. The 250 default added nothing on top
	// of that, only divergence.
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

// apiError describes one /api/ error response.
type apiError struct {
	// Status is the HTTP status code.
	Status int
	// Detail is the error text: meta.error in the upstream form, detail
	// in the problem+json form.
	Detail string
	// EmptyData adds "data": [] to the upstream form (the unique-query
	// 404, upstream 2.83.0 rest.py:809-815).
	EmptyData bool
	// Meta holds more meta keys for the upstream form.
	Meta map[string]any
	// budget selects the WriteBudgetProblem body in problem+json mode.
	budget *BudgetExceeded
}

// writeError writes an /api/ error. The default is the upstream form
// {"meta": {"error": "<detail>"}} (2.83.0 renderers.py:134-148). A
// request whose Accept header names application/problem+json gets an
// RFC 9457 body instead. Every error path of the /api/ surface goes
// through this function, so both forms stay in step.
func writeError(w http.ResponseWriter, r *http.Request, e apiError) {
	w.Header().Set("X-Powered-By", poweredByHeader)
	w.Header().Add("Vary", "Accept")

	if httperr.WantsProblemJSON(r.Header) {
		if e.budget != nil {
			WriteBudgetProblem(w, r.URL.Path, *e.budget)
			return
		}
		httperr.WriteProblem(w, httperr.WriteProblemInput{
			Status:   e.Status,
			Detail:   e.Detail,
			Instance: r.URL.Path,
		})
		return
	}
	httperr.WriteMetaError(w, httperr.MetaErrorInput{
		Status:    e.Status,
		Error:     e.Detail,
		Meta:      e.Meta,
		EmptyData: e.EmptyData,
	})
}

// Upstream error texts of the list parameters.
var (
	// errSkipNotNumber is upstream 2.83.0 rest.py:511-514.
	errSkipNotNumber = errors.New("'skip' needs to be a number")
	// errLimitNotNumber is upstream 2.83.0 rest.py:515-518.
	errLimitNotNumber = errors.New("'limit' needs to be a number")
	// errSinceNotTimestamp is upstream 2.83.0 rest.py:505-510.
	errSinceNotTimestamp = errors.New("'since' needs to be a unix timestamp (epoch seconds)")
	// errNegativeSkip is the Django text for a negative slice
	// (db/models/query.py:403-417), which list() returns as a 400
	// (2.83.0 rest.py:757-760, :824-827). The text is the upstream
	// message, so it keeps its capital letter and period.
	errNegativeSkip = errors.New("Negative indexing is not supported.") //nolint:revive,staticcheck // exact upstream message text
)

// lastParam returns the last value of key and whether the key is
// present, as Django QueryDict.get does. A present key with an empty
// value returns ("", true).
func lastParam(params url.Values, key string) (string, bool) {
	vals, ok := params[key]
	if !ok || len(vals) == 0 {
		return "", false
	}
	return vals[len(vals)-1], true
}

// ParsePaginationParams returns limit and skip as upstream parses them
// (2.83.0 rest.py:511-518): the last value of each key, Python int()
// rules (pyInt), skip first. An empty or non-integer value is an error
// with the upstream text.
//
// The values keep their sign, and the caller decides what a negative
// value does. serveList serves every row for a negative limit, as
// upstream slices only when limit > 0 (rest.py:757-760), and returns
// 400 for a negative skip (errNegativeSkip).
//
// Without a limit key the limit is DefaultLimit (0 = every row), as
// upstream (rest.py:516). A positive limit has no upper cap
// (rest.py:757-758). The response memory budget bounds the response:
// when the count × TypicalRowBytes is over the budget, the handler
// returns 413 before it reads any row. A value too large for an int
// saturates: a huge limit serves every row and a huge skip serves no
// row.
func ParsePaginationParams(params url.Values) (limit, skip int, err error) {
	limit = DefaultLimit
	if v, ok := lastParam(params, "skip"); ok {
		if skip, _, err = pyInt(v); err != nil {
			return 0, 0, errSkipNotNumber
		}
	}
	if v, ok := lastParam(params, "limit"); ok {
		if limit, _, err = pyInt(v); err != nil {
			return 0, 0, errLimitNotNumber
		}
	}
	return limit, skip, nil
}

// parseSince returns the raw ?since= value: the last value, Python
// int() rules (2.83.0 rest.py:505-510, api_cache.py:80). present is
// false when the key is absent. An empty or non-integer value is
// errSinceNotTimestamp. Every reader of since calls this function, so
// the value is parsed one way only.
func parseSince(params url.Values) (n int, present bool, err error) {
	v, ok := lastParam(params, "since")
	if !ok {
		return 0, false, nil
	}
	n, _, err = pyInt(v)
	if err != nil {
		return 0, true, errSinceNotTimestamp
	}
	return n, true, nil
}

// ParseSinceParam parses ?since= as a Unix timestamp (see parseSince).
// It returns nil when the key is absent and when the value is 0 or
// less: upstream activates the since matrix only if since > 0 (2.83.0
// rest.py:719), so ?since=0 gives the plain live-status list. A zero
// boundary here would flip the status matrix and serve every
// tombstone.
//
// The time is in UTC. The SQLite driver binds a time.Time as text in
// the zone of the value, and the stored timestamps are UTC text, so the
// comparison is only correct when both sides are UTC. time.Unix returns
// the process zone, which is UTC in production but not on every host.
func ParseSinceParam(params url.Values) (*time.Time, error) {
	n, present, err := parseSince(params)
	if err != nil {
		return nil, err
	}
	if !present || n <= 0 {
		return nil, nil
	}
	t := time.Unix(int64(n), 0).UTC()
	return &t, nil
}
