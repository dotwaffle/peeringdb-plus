package main

import (
	"sort"
	"testing"
	"time"
)

// TestPercentilesFromSorted verifies the nearest-rank percentile
// calculation against a fixed input distribution: 99 × 1ms + 1 × 100ms.
// Expectations:
//   - p50 = 1ms (50th of 100 items is index 49 → 1ms)
//   - p95 = 1ms (95th of 100 items is index 94 → 1ms)
//   - p99 = 1ms (99th of 100 items is index 98 → 1ms; the 100ms outlier is at index 99 == p100)
//   - p100 = 100ms (top-of-distribution)
//
// This locks the contract that p99 of {1ms × 99, 100ms × 1} reports
// the typical-latency, with the outlier surfacing only at p100. Soak
// reports do NOT print p100, so a single anomalous request will not
// register on the dashboard — operators read p99 and the err count
// instead. The doc comment in report.go documents this trade-off.
func TestPercentilesFromSorted(t *testing.T) {
	t.Parallel()

	in := make([]time.Duration, 0, 100)
	for range 99 {
		in = append(in, 1*time.Millisecond)
	}
	in = append(in, 100*time.Millisecond)
	sort.Slice(in, func(i, j int) bool { return in[i] < in[j] })

	cases := []struct {
		p    float64
		want time.Duration
	}{
		{50, 1 * time.Millisecond},
		{95, 1 * time.Millisecond},
		{99, 1 * time.Millisecond},
		{100, 100 * time.Millisecond},
	}
	for _, c := range cases {
		got := percentilesFromSorted(in, c.p)
		if got != c.want {
			t.Errorf("p%.0f = %v, want %v", c.p, got, c.want)
		}
	}
}

// TestPercentilesFromSorted_TightDistribution covers the bucket-of-1
// and bucket-of-N-identical edge cases.
func TestPercentilesFromSorted_TightDistribution(t *testing.T) {
	t.Parallel()

	if got := percentilesFromSorted(nil, 50); got != 0 {
		t.Errorf("empty p50 = %v, want 0", got)
	}
	if got := percentilesFromSorted([]time.Duration{42 * time.Millisecond}, 99); got != 42*time.Millisecond {
		t.Errorf("singleton p99 = %v, want 42ms", got)
	}
	uniform := []time.Duration{5 * time.Millisecond, 5 * time.Millisecond, 5 * time.Millisecond}
	if got := percentilesFromSorted(uniform, 95); got != 5*time.Millisecond {
		t.Errorf("uniform p95 = %v, want 5ms", got)
	}
}
