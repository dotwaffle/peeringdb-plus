package sync

import "github.com/dotwaffle/peeringdb-plus/internal/peeringdb"

// Status filter functions for each PeeringDB type per D-32.
// Each removes objects with status="deleted".

func filterOrgsByStatus(items []peeringdb.Organization) []peeringdb.Organization {
	result := make([]peeringdb.Organization, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterCampusesByStatus(items []peeringdb.Campus) []peeringdb.Campus {
	result := make([]peeringdb.Campus, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterFacilitiesByStatus(items []peeringdb.Facility) []peeringdb.Facility {
	result := make([]peeringdb.Facility, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterCarriersByStatus(items []peeringdb.Carrier) []peeringdb.Carrier {
	result := make([]peeringdb.Carrier, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterCarrierFacilitiesByStatus(items []peeringdb.CarrierFacility) []peeringdb.CarrierFacility {
	result := make([]peeringdb.CarrierFacility, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterInternetExchangesByStatus(items []peeringdb.InternetExchange) []peeringdb.InternetExchange {
	result := make([]peeringdb.InternetExchange, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterIxLansByStatus(items []peeringdb.IxLan) []peeringdb.IxLan {
	result := make([]peeringdb.IxLan, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterIxPrefixesByStatus(items []peeringdb.IxPrefix) []peeringdb.IxPrefix {
	result := make([]peeringdb.IxPrefix, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterIxFacilitiesByStatus(items []peeringdb.IxFacility) []peeringdb.IxFacility {
	result := make([]peeringdb.IxFacility, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterNetworksByStatus(items []peeringdb.Network) []peeringdb.Network {
	result := make([]peeringdb.Network, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterPocsByStatus(items []peeringdb.Poc) []peeringdb.Poc {
	result := make([]peeringdb.Poc, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterNetworkFacilitiesByStatus(items []peeringdb.NetworkFacility) []peeringdb.NetworkFacility {
	result := make([]peeringdb.NetworkFacility, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}

func filterNetworkIxLansByStatus(items []peeringdb.NetworkIxLan) []peeringdb.NetworkIxLan {
	result := make([]peeringdb.NetworkIxLan, 0, len(items))
	for _, item := range items {
		if item.Status != "deleted" {
			result = append(result, item)
		}
	}
	return result
}
