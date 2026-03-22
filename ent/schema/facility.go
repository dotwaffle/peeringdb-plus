package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Facility holds the schema definition for the Facility entity.
// Maps to the PeeringDB "fac" object type.
type Facility struct {
	ent.Schema
}

// Fields of the Facility.
func (Facility) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").
			Positive().
			Immutable().
			Comment("PeeringDB facility ID"),
		field.Int("org_id").
			Optional().
			Nillable().
			Comment("FK to organization"),
		field.String("org_name").
			Optional().
			Default("").
			Comment("Organization name (computed)"),
		field.Int("campus_id").
			Optional().
			Nillable().
			Comment("FK to campus"),
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
			).
			Comment("Facility name"),
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
			Comment("Facility website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Comment("Social media links"),
		field.String("clli").
			Optional().
			MaxLen(18).
			Default("").
			Comment("CLLI code"),
		field.String("rencode").
			Optional().
			MaxLen(18).
			Default("").
			Comment("Rencode"),
		field.String("npanxx").
			Optional().
			MaxLen(21).
			Default("").
			Comment("NPANXX"),
		field.String("tech_email").
			Optional().
			MaxLen(254).
			Default("").
			Comment("Technical contact email"),
		field.String("tech_phone").
			Optional().
			MaxLen(192).
			Default("").
			Comment("Technical contact phone"),
		field.String("sales_email").
			Optional().
			MaxLen(254).
			Default("").
			Comment("Sales contact email"),
		field.String("sales_phone").
			Optional().
			MaxLen(192).
			Default("").
			Comment("Sales contact phone"),
		field.String("property").
			Optional().
			Nillable().
			MaxLen(27).
			Comment("Property type"),
		field.Bool("diverse_serving_substations").
			Optional().
			Nillable().
			Comment("Has diverse serving substations"),
		field.JSON("available_voltage_services", []string{}).
			Optional().
			Comment("Available voltage services"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("region_continent").
			Optional().
			Nillable().
			Comment("Region / continent"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Comment("Status dashboard URL"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),

		// Computed fields (from serializer, stored per D-40)
		field.Int("net_count").
			Optional().
			Default(0).
			Comment("Network count (computed)"),
		field.Int("ix_count").
			Optional().
			Default(0).
			Comment("Internet exchange count (computed)"),
		field.Int("carrier_count").
			Optional().
			Default(0).
			Comment("Carrier count (computed)"),

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
			MaxLen(48).
			Default("").
			Comment("Postal / ZIP code"),
		field.String("suite").
			Optional().
			MaxLen(255).
			Default("").
			Comment("Suite number"),
		field.String("floor").
			Optional().
			MaxLen(255).
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

// Edges of the Facility.
func (Facility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("facilities").
			Field("org_id").
			Unique(),
		edge.From("campus", Campus.Type).
			Ref("facilities").
			Field("campus_id").
			Unique(),
		edge.To("network_facilities", NetworkFacility.Type),
		edge.To("ix_facilities", IxFacility.Type),
		edge.To("carrier_facilities", CarrierFacility.Type),
	}
}

// Indexes of the Facility.
func (Facility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name"),
		index.Fields("status"),
		index.Fields("org_id"),
		index.Fields("campus_id"),
	}
}

// Annotations of the Facility.
func (Facility) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entgql.RelayConnection(),
		entgql.QueryField(),
	}
}

// Hooks returns Facility mutation hooks for OTel tracing per D-46.
func (Facility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Facility"),
	}
}
