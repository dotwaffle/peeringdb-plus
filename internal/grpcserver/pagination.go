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

// streamCursor is the compound keyset cursor used by Stream* RPCs. It carries
// the full three-key keyset (`updated`, `created`, `id`) so that resume
// positions stay exact under the `ORDER BY updated DESC, created DESC, id DESC`
// default order (Phase 67 ORDER-02, CONTEXT.md D-01). A two-key cursor
// (updated, id) silently drops rows at a batch boundary whenever an
// equal-`updated` group is ordered by `created` DESC but resumed on `id`
// alone — `created` is the middle ORDER BY key and must travel in the cursor.
//
// The wire envelope remains the opaque `string page_token` proto field
// (RESEARCH §G-08); only the base64-encoded body shape changes — no proto
// regen required. Stream cursors are internal page-through state, not a
// persisted client contract, so the body format is free to change. Existing
// offset-based encodePageToken / decodePageToken are retained for List* RPCs
// per RESEARCH §4 "Note on ListEntities".
type streamCursor struct {
	Updated time.Time
	Created time.Time
	ID      int
}

// streamCursorDelim separates the three encoded cursor fields. The pipe is
// chosen because it never appears in an RFC3339Nano timestamp (digits, '-',
// 'T', ':', '.', '+', 'Z') nor in a base-10 integer, so a fixed split into
// exactly three parts is unambiguous — unlike the previous last-colon split,
// which only worked because the id had no colons.
const streamCursorDelim = "|"

// empty reports whether the cursor is a zero value, which signals either
// the start of a stream (on decode) or the end of one (on encode).
func (c streamCursor) empty() bool {
	return c.Updated.IsZero() && c.Created.IsZero() && c.ID == 0
}

// encodeStreamCursor emits the base64-encoded cursor body in the form
// `<updatedRFC3339Nano>|<createdRFC3339Nano>|<id>`. Returns an empty string
// for a zero-value cursor so callers can propagate "no next page" without a
// special sentinel.
func encodeStreamCursor(c streamCursor) string {
	if c.empty() {
		return ""
	}
	body := fmt.Sprintf("%s%s%s%s%d",
		c.Updated.UTC().Format(time.RFC3339Nano), streamCursorDelim,
		c.Created.UTC().Format(time.RFC3339Nano), streamCursorDelim,
		c.ID)
	return base64.StdEncoding.EncodeToString([]byte(body))
}

// decodeStreamCursor parses a page_token produced by encodeStreamCursor. An
// empty token decodes to a zero-value cursor (start of stream). Base64, both
// timestamps, and the id are all validated; the body must split into exactly
// three pipe-delimited fields. A negative id is rejected because the id is a
// positive-monotonic primary-key surrogate (threat T-67-04-01).
func decodeStreamCursor(token string) (streamCursor, error) {
	if token == "" {
		return streamCursor{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return streamCursor{}, fmt.Errorf("decode stream cursor: %w", err)
	}
	parts := strings.Split(string(raw), streamCursorDelim)
	if len(parts) != 3 {
		return streamCursor{}, fmt.Errorf("invalid stream cursor body: want 3 fields, got %d", len(parts))
	}
	updated, err := parseCursorTime("updated", parts[0])
	if err != nil {
		return streamCursor{}, err
	}
	created, err := parseCursorTime("created", parts[1])
	if err != nil {
		return streamCursor{}, err
	}
	if parts[2] == "" {
		return streamCursor{}, fmt.Errorf("invalid stream cursor: empty id")
	}
	id, err := strconv.Atoi(parts[2])
	if err != nil {
		return streamCursor{}, fmt.Errorf("parse stream cursor id %q: %w", parts[2], err)
	}
	if id < 0 {
		return streamCursor{}, fmt.Errorf("invalid stream cursor: negative id %d", id)
	}
	return streamCursor{Updated: updated, Created: created, ID: id}, nil
}

// parseCursorTime validates and parses one RFC3339Nano timestamp field of a
// stream cursor, attributing failures to the named field for diagnostics.
func parseCursorTime(field, value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("invalid stream cursor: empty %s", field)
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse stream cursor %s %q: %w", field, value, err)
	}
	return t, nil
}
