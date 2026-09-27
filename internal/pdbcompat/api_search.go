package pdbcompat

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/facility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/organization"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the /api/search route of upstream PeeringDB 2.83.0
// (search_api_view, rest.py:2009-2041, route rest.py:2119). The view
// sends the q values to the Elasticsearch search of the web site
// (search_v2, search_v2.py:861-978). The mirror runs the matches of
// the name_search key (name_search.go) on the 6 indexed types.

// searchPath reports whether rest, the path after /api/, is the search
// route ^search/?$.
func searchPath(rest string) bool {
	return rest == "search" || rest == "search/"
}

// searchLimit is SEARCH_RESULTS_LIMIT (settings/__init__.py:1516): the
// most hits that one search returns, over all types.
const searchLimit = 1000

// The bodies of the two 401 responses of search_api_view
// (rest.py:2016-2027): JsonResponse with the default json.dumps
// separators.
const (
	searchNoKeyBody    = `{"error": "No API key provided. Please include an Authorization header with your API key."}`
	searchEmptyKeyBody = `{"error": "API key cannot be empty"}`
)

// searchHit is one search result. OrgID is the id of the organization
// of the row, its own id for an org. ASN is set for a net only.
type searchHit struct {
	ID    int
	Name  string
	OrgID *int
	ASN   *int
	// order is the position of the type in searchTypes.
	order int
}

// searchType is one type of the search, in the order of the upstream
// index list (search_v2.py:911), which is also the key order of the
// response (process_search_results, :766).
type searchType struct {
	tag string
	// query returns at most limit ok rows of the type that match pred,
	// in lower(name), id order.
	query func(ctx context.Context, h *Handler, pred func(*sql.Selector), limit int) ([]searchHit, error)
}

// searchRow is the scan target of the per-type queries.
type searchRow struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	OrgID *int   `json:"org_id"`
	ASN   *int   `json:"asn"`
}

// searchOrder orders a search query by lower(name), then id.
func searchOrder(s *sql.Selector) {
	s.OrderExpr(sql.Expr("lower(" + s.C("name") + ")"))
	s.OrderBy(s.C("id"))
}

// hitsFromRows converts the scanned rows. selfOrg sets OrgID to the id of
// the row, for org.
func hitsFromRows(rows []searchRow, selfOrg bool) []searchHit {
	hits := make([]searchHit, len(rows))
	for i, r := range rows {
		hits[i] = searchHit{ID: r.ID, Name: r.Name, OrgID: r.OrgID, ASN: r.ASN}
		if selfOrg {
			id := r.ID
			hits[i].OrgID = &id
		}
	}
	return hits
}

var searchTypes = []searchType{
	{peeringdb.TypeFac, func(ctx context.Context, h *Handler, pred func(*sql.Selector), limit int) ([]searchHit, error) {
		var rows []searchRow
		err := h.client.Facility.Query().Where(pred).Order(searchOrder).Limit(limit).
			Select(facility.FieldID, facility.FieldName, facility.FieldOrgID).Scan(ctx, &rows)
		return hitsFromRows(rows, false), err
	}},
	{peeringdb.TypeIX, func(ctx context.Context, h *Handler, pred func(*sql.Selector), limit int) ([]searchHit, error) {
		var rows []searchRow
		err := h.client.InternetExchange.Query().Where(pred).Order(searchOrder).Limit(limit).
			Select(internetexchange.FieldID, internetexchange.FieldName, internetexchange.FieldOrgID).Scan(ctx, &rows)
		return hitsFromRows(rows, false), err
	}},
	{peeringdb.TypeNet, func(ctx context.Context, h *Handler, pred func(*sql.Selector), limit int) ([]searchHit, error) {
		var rows []searchRow
		err := h.client.Network.Query().Where(pred).Order(searchOrder).Limit(limit).
			Select(network.FieldID, network.FieldName, network.FieldOrgID, network.FieldAsn).Scan(ctx, &rows)
		return hitsFromRows(rows, false), err
	}},
	{peeringdb.TypeOrg, func(ctx context.Context, h *Handler, pred func(*sql.Selector), limit int) ([]searchHit, error) {
		var rows []searchRow
		err := h.client.Organization.Query().Where(pred).Order(searchOrder).Limit(limit).
			Select(organization.FieldID, organization.FieldName).Scan(ctx, &rows)
		return hitsFromRows(rows, true), err
	}},
	{peeringdb.TypeCampus, func(ctx context.Context, h *Handler, pred func(*sql.Selector), limit int) ([]searchHit, error) {
		var rows []searchRow
		err := h.client.Campus.Query().Where(pred).Order(searchOrder).Limit(limit).
			Select(campus.FieldID, campus.FieldName, campus.FieldOrgID).Scan(ctx, &rows)
		return hitsFromRows(rows, false), err
	}},
	{peeringdb.TypeCarrier, func(ctx context.Context, h *Handler, pred func(*sql.Selector), limit int) ([]searchHit, error) {
		var rows []searchRow
		err := h.client.Carrier.Query().Where(pred).Order(searchOrder).Limit(limit).
			Select(carrier.FieldID, carrier.FieldName, carrier.FieldOrgID).Scan(ctx, &rows)
		return hitsFromRows(rows, false), err
	}},
}

