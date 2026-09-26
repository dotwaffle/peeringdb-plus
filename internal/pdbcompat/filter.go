package pdbcompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/unifold"
)

// errEmptyIn is a sentinel returned by buildIn when an __in filter has no
// values (e.g. ?asn__in=). ParseFilters catches it and short-circuits the
// whole request via QueryOptions.EmptyResult.
var errEmptyIn = errors.New("empty __in")

// parseFieldOp splits a filter parameter key into relation segments, final
// field name, and operator. Syntax:
//
//	[<relation>__]* <field> [__<op>]
//
// Returns the full split (never truncates); the caller is responsible for
// enforcing the 2-hop cap. A len(relationSegments) > 2
// return value is a signal that the key is too deep to traverse — caller
// MUST treat as unknown.
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
// insensitive variants produced by coerceToCaseInsensitive.
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

// applyStatusMatrix returns the upstream status predicate for list
// requests (2.83.0 rest.py:719-750). live is the type's live status set
// (pdbtypes.LiveStatuses). sinceSet=false admits only the live statuses
// (rest.py:748). sinceSet=true admits live + deleted, plus pending when
// isCampus (rest.py:723-735). Always returns a non-nil predicate, because
// every list request needs a status filter.
//
// A single live status emits status = ?. SQLite then reads the
// single-column status index, which returns the rows in (status, rowid)
// order, so the default id order needs no sort. A set of two or more
// statuses emits likelyStatusIn instead of a plain IN.
func applyStatusMatrix(live []string, isCampus, sinceSet bool) func(*sql.Selector) {
	if !sinceSet {
		if len(live) == 1 {
			return sql.FieldEQ("status", live[0])
		}
		return likelyStatusIn(live)
	}
	allowed := append(slices.Clone(live), "deleted")
	if isCampus {
		allowed = append(allowed, "pending")
	}
	return likelyStatusIn(allowed)
}

// likelyStatusIn returns `likely(status IN (...))`. The status set of
// the matrix admits almost every row, but without ANALYZE statistics
// the SQLite planner treats a plain IN on an indexed column as
// selective. It then reads the rows through a status-leading index, one
// range per status, and sorts them in a temp B-tree. The likely() hint
// tells the planner that the IN is not selective, so it reads the rows
// in the list order instead: the rowid table for id order, and the
// updated index for the ?since= order. COUNT queries keep the covering
// status index. Measured on 69,700 netixlan rows: a 250-row page at
// skip=30000 went from 119 ms to 9 ms, and a 250-row ?since=1 page from
// 26 ms to 2 ms.
func likelyStatusIn(statuses []string) func(*sql.Selector) {
	args := make([]any, len(statuses))
	for i, v := range statuses {
		args[i] = v
	}
	return func(s *sql.Selector) {
		s.Where(sql.P(func(b *sql.Builder) {
			b.WriteString("likely(").Join(sql.In(s.C("status"), args...)).WriteString(")")
		}))
	}
}

// likelyOK is likely(status IN ('ok')). It is the filter of a depth set
// whose child type has the one live status "ok", of the childSets count
// for that set, and of a relation-key status pin. Each of these
// queries selects the rows of one parent through an FK column. Without
// ANALYZE statistics, SQLite scores status = ? as selective as the FK
// equality. For pocs and ixlans it then reads every "ok" row through
// the status index. On a database with 40k pocs, /api/net/<id> at depth
// 2 took 20 ms instead of 1 ms, and /api/net?ixlan=<id> took 23 ms
// instead of 0.2 ms. The likely() hint keeps these plans on the FK
// index.
var likelyOK = likelyStatusIn([]string{"ok"})

// likelyNetIXLanSet is likely(status IN ('ok', 'not-operational')), the
// netixlan live statuses. It is the filter of the childSets counts of the
// two sets of netixlan rows (net.netixlan_set and the join rows of
// ixlan.net_set). These counts select the rows of many parents with an
// FK IN json_each list. With a plain status IN, SQLite reads the netixlan
// status index, and then sorts the rows in a temp B-tree for the GROUP
// BY. The likely() hint keeps the plan on the FK index, which also gives
// the group order.
var likelyNetIXLanSet = likelyStatusIn([]string{"ok", "not-operational"})

// coerceToCaseInsensitive maps the subset of operators that upstream
// (2.83.0 rest.py:657-662) forces to case-insensitive variants. Non-matching operators
// pass through unchanged (scope: contains + startswith only).
//
// The coercion is purely nominal — the existing
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

// coerceLocationFilterOp mirrors upstream 2.83.0 rest.py:583-595, which
// special-cases bare location filters before generic handling:
// `address1`, `city`, and `state` become `<field>__icontains`
// (substring match — ?city=Frankfurt also matches "Frankfurt am
// Main"), and `country` becomes `__iexact` for 2-char values /
// `__icontains` for longer ones. Only BARE local filters coerce:
// upstream's special-case list matches the raw param name, so an
// explicit operator suffix or a relation prefix (fac__city=…) takes
// the generic path there too. The 2-char country case needs no
// rewrite because buildExact's string branch is already
// case-insensitive (FieldEqualFold == iexact).
func coerceLocationFilterOp(field, op, value string) string {
	if op != "" {
		return op
	}
	switch field {
	case "address1", "city", "state":
		return "icontains"
	case "country":
		if len(value) != 2 {
			return "icontains"
		}
	}
	return op
}

// coerceIPAddr ports upstream coerce_ipaddr (2.83.0 util.py:61-73). It
// returns the canonical text of an IP address, or the value as given
// when it is not one. netip.Addr.String gives the same text as CPython
// 3.13+ str(ipaddress.ip_address(v)): compressed, lower case, and an
// IPv4-mapped address in dotted form. CPython rejects a zone that
// contains "%" and Go accepts it, so such a value stays as given.
func coerceIPAddr(v string) string {
	a, err := netip.ParseAddr(v)
	if err != nil || strings.Contains(a.Zone(), "%") {
		return v
	}
	return a.String()
}

// ipaddr6Predicate filters netixlan ipaddr6 for the bare key. Upstream
// runs unidecode on every value before coerce_ipaddr (2.83.0
// rest.py:597, :605-606), so the value is folded first; Fold also
// lower-cases it. Sync stores the address text of the upstream API,
// which is the CPython canonical form and always lower case (django-inet
// get_prep_value, DRF CharField.to_representation). So a plain = gives
// the same match as upstream's __iexact, and SQLite can read the
// networkixlan_ipaddr6 index. A value that is not an address matches no
// stored row either way.
func ipaddr6Predicate(value string) func(*sql.Selector) {
	return sql.FieldEQ("ipaddr6", coerceIPAddr(unifold.Fold(value)))
}

