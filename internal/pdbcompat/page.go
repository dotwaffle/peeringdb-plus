package pdbcompat

import (
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Page-number pagination (?page=, ?per_page=). Upstream paginates a list
// when the query has a page key (UnlimitedIfNoPagePagination, 2.83.0
// pagination.py:20-40, rest.py:790-806), after get_queryset has applied
// the filters, skip, limit and the depth cut. The rows of a page render
// as the rows of the list: the page is a Python list, so the serializer
// takes the detail depth defaults (serializers.py:933-934, :1024-1039),
// but the root is still the list serializer, which removes
// Meta.list_exclude (every forward FK object, :1286-1290), and without
// ?depth= no _set is prefetched (rest.py:774-777), so DRF skips each
// _set field (rest_framework/fields.py:445-451).

const (
	// pageParam and perPageParam are the upstream page_query_param and
	// page_size_query_param (pagination.py:22, DRF pagination.py:170).
	pageParam    = "page"
	perPageParam = "per_page"
	// pageSize is upstream PAGE_SIZE and max_page_size
	// (settings/__init__.py:1511, pagination.py:21-23).
	pageSize = 250
	// lastPage is DRF last_page_strings (pagination.py:182).
	lastPage = "last"
)

// errInvalidPage is DRF invalid_page_message (pagination.py:186), the
// 404 of a page that is not an integer or is out of range.
const errInvalidPage = "Invalid page."

// pageMeta is meta.pagination (build_pagination_meta,
// pagination.py:87-99).
type pageMeta struct {
	Count       int     `json:"count"`
	HasNext     bool    `json:"has_next"`
	HasPrevious bool    `json:"has_previous"`
	Next        *string `json:"next"`
	Previous    *string `json:"previous"`
	Page        int     `json:"page"`
	PerPage     int     `json:"per_page"`
	TotalPages  int     `json:"total_pages"`
}

// pageValue returns the page value of a list and whether the list is
// paginated. The live path paginates when the query has the page key
// (pagination.py:35). The API cache path paginates only when the last
// page value is not empty (api_cache.py:81, :146), so ?page= with an
// empty value serves the whole cached list.
func pageValue(params url.Values, cached bool) (string, bool) {
	v, ok := lastParam(params, pageParam)
	if !ok || (cached && v == "") {
		return "", false
	}
	return v, true
}

// perPage returns the page size: the last per_page value when int()
// accepts it and it is above 0, capped at pageSize, else pageSize
// (DRF get_page_size and _positive_int with strict=True,
// pagination.py:23-32, :256-264).
func perPage(params url.Values) int {
	v, ok := lastParam(params, perPageParam)
	if !ok {
		return pageSize
	}
	n, _, err := pyInt(v)
	if err != nil || n <= 0 {
		return pageSize
	}
	return min(n, pageSize)
}

// resolvePage returns the page number of value for count rows in pages
// of size rows, and the number of pages. An empty value is page 1 and
// "last" is the last page (DRF get_page_number, pagination.py:215-219).
// ok is false when int() does not accept the value or the page is out
// of range (Django Paginator.validate_number). An empty list has one
// page (Paginator.num_pages with allow_empty_first_page).
func resolvePage(value string, count, size int) (page, pages int, ok bool) {
	pages = (max(count, 1) + size - 1) / size
	switch value {
	case "":
		return 1, pages, true
	case lastPage:
		return pages, pages, true
	}
	n, _, err := pyInt(value)
	if err != nil || n < 1 || n > pages {
		return 0, pages, false
	}
	return n, pages, true
}

// paginateList applies ?page= to a list. It counts the rows that the
// list serves (after skip and limit), cuts a live depth list to
// apiDepthRowLimit rows as upstream get_queryset does, and narrows opts
// to the rows of the page. It returns the meta of the response: meta
// with the pagination key, and the truncated key when the list was cut.
// On a page that does not exist it writes the 404 and returns ok=false.
func (h *Handler) paginateList(w http.ResponseWriter, r *http.Request, tc TypeConfig, opts *QueryOptions, value string, live bool, depthText string, meta any) (any, bool) {
	count := 0
	if !opts.EmptyResult {
		var err error
		if count, err = tc.Count(r.Context(), h.client, *opts); err != nil {
			h.writeCountError(w, r, tc, err)
			return nil, false
		}
	}
	if live && count > apiDepthRowLimit {
		count = apiDepthRowLimit
		meta = truncatedMeta(depthText)
	}
	size := perPage(r.URL.Query())
	page, pages, ok := resolvePage(value, count, size)
	if !ok {
		// The NotFound is raised in list() after get_queryset, so the
		// meta keys that get_queryset set stay (renderers.py:111-118).
		// The API cache path loses meta.generated: load() builds it in
		// a local variable.
		var truncated map[string]any
		if _, ok := meta.(map[string]string); ok {
			truncated = metaMap(meta)
		}
		writeError(w, r, apiError{
			Status: http.StatusNotFound,
			Detail: errInvalidPage,
			Meta:   truncated,
		})
		return nil, false
	}
	first := (page - 1) * size
	if n := min(size, count-first); n > 0 {
		opts.Skip += first
		opts.Limit = n
	} else {
		opts.EmptyResult = true
	}
	pm := pageMeta{
		Count:       count,
		HasNext:     page < pages,
		HasPrevious: page > 1,
		Page:        page,
		PerPage:     size,
		TotalPages:  pages,
	}
	if pm.HasNext {
		pm.Next = pageLink(r, page+1)
	}
	if pm.HasPrevious {
		pm.Previous = pageLink(r, page-1)
	}
	out := metaMap(meta)
	out["pagination"] = pm
	return out, true
}

// metaMap returns a new map with the keys of a list meta object: an
// empty object, meta.truncated or meta.generated.
func metaMap(meta any) map[string]any {
	out := map[string]any{}
	switch m := meta.(type) {
	case map[string]string:
		for k, v := range m {
			out[k] = v
		}
	case map[string]any:
		maps.Copy(out, m)
	}
	return out
}

// pageLink returns the URL of page n of the request, as DRF builds it
// from request.build_absolute_uri() (pagination.py:266-281,
// utils/urls.py): the query is parsed with parse_qs(keep_blank_values),
// the page key is set to n, or removed for page 1, and the keys are
// sorted and encoded with urlencode(doseq=True).
func pageLink(r *http.Request, n int) *string {
	q := pyParseQuery(r.URL.RawQuery)
	if n == 1 {
		q.Del(pageParam)
	} else {
		q.Set(pageParam, strconv.Itoa(n))
	}
	link := requestScheme(r) + "://" + r.Host + r.URL.EscapedPath()
	if enc := q.Encode(); enc != "" {
		link += "?" + enc
	}
	return &link
}

// requestScheme returns the scheme that the client used: the
// X-Forwarded-Proto value that the proxy sets, else https for a TLS
// connection and http for a plain one.
func requestScheme(r *http.Request) string {
	if xfp := r.Header.Get("X-Forwarded-Proto"); xfp != "" {
		return xfp
	}
	if r.TLS == nil {
		return "http"
	}
	return "https"
}

// pyParseQuery parses a raw query as Python urllib.parse.parse_qs does
// with keep_blank_values=True: the pairs split on "&" only, an empty
// pair is dropped, a pair without "=" has an empty value, "+" is a
// space, and an escape that is not valid stays as it is. url.ParseQuery
// differs: it splits on ";" too and drops a pair with a bad escape.
// url.Values.Encode then gives the urlencode(sorted(...), doseq=True)
// form: sorted keys, the values of a key in order, and the same set of
// unescaped bytes as quote_plus.
func pyParseQuery(raw string) url.Values {
	q := url.Values{}
	for pair := range strings.SplitSeq(raw, "&") {
		if pair == "" {
			continue
		}
		k, v, _ := strings.Cut(pair, "=")
		k = pyUnquotePlus(k)
		q[k] = append(q[k], pyUnquotePlus(v))
	}
	return q
}

// pyUnquotePlus decodes s as Python urllib.parse.unquote_plus does: "+"
// is a space, each valid %XX escape is a byte, and the bytes decode as
// UTF-8 with errors="replace".
func pyUnquotePlus(s string) string {
	s = strings.ReplaceAll(s, "+", " ")
	if !strings.Contains(s, "%") {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b = append(b, unhex(s[i+1])<<4|unhex(s[i+2]))
			i += 2
			continue
		}
		b = append(b, s[i])
	}
	return pyDecodeUTF8(b)
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c <= '9':
		return c - '0'
	case c <= 'F':
		return c - 'A' + 10
	default:
		return c - 'a' + 10
	}
}

