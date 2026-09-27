package pdbcompat

import (
	"errors"
	"math"
	"net/url"
	"strings"
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
		{"since=nan", 0, errSinceNotTimestamp},
		{"since=inf", 0, errSinceNotTimestamp},
		{"since=1e999", 0, errSinceNotTimestamp},
		{"since=0x10", 0, errSinceNotTimestamp},
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

// TestParseDepthParam locks the upstream parse of depth (2.83.0
// rest.py:520-523): last value, Python int() rules, empty = 400.
func TestParseDepthParam(t *testing.T) {
	t.Parallel()

	big := "99999999999999999999"
	max4300 := "1" + strings.Repeat("0", 4299)
	for _, tc := range []struct {
		name        string
		vals        []string // nil = key absent
		want        int
		wantText    string
		wantPresent bool
		wantErr     error
	}{
		{"absent", nil, 0, "0", false, nil},
		{"zero", []string{"0"}, 0, "0", true, nil},
		{"two", []string{"2"}, 2, "2", true, nil},
		{"negative", []string{"-3"}, -3, "-3", true, nil},
		{"white_space", []string{" 2 "}, 2, "2", true, nil},
		{"plus_sign", []string{"+2"}, 2, "2", true, nil},
		{"underscore", []string{"0_2"}, 2, "2", true, nil},
		{"double_underscore", []string{"1__2"}, 0, "", true, errDepthNotNumber},
		{"leading_underscore", []string{"_1"}, 0, "", true, errDepthNotNumber},
		{"trailing_underscore", []string{"1_"}, 0, "", true, errDepthNotNumber},
		{"letters", []string{"abc"}, 0, "", true, errDepthNotNumber},
		{"empty", []string{""}, 0, "", true, errDepthNotNumber},
		{"float", []string{"1.5"}, 0, "", true, errDepthNotNumber},
		{"hex", []string{"0x1"}, 0, "", true, errDepthNotNumber},
		{"arabic_indic_digit", []string{"\u0662"}, 2, "2", true, nil},
		{"file_separator_not_space", []string{"\x1c2"}, 0, "", true, errDepthNotNumber},
		{"ideographic_space", []string{"\u30002\u3000"}, 2, "2", true, nil},
		{"saturated", []string{big}, math.MaxInt, big, true, nil},
		{"negative_saturated", []string{"-" + big}, math.MinInt, "-" + big, true, nil},
		{"max_digits", []string{max4300}, math.MaxInt, max4300, true, nil},
		{"over_max_digits", []string{max4300 + "0"}, 0, "", true, errDepthNotNumber},
		{"leading_zeros_count", []string{strings.Repeat("0", 4301)}, 0, "", true, errDepthNotNumber},
		{"last_value_wins", []string{"abc", "1"}, 1, "1", true, nil},
		{"last_value_bad", []string{"1", "abc"}, 0, "", true, errDepthNotNumber},
	} {
		params := url.Values{}
		if tc.vals != nil {
			params["depth"] = tc.vals
		}
		raw, text, present, err := ParseDepthParam(params)
		if !errors.Is(err, tc.wantErr) || raw != tc.want || text != tc.wantText || present != tc.wantPresent {
			t.Errorf("%s: (%d, %q, %v, %v), want (%d, %q, %v, %v)",
				tc.name, raw, text, present, err, tc.want, tc.wantText, tc.wantPresent, tc.wantErr)
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

// TestParseSince_FloatForms checks that a since value that Python
// float() accepts and int() rejects is a *sinceIntError with the Python
// message (2.83.0 rest.py:505-510, api_cache.py:80).
func TestParseSince_FloatForms(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ query, want string }{
		{"since=1.5", "invalid literal for int() with base 10: '1.5'"},
		{"since=1e3", "invalid literal for int() with base 10: '1e3'"},
		{"since=%201.5%20", "invalid literal for int() with base 10: ' 1.5 '"},
		{"since=-.5", "invalid literal for int() with base 10: '-.5'"},
		{"since=1_0.5", "invalid literal for int() with base 10: '1_0.5'"},
		{"since=5.", "invalid literal for int() with base 10: '5.'"},
	} {
		params, err := url.ParseQuery(tc.query)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", tc.query, err)
		}
		_, present, err := parseSince(params)
		var sie *sinceIntError
		if !present || !errors.As(err, &sie) || err.Error() != tc.want {
			t.Errorf("%q: present %v, err %v; want a sinceIntError %q", tc.query, present, err, tc.want)
		}
		p, err := parseRequestParams(params)
		if err != nil || p.sinceInt == nil || p.since != nil {
			t.Errorf("%q: parseRequestParams = %+v, %v; want sinceInt set and no error", tc.query, p, err)
		}
	}
}

func TestClassifyPyFloat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want pyFloatKind
	}{
		{"1", pyFloatFinite},
		{"1.5", pyFloatFinite},
		{" -1.5e-3\n", pyFloatFinite},
		{".5", pyFloatFinite},
		{"5.", pyFloatFinite},
		{"1_000.000_1e1_0", pyFloatFinite},
		{"\u0661.\u0665", pyFloatFinite},
		{"1e-999", pyFloatFinite},
		{"INF", pyFloatInf},
		{"-Infinity", pyFloatInf},
		{"1e999", pyFloatInf},
		{"nan", pyFloatNaN},
		{"+NaN", pyFloatNaN},
		{"", pyFloatInvalid},
		{".", pyFloatInvalid},
		{"e3", pyFloatInvalid},
		{".e3", pyFloatInvalid},
		{"1__0", pyFloatInvalid},
		{"_1", pyFloatInvalid},
		{"1_", pyFloatInvalid},
		{"1_.5", pyFloatInvalid},
		{"1._5", pyFloatInvalid},
		{"0x10", pyFloatInvalid},
		{"1e", pyFloatInvalid},
		{"infin", pyFloatInvalid},
		{"--1", pyFloatInvalid},
	} {
		if got := classifyPyFloat(tc.in); got != tc.want {
			t.Errorf("classifyPyFloat(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestPyRepr(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"abc", "'abc'"},
		{"it's", `"it's"`},
		{`it's "x"`, `'it\'s "x"'`},
		{`a\b`, `'a\\b'`},
		{"a\tb\nc\rd", `'a\tb\nc\rd'`},
		{"\x00\x7f\u0085", `'\x00\x7f\x85'`},
		{"\u200b\u2028", `'\u200b\u2028'`},
		{"é ü \u00a0", `'é ü \xa0'`},
	} {
		if got := pyRepr(tc.in); got != tc.want {
			t.Errorf("pyRepr(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}
