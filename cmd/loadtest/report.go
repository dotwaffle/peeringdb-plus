//go:build loadtest

package main

import (
	"fmt"
	"io"
	"math"
	"sort"
	"sync"
	"time"
)

// Report is the in-memory accumulator for Result values. It is safe
// for concurrent Append calls — soak mode appends from N workers in
// parallel.
type Report struct {
	mu    sync.Mutex
	rows  []Result
	start time.Time
}

// NewReport returns a Report with start-time stamped to time.Now().
func NewReport() *Report {
	return &Report{start: time.Now()}
}

// Append records one Result.
func (r *Report) Append(res Result) {
	r.mu.Lock()
	r.rows = append(r.rows, res)
	r.mu.Unlock()
}

// Len returns the number of recorded Results.
func (r *Report) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

// Snapshot returns a copy of the current rows for read-only inspection
// in tests. The wall-clock duration since NewReport.
func (r *Report) Snapshot() ([]Result, time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Result, len(r.rows))
	copy(out, r.rows)
	return out, time.Since(r.start)
}

// surfaceBucket aggregates per-surface latency + status counts.
type surfaceBucket struct {
	count   int
	success int
	errors  int
	latency []time.Duration
}

func (b *surfaceBucket) percentiles() (p50, p95, p99 time.Duration) {
	return percentilesFromSorted(b.latency, 50), percentilesFromSorted(b.latency, 95), percentilesFromSorted(b.latency, 99)
}

// percentilesFromSorted assumes the slice is sorted ascending and
// returns the value at the given percentile (0-100). Empty slice
// returns 0; single element returns itself; otherwise integer-index
// arithmetic per the standard nearest-rank method.
func percentilesFromSorted(s []time.Duration, p float64) time.Duration {
	n := len(s)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return s[0]
	}
	idx := int(math.Ceil(p/100*float64(n))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= n {
		idx = n - 1
	}
	return s[idx]
}

// Print emits a tab-separated per-surface breakdown plus an overall
// summary footer to w. Header is `column -t -s$'\t'` friendly.
func (r *Report) Print(w io.Writer, mode string) {
	rows, wall := r.Snapshot()

	buckets := map[Surface]*surfaceBucket{}
	overall := &surfaceBucket{}
	for _, res := range rows {
		surf := res.Endpoint.Surface
		b, ok := buckets[surf]
		if !ok {
			b = &surfaceBucket{}
			buckets[surf] = b
		}
		b.count++
		overall.count++
		if res.OK() {
			b.success++
			overall.success++
		} else {
			b.errors++
			overall.errors++
		}
		b.latency = append(b.latency, res.Latency)
		overall.latency = append(overall.latency, res.Latency)
	}
	for _, b := range buckets {
		sort.Slice(b.latency, func(i, j int) bool { return b.latency[i] < b.latency[j] })
	}
	sort.Slice(overall.latency, func(i, j int) bool { return overall.latency[i] < overall.latency[j] })

	fmt.Fprintf(w, "\n=== loadtest %s summary ===\n", mode)
	fmt.Fprintf(w, "wall-clock\t%s\n", wall.Round(time.Millisecond))
	if overall.count > 0 && wall > 0 {
		rps := float64(overall.count) / wall.Seconds()
		fmt.Fprintf(w, "observed-rps\t%.2f req/s\n", rps)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "surface\tcount\tok\terr\tsuccess%\tp50\tp95\tp99")
	// Stable surface order in output.
	for _, surf := range []Surface{SurfacePdbCompat, SurfaceEntRest, SurfaceGraphQL, SurfaceConnectRPC, SurfaceWebUI} {
		b, ok := buckets[surf]
		if !ok {
			continue
		}
		printBucket(w, string(surf), b)
	}
	printBucket(w, "TOTAL", overall)
}

func printBucket(w io.Writer, label string, b *surfaceBucket) {
	if b.count == 0 {
		fmt.Fprintf(w, "%s\t0\t0\t0\t—\t—\t—\t—\n", label)
		return
	}
	p50, p95, p99 := b.percentiles()
	successPct := 100.0 * float64(b.success) / float64(b.count)
	fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%.1f%%\t%s\t%s\t%s\n",
		label, b.count, b.success, b.errors, successPct,
		p50.Round(time.Microsecond), p95.Round(time.Microsecond), p99.Round(time.Microsecond))
}
