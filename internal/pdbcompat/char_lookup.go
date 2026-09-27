package pdbcompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
)

// errRangeValue is the error of a range lookup whose value is not two
// characters long. Upstream then fails with a 500 (see docs/API.md §
// Known Divergences).
var errRangeValue = errors.New("a range lookup needs a value of two characters")

// ixLanNameLookup returns the seed of a netixlan name key that
// get_relation_filters keeps as a Django lookup on the name of the
// ixlan (2.83.0 serializers.py:641-654, related_to_name in
// models.py:6172-6197): name__X, where X is not an operator of
// get_relation_filters, and name__X__Y. Upstream strips an "_id" suffix
// from X (queryable_field_xl, serializers.py:403-425). A Y that is an
// operator follows the lookup X, and Django rejects the key. Another Y
// is dropped. The renames of queryable_field_xl give no lookup name,
// so only the suffix matters. The other key forms return ok=false.
func ixLanNameLookup(tail []string) (relationSeed, bool) {
	switch {
	case len(tail) == 1 && !metaOperators[tail[0]]:
	case len(tail) == 2:
	default:
		return relationSeed{}, false
	}
	lookup := stripIDSuffix(tail[0])
	if len(tail) == 2 && metaOperators[tail[1]] {
		// No transform exists, so charFieldLookup rejects it.
		lookup += "__" + tail[1]
	}
	return relationSeed{hops: []string{"ixlan"}, pinAt: 1, shape: shapeWholeKey, field: "name", bareOp: lookup, lookup: true}, true
}

// charFieldLookup returns the filter of the Django lookup on a string
// column, as upstream runs it on MySQL with a utf8_unicode_ci collation
// (django/db/backends/mysql/base.py operators). The collation ignores
// case, and "=" ignores trailing spaces (PAD SPACE). The lookups with
// LIKE BINARY (contains, startswith, endswith) compare the case too.
// The value is used as given: prepare_query reads the query value
// before the filter loop folds it. A lookup that Django does not know
// returns errInvalidQuery, the 400 of a FieldError (rest.py:499-500).
func charFieldLookup(col, lookup, value string) (func(*sql.Selector), error) {
	switch lookup {
	case "exact":
		return charFieldIn(col, []string{value}), nil
	case "iexact", "icontains", "istartswith":
		return buildPredicate(col, lookup, value, FieldString, false)
	case "iendswith":
		return textLike(col, "LOWER(%s)", "%"+likeEscape(strings.ToLower(value))), nil
	case "contains":
		return caseSensitiveGlob(col, "*"+globEscape(value)+"*"), nil
	case "startswith":
		return caseSensitiveGlob(col, globEscape(value)+"*"), nil
	case "endswith":
		return caseSensitiveGlob(col, "*"+globEscape(value)), nil
	case "lt", "lte", "gt", "gte":
		return buildStringComparison(col, lookup, value, false), nil
	case "in":
		// Django iterates the characters of the string
		// (FieldGetDbPrepValueIterableMixin). No character matches no
		// row.
		chars := strings.Split(value, "")
		if len(chars) == 0 {
			return nil, errEmptyIn
		}
		return charFieldIn(col, chars), nil
	case "range":
		// BETWEEN the first and the second character. Another length
		// fails as upstream builds the SQL (lookups.py Range).
		chars := strings.Split(value, "")
		if len(chars) != 2 {
			return nil, errRangeValue
		}
		low := buildStringComparison(col, "gte", chars[0], false)
		high := buildStringComparison(col, "lte", chars[1], false)
		return func(s *sql.Selector) {
			low(s)
			high(s)
		}, nil
	case "isnull":
		return nil, errIsNullValue
	case "regex", "iregex":
		// MySQL and SQLite do not share a regular expression dialect
		// (see docs/API.md § Known Divergences).
		return nil, fmt.Errorf("the %s lookup is not supported", lookup)
	}
	return nil, errInvalidQuery
}

// charFieldIn keeps the rows whose col equals one of vals, with case
// and trailing spaces ignored on both sides.
func charFieldIn(col string, vals []string) func(*sql.Selector) {
	lowered := make([]string, len(vals))
	for i, v := range vals {
		lowered[i] = strings.ToLower(strings.TrimRight(v, " "))
	}
	arr, _ := json.Marshal(lowered) // a []string always marshals
	return func(s *sql.Selector) {
		s.Where(sql.ExprP("RTRIM(LOWER("+s.C(col)+"), ' ') IN (SELECT value FROM json_each(?))", string(arr)))
	}
}

// caseSensitiveGlob keeps the rows whose col matches the GLOB pattern.
// GLOB compares the case, as LIKE BINARY does.
func caseSensitiveGlob(col, pattern string) func(*sql.Selector) {
	return func(s *sql.Selector) {
		s.Where(sql.ExprP(s.C(col)+" GLOB ?", pattern))
	}
}

// globEscape makes the GLOB wildcards of v match themselves.
func globEscape(v string) string {
	return strings.NewReplacer("*", "[*]", "?", "[?]", "[", "[[]").Replace(v)
}
