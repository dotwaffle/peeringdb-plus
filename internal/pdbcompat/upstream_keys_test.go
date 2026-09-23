package pdbcompat

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// TestUpstreamFieldName locks the port of the upstream key translation
// (2.83.0 rest.py:608-610 plus serializers.py:403-441).
func TestUpstreamFieldName(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"org":         "org",
		"org_id":      "org",
		"org_id_id":   "org",
		"fac":         "facility",
		"fac_id":      "facility",
		"facility_id": "facility",
		"net":         "network",
		"net_id":      "network",
		"network_id":  "network",
		"net_side":    "network_side",
		"net_side_id": "network_side",
		"ix_side_id":  "ix_side",
		"ix_id":       "ix",
		"fac_count":   "facility_count",
		"net_count":   "network_count",
		// Too short for ^.+[^_]_id$, or an underscore before _id.
		"a_id":  "a_id",
		"x__id": "x__id",
		// The prefix rules need a character after the underscore.
		"net_": "net_",
		"id":   "id",
	}
	for in, want := range tests {
		if got := upstreamFieldName(in); got != want {
			t.Errorf("upstreamFieldName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestRegistryForeignKeys_AlignWithEdges checks the ForeignKeys tables
// against the generated Edges map. Every value must be an int column in
// Fields. Every forward (OwnFK) edge must appear under its upstream
// name, which is the traversal key after the net/fac renames. The only
// FK without an edge is netixlan.ix_side: the mirror stores the column
// but models no edge to the facility.
func TestRegistryForeignKeys_AlignWithEdges(t *testing.T) {
	t.Parallel()
	noEdge := map[string]bool{peeringdb.TypeNetIXLan + ".ix_side": true}
	for typ, tc := range Registry {
		byCol := make(map[string]string, len(tc.ForeignKeys))
		for name, col := range tc.ForeignKeys {
			if ft, ok := tc.Fields[col]; !ok || ft != FieldInt {
				t.Errorf("%s.ForeignKeys[%q] = %q: not an int column in Fields", typ, name, col)
			}
			if _, ok := tc.Fields[name]; ok {
				t.Errorf("%s.ForeignKeys[%q] shadows a Fields key", typ, name)
			}
			byCol[col] = name
		}
		seen := map[string]bool{}
		for _, e := range Edges[typ] {
			if !e.OwnFK {
				continue
			}
			name, ok := byCol[e.ParentFKColumn]
			if !ok {
				t.Errorf("%s: forward edge %q (column %s) has no ForeignKeys entry", typ, e.TraversalKey, e.ParentFKColumn)
				continue
			}
			if want := renameNetFac(e.TraversalKey); name != want {
				t.Errorf("%s: forward edge %q is under ForeignKeys[%q], want [%q]", typ, e.TraversalKey, name, want)
			}
			seen[name] = true
		}
		for name := range tc.ForeignKeys {
			if !seen[name] && !noEdge[typ+"."+name] {
				t.Errorf("%s.ForeignKeys[%q] matches no forward edge", typ, name)
			}
		}
	}
}

// TestParseFilters_UpstreamFKKeys checks which column each upstream FK
// spelling filters, and that the net_side spellings stay unknown.
func TestParseFilters_UpstreamFKKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, key, wantCol string
	}{
		{peeringdb.TypeNet, "org", "org_id"},
		{peeringdb.TypeNet, "org__in", "org_id"},
		{peeringdb.TypeNet, "org_id_id", "org_id"},
		{peeringdb.TypeFac, "campus", "campus_id"},
		{peeringdb.TypeIXLan, "ix", "ix_id"},
		{peeringdb.TypeIXPfx, "ixlan", "ixlan_id"},
		{peeringdb.TypePoc, "net", "net_id"},
		{peeringdb.TypePoc, "network", "net_id"},
		{peeringdb.TypePoc, "network_id", "net_id"},
		{peeringdb.TypeNetFac, "fac", "fac_id"},
		{peeringdb.TypeNetFac, "facility_id", "fac_id"},
		{peeringdb.TypeIXFac, "facility__gt", "fac_id"},
		{peeringdb.TypeNetIXLan, "ix_side", "ix_side_id"},
		{peeringdb.TypeNetIXLan, "ixlan", "ixlan_id"},
		{peeringdb.TypeCarrierFac, "carrier", "carrier_id"},
		{peeringdb.TypeOrg, "org", ""},
		{peeringdb.TypeNetIXLan, "net_side", ""},
		{peeringdb.TypeNetIXLan, "net_side__in", ""},
		{peeringdb.TypeNetIXLan, "network_side_id", ""},
	}
	for _, tt := range tests {
		t.Run(tt.typ+"?"+tt.key, func(t *testing.T) {
			t.Parallel()
			ctx := WithUnknownFields(context.Background())
			preds, _, err := ParseFiltersCtx(ctx, url.Values{tt.key: {"1"}}, Registry[tt.typ])
			if err != nil {
				t.Fatalf("ParseFiltersCtx: %v", err)
			}
			if tt.wantCol == "" {
				if len(preds) != 0 || len(UnknownFieldsFromCtx(ctx)) != 1 {
					t.Fatalf("preds = %d, unknown = %v; want the key unknown", len(preds), UnknownFieldsFromCtx(ctx))
				}
				return
			}
			if len(preds) != 1 {
				t.Fatalf("preds = %d, want 1 (unknown = %v)", len(preds), UnknownFieldsFromCtx(ctx))
			}
			s := sql.Select("*").From(sql.Table("t"))
			preds[0](s)
			query, _ := s.Query()
			if want := "`t`.`" + tt.wantCol + "`"; !strings.Contains(query, want) {
				t.Errorf("query %q does not filter %s", query, want)
			}
		})
	}
}

// TestParseFilters_RelationStatusKeys checks that a relation key filters
// status only through a forward edge one hop away, as upstream
// queryable_relations (2.83.0 serializers.py:970-996).
func TestParseFilters_RelationStatusKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typ, key string
		resolves bool
	}{
		{peeringdb.TypeNet, "org__status", true},
		{peeringdb.TypeNet, "org__status__in", true},
		{peeringdb.TypePoc, "network__status", true},
		{peeringdb.TypeIXPfx, "ixlan__status", true},
		{peeringdb.TypeOrg, "net__status", false},
		{peeringdb.TypeOrg, "network__status", false},
		{peeringdb.TypeFac, "netfac__status__in", false},
		{peeringdb.TypeNetIXLan, "net__org__status", false},
		{peeringdb.TypeIXPfx, "ixlan__ix__status", false},
		// The relation keys of a prepare_query filter status as
		// upstream does (see relationSeeds).
		{peeringdb.TypeIX, "ixlan__status__in", true},
		{peeringdb.TypeNet, "netixlan__status", true},
		// Other fields on the same keys still resolve.
		{peeringdb.TypeOrg, "net__name", true},
		{peeringdb.TypeNetIXLan, "net__org__name", true},
	}
	for _, tt := range tests {
		t.Run(tt.typ+"?"+tt.key, func(t *testing.T) {
			t.Parallel()
			ctx := WithUnknownFields(context.Background())
			preds, _, err := ParseFiltersCtx(ctx, url.Values{tt.key: {"ok"}}, Registry[tt.typ])
			if err != nil {
				t.Fatalf("ParseFiltersCtx: %v", err)
			}
			if got := len(preds) == 1; got != tt.resolves {
				t.Errorf("resolves = %v, want %v (unknown = %v)", got, tt.resolves, UnknownFieldsFromCtx(ctx))
			}
		})
	}
}
