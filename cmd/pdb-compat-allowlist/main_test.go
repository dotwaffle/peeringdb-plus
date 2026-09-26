package main

import (
	"go/format"
	"reflect"
	"slices"
	"strings"
	"testing"

	"entgo.io/ent/entc/gen"
	"entgo.io/ent/schema/field"

	"github.com/dotwaffle/peeringdb-plus/ent/schema"
	"github.com/dotwaffle/peeringdb-plus/internal/pdbtypes"
)

// TestBuildAllowlistEntry locks the verbatim-field-list → NodeEntry
// contract. This is the Path A ingestion point since the
// sibling-files refactor moved Path A source-of-truth from ent schema
// annotations into the hand-written schema.PrepareQueryAllows map.
// Fields are split on "__" and routed to Direct / Via (or dropped with
// a warn for 0/1/4+ segment counts — the 2-hop cap).
func TestBuildAllowlistEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pdbType string
		fields  []string
		want    *NodeEntry
	}{
		{
			name:    "direct_only",
			pdbType: "poc",
			fields:  []string{"net__name", "net__asn"},
			want: &NodeEntry{
				GoName:  "Poc",
				PDBType: "poc",
				Direct:  []string{"net__asn", "net__name"}, // sorted
			},
		},
		{
			name:    "direct_plus_via",
			pdbType: "fac",
			fields:  []string{"org__name", "ixlan__ix__fac_count"},
			want: &NodeEntry{
				GoName:  "Facility",
				PDBType: "fac",
				Direct:  []string{"org__name"},
				Via: []ViaEntry{
					{FirstHop: "ixlan", Tails: []string{"ix__fac_count"}},
				},
			},
		},
		{
			name:    "empty_fields_yields_nil",
			pdbType: "net",
			fields:  nil,
			want:    nil,
		},
		{
			name:    "empty_pdbType_yields_nil",
			pdbType: "",
			fields:  []string{"org__name"},
			want:    nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildAllowlistEntry(tc.pdbType, tc.fields)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("buildAllowlistEntry(%q, %v) =\n  %+v\nwant\n  %+v", tc.pdbType, tc.fields, got, tc.want)
			}
		})
	}
}

// TestBuildAllowlistEntry_DropsInvalidHops verifies the 0/1-segment
// (malformed) and 4+-segment (beyond the 2-hop cap) field strings are dropped —
// without affecting the valid entries alongside them. The function
// logs a warn for each but returns the remaining shape intact.
func TestBuildAllowlistEntry_DropsInvalidHops(t *testing.T) {
	t.Parallel()
	got := buildAllowlistEntry("net", []string{
		"org__name",         // valid direct
		"noSeparator",       // 1-segment — dropped
		"a__b__c__d",        // 4-segment — exceeds 2-hop cap, dropped
		"netfac__fac__name", // valid via
	})
	want := &NodeEntry{
		GoName:  "Network",
		PDBType: "net",
		Direct:  []string{"org__name"},
		Via: []ViaEntry{
			{FirstHop: "netfac", Tails: []string{"fac__name"}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildAllowlistEntry with invalid hops =\n  %+v\nwant\n  %+v", got, want)
	}
}

// TestPdbTypeFor_AllThirteen locks the ent-Go-name → pdb-type mapping
// that the traversal allowlist codegen relies on. If a future schema is
// added, the test will fail and force the author to extend
// internal/pdbtypes.All.
func TestPdbTypeFor_AllThirteen(t *testing.T) {
	t.Parallel()
	expected := map[string]string{
		"Organization":     "org",
		"Network":          "net",
		"Facility":         "fac",
		"InternetExchange": "ix",
		"Poc":              "poc",
		"IxLan":            "ixlan",
		"IxPrefix":         "ixpfx",
		"NetworkIxLan":     "netixlan",
		"NetworkFacility":  "netfac",
		"IxFacility":       "ixfac",
		"Carrier":          "carrier",
		"CarrierFacility":  "carrierfac",
		"Campus":           "campus",
	}
	for goName, wantPDB := range expected {
		if got := pdbTypeFor(goName); got != wantPDB {
			t.Errorf("pdbTypeFor(%q) = %q, want %q", goName, got, wantPDB)
		}
	}
}

// TestPdbTypeFor_UnknownReturnsEmpty documents the fallback contract:
// unknown Go names return "" (caller skips them) rather than error.
// This keeps the tool resilient if the schema grows a new type before
// internal/pdbtypes is updated.
func TestPdbTypeFor_UnknownReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := pdbTypeFor("NotARealEntity"); got != "" {
		t.Errorf("pdbTypeFor(unknown) = %q, want empty string", got)
	}
}

// TestGoNameFor_RoundTrip locks the reverse mapping — every canonical
// type must round-trip pdbTypeFor ⇌ goNameFor. Guarantees that any
// future extension of internal/pdbtypes keeps both directions
// consistent.
func TestGoNameFor_RoundTrip(t *testing.T) {
	t.Parallel()
	for _, ty := range pdbtypes.All {
		if got := goNameFor(pdbTypeFor(ty.GoName)); got != ty.GoName {
			t.Errorf("goNameFor(pdbTypeFor(%q)) = %q, want round-trip", ty.GoName, got)
		}
	}
}

// TestGoNameFor_UnknownReturnsEmpty — parallel of the pdbTypeFor
// fallback contract: unknown inputs yield an empty-string sentinel
// so the caller can skip rather than panic.
func TestGoNameFor_UnknownReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := goNameFor("not-a-real-pdb-type"); got != "" {
		t.Errorf("goNameFor(unknown) = %q, want empty string", got)
	}
}

