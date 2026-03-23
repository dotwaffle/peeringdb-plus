package pdbcompat

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/schema"
	"github.com/dotwaffle/peeringdb-plus/internal/peeringdb"
)

func TestSerializerNetworkFromEnt(t *testing.T) {
	t.Parallel()

	orgID := 42
	infoPfx4 := 1000
	infoPfx6 := 200
	statusDash := "https://status.example.com"
	rirStatus := "ALLOCATED"
	rirUpdated := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	logo := "https://example.com/logo.png"
	nixUpdated := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	nfacUpdated := time.Date(2025, 7, 2, 0, 0, 0, 0, time.UTC)
	pocUpdated := time.Date(2025, 7, 3, 0, 0, 0, 0, time.UTC)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	entNet := &ent.Network{
		ID:                      13335,
		OrgID:                   &orgID,
		Name:                    "Cloudflare, Inc.",
		Aka:                     "Cloudflare",
		NameLong:                "Cloudflare, Inc. - Global Network",
		Website:                 "https://cloudflare.com",
		SocialMedia:             []schema.SocialMedia{{Service: "twitter", Identifier: "@cloudflare"}},
		Asn:                     13335,
		LookingGlass:            "https://lg.cloudflare.com",
		RouteServer:             "",
		IrrAsSet:                "AS-CLOUDFLARE",
		InfoType:                "Content",
		InfoTypes:               []string{"Content"},
		InfoPrefixes4:           &infoPfx4,
		InfoPrefixes6:           &infoPfx6,
		InfoTraffic:             "100+ Tbps",
		InfoRatio:               "Mostly Outbound",
		InfoScope:               "Global",
		InfoUnicast:             true,
		InfoMulticast:           false,
		InfoIpv6:                true,
		InfoNeverViaRouteServers: false,
		Notes:                   "Test notes",
		PolicyURL:               "https://cloudflare.com/peering",
		PolicyGeneral:           "Open",
		PolicyLocations:         "Required - US",
		PolicyRatio:             false,
		PolicyContracts:         "Not Required",
		AllowIxpUpdate:          true,
		StatusDashboard:         &statusDash,
		RirStatus:               &rirStatus,
		RirStatusUpdated:        &rirUpdated,
		Logo:                    &logo,
		IxCount:                 300,
		FacCount:                200,
		NetixlanUpdated:         &nixUpdated,
		NetfacUpdated:           &nfacUpdated,
		PocUpdated:              &pocUpdated,
		Created:                 now,
		Updated:                 now,
		Status:                  "ok",
	}

	got := networkFromEnt(entNet)

	// Verify key field mappings.
	if got.ID != 13335 {
		t.Errorf("ID = %d, want 13335", got.ID)
	}
	if got.OrgID != 42 {
		t.Errorf("OrgID = %d, want 42", got.OrgID)
	}
	if got.ASN != 13335 {
		t.Errorf("ASN = %d, want 13335", got.ASN)
	}
	if got.Name != "Cloudflare, Inc." {
		t.Errorf("Name = %q, want %q", got.Name, "Cloudflare, Inc.")
	}
	if got.IRRASSet != "AS-CLOUDFLARE" {
		t.Errorf("IRRASSet = %q, want %q", got.IRRASSet, "AS-CLOUDFLARE")
	}
	if got.InfoUnicast != true {
		t.Errorf("InfoUnicast = %v, want true", got.InfoUnicast)
	}
	if got.InfoIPv6 != true {
		t.Errorf("InfoIPv6 = %v, want true", got.InfoIPv6)
	}
	if got.InfoNeverViaRouteServer != false {
		t.Errorf("InfoNeverViaRouteServer = %v, want false", got.InfoNeverViaRouteServer)
	}
	if got.IXCount != 300 {
		t.Errorf("IXCount = %d, want 300", got.IXCount)
	}
	if len(got.SocialMedia) != 1 || got.SocialMedia[0].Service != "twitter" {
		t.Errorf("SocialMedia = %v, want [{twitter @cloudflare}]", got.SocialMedia)
	}
	if *got.InfoPrefixes4 != 1000 {
		t.Errorf("InfoPrefixes4 = %v, want 1000", got.InfoPrefixes4)
	}
	if *got.StatusDashboard != statusDash {
		t.Errorf("StatusDashboard = %v, want %q", got.StatusDashboard, statusDash)
	}

	// Verify JSON marshaling includes all fields (no omitempty gaps).
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// These fields MUST be present in JSON output, including zero values.
	requiredFields := []string{
		"id", "org_id", "name", "aka", "name_long", "website", "social_media",
		"asn", "looking_glass", "route_server", "irr_as_set", "info_type",
		"info_types", "info_prefixes4", "info_prefixes6", "info_traffic",
		"info_ratio", "info_scope", "info_unicast", "info_multicast", "info_ipv6",
		"info_never_via_route_servers", "notes", "policy_url", "policy_general",
		"policy_locations", "policy_ratio", "policy_contracts", "allow_ixp_update",
		"status_dashboard", "rir_status", "rir_status_updated", "logo",
		"ix_count", "fac_count", "netixlan_updated", "netfac_updated",
		"poc_updated", "created", "updated", "status",
	}
	for _, f := range requiredFields {
		if _, ok := m[f]; !ok {
			t.Errorf("JSON missing required field %q", f)
		}
	}
}