// pyDecodeUTF8 decodes b as Python bytes.decode("utf-8", "replace")
// does: each maximal subpart of an ill-formed sequence becomes one
// U+FFFD (Unicode 15 section 3.9, Table 3-8). utf8.DecodeRune reports
// width 1 for every invalid byte, so the subpart is measured here.
func pyDecodeUTF8(b []byte) string {
	var sb strings.Builder
	for len(b) > 0 {
		r, n := utf8.DecodeRune(b)
		if r != utf8.RuneError || n > 1 {
			sb.WriteRune(r)
			b = b[n:]
			continue
		}
		sb.WriteRune(utf8.RuneError)
		b = b[maximalSubpart(b):]
	}
	return sb.String()
}

// maximalSubpart returns the length of the ill-formed sequence at the
// start of b that Python replaces with one U+FFFD: the lead byte and
// the continuation bytes after it that could still start a well-formed
// sequence (Unicode Table 3-7).
func maximalSubpart(b []byte) int {
	lo, hi := byte(0x80), byte(0xBF)
	var need int
	switch c := b[0]; {
	case c >= 0xC2 && c <= 0xDF:
		need = 1
	case c == 0xE0:
		need, lo = 2, 0xA0
	case c >= 0xE1 && c <= 0xEC, c == 0xEE, c == 0xEF:
		need = 2
	case c == 0xED:
		need, hi = 2, 0x9F
	case c == 0xF0:
		need, lo = 3, 0x90
	case c >= 0xF1 && c <= 0xF3:
		need = 3
	case c == 0xF4:
		need, hi = 3, 0x8F
	default:
		return 1
	}
	n := 1
	for ; n <= need && n < len(b); n++ {
		if b[n] < lo || b[n] > hi {
			break
		}
		lo, hi = 0x80, 0xBF
	}
	return n
}
