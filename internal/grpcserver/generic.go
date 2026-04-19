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
	// QueryBatch fetches the next batch under compound keyset pagination. When
	// cursor.empty() the query runs without a cursor predicate; otherwise the
	// handler emits the compound keyset predicate
	//   WHERE (updated < cursor.Updated) OR (updated = cursor.Updated AND id < cursor.ID)
	// under the `(-updated, -created, -id)` default order (Phase 67, CONTEXT.md
	// D-01 / D-05).
	QueryBatch func(ctx context.Context, predicates []func(*sql.Selector), cursor streamCursor, limit int) ([]*E, error)
	Convert    func(*E) *P
	GetID      func(*E) int
	// GetUpdated extracts the `updated` timestamp from an entity for the
	// compound cursor's primary key. Required by StreamEntities to emit the
	// next-batch cursor after each batch drains.
	GetUpdated func(*E) time.Time
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

	// Stream records in batches under compound (updated, id) keyset
	// pagination. The cursor starts empty (full table scan) and advances to
	// the last emitted row at the end of each batch. D-05: SinceID /
	// UpdatedSince are predicates already applied via the predicates slice
	// above; they do NOT seed the keyset cursor.
	var cursor streamCursor
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		batch, err := params.QueryBatch(ctx, predicates, cursor, streamBatchSize)
		if err != nil {
			return connect.NewError(connect.CodeInternal,
				fmt.Errorf("stream %s batch after cursor %+v: %w", params.EntityName, cursor, err))
		}
		if len(batch) == 0 {
			return nil
		}

		for _, e := range batch {
			if err := stream.Send(params.Convert(e)); err != nil {
				return err
			}
		}

		last := batch[len(batch)-1]
		cursor = streamCursor{
			Updated: params.GetUpdated(last),
			ID:      params.GetID(last),
		}
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

// keysetCursorPredicate returns the compound keyset predicate used by every
// Stream* RPC's QueryBatch closure under the `(-updated, -created, -id)`
// default ordering (Phase 67 ORDER-02). Caller must check `cursor.empty()`
// first — an empty cursor means "no resume predicate".
//
//	WHERE (updated < cursor.Updated) OR (updated = cursor.Updated AND id < cursor.ID)
//
// The predicate matches the DESC ordering: each row strictly-less-than the
// cursor is emitted. The id tiebreaker guarantees monotonic progress even
// when multiple rows share a timestamp (CONTEXT.md D-01). Built with ent's
// canonical variadic predicate composers (sql.AndPredicates / sql.OrPredicates
// — see ent/network/where.go:2452,2457).
func keysetCursorPredicate(cursor streamCursor) func(*sql.Selector) {
	return sql.OrPredicates(
		sql.FieldLT("updated", cursor.Updated),
		sql.AndPredicates(
			sql.FieldEQ("updated", cursor.Updated),
			sql.FieldLT("id", cursor.ID),
		),
	)
}
