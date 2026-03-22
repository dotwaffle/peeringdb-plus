// Package schema defines the entgo schema types for PeeringDB objects.
package schema

// SocialMedia represents a social media link from PeeringDB.
// Used by Organization, Network, Facility, InternetExchange, Carrier, and Campus schemas.
type SocialMedia struct {
	Service    string `json:"service"`
	Identifier string `json:"identifier"`
}
