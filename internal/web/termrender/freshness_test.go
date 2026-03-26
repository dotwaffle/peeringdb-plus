package termrender

import (
	"strings"
	"testing"
	"time"
)

func TestFormatFreshness_Recent(t *testing.T) {
	t.Parallel()

	ts := time.Now().Add(-12 * time.Minute)
	got := FormatFreshness(ts)

	if !strings.Contains(got, "12 minutes ago") {
		t.Errorf("FormatFreshness(-12m) should contain '12 minutes ago', got: %q", got)
	}
	if !strings.Contains(got, ts.UTC().Format(time.RFC3339)) {
		t.Errorf("FormatFreshness(-12m) should contain RFC3339 timestamp, got: %q", got)
	}
	if !strings.Contains(got, "Data:") {
		t.Errorf("FormatFreshness(-12m) should contain 'Data:' prefix, got: %q", got)
	}
}

func TestFormatFreshness_Hours(t *testing.T) {
	t.Parallel()

	ts := time.Now().Add(-3 * time.Hour)
	got := FormatFreshness(ts)

	if !strings.Contains(got, "3 hours ago") {
		t.Errorf("FormatFreshness(-3h) should contain '3 hours ago', got: %q", got)
	}
}

func TestFormatFreshness_Zero(t *testing.T) {
	t.Parallel()

	got := FormatFreshness(time.Time{})
	if got != "" {
		t.Errorf("FormatFreshness(zero) should return empty string, got: %q", got)
	}
}

func TestFormatFreshness_JustNow(t *testing.T) {
	t.Parallel()

	ts := time.Now().Add(-30 * time.Second)
	got := FormatFreshness(ts)

	if !strings.Contains(got, "just now") {
		t.Errorf("FormatFreshness(-30s) should contain 'just now', got: %q", got)
	}
}

func TestFormatFreshness_Days(t *testing.T) {
	t.Parallel()

	ts := time.Now().Add(-48 * time.Hour)
	got := FormatFreshness(ts)

	if !strings.Contains(got, "2 days ago") {
		t.Errorf("FormatFreshness(-48h) should contain '2 days ago', got: %q", got)
	}
}

func TestFormatFreshness_HasNewlines(t *testing.T) {
	t.Parallel()

	ts := time.Now().Add(-5 * time.Minute)
	got := FormatFreshness(ts)

	if !strings.HasPrefix(got, "\n") {
		t.Errorf("FormatFreshness should start with leading newline, got: %q", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("FormatFreshness should end with trailing newline, got: %q", got)
	}
}

func TestFormatFreshness_SingleMinute(t *testing.T) {
	t.Parallel()

	ts := time.Now().Add(-1 * time.Minute)
	got := FormatFreshness(ts)

	if !strings.Contains(got, "1 minute ago") {
		t.Errorf("FormatFreshness(-1m) should contain '1 minute ago', got: %q", got)
	}
}

func TestFormatFreshness_SingleHour(t *testing.T) {
	t.Parallel()

	ts := time.Now().Add(-1 * time.Hour)
	got := FormatFreshness(ts)

	if !strings.Contains(got, "1 hour ago") {
		t.Errorf("FormatFreshness(-1h) should contain '1 hour ago', got: %q", got)
	}
}
