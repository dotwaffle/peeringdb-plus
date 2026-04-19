package grpcserver

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestNormalizePageSize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		requested int32
		want      int
	}{
		{name: "zero returns default", requested: 0, want: 100},
		{name: "negative returns default", requested: -1, want: 100},
		{name: "within range", requested: 50, want: 50},
		{name: "at default", requested: 100, want: 100},
		{name: "at max", requested: 1000, want: 1000},
		{name: "above max clamped", requested: 1001, want: 1000},
		{name: "far above max clamped", requested: 5000, want: 1000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizePageSize(tt.requested); got != tt.want {
				t.Errorf("normalizePageSize(%d) = %d, want %d", tt.requested, got, tt.want)
			}
		})
	}
}

func TestDecodePageToken(t *testing.T) {
	t.Parallel()
	validToken := base64.StdEncoding.EncodeToString([]byte("100"))
	invalidBase64 := "not-valid-base64!!!"
	nonNumericToken := base64.StdEncoding.EncodeToString([]byte("abc"))
	negativeToken := base64.StdEncoding.EncodeToString([]byte("-1"))

	tests := []struct {
		name      string
		token     string
		want      int
		wantError bool
	}{
		{name: "empty token returns zero", token: "", want: 0, wantError: false},
		{name: "valid token", token: validToken, want: 100, wantError: false},
		{name: "invalid base64", token: invalidBase64, want: 0, wantError: true},
		{name: "non-numeric value", token: nonNumericToken, want: 0, wantError: true},
		{name: "negative offset", token: negativeToken, want: 0, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := decodePageToken(tt.token)
			if (err != nil) != tt.wantError {
				t.Errorf("decodePageToken(%q) error = %v, wantError %v", tt.token, err, tt.wantError)
				return
			}
			if got != tt.want {
				t.Errorf("decodePageToken(%q) = %d, want %d", tt.token, got, tt.want)
			}
		})
	}
}

func TestEncodePageToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		offset int
		want   string
	}{
		{name: "zero returns empty", offset: 0, want: ""},
		{name: "negative returns empty", offset: -1, want: ""},
		{name: "positive offset", offset: 100, want: base64.StdEncoding.EncodeToString([]byte("100"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := encodePageToken(tt.offset); got != tt.want {
				t.Errorf("encodePageToken(%d) = %q, want %q", tt.offset, got, tt.want)
			}
		})
	}
}

func TestPageTokenRoundTrip(t *testing.T) {
	t.Parallel()
	offsets := []int{1, 50, 100, 500, 999}
	for _, offset := range offsets {
		token := encodePageToken(offset)
		got, err := decodePageToken(token)
		if err != nil {
			t.Errorf("round-trip failed for offset %d: encode=%q, decode error=%v", offset, token, err)
			continue
		}
		if got != offset {
			t.Errorf("round-trip mismatch: encodePageToken(%d) = %q, decodePageToken(%q) = %d", offset, token, token, got)
		}
	}
}

// TestStreamCursorRoundTrip verifies that streamCursor values survive an
// encode/decode cycle intact across a range of timestamp boundaries and ids.
// The compound keyset cursor is the foundation for Phase 67's default
// (-updated, -created, -id) ordering (CONTEXT.md D-01); a round-trip failure
// here would silently corrupt stream resume positions.
func TestStreamCursorRoundTrip(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   streamCursor
	}{
		{name: "epoch", in: streamCursor{Updated: time.Unix(0, 0).UTC(), ID: 1}},
		{name: "nano precision", in: streamCursor{Updated: time.Date(2026, 4, 19, 12, 0, 0, 123456789, time.UTC), ID: 42}},
		{name: "future", in: streamCursor{Updated: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), ID: 99999}},
		{name: "id one", in: streamCursor{Updated: time.Date(2025, 6, 15, 8, 30, 45, 0, time.UTC), ID: 1}},
		{name: "large id", in: streamCursor{Updated: time.Date(2024, 12, 31, 23, 59, 59, 999999999, time.UTC), ID: 2147483647}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enc := encodeStreamCursor(tc.in)
			if enc == "" {
				t.Fatalf("encodeStreamCursor(%+v) returned empty string for non-empty cursor", tc.in)
			}
			got, err := decodeStreamCursor(enc)
			if err != nil {
				t.Fatalf("decodeStreamCursor(%q) error = %v", enc, err)
			}
			if !got.Updated.Equal(tc.in.Updated) {
				t.Errorf("round-trip Updated mismatch: got %v, want %v", got.Updated, tc.in.Updated)
			}
			if got.ID != tc.in.ID {
				t.Errorf("round-trip ID mismatch: got %d, want %d", got.ID, tc.in.ID)
			}
		})
	}
}

