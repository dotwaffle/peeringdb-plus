package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/network"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// NetworkService implements the peeringdb.v1.NetworkService ConnectRPC handler
// interface. It queries the ent database layer and converts results to protobuf
// messages.
type NetworkService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// networkListFilters is the generic filter table consumed by
// applyNetworkListFilters. Entries run in slice order. Each entry extracts
// an optional field (nil pointer skipped), optionally validates the
// dereferenced value, and emits a sql.Selector predicate.
//
// See internal/grpcserver/filter.go for the filterFn[REQ] contract.
var networkListFilters = []filterFn[pb.ListNetworksRequest]{
	validatingFilter("id",
		func(r *pb.ListNetworksRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(network.FieldID)),
	validatingFilter("asn",
		func(r *pb.ListNetworksRequest) *int64 { return r.Asn },
		positiveInt64(), fieldEQInt(network.FieldAsn)),
	validatingFilter("org_id",
		func(r *pb.ListNetworksRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(network.FieldOrgID)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.Name },
		fieldContainsFold(network.FieldName)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.Aka },
		fieldContainsFold(network.FieldAka)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.NameLong },
		fieldContainsFold(network.FieldNameLong)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.Status },
		fieldEQString(network.FieldStatus)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.Website },
		fieldEQString(network.FieldWebsite)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.LookingGlass },
		fieldEQString(network.FieldLookingGlass)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.RouteServer },
		fieldEQString(network.FieldRouteServer)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.IrrAsSet },
		fieldEQString(network.FieldIrrAsSet)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.InfoType },
		fieldEQString(network.FieldInfoType)),
	eqFilter(func(r *pb.ListNetworksRequest) *int64 { return r.InfoPrefixes4 },
		fieldEQInt(network.FieldInfoPrefixes4)),
	eqFilter(func(r *pb.ListNetworksRequest) *int64 { return r.InfoPrefixes6 },
		fieldEQInt(network.FieldInfoPrefixes6)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.InfoTraffic },
		fieldEQString(network.FieldInfoTraffic)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.InfoRatio },
		fieldEQString(network.FieldInfoRatio)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.InfoScope },
		fieldEQString(network.FieldInfoScope)),
	eqFilter(func(r *pb.ListNetworksRequest) *bool { return r.InfoUnicast },
		fieldEQBool(network.FieldInfoUnicast)),
	eqFilter(func(r *pb.ListNetworksRequest) *bool { return r.InfoMulticast },
		fieldEQBool(network.FieldInfoMulticast)),
	eqFilter(func(r *pb.ListNetworksRequest) *bool { return r.InfoIpv6 },
		fieldEQBool(network.FieldInfoIpv6)),
	eqFilter(func(r *pb.ListNetworksRequest) *bool { return r.InfoNeverViaRouteServers },
		fieldEQBool(network.FieldInfoNeverViaRouteServers)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.Notes },
		fieldEQString(network.FieldNotes)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.PolicyUrl },
		fieldEQString(network.FieldPolicyURL)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.PolicyGeneral },
		fieldEQString(network.FieldPolicyGeneral)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.PolicyLocations },
		fieldEQString(network.FieldPolicyLocations)),
	eqFilter(func(r *pb.ListNetworksRequest) *bool { return r.PolicyRatio },
		fieldEQBool(network.FieldPolicyRatio)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.PolicyContracts },
		fieldEQString(network.FieldPolicyContracts)),
	eqFilter(func(r *pb.ListNetworksRequest) *bool { return r.AllowIxpUpdate },
		fieldEQBool(network.FieldAllowIxpUpdate)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.StatusDashboard },
		fieldEQString(network.FieldStatusDashboard)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.RirStatus },
		fieldEQString(network.FieldRirStatus)),
	eqFilter(func(r *pb.ListNetworksRequest) *string { return r.Logo },
		fieldEQString(network.FieldLogo)),
}