// unknownFieldsCtxKey is an unexported context key used by ParseFiltersCtx
// to record filter params whose fields don't resolve.
// Retrieved via UnknownFieldsFromCtx at the handler layer for OTel span
// attribute + slog.DebugContext emission.
type unknownFieldsCtxKey struct{}

// WithUnknownFields returns a new context carrying an empty unknown-fields
// accumulator. Handler creates this before calling ParseFiltersCtx; the
// parser appends to the accumulator as it encounters unknown keys.
//
// Ctx threading (rather than a return slice) keeps ParseFilters' existing
// return signature stable and avoids churning every call site that passes
// params straight through to ent.
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
// fields are silently ignored.
//
// Calls ParseFiltersCtx with context.Background — unknown fields are
// discarded rather than surfaced. Production handlers MUST use
// ParseFiltersCtx with a ctx from WithUnknownFields to emit diagnostics.
//
// Return values:
//   - preds: the predicate slice to pass to ent as Where arguments
//   - emptyResult: true when an __in filter was empty (?asn__in=); the caller
//     MUST short-circuit the whole request and emit an empty data array
//     without running SQL
//   - err: set only for known fields with invalid values / operators
func ParseFilters(params url.Values, tc TypeConfig) ([]func(*sql.Selector), bool, error) {
	return ParseFiltersCtx(context.Background(), params, tc)
}

// ParseFiltersCtx is the context-aware filter parser.
// Unknown filter fields (including over-cap traversal keys)
// are silently ignored for the HTTP response AND appended to the ctx-attached
// accumulator so operators can observe them via slog.DebugContext + OTel.
//
// A key without relation segments filters a local column. A key that
// names a forward FK in upstream spelling (org, network_id, facility)
// filters the FK column (resolveLocalField). A key that upstream never
// filters (TypeConfig.UpstreamIgnored) is unknown, and so is a relation
// key whose field is a FK column (namesFKColumn). A relation key filters
// status only through a forward edge one hop away
// (relationStatusFilterable).
//
// Traversal resolution order (1-hop and 2-hop, len(relSegs) <= 2):
//  1. Path A: Allowlists[tc.Name].Direct or .Via exact match
//  2. Path B: LookupEdge + TargetFields introspection
//
// An upstream FK name as the first relation segment (network__asn,
// facility__name) resolves as the mirror traversal key of the same edge
// (traversalKeyFor).
//
// Keys with len(relSegs) > 2 are silently rejected.
//
// parseListFilters reads the keys in the upstream order: the fac and
// org distance key (parseDistanceSearch), the prepare_query keys,
// name_search (resolveNameSearch), then the other keys.
//
// For each key (filterState.addKey), the filterable meta keys of the type (netixlan
// meta__<path> and the upstream meta_* column names, see
// lookupMetaFilter) resolve first,
// before the key is split for traversal. The presence keys of an
// upstream prepare_query (presenceKeys, for example net?not_ix= and
// fac?all_net=) resolve next, then the ix ipblock key
// (lookupIPBlockKey, a text prefix match on the ixpfx prefix), then the
// ixpfx whereis key (lookupWhereisKey, the prefixes that contain an
// address), then the ix capacity key (lookupCapacityFilter, the sum of
// the netixlan speeds), then the relation keys (relationSeeds, for
// example fac?net= and net?ix__name=), with their own path and status
// rules (buildPresencePredicate, buildRelationSeedPredicate).
// The bare netixlan ipaddr6 key resolves next and compares the
// canonical text of the address (ipaddr6Predicate).
//
// The status matrix and the _fold-routing / empty-__in invariants
// are preserved: traversal predicates wrap around buildPredicate which still
// consults FoldedFields on the target TypeConfig, and the empty-__in
// emptyResult sentinel bubbles back up from subquery construction.
//
// An empty result (an empty __in) is returned after every key is
// parsed, so an error of any key wins.
//
// ParseFiltersCtx wraps parseListFilters and drops the sort key of a
// distance search: see parseListFilters for the pre-pass.
func ParseFiltersCtx(ctx context.Context, params url.Values, tc TypeConfig) ([]func(*sql.Selector), bool, error) {
	lf, err := parseListFilters(ctx, params, tc, nil)
	if err != nil {
		return nil, false, err
	}
	return lf.preds, lf.emptyResult, nil
}

// listFilters is the parsed filter set of a list or detail request.
type listFilters struct {
	// preds are the filter predicates. nil when emptyResult is set.
	preds []func(*sql.Selector)
	// emptyResult is set when a filter matches no row without a query
	// (an empty __in). The request returns an empty result.
	emptyResult bool
	// orderBy is the primary sort key that a filter asks for: the
	// distance of a fac or org distance search. nil otherwise. A detail
	// request does not use it.
	orderBy func(*sql.Selector)
	// none is set with emptyResult when name_search matches no row
	// without a query (upstream qset.none(), 2.83.0 rest.py:550-553).
	// Upstream returns it before the slice, so it wins over the slice
	// 404 of a detail request and over a negative skip
	// (nameSearchMisses).
	none bool
	// searchHit is set when name_search runs a search: it keeps the
	// ok rows that the search matches. Whether the search has a hit is
	// known only when a query runs. nameSearchMisses runs it for a
	// detail request with a limit or skip that is not 0 and for a list
	// with a negative skip, as a search with no hit is upstream
	// qset.none() before the slice.
	searchHit func(*sql.Selector)
	// upstreamFilter is set when the request has a key that upstream
	// counts in the gate of its API cache (2.83.0 rest.py:705-709,
	// api_cache.py:109-110): a key that adds a predicate or an empty
	// result, a legacy net info_type key (query_adjusted,
	// serializers.py:3775-3810), and a relation key of a prepare_query
	// with at most 3 segments, also when the mirror ignores its form
	// (get_relation_filters puts it in p_filters, :614-654). It is set
	// even when the key matches every row. A list at depth > 0 with
	// such a key is cut to 250 rows, as upstream (rest.py:766-772).
	upstreamFilter bool
}

// filterState collects the predicates of parseListFilters while it
// reads the keys of a request.
type filterState struct {
	ctx      context.Context
	tc       TypeConfig
	tier     privctx.Tier
	spatial  bool
	consumed map[string]bool
	preds    []func(*sql.Selector)
	// empty records an empty __in.
	empty bool
	// adjusted records a key that upstream counts in its API cache gate
	// but that adds no predicate (see listFilters.upstreamFilter).
	adjusted bool
}

