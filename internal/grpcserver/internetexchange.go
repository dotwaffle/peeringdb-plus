package grpcserver

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"

	"github.com/dotwaffle/peeringdb-plus/ent"
	"github.com/dotwaffle/peeringdb-plus/ent/internetexchange"
	"github.com/dotwaffle/peeringdb-plus/ent/predicate"
	pb "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1"
)

// InternetExchangeService implements the peeringdb.v1.InternetExchangeService
// ConnectRPC handler interface. It queries the ent database layer and converts
// results to protobuf messages.
type InternetExchangeService struct {
	Client        *ent.Client
	StreamTimeout time.Duration
}

// GetInternetExchange returns a single internet exchange by ID. Returns
// NOT_FOUND if the internet exchange does not exist.
func (s *InternetExchangeService) GetInternetExchange(ctx context.Context, req *pb.GetInternetExchangeRequest) (*pb.GetInternetExchangeResponse, error) {
	ix, err := s.Client.InternetExchange.Get(ctx, int(req.GetId()))
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("entity internetexchange %d not found", req.GetId()))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("get internetexchange %d: %w", req.GetId(), err))
	}
	return &pb.GetInternetExchangeResponse{InternetExchange: internetExchangeToProto(ix)}, nil
}

func applyInternetExchangeListFilters(req *pb.ListInternetExchangesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.Id != nil {
		if *req.Id <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(internetexchange.FieldID, int(*req.Id)))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(internetexchange.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldStatus, *req.Status))
	}
	if req.RegionContinent != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldRegionContinent, *req.RegionContinent))
	}
	if req.Media != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldMedia, *req.Media))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldNotes, *req.Notes))
	}
	if req.ProtoUnicast != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldProtoUnicast, *req.ProtoUnicast))
	}
	if req.ProtoMulticast != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldProtoMulticast, *req.ProtoMulticast))
	}
	if req.ProtoIpv6 != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldProtoIpv6, *req.ProtoIpv6))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldWebsite, *req.Website))
	}
	if req.UrlStats != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldURLStats, *req.UrlStats))
	}
	if req.TechEmail != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldTechEmail, *req.TechEmail))
	}
	if req.TechPhone != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldTechPhone, *req.TechPhone))
	}
	if req.PolicyEmail != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldPolicyEmail, *req.PolicyEmail))
	}
	if req.PolicyPhone != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldPolicyPhone, *req.PolicyPhone))
	}
	if req.SalesEmail != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldSalesEmail, *req.SalesEmail))
	}
	if req.SalesPhone != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldSalesPhone, *req.SalesPhone))
	}
	if req.ServiceLevel != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldServiceLevel, *req.ServiceLevel))
	}
	if req.Terms != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldTerms, *req.Terms))
	}
	if req.StatusDashboard != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldStatusDashboard, *req.StatusDashboard))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldLogo, *req.Logo))
	}
	return preds, nil
}

func applyInternetExchangeStreamFilters(req *pb.StreamInternetExchangesRequest) ([]func(*sql.Selector), error) {
	var preds []func(*sql.Selector)
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid filter: org_id must be positive"))
		}
		preds = append(preds, sql.FieldEQ(internetexchange.FieldOrgID, int(*req.OrgId)))
	}
	if req.Name != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldName, *req.Name))
	}
	if req.Aka != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldAka, *req.Aka))
	}
	if req.NameLong != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldNameLong, *req.NameLong))
	}
	if req.Country != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldCountry, *req.Country))
	}
	if req.City != nil {
		preds = append(preds, sql.FieldContainsFold(internetexchange.FieldCity, *req.City))
	}
	if req.Status != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldStatus, *req.Status))
	}
	if req.RegionContinent != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldRegionContinent, *req.RegionContinent))
	}
	if req.Media != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldMedia, *req.Media))
	}
	if req.Notes != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldNotes, *req.Notes))
	}
	if req.ProtoUnicast != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldProtoUnicast, *req.ProtoUnicast))
	}
	if req.ProtoMulticast != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldProtoMulticast, *req.ProtoMulticast))
	}
	if req.ProtoIpv6 != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldProtoIpv6, *req.ProtoIpv6))
	}
	if req.Website != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldWebsite, *req.Website))
	}
	if req.UrlStats != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldURLStats, *req.UrlStats))
	}
	if req.TechEmail != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldTechEmail, *req.TechEmail))
	}
	if req.TechPhone != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldTechPhone, *req.TechPhone))
	}
	if req.PolicyEmail != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldPolicyEmail, *req.PolicyEmail))
	}
	if req.PolicyPhone != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldPolicyPhone, *req.PolicyPhone))
	}
	if req.SalesEmail != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldSalesEmail, *req.SalesEmail))
	}
	if req.SalesPhone != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldSalesPhone, *req.SalesPhone))
	}
	if req.ServiceLevel != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldServiceLevel, *req.ServiceLevel))
	}
	if req.Terms != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldTerms, *req.Terms))
	}
	if req.StatusDashboard != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldStatusDashboard, *req.StatusDashboard))
	}
	if req.Logo != nil {
		preds = append(preds, sql.FieldEQ(internetexchange.FieldLogo, *req.Logo))
	}
	return preds, nil
}

