package pdbcompat

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// TestRelationSeeds_EdgesResolve checks every relation seed against the
// generated Edges map and the Registry: each hop must be an edge of the
// row before it, the pinned row must be on the path, and a fixed field
// must be a model field of the last row.
func TestRelationSeeds_EdgesResolve(t *testing.T) {
	t.Parallel()
	for typ, seeds := range relationSeeds {
		if _, ok := Registry[typ]; !ok {
			t.Errorf("relationSeeds[%q]: not a Registry type", typ)
			continue
		}
		for name, sd := range seeds {
			rowType := typ
			for _, hop := range sd.hops {
				e, ok := LookupEdge(rowType, hop)
				if !ok {
					t.Errorf("%s?%s: no edge %q on %s", typ, name, hop, rowType)
					break
				}
				rowType = e.TargetType
			}
			if sd.pinAt != noPin && (sd.pinAt < 0 || sd.pinAt > len(sd.hops)) {
				t.Errorf("%s?%s: pinAt %d is not a row of a %d-hop path", typ, name, sd.pinAt, len(sd.hops))
			}
			if sd.field != "" {
				leaf := Registry[rowType]
				if _, _, ok := resolveLocalField(leaf, sd.field); !ok || leaf.NonModelFields[sd.field] {
					t.Errorf("%s?%s: field %q is not a model field of %s", typ, name, sd.field, rowType)
				}
			}
			if (sd.shape == shapeRelation) != (sd.field == "") {
				t.Errorf("%s?%s: a shapeRelation seed has no fixed field, and every other shape has one", typ, name)
			}
		}
	}
}

// TestRelationSeed_ParseTail locks how the key segments after a seed
// name map onto a field and an operator (2.83.0 serializers.py:614-656).
func TestRelationSeed_ParseTail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, key  string
		wantField string
		wantOp    string
		wantOK    bool
	}{
		{peeringdb.TypeNet, "ix", "id", "", true},
		{peeringdb.TypeNet, "ix_id__in", "id", "in", true},
		{peeringdb.TypeNet, "ix__name", "name", "", true},
		{peeringdb.TypeNet, "ix__name__contains", "name", "contains", true},
		// A second segment that is not an operator is dropped.
		{peeringdb.TypeNet, "netfac__fac__name", "facility", "", true},
		{peeringdb.TypeIX, "ixfac__fac_id", "facility", "", true},
		{peeringdb.TypeNet, "ix__org__name__x", "", "", false},
		{peeringdb.TypeNetIXLan, "name", "name", "", true},
		{peeringdb.TypeNetIXLan, "name__contains", "name", "contains", true},
		{peeringdb.TypeNetIXLan, "name__x", "", "", false},
		// get_relation_filters keeps the whole key, and related_to_name
		// applies the lookup to the ixlan name.
		{peeringdb.TypeNetIXLan, "name__iexact", "name", "iexact", true},
		{peeringdb.TypeNetIXLan, "name__icontains__x", "", "", false},
		{peeringdb.TypeFac, "org_name", "name", "icontains", true},
		{peeringdb.TypeFac, "org_name__in", "name", "in", true},
		{peeringdb.TypeFac, "org_name__x", "", "", false},
		// The prepare_query ignores org_name with a suffix that
		// get_relation_filters does not parse.
		{peeringdb.TypeFac, "org_name__iexact", "", "", false},
		// netfac and ixfac replace the key with the fixed field, so a
		// segment that is not an operator has no effect.
		{peeringdb.TypeNetFac, "city", "city", "", true},
		{peeringdb.TypeNetFac, "name__x", "name", "", true},
		{peeringdb.TypeIXFac, "name__x__contains", "name", "contains", true},
		{peeringdb.TypeIXFac, "country__in", "country", "in", true},
		{peeringdb.TypeNetFac, "name__a__b__c", "", "", false},
		{peeringdb.TypeCampus, "facility", "id", "", true},
		{peeringdb.TypeCampus, "facility__name__contains", "name", "contains", true},
		{peeringdb.TypeOrg, "asn", "asn", "", true},
		{peeringdb.TypeOrg, "asn__in", "", "", false},
		{peeringdb.TypeCarrier, "carrierfac_set__facility_id", "fac_id", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.typ+"?"+tt.key, func(t *testing.T) {
			t.Parallel()
			sd, tail, ok := lookupRelationSeed(tt.typ, tt.key)
			if !ok {
				t.Fatalf("lookupRelationSeed(%s, %q): not a seed", tt.typ, tt.key)
			}
			field, op, ok := sd.parseTail(tail)
			if field != tt.wantField || op != tt.wantOp || ok != tt.wantOK {
				t.Errorf("parseTail = (%q, %q, %v), want (%q, %q, %v)",
					field, op, ok, tt.wantField, tt.wantOp, tt.wantOK)
			}
		})
	}
}

