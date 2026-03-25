// Package grpcserver provides ConnectRPC service handlers for the PeeringDB
// gRPC API. Each handler implements the generated service interface and queries
// the ent database layer.
package grpcserver

import (
	"time"

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
