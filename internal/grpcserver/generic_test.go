package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"entgo.io/ent/dialect/sql"
)

func TestListEntities(t *testing.T) {
	t.Parallel()

	type mockEntity struct {
		ID   int
		Name string
	}
	type mockProto struct {
		ID   int64
		Name string
	}

	convert := func(e *mockEntity) *mockProto {
		return &mockProto{ID: int64(e.ID), Name: e.Name}
	}

	// makeMockData generates n mock entities starting from ID 1.
	makeMockData := func(n int) []*mockEntity {
		out := make([]*mockEntity, n)
		for i := range n {
			out[i] = &mockEntity{ID: i + 1, Name: fmt.Sprintf("entity-%d", i+1)}
		}
		return out
	}

	tests := []struct {
		name          string
		params        ListParams[mockEntity, mockProto]
		wantLen       int
		wantNextToken bool
		wantErr       connect.Code
	}{
		{
			name: "empty result",
			params: ListParams[mockEntity, mockProto]{
				EntityName:   "test",
				PageSize:     5,
				ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
				Query: func(_ context.Context, _ []func(*sql.Selector), _, _ int) ([]*mockEntity, error) {
					return nil, nil
				},
				Convert: convert,
			},
			wantLen:       0,
			wantNextToken: false,
		},
		{
			name: "single page",
			params: ListParams[mockEntity, mockProto]{
				EntityName:   "test",
				PageSize:     5,
				ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
				Query: func(_ context.Context, _ []func(*sql.Selector), limit, _ int) ([]*mockEntity, error) {
					data := makeMockData(3)
					if limit < len(data) {
						data = data[:limit]
					}
					return data, nil
				},
				Convert: convert,
			},
			wantLen:       3,
			wantNextToken: false,
		},
		{
			name: "pagination with next page",
			params: ListParams[mockEntity, mockProto]{
				EntityName:   "test",
				PageSize:     5,
				ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
				Query: func(_ context.Context, _ []func(*sql.Selector), limit, _ int) ([]*mockEntity, error) {
					// Return 6 items when limit is 6 (pageSize+1).
					data := makeMockData(6)
					if limit < len(data) {
						data = data[:limit]
					}
					return data, nil
				},
				Convert: convert,
			},
			wantLen:       5,
			wantNextToken: true,
		},
		{
			name: "invalid page token",
			params: ListParams[mockEntity, mockProto]{
				EntityName:   "test",
				PageSize:     5,
				PageToken:    "not-valid-base64!!!",
				ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
				Query: func(_ context.Context, _ []func(*sql.Selector), _, _ int) ([]*mockEntity, error) {
					return nil, nil
				},
				Convert: convert,
			},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name: "filter error propagates",
			params: ListParams[mockEntity, mockProto]{
				EntityName: "test",
				PageSize:   5,
				ApplyFilters: func() ([]func(*sql.Selector), error) {
					return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("bad filter"))
				},
				Query: func(_ context.Context, _ []func(*sql.Selector), _, _ int) ([]*mockEntity, error) {
					return nil, nil
				},
				Convert: convert,
			},
			wantErr: connect.CodeInvalidArgument,
		},
		{
			name: "query error returns internal",
			params: ListParams[mockEntity, mockProto]{
				EntityName:   "test",
				PageSize:     5,
				ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
				Query: func(_ context.Context, _ []func(*sql.Selector), _, _ int) ([]*mockEntity, error) {
					return nil, errors.New("db connection failed")
				},
				Convert: convert,
			},
			wantErr: connect.CodeInternal,
		},
		{
			name: "default page size when zero",
			params: ListParams[mockEntity, mockProto]{
				EntityName:   "test",
				PageSize:     0,
				ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
				Query: func(_ context.Context, _ []func(*sql.Selector), limit, _ int) ([]*mockEntity, error) {
					// The default page size is 100, so limit should be 101.
					if limit != defaultPageSize+1 {
						return nil, fmt.Errorf("expected limit %d, got %d", defaultPageSize+1, limit)
					}
					return makeMockData(3), nil
				},
				Convert: convert,
			},
			wantLen:       3,
			wantNextToken: false,
		},
		{
			name: "max page size clamped",
			params: ListParams[mockEntity, mockProto]{
				EntityName:   "test",
				PageSize:     5000,
				ApplyFilters: func() ([]func(*sql.Selector), error) { return nil, nil },
				Query: func(_ context.Context, _ []func(*sql.Selector), limit, _ int) ([]*mockEntity, error) {
					// Max page size is 1000, so limit should be 1001.
					if limit != maxPageSize+1 {
						return nil, fmt.Errorf("expected limit %d, got %d", maxPageSize+1, limit)
					}
					return makeMockData(3), nil
				},
				Convert: convert,
			},
			wantLen:       3,
			wantNextToken: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			items, nextToken, err := ListEntities(ctx, tt.params)

			if tt.wantErr != 0 {
				if err == nil {
					t.Fatalf("expected error code %v, got nil", tt.wantErr)
				}
				if code := connect.CodeOf(err); code != tt.wantErr {
					t.Errorf("error code = %v, want %v", code, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := len(items); got != tt.wantLen {
				t.Errorf("got %d items, want %d", got, tt.wantLen)
			}
			if tt.wantNextToken && nextToken == "" {
				t.Error("expected non-empty next page token")
			}
			if !tt.wantNextToken && nextToken != "" {
				t.Errorf("expected empty next page token, got %q", nextToken)
			}
		})
	}
}

func TestCastPredicates(t *testing.T) {
	t.Parallel()

	// Create 3 sql.Selector predicates.
	called := make([]bool, 3)
	preds := make([]func(*sql.Selector), 3)
	for i := range 3 {
		idx := i
		preds[i] = func(_ *sql.Selector) {
			called[idx] = true
		}
	}

	// Cast to a concrete predicate type.
	type myPred func(*sql.Selector)
	result := castPredicates[myPred](preds)

	if len(result) != 3 {
		t.Fatalf("got %d predicates, want 3", len(result))
	}

	// Verify each function is callable and matches the original.
	for i, fn := range result {
		fn(nil)
		if !called[i] {
			t.Errorf("predicate %d was not called", i)
		}
	}
}

func TestCastPredicatesEmpty(t *testing.T) {
	t.Parallel()

	type myPred func(*sql.Selector)
	result := castPredicates[myPred](nil)

	if len(result) != 0 {
		t.Fatalf("got %d predicates from nil input, want 0", len(result))
	}
}