// parseListFilters parses the filter keys of a request (see
// ParseFiltersCtx for the key rules), in the order of upstream
// get_queryset (2.83.0 rest.py:477-703):
//
//  1. the prepare_query keys (:486-500): the fac and org distance
//     search (parseDistanceSearch) and every key of isPrepareQueryKey;
//  2. afterPrepare, when it is not nil: the caller parses since, skip,
//     limit and depth there (:505-523), so their errors lose to a
//     prepare_query error and win over the others;
//  3. name_search (:531-553, resolveNameSearch);
//  4. the other keys, the filter loop (:564-703). When name_search can
//     match no row, upstream returns before the loop, so these keys are
//     not read.
//
// The distance pre-pass runs first, because a distance search changes
// how the other keys are read: the loop skips the location keys of
// spatialSkipKeys and matches a bare country exactly (2.83.0
// rest.py:569-597). A distance error wins over an error in another
// prepare_query key, although upstream fac checks its presence keys
// first (serializers.py:2126-2208); the status is 400 on both sides,
// only the message differs. Each pass reads the keys in sorted order,
// so a request with two bad keys always gets the same message.
//
// An error of afterPrepare is returned as is. The one empty-result exit
// is after the loop.
func parseListFilters(ctx context.Context, params url.Values, tc TypeConfig, afterPrepare func() error) (listFilters, error) {
	st := &filterState{
		ctx:      ctx,
		tc:       tc,
		tier:     privctx.TierFrom(ctx),
		consumed: map[string]bool{},
	}
	var orderBy func(*sql.Selector)
	ds, err := parseDistanceSearch(tc.Name, params)
	if err != nil {
		return listFilters{}, fmt.Errorf("filter %w", err)
	}
	if distanceTypes[tc.Name] {
		// A known key also when it is a no-op (a value of 0 or less).
		st.consumed["distance"] = true
	}
	st.spatial = ds != nil
	if st.spatial {
		st.preds = append(st.preds, ds.predicate())
		orderBy = ds.order()
		for k := range spatialSkipKeys {
			st.consumed[k] = true
		}
	}
	keys := slices.Sorted(maps.Keys(params))
	for _, key := range keys {
		if isPrepareQueryKey(tc, key) {
			if err := st.addKey(key, params[key]); err != nil {
				return listFilters{}, err
			}
		}
	}
	if afterPrepare != nil {
		if err := afterPrepare(); err != nil {
			return listFilters{}, err
		}
	}
	ns, err := resolveNameSearch(tc, params)
	if err != nil {
		return listFilters{}, err
	}
	maps.Copy(st.consumed, ns.consumed)
	if ns.pred != nil {
		st.preds = append(st.preds, ns.pred)
	}
	// Upstream returns qset.none() for a name_search that matches
	// nothing before finalize_query_params and the filter loop
	// (rest.py:550-553). The other keys are not read, so they are not
	// unknown either.
	if !ns.none {
		for _, key := range keys {
			if !isPrepareQueryKey(tc, key) {
				if err := st.addKey(key, params[key]); err != nil {
					return listFilters{}, err
				}
			}
		}
	}
	if st.empty || ns.none {
		return listFilters{emptyResult: true, none: ns.none, searchHit: ns.hit, upstreamFilter: true}, nil
	}
	return listFilters{
		preds:          st.preds,
		orderBy:        orderBy,
		searchHit:      ns.hit,
		upstreamFilter: st.adjusted || len(st.preds) > 0,
	}, nil
}

