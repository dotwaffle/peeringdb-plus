// Package schematypes hosts Go value types referenced by ent schemas
// via field.JSON. The type lives outside of ent/schema so that the
// generated ent/*.go code — which re-imports the struct as the
// materialised JSON field type — does not pull the ent/schema package
// into ent/*'s import graph.
//
// Why split: enabling gen.FeaturePrivacy in ent/entc.go (Phase 59
// VIS-04) lets a schema Policy() refer to generated where-predicates
// like poc.VisibleEQ and the ent/privacy adapter. Those live under
// ent/poc and ent/privacy, which already depend on ent/. If ent/schema
// continues to be both "the place that defines field value types" and
// "the place that defines schema Policy()", the Policy() body's import
// of ent/poc triggers a cycle: ent/poc → ent (generated) → ent/schema
// (SocialMedia) → (Policy imports) → ent/poc.
//
// By lifting plain value types (no ent tagging, no ent-schema dependency)
// into this sibling package, the cycle is broken: ent/schema can
// continue to import ent/poc/ent/privacy/internal/privctx for the
// Policy(), while ent/* keeps referencing ent/schematypes only for
// its field value type. The package name mirrors the Go convention of
// keeping pure data types in leaf packages (e.g. net/url, time).
package schematypes

// SocialMedia represents a social media link from PeeringDB.
// Used by Organization, Network, Facility, InternetExchange, Carrier,
// and Campus schemas as the element type of the social_media JSON field.
type SocialMedia struct {
	Service    string `json:"service"`
	Identifier string `json:"identifier"`
}
