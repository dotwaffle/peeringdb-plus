package grpcserver

import (
	"encoding/base64"
	"fmt"
	"strconv"
)

const (
	// defaultPageSize is the number of results returned when the client does
	// not specify a page size.
	defaultPageSize = 100
	// maxPageSize is the upper bound on results per page, preventing
	// accidental full-table dumps.
	maxPageSize = 1000
	// streamBatchSize is the number of rows fetched per database round-trip
	// during streaming RPCs. Hardcoded at 500 per user decision.
	streamBatchSize = 500
)

// normalizePageSize clamps the requested page size to the allowed range.
// Zero or negative values return the default; values exceeding the maximum
// are capped.
func normalizePageSize(requested int32) int {
	if requested <= 0 {
		return defaultPageSize
	}
	if requested > maxPageSize {
		return maxPageSize
	}
	return int(requested)
}

// decodePageToken decodes an opaque base64 page token into an integer offset.
// An empty token returns 0 (first page). Returns an error for malformed tokens
// or negative offsets.
func decodePageToken(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return 0, fmt.Errorf("decode page token: %w", err)
	}
	offset, err := strconv.Atoi(string(raw))
	if err != nil {
		return 0, fmt.Errorf("parse page token offset: %w", err)
	}
	if offset < 0 {
		return 0, fmt.Errorf("invalid page token: negative offset %d", offset)
	}
	return offset, nil
}

// encodePageToken encodes an integer offset into an opaque base64 page token.
// Returns an empty string for offsets at or below zero, which signals the
// absence of a next page.
func encodePageToken(offset int) string {
	if offset <= 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}
