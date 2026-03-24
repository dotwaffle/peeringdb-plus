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
		field.Int("campus_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to campus"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ | entrest.FilterNEQ | entrest.FilterGT | entrest.FilterGTE | entrest.FilterLT | entrest.FilterLTE | entrest.FilterIn | entrest.FilterNotIn)).
			Comment("FK to organization"),
		field.String("address1").
			MaxLen(255).
			Optional().
			Default("").
			Comment("Address line 1"),
		field.String("address2").
			MaxLen(255).
			Optional().
			Default("").
			Comment("Address line 2"),
		field.String("aka").
			MaxLen(255).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Also known as"),
		field.JSON("available_voltage_services", []string{}).
			Optional().
			Comment("Available voltage services"),
		field.String("city").
			MaxLen(255).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("City"),
		field.String("clli").
			MaxLen(18).
			Optional().
			Default("").
			Comment("CLLI code"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Country code"),
		field.Bool("diverse_serving_substations").
			Optional().
			Nillable().
			Comment("Diverse serving substations"),
		field.String("floor").
			MaxLen(255).
			Optional().
			Default("").
			Comment("Floor"),
		field.Float("latitude").
			Optional().
			Nillable().
			Comment("Latitude"),
		field.String("logo").
			Optional().
			Nillable().
			Comment("Logo URL"),
		field.Float("longitude").
			Optional().
			Nillable().
			Comment("Longitude"),
		field.String("name").
			MaxLen(255).
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
			).
			Comment("Facility name"),
		field.String("name_long").
			MaxLen(255).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Comment("Notes"),
		field.String("npanxx").
			MaxLen(21).
			Optional().
			Default("").
			Comment("NPA-NXX code"),
		field.String("property").
			MaxLen(27).
			Optional().
			Nillable().
			Comment("Property type"),
		field.String("region_continent").
			Optional().
			Nillable().
			Comment("Region/continent"),
		field.String("rencode").
			MaxLen(18).
			Optional().
			Default("").
			Comment("Rencode"),
		field.String("sales_email").
			MaxLen(254).
			Optional().
			Default("").
			Comment("Sales email"),
		field.String("sales_phone").
			MaxLen(192).
			Optional().
			Default("").
			Comment("Sales phone"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema())).
			Comment("Social media links"),
		field.String("state").
			MaxLen(255).
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)).
			Comment("State or province"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Comment("Status dashboard URL"),
		field.String("suite").
			MaxLen(255).
			Optional().
			Default("").
			Comment("Suite number"),
		field.String("tech_email").
			MaxLen(254).
			Optional().
			Default("").
			Comment("Technical email"),
		field.String("tech_phone").
			MaxLen(192).
			Optional().
			Default("").
			Comment("Technical phone"),
		field.String("website").
			Optional().
			Default("").
			Comment("Facility website URL"),
		field.String("zipcode").
			MaxLen(48).
			Optional().
			Default("").
			Comment("Postal / ZIP code"),

		// Computed fields (from serializer, stored per D-40)
		field.String("org_name").
			Optional().
			Default("").
			Comment("Org Name (computed)"),
		field.Int("net_count").
			Optional().
			Default(0).
			Comment("Net Count (computed)"),
		field.Int("ix_count").
			Optional().
			Default(0).
			Comment("Ix Count (computed)"),
		field.Int("carrier_count").
			Optional().
			Default(0).
			Comment("Carrier Count (computed)"),

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
		edge.From("campus", Campus.Type).
			Ref("facilities").
			Field("campus_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("carrier_facilities", CarrierFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("ix_facilities", IxFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.To("network_facilities", NetworkFacility.Type).
			Annotations(entrest.WithEagerLoad(true)),
		edge.From("organization", Organization.Type).
			Ref("facilities").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true)),
	}
}

// Indexes of the Facility.
func (Facility) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("campus_id"),
		index.Fields("name"),
		index.Fields("org_id"),
		index.Fields("status"),
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
