package pdbcompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// errEmptyIn is a sentinel returned by buildIn when an __in filter has no
// values (e.g. ?asn__in=). ParseFilters catches it and short-circuits the
// whole request via QueryOptions.EmptyResult (Phase 69 D-06, IN-02).
var errEmptyIn = errors.New("empty __in")

// parseFieldOp splits a query parameter key on the last "__" separator into
// field name and operator. If no separator is found, the operator is empty
// (meaning exact match).
func parseFieldOp(key string) (field, op string) {
	idx := strings.LastIndex(key, "__")
	if idx < 0 {
		return key, ""
	}
	return key[:idx], key[idx+2:]
}

// applyStatusMatrix returns the upstream rest.py:694-727 status predicate
// for list requests. sinceSet=false => status=ok (rest.py:725); sinceSet=true
// => status IN (ok, deleted), plus pending when isCampus (rest.py:700-712).
// Always returns a non-nil predicate — every list request needs a status
// filter per Phase 68 D-05/D-07.
func applyStatusMatrix(isCampus, sinceSet bool) func(*sql.Selector) {
	if !sinceSet {
		return sql.FieldEQ("status", "ok")
	}
	allowed := []string{"ok", "deleted"}
	if isCampus {
		allowed = append(allowed, "pending")
	}
	return sql.FieldIn("status", allowed...)
}

// coerceToCaseInsensitive maps the subset of operators that upstream
// rest.py:638-641 forces to case-insensitive variants. Non-matching operators
// pass through unchanged per D-04 (scope: contains + startswith only).
//
// Phase 69 UNICODE-02. The coercion is purely nominal — the existing
// buildContains / buildStartsWith paths already route through
// sql.FieldContainsFold / sql.FieldHasPrefixFold, which are case-insensitive
// at the SQL layer. Renaming the op here keeps the semantic contract
// explicit and gives the switch in buildPredicate a single case per
// upstream-equivalent operator.
func coerceToCaseInsensitive(op string) string {
	switch op {
	case "contains":
		return "icontains"
	case "startswith":
		return "istartswith"
	}
	return op
}

// ParseFilters translates Django-style query parameters into ent sql.Selector
// predicates. Reserved parameters (limit, skip, etc.) are skipped. Unknown
// fields are silently ignored per D-20.
//
// Return values:
//   - preds: the predicate slice to pass to ent as Where arguments
//   - emptyResult: true when an __in filter was empty (?asn__in=); the caller
//     MUST short-circuit the whole request and emit an empty data array
//     without running SQL (Phase 69 D-06, IN-02)
//   - err: set only for known fields with invalid values / operators
func ParseFilters(params url.Values, tc TypeConfig) ([]func(*sql.Selector), bool, error) {
	var predicates []func(*sql.Selector)
	for key, vals := range params {
		if len(vals) == 0 {
			continue
		}
		// Skip reserved pagination/control parameters.
		if reservedParams[key] {
			continue
		}
		field, op := parseFieldOp(key)
		// Also check if the raw key (before splitting) is reserved,
		// e.g. "fields" won't have an operator but should be skipped.
		if reservedParams[field] {
			continue
		}
		ft, ok := tc.Fields[field]
		if !ok {
			// Unknown field: silently ignore per D-20.
			continue
		}
		// Nil-map read is safe — returns false.
		folded := tc.FoldedFields[field]
		p, err := buildPredicate(field, op, vals[0], ft, folded)
		if err != nil {
			if errors.Is(err, errEmptyIn) {
				// An empty __in AND'd with anything is still empty.
				// Short-circuit the whole request.
				return nil, true, nil
			}
			return nil, false, fmt.Errorf("filter %s: %w", key, err)
		}
		predicates = append(predicates, p)
	}
	return predicates, false, nil
}

// buildPredicate maps a field, operator, raw value, and field type to an ent
// sql.Selector predicate function. folded=true indicates the field has a
// sibling <field>_fold column — string predicates route to it with a
// unifold.Fold(value) RHS for diacritic-insensitive matching (UNICODE-01).
func buildPredicate(field, op, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	op = coerceToCaseInsensitive(op)
	switch op {
	case "": // exact match
		return buildExact(field, value, ft, folded)
	case "icontains":
		return buildContains(field, value, ft, folded)
	case "istartswith":
		return buildStartsWith(field, value, ft, folded)
	case "iexact":
		// iexact on string routes through fold branch; on non-string
		// falls back to buildExact's per-type handling.
		return buildExact(field, value, ft, folded)
	case "in":
		return buildIn(field, value, ft)
	case "lt":
		return buildComparison(field, value, ft, sql.FieldLT)
	case "gt":
		return buildComparison(field, value, ft, sql.FieldGT)
	case "lte":
		return buildComparison(field, value, ft, sql.FieldLTE)
	case "gte":
		return buildComparison(field, value, ft, sql.FieldGTE)
	default:
		return nil, fmt.Errorf("unsupported operator %q", op)
	}
}

