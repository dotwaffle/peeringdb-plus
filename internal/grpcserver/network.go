package grpcserver

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// NetworkService implements the peeringdb.v1.NetworkService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type NetworkService struct {
	Client *ent.Client
}

// GetNetwork returns a single network by ID. Returns NOT_FOUND if the network
// does not exist.
func (s *NetworkService) GetNetwork(ctx context.Context, req *pb.GetNetworkRequest) (*pb.GetNetworkResponse, error) {
	n, err := s.Client.Network.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity network %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get network %d: %w", req.GetId(), err))
	}
	return &pb.GetNetworkResponse{Network: networkToProto(n)}, nil
}

// ListNetworks returns a paginated list of networks ordered by ID ascending.
// Supports page_size, page_token, and optional filter fields (asn, name,
// status, org_id). Multiple filters combine with AND logic. Name uses
// case-insensitive substring matching; other fields use exact match.
func (s *NetworkService) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.Network
	if req.Asn != nil {
		if *req.Asn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: asn must be positive"))
		}
		predicates = append(predicates, network.AsnEQ(int(*req.Asn)))
	}
	if req.Name != nil {
		predicates = append(predicates, network.NameContainsFold(*req.Name))
	}
	if req.Status != nil {
		predicates = append(predicates, network.StatusEQ(*req.Status))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: org_id must be positive"))
		}
		predicates = append(predicates, network.OrgIDEQ(int(*req.OrgId)))
	}

	query := s.Client.Network.Query().
		Order(ent.Asc(network.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(network.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list networks: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	networks := make([]*pb.Network, len(results))
	for i, n := range results {
		networks[i] = networkToProto(n)
	}

	return &pb.ListNetworksResponse{
		Networks:      networks,
		NextPageToken: nextPageToken,
	}, nil
}

// networkToProto converts an ent Network entity to a protobuf Network message.
func networkToProto(n *ent.Network) *pb.Network {
	return &pb.Network{
		Id:                       int64(n.ID),
		OrgId:                    int64PtrVal(n.OrgID),
		Aka:                      stringVal(n.Aka),
		AllowIxpUpdate:           n.AllowIxpUpdate,
		Asn:                      int64(n.Asn),
		InfoIpv6:                 n.InfoIpv6,
		InfoMulticast:            n.InfoMulticast,
		InfoNeverViaRouteServers: n.InfoNeverViaRouteServers,
		InfoPrefixes4:            int64PtrVal(n.InfoPrefixes4),
		InfoPrefixes6:            int64PtrVal(n.InfoPrefixes6),
		InfoRatio:                stringVal(n.InfoRatio),
		InfoScope:                stringVal(n.InfoScope),
		InfoTraffic:              stringVal(n.InfoTraffic),
		InfoType:                 stringVal(n.InfoType),
		InfoTypes:                n.InfoTypes,
		InfoUnicast:              n.InfoUnicast,
		IrrAsSet:                 stringVal(n.IrrAsSet),
		Logo:                     stringPtrVal(n.Logo),
		LookingGlass:             stringVal(n.LookingGlass),
		Name:                     n.Name,
		NameLong:                 stringVal(n.NameLong),
		Notes:                    stringVal(n.Notes),
		PolicyContracts:          stringVal(n.PolicyContracts),
		PolicyGeneral:            stringVal(n.PolicyGeneral),
		PolicyLocations:          stringVal(n.PolicyLocations),
		PolicyRatio:              n.PolicyRatio,
		PolicyUrl:                stringVal(n.PolicyURL),
		RirStatus:                stringPtrVal(n.RirStatus),
		RirStatusUpdated:         timestampPtrVal(n.RirStatusUpdated),
		RouteServer:              stringVal(n.RouteServer),
		StatusDashboard:          stringPtrVal(n.StatusDashboard),
		Website:                  stringVal(n.Website),
		IxCount:                  int64Val(n.IxCount),
		FacCount:                 int64Val(n.FacCount),
		NetixlanUpdated:          timestampPtrVal(n.NetixlanUpdated),
		NetfacUpdated:            timestampPtrVal(n.NetfacUpdated),
		PocUpdated:               timestampPtrVal(n.PocUpdated),
		Created:                  timestampVal(n.Created),
		Updated:                  timestampVal(n.Updated),
		Status:                   n.Status,
	}
}
