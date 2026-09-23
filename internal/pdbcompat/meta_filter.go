package pdbcompat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// metaKind is the type of the column that upstream derives from a meta
// key. It selects the operators that apply and how values compare.
type metaKind int

const (
	metaString metaKind = iota // CharField: text, case-insensitive match
	metaDate                   // DateField: YYYY-MM-DD text
	metaBool                   // BooleanField(null=True): JSON true or false
)

// metaColumn is one filterable meta key. It mirrors an upstream
// GeneratedColumn (2.83.0 meta_registry.py:158-191): upstream stores the
// key in a typed, indexed column and filters on that column. The mirror
// has no such column. It reads the key from the stored meta document at
// query time.
type metaColumn struct {
	param    string // public filter key, meta__<path>
	column   string // upstream column name, also accepted as a key
	jsonPath string // SQLite JSON path of the key in the document
	kind     metaKind
}

// metaFilterColumns lists the filterable meta keys of each type, from
// the upstream registry (2.83.0 meta_registry.py:277-313). Only netixlan
// keys have columns. The net keys preferred_ip_mtu and rtbh_community
// have none, so upstream ignores net meta__ filters
// (docs/api/object_metadata.md:178-181), and the mirror does too.
var metaFilterColumns = map[string][]metaColumn{
	peeringdb.TypeNetIXLan: {
		{
			param:    "meta__planned_status_change__status",
			column:   "meta_planned_status_change_status",
			jsonPath: "$.planned_status_change.status",
			kind:     metaString,
		},
		{
			param:    "meta__planned_status_change__date",
			column:   "meta_planned_status_change_date",
			jsonPath: "$.planned_status_change.date",
			kind:     metaDate,
		},
		{
			param:    "meta__rfc8950",
			column:   "meta_rfc8950",
			jsonPath: "$.rfc8950",
			kind:     metaBool,
		},
	},
}

// metaComparisons maps the comparison operators to SQL.
var metaComparisons = map[string]string{"lt": "<", "lte": "<=", "gt": ">", "gte": ">="}

// lookupMetaFilter resolves a filter key to a filterable meta key of
// typeName. It mirrors NetworkIXLanSerializer.finalize_query_params
// (2.83.0 serializers.py:3129-3149), which rewrites the key before the
// filter loop runs (rest.py:559-563). The key is the meta__<path> name or
// the upstream column name, alone or followed by "__" and an operator.
// Upstream also accepts the column name, because the generated column is
// a model field (rest.py:525-528). suffix is the part of the key after
// the name: "" or "__<op>". ok=false means the key names no meta key.
func lookupMetaFilter(typeName, key string) (col metaColumn, suffix string, ok bool) {
	for _, c := range metaFilterColumns[typeName] {
		for _, name := range [...]string{c.param, c.column} {
			if key == name || strings.HasPrefix(key, name+"__") {
				return c, key[len(name):], true
			}
		}
	}
	return metaColumn{}, "", false
}

// metaOperators is the operator set of upstream's filter loop, the only
// suffixes that upstream applies to a meta column (2.83.0 rest.py:616).
// The case-insensitive names (iexact, icontains, istartswith) are not in
// it: upstream ignores a meta key with one of them.
var metaOperators = map[string]bool{
	"lt": true, "lte": true, "gt": true, "gte": true,
	"contains": true, "startswith": true, "in": true,
}