// serveSearch answers the search route. search_api_view is a plain
// Django view: there is no content negotiation, no format and no meta
// envelope, and ?pretty does not apply (the body is always indented).
//
//  1. No Authorization header: 401. A header that does not split into
//     two words: 401 (rest.py:2016-2027). Any other header runs the
//     search. The mirror does not check the key (docs/API.md § Known
//     Divergences).
//  2. No q key: {} (rest.py:2031-2032). The q values are joined with a
//     space; an empty result is {} too (perform_search,
//     views.py:3680-3682).
//  3. The words from a "near" or "in" word on are location words
//     (searchTerms). The mirror has no geocoder, so it drops them and
//     does not filter by location.
//  4. The search runs the name_search match of each type, ok rows only,
//     and sorts the hits (searchOrderHits).
//
// The view does not check the method, so OPTIONS and TRACE search as
// GET does. Another method first gets the Django CSRF check, which
// always answers 403 in the mirror (csrfRejectBody).
func (h *Handler) serveSearch(w http.ResponseWriter, r *http.Request) {
	// The ?pretty middleware re-indents DRF output only.
	if pw, ok := w.(*prettyWriter); ok {
		w = pw.ResponseWriter
	}
	if !csrfSafeMethod(r.Method) {
		writeSearchBody(w, http.StatusForbidden, csrfRejectBody(r))
		return
	}
	auth := strings.Join(r.Header.Values("Authorization"), ",")
	if auth == "" {
		writeSearchBody(w, http.StatusUnauthorized, searchNoKeyBody)
		return
	}
	if n := len(pyLatin1Fields(auth)); n != 2 {
		writeSearchBody(w, http.StatusUnauthorized, searchEmptyKeyBody)
		return
	}
	q, ok := pyParseQuery(r.URL.RawQuery)["q"]
	original := strings.Join(q, " ")
	if !ok || original == "" {
		writeSearchBody(w, http.StatusOK, "{}")
		return
	}
	qs := strings.Join(searchTerms(q), " ")
	parsed, err := parseNameSearch(qs)
	if err != nil {
		writeSearchBody(w, http.StatusBadRequest, `{"error": `+pyJSONString(err.Error())+`}`)
		return
	}
	hits, err := h.runSearch(r.Context(), parsed)
	if err != nil {
		slog.ErrorContext(r.Context(), "pdbcompat: search query failed",
			slog.String("error", err.Error()),
		)
		writeSearchBody(w, http.StatusInternalServerError, `{"error": "failed to query records"}`)
		return
	}
	writeSearchBody(w, http.StatusOK, renderSearch(searchOrderHits(hits, searchExactTerms(qs), original)))
}

// runSearch runs the match of q on each type and returns at most
// searchLimit hits: the first ones in lower-case name order over all
// types (docs/API.md § Known Divergences). Each type returns at most
// searchLimit rows, so the cut is exact up to SQLite lower(), which
// folds ASCII letters only.
func (h *Handler) runSearch(ctx context.Context, q nameSearchQuery) ([]searchHit, error) {
	var all []searchHit
	for i, st := range searchTypes {
		match := q.match(st.tag)
		if match == nil {
			continue
		}
		pred := func(s *sql.Selector) { s.Where(sql.And(nameSearchOK(s.C), match(s.C))) }
		hits, err := st.query(ctx, h, pred, searchLimit)
		if err != nil {
			return nil, fmt.Errorf("search %s: %w", st.tag, err)
		}
		for j := range hits {
			hits[j].order = i
		}
		all = append(all, hits...)
	}
	slices.SortStableFunc(all, func(a, b searchHit) int {
		return cmp.Or(
			strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
			cmp.Compare(a.order, b.order),
			cmp.Compare(a.ID, b.ID),
		)
	})
	if len(all) > searchLimit {
		all = all[:searchLimit]
	}
	return all, nil
}

