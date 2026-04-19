package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/ixlan"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
	"github.com/dotwaffle/peeringdb-plus/internal/privfield"
)

// IxLanService implements the peeringdb.v1.IxLanService ConnectRPC handler
// interface.
type IxLanService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// ixLanListFilters is the generic filter table consumed by
// applyIxLanListFilters. Entries run in slice order. See
// internal/grpcserver/filter.go for the filterFn[REQ] contract and the
// reusable predicate builders.
var ixLanListFilters = []filterFn[pb.ListIxLansRequest]{
	validatingFilter("id",
		func(r *pb.ListIxLansRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(ixlan.FieldID)),
	validatingFilter("ix_id",
		func(r *pb.ListIxLansRequest) *int64 { return r.IxId },
		positiveInt64(), fieldEQInt(ixlan.FieldIxID)),
	eqFilter(func(r *pb.ListIxLansRequest) *string { return r.Name },
		fieldContainsFold(ixlan.FieldName)),
	eqFilter(func(r *pb.ListIxLansRequest) *string { return r.Status },
		fieldEQString(ixlan.FieldStatus)),
	eqFilter(func(r *pb.ListIxLansRequest) *string { return r.Descr },
		fieldEQString(ixlan.FieldDescr)),
	eqFilter(func(r *pb.ListIxLansRequest) *int64 { return r.Mtu },
		fieldEQInt(ixlan.FieldMtu)),
	eqFilter(func(r *pb.ListIxLansRequest) *bool { return r.Dot1QSupport },
		fieldEQBool(ixlan.FieldDot1qSupport)),
	validatingFilter("rs_asn",
		func(r *pb.ListIxLansRequest) *int64 { return r.RsAsn },
		positiveInt64(), fieldEQInt(ixlan.FieldRsAsn)),
	eqFilter(func(r *pb.ListIxLansRequest) *string { return r.ArpSponge },
		fieldEQString(ixlan.FieldArpSponge)),
	eqFilter(func(r *pb.ListIxLansRequest) *string { return r.IxfIxpMemberListUrlVisible },
		fieldEQString(ixlan.FieldIxfIxpMemberListURLVisible)),
	eqFilter(func(r *pb.ListIxLansRequest) *bool { return r.IxfIxpImportEnabled },
		fieldEQBool(ixlan.FieldIxfIxpImportEnabled)),
}

// ixLanStreamFilters mirrors ixLanListFilters but omits the id entry —
// Stream uses SinceID handled by generic.StreamEntities.
var ixLanStreamFilters = []filterFn[pb.StreamIxLansRequest]{
	validatingFilter("ix_id",
		func(r *pb.StreamIxLansRequest) *int64 { return r.IxId },
		positiveInt64(), fieldEQInt(ixlan.FieldIxID)),
	eqFilter(func(r *pb.StreamIxLansRequest) *string { return r.Name },
		fieldContainsFold(ixlan.FieldName)),
	eqFilter(func(r *pb.StreamIxLansRequest) *string { return r.Status },
		fieldEQString(ixlan.FieldStatus)),
	eqFilter(func(r *pb.StreamIxLansRequest) *string { return r.Descr },
		fieldEQString(ixlan.FieldDescr)),
	eqFilter(func(r *pb.StreamIxLansRequest) *int64 { return r.Mtu },
		fieldEQInt(ixlan.FieldMtu)),
	eqFilter(func(r *pb.StreamIxLansRequest) *bool { return r.Dot1QSupport },
		fieldEQBool(ixlan.FieldDot1qSupport)),
	validatingFilter("rs_asn",
		func(r *pb.StreamIxLansRequest) *int64 { return r.RsAsn },
		positiveInt64(), fieldEQInt(ixlan.FieldRsAsn)),
	eqFilter(func(r *pb.StreamIxLansRequest) *string { return r.ArpSponge },
		fieldEQString(ixlan.FieldArpSponge)),
	eqFilter(func(r *pb.StreamIxLansRequest) *string { return r.IxfIxpMemberListUrlVisible },
		fieldEQString(ixlan.FieldIxfIxpMemberListURLVisible)),
	eqFilter(func(r *pb.StreamIxLansRequest) *bool { return r.IxfIxpImportEnabled },
		fieldEQBool(ixlan.FieldIxfIxpImportEnabled)),
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
	return &pb.GetIxLanResponse{IxLan: ixLanToProto(ctx, il)}, nil
}

// applyIxLanListFilters builds filter predicates from the generic filter
// table. See ixLanListFilters and internal/grpcserver/filter.go.
func applyIxLanListFilters(req *pb.ListIxLansRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, ixLanListFilters)
}