// TestStreamCursorEmpty verifies the zero-value contract: empty string
// decodes to zero-value streamCursor (start of stream), and a zero-value
// streamCursor encodes to an empty string (signals "no next page").
func TestStreamCursorEmpty(t *testing.T) {
	t.Parallel()

	t.Run("empty string decodes to zero cursor", func(t *testing.T) {
		t.Parallel()
		got, err := decodeStreamCursor("")
		if err != nil {
			t.Fatalf("decodeStreamCursor(\"\") error = %v", err)
		}
		if !got.empty() {
			t.Errorf("decodeStreamCursor(\"\") = %+v, want empty cursor", got)
		}
		if !got.Updated.IsZero() || got.ID != 0 {
			t.Errorf("decodeStreamCursor(\"\") = %+v, want zero-value", got)
		}
	})

	t.Run("zero cursor encodes to empty string", func(t *testing.T) {
		t.Parallel()
		enc := encodeStreamCursor(streamCursor{})
		if enc != "" {
			t.Errorf("encodeStreamCursor(zero) = %q, want empty string", enc)
		}
	})
}

// TestStreamCursorInvalidBase64 verifies that malformed base64 input is
// rejected with a decode error rather than silently returning garbage.
// Trust-boundary input: page_token arrives over the wire from untrusted
// clients (threat T-67-04-01).
func TestStreamCursorInvalidBase64(t *testing.T) {
	t.Parallel()
	bad := []string{
		"!!!not-valid-base64!!!",
		"====",
		"abc def",
	}
	for _, token := range bad {
		t.Run(token, func(t *testing.T) {
			t.Parallel()
			got, err := decodeStreamCursor(token)
			if err == nil {
				t.Fatalf("decodeStreamCursor(%q) expected error, got cursor %+v", token, got)
			}
		})
	}
}

// TestStreamCursorInvalidFormat verifies that a valid base64 body with an
// unparseable timestamp is rejected with a timestamp-parse error. Protects
// against a crafted token forcing nonsense into the ORDER BY predicate.
func TestStreamCursorInvalidFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
	}{
		{name: "not a timestamp", body: "notatimestamp:5"},
		{name: "missing colon separator", body: "2026-04-19T12:00:00Z"},
		{name: "garbage id", body: "2026-04-19T12:00:00Z:abc"},
		{name: "empty timestamp", body: ":5"},
		{name: "empty id", body: "2026-04-19T12:00:00Z:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			token := base64.StdEncoding.EncodeToString([]byte(tc.body))
			got, err := decodeStreamCursor(token)
			if err == nil {
				t.Fatalf("decodeStreamCursor(%q=body %q) expected error, got cursor %+v", token, tc.body, got)
			}
		})
	}
}

// TestStreamCursorNegativeID rejects cursors with negative ids. The id is
// a primary-key surrogate and monotonic-positive by construction; a
// negative id is always a tampered token or encoder bug.
func TestStreamCursorNegativeID(t *testing.T) {
	t.Parallel()
	token := base64.StdEncoding.EncodeToString([]byte("2026-01-01T00:00:00Z:-5"))
	got, err := decodeStreamCursor(token)
	if err == nil {
		t.Fatalf("decodeStreamCursor with negative id expected error, got cursor %+v", got)
	}
}

// TestStreamCursorColonsInTimestamp verifies that RFC3339Nano timestamps
// (which contain three colons of their own: HH:MM:SS and the TZ offset)
// round-trip correctly. The decoder MUST split on the LAST colon so the
// timestamp body stays intact.
func TestStreamCursorColonsInTimestamp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   streamCursor
	}{
		{name: "nano with colons", in: streamCursor{Updated: time.Date(2026, 4, 19, 12, 0, 0, 123456789, time.UTC), ID: 1234}},
		{name: "subsecond zero", in: streamCursor{Updated: time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC), ID: 7}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			enc := encodeStreamCursor(tc.in)
			got, err := decodeStreamCursor(enc)
			if err != nil {
				t.Fatalf("decodeStreamCursor(%q) error = %v", enc, err)
			}
			if !got.Updated.Equal(tc.in.Updated) || got.ID != tc.in.ID {
				t.Errorf("round-trip with colons in timestamp failed: got %+v, want %+v", got, tc.in)
			}
		})
	}
}
