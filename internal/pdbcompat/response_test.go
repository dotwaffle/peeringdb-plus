package pdbcompat

import (
	"errors"
	"net/url"
	"testing"
	"time"
)

// TestParsePaginationParams locks the upstream parse of limit and skip
// (2.83.0 rest.py:511-518): last value, Python int() rules, skip first,
// signed values.
func TestParsePaginationParams(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		query     string
		wantLimit int
		wantSkip  int
		wantErr   error
	}{
		{"", DefaultLimit, 0, nil},
		{"limit=5&skip=2", 5, 2, nil},
		{"limit=-5", -5, 0, nil},
		{"skip=-1", DefaultLimit, -1, nil},
		{"limit=%205&skip=%2B2", 5, 2, nil},
		{"limit=1_0", 10, 0, nil},
		{"limit=%EF%BC%95", 5, 0, nil},
		{"limit=1&limit=2", 2, 0, nil},
		{"skip=1&skip=3", DefaultLimit, 3, nil},
		{"limit=", 0, 0, errLimitNotNumber},
		{"skip=", 0, 0, errSkipNotNumber},
		{"limit=abc", 0, 0, errLimitNotNumber},
		{"limit=1.5", 0, 0, errLimitNotNumber},
		{"skip=abc&limit=abc", 0, 0, errSkipNotNumber},
		{"limit=abc&limit=3", 3, 0, nil},
		{"limit=3&limit=abc", 0, 0, errLimitNotNumber},
	} {
		params, err := url.ParseQuery(tc.query)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", tc.query, err)
		}
		limit, skip, err := ParsePaginationParams(params)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%q: err = %v, want %v", tc.query, err, tc.wantErr)
			continue
		}
		if err == nil && (limit != tc.wantLimit || skip != tc.wantSkip) {
			t.Errorf("%q: (limit, skip) = (%d, %d), want (%d, %d)", tc.query, limit, skip, tc.wantLimit, tc.wantSkip)
		}
	}
}

// TestParseSinceParam locks the upstream parse of since (2.83.0
// rest.py:505-510, :719): last value, Python int() rules, empty = 400,
// 0 or less = no since.
func TestParseSinceParam(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		query   string
		want    int64 // 0 = nil result
		wantErr error
	}{
		{"", 0, nil},
		{"since=", 0, errSinceNotTimestamp},
		{"since=abc", 0, errSinceNotTimestamp},
		{"since=1.5", 0, errSinceNotTimestamp},
		{"since=0", 0, nil},
		{"since=-5", 0, nil},
		{"since=%2010%20", 10, nil},
		{"since=1_0", 10, nil},
		{"since=1&since=2", 2, nil},
		{"since=abc&since=7", 7, nil},
	} {
		params, err := url.ParseQuery(tc.query)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", tc.query, err)
		}
		got, err := ParseSinceParam(params)
		if !errors.Is(err, tc.wantErr) {
			t.Errorf("%q: err = %v, want %v", tc.query, err, tc.wantErr)
			continue
		}
		switch {
		case tc.want == 0 && got != nil:
			t.Errorf("%q: since = %v, want nil", tc.query, *got)
		case tc.want != 0 && (got == nil || !got.Equal(time.Unix(tc.want, 0)) || got.Location() != time.UTC):
			t.Errorf("%q: since = %v, want %v UTC", tc.query, got, time.Unix(tc.want, 0).UTC())
		}
	}
}

// TestParseSince locks the raw value and the present flag of
// parseSince.
func TestParseSince(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		query       string
		want        int
		wantPresent bool
		wantErr     error
	}{
		{"", 0, false, nil},
		{"since=0", 0, true, nil},
		{"since=-5", -5, true, nil},
		{"since=", 0, true, errSinceNotTimestamp},
		{"since=%EF%BC%91", 1, true, nil},
	} {
		params, err := url.ParseQuery(tc.query)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", tc.query, err)
		}
		n, present, err := parseSince(params)
		if !errors.Is(err, tc.wantErr) || n != tc.want || present != tc.wantPresent {
			t.Errorf("%q: (%d, %v, %v), want (%d, %v, %v)", tc.query, n, present, err, tc.want, tc.wantPresent, tc.wantErr)
		}
	}
}

// TestLastParam locks the QueryDict.get rule: the last value, and
// ("", true) for a present empty value.
func TestLastParam(t *testing.T) {
	t.Parallel()

	params := url.Values{"a": {"1", "2"}, "b": {""}, "c": {}}
	for _, tc := range []struct {
		key    string
		want   string
		wantOK bool
	}{
		{"a", "2", true},
		{"b", "", true},
		{"c", "", false},
		{"d", "", false},
	} {
		if got, ok := lastParam(params, tc.key); got != tc.want || ok != tc.wantOK {
			t.Errorf("lastParam(%q) = (%q, %v), want (%q, %v)", tc.key, got, ok, tc.want, tc.wantOK)
		}
	}
}
