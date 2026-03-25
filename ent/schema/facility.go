package schema

import (
	"entgo.io/contrib/entgql"
	"entgo.io/contrib/entproto"
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
			Annotations(entproto.Field(1)).
			Comment("PeeringDB facility ID"),
		field.Int("campus_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(2)).
			Comment("FK to campus"),
		field.Int("org_id").
			Optional().
			Nillable().
			Annotations(entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE|entrest.FilterIn|entrest.FilterNotIn), entproto.Field(3)).
			Comment("FK to organization"),
		field.String("address1").
			Optional().
			Default("").
			Annotations(entproto.Field(4)).
			Comment("Address line 1"),
		field.String("address2").
			Optional().
			Default("").
			Annotations(entproto.Field(5)).
			Comment("Address line 2"),
		field.String("aka").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(6)).
			Comment("Also known as"),
		field.JSON("available_voltage_services", []string{}).
			Optional().
			Annotations(entproto.Field(7)).
			Comment("Available voltage services"),
		field.String("city").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(8)).
			Comment("City"),
		field.String("clli").
			Optional().
			Default("").
			Annotations(entproto.Field(9)).
			Comment("CLLI code"),
		field.String("country").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(10)).
			Comment("Country code"),
		field.Bool("diverse_serving_substations").
			Optional().
			Nillable().
			Annotations(entproto.Field(11)).
			Comment("Diverse serving substations"),
		field.String("floor").
			Optional().
			Default("").
			Annotations(entproto.Field(12)).
			Comment("Floor"),
		field.Float("latitude").
			Optional().
			Nillable().
			Annotations(entproto.Field(13)).
			Comment("Latitude"),
		field.String("logo").
			Optional().
			Nillable().
			Annotations(entproto.Field(14)).
			Comment("Logo URL"),
		field.Float("longitude").
			Optional().
			Nillable().
			Annotations(entproto.Field(15)).
			Comment("Longitude"),
		field.String("name").
			NotEmpty().
			Unique().
			Annotations(
				entgql.OrderField("NAME"),
				entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
				entproto.Field(16),
			).
			Comment("Facility name"),
		field.String("name_long").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(17)).
			Comment("Long name"),
		field.String("notes").
			Optional().
			Default("").
			Annotations(entproto.Field(18)).
			Comment("Notes"),
		field.String("npanxx").
			Optional().
			Default("").
			Annotations(entproto.Field(19)).
			Comment("NPA-NXX code"),
		field.String("property").
			Optional().
			Nillable().
			Annotations(entproto.Field(20)).
			Comment("Property type"),
		field.String("region_continent").
			Optional().
			Nillable().
			Annotations(entproto.Field(21)).
			Comment("Region/continent"),
		field.String("rencode").
			Optional().
			Default("").
			Annotations(entproto.Field(22)).
			Comment("Rencode"),
		field.String("sales_email").
			Optional().
			Default("").
			Annotations(entproto.Field(23)).
			Comment("Sales email"),
		field.String("sales_phone").
			Optional().
			Default("").
			Annotations(entproto.Field(24)).
			Comment("Sales phone"),
		field.JSON("social_media", []SocialMedia{}).
			Optional().
			Annotations(entrest.WithSchema(socialMediaSchema()), entproto.Skip()).
			Comment("Social media links"),
		field.String("state").
			Optional().
			Default("").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(25)).
			Comment("State or province"),
		field.String("status_dashboard").
			Optional().
			Nillable().
			Annotations(entproto.Field(26)).
			Comment("Status dashboard URL"),
		field.String("suite").
			Optional().
			Default("").
			Annotations(entproto.Field(27)).
			Comment("Suite number"),
		field.String("tech_email").
			Optional().
			Default("").
			Annotations(entproto.Field(28)).
			Comment("Technical email"),
		field.String("tech_phone").
			Optional().
			Default("").
			Annotations(entproto.Field(29)).
			Comment("Technical phone"),
		field.String("website").
			Optional().
			Default("").
			Annotations(entproto.Field(30)).
			Comment("Facility website URL"),
		field.String("zipcode").
			Optional().
			Default("").
			Annotations(entproto.Field(31)).
			Comment("Postal / ZIP code"),

		// Computed fields (from serializer, stored per D-40)
		field.String("org_name").
			Optional().
			Default("").
			Annotations(entproto.Field(32)).
			Comment("Org Name (computed)"),
		field.Int("net_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(33)).
			Comment("Net Count (computed)"),
		field.Int("ix_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(34)).
			Comment("Ix Count (computed)"),
		field.Int("carrier_count").
			Optional().
			Default(0).
			Annotations(entproto.Field(35)).
			Comment("Carrier Count (computed)"),

		// HandleRefModel common fields
		field.Time("created").
			Immutable().
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(36)).
			Comment("PeeringDB creation timestamp"),
		field.Time("updated").
			Annotations(entrest.WithFilter(entrest.FilterGT|entrest.FilterGTE|entrest.FilterLT|entrest.FilterLTE), entproto.Field(37)).
			Comment("PeeringDB last update timestamp"),
		field.String("status").
			Default("ok").
			Annotations(entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray), entproto.Field(38)).
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
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("carrier_facilities", CarrierFacility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("ix_facilities", IxFacility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.To("network_facilities", NetworkFacility.Type).
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
		edge.From("organization", Organization.Type).
			Ref("facilities").
			Field("org_id").
			Unique().
			Annotations(entrest.WithEagerLoad(true), entproto.Skip()),
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
		entproto.Message(entproto.PackageName("peeringdb.v1")),
	}
}

// Hooks returns Facility mutation hooks for OTel tracing per D-46.
func (Facility) Hooks() []ent.Hook {
	return []ent.Hook{
		otelMutationHook("Facility"),
	}
}
