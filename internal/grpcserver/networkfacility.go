package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/networkfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// NetworkFacilityService implements the peeringdb.v1.NetworkFacilityService
// ConnectRPC handler interface.
type NetworkFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetNetworkFacility returns a single network facility by ID.
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

func applyNetworkFacilityListFilters(req *pb.ListNetworkFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkfacility.FieldID, int(*req.Id)))
	}
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkfacility.FieldNetID, int(*req.NetId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkfacility.FieldFacID, int(*req.FacId)))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(networkfacility.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(networkfacility.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(networkfacility.FieldStatus, *req.Status))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(networkfacility.FieldName, *req.Name))
	}
	if req.LocalAsn != nil {
		if *req.LocalAsn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: local_asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkfacility.FieldLocalAsn, int(*req.LocalAsn)))
	}
	return preds, nil
}

func applyNetworkFacilityStreamFilters(req *pb.StreamNetworkFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.NetId != nil {
		if *req.NetId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: net_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkfacility.FieldNetID, int(*req.NetId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkfacility.FieldFacID, int(*req.FacId)))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(networkfacility.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(networkfacility.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(networkfacility.FieldStatus, *req.Status))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(networkfacility.FieldName, *req.Name))
	}
	if req.LocalAsn != nil {
		if *req.LocalAsn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: local_asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(networkfacility.FieldLocalAsn, int(*req.LocalAsn)))
	}
	return preds, nil
}

// ListNetworkFacilities returns a paginated list of network facilities.
func (s *NetworkFacilityService) ListNetworkFacilities(ctx context.Context, req *pb.ListNetworkFacilitiesRequest) (*pb.ListNetworkFacilitiesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.NetworkFacility, pb.NetworkFacility]{
		EntityName: "networkfacilities",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyNetworkFacilityListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.NetworkFacility, error) {
			q := s.Client.NetworkFacility.Query().
				Order(ent.Asc(networkfacility.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(networkfacility.And(castPredicates[predicate.NetworkFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: networkFacilityToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListNetworkFacilitiesResponse{NetworkFacilities: items, NextPageToken: nextToken}, nil
}

// StreamNetworkFacilities streams all matching network facilities.
func (s *NetworkFacilityService) StreamNetworkFacilities(ctx context.Context, req *pb.StreamNetworkFacilitiesRequest, stream *connect.ServerStream[pb.NetworkFacility]) error {
	return StreamEntities(ctx, StreamParams[ent.NetworkFacility, pb.NetworkFacility]{
		EntityName:   "network facilities",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyNetworkFacilityStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.NetworkFacility.Query()
			if len(preds) > 0 {
				q = q.Where(networkfacility.And(castPredicates[predicate.NetworkFacility](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.NetworkFacility, error) {
			q := s.Client.NetworkFacility.Query().
				Where(networkfacility.IDGT(afterID)).
				Order(ent.Asc(networkfacility.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(networkfacility.And(castPredicates[predicate.NetworkFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: networkFacilityToProto,
		GetID:   func(nf *ent.NetworkFacility) int { return nf.ID },
	}, stream)
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