func TestSerializerOrganizationFromEnt(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	lat := 37.7749
	lng := -122.4194

	entOrg := &ent.Organization{
		ID:          1,
		Name:        "Test Org",
		Aka:         "TO",
		NameLong:    "Test Organization LLC",
		Website:     "https://testorg.com",
		SocialMedia: []schema.SocialMedia{},
		Notes:       "",
		Logo:        nil,
		Address1:    "123 Main St",
		Address2:    "",
		City:        "San Francisco",
		State:       "CA",
		Country:     "US",
		Zipcode:     "94105",
		Suite:       "",
		Floor:       "",
		Latitude:    &lat,
		Longitude:   &lng,
		Created:     now,
		Updated:     now,
		Status:      "ok",
	}

	got := organizationFromEnt(entOrg)

	if got.ID != 1 {
		t.Errorf("ID = %d, want 1", got.ID)
	}
	if got.Name != "Test Org" {
		t.Errorf("Name = %q, want %q", got.Name, "Test Org")
	}
	if got.City != "San Francisco" {
		t.Errorf("City = %q, want %q", got.City, "San Francisco")
	}
	if *got.Latitude != 37.7749 {
		t.Errorf("Latitude = %v, want 37.7749", *got.Latitude)
	}
	if got.Logo != nil {
		t.Errorf("Logo = %v, want nil", got.Logo)
	}

	// Verify JSON has all fields present.
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	requiredFields := []string{
		"id", "name", "aka", "name_long", "website", "social_media",
		"notes", "address1", "address2", "city", "state", "country",
		"zipcode", "suite", "floor", "latitude", "longitude",
		"created", "updated", "status",
	}
	for _, f := range requiredFields {
		if _, ok := m[f]; !ok {
			t.Errorf("JSON missing required field %q", f)
		}
	}

	// Verify empty string fields are present (not omitted).
	if v, ok := m["aka"]; !ok {
		t.Errorf("JSON missing field 'aka'")
	} else if v != "TO" {
		t.Errorf("aka = %v, want %q", v, "TO")
	}
}

func TestSerializerNetworkZeroValues(t *testing.T) {
	t.Parallel()

	// Network with nil optional fields -- derefInt should return 0.
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entNet := &ent.Network{
		ID:      1,
		OrgID:   nil, // nil pointer
		Name:    "Test",
		Asn:     64512,
		Created: now,
		Updated: now,
		Status:  "ok",
	}

	got := networkFromEnt(entNet)

	if got.OrgID != 0 {
		t.Errorf("OrgID with nil pointer = %d, want 0", got.OrgID)
	}
	if got.InfoPrefixes4 != nil {
		t.Errorf("InfoPrefixes4 should be nil, got %v", got.InfoPrefixes4)
	}
}

func TestSerializerNetworkJSON_OrgIDFieldName(t *testing.T) {
	t.Parallel()

	orgID := 42
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	entNet := &ent.Network{
		ID:      1,
		OrgID:   &orgID,
		Name:    "Test",
		Asn:     64512,
		Created: now,
		Updated: now,
		Status:  "ok",
	}

	got := networkFromEnt(entNet)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Must be "org_id", not "org".
	if _, ok := m["org_id"]; !ok {
		t.Error("JSON must contain 'org_id' field, not 'org'")
	}
	if _, ok := m["org"]; ok {
		t.Error("JSON must not contain 'org' field (should be 'org_id')")
	}
}

// TestSerializerAllTypesCompile ensures all 13 serializer functions compile
// and are callable.
func TestSerializerAllTypesCompile(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	// Verify each function exists and returns the correct type.
	_ = organizationFromEnt(&ent.Organization{Created: now, Updated: now})
	_ = networkFromEnt(&ent.Network{Created: now, Updated: now})
	_ = facilityFromEnt(&ent.Facility{Created: now, Updated: now})
	_ = internetExchangeFromEnt(&ent.InternetExchange{Created: now, Updated: now})
	_ = pocFromEnt(&ent.Poc{Created: now, Updated: now})
	_ = ixLanFromEnt(&ent.IxLan{Created: now, Updated: now})
	_ = ixPrefixFromEnt(&ent.IxPrefix{Created: now, Updated: now})
	_ = networkIxLanFromEnt(&ent.NetworkIxLan{Created: now, Updated: now})
	_ = networkFacilityFromEnt(&ent.NetworkFacility{Created: now, Updated: now})
	_ = ixFacilityFromEnt(&ent.IxFacility{Created: now, Updated: now})
	_ = carrierFromEnt(&ent.Carrier{Created: now, Updated: now})
	_ = carrierFacilityFromEnt(&ent.CarrierFacility{Created: now, Updated: now})
	_ = campusFromEnt(&ent.Campus{Created: now, Updated: now})

	// Verify slice mappers compile.
	_ = organizationsFromEnt(nil)
	_ = networksFromEnt(nil)
	_ = facilitiesFromEnt(nil)
	_ = internetExchangesFromEnt(nil)
	_ = pocsFromEnt(nil)
	_ = ixLansFromEnt(nil)
	_ = ixPrefixesFromEnt(nil)
	_ = networkIxLansFromEnt(nil)
	_ = networkFacilitiesFromEnt(nil)
	_ = ixFacilitiesFromEnt(nil)
	_ = carriersFromEnt(nil)
	_ = carrierFacilitiesFromEnt(nil)
	_ = campusesFromEnt(nil)
}

// TestSerializerSocialMediaConversion verifies schema.SocialMedia to
// peeringdb.SocialMedia conversion.
func TestSerializerSocialMediaConversion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []schema.SocialMedia
		want  []peeringdb.SocialMedia
	}{
		{
			name:  "nil input returns empty slice",
			input: nil,
			want:  []peeringdb.SocialMedia{},
		},
		{
			name:  "empty slice returns empty slice",
			input: []schema.SocialMedia{},
			want:  []peeringdb.SocialMedia{},
		},
		{
			name: "single entry",
			input: []schema.SocialMedia{
				{Service: "twitter", Identifier: "@test"},
			},
			want: []peeringdb.SocialMedia{
				{Service: "twitter", Identifier: "@test"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := socialMediaFromSchema(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
