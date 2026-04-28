// Package pdbcompat provides a PeeringDB-compatible REST API layer that
// translates Django-style query parameters to ent predicates and serializes
// ent entities to PeeringDB's exact JSON response format.
package pdbcompat

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

// FieldType represents the data type of a filterable field.
type FieldType int

const (
	// FieldString indicates a string-typed field.
	FieldString FieldType = iota
	// FieldInt indicates an integer-typed field.
	FieldInt
	// FieldBool indicates a boolean-typed field.
	FieldBool
	// FieldTime indicates a time.Time-typed field.
	FieldTime
	// FieldFloat indicates a float64-typed field.
	FieldFloat
)

// QueryOptions holds parsed query parameters for list endpoints.
type QueryOptions struct {
	Filters []func(*sql.Selector)
	Limit   int
	Skip    int
	Since   *time.Time
	Search  string   // ?q= parameter
	Fields  []string // ?fields= parameter
	Depth   int      // depth parameter (only used on detail)

	// EmptyResult is set by ParseFilters when the request contains an
	// __in filter with zero values (e.g. ?asn__in=). Each list closure in
	// registry_funcs.go short-circuits on this flag and returns an empty
	// result set without issuing any SQL — matches Django ORM
	// Model.objects.filter(id__in=[]) per Phase 69 D-06 (IN-02).
	EmptyResult bool
}

// ListFunc queries entities and returns serialized objects plus total count.
type ListFunc func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, int, error)

// CountFunc runs the predicate chain for a list query and returns the
// matching row count WITHOUT fetching row data. Used by Phase 71
// serveList pre-flight budget check to decide whether to 413 up-front
// before committing to an expensive .All(ctx) fetch.
//
// The returned count reflects what WOULD be served after Offset/Limit
// are applied — not the raw total. This matches the budget math that
// multiplies count × typicalRowBytes.
type CountFunc func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error)

// GetFunc queries a single entity by ID and returns its serialized form.
type GetFunc func(ctx context.Context, client *ent.Client, id int, depth int) (any, error)

// TypeConfig describes a PeeringDB object type for the compatibility layer.
type TypeConfig struct {
	Name         string
	Fields       map[string]FieldType
	SearchFields []string
	List         ListFunc
	Count        CountFunc
	Get          GetFunc

	// FoldedFields lists the string fields on this type that have a sibling
	// <field>_fold column populated by the sync worker (Phase 69 Plan 03).
	// When non-nil, substring / prefix / iexact filters on these fields are
	// routed to the _fold column with unifold.Fold(value) on the RHS for
	// diacritic-insensitive matching (Phase 69 UNICODE-01). Nil is safe —
	// map reads on nil return the zero value (false).
	FoldedFields map[string]bool
}

// reservedParams lists query parameter names that are not filter fields.
var reservedParams = map[string]bool{
	"limit":  true,
	"skip":   true,
	"depth":  true,
	"since":  true,
	"q":      true,
	"fields": true,
}

