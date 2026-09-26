package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the name_search key of the upstream list views
// (PeeringDB 2.83.0 ModelViewSet.get_queryset, rest.py:532-553, and
// search_v2, search_v2.py:861-978). Upstream sends the value to its
// Elasticsearch index and filters the list by the ids of the hits. The
// mirror runs the same kinds of match in SQL: words as substrings of the
// indexed text fields, digits as an ASN prefix, and a partial IP address
// as a prefix of the netixlan addresses.

// nameSearchMode is the kind of match that a name_search value selects.
type nameSearchMode int

const (
	nsNone  nameSearchMode = iota // matches no row
	nsWords                       // every word is a substring of a field
	nsASN                         // a digit value
	nsIPv4                        // a netixlan ipaddr4 prefix
	nsIPv6                        // a netixlan ipaddr6 prefix
)

// nameSearchQuery is a parsed name_search value.
type nameSearchQuery struct {
	mode nameSearchMode
	// words are the lower-case words of nsWords, without the operators
	// AND and OR.
	words []string
	// digits is clean_term of nsASN as upstream builds it (the digits of
	// any script).
	digits string
	// asn is int(clean_term) of nsASN. hasASN is false when the value
	// does not fit in 64 bits.
	asn    int64
	hasASN bool
	// prefix is the lower-case address prefix of nsIPv4 and nsIPv6.
	prefix string
}

var (
	// partialIPv4 is PARTIAL_IPV4_ADDRESS (search_v2.py:24).
	partialIPv4 = regexp.MustCompile(`^([0-9]{1,3}\.){1,3}([0-9]{1,3})?$`)
	// partialIPv6 is PARTIAL_IPV6_ADDRESS (search_v2.py:25).
	partialIPv6 = regexp.MustCompile(`^([0-9A-Fa-f]{1,4}|:):[0-9A-Fa-f:]*$`)
)

// pyDigitNotDecimal holds the characters for which Python str.isdigit
// is true and that are not Unicode Nd (Numeric_Type=Digit). int()
// rejects them. Generated with CPython 3.13 (unicodedata 15.1.0):
//
//	[c for c in range(0x110000) if chr(c).isdigit() and not chr(c).isdecimal()]
var pyDigitNotDecimal = &unicode.RangeTable{
	R16: []unicode.Range16{
		{Lo: 0x00b2, Hi: 0x00b3, Stride: 1}, {Lo: 0x00b9, Hi: 0x00b9, Stride: 1},
		{Lo: 0x1369, Hi: 0x1371, Stride: 1}, {Lo: 0x19da, Hi: 0x19da, Stride: 1},
		{Lo: 0x2070, Hi: 0x2070, Stride: 1}, {Lo: 0x2074, Hi: 0x2079, Stride: 1},
		{Lo: 0x2080, Hi: 0x2089, Stride: 1}, {Lo: 0x2460, Hi: 0x2468, Stride: 1},
		{Lo: 0x2474, Hi: 0x247c, Stride: 1}, {Lo: 0x2488, Hi: 0x2490, Stride: 1},
		{Lo: 0x24ea, Hi: 0x24ea, Stride: 1}, {Lo: 0x24f5, Hi: 0x24fd, Stride: 1},
		{Lo: 0x24ff, Hi: 0x24ff, Stride: 1}, {Lo: 0x2776, Hi: 0x277e, Stride: 1},
		{Lo: 0x2780, Hi: 0x2788, Stride: 1}, {Lo: 0x278a, Hi: 0x2792, Stride: 1},
	},
	R32: []unicode.Range32{
		{Lo: 0x10a40, Hi: 0x10a43, Stride: 1}, {Lo: 0x10e60, Hi: 0x10e68, Stride: 1},
		{Lo: 0x11052, Hi: 0x1105a, Stride: 1}, {Lo: 0x1f100, Hi: 0x1f10a, Stride: 1},
	},
}

