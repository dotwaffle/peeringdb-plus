package pdbcompat

import (
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/ixprefix"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// This file ports the ix key ipblock (PeeringDB 2.83.0
// InternetExchangeSerializer.prepare_query, serializers.py:4548-4552,
// and InternetExchange.related_to_ipblock, models.py:2830-2843). The
// key keeps the exchanges that have a prefix whose text starts with the
// value.

// lookupIPBlockKey reports whether key is the ipblock key of typ. Only
// the exact key applies: upstream tests `"ipblock" in kwargs`, so
// ipblock__in is an unknown key.
func lookupIPBlockKey(typ, key string) bool {
	return typ == peeringdb.TypeIX && key == "ipblock"
}

// buildIPBlockPredicate keeps the exchanges that have an ixpfx whose
// prefix text starts with value. It is a string match, not containment:
// 10.0.0.0/2 matches 10.0.0.0/24 and 10.0.0.5 does not. The comparison
// is case-sensitive and % and _ are literal, as the LIKE BINARY of
// upstream is. Neither the ixpfx nor the ixlan has a status check
// (IXLanPrefix.objects), and the exchange id is the ix_id of the joined
// ixlan (ixlan__ix_id). An empty value matches every prefix. No value
// is an error.
func buildIPBlockPredicate(value string) func(*sql.Selector) {
	return func(s *sql.Selector) {
		pfx := sql.Table(ixprefix.Table)
		lan := sql.Table(ixlan.Table)
		sub := sql.Select().From(pfx)
		// Join gives lan an alias, so lan.C must run after it.
		sub.Join(lan).On(pfx.C(ixprefix.FieldIxlanID), lan.C(ixlan.FieldID))
		sub.Select(lan.C(ixlan.FieldIxID))
		sub.Where(sql.ExprP("substr("+pfx.C(ixprefix.FieldPrefix)+", 1, length(?)) = ?", value, value))
		s.Where(sql.In(s.C(parentPKColumn), sub))
	}
}
