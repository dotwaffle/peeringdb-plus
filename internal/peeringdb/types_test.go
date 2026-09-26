package peeringdb

import (
	"encoding/json"
	"testing"
	"time"
)

// Response mirrors the PeeringDB API envelope. Test-only since the
// production client streams elements (StreamAll) instead of decoding
// whole envelopes; the entity-struct decode tests and stream_test.go's
// legacy-path comparisons still want the envelope shape.
type Response[T any] struct {
	Meta json.RawMessage `json:"meta"`
	Data []T             `json:"data"`
}

func TestResponseDeserialization(t *testing.T) {
	t.Parallel()

	t.Run("Response[Organization] with meta and data", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"meta": {},
			"data": [
				{
					"id": 1,
					"name": "Test Org",
					"aka": "",
					"name_long": "Test Organization Inc",
					"website": "https://example.com",
					"social_media": [{"service": "twitter", "identifier": "@test"}],
					"notes": "some notes",
					"logo": null,
					"address1": "123 Main St",
					"address2": "",
					"city": "Anytown",
					"state": "CA",
					"country": "US",
					"zipcode": "12345",
					"suite": "",
					"floor": "",
					"latitude": 37.7749,
					"longitude": -122.4194,
					"created": "2020-01-01T00:00:00Z",
					"updated": "2024-06-15T12:30:00Z",
					"status": "ok"
				}
			]
		}`

		var resp Response[Organization]
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unmarshal Response[Organization]: %v", err)
		}

		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 item, got %d", len(resp.Data))
		}

		org := resp.Data[0]
		if org.ID != 1 {
			t.Errorf("ID = %d, want 1", org.ID)
		}
		if org.Name != "Test Org" {
			t.Errorf("Name = %q, want %q", org.Name, "Test Org")
		}
		if org.NameLong != "Test Organization Inc" {
			t.Errorf("NameLong = %q, want %q", org.NameLong, "Test Organization Inc")
		}
		if org.Logo != nil {
			t.Errorf("Logo = %v, want nil", org.Logo)
		}
		if org.Latitude == nil || *org.Latitude != 37.7749 {
			t.Errorf("Latitude = %v, want 37.7749", org.Latitude)
		}
		if org.Longitude == nil || *org.Longitude != -122.4194 {
			t.Errorf("Longitude = %v, want -122.4194", org.Longitude)
		}
		if len(org.SocialMedia) != 1 {
			t.Fatalf("SocialMedia length = %d, want 1", len(org.SocialMedia))
		}
		if org.SocialMedia[0].Service != "twitter" {
			t.Errorf("SocialMedia[0].Service = %q, want %q", org.SocialMedia[0].Service, "twitter")
		}
		if org.Status != "ok" {
			t.Errorf("Status = %q, want %q", org.Status, "ok")
		}
		expectedCreated := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		if !org.Created.Equal(expectedCreated) {
			t.Errorf("Created = %v, want %v", org.Created, expectedCreated)
		}
	})

	t.Run("Response[json.RawMessage] for generic fetching", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"meta": {},
			"data": [
				{"id": 1, "name": "item1"},
				{"id": 2, "name": "item2"}
			]
		}`

		var resp Response[json.RawMessage]
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unmarshal Response[json.RawMessage]: %v", err)
		}

		if len(resp.Data) != 2 {
			t.Fatalf("expected 2 items, got %d", len(resp.Data))
		}

		// Verify we can unmarshal the raw messages
		var item struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := json.Unmarshal(resp.Data[0], &item); err != nil {
			t.Fatalf("unmarshal item: %v", err)
		}
		if item.ID != 1 || item.Name != "item1" {
			t.Errorf("item = %+v, want {ID:1 Name:item1}", item)
		}
	})

	t.Run("unknown fields silently ignored", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"meta": {},
			"data": [
				{
					"id": 42,
					"name": "Test Org",
					"aka": "",
					"name_long": "",
					"website": "",
					"social_media": [],
					"notes": "",
					"logo": null,
					"address1": "",
					"address2": "",
					"city": "",
					"state": "",
					"country": "",
					"zipcode": "",
					"suite": "",
					"floor": "",
					"latitude": null,
					"longitude": null,
					"created": "2020-01-01T00:00:00Z",
					"updated": "2020-01-01T00:00:00Z",
					"status": "ok",
					"brand_new_field": "should be ignored",
					"another_unknown": 999
				}
			]
		}`

		var resp Response[Organization]
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unknown fields should be silently ignored, got error: %v", err)
		}

		if resp.Data[0].ID != 42 {
			t.Errorf("ID = %d, want 42", resp.Data[0].ID)
		}
	})

	t.Run("null fields deserialize correctly", func(t *testing.T) {
		t.Parallel()

		raw := `{
			"meta": {},
			"data": [
				{
					"id": 100,
					"org_id": 1,
					"name": "Test Net",
					"aka": "",
					"name_long": "",
					"website": "",
					"social_media": [],
					"asn": 65001,
					"looking_glass": "",
					"route_server": "",
					"irr_as_set": "",
					"info_type": "",
					"info_types": [],
					"info_prefixes4": null,
					"info_prefixes6": null,
					"info_traffic": "",
					"info_ratio": "",
					"info_scope": "",
					"info_unicast": false,
					"info_multicast": false,
					"info_ipv6": false,
					"info_never_via_route_servers": false,
					"notes": "",
					"policy_url": "",
					"policy_general": "",
					"policy_locations": "",
					"policy_ratio": false,
					"policy_contracts": "",
					"allow_ixp_update": false,
					"status_dashboard": null,
					"rir_status": null,
					"rir_status_updated": null,
					"logo": null,
					"ix_count": 5,
					"fac_count": 3,
					"netixlan_updated": null,
					"netfac_updated": null,
					"poc_updated": null,
					"created": "2020-01-01T00:00:00Z",
					"updated": "2024-01-01T00:00:00Z",
					"status": "ok"
				}
			]
		}`

		var resp Response[Network]
		if err := json.Unmarshal([]byte(raw), &resp); err != nil {
			t.Fatalf("unmarshal Response[Network]: %v", err)
		}

		net := resp.Data[0]
		if net.InfoPrefixes4 != nil {
			t.Errorf("InfoPrefixes4 = %v, want nil", net.InfoPrefixes4)
		}
		if net.InfoPrefixes6 != nil {
			t.Errorf("InfoPrefixes6 = %v, want nil", net.InfoPrefixes6)
		}
		if net.StatusDashboard != nil {
			t.Errorf("StatusDashboard = %v, want nil", net.StatusDashboard)
		}
		if net.RIRStatus != nil {
			t.Errorf("RIRStatus = %v, want nil", net.RIRStatus)
		}
		if net.RIRStatusUpdated != nil {
			t.Errorf("RIRStatusUpdated = %v, want nil", net.RIRStatusUpdated)
		}
		if net.IXCount != 5 {
			t.Errorf("IXCount = %d, want 5", net.IXCount)
		}
		if net.FacCount != 3 {
			t.Errorf("FacCount = %d, want 3", net.FacCount)
		}
	})
}

func TestTypeConstants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{"TypeOrg", TypeOrg, "org"},
		{"TypeNet", TypeNet, "net"},
		{"TypeFac", TypeFac, "fac"},
		{"TypeIX", TypeIX, "ix"},
		{"TypePoc", TypePoc, "poc"},
		{"TypeIXLan", TypeIXLan, "ixlan"},
		{"TypeIXPfx", TypeIXPfx, "ixpfx"},
		{"TypeNetIXLan", TypeNetIXLan, "netixlan"},
		{"TypeNetFac", TypeNetFac, "netfac"},
		{"TypeIXFac", TypeIXFac, "ixfac"},
		{"TypeCarrier", TypeCarrier, "carrier"},
		{"TypeCarrierFac", TypeCarrierFac, "carrierfac"},
		{"TypeCampus", TypeCampus, "campus"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if tt.constant != tt.want {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.want)
			}
		})
	}
}

// TestIxLan_URLField_JSONRoundTrip locks the contract for the
// ixf_ixp_member_list_url field on peeringdb.IxLan. Sync must keep a null
// value apart from "": upstream stores both (django-peeringdb
// abstract.py:819-824, null=True, blank=True) and renders NULL as null.
//   - decode: an absent key and null give nil; "" gives a non-nil "".
//   - encode: nil omits the key (,omitempty); a non-nil "" emits it.
func TestIxLan_URLField_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	const url = "https://example.test/members.json"
	decodes := []struct {
		name string
		raw  string
		want *string
	}{
		{"absent key", `{"id":1,"name":"test"}`, nil},
		{"null", `{"id":1,"ixf_ixp_member_list_url":null}`, nil},
		{"empty string", `{"id":1,"ixf_ixp_member_list_url":""}`, new("")},
		{"url", `{"id":1,"ixf_ixp_member_list_url":"` + url + `"}`, new(url)},
	}
	for _, tc := range decodes {
		t.Run("decode "+tc.name, func(t *testing.T) {
			t.Parallel()
			var il IxLan
			if err := json.Unmarshal([]byte(tc.raw), &il); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got := il.IXFIXPMemberListURL
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("IXFIXPMemberListURL = %q, want nil", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("IXFIXPMemberListURL = nil, want %q", *tc.want)
			case tc.want != nil && *got != *tc.want:
				t.Fatalf("IXFIXPMemberListURL = %q, want %q", *got, *tc.want)
			}
		})
	}

	encodes := []struct {
		name      string
		value     *string
		wantKey   bool
		wantValue string
	}{
		{"nil omits the key", nil, false, ""},
		{"empty string keeps the key", new(""), true, ""},
		{"url keeps the key", new(url), true, url},
	}
	for _, tc := range encodes {
		t.Run("encode "+tc.name, func(t *testing.T) {
			t.Parallel()
			b, err := json.Marshal(IxLan{ID: 1, IXFIXPMemberListURL: tc.value})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			got, ok := m["ixf_ixp_member_list_url"]
			if ok != tc.wantKey {
				t.Fatalf("key present = %v, want %v: %s", ok, tc.wantKey, b)
			}
			if ok && got != tc.wantValue {
				t.Fatalf("value = %v, want %q", got, tc.wantValue)
			}
			if _, vis := m["ixf_ixp_member_list_url_visible"]; !vis {
				t.Fatalf("_visible companion missing: %s", b)
			}
		})
	}
}

// TestPoc_BlankDeletedContact locks the upstream tombstone rule: a
// deleted contact loses name, phone, email and url, and keeps every other
// field. A contact with any other status is returned unchanged.
// upstream: 2.83.0 serializers.py:2941-2954 (#569)
func TestPoc_BlankDeletedContact(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	full := Poc{
		ID: 7, NetID: 42, Role: "NOC", Visible: "Users",
		Name: "Jane Doe", Phone: "+1 555 0100", Email: "jane@example.invalid", URL: "https://example.invalid/jane",
		Created: ts, Updated: ts,
	}

	for _, status := range []string{"ok", "pending", ""} {
		p := full
		p.Status = status
		if got := p.BlankDeletedContact(); got != p {
			t.Errorf("status %q: got %+v, want the input unchanged", status, got)
		}
	}

	deleted := full
	deleted.Status = "deleted"
	want := deleted
	want.Name, want.Phone, want.Email, want.URL = "", "", "", ""
	if got := deleted.BlankDeletedContact(); got != want {
		t.Errorf("status deleted: got %+v, want %+v", got, want)
	}
	if deleted.Name != full.Name {
		t.Errorf("BlankDeletedContact changed its receiver: Name = %q", deleted.Name)
	}
}
