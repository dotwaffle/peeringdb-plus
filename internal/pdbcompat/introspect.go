package pdbcompat

// EdgeMetadata describes one ent edge for Path B traversal lookup.
// Emitted into allowlist_gen.go Edges by cmd/pdb-compat-allowlist at
// `go generate` time. See Phase 70 D-02 (amended 2026-04-19: the
// codegen-emitted static map replaces the originally-proposed runtime
// client.Schema.Tables walk. Rationale: deterministic, testable, no
// init-order coupling, matches the v1.15 Phase 63 codegen precedent;
// freshness is enforced by the existing go-generate drift-check CI
// gate).
//
// Parser-facing fields:
//   - Name: local ent edge name (e.g. "organization", "network_facilities")
//   - TargetType: PeeringDB type string of the edge target (e.g. "org")
//   - TraversalKey: the <fk> token used in filter params
//     ("?org__name=foo" → TraversalKey "org"). Equals TargetType for all
//     edges today; kept separate for future aliasing.
//   - Excluded: true when the edge has WithFilterExcludeFromTraversal.
//     LookupEdge returns (zero, false) for excluded edges.
//
// SQL-join-facing fields (consumed read-only by Plan 70-05's subquery
// construction):
//   - ParentFKColumn: foreign-key column name for this edge sourced from
//     gen.Relation.Column() at codegen time. For M2O edges (edge.From
//     with Ref().Field()) the column lives on the parent table
//     (networks.org_id); for O2M edges (edge.To) the column lives on
//     the child table (netfac.network_id). Plan 70-05 decides the
//     subquery shape based on edge direction.
//   - TargetTable: SQL table name on the target side (e.g. "organizations").
//   - TargetIDColumn: typically "id"; emitted explicitly so the codegen
//     output is self-contained and doesn't rely on a convention.
type EdgeMetadata struct {
	Name           string
	TargetType     string
	TraversalKey   string
	Excluded       bool
	ParentFKColumn string
	TargetTable    string
	TargetIDColumn string
}

// LookupEdge returns the edge metadata for a single hop from entityType
// via the filter-param token fk (e.g. LookupEdge("net", "org") returns
// the Network→Organization edge).
//
// Returns (zero, false) when:
//   - entityType is not in Edges (unknown PeeringDB type)
//   - no edge on entityType has TraversalKey == fk
//   - the matching edge is Excluded
//
// Plan 70-05's parser calls this after a param fails direct-field
// lookup and also fails Path A Allowlists lookup. When this returns
// false, the param is the unknown-filter-field case and goes through
// the silent-ignore + DEBUG log + OTel attr path per D-05.
//
// The generated Edges map is populated at package init by Go's normal
// mechanism and is safe for concurrent readers with no runtime guard —
// the codegen path makes the map effectively immutable after init.
func LookupEdge(entityType, fk string) (EdgeMetadata, bool) {
	edges, ok := Edges[entityType]
	if !ok {
		return EdgeMetadata{}, false
	}
	for _, e := range edges {
		if e.Excluded {
			continue
		}
		if e.TraversalKey == fk {
			return e, true
		}
	}
	return EdgeMetadata{}, false
}

// ResolveEdges returns all non-excluded edges for entityType. Used in
// tests and by future diagnostic endpoints; not on the request-time
// hot path. Returns nil (not a zero-length slice) for an unknown
// entityType so callers can distinguish "no such entity" from
// "entity with no traversable edges".
func ResolveEdges(entityType string) []EdgeMetadata {
	edges, ok := Edges[entityType]
	if !ok {
		return nil
	}
	out := make([]EdgeMetadata, 0, len(edges))
	for _, e := range edges {
		if e.Excluded {
			continue
		}
		out = append(out, e)
	}
	return out
}

// TargetFields returns the filterable field set on the target entity of
// an edge lookup. Parser uses this to validate the <field> portion of
// <fk>__<field> before translating to a predicate in Plan 70-05.
//
// Consults Registry[targetType].Fields — the existing TypeConfig map.
// Nil-safe: returns nil for an unknown target.
func TargetFields(targetType string) map[string]FieldType {
	tc, ok := Registry[targetType]
	if !ok {
		return nil
	}
	return tc.Fields
}
