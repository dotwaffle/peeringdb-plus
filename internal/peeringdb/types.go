// Package peeringdb provides a client for the PeeringDB API with rate
// limiting, pagination, retry logic, and response types for all 13
// PeeringDB object types.
package peeringdb

import (
	"encoding/json"
	"time"
)

// Response wraps PeeringDB API responses. The meta field is always
// empty in practice. Data contains the array of objects.
type Response[T any] struct {
	Meta json.RawMessage `json:"meta"`
	Data []T             `json:"data"`
}

// Object type constants for PeeringDB API paths.
const (
	TypeOrg        = "org"
	TypeNet        = "net"
	TypeFac        = "fac"
	TypeIX         = "ix"
	TypePoc        = "poc"
	TypeIXLan      = "ixlan"
	TypeIXPfx      = "ixpfx"
	TypeNetIXLan   = "netixlan"
	TypeNetFac     = "netfac"
	TypeIXFac      = "ixfac"
	TypeCarrier    = "carrier"
	TypeCarrierFac = "carrierfac"
	TypeCampus     = "campus"
)

// SocialMedia represents a social media link on a PeeringDB object.
type SocialMedia struct {
	Service    string `json:"service"`
	Identifier string `json:"identifier"`
}

// Organization represents a PeeringDB organization (org).
type Organization struct {
	ID          int           `json:"id"`
	Name        string        `json:"name"`
	Aka         string        `json:"aka"`
	NameLong    string        `json:"name_long"`
	Website     string        `json:"website"`
	SocialMedia []SocialMedia `json:"social_media"`
	Notes       string        `json:"notes"`
	Logo        *string       `json:"logo"`
	Address1    string        `json:"address1"`
	Address2    string        `json:"address2"`
	City        string        `json:"city"`
	State       string        `json:"state"`
	Country     string        `json:"country"`
	Zipcode     string        `json:"zipcode"`
	Suite       string        `json:"suite"`
	Floor       string        `json:"floor"`
	Latitude    *float64      `json:"latitude"`
	Longitude   *float64      `json:"longitude"`
	Created     time.Time     `json:"created"`
	Updated     time.Time     `json:"updated"`
	Status      string        `json:"status"`
}

// Network represents a PeeringDB network (net).
type Network struct {
	ID                      int           `json:"id"`
	OrgID                   int           `json:"org_id"`
	Name                    string        `json:"name"`
	Aka                     string        `json:"aka"`
	NameLong                string        `json:"name_long"`
	Website                 string        `json:"website"`
	SocialMedia             []SocialMedia `json:"social_media"`
	ASN                     int           `json:"asn"`
	LookingGlass            string        `json:"looking_glass"`
	RouteServer             string        `json:"route_server"`
	IRRASSet                string        `json:"irr_as_set"`
	InfoType                string        `json:"info_type"`
	InfoTypes               []string      `json:"info_types"`
	InfoPrefixes4           *int          `json:"info_prefixes4"`
	InfoPrefixes6           *int          `json:"info_prefixes6"`
	InfoTraffic             string        `json:"info_traffic"`
	InfoRatio               string        `json:"info_ratio"`
	InfoScope               string        `json:"info_scope"`
	InfoUnicast             bool          `json:"info_unicast"`
	InfoMulticast           bool          `json:"info_multicast"`
	InfoIPv6                bool          `json:"info_ipv6"`
	InfoNeverViaRouteServer bool          `json:"info_never_via_route_servers"`
	Notes                   string        `json:"notes"`
	PolicyURL               string        `json:"policy_url"`
	PolicyGeneral           string        `json:"policy_general"`
	PolicyLocations         string        `json:"policy_locations"`
	PolicyRatio             bool          `json:"policy_ratio"`
	PolicyContracts         string        `json:"policy_contracts"`
	AllowIXPUpdate          bool          `json:"allow_ixp_update"`
	StatusDashboard         *string       `json:"status_dashboard"`
	RIRStatus               *string       `json:"rir_status"`
	RIRStatusUpdated        *time.Time    `json:"rir_status_updated"`
	Logo                    *string       `json:"logo"`
	IXCount                 int           `json:"ix_count"`
	FacCount                int           `json:"fac_count"`
	NetIXLanUpdated         *time.Time    `json:"netixlan_updated"`
	NetFacUpdated           *time.Time    `json:"netfac_updated"`
	PocUpdated              *time.Time    `json:"poc_updated"`
	Created                 time.Time     `json:"created"`
	Updated                 time.Time     `json:"updated"`
	Status                  string        `json:"status"`
}

