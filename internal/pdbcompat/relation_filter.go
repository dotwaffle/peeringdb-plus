package pdbcompat

import (
	"errors"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// This file ports the relation keys that an upstream serializer handles
// in prepare_query (PeeringDB 2.83.0). get_relation_filters
// (serializers.py:614-656) parses the key, and a related_to_<x> model
// method filters the related rows through make_relation_filter
// (models.py:221-234). make_relation_filter also keeps only the rows of
// one table with status "ok", whatever the key asks for. The mirror
// walks the same rows through the generated Edges.

var (
	// errInvalidQuery is the 400 that upstream returns when the Django
	// filter of a prepare_query raises FieldError (2.83.0
	// rest.py:499-500): a relation key names a field that the related
	// model does not have.
	errInvalidQuery = errors.New("Invalid query") //nolint:staticcheck // exact upstream message text
	// errIsNullValue is the 400 that upstream returns for isnull as the
	// field of a relation key. Django rejects the string value when it
	// compiles the query (django/db/models/lookups.py:677-680), and
	// list() returns the text (rest.py:824-827).
	errIsNullValue = errors.New("The QuerySet value for an isnull lookup must be True or False.") //nolint:revive,staticcheck // exact upstream message text
)

// unservedModelNames lists, per leaf type of a shapeRelation seed, the
// upstream model names that Django resolves and that the mirror does
// not filter: model columns that the API does not serialize or that the
// mirror does not keep, the stored meta document (no plain filter key),
// and reverse related names. Upstream filters on them. The mirror does
// not, so a relation key with such a field is ignored (see docs/API.md § Known Divergences). Every other field that
// the related model does not have returns errInvalidQuery.
//
// Sources: django-handleref models.py:86-90 (version on every model),
// django-peeringdb models/abstract.py (abstract.py below), the mixins
// in models.py:533-599, and the related_name of each FK in models.py
// (PeeringDB 2.83.0). ixf_ixp_member_list_url is
// privacy-gated in the mirror, so a filter on it must never apply.
var unservedModelNames = map[string]map[string]bool{
	// Network (models.py:5310).
	peeringdb.TypeNet: {
		"version":                          true,
		"social_media":                     true,
		"meta":                             true, // abstract.py:417
		"notes_private":                    true, // abstract.py:436
		"ixp_update_exclude":               true, // models.py:5332
		"rir_status_notified":              true, // :5345
		"irr_as_set_auto_prefix_candidate": true, // :5350
		"irr_as_set_auto_prefix_checked":   true, // :5353
		"irr_as_set_notified":              true, // :5360
		"irr_as_set_cap_notified":          true, // :5364
		"irr_as_set_status":                true, // :5374
		"irr_as_set_verified":              true, // :5383
		"irr_as_set_missing_since":         true, // :5389
		"irr_as_set_verify_notified":       true, // :5396
		"poc_set":                          true, // :5883
		"netfac_set":                       true, // :5988
		"netixlan_set":                     true, // :6083
		"network_email_set":                true, // :7296
	},
	// InternetExchange (models.py:2580).
	peeringdb.TypeIX: {
		"version":                 true,
		"social_media":            true,
		"ixf_import_request_user": true, // models.py:2612
		"ixfac_set":               true, // :3230
		"ixlan_set":               true, // :3308
		"ix_email_set":            true, // :7303
	},
	// IXLan (models.py:3297).
	peeringdb.TypeIXLan: {
		"version":                          true,
		"vlan":                             true, // abstract.py:791
		"ixf_ixp_member_list_url":          true, // abstract.py:819
		"ixf_ixp_import_error":             true, // models.py:3319
		"ixf_ixp_import_error_notified":    true, // :3325
		"ixf_ixp_import_protocol_conflict": true, // :3331
		"ixf_import_attempt":               true, // :3723
		"ixf_import_log_set":               true, // :3736
		"ixf_set":                          true, // :3903
		"ixpfx_set":                        true, // :5149
		"netixlan_set":                     true, // :6086
	},
	// InternetExchangeFacility (models.py:3222).
	peeringdb.TypeIXFac: {
		"version": true,
	},
	// Facility (models.py:2194). net_side_set is not here:
	// queryable_field_xl renames it to network_side_set, which names no
	// relation.
	peeringdb.TypeFac: {
		"version":                true,
		"social_media":           true,
		"location_method":        true, // models.py:2207
		"location_place_id":      true, // :2214
		"notified_for_geocoords": true, // :2257
		"geocode_status":         true, // :591
		"geocode_date":           true, // :597
		"ixfac_set":              true, // :3233
		"netfac_set":             true, // :5991
		"carrierfac_set":         true, // :6598
		"ix_side_set":            true, // :6100
		"ixf_member_data_set":    true, // :3888
	},
	// NetworkIXLan (models.py:6075).
	peeringdb.TypeNetIXLan: {
		"version":                true,
		"meta":                   true, // abstract.py:953
		"ixf_import_log_entries": true, // models.py:3786
	},
	// NetworkFacility (models.py:5980).
	peeringdb.TypeNetFac: {
		"version":        true,
		"avail_sonet":    true, // abstract.py:879-889
		"avail_ethernet": true,
		"avail_atm":      true,
	},
}

// seedShape selects the key forms that a relation seed accepts.
type seedShape int

const (
	// shapeRelation accepts the forms of get_relation_filters: the seed
	// name alone, with an operator, with a field of the related row, and
	// with a field and an operator. A third segment that is not an
	// operator is dropped (serializers.py:643-654).
	shapeRelation seedShape = iota
	// shapeFixedField accepts the seed name alone or with an operator.
	// The key filters relationSeed.field.
	shapeFixedField
	// shapeReplacedField accepts the forms of shapeRelation, but the
	// prepare_query replaces the parsed key with relationSeed.field, so
	// a segment that is not an operator has no effect (netfac and ixfac,
	// serializers.py:3417-3424 and :2840-2847).
	shapeReplacedField
	// shapeWholeKey accepts only the whole key, with no segment after
	// it. The key filters relationSeed.field with relationSeed.bareOp.
	shapeWholeKey
)

// noPin is the relationSeed.pinAt value of a seed that pins no row to
// status "ok": the prepare_query filters without make_relation_filter.
const noPin = -1

// relationSeed describes one entry of the flds list that a prepare_query
// gives get_relation_filters.
type relationSeed struct {
	// hops lists the traversal keys from the listed type to the related
	// rows. Each hop is an edge in Edges.
	hops []string
	// pinAt is the row that make_relation_filter pins to status "ok".
	// 0 is the listed row, and i is the row after hops[i-1]. noPin
	// pins no row.
	pinAt int
	shape seedShape
	// field is the column that a shapeFixedField, shapeReplacedField or
	// shapeWholeKey seed filters on the last row.
	field string
	// bareOp is the operator of the seed name without an operator
	// suffix, and of a shapeWholeKey seed. Empty means an exact match.
	bareOp string
	// prefix is the prefix= argument of make_relation_filter for a
	// shapeRelation seed that filters the pinned row itself
	// (models.py:2723, :2740, :5629, :5644). Empty for every other
	// seed.
	prefix string
}

// facilityFieldSeeds returns the netfac and ixfac seeds name, country
// and city. The prepare_query of both types replaces the key with
// facility__<seed> (serializers.py:3417-3424, :2840-2847), and
// related_to_<seed> pins the listed row (models.py:6002-6037,
// :3239-3274). The keys are not model fields, so the location rewrite
// of rest.py:583-595 does not apply: city and country are exact
// matches.
func facilityFieldSeeds() map[string]relationSeed {
	m := make(map[string]relationSeed, 3)
	for _, f := range []string{"name", "country", "city"} {
		m[f] = relationSeed{hops: []string{"fac"}, pinAt: 0, shape: shapeReplacedField, field: f}
	}
	return m
}

// relationSeeds maps a type to its relation seeds, keyed by the first
// segment of the query key, or by the full key for a shapeWholeKey
// seed. Some shapeRelation seeds also answer to the "<name>_id"
// spelling, when the upstream seed list names both (withIDSpellings):
// get_relation_filters strips "_id" from the key (queryable_field_xl,
// serializers.py:403-441).
var relationSeeds = map[string]map[string]relationSeed{
	// FacilitySerializer.prepare_query (serializers.py:2092-2117):
	// related_to_net and related_to_ix filter the netfac and ixfac rows
	// (models.py:2332-2346, :2402-2416). org_name filters org__name, a
	// substring match when the key has no operator, and pins no row.
	peeringdb.TypeFac: func() map[string]relationSeed {
		m := withIDSpellings(map[string]relationSeed{
			"net": {hops: []string{"netfac", "net"}, pinAt: 1},
			"ix":  {hops: []string{"ixfac", "ix"}, pinAt: 1},
		})
		m["org_name"] = relationSeed{hops: []string{"org"}, pinAt: noPin, shape: shapeFixedField, field: "name", bareOp: "icontains"}
		return m
	}(),
	// InternetExchangeSerializer.prepare_query (serializers.py:4503-4529)
	// and models.py:2712-2774. related_to_net pins the netixlan row, and
	// the ixlan row between it and the exchange has no status check.
	peeringdb.TypeIX: withIDSpellings(map[string]relationSeed{
		"ixlan": {hops: []string{"ixlan"}, pinAt: 1, prefix: "ixlan"},
		"ixfac": {hops: []string{"ixfac"}, pinAt: 1, prefix: "ixfac"},
		"fac":   {hops: []string{"ixfac", "fac"}, pinAt: 1},
		"net":   {hops: []string{"ixlan", "netixlan", "net"}, pinAt: 2},
	}),
	// NetworkSerializer.prepare_query (serializers.py:3708-3740) and
	// models.py:5587-5680.
	peeringdb.TypeNet: withIDSpellings(map[string]relationSeed{
		"ix":       {hops: []string{"netixlan", "ixlan", "ix"}, pinAt: 1},
		"ixlan":    {hops: []string{"netixlan", "ixlan"}, pinAt: 1},
		"netixlan": {hops: []string{"netixlan"}, pinAt: 1, prefix: "netixlan"},
		"netfac":   {hops: []string{"netfac"}, pinAt: 1, prefix: "netfac"},
		"fac":      {hops: []string{"netfac", "fac"}, pinAt: 1},
	}),
	// NetworkIXLanSerializer.prepare_query (serializers.py:3152-3169):
	// related_to_ix and related_to_name filter the ixlan rows
	// (models.py:6172-6197). A bare name, and name with an operator that
	// get_relation_filters parses, becomes ix__name. get_relation_filters
	// does not parse iexact, icontains or istartswith, so it keeps the
	// whole key (serializers.py:643-654), and related_to_name applies
	// the lookup to the name of the ixlan.
	peeringdb.TypeNetIXLan: func() map[string]relationSeed {
		m := withIDSpellings(map[string]relationSeed{
			"ix": {hops: []string{"ixlan", "ix"}, pinAt: 1},
		})
		m["name"] = relationSeed{hops: []string{"ixlan", "ix"}, pinAt: 1, shape: shapeFixedField, field: "name"}
		for _, op := range []string{"iexact", "icontains", "istartswith"} {
			m["name__"+op] = relationSeed{hops: []string{"ixlan"}, pinAt: 1, shape: shapeWholeKey, field: "name", bareOp: op}
		}
		return m
	}(),
	// IXLanPrefixSerializer.prepare_query (serializers.py:4154-4163):
	// related_to_ix filters the prefix itself on ixlan__<field> and pins
	// the prefix row (models.py:5167-5177).
	peeringdb.TypeIXPfx: withIDSpellings(map[string]relationSeed{
		"ix": {hops: []string{"ixlan", "ix"}, pinAt: 0},
	}),
	peeringdb.TypeNetFac: facilityFieldSeeds(),
	peeringdb.TypeIXFac:  facilityFieldSeeds(),
	// CampusSerializer.prepare_query (serializers.py:4854-4869) renames
	// facility to fac_set, and related_to_facility pins the campus row
	// (models.py:2101-2111). The seed has no _id spelling: facility_id
	// is not a seed, and upstream ignores it.
	peeringdb.TypeCampus: {
		"facility": {hops: []string{"fac"}, pinAt: 0},
	},
	// OrganizationSerializer.prepare_query (serializers.py:4980-4983)
	// filters net_set__asn and net_set__status="ok" in one filter call,
	// so the same network must match both. asn__in and the other
	// operators are ignored.
	peeringdb.TypeOrg: {
		"asn": {hops: []string{"net"}, pinAt: 1, shape: shapeWholeKey, field: "asn"},
	},
	// CarrierSerializer.prepare_query (serializers.py:2712-2740) filters
	// carrierfac_set__facility without a status check. Only the full key
	// is a seed: with an operator, the key has three segments and
	// get_relation_filters does not match it.
	peeringdb.TypeCarrier: {
		"carrierfac_set__facility_id": {hops: []string{"carrierfac"}, pinAt: noPin, shape: shapeWholeKey, field: "fac_id"},
	},
}

// withIDSpellings adds the "<name>_id" spelling of each seed in m.
func withIDSpellings(m map[string]relationSeed) map[string]relationSeed {
	out := make(map[string]relationSeed, 2*len(m))
	for name, sd := range m {
		out[name] = sd
		out[name+"_id"] = sd
	}
	return out
}

// lookupRelationSeed reports whether key is or starts with a relation
// seed of typ, and returns the seed and the key segments after the seed
// name.
func lookupRelationSeed(typ, key string) (relationSeed, []string, bool) {
	if sd, ok := relationSeeds[typ][key]; ok {
		return sd, nil, true
	}
	segs := strings.Split(key, "__")
	sd, ok := relationSeeds[typ][segs[0]]
	return sd, segs[1:], ok
}

// parseTail returns the field (in upstream spelling) and the operator
// that the key segments after the seed name give, or ok=false when
// upstream does not filter the key form. On a shapeRelation or
// shapeReplacedField seed, the mirror also applies the iexact,
// icontains and istartswith suffixes, which get_relation_filters does
// not parse (see docs/API.md § Known Divergences).
func (sd relationSeed) parseTail(tail []string) (field, op string, ok bool) {
	switch sd.shape {
	case shapeWholeKey:
		if len(tail) == 0 {
			return sd.field, sd.bareOp, true
		}
		return "", "", false
	case shapeFixedField:
		// Only the operators of get_relation_filters apply, the same
		// set as the upstream filter loop (metaOperators). With another
		// suffix, the prepare_query does not match the key.
		switch {
		case len(tail) == 0:
			return sd.field, sd.bareOp, true
		case len(tail) == 1 && metaOperators[tail[0]]:
			return sd.field, tail[0], true
		}
		return "", "", false
	case shapeReplacedField:
		if len(tail) > 2 {
			return "", "", false
		}
		if len(tail) > 0 && isKnownOperator(tail[len(tail)-1]) {
			op = tail[len(tail)-1]
		}
		return sd.field, op, true
	case shapeRelation:
		return parseRelationTail(tail)
	}
	return "", "", false
}

// parseRelationTail parses the key segments after a shapeRelation seed
// name as get_relation_filters does (serializers.py:614-656). No tail,
// or an operator alone, compares the id of the related row. A field of
// the related row can follow, and then an operator. A second segment
// that is not an operator is dropped, and a longer key is ignored.
func parseRelationTail(tail []string) (field, op string, ok bool) {
	switch len(tail) {
	case 0:
		return "id", "", true
	case 1:
		if isKnownOperator(tail[0]) {
			return "id", tail[0], true
		}
		return queryableFieldXL(tail[0]), "", true
	case 2:
		if isKnownOperator(tail[1]) {
			op = tail[1]
		}
		return queryableFieldXL(tail[0]), op, true
	}
	return "", "", false
}

// buildRelationSeedPredicate builds the filter of a relation seed key.
// It returns ok=false when upstream does not match the key form, or
// when the field is an upstream model name that the mirror does not
// store (unservedModelNames). It returns errInvalidQuery when the
// related model has no such field: the Django filter of the
// prepare_query raises FieldError, and upstream returns 400 (2.83.0
// rest.py:488-500).
//
// On a shapeRelation seed, the field resolves as a Django name
// (resolveModelName): a field that is not a model field upstream
// (TypeConfig.NonModelFields) is unknown. A prefix seed first applies
// the prefix rules of make_relation_filter (stripRelationPrefix), and
// the Django lookup names as the field follow relationLookupName.
//
// The predicate walks sd.hops with nested IN subqueries and pins the
// row at sd.pinAt, if any, to status "ok". When the key filters the status of the
// pinned row without an operator, make_relation_filter replaces the
// value with "ok", so only the pin applies. A key that compares the id
// of a row reached through a forward FK compares the FK column instead,
// unless that row is the pinned row. A multi-value field without an
// operator compares the value in its stored form (opCanonicalExact).
func buildRelationSeedPredicate(tc TypeConfig, sd relationSeed, tail []string, value string, tier privctx.Tier) (func(*sql.Selector), bool, bool, error) {
	field, op, ok := sd.parseTail(tail)
	if !ok {
		return nil, false, false, nil
	}
	if sd.shape == shapeRelation {
		// Only a field segment can repeat the prefix. The seed name
		// alone, or with an operator alone, already compares the id.
		if sd.prefix != "" && len(tail) > 0 && !isKnownOperator(tail[0]) {
			field = stripRelationPrefix(sd.prefix, field)
		}
		var err error
		field, op, err = relationLookupName(sd, field, op)
		if err != nil {
			return nil, false, false, err
		}
	}
	edges := make([]EdgeMetadata, len(sd.hops))
	rowType := tc.Name
	for i, hop := range sd.hops {
		e, ok := LookupEdge(rowType, hop)
		if !ok {
			return nil, false, false, nil
		}
		edges[i] = e
		rowType = e.TargetType
	}
	leafTC, ok := Registry[rowType]
	if !ok {
		return nil, false, false, nil
	}
	var col string
	var ft FieldType
	if sd.shape == shapeRelation {
		col, ft, ok = resolveModelName(leafTC, field)
	} else {
		col, ft, ok = resolveLocalField(leafTC, field)
		ok = ok && !leafTC.NonModelFields[col]
	}
	if !ok {
		if sd.shape == shapeRelation && !unservedModelNames[rowType][field] {
			return nil, false, false, errInvalidQuery
		}
		return nil, false, false, nil
	}
	folded := leafTC.FoldedFields[col]
	n := len(edges)
	if col == "id" && n > 0 && edges[n-1].OwnFK && sd.pinAt < n {
		col, ft, folded = edges[n-1].ParentFKColumn, FieldInt, false
		n--
	}
	if ft == FieldMultiChoice && op == "" {
		op = opCanonicalExact
	}
	var leaf func(*sql.Selector)
	if col != "status" || op != "" || sd.pinAt != n {
		p, err := buildPredicate(col, op, value, ft, folded)
		if err != nil {
			if errors.Is(err, errEmptyIn) {
				return nil, false, true, nil
			}
			return nil, false, false, err
		}
		leaf = p
	}
	return relationPathPredicate(edges[:n], sd.pinAt, leaf, tier), true, false, nil
}

// stripRelationPrefix applies the prefix rules of make_relation_filter
// (models.py:224-227) to the field segment of a relation key: it
// removes "<prefix>_" once, and changes a result equal to the prefix to
// "id". A segment holds no "__", so the "<prefix>__" rule of upstream,
// which removes the seed name from the whole key, has no equivalent
// here.
func stripRelationPrefix(prefix, field string) string {
	field = strings.TrimPrefix(field, prefix+"_")
	if field == prefix {
		return "id"
	}
	return field
}

// relationLookupName handles a Django lookup name as the field segment
// of a shapeRelation seed key, and returns the field and operator to
// filter. A relation through a FK accepts the lookups exact, lt, lte,
// gt, gte, in and isnull (django/db/models/fields/related.py:949-955):
// exact, lt, lte, gt and gte compare the id, as the seed name with an
// operator does. pk names the id on every seed. Any other field passes
// unchanged.
func relationLookupName(sd relationSeed, field, op string) (string, string, error) {
	switch field {
	case "pk":
		return "id", op, nil
	case "exact", "lt", "lte", "gt", "gte", "in", "isnull":
	default:
		return field, op, nil
	}
	if sd.prefix != "" {
		// A prefix seed filters the pinned model itself, which has no
		// field of that name.
		return "", "", errInvalidQuery
	}
	if op == "iexact" || op == "icontains" || op == "istartswith" {
		// get_relation_filters drops a third segment that it does not
		// parse (serializers.py:643-654).
		op = ""
	}
	if op != "" {
		// A lookup after a lookup (django/db/models/sql/query.py:1461).
		return "", "", errInvalidQuery
	}
	switch field {
	case "isnull":
		return "", "", errIsNullValue
	case "in":
		// Upstream iterates the characters of the value (RelatedIn,
		// django/db/models/fields/related_lookups.py:48-68), so 34
		// means the ids 3 and 4. The mirror does not copy this.
		return "", "", errInvalidQuery
	case "exact":
		return "id", "", nil
	}
	return "id", field, nil
}

// relationPathPredicate returns the predicate on the listed row that
// keeps the rows whose path through edges reaches a row that leaf
// matches. leaf applies to the last row and can be nil. The row at pinAt
// (0 is the listed row) must have status "ok".
//
// A forward edge (OwnFK) compares the FK column of the row with the ids
// of the next row. A reverse edge compares the id of the row with the
// FK column of the next row. Each subquery gets the row-visibility gate
// of its table.
func relationPathPredicate(edges []EdgeMetadata, pinAt int, leaf func(*sql.Selector), tier privctx.Tier) func(*sql.Selector) {
	pred := leaf
	for i := len(edges); i >= 0; i-- {
		pred = withStatusPin(pred, pinAt == i)
		if i == 0 {
			break
		}
		pred = hopPredicate(edges[i-1], pred, tier)
	}
	return pred
}

// withStatusPin adds the status "ok" test (likelyOK) to pred when pin is
// true.
func withStatusPin(pred func(*sql.Selector), pin bool) func(*sql.Selector) {
	if !pin {
		return pred
	}
	return func(s *sql.Selector) {
		if pred != nil {
			pred(s)
		}
		likelyOK(s)
	}
}

// hopPredicate returns the predicate on the row before e that keeps the
// rows whose next row through e matches inner.
func hopPredicate(e EdgeMetadata, inner func(*sql.Selector), tier privctx.Tier) func(*sql.Selector) {
	subCol, outerCol := e.ParentFKColumn, parentPKColumn
	if e.OwnFK {
		subCol, outerCol = e.TargetIDColumn, e.ParentFKColumn
	}
	return func(s *sql.Selector) {
		t := sql.Table(e.TargetTable)
		sub := sql.Select(t.C(subCol)).From(t)
		if inner != nil {
			inner(sub)
		}
		applyVisibilityGate(sub, e.TargetTable, tier)
		s.Where(sql.In(s.C(outerCol), sub))
	}
}
