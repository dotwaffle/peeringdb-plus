package grpcserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"connectrpc.com/connect"

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

// ListInternetExchanges returns a paginated list of internet exchanges ordered
// by ID ascending. Supports page_size, page_token, and optional filter fields
// (name, country, city, status, org_id). Multiple filters combine with AND
// logic.
func (s *InternetExchangeService) ListInternetExchanges(ctx context.Context, req *pb.ListInternetExchangesRequest) (*pb.ListInternetExchangesResponse, error) {
	pageSize := normalizePageSize(req.GetPageSize())
	offset, err := decodePageToken(req.GetPageToken())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	// Build filter predicates from optional fields.
	var predicates []predicate.InternetExchange
	if req.Name != nil {
		predicates = append(predicates, internetexchange.NameContainsFold(*req.Name))
	}
	if req.Country != nil {
		predicates = append(predicates, internetexchange.CountryEQ(*req.Country))
	}
	if req.City != nil {
		predicates = append(predicates, internetexchange.CityContainsFold(*req.City))
	}
	if req.Status != nil {
		predicates = append(predicates, internetexchange.StatusEQ(*req.Status))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: org_id must be positive"))
		}
		predicates = append(predicates, internetexchange.OrgIDEQ(int(*req.OrgId)))
	}

	query := s.Client.InternetExchange.Query().
		Order(ent.Asc(internetexchange.FieldID)).
		Limit(pageSize + 1).
		Offset(offset)
	if len(predicates) > 0 {
		query = query.Where(internetexchange.And(predicates...))
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := query.All(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list internetexchanges: %w", err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	exchanges := make([]*pb.InternetExchange, len(results))
	for i, ix := range results {
		exchanges[i] = internetExchangeToProto(ix)
	}

	return &pb.ListInternetExchangesResponse{
		InternetExchanges: exchanges,
		NextPageToken:     nextPageToken,
	}, nil
}

// StreamInternetExchanges streams all matching internet exchanges one message
// at a time using batched keyset pagination. Filters match the
// ListInternetExchanges behavior.
func (s *InternetExchangeService) StreamInternetExchanges(ctx context.Context, req *pb.StreamInternetExchangesRequest, stream *connect.ServerStream[pb.InternetExchange]) error {
	// Apply stream timeout.
	if s.StreamTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.StreamTimeout)
		defer cancel()
	}

	// Build filter predicates (identical to ListInternetExchanges).
	var predicates []predicate.InternetExchange
	if req.Name != nil {
		predicates = append(predicates, internetexchange.NameContainsFold(*req.Name))
	}
	if req.Country != nil {
		predicates = append(predicates, internetexchange.CountryEQ(*req.Country))
	}
	if req.City != nil {
		predicates = append(predicates, internetexchange.CityContainsFold(*req.City))
	}
	if req.Status != nil {
		predicates = append(predicates, internetexchange.StatusEQ(*req.Status))
	}
	if req.OrgId != nil {
		if *req.OrgId <= 0 {
			return connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: org_id must be positive"))
		}
		predicates = append(predicates, internetexchange.OrgIDEQ(int(*req.OrgId)))
	}

	// Resume and incremental filter support.
	if req.SinceId != nil {
		predicates = append(predicates, internetexchange.IDGT(int(*req.SinceId)))
	}
	if req.UpdatedSince != nil {
		predicates = append(predicates, internetexchange.UpdatedGT(req.UpdatedSince.AsTime()))
	}

	// Count total matching records for header metadata.
	countQuery := s.Client.InternetExchange.Query()
	if len(predicates) > 0 {
		countQuery = countQuery.Where(internetexchange.And(predicates...))
	}
	total, err := countQuery.Count(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("count internet exchanges: %w", err))
	}
	stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(total))

	// Stream records in batches using keyset pagination.
	lastID := 0
	if req.SinceId != nil {
		lastID = int(*req.SinceId)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		query := s.Client.InternetExchange.Query().
			Where(internetexchange.IDGT(lastID)).
			Order(ent.Asc(internetexchange.FieldID)).
			Limit(streamBatchSize)
		if len(predicates) > 0 {
			query = query.Where(internetexchange.And(predicates...))
		}

		batch, err := query.All(ctx)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("stream internet exchanges batch after id %d: %w", lastID, err))
		}
		if len(batch) == 0 {
			return nil
		}

		for _, ix := range batch {
			if err := stream.Send(internetExchangeToProto(ix)); err != nil {
				return err
			}
		}

		lastID = batch[len(batch)-1].ID
		if len(batch) < streamBatchSize {
			return nil
		}
	}
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
