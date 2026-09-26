package pdbcompat

import (
	"encoding/json"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the ix key capacity (PeeringDB 2.83.0
// InternetExchangeSerializer.prepare_query, serializers.py:4503-4546,
// and InternetExchange.filter_capacity, models.py:2895-2942). The
// capacity of an exchange is the sum of the speed of its netixlans that
// are not deleted.

// lookupCapacityFilter reports whether key is a capacity key of typ, and
// returns its operator ("" for the bare key). Only ix has the key. The
// operators are those of get_relation_filters (serializers.py:617), the
// metaOperators set. With another suffix upstream stores the key under
// another name, and prepare_query ignores it, so it is not a key here.
func lookupCapacityFilter(typ, key string) (op string, ok bool) {
	if typ != peeringdb.TypeIX {
		return "", false
	}
	if key == "capacity" {
		return "", true
	}
	op, found := strings.CutPrefix(key, "capacity__")
	if !found || !metaOperators[op] {
		return "", false
	}
	return op, true
}

// buildCapacityPredicate keeps the exchanges whose capacity matches op
// and value. The exact form, the comparisons and each item of __in must
// be an integer, as Django IntegerField.get_prep_value converts them
// with int() (db/models/fields/__init__.py:2123-2132); else the error is
// a 400 (rest.py:493-500). So capacity__in= is an error, not an empty
// result. A value outside the int range saturates, which gives the rows
// of Django's IntegerFieldOverflow rules (lookups.py:461-515) for every
// stored sum. contains and startswith do not convert the value
// (PatternLookup.prepare_rhs is False) and match the decimal text of the
// sum.
func buildCapacityPredicate(op, value string) (func(*sql.Selector), error) {
	switch op {
	case "":
		n, err := capacityInt(value)
		if err != nil {
			return nil, err
		}
		return capacityWhere("%s = ?", n), nil
	case "lt", "lte", "gt", "gte":
		n, err := capacityInt(value)
		if err != nil {
			return nil, err
		}
		return capacityWhere("%s "+metaComparisons[op]+" ?", n), nil
	case "in":
		// Upstream splits the value on commas and converts each item.
		items := strings.Split(value, ",")
		ns := make([]int, len(items))
		for i, item := range items {
			n, err := capacityInt(item)
			if err != nil {
				return nil, err
			}
			ns[i] = n
		}
		list, err := json.Marshal(ns)
		if err != nil {
			return nil, fmt.Errorf("marshal capacity values: %w", err)
		}
		// The values bind as one JSON array, so a long list adds no SQL
		// term per item.
		return capacityWhere("%s IN (SELECT value FROM json_each(?))", string(list)), nil
	case "contains":
		return capacityWhere(`CAST(%s AS TEXT) LIKE ? ESCAPE '\'`, "%"+likeEscape(value)+"%"), nil
	case "startswith":
		return capacityWhere(`CAST(%s AS TEXT) LIKE ? ESCAPE '\'`, likeEscape(value)+"%"), nil
	}
	return nil, fmt.Errorf("unsupported operator %q", op)
}

// capacityInt converts one capacity value with Python int() rules.
func capacityInt(s string) (int, error) {
	n, _, err := pyInt(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not an integer", s)
	}
	return n, nil
}

// capacityWhere keeps the exchanges whose capacity meets the HAVING
// condition cond, which holds one %s for the sum and one ? for arg. It
// mirrors filter_capacity (models.py:2926-2941): the sum of speed over
// the netixlans that are not deleted (handleref undeleted()), grouped
// by ixlan_id, which upstream takes as the exchange id. There is no join
// to the ixlan, and no status check on the ixlan or the network. The
// subquery is not correlated, so SQLite runs it once per statement.
func capacityWhere(cond string, arg any) func(*sql.Selector) {
	return func(s *sql.Selector) {
		nixl := sql.Table(networkixlan.Table)
		sum := "SUM(" + nixl.C(networkixlan.FieldSpeed) + ")"
		sub := sql.Select(nixl.C(networkixlan.FieldIxlanID)).From(nixl).
			Where(sql.NEQ(nixl.C(networkixlan.FieldStatus), "deleted")).
			GroupBy(nixl.C(networkixlan.FieldIxlanID)).
			Having(sql.ExprP(fmt.Sprintf(cond, sum), arg))
		s.Where(sql.In(s.C(parentPKColumn), sub))
	}
}
