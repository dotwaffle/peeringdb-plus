package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/carrier"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// CarrierService implements the peeringdb.v1.CarrierService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type CarrierService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetCarrier returns a single carrier by ID. Returns NOT_FOUND if the carrier
// does not exist.
func (s *CarrierService) GetCarrier(ctx context.Context, req *pb.GetCarrierRequest) (*pb.GetCarrierResponse, error) {
	c, err := s.Client.Carrier.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity carrier %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get carrier %d: %w", req.GetId(), err))
	}
	return &pb.GetCarrierResponse{Carrier: carrierToProto(c)}, nil
}

func applyCarrierListFilters(req *pb.ListCarriersRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrier.FieldID, int(*req.Id)))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrier.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(carrier.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(carrier.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(carrier.FieldNameLong, *req.NameLong))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldWebsite, *req.Website))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldNotes, *req.Notes))
	}
	// org_name filter -- stored as denormalized field on entity.
	if req.OrgName != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldOrgName, *req.OrgName))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldLogo, *req.Logo))
	}
	return preds, nil
}

func applyCarrierStreamFilters(req *pb.StreamCarriersRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(carrier.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(carrier.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(carrier.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(carrier.FieldNameLong, *req.NameLong))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldWebsite, *req.Website))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldNotes, *req.Notes))
	}
	if req.OrgName != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldOrgName, *req.OrgName))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(carrier.FieldLogo, *req.Logo))
	}
	return preds, nil
}

// ListCarriers returns a paginated list of carriers ordered by ID ascending.
func (s *CarrierService) ListCarriers(ctx context.Context, req *pb.ListCarriersRequest) (*pb.ListCarriersResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.Carrier, pb.Carrier]{
		EntityName: "carriers",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Carrier, error) {
			q := s.Client.Carrier.Query().
				Order(ent.Asc(carrier.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(carrier.And(castPredicates[predicate.Carrier](preds)...))
			}
			return q.All(ctx)
		},
		Convert: carrierToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListCarriersResponse{Carriers: items, NextPageToken: nextToken}, nil
}

// StreamCarriers streams all matching carriers one message at a time.
func (s *CarrierService) StreamCarriers(ctx context.Context, req *pb.StreamCarriersRequest, stream *connect.ServerStream[pb.Carrier]) error {
	return StreamEntities(ctx, StreamParams[ent.Carrier, pb.Carrier]{
		EntityName:   "carriers",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyCarrierStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.Carrier.Query()
			if len(preds) > 0 {
				q = q.Where(carrier.And(castPredicates[predicate.Carrier](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.Carrier, error) {
			q := s.Client.Carrier.Query().
				Where(carrier.IDGT(afterID)).
				Order(ent.Asc(carrier.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(carrier.And(castPredicates[predicate.Carrier](preds)...))
			}
			return q.All(ctx)
		},
		Convert: carrierToProto,
		GetID:   func(c *ent.Carrier) int { return c.ID },
	}, stream)
}

// carrierToProto converts an ent Carrier entity to a protobuf Carrier message.
func carrierToProto(c *ent.Carrier) *pb.Carrier {
	return &pb.Carrier{
		Id:       int64(c.ID),
		OrgId:    int64PtrVal(c.OrgID),
		Aka:      stringVal(c.Aka),
		Logo:     stringPtrVal(c.Logo),
		Name:     c.Name,
		NameLong: stringVal(c.NameLong),
		Notes:    stringVal(c.Notes),
		Website:  stringVal(c.Website),
		OrgName:  stringVal(c.OrgName),
		FacCount: int64Val(c.FacCount),
		Created:  timestampVal(c.Created),
		Updated:  timestampVal(c.Updated),
		Status:   c.Status,
	}
}