// nameSearchFields lists the columns that the upstream search index of a
// type holds among name, name_long, aka, city and irr_as_set
// (documents.py:282-817). A type that is not in the map has no index:
// a non-empty name_search matches no row (rest.py:536-553).
var nameSearchFields = map[string][]string{
	peeringdb.TypeOrg:     {"name", "aka", "name_long", "city"},
	peeringdb.TypeFac:     {"name", "aka", "name_long", "city"},
	peeringdb.TypeIX:      {"name", "aka", "name_long", "city"},
	peeringdb.TypeNet:     {"name", "aka", "name_long", "irr_as_set"},
	peeringdb.TypeCampus:  {"name", "aka", "name_long", "city"},
	peeringdb.TypeCarrier: {"name", "aka", "name_long"},
}

// nameSearchASNFields are the text fields of the ASN query
// (search_v2.py:398-413).
var nameSearchASNFields = []string{"name", "aka", "name_long"}

// pyIsSpace reports whether Python str.isspace is true for r. It is
// unicode.IsSpace plus U+001C..U+001F, which str.split() also splits
// on.
func pyIsSpace(r rune) bool {
	return unicode.IsSpace(r) || r >= 0x1c && r <= 0x1f
}

// escapeQueryString ports escape_query_string (search_v2.py:208-252). It
// puts a backslash before each special character and doubles a
// backslash. "&&" and "||" are in the upstream list, but the loop
// compares one character at a time, so they are never escaped.
func escapeQueryString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case strings.ContainsRune(`":*?+-=><!(){}[]^~/`, r):
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isSearchOperator reports whether a keyword is one of the operators
// that add_and_between_keywords (search_v2.py:255-280) keeps.
func isSearchOperator(kw string) bool {
	return kw == "AND" || kw == "OR"
}

// validPartialIPv4 ports valid_partial_ipv4_address (search_v2.py:28-29):
// every non-empty octet is at most 255.
func validPartialIPv4(ip string) bool {
	for octet := range strings.SplitSeq(ip, ".") {
		if octet == "" {
			continue
		}
		n, err := strconv.Atoi(octet)
		if err != nil || n > 255 {
			return false
		}
	}
	return true
}

// parseNameSearch parses a non-empty name_search value as search_v2
// routes it (search_v2.py:880-909) and construct_query_body picks the
// query (:599-718). An error is a value that upstream int() rejects in
// the ASN query (:385, :393), which becomes 400 (rest.py:824-827).
//
// A partial IPv4 address with an octet above 255 is searched as words.
// Upstream answers 500 for it (see docs/API.md § Known Divergences).
func parseNameSearch(v string) (nameSearchQuery, error) {
	fields := strings.FieldsFunc(v, pyIsSpace)
	norm := strings.Join(fields, " ")
	switch {
	case partialIPv4.MatchString(norm):
		if validPartialIPv4(norm) {
			return nameSearchQuery{mode: nsIPv4, prefix: norm}, nil
		}
	case partialIPv6.MatchString(norm):
		s := norm
		if strings.HasSuffix(s, ":") && !strings.HasSuffix(s, "::") {
			s = strings.TrimSuffix(s, ":")
		}
		// construct_query_body runs the digit test before the IPv6
		// query (search_v2.py:618-623), so 2001: is an ASN query.
		q, isDigit, err := nameSearchDigits(s)
		if err != nil || isDigit {
			return q, err
		}
		return nameSearchQuery{mode: nsIPv6, prefix: strings.ToLower(s)}, nil
	}
	// Keywords (search_v2.py:897-909). The term is built from the
	// escaped keywords, as upstream builds it, so clean_term keeps the
	// escape backslashes.
	keywords := strings.FieldsFunc(escapeQueryString(v), pyIsSpace)
	var term strings.Builder
	for i, kw := range keywords {
		if isSearchOperator(kw) {
			term.WriteString(" " + kw)
		} else {
			term.WriteString(" *" + kw + "*")
		}
		if i < len(keywords)-1 && !isSearchOperator(kw) && !isSearchOperator(keywords[i+1]) {
			term.WriteString(" AND")
		}
	}
	clean := strings.TrimFunc(strings.ReplaceAll(strings.ReplaceAll(term.String(), "*", ""), "AND", ""), pyIsSpace)
	q, isDigit, err := nameSearchDigits(clean)
	if err != nil || isDigit {
		return q, err
	}
	var words []string
	for _, f := range fields {
		if !isSearchOperator(f) {
			words = append(words, strings.ToLower(f))
		}
	}
	if len(words) == 0 {
		return nameSearchQuery{mode: nsNone}, nil
	}
	return nameSearchQuery{mode: nsWords, words: words}, nil
}

