package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// IxLanService implements the peeringdb.v1.IxLanService ConnectRPC handler
// interface.
type IxLanService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetIxLan returns a single IX LAN by ID.
func (s *IxLanService) GetIxLan(ctx context.Context, req *pb.GetIxLanRequest) (*pb.GetIxLanResponse, error) {
	il, err := s.Client.IxLan.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity ixlan %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get ixlan %d: %w", req.GetId(), err))
	}
	return &pb.GetIxLanResponse{IxLan: ixLanToProto(il)}, nil
}

func applyIxLanListFilters(req *pb.ListIxLansRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixlan.FieldID, int(*req.Id)))
	}
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixlan.FieldIxID, int(*req.IxId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(ixlan.FieldName, *req.Name))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldStatus, *req.Status))
	}
	if req.Descr != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldDescr, *req.Descr))
	}
	if req.Mtu != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldMtu, int(*req.Mtu)))
	}
	if req.Dot1QSupport != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldDot1qSupport, *req.Dot1QSupport))
	}
	if req.RsAsn != nil {
		if *req.RsAsn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: rs_asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixlan.FieldRsAsn, int(*req.RsAsn)))
	}
	if req.ArpSponge != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldArpSponge, *req.ArpSponge))
	}
	if req.IxfIxpMemberListUrlVisible != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldIxfIxpMemberListURLVisible, *req.IxfIxpMemberListUrlVisible))
	}
	if req.IxfIxpImportEnabled != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldIxfIxpImportEnabled, *req.IxfIxpImportEnabled))
	}
	return preds, nil
}

func applyIxLanStreamFilters(req *pb.StreamIxLansRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.IxId != nil {
		if *req.IxId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: ix_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixlan.FieldIxID, int(*req.IxId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(ixlan.FieldName, *req.Name))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldStatus, *req.Status))
	}
	if req.Descr != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldDescr, *req.Descr))
	}
	if req.Mtu != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldMtu, int(*req.Mtu)))
	}
	if req.Dot1QSupport != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldDot1qSupport, *req.Dot1QSupport))
	}
	if req.RsAsn != nil {
		if *req.RsAsn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: rs_asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(ixlan.FieldRsAsn, int(*req.RsAsn)))
	}
	if req.ArpSponge != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldArpSponge, *req.ArpSponge))
	}
	if req.IxfIxpMemberListUrlVisible != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldIxfIxpMemberListURLVisible, *req.IxfIxpMemberListUrlVisible))
	}
	if req.IxfIxpImportEnabled != nil {
		preds = append(preds, sql.FieldEQ(ixlan.FieldIxfIxpImportEnabled, *req.IxfIxpImportEnabled))
	}
	return preds, nil
}

// ListIxLans returns a paginated list of IX LANs.
func (s *IxLanService) ListIxLans(ctx context.Context, req *pb.ListIxLansRequest) (*pb.ListIxLansResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.IxLan, pb.IxLan]{
		EntityName: "ixlans",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxLanListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.IxLan, error) {
			q := s.Client.IxLan.Query().
				Order(ent.Asc(ixlan.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(ixlan.And(castPredicates[predicate.IxLan](preds)...))
			}
			return q.All(ctx)
		},
		Convert: ixLanToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListIxLansResponse{IxLans: items, NextPageToken: nextToken}, nil
}

// StreamIxLans streams all matching IX LANs.
func (s *IxLanService) StreamIxLans(ctx context.Context, req *pb.StreamIxLansRequest, stream *connect.ServerStream[pb.IxLan]) error {
	return StreamEntities(ctx, StreamParams[ent.IxLan, pb.IxLan]{
		EntityName:   "ixlans",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyIxLanStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.IxLan.Query()
			if len(preds) > 0 {
				q = q.Where(ixlan.And(castPredicates[predicate.IxLan](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.IxLan, error) {
			q := s.Client.IxLan.Query().
				Where(ixlan.IDGT(afterID)).
				Order(ent.Asc(ixlan.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(ixlan.And(castPredicates[predicate.IxLan](preds)...))
			}
			return q.All(ctx)
		},
		Convert: ixLanToProto,
		GetID:   func(il *ent.IxLan) int { return il.ID },
	}, stream)
}

// ixLanToProto converts an ent IxLan entity to a protobuf IxLan message.
func ixLanToProto(il *ent.IxLan) *pb.IxLan {
	return &pb.IxLan{
		Id:                         int64(il.ID),
		IxId:                       int64PtrVal(il.IxID),
		ArpSponge:                  stringPtrVal(il.ArpSponge),
		Descr:                      stringVal(il.Descr),
		Dot1QSupport:               il.Dot1qSupport,
		IxfIxpImportEnabled:        il.IxfIxpImportEnabled,
		IxfIxpMemberListUrlVisible: stringVal(il.IxfIxpMemberListURLVisible),
		Mtu:                        int64Val(il.Mtu),
		Name:                       stringVal(il.Name),
		RsAsn:                      int64PtrVal(il.RsAsn),
		Created:                    timestampVal(il.Created),
		Updated:                    timestampVal(il.Updated),
		Status:                     il.Status,
	}
}