// applyIxLanStreamFilters builds filter predicates from the generic filter
// table. See ixLanStreamFilters and internal/grpcserver/filter.go.
func applyIxLanStreamFilters(req *pb.StreamIxLansRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, ixLanStreamFilters)
}

// ListIxLans returns a paginated list of IX LANs under the compound default
// order (-updated, -created, -id) per Phase 67 ORDER-02.
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
				Order(ent.Desc(ixlan.FieldUpdated), ent.Desc(ixlan.FieldCreated), ent.Desc(ixlan.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(ixlan.And(castPredicates[predicate.IxLan](preds)...))
			}
			return q.All(ctx)
		},
		// Phase 64: closure adapter so ixLanToProto can receive ctx without
		// altering the generic pagination helper's Convert field type
		// (changing to func(ctx, *E) *P would cascade to all 13 entity
		// types). The enclosing handler's ctx is captured by reference and
		// carries the caller's tier via middleware.PrivacyTier.
		Convert: func(il *ent.IxLan) *pb.IxLan { return ixLanToProto(ctx, il) },
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListIxLansResponse{IxLans: items, NextPageToken: nextToken}, nil
}

// StreamIxLans streams all matching IX LANs via compound (updated, id) keyset
// pagination under the (-updated, -created, -id) default order.
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
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.IxLan, error) {
			q := s.Client.IxLan.Query().
				Order(ent.Desc(ixlan.FieldUpdated), ent.Desc(ixlan.FieldCreated), ent.Desc(ixlan.FieldID)).
				Limit(limit)
			if !cursor.empty() {
				q = q.Where(predicate.IxLan(keysetCursorPredicate(cursor)))
			}
			if len(preds) > 0 {
				q = q.Where(ixlan.And(castPredicates[predicate.IxLan](preds)...))
			}
			return q.All(ctx)
		},
		// Phase 64: closure adapter (see ListIxLans for rationale).
		Convert:    func(il *ent.IxLan) *pb.IxLan { return ixLanToProto(ctx, il) },
		GetID:      func(il *ent.IxLan) int { return il.ID },
		GetUpdated: func(il *ent.IxLan) time.Time { return il.Updated },
	}, stream)
}

// ixLanToProto converts an ent IxLan entity to a protobuf IxLan message,
// applying Phase 64 VIS-09 field-level redaction for the
// ixf_ixp_member_list_url field via internal/privfield.Redact.
//
// ctx MUST carry the caller's privacy tier (stamped by the PrivacyTier HTTP
// middleware). Unstamped ctx fail-closes to TierPublic per privfield.Redact
// semantics (Phase 64 D-03).
//
// Proto3 wrapper field semantics: a nil *wrapperspb.StringValue is omitted
// on the wire (matches upstream behaviour of omitting the JSON key for
// un-authenticated callers, D-04). The _visible companion is always
// emitted via stringVal (D-05 upstream parity).
func ixLanToProto(ctx context.Context, il *ent.IxLan) *pb.IxLan {
	url, omit := privfield.Redact(ctx, il.IxfIxpMemberListURLVisible, il.IxfIxpMemberListURL)
	var urlProto *wrapperspb.StringValue
	if !omit && url != "" {
		urlProto = wrapperspb.String(url)
	}
	return &pb.IxLan{
		Id:                         int64(il.ID),
		IxId:                       int64PtrVal(il.IxID),
		ArpSponge:                  stringPtrVal(il.ArpSponge),
		Descr:                      stringVal(il.Descr),
		Dot1QSupport:               il.Dot1qSupport,
		IxfIxpImportEnabled:        il.IxfIxpImportEnabled,
		IxfIxpMemberListUrlVisible: stringVal(il.IxfIxpMemberListURLVisible),
		IxfIxpMemberListUrl:        urlProto,
		Mtu:                        int64Val(il.Mtu),
		Name:                       stringVal(il.Name),
		RsAsn:                      int64PtrVal(il.RsAsn),
		Created:                    timestampVal(il.Created),
		Updated:                    timestampVal(il.Updated),
		Status:                     il.Status,
	}
}
