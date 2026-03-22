package graph

import "fmt"

const (
	// DefaultLimit is the default page size for offset/limit queries.
	DefaultLimit = 100
	// MaxLimit is the maximum page size per D-14.
	MaxLimit = 1000
)

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