// buildExact builds a predicate for exact match. String fields use
// case-insensitive matching per D-10. When folded=true, string matches go
// through the <field>_fold column with unifold.Fold(value) (UNICODE-01).
func buildExact(field, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	switch ft {
	case FieldString:
		if folded {
			return sql.FieldEqualFold(field+"_fold", unifold.Fold(value)), nil
		}
		return sql.FieldEqualFold(field, value), nil
	case FieldInt:
		v, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("convert %q to int: %w", value, err)
		}
		return sql.FieldEQ(field, v), nil
	case FieldBool:
		v, err := parseBool(value)
		if err != nil {
			return nil, fmt.Errorf("convert %q to bool: %w", value, err)
		}
		return sql.FieldEQ(field, v), nil
	case FieldTime:
		v, err := parseTime(value)
		if err != nil {
			return nil, fmt.Errorf("convert %q to time: %w", value, err)
		}
		return sql.FieldEQ(field, v), nil
	case FieldFloat:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("convert %q to float: %w", value, err)
		}
		return sql.FieldEQ(field, v), nil
	default:
		return nil, fmt.Errorf("unsupported field type %d for exact match", ft)
	}
}

// buildContains builds a case-insensitive contains predicate per D-10.
// folded=true routes to the <field>_fold column with unifold.Fold(value) for
// diacritic-insensitive matching (UNICODE-01).
func buildContains(field, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	if ft != FieldString {
		return nil, fmt.Errorf("contains operator not supported on non-string field %q", field)
	}
	if folded {
		return sql.FieldContainsFold(field+"_fold", unifold.Fold(value)), nil
	}
	return sql.FieldContainsFold(field, value), nil
}

// buildStartsWith builds a case-insensitive prefix match predicate per D-10.
// folded=true routes to the <field>_fold column with unifold.Fold(value) for
// diacritic-insensitive matching (UNICODE-01).
func buildStartsWith(field, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	if ft != FieldString {
		return nil, fmt.Errorf("startswith operator not supported on non-string field %q", field)
	}
	if folded {
		return sql.FieldHasPrefixFold(field+"_fold", unifold.Fold(value)), nil
	}
	return sql.FieldHasPrefixFold(field, value), nil
}

// buildIn builds an IN predicate using SQLite's json_each() table-valued
// function so the whole value list binds as a single JSON parameter rather
// than expanding to N `?` placeholders. This bypasses SQLite's
// SQLITE_MAX_VARIABLE_NUMBER limit regardless of the compiled default
// (modernc.org/sqlite v1.48.2 = 32766) and keeps the query plan stable
// at any list size (Phase 69 D-05, IN-01).
//
// An empty value (?asn__in=) returns errEmptyIn which ParseFilters
// translates to QueryOptions.EmptyResult=true (Phase 69 D-06, IN-02).
func buildIn(field, value string, ft FieldType) (func(*sql.Selector), error) {
	if value == "" {
		return nil, errEmptyIn
	}
	parts := strings.Split(value, ",")
	if len(parts) == 0 {
		// Defensive — strings.Split never returns []; "" is handled above.
		return nil, errEmptyIn
	}
	var jsonArr []byte
	var marshalErr error
	switch ft { //nolint:exhaustive // default case handles remaining types (Bool, Time, Float) with error
	case FieldString:
		trimmed := make([]string, len(parts))
		for i, p := range parts {
			trimmed[i] = strings.TrimSpace(p)
		}
		jsonArr, marshalErr = json.Marshal(trimmed)
	case FieldInt:
		ints := make([]int, 0, len(parts))
		for _, p := range parts {
			// Use parseErr here so a future refactor that introduces an
			// outer `err` can't silently shadow the loop error (W1 fix).
			v, parseErr := strconv.Atoi(strings.TrimSpace(p))
			if parseErr != nil {
				return nil, fmt.Errorf("convert %q to int for IN: %w", p, parseErr)
			}
			ints = append(ints, v)
		}
		jsonArr, marshalErr = json.Marshal(ints)
	default:
		return nil, fmt.Errorf("in operator not supported on field type %d for field %q", ft, field)
	}
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal IN array: %w", marshalErr)
	}
	jsonStr := string(jsonArr)
	return func(s *sql.Selector) {
		// s.C(field) quotes the column identifier via the ent builder —
		// the column name itself is already validated against tc.Fields
		// by ParseFilters, so no injection surface. The JSON payload
		// binds as a single parameter via ExprP (T-69-04-01 mitigation).
		s.Where(sql.ExprP(s.C(field)+" IN (SELECT value FROM json_each(?))", jsonStr))
	}, nil
}

// buildComparison builds a comparison predicate (lt, gt, lte, gte) with value
// type conversion.
func buildComparison(field, value string, ft FieldType, cmp func(string, any) func(*sql.Selector)) (func(*sql.Selector), error) {
	v, err := convertValue(value, ft)
	if err != nil {
		return nil, err
	}
	return cmp(field, v), nil
}

// convertValue converts a string value to the appropriate Go type based on
// FieldType.
func convertValue(s string, ft FieldType) (any, error) {
	switch ft {
	case FieldString:
		return s, nil
	case FieldInt:
		return strconv.Atoi(s)
	case FieldBool:
		return parseBool(s)
	case FieldTime:
		return parseTime(s)
	case FieldFloat:
		return strconv.ParseFloat(s, 64)
	default:
		return nil, fmt.Errorf("unsupported field type %d", ft)
	}
}

// parseBool converts PeeringDB-style boolean values. Accepts "1"/"0",
// "true"/"false".
func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "1", "true":
		return true, nil
	case "0", "false":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool value %q", s)
	}
}

// parseTime converts a Unix timestamp string to time.Time per D-15.
func parseTime(s string) (time.Time, error) {
	epoch, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid unix timestamp %q: %w", s, err)
	}
	return time.Unix(epoch, 0), nil
}
