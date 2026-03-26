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

// applyNetworkListFilters builds filter predicates from ListNetworksRequest
// optional fields. Covers all pdbcompat Registry fields for networks.
func applyNetworkListFilters(req *pb.ListNetworksRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(network.FieldID, int(*req.Id)))
	}
	if req.Asn != nil {
		if *req.Asn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(network.FieldAsn, int(*req.Asn)))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(network.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(network.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(network.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(network.FieldNameLong, *req.NameLong))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(network.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(network.FieldWebsite, *req.Website))
	}
	if req.LookingGlass != nil {
		preds = append(preds, sql.FieldEQ(network.FieldLookingGlass, *req.LookingGlass))
	}
	if req.RouteServer != nil {
		preds = append(preds, sql.FieldEQ(network.FieldRouteServer, *req.RouteServer))
	}
	if req.IrrAsSet != nil {
		preds = append(preds, sql.FieldEQ(network.FieldIrrAsSet, *req.IrrAsSet))
	}
	if req.InfoType != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoType, *req.InfoType))
	}
	if req.InfoPrefixes4 != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoPrefixes4, int(*req.InfoPrefixes4)))
	}
	if req.InfoPrefixes6 != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoPrefixes6, int(*req.InfoPrefixes6)))
	}
	if req.InfoTraffic != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoTraffic, *req.InfoTraffic))
	}
	if req.InfoRatio != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoRatio, *req.InfoRatio))
	}
	if req.InfoScope != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoScope, *req.InfoScope))
	}
	if req.InfoUnicast != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoUnicast, *req.InfoUnicast))
	}
	if req.InfoMulticast != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoMulticast, *req.InfoMulticast))
	}
	if req.InfoIpv6 != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoIpv6, *req.InfoIpv6))
	}
	if req.InfoNeverViaRouteServers != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoNeverViaRouteServers, *req.InfoNeverViaRouteServers))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(network.FieldNotes, *req.Notes))
	}
	if req.PolicyUrl != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyURL, *req.PolicyUrl))
	}
	if req.PolicyGeneral != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyGeneral, *req.PolicyGeneral))
	}
	if req.PolicyLocations != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyLocations, *req.PolicyLocations))
	}
	if req.PolicyRatio != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyRatio, *req.PolicyRatio))
	}
	if req.PolicyContracts != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyContracts, *req.PolicyContracts))
	}
	if req.AllowIxpUpdate != nil {
		preds = append(preds, sql.FieldEQ(network.FieldAllowIxpUpdate, *req.AllowIxpUpdate))
	}
	if req.StatusDashboard != nil {
		preds = append(preds, sql.FieldEQ(network.FieldStatusDashboard, *req.StatusDashboard))
	}
	if req.RirStatus != nil {
		preds = append(preds, sql.FieldEQ(network.FieldRirStatus, *req.RirStatus))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(network.FieldLogo, *req.Logo))
	}
	return preds, nil
}

// applyNetworkStreamFilters builds filter predicates from StreamNetworksRequest
// optional fields. Same filters as List minus id. SinceID and UpdatedSince are
// handled by the generic StreamEntities helper.
func applyNetworkStreamFilters(req *pb.StreamNetworksRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Asn != nil {
		if *req.Asn <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: asn must be positive"))
		}
		preds = append(preds, sql.FieldEQ(network.FieldAsn, int(*req.Asn)))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(network.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(network.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(network.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(network.FieldNameLong, *req.NameLong))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(network.FieldStatus, *req.Status))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(network.FieldWebsite, *req.Website))
	}
	if req.LookingGlass != nil {
		preds = append(preds, sql.FieldEQ(network.FieldLookingGlass, *req.LookingGlass))
	}
	if req.RouteServer != nil {
		preds = append(preds, sql.FieldEQ(network.FieldRouteServer, *req.RouteServer))
	}
	if req.IrrAsSet != nil {
		preds = append(preds, sql.FieldEQ(network.FieldIrrAsSet, *req.IrrAsSet))
	}
	if req.InfoType != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoType, *req.InfoType))
	}
	if req.InfoPrefixes4 != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoPrefixes4, int(*req.InfoPrefixes4)))
	}
	if req.InfoPrefixes6 != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoPrefixes6, int(*req.InfoPrefixes6)))
	}
	if req.InfoTraffic != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoTraffic, *req.InfoTraffic))
	}
	if req.InfoRatio != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoRatio, *req.InfoRatio))
	}
	if req.InfoScope != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoScope, *req.InfoScope))
	}
	if req.InfoUnicast != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoUnicast, *req.InfoUnicast))
	}
	if req.InfoMulticast != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoMulticast, *req.InfoMulticast))
	}
	if req.InfoIpv6 != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoIpv6, *req.InfoIpv6))
	}
	if req.InfoNeverViaRouteServers != nil {
		preds = append(preds, sql.FieldEQ(network.FieldInfoNeverViaRouteServers, *req.InfoNeverViaRouteServers))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(network.FieldNotes, *req.Notes))
	}
	if req.PolicyUrl != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyURL, *req.PolicyUrl))
	}
	if req.PolicyGeneral != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyGeneral, *req.PolicyGeneral))
	}
	if req.PolicyLocations != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyLocations, *req.PolicyLocations))
	}
	if req.PolicyRatio != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyRatio, *req.PolicyRatio))
	}
	if req.PolicyContracts != nil {
		preds = append(preds, sql.FieldEQ(network.FieldPolicyContracts, *req.PolicyContracts))
	}
	if req.AllowIxpUpdate != nil {
		preds = append(preds, sql.FieldEQ(network.FieldAllowIxpUpdate, *req.AllowIxpUpdate))
	}
	if req.StatusDashboard != nil {
		preds = append(preds, sql.FieldEQ(network.FieldStatusDashboard, *req.StatusDashboard))
	}
	if req.RirStatus != nil {
		preds = append(preds, sql.FieldEQ(network.FieldRirStatus, *req.RirStatus))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(network.FieldLogo, *req.Logo))
	}
	return preds, nil
}

// ListNetworks returns a paginated list of networks ordered by ID ascending.
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
				Order(ent.Asc(network.FieldID)).
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
// batched keyset pagination. Supports all pdbcompat-parity filter fields.
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
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.Network, error) {
			q := s.Client.Network.Query().
				Where(network.IDGT(afterID)).
				Order(ent.Asc(network.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(network.And(castPredicates[predicate.Network](preds)...))
			}
			return q.All(ctx)
		},
		Convert: networkToProto,
		GetID:   func(n *ent.Network) int { return n.ID },
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
