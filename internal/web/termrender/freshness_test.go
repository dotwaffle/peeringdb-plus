package termrender

import (
	"strings"
	"testing"
	"time"
)

// TestFormatFreshness_AbsoluteTimestamp verifies the footer contains the
// absolute UTC timestamp with the "Data: " prefix and contains NO wall-clock-
// relative phrases. The cached HTTP response body includes this footer, so
// any relative text ("N minutes ago", "just now") would freeze at cache-
// creation time and mislead readers for up to a full sync interval.
func TestFormatFreshness_AbsoluteTimestamp(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 11, 12, 27, 46, 0, time.UTC)
	got := FormatFreshness(ts)

	if !strings.Contains(got, "Data: ") {
		t.Errorf("output missing 'Data: ' prefix, got: %q", got)
	}
	if want := ts.Format(time.RFC3339); !strings.Contains(got, want) {
		t.Errorf("output missing RFC3339 timestamp %q, got: %q", want, got)
	}

	// Cache-safety regression lock: no wall-clock-relative phrasing.
	forbidden := []string{"ago", "just now", "minute", "hour", " day"}
	for _, bad := range forbidden {
		if strings.Contains(got, bad) {
			t.Errorf("output contains forbidden relative-time phrase %q (cache would serve stale text), got: %q", bad, got)
		}
	}
}

// TestFormatFreshness_Deterministic asserts the output is byte-identical
// across two calls with the same input but separated in wall-clock time.
// This is the stronger proof that no relative-time computation leaks into
// the rendered output: if any time.Since-style expression remained, the two
// renders would diverge and this test would fail.
func TestFormatFreshness_Deterministic(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 11, 12, 27, 46, 0, time.UTC)
	first := FormatFreshness(ts)
	time.Sleep(15 * time.Millisecond)
	second := FormatFreshness(ts)
	if first != second {
		t.Errorf("FormatFreshness output changed between calls:\n first=%q\nsecond=%q", first, second)
	}
}

// TestFormatFreshness_Zero verifies zero-time returns an empty footer so
// pre-sync responses don't render a misleading "Data: 0001-01-01..." line.
func TestFormatFreshness_Zero(t *testing.T) {
	t.Parallel()

	got := FormatFreshness(time.Time{})
	if got != "" {
		t.Errorf("FormatFreshness(zero) should return empty string, got: %q", got)
	}
}

// TestFormatFreshness_HasNewlines locks in the leading/trailing newline
// padding so callers that concatenate the footer directly onto rendered
// content don't have to insert their own whitespace.
func TestFormatFreshness_HasNewlines(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 4, 11, 12, 27, 46, 0, time.UTC)
	got := FormatFreshness(ts)

	if !strings.HasPrefix(got, "\n") {
		t.Errorf("FormatFreshness should start with leading newline, got: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("FormatFreshness should end with trailing newline, got: %q", got)
	}
}