// nameSearchDigits runs the digit test of construct_query_body on
// clean_term c: Python str.isdigit (search_v2.py:619), then int() in
// construct_asn_number_query (:385, :393). isDigit is false when
// isdigit is false. err is set when isdigit is true and int() raises
// ValueError: for a character that is a digit but not a decimal digit
// (pyDigitNotDecimal, for example ²), or for more than
// pyIntMaxStrDigits digits.
func nameSearchDigits(c string) (q nameSearchQuery, isDigit bool, err error) {
	if c == "" {
		return nameSearchQuery{}, false, nil
	}
	notDecimal := false
	for _, r := range c {
		if ndValue(r) >= 0 {
			continue
		}
		if !unicode.Is(pyDigitNotDecimal, r) {
			return nameSearchQuery{}, false, nil
		}
		notDecimal = true
	}
	if notDecimal {
		return nameSearchQuery{}, true, fmt.Errorf("invalid literal for int() with base 10: '%s'", c)
	}
	if n := utf8.RuneCountInString(c); n > pyIntMaxStrDigits {
		return nameSearchQuery{}, true, fmt.Errorf("Exceeds the limit (%d digits) for integer string conversion: value has %d digits; use sys.set_int_max_str_digits() to increase the limit", pyIntMaxStrDigits, n) //nolint:staticcheck // upstream text, starts with a capital
	}
	ascii := make([]byte, 0, len(c))
	for _, r := range c {
		ascii = append(ascii, asciiDigits[ndValue(r)])
	}
	q = nameSearchQuery{mode: nsASN, digits: c}
	if asn, perr := strconv.ParseInt(string(ascii), 10, 64); perr == nil {
		q.asn, q.hasASN = asn, true
	}
	return q, true, nil
}

// nameSearchColumn returns the SQL text of a column. The outer query
// uses the listed table; the gate of the id__in union uses an alias.
type nameSearchColumn func(column string) string

// match returns the match predicate of q on typ, or nil when q can
// match no row of typ: nsNone, or an IP mode on a type whose index has
// no IP fields (documents.py:140-176: only net and ix).
func (q nameSearchQuery) match(typ string) func(c nameSearchColumn) *sql.Predicate {
	switch q.mode {
	case nsWords:
		cols := nameSearchFields[typ]
		return func(c nameSearchColumn) *sql.Predicate {
			return nameSearchWords(c, cols, q.words)
		}
	case nsASN:
		words := []string{strings.ToLower(q.digits)}
		return func(c nameSearchColumn) *sql.Predicate {
			p := nameSearchWords(c, nameSearchASNFields, words)
			if typ != peeringdb.TypeNet || !q.hasASN {
				return p
			}
			// The term and prefix clauses on asn and asn.raw
			// (search_v2.py:380-395): the decimal text of the ASN starts
			// with the digits of int(clean_term). The exact match is a
			// case of the prefix.
			return sql.Or(p, sql.ExprP("CAST("+c("asn")+" AS TEXT) LIKE ?", strconv.FormatInt(q.asn, 10)+"%"))
		}
	case nsIPv4, nsIPv6:
		if typ != peeringdb.TypeNet && typ != peeringdb.TypeIX {
			return nil
		}
		col := networkixlan.FieldIpaddr4
		if q.mode == nsIPv6 {
			col = networkixlan.FieldIpaddr6
		}
		return func(c nameSearchColumn) *sql.Predicate {
			return nameSearchIP(c, typ, col, q.prefix)
		}
	case nsNone:
	}
	return nil
}

