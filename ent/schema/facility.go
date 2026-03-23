package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/lrstanley/entrest"
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
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("org_name").
			Optional().
			Default("").
			Comment("Organization name (computed)"),
		field.Int("campus_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to campus"),
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray),
			).
			Comment("Facility name"),
		field.String("aka").
			Optional().
			MaxLen(255).
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Also known as"),
		field.String("name_long").
			Optional().
			MaxLen(255).
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Long name"),
		field.String("website").
			Optional().
			Default("").
			Comment("Facility website URL"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
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
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("City"),
		field.String("state").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("State or province"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
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
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			MaxLen(255).
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Record status"),
	}
}

// Edges of the Facility.
func (Facility) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("organization", Organization.Type).
			Ref("facilities").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("campus", Campus.Type).
			Ref("facilities").
			Field("campus_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("network_facilities", NetworkFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("ix_facilities", IxFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("carrier_facilities", CarrierFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
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
		entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
	}
}

// Hooks returns Facility mutation hooks for OTel tracing per D-46.
func (Facility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Facility"),
	}
}
