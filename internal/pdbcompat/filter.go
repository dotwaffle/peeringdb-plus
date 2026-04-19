package pdbcompat

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
)

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

// ParseFilters translates Django-style query parameters into ent sql.Selector
// predicates. Reserved parameters (limit, skip, etc.) are skipped. Unknown
// fields are silently ignored per D-20. Returns an error only for known fields
// with unsupported operators.
func ParseFilters(params url.Values, fields map[string]FieldType) ([]func(*sql.Selector), error) {
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
		ft, ok := fields[field]
		if !ok {
			// Unknown field: silently ignore per D-20.
			continue
		}
		p, err := buildPredicate(field, op, vals[0], ft)
		if err != nil {
			return nil, fmt.Errorf("filter %s: %w", key, err)
		}
		predicates = append(predicates, p)
	}
	return predicates, nil
}

// buildPredicate maps a field, operator, raw value, and field type to an ent
// sql.Selector predicate function.
func buildPredicate(field, op, value string, ft FieldType) (func(*sql.Selector), error) {
	switch op {
	case "": // exact match
		return buildExact(field, value, ft)
	case "contains":
		return buildContains(field, value, ft)
	case "startswith":
		return buildStartsWith(field, value, ft)
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
// case-insensitive matching per D-10.
func buildExact(field, value string, ft FieldType) (func(*sql.Selector), error) {
	switch ft {
	case FieldString:
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
func buildContains(field, value string, ft FieldType) (func(*sql.Selector), error) {
	if ft != FieldString {
		return nil, fmt.Errorf("contains operator not supported on non-string field %q", field)
	}
	return sql.FieldContainsFold(field, value), nil
}

// buildStartsWith builds a case-insensitive prefix match predicate per D-10.
func buildStartsWith(field, value string, ft FieldType) (func(*sql.Selector), error) {
	if ft != FieldString {
		return nil, fmt.Errorf("startswith operator not supported on non-string field %q", field)
	}
	return sql.FieldHasPrefixFold(field, value), nil
}

// buildIn builds an IN predicate with proper type conversion per Pitfall 5.
func buildIn(field, value string, ft FieldType) (func(*sql.Selector), error) {
	parts := strings.Split(value, ",")
	switch ft { //nolint:exhaustive // default case handles remaining types (Bool, Time, Float) with error
	case FieldString:
		return sql.FieldIn(field, parts...), nil
	case FieldInt:
		ints := make([]int, 0, len(parts))
		for _, p := range parts {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				return nil, fmt.Errorf("convert %q to int for IN: %w", p, err)
			}
			ints = append(ints, v)
		}
		return sql.FieldIn(field, ints...), nil
	default:
		return nil, fmt.Errorf("in operator not supported on field type %d for field %q", ft, field)
	}
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
