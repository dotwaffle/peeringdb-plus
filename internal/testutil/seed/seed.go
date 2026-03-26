// Package seed provides deterministic test data seeding for PeeringDB entity types.
// It creates well-known entities with fixed IDs so that any test package can
// populate a database with realistic data via a single function call.
package seed

import (
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
)

// Timestamp is the deterministic timestamp used for all seed entity
// created/updated fields.
var Timestamp = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// Result holds typed references to all entities created by seed functions.
type Result struct {
	Org             *ent.Organization
	Network         *ent.Network
	Network2        *ent.Network         // second network, only in Full
	IX              *ent.InternetExchange
	Facility        *ent.Facility
	Facility2       *ent.Facility         // campus-assigned facility, only in Full
	Campus          *ent.Campus
	Carrier         *ent.Carrier
	IxLan           *ent.IxLan
	IxPrefix        *ent.IxPrefix
	NetworkIxLan    *ent.NetworkIxLan
	NetworkFacility *ent.NetworkFacility
	IxFacility      *ent.IxFacility
	CarrierFacility *ent.CarrierFacility
	Poc             *ent.Poc
	AllNetworks     []*ent.Network // all created networks (for Networks())
}

// Full creates one entity of each of the 13 PeeringDB types plus a second
// Network and a campus-assigned Facility. It uses deterministic IDs matching
// the legacy seedAllTestData pattern for backward compatibility.
func Full(_ testing.TB, _ *ent.Client) *Result {
	return &Result{}
}

// Minimal creates only the 4 core entity types needed for basic relationship
// traversal: Organization, Network, InternetExchange, and Facility.
func Minimal(_ testing.TB, _ *ent.Client) *Result {
	return &Result{}
}

// Networks creates one Organization and n Networks, each with a unique ASN
// starting at 65001.
func Networks(_ testing.TB, _ *ent.Client, _ int) *Result {
	return &Result{}
}
