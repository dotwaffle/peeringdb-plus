package templates

// NetworkDetail holds display data for a network detail page.
type NetworkDetail struct {
	// ID is the network's PeeringDB internal identifier.
	ID int
	// ASN is the Autonomous System Number.
	ASN int
	// Name is the network's display name.
	Name string
	// NameLong is the network's long-form name.
	NameLong string
	// AKA is the "also known as" alias.
	AKA string
	// Website is the network's website URL.
	Website string
	// OrgName is the name of the parent organization.
	OrgName string
	// OrgID is the parent organization's PeeringDB ID.
	OrgID int
	// IRRAsSet is the IRR AS-SET identifier.
	IRRAsSet string
	// InfoType is the network type classification.
	InfoType string
	// InfoScope is the geographic scope.
	InfoScope string
	// InfoTraffic is the traffic level.
	InfoTraffic string
	// InfoRatio is the traffic ratio.
	InfoRatio string
	// InfoUnicast indicates unicast support.
	InfoUnicast bool
	// InfoMulticast indicates multicast support.
	InfoMulticast bool
	// InfoIPv6 indicates IPv6 support.
	InfoIPv6 bool
	// InfoPrefixes4 is the IPv4 prefix count.
	InfoPrefixes4 int
	// InfoPrefixes6 is the IPv6 prefix count.
	InfoPrefixes6 int
	// LookingGlass is the looking glass URL.
	LookingGlass string
	// RouteServer is the route server URL.
	RouteServer string
	// PolicyGeneral is the general peering policy.
	PolicyGeneral string
	// PolicyURL is the peering policy URL.
	PolicyURL string
	// Notes contains freeform notes.
	Notes string
	// Status is the record status.
	Status string
	// IXCount is the pre-computed count of IX presences.
	IXCount int
	// FacCount is the pre-computed count of facility presences.
	FacCount int
	// PocCount is the count of contacts (requires query).
	PocCount int
	// AggregateBW is the total bandwidth in Mbps across all IX presences (for header display).
	AggregateBW int
	// IXPresences holds eager-loaded IX presence rows for terminal/JSON rendering.
	// Nil for web UI requests that lazy-load via htmx fragments.
	IXPresences []NetworkIXLanRow `json:"ixPresences,omitempty"`
	// FacPresences holds eager-loaded facility presence rows for terminal/JSON rendering.
	// Nil for web UI requests that lazy-load via htmx fragments.
	FacPresences []NetworkFacRow `json:"facPresences,omitempty"`
}

// IXDetail holds display data for an IXP detail page.
type IXDetail struct {
	// ID is the IXP's PeeringDB identifier.
	ID int
	// Name is the IXP's display name.
	Name string
	// NameLong is the IXP's long-form name.
	NameLong string
	// AKA is the "also known as" alias.
	AKA string
	// Website is the IXP's website URL.
	Website string
	// OrgName is the name of the parent organization.
	OrgName string
	// OrgID is the parent organization's PeeringDB ID.
	OrgID int
	// City is the IXP's city.
	City string
	// Country is the IXP's country code.
	Country string
	// RegionContinent is the IXP's region/continent.
	RegionContinent string
	// Media is the exchange media type.
	Media string
	// ProtoUnicast indicates unicast support.
	ProtoUnicast bool
	// ProtoMulticast indicates multicast support.
	ProtoMulticast bool
	// ProtoIPv6 indicates IPv6 support.
	ProtoIPv6 bool
	// Notes contains freeform notes.
	Notes string
	// Status is the record status.
	Status string
	// NetCount is the pre-computed count of network participants.
	NetCount int
	// FacCount is the pre-computed count of facilities.
	FacCount int
	// PrefixCount is the count of peering LAN prefixes (requires IxLan traversal).
	PrefixCount int
	// AggregateBW is the total bandwidth in Mbps across all participants (for header display).
	AggregateBW int
}

