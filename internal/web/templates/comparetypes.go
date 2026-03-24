package templates

// CompareData holds the full comparison result for template rendering.
type CompareData struct {
	// NetA is the first network's summary info.
	NetA CompareNetwork
	// NetB is the second network's summary info.
	NetB CompareNetwork
	// SharedIXPs lists IXPs where both networks are present.
	SharedIXPs []CompareIXP
	// SharedFacilities lists facilities where both networks are present.
	SharedFacilities []CompareFacility
	// SharedCampuses lists campuses where both networks have facility presence.
	SharedCampuses []CompareCampus
	// AllIXPs lists all IXPs for both networks with a shared flag (for full view).
	AllIXPs []CompareIXP
	// AllFacilities lists all facilities for both networks with shared flag (for full view).
	AllFacilities []CompareFacility
	// ViewMode is "shared" (default) or "full".
	ViewMode string
}

// CompareNetwork holds summary info for one side of the comparison.
type CompareNetwork struct {
	// ASN is the Autonomous System Number.
	ASN int
	// Name is the network's display name.
	Name string
	// ID is the network's PeeringDB internal identifier.
	ID int
}

// CompareIXP holds comparison data for a single IXP showing both networks' presence info.
type CompareIXP struct {
	// IXID is the exchange's PeeringDB ID for cross-linking.
	IXID int
	// IXName is the exchange display name.
	IXName string
	// Shared indicates both networks are present at this IXP.
	Shared bool
	// NetA holds network A's presence details (nil if not present).
	NetA *CompareIXPresence
	// NetB holds network B's presence details (nil if not present).
	NetB *CompareIXPresence
}

// CompareIXPresence holds one network's presence details at an IXP.
type CompareIXPresence struct {
	// Speed is the port speed in Mbps.
	Speed int
	// IPAddr4 is the IPv4 peering address.
	IPAddr4 string
	// IPAddr6 is the IPv6 peering address.
	IPAddr6 string
	// IsRSPeer indicates route server peering.
	IsRSPeer bool
	// Operational indicates whether the connection is operational.
	Operational bool
}

// CompareFacility holds comparison data for a single facility.
type CompareFacility struct {
	// FacID is the facility's PeeringDB ID for cross-linking.
	FacID int
	// FacName is the facility display name.
	FacName string
	// City is the facility's city.
	City string
	// Country is the facility's country code.
	Country string
	// Shared indicates both networks are present at this facility.
	Shared bool
	// NetA holds network A's presence info (nil if not present).
	NetA *CompareFacPresence
	// NetB holds network B's presence info (nil if not present).
	NetB *CompareFacPresence
}

// CompareFacPresence holds one network's presence details at a facility.
type CompareFacPresence struct {
	// LocalASN is the local ASN used at this facility.
	LocalASN int
}

// CompareCampus holds comparison data for a shared campus.
type CompareCampus struct {
	// CampusID is the campus's PeeringDB ID for cross-linking.
	CampusID int
	// CampusName is the campus display name.
	CampusName string
	// SharedFacilities lists facilities within this campus where both networks are present.
	SharedFacilities []CompareCampusFacility
}

// CompareCampusFacility holds a facility within a shared campus.
type CompareCampusFacility struct {
	// FacID is the facility's PeeringDB ID for cross-linking.
	FacID int
	// FacName is the facility display name.
	FacName string
}
