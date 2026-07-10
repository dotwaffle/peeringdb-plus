package graph

// Hand-written helpers shared by the 13 offset/limit list resolvers in
// custom.resolvers.go and the 13 relay connection resolvers in
// schema.resolvers.go. gqlgen copies resolver BODIES through on every
// regeneration but never touches sibling files (same arrangement as
// pagination.go), so the shared logic lives here and each resolver body
// is a thin delegation.

import (
	"context"
	"fmt"

	"entgo.io/ent/dialect/sql"
)

// gqlListQuery is the generated ent builder shape the offset/limit list
// resolvers drive. Q is the concrete builder (self-referential so chained
// calls keep the concrete type), P its predicate type, and E the row type
// returned by All.
type gqlListQuery[Q any, P ~func(*sql.Selector), E any] interface {
	Offset(int) Q
	Limit(int) Q
	Where(...P) Q
	CollectFields(context.Context, ...string) (Q, error)
	All(context.Context) ([]E, error)
}

// listResolveInput carries one list resolver's arguments into listResolve.
type listResolveInput[Q any, P ~func(*sql.Selector)] struct {
	offset *int
	limit  *int
	entity string // noun for the "apply <entity> filter" error prefix
	query  Q
	whereP func() (P, error) // nil when the where argument was nil
}

// listResolve implements the shared offset/limit list resolver flow:
// validate pagination, apply the optional where-input predicate, then
// CollectFields before All.
//
// CollectFields eager-loads the GraphQL-selected edges in O(edges)
// batched queries; without it every nested edge selection falls into
// ent's per-row lazy-load path (N+1 queries and N otelsql spans).
// The relay connection resolvers get this from entgql's Paginate.
func listResolve[Q gqlListQuery[Q, P, E], P ~func(*sql.Selector), E any](
	ctx context.Context, in listResolveInput[Q, P],
) ([]E, error) {
	ol, err := ValidateOffsetLimit(in.offset, in.limit)
	if err != nil {
		return nil, err
	}
	query := in.query.Offset(ol.Offset).Limit(ol.Limit)
	if in.whereP != nil {
		p, err := in.whereP()
		if err != nil {
			return nil, fmt.Errorf("apply %s filter: %w", in.entity, err)
		}
		query = query.Where(p)
	}
	query, err = query.CollectFields(ctx)
	if err != nil {
		return nil, fmt.Errorf("collect fields: %w", err)
	}
	return query.All(ctx)
}

// wherePredicate adapts a possibly-nil gqlgen where-input into the
// predicate thunk listResolveInput.whereP expects. The nil check must
// happen on the concrete pointer type — inside listResolve the input
// would be a non-nil interface wrapping a nil pointer.
func wherePredicate[P any, W any, PW interface {
	*W
	P() (P, error)
}](where PW) func() (P, error) {
	if where == nil {
		return nil
	}
	return where.P
}

// connResolve applies the shared relay-connection guardrails — the
// MaxLimit page-size check and the defaultFirst bound on otherwise
// unbounded queries — then delegates to the entity's entgql paginator.
// The paginate closure receives the (possibly defaulted) first value;
// everything else it needs is captured from the resolver's scope.
func connResolve[C any](first, last *int, paginate func(first *int) (*C, error)) (*C, error) {
	if err := validatePageSize(first, last); err != nil {
		return nil, err
	}
	return paginate(defaultFirst(first, last))
}