// buildMetaPredicate builds the predicate for a meta filter. suffix comes
// from lookupMetaFilter. The return shape matches buildLocalPredicate.
// ok=false means an operator that is not in metaOperators. Upstream's
// operator pattern then fails to match, the key names no field, and
// upstream ignores it (rest.py:616, :670), so the caller ignores it as
// unknown.
//
// A row without the key never matches, for any operator: json_extract
// and json_type return NULL for a missing path, and the upstream
// generated column is NULL for such a row.
func buildMetaPredicate(col metaColumn, suffix, value string) (func(*sql.Selector), bool, bool, error) {
	op, hasOp := strings.CutPrefix(suffix, "__")
	if hasOp && !metaOperators[op] {
		return nil, false, false, nil
	}
	op = coerceToCaseInsensitive(op)
	var (
		p   func(*sql.Selector)
		err error
	)
	switch col.kind {
	case metaString:
		p, err = metaStringPredicate(col, op, value)
	case metaDate:
		p, err = metaDatePredicate(col, op, value)
	case metaBool:
		p, err = metaBoolPredicate(col, op, value)
	default:
		err = fmt.Errorf("unsupported meta key type %d", col.kind)
	}
	if errors.Is(err, errEmptyIn) {
		return nil, true, false, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	return p, false, true, nil
}

// metaStringPredicate filters a text key. Exact match, contains and
// startswith ignore case, as upstream's iexact, icontains and
// istartswith (rest.py:659-662, :682-683).
func metaStringPredicate(col metaColumn, op, value string) (func(*sql.Selector), error) {
	lower := strings.ToLower(value)
	switch op {
	case "":
		return col.where("LOWER(%s) = ?", lower), nil
	case "icontains":
		return col.where("instr(LOWER(%s), ?) > 0", lower), nil
	case "istartswith":
		return col.where("instr(LOWER(%s), ?) = 1", lower), nil
	case "in":
		list, err := metaInList(value, func(v string) (string, error) { return strings.ToLower(v), nil })
		if err != nil {
			return nil, err
		}
		return col.where("LOWER(%s) IN (SELECT value FROM json_each(?))", list), nil
	case "lt", "lte", "gt", "gte":
		return col.where("%s "+metaComparisons[op]+" ?", value), nil
	}
	return nil, fmt.Errorf("unsupported operator %q", op)
}

// metaDatePredicate filters a date key. The document stores the date as
// YYYY-MM-DD text, which sorts in date order.
//
// Exact match is a prefix match on that text, as upstream's startswith
// on a DateField (rest.py:678-679), so 2026-10 matches every day in
// October 2026. A comparison uses the date of the value in UTC: upstream
// parses the value as a datetime (rest.py:642-655), and the DateField
// compares only its date. __in matches whole dates. Upstream fails on
// __in for a date field (rest.py:649-651, :665-666): see docs/API.md
// § Known Divergences.
func metaDatePredicate(col metaColumn, op, value string) (func(*sql.Selector), error) {
	switch op {
	case "":
		return col.where("instr(%s, ?) = 1", value), nil
	case "lt", "lte", "gt", "gte":
		d, err := metaDateValue(value)
		if err != nil {
			return nil, err
		}
		return col.where("%s "+metaComparisons[op]+" ?", d), nil
	case "in":
		list, err := metaInList(value, metaDateValue)
		if err != nil {
			return nil, err
		}
		return col.where("%s IN (SELECT value FROM json_each(?))", list), nil
	}
	return nil, fmt.Errorf("operator %q not supported on a date key", op)
}

// metaDateValue converts a filter value to a YYYY-MM-DD date in UTC.
func metaDateValue(value string) (string, error) {
	t, _, err := parseTimeValue(value)
	if err != nil {
		return "", err
	}
	return t.UTC().Format(time.DateOnly), nil
}

// metaBoolPredicate filters a boolean key. Exact match follows upstream
// (rest.py:680-681): "true" in any case, or "1", selects true, and any
// other value selects false. __in parses each value like the other
// boolean filters.
func metaBoolPredicate(col metaColumn, op, value string) (func(*sql.Selector), error) {
	switch op {
	case "":
		want := strings.ToLower(value) == "true" || value == "1"
		return col.where("%s = ?", strconv.FormatBool(want)), nil
	case "in":
		list, err := metaInList(value, func(v string) (string, error) {
			b, err := parseBool(v)
			return strconv.FormatBool(b), err
		})
		if err != nil {
			return nil, err
		}
		return col.where("%s IN (SELECT value FROM json_each(?))", list), nil
	}
	return nil, fmt.Errorf("operator %q not supported on a boolean key", op)
}

// metaInList converts a CSV __in value into a JSON array for json_each.
// conv converts each trimmed part. An empty value returns errEmptyIn, as
// buildIn does.
func metaInList(value string, conv func(string) (string, error)) (string, error) {
	if value == "" {
		return "", errEmptyIn
	}
	parts := strings.Split(value, ",")
	out := make([]string, len(parts))
	for i, part := range parts {
		v, err := conv(strings.TrimSpace(part))
		if err != nil {
			return "", err
		}
		out[i] = v
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("marshal IN array: %w", err)
	}
	return string(b), nil
}

// where returns a predicate that applies cond to the value of the key.
// cond holds one %s for the SQL expression that reads the value, and one
// ? for each arg. The expression uses only the constant JSON path from
// metaFilterColumns, never caller input.
//
// A boolean key is read with json_type, which returns "true" or "false"
// for a JSON boolean. json_extract would return 1 for both a JSON true
// and the number 1.
func (c metaColumn) where(cond string, args ...any) func(*sql.Selector) {
	fn := "json_extract"
	if c.kind == metaBool {
		fn = "json_type"
	}
	return func(s *sql.Selector) {
		// Only netixlan has filterable keys. Its column is meta.
		expr := fn + "(" + s.C(networkixlan.FieldMeta) + ", '" + c.jsonPath + "')"
		s.Where(sql.ExprP(fmt.Sprintf(cond, expr), args...))
	}
}