// nameSearchWords keeps the rows where every word is a case-insensitive
// substring of at least one of cols. Different words can match
// different columns. This approximates the query_string clause
// `*w1* AND *w2*` over the index fields (search_v2.py:574-585). The
// words bind as ONE JSON array, so a long value adds no SQL term per
// word. coalesce is load-bearing: a NULL column would make the inner
// NOT NULL, and NOT EXISTS would then keep the row. SQLite lower()
// folds ASCII letters only; the words are folded in Go.
func nameSearchWords(c nameSearchColumn, cols, words []string) *sql.Predicate {
	terms := make([]string, len(cols))
	for i, col := range cols {
		terms[i] = "instr(lower(coalesce(" + c(col) + ", '')), w.value) > 0"
	}
	list, err := json.Marshal(words)
	if err != nil {
		// A []string always marshals.
		panic(fmt.Sprintf("marshal name_search words: %v", err))
	}
	return sql.ExprP("NOT EXISTS (SELECT 1 FROM json_each(?) AS w WHERE NOT ("+strings.Join(terms, " OR ")+"))", string(list))
}

// nameSearchIP keeps the net or ix rows that have a live netixlan
// (ok or not-operational) whose address starts with prefix, the prefix
// clause on the .raw field (search_v2.py:427-522). On ix the netixlan
// must be on an ok ixlan (documents.py:150-163). The prefix match is a
// range on the stored text: lo <= addr < hi, where hi is prefix with its
// last byte plus one. The regexes allow only 0-9 . a-f :, so the last
// byte is below 0x7f. The range reads the netixlan address index.
func nameSearchIP(c nameSearchColumn, typ, col, prefix string) *sql.Predicate {
	hi := ipPrefixUpperBound(prefix)
	nixl := sql.Table(networkixlan.Table)
	sub := sql.Select().From(nixl)
	conds := []*sql.Predicate{
		likelyStatusInP(nixl.C(networkixlan.FieldStatus), pdbtypes.LiveStatuses(peeringdb.TypeNetIXLan)),
		sql.GTE(nixl.C(col), prefix),
		sql.LT(nixl.C(col), hi),
	}
	if typ == peeringdb.TypeIX {
		lan := sql.Table(ixlan.Table)
		// Join gives lan an alias, so lan.C must run after it.
		sub.Join(lan).On(nixl.C(networkixlan.FieldIxlanID), lan.C(ixlan.FieldID))
		sub.Select(lan.C(ixlan.FieldIxID))
		conds = append(conds, likelyStatusInP(lan.C(ixlan.FieldStatus), []string{"ok"}))
	} else {
		sub.Select(nixl.C(networkixlan.FieldNetID))
	}
	sub.Where(sql.And(conds...))
	return sql.In(c(parentPKColumn), sub)
}

// ipPrefixUpperBound returns the smallest string above every string
// that starts with prefix: prefix with its last byte plus one.
func ipPrefixUpperBound(prefix string) string {
	b := []byte(prefix)
	b[len(b)-1]++
	return string(b)
}

// likelyStatusInP is the predicate form of likelyStatusIn on column
// col.
func likelyStatusInP(col string, statuses []string) *sql.Predicate {
	args := make([]any, len(statuses))
	for i, v := range statuses {
		args[i] = v
	}
	return sql.P(func(b *sql.Builder) {
		b.WriteString("likely(").Join(sql.In(col, args...)).WriteString(")")
	})
}

