// Package schema defines the entgo schema types for PeeringDB objects.
package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Organization holds the schema definition for the Organization entity.
// Maps to the PeeringDB "org" object type.
type Organization struct {
	ent.Schema
}

// Fields of the Organization.
func (Organization) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB organization ID"),
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Annotations(
				entgql.OrderField("NAME"),
			).
			Comment("Organization name"),
		field.String("aka").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Also known as"),
		field.String("name_long").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Long name"),
		field.String("website").
			Optional().
			Default("").
			Comment("Organization website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Comment("Social media links"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),

		// AddressModel fields
		field.String("address1").
			Optional().
			Default("").
			Comment("Address line 1"),
		field.String("address2").
			Optional().
			Default("").
			Comment("Address line 2"),
		field.String("city").
			Optional().
			Default("").
			Comment("City"),
		field.String("state").
			Optional().
			Default("").
			Comment("State or province"),
		field.String("country").
			Optional().
			Default("").
			Comment("Country code"),
		field.String("zipcode").
			Optional().
			Default("").
			Comment("Postal / ZIP code"),
		field.String("suite").
			Optional().
			Default("").
			Comment("Suite number"),
		field.String("floor").
			Optional().
			Default("").
			Comment("Floor"),
		field.Float("latitude").
			Optional().
			Nillable().
			Comment("Latitude"),
		field.Float("longitude").
			Optional().
			Nillable().
			Comment("Longitude"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			MaxLen(255).
			Default("ok").
			Comment("Record status"),
	}
}

// Edges of the Organization.
func (Organization) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("networks", Network.Type),
		edge.To("facilities", Facility.Type),
		edge.To("internet_exchanges", InternetExchange.Type),
		edge.To("carriers", Carrier.Type),
		edge.To("campuses", Campus.Type),
	}
}

// Indexes of the Organization.
func (Organization) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
	}
}

// Annotations of the Organization.
func (Organization) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns Organization's mutation hooks for OTel tracing per D-46.
func (Organization) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Organization"),
	}
}
