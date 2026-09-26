// Package pdbcompat provides a PeeringDB-compatible REST API layer that
// translates Django-style query parameters to ent predicates and serializes
// ent entities to PeeringDB's exact JSON response format.
package pdbcompat

import (
	"context"
	"fmt"
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
	// FieldMultiChoice indicates a multi-value choice field, stored as a
	// JSON array of strings. The filters compare the string that
	// upstream stores (see multichoice_filter.go).
	FieldMultiChoice
)

// String returns the human-readable name of the field type. It is used in
// client-facing 400 filter-error messages so callers see "bool" rather
// than the internal enum integer. Unknown values render as
// "unknown(<n>)" rather than panicking — filter errors must never crash
// the request path.
func (ft FieldType) String() string {
	switch ft {
	case FieldString:
		return "string"
	case FieldInt:
		return "int"
	case FieldBool:
		return "bool"
	case FieldTime:
		return "time"
	case FieldFloat:
		return "float"
	case FieldMultiChoice:
		return "multichoice"
	default:
		return fmt.Sprintf("unknown(%d)", int(ft))
	}
}

// QueryOptions holds parsed query parameters for list endpoints.
type QueryOptions struct {
	Filters []func(*sql.Selector)
	Limit   int
	Skip    int
	Since   *time.Time

	// EmptyResult is set by ParseFilters when the request contains an
	// __in filter with zero values (e.g. ?asn__in=). Each list closure in
	// registry_funcs.go short-circuits on this flag and returns an empty
	// result set without issuing any SQL — matches Django ORM
	// Model.objects.filter(id__in=[]).
	EmptyResult bool

	// OrderBy is a primary sort key that a filter asks for: the distance
	// of a fac or org distance search. listOrder puts it before the id
	// tiebreak on a plain list. A ?since= list ignores it.
	OrderBy func(*sql.Selector)
}

// ListFunc queries entities and returns their serialized objects.
type ListFunc func(ctx context.Context, client *ent.Client, opts QueryOptions) ([]any, error)

// CountFunc runs the predicate chain for a list query and returns the
// matching row count WITHOUT fetching row data. Used by the
// serveList pre-flight budget check to decide whether to 413 up-front
// before committing to an expensive .All(ctx) fetch.
//
// The returned count reflects what WOULD be served after Offset/Limit
// are applied — not the raw total. This matches the budget math that
// multiplies count × typicalRowBytes.
type CountFunc func(ctx context.Context, client *ent.Client, opts QueryOptions) (int, error)

// GetFunc queries a single entity by ID and returns its serialized form.
type GetFunc func(ctx context.Context, client *ent.Client, id int, depth int) (any, error)

// MatchFunc reports whether the row with the given id matches filters.
// A detail request uses it to apply the list filters: upstream
// get_object filters get_queryset(), which runs the same filters as a
// list (2.83.0 rest.py:849-855, :477-703). It adds no status of its
// own. The inline StatusIn of the get<Type>WithDepth PK lookup stays
// the detail status set. A filter can pin a status, as upstream does
// (a relation seed, make_relation_filter).
type MatchFunc func(ctx context.Context, client *ent.Client, id int, filters []func(*sql.Selector)) (bool, error)