// searchOrderHits groups the hits by type and orders each type as
// order_results_alphabetically does (search_v2.py:298-356), with no
// search score: by lower-case name, then, when the query has " OR ",
// the rows whose name holds the term after the first " OR " first, or
// else the first row whose lower-case name is one of exact to the
// front. hits must be in lower-case name order.
func searchOrderHits(hits []searchHit, exact []string, original string) [][]searchHit {
	byType := make([][]searchHit, len(searchTypes))
	for _, hit := range hits {
		byType[hit.order] = append(byType[hit.order], hit)
	}
	_, after, isOR := strings.Cut(original, " OR ")
	if isOR {
		// Python str.split(" OR ")[1]: the text up to the next " OR ".
		after, _, _ = strings.Cut(after, " OR ")
		orTerm := strings.ToLower(strings.TrimFunc(after, pyIsSpace))
		for i, list := range byType {
			var first, rest []searchHit
			for _, hit := range list {
				if strings.Contains(strings.ToLower(hit.Name), orTerm) {
					first = append(first, hit)
				} else {
					rest = append(rest, hit)
				}
			}
			byType[i] = append(first, rest...)
		}
		return byType
	}
	for _, list := range byType {
		for j, hit := range list {
			if slices.Contains(exact, strings.ToLower(hit.Name)) {
				copy(list[1:j+1], list[:j])
				list[0] = hit
				break
			}
		}
	}
	return byType
}

// searchExactTerms returns the names that search_v2 puts in front of a
// type (look_for_exact_matches, search_v2.py:897-909, and
// order_results_alphabetically, :319-323): the lower-case escaped
// keywords of qs without AND and OR, and all of them joined with a
// space. A partial IP address query has no keywords, so only the empty
// join is left.
func searchExactTerms(qs string) []string {
	var exact []string
	norm := strings.Join(strings.FieldsFunc(qs, pyIsSpace), " ")
	if !partialIPv4.MatchString(norm) && !partialIPv6.MatchString(norm) {
		for _, kw := range strings.FieldsFunc(escapeQueryString(qs), pyIsSpace) {
			if !isSearchOperator(kw) {
				exact = append(exact, strings.ToLower(kw))
			}
		}
	}
	return append(exact, strings.Join(exact, " "))
}

// searchLatitude and searchLongitude are is_valid_latitude and
// is_valid_longitude (search_v2.py:283-296). Python \d matches every
// Unicode decimal digit.
var (
	searchLatitude  = regexp.MustCompile(`^-?(([0-8]?[0-9]\.\p{Nd}+)|(90(\.0+)?))$`)
	searchLongitude = regexp.MustCompile(`^-?((((1[0-7][0-9])|([0-9]?[0-9]))\.\p{Nd}+)|180(\.0+)?)$`)
)

// searchTerms returns the q values with the location words removed, as
// process_near_search and process_in_search (views.py:3823-3878) change
// q before the search:
//
//   - "near" (any case) with a next word "<lat>,<lon>" or
//     "<lat>,<lon>,<dist>" removes the two words
//     (handle_coordinate_search).
//   - Another "near" removes the words from it to the end of the value
//     (handle_city_country_search, which runs when no coordinates were
//     found; the mirror has no geocoder, so no later "near" finds a
//     city).
//   - Then the first "in" (any case) removes the words from it to the end
//     of the value.
//
// A value with such a word is first rewritten with ",\s*" as "," and
// split into words. Each "near" works on the words of the value before
// any change, so the last "near" of a value decides.
func searchTerms(q []string) []string {
	q = slices.Clone(q)
	coords := false
	for idx := range q {
		words := strings.FieldsFunc(pyCommaSpace(q[idx]), pyIsSpace)
		for i, w := range words {
			if strings.ToLower(w) != "near" {
				continue
			}
			if i+1 < len(words) {
				parts := strings.Split(words[i+1], ",")
				if (len(parts) == 2 || len(parts) == 3) && searchLatitude.MatchString(parts[0]) && searchLongitude.MatchString(parts[1]) {
					q[idx] = strings.Join(slices.Concat(words[:i], words[i+2:]), " ")
					coords = true
				}
			}
			if !coords {
				q[idx] = strings.Join(words[:i], " ")
			}
		}
	}
	for idx := range q {
		words := strings.FieldsFunc(pyCommaSpace(q[idx]), pyIsSpace)
		for i, w := range words {
			if strings.ToLower(w) == "in" {
				q[idx] = strings.Join(words[:i], " ")
				break
			}
		}
	}
	return q
}

