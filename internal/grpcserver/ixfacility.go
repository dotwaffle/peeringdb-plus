package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixfacility"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// IxFacilityService implements the peeringdb.v1.IxFacilityService ConnectRPC
// handler interface.
type IxFacilityService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetIxFacility returns a single IX facility by ID.
func (s *IxFacilityService) GetIxFacility(ctx context.Context, req *pb.GetIxFacilityRequest) (*pb.GetIxFacilityResponse, error) {
	ixf, err := s.Client.IxFacility.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity ixfacility %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get ixfacility %d: %w", req.GetId(), err))
	}
	return &pb.GetIxFacilityResponse{IxFacility: ixFacilityToProto(ixf)}, nil
}

func applyIxFacilityListFilters(req *pb.ListIxFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixfacility.FieldID, int(*req.Id)))
	}
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixfacility.FieldIxID, int(*req.IxId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixfacility.FieldFacID, int(*req.FacId)))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(ixfacility.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(ixfacility.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(ixfacility.FieldStatus, *req.Status))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(ixfacility.FieldName, *req.Name))
	}
	return preds, nil
}

func applyIxFacilityStreamFilters(req *pb.StreamIxFacilitiesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixfacility.FieldIxID, int(*req.IxId)))
	}
	if req.FacId != nil {
		if *req.FacId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: fac_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixfacility.FieldFacID, int(*req.FacId)))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(ixfacility.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(ixfacility.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(ixfacility.FieldStatus, *req.Status))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(ixfacility.FieldName, *req.Name))
	}
	return preds, nil
}

// ListIxFacilities returns a paginated list of IX facilities.
func (s *IxFacilityService) ListIxFacilities(ctx context.Context, req *pb.ListIxFacilitiesRequest) (*pb.ListIxFacilitiesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.IxFacility, pb.IxFacility]{
		EntityName: "ixfacilities",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxFacilityListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.IxFacility, error) {
			q := s.Client.IxFacility.Query().
				Order(ent.Asc(ixfacility.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(ixfacility.And(castPredicates[predicate.IxFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: ixFacilityToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListIxFacilitiesResponse{IxFacilities: items, NextPageToken: nextToken}, nil
}

// StreamIxFacilities streams all matching IX facilities.
func (s *IxFacilityService) StreamIxFacilities(ctx context.Context, req *pb.StreamIxFacilitiesRequest, stream *connect.ServerStream[pb.IxFacility]) error {
	return StreamEntities(ctx, StreamParams[ent.IxFacility, pb.IxFacility]{
		EntityName:   "ixfacilities",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxFacilityStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.IxFacility.Query()
			if len(preds) > 0 {
				q = q.Where(ixfacility.And(castPredicates[predicate.IxFacility](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.IxFacility, error) {
			q := s.Client.IxFacility.Query().
				Where(ixfacility.IDGT(afterID)).
				Order(ent.Asc(ixfacility.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(ixfacility.And(castPredicates[predicate.IxFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert: ixFacilityToProto,
		GetID:   func(ixf *ent.IxFacility) int { return ixf.ID },
	}, stream)
}

// ixFacilityToProto converts an ent IxFacility entity to a protobuf IxFacility
// message.
func ixFacilityToProto(ixf *ent.IxFacility) *pb.IxFacility {
	return &pb.IxFacility{
		Id:      int64(ixf.ID),
		FacId:   int64PtrVal(ixf.FacID),
		IxId:    int64PtrVal(ixf.IxID),
		Name:    stringVal(ixf.Name),
		City:    stringVal(ixf.City),
		Country: stringVal(ixf.Country),
		Created: timestampVal(ixf.Created),
		Updated: timestampVal(ixf.Updated),
		Status:  ixf.Status,
	}
}
