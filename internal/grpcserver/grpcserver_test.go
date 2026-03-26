package grpcserver

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/dotwaffle/peeringdb-plus/ent"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1/peeringdbv1connect"
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
func seedStreamNetworks(t *testing.T, client *ent.Client) {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

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
			req:     &pb.StreamNetworksRequest{Name: proto.String("cloud")},
			wantLen: 1,
		},
		{
			name:    "filter by status",
			req:     &pb.StreamNetworksRequest{Status: proto.String("ok")},
			wantLen: 2,
		},
		{
			name:    "combined filters",
			req:     &pb.StreamNetworksRequest{Asn: proto.Int64(15169), Status: proto.String("ok")},
			wantLen: 1,
		},
		{
			name:    "no matches returns empty stream",
			req:     &pb.StreamNetworksRequest{Asn: proto.Int64(99999)},
			wantLen: 0,
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
			ctx := context.Background()

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
			req:       &pb.StreamNetworksRequest{Status: proto.String("ok")},
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
			ctx := context.Background()

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

	ctx, cancel := context.WithCancel(context.Background())

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

	tests := []struct {
		name      string
		req       *pb.StreamNetworksRequest
		wantLen   int
		wantCount string
	}{
		{
			name:      "since_id returns records after given ID",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(1)},
			wantLen:   2,
			wantCount: "2",
		},
		{
			name:      "since_id beyond max returns empty",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(9999)},
			wantLen:   0,
			wantCount: "0",
		},
		{
			name:      "since_id with status filter composes via AND",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(1), Status: proto.String("ok")},
			wantLen:   1,
			wantCount: "1",
		},
		{
			name:      "since_id zero returns all (same as omitted)",
			req:       &pb.StreamNetworksRequest{SinceId: proto.Int64(0)},
			wantLen:   3,
			wantCount: "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entClient := testutil.SetupClient(t)
			seedStreamNetworks(t, entClient)
			rpcClient := setupStreamTestServer(t, entClient)
			ctx := context.Background()

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

			// Check response header for total count.
			got := stream.ResponseHeader().Get("Grpc-Total-Count")
			if got == "" {
				got = stream.ResponseHeader().Get("grpc-total-count")
			}
			if got != tt.wantCount {
				t.Errorf("grpc-total-count = %q, want %q", got, tt.wantCount)
			}
		})
	}
}

func TestStreamNetworksUpdatedSince(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		req       *pb.StreamNetworksRequest
		wantLen   int
		wantCount string
	}{
		{
			name: "updated_since before seed time returns all",
			req: &pb.StreamNetworksRequest{
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)),
			},
			wantLen:   3,
			wantCount: "3",
		},
		{
			name: "updated_since after seed time returns none",
			req: &pb.StreamNetworksRequest{
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 16, 0, 0, 0, 0, time.UTC)),
			},
			wantLen:   0,
			wantCount: "0",
		},
		{
			name: "updated_since with status filter composes via AND",
			req: &pb.StreamNetworksRequest{
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)),
				Status:       proto.String("ok"),
			},
			wantLen:   2,
			wantCount: "2",
		},
		{
			name: "since_id and updated_since compose together",
			req: &pb.StreamNetworksRequest{
				SinceId:      proto.Int64(1),
				UpdatedSince: timestamppb.New(time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC)),
			},
			wantLen:   2,
			wantCount: "2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			entClient := testutil.SetupClient(t)
			seedStreamNetworks(t, entClient)
			rpcClient := setupStreamTestServer(t, entClient)
			ctx := context.Background()

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

			// Check response header for total count.
			got := stream.ResponseHeader().Get("Grpc-Total-Count")
			if got == "" {
				got = stream.ResponseHeader().Get("grpc-total-count")
			}
			if got != tt.wantCount {
				t.Errorf("grpc-total-count = %q, want %q", got, tt.wantCount)
			}
		})
	}
}

// =======================================================================
// Missing entity type tests: Campus, Carrier, InternetExchange,
// IxFacility, IxLan, NetworkFacility
// =======================================================================