// FacilityDetail holds display data for a facility detail page.
type FacilityDetail struct {
	// ID is the facility's PeeringDB identifier.
	ID int
	// Name is the facility's display name.
	Name string
	// NameLong is the facility's long-form name.
	NameLong string
	// AKA is the "also known as" alias.
	AKA string
	// Website is the facility's website URL.
	Website string
	// OrgName is the name of the parent organization.
	OrgName string
	// OrgID is the parent organization's PeeringDB ID.
	OrgID int
	// CampusName is the name of the parent campus, if any.
	CampusName string
	// CampusID is the parent campus's PeeringDB ID.
	CampusID int
	// Address1 is the primary street address.
	Address1 string
	// Address2 is the secondary address line.
	Address2 string
	// City is the facility's city.
	City string
	// State is the facility's state or province.
	State string
	// Country is the facility's country code.
	Country string
	// Zipcode is the postal code.
	Zipcode string
	// RegionContinent is the facility's region/continent.
	RegionContinent string
	// CLLI is the CLLI code.
	CLLI string
	// Notes contains freeform notes.
	Notes string
	// Status is the record status.
	Status string
	// NetCount is the pre-computed count of networks present.
	NetCount int
	// IXCount is the pre-computed count of IXPs present.
	IXCount int
	// CarrierCount is the pre-computed count of carriers.
	CarrierCount int
}

// OrgDetail holds display data for an organization detail page.
type OrgDetail struct {
	// ID is the organization's PeeringDB identifier.
	ID int
	// Name is the organization's display name.
	Name string
	// NameLong is the organization's long-form name.
	NameLong string
	// AKA is the "also known as" alias.
	AKA string
	// Website is the organization's website URL.
	Website string
	// Address1 is the primary street address.
	Address1 string
	// Address2 is the secondary address line.
	Address2 string
	// City is the organization's city.
	City string
	// State is the organization's state or province.
	State string
	// Country is the organization's country code.
	Country string
	// Zipcode is the postal code.
	Zipcode string
	// Notes contains freeform notes.
	Notes string
	// Status is the record status.
	Status string
	// NetCount is the pre-computed count of networks.
	NetCount int
	// FacCount is the pre-computed count of facilities.
	FacCount int
	// IXCount is the count of IXPs (requires query).
	IXCount int
	// CampusCount is the count of campuses (requires query).
	CampusCount int
	// CarrierCount is the count of carriers (requires query).
	CarrierCount int
}

// CampusDetail holds display data for a campus detail page.
type CampusDetail struct {
	// ID is the campus's PeeringDB identifier.
	ID int
	// Name is the campus's display name.
	Name string
	// NameLong is the campus's long-form name.
	NameLong string
	// AKA is the "also known as" alias.
	AKA string
	// Website is the campus's website URL.
	Website string
	// OrgName is the name of the parent organization.
	OrgName string
	// OrgID is the parent organization's PeeringDB ID.
	OrgID int
	// City is the campus's city.
	City string
	// State is the campus's state or province.
	State string
	// Country is the campus's country code.
	Country string
	// Zipcode is the postal code.
	Zipcode string
	// Notes contains freeform notes.
	Notes string
	// Status is the record status.
	Status string
	// FacCount is the count of facilities (requires query).
	FacCount int
}

// CarrierDetail holds display data for a carrier detail page.
type CarrierDetail struct {
	// ID is the carrier's PeeringDB identifier.
	ID int
	// Name is the carrier's display name.
	Name string
	// NameLong is the carrier's long-form name.
	NameLong string
	// AKA is the "also known as" alias.
	AKA string
	// Website is the carrier's website URL.
	Website string
	// OrgName is the name of the parent organization.
	OrgName string
	// OrgID is the parent organization's PeeringDB ID.
	OrgID int
	// Notes contains freeform notes.
	Notes string
	// Status is the record status.
	Status string
	// FacCount is the pre-computed count of facilities.
	FacCount int
}

// NetworkIXLanRow holds display data for a network's IX presence row.
type NetworkIXLanRow struct {
	// IXName is the exchange name.
	IXName string
	// IXID is the exchange's PeeringDB ID for cross-linking.
	IXID int
	// Speed is the port speed in Mbps.
	Speed int
	// IPAddr4 is the IPv4 peering address.
	IPAddr4 string
	// IPAddr6 is the IPv6 peering address.
	IPAddr6 string
	// IsRSPeer indicates route server peering.
	IsRSPeer bool
}

