// Package schema defines the entgo schema types for PeeringDB objects.
package schema

import "github.com/ogen-go/ogen"

// SocialMedia represents a social media link from PeeringDB.
// Used by Organization, Network, Facility, InternetExchange, Carrier, and Campus schemas.
type SocialMedia struct {
	Service    string `json:"service"`
	Identifier string `json:"identifier"`
}

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