func TestGetCampus(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
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
	ctx := context.Background()
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
			req:     &pb.ListCampusesRequest{Country: proto.String("US")},
			wantLen: 2,
		},
		{
			name:    "filter by name case-insensitive",
			req:     &pb.ListCampusesRequest{Name: proto.String("alpha")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListCampusesRequest{Status: proto.String("ok")},
			wantLen: 2,
		},
		{
			name:    "combined country and status",
			req:     &pb.ListCampusesRequest{Country: proto.String("US"), Status: proto.String("ok")},
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
	ctx := context.Background()
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
	ctx := context.Background()
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
			req:     &pb.ListCarriersRequest{Name: proto.String("zayo")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListCarriersRequest{Status: proto.String("ok")},
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
	ctx := context.Background()
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
	ctx := context.Background()
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
			req:     &pb.ListInternetExchangesRequest{Country: proto.String("DE")},
			wantLen: 2,
		},
		{
			name:    "filter by name case-insensitive",
			req:     &pb.ListInternetExchangesRequest{Name: proto.String("ams")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListInternetExchangesRequest{Status: proto.String("ok")},
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
			req:     &pb.ListIxLansRequest{Name: proto.String("lan-a")},
			wantLen: 1,
		},
		{
			name:    "filter by status ok",
			req:     &pb.ListIxLansRequest{Status: proto.String("ok")},
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
	ctx := context.Background()
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
	ctx := context.Background()
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
			req:     &pb.ListNetworkFacilitiesRequest{Status: proto.String("ok")},
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
	ctx := context.Background()
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
			req:     &pb.ListNetworksRequest{InfoType: proto.String("Content")},
			wantLen: 1,
		},
		{
			name:    "filter by info_type NSP",
			req:     &pb.ListNetworksRequest{InfoType: proto.String("NSP")},
			wantLen: 1,
		},
		{
			name:    "filter by info_unicast true",
			req:     &pb.ListNetworksRequest{InfoUnicast: proto.Bool(true)},
			wantLen: 3,
		},
		{
			name:    "filter by info_ipv6 true",
			req:     &pb.ListNetworksRequest{InfoIpv6: proto.Bool(true)},
			wantLen: 1,
		},
		{
			name:    "combined info_type and ASN",
			req:     &pb.ListNetworksRequest{InfoType: proto.String("Content"), Asn: proto.Int64(65001)},
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Facility.Create().
		SetID(1).SetName("Equinix DA1").SetCountry("US").SetCity("Dallas").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Facility.Create().
		SetID(2).SetName("Equinix LD5").SetCountry("GB").SetCity("London").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
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
			req:     &pb.StreamFacilitiesRequest{Country: proto.String("US")},
			wantLen: 1,
		},
		{
			name:    "filter by name case insensitive",
			req:     &pb.StreamFacilitiesRequest{Name: proto.String("ld5")},
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Organization.Create().
		SetID(1).SetName("Google LLC").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Organization.Create().
		SetID(2).SetName("Cloudflare Inc").
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
			req:     &pb.StreamOrganizationsRequest{Name: proto.String("google")},
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.Campus.Create().
		SetID(1).SetName("Campus Alpha").SetCountry("US").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.Campus.Create().
		SetID(2).SetName("Campus Beta").SetCountry("GB").
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
			req:     &pb.StreamCampusesRequest{Country: proto.String("US")},
			wantLen: 1,
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
	ctx := context.Background()
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
			req:     &pb.StreamCarriersRequest{Name: proto.String("zayo")},
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.InternetExchange.Create().
		SetID(1).SetName("DE-CIX Frankfurt").SetCountry("DE").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.InternetExchange.Create().
		SetID(2).SetName("AMS-IX").SetCountry("NL").
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
			req:     &pb.StreamInternetExchangesRequest{Country: proto.String("DE")},
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
	ctx := context.Background()
	client := testutil.SetupClient(t)
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	client.InternetExchange.Create().
		SetID(10).SetName("IX-10").
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
			req:     &pb.StreamIxLansRequest{Name: proto.String("lan-a")},
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
	ctx := context.Background()
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
		SetID(1).SetIxID(10).SetFacID(100).SetName("IXF-A").
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.IxFacility.Create().
		SetID(2).SetIxID(10).SetFacID(100).SetName("IXF-B").
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
	ctx := context.Background()
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
		SetCreated(now).SetUpdated(now).SetStatus("ok").
		SaveX(ctx)
	client.NetworkFacility.Create().
		SetID(2).SetNetID(100).SetFacID(200).SetLocalAsn(65001).SetName("NF-B").
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

func TestStreamNetworksInfoTypeFilter(t *testing.T) {
	t.Parallel()
	entClient := testutil.SetupClient(t)
	ctx := context.Background()
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
		InfoType: proto.String("Content"),
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