// Facility represents a PeeringDB facility (fac).
type Facility struct {
	ID                        int           `json:"id"`
	OrgID                     int           `json:"org_id"`
	OrgName                   string        `json:"org_name"`
	CampusID                  *int          `json:"campus_id"`
	Name                      string        `json:"name"`
	Aka                       string        `json:"aka"`
	NameLong                  string        `json:"name_long"`
	Website                   string        `json:"website"`
	SocialMedia               []SocialMedia `json:"social_media"`
	CLLI                      string        `json:"clli"`
	Rencode                   string        `json:"rencode"`
	NPANXX                    string        `json:"npanxx"`
	TechEmail                 string        `json:"tech_email"`
	TechPhone                 string        `json:"tech_phone"`
	SalesEmail                string        `json:"sales_email"`
	SalesPhone                string        `json:"sales_phone"`
	Property                  *string       `json:"property"`
	DiverseServingSubstations *bool         `json:"diverse_serving_substations"`
	AvailableVoltageServices  []string      `json:"available_voltage_services"`
	Notes                     string        `json:"notes"`
	RegionContinent           *string       `json:"region_continent"`
	StatusDashboard           *string       `json:"status_dashboard"`
	Logo                      *string       `json:"logo"`
	NetCount                  int           `json:"net_count"`
	IXCount                   int           `json:"ix_count"`
	CarrierCount              int           `json:"carrier_count"`
	Address1                  string        `json:"address1"`
	Address2                  string        `json:"address2"`
	City                      string        `json:"city"`
	State                     string        `json:"state"`
	Country                   string        `json:"country"`
	Zipcode                   string        `json:"zipcode"`
	Suite                     string        `json:"suite"`
	Floor                     string        `json:"floor"`
	Latitude                  *float64      `json:"latitude"`
	Longitude                 *float64      `json:"longitude"`
	Created                   time.Time     `json:"created"`
	Updated                   time.Time     `json:"updated"`
	Status                    string        `json:"status"`
}

// InternetExchange represents a PeeringDB internet exchange (ix).
type InternetExchange struct {
	ID                     int           `json:"id"`
	OrgID                  int           `json:"org_id"`
	Name                   string        `json:"name"`
	Aka                    string        `json:"aka"`
	NameLong               string        `json:"name_long"`
	City                   string        `json:"city"`
	Country                string        `json:"country"`
	RegionContinent        string        `json:"region_continent"`
	Media                  string        `json:"media"`
	Notes                  string        `json:"notes"`
	ProtoUnicast           bool          `json:"proto_unicast"`
	ProtoMulticast         bool          `json:"proto_multicast"`
	ProtoIPv6              bool          `json:"proto_ipv6"`
	Website                string        `json:"website"`
	SocialMedia            []SocialMedia `json:"social_media"`
	URLStats               string        `json:"url_stats"`
	TechEmail              string        `json:"tech_email"`
	TechPhone              string        `json:"tech_phone"`
	PolicyEmail            string        `json:"policy_email"`
	PolicyPhone            string        `json:"policy_phone"`
	SalesEmail             string        `json:"sales_email"`
	SalesPhone             string        `json:"sales_phone"`
	NetCount               int           `json:"net_count"`
	FacCount               int           `json:"fac_count"`
	IXFNetCount            int           `json:"ixf_net_count"`
	IXFLastImport          *time.Time    `json:"ixf_last_import"`
	IXFImportRequest       *string       `json:"ixf_import_request"`
	IXFImportRequestStatus string        `json:"ixf_import_request_status"`
	ServiceLevel           string        `json:"service_level"`
	Terms                  string        `json:"terms"`
	StatusDashboard        *string       `json:"status_dashboard"`
	Logo                   *string       `json:"logo"`
	Created                time.Time     `json:"created"`
	Updated                time.Time     `json:"updated"`
	Status                 string        `json:"status"`
}

