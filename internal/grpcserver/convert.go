// Package grpcserver provides ConnectRPC service handlers for the PeeringDB
// gRPC API. Each handler implements the generated service interface and queries
// the ent database layer.
package grpcserver

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// stringVal wraps a non-empty string as a StringValue. Returns nil for empty
// strings, which maps to proto field absence.
func stringVal(s string) *wrapperspb.StringValue {
	if s == "" {
		return nil
	}
	return wrapperspb.String(s)
}

// stringPtrVal wraps a *string as a StringValue. Returns nil when the pointer
// is nil, preserving Go nillability in proto representation.
func stringPtrVal(s *string) *wrapperspb.StringValue {
	if s == nil {
		return nil
	}
	return wrapperspb.String(*s)
}

// int64Val wraps an int as an Int64Value. Always returns a non-nil value since
// the source int is never absent.
func int64Val(n int) *wrapperspb.Int64Value {
	return wrapperspb.Int64(int64(n))
}

// int64PtrVal wraps a *int as an Int64Value. Returns nil when the pointer is
// nil, preserving nillability.
func int64PtrVal(n *int) *wrapperspb.Int64Value {
	if n == nil {
		return nil
	}
	return wrapperspb.Int64(int64(*n))
}

// boolPtrVal wraps a *bool as a BoolValue. Returns nil when the pointer is
// nil.
func boolPtrVal(b *bool) *wrapperspb.BoolValue {
	if b == nil {
		return nil
	}
	return wrapperspb.Bool(*b)
}

// float64PtrVal wraps a *float64 as a DoubleValue. Returns nil when the
// pointer is nil.
func float64PtrVal(f *float64) *wrapperspb.DoubleValue {
	if f == nil {
		return nil
	}
	return wrapperspb.Double(*f)
}

// timestampVal converts a time.Time to a protobuf Timestamp.
func timestampVal(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}

// timestampPtrVal converts a *time.Time to a protobuf Timestamp. Returns nil
// when the pointer is nil.
func timestampPtrVal(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

// metaStruct converts a stored meta document to a protobuf Struct. A nil
// or empty document becomes an empty Struct, so the field is present on
// every message, as upstream serializes an unset meta as {}. A document
// that structpb cannot represent is logged and omitted (nil) so that one
// bad row does not fail the whole RPC.
func metaStruct(ctx context.Context, entity string, id int, doc map[string]any) *structpb.Struct {
	if len(doc) == 0 {
		return &structpb.Struct{}
	}
	s, err := structpb.NewStruct(doc)
	if err != nil {
		slog.WarnContext(ctx, "grpcserver: omitting unconvertible meta document",
			slog.String("entity", entity),
			slog.Int("id", id),
			slog.Any("error", err),
		)
		return nil
	}
	return s
}