// TestLookupRelationSeed_NotSeeds lists keys that name no relation seed
// upstream, next to the seeds they resemble.
func TestLookupRelationSeed_NotSeeds(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ typ, key string }{
		// The campus seed list names only facility (serializers.py:4861).
		{peeringdb.TypeCampus, "facility_id"},
		// Three segments do not match the carrier seed
		// (serializers.py:2722-2729).
		{peeringdb.TypeCarrier, "carrierfac_set__facility_id__in"},
		{peeringdb.TypeCarrier, "carrierfac_set"},
		{peeringdb.TypeNetFac, "name_id"},
		{peeringdb.TypeFac, "org_name_id"},
		{peeringdb.TypeNet, "ix_count"},
	} {
		if _, _, ok := lookupRelationSeed(tt.typ, tt.key); ok {
			t.Errorf("lookupRelationSeed(%s, %q) found a seed, want none", tt.typ, tt.key)
		}
	}
}

// TestParseFilters_RelationSeedSQL checks the SQL shape of relation
// keys: the path of nested IN subqueries, the row pinned to status ok
// (likelyOK), and the FK compare that skips the last row of a bare key.
func TestParseFilters_RelationSeedSQL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, query string
		want       []string
	}{
		{
			peeringdb.TypeFac, "net=100",
			[]string{"`facilities`.`id` IN (SELECT `network_facilities`.`fac_id` FROM `network_facilities` WHERE `network_facilities`.`net_id` = ? AND likely(`network_facilities`.`status` IN (?)))"},
		},
		{
			peeringdb.TypeNet, "ix__name=X",
			[]string{"`networks`.`id` IN (SELECT `network_ix_lans`.`net_id` FROM `network_ix_lans` WHERE `network_ix_lans`.`ixlan_id` IN (SELECT `ix_lans`.`id` FROM `ix_lans` WHERE `ix_lans`.`ix_id` IN (SELECT `internet_exchanges`.`id` FROM `internet_exchanges` WHERE", "AND likely(`network_ix_lans`.`status` IN (?)))"},
		},
		{
			peeringdb.TypeIXPfx, "ix=20",
			[]string{"`ix_prefixes`.`ixlan_id` IN (SELECT `ix_lans`.`id` FROM `ix_lans` WHERE `ix_lans`.`ix_id` = ?)", "likely(`ix_prefixes`.`status` IN (?))"},
		},
		{
			peeringdb.TypeOrg, "asn=64500",
			[]string{"`organizations`.`id` IN (SELECT `networks`.`org_id` FROM `networks` WHERE `networks`.`asn` = ? AND likely(`networks`.`status` IN (?)))"},
		},
		{
			peeringdb.TypeCarrier, "carrierfac_set__facility_id=400",
			[]string{"`carriers`.`id` IN (SELECT `carrier_facilities`.`carrier_id` FROM `carrier_facilities` WHERE `carrier_facilities`.`fac_id` = ?)"},
		},
		{
			peeringdb.TypeNetIXLan, "name__iexact=LanA",
			[]string{"`network_ix_lans`.`ixlan_id` IN (SELECT `ix_lans`.`id` FROM `ix_lans` WHERE", "likely(`ix_lans`.`status` IN (?)))"},
		},
		{
			peeringdb.TypeFac, "org_name=Equinix",
			[]string{"`facilities`.`org_id` IN (SELECT `organizations`.`id` FROM `organizations` WHERE LOWER(`organizations`.`name_fold`) LIKE ?)"},
		},
		{
			peeringdb.TypeNetFac, "city=Berlin",
			[]string{"`network_facilities`.`fac_id` IN (SELECT `facilities`.`id` FROM `facilities` WHERE LOWER(`facilities`.`city_fold`) = ?)", "likely(`network_facilities`.`status` IN (?))"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.typ+"?"+tt.query, func(t *testing.T) {
			t.Parallel()
			params, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}
			preds, empty, err := ParseFiltersCtx(WithUnknownFields(context.Background()), params, Registry[tt.typ])
			if err != nil || empty || len(preds) != 1 {
				t.Fatalf("ParseFiltersCtx = %d preds, empty=%v, err=%v; want 1 pred", len(preds), empty, err)
			}
			s := sql.Select("*").From(sql.Table(tableFor(t, tt.typ)))
			preds[0](s)
			got, _ := s.Query()
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("SQL %q\ndoes not contain %q", got, w)
				}
			}
		})
	}
}

// tableFor returns the SQL table of typ from any edge that targets it.
func tableFor(t *testing.T, typ string) string {
	t.Helper()
	for _, edges := range Edges {
		i := slices.IndexFunc(edges, func(e EdgeMetadata) bool { return e.TargetType == typ })
		if i >= 0 {
			return edges[i].TargetTable
		}
	}
	t.Fatalf("no edge targets %s", typ)
	return ""
}