// ListInternetExchanges returns a paginated list of internet exchanges.
func (s *InternetExchangeService) ListInternetExchanges(ctx context.Context, req *pb.ListInternetExchangesRequest) (*pb.ListInternetExchangesResponse, error) {
	items, nextToken, err := ListEntities(ctx, ListParams[ent.InternetExchange, pb.InternetExchange]{
		EntityName: "internetexchanges",
		PageSize:   req.GetPageSize(),
		PageToken:  req.GetPageToken(),
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyInternetExchangeListFilters(req)
		},
		Query: func(ctx context.Context, preds []func(*sql.Selector), limit, offset int) ([]*ent.InternetExchange, error) {
			q := s.Client.InternetExchange.Query().
				Order(ent.Asc(internetexchange.FieldID)).
				Limit(limit).Offset(offset)
			if len(preds) > 0 {
				q = q.Where(internetexchange.And(castPredicates[predicate.InternetExchange](preds)...))
			}
			return q.All(ctx)
		},
		Convert: internetExchangeToProto,
	})
	if err != nil {
		return nil, err
	}
	return &pb.ListInternetExchangesResponse{InternetExchanges: items, NextPageToken: nextToken}, nil
}

// StreamInternetExchanges streams all matching internet exchanges.
func (s *InternetExchangeService) StreamInternetExchanges(ctx context.Context, req *pb.StreamInternetExchangesRequest, stream *connect.ServerStream[pb.InternetExchange]) error {
	return StreamEntities(ctx, StreamParams[ent.InternetExchange, pb.InternetExchange]{
		EntityName:   "internet exchanges",
		Timeout:      s.StreamTimeout,
		SinceID:      req.SinceId,
		UpdatedSince: req.UpdatedSince,
		ApplyFilters: func() ([]func(*sql.Selector), error) {
			return applyInternetExchangeStreamFilters(req)
		},
		Count: func(ctx context.Context, preds []func(*sql.Selector)) (int, error) {
			q := s.Client.InternetExchange.Query()
			if len(preds) > 0 {
				q = q.Where(internetexchange.And(castPredicates[predicate.InternetExchange](preds)...))
			}
			return q.Count(ctx)
		},
		QueryBatch: func(ctx context.Context, preds []func(*sql.Selector), afterID, limit int) ([]*ent.InternetExchange, error) {
			q := s.Client.InternetExchange.Query().
				Where(internetexchange.IDGT(afterID)).
				Order(ent.Asc(internetexchange.FieldID)).
				Limit(limit)
			if len(preds) > 0 {
				q = q.Where(internetexchange.And(castPredicates[predicate.InternetExchange](preds)...))
			}
			return q.All(ctx)
		},
		Convert: internetExchangeToProto,
		GetID:   func(ix *ent.InternetExchange) int { return ix.ID },
	}, stream)
}

// internetExchangeToProto converts an ent InternetExchange entity to a
// protobuf InternetExchange message.
func internetExchangeToProto(ix *ent.InternetExchange) *pb.InternetExchange {
	return &pb.InternetExchange{
		Id:                     int64(ix.ID),
		OrgId:                  int64PtrVal(ix.OrgID),
		Aka:                    stringVal(ix.Aka),
		City:                   stringVal(ix.City),
		Country:                stringVal(ix.Country),
		IxfLastImport:          timestampPtrVal(ix.IxfLastImport),
		IxfNetCount:            int64Val(ix.IxfNetCount),
		Logo:                   stringPtrVal(ix.Logo),
		Media:                  stringVal(ix.Media),
		Name:                   ix.Name,
		NameLong:               stringVal(ix.NameLong),
		Notes:                  stringVal(ix.Notes),
		PolicyEmail:            stringVal(ix.PolicyEmail),
		PolicyPhone:            stringVal(ix.PolicyPhone),
		ProtoIpv6:              ix.ProtoIpv6,
		ProtoMulticast:         ix.ProtoMulticast,
		ProtoUnicast:           ix.ProtoUnicast,
		RegionContinent:        stringVal(ix.RegionContinent),
		SalesEmail:             stringVal(ix.SalesEmail),
		SalesPhone:             stringVal(ix.SalesPhone),
		ServiceLevel:           stringVal(ix.ServiceLevel),
		StatusDashboard:        stringPtrVal(ix.StatusDashboard),
		TechEmail:              stringVal(ix.TechEmail),
		TechPhone:              stringVal(ix.TechPhone),
		Terms:                  stringVal(ix.Terms),
		UrlStats:               stringVal(ix.URLStats),
		Website:                stringVal(ix.Website),
		NetCount:               int64Val(ix.NetCount),
		FacCount:               int64Val(ix.FacCount),
		IxfImportRequest:       stringPtrVal(ix.IxfImportRequest),
		IxfImportRequestStatus: stringVal(ix.IxfImportRequestStatus),
		Created:                timestampVal(ix.Created),
		Updated:                timestampVal(ix.Updated),
		Status:                 ix.Status,
	}
}