// addKey parses one filter key. vals holds every value of the key.
func (st *filterState) addKey(key string, vals []string) error {
	ctx, tc, tier := st.ctx, st.tc, st.tier
	if len(vals) == 0 {
		return nil
	}
	// Repeated params (?foo=a&foo=b) take the LAST value, matching
	// Django's QueryDict.__getitem__ (upstream PeeringDB's request
	// layer). url.Values preserves insertion order, so vals[len-1] is
	// the last value seen on the wire.
	value := vals[len(vals)-1]
	// Skip reserved pagination/control parameters and the keys that
	// a pre-pass handled.
	if reservedParams[key] || st.consumed[key] {
		return nil
	}
	// Meta keys resolve before the key is split, as upstream
	// rewrites them before its filter loop (2.83.0
	// serializers.py:3129-3149). They are not traversals: a split
	// would read meta__planned_status_change__date__lt as a 2-hop
	// path and ignore it.
	if col, suffix, isMeta := lookupMetaFilter(tc.Name, key); isMeta {
		p, empty, ok, err := buildMetaPredicate(col, suffix, value)
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		if empty {
			st.empty = true
			return nil
		}
		if !ok {
			appendUnknown(ctx, key)
			return nil
		}
		st.preds = append(st.preds, p)
		return nil
	}
	// The legacy net info_type keys, and info_types with __in or
	// __startswith, resolve before the key is split, as upstream
	// rewrites them before its filter loop (2.83.0
	// serializers.py:3768-3813, rest.py:559-563).
	if patterns, ok := legacyInfoTypePatterns(tc.Name, key, value); ok {
		if patterns == nil {
			// A pattern matches every network. Upstream still sets
			// query_adjusted, so the key counts as a filter.
			st.adjusted = true
			return nil
		}
		p, err := multiChoiceLikeAny("info_types", patterns)
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		st.preds = append(st.preds, p)
		return nil
	}
	// The presence keys of an upstream prepare_query (not_ix,
	// all_net, org_present and the others) use the first value of a
	// repeated key, as prepare_query reads kwargs.get(key)[0].
	if pk, isPresence := lookupPresenceKey(tc.Name, key); isPresence {
		p, err := buildPresencePredicate(tc, pk, vals[0], tier)
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		st.preds = append(st.preds, p)
		return nil
	}
	// The ix ipblock key of an upstream prepare_query uses the first
	// value of a repeated key, as prepare_query reads
	// kwargs.get(key)[0] (2.83.0 serializers.py:4548-4552). No value
	// is an error, and an empty value is not an empty result.
	if lookupIPBlockKey(tc.Name, key) {
		st.preds = append(st.preds, buildIPBlockPredicate(vals[0]))
		return nil
	}
	// The ixpfx whereis key of an upstream prepare_query and its
	// operator forms use the first value of a repeated key
	// (2.83.0 serializers.py:618-619). A value that is not an
	// address, and the __in form, are an error.
	if inList, isWhereis := lookupWhereisKey(tc.Name, key); isWhereis {
		p, err := buildWhereisPredicate(vals[0], inList)
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		st.preds = append(st.preds, p)
		return nil
	}
	// The ix capacity key of an upstream prepare_query and its
	// operator forms use the first value of a repeated key (2.83.0
	// serializers.py:618-619). A value that is not an integer is an
	// error, also as an item of __in.
	if op, isCapacity := lookupCapacityFilter(tc.Name, key); isCapacity {
		p, err := buildCapacityPredicate(op, vals[0])
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		st.preds = append(st.preds, p)
		return nil
	}
	// The relation keys of an upstream prepare_query resolve before
	// the other keys, as upstream handles them apart from its
	// model-field filters. They use the first value of a repeated
	// key, as get_relation_filters does (2.83.0
	// serializers.py:618-619).
	if sd, tail, isSeed := lookupRelationSeed(tc.Name, key); isSeed {
		p, ok, empty, err := buildRelationSeedPredicate(tc, sd, tail, vals[0], tier)
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		if empty {
			st.empty = true
			return nil
		}
		if !ok {
			// Upstream puts a key of up to 3 segments in p_filters,
			// also when prepare_query does not apply it (2.83.0
			// serializers.py:641-654).
			if len(tail) <= 2 {
				st.adjusted = true
			}
			appendUnknown(ctx, key)
			return nil
		}
		st.preds = append(st.preds, p)
		return nil
	}
	// A bare ipaddr6 key on netixlan compares the canonical text of
	// the address, as upstream does (2.83.0 rest.py:605-606,
	// util.py:61-73). Only the exact key: the value of ipaddr6__in,
	// of the other suffixes and of ipaddr4 is not canonicalized.
	if tc.Name == peeringdb.TypeNetIXLan && key == "ipaddr6" {
		st.preds = append(st.preds, ipaddr6Predicate(value))
		return nil
	}
	// A reverse relation key in upstream spelling (fac?ix_side_set__asn=)
	// where no traversal key reaches the related rows.
	if rs, relField, relOp, isSet := lookupReverseSetKey(tc.Name, key); isSet {
		p, ok, empty, err := buildReverseSetPredicate(rs, relField, relOp, value)
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		if empty {
			st.empty = true
			return nil
		}
		if !ok {
			appendUnknown(ctx, key)
			return nil
		}
		st.preds = append(st.preds, p)
		return nil
	}
	relSegs, field, op := parseFieldOp(key)
	// Also check if the raw final field is a reserved name
	// (e.g. "fields" on a top-level single-segment key).
	if len(relSegs) == 0 && reservedParams[field] {
		return nil
	}
	// Upstream ignores a relation key whose field is a FK column
	// (net__org_id, see namesFKColumn), and status on a reverse or
	// 2-hop key (see relationStatusFilterable).
	if len(relSegs) > 0 && (namesFKColumn(field) ||
		field == "status" && !relationStatusFilterable(tc, relSegs)) {
		appendUnknown(ctx, key)
		return nil
	}
	// Hard cap: >2 relation segments is silently rejected.
	if len(relSegs) > 2 {
		appendUnknown(ctx, key)
		return nil
	}
	// Malformed split (empty final field, empty leading segment)
	// falls through to unknown-field handling.
	if field == "" {
		appendUnknown(ctx, key)
		return nil
	}

	if len(relSegs) == 0 {
		// Direct local field path — the original local-field behaviour.
		// A count seed is a prepare_query key: it uses the first
		// value of a repeated key (2.83.0 serializers.py:618-619).
		if tc.ExactCounts[field] {
			value = vals[0]
		}
		p, empty, ok, err := buildLocalPredicate(field, op, value, tc, st.spatial)
		if err != nil {
			return fmt.Errorf("filter %s: %w", key, err)
		}
		if empty {
			st.empty = true
			return nil
		}
		if !ok {
			appendUnknown(ctx, key)
			return nil
		}
		st.preds = append(st.preds, p)
		return nil
	}

	// Traversal path (1-hop or 2-hop). An upstream FK name as the
	// first segment (network__asn) walks the matching mirror edge.
	relSegs[0] = traversalKeyFor(tc, relSegs[0])
	p, ok, empty, err := buildTraversalPredicate(tc, relSegs, field, op, value, tier)
	if err != nil {
		return fmt.Errorf("filter %s: %w", key, err)
	}
	if empty {
		st.empty = true
		return nil
	}
	if !ok {
		appendUnknown(ctx, key)
		return nil
	}
	st.preds = append(st.preds, p)
	return nil
}

// isPrepareQueryKey reports whether an upstream prepare_query of typ
// handles key. prepare_query runs before name_search (2.83.0
// rest.py:488-500, :532-553), so its errors still return 400 when
// name_search matches no row. Every resolver of a prepare_query key
// must be listed here: the presence keys (asn_overlap included), the
// ix ipblock and capacity keys, the ixpfx whereis key, the relation
// keys, and the count seeds (TypeConfig.ExactCounts, with or without
// an operator). The bare netixlan ipaddr6 key, the meta keys and the
// legacy info_type keys are not: upstream handles them after
// name_search.
func isPrepareQueryKey(tc TypeConfig, key string) bool {
	typ := tc.Name
	if _, ok := lookupPresenceKey(typ, key); ok {
		return true
	}
	if lookupIPBlockKey(typ, key) {
		return true
	}
	if _, ok := lookupWhereisKey(typ, key); ok {
		return true
	}
	if _, ok := lookupCapacityFilter(typ, key); ok {
		return true
	}
	if _, _, ok := lookupRelationSeed(typ, key); ok {
		return true
	}
	relSegs, field, _ := parseFieldOp(key)
	return len(relSegs) == 0 && tc.ExactCounts[field]
}

// buildLocalPredicate extracts the original local-field behaviour into a
// helper returning a uniform (predicate, emptyResult, ok, err) shape so
// ParseFiltersCtx can treat local and traversal paths symmetrically.
//
// spatial is set in a distance search. Upstream then skips the location
// rules of its non-spatial branch (2.83.0 rest.py:582-595), so a bare
// country is an exact match for a value of any length. The other
// location keys of that branch do not reach this function in a
// distance search (spatialSkipKeys).
//
// ok=false => the field is unknown on tc; caller silently ignores.
// emptyResult=true => empty __in sentinel; caller short-circuits.
func buildLocalPredicate(field, op, value string, tc TypeConfig, spatial bool) (func(*sql.Selector), bool, bool, error) {
	col, ft, exists := resolveLocalField(tc, field)
	if !exists {
		return nil, false, false, nil
	}
	folded := tc.FoldedFields[col]
	if !spatial {
		op = coerceLocationFilterOp(col, op, value)
	}
	var p func(*sql.Selector)
	var err error
	if tc.ExactCounts[col] {
		// A prepare_query key: the raw value, converted with int().
		p, err = buildPredicate(col, op, value, ft, folded)
	} else {
		p, err = buildModelFieldPredicate(col, op, value, ft, folded, isFKColumn(tc, col))
	}
	if err != nil {
		if errors.Is(err, errEmptyIn) {
			return nil, true, false, nil
		}
		return nil, false, false, err
	}
	return p, false, true, nil
}