// pyCommaSpace is re.sub(r",\s*", ",", s).
func pyCommaSpace(s string) string {
	var b strings.Builder
	skip := false
	for _, r := range s {
		switch {
		case r == ',':
			skip = true
			b.WriteRune(r)
		case skip && pyIsSpace(r):
		default:
			skip = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

// pyLatin1Fields is Python str.split() on a header value, which WSGI
// decodes as ISO-8859-1: besides the ASCII spaces, 0x1c-0x1f, 0x85 and
// 0xa0 split too.
func pyLatin1Fields(s string) []string {
	return strings.FieldsFunc(latin1(s), pyIsSpace)
}

// latin1 decodes s as ISO-8859-1.
func latin1(s string) string {
	var b strings.Builder
	for i := range len(s) {
		b.WriteRune(rune(s[i]))
	}
	return b.String()
}

// renderSearch writes the result as JsonResponse(json_dumps_params=
// {"indent": 2, "ensure_ascii": False}) writes serialize_search_results
// (rest.py:1975-2006): an object with the 6 type keys, and for each hit
// id, name, org_id and, on net, asn.
func renderSearch(byType [][]searchHit) string {
	var b strings.Builder
	b.WriteString("{")
	for i, list := range byType {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString("\n  " + pyJSONString(searchTypes[i].tag) + ": ")
		if len(list) == 0 {
			b.WriteString("[]")
			continue
		}
		b.WriteString("[")
		for j, hit := range list {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString("\n    {\n      \"id\": " + strconv.Itoa(hit.ID))
			b.WriteString(",\n      \"name\": " + pyJSONString(hit.Name))
			b.WriteString(",\n      \"org_id\": " + pyJSONInt(hit.OrgID))
			if searchTypes[i].tag == peeringdb.TypeNet {
				b.WriteString(",\n      \"asn\": " + pyJSONInt(hit.ASN))
			}
			b.WriteString("\n    }")
		}
		b.WriteString("\n  ]")
	}
	b.WriteString("\n}")
	return b.String()
}

// pyJSONInt writes an optional integer as JSON.
func pyJSONInt(v *int) string {
	if v == nil {
		return "null"
	}
	return strconv.Itoa(*v)
}

// pyJSONString quotes s as json.dumps with ensure_ascii=False does: it
// escapes the quote, the backslash and the characters below 0x20
// (\n, \r, \t, \b and \f in short form, the others as \u00XX), and
// writes every other character as it is, U+2028 and U+2029 included.
func pyJSONString(s string) string {
	return pyJSONQuote(s, false)
}

// pyJSONASCIIString quotes s as json.dumps with ensure_ascii=True does:
// as pyJSONString, but every character that is not printable ASCII
// (0x20-0x7e) is a \uXXXX escape, with a surrogate pair above U+FFFF.
func pyJSONASCIIString(s string) string {
	return pyJSONQuote(s, true)
}

// pyJSONQuote quotes s as json.dumps does with ensure_ascii set to
// ascii.
func pyJSONQuote(s string, ascii bool) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			switch {
			case r < 0x20, ascii && r > 0x7e && r <= 0xffff:
				fmt.Fprintf(&b, `\u%04x`, r)
			case ascii && r > 0xffff:
				r -= 0x10000
				fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+r>>10, 0xdc00+r&0x3ff)
			default:
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// writeSearchBody writes a search response with the Content-Type of
// Django JsonResponse.
func writeSearchBody(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}
