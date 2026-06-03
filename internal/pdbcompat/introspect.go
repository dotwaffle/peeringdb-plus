package pdbcompat

// EdgeMetadata describes one ent edge for Path B traversal lookup.
// Emitted into allowlist_gen.go Edges by cmd/pdb-compat-allowlist at
// `go generate` time. The codegen-emitted static map is used instead of
// a runtime client.Schema.Tables walk: it is deterministic, testable,
// has no init-order coupling, and matches the v1.15 codegen precedent;
// freshness is enforced by the existing go-generate drift-check CI
// gate.
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
// SQL-join-facing fields (consumed read-only by the traversal subquery
// construction):
//   - ParentFKColumn: foreign-key column name for this edge sourced from
//     gen.Relation.Column() at codegen time. For M2O edges (edge.From
//     with Ref().Field()) the column lives on the parent table
//     (networks.org_id); for O2M edges (edge.To) the column lives on
//     the child table (pocs.net_id). The subquery shape depends on
//     which side of the join owns the column — see OwnFK below.
//   - OwnFK: true when ParentFKColumn lives on the PARENT table (M2O /
//     O2O-from-inverse); false when it lives on the CHILD / target
//     table (O2M / O2O-from-edge). Populated at codegen time from
//     gen.Edge.Rel.Type (M2O → true; O2M → false). Used by
//     buildSinglHop / buildTwoHop to decide whether to filter the
//     parent's PK against a `SELECT child.<fk>` subquery (O2M) or the
//     parent's FK column against a `SELECT target.<id>` subquery (M2O).
//     Without this flag, O2M edges like net→pocs produce invalid SQL
//     referencing a non-existent networks.net_id column.
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
	OwnFK          bool
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
// The traversal parser calls this after a param fails direct-field
// lookup and also fails Path A Allowlists lookup. When this returns
// false, the param is the unknown-filter-field case and goes through
// the silent-ignore + DEBUG log + OTel attr path.
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
// <fk>__<field> before translating to a predicate.
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