// buildTraversalPredicate resolves a 1-hop or 2-hop cross-entity filter.
// relSegs has len 1 or 2 (caller enforced the 2-hop cap). Resolution order is
// Path A (Allowlists) first, then Path B (LookupEdge introspection).
//
// Return shape mirrors buildLocalPredicate:
//   - predicate, true, false, nil on a resolved key
//   - nil, false, false, nil on an unknown key (silent ignore)
//   - nil, false, true, nil on an empty __in sentinel bubbling up from the
//     target-side buildPredicate
//   - nil, false, false, err on conversion errors (int, bool, etc.)
func buildTraversalPredicate(tc TypeConfig, relSegs []string, field, op, value string, tier privctx.Tier) (func(*sql.Selector), bool, bool, error) {
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
	// schema could suppress a resolution that would have worked.
	// Hard errors (emptyResult, conversion err) still propagate
	// immediately.
	entry, hasAllowlist := Allowlists[tc.Name]
	if hasAllowlist {
		if len(relSegs) == 1 {
			if slices.Contains(entry.Direct, fullKey) {
				p, ok, empty, err := buildSinglHop(tc.Name, relSegs[0], field, op, value, tier)
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
					p, ok, empty, err := buildTwoHop(tc.Name, relSegs[0], relSegs[1], field, op, value, tier)
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
		return buildSinglHop(tc.Name, relSegs[0], field, op, value, tier)
	}
	// 2-hop Path B: second hop edge must exist on the intermediate target.
	edge2, okEdge2 := LookupEdge(edge.TargetType, relSegs[1])
	if !okEdge2 {
		return nil, false, false, nil
	}
	if _, hasField := TargetFields(edge2.TargetType)[field]; !hasField {
		return nil, false, false, nil
	}
	return buildTwoHop(tc.Name, relSegs[0], relSegs[1], field, op, value, tier)
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
// All SQL identifiers come from the codegen-emitted EdgeMetadata
// — never from user input. The inner predicate is produced by the existing
// buildPredicate path on the target TypeConfig, so _fold routing
// and empty-__in sentinels apply at the target entity.
//
// Parent-side PK is always "id" for every PeeringDB entity — an invariant
// baked into the schema generator; no entity overrides its ID column.
// tierGatedTables maps a traversal target table to the column carrying its
// row-level visibility signal. A subquery built against one of these tables
// MUST reproduce the visibility filter that the entity's ent Privacy policy
// enforces on direct queries (ent/schema/poc_policy.go). Without it a
// cross-entity filter becomes a boolean oracle: a caller could probe rows
// that its tier cannot read. For example, GET /api/net?pocs__email__startswith=
// would leak Users-tier poc PII to TierPublic, or Private poc PII to
// TierUsers, that the policy hides on /api/poc.
var tierGatedTables = map[string]string{
	"pocs": "visible", // poc.visible: admitted per privctx.Tier.AdmittedVisibilities
}

// applyVisibilityGate ANDs the row-visibility predicate onto a traversal
// subquery when the target table is privacy-gated. The predicate admits
// the visibility values of the caller's tier
// (privctx.Tier.AdmittedVisibilities), the same set as the ent policy. A
// NULL visible value is treated as the column default ("Public") and
// therefore visible, mirroring poc_policy.go NULL-safety.
func applyVisibilityGate(sel *sql.Selector, table string, tier privctx.Tier) {
	col, gated := tierGatedTables[table]
	if !gated {
		return
	}
	admitted := tier.AdmittedVisibilities()
	args := make([]any, len(admitted))
	for i, v := range admitted {
		args[i] = v
	}
	sel.Where(sql.Or(
		sql.In(sel.C(col), args...),
		sql.IsNull(sel.C(col)),
	))
}

// traversalTargetField returns the type of field on the last row of a
// Path A or Path B traversal key, or ok=false when the key cannot filter
// it. A field that is not an upstream model field
// (TypeConfig.NonModelFields) is no target: queryable_relations offers
// only model fields (2.83.0 serializers.py:970-996), so upstream ignores
// the key. A model field that UpstreamIgnored hides from the local key
// stays a target (carrierfac?carrier__fac_count=).
func traversalTargetField(tc TypeConfig, field string) (FieldType, bool) {
	if tc.NonModelFields[field] {
		return 0, false
	}
	ft, ok := tc.Fields[field]
	return ft, ok
}

func buildSinglHop(entityType, fk, field, op, value string, tier privctx.Tier) (func(*sql.Selector), bool, bool, error) {
	edge, ok := LookupEdge(entityType, fk)
	if !ok {
		return nil, false, false, nil
	}
	targetTC, hasTarget := Registry[edge.TargetType]
	if !hasTarget {
		return nil, false, false, nil
	}
	ft, hasField := traversalTargetField(targetTC, field)
	if !hasField {
		return nil, false, false, nil
	}
	folded := targetTC.FoldedFields[field]
	innerPred, err := buildModelFieldPredicate(field, op, value, ft, folded, false)
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
			applyVisibilityGate(innerSel, targetTable, tier)
			s.Where(sql.In(s.C(fkCol), innerSel))
			return
		}
		// O2M: FK on child. Select child.<fk>, filter parent.<id>.
		innerSel := sql.Select(t.C(fkCol)).From(t)
		innerPred(innerSel)
		applyVisibilityGate(innerSel, targetTable, tier)
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
// Hard-capped at 2 hops. Identifiers from two EdgeMetadata lookups;
// values bind via the innermost buildPredicate. Parent PK is always "id"
// (see parentPKColumn — schema generator invariant).
// Both subqueries get the row-visibility gate of their table
// (applyVisibilityGate).
func buildTwoHop(entityType, fk1, fk2, field, op, value string, tier privctx.Tier) (func(*sql.Selector), bool, bool, error) {
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
	ft, hasField := traversalTargetField(leafTC, field)
	if !hasField {
		return nil, false, false, nil
	}
	folded := leafTC.FoldedFields[field]
	innerPred, err := buildModelFieldPredicate(field, op, value, ft, folded, false)
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
		applyVisibilityGate(leafSel, leafTable, tier)

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
		// Gate the middle rows too. A middle hop on pocs
		// (net?poc__net__<field>=) would otherwise match the networks
		// that have a contact the tier cannot read.
		applyVisibilityGate(midSel, midTable, tier)

		// Outer filter: parent's FK column (M2O) or parent's PK (O2M).
		if ownFK1 {
			s.Where(sql.In(s.C(fk1Col), midSel))
		} else {
			s.Where(sql.In(s.C(parentPKColumn), midSel))
		}
	}, true, false, nil
}

// buildModelFieldPredicate builds the predicate of a key that the
// upstream filter loop resolves as a model field (2.83.0
// rest.py:670-683). The loop folds every value with unidecode first
// (rest.py:597), which also turns every Unicode decimal digit into its
// ASCII digit (ndToASCII). A plain key on an integer field is an
// __iexact filter upstream, and Django does not convert an iexact value
// (IExact.prepare_rhs=False, django/db/models/lookups.py:430-438):
// MySQL compares the integer as decimal text, so only the decimal form
// of the stored value matches. A FK column (exactInt) and the operators
// convert the value with int() (pyInt), so a bad value is a 400.
func buildModelFieldPredicate(col, op, value string, ft FieldType, folded, exactInt bool) (func(*sql.Selector), error) {
	if ft == FieldInt {
		value = ndToASCII(unifold.Fold(value))
		if !exactInt && (op == "" || op == "iexact") {
			return intTextMatch(col, value), nil
		}
	}
	if expr := numericText(ft); expr != "" && !exactInt {
		text := likeEscape(ndToASCII(unifold.Fold(value)))
		switch coerceToCaseInsensitive(op) {
		case "icontains":
			return textLike(col, expr, "%"+text+"%"), nil
		case "istartswith":
			return textLike(col, expr, text+"%"), nil
		case "", "iexact":
			if ft == FieldFloat {
				return textLike(col, expr, text), nil
			}
		}
	}
	if ft == FieldTime {
		switch op := coerceToCaseInsensitive(op); op {
		case "":
			return dateTextPrefix(col, value), nil
		case "lt", "lte", "gt", "gte", "icontains", "istartswith":
			return buildDateOperator(col, op, value)
		}
	}
	if ft == FieldBool {
		switch op {
		case "":
			// upstream rest.py:680-681: v.lower() == "true" or v == "1",
			// any other value selects false.
			f := ndToASCII(unifold.Fold(value))
			return sql.FieldEQ(col, f == "true" || f == "1"), nil
		case "lt", "lte", "gt", "gte", "in":
			return buildBoolOperator(col, op, value)
		}
	}
	return buildPredicate(col, op, value, ft, folded)
}

// numericText returns the SQL expression, with one %s for the column,
// that renders a column of type ft as MySQL renders it as text, or ""
// for a type that is not numeric. Upstream does not convert the value of
// __icontains, __istartswith and __iexact (PatternLookup and IExact set
// prepare_rhs=False, django/db/models/lookups.py), so MySQL compares
// the column as text with LIKE: an integer as decimal text, a boolean
// (tinyint) as 1 or 0, and a DecimalField(max_digits=9,
// decimal_places=6) as its text with 6 decimals, for example 52.500000.
// printf renders NULL as 0.000000, so the decimal form keeps NULL.
func numericText(ft FieldType) string {
	switch ft {
	case FieldInt, FieldBool:
		return "CAST(%s AS TEXT)"
	case FieldFloat:
		return "CASE WHEN %[1]s IS NULL THEN NULL ELSE printf('%%.6f', %[1]s) END"
	case FieldString, FieldTime, FieldMultiChoice:
		return ""
	default:
		return ""
	}
}

// textLike returns the predicate expr LIKE pattern, where expr holds one
// %s for col and pattern is escaped for ESCAPE '\'. SQLite LIKE ignores
// ASCII case, as the MySQL collation does.
func textLike(col, expr, pattern string) func(*sql.Selector) {
	return func(s *sql.Selector) {
		s.Where(sql.ExprP(fmt.Sprintf(expr, s.C(col))+` LIKE ? ESCAPE '\'`, pattern))
	}
}

// dateTextPrefix builds the predicate of a date key without an
// operator. Upstream filters it with __startswith (2.83.0
// rest.py:678-679), which does not convert the value, so MySQL matches
// the value as a prefix of the DATETIME(6) text of the column,
// "YYYY-MM-DD HH:MM:SS.ffffff" in UTC: ?created=2024-01 matches the
// month and ?created=1700000000 matches no row. The mirror stores the
// time as "YYYY-MM-DD HH:MM:SS +0000 UTC" (time.Time.String, whole
// seconds), so the first 19 bytes are the upstream text up to the
// seconds. The microseconds are not stored: a value that goes on with a
// decimal point and up to 6 digits matches every row of its second.
func dateTextPrefix(col, value string) func(*sql.Selector) {
	value = ndToASCII(value)
	const secondsLen = len("2006-01-02 15:04:05")
	if len(value) > secondsLen {
		frac := value[secondsLen:]
		if frac[0] != '.' || len(frac) > 7 {
			return func(s *sql.Selector) { s.Where(sql.False()) }
		}
		if _, ok := digitsAt(frac, 1, len(frac)-1); !ok {
			return func(s *sql.Selector) { s.Where(sql.False()) }
		}
		value = value[:secondsLen]
	}
	return textLike(col, "substr(%s, 1, 19)", likeEscape(value)+"%")
}

// buildDateOperator builds an operator predicate on a date model field,
// as upstream does (2.83.0 rest.py:640-662): for gt and lte, a value of
// 10 characters (a date) gets " 23:59:59.999", so the whole day counts.
// Then Django DateTimeField.to_python converts the value
// (djangoDateTime), and a value that it rejects is a 400. icontains and
// istartswith compare the text of the converted value, which ends in a
// zone ("+00:00"), with the MySQL text of the column, which has none, so
// they match no row.
func buildDateOperator(col, op, value string) (func(*sql.Selector), error) {
	if (op == "gt" || op == "lte") && utf8.RuneCountInString(value) == 10 {
		value += " 23:59:59.999"
	}
	t, err := djangoDateTime(value)
	if err != nil {
		return nil, fmt.Errorf("convert %q to time: %w", value, err)
	}
	switch op {
	case "lt":
		return sql.FieldLT(col, t), nil
	case "lte":
		return sql.FieldLTE(col, t), nil
	case "gt":
		return sql.FieldGT(col, t), nil
	case "gte":
		return sql.FieldGTE(col, t), nil
	default:
		return func(s *sql.Selector) { s.Where(sql.False()) }, nil
	}
}

// nullableBoolFields lists the boolean model fields that allow NULL
// upstream (django-peeringdb models/abstract.py:259). Django converts an
// empty value on these fields to None.
var nullableBoolFields = map[string]bool{"diverse_serving_substations": true}

// buildBoolOperator builds an operator predicate on a boolean model
// field. Upstream passes the value to the lookup as is (rest.py:664-669),
// and Django converts it with BooleanField.to_python
// (django/db/models/fields/__init__.py BooleanField.to_python): only t,
// True, 1, f, False and 0 are valid, and the case counts. __in splits the
// value on commas and does not strip the items. On a nullable field an
// empty value is None: the comparisons reject it, and __in drops it (In
// discards None, and a list with no other item matches no row).
func buildBoolOperator(col, op, value string) (func(*sql.Selector), error) {
	nullable := nullableBoolFields[col]
	if op != "in" {
		v, err := djangoBool(value, nullable)
		if err != nil {
			return nil, err
		}
		if v == nil {
			return nil, fmt.Errorf("convert %q to bool: cannot use None as a query value", value)
		}
		switch op {
		case "lt":
			return sql.FieldLT(col, *v), nil
		case "lte":
			return sql.FieldLTE(col, *v), nil
		case "gt":
			return sql.FieldGT(col, *v), nil
		default:
			return sql.FieldGTE(col, *v), nil
		}
	}
	var items []any
	for p := range strings.SplitSeq(value, ",") {
		v, err := djangoBool(p, nullable)
		if err != nil {
			return nil, fmt.Errorf("convert %q to bool for IN: %w", p, err)
		}
		if v != nil {
			items = append(items, *v)
		}
	}
	if len(items) == 0 {
		return nil, errEmptyIn
	}
	return sql.FieldIn(col, items...), nil
}

// djangoBool converts s as Django BooleanField.to_python does. It
// returns nil for an empty value on a nullable field.
func djangoBool(s string, nullable bool) (*bool, error) {
	var v bool
	switch s {
	case "t", "True", "1":
		v = true
	case "f", "False", "0":
		v = false
	case "":
		if nullable {
			return nil, nil
		}
		return nil, fmt.Errorf("invalid bool value %q: use t, True, 1, f, False or 0", s)
	default:
		return nil, fmt.Errorf("invalid bool value %q: use t, True, 1, f, False or 0", s)
	}
	return &v, nil
}

// intTextMatch returns the predicate col = n when value is the decimal
// text of the int n (ASCII digits, an optional '-', no leading zeros),
// and a predicate that matches no row otherwise. It never returns the
// empty-result sentinel: upstream runs every filter in one
// qset.filter() call, so an error of another key must still win.
func intTextMatch(col, value string) func(*sql.Selector) {
	n, err := strconv.Atoi(value)
	if err != nil || strconv.Itoa(n) != value {
		return func(s *sql.Selector) { s.Where(sql.False()) }
	}
	return sql.FieldEQ(col, n)
}

// buildPredicate maps a field, operator, raw value, and field type to an ent
// sql.Selector predicate function. folded=true indicates the field has a
// sibling <field>_fold column — string predicates route to it with a
// unifold.Fold(value) RHS for diacritic-insensitive matching. A
// multi-value field has its own operators (buildMultiChoicePredicate).
func buildPredicate(field, op, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	if ft == FieldMultiChoice {
		return buildMultiChoicePredicate(field, op, value)
	}
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
		return buildIn(field, value, ft, folded)
	case "lt", "gt", "lte", "gte":
		if ft == FieldString {
			return buildStringComparison(field, op, value, folded), nil
		}
	}
	switch op {
	case "lt":
		return buildComparison(field, op, value, ft, sql.FieldLT)
	case "gt":
		return buildComparison(field, op, value, ft, sql.FieldGT)
	case "lte":
		return buildComparison(field, op, value, ft, sql.FieldLTE)
	case "gte":
		return buildComparison(field, op, value, ft, sql.FieldGTE)
	default:
		return nil, fmt.Errorf("unsupported operator %q", op)
	}
}

// buildExact builds a predicate for exact match. String fields use
// case-insensitive matching. When folded=true, string matches go
// through the <field>_fold column with unifold.Fold(value).
func buildExact(field, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	switch ft {
	case FieldString:
		if folded {
			return sql.FieldEqualFold(field+"_fold", unifold.Fold(value)), nil
		}
		return sql.FieldEqualFold(field, value), nil
	case FieldInt:
		v, _, err := pyInt(value)
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
		t, dateOnly, err := parseTimeValue(value)
		if err != nil {
			return nil, fmt.Errorf("convert %q to time: %w", value, err)
		}
		if dateOnly {
			// upstream 2.83.0 rest.py:678-679 turns bare datetime equality
			// into __startswith — ?created=2024-01-01 matches the
			// whole day, not the instant of midnight.
			end := t.Add(24 * time.Hour)
			return func(s *sql.Selector) {
				sql.FieldGTE(field, t)(s)
				sql.FieldLT(field, end)(s)
			}, nil
		}
		return sql.FieldEQ(field, t), nil
	case FieldFloat:
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("convert %q to float: %w", value, err)
		}
		return sql.FieldEQ(field, v), nil
	case FieldMultiChoice:
		return buildMultiChoicePredicate(field, "", value)
	default:
		return nil, fmt.Errorf("unsupported field type %s for exact match", ft)
	}
}

