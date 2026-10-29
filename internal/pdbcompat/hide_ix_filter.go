package pdbcompat

import (
	"net/url"
	"strings"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/networkixlan"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// hideIXNoFac ports IXFilterMixin (2.84.0 rest.py:1303-1330). It reads
// stored counts and does not change API cache eligibility. Upstream's
// cache redirect occurs before the mixin, so cached lists bypass it.
func hideIXNoFac(typ string, params url.Values) (func(*sql.Selector), bool) {
	switch typ {
	case peeringdb.TypeIX, peeringdb.TypeIXLan, peeringdb.TypeNet, peeringdb.TypeNetIXLan:
	default:
		return nil, false
	}
	values := params["hide_ix_no_fac"]
	if len(values) == 0 {
		return nil, true
	}
	value := strings.ToLower(values[len(values)-1])
	if value != "true" && value != "1" {
		return nil, true
	}
	return func(s *sql.Selector) {
		switch typ {
		case peeringdb.TypeIX:
			s.Where(sql.GT(s.C(internetexchange.FieldFacCount), 0))
		case peeringdb.TypeNet:
			s.Where(sql.GT(s.C(network.FieldIxCount), 0))
		case peeringdb.TypeIXLan, peeringdb.TypeNetIXLan:
			ix := sql.Table(internetexchange.Table)
			exchanges := sql.Select(ix.C(internetexchange.FieldID)).From(ix).
				Where(sql.GT(ix.C(internetexchange.FieldFacCount), 0))
			if typ == peeringdb.TypeIXLan {
				s.Where(sql.In(s.C(ixlan.FieldIxID), exchanges))
				return
			}
			lan := sql.Table(ixlan.Table)
			lans := sql.Select(lan.C(ixlan.FieldID)).From(lan).
				Where(sql.In(lan.C(ixlan.FieldIxID), exchanges))
			s.Where(sql.In(s.C(networkixlan.FieldIxlanID), lans))
		}
	}, true
}
