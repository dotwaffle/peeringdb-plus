package pdbcompat

import (
	"encoding/json"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// A multi-value choice field (net info_types, fac
// available_voltage_services) is a django-peeringdb MultipleChoiceField.
// Upstream stores it as one string: the selected choices in the order
// of the choice list, joined with commas (django-peeringdb 969dd11
// fields.py:61-71). Upstream filters compare that string.
//
// The mirror stores the API value, a JSON array. Its order is not the
// order of the choice list, because DRF serializes the values as a set.
// multiChoiceExpr rebuilds the upstream string from the array in SQL, so
// the mirror compares the same text as upstream.

// multiChoiceLists holds the choice list of each multi-value field, in
// upstream order (django-peeringdb 969dd11 const.py:114-125, :203-208).
// The keys are the column names.
var multiChoiceLists = map[string][]string{
	"info_types": {
		"NSP", "Content", "Cable/DSL/ISP", "Enterprise",
		"Educational/Research", "Non-Profit", "Route Server",
		"Network Services", "Route Collector", "Government",
	},
	"available_voltage_services": {"No Power", "48 VDC", "400 VAC", "480 VAC"},
}

// opCanonicalExact is the operator of an exact match that converts the
// filter value to the stored form first. It is not a query operator.
// Only a relation key without an operator uses it: make_relation_filter
// builds an exact lookup (2.83.0 models.py:221-234), and Django converts
// the value of an exact lookup with get_prep_value. The iexact lookup of
// the filter loop compares the value as given.
const opCanonicalExact = "canonical_exact"

// canonicalChoices converts a filter value to the stored form, as
// get_prep_value does: the choices that occur in value as a substring,
// in choice-list order, joined with commas. A value that contains no
// choice becomes "". Upstream converts the value of an __in item and of
// a comparison (Django lookups with prepare_rhs), but not the value of
// iexact, icontains or istartswith.
func canonicalChoices(choices []string, value string) string {
	var picked []string
	for _, c := range choices {
		if strings.Contains(value, c) {
			picked = append(picked, c)
		}
	}
	return strings.Join(picked, ",")
}

// multiChoiceExpr returns the SQL expression of the upstream string of
// col: the array values in choice-list order, joined with commas, or ""
// for an empty or NULL array. A value that is not in the choice list
// sorts after the known choices, in array order. col is a quoted
// column reference. The CASE uses only the constant choice list.
func multiChoiceExpr(col string, choices []string) string {
	var b strings.Builder
	b.WriteString("COALESCE((SELECT group_concat(je.value, ',' ORDER BY CASE je.value")
	for i, c := range choices {
		fmt.Fprintf(&b, " WHEN '%s' THEN %d", strings.ReplaceAll(c, "'", "''"), i)
	}
	fmt.Fprintf(&b, " ELSE %d END, je.key) FROM json_each(%s) AS je), '')", len(choices), col)
	return b.String()
}

// likeEscape escapes the LIKE wildcards in v for ESCAPE '\'. Django
// escapes them the same way for contains, startswith and endswith.
func likeEscape(v string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(v)
}

// multiChoiceWhere returns a predicate that applies cond to the lower
// case upstream string of col. cond holds one %s for the expression and
// one ? for each arg.
func multiChoiceWhere(col string, choices []string, cond string, args ...any) func(*sql.Selector) {
	return func(s *sql.Selector) {
		expr := "LOWER(" + multiChoiceExpr(s.C(col), choices) + ")"
		s.Where(sql.ExprP(fmt.Sprintf(cond, expr), args...))
	}
}

// multiChoiceLikeAny returns a predicate that keeps the rows whose
// upstream string of col matches one of the LIKE patterns. The patterns
// must be lower case, with the wildcards of the value escaped.
//
// The predicate builds the upstream string once per row and binds the
// patterns as one JSON array, as buildIn does. The LIMIT stops SQLite
// from flattening the derived table, which would build the string again
// for each pattern. The cost is still rows x patterns, as for the OR of
// LIKE terms that upstream runs, but the SQL does not grow with the
// number of patterns.
func multiChoiceLikeAny(col string, patterns []string) (func(*sql.Selector), error) {
	list, err := json.Marshal(patterns)
	if err != nil {
		return nil, fmt.Errorf("marshal LIKE patterns: %w", err)
	}
	return multiChoiceWhere(col, multiChoiceLists[col],
		`EXISTS (SELECT 1 FROM (SELECT %s AS s LIMIT 1) AS x, json_each(?) AS p WHERE x.s LIKE p.value ESCAPE '\')`,
		string(list)), nil
}

// buildMultiChoicePredicate builds the filter of a multi-value field.
// MySQL compares text without case under the upstream collation, so
// each operator compares lower case text.
//
//   - exact and iexact compare the value as given with the stored string
//     (rest.py:682-683).
//   - icontains and istartswith match the value as a substring or a
//     prefix of the stored string (rest.py:659-662).
//   - __in converts each item to the stored form and matches a stored
//     string that equals one of them (rest.py:663-666).
//   - A comparison converts the value to the stored form and compares
//     the two strings.
func buildMultiChoicePredicate(col, op, value string) (func(*sql.Selector), error) {
	choices, ok := multiChoiceLists[col]
	if !ok {
		return nil, fmt.Errorf("no choice list for multi-value field %q", col)
	}
	lower := strings.ToLower(value)
	switch coerceToCaseInsensitive(op) {
	case "", "iexact":
		return multiChoiceWhere(col, choices, "%s = ?", lower), nil
	case opCanonicalExact:
		return multiChoiceWhere(col, choices, "%s = ?", strings.ToLower(canonicalChoices(choices, value))), nil
	case "icontains":
		return multiChoiceLikeAny(col, []string{"%" + likeEscape(lower) + "%"})
	case "istartswith":
		return multiChoiceLikeAny(col, []string{likeEscape(lower) + "%"})
	case "in":
		if value == "" {
			return nil, errEmptyIn
		}
		parts := strings.Split(value, ",")
		for i, part := range parts {
			parts[i] = strings.ToLower(canonicalChoices(choices, part))
		}
		list, err := json.Marshal(parts)
		if err != nil {
			return nil, fmt.Errorf("marshal IN array: %w", err)
		}
		return multiChoiceWhere(col, choices, "%s IN (SELECT value FROM json_each(?))", string(list)), nil
	case "lt", "lte", "gt", "gte":
		return multiChoiceWhere(col, choices, "%s "+metaComparisons[op]+" ?",
			strings.ToLower(canonicalChoices(choices, value))), nil
	}
	return nil, fmt.Errorf("unsupported operator %q", op)
}

// legacyInfoTypePatterns returns the LIKE patterns of a net filter key
// that NetworkSerializer.finalize_query_params rewrites onto info_types
// (2.83.0 serializers.py:3768-3813). A net matches when its info_types
// string matches one of the patterns. ok=false for any other key. A
// nil result with ok=true means that a pattern matches every network,
// so the key filters nothing.
//
// The legacy info_type is a model property (models.py:5812-5816), so
// upstream ignores the other info_type keys. The info_types keys that
// the method does not handle reach the filter loop as a model field.
func legacyInfoTypePatterns(typ, key, value string) ([]string, bool) {
	if typ != peeringdb.TypeNet {
		return nil, false
	}
	v := likeEscape(strings.ToLower(value))
	switch key {
	case "info_type":
		// A value at the start, in the middle or at the end of the
		// string. The first pattern is a prefix of the whole string, so
		// ?info_type=NS matches a network whose first type is NSP.
		return anyLikePatterns(v+"%", "%,"+v+",%", "%,"+v), true
	case "info_type__contains":
		return anyLikePatterns("%" + v + "%"), true
	case "info_type__in", "info_types__in":
		// Each item is a substring, with spaces removed at the ends. An
		// empty item matches every network.
		parts := strings.Split(value, ",")
		for i, part := range parts {
			parts[i] = "%" + likeEscape(strings.ToLower(strings.TrimSpace(part))) + "%"
		}
		return anyLikePatterns(parts...), true
	case "info_type__startswith", "info_types__startswith":
		return anyLikePatterns(v+"%", "%,"+v+"%"), true
	}
	return nil, false
}

// anyLikePatterns returns the patterns of an OR of LIKE terms without
// duplicates, in their first order. It returns nil when one pattern
// holds only the % wildcard: that pattern matches every string, so the
// OR matches every row. The wildcards of a value are escaped, so such a
// pattern comes from an empty value.
func anyLikePatterns(patterns ...string) []string {
	out := make([]string, 0, len(patterns))
	seen := make(map[string]bool, len(patterns))
	for _, p := range patterns {
		if strings.Trim(p, "%") == "" {
			return nil
		}
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