// TypeConfig describes a PeeringDB object type for the compatibility layer.
type TypeConfig struct {
	Name         string
	Fields       map[string]FieldType
	SearchFields []string
	List         ListFunc
	Count        CountFunc
	Get          GetFunc
	Match        MatchFunc

	// FoldedFields lists the string fields on this type that have a sibling
	// <field>_fold column populated by the sync worker.
	// When non-nil, substring / prefix / iexact filters on these fields are
	// routed to the _fold column with unifold.Fold(value) on the RHS for
	// diacritic-insensitive matching. Nil is safe —
	// map reads on nil return the zero value (false).
	FoldedFields map[string]bool

	// ForeignKeys maps the upstream name of each forward foreign key of
	// the type (the Django field name, for example "org" or "network")
	// to the local FK column in Fields. Upstream filters a key that
	// names the FK (?org=1, ?network__in=1,2, ?facility_id=2) on the FK
	// column (2.83.0 rest.py:608-631, :670-677). A relation key that
	// starts with the upstream name (?network__asn=) walks the edge
	// that owns the column. Nil when the type has no forward FK.
	ForeignKeys map[string]string

	// UpstreamIgnored lists the Fields keys that upstream never filters
	// as a key without relation segments. Such a key is ignored for any
	// operator, as upstream ignores it. Two causes exist:
	//   - queryable_field_xl renames the key to a name that matches no
	//     field (2.83.0 serializers.py:428-438): carrier fac_count
	//     becomes facility_count, netixlan net_side_id becomes
	//     network_side.
	//   - The key is a serializer field or a model property, not a
	//     model field, and no prepare_query handles it (rest.py:525-528,
	//     :633, :670): for example campus city and carrier org_name.
	// The columns stay in Fields, so they stay valid traversal targets
	// where upstream filters them (carrierfac?carrier__fac_count=).
	UpstreamIgnored map[string]bool

	// NonModelFields lists the Fields keys that are not fields of the
	// upstream model: serializer fields and model properties. The
	// upstream filter loop filters only model fields and
	// queryable_relations (2.83.0 rest.py:525-528, :633, :670), so each
	// of these keys is a prepare_query key or in UpstreamIgnored. No
	// relation key filters one of these fields on the related row.
	// queryable_relations offers only model fields
	// (serializers.py:970-996), so upstream ignores a traversal key such
	// as fac?campus__city=, and so does the mirror. In a relation key of
	// a prepare_query, the Django filter raises FieldError, and the key
	// returns 400 (Invalid query), as upstream (rest.py:488-500).
	// UpstreamIgnored is not the same set: it also holds model fields
	// that queryable_field_xl renames (carrier fac_count).
	NonModelFields map[string]bool

	// ExactCounts lists the Fields keys that a prepare_query filters with
	// an exact lookup or a get_relation_filters operator (the count
	// seeds), with the first value of the key. The filter loop ignores
	// them (queryable_field_xl renames them). A value that int() does not
	// accept is a 400, as upstream (2.83.0 serializers.py:2119-2124,
	// :3743-3748, :4531-4543). Every other plain integer key matches the
	// decimal text of the value (buildModelFieldPredicate).
	ExactCounts map[string]bool
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
//
// Every type declares "status" as an ordinary filter field. Upstream
// (2.83.0) turns ?status=X into status__iexact (rest.py:683) and applies
// it before the list status matrix (rest.py:695, :745-748), so the two
// filters AND together. wireEntity appends applyStatusMatrix last, so a
// caller filter can narrow the admitted statuses but never widen them.
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
			"status":    FieldString,
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
			"info_types":                   FieldMultiChoice,
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
			"status":                       FieldString,
		},
		SearchFields: []string{"name", "aka", "name_long", "irr_as_set"},
		FoldedFields: map[string]bool{"name": true, "aka": true, "name_long": true},
		ForeignKeys:  map[string]string{"org": "org_id"},
		// serializers.py:3743-3748.
		ExactCounts: map[string]bool{"fac_count": true},
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
			"available_voltage_services":  FieldMultiChoice,
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
			"status":                      FieldString,
		},
		SearchFields: []string{"name", "aka", "name_long", "city", "country"},
		FoldedFields: map[string]bool{"name": true, "aka": true, "city": true},
		ForeignKeys:  map[string]string{"org": "org_id", "campus": "campus_id"},
		// org_name is a serializer field (serializers.py:1947).
		NonModelFields: map[string]bool{"org_name": true},
		// serializers.py:2119-2124.
		ExactCounts: map[string]bool{"net_count": true},
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
			"status":                    FieldString,
		},
		SearchFields: []string{"name", "aka", "name_long", "city", "country"},
		FoldedFields: map[string]bool{"name": true, "aka": true, "name_long": true, "city": true},
		ForeignKeys:  map[string]string{"org": "org_id"},
		// serializers.py:4531-4543.
		ExactCounts: map[string]bool{"net_count": true, "fac_count": true},
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
			"status":  FieldString,
		},
		SearchFields: []string{"name", "email"},
		ForeignKeys:  map[string]string{"network": "net_id"},
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
			"status":                          FieldString,
		},
		SearchFields: []string{"name", "descr"},
		ForeignKeys:  map[string]string{"ix": "ix_id"},
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
			"status":   FieldString,
		},
		SearchFields: []string{"prefix"},
		// The ix keys are prepare_query keys, see relationSeeds.
		ForeignKeys: map[string]string{"ixlan": "ixlan_id"},
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
			"status":      FieldString,
		},
		SearchFields: []string{"name"},
		// The ix keys are prepare_query keys, see relationSeeds. Upstream
		// cannot reach net_side: queryable_field_xl renames it to
		// network_side, which names no field (serializers.py:428-432).
		ForeignKeys: map[string]string{
			"network": "net_id",
			"ixlan":   "ixlan_id",
			"ix_side": "ix_side_id",
		},
		// models.py:6088, serializers.py:428-432.
		UpstreamIgnored: map[string]bool{"net_side_id": true},
		// name and ix_id are properties (models.py:6113-6115, :6131-6133).
		NonModelFields: map[string]bool{"name": true, "ix_id": true},
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
			"status":    FieldString,
		},
		SearchFields: []string{"name"},
		ForeignKeys:  map[string]string{"network": "net_id", "facility": "fac_id"},
		// local_asn is a property (models.py:6046-6051).
		UpstreamIgnored: map[string]bool{"local_asn": true},
		// name, city and country are serializer fields
		// (serializers.py:3372-3380), and local_asn is a property.
		NonModelFields: map[string]bool{
			"name":      true,
			"city":      true,
			"country":   true,
			"local_asn": true,
		},
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
			"status":  FieldString,
		},
		SearchFields: []string{"name"},
		ForeignKeys:  map[string]string{"ix": "ix_id", "facility": "fac_id"},
		// Serializer fields (serializers.py:2792-2800).
		NonModelFields: map[string]bool{"name": true, "city": true, "country": true},
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
			"status":    FieldString,
		},
		SearchFields: []string{"name", "aka", "name_long"},
		FoldedFields: map[string]bool{"name": true, "aka": true},
		ForeignKeys:  map[string]string{"org": "org_id"},
		// fac_count: models.py:6536, renamed by serializers.py:434-438.
		// org_name: serializer field only (serializers.py:2667).
		UpstreamIgnored: map[string]bool{"fac_count": true, "org_name": true},
		NonModelFields:  map[string]bool{"org_name": true},
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
			"status":     FieldString,
		},
		SearchFields: []string{"name"},
		ForeignKeys:  map[string]string{"carrier": "carrier_id", "facility": "fac_id"},
		// name: serializer field only (serializers.py:2601). The
		// serializer has no prepare_query.
		UpstreamIgnored: map[string]bool{"name": true},
		NonModelFields:  map[string]bool{"name": true},
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
			"status":    FieldString,
		},
		SearchFields: []string{"name"},
		FoldedFields: map[string]bool{"name": true},
		ForeignKeys:  map[string]string{"org": "org_id"},
		// org_name is a serializer field (serializers.py:4792). city,
		// country, state and zipcode are properties (models.py:2113-2147).
		UpstreamIgnored: map[string]bool{
			"org_name": true,
			"city":     true,
			"country":  true,
			"state":    true,
			"zipcode":  true,
		},
		NonModelFields: map[string]bool{
			"org_name": true,
			"city":     true,
			"country":  true,
			"state":    true,
			"zipcode":  true,
		},
	},
}
