package graph

import "fmt"

const (
	// DefaultLimit is the default page size for offset/limit queries.
	DefaultLimit = 100
	// MaxLimit is the maximum page size per D-14.
	MaxLimit = 1000
)

// validatePageSize checks that first/last do not exceed MaxLimit for cursor-based pagination.
func validatePageSize(first, last *int) error {
	if first != nil && *first > MaxLimit {
		return fmt.Errorf("first must not exceed %d, got %d", MaxLimit, *first)
	}
	if last != nil && *last > MaxLimit {
		return fmt.Errorf("last must not exceed %d, got %d", MaxLimit, *last)
	}
	return nil
}

// defaultFirst bounds an otherwise-unbounded Relay connection. When the
// client supplies neither first nor last, the entgql paginator applies no
// LIMIT and materializes the entire table (a full-table scan plus a
// connection of every row). Default first to DefaultLimit so a parameterless
// connection query returns a bounded first page; explicit first/last (already
// max-checked by validatePageSize) are passed through unchanged.
func defaultFirst(first, last *int) *int {
	if first == nil && last == nil {
		d := DefaultLimit
		return &d
	}
	return first
}

// OffsetLimitInput holds validated offset and limit values.
type OffsetLimitInput struct {
	Offset int
	Limit  int
}

// ValidateOffsetLimit validates and applies defaults to offset/limit arguments.
// offset defaults to 0, limit defaults to DefaultLimit (100).
// Returns an error if limit exceeds MaxLimit (1000) or either value is negative.
func ValidateOffsetLimit(offset *int, limit *int) (OffsetLimitInput, error) {
	result := OffsetLimitInput{
		Offset: 0,
		Limit:  DefaultLimit,
	}
	if offset != nil {
		if *offset < 0 {
			return result, fmt.Errorf("offset must be non-negative, got %d", *offset)
		}
		result.Offset = *offset
	}
	if limit != nil {
		if *limit < 1 {
			return result, fmt.Errorf("limit must be at least 1, got %d", *limit)
		}
		if *limit > MaxLimit {
			return result, fmt.Errorf("limit must not exceed %d, got %d", MaxLimit, *limit)
		}
		result.Limit = *limit
	}
	return result, nil
}
