package grpcserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ListParams holds type-specific callbacks for a paginated list query.
// E is the ent entity type, P is the proto message type.
type ListParams[E any, P any] struct {
	EntityName   string
	PageSize     int32
	PageToken    string
	ApplyFilters func() ([]func(*sql.Selector), error)
	Query        func(ctx context.Context, predicates []func(*sql.Selector), limit, offset int) ([]*E, error)
	Convert      func(*E) *P
}

// ListEntities executes a paginated list query using type-specific callbacks.
// Returns converted proto items, next page token, and any error.
func ListEntities[E any, P any](ctx context.Context, params ListParams[E, P]) ([]*P, string, error) {
	pageSize := normalizePageSize(params.PageSize)
	offset, err := decodePageToken(params.PageToken)
	if err != nil {
		return nil, "", connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("invalid page_token: %w", err))
	}

	predicates, err := params.ApplyFilters()
	if err != nil {
		return nil, "", err // ApplyFilters returns connect errors directly.
	}

	// Fetch one extra to detect whether there is a next page.
	results, err := params.Query(ctx, predicates, pageSize+1, offset)
	if err != nil {
		return nil, "", connect.NewError(connect.CodeInternal,
			fmt.Errorf("list %s: %w", params.EntityName, err))
	}

	var nextPageToken string
	if len(results) > pageSize {
		results = results[:pageSize]
		nextPageToken = encodePageToken(offset + pageSize)
	}

	items := make([]*P, len(results))
	for i, e := range results {
		items[i] = params.Convert(e)
	}
	return items, nextPageToken, nil
}

// StreamParams holds type-specific callbacks for a streaming query.
// E is the ent entity type, P is the proto message type.
type StreamParams[E any, P any] struct {
	EntityName   string
	Timeout      time.Duration
	SinceID      *int64
	UpdatedSince *timestamppb.Timestamp
	ApplyFilters func() ([]func(*sql.Selector), error)
	Count        func(ctx context.Context, predicates []func(*sql.Selector)) (int, error)
	// TODO(phase-67 plan 05): replace afterID with streamCursor for compound keyset pagination.
	QueryBatch func(ctx context.Context, predicates []func(*sql.Selector), afterID, limit int) ([]*E, error)
	Convert      func(*E) *P
	GetID        func(*E) int
}

// StreamEntities streams all matching entities using batched keyset pagination.
// Handles timeout, count header, SinceID/UpdatedSince, and batch iteration.
//
// Header contract (PERF-06): on full streams (SinceID == nil AND UpdatedSince ==
// nil) a SELECT COUNT(*) preflight runs and the grpc-total-count response header
// is set to the total matching row count. On delta streams (SinceID or
// UpdatedSince set) the COUNT preflight is skipped entirely and
// the grpc-total-count response header is absent — not "present with 0", not
// "present with -1". Delta clients pull incremental ranges and have no use for a
// full-table total, so we do not pay for the count.
func StreamEntities[E any, P any](ctx context.Context, params StreamParams[E, P], stream *connect.ServerStream[P]) error {
	// Apply stream timeout.
	if params.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, params.Timeout)
		defer cancel()
	}

	predicates, err := params.ApplyFilters()
	if err != nil {
		return err // ApplyFilters returns connect errors directly.
	}

	// Handle SinceID: add predicate and set initial cursor.
	// Per RESEARCH.md Pitfall 4: handled here, not in per-type filter functions.
	if params.SinceID != nil {
		predicates = append(predicates, sql.FieldGT("id", int(*params.SinceID)))
	}
	if params.UpdatedSince != nil {
		predicates = append(predicates, sql.FieldGT("updated", params.UpdatedSince.AsTime()))
	}

	// Count total matching records for header metadata. Skipped on delta streams
	// (SinceID or UpdatedSince set) because delta clients pull incremental ranges
	// and have no use for a full-table total — see PERF-06.
	if params.SinceID == nil && params.UpdatedSince == nil {
		total, err := params.Count(ctx, predicates)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("count %s: %w", params.EntityName, err))
		}
		stream.ResponseHeader().Set("grpc-total-count", strconv.Itoa(total))
	}

	// Stream records in batches using keyset pagination.
	lastID := 0
	if params.SinceID != nil {
		lastID = int(*params.SinceID)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		batch, err := params.QueryBatch(ctx, predicates, lastID, streamBatchSize)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("stream %s batch after id %d: %w", params.EntityName, lastID, err))
		}
		if len(batch) == 0 {
			return nil
		}

		for _, e := range batch {
			if err := stream.Send(params.Convert(e)); err != nil {
				return err
			}
		}

		lastID = params.GetID(batch[len(batch)-1])
		if len(batch) < streamBatchSize {
			return nil
		}
	}
}

// castPredicates converts generic sql.Selector functions to typed ent predicates.
func castPredicates[T ~func(*sql.Selector)](preds []func(*sql.Selector)) []T {
	out := make([]T, len(preds))
	for i, f := range preds {
		out[i] = T(f)
	}
	return out
}
