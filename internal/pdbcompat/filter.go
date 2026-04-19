package pdbcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
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

// parseFieldOp splits a filter parameter key into relation segments, final
// field name, and operator. Syntax:
//
//	[<relation>__]* <field> [__<op>]
//
// Returns the full split (never truncates); the caller is responsible for
// enforcing the 2-hop cap per Phase 70 D-04. A len(relationSegments) > 2
// return value is a signal that the key is too deep to traverse — caller
// MUST treat as unknown per D-04/D-05.
//
// The operator suffix is detected by matching the LAST segment against a
// fixed set of known operators (isKnownOperator). Segments that look like
// field names with an embedded operator-like suffix (e.g. "info_prefixes4"
// ending in a number) still work because no "in"/"gt"/... alias collides.
//
// Empty field name or malformed input (leading "__", consecutive "__"
// producing empty segments) returns the best-effort split; callers
// validate finalField != "" and reject otherwise.
func parseFieldOp(key string) (relationSegments []string, finalField string, op string) {
	parts := strings.Split(key, "__")
	// If the last segment is a recognised operator AND there's at least
	// one preceding segment to act as the field name, strip it.
	if len(parts) >= 2 && isKnownOperator(parts[len(parts)-1]) {
		op = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 {
		return nil, "", op
	}
	finalField = parts[len(parts)-1]
	if len(parts) == 1 {
		return nil, finalField, op
	}
	relationSegments = parts[:len(parts)-1]
	return relationSegments, finalField, op
}

// isKnownOperator reports whether suffix matches an operator supported by
// buildPredicate. Used by parseFieldOp to disambiguate "<field>__<op>" from
// a field name whose underlying identifier itself contains a trailing
// segment after "__" (e.g. parent column names like "info_prefixes4").
//
// The list mirrors the operator switch in buildPredicate plus the case-
// insensitive variants produced by coerceToCaseInsensitive (Phase 69).
func isKnownOperator(suffix string) bool {
	switch suffix {
	case "contains", "icontains",
		"startswith", "istartswith",
		"iexact",
		"in",
		"lt", "gt", "lte", "gte":
		return true
	}
	return false
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

// unknownFieldsCtxKey is an unexported context key used by ParseFiltersCtx
// to record filter params whose fields don't resolve (per Phase 70 D-05).
// Retrieved via UnknownFieldsFromCtx at the handler layer for OTel span
// attribute + slog.DebugContext emission.
type unknownFieldsCtxKey struct{}

// WithUnknownFields returns a new context carrying an empty unknown-fields
// accumulator. Handler creates this before calling ParseFiltersCtx; the
// parser appends to the accumulator as it encounters unknown keys.
//
// Ctx threading (rather than a return slice) keeps ParseFilters' existing
// return signature stable and avoids churning every call site that passes
// params straight through to ent (Phase 70 D-05).
func WithUnknownFields(ctx context.Context) context.Context {
	return context.WithValue(ctx, unknownFieldsCtxKey{}, &[]string{})
}

// UnknownFieldsFromCtx returns the current accumulator (possibly nil when
// no accumulator was attached). Callers emit slog.DebugContext + OTel
// span attribute from the returned slice after ParseFiltersCtx returns.
func UnknownFieldsFromCtx(ctx context.Context) []string {
	v, _ := ctx.Value(unknownFieldsCtxKey{}).(*[]string)
	if v == nil {
		return nil
	}
	return *v
}

// appendUnknown records key on the ctx's accumulator when one is present.
// No-op when ctx has no accumulator (e.g. tests using ParseFilters shim).
func appendUnknown(ctx context.Context, key string) {
	v, _ := ctx.Value(unknownFieldsCtxKey{}).(*[]string)
	if v != nil {
		*v = append(*v, key)
	}
}

// ParseFilters translates Django-style query parameters into ent sql.Selector
// predicates. Reserved parameters (limit, skip, etc.) are skipped. Unknown
// fields are silently ignored per D-20 + Phase 70 D-05 / TRAVERSAL-04.
//
// Calls ParseFiltersCtx with context.Background — unknown fields are
// discarded rather than surfaced. Production handlers MUST use
// ParseFiltersCtx with a ctx from WithUnknownFields to emit diagnostics.
//
// Return values:
//   - preds: the predicate slice to pass to ent as Where arguments
//   - emptyResult: true when an __in filter was empty (?asn__in=); the caller
//     MUST short-circuit the whole request and emit an empty data array
//     without running SQL (Phase 69 D-06, IN-02)
//   - err: set only for known fields with invalid values / operators
func ParseFilters(params url.Values, tc TypeConfig) ([]func(*sql.Selector), bool, error) {
	return ParseFiltersCtx(context.Background(), params, tc)
}

// ParseFiltersCtx is the context-aware filter parser introduced by Phase 70
// D-05. Unknown filter fields (including over-cap traversal keys per D-04)
// are silently ignored for the HTTP response AND appended to the ctx-attached
// accumulator so operators can observe them via slog.DebugContext + OTel.
//
// Traversal resolution order (1-hop and 2-hop, len(relSegs) <= 2):
//  1. Path A: Allowlists[tc.Name].Direct or .Via exact match
//  2. Path B: LookupEdge + TargetFields introspection
//
// Keys with len(relSegs) > 2 are silently rejected per D-04.
//
// Phase 68 (status matrix) and Phase 69 (_fold routing, empty __in) invariants
// are preserved: traversal predicates wrap around buildPredicate which still
// consults FoldedFields on the target TypeConfig, and the empty-__in
// emptyResult sentinel bubbles back up from subquery construction.
func ParseFiltersCtx(ctx context.Context, params url.Values, tc TypeConfig) ([]func(*sql.Selector), bool, error) {
	var predicates []func(*sql.Selector)
	for key, vals := range params {
		if len(vals) == 0 {
			continue
		}
		// Skip reserved pagination/control parameters.
		if reservedParams[key] {
			continue
		}
		relSegs, field, op := parseFieldOp(key)
		// Also check if the raw final field is a reserved name
		// (e.g. "fields" on a top-level single-segment key).
		if len(relSegs) == 0 && reservedParams[field] {
			continue
		}
		// D-04 hard cap: >2 relation segments is silently rejected.
		if len(relSegs) > 2 {
			appendUnknown(ctx, key)
			continue
		}
		// Malformed split (empty final field, empty leading segment)
		// falls through to unknown-field handling.
		if field == "" {
			appendUnknown(ctx, key)
			continue
		}

		if len(relSegs) == 0 {
			// Direct local field path — pre-Phase-70 behaviour.
			p, emptyResult, ok, err := buildLocalPredicate(field, op, vals[0], tc)
			if err != nil {
				return nil, false, fmt.Errorf("filter %s: %w", key, err)
			}
			if emptyResult {
				return nil, true, nil
			}
			if !ok {
				appendUnknown(ctx, key)
				continue
			}
			predicates = append(predicates, p)
			continue
		}

		// Traversal path (1-hop or 2-hop).
		p, ok, emptyResult, err := buildTraversalPredicate(tc, relSegs, field, op, vals[0])
		if err != nil {
			return nil, false, fmt.Errorf("filter %s: %w", key, err)
		}
		if emptyResult {
			return nil, true, nil
		}
		if !ok {
			appendUnknown(ctx, key)
			continue
		}
		predicates = append(predicates, p)
	}
	return predicates, false, nil
}

// buildLocalPredicate extracts the pre-Phase-70 local-field behaviour into a
// helper returning a uniform (predicate, emptyResult, ok, err) shape so
// ParseFiltersCtx can treat local and traversal paths symmetrically.
//
// ok=false => the field is unknown on tc; caller silently ignores.
// emptyResult=true => empty __in sentinel; caller short-circuits.
func buildLocalPredicate(field, op, value string, tc TypeConfig) (func(*sql.Selector), bool, bool, error) {
	ft, exists := tc.Fields[field]
	if !exists {
		return nil, false, false, nil
	}
	folded := tc.FoldedFields[field]
	p, err := buildPredicate(field, op, value, ft, folded)
	if err != nil {
		if errors.Is(err, errEmptyIn) {
			return nil, true, false, nil
		}
		return nil, false, false, err
	}
	return p, false, true, nil
}

// buildTraversalPredicate resolves a 1-hop or 2-hop cross-entity filter.
// relSegs has len 1 or 2 (caller enforced D-04 cap). Resolution order is
// Path A (Allowlists) first, then Path B (LookupEdge introspection).
//
// Return shape mirrors buildLocalPredicate:
//   - predicate, true, false, nil on a resolved key
//   - nil, false, false, nil on an unknown key (silent ignore)
//   - nil, false, true, nil on an empty __in sentinel bubbling up from the
//     target-side buildPredicate
//   - nil, false, false, err on conversion errors (int, bool, etc.)
func buildTraversalPredicate(tc TypeConfig, relSegs []string, field, op, value string) (func(*sql.Selector), bool, bool, error) {
	// Reconstruct the allowlist key (without operator suffix): matches the
	// shape emitted by cmd/pdb-compat-allowlist for Allowlists entries.
	fullKey := strings.Join(relSegs, "__") + "__" + field

	// Path A: consult Allowlists[tc.Name] first. A hit here commits to
	// the Path A construction UNLESS the downstream buildSinglHop /
	// buildTwoHop reports ok=false with no emptyResult/err (e.g. the
	// edge or target Registry entry is missing). In that soft-miss case
	// we fall through to Path B rather than short-circuit the key to
	// silent-ignore — otherwise an allowlist entry whose first segment
	// happens to also be a valid Path B TraversalKey on a different
	// schema could suppress a resolution that would have worked
	// (Phase 70 REVIEW WR-03). Hard errors (emptyResult, conversion
	// err) still propagate immediately.
	entry, hasAllowlist := Allowlists[tc.Name]
	if hasAllowlist {
		if len(relSegs) == 1 {
			if slices.Contains(entry.Direct, fullKey) {
				p, ok, empty, err := buildSinglHop(tc.Name, relSegs[0], field, op, value)
				if err != nil || empty || ok {
					return p, ok, empty, err
				}
				// ok=false, no err, no empty — soft miss, fall through.
			}
		} else {
			// 2-hop: Via[<first-hop>] contains "<second-hop>__<field>".
			if tails, okVia := entry.Via[relSegs[0]]; okVia {
				tailKey := relSegs[1] + "__" + field
				if slices.Contains(tails, tailKey) {
					p, ok, empty, err := buildTwoHop(tc.Name, relSegs[0], relSegs[1], field, op, value)
					if err != nil || empty || ok {
						return p, ok, empty, err
					}
					// Soft miss — fall through to Path B.
				}
			}
		}
	}

	// Path B: introspection via LookupEdge + TargetFields.
	edge, okEdge := LookupEdge(tc.Name, relSegs[0])
	if !okEdge {
		return nil, false, false, nil
	}
	if len(relSegs) == 1 {
		if _, hasField := TargetFields(edge.TargetType)[field]; !hasField {
			return nil, false, false, nil
		}
		return buildSinglHop(tc.Name, relSegs[0], field, op, value)
	}
	// 2-hop Path B: second hop edge must exist on the intermediate target.
	edge2, okEdge2 := LookupEdge(edge.TargetType, relSegs[1])
	if !okEdge2 {
		return nil, false, false, nil
	}
	if _, hasField := TargetFields(edge2.TargetType)[field]; !hasField {
		return nil, false, false, nil
	}
	return buildTwoHop(tc.Name, relSegs[0], relSegs[1], field, op, value)
}

// buildSinglHop emits a 1-hop traversal predicate. The SQL shape depends
// on which side of the join owns the foreign-key column (edge.OwnFK):
//
// M2O (OwnFK == true — FK on parent, e.g. networks.org_id → organizations.id):
//
//	parent.<ParentFKColumn> IN (
//	    SELECT target.<TargetIDColumn> FROM <TargetTable> WHERE <inner>
//	)
//
// O2M (OwnFK == false — FK on child, e.g. pocs.net_id → networks.id):
//
//	parent.id IN (
//	    SELECT child.<ParentFKColumn> FROM <TargetTable> WHERE <inner>
//	)
//
// All SQL identifiers come from the codegen-emitted EdgeMetadata (Plan 70-04)
// — never from user input. The inner predicate is produced by the existing
// buildPredicate path on the target TypeConfig, so Phase 69 _fold routing
// and Phase 69 empty-__in sentinels apply at the target entity.
//
// Parent-side PK is always "id" for every PeeringDB entity — an invariant
// baked into the schema generator; no entity overrides its ID column.
func buildSinglHop(entityType, fk, field, op, value string) (func(*sql.Selector), bool, bool, error) {
	edge, ok := LookupEdge(entityType, fk)
	if !ok {
		return nil, false, false, nil
	}
	targetTC, hasTarget := Registry[edge.TargetType]
	if !hasTarget {
		return nil, false, false, nil
	}
	ft, hasField := targetTC.Fields[field]
	if !hasField {
		return nil, false, false, nil
	}
	folded := targetTC.FoldedFields[field]
	innerPred, err := buildPredicate(field, op, value, ft, folded)
	if err != nil {
		if errors.Is(err, errEmptyIn) {
			return nil, false, true, nil
		}
		return nil, false, false, err
	}
	fkCol := edge.ParentFKColumn
	targetTable := edge.TargetTable
	targetID := edge.TargetIDColumn
	ownFK := edge.OwnFK
	return func(s *sql.Selector) {
		t := sql.Table(targetTable)
		if ownFK {
			// M2O: FK on parent. Select target.<id>, filter parent.<fk>.
			innerSel := sql.Select(t.C(targetID)).From(t)
			innerPred(innerSel)
			s.Where(sql.In(s.C(fkCol), innerSel))
			return
		}
		// O2M: FK on child. Select child.<fk>, filter parent.<id>.
		innerSel := sql.Select(t.C(fkCol)).From(t)
		innerPred(innerSel)
		s.Where(sql.In(s.C(parentPKColumn), innerSel))
	}, true, false, nil
}

// parentPKColumn is the primary-key column name for every PeeringDB
// entity in this schema. Hard-coded "id" rather than plumbed through
// Registry because the schema generator guarantees this invariant and
// inlining avoids an extra lookup on the request-time hot path.
const parentPKColumn = "id"

// buildTwoHop emits a 2-hop traversal predicate with two nested subqueries.
// Each hop independently branches on its edge's OwnFK flag (M2O vs O2M),
// producing one of four possible shapes:
//
// hop1 M2O, hop2 M2O (e.g. ixpfx → ixlan → ix):
//
//	parent.<fk1> IN (
//	    SELECT mid.<mid_id> FROM <mid> WHERE mid.<fk2> IN (
//	        SELECT leaf.<leaf_id> FROM <leaf> WHERE <inner>
//	    )
//	)
//
// hop1 O2M, hop2 M2O (e.g. org → networks → poc):
//
//	parent.id IN (
//	    SELECT mid.<fk1> FROM <mid> WHERE mid.<fk2> IN (
//	        SELECT leaf.<leaf_id> FROM <leaf> WHERE <inner>
//	    )
//	)
//
// hop1 M2O, hop2 O2M (e.g. netfac → network → pocs):
//
//	parent.<fk1> IN (
//	    SELECT mid.<mid_id> FROM <mid> WHERE mid.id IN (
//	        SELECT leaf.<fk2> FROM <leaf> WHERE <inner>
//	    )
//	)
//
// hop1 O2M, hop2 O2M:
//
//	parent.id IN (
//	    SELECT mid.<fk1> FROM <mid> WHERE mid.id IN (
//	        SELECT leaf.<fk2> FROM <leaf> WHERE <inner>
//	    )
//	)
//
// Hard-capped at 2 hops per D-04. Identifiers from two EdgeMetadata lookups;
// values bind via the innermost buildPredicate. Parent PK is always "id"
// (see parentPKColumn — schema generator invariant).
func buildTwoHop(entityType, fk1, fk2, field, op, value string) (func(*sql.Selector), bool, bool, error) {
	edge1, ok := LookupEdge(entityType, fk1)
	if !ok {
		return nil, false, false, nil
	}
	edge2, ok := LookupEdge(edge1.TargetType, fk2)
	if !ok {
		return nil, false, false, nil
	}
	leafTC, hasLeaf := Registry[edge2.TargetType]
	if !hasLeaf {
		return nil, false, false, nil
	}
	ft, hasField := leafTC.Fields[field]
	if !hasField {
		return nil, false, false, nil
	}
	folded := leafTC.FoldedFields[field]
	innerPred, err := buildPredicate(field, op, value, ft, folded)
	if err != nil {
		if errors.Is(err, errEmptyIn) {
			return nil, false, true, nil
		}
		return nil, false, false, err
	}
	fk1Col := edge1.ParentFKColumn
	midTable := edge1.TargetTable
	midIDCol := edge1.TargetIDColumn
	ownFK1 := edge1.OwnFK
	fk2Col := edge2.ParentFKColumn
	leafTable := edge2.TargetTable
	leafIDCol := edge2.TargetIDColumn
	ownFK2 := edge2.OwnFK
	return func(s *sql.Selector) {
		leafT := sql.Table(leafTable)
		// Inner (leaf) subquery: column depends on hop-2 direction.
		//   M2O: SELECT leaf.<leaf_id>  (filter mid's FK column against it)
		//   O2M: SELECT leaf.<fk2>      (filter mid.id against it)
		var leafSel *sql.Selector
		if ownFK2 {
			leafSel = sql.Select(leafT.C(leafIDCol)).From(leafT)
		} else {
			leafSel = sql.Select(leafT.C(fk2Col)).From(leafT)
		}
		innerPred(leafSel)

		// Middle subquery: which mid column joins to the leaf subquery
		// is chosen by hop-2 direction; which mid column is SELECTed
		// (to feed the outer filter) is chosen by hop-1 direction.
		midT := sql.Table(midTable)
		var midJoin func(*sql.Selector)
		if ownFK2 {
			midJoin = func(ms *sql.Selector) { ms.Where(sql.In(ms.C(fk2Col), leafSel)) }
		} else {
			midJoin = func(ms *sql.Selector) { ms.Where(sql.In(ms.C(parentPKColumn), leafSel)) }
		}
		var midSel *sql.Selector
		if ownFK1 {
			midSel = sql.Select(midT.C(midIDCol)).From(midT)
		} else {
			midSel = sql.Select(midT.C(fk1Col)).From(midT)
		}
		midJoin(midSel)

		// Outer filter: parent's FK column (M2O) or parent's PK (O2M).
		if ownFK1 {
			s.Where(sql.In(s.C(fk1Col), midSel))
		} else {
			s.Where(sql.In(s.C(parentPKColumn), midSel))
		}
	}, true, false, nil
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
