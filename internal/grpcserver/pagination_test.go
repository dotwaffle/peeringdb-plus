package grpcserver

import (
	"encoding/base64"
	"testing"
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
