package pdbcompat

import (
	"testing"
)

// TestLookupEdge_AllThirteenEntitiesCovered asserts every PeeringDB
// type string has a codegen-emitted entry in Edges. This regression-
// guards against the codegen tool silently dropping an entity.
func TestLookupEdge_AllThirteenEntitiesCovered(t *testing.T) {
	t.Parallel()
	types := []string{
		"org", "net", "fac", "ix", "poc", "ixlan", "ixpfx",
		"netixlan", "netfac", "ixfac", "carrier", "carrierfac", "campus",
	}
	for _, typ := range types {
		if _, ok := Edges[typ]; !ok {
			t.Errorf("type %q: missing from Edges map — codegen did not emit entry", typ)
		}
	}
}

// TestLookupEdge_KnownHops locks a handful of well-known hops so a
// future schema change that silently breaks Path B resolution fails
// loudly here rather than at request time.
func TestLookupEdge_KnownHops(t *testing.T) {
	t.Parallel()
	tests := []struct {
		entity        string
		fk            string
		wantFound     bool
		wantTarget    string
		wantParentFK  string // checked when non-empty
		wantTargetTbl string // checked when non-empty
	}{
		// Network has an organization edge → org; M2O with FK on parent.
		{entity: "net", fk: "org", wantFound: true, wantTarget: "org", wantParentFK: "org_id", wantTargetTbl: "organizations"},
		// Facility has an organization edge → org.
		{entity: "fac", fk: "org", wantFound: true, wantTarget: "org"},
		// Facility has a campus edge → campus.
		{entity: "fac", fk: "campus", wantFound: true, wantTarget: "campus"},
		// Unknown entity (not in Edges at all).
		{entity: "bogus", fk: "org", wantFound: false},
		// Unknown FK on a known entity.
		{entity: "net", fk: "bogus", wantFound: false},
	}
	for _, tc := range tests {
		t.Run(tc.entity+"__"+tc.fk, func(t *testing.T) {
			t.Parallel()
			got, ok := LookupEdge(tc.entity, tc.fk)
			if ok != tc.wantFound {
				t.Errorf("LookupEdge(%q, %q) found=%v, want %v (got=%+v)", tc.entity, tc.fk, ok, tc.wantFound, got)
			}
			if ok && got.TargetType != tc.wantTarget {
				t.Errorf("LookupEdge(%q, %q) target=%q, want %q", tc.entity, tc.fk, got.TargetType, tc.wantTarget)
			}
			if ok && tc.wantParentFK != "" && got.ParentFKColumn != tc.wantParentFK {
				t.Errorf("LookupEdge(%q, %q) ParentFKColumn=%q, want %q", tc.entity, tc.fk, got.ParentFKColumn, tc.wantParentFK)
			}
			if ok && tc.wantTargetTbl != "" && got.TargetTable != tc.wantTargetTbl {
				t.Errorf("LookupEdge(%q, %q) TargetTable=%q, want %q", tc.entity, tc.fk, got.TargetTable, tc.wantTargetTbl)
			}
		})
	}
}

// TestLookupEdge_SQLMetadataPopulated asserts that every non-excluded
// edge in Edges carries non-empty ParentFKColumn, TargetTable, and
// TargetIDColumn. Plan 70-05's join construction assumes these are
// populated; the codegen tool must either emit them or skip the edge
// entirely.
func TestLookupEdge_SQLMetadataPopulated(t *testing.T) {
	t.Parallel()
	for typ, edges := range Edges {
		for _, e := range edges {
			if e.Excluded {
				continue
			}
			if e.ParentFKColumn == "" {
				t.Errorf("%s→%s (%s): ParentFKColumn empty", typ, e.TargetType, e.Name)
			}
			if e.TargetTable == "" {
				t.Errorf("%s→%s (%s): TargetTable empty", typ, e.TargetType, e.Name)
			}
			if e.TargetIDColumn == "" {
				t.Errorf("%s→%s (%s): TargetIDColumn empty", typ, e.TargetType, e.Name)
			}
		}
	}
}

// TestLookupEdge_RespectsExclusion builds a synthetic Edges entry with
// an excluded edge and asserts LookupEdge skips it. Locks the contract
// that a future WithFilterExcludeFromTraversal annotation on an ent
// edge will propagate through codegen to runtime filtering.
//
// Not t.Parallel — mutates the package-level Edges map.
func TestLookupEdge_RespectsExclusion(t *testing.T) {
	saved, had := Edges["testexcl"]
	t.Cleanup(func() {
		if had {
			Edges["testexcl"] = saved
		} else {
			delete(Edges, "testexcl")
		}
	})
	Edges["testexcl"] = []EdgeMetadata{
		{Name: "allowed_edge", TargetType: "org", TraversalKey: "org", Excluded: false, ParentFKColumn: "org_id", TargetTable: "organizations", TargetIDColumn: "id"},
		{Name: "blocked_edge", TargetType: "fac", TraversalKey: "fac", Excluded: true, ParentFKColumn: "fac_id", TargetTable: "facilities", TargetIDColumn: "id"},
	}
	if _, ok := LookupEdge("testexcl", "org"); !ok {
		t.Error("expected allowed edge to resolve")
	}
	if got, ok := LookupEdge("testexcl", "fac"); ok {
		t.Errorf("expected excluded edge to be hidden, got %+v", got)
	}
	edges := ResolveEdges("testexcl")
	if len(edges) != 1 {
		t.Errorf("ResolveEdges len = %d, want 1 (excluded edge should be filtered)", len(edges))
	}
}

// TestResolveEdges_UnknownType returns nil for a type not in Edges.
func TestResolveEdges_UnknownType(t *testing.T) {
	t.Parallel()
	if got := ResolveEdges("bogus"); got != nil {
		t.Errorf("ResolveEdges(\"bogus\") = %v, want nil", got)
	}
}

// TestTargetFields_KnownAndUnknown validates the Registry lookup
// wrapper — known type returns a non-nil Fields map with expected
// entries; unknown type returns nil.
func TestTargetFields_KnownAndUnknown(t *testing.T) {
	t.Parallel()
	got := TargetFields("org")
	if got == nil {
		t.Fatal("TargetFields(\"org\") = nil, want non-nil map")
	}
	if got["name"] != FieldString {
		t.Errorf("TargetFields(\"org\")[\"name\"] = %v, want FieldString", got["name"])
	}
	if nilGot := TargetFields("bogus"); nilGot != nil {
		t.Errorf("TargetFields(\"bogus\") = %+v, want nil", nilGot)
	}
}