// TestGroupExcludes_TwoEdgesOneEntity locks the duplicate-key codegen
// fix: two excluded edges on one entity must fold into a single
// ExcludeGroup (one outer map literal) — the previous one-entry-per-
// tuple template emitted duplicate map keys, i.e. generated Go that
// does not compile.
func TestGroupExcludes_TwoEdgesOneEntity(t *testing.T) {
	t.Parallel()
	got := groupExcludes([]ExcludeEntry{
		{Entity: "Network", Edge: "pocs"},
		{Entity: "Campus", Edge: "facilities"},
		{Entity: "Network", Edge: "network_ix_lans"},
	})
	want := []ExcludeGroup{
		{Entity: "Campus", Edges: []string{"facilities"}},
		{Entity: "Network", Edges: []string{"network_ix_lans", "pocs"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("groupExcludes = %+v, want %+v", got, want)
	}
}

// TestRender_TwoExcludedEdgesCompiles proves the rendered FilterExcludes
// block parses as Go when an entity carries two excluded edges — the
// go/format.Source pass inside render acts as the syntax gate (it
// rejects duplicate map keys in a literal).
func TestRender_TwoExcludedEdgesCompiles(t *testing.T) {
	t.Parallel()
	src, err := render(AllowlistData{
		FilterExcludes: groupExcludes([]ExcludeEntry{
			{Entity: "Network", Edge: "pocs"},
			{Entity: "Network", Edge: "network_ix_lans"},
		}),
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, err := format.Source(src); err != nil {
		t.Fatalf("rendered source does not parse: %v\n%s", err, src)
	}
	if !strings.Contains(string(src), `"Network": {`) {
		t.Errorf("rendered source missing grouped Network entry:\n%s", src)
	}
	if strings.Count(string(src), `"Network": {`) != 1 {
		t.Errorf("Network must appear exactly once in FilterExcludes:\n%s", src)
	}
}

// columnEdgeFixture returns literal ent graph nodes for TestAddColumnEdges:
// NetworkIxLan with one indexed nillable int column (ix_side_id), one
// nillable int column with no index (net_side_id), one int column that
// is not nillable (asn) and one string column (name), and Facility as
// the target. No graph is loaded.
func columnEdgeFixture() []*gen.Type {
	intT := &field.TypeInfo{Type: field.TypeInt}
	nix := &gen.Type{
		Name: "NetworkIxLan",
		ID:   &gen.Field{Name: "id", Type: intT},
		Fields: []*gen.Field{
			{Name: "ix_side_id", Type: intT, Nillable: true, Optional: true},
			{Name: "net_side_id", Type: intT, Nillable: true, Optional: true},
			{Name: "asn", Type: intT},
			{Name: "name", Type: &field.TypeInfo{Type: field.TypeString}, Nillable: true},
		},
		Indexes: []*gen.Index{{Columns: []string{"ix_side_id"}}},
	}
	fac := &gen.Type{Name: "Facility", ID: &gen.Field{Name: "id", Type: intT}}
	return []*gen.Type{nix, fac}
}

// netixlanEntEdges returns the rows that extractEdges emits for the ent
// edges of netixlan.
func netixlanEntEdges() []EdgeMapRow {
	return []EdgeMapRow{
		{Name: "ix_lan", TargetType: "ixlan", TraversalKey: "ixlan", ParentFKColumn: "ixlan_id", TargetTable: "ix_lans", TargetIDColumn: "id", OwnFK: true},
		{Name: "network", TargetType: "net", TraversalKey: "net", ParentFKColumn: "net_id", TargetTable: "networks", TargetIDColumn: "id", OwnFK: true},
	}
}

// cloneEntries copies entries and their row slices, so that a test can
// check that addColumnEdges leaves its input unchanged.
func cloneEntries(entries []EdgeMapEntry) []EdgeMapEntry {
	out := slices.Clone(entries)
	for i := range out {
		out[i].Edges = slices.Clone(out[i].Edges)
	}
	return out
}

// TestAddColumnEdges locks the checks and the output of the column edges
// that cmd/pdb-compat-allowlist adds from schema.ColumnEdges to the
// Path B edge map. A bad declaration must return an error (codegen
// stops) and never a partial map.
func TestAddColumnEdges(t *testing.T) {
	t.Parallel()

	ixSide := EdgeMapRow{Name: "ix_side", TargetType: "fac", TraversalKey: "ix_side", ParentFKColumn: "ix_side_id", TargetTable: "facilities", TargetIDColumn: "id", OwnFK: true}
	ixlanEntry := EdgeMapEntry{PDBType: "ixlan", Edges: []EdgeMapRow{
		{Name: "ix", TargetType: "ix", TraversalKey: "ix", ParentFKColumn: "ix_id", TargetTable: "internet_exchanges", TargetIDColumn: "id", OwnFK: true},
	}}
	nixEntry := EdgeMapEntry{PDBType: "netixlan", Edges: netixlanEntEdges()}
	decl := func(edges ...schema.ColumnEdge) map[string][]schema.ColumnEdge {
		return map[string][]schema.ColumnEdge{"netixlan": edges}
	}
	good := schema.ColumnEdge{TraversalKey: "ix_side", Column: "ix_side_id", TargetType: "fac"}

	tests := []struct {
		name    string
		entries []EdgeMapEntry
		decl    map[string][]schema.ColumnEdge
		want    []EdgeMapEntry
		wantErr string
	}{
		{
			name:    "valid_edge_sorted_between_ent_edges",
			entries: []EdgeMapEntry{ixlanEntry, nixEntry},
			decl:    decl(good),
			want: []EdgeMapEntry{ixlanEntry, {PDBType: "netixlan", Edges: []EdgeMapRow{
				netixlanEntEdges()[0], ixSide, netixlanEntEdges()[1],
			}}},
		},
		{
			name:    "source_without_entry_gets_new_entry",
			entries: []EdgeMapEntry{ixlanEntry},
			decl:    decl(good),
			want:    []EdgeMapEntry{ixlanEntry, {PDBType: "netixlan", Edges: []EdgeMapRow{ixSide}}},
		},
		{
			name:    "empty_decl_leaves_entries_unchanged",
			entries: []EdgeMapEntry{ixlanEntry, nixEntry},
			decl:    map[string][]schema.ColumnEdge{},
			want:    []EdgeMapEntry{ixlanEntry, nixEntry},
		},
		{
			name:    "unknown_column",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{TraversalKey: "ix_side", Column: "bogus_id", TargetType: "fac"}),
			wantErr: `no column "bogus_id"`,
		},
		{
			name:    "int_column_not_nillable",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{TraversalKey: "asn", Column: "asn", TargetType: "fac"}),
			wantErr: `column "asn" is not nillable`,
		},
		{
			name:    "string_column",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{TraversalKey: "name", Column: "name", TargetType: "fac"}),
			wantErr: `column "name" is not an int column`,
		},
		{
			name:    "column_without_index",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{TraversalKey: "net_side", Column: "net_side_id", TargetType: "fac"}),
			wantErr: `no index of NetworkIxLan starts with column "net_side_id"`,
		},
		{
			name:    "empty_traversal_key",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{Column: "ix_side_id", TargetType: "fac"}),
			wantErr: "empty traversal key or column",
		},
		{
			name:    "unknown_target_type",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{TraversalKey: "ix_side", Column: "ix_side_id", TargetType: "bogus"}),
			wantErr: `unknown target type "bogus"`,
		},
		{
			name:    "target_type_not_in_graph",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{TraversalKey: "ix_side", Column: "ix_side_id", TargetType: "org"}),
			wantErr: `unknown target type "org"`,
		},
		{
			name:    "source_type_not_in_graph",
			entries: []EdgeMapEntry{nixEntry},
			decl:    map[string][]schema.ColumnEdge{"org": {good}},
			wantErr: "org: unknown source type",
		},
		{
			name:    "clash_by_traversal_key",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(schema.ColumnEdge{TraversalKey: "net", Column: "ix_side_id", TargetType: "fac"}),
			wantErr: `clashes with edge "network"`,
		},
		{
			name: "clash_by_column",
			entries: []EdgeMapEntry{{PDBType: "netixlan", Edges: []EdgeMapRow{
				{Name: "other", TargetType: "fac", TraversalKey: "other", ParentFKColumn: "ix_side_id", TargetTable: "facilities", TargetIDColumn: "id", OwnFK: true},
			}}},
			decl:    decl(good),
			wantErr: `clashes with edge "other"`,
		},
		{
			name:    "clash_with_earlier_column_edge",
			entries: []EdgeMapEntry{nixEntry},
			decl:    decl(good, good),
			wantErr: `clashes with edge "ix_side"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			before := cloneEntries(tc.entries)
			got, err := addColumnEdges(columnEdgeFixture(), tc.entries, tc.decl)
			if !reflect.DeepEqual(tc.entries, before) {
				t.Errorf("addColumnEdges modified its input entries:\n  %+v\nwant\n  %+v", tc.entries, before)
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("addColumnEdges error = %v, want it to contain %q", err, tc.wantErr)
				}
				if got != nil {
					t.Errorf("addColumnEdges returned %+v with an error, want nil", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("addColumnEdges: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("addColumnEdges =\n  %+v\nwant\n  %+v", got, tc.want)
			}
		})
	}
}
