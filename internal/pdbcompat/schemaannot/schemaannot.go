// Package schemaannot exposes ent schema annotation types consumed by
// ent/schema/*.go to describe pdbcompat's Path A allowlist (Phase 70 D-01)
// and per-edge FILTER_EXCLUDE (D-03). These types are split out of the
// parent internal/pdbcompat package so that ent/schema files can import
// them without creating an import cycle at test-compile time
// (pdbcompat → ent; ent/runtime → ent/schema; ent/schema → pdbcompat would
// cycle).
//
// The codegen tool cmd/pdb-compat-allowlist reads annotations by NAME
// string (not by Go type), so relocating the types here is transparent to
// the codegen pipeline. The parent pdbcompat package re-exports the
// constructors for existing callers.
package schemaannot

// PrepareQueryAllowAnnotation is the ent schema.Annotation form of
// upstream PeeringDB's per-serializer prepare_query allowlist (Phase 70 D-01).
// Attached to a schema (node-level), its Fields slice enumerates the
// filter keys that the pdbcompat parser will resolve via Path A. The
// codegen tool cmd/pdb-compat-allowlist reads this annotation at
// gen.Graph walk time and emits internal/pdbcompat/allowlist_gen.go.
//
// Fields are stored verbatim (e.g. "org__name", "ixlan__ix__fac_count")
// — the codegen tool splits on "__" when populating AllowlistEntry.Via.
type PrepareQueryAllowAnnotation struct {
	Fields []string
}

// Name returns the stable annotation name consulted by the codegen tool
// when walking gen.Graph. Do NOT rename — grep-ability across ent
// schema files, cmd/pdb-compat-allowlist, and generated output is
// load-bearing.
func (PrepareQueryAllowAnnotation) Name() string { return "PrepareQueryAllow" }

// WithPrepareQueryAllow is the ent-schema-facing constructor. Use in
// schema Annotations() slices, e.g.
//
//	func (Network) Annotations() []schema.Annotation {
//	    return []schema.Annotation{
//	        schemaannot.WithPrepareQueryAllow(
//	            "org__name",       // upstream serializers.py:2732
//	            "ixlan__ix__fac_count",
//	        ),
//	        // ...existing annotations...
//	    }
//	}
func WithPrepareQueryAllow(fields ...string) PrepareQueryAllowAnnotation {
	// Copy to avoid aliasing the caller's slice.
	out := make([]string, len(fields))
	copy(out, fields)
	return PrepareQueryAllowAnnotation{Fields: out}
}

// FilterExcludeFromTraversalAnnotation is the ent-edge-level form of
// upstream serializers.py:128-157's FILTER_EXCLUDE list (Phase 70 D-03).
// Attached to an edge (e.g. network.social_media), it signals the
// codegen tool to mark the edge as un-traversable — Path B
// introspection will skip it regardless of whether it matches the
// <fk>__<field> shape.
//
// No fields: presence is the signal.
type FilterExcludeFromTraversalAnnotation struct{}

// Name returns the stable annotation name. Do NOT rename.
func (FilterExcludeFromTraversalAnnotation) Name() string {
	return "FilterExcludeFromTraversal"
}

// WithFilterExcludeFromTraversal is the ent-schema-facing constructor.
// Attach to an edge to exclude it from Path B traversal, e.g.
//
//	edge.To("pocs", Poc.Type).
//	    Annotations(schemaannot.WithFilterExcludeFromTraversal()),
func WithFilterExcludeFromTraversal() FilterExcludeFromTraversalAnnotation {
	return FilterExcludeFromTraversalAnnotation{}
}
