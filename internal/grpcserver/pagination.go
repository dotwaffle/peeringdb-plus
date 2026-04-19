package grpcserver

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
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

// streamCursor is the compound keyset cursor used by Stream* RPCs once Plan 05
// flips the StreamParams.QueryBatch signature. Pairs the upstream `updated`
// timestamp with the row id tiebreaker so `ORDER BY updated DESC, id DESC`
// resume positions are stable across concurrent edits (CONTEXT.md D-01).
//
// The wire envelope remains the opaque `string page_token` proto field
// (RESEARCH §G-08); only the base64-encoded body shape changes — no proto
// regen required. Existing offset-based encodePageToken / decodePageToken are
// retained for List* RPCs per RESEARCH §4 "Note on ListEntities".
type streamCursor struct {
	Updated time.Time
	ID      int
}

// empty reports whether the cursor is a zero value, which signals either
// the start of a stream (on decode) or the end of one (on encode).
func (c streamCursor) empty() bool {
	return c.Updated.IsZero() && c.ID == 0
}

// encodeStreamCursor emits the base64-encoded cursor body in the form
// `RFC3339Nano:id`. Returns an empty string for a zero-value cursor so
// callers can propagate "no next page" without a special sentinel.
func encodeStreamCursor(c streamCursor) string {
	if c.empty() {
		return ""
	}
	body := fmt.Sprintf("%s:%d", c.Updated.UTC().Format(time.RFC3339Nano), c.ID)
	return base64.StdEncoding.EncodeToString([]byte(body))
}

// decodeStreamCursor parses a page_token produced by encodeStreamCursor. An
// empty token decodes to a zero-value cursor (start of stream). Base64,
// timestamp, and id are all validated; a negative id is rejected because
// the id is a positive-monotonic primary-key surrogate (threat
// T-67-04-01).
//
// The RFC3339Nano timestamp body contains its own colons (HH:MM:SS and
// possibly a zone offset), so the parser splits on the LAST colon to keep
// the timestamp intact.
func decodeStreamCursor(token string) (streamCursor, error) {
	if token == "" {
		return streamCursor{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return streamCursor{}, fmt.Errorf("decode stream cursor: %w", err)
	}
	s := string(raw)
	idx := strings.LastIndex(s, ":")
	if idx < 0 {
		return streamCursor{}, fmt.Errorf("invalid stream cursor body: %q", s)
	}
	tsPart := s[:idx]
	if tsPart == "" {
		return streamCursor{}, fmt.Errorf("invalid stream cursor: empty timestamp")
	}
	t, err := time.Parse(time.RFC3339Nano, tsPart)
	if err != nil {
		return streamCursor{}, fmt.Errorf("parse stream cursor timestamp %q: %w", tsPart, err)
	}
	idPart := s[idx+1:]
	if idPart == "" {
		return streamCursor{}, fmt.Errorf("invalid stream cursor: empty id")
	}
	id, err := strconv.Atoi(idPart)
	if err != nil {
		return streamCursor{}, fmt.Errorf("parse stream cursor id %q: %w", idPart, err)
	}
	if id < 0 {
		return streamCursor{}, fmt.Errorf("invalid stream cursor: negative id %d", id)
	}
	return streamCursor{Updated: t, ID: id}, nil
}