// buildContains builds a case-insensitive contains predicate.
// folded=true routes to the <field>_fold column with unifold.Fold(value) for
// diacritic-insensitive matching.
func buildContains(field, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	if ft != FieldString {
		return nil, fmt.Errorf("contains operator not supported on non-string field %q", field)
	}
	if folded {
		return sql.FieldContainsFold(field+"_fold", unifold.Fold(value)), nil
	}
	return sql.FieldContainsFold(field, value), nil
}

// buildStartsWith builds a case-insensitive prefix match predicate.
// folded=true routes to the <field>_fold column with unifold.Fold(value) for
// diacritic-insensitive matching.
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
// at any list size.
//
// Upstream splits the value with str.split(","), which keeps empty
// items, and Django converts each item for the field (2.83.0
// rest.py:664-666). So an empty item of a string field matches an empty
// value, and on the other types it is a value that does not convert
// (400): ?asn__in= and ?asn__in=1, return 400.
func buildIn(field, value string, ft FieldType, folded bool) (func(*sql.Selector), error) {
	parts := strings.Split(value, ",")
	// Bool, float, and time IN lists bind each value as a parameter via
	// ent's converter (sql.FieldIn), exactly like buildExact's FieldEQ.
	// This keeps IN comparison semantics identical to single-value
	// matches — critical for time, whose stored text layout must not be
	// reconstructed by hand the way a json_each string compare would. These
	// columns never carry the very large value lists (e.g. asn__in=) that
	// motivated the json_each path, so the bound-parameter count stays well
	// under SQLite's variable limit.
	var jsonArr []byte
	var marshalErr error
	switch ft {
	case FieldString:
		// Upstream folds ALL filter values with unidecode (2.83.0 rest.py:597)
		// and matches under MySQL's case-insensitive collation, so
		// string __in is case- and diacritic-insensitive there. SQLite
		// resolves a bare IN with the column's BINARY collation, so
		// lower-case both sides here (and route folded fields through
		// the <field>_fold shadow column), keeping __in consistent
		// with the exact/contains/startswith operators on the same
		// field.
		// The MySQL collations pad with spaces (PAD SPACE), so trailing
		// spaces do not count and leading spaces do.
		trimmed := make([]string, len(parts))
		for i, p := range parts {
			v := strings.TrimRight(p, " ")
			if folded {
				v = unifold.Fold(v)
			}
			trimmed[i] = strings.ToLower(v)
		}
		jsonArr, marshalErr = json.Marshal(trimmed)
	case FieldInt:
		ints := make([]int, 0, len(parts))
		for _, p := range parts {
			// Use parseErr here so a future refactor that introduces an
			// outer `err` can't silently shadow the loop error (W1 fix).
			v, _, parseErr := pyInt(p)
			if parseErr != nil {
				return nil, fmt.Errorf("convert %q to int for IN: %w", p, parseErr)
			}
			ints = append(ints, v)
		}
		jsonArr, marshalErr = json.Marshal(ints)
	case FieldBool:
		bools := make([]bool, 0, len(parts))
		for _, p := range parts {
			v, parseErr := parseBool(strings.TrimSpace(p))
			if parseErr != nil {
				return nil, fmt.Errorf("convert %q to bool for IN: %w", p, parseErr)
			}
			bools = append(bools, v)
		}
		return sql.FieldIn(field, bools...), nil
	case FieldFloat:
		floats := make([]float64, 0, len(parts))
		for _, p := range parts {
			v, parseErr := strconv.ParseFloat(strings.TrimSpace(p), 64)
			if parseErr != nil {
				return nil, fmt.Errorf("convert %q to float for IN: %w", p, parseErr)
			}
			floats = append(floats, v)
		}
		return sql.FieldIn(field, floats...), nil
	case FieldTime:
		times := make([]time.Time, 0, len(parts))
		for _, p := range parts {
			v, _, parseErr := parseTimeValue(strings.TrimSpace(p))
			if parseErr != nil {
				return nil, fmt.Errorf("convert %q to time for IN: %w", p, parseErr)
			}
			times = append(times, v)
		}
		return sql.FieldIn(field, times...), nil
	case FieldMultiChoice:
		return buildMultiChoicePredicate(field, "in", value)
	default:
		return nil, fmt.Errorf("in operator not supported on field type %s for field %q", ft, field)
	}
	if marshalErr != nil {
		return nil, fmt.Errorf("marshal IN array: %w", marshalErr)
	}
	jsonStr := string(jsonArr)
	col := field
	if ft == FieldString && folded {
		col = field + "_fold"
	}
	lowered := ft == FieldString
	return func(s *sql.Selector) {
		// s.C(col) quotes the column identifier via the ent builder —
		// the column name itself is already validated against tc.Fields
		// by ParseFilters, so no injection surface. The JSON payload
		// binds as a single parameter via ExprP (SQL-injection mitigation).
		// String lists were lower-cased at build time, so the column is
		// wrapped in LOWER() to match; int lists compare numerically.
		expr := s.C(col)
		if lowered {
			expr = "LOWER(" + expr + ")"
		}
		s.Where(sql.ExprP(expr+" IN (SELECT value FROM json_each(?))", jsonStr))
	}, nil
}