// Poc represents a PeeringDB network point of contact (poc).
type Poc struct {
	ID      int       `json:"id"`
	NetID   int       `json:"net_id"`
	Role    string    `json:"role"`
	Visible string    `json:"visible"`
	Name    string    `json:"name"`
	Phone   string    `json:"phone"`
	Email   string    `json:"email"`
	URL     string    `json:"url"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Status  string    `json:"status"`
}

// IxLan represents a PeeringDB IX LAN (ixlan).
type IxLan struct {
	ID                         int       `json:"id"`
	IXID                       int       `json:"ix_id"`
	Name                       string    `json:"name"`
	Descr                      string    `json:"descr"`
	MTU                        int       `json:"mtu"`
	Dot1QSupport               bool      `json:"dot1q_support"`
	RSASN                      *int      `json:"rs_asn"`
	ARPSponge                  *string   `json:"arp_sponge"`
	IXFIXPMemberListURLVisible string    `json:"ixf_ixp_member_list_url_visible"`
	IXFIXPMemberListURL        string    `json:"ixf_ixp_member_list_url,omitempty"`
	IXFIXPImportEnabled        bool      `json:"ixf_ixp_import_enabled"`
	Created                    time.Time `json:"created"`
	Updated                    time.Time `json:"updated"`
	Status                     string    `json:"status"`
}

// IxPrefix represents a PeeringDB IX prefix (ixpfx).
//
// Phase 63 (D-01): dropped the "notes" field to match upstream PeeringDB's
// live /api/ixpfx response, which omits "notes" from every row.
type IxPrefix struct {
	ID       int       `json:"id"`
	IXLanID  int       `json:"ixlan_id"`
	Protocol string    `json:"protocol"`
	Prefix   string    `json:"prefix"`
	InDFZ    bool      `json:"in_dfz"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Status   string    `json:"status"`
}

// NetworkIxLan represents a PeeringDB network-IX LAN association (netixlan).
type NetworkIxLan struct {
	ID          int       `json:"id"`
	NetID       int       `json:"net_id"`
	IXID        int       `json:"ix_id"`
	IXLanID     int       `json:"ixlan_id"`
	Name        string    `json:"name"`
	Notes       string    `json:"notes"`
	Speed       int       `json:"speed"`
	ASN         int       `json:"asn"`
	IPAddr4     *string   `json:"ipaddr4"`
	IPAddr6     *string   `json:"ipaddr6"`
	IsRSPeer    bool      `json:"is_rs_peer"`
	BFDSupport  bool      `json:"bfd_support"`
	Operational bool      `json:"operational"`
	NetSideID   *int      `json:"net_side_id"`
	IXSideID    *int      `json:"ix_side_id"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Status      string    `json:"status"`
}

// NetworkFacility represents a PeeringDB network-facility association (netfac).
type NetworkFacility struct {
	ID       int       `json:"id"`
	NetID    int       `json:"net_id"`
	FacID    int       `json:"fac_id"`
	Name     string    `json:"name"`
	City     string    `json:"city"`
	Country  string    `json:"country"`
	LocalASN int       `json:"local_asn"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
	Status   string    `json:"status"`
}

// IxFacility represents a PeeringDB IX-facility association (ixfac).
type IxFacility struct {
	ID      int       `json:"id"`
	IXID    int       `json:"ix_id"`
	FacID   int       `json:"fac_id"`
	Name    string    `json:"name"`
	City    string    `json:"city"`
	Country string    `json:"country"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Status  string    `json:"status"`
}

// Carrier represents a PeeringDB carrier.
type Carrier struct {
	ID          int           `json:"id"`
	OrgID       int           `json:"org_id"`
	OrgName     string        `json:"org_name"`
	Name        string        `json:"name"`
	Aka         string        `json:"aka"`
	NameLong    string        `json:"name_long"`
	Website     string        `json:"website"`
	SocialMedia []SocialMedia `json:"social_media"`
	Notes       string        `json:"notes"`
	FacCount    int           `json:"fac_count"`
	Logo        *string       `json:"logo"`
	Created     time.Time     `json:"created"`
	Updated     time.Time     `json:"updated"`
	Status      string        `json:"status"`
}

// CarrierFacility represents a PeeringDB carrier-facility association (carrierfac).
type CarrierFacility struct {
	ID        int       `json:"id"`
	CarrierID int       `json:"carrier_id"`
	FacID     int       `json:"fac_id"`
	Name      string    `json:"name"`
	Created   time.Time `json:"created"`
	Updated   time.Time `json:"updated"`
	Status    string    `json:"status"`
}

// Campus represents a PeeringDB campus.
type Campus struct {
	ID          int           `json:"id"`
	OrgID       int           `json:"org_id"`
	OrgName     string        `json:"org_name"`
	Name        string        `json:"name"`
	NameLong    *string       `json:"name_long"`
	Aka         *string       `json:"aka"`
	Website     string        `json:"website"`
	SocialMedia []SocialMedia `json:"social_media"`
	Notes       string        `json:"notes"`
	Country     string        `json:"country"`
	City        string        `json:"city"`
	Zipcode     string        `json:"zipcode"`
	State       string        `json:"state"`
	Logo        *string       `json:"logo"`
	Created     time.Time     `json:"created"`
	Updated     time.Time     `json:"updated"`
	Status      string        `json:"status"`
}
