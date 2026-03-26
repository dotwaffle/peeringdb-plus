package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/carrierfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// CarrierFacilityService implements the peeringdb.v1.CarrierFacilityService
// ConnectRPC handler interface.
type CarrierFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetCarrierFacility returns a single carrier facility by ID.
func (s *CarrierFacilityService) GetCarrierFacility(ctx context.Context, req *pb.GetCarrierFacilityRequest) (*pb.GetCarrierFacilityResponse, error) {
	cf, err := s.Client.CarrierFacility.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity carrierfacility %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get carrierfacility %d: %w", req.GetId(), err))
	}
	return &pb.GetCarrierFacilityResponse{CarrierFacility: carrierFacilityToProto(cf)}, nil
}

func applyCarrierFacilityListFilters(req *pb.ListCarrierFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrierfacility.FieldID, int(*req.Id)))
	}
	if req.CarrierId != nil {
		if *req.CarrierId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: carrier_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrierfacility.FieldCarrierID, int(*req.CarrierId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrierfacility.FieldFacID, int(*req.FacId)))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(carrierfacility.FieldStatus, *req.Status))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(carrierfacility.FieldName, *req.Name))
	}
	return preds, nil
}

func applyCarrierFacilityStreamFilters(req *pb.StreamCarrierFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.CarrierId != nil {
		if *req.CarrierId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: carrier_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrierfacility.FieldCarrierID, int(*req.CarrierId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrierfacility.FieldFacID, int(*req.FacId)))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(carrierfacility.FieldStatus, *req.Status))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(carrierfacility.FieldName, *req.Name))
	}
	return preds, nil
}

// ListCarrierFacilities returns a paginated list of carrier facilities.
func (s *CarrierFacilityService) ListCarrierFacilities(ctx context.Context, req *pb.ListCarrierFacilitiesRequest) (*pb.ListCarrierFacilitiesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.CarrierFacility, pb.CarrierFacility]{
		EntityName: "carrierfacilities",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierFacilityListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.CarrierFacility, error) {
			q := s.Client.CarrierFacility.Query().
				Order(ent.Asc(carrierfacility.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(carrierfacility.And(castPredicates[predicate.CarrierFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: carrierFacilityToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListCarrierFacilitiesResponse{CarrierFacilities: items, NextPageToken: nextToken}, nil
}

// StreamCarrierFacilities streams all matching carrier facilities.
func (s *CarrierFacilityService) StreamCarrierFacilities(ctx context.Context, req *pb.StreamCarrierFacilitiesRequest, stream *connect.ServerStream[pb.CarrierFacility]) error {
	return StreamEntities(ctx, StreamParams[ent.CarrierFacility, pb.CarrierFacility]{
		EntityName:   "carrier facilities",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierFacilityStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.CarrierFacility.Query()
			if len(preds) > 0 {
				q = q.Where(carrierfacility.And(castPredicates[predicate.CarrierFacility](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.CarrierFacility, error) {
			q := s.Client.CarrierFacility.Query().
				Where(carrierfacility.IDGT(afterID)).
				Order(ent.Asc(carrierfacility.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(carrierfacility.And(castPredicates[predicate.CarrierFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: carrierFacilityToProto,
		GetID:   func(cf *ent.CarrierFacility) int { return cf.ID },
	}, stream)
}

// carrierFacilityToProto converts an ent CarrierFacility entity to a protobuf
// CarrierFacility message.
func carrierFacilityToProto(cf *ent.CarrierFacility) *pb.CarrierFacility {
	return &pb.CarrierFacility{
		Id:        int64(cf.ID),
		CarrierId: int64PtrVal(cf.CarrierID),
		FacId:     int64PtrVal(cf.FacID),
		Name:      stringVal(cf.Name),
		Created:   timestampVal(cf.Created),
		Updated:   timestampVal(cf.Updated),
		Status:    cf.Status,
	}
}
