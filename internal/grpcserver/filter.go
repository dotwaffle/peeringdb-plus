package grpcserver

// This file defines the type-safe, table-driven filter runner used by
// per-entity ConnectRPC List/Stream handlers. The entry points are the
// filterFn[REQ any] closure type and the applyFilters runner — the per-entity
// files in this package define []filterFn[REQ] tables and invoke applyFilters
// from their apply*ListFilters / apply*StreamFilters wrappers. No any-boxing
// on the hot path: the REQ and V type parameters flow through the extractor /
// validator / predicate trio at compile time, so a mismatched column/type
// pairing fails to build rather than panicking at runtime.

import (
	"fmt"
	"time"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"
)

// filterFn is a closure that extracts an optional field from a typed proto
// request, validates it, and returns a sql.Selector predicate if the field
// is set. It preserves compile-time type safety — there is no any-boxing.
type filterFn[REQ any] func(*REQ) (func(*sql.Selector), error)

// eqFilter constructs a filterFn for an equality predicate on a pointer-typed
// optional field. Type parameter V ensures the predicate's value type matches
// the extractor's return type at compile time.
func eqFilter[REQ, V any](
	get func(*REQ) *V,
	pred func(V) func(*sql.Selector),
) filterFn[REQ] {
	return func(req *REQ) (func(*sql.Selector), error) {
		v := get(req)
		if v == nil {
			return nil, nil
		}
		return pred(*v), nil
	}
}

// validatingFilter is like eqFilter but runs a validator over the dereferenced
// value before constructing the predicate. Validation failures are wrapped as
// connect.CodeInvalidArgument with the filter name included in the error text.
//
// The error format is "invalid filter: <name> <validator_err>" — e.g.
// "invalid filter: asn must be positive". This matches the pre-Phase-56
// per-entity error format verbatim so that existing grpcserver_test.go
// assertions (containsStr on substrings like "asn must be positive") stay
// green without test modification. See 56-02-PLAN.md Task 1 Step 0 (option A).
func validatingFilter[REQ, V any](
	name string,
	get func(*REQ) *V,
	validate func(V) error,
	pred func(V) func(*sql.Selector),
) filterFn[REQ] {
	return func(req *REQ) (func(*sql.Selector), error) {
		v := get(req)
		if v == nil {
			return nil, nil
		}
		if err := validate(*v); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid filter: %s %w", name, err))
		}
		return pred(*v), nil
	}
}

// applyFilters runs all filter closures over the request and returns the
// accumulated predicates. Validation errors are already wrapped as
// connect.CodeInvalidArgument by validatingFilter and propagate unchanged.
func applyFilters[REQ any](req *REQ, fns []filterFn[REQ]) ([]func(*sql.Selector), error) {
	preds := make([]func(*sql.Selector), 0, len(fns))
	for _, fn := range fns {
		p, err := fn(req)
		if err != nil {
			return nil, err
		}
		if p != nil {
			preds = append(preds, p)
		}
	}
	return preds, nil
}

// fieldEQInt builds an int-equality predicate factory for the given ent field.
// The returned closure accepts a proto-idiomatic int64 and truncates to int
// on the sql layer, matching pre-Phase-56 per-entity behavior (see
// network.go applyNetworkListFilters before consolidation).
func fieldEQInt(field string) func(int64) func(*sql.Selector) {
	return func(v int64) func(*sql.Selector) {
		return sql.FieldEQ(field, int(v))
	}
}

// fieldEQString builds a string-equality predicate factory for the given ent
// field.
func fieldEQString(field string) func(string) func(*sql.Selector) {
	return func(v string) func(*sql.Selector) {
		return sql.FieldEQ(field, v)
	}
}

// fieldContainsFold builds a case-insensitive substring predicate factory for
// the given ent field.
func fieldContainsFold(field string) func(string) func(*sql.Selector) {
	return func(v string) func(*sql.Selector) {
		return sql.FieldContainsFold(field, v)
	}
}

// fieldEQBool builds a bool-equality predicate factory for the given ent field.
func fieldEQBool(field string) func(bool) func(*sql.Selector) {
	return func(v bool) func(*sql.Selector) {
		return sql.FieldEQ(field, v)
	}
}

// fieldEQFloat64 builds a float64-equality predicate factory for the given ent
// field.
func fieldEQFloat64(field string) func(float64) func(*sql.Selector) {
	return func(v float64) func(*sql.Selector) {
		return sql.FieldEQ(field, v)
	}
}

// fieldInTimeRange builds a lower-bound-exclusive time predicate factory
// matching the StreamEntities UpdatedSince semantic at generic.go:103. "In
// range" here is the shape used by the only current consumer — a strict
// greater-than, not a BETWEEN clause.
func fieldInTimeRange(field string) func(time.Time) func(*sql.Selector) {
	return func(v time.Time) func(*sql.Selector) {
		return sql.FieldGT(field, v)
	}
}

// positiveInt64 returns a validator that rejects non-positive int64 values
// with the canonical "must be positive" error. Preserves the pre-Phase-56
// semantic that 0 is NOT positive (see network.go line 44 in the
// pre-consolidation code).
//
// The error text deliberately omits a "value" prefix so that the
// validatingFilter wrapper produces "invalid filter: <name> must be positive"
// — matching the pre-consolidation per-entity error strings that existing
// grpcserver_test.go assertions depend on.
func positiveInt64() func(int64) error {
	return func(v int64) error {
		if v <= 0 {
			return fmt.Errorf("must be positive")
		}
		return nil
	}
}

// nonEmptyString returns a validator that rejects zero-length strings.
// See positiveInt64 for error-format rationale.
func nonEmptyString() func(string) error {
	return func(v string) error {
		if len(v) == 0 {
			return fmt.Errorf("must not be empty")
		}
		return nil
	}
}