// Registry maps PeeringDB type name strings to their TypeConfig.
// List and Get functions are nil until serializers are wired up.
var Registry = map[string]TypeConfig{
	peeringdb.TypeOrg: {
		Name: peeringdb.TypeOrg,
		Fields: map[string]FieldType{
			"id":        FieldInt,
			"name":      FieldString,
			"aka":       FieldString,
			"name_long": FieldString,
			"website":   FieldString,
			"notes":     FieldString,
			"logo":      FieldString,
			"address1":  FieldString,
			"address2":  FieldString,
			"city":      FieldString,
			"state":     FieldString,
			"country":   FieldString,
			"zipcode":   FieldString,
			"suite":     FieldString,
			"floor":     FieldString,
			"latitude":  FieldFloat,
			"longitude": FieldFloat,
			"created":   FieldTime,
			"updated":   FieldTime,
		},
		SearchFields: []string{"name", "aka", "name_long"},
		FoldedFields: map[string]bool{"name": true, "aka": true, "city": true},
	},
	peeringdb.TypeNet: {
		Name: peeringdb.TypeNet,
		Fields: map[string]FieldType{
			"id":                           FieldInt,
			"org_id":                       FieldInt,
			"name":                         FieldString,
			"aka":                          FieldString,
			"name_long":                    FieldString,
			"website":                      FieldString,
			"asn":                          FieldInt,
			"looking_glass":                FieldString,
			"route_server":                 FieldString,
			"irr_as_set":                   FieldString,
			"info_type":                    FieldString,
			"info_prefixes4":               FieldInt,
			"info_prefixes6":               FieldInt,
			"info_traffic":                 FieldString,
			"info_ratio":                   FieldString,
			"info_scope":                   FieldString,
			"info_unicast":                 FieldBool,
			"info_multicast":               FieldBool,
			"info_ipv6":                    FieldBool,
			"info_never_via_route_servers": FieldBool,
			"notes":                        FieldString,
			"policy_url":                   FieldString,
			"policy_general":               FieldString,
			"policy_locations":             FieldString,
			"policy_ratio":                 FieldBool,
			"policy_contracts":             FieldString,
			"allow_ixp_update":             FieldBool,
			"status_dashboard":             FieldString,
			"rir_status":                   FieldString,
			"rir_status_updated":           FieldTime,
			"logo":                         FieldString,
			"ix_count":                     FieldInt,
			"fac_count":                    FieldInt,
			"netixlan_updated":             FieldTime,
			"netfac_updated":               FieldTime,
			"poc_updated":                  FieldTime,
			"created":                      FieldTime,
			"updated":                      FieldTime,
		},
		SearchFields: []string{"name", "aka", "name_long", "irr_as_set"},
		FoldedFields: map[string]bool{"name": true, "aka": true, "name_long": true},
	},
	peeringdb.TypeFac: {
		Name: peeringdb.TypeFac,
		Fields: map[string]FieldType{
			"id":                          FieldInt,
			"org_id":                      FieldInt,
			"org_name":                    FieldString,
			"campus_id":                   FieldInt,
			"name":                        FieldString,
			"aka":                         FieldString,
			"name_long":                   FieldString,
			"website":                     FieldString,
			"clli":                        FieldString,
			"rencode":                     FieldString,
			"npanxx":                      FieldString,
			"tech_email":                  FieldString,
			"tech_phone":                  FieldString,
			"sales_email":                 FieldString,
			"sales_phone":                 FieldString,
			"property":                    FieldString,
			"diverse_serving_substations": FieldBool,
			"notes":                       FieldString,
			"region_continent":            FieldString,
			"status_dashboard":            FieldString,
			"logo":                        FieldString,
			"net_count":                   FieldInt,
			"ix_count":                    FieldInt,
			"carrier_count":               FieldInt,
			"address1":                    FieldString,
			"address2":                    FieldString,
			"city":                        FieldString,
			"state":                       FieldString,
			"country":                     FieldString,
			"zipcode":                     FieldString,
			"suite":                       FieldString,
			"floor":                       FieldString,
			"latitude":                    FieldFloat,
			"longitude":                   FieldFloat,
			"created":                     FieldTime,
			"updated":                     FieldTime,
		},
		SearchFields: []string{"name", "aka", "name_long", "city", "country"},
		FoldedFields: map[string]bool{"name": true, "aka": true, "city": true},
	},
	peeringdb.TypeIX: {
		Name: peeringdb.TypeIX,
		Fields: map[string]FieldType{
			"id":                        FieldInt,
			"org_id":                    FieldInt,
			"name":                      FieldString,
			"aka":                       FieldString,
			"name_long":                 FieldString,
			"city":                      FieldString,
			"country":                   FieldString,
			"region_continent":          FieldString,
			"media":                     FieldString,
			"notes":                     FieldString,
			"proto_unicast":             FieldBool,
			"proto_multicast":           FieldBool,
			"proto_ipv6":                FieldBool,
			"website":                   FieldString,
			"url_stats":                 FieldString,
			"tech_email":                FieldString,
			"tech_phone":                FieldString,
			"policy_email":              FieldString,
			"policy_phone":              FieldString,
			"sales_email":               FieldString,
			"sales_phone":               FieldString,
			"net_count":                 FieldInt,
			"fac_count":                 FieldInt,
			"ixf_net_count":             FieldInt,
			"ixf_last_import":           FieldTime,
			"ixf_import_request":        FieldString,
			"ixf_import_request_status": FieldString,
			"service_level":             FieldString,
			"terms":                     FieldString,
			"status_dashboard":          FieldString,
			"logo":                      FieldString,
			"created":                   FieldTime,
			"updated":                   FieldTime,
		},
		SearchFields: []string{"name", "aka", "name_long", "city", "country"},
		FoldedFields: map[string]bool{"name": true, "aka": true, "name_long": true, "city": true},
	},
	peeringdb.TypePoc: {
		Name: peeringdb.TypePoc,
		Fields: map[string]FieldType{
			"id":      FieldInt,
			"net_id":  FieldInt,
			"role":    FieldString,
			"visible": FieldString,
			"name":    FieldString,
			"phone":   FieldString,
			"email":   FieldString,
			"url":     FieldString,
			"created": FieldTime,
			"updated": FieldTime,
		},
		SearchFields: []string{"name", "email"},
	},
	peeringdb.TypeIXLan: {
		Name: peeringdb.TypeIXLan,
		Fields: map[string]FieldType{
			"id":                              FieldInt,
			"ix_id":                           FieldInt,
			"name":                            FieldString,
			"descr":                           FieldString,
			"mtu":                             FieldInt,
			"dot1q_support":                   FieldBool,
			"rs_asn":                          FieldInt,
			"arp_sponge":                      FieldString,
			"ixf_ixp_member_list_url_visible": FieldString,
			"ixf_ixp_import_enabled":          FieldBool,
			"created":                         FieldTime,
			"updated":                         FieldTime,
		},
		SearchFields: []string{"name", "descr"},
	},
	peeringdb.TypeIXPfx: {
		Name: peeringdb.TypeIXPfx,
		Fields: map[string]FieldType{
			"id":       FieldInt,
			"ixlan_id": FieldInt,
			"protocol": FieldString,
			"prefix":   FieldString,
			"in_dfz":   FieldBool,
			"created":  FieldTime,
			"updated":  FieldTime,
		},
		SearchFields: []string{"prefix"},
	},
	peeringdb.TypeNetIXLan: {
		Name: peeringdb.TypeNetIXLan,
		Fields: map[string]FieldType{
			"id":          FieldInt,
			"net_id":      FieldInt,
			"ix_id":       FieldInt,
			"ixlan_id":    FieldInt,
			"name":        FieldString,
			"notes":       FieldString,
			"speed":       FieldInt,
			"asn":         FieldInt,
			"ipaddr4":     FieldString,
			"ipaddr6":     FieldString,
			"is_rs_peer":  FieldBool,
			"bfd_support": FieldBool,
			"operational": FieldBool,
			"net_side_id": FieldInt,
			"ix_side_id":  FieldInt,
			"created":     FieldTime,
			"updated":     FieldTime,
		},
		SearchFields: []string{"name"},
	},
	peeringdb.TypeNetFac: {
		Name: peeringdb.TypeNetFac,
		Fields: map[string]FieldType{
			"id":        FieldInt,
			"net_id":    FieldInt,
			"fac_id":    FieldInt,
			"name":      FieldString,
			"city":      FieldString,
			"country":   FieldString,
			"local_asn": FieldInt,
			"created":   FieldTime,
			"updated":   FieldTime,
		},
		SearchFields: []string{"name"},
	},
	peeringdb.TypeIXFac: {
		Name: peeringdb.TypeIXFac,
		Fields: map[string]FieldType{
			"id":      FieldInt,
			"ix_id":   FieldInt,
			"fac_id":  FieldInt,
			"name":    FieldString,
			"city":    FieldString,
			"country": FieldString,
			"created": FieldTime,
			"updated": FieldTime,
		},
		SearchFields: []string{"name"},
	},
	peeringdb.TypeCarrier: {
		Name: peeringdb.TypeCarrier,
		Fields: map[string]FieldType{
			"id":        FieldInt,
			"org_id":    FieldInt,
			"org_name":  FieldString,
			"name":      FieldString,
			"aka":       FieldString,
			"name_long": FieldString,
			"website":   FieldString,
			"notes":     FieldString,
			"fac_count": FieldInt,
			"logo":      FieldString,
			"created":   FieldTime,
			"updated":   FieldTime,
		},
		SearchFields: []string{"name", "aka", "name_long"},
		FoldedFields: map[string]bool{"name": true, "aka": true},
	},
	peeringdb.TypeCarrierFac: {
		Name: peeringdb.TypeCarrierFac,
		Fields: map[string]FieldType{
			"id":         FieldInt,
			"carrier_id": FieldInt,
			"fac_id":     FieldInt,
			"name":       FieldString,
			"created":    FieldTime,
			"updated":    FieldTime,
		},
		SearchFields: []string{"name"},
	},
	peeringdb.TypeCampus: {
		Name: peeringdb.TypeCampus,
		Fields: map[string]FieldType{
			"id":        FieldInt,
			"org_id":    FieldInt,
			"org_name":  FieldString,
			"name":      FieldString,
			"name_long": FieldString,
			"aka":       FieldString,
			"website":   FieldString,
			"notes":     FieldString,
			"country":   FieldString,
			"city":      FieldString,
			"zipcode":   FieldString,
			"state":     FieldString,
			"logo":      FieldString,
			"created":   FieldTime,
			"updated":   FieldTime,
		},
		SearchFields: []string{"name"},
		FoldedFields: map[string]bool{"name": true},
	},
}
