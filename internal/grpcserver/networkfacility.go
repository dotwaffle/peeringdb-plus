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

// networkFacilityListFilters is the generic filter table consumed by
// applyNetworkFacilityListFilters. Entries run in slice order.
var networkFacilityListFilters = []filterFn[pb.ListNetworkFacilitiesRequest]{
	validatingFilter("id",
		func(r *pb.ListNetworkFacilitiesRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(networkfacility.FieldID)),
	validatingFilter("net_id",
		func(r *pb.ListNetworkFacilitiesRequest) *int64 { return r.NetId },
		positiveInt64(), fieldEQInt(networkfacility.FieldNetID)),
	validatingFilter("fac_id",
		func(r *pb.ListNetworkFacilitiesRequest) *int64 { return r.FacId },
		positiveInt64(), fieldEQInt(networkfacility.FieldFacID)),
	eqFilter(func(r *pb.ListNetworkFacilitiesRequest) *string { return r.Country },
		fieldEQString(networkfacility.FieldCountry)),
	eqFilter(func(r *pb.ListNetworkFacilitiesRequest) *string { return r.City },
		fieldContainsFold(networkfacility.FieldCity)),
	eqFilter(func(r *pb.ListNetworkFacilitiesRequest) *string { return r.Status },
		fieldEQString(networkfacility.FieldStatus)),
	eqFilter(func(r *pb.ListNetworkFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(networkfacility.FieldName)),
	validatingFilter("local_asn",
		func(r *pb.ListNetworkFacilitiesRequest) *int64 { return r.LocalAsn },
		positiveInt64(), fieldEQInt(networkfacility.FieldLocalAsn)),
}

// networkFacilityStreamFilters mirrors networkFacilityListFilters but omits
// the id entry — Stream uses SinceID handled by generic.StreamEntities.
var networkFacilityStreamFilters = []filterFn[pb.StreamNetworkFacilitiesRequest]{
	validatingFilter("net_id",
		func(r *pb.StreamNetworkFacilitiesRequest) *int64 { return r.NetId },
		positiveInt64(), fieldEQInt(networkfacility.FieldNetID)),
	validatingFilter("fac_id",
		func(r *pb.StreamNetworkFacilitiesRequest) *int64 { return r.FacId },
		positiveInt64(), fieldEQInt(networkfacility.FieldFacID)),
	eqFilter(func(r *pb.StreamNetworkFacilitiesRequest) *string { return r.Country },
		fieldEQString(networkfacility.FieldCountry)),
	eqFilter(func(r *pb.StreamNetworkFacilitiesRequest) *string { return r.City },
		fieldContainsFold(networkfacility.FieldCity)),
	eqFilter(func(r *pb.StreamNetworkFacilitiesRequest) *string { return r.Status },
		fieldEQString(networkfacility.FieldStatus)),
	eqFilter(func(r *pb.StreamNetworkFacilitiesRequest) *string { return r.Name },
		fieldContainsFold(networkfacility.FieldName)),
	validatingFilter("local_asn",
		func(r *pb.StreamNetworkFacilitiesRequest) *int64 { return r.LocalAsn },
		positiveInt64(), fieldEQInt(networkfacility.FieldLocalAsn)),
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

// applyNetworkFacilityListFilters builds filter predicates from the generic filter table.
func applyNetworkFacilityListFilters(req *pb.ListNetworkFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, networkFacilityListFilters)
}

// applyNetworkFacilityStreamFilters builds filter predicates from the generic filter table.
func applyNetworkFacilityStreamFilters(req *pb.StreamNetworkFacilitiesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, networkFacilityStreamFilters)
}

// ListNetworkFacilities returns a paginated list of network facilities under
// the compound default order (-updated, -created, -id) per Phase 67 ORDER-02.
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
				Order(ent.Desc(networkfacility.FieldUpdated), ent.Desc(networkfacility.FieldCreated), ent.Desc(networkfacility.FieldID)).
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

// StreamNetworkFacilities streams all matching network facilities via compound
// (updated, id) keyset pagination under the (-updated, -created, -id) order.
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
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.NetworkFacility, error) {
			q := s.Client.NetworkFacility.Query().
				Order(ent.Desc(networkfacility.FieldUpdated), ent.Desc(networkfacility.FieldCreated), ent.Desc(networkfacility.FieldID)).
				Limit(limit)
			if !cursor.empty() {
				q = q.Where(predicate.NetworkFacility(keysetCursorPredicate(cursor)))
			}
			if len(preds) > 0 {
				q = q.Where(networkfacility.And(castPredicates[predicate.NetworkFacility](preds)...))
			}
			return q.All(ctx)
		},
		Convert:    networkFacilityToProto,
		GetID:      func(nf *ent.NetworkFacility) int { return nf.ID },
		GetUpdated: func(nf *ent.NetworkFacility) time.Time { return nf.Updated },
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