// networkStreamFilters mirrors networkListFilters but omits the id entry —
// Stream uses SinceID handled by generic.StreamEntities.
var networkStreamFilters = []filterFn[pb.StreamNetworksRequest]{
	validatingFilter("asn",
		func(r *pb.StreamNetworksRequest) *int64 { return r.Asn },
		positiveInt64(), fieldEQInt(network.FieldAsn)),
	validatingFilter("org_id",
		func(r *pb.StreamNetworksRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(network.FieldOrgID)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.Name },
		fieldContainsFold(network.FieldName)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.Aka },
		fieldContainsFold(network.FieldAka)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.NameLong },
		fieldContainsFold(network.FieldNameLong)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.Status },
		fieldEQString(network.FieldStatus)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.Website },
		fieldEQString(network.FieldWebsite)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.LookingGlass },
		fieldEQString(network.FieldLookingGlass)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.RouteServer },
		fieldEQString(network.FieldRouteServer)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.IrrAsSet },
		fieldEQString(network.FieldIrrAsSet)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.InfoType },
		fieldEQString(network.FieldInfoType)),
	eqFilter(func(r *pb.StreamNetworksRequest) *int64 { return r.InfoPrefixes4 },
		fieldEQInt(network.FieldInfoPrefixes4)),
	eqFilter(func(r *pb.StreamNetworksRequest) *int64 { return r.InfoPrefixes6 },
		fieldEQInt(network.FieldInfoPrefixes6)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.InfoTraffic },
		fieldEQString(network.FieldInfoTraffic)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.InfoRatio },
		fieldEQString(network.FieldInfoRatio)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.InfoScope },
		fieldEQString(network.FieldInfoScope)),
	eqFilter(func(r *pb.StreamNetworksRequest) *bool { return r.InfoUnicast },
		fieldEQBool(network.FieldInfoUnicast)),
	eqFilter(func(r *pb.StreamNetworksRequest) *bool { return r.InfoMulticast },
		fieldEQBool(network.FieldInfoMulticast)),
	eqFilter(func(r *pb.StreamNetworksRequest) *bool { return r.InfoIpv6 },
		fieldEQBool(network.FieldInfoIpv6)),
	eqFilter(func(r *pb.StreamNetworksRequest) *bool { return r.InfoNeverViaRouteServers },
		fieldEQBool(network.FieldInfoNeverViaRouteServers)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.Notes },
		fieldEQString(network.FieldNotes)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.PolicyUrl },
		fieldEQString(network.FieldPolicyURL)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.PolicyGeneral },
		fieldEQString(network.FieldPolicyGeneral)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.PolicyLocations },
		fieldEQString(network.FieldPolicyLocations)),
	eqFilter(func(r *pb.StreamNetworksRequest) *bool { return r.PolicyRatio },
		fieldEQBool(network.FieldPolicyRatio)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.PolicyContracts },
		fieldEQString(network.FieldPolicyContracts)),
	eqFilter(func(r *pb.StreamNetworksRequest) *bool { return r.AllowIxpUpdate },
		fieldEQBool(network.FieldAllowIxpUpdate)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.StatusDashboard },
		fieldEQString(network.FieldStatusDashboard)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.RirStatus },
		fieldEQString(network.FieldRirStatus)),
	eqFilter(func(r *pb.StreamNetworksRequest) *string { return r.Logo },
		fieldEQString(network.FieldLogo)),
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

// applyNetworkListFilters builds filter predicates from the generic filter
// table. See networkListFilters and internal/grpcserver/filter.go.
func applyNetworkListFilters(req *pb.ListNetworksRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, networkListFilters)
}

// applyNetworkStreamFilters builds filter predicates from the generic filter
// table. See networkStreamFilters and internal/grpcserver/filter.go.
func applyNetworkStreamFilters(req *pb.StreamNetworksRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, networkStreamFilters)
}

// ListNetworks returns a paginated list of networks ordered by the compound
// default order (-updated, -created, -id) per Phase 67 ORDER-02.
// Supports all pdbcompat-parity filter fields with AND logic.
func (s *NetworkService) ListNetworks(ctx context.Context, req *pb.ListNetworksRequest) (*pb.ListNetworksResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.Network, pb.Network]{
		EntityName: "networks",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyNetworkListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.Network, error) {
			q := s.Client.Network.Query().
				Order(ent.Desc(network.FieldUpdated), ent.Desc(network.FieldCreated), ent.Desc(network.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(network.And(castPredicates[predicate.Network](preds)...))
			}
			return q.All(ctx)
		},
		Convert: networkToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListNetworksResponse{Networks: items, NextPageToken: nextToken}, nil
}

// StreamNetworks streams all matching networks one message at a time using
// batched compound (updated, id) keyset pagination under the
// (-updated, -created, -id) default order. Supports all pdbcompat-parity
// filter fields.
func (s *NetworkService) StreamNetworks(ctx context.Context, req *pb.StreamNetworksRequest, stream *connect.ServerStream[pb.Network]) error {
	return StreamEntities(ctx, StreamParams[ent.Network, pb.Network]{
		EntityName:   "networks",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyNetworkStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.Network.Query()
			if len(preds) > 0 {
				q = q.Where(network.And(castPredicates[predicate.Network](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), cursor streamCursor, limit int) ([]*ent.Network, error) {
			q := s.Client.Network.Query().
				Order(ent.Desc(network.FieldUpdated), ent.Desc(network.FieldCreated), ent.Desc(network.FieldID)).
				Limit(limit)
			if !cursor.empty() {
				q = q.Where(predicate.Network(keysetCursorPredicate(cursor)))
			}
			if len(preds) > 0 {
				q = q.Where(network.And(castPredicates[predicate.Network](preds)...))
			}
			return q.All(ctx)
		},
		Convert:    networkToProto,
		GetID:      func(n *ent.Network) int { return n.ID },
		GetUpdated: func(n *ent.Network) time.Time { return n.Updated },
	}, stream)
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
