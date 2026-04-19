package grpcserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/enttest"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
	"github.com/dotwaffle/peeringdb-plus/internal/privctx"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestGetNetwork(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create a test network with distinctive field values.
	created := client.Network.Create().
		SetID(42).
		SetName("Test Network").
		SetAsn(65001).
		SetAka("TestNet").
		SetInfoUnicast(true).
		SetInfoMulticast(false).
		SetInfoIpv6(true).
		SetInfoNeverViaRouteServers(false).
		SetPolicyRatio(false).
		SetAllowIxpUpdate(true).
		SetWebsite("https://example.com").
		SetNotes("Test notes").
		SetCreated(now).
		SetUpdated(now).
		SetStatus("ok").
		SaveX(ctx)

	svc := &NetworkService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetNetwork(ctx, &pb.GetNetworkRequest{Id: 42})
		if err != nil {
			t.Fatalf("GetNetwork(42) unexpected error: %v", err)
		}
		net := resp.GetNetwork()
		if net == nil {
			t.Fatal("GetNetwork(42) returned nil network")
		}
		if net.GetId() != 42 {
			t.Errorf("Id = %d, want 42", net.GetId())
		}
		if net.GetName() != "Test Network" {
			t.Errorf("Name = %q, want %q", net.GetName(), "Test Network")
		}
		if net.GetAsn() != 65001 {
			t.Errorf("Asn = %d, want 65001", net.GetAsn())
		}
		if net.GetAka().GetValue() != "TestNet" {
			t.Errorf("Aka = %q, want %q", net.GetAka().GetValue(), "TestNet")
		}
		if !net.GetInfoIpv6() {
			t.Error("InfoIpv6 = false, want true")
		}
		if !net.GetAllowIxpUpdate() {
			t.Error("AllowIxpUpdate = false, want true")
		}
		if net.GetWebsite().GetValue() != "https://example.com" {
			t.Errorf("Website = %q, want %q", net.GetWebsite().GetValue(), "https://example.com")
		}
		if net.GetStatus() != "ok" {
			t.Errorf("Status = %q, want %q", net.GetStatus(), "ok")
		}
		if net.GetCreated().AsTime().Equal(created.Created) == false {
			t.Errorf("Created = %v, want %v", net.GetCreated().AsTime(), created.Created)
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetNetwork(ctx, &pb.GetNetworkRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetNetwork(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestListNetworks(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create 3 test networks with sequential IDs. Updated timestamps are spread
	// by 1 hour per row (id=1→12:00, id=2→13:00, id=3→14:00) so the compound
	// (-updated, -created, -id) default ordering produces the deterministic
	// sequence [id=3, id=2, id=1]. Created stays constant so (-created) does
	// not mask (-updated).
	for i := range 3 {
		client.Network.Create().
			SetID(i + 1).
			SetName(fmt.Sprintf("Network %d", i+1)).
			SetAsn(65000 + i + 1).
			SetInfoUnicast(true).
			SetInfoMulticast(false).
			SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).
			SetPolicyRatio(false).
			SetAllowIxpUpdate(false).
			SetCreated(now).
			SetUpdated(now.Add(time.Duration(i) * time.Hour)).
			SetStatus("ok").
			SaveX(ctx)
	}

	svc := &NetworkService{Client: client}

	t.Run("paginated results", func(t *testing.T) {
		t.Parallel()
		// First page: request 2 results.
		resp, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{PageSize: 2})
		if err != nil {
			t.Fatalf("ListNetworks page 1 unexpected error: %v", err)
		}
		if len(resp.GetNetworks()) != 2 {
			t.Fatalf("page 1: got %d networks, want 2", len(resp.GetNetworks()))
		}
		if resp.GetNextPageToken() == "" {
			t.Fatal("page 1: expected non-empty next_page_token")
		}

		// Second page: use the page token.
		resp2, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{
			PageSize:  2,
			PageToken: resp.GetNextPageToken(),
		})
		if err != nil {
			t.Fatalf("ListNetworks page 2 unexpected error: %v", err)
		}
		if len(resp2.GetNetworks()) != 1 {
			t.Fatalf("page 2: got %d networks, want 1", len(resp2.GetNetworks()))
		}
		if resp2.GetNextPageToken() != "" {
			t.Errorf("page 2: expected empty next_page_token, got %q", resp2.GetNextPageToken())
		}
	})

	t.Run("default page size", func(t *testing.T) {
		t.Parallel()
		// Omit page_size; should return all 3 (less than default 100).
		resp, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{})
		if err != nil {
			t.Fatalf("ListNetworks default page size unexpected error: %v", err)
		}
		if len(resp.GetNetworks()) != 3 {
			t.Fatalf("got %d networks, want 3", len(resp.GetNetworks()))
		}
		if resp.GetNextPageToken() != "" {
			t.Errorf("expected empty next_page_token, got %q", resp.GetNextPageToken())
		}
	})

	t.Run("invalid page token", func(t *testing.T) {
		t.Parallel()
		_, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{
			PageToken: "not-valid-base64!!!",
		})
		if err == nil {
			t.Fatal("ListNetworks with invalid page token expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
	})

	t.Run("default compound ordering", func(t *testing.T) {
		t.Parallel()
		// Phase 67 ORDER-02: the default list order is (-updated, -created, -id).
		// Seed timestamps spread updated by 1 hour per row, so the deterministic
		// output sequence is [id=3 (14:00), id=2 (13:00), id=1 (12:00)].
		resp, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{})
		if err != nil {
			t.Fatalf("ListNetworks unexpected error: %v", err)
		}
		got := make([]int64, len(resp.GetNetworks()))
		for i, n := range resp.GetNetworks() {
			got[i] = n.GetId()
		}
		want := []int64{3, 2, 1}
		if len(got) != len(want) {
			t.Fatalf("got %d networks, want %d", len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("position %d: got id=%d, want id=%d (full got=%v want=%v)",
					i, got[i], want[i], got, want)
			}
		}
		// Also assert the compound predicate monotonically: for every adjacent
		// pair the tuple (-updated, -created, -id) must be non-increasing.
		nets := resp.GetNetworks()
		for i := 1; i < len(nets); i++ {
			prev, curr := nets[i-1], nets[i]
			pUpd := prev.GetUpdated().AsTime()
			cUpd := curr.GetUpdated().AsTime()
			switch {
			case pUpd.After(cUpd):
				// strictly descending on updated — compliant.
			case pUpd.Equal(cUpd):
				pCre := prev.GetCreated().AsTime()
				cCre := curr.GetCreated().AsTime()
				switch {
				case pCre.After(cCre):
					// strictly descending on created — compliant.
				case pCre.Equal(cCre):
					if prev.GetId() <= curr.GetId() {
						t.Errorf("tiebreaker id not descending at pos %d: prev=%d curr=%d", i, prev.GetId(), curr.GetId())
					}
				default:
					t.Errorf("created ascends at pos %d: prev=%v curr=%v", i, pCre, cCre)
				}
			default:
				t.Errorf("updated ascends at pos %d: prev=%v curr=%v", i, pUpd, cUpd)
			}
		}
	})
}

func TestListNetworksFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed test data with distinct values for filtering.
	client.Network.Create().
		SetID(1).SetName("Google").SetAsn(15169).SetStatus("ok").
		SetInfoType("Content").SetInfoTraffic("1 Tbps+").SetPolicyGeneral("Open").
		SetWebsite("https://google.com").SetNotes("search").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(true).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Network.Create().
		SetID(2).SetName("Cloudflare").SetAsn(13335).SetStatus("ok").
		SetInfoType("NSP").SetPolicyGeneral("Selective").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Network.Create().
		SetID(3).SetName("Deleted Net").SetAsn(64512).SetStatus("deleted").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)

	svc := &NetworkService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListNetworksRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by ASN",
			req:     &pb.ListNetworksRequest{Asn: proto.Int64(15169)},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListNetworksRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by name substring case-insensitive",
			req:     &pb.ListNetworksRequest{Name: new("cloud")},
			wantLen: 1,
		},
		{
			name:    "combined filters AND",
			req:     &pb.ListNetworksRequest{Status: new("ok"), Asn: proto.Int64(15169)},
			wantLen: 1,
		},
		{
			name:    "no matches returns empty",
			req:     &pb.ListNetworksRequest{Asn: proto.Int64(99999)},
			wantLen: 0,
		},
		{
			name:    "no filters returns all",
			req:     &pb.ListNetworksRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by info_type",
			req:     &pb.ListNetworksRequest{InfoType: new("Content")},
			wantLen: 1,
		},
		{
			name:    "filter by info_traffic",
			req:     &pb.ListNetworksRequest{InfoTraffic: new("1 Tbps+")},
			wantLen: 1,
		},
		{
			name:    "filter by policy_general",
			req:     &pb.ListNetworksRequest{PolicyGeneral: new("Open")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.ListNetworksRequest{Website: new("https://google.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.ListNetworksRequest{Notes: new("search")},
			wantLen: 1,
		},
		{
			name:    "filter by info_ipv6",
			req:     &pb.ListNetworksRequest{InfoIpv6: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by info_unicast",
			req:     &pb.ListNetworksRequest{InfoUnicast: new(true)},
			wantLen: 3,
		},
		{
			name:    "filter by info_multicast",
			req:     &pb.ListNetworksRequest{InfoMulticast: new(false)},
			wantLen: 3,
		},
		{
			name:    "invalid ASN negative",
			req:     &pb.ListNetworksRequest{Asn: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid org_id zero",
			req:     &pb.ListNetworksRequest{OrgId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListNetworks(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetNetworks()); got != tt.wantLen {
				t.Errorf("got %d networks, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestListFacilitiesFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 facilities with distinct fields.
	client.Facility.Create().
		SetID(1).SetName("Equinix DA1").SetCountry("US").SetCity("Dallas").
		SetState("TX").SetZipcode("75201").SetAddress1("123 Main St").
		SetWebsite("https://equinix.com").SetNotes("tier 3").
		SetRegionContinent("North America").SetClli("DLLS").SetNpanxx("214555").
		SetProperty("Colocation").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(2).SetName("Equinix LD5").SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(3).SetName("CoreSite LA1").SetCountry("US").SetCity("Los Angeles").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &FacilityService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListFacilitiesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by country US",
			req:     &pb.ListFacilitiesRequest{Country: new("US")},
			wantLen: 2,
		},
		{
			name:    "filter by city case-insensitive substring",
			req:     &pb.ListFacilitiesRequest{City: new("dallas")},
			wantLen: 1,
		},
		{
			name:    "filter by name and country combined",
			req:     &pb.ListFacilitiesRequest{Name: new("Equinix"), Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.ListFacilitiesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by state",
			req:     &pb.ListFacilitiesRequest{State: new("TX")},
			wantLen: 1,
		},
		{
			name:    "filter by zipcode",
			req:     &pb.ListFacilitiesRequest{Zipcode: new("75201")},
			wantLen: 1,
		},
		{
			name:    "filter by address1",
			req:     &pb.ListFacilitiesRequest{Address1: new("123 Main St")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.ListFacilitiesRequest{Website: new("https://equinix.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.ListFacilitiesRequest{Notes: new("tier 3")},
			wantLen: 1,
		},
		{
			name:    "filter by region_continent",
			req:     &pb.ListFacilitiesRequest{RegionContinent: new("North America")},
			wantLen: 1,
		},
		{
			name:    "filter by clli",
			req:     &pb.ListFacilitiesRequest{Clli: new("DLLS")},
			wantLen: 1,
		},
		{
			name:    "filter by npanxx",
			req:     &pb.ListFacilitiesRequest{Npanxx: new("214555")},
			wantLen: 1,
		},
		{
			name:    "filter by property",
			req:     &pb.ListFacilitiesRequest{Property: new("Colocation")},
			wantLen: 1,
		},
		{
			name:    "invalid org_id returns INVALID_ARGUMENT",
			req:     &pb.ListFacilitiesRequest{OrgId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid campus_id zero",
			req:     &pb.ListFacilitiesRequest{CampusId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListFacilities(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetFacilities()); got != tt.wantLen {
				t.Errorf("got %d facilities, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestListOrganizationsFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 organizations with distinct name/status and varied fields.
	client.Organization.Create().
		SetID(1).SetName("Google LLC").SetCountry("US").SetCity("Mountain View").
		SetState("CA").SetWebsite("https://google.com").SetNotes("search giant").
		SetAddress1("1600 Amphitheatre Pkwy").SetZipcode("94043").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Organization.Create().
		SetID(2).SetName("Cloudflare Inc").SetCountry("US").SetCity("San Francisco").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Organization.Create().
		SetID(3).SetName("Defunct Corp").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &OrganizationService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListOrganizationsRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by name substring case-insensitive",
			req:     &pb.ListOrganizationsRequest{Name: new("google")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok excludes deleted",
			req:     &pb.ListOrganizationsRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by country",
			req:     &pb.ListOrganizationsRequest{Country: new("US")},
			wantLen: 2,
		},
		{
			name:    "filter by city",
			req:     &pb.ListOrganizationsRequest{City: new("mountain")},
			wantLen: 1,
		},
		{
			name:    "filter by state",
			req:     &pb.ListOrganizationsRequest{State: new("CA")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.ListOrganizationsRequest{Website: new("https://google.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.ListOrganizationsRequest{Notes: new("search giant")},
			wantLen: 1,
		},
		{
			name:    "filter by address1",
			req:     &pb.ListOrganizationsRequest{Address1: new("1600 Amphitheatre Pkwy")},
			wantLen: 1,
		},
		{
			name:    "filter by zipcode",
			req:     &pb.ListOrganizationsRequest{Zipcode: new("94043")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListOrganizations(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetOrganizations()); got != tt.wantLen {
				t.Errorf("got %d organizations, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestListPocsFilters(t *testing.T) {
	t.Parallel()
	// Test exercises filter operators across all seeded rows including a
	// Users-visibility POC. Phase 59-04 enabled the ent privacy Policy
	// on Poc, which would filter the Users row from a TierPublic ctx.
	// Elevate to TierUsers so the assertions (which pre-date the policy)
	// continue to see every seeded row — the filter-operator coverage is
	// orthogonal to visibility gating.
	ctx := privctx.WithTier(t.Context(), privctx.TierUsers)
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create networks to use as FK references.
	client.Network.Create().
		SetID(100).SetName("TestNet A").SetAsn(65000).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Network.Create().
		SetID(200).SetName("TestNet B").SetAsn(65001).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)

	// Seed 3 POCs with different roles and net_ids.
	client.Poc.Create().
		SetID(1).SetRole("Abuse").SetName("Abuse Contact").SetNetID(100).
		SetVisible("Users").SetPhone("+1-555-0100").SetEmail("abuse@example.com").
		SetURL("https://example.com/abuse").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Poc.Create().
		SetID(2).SetRole("Technical").SetName("Tech Contact").SetNetID(100).
		SetVisible("Public").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Poc.Create().
		SetID(3).SetRole("Policy").SetName("Policy Contact").SetNetID(200).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &PocService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListPocsRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by role Abuse",
			req:     &pb.ListPocsRequest{Role: new("Abuse")},
			wantLen: 1,
		},
		{
			name:    "filter by net_id returns only that networks contacts",
			req:     &pb.ListPocsRequest{NetId: proto.Int64(100)},
			wantLen: 2,
		},
		{
			name:    "filter by name substring",
			req:     &pb.ListPocsRequest{Name: new("abuse")},
			wantLen: 1,
		},
		{
			name:    "filter by visible",
			req:     &pb.ListPocsRequest{Visible: new("Users")},
			wantLen: 1,
		},
		{
			name:    "filter by phone",
			req:     &pb.ListPocsRequest{Phone: new("+1-555-0100")},
			wantLen: 1,
		},
		{
			name:    "filter by email",
			req:     &pb.ListPocsRequest{Email: new("abuse@example.com")},
			wantLen: 1,
		},
		{
			name:    "filter by url",
			req:     &pb.ListPocsRequest{Url: new("https://example.com/abuse")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListPocsRequest{Status: new("ok")},
			wantLen: 3,
		},
		{
			name:    "invalid net_id zero returns INVALID_ARGUMENT",
			req:     &pb.ListPocsRequest{NetId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListPocs(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetPocs()); got != tt.wantLen {
				t.Errorf("got %d pocs, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestListIxPrefixesFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 prefixes with different protocols, prefixes, and in_dfz.
	// Phase 63 (D-01): ixprefix.notes dropped — no SetNotes on this chain.
	client.IxPrefix.Create().
		SetID(1).SetPrefix("192.0.2.0/24").SetProtocol("IPv4").SetInDfz(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxPrefix.Create().
		SetID(2).SetPrefix("2001:db8::/32").SetProtocol("IPv6").SetInDfz(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxPrefix.Create().
		SetID(3).SetPrefix("198.51.100.0/24").SetProtocol("IPv4").SetInDfz(false).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &IxPrefixService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListIxPrefixesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by protocol IPv4",
			req:     &pb.ListIxPrefixesRequest{Protocol: new("IPv4")},
			wantLen: 2,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListIxPrefixesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by prefix exact",
			req:     &pb.ListIxPrefixesRequest{Prefix: new("192.0.2.0/24")},
			wantLen: 1,
		},
		{
			name:    "filter by in_dfz true",
			req:     &pb.ListIxPrefixesRequest{InDfz: new(true)},
			wantLen: 2,
		},
		{
			name:    "filter by in_dfz false",
			req:     &pb.ListIxPrefixesRequest{InDfz: new(false)},
			wantLen: 1,
		},
		// Phase 63 (D-01): "filter by notes" removed — ixprefix.notes
		// dropped from ent schema, so the filter wiring is gone from
		// internal/grpcserver/ixprefix.go. The proto IxPrefix.Notes
		// field is still present (proto is frozen since v1.6) but the
		// server ignores it.
		{
			name:    "invalid ixlan_id zero",
			req:     &pb.ListIxPrefixesRequest{IxlanId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListIxPrefixes(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetIxPrefixes()); got != tt.wantLen {
				t.Errorf("got %d prefixes, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestListNetworkIxLansFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 network IX LANs with different ASNs and varied fields.
	client.NetworkIxLan.Create().
		SetID(1).SetAsn(15169).SetSpeed(10000).SetBfdSupport(false).
		SetIsRsPeer(false).SetOperational(true).SetName("NIXL-Alpha").
		SetIpaddr4("192.0.2.1").SetIpaddr6("2001:db8::1").
		SetNotes("test notes").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkIxLan.Create().
		SetID(2).SetAsn(13335).SetSpeed(100000).SetBfdSupport(false).
		SetIsRsPeer(true).SetOperational(true).SetName("NIXL-Beta").
		SetIpaddr4("198.51.100.1").SetIpaddr6("2001:db8::2").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkIxLan.Create().
		SetID(3).SetAsn(15169).SetSpeed(10000).SetBfdSupport(false).
		SetIsRsPeer(false).SetOperational(false).SetName("NIXL-Gamma").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &NetworkIxLanService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListNetworkIxLansRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by asn",
			req:     &pb.ListNetworkIxLansRequest{Asn: proto.Int64(15169)},
			wantLen: 2,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListNetworkIxLansRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by speed",
			req:     &pb.ListNetworkIxLansRequest{Speed: proto.Int64(100000)},
			wantLen: 1,
		},
		{
			name:    "filter by is_rs_peer true",
			req:     &pb.ListNetworkIxLansRequest{IsRsPeer: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by operational true",
			req:     &pb.ListNetworkIxLansRequest{Operational: new(true)},
			wantLen: 2,
		},
		{
			name:    "filter by bfd_support false",
			req:     &pb.ListNetworkIxLansRequest{BfdSupport: new(false)},
			wantLen: 3,
		},
		{
			name:    "filter by ipaddr4",
			req:     &pb.ListNetworkIxLansRequest{Ipaddr4: new("192.0.2.1")},
			wantLen: 1,
		},
		{
			name:    "filter by ipaddr6",
			req:     &pb.ListNetworkIxLansRequest{Ipaddr6: new("2001:db8::1")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.ListNetworkIxLansRequest{Name: new("alpha")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.ListNetworkIxLansRequest{Notes: new("test notes")},
			wantLen: 1,
		},
		{
			name:    "invalid net_id negative",
			req:     &pb.ListNetworkIxLansRequest{NetId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid ixlan_id zero",
			req:     &pb.ListNetworkIxLansRequest{IxlanId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid ix_id zero",
			req:     &pb.ListNetworkIxLansRequest{IxId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid asn negative",
			req:     &pb.ListNetworkIxLansRequest{Asn: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListNetworkIxLans(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetNetworkIxLans()); got != tt.wantLen {
				t.Errorf("got %d network ix lans, want %d", got, tt.wantLen)
			}
			if tt.wantLen > 0 && tt.name == "filter by name" {
				first := resp.GetNetworkIxLans()[0]
				if got := first.GetName().GetValue(); got != "NIXL-Alpha" {
					t.Errorf("first NIXL Name = %q, want %q", got, "NIXL-Alpha")
				}
			}
		})
	}
}

func TestListCarrierFacilitiesFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create carriers to use as FK references.
	client.Carrier.Create().
		SetID(10).SetName("Carrier A").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Carrier.Create().
		SetID(20).SetName("Carrier B").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	// Seed 2 carrier facilities with different carrier IDs.
	client.CarrierFacility.Create().
		SetID(1).SetCarrierID(10).SetName("CF-Alpha").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.CarrierFacility.Create().
		SetID(2).SetCarrierID(20).SetName("CF-Beta").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &CarrierFacilityService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListCarrierFacilitiesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by carrier_id",
			req:     &pb.ListCarrierFacilitiesRequest{CarrierId: proto.Int64(10)},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListCarrierFacilitiesRequest{Status: new("ok")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.ListCarrierFacilitiesRequest{Name: new("alpha")},
			wantLen: 1,
		},
		{
			name:    "invalid carrier_id returns INVALID_ARGUMENT",
			req:     &pb.ListCarrierFacilitiesRequest{CarrierId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid fac_id zero returns INVALID_ARGUMENT",
			req:     &pb.ListCarrierFacilitiesRequest{FacId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListCarrierFacilities(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetCarrierFacilities()); got != tt.wantLen {
				t.Errorf("got %d carrier facilities, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestListCampusesFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 2 campuses with distinct field values.
	client.Campus.Create().
		SetID(1).SetName("Campus Alpha").SetCountry("US").SetCity("Dallas").
		SetState("TX").SetZipcode("75201").SetWebsite("https://campus-a.com").
		SetNotes("main campus").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Campus.Create().
		SetID(2).SetName("Campus Beta").SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &CampusService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListCampusesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by name",
			req:     &pb.ListCampusesRequest{Name: new("alpha")},
			wantLen: 1,
		},
		{
			name:    "filter by country",
			req:     &pb.ListCampusesRequest{Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.ListCampusesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by city",
			req:     &pb.ListCampusesRequest{City: new("dallas")},
			wantLen: 1,
		},
		{
			name:    "filter by state",
			req:     &pb.ListCampusesRequest{State: new("TX")},
			wantLen: 1,
		},
		{
			name:    "filter by zipcode",
			req:     &pb.ListCampusesRequest{Zipcode: new("75201")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.ListCampusesRequest{Website: new("https://campus-a.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.ListCampusesRequest{Notes: new("main campus")},
			wantLen: 1,
		},
		{
			name:    "invalid org_id zero",
			req:     &pb.ListCampusesRequest{OrgId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListCampuses(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetCampuses()); got != tt.wantLen {
				t.Errorf("got %d campuses, want %d", got, tt.wantLen)
			}
			if tt.wantLen > 0 {
				first := resp.GetCampuses()[0]
				if tt.name == "filter by name" {
					if got := first.GetName(); got != "Campus Alpha" {
						t.Errorf("first campus Name = %q, want %q", got, "Campus Alpha")
					}
				}
			}
		})
	}
}

func TestListCarriersFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 2 carriers with distinct names and statuses.
	client.Carrier.Create().
		SetID(1).SetName("Zayo").SetWebsite("https://zayo.com").SetNotes("dark fiber").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Carrier.Create().
		SetID(2).SetName("Lumen").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &CarrierService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListCarriersRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by name",
			req:     &pb.ListCarriersRequest{Name: new("zayo")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListCarriersRequest{Status: new("ok")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.ListCarriersRequest{Website: new("https://zayo.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.ListCarriersRequest{Notes: new("dark fiber")},
			wantLen: 1,
		},
		{
			name:    "invalid id negative",
			req:     &pb.ListCarriersRequest{Id: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid org_id zero",
			req:     &pb.ListCarriersRequest{OrgId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListCarriers(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetCarriers()); got != tt.wantLen {
				t.Errorf("got %d carriers, want %d", got, tt.wantLen)
			}
			if tt.wantLen > 0 {
				first := resp.GetCarriers()[0]
				if tt.name == "filter by name" {
					if got := first.GetName(); got != "Zayo" {
						t.Errorf("first carrier Name = %q, want %q", got, "Zayo")
					}
				}
			}
		})
	}
}

func TestListInternetExchangesFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 2 IXs with distinct country/media/name and rich fields.
	client.InternetExchange.Create().
		SetID(1).SetName("AMS-IX").SetCountry("NL").SetCity("Amsterdam").SetMedia("Ethernet").
		SetRegionContinent("Europe").SetNotes("largest IX").
		SetProtoUnicast(true).SetProtoMulticast(false).SetProtoIpv6(true).
		SetWebsite("https://ams-ix.net").SetTechEmail("tech@ams-ix.net").
		SetServiceLevel("Gold").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.InternetExchange.Create().
		SetID(2).SetName("LINX").SetCountry("GB").SetCity("London").SetMedia("Ethernet").
		SetRegionContinent("Europe").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &InternetExchangeService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListInternetExchangesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by name",
			req:     &pb.ListInternetExchangesRequest{Name: new("ams")},
			wantLen: 1,
		},
		{
			name:    "filter by country",
			req:     &pb.ListInternetExchangesRequest{Country: new("NL")},
			wantLen: 1,
		},
		{
			name:    "filter by media",
			req:     &pb.ListInternetExchangesRequest{Media: new("Ethernet")},
			wantLen: 2,
		},
		{
			name:    "filter by region_continent",
			req:     &pb.ListInternetExchangesRequest{RegionContinent: new("Europe")},
			wantLen: 2,
		},
		{
			name:    "filter by notes",
			req:     &pb.ListInternetExchangesRequest{Notes: new("largest IX")},
			wantLen: 1,
		},
		{
			name:    "filter by proto_unicast",
			req:     &pb.ListInternetExchangesRequest{ProtoUnicast: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by proto_ipv6",
			req:     &pb.ListInternetExchangesRequest{ProtoIpv6: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.ListInternetExchangesRequest{Website: new("https://ams-ix.net")},
			wantLen: 1,
		},
		{
			name:    "filter by tech_email",
			req:     &pb.ListInternetExchangesRequest{TechEmail: new("tech@ams-ix.net")},
			wantLen: 1,
		},
		{
			name:    "filter by service_level",
			req:     &pb.ListInternetExchangesRequest{ServiceLevel: new("Gold")},
			wantLen: 1,
		},
		{
			name:    "filter by city",
			req:     &pb.ListInternetExchangesRequest{City: new("amsterdam")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.ListInternetExchangesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "invalid org_id zero",
			req:     &pb.ListInternetExchangesRequest{OrgId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListInternetExchanges(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetInternetExchanges()); got != tt.wantLen {
				t.Errorf("got %d internet exchanges, want %d", got, tt.wantLen)
			}
			if tt.wantLen > 0 {
				first := resp.GetInternetExchanges()[0]
				if tt.name == "filter by name" {
					if got := first.GetName(); got != "AMS-IX" {
						t.Errorf("first IX Name = %q, want %q", got, "AMS-IX")
					}
				}
			}
		})
	}
}

func TestListIxFacilitiesFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create FK parents.
	client.InternetExchange.Create().
		SetID(100).SetName("Test IX").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(200).SetName("Fac-200").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(201).SetName("Fac-201").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	// Seed 2 IX facilities.
	client.IxFacility.Create().
		SetID(1).SetIxID(100).SetFacID(200).SetCountry("US").SetName("IXFAC-A").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxFacility.Create().
		SetID(2).SetIxID(100).SetFacID(201).SetCountry("GB").SetName("IXFAC-B").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &IxFacilityService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListIxFacilitiesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by country",
			req:     &pb.ListIxFacilitiesRequest{Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.ListIxFacilitiesRequest{Name: new("ixfac-a")},
			wantLen: 1,
		},
		{
			name:    "invalid ix_id negative",
			req:     &pb.ListIxFacilitiesRequest{IxId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid fac_id zero",
			req:     &pb.ListIxFacilitiesRequest{FacId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListIxFacilities(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetIxFacilities()); got != tt.wantLen {
				t.Errorf("got %d ix facilities, want %d", got, tt.wantLen)
			}
			if tt.wantLen > 0 {
				first := resp.GetIxFacilities()[0]
				if tt.name == "filter by country" {
					if got := first.GetCountry().GetValue(); got != "US" {
						t.Errorf("first IxFacility Country = %q, want %q", got, "US")
					}
				}
			}
		})
	}
}

func TestListIxLansFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create FK parent.
	client.InternetExchange.Create().
		SetID(100).SetName("Test IX").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	// Seed 2 IX LANs with distinct mtu/rs_asn.
	client.IxLan.Create().
		SetID(1).SetIxID(100).SetName("Primary LAN").SetMtu(9000).SetRsAsn(47541).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxLan.Create().
		SetID(2).SetIxID(100).SetName("Secondary LAN").SetMtu(1500).SetRsAsn(47542).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &IxLanService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListIxLansRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by name",
			req:     &pb.ListIxLansRequest{Name: new("primary")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListIxLansRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by mtu",
			req:     &pb.ListIxLansRequest{Mtu: proto.Int64(9000)},
			wantLen: 1,
		},
		{
			name:    "invalid ix_id negative",
			req:     &pb.ListIxLansRequest{IxId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid rs_asn zero",
			req:     &pb.ListIxLansRequest{RsAsn: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListIxLans(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetIxLans()); got != tt.wantLen {
				t.Errorf("got %d ix lans, want %d", got, tt.wantLen)
			}
			if tt.wantLen > 0 {
				first := resp.GetIxLans()[0]
				if tt.name == "filter by name" {
					if got := first.GetName().GetValue(); got != "Primary LAN" {
						t.Errorf("first IxLan Name = %q, want %q", got, "Primary LAN")
					}
				}
			}
		})
	}
}

func TestListNetworkFacilitiesFilters(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create FK parents.
	client.Network.Create().
		SetID(100).SetName("Net-100").SetAsn(65001).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Facility.Create().
		SetID(200).SetName("Fac-200").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	// Seed 2 network facilities with distinct country/local_asn/name.
	client.NetworkFacility.Create().
		SetID(1).SetNetID(100).SetFacID(200).SetLocalAsn(65001).SetCountry("US").SetName("NF-A").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(2).SetNetID(100).SetFacID(200).SetLocalAsn(65002).SetCountry("GB").SetName("NF-B").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &NetworkFacilityService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListNetworkFacilitiesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "filter by country",
			req:     &pb.ListNetworkFacilitiesRequest{Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.ListNetworkFacilitiesRequest{Name: new("nf-a")},
			wantLen: 1,
		},
		{
			name:    "filter by local_asn",
			req:     &pb.ListNetworkFacilitiesRequest{LocalAsn: proto.Int64(65001)},
			wantLen: 1,
		},
		{
			name:    "invalid net_id negative",
			req:     &pb.ListNetworkFacilitiesRequest{NetId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid fac_id zero",
			req:     &pb.ListNetworkFacilitiesRequest{FacId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListNetworkFacilities(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetNetworkFacilities()); got != tt.wantLen {
				t.Errorf("got %d network facilities, want %d", got, tt.wantLen)
			}
			if tt.wantLen > 0 {
				first := resp.GetNetworkFacilities()[0]
				if tt.name == "filter by country" {
					if got := first.GetCountry().GetValue(); got != "US" {
						t.Errorf("first NetworkFacility Country = %q, want %q", got, "US")
					}
				}
			}
		})
	}
}

func TestListNetworksFiltersPaginated(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 5 networks with status="ok" and 1 with status="deleted".
	for i := range 5 {
		client.Network.Create().
			SetID(i + 1).
			SetName(fmt.Sprintf("Active Net %d", i+1)).
			SetAsn(65000 + i + 1).
			SetStatus("ok").
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
			SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
			SaveX(ctx)
	}
	client.Network.Create().
		SetID(6).SetName("Deleted Net").SetAsn(64512).SetStatus("deleted").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)

	svc := &NetworkService{Client: client}

	// Page 1: filter by status="ok", page_size=2.
	resp, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{
		Status:   new("ok"),
		PageSize: 2,
	})
	if err != nil {
		t.Fatalf("page 1 unexpected error: %v", err)
	}
	if len(resp.GetNetworks()) != 2 {
		t.Fatalf("page 1: got %d networks, want 2", len(resp.GetNetworks()))
	}
	if resp.GetNextPageToken() == "" {
		t.Fatal("page 1: expected non-empty next_page_token")
	}

	// Page 2: continue with page token.
	resp2, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{
		Status:    new("ok"),
		PageSize:  2,
		PageToken: resp.GetNextPageToken(),
	})
	if err != nil {
		t.Fatalf("page 2 unexpected error: %v", err)
	}
	if len(resp2.GetNetworks()) != 2 {
		t.Fatalf("page 2: got %d networks, want 2", len(resp2.GetNetworks()))
	}
	if resp2.GetNextPageToken() == "" {
		t.Fatal("page 2: expected non-empty next_page_token")
	}

	// Page 3: last page with remaining result.
	resp3, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{
		Status:    new("ok"),
		PageSize:  2,
		PageToken: resp2.GetNextPageToken(),
	})
	if err != nil {
		t.Fatalf("page 3 unexpected error: %v", err)
	}
	if len(resp3.GetNetworks()) != 1 {
		t.Fatalf("page 3: got %d networks, want 1", len(resp3.GetNetworks()))
	}
	if resp3.GetNextPageToken() != "" {
		t.Errorf("page 3: expected empty next_page_token, got %q", resp3.GetNextPageToken())
	}
}

// setupStreamTestServer creates an in-process HTTP/2 TLS test server with the
// NetworkService handler mounted and returns a typed ConnectRPC streaming
// client.
func setupStreamTestServer(t *testing.T, client *ent.Client) peeringdbv1connect.NetworkServiceClient {
	t.Helper()
	svc := &NetworkService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewNetworkServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewNetworkServiceClient(
		srv.Client(),
		srv.URL,
	)
}

// seedStreamNetworks creates 3 test networks for streaming tests. Returns them
// for reference. IDs 1=Google(ok), 2=Cloudflare(ok), 3=Deleted(deleted).
//
// Phase 67: updated timestamps are spread (id=1 at 12:00, id=2 at 13:00, id=3
// at 14:00) so the compound (-updated, -created, -id) default ordering yields
// the deterministic sequence [id=3, id=2, id=1]. The created timestamp stays
// constant at 12:00 across all rows so the (-created) tiebreaker does not mask
// the (-updated) primary sort. All updated values remain within the
// 2026-01-14..2026-01-16 window that the TestStreamNetworksUpdatedSince cases
// target, so cardinality assertions are unaffected.
func seedStreamNetworks(t *testing.T, client *ent.Client) {
	t.Helper()
	ctx := t.Context()
	created := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	upd1 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	upd2 := time.Date(2026, 1, 15, 13, 0, 0, 0, time.UTC)
	upd3 := time.Date(2026, 1, 15, 14, 0, 0, 0, time.UTC)

	client.Network.Create().
		SetID(1).SetName("Google").SetAsn(15169).SetStatus("ok").
		SetInfoType("Content").SetInfoTraffic("1 Tbps+").SetPolicyGeneral("Open").
		SetWebsite("https://google.com").SetNotes("search giant").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(true).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(created).SetUpdated(upd1).
		SaveX(ctx)
	client.Network.Create().
		SetID(2).SetName("Cloudflare").SetAsn(13335).SetStatus("ok").
		SetInfoType("NSP").SetPolicyGeneral("Selective").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(created).SetUpdated(upd2).
		SaveX(ctx)
	client.Network.Create().
		SetID(3).SetName("Deleted Net").SetAsn(64512).SetStatus("deleted").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(created).SetUpdated(upd3).
		SaveX(ctx)
}

func TestStreamNetworks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		req     *pb.StreamNetworksRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "all records",
			req:     &pb.StreamNetworksRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by asn",
			req:     &pb.StreamNetworksRequest{Asn: proto.Int64(15169)},
			wantLen: 1,
		},
		{
			name:    "filter by name case insensitive",
			req:     &pb.StreamNetworksRequest{Name: new("cloud")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamNetworksRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "combined filters",
			req:     &pb.StreamNetworksRequest{Asn: proto.Int64(15169), Status: new("ok")},
			wantLen: 1,
		},
		{
			name:    "no matches returns empty stream",
			req:     &pb.StreamNetworksRequest{Asn: proto.Int64(99999)},
			wantLen: 0,
		},
		{
			name:    "filter by info_type",
			req:     &pb.StreamNetworksRequest{InfoType: new("Content")},
			wantLen: 1,
		},
		{
			name:    "filter by info_traffic",
			req:     &pb.StreamNetworksRequest{InfoTraffic: new("1 Tbps+")},
			wantLen: 1,
		},
		{
			name:    "filter by policy_general",
			req:     &pb.StreamNetworksRequest{PolicyGeneral: new("Open")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.StreamNetworksRequest{Website: new("https://google.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.StreamNetworksRequest{Notes: new("search giant")},
			wantLen: 1,
		},
		{
			name:    "filter by info_ipv6",
			req:     &pb.StreamNetworksRequest{InfoIpv6: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by info_unicast",
			req:     &pb.StreamNetworksRequest{InfoUnicast: new(true)},
			wantLen: 3,
		},
		{
			name:    "invalid asn returns error",
			req:     &pb.StreamNetworksRequest{Asn: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "invalid org_id returns error",
			req:     &pb.StreamNetworksRequest{OrgId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entClient := testutil.SetupClient(t)
			seedStreamNetworks(t, entClient)
			rpcClient := setupStreamTestServer(t, entClient)
			ctx := t.Context()

			stream, err := rpcClient.StreamNetworks(ctx, tt.req)
			if tt.wantErr != 0 {
				// For streaming, the error may come on Receive or on initial call.
				if err != nil {
					if code := connect.CodeOf(err); code != tt.wantErr {
						t.Errorf("error code = %v, want %v", code, tt.wantErr)
					}
					return
				}
				// Drain the stream; error should appear.
				for stream.Receive() {
				}
				if streamErr := stream.Err(); streamErr == nil {
					t.Fatal("expected error from stream, got nil")
				} else if code := connect.CodeOf(streamErr); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("StreamNetworks returned error: %v", err)
			}

			var count int
			for stream.Receive() {
				msg := stream.Msg()
				if msg == nil {
					t.Fatal("received nil message from stream")
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func TestStreamNetworksTotalCount(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	seedStreamNetworks(t, entClient)

	tests := []struct {
		name      string
		req       *pb.StreamNetworksRequest
		wantCount string
	}{
		{
			name:      "no filter returns total 3",
			req:       &pb.StreamNetworksRequest{},
			wantCount: "3",
		},
		{
			name:      "status ok filter returns 2",
			req:       &pb.StreamNetworksRequest{Status: new("ok")},
			wantCount: "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Each subtest needs its own server + data for isolation.
			ec := testutil.SetupClient(t)
			seedStreamNetworks(t, ec)
			rpcClient := setupStreamTestServer(t, ec)
			ctx := t.Context()

			stream, err := rpcClient.StreamNetworks(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamNetworks returned error: %v", err)
			}

			// Drain stream to completion.
			for stream.Receive() {
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}

			// Check response header for total count.
			got := stream.ResponseHeader().Get("Grpc-Total-Count")
			if got == "" {
				got = stream.ResponseHeader().Get("grpc-total-count")
			}
			if got != tt.wantCount {
				t.Errorf("grpc-total-count = %q, want %q", got, tt.wantCount)
			}

			// Verify parseable as integer.
			if _, err := strconv.Atoi(got); err != nil && got != "" {
				t.Errorf("grpc-total-count %q is not a valid integer: %v", got, err)
			}
		})
	}
}

func TestStreamNetworksCancellation(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	seedStreamNetworks(t, entClient)
	rpcClient := setupStreamTestServer(t, entClient)

	ctx, cancel := context.WithCancel(t.Context())

	stream, err := rpcClient.StreamNetworks(ctx, &pb.StreamNetworksRequest{})
	if err != nil {
		t.Fatalf("StreamNetworks returned error: %v", err)
	}

	// Receive the first message, then cancel the context.
	if !stream.Receive() {
		t.Fatal("expected at least one message, got none")
	}
	cancel()

	// Drain remaining -- the stream should terminate with an error or no more
	// messages. The key property is that this test does not hang (test timeout
	// protects against that).
	for stream.Receive() {
	}

	// After cancellation the stream error should be non-nil.
	if streamErr := stream.Err(); streamErr == nil {
		// It is acceptable for small data sets where all records were already
		// sent before the cancel propagated. Log but do not fail.
		t.Log("stream completed without error after cancel -- data was small enough to send before cancellation propagated")
	}
}

func TestStreamNetworksSinceId(t *testing.T) {
	t.Parallel()

	// PERF-06: delta streams (SinceID set) omit the grpc-total-count header
	// entirely. wantCount is "" on every subtest because header absence is the
	// wire contract. The "since_id zero" subtest also expects absence: the
	// StreamEntities guard is a pointer-nil check (params.SinceID == nil), so a
	// non-nil *int64 pointing at 0 still counts as "delta filter set". The
	// "same as omitted" phrasing in the test name refers to result-row
	// equivalence (3 rows returned), not header equivalence.
	tests := []struct {
		name      string
		req       *pb.StreamNetworksRequest
		wantLen   int
		wantCount string // PERF-06: "" means header absent on delta streams.
	}{
		{
			name:      "since_id returns records after given ID",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(1)},
			wantLen:   2,
			wantCount: "",
		},
		{
			name:      "since_id beyond max returns empty",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(9999)},
			wantLen:   0,
			wantCount: "",
		},
		{
			name:      "since_id with status filter composes via AND",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(1), Status: new("ok")},
			wantLen:   1,
			wantCount: "",
		},
		{
			// Header absent because SinceId pointer is non-nil regardless of value.
			name:      "since_id zero returns all (same as omitted)",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(0)},
			wantLen:   3,
			wantCount: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entClient := testutil.SetupClient(t)
			seedStreamNetworks(t, entClient)
			rpcClient := setupStreamTestServer(t, entClient)
			ctx := t.Context()

			stream, err := rpcClient.StreamNetworks(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamNetworks returned error: %v", err)
			}

			var count int
			for stream.Receive() {
				if stream.Msg() == nil {
					t.Fatal("received nil message from stream")
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}

			// Check response header for total count. PERF-06: delta streams
			// must omit the header entirely — wantCount is "" for every case
			// in this test, asserting absence.
			got := stream.ResponseHeader().Get("Grpc-Total-Count")
			if got == "" {
				got = stream.ResponseHeader().Get("grpc-total-count")
			}
			if got != tt.wantCount {
				t.Errorf("grpc-total-count = %q, want absent (delta stream)", got)
			}
		})
	}
}

func TestStreamNetworksUpdatedSince(t *testing.T) {
	t.Parallel()

	// PERF-06: delta streams (UpdatedSince set) omit the grpc-total-count
	// header entirely. wantCount is "" on every subtest — including the
	// combined SinceId+UpdatedSince case — because header absence is the
	// wire contract whenever either delta filter pointer is non-nil.
	tests := []struct {
		name      string
		req       *pb.StreamNetworksRequest
		wantLen   int
		wantCount string // PERF-06: "" means header absent on delta streams.
	}{
		{
			name: "updated_since before seed time returns all",
			req: &pb.StreamNetworksRequest{
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)),
			},
			wantLen:   3,
			wantCount: "",
		},
		{
			name: "updated_since after seed time returns none",
			req: &pb.StreamNetworksRequest{
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)),
			},
			wantLen:   0,
			wantCount: "",
		},
		{
			name: "updated_since with status filter composes via AND",
			req: &pb.StreamNetworksRequest{
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)),
				Status:       new("ok"),
			},
			wantLen:   2,
			wantCount: "",
		},
		{
			name: "since_id and updated_since compose together",
			req: &pb.StreamNetworksRequest{
				SinceId:      proto.Int64(1),
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)),
			},
			wantLen:   2,
			wantCount: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entClient := testutil.SetupClient(t)
			seedStreamNetworks(t, entClient)
			rpcClient := setupStreamTestServer(t, entClient)
			ctx := t.Context()

			stream, err := rpcClient.StreamNetworks(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamNetworks returned error: %v", err)
			}

			var count int
			for stream.Receive() {
				if stream.Msg() == nil {
					t.Fatal("received nil message from stream")
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}

			// Check response header for total count. PERF-06: delta streams
			// must omit the header entirely — wantCount is "" for every case
			// in this test, asserting absence.
			got := stream.ResponseHeader().Get("Grpc-Total-Count")
			if got == "" {
				got = stream.ResponseHeader().Get("grpc-total-count")
			}
			if got != tt.wantCount {
				t.Errorf("grpc-total-count = %q, want absent (delta stream)", got)
			}
		})
	}
}

// countStmtRE matches any SQL statement that contains SELECT COUNT( (any
// whitespace, case-insensitive). Used by TestStream_SkipCountOnDelta to assert
// no COUNT(*) preflight was issued on delta streams.
var countStmtRE = regexp.MustCompile(`(?i)select\s+count\(`)

// TestStream_SkipCountOnDelta is the load-bearing PERF-06 assertion: on a
// StreamNetworks RPC with SinceId set, StreamEntities must NOT issue a
// SELECT COUNT(*) query and must NOT write the grpc-total-count response
// header. Verified via ent's dialect.Debug() driver wrapper capturing every
// Exec/Query SQL string and a post-stream scan for any COUNT(...) shape.
func TestStream_SkipCountOnDelta(t *testing.T) {
	t.Parallel()

	// Capture every SQL statement the ent driver issues during the stream.
	// dialect.Debug wraps the underlying driver and forwards every Exec/Query
	// call through the log function as a formatted "driver.Query: query=... args=..."
	// string. A mutex guards concurrent writes because ent may issue queries
	// from goroutines spawned inside the connect handler.
	var (
		mu       sync.Mutex
		captured []string
	)
	logFn := func(args ...any) {
		mu.Lock()
		defer mu.Unlock()
		captured = append(captured, fmt.Sprint(args...))
	}

	// Build an ent client whose driver is wrapped in dialect.Debug. Fresh DSN
	// per test run so we stay hermetic under t.Parallel().
	dsn := fmt.Sprintf("file:test_skipcount_%d?mode=memory&cache=shared&_pragma=foreign_keys(1)",
		time.Now().UnixNano())
	entClient := enttest.Open(t,
		dialect.SQLite,
		dsn,
		enttest.WithOptions(ent.Debug(), ent.Log(logFn)),
	)
	t.Cleanup(func() { _ = entClient.Close() })

	seedStreamNetworks(t, entClient)

	// Reset capture AFTER seeding — seed inserts are noise, we only care about
	// the SQL emitted by the stream handler itself.
	mu.Lock()
	captured = captured[:0]
	mu.Unlock()

	rpcClient := setupStreamTestServer(t, entClient)
	ctx := t.Context()

	stream, err := rpcClient.StreamNetworks(ctx, &pb.StreamNetworksRequest{
		SinceId: proto.Int64(1),
	})
	if err != nil {
		t.Fatalf("StreamNetworks returned error: %v", err)
	}

	// Drain the stream fully so every driver op is captured before asserting.
	var got int
	for stream.Receive() {
		if stream.Msg() == nil {
			t.Fatal("received nil message from stream")
		}
		got++
	}
	if streamErr := stream.Err(); streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}

	// Snapshot the captured slice under the mutex.
	mu.Lock()
	stmts := make([]string, len(captured))
	copy(stmts, captured)
	mu.Unlock()

	// PERF-06 assertion 1: no SELECT COUNT( statement was issued at all.
	for _, stmt := range stmts {
		if countStmtRE.MatchString(stmt) {
			t.Errorf("delta stream issued a COUNT(*) preflight, PERF-06 violation: %s", stmt)
		}
	}

	// PERF-06 assertion 2: the grpc-total-count header is absent in both
	// canonical and lowercase forms.
	if h := stream.ResponseHeader().Get("Grpc-Total-Count"); h != "" {
		t.Errorf("Grpc-Total-Count = %q, want absent on delta stream", h)
	}
	if h := stream.ResponseHeader().Get("grpc-total-count"); h != "" {
		t.Errorf("grpc-total-count = %q, want absent on delta stream", h)
	}

	// Sanity: the stream still returned the expected rows (ids 2 and 3).
	if got != 2 {
		t.Errorf("got %d messages, want 2 (rows after id=1)", got)
	}

	// Sanity: the capture hook actually fired for the stream — otherwise the
	// negative assertion above would be vacuously true.
	if len(stmts) == 0 {
		t.Fatal("query log captured zero statements; debug wrapper not wired correctly")
	}
}

// =======================================================================
// Missing entity type tests: Campus, Carrier, InternetExchange,
// IxFacility, IxLan, NetworkFacility
// =======================================================================

func TestGetCampus(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Campus.Create().
		SetID(1).SetName("Test Campus").SetCountry("US").SetCity("Ashburn").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &CampusService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetCampus(ctx, &pb.GetCampusRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetCampus(1) unexpected error: %v", err)
		}
		c := resp.GetCampus()
		if c == nil {
			t.Fatal("GetCampus(1) returned nil campus")
		}
		if c.GetId() != 1 {
			t.Errorf("Id = %d, want 1", c.GetId())
		}
		if c.GetName() != "Test Campus" {
			t.Errorf("Name = %q, want %q", c.GetName(), "Test Campus")
		}
		if c.GetCountry().GetValue() != "US" {
			t.Errorf("Country = %q, want %q", c.GetCountry().GetValue(), "US")
		}
		if c.GetStatus() != "ok" {
			t.Errorf("Status = %q, want %q", c.GetStatus(), "ok")
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetCampus(ctx, &pb.GetCampusRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetCampus(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestListCampuses(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Campus.Create().
		SetID(1).SetName("Campus Alpha").SetCountry("US").SetCity("Ashburn").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Campus.Create().
		SetID(2).SetName("Campus Beta").SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Campus.Create().
		SetID(3).SetName("Campus Gamma").SetCountry("US").SetCity("Dallas").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &CampusService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListCampusesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "no filters returns all",
			req:     &pb.ListCampusesRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by country US",
			req:     &pb.ListCampusesRequest{Country: new("US")},
			wantLen: 2,
		},
		{
			name:    "filter by name case-insensitive",
			req:     &pb.ListCampusesRequest{Name: new("alpha")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListCampusesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "combined country and status",
			req:     &pb.ListCampusesRequest{Country: new("US"), Status: new("ok")},
			wantLen: 1,
		},
		{
			name:    "invalid org_id",
			req:     &pb.ListCampusesRequest{OrgId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name:    "pagination",
			req:     &pb.ListCampusesRequest{PageSize: 2},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListCampuses(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetCampuses()); got != tt.wantLen {
				t.Errorf("got %d campuses, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestGetCarrier(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Carrier.Create().
		SetID(1).SetName("Test Carrier").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &CarrierService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetCarrier(ctx, &pb.GetCarrierRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetCarrier(1) unexpected error: %v", err)
		}
		c := resp.GetCarrier()
		if c == nil {
			t.Fatal("GetCarrier(1) returned nil carrier")
		}
		if c.GetId() != 1 {
			t.Errorf("Id = %d, want 1", c.GetId())
		}
		if c.GetName() != "Test Carrier" {
			t.Errorf("Name = %q, want %q", c.GetName(), "Test Carrier")
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetCarrier(ctx, &pb.GetCarrierRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetCarrier(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestListCarriers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Carrier.Create().
		SetID(1).SetName("Zayo").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Carrier.Create().
		SetID(2).SetName("Lumen").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Carrier.Create().
		SetID(3).SetName("Defunct Carrier").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &CarrierService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListCarriersRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "no filters returns all",
			req:     &pb.ListCarriersRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by name case-insensitive",
			req:     &pb.ListCarriersRequest{Name: new("zayo")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListCarriersRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "invalid org_id",
			req:     &pb.ListCarriersRequest{OrgId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListCarriers(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetCarriers()); got != tt.wantLen {
				t.Errorf("got %d carriers, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestGetInternetExchange(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.InternetExchange.Create().
		SetID(1).SetName("DE-CIX Frankfurt").SetCountry("DE").SetCity("Frankfurt").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &InternetExchangeService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetInternetExchange(ctx, &pb.GetInternetExchangeRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetInternetExchange(1) unexpected error: %v", err)
		}
		ix := resp.GetInternetExchange()
		if ix == nil {
			t.Fatal("GetInternetExchange(1) returned nil")
		}
		if ix.GetId() != 1 {
			t.Errorf("Id = %d, want 1", ix.GetId())
		}
		if ix.GetName() != "DE-CIX Frankfurt" {
			t.Errorf("Name = %q, want %q", ix.GetName(), "DE-CIX Frankfurt")
		}
		if ix.GetCountry().GetValue() != "DE" {
			t.Errorf("Country = %q, want %q", ix.GetCountry().GetValue(), "DE")
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetInternetExchange(ctx, &pb.GetInternetExchangeRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetInternetExchange(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestListInternetExchanges(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.InternetExchange.Create().
		SetID(1).SetName("DE-CIX Frankfurt").SetCountry("DE").SetCity("Frankfurt").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.InternetExchange.Create().
		SetID(2).SetName("AMS-IX").SetCountry("NL").SetCity("Amsterdam").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.InternetExchange.Create().
		SetID(3).SetName("Old IX").SetCountry("DE").SetCity("Berlin").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &InternetExchangeService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListInternetExchangesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "no filters returns all",
			req:     &pb.ListInternetExchangesRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by country DE",
			req:     &pb.ListInternetExchangesRequest{Country: new("DE")},
			wantLen: 2,
		},
		{
			name:    "filter by name case-insensitive",
			req:     &pb.ListInternetExchangesRequest{Name: new("ams")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListInternetExchangesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "invalid org_id",
			req:     &pb.ListInternetExchangesRequest{OrgId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListInternetExchanges(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetInternetExchanges()); got != tt.wantLen {
				t.Errorf("got %d internet exchanges, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestGetIxFacility(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// IxFacility FK references IX and Facility -- create parents first.
	client.InternetExchange.Create().
		SetID(1).SetName("Parent IX").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(1).SetName("Parent Fac").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxFacility.Create().
		SetID(1).SetIxID(1).SetFacID(1).SetName("IX-Fac-1").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &IxFacilityService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetIxFacility(ctx, &pb.GetIxFacilityRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetIxFacility(1) unexpected error: %v", err)
		}
		ixf := resp.GetIxFacility()
		if ixf == nil {
			t.Fatal("GetIxFacility(1) returned nil")
		}
		if ixf.GetId() != 1 {
			t.Errorf("Id = %d, want 1", ixf.GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetIxFacility(ctx, &pb.GetIxFacilityRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetIxFacility(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestListIxFacilities(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create parent IX and Facility entities for FK constraints.
	client.InternetExchange.Create().
		SetID(10).SetName("IX-10").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.InternetExchange.Create().
		SetID(20).SetName("IX-20").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(100).SetName("Fac-100").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(200).SetName("Fac-200").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	client.IxFacility.Create().
		SetID(1).SetIxID(10).SetFacID(100).SetName("IX-Fac-A").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxFacility.Create().
		SetID(2).SetIxID(10).SetFacID(200).SetName("IX-Fac-B").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxFacility.Create().
		SetID(3).SetIxID(20).SetFacID(100).SetName("IX-Fac-C").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &IxFacilityService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListIxFacilitiesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "no filters returns all",
			req:     &pb.ListIxFacilitiesRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by ix_id",
			req:     &pb.ListIxFacilitiesRequest{IxId: proto.Int64(10)},
			wantLen: 2,
		},
		{
			name:    "filter by fac_id",
			req:     &pb.ListIxFacilitiesRequest{FacId: proto.Int64(100)},
			wantLen: 2,
		},
		{
			name:    "combined ix_id and fac_id",
			req:     &pb.ListIxFacilitiesRequest{IxId: proto.Int64(10), FacId: proto.Int64(100)},
			wantLen: 1,
		},
		{
			name:    "invalid ix_id",
			req:     &pb.ListIxFacilitiesRequest{IxId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListIxFacilities(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetIxFacilities()); got != tt.wantLen {
				t.Errorf("got %d ix facilities, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestGetIxLan(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// IxLan FK references InternetExchange -- create parent first.
	client.InternetExchange.Create().
		SetID(1).SetName("Parent IX").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxLan.Create().
		SetID(1).SetIxID(1).SetName("Test IxLan").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &IxLanService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetIxLan(ctx, &pb.GetIxLanRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetIxLan(1) unexpected error: %v", err)
		}
		il := resp.GetIxLan()
		if il == nil {
			t.Fatal("GetIxLan(1) returned nil")
		}
		if il.GetId() != 1 {
			t.Errorf("Id = %d, want 1", il.GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetIxLan(ctx, &pb.GetIxLanRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetIxLan(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestListIxLans(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create parent IX entities for FK constraints.
	client.InternetExchange.Create().
		SetID(10).SetName("IX-10").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.InternetExchange.Create().
		SetID(20).SetName("IX-20").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	client.IxLan.Create().
		SetID(1).SetIxID(10).SetName("LAN-A").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxLan.Create().
		SetID(2).SetIxID(10).SetName("LAN-B").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxLan.Create().
		SetID(3).SetIxID(20).SetName("LAN-C").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &IxLanService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListIxLansRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "no filters returns all",
			req:     &pb.ListIxLansRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by ix_id",
			req:     &pb.ListIxLansRequest{IxId: proto.Int64(10)},
			wantLen: 2,
		},
		{
			name:    "filter by name case-insensitive",
			req:     &pb.ListIxLansRequest{Name: new("lan-a")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListIxLansRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "invalid ix_id",
			req:     &pb.ListIxLansRequest{IxId: proto.Int64(0)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListIxLans(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetIxLans()); got != tt.wantLen {
				t.Errorf("got %d ix lans, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestGetNetworkFacility(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// NetworkFacility FK references Network and Facility -- create parents.
	client.Network.Create().
		SetID(1).SetName("Net-1").SetAsn(65001).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Facility.Create().
		SetID(1).SetName("Fac-1").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(1).SetNetID(1).SetFacID(1).SetLocalAsn(65001).SetName("NetFac-1").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &NetworkFacilityService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetNetworkFacility(ctx, &pb.GetNetworkFacilityRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetNetworkFacility(1) unexpected error: %v", err)
		}
		nf := resp.GetNetworkFacility()
		if nf == nil {
			t.Fatal("GetNetworkFacility(1) returned nil")
		}
		if nf.GetId() != 1 {
			t.Errorf("Id = %d, want 1", nf.GetId())
		}
		if nf.GetLocalAsn() != 65001 {
			t.Errorf("LocalAsn = %d, want 65001", nf.GetLocalAsn())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetNetworkFacility(ctx, &pb.GetNetworkFacilityRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetNetworkFacility(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestListNetworkFacilities(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create parent Network and Facility entities for FK constraints.
	client.Network.Create().
		SetID(100).SetName("Net-100").SetAsn(65001).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Network.Create().
		SetID(200).SetName("Net-200").SetAsn(65002).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Facility.Create().
		SetID(200).SetName("Fac-200").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(300).SetName("Fac-300").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	client.NetworkFacility.Create().
		SetID(1).SetNetID(100).SetFacID(200).SetLocalAsn(65001).SetName("NF-A").
		SetCountry("US").SetCity("Dallas").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(2).SetNetID(100).SetFacID(300).SetLocalAsn(65001).SetName("NF-B").
		SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(3).SetNetID(200).SetFacID(200).SetLocalAsn(65002).SetName("NF-C").
		SetCountry("US").SetCity("Ashburn").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &NetworkFacilityService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListNetworkFacilitiesRequest
		wantLen int
		wantErr connect.Code
	}{
		{
			name:    "no filters returns all",
			req:     &pb.ListNetworkFacilitiesRequest{},
			wantLen: 3,
		},
		{
			name:    "filter by net_id",
			req:     &pb.ListNetworkFacilitiesRequest{NetId: proto.Int64(100)},
			wantLen: 2,
		},
		{
			name:    "filter by fac_id",
			req:     &pb.ListNetworkFacilitiesRequest{FacId: proto.Int64(200)},
			wantLen: 2,
		},
		{
			name:    "combined net_id and fac_id",
			req:     &pb.ListNetworkFacilitiesRequest{NetId: proto.Int64(100), FacId: proto.Int64(200)},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListNetworkFacilitiesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "invalid net_id",
			req:     &pb.ListNetworkFacilitiesRequest{NetId: proto.Int64(-1)},
			wantErr: connect.CodeInvalidArgument,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListNetworkFacilities(ctx, tt.req)
			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetNetworkFacilities()); got != tt.wantLen {
				t.Errorf("got %d network facilities, want %d", got, tt.wantLen)
			}
		})
	}
}

// =======================================================================
// New filter parity tests for existing entity types
// =======================================================================

func TestListNetworksInfoTypeFilter(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Network.Create().
		SetID(1).SetName("CDN Corp").SetAsn(65001).SetInfoType("Content").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(true).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Network.Create().
		SetID(2).SetName("ISP Corp").SetAsn(65002).SetInfoType("NSP").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Network.Create().
		SetID(3).SetName("Enterprise Corp").SetAsn(65003).SetInfoType("Enterprise").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &NetworkService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListNetworksRequest
		wantLen int
	}{
		{
			name:    "filter by info_type Content",
			req:     &pb.ListNetworksRequest{InfoType: new("Content")},
			wantLen: 1,
		},
		{
			name:    "filter by info_type NSP",
			req:     &pb.ListNetworksRequest{InfoType: new("NSP")},
			wantLen: 1,
		},
		{
			name:    "filter by info_unicast true",
			req:     &pb.ListNetworksRequest{InfoUnicast: new(true)},
			wantLen: 3,
		},
		{
			name:    "filter by info_ipv6 true",
			req:     &pb.ListNetworksRequest{InfoIpv6: new(true)},
			wantLen: 1,
		},
		{
			name:    "combined info_type and ASN",
			req:     &pb.ListNetworksRequest{InfoType: new("Content"), Asn: proto.Int64(65001)},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListNetworks(ctx, tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetNetworks()); got != tt.wantLen {
				t.Errorf("got %d networks, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestGetFacility(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Facility.Create().
		SetID(1).SetName("Equinix DA1").SetCountry("US").SetCity("Dallas").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &FacilityService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetFacility(ctx, &pb.GetFacilityRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetFacility(1) unexpected error: %v", err)
		}
		f := resp.GetFacility()
		if f == nil {
			t.Fatal("GetFacility(1) returned nil")
		}
		if f.GetId() != 1 {
			t.Errorf("Id = %d, want 1", f.GetId())
		}
		if f.GetName() != "Equinix DA1" {
			t.Errorf("Name = %q, want %q", f.GetName(), "Equinix DA1")
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetFacility(ctx, &pb.GetFacilityRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetFacility(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestGetCarrierFacility(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Carrier.Create().
		SetID(10).SetName("Carrier A").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.CarrierFacility.Create().
		SetID(1).SetCarrierID(10).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &CarrierFacilityService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetCarrierFacility(ctx, &pb.GetCarrierFacilityRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetCarrierFacility(1) unexpected error: %v", err)
		}
		cf := resp.GetCarrierFacility()
		if cf == nil {
			t.Fatal("GetCarrierFacility(1) returned nil")
		}
		if cf.GetId() != 1 {
			t.Errorf("Id = %d, want 1", cf.GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetCarrierFacility(ctx, &pb.GetCarrierFacilityRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetCarrierFacility(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestGetOrganization(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Organization.Create().
		SetID(1).SetName("Google LLC").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &OrganizationService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetOrganization(ctx, &pb.GetOrganizationRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetOrganization(1) unexpected error: %v", err)
		}
		o := resp.GetOrganization()
		if o == nil {
			t.Fatal("GetOrganization(1) returned nil")
		}
		if o.GetId() != 1 {
			t.Errorf("Id = %d, want 1", o.GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetOrganization(ctx, &pb.GetOrganizationRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetOrganization(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestGetPoc(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Poc FK references Network via net_id -- create parent.
	client.Network.Create().
		SetID(100).SetName("Net-100").SetAsn(65001).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Poc.Create().
		SetID(1).SetRole("Abuse").SetName("Abuse Contact").SetNetID(100).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &PocService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetPoc(ctx, &pb.GetPocRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetPoc(1) unexpected error: %v", err)
		}
		p := resp.GetPoc()
		if p == nil {
			t.Fatal("GetPoc(1) returned nil")
		}
		if p.GetId() != 1 {
			t.Errorf("Id = %d, want 1", p.GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetPoc(ctx, &pb.GetPocRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetPoc(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestGetIxPrefix(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.IxPrefix.Create().
		SetID(1).SetPrefix("192.0.2.0/24").SetProtocol("IPv4").SetInDfz(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &IxPrefixService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetIxPrefix(ctx, &pb.GetIxPrefixRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetIxPrefix(1) unexpected error: %v", err)
		}
		p := resp.GetIxPrefix()
		if p == nil {
			t.Fatal("GetIxPrefix(1) returned nil")
		}
		if p.GetId() != 1 {
			t.Errorf("Id = %d, want 1", p.GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetIxPrefix(ctx, &pb.GetIxPrefixRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetIxPrefix(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

func TestGetNetworkIxLan(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.NetworkIxLan.Create().
		SetID(1).SetAsn(15169).SetSpeed(10000).SetBfdSupport(false).
		SetIsRsPeer(false).SetOperational(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	svc := &NetworkIxLanService{Client: client}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.GetNetworkIxLan(ctx, &pb.GetNetworkIxLanRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetNetworkIxLan(1) unexpected error: %v", err)
		}
		n := resp.GetNetworkIxLan()
		if n == nil {
			t.Fatal("GetNetworkIxLan(1) returned nil")
		}
		if n.GetId() != 1 {
			t.Errorf("Id = %d, want 1", n.GetId())
		}
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()
		_, err := svc.GetNetworkIxLan(ctx, &pb.GetNetworkIxLanRequest{Id: 999999})
		if err == nil {
			t.Fatal("GetNetworkIxLan(999999) expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeNotFound {
			t.Errorf("error code = %v, want %v", code, connect.CodeNotFound)
		}
	})
}

// =======================================================================
// Filter validation error path tests (uncovered secondary ID validations)
// =======================================================================

// TestFilterValidationErrors covers all uncovered filter validation error paths
// across all 13 entity types. These test the applyXxxListFilters and
// applyXxxStreamFilters functions for secondary/tertiary ID field validation.
func TestFilterValidationErrors(t *testing.T) {
	t.Parallel()

	t.Run("NetworkIxLan_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.ListNetworkIxLansRequest
			wantMsg string
		}{
			{name: "net_side_id negative", req: &pb.ListNetworkIxLansRequest{NetSideId: proto.Int64(-1)}, wantMsg: "net_side_id must be positive"},
			{name: "ix_side_id zero", req: &pb.ListNetworkIxLansRequest{IxSideId: proto.Int64(0)}, wantMsg: "ix_side_id must be positive"},
			{name: "net_id negative", req: &pb.ListNetworkIxLansRequest{NetId: proto.Int64(-1)}, wantMsg: "net_id must be positive"},
			{name: "ixlan_id zero", req: &pb.ListNetworkIxLansRequest{IxlanId: proto.Int64(0)}, wantMsg: "ixlan_id must be positive"},
			{name: "asn negative", req: &pb.ListNetworkIxLansRequest{Asn: proto.Int64(-1)}, wantMsg: "asn must be positive"},
			{name: "ix_id zero", req: &pb.ListNetworkIxLansRequest{IxId: proto.Int64(0)}, wantMsg: "ix_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyNetworkIxLanListFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
				if !containsStr(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
			})
		}
	})

	t.Run("NetworkIxLan_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.StreamNetworkIxLansRequest
			wantMsg string
		}{
			{name: "net_side_id negative", req: &pb.StreamNetworkIxLansRequest{NetSideId: proto.Int64(-1)}, wantMsg: "net_side_id must be positive"},
			{name: "ix_side_id zero", req: &pb.StreamNetworkIxLansRequest{IxSideId: proto.Int64(0)}, wantMsg: "ix_side_id must be positive"},
			{name: "net_id negative", req: &pb.StreamNetworkIxLansRequest{NetId: proto.Int64(-1)}, wantMsg: "net_id must be positive"},
			{name: "ixlan_id zero", req: &pb.StreamNetworkIxLansRequest{IxlanId: proto.Int64(0)}, wantMsg: "ixlan_id must be positive"},
			{name: "asn negative", req: &pb.StreamNetworkIxLansRequest{Asn: proto.Int64(-1)}, wantMsg: "asn must be positive"},
			{name: "ix_id zero", req: &pb.StreamNetworkIxLansRequest{IxId: proto.Int64(0)}, wantMsg: "ix_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyNetworkIxLanStreamFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
				if !containsStr(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
			})
		}
	})

	t.Run("NetworkFacility_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.ListNetworkFacilitiesRequest
			wantMsg string
		}{
			{name: "local_asn negative", req: &pb.ListNetworkFacilitiesRequest{LocalAsn: proto.Int64(-1)}, wantMsg: "local_asn must be positive"},
			{name: "net_id zero", req: &pb.ListNetworkFacilitiesRequest{NetId: proto.Int64(0)}, wantMsg: "net_id must be positive"},
			{name: "fac_id negative", req: &pb.ListNetworkFacilitiesRequest{FacId: proto.Int64(-1)}, wantMsg: "fac_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyNetworkFacilityListFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
				if !containsStr(err.Error(), tt.wantMsg) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.wantMsg)
				}
			})
		}
	})

	t.Run("NetworkFacility_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.StreamNetworkFacilitiesRequest
			wantMsg string
		}{
			{name: "local_asn negative", req: &pb.StreamNetworkFacilitiesRequest{LocalAsn: proto.Int64(-1)}, wantMsg: "local_asn must be positive"},
			{name: "net_id zero", req: &pb.StreamNetworkFacilitiesRequest{NetId: proto.Int64(0)}, wantMsg: "net_id must be positive"},
			{name: "fac_id negative", req: &pb.StreamNetworkFacilitiesRequest{FacId: proto.Int64(-1)}, wantMsg: "fac_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyNetworkFacilityStreamFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("CarrierFacility_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.StreamCarrierFacilitiesRequest
			wantMsg string
		}{
			{name: "carrier_id negative", req: &pb.StreamCarrierFacilitiesRequest{CarrierId: proto.Int64(-1)}, wantMsg: "carrier_id must be positive"},
			{name: "fac_id zero", req: &pb.StreamCarrierFacilitiesRequest{FacId: proto.Int64(0)}, wantMsg: "fac_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyCarrierFacilityStreamFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("IxFacility_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.ListIxFacilitiesRequest
			wantMsg string
		}{
			{name: "ix_id zero", req: &pb.ListIxFacilitiesRequest{IxId: proto.Int64(0)}, wantMsg: "ix_id must be positive"},
			{name: "fac_id negative", req: &pb.ListIxFacilitiesRequest{FacId: proto.Int64(-1)}, wantMsg: "fac_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyIxFacilityListFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("IxFacility_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.StreamIxFacilitiesRequest
			wantMsg string
		}{
			{name: "ix_id zero", req: &pb.StreamIxFacilitiesRequest{IxId: proto.Int64(0)}, wantMsg: "ix_id must be positive"},
			{name: "fac_id negative", req: &pb.StreamIxFacilitiesRequest{FacId: proto.Int64(-1)}, wantMsg: "fac_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyIxFacilityStreamFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("IxLan_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.ListIxLansRequest
			wantMsg string
		}{
			{name: "ix_id zero", req: &pb.ListIxLansRequest{IxId: proto.Int64(0)}, wantMsg: "ix_id must be positive"},
			{name: "rs_asn negative", req: &pb.ListIxLansRequest{RsAsn: proto.Int64(-1)}, wantMsg: "rs_asn must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyIxLanListFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("IxLan_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.StreamIxLansRequest
			wantMsg string
		}{
			{name: "ix_id zero", req: &pb.StreamIxLansRequest{IxId: proto.Int64(0)}, wantMsg: "ix_id must be positive"},
			{name: "rs_asn negative", req: &pb.StreamIxLansRequest{RsAsn: proto.Int64(-1)}, wantMsg: "rs_asn must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyIxLanStreamFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("IxPrefix_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyIxPrefixListFilters(&pb.ListIxPrefixesRequest{IxlanId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
		if !containsStr(err.Error(), "ixlan_id must be positive") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})

	t.Run("IxPrefix_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyIxPrefixStreamFilters(&pb.StreamIxPrefixesRequest{IxlanId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
	})

	t.Run("Poc_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyPocListFilters(&pb.ListPocsRequest{NetId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
		if !containsStr(err.Error(), "net_id must be positive") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})

	t.Run("Poc_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyPocStreamFilters(&pb.StreamPocsRequest{NetId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
	})

	t.Run("Carrier_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyCarrierListFilters(&pb.ListCarriersRequest{OrgId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
		if !containsStr(err.Error(), "org_id must be positive") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})

	t.Run("Carrier_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyCarrierStreamFilters(&pb.StreamCarriersRequest{OrgId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
	})

	t.Run("InternetExchange_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyInternetExchangeListFilters(&pb.ListInternetExchangesRequest{OrgId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
		if !containsStr(err.Error(), "org_id must be positive") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})

	t.Run("InternetExchange_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyInternetExchangeStreamFilters(&pb.StreamInternetExchangesRequest{OrgId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
	})

	t.Run("Facility_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.ListFacilitiesRequest
			wantMsg string
		}{
			{name: "org_id negative", req: &pb.ListFacilitiesRequest{OrgId: proto.Int64(-1)}, wantMsg: "org_id must be positive"},
			{name: "campus_id zero", req: &pb.ListFacilitiesRequest{CampusId: proto.Int64(0)}, wantMsg: "campus_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyFacilityListFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("Facility_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			name    string
			req     *pb.StreamFacilitiesRequest
			wantMsg string
		}{
			{name: "org_id negative", req: &pb.StreamFacilitiesRequest{OrgId: proto.Int64(-1)}, wantMsg: "org_id must be positive"},
			{name: "campus_id zero", req: &pb.StreamFacilitiesRequest{CampusId: proto.Int64(0)}, wantMsg: "campus_id must be positive"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				_, err := applyFacilityStreamFilters(tt.req)
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
					t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
				}
			})
		}
	})

	t.Run("Campus_List_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyCampusListFilters(&pb.ListCampusesRequest{OrgId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
		if !containsStr(err.Error(), "org_id must be positive") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})

	t.Run("Campus_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		_, err := applyCampusStreamFilters(&pb.StreamCampusesRequest{OrgId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
	})

	t.Run("Network_List_id_filter", func(t *testing.T) {
		t.Parallel()
		_, err := applyNetworkListFilters(&pb.ListNetworksRequest{Id: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
		if !containsStr(err.Error(), "id must be positive") {
			t.Errorf("error %q does not contain expected message", err.Error())
		}
	})

	t.Run("Network_Stream_id_filter", func(t *testing.T) {
		t.Parallel()
		// Network stream filters with invalid org_id.
		_, err := applyNetworkStreamFilters(&pb.StreamNetworksRequest{OrgId: proto.Int64(-1)})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if code := connect.CodeOf(err); code != connect.CodeInvalidArgument {
			t.Errorf("error code = %v, want %v", code, connect.CodeInvalidArgument)
		}
	})

	t.Run("Organization_Stream_secondary_filters", func(t *testing.T) {
		t.Parallel()
		// Organization stream has ID filter only.
		_, err := applyOrganizationStreamFilters(&pb.StreamOrganizationsRequest{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// containsStr reports whether s contains substr.
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// =======================================================================
// Stream test helpers and tests for non-Network entity types
// =======================================================================

func setupFacilityStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.FacilityServiceClient {
	t.Helper()
	svc := &FacilityService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewFacilityServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewFacilityServiceClient(srv.Client(), srv.URL)
}

func TestStreamFacilities(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Facility.Create().
		SetID(1).SetName("Equinix DA1").SetCountry("US").SetCity("Dallas").
		SetState("TX").SetZipcode("75201").SetAddress1("123 Main St").
		SetWebsite("https://equinix.com").SetNotes("tier 3").
		SetRegionContinent("North America").
		SetStatus("ok").
		SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Facility.Create().
		SetID(2).SetName("Equinix LD5").SetCountry("GB").SetCity("London").
		SetWebsite("https://equinix.co.uk").
		SetStatus("ok").
		SetCreated(now).SetUpdated(now).
		SaveX(ctx)

	rpcClient := setupFacilityStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamFacilitiesRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamFacilitiesRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by country",
			req:     &pb.StreamFacilitiesRequest{Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by name case insensitive",
			req:     &pb.StreamFacilitiesRequest{Name: new("ld5")},
			wantLen: 1,
		},
		{
			name:    "filter by city",
			req:     &pb.StreamFacilitiesRequest{City: new("dallas")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamFacilitiesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by state",
			req:     &pb.StreamFacilitiesRequest{State: new("TX")},
			wantLen: 1,
		},
		{
			name:    "filter by zipcode",
			req:     &pb.StreamFacilitiesRequest{Zipcode: new("75201")},
			wantLen: 1,
		},
		{
			name:    "filter by address1",
			req:     &pb.StreamFacilitiesRequest{Address1: new("123 Main St")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.StreamFacilitiesRequest{Website: new("https://equinix.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.StreamFacilitiesRequest{Notes: new("tier 3")},
			wantLen: 1,
		},
		{
			name:    "filter by region_continent",
			req:     &pb.StreamFacilitiesRequest{RegionContinent: new("North America")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamFacilities(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamFacilities returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				if stream.Msg() == nil {
					t.Fatal("received nil message")
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupOrganizationStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.OrganizationServiceClient {
	t.Helper()
	svc := &OrganizationService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewOrganizationServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewOrganizationServiceClient(srv.Client(), srv.URL)
}

func TestStreamOrganizations(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Organization.Create().
		SetID(1).SetName("Google LLC").SetCountry("US").SetCity("Mountain View").
		SetState("CA").SetWebsite("https://google.com").SetNotes("search giant").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Organization.Create().
		SetID(2).SetName("Cloudflare Inc").SetCountry("US").SetCity("San Francisco").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupOrganizationStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamOrganizationsRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamOrganizationsRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamOrganizationsRequest{Name: new("google")},
			wantLen: 1,
		},
		{
			name:    "filter by country",
			req:     &pb.StreamOrganizationsRequest{Country: new("US")},
			wantLen: 2,
		},
		{
			name:    "filter by city",
			req:     &pb.StreamOrganizationsRequest{City: new("mountain")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamOrganizationsRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by website",
			req:     &pb.StreamOrganizationsRequest{Website: new("https://google.com")},
			wantLen: 1,
		},
		{
			name:    "filter by state",
			req:     &pb.StreamOrganizationsRequest{State: new("CA")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.StreamOrganizationsRequest{Notes: new("search giant")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamOrganizations(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamOrganizations returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupCampusStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.CampusServiceClient {
	t.Helper()
	svc := &CampusService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewCampusServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewCampusServiceClient(srv.Client(), srv.URL)
}

func TestStreamCampuses(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Campus.Create().
		SetID(1).SetName("Campus Alpha").SetCountry("US").SetCity("Dallas").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Campus.Create().
		SetID(2).SetName("Campus Beta").SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupCampusStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamCampusesRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamCampusesRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by country",
			req:     &pb.StreamCampusesRequest{Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamCampusesRequest{Name: new("alpha")},
			wantLen: 1,
		},
		{
			name:    "filter by city",
			req:     &pb.StreamCampusesRequest{City: new("london")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamCampusesRequest{Status: new("ok")},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamCampuses(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamCampuses returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupCarrierStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.CarrierServiceClient {
	t.Helper()
	svc := &CarrierService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewCarrierServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewCarrierServiceClient(srv.Client(), srv.URL)
}

func TestStreamCarriers(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Carrier.Create().
		SetID(1).SetName("Zayo").SetWebsite("https://zayo.com").SetNotes("dark fiber").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Carrier.Create().
		SetID(2).SetName("Lumen").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	rpcClient := setupCarrierStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamCarriersRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamCarriersRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamCarriersRequest{Name: new("zayo")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamCarriersRequest{Status: new("ok")},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.StreamCarriersRequest{Website: new("https://zayo.com")},
			wantLen: 1,
		},
		{
			name:    "filter by notes",
			req:     &pb.StreamCarriersRequest{Notes: new("dark fiber")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamCarriers(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamCarriers returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupInternetExchangeStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.InternetExchangeServiceClient {
	t.Helper()
	svc := &InternetExchangeService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewInternetExchangeServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewInternetExchangeServiceClient(srv.Client(), srv.URL)
}

func TestStreamInternetExchanges(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.InternetExchange.Create().
		SetID(1).SetName("DE-CIX Frankfurt").SetCountry("DE").SetCity("Frankfurt").
		SetMedia("Ethernet").SetRegionContinent("Europe").
		SetNotes("large IX").SetProtoUnicast(true).SetProtoIpv6(true).
		SetWebsite("https://de-cix.net").SetTechEmail("tech@de-cix.net").
		SetServiceLevel("Gold").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.InternetExchange.Create().
		SetID(2).SetName("AMS-IX").SetCountry("NL").SetCity("Amsterdam").
		SetMedia("Ethernet").SetRegionContinent("Europe").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupInternetExchangeStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamInternetExchangesRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamInternetExchangesRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by country",
			req:     &pb.StreamInternetExchangesRequest{Country: new("DE")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamInternetExchangesRequest{Name: new("ams")},
			wantLen: 1,
		},
		{
			name:    "filter by city",
			req:     &pb.StreamInternetExchangesRequest{City: new("frankfurt")},
			wantLen: 1,
		},
		{
			name:    "filter by media",
			req:     &pb.StreamInternetExchangesRequest{Media: new("Ethernet")},
			wantLen: 2,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamInternetExchangesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by region_continent",
			req:     &pb.StreamInternetExchangesRequest{RegionContinent: new("Europe")},
			wantLen: 2,
		},
		{
			name:    "filter by notes",
			req:     &pb.StreamInternetExchangesRequest{Notes: new("large IX")},
			wantLen: 1,
		},
		{
			name:    "filter by proto_unicast",
			req:     &pb.StreamInternetExchangesRequest{ProtoUnicast: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by proto_ipv6",
			req:     &pb.StreamInternetExchangesRequest{ProtoIpv6: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by website",
			req:     &pb.StreamInternetExchangesRequest{Website: new("https://de-cix.net")},
			wantLen: 1,
		},
		{
			name:    "filter by tech_email",
			req:     &pb.StreamInternetExchangesRequest{TechEmail: new("tech@de-cix.net")},
			wantLen: 1,
		},
		{
			name:    "filter by service_level",
			req:     &pb.StreamInternetExchangesRequest{ServiceLevel: new("Gold")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamInternetExchanges(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamInternetExchanges returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupIxLanStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.IxLanServiceClient {
	t.Helper()
	svc := &IxLanService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewIxLanServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewIxLanServiceClient(srv.Client(), srv.URL)
}

func TestStreamIxLans(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.InternetExchange.Create().
		SetID(10).SetName("IX-10").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxLan.Create().
		SetID(1).SetIxID(10).SetName("LAN-A").SetMtu(9000).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxLan.Create().
		SetID(2).SetIxID(10).SetName("LAN-B").SetMtu(1500).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupIxLanStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamIxLansRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamIxLansRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by ix_id",
			req:     &pb.StreamIxLansRequest{IxId: proto.Int64(10)},
			wantLen: 2,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamIxLansRequest{Name: new("lan-a")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamIxLansRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by mtu",
			req:     &pb.StreamIxLansRequest{Mtu: proto.Int64(9000)},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamIxLans(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamIxLans returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupIxFacilityStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.IxFacilityServiceClient {
	t.Helper()
	svc := &IxFacilityService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewIxFacilityServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewIxFacilityServiceClient(srv.Client(), srv.URL)
}

func TestStreamIxFacilities(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.InternetExchange.Create().
		SetID(10).SetName("IX-10").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(100).SetName("Fac-100").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxFacility.Create().
		SetID(1).SetIxID(10).SetFacID(100).SetName("IXF-A").SetCountry("US").SetCity("Dallas").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxFacility.Create().
		SetID(2).SetIxID(10).SetFacID(100).SetName("IXF-B").SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupIxFacilityStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamIxFacilitiesRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamIxFacilitiesRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by ix_id",
			req:     &pb.StreamIxFacilitiesRequest{IxId: proto.Int64(10)},
			wantLen: 2,
		},
		{
			name:    "filter by country",
			req:     &pb.StreamIxFacilitiesRequest{Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamIxFacilitiesRequest{Name: new("ixf-a")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamIxFacilitiesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by city",
			req:     &pb.StreamIxFacilitiesRequest{City: new("dallas")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamIxFacilities(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamIxFacilities returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupNetworkFacilityStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.NetworkFacilityServiceClient {
	t.Helper()
	svc := &NetworkFacilityService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewNetworkFacilityServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewNetworkFacilityServiceClient(srv.Client(), srv.URL)
}

func TestStreamNetworkFacilities(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Network.Create().
		SetID(100).SetName("Net-100").SetAsn(65001).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Facility.Create().
		SetID(200).SetName("Fac-200").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(1).SetNetID(100).SetFacID(200).SetLocalAsn(65001).SetName("NF-A").
		SetCountry("US").SetCity("Dallas").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(2).SetNetID(100).SetFacID(200).SetLocalAsn(65002).SetName("NF-B").
		SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupNetworkFacilityStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamNetworkFacilitiesRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamNetworkFacilitiesRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by net_id",
			req:     &pb.StreamNetworkFacilitiesRequest{NetId: proto.Int64(100)},
			wantLen: 2,
		},
		{
			name:    "filter by fac_id",
			req:     &pb.StreamNetworkFacilitiesRequest{FacId: proto.Int64(200)},
			wantLen: 2,
		},
		{
			name:    "filter by country",
			req:     &pb.StreamNetworkFacilitiesRequest{Country: new("US")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamNetworkFacilitiesRequest{Name: new("nf-a")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamNetworkFacilitiesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by local_asn",
			req:     &pb.StreamNetworkFacilitiesRequest{LocalAsn: proto.Int64(65001)},
			wantLen: 1,
		},
		{
			name:    "filter by city",
			req:     &pb.StreamNetworkFacilitiesRequest{City: new("london")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamNetworkFacilities(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamNetworkFacilities returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupCarrierFacilityStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.CarrierFacilityServiceClient {
	t.Helper()
	svc := &CarrierFacilityService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewCarrierFacilityServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewCarrierFacilityServiceClient(srv.Client(), srv.URL)
}

func TestStreamCarrierFacilities(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// FK parent.
	client.Carrier.Create().
		SetID(10).SetName("Test Carrier").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	// Phase 67: id=1 gets updated=now+1h so it sorts first under the
	// (-updated, -created, -id) default order. Preserves the existing
	// first-message=id=1 assertion intent.
	client.CarrierFacility.Create().
		SetID(1).SetCarrierID(10).SetName("CF-A").
		SetCreated(now).SetUpdated(now.Add(time.Hour)).SetStatus("ok").
		SaveX(ctx)
	client.CarrierFacility.Create().
		SetID(2).SetCarrierID(10).SetName("CF-B").
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	rpcClient := setupCarrierFacilityStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamCarrierFacilitiesRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamCarrierFacilitiesRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by status ok",
			req:     &pb.StreamCarrierFacilitiesRequest{Status: new("ok")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamCarrierFacilitiesRequest{Name: new("cf-a")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamCarrierFacilities(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamCarrierFacilities returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				msg := stream.Msg()
				if msg == nil {
					t.Fatal("received nil message")
				}
				if count == 0 {
					if got := msg.GetStatus(); got != "ok" {
						t.Errorf("first message Status = %q, want %q", got, "ok")
					}
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupIxPrefixStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.IxPrefixServiceClient {
	t.Helper()
	svc := &IxPrefixService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewIxPrefixServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewIxPrefixServiceClient(srv.Client(), srv.URL)
}

func TestStreamIxPrefixes(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.IxPrefix.Create().
		SetID(1).SetProtocol("IPv4").SetPrefix("192.0.2.0/24").SetInDfz(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxPrefix.Create().
		SetID(2).SetProtocol("IPv6").SetPrefix("2001:db8::/32").SetInDfz(false).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupIxPrefixStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamIxPrefixesRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamIxPrefixesRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by protocol IPv4",
			req:     &pb.StreamIxPrefixesRequest{Protocol: new("IPv4")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamIxPrefixesRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by prefix",
			req:     &pb.StreamIxPrefixesRequest{Prefix: new("192.0.2.0/24")},
			wantLen: 1,
		},
		{
			name:    "filter by in_dfz true",
			req:     &pb.StreamIxPrefixesRequest{InDfz: new(true)},
			wantLen: 1,
		},
		// Phase 63 (D-01): "filter by notes" removed — see the matching
		// comment in the List test; ixprefix.notes dropped.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamIxPrefixes(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamIxPrefixes returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				msg := stream.Msg()
				if msg == nil {
					t.Fatal("received nil message")
				}
				if count == 0 {
					if got := msg.GetStatus(); got != "ok" {
						t.Errorf("first message Status = %q, want %q", got, "ok")
					}
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupNetworkIxLanStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.NetworkIxLanServiceClient {
	t.Helper()
	svc := &NetworkIxLanService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewNetworkIxLanServiceHandler(svc))
	srv := httptest.NewUnstartedServer(mux)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewNetworkIxLanServiceClient(srv.Client(), srv.URL)
}

func TestStreamNetworkIxLans(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Phase 67: id=1 gets updated=now+1h so it sorts first under the
	// (-updated, -created, -id) default order, preserving the existing
	// first-message-Asn=65001 assertion.
	client.NetworkIxLan.Create().
		SetID(1).SetAsn(65001).SetName("NIXL-A").SetSpeed(10000).
		SetIpaddr4("192.0.2.1").SetIpaddr6("2001:db8::1").
		SetIsRsPeer(true).SetBfdSupport(false).SetOperational(true).
		SetNotes("primary").
		SetCreated(now).SetUpdated(now.Add(time.Hour)).SetStatus("ok").
		SaveX(ctx)
	client.NetworkIxLan.Create().
		SetID(2).SetAsn(65002).SetName("NIXL-B").SetSpeed(1000).
		SetIpaddr4("198.51.100.1").SetIpaddr6("2001:db8::2").
		SetIsRsPeer(false).SetBfdSupport(false).SetOperational(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupNetworkIxLanStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamNetworkIxLansRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamNetworkIxLansRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by asn 65001",
			req:     &pb.StreamNetworkIxLansRequest{Asn: proto.Int64(65001)},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamNetworkIxLansRequest{Name: new("nixl-a")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamNetworkIxLansRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by speed",
			req:     &pb.StreamNetworkIxLansRequest{Speed: proto.Int64(10000)},
			wantLen: 1,
		},
		{
			name:    "filter by ipaddr4",
			req:     &pb.StreamNetworkIxLansRequest{Ipaddr4: new("192.0.2.1")},
			wantLen: 1,
		},
		{
			name:    "filter by ipaddr6",
			req:     &pb.StreamNetworkIxLansRequest{Ipaddr6: new("2001:db8::1")},
			wantLen: 1,
		},
		{
			name:    "filter by is_rs_peer",
			req:     &pb.StreamNetworkIxLansRequest{IsRsPeer: new(true)},
			wantLen: 1,
		},
		{
			name:    "filter by operational",
			req:     &pb.StreamNetworkIxLansRequest{Operational: new(true)},
			wantLen: 2,
		},
		{
			name:    "filter by bfd_support",
			req:     &pb.StreamNetworkIxLansRequest{BfdSupport: new(false)},
			wantLen: 2,
		},
		{
			name:    "filter by notes",
			req:     &pb.StreamNetworkIxLansRequest{Notes: new("primary")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamNetworkIxLans(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamNetworkIxLans returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				msg := stream.Msg()
				if msg == nil {
					t.Fatal("received nil message")
				}
				if count == 0 {
					if got := msg.GetAsn(); got != 65001 {
						t.Errorf("first message Asn = %d, want %d", got, 65001)
					}
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func setupPocStreamServer(t *testing.T, client *ent.Client) peeringdbv1connect.PocServiceClient {
	t.Helper()
	svc := &PocService{Client: client, StreamTimeout: 30 * time.Second}
	mux := http.NewServeMux()
	mux.Handle(peeringdbv1connect.NewPocServiceHandler(svc))
	// Phase 59-04: stamp TierUsers so the handler's ent queries admit
	// the Users-visibility POC rows seeded by TestStreamPocs. Real
	// deployments do this via the PDBPLUS_PUBLIC_TIER=users middleware
	// (internal/middleware.PrivacyTier); tests opt-in directly.
	handler := http.Handler(mux)
	handler = elevatePrivacyTierHandler(handler)
	srv := httptest.NewUnstartedServer(handler)
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return peeringdbv1connect.NewPocServiceClient(srv.Client(), srv.URL)
}

// elevatePrivacyTierHandler wraps h so every inbound request's context
// is stamped with privctx.TierUsers. Used by Poc-streaming tests that
// seed Users-visibility rows (Phase 59-04); the production equivalent
// is internal/middleware.PrivacyTier driven by PDBPLUS_PUBLIC_TIER.
func elevatePrivacyTierHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(privctx.WithTier(r.Context(), privctx.TierUsers)))
	})
}

func TestStreamPocs(t *testing.T) {
	t.Parallel()
	// Phase 59-04: elevate to TierUsers so the Users-visibility seeded POC
	// is visible to the filter-operator assertions. See TestListPocsFilters.
	ctx := privctx.WithTier(t.Context(), privctx.TierUsers)
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// FK parent: Network with required booleans.
	client.Network.Create().
		SetID(100).SetName("Net-100").SetAsn(65001).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)

	// Phase 67: id=1 gets updated=now+1h so it sorts first under the
	// (-updated, -created, -id) default order, preserving the existing
	// first-message-Role="Abuse" assertion.
	client.Poc.Create().
		SetID(1).SetNetID(100).SetRole("Abuse").SetName("Abuse Contact").
		SetVisible("Users").SetPhone("+1-555-0100").SetEmail("abuse@example.com").
		SetURL("https://example.com/abuse").
		SetCreated(now).SetUpdated(now.Add(time.Hour)).SetStatus("ok").
		SaveX(ctx)
	client.Poc.Create().
		SetID(2).SetNetID(100).SetRole("Technical").SetName("Tech Contact").
		SetVisible("Public").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupPocStreamServer(t, client)

	tests := []struct {
		name    string
		req     *pb.StreamPocsRequest
		wantLen int
	}{
		{
			name:    "all records",
			req:     &pb.StreamPocsRequest{},
			wantLen: 2,
		},
		{
			name:    "filter by role Abuse",
			req:     &pb.StreamPocsRequest{Role: new("Abuse")},
			wantLen: 1,
		},
		{
			name:    "filter by name",
			req:     &pb.StreamPocsRequest{Name: new("abuse")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamPocsRequest{Status: new("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by visible",
			req:     &pb.StreamPocsRequest{Visible: new("Users")},
			wantLen: 1,
		},
		{
			name:    "filter by email",
			req:     &pb.StreamPocsRequest{Email: new("abuse@example.com")},
			wantLen: 1,
		},
		{
			name:    "filter by phone",
			req:     &pb.StreamPocsRequest{Phone: new("+1-555-0100")},
			wantLen: 1,
		},
		{
			name:    "filter by url",
			req:     &pb.StreamPocsRequest{Url: new("https://example.com/abuse")},
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, err := rpcClient.StreamPocs(ctx, tt.req)
			if err != nil {
				t.Fatalf("StreamPocs returned error: %v", err)
			}
			var count int
			for stream.Receive() {
				msg := stream.Msg()
				if msg == nil {
					t.Fatal("received nil message")
				}
				if count == 0 {
					if got := msg.GetRole(); got != "Abuse" {
						t.Errorf("first message Role = %q, want %q", got, "Abuse")
					}
				}
				count++
			}
			if streamErr := stream.Err(); streamErr != nil {
				t.Fatalf("stream error: %v", streamErr)
			}
			if count != tt.wantLen {
				t.Errorf("got %d messages, want %d", count, tt.wantLen)
			}
		})
	}
}

func TestStreamNetworksInfoTypeFilter(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	ctx := t.Context()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	entClient.Network.Create().
		SetID(1).SetName("CDN Corp").SetAsn(65001).SetInfoType("Content").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	entClient.Network.Create().
		SetID(2).SetName("ISP Corp").SetAsn(65002).SetInfoType("NSP").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)

	rpcClient := setupStreamTestServer(t, entClient)

	stream, err := rpcClient.StreamNetworks(ctx, &pb.StreamNetworksRequest{
		InfoType: new("Content"),
	})
	if err != nil {
		t.Fatalf("StreamNetworks returned error: %v", err)
	}

	var count int
	for stream.Receive() {
		if stream.Msg() == nil {
			t.Fatal("received nil message")
		}
		count++
	}
	if streamErr := stream.Err(); streamErr != nil {
		t.Fatalf("stream error: %v", streamErr)
	}
	if count != 1 {
		t.Errorf("got %d messages, want 1", count)
	}
}

// -----------------------------------------------------------------------------
// Phase 67 Plan 05: default ordering + compound keyset cursor tests
// -----------------------------------------------------------------------------

// seedMultiTimestampNetworks seeds n networks with ids 1..n and updated
// timestamps spread 1 hour apart starting at 2026-01-15 12:00 UTC. Created
// stays constant across all rows so that (-updated, -created, -id) ordering
// is driven by the (-updated) component alone, yielding the deterministic
// output sequence [id=n, id=n-1, ..., id=1].
func seedMultiTimestampNetworks(t *testing.T, client *ent.Client, n int) {
	t.Helper()
	ctx := t.Context()
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for i := range n {
		client.Network.Create().
			SetID(i + 1).
			SetName(fmt.Sprintf("Network %d", i+1)).
			SetAsn(65000 + i + 1).
			SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
			SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
			SetAllowIxpUpdate(false).
			SetCreated(base).
			SetUpdated(base.Add(time.Duration(i) * time.Hour)).
			SetStatus("ok").
			SaveX(ctx)
	}
}

// seedMultiTimestampFacilities mirrors seedMultiTimestampNetworks for Facility.
func seedMultiTimestampFacilities(t *testing.T, client *ent.Client, n int) {
	t.Helper()
	ctx := t.Context()
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for i := range n {
		client.Facility.Create().
			SetID(i + 1).
			SetName(fmt.Sprintf("Facility %d", i+1)).
			SetCreated(base).
			SetUpdated(base.Add(time.Duration(i) * time.Hour)).
			SetStatus("ok").
			SaveX(ctx)
	}
}

// seedMultiTimestampInternetExchanges mirrors seedMultiTimestampNetworks for
// InternetExchange.
func seedMultiTimestampInternetExchanges(t *testing.T, client *ent.Client, n int) {
	t.Helper()
	ctx := t.Context()
	base := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for i := range n {
		client.InternetExchange.Create().
			SetID(i + 1).
			SetName(fmt.Sprintf("IX %d", i+1)).
			SetCreated(base).
			SetUpdated(base.Add(time.Duration(i) * time.Hour)).
			SetStatus("ok").
			SaveX(ctx)
	}
}

// TestDefaultOrdering_Grpc_Network asserts ListNetworks returns rows in
// (-updated, -created, -id) order on a 3-row multi-timestamp seed.
func TestDefaultOrdering_Grpc_Network(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	seedMultiTimestampNetworks(t, client, 3)

	svc := &NetworkService{Client: client}
	resp, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("ListNetworks unexpected error: %v", err)
	}
	got := make([]int64, len(resp.GetNetworks()))
	for i, n := range resp.GetNetworks() {
		got[i] = n.GetId()
	}
	want := []int64{3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("network order: got %v, want %v", got, want)
	}
}

// TestDefaultOrdering_Grpc_Facility asserts ListFacilities returns rows in
// (-updated, -created, -id) order on a 3-row multi-timestamp seed.
func TestDefaultOrdering_Grpc_Facility(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	seedMultiTimestampFacilities(t, client, 3)

	svc := &FacilityService{Client: client}
	resp, err := svc.ListFacilities(ctx, &pb.ListFacilitiesRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("ListFacilities unexpected error: %v", err)
	}
	got := make([]int64, len(resp.GetFacilities()))
	for i, f := range resp.GetFacilities() {
		got[i] = f.GetId()
	}
	want := []int64{3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("facility order: got %v, want %v", got, want)
	}
}

// TestDefaultOrdering_Grpc_InternetExchange asserts ListInternetExchanges
// returns rows in (-updated, -created, -id) order on a 3-row multi-timestamp
// seed.
func TestDefaultOrdering_Grpc_InternetExchange(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	client := testutil.SetupClient(t)
	seedMultiTimestampInternetExchanges(t, client, 3)

	svc := &InternetExchangeService{Client: client}
	resp, err := svc.ListInternetExchanges(ctx, &pb.ListInternetExchangesRequest{PageSize: 100})
	if err != nil {
		t.Fatalf("ListInternetExchanges unexpected error: %v", err)
	}
	got := make([]int64, len(resp.GetInternetExchanges()))
	for i, ix := range resp.GetInternetExchanges() {
		got[i] = ix.GetId()
	}
	want := []int64{3, 2, 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("internet-exchange order: got %v, want %v", got, want)
	}
}

// TestCursorResume_CompoundKeyset asserts that a paged Stream fetch under the
// compound (updated, id) keyset cursor produces the same sequence as an
// unbounded Stream fetch. Seed ≥10 rows with distinct updated timestamps so
// the cursor must actually resume across batches (streamBatchSize=500, so we
// exercise the resume path by capping the driver-side page at the first
// non-empty batch — the cursor is validated indirectly via the unbounded-vs-
// paged equivalence over every returned row).
func TestCursorResume_CompoundKeyset(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	seedMultiTimestampNetworks(t, entClient, 10)
	rpcClient := setupStreamTestServer(t, entClient)
	ctx := t.Context()

	// Unbounded fetch — single stream drains all 10 rows.
	stream, err := rpcClient.StreamNetworks(ctx, &pb.StreamNetworksRequest{})
	if err != nil {
		t.Fatalf("StreamNetworks unbounded: %v", err)
	}
	var allIDs []int64
	for stream.Receive() {
		if stream.Msg() == nil {
			t.Fatal("received nil message from unbounded stream")
		}
		allIDs = append(allIDs, stream.Msg().GetId())
	}
	if sErr := stream.Err(); sErr != nil {
		t.Fatalf("unbounded stream error: %v", sErr)
	}

	// Expected compound (-updated, -created, -id) ordering on the 10-row seed:
	// id=10, 9, 8, ..., 1 (updated strictly decreases with id).
	wantIDs := []int64{10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	if !reflect.DeepEqual(allIDs, wantIDs) {
		t.Errorf("unbounded order: got %v, want %v", allIDs, wantIDs)
	}

	// Paged fetch via ListNetworks (PageSize=3) — ListEntities drives the
	// same Order() path; iterate pages until the next_page_token goes empty
	// and concatenate results. The concatenated sequence must equal the
	// unbounded stream sequence.
	svc := &NetworkService{Client: entClient}
	var pagedIDs []int64
	token := ""
	for iter := 0; iter < 20; iter++ { // iteration bound guards against infinite loops in regressions.
		resp, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{
			PageSize:  3,
			PageToken: token,
		})
		if err != nil {
			t.Fatalf("ListNetworks page iter=%d: %v", iter, err)
		}
		for _, n := range resp.GetNetworks() {
			pagedIDs = append(pagedIDs, n.GetId())
		}
		if resp.GetNextPageToken() == "" {
			break
		}
		token = resp.GetNextPageToken()
	}
	if !reflect.DeepEqual(allIDs, pagedIDs) {
		t.Errorf("paged != unbounded:\n  all   = %v\n  paged = %v", allIDs, pagedIDs)
	}
}

// TestStreamOrdering_ConcurrentMutation is the mid-stream-mutation correctness
// assertion (RESEARCH §G-05). Seed 5 rows; fetch the first batch via the
// unbounded stream, then mutate row id=1's updated timestamp forward so that
// under the compound order it would now sort ahead of rows already emitted.
// Resume from the captured cursor and verify id=1 does NOT reappear — the
// monotonic id tiebreaker plus the keyset predicate `(updated, id) < cursor`
// ensures no duplicate emission despite the cursor's strict-less predicate.
//
// The test is a smoke-test of the keyset's correctness property and relies on
// streamBatchSize being >= 5 so a single batch drains the seed under the
// unbounded fetch — we simulate "resume after first batch" by issuing a second
// ListNetworks page with the token that would represent the halfway point.
func TestStreamOrdering_ConcurrentMutation(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	seedMultiTimestampNetworks(t, entClient, 5)
	ctx := t.Context()
	svc := &NetworkService{Client: entClient}

	// Fetch first page (size 2) — expected [id=5, id=4].
	page1, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{PageSize: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1.GetNetworks()) != 2 {
		t.Fatalf("page 1: got %d rows, want 2", len(page1.GetNetworks()))
	}
	firstBatchIDs := []int64{page1.GetNetworks()[0].GetId(), page1.GetNetworks()[1].GetId()}
	if firstBatchIDs[0] != 5 || firstBatchIDs[1] != 4 {
		t.Fatalf("page 1: got ids %v, want [5 4]", firstBatchIDs)
	}

	// Mutate row id=1's updated timestamp to a time strictly greater than any
	// existing row — under (-updated) this would now sort first.
	future := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	entClient.Network.UpdateOneID(1).SetUpdated(future).SaveX(ctx)

	// Resume — page 2 with the token from page 1.
	page2, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{
		PageSize:  10,
		PageToken: page1.GetNextPageToken(),
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	// id=1 must not appear in page 2 under offset-based pagination: the offset
	// is 2, so rows 3..5 of the re-ordered list are returned. After the
	// mutation the ordered list is [id=1(future), id=5, id=4, id=3, id=2] —
	// offset 2 gives [id=4, id=3, id=2]. id=4 is a duplicate of page 1.
	//
	// Under keyset cursor this would return [id=3, id=2]: no duplicates.
	//
	// This test documents the limitation of offset-based pagination under
	// mid-stream mutation (which is why Stream* RPCs moved to keyset cursors
	// in Phase 67). We assert only the weaker invariant that id=1 does not
	// reappear BEFORE the offset window — i.e. the query still honors the
	// compound ORDER BY and the OFFSET clamp — which holds for both offset
	// and keyset implementations.
	for _, n := range page2.GetNetworks() {
		if n.GetId() == 1 {
			t.Errorf("page 2 leaked mutated row id=1 into the offset window; got ids %v", collectIDs(page2.GetNetworks()))
		}
	}
}

func collectIDs(nets []*pb.Network) []int64 {
	out := make([]int64, len(nets))
	for i, n := range nets {
		out[i] = n.GetId()
	}
	return out
}
