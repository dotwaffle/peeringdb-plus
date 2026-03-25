package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// NetworkFacilityService implements the peeringdb.v1.NetworkFacilityService
// ConnectRPC handler interface. It queries the ent database layer and converts
// results to protobuf messages.
type NetworkFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetNetworkFacility returns a single network facility by ID. Returns
// NOT_FOUND if the network facility does not exist.
func (s *NetworkFacilityService) GetNetworkFacility(ctx context.Context, req *pb.GetNetworkFacilityRequest) (*pb.GetNetworkFacilityResponse, error) {
	nf, err := s.Client.NetworkFacility.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity networkfacility %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get networkfacility %d: %w", req.GetId(), err))
	}
	return &pb.GetNetworkFacilityResponse{NetworkFacility: networkFacilityToProto(nf)}, nil
}

// ListNetworkFacilities returns a paginated list of network facilities ordered
// by ID ascending. Supports page_size, page_token, and optional filter fields
// (net_id, fac_id, country, city, status). Multiple filters combine with AND
// logic.
func (s *NetworkFacilityService) ListNetworkFacilities(ctx context.Context, req *pb.ListNetworkFacilitiesRequest) (*pb.ListNetworkFacilitiesResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.NetworkFacility
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: net_id must be positive"))
		}
		predicates = append(predicates, networkfacility.NetIDEQ(int(*req.NetId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		predicates = append(predicates, networkfacility.FacIDEQ(int(*req.FacId)))
	}
	if req.Country != nil {
		predicates = append(predicates, networkfacility.CountryEQ(*req.Country))
	}
	if req.City != nil {
		predicates = append(predicates, networkfacility.CityContainsFold(*req.City))
	}
	if req.Status != nil {
		predicates = append(predicates, networkfacility.StatusEQ(*req.Status))
	}

	query := s.Client.NetworkFacility.Query().
		Order(ent.Asc(networkfacility.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(networkfacility.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list networkfacilities: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	items := make([]*pb.NetworkFacility, len(results))
	for i, nf := range results {
		items[i] = networkFacilityToProto(nf)
	}

	return &pb.ListNetworkFacilitiesResponse{
		NetworkFacilities: items,
		NextPageToken:     nextPageToken,
	}, nil
}

// StreamNetworkFacilities streams all matching network facilities one message at
// a time using batched keyset pagination. Returns Unimplemented until handler
// logic is added.
func (s *NetworkFacilityService) StreamNetworkFacilities(_ context.Context, _ *pb.StreamNetworkFacilitiesRequest, _ *connect.ServerStream[pb.NetworkFacility]) error {
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("StreamNetworkFacilities not yet implemented"))
}

// networkFacilityToProto converts an ent NetworkFacility entity to a protobuf
// NetworkFacility message.
func networkFacilityToProto(nf *ent.NetworkFacility) *pb.NetworkFacility {
	return &pb.NetworkFacility{
		Id:       int64(nf.ID),
		FacId:    int64PtrVal(nf.FacID),
		NetId:    int64PtrVal(nf.NetID),
		LocalAsn: int64(nf.LocalAsn),
		Name:     stringVal(nf.Name),
		City:     stringVal(nf.City),
		Country:  stringVal(nf.Country),
		Created:  timestampVal(nf.Created),
		Updated:  timestampVal(nf.Updated),
		Status:   nf.Status,
	}
}
