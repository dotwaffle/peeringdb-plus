package grpcserver

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/internal/testutil"
)

func TestGetNetwork(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Create 3 test networks with sequential IDs.
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
			SetUpdated(now).
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

	t.Run("ordering by ID ascending", func(t *testing.T) {
		t.Parallel()
		resp, err := svc.ListNetworks(ctx, &pb.ListNetworksRequest{})
		if err != nil {
			t.Fatalf("ListNetworks unexpected error: %v", err)
		}
		for i := 1; i < len(resp.GetNetworks()); i++ {
			prev := resp.GetNetworks()[i-1].GetId()
			curr := resp.GetNetworks()[i].GetId()
			if prev >= curr {
				t.Errorf("networks not ordered by ID ascending: id[%d]=%d >= id[%d]=%d", i-1, prev, i, curr)
			}
		}
	})
}

func TestListNetworksFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed test data with distinct values for filtering.
	client.Network.Create().
		SetID(1).SetName("Google").SetAsn(15169).SetStatus("ok").
		SetInfoUnicast(true).SetInfoMulticast(false).SetInfoIpv6(false).
		SetInfoNeverViaRouteServers(false).SetPolicyRatio(false).
		SetAllowIxpUpdate(false).SetCreated(now).SetUpdated(now).
		SaveX(ctx)
	client.Network.Create().
		SetID(2).SetName("Cloudflare").SetAsn(13335).SetStatus("ok").
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
			req:     &pb.ListNetworksRequest{Status: proto.String("ok")},
			wantLen: 2,
		},
		{
			name:    "filter by name substring case-insensitive",
			req:     &pb.ListNetworksRequest{Name: proto.String("cloud")},
			wantLen: 1,
		},
		{
			name:    "combined filters AND",
			req:     &pb.ListNetworksRequest{Status: proto.String("ok"), Asn: proto.Int64(15169)},
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 facilities with distinct country/city/name/status.
	client.Facility.Create().
		SetID(1).SetName("Equinix DA1").SetCountry("US").SetCity("Dallas").
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
			req:     &pb.ListFacilitiesRequest{Country: proto.String("US")},
			wantLen: 2,
		},
		{
			name:    "filter by city case-insensitive substring",
			req:     &pb.ListFacilitiesRequest{City: proto.String("dallas")},
			wantLen: 1,
		},
		{
			name:    "filter by name and country combined",
			req:     &pb.ListFacilitiesRequest{Name: proto.String("Equinix"), Country: proto.String("US")},
			wantLen: 1,
		},
		{
			name:    "invalid org_id returns INVALID_ARGUMENT",
			req:     &pb.ListFacilitiesRequest{OrgId: proto.Int64(-1)},
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 organizations with distinct name/status.
	client.Organization.Create().
		SetID(1).SetName("Google LLC").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Organization.Create().
		SetID(2).SetName("Cloudflare Inc").
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
	}{
		{
			name:    "filter by name substring case-insensitive",
			req:     &pb.ListOrganizationsRequest{Name: proto.String("google")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok excludes deleted",
			req:     &pb.ListOrganizationsRequest{Status: proto.String("ok")},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListOrganizations(ctx, tt.req)
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
	ctx := context.Background()
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
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Poc.Create().
		SetID(2).SetRole("Technical").SetName("Tech Contact").SetNetID(100).
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
			req:     &pb.ListPocsRequest{Role: proto.String("Abuse")},
			wantLen: 1,
		},
		{
			name:    "filter by net_id returns only that networks contacts",
			req:     &pb.ListPocsRequest{NetId: proto.Int64(100)},
			wantLen: 2,
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 prefixes with different protocols.
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
	}{
		{
			name:    "filter by protocol IPv4",
			req:     &pb.ListIxPrefixesRequest{Protocol: proto.String("IPv4")},
			wantLen: 2,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListIxPrefixesRequest{Status: proto.String("ok")},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListIxPrefixes(ctx, tt.req)
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	// Seed 3 network IX LANs with different ASNs.
	client.NetworkIxLan.Create().
		SetID(1).SetAsn(15169).SetSpeed(10000).SetBfdSupport(false).
		SetIsRsPeer(false).SetOperational(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkIxLan.Create().
		SetID(2).SetAsn(13335).SetSpeed(100000).SetBfdSupport(false).
		SetIsRsPeer(true).SetOperational(true).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkIxLan.Create().
		SetID(3).SetAsn(15169).SetSpeed(10000).SetBfdSupport(false).
		SetIsRsPeer(false).SetOperational(false).
		SetCreated(now).SetUpdated(now).SetStatus("deleted").
		SaveX(ctx)

	svc := &NetworkIxLanService{Client: client}

	tests := []struct {
		name    string
		req     *pb.ListNetworkIxLansRequest
		wantLen int
	}{
		{
			name:    "filter by asn",
			req:     &pb.ListNetworkIxLansRequest{Asn: proto.Int64(15169)},
			wantLen: 2,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListNetworkIxLansRequest{Status: proto.String("ok")},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := svc.ListNetworkIxLans(ctx, tt.req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(resp.GetNetworkIxLans()); got != tt.wantLen {
				t.Errorf("got %d network ix lans, want %d", got, tt.wantLen)
			}
		})
	}
}

func TestListCarrierFacilitiesFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
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
		SetID(1).SetCarrierID(10).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.CarrierFacility.Create().
		SetID(2).SetCarrierID(20).
		SetCreated(now).SetUpdated(now).SetStatus("ok").
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
			name:    "invalid carrier_id returns INVALID_ARGUMENT",
			req:     &pb.ListCarrierFacilitiesRequest{CarrierId: proto.Int64(-1)},
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

func TestListNetworksFiltersPaginated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
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
		Status:   proto.String("ok"),
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
		Status:    proto.String("ok"),
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
		Status:    proto.String("ok"),
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