// NetworkFacRow holds display data for a network's facility presence row.
type NetworkFacRow struct {
	// FacName is the facility name.
	FacName string
	// FacID is the facility's PeeringDB ID for cross-linking.
	FacID int
	// LocalASN is the local ASN at this facility.
	LocalASN int
	// City is the facility's city.
	City string
	// Country is the facility's country code.
	Country string
}

// ContactRow holds display data for a network contact (poc) row.
type ContactRow struct {
	// Name is the contact's name.
	Name string
	// Role is the contact's role (e.g. "NOC", "Policy").
	Role string
	// Email is the contact's email address.
	Email string
	// Phone is the contact's phone number.
	Phone string
	// URL is the contact's URL.
	URL string
}

// IXParticipantRow holds display data for an IXP participant row.
type IXParticipantRow struct {
	// NetName is the participant network's name.
	NetName string
	// ASN is the participant's Autonomous System Number.
	ASN int
	// Speed is the port speed in Mbps.
	Speed int
	// IPAddr4 is the IPv4 peering address.
	IPAddr4 string
	// IPAddr6 is the IPv6 peering address.
	IPAddr6 string
	// IsRSPeer indicates route server peering.
	IsRSPeer bool
}

// IXFacilityRow holds display data for an IXP facility row.
type IXFacilityRow struct {
	// FacName is the facility name.
	FacName string
	// FacID is the facility's PeeringDB ID for cross-linking.
	FacID int
	// City is the facility's city.
	City string
	// Country is the facility's country code.
	Country string
}

// IXPrefixRow holds display data for an IXP prefix row.
type IXPrefixRow struct {
	// Prefix is the peering LAN prefix.
	Prefix string
	// Protocol is the address family ("IPv4" or "IPv6").
	Protocol string
	// InDFZ indicates whether the prefix is in the default-free zone.
	InDFZ bool
}

// FacNetworkRow holds display data for a facility's network row.
type FacNetworkRow struct {
	// NetName is the network's name.
	NetName string
	// ASN is the network's Autonomous System Number.
	ASN int
}

// FacIXRow holds display data for a facility's IXP row.
type FacIXRow struct {
	// IXName is the exchange name.
	IXName string
	// IXID is the exchange's PeeringDB ID for cross-linking.
	IXID int
}

// FacCarrierRow holds display data for a facility's carrier row.
type FacCarrierRow struct {
	// CarrierName is the carrier name.
	CarrierName string
	// CarrierID is the carrier's PeeringDB ID for cross-linking.
	CarrierID int
}

// OrgNetworkRow holds display data for an org's network row.
type OrgNetworkRow struct {
	// NetName is the network's name.
	NetName string
	// ASN is the network's Autonomous System Number.
	ASN int
}

// OrgIXRow holds display data for an org's IXP row.
type OrgIXRow struct {
	// IXName is the exchange name.
	IXName string
	// IXID is the exchange's PeeringDB ID for cross-linking.
	IXID int
}

// OrgFacilityRow holds display data for an org's facility row.
type OrgFacilityRow struct {
	// FacName is the facility name.
	FacName string
	// FacID is the facility's PeeringDB ID for cross-linking.
	FacID int
	// City is the facility's city.
	City string
	// Country is the facility's country code.
	Country string
}

// OrgCampusRow holds display data for an org's campus row.
type OrgCampusRow struct {
	// CampusName is the campus name.
	CampusName string
	// CampusID is the campus's PeeringDB ID for cross-linking.
	CampusID int
}

// OrgCarrierRow holds display data for an org's carrier row.
type OrgCarrierRow struct {
	// CarrierName is the carrier name.
	CarrierName string
	// CarrierID is the carrier's PeeringDB ID for cross-linking.
	CarrierID int
}

// CampusFacilityRow holds display data for a campus's facility row.
type CampusFacilityRow struct {
	// FacName is the facility name.
	FacName string
	// FacID is the facility's PeeringDB ID for cross-linking.
	FacID int
	// City is the facility's city.
	City string
	// Country is the facility's country code.
	Country string
}

// CarrierFacilityRow holds display data for a carrier's facility row.
type CarrierFacilityRow struct {
	// FacName is the facility name.
	FacName string
	// FacID is the facility's PeeringDB ID for cross-linking.
	FacID int
}
