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

// internetExchangeListFilters is the generic filter table consumed by
// applyInternetExchangeListFilters. Entries run in slice order. See
// internal/grpcserver/filter.go for the filterFn[REQ] contract and the
// reusable predicate builders.
var internetExchangeListFilters = []filterFn[pb.ListInternetExchangesRequest]{
	validatingFilter("id",
		func(r *pb.ListInternetExchangesRequest) *int64 { return r.Id },
		positiveInt64(), fieldEQInt(internetexchange.FieldID)),
	validatingFilter("org_id",
		func(r *pb.ListInternetExchangesRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(internetexchange.FieldOrgID)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Name },
		fieldContainsFold(internetexchange.FieldName)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Aka },
		fieldContainsFold(internetexchange.FieldAka)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.NameLong },
		fieldContainsFold(internetexchange.FieldNameLong)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Country },
		fieldEQString(internetexchange.FieldCountry)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.City },
		fieldContainsFold(internetexchange.FieldCity)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Status },
		fieldEQString(internetexchange.FieldStatus)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.RegionContinent },
		fieldEQString(internetexchange.FieldRegionContinent)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Media },
		fieldEQString(internetexchange.FieldMedia)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Notes },
		fieldEQString(internetexchange.FieldNotes)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *bool { return r.ProtoUnicast },
		fieldEQBool(internetexchange.FieldProtoUnicast)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *bool { return r.ProtoMulticast },
		fieldEQBool(internetexchange.FieldProtoMulticast)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *bool { return r.ProtoIpv6 },
		fieldEQBool(internetexchange.FieldProtoIpv6)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Website },
		fieldEQString(internetexchange.FieldWebsite)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.UrlStats },
		fieldEQString(internetexchange.FieldURLStats)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.TechEmail },
		fieldEQString(internetexchange.FieldTechEmail)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.TechPhone },
		fieldEQString(internetexchange.FieldTechPhone)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.PolicyEmail },
		fieldEQString(internetexchange.FieldPolicyEmail)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.PolicyPhone },
		fieldEQString(internetexchange.FieldPolicyPhone)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.SalesEmail },
		fieldEQString(internetexchange.FieldSalesEmail)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.SalesPhone },
		fieldEQString(internetexchange.FieldSalesPhone)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.ServiceLevel },
		fieldEQString(internetexchange.FieldServiceLevel)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Terms },
		fieldEQString(internetexchange.FieldTerms)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.StatusDashboard },
		fieldEQString(internetexchange.FieldStatusDashboard)),
	eqFilter(func(r *pb.ListInternetExchangesRequest) *string { return r.Logo },
		fieldEQString(internetexchange.FieldLogo)),
}

// internetExchangeStreamFilters mirrors internetExchangeListFilters but omits
// the id entry — Stream uses SinceID handled by generic.StreamEntities.
var internetExchangeStreamFilters = []filterFn[pb.StreamInternetExchangesRequest]{
	validatingFilter("org_id",
		func(r *pb.StreamInternetExchangesRequest) *int64 { return r.OrgId },
		positiveInt64(), fieldEQInt(internetexchange.FieldOrgID)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Name },
		fieldContainsFold(internetexchange.FieldName)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Aka },
		fieldContainsFold(internetexchange.FieldAka)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.NameLong },
		fieldContainsFold(internetexchange.FieldNameLong)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Country },
		fieldEQString(internetexchange.FieldCountry)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.City },
		fieldContainsFold(internetexchange.FieldCity)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Status },
		fieldEQString(internetexchange.FieldStatus)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.RegionContinent },
		fieldEQString(internetexchange.FieldRegionContinent)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Media },
		fieldEQString(internetexchange.FieldMedia)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Notes },
		fieldEQString(internetexchange.FieldNotes)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *bool { return r.ProtoUnicast },
		fieldEQBool(internetexchange.FieldProtoUnicast)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *bool { return r.ProtoMulticast },
		fieldEQBool(internetexchange.FieldProtoMulticast)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *bool { return r.ProtoIpv6 },
		fieldEQBool(internetexchange.FieldProtoIpv6)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Website },
		fieldEQString(internetexchange.FieldWebsite)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.UrlStats },
		fieldEQString(internetexchange.FieldURLStats)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.TechEmail },
		fieldEQString(internetexchange.FieldTechEmail)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.TechPhone },
		fieldEQString(internetexchange.FieldTechPhone)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.PolicyEmail },
		fieldEQString(internetexchange.FieldPolicyEmail)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.PolicyPhone },
		fieldEQString(internetexchange.FieldPolicyPhone)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.SalesEmail },
		fieldEQString(internetexchange.FieldSalesEmail)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.SalesPhone },
		fieldEQString(internetexchange.FieldSalesPhone)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.ServiceLevel },
		fieldEQString(internetexchange.FieldServiceLevel)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Terms },
		fieldEQString(internetexchange.FieldTerms)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.StatusDashboard },
		fieldEQString(internetexchange.FieldStatusDashboard)),
	eqFilter(func(r *pb.StreamInternetExchangesRequest) *string { return r.Logo },
		fieldEQString(internetexchange.FieldLogo)),
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

// applyInternetExchangeListFilters builds filter predicates from the generic
// filter table. See internetExchangeListFilters and internal/grpcserver/filter.go.
func applyInternetExchangeListFilters(req *pb.ListInternetExchangesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, internetExchangeListFilters)
}

// applyInternetExchangeStreamFilters builds filter predicates from the generic
// filter table. See internetExchangeStreamFilters and internal/grpcserver/filter.go.
func applyInternetExchangeStreamFilters(req *pb.StreamInternetExchangesRequest) ([]func(*sql.Selector), error) {
	return applyFilters(req, internetExchangeStreamFilters)
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
