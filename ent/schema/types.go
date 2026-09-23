// Package schema defines the entgo schema types for PeeringDB objects.
package schema

import "github.com/ogen-go/ogen"

// The SocialMedia value type moved to ent/schematypes to
// break an import cycle introduced when poc.go's Policy() started
// importing ent/poc for generated where-predicates. See
// ent/schematypes/schematypes.go for the full rationale.

// socialMediaSchema returns the OpenAPI schema for the social_media JSON field.
// Required because entrest cannot auto-infer the schema for custom struct types.
func socialMediaSchema() *ogen.Schema {
	return ogen.NewSchema().SetType("array").SetItems(
		ogen.NewSchema().SetType("object").
			AddRequiredProperties(
				ogen.String().ToProperty("service"),
				ogen.String().ToProperty("identifier"),
			),
	)
}

// jsonObjectSchema returns the OpenAPI schema for an opaque JSON object
// field. The key set is open, so any property is allowed.
func jsonObjectSchema() *ogen.Schema {
	s := ogen.NewSchema().SetType("object")
	s.AdditionalProperties = &ogen.AdditionalProperties{Bool: new(true)}
	return s
}
