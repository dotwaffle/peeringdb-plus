package pdbcompat

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

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
	w.Header().Set("Content-Type", httperr.MetaJSONContentType)
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
	// Fields holds the field errors of a DRF serializer. The upstream
	// form puts each field at the top level of the body, in serializer
	// field order, and meta.error is the reason phrase of Status
	// (renderers.py:134-141). Detail is not used.
	Fields []fieldError
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
		detail := e.Detail
		if len(e.Fields) > 0 {
			detail = fieldErrorsDetail(e.Fields)
		}
		httperr.WriteProblem(w, httperr.WriteProblemInput{
			Status:   e.Status,
			Detail:   detail,
			Instance: r.URL.Path,
		})
		return
	}
	if len(e.Fields) > 0 {
		writeFieldErrors(w, e.Status, e.Fields)
		return
	}
	httperr.WriteMetaError(w, httperr.MetaErrorInput{
		Status:    e.Status,
		Error:     e.Detail,
		Meta:      e.Meta,
		EmptyData: e.EmptyData,
	})
}

// fieldError is one field error of a DRF serializer. Message is WTF-8:
// UTF-8 that can also hold a lone surrogate (wtf8String), which a JSON
// value of the request can put in the text.
type fieldError struct {
	Field, Message string
}

// fieldErrorsDetail joins field errors as "<field>: <message>" for the
// problem+json detail.
func fieldErrorsDetail(errs []fieldError) string {
	parts := make([]string, len(errs))
	for i, e := range errs {
		parts[i] = e.Field + ": " + string(wtf8Runes(e.Message))
	}
	return strings.Join(parts, "; ")
}

// writeFieldErrors writes the upstream body of DRF field errors:
// {"<field>": ["<message>"], ..., "meta": {"error": "<reason>"}}. A
// field with more than one error has one entry per error, in order.
// Upstream writes every character that is not ASCII as an escape
// (renderers.py:44-47, ensure_ascii), so the body does too.
func writeFieldErrors(w http.ResponseWriter, status int, errs []fieldError) {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i < len(errs); {
		j := i
		for j < len(errs) && errs[j].Field == errs[i].Field {
			j++
		}
		b.WriteString(pyJSONQuote(errs[i].Field, true))
		b.WriteString(":[")
		for k := i; k < j; k++ {
			if k > i {
				b.WriteByte(',')
			}
			b.WriteString(pyJSONQuoteRunes(wtf8Runes(errs[k].Message), true))
		}
		b.WriteString("],")
		i = j
	}
	b.WriteString(`"meta":{"error":`)
	b.WriteString(pyJSONQuote(http.StatusText(status), true))
	b.WriteString("}}\n")
	w.Header().Set("Content-Type", httperr.MetaJSONContentType)
	w.WriteHeader(status)
	_, _ = io.WriteString(w, b.String())
}

