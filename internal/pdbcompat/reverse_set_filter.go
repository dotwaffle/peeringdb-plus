package pdbcompat

import (
	"errors"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/campus"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/poc"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
)

// reverseSet maps an upstream related_name to the child table and its
// foreign key. Related rows have no implicit status filter.
type reverseSet struct {
	// name is the upstream related_name.
	name string
	// target is the PeeringDB type of the related rows.
	target string
	// table is the table of the related rows.
	table string
	// column is the FK column of table that holds the id of the listed
	// row.
	column string
}

// reverseSets maps a PeeringDB type to its reverse relation keys.
var reverseSets = map[string][]reverseSet{
	peeringdb.TypeOrg: {
		{"ix_set", peeringdb.TypeIX, internetexchange.Table, internetexchange.FieldOrgID},
		{"carrier_set", peeringdb.TypeCarrier, carrier.Table, carrier.FieldOrgID},
		{"campus_set", peeringdb.TypeCampus, campus.Table, campus.FieldOrgID},
	},
	peeringdb.TypeIX: {
		{"ixlan_set", peeringdb.TypeIXLan, ixlan.Table, ixlan.FieldIxID},
		{"ixfac_set", peeringdb.TypeIXFac, ixfacility.Table, ixfacility.FieldIxID},
	},
	peeringdb.TypeIXLan: {
		{"netixlan_set", peeringdb.TypeNetIXLan, networkixlan.Table, networkixlan.FieldIxlanID},
		{"ixpfx_set", peeringdb.TypeIXPfx, ixprefix.Table, ixprefix.FieldIxlanID},
	},
	peeringdb.TypeNet: {
		{"netfac_set", peeringdb.TypeNetFac, networkfacility.Table, networkfacility.FieldNetID},
		{"netixlan_set", peeringdb.TypeNetIXLan, networkixlan.Table, networkixlan.FieldNetID},
		{"poc_set", peeringdb.TypePoc, poc.Table, poc.FieldNetID},
	},
	peeringdb.TypeFac: {
		{"netfac_set", peeringdb.TypeNetFac, networkfacility.Table, networkfacility.FieldFacID},
		{"ixfac_set", peeringdb.TypeIXFac, ixfacility.Table, ixfacility.FieldFacID},
		{"carrierfac_set", peeringdb.TypeCarrierFac, carrierfacility.Table, carrierfacility.FieldFacID},
		{"ix_side_set", peeringdb.TypeNetIXLan, networkixlan.Table, networkixlan.FieldIxSideID},
	},
	peeringdb.TypeCarrier: {
		{"carrierfac_set", peeringdb.TypeCarrierFac, carrierfacility.Table, carrierfacility.FieldCarrierID},
	},
}

// lookupReverseSetKey returns the reverse relation of typ that key
// names, the field of the related rows ("" for the relation itself) and
// the operator. It translates the key as the upstream filter loop does
// (2.83.0 rest.py:608-632): strip one "_id" from the key, split off an
// operator of the loop, then run queryable_field_xl on the rest, which
// can strip one more "_id" (and the operator path one more after that).
// A reverse relation reports the ForeignKey type, so field_names holds
// the related name, and queryable_relations adds
// <related_name>__<field> for each field of the related model that is
// not a ForeignKey (serializers.py:970-996). ok is false for a key that
// names neither.
func lookupReverseSetKey(typ, key string) (rs reverseSet, field, op string, ok bool) {
	sets, found := reverseSets[typ]
	if !found {
		return reverseSet{}, "", "", false
	}
	k := stripIDSuffix(key)
	if i := strings.LastIndex(k, "__"); i > 0 && metaOperators[k[i+2:]] {
		op = k[i+2:]
		k = stripIDSuffix(queryableFieldXL(k[:i]))
	} else {
		k = queryableFieldXL(k)
	}
	for _, rs := range sets {
		rest, found := strings.CutPrefix(k, rs.name)
		switch {
		case !found:
			continue
		case rest == "":
			return rs, "", op, true
		case strings.HasPrefix(rest, "__"):
			field = rest[2:]
			if field == "" || strings.Contains(field, "__") {
				return reverseSet{}, "", "", false
			}
			return rs, field, op, true
		}
	}
	return reverseSet{}, "", "", false
}

// buildReverseSetPredicate builds the filter of a reverse relation key:
// the listed rows that have at least one related row that matches. The
// related rows get no status filter (rest.py:683-704). Contact row
// visibility still applies, as on other mirror traversals. With no field, the relation takes the
// operators of a Django relation only (ForeignObject lookups,
// django/db/models/fields/related.py:949-955), which compare the id of
// the related row. The key without an operator becomes an exact lookup
// on <related_name>_id (rest.py:676-677), which Django cannot resolve,
// and contains and startswith become icontains and istartswith
// (:657-662), which a relation does not have. Both raise FieldError,
// the 400 "Invalid query" (:702-703). With a field, the key filters the
// field as a model field of the related type. ok is false for a field
// that the mirror does not filter; empty is true for an __in filter
// that can match no row.
func buildReverseSetPredicate(rs reverseSet, field, op, value string, tier privctx.Tier) (p func(*sql.Selector), ok, empty bool, err error) {
	var inner func(*sql.Selector)
	if field == "" {
		switch op {
		case "", "contains", "startswith":
			return nil, false, false, errInvalidQuery
		}
		inner, err = buildModelFieldPredicate(parentPKColumn, op, value, FieldInt, false, true)
	} else {
		ft, isField := traversalTargetField(Registry[rs.target], field)
		if !isField {
			return nil, false, false, nil
		}
		inner, err = buildModelFieldPredicate(field, op, value, ft, false, false)
	}
	if errors.Is(err, errEmptyIn) {
		return nil, false, true, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	return func(s *sql.Selector) {
		t := sql.Table(rs.table)
		sel := sql.Select(t.C(rs.column)).From(t)
		inner(sel)
		applyVisibilityGate(sel, rs.table, tier)
		s.Where(sql.In(s.C(parentPKColumn), sel))
	}, true, false, nil
}