// stringComparisons maps a comparison operator to its SQL operator.
var stringComparisons = map[string]string{"lt": "<", "lte": "<=", "gt": ">", "gte": ">="}

// buildStringComparison compares a string field as the MySQL collation
// of upstream does: case does not count, and upstream folds the value
// with unidecode (2.83.0 rest.py:597), with accents equal to their base
// letter under the collation. A folded field compares its <field>_fold
// column with the folded value; another field compares the lower case
// of both sides. The collation also weighs punctuation differently from
// the byte order that SQLite uses, which this does not copy.
func buildStringComparison(field, op, value string, folded bool) func(*sql.Selector) {
	cmp := stringComparisons[op]
	if folded {
		return func(s *sql.Selector) {
			s.Where(sql.ExprP(s.C(field+"_fold")+" "+cmp+" ?", unifold.Fold(value)))
		}
	}
	return func(s *sql.Selector) {
		s.Where(sql.ExprP("LOWER("+s.C(field)+") "+cmp+" ?", strings.ToLower(unifold.Fold(value))))
	}
}

// buildComparison builds a comparison predicate (lt, gt, lte, gte) with value
// type conversion.
func buildComparison(field, op, value string, ft FieldType, cmp func(string, any) func(*sql.Selector)) (func(*sql.Selector), error) {
	if ft == FieldTime {
		t, dateOnly, err := parseTimeValue(value)
		if err != nil {
			return nil, err
		}
		if dateOnly && (op == "gt" || op == "lte") {
			// upstream 2.83.0 rest.py:642-645: a 10-char date in gt/lte gets
			// its time forced to end-of-day (23:59:59.999), so
			// updated__gt=2024-01-01 means "after that whole day"
			// and updated__lte=2024-01-01 includes the whole day.
			t = t.Add(24*time.Hour - time.Millisecond)
		}
		return cmp(field, t), nil
	}
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
		v, _, err := pyInt(s)
		if err != nil {
			return nil, fmt.Errorf("convert %q to int: %w", s, err)
		}
		return v, nil
	case FieldBool:
		return parseBool(s)
	case FieldTime:
		t, _, err := parseTimeValue(s)
		return t, err
	case FieldFloat:
		return strconv.ParseFloat(s, 64)
	case FieldMultiChoice:
		// A comparison converts the value to the stored form, which
		// needs the choice list (buildMultiChoicePredicate).
		return nil, fmt.Errorf("unsupported field type %s", ft)
	default:
		return nil, fmt.Errorf("unsupported field type %s", ft)
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

// parseTimeValue converts a time-filter value. Accepts Unix epoch seconds
// plus the ISO 8601 layouts DRF's DateTimeField().to_python accepts
// upstream (2.83.0 rest.py:647-653): date-only, datetime with 'T' or
// space separator, and RFC 3339 with offset. dateOnly reports a bare
// 10-char date, which carries day-window semantics upstream
// (rest.py:640-679).
// Layouts without an explicit offset are interpreted as UTC, matching
// the stored timestamps. Every result is converted to UTC: the SQLite
// driver binds a time.Time as text in the zone of the value, and the
// stored timestamps are UTC text, so a value with an offset such as
// +01:00 (or an epoch value in a process zone that is not UTC) would
// compare wrongly.
func parseTimeValue(s string) (t time.Time, dateOnly bool, err error) {
	if epoch, perr := strconv.ParseInt(s, 10, 64); perr == nil {
		return time.Unix(epoch, 0).UTC(), false, nil
	}
	if t, perr := time.ParseInLocation(time.DateOnly, s, time.UTC); perr == nil {
		return t, true, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
		if t, perr := time.ParseInLocation(layout, s, time.UTC); perr == nil {
			return t.UTC(), false, nil
		}
	}
	return time.Time{}, false, fmt.Errorf("invalid time value %q (want unix epoch seconds or ISO 8601)", s)
}