// nameSearchResult is the outcome of the name_search pre-pass.
type nameSearchResult struct {
	// pred is the filter predicate. nil when the key is absent or
	// empty, or when none or empty is set.
	pred func(*sql.Selector)
	// consumed holds the keys that the filter loop must skip.
	consumed map[string]bool
	// none is upstream qset.none(): no row can match, and upstream
	// returns before its filter loop (rest.py:550-553).
	none bool
	// empty is set for an empty id__in together with name_search.
	empty bool
	// hit is the search part of pred alone: the ok rows that match,
	// without the id__in union. It is nil when no search runs: the key
	// is absent or empty, or none is set. A request with a slice or a
	// negative skip uses it to find out if the search has a hit.
	hit func(*sql.Selector)
}

// resolveNameSearch handles the name_search key before the filter loop.
// Only the exact key applies, with its last value (QueryDict.get). An
// empty value does nothing (rest.py:535). On a type without a search
// index, a non-empty value matches no row, and the value is not parsed,
// as upstream never calls search_v2 then.
//
// Matches are ok rows only, as the index holds only ok objects
// (documents.py:97-136, search_v2.py:774). An id__in key in the same
// request is unioned with the matches (rest.py:685-690): the result is
// the listed ids or the matches, and every other filter applies. When
// nothing matches, the result is empty also with id__in, as upstream
// returns qset.none() first.
//
// The error is prefixed with the key ("filter name_search: ..." or
// "filter id__in: ...") and becomes 400.
func resolveNameSearch(tc TypeConfig, params url.Values) (nameSearchResult, error) {
	vals := params["name_search"]
	if len(vals) == 0 {
		return nameSearchResult{}, nil
	}
	res := nameSearchResult{consumed: map[string]bool{"name_search": true}}
	v := vals[len(vals)-1]
	if v == "" {
		return res, nil
	}
	if _, indexed := nameSearchFields[tc.Name]; !indexed {
		res.none = true
		return res, nil
	}
	q, err := parseNameSearch(v)
	if err != nil {
		return nameSearchResult{}, fmt.Errorf("filter name_search: %w", err)
	}
	match := q.match(tc.Name)
	if match == nil {
		res.none = true
		return res, nil
	}
	res.hit = func(s *sql.Selector) {
		s.Where(sql.And(nameSearchOK(s.C), match(s.C)))
	}
	ids := params["id__in"]
	if len(ids) == 0 {
		res.pred = res.hit
		return res, nil
	}
	pIn, empty, _, err := buildLocalPredicate("id", "in", ids[len(ids)-1], tc, false)
	if err != nil {
		return nameSearchResult{}, fmt.Errorf("filter id__in: %w", err)
	}
	res.consumed["id__in"] = true
	if empty {
		res.empty = true
		return res, nil
	}
	res.pred = func(s *sql.Selector) {
		table := s.TableName()
		u := sql.Table(table).As("u")
		listed := sql.Select(u.C(parentPKColumn)).From(u)
		pIn(listed)
		// The gate is upstream's "no hit, then qset.none()". LIMIT 1
		// keeps it an uncorrelated scalar subquery that runs once;
		// without it SQLite turns the EXISTS into a semi-join and sorts
		// the page in a temp B-tree.
		m := sql.Table(table).As("m")
		gate := sql.Select(m.C(parentPKColumn)).From(m)
		gate.Where(sql.And(nameSearchOK(gate.C), match(gate.C))).Limit(1)
		s.Where(sql.And(
			sql.Or(sql.In(s.C(parentPKColumn), listed), sql.And(nameSearchOK(s.C), match(s.C))),
			sql.Exists(gate),
		))
	}
	return res, nil
}

// nameSearchOK pins a match to status ok. likely() keeps the list on
// its order index.
func nameSearchOK(c nameSearchColumn) *sql.Predicate {
	return likelyStatusInP(c("status"), []string{"ok"})
}