// wtf8String encodes rs as UTF-8, with a lone surrogate as its 3-byte
// form (WTF-8), which utf8.EncodeRune would replace with U+FFFD.
func wtf8String(rs []rune) string {
	var b strings.Builder
	for _, r := range rs {
		if isSurrogate(r) {
			u := uint16(r) //nolint:gosec // G115: a surrogate fits in 16 bits
			b.Write([]byte{0xe0 | byte(u>>12), 0x80 | byte(u>>6)&0x3f, 0x80 | byte(u)&0x3f})
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// wtf8Runes decodes a wtf8String.
func wtf8Runes(s string) []rune {
	out := make([]rune, 0, len(s))
	for len(s) > 0 {
		if len(s) >= 3 && s[0] == 0xed && s[1] >= 0xa0 && s[1] <= 0xbf && s[2] >= 0x80 && s[2] <= 0xbf {
			out = append(out, rune(s[0]&0x0f)<<12|rune(s[1]&0x3f)<<6|rune(s[2]&0x3f))
			s = s[3:]
			continue
		}
		r, n := utf8.DecodeRuneInString(s)
		out = append(out, r)
		s = s[n:]
	}
	return out
}

// Upstream error texts of the list parameters.
var (
	// errSkipNotNumber is upstream 2.83.0 rest.py:511-514.
	errSkipNotNumber = errors.New("'skip' needs to be a number")
	// errLimitNotNumber is upstream 2.83.0 rest.py:515-518.
	errLimitNotNumber = errors.New("'limit' needs to be a number")
	// errSinceNotTimestamp is upstream 2.83.0 rest.py:505-510.
	errSinceNotTimestamp = errors.New("'since' needs to be a unix timestamp (epoch seconds)")
	// errDepthNotNumber is upstream 2.83.0 rest.py:520-523.
	errDepthNotNumber = errors.New("'depth' needs to be a number")
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

// sinceIntError is the error of a since value that Python float()
// accepts and int() does not, for example 1.5 or 1e3. Upstream first
// parses since with int(float(v)) (2.83.0 rest.py:505-510), so the
// value passes, and then parses it again with int(v) when it builds its
// API cache loader after the filter loop (rest.py:707,
// api_cache.py:80). list() returns that ValueError as a 400 with the
// Python message (rest.py:824-827).
type sinceIntError struct {
	value string
}

func (e *sinceIntError) Error() string {
	return pyIntValueError(e.value)
}

// parseSince returns the raw ?since= value: the last value, Python
// int() rules (2.83.0 rest.py:505-510, api_cache.py:80). present is
// false when the key is absent. An empty value, or a value that
// float() rejects or that is nan, is errSinceNotTimestamp (int(float())
// raises ValueError). An infinite value is errSinceNotTimestamp too:
// upstream int(float()) raises OverflowError, which it does not catch
// (a 500, see docs/API.md § Known Divergences). A finite value that
// int() rejects is a *sinceIntError, which the caller reports after the
// filters. Every reader of since calls this function, so the value is
// parsed one way only.
func parseSince(params url.Values) (n int, present bool, err error) {
	v, ok := lastParam(params, "since")
	if !ok {
		return 0, false, nil
	}
	n, _, err = pyInt(v)
	if err == nil {
		return n, true, nil
	}
	if classifyPyFloat(v) == pyFloatFinite {
		return 0, true, &sinceIntError{value: v}
	}
	return 0, true, errSinceNotTimestamp
}

// ParseDepthParam returns the ?depth= value as upstream parses it
// (2.83.0 rest.py:520-523): the last value, Python int() rules (pyInt).
// raw is the value, saturated to the int range. text is its canonical
// decimal form, for messages that print the value. present is false
// when the key is absent, and the caller then applies its own default.
// An empty or non-integer value is errDepthNotNumber. The caller clamps
// raw to its depth range.
func ParseDepthParam(params url.Values) (raw int, text string, present bool, err error) {
	v, ok := lastParam(params, "depth")
	if !ok {
		return 0, "0", false, nil
	}
	raw, text, err = pyInt(v)
	if err != nil {
		return 0, "", true, errDepthNotNumber
	}
	return raw, text, true, nil
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

// requestParams holds since, skip, limit and depth of a list or detail
// request.
type requestParams struct {
	// limit and skip keep their sign (see ParsePaginationParams).
	limit, skip int
	// since is nil when the key is absent or its value is 0 or less
	// (ParseSinceParam).
	since *time.Time
	// sinceInt is set when the since value is a *sinceIntError. since
	// is then nil. The caller returns it as a 400 after the filters.
	sinceInt error
	// depth is the raw depth value and depthText its decimal form
	// (ParseDepthParam). depthPresent is false when the key is absent:
	// the caller applies its own default.
	depth        int
	depthText    string
	depthPresent bool
}

// parseRequestParams parses since, skip, limit and depth in the order of
// upstream get_queryset (2.83.0 rest.py:505-523), so a request with two
// bad values gets the error of the first one that upstream checks.
// Upstream parses them after prepare_query (:486-500) and before
// name_search and its filter loop (:531-703): the callers run this
// function as the afterPrepare step of parseListFilters.
func parseRequestParams(params url.Values) (requestParams, error) {
	var p requestParams
	var err error
	var sie *sinceIntError
	p.since, err = ParseSinceParam(params)
	switch {
	case errors.As(err, &sie):
		p.sinceInt = err
	case err != nil:
		return requestParams{}, err
	}
	if p.limit, p.skip, err = ParsePaginationParams(params); err != nil {
		return requestParams{}, err
	}
	if p.depth, p.depthText, p.depthPresent, err = ParseDepthParam(params); err != nil {
		return requestParams{}, err
	}
	return p, nil
}

// detailSlice returns the slice rules of a single-object GET. Upstream
// parses limit and skip for a detail too and slices the query before
// get() (2.83.0 rest.py:511-518, :755-760). Django cannot filter a
// sliced query, so get() fails and DRF answers 404 Not found. (Django
// query.py:1505-1507, DRF generics.py:13-21). sliced is true for a limit
// above 0 or a skip above 0. A negative limit does not slice (upstream
// slices only when limit > 0), so the object is returned. negativeSkip
// is true for a skip below 0. The caller returns 400 errNegativeSkip
// after the filters, as serveList does (upstream: 500, see docs/API.md
// § Known Divergences).
func (p requestParams) detailSlice() (sliced, negativeSkip bool) {
	return p.limit > 0 || p.skip > 0, p.skip < 0
}
