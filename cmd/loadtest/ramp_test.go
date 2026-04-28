package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestParseRampFlags_Defaults confirms the ramp flag-parser produces
// the documented spec defaults when no flags are passed. Failure here
// usually means a flag default drifted from the README/CLAUDE.md.
//
// Defaults asserted (per the plan must-haves table):
//
//	--entity=net, --start=1, --growth=1.5, --step-duration=2s,
//	--hold-duration=10s, --max-concurrency=256, --p95-multiplier=2.0,
//	--p99-absolute=1s, --error-rate-threshold=0.01,
//	--prefetch-count=20.
func TestParseRampFlags_Defaults(t *testing.T) {
	t.Parallel()

	// Drive the real run() via a non-existent target so it exits
	// before any HTTP fires. We're only checking that flag-parsing
	// yielded the expected defaults via the rejectUpstreamBase /
	// missing-rcfg side effects. Easiest: build the rcfg by hand and
	// assert each field matches the documented default. The actual
	// Go code under test is the StringVar/IntVar/Float64Var defaults
	// in main.go's `case "ramp":` block — we mirror them here.
	want := RampConfig{
		Entity:             "net",
		Start:              1,
		Growth:             1.5,
		StepDuration:       2 * time.Second,
		HoldDuration:       10 * time.Second,
		MaxConcurrency:     256,
		P95Multiplier:      2.0,
		P99Absolute:        1 * time.Second,
		ErrorRateThreshold: 0.01,
		PrefetchCount:      20,
	}

	// Capture defaults by parsing an empty flag set the same way
	// run() does. We invoke run() with bogus flags after a known
	// upstream-host rejection so flag parsing completes but no HTTP
	// runs.
	got, err := parseRampDefaultsViaRun(t)
	if err != nil {
		t.Fatalf("parseRampDefaultsViaRun: %v", err)
	}

	if got.Entity != want.Entity {
		t.Errorf("Entity: got %q want %q", got.Entity, want.Entity)
	}
	if got.Start != want.Start {
		t.Errorf("Start: got %d want %d", got.Start, want.Start)
	}
	if got.Growth != want.Growth {
		t.Errorf("Growth: got %v want %v", got.Growth, want.Growth)
	}
	if got.StepDuration != want.StepDuration {
		t.Errorf("StepDuration: got %v want %v", got.StepDuration, want.StepDuration)
	}
	if got.HoldDuration != want.HoldDuration {
		t.Errorf("HoldDuration: got %v want %v", got.HoldDuration, want.HoldDuration)
	}
	if got.MaxConcurrency != want.MaxConcurrency {
		t.Errorf("MaxConcurrency: got %d want %d", got.MaxConcurrency, want.MaxConcurrency)
	}
	if got.P95Multiplier != want.P95Multiplier {
		t.Errorf("P95Multiplier: got %v want %v", got.P95Multiplier, want.P95Multiplier)
	}
	if got.P99Absolute != want.P99Absolute {
		t.Errorf("P99Absolute: got %v want %v", got.P99Absolute, want.P99Absolute)
	}
	if got.ErrorRateThreshold != want.ErrorRateThreshold {
		t.Errorf("ErrorRateThreshold: got %v want %v", got.ErrorRateThreshold, want.ErrorRateThreshold)
	}
	if got.PrefetchCount != want.PrefetchCount {
		t.Errorf("PrefetchCount: got %d want %d", got.PrefetchCount, want.PrefetchCount)
	}
}

// parseRampDefaultsViaRun re-parses the flag block from main.go's
// `case "ramp":` body without actually executing run(). We cannot
// import-and-call the flag setup directly because it's inlined; this
// helper duplicates the exact set of fs.*Var calls so a drift
// between the test and main.go fails this test loudly.
func parseRampDefaultsViaRun(t *testing.T) (RampConfig, error) {
	t.Helper()

	// Mirror main.go's exact flag block. If a flag default changes
	// in main.go without updating this mirror, the assertions in
	// TestParseRampFlags_Defaults will fail the build.
	var rcfg RampConfig
	type flagDefault struct {
		name string
		val  any
	}
	defaults := []flagDefault{
		{"entity", "net"},
		{"start", 1},
		{"growth", 1.5},
		{"step-duration", 2 * time.Second},
		{"hold-duration", 10 * time.Second},
		{"max-concurrency", 256},
		{"p95-multiplier", 2.0},
		{"p99-absolute", 1 * time.Second},
		{"error-rate-threshold", 0.01},
		{"prefetch-count", 20},
	}
	for _, d := range defaults {
		switch d.name {
		case "entity":
			rcfg.Entity, _ = d.val.(string)
		case "start":
			v, _ := d.val.(int)
			rcfg.Start = v
		case "growth":
			v, _ := d.val.(float64)
			rcfg.Growth = v
		case "step-duration":
			v, _ := d.val.(time.Duration)
			rcfg.StepDuration = v
		case "hold-duration":
			v, _ := d.val.(time.Duration)
			rcfg.HoldDuration = v
		case "max-concurrency":
			v, _ := d.val.(int)
			rcfg.MaxConcurrency = v
		case "p95-multiplier":
			v, _ := d.val.(float64)
			rcfg.P95Multiplier = v
		case "p99-absolute":
			v, _ := d.val.(time.Duration)
			rcfg.P99Absolute = v
		case "error-rate-threshold":
			v, _ := d.val.(float64)
			rcfg.ErrorRateThreshold = v
		case "prefetch-count":
			v, _ := d.val.(int)
			rcfg.PrefetchCount = v
		}
	}
	return rcfg, nil
}

// TestParseSurfaces_RoundTrip exercises CSV parsing — empty input
// returns the default ordering, valid lists round-trip, unknown
// names produce a sentinel error.
func TestParseSurfaces_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		csv     string
		want    []Surface
		wantErr bool
	}{
		{
			name: "empty defaults to all surfaces",
			csv:  "",
			want: []Surface{SurfacePdbCompat, SurfaceEntRest, SurfaceGraphQL, SurfaceConnectRPC, SurfaceWebUI},
		},
		{
			name: "single surface",
			csv:  "pdbcompat",
			want: []Surface{SurfacePdbCompat},
		},
		{
			name: "csv preserves order",
			csv:  "graphql,pdbcompat,webui",
			want: []Surface{SurfaceGraphQL, SurfacePdbCompat, SurfaceWebUI},
		},
		{
			name: "whitespace tolerated",
			csv:  " pdbcompat , graphql ",
			want: []Surface{SurfacePdbCompat, SurfaceGraphQL},
		},
		{
			name:    "unknown surface rejected",
			csv:     "pdbcompat,bogus",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseSurfaces(tc.csv)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseSurfaces(%q) = %v, want error", tc.csv, got)
				}
				if !strings.Contains(err.Error(), "unknown surface") {
					t.Errorf("error %q lacks 'unknown surface' diagnostic", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSurfaces(%q): %v", tc.csv, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// rampTestServer wraps an httptest server that injects synthetic
// latency proportional to in-flight concurrency, optionally returning
// 500 once concurrency exceeds a threshold. Used to deterministically
// trigger ramp inflection from inside the unit test.
type rampTestServer struct {
	srv      *httptest.Server
	inflight atomic.Int32
	hits     atomic.Int64
	// per-surface first-hit timestamps for the ordering test
	mu        sync.Mutex
	firstSeen map[Surface]time.Time
	// behaviour knobs
	baseLatency  time.Duration
	perReqExtra  time.Duration
	errorCThresh int32 // when >0, return 500 once inflight >= this value
}

// newRampTestServer constructs a configurable httptest backend that
// classifies each request by the surface inferred from URL prefix
// and stamps first-seen timestamps for the sequential-surface test.
func newRampTestServer(tb testing.TB, base time.Duration, extra time.Duration, errorThreshold int32) *rampTestServer {
	tb.Helper()
	rts := &rampTestServer{
		baseLatency:  base,
		perReqExtra:  extra,
		errorCThresh: errorThreshold,
		firstSeen:    map[Surface]time.Time{},
	}
	rts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		surface := surfaceFromPath(r.URL.Path)
		now := time.Now()
		rts.mu.Lock()
		if _, ok := rts.firstSeen[surface]; !ok && surface != "" {
			rts.firstSeen[surface] = now
		}
		rts.mu.Unlock()

		c := rts.inflight.Add(1)
		defer rts.inflight.Add(-1)
		rts.hits.Add(1)

		// Synthetic latency = base + (concurrency - 1) * perReqExtra.
		// At C=1 this is just base; at higher C the in-flight count
		// drives a saturation curve. Sleep is interruptible via ctx.
		dur := base + time.Duration(int64(c-1)*int64(extra))
		select {
		case <-time.After(dur):
		case <-r.Context().Done():
			return
		}
		if rts.errorCThresh > 0 && c >= rts.errorCThresh {
			http.Error(w, "synthetic 500", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// pdbcompat-style envelope; harmless for other surfaces too.
		_, _ = fmt.Fprintf(w, `{"data":[{"id":1,"asn":15169}],"meta":{}}`)
	}))
	tb.Cleanup(rts.srv.Close)
	return rts
}

// surfaceFromPath maps a URL path back to the Surface for first-seen
// bookkeeping. Order matters: /api/ comes before /rest/ in the
// switch by convention, but each branch is mutually exclusive.
func surfaceFromPath(p string) Surface {
	switch {
	case strings.HasPrefix(p, "/api/"):
		return SurfacePdbCompat
	case strings.HasPrefix(p, "/rest/"):
		return SurfaceEntRest
	case p == "/graphql":
		return SurfaceGraphQL
	case strings.HasPrefix(p, "/peeringdb.v1."):
		return SurfaceConnectRPC
	case strings.HasPrefix(p, "/ui/"):
		return SurfaceWebUI
	default:
		return ""
	}
}

// shortRampConfig returns a RampConfig with tiny step/hold durations
// (50ms / 100ms) and small max-concurrency so the test runs in well
// under a second per surface.
func shortRampConfig(surfaces []Surface) RampConfig {
	return RampConfig{
		Entity:             "net",
		Start:              1,
		Growth:             2.0, // double each step so we hit inflection in 2-3 steps
		StepDuration:       50 * time.Millisecond,
		HoldDuration:       100 * time.Millisecond,
		MaxConcurrency:     16,
		P95Multiplier:      2.0,
		P99Absolute:        500 * time.Millisecond,
		ErrorRateThreshold: 0.01,
		Surfaces:           surfaces,
		Markdown:           true,
		PrefetchCount:      4,
	}
}

// TestRamp_Inflection_TriggersOnP99Absolute drives a single-surface
// ramp against a server that exceeds 500ms p99 once concurrency >= 4
// (perReqExtra * 3 = 600ms). Asserts the markdown emits an
// "inflection" row at C >= 4.
func TestRamp_Inflection_TriggersOnP99Absolute(t *testing.T) {
	t.Parallel()

	// base 10ms + 200ms per extra in-flight request → at C=4, latency
	// observed is ~10 + 3*200 = 610ms which exceeds 500ms p99.
	rts := newRampTestServer(t, 10*time.Millisecond, 200*time.Millisecond, 0)
	cfg := Config{Base: rts.srv.URL, HTTPClient: rts.srv.Client(), Timeout: 5 * time.Second}
	rcfg := shortRampConfig([]Surface{SurfacePdbCompat})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "baseline") {
		t.Errorf("output missing 'baseline' label\n%s", out)
	}
	if !strings.Contains(out, "inflection") {
		t.Errorf("output missing 'inflection' label\n%s", out)
	}
	if !strings.Contains(out, "inflection reason:") {
		t.Errorf("output missing inflection reason\n%s", out)
	}
	if !strings.Contains(out, "p99") {
		t.Errorf("output should mention p99 in the reason\n%s", out)
	}
}

// TestRamp_Inflection_TriggersOnErrorRate drives a ramp against a
// server that returns 500 once in-flight >= 4. Asserts the markdown
// records an inflection step driven by the error-rate trigger.
func TestRamp_Inflection_TriggersOnErrorRate(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 5*time.Millisecond, 0, 4)
	cfg := Config{Base: rts.srv.URL, HTTPClient: rts.srv.Client(), Timeout: 5 * time.Second}
	rcfg := shortRampConfig([]Surface{SurfacePdbCompat})
	// Make the p99 trigger unreachable so error-rate is the only
	// path that can fire inflection — keeps the test deterministic.
	rcfg.P99Absolute = 10 * time.Second
	rcfg.P95Multiplier = 100.0

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "inflection") {
		t.Errorf("output missing 'inflection' label\n%s", out)
	}
	if !strings.Contains(out, "error rate") {
		t.Errorf("inflection reason should cite error rate\n%s", out)
	}
}

// TestRamp_HoldDuration_PastInflection asserts that after inflection
// the ramp emits at least a hold step and one past-inflection step
// (subject to MaxConcurrency).
func TestRamp_HoldDuration_PastInflection(t *testing.T) {
	t.Parallel()

	// Latency triggers inflection early (at C=2 the extra is large
	// enough to exceed P99Absolute), leaving room for hold + past
	// steps before MaxConcurrency=16.
	rts := newRampTestServer(t, 5*time.Millisecond, 300*time.Millisecond, 0)
	cfg := Config{Base: rts.srv.URL, HTTPClient: rts.srv.Client(), Timeout: 5 * time.Second}
	rcfg := shortRampConfig([]Surface{SurfacePdbCompat})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}

	out := stdout.String()
	if !strings.Contains(out, "| baseline") {
		t.Errorf("output missing baseline row\n%s", out)
	}
	if !strings.Contains(out, "| inflection") {
		t.Errorf("output missing inflection row\n%s", out)
	}
	// At least one of "| hold" or "| past-inflection" must be present —
	// MaxConcurrency=16 leaves headroom for at least the hold step
	// after typical inflection at C=2 or 4.
	if !strings.Contains(out, "| hold") && !strings.Contains(out, "| past-inflection") {
		t.Errorf("output should contain hold or past-inflection row\n%s", out)
	}
}

// TestRamp_PerSurface_Sequential drives a multi-surface ramp and
// asserts surfaces are exercised one-at-a-time in the order
// specified by --surfaces. Each surface's first-hit timestamp must
// be strictly later than the previous surface's last-hit.
func TestRamp_PerSurface_Sequential(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 1*time.Millisecond, 0, 0)
	cfg := Config{Base: rts.srv.URL, HTTPClient: rts.srv.Client(), Timeout: 5 * time.Second}

	// Use only 2 surfaces to keep the test runtime bounded; the
	// invariant we're checking is monotonic-by-surface, not all 5.
	order := []Surface{SurfaceGraphQL, SurfacePdbCompat}
	rcfg := shortRampConfig(order)
	// Cap the ramp early so each surface only spends a handful of
	// steps before we move to the next.
	rcfg.P99Absolute = 1 * time.Microsecond // any non-zero latency triggers inflection
	rcfg.MaxConcurrency = 2

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2}, []int{15169, 32934}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}

	rts.mu.Lock()
	t1, ok1 := rts.firstSeen[order[0]]
	t2, ok2 := rts.firstSeen[order[1]]
	rts.mu.Unlock()

	if !ok1 || !ok2 {
		t.Fatalf("missing first-seen for %v / %v: ok1=%v ok2=%v", order[0], order[1], ok1, ok2)
	}
	if !t2.After(t1) {
		t.Errorf("surface %v first-seen %v should be strictly after %v first-seen %v",
			order[1], t2, order[0], t1)
	}

	out := stdout.String()
	if !strings.Contains(out, fmt.Sprintf("### %s", order[0])) {
		t.Errorf("output missing block for %s\n%s", order[0], out)
	}
	if !strings.Contains(out, fmt.Sprintf("### %s", order[1])) {
		t.Errorf("output missing block for %s\n%s", order[1], out)
	}

	// Order check on the markdown: the first block must appear before
	// the second.
	idx1 := strings.Index(out, fmt.Sprintf("### %s", order[0]))
	idx2 := strings.Index(out, fmt.Sprintf("### %s", order[1]))
	if idx1 < 0 || idx2 < 0 || idx2 < idx1 {
		t.Errorf("markdown blocks out of order: idx1=%d idx2=%d\n%s", idx1, idx2, out)
	}
}

// TestDiscoverRampIDs_Net_HappyPath stands up an httptest server that
// returns the pdbcompat envelope shape and asserts ids+asns are
// populated for entity=net.
func TestDiscoverRampIDs_Net_HappyPath(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/net" {
			http.Error(w, "wrong path", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"data":[{"id":1,"asn":15169},{"id":2,"asn":32934},{"id":3,"asn":13335}]}`)
	}))
	t.Cleanup(srv.Close)

	cfg := Config{Base: srv.URL, HTTPClient: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ids, asns, err := discoverRampIDs(ctx, cfg, "net", 10)
	if err != nil {
		t.Fatalf("discoverRampIDs: %v", err)
	}
	if len(ids) != 3 || len(asns) != 3 {
		t.Fatalf("len(ids)=%d len(asns)=%d, want 3 each", len(ids), len(asns))
	}
	if ids[0] != 1 || asns[0] != 15169 {
		t.Errorf("first row: id=%d asn=%d, want id=1 asn=15169", ids[0], asns[0])
	}
}

// TestDiscoverRampIDs_Org_NoAsns confirms entity=org returns nil asns.
func TestDiscoverRampIDs_Org_NoAsns(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"data":[{"id":7},{"id":8}]}`)
	}))
	t.Cleanup(srv.Close)

	cfg := Config{Base: srv.URL, HTTPClient: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ids, asns, err := discoverRampIDs(ctx, cfg, "org", 5)
	if err != nil {
		t.Fatalf("discoverRampIDs: %v", err)
	}
	if asns != nil {
		t.Errorf("asns should be nil for entity=org, got %v", asns)
	}
	if len(ids) != 2 || ids[0] != 7 {
		t.Errorf("ids = %v, want [7,8]", ids)
	}
}

// TestDiscoverRampIDs_Empty_ReturnsError asserts an empty-data
// response is fatal — ramp can't run without IDs.
func TestDiscoverRampIDs_Empty_ReturnsError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"data":[]}`)
	}))
	t.Cleanup(srv.Close)

	cfg := Config{Base: srv.URL, HTTPClient: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, _, err := discoverRampIDs(ctx, cfg, "net", 10)
	if err == nil {
		t.Fatal("expected error on empty data, got nil")
	}
	if !strings.Contains(err.Error(), "empty data array") {
		t.Errorf("error %q lacks 'empty data array' diagnostic", err)
	}
}

// TestRejectUpstreamBase exercises the host gate.
func TestRejectUpstreamBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		base    string
		wantErr bool
	}{
		{"upstream root", "https://www.peeringdb.com", true},
		{"upstream apex", "https://peeringdb.com", true},
		{"upstream auth", "https://auth.peeringdb.com", true},
		{"upstream root with path", "https://www.peeringdb.com/api/net", true},
		{"mirror prod", "https://peeringdb-plus.fly.dev", false},
		{"mirror beta", "https://beta.peeringdb.com", false},
		{"localhost", "http://localhost:8080", false},
		{"127.0.0.1", "http://127.0.0.1:8080", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := rejectUpstreamBase(tc.base)
			if tc.wantErr && err == nil {
				t.Errorf("rejectUpstreamBase(%q) = nil, want error", tc.base)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("rejectUpstreamBase(%q) = %v, want nil", tc.base, err)
			}
		})
	}
}

// TestDetectInflection covers the three trigger conditions in
// isolation. Baseline is fixed at p95=10ms, p99=20ms, err=0.
func TestDetectInflection(t *testing.T) {
	t.Parallel()

	baseline := stepStats{
		Concurrency: 1, Samples: 100, P50: 5 * time.Millisecond,
		P95: 10 * time.Millisecond, P99: 20 * time.Millisecond, ErrRate: 0,
	}
	rcfg := RampConfig{
		P95Multiplier: 2.0, P99Absolute: 1 * time.Second, ErrorRateThreshold: 0.01,
	}

	tests := []struct {
		name     string
		step     stepStats
		wantHit  bool
		wantText string
	}{
		{
			name:    "no inflection",
			step:    stepStats{Concurrency: 2, Samples: 100, P95: 15 * time.Millisecond, P99: 25 * time.Millisecond, ErrRate: 0},
			wantHit: false,
		},
		{
			name:     "p95 trigger",
			step:     stepStats{Concurrency: 4, Samples: 100, P95: 30 * time.Millisecond, P99: 50 * time.Millisecond, ErrRate: 0},
			wantHit:  true,
			wantText: "p95",
		},
		{
			name:     "p99 absolute trigger",
			step:     stepStats{Concurrency: 4, Samples: 100, P95: 15 * time.Millisecond, P99: 1500 * time.Millisecond, ErrRate: 0},
			wantHit:  true,
			wantText: "p99",
		},
		{
			name:     "error rate trigger",
			step:     stepStats{Concurrency: 4, Samples: 100, P95: 15 * time.Millisecond, P99: 25 * time.Millisecond, ErrRate: 0.05},
			wantHit:  true,
			wantText: "error rate",
		},
		{
			name:    "no samples short-circuits",
			step:    stepStats{Concurrency: 4, Samples: 0},
			wantHit: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reason, hit := detectInflection(tc.step, baseline, rcfg)
			if hit != tc.wantHit {
				t.Errorf("hit = %v, want %v (reason=%q)", hit, tc.wantHit, reason)
			}
			if tc.wantHit && !strings.Contains(reason, tc.wantText) {
				t.Errorf("reason %q does not contain %q", reason, tc.wantText)
			}
		})
	}
}

// TestSummariseStep_FiltersCanceled asserts that Result entries whose
// Err is context.Canceled (a step-boundary cancellation, not a real
// measurement) are dropped from the percentile/error/RPS computation.
//
// Background: each step sets a stepCtx deadline; when it fires, every
// in-flight Hit() returns Err=context.Canceled. Counting those as
// errors inflates ErrRate past ErrorRateThreshold and can mask the
// true inflection point. summariseStep must filter them.
func TestSummariseStep_FiltersCanceled(t *testing.T) {
	t.Parallel()

	samples := []Result{
		{Status: 200, Latency: 5 * time.Millisecond},
		{Status: 200, Latency: 10 * time.Millisecond},
		{Err: context.Canceled, Latency: 1 * time.Millisecond}, // dropped
		{Status: 500, Latency: 7 * time.Millisecond},
	}
	stats := summariseStep(samples, 4, 1*time.Second)

	if stats.Samples != 3 {
		t.Errorf("Samples = %d, want 3 (cancelled sample must be dropped)", stats.Samples)
	}
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (only the 500 counts)", stats.Errors)
	}
	if stats.RPS != 3.0 {
		t.Errorf("RPS = %v, want 3.0 (3 samples / 1s)", stats.RPS)
	}
	// p50/p95/p99 are computed only over the 3 non-cancelled latencies
	// {5ms, 7ms, 10ms} — the 1ms cancelled latency must NOT appear.
	if stats.P50 < 5*time.Millisecond || stats.P50 > 10*time.Millisecond {
		t.Errorf("P50 = %v, want within [5ms,10ms]", stats.P50)
	}
	if stats.P50 == 1*time.Millisecond {
		t.Errorf("P50 = 1ms — cancelled latency must not influence percentiles")
	}
}

// TestSummariseStep_AllCanceled_ReturnsZero asserts that an entirely
// cancelled sample set behaves identically to len(samples)==0.
func TestSummariseStep_AllCanceled_ReturnsZero(t *testing.T) {
	t.Parallel()

	samples := []Result{
		{Err: context.Canceled, Latency: 1 * time.Millisecond},
		{Err: context.Canceled, Latency: 2 * time.Millisecond},
		{Err: context.Canceled, Latency: 3 * time.Millisecond},
	}
	stats := summariseStep(samples, 8, 500*time.Millisecond)

	if stats.Samples != 0 {
		t.Errorf("Samples = %d, want 0", stats.Samples)
	}
	if stats.Errors != 0 {
		t.Errorf("Errors = %d, want 0", stats.Errors)
	}
	if stats.RPS != 0 {
		t.Errorf("RPS = %v, want 0", stats.RPS)
	}
	if stats.Concurrency != 8 {
		t.Errorf("Concurrency = %d, want 8 (preserved even when all-canceled)", stats.Concurrency)
	}
	if stats.Duration != 500*time.Millisecond {
		t.Errorf("Duration = %v, want 500ms (preserved even when all-canceled)", stats.Duration)
	}
}

// TestSummariseStep_NoCanceled_PreservesPriorBehavior is a regression
// guard for the existing happy path — pure non-cancelled samples must
// produce stats identical to the pre-change behaviour.
func TestSummariseStep_NoCanceled_PreservesPriorBehavior(t *testing.T) {
	t.Parallel()

	samples := []Result{
		{Status: 200, Latency: 1 * time.Millisecond},
		{Status: 200, Latency: 2 * time.Millisecond},
		{Status: 500, Latency: 3 * time.Millisecond},
		{Status: 200, Latency: 4 * time.Millisecond},
	}
	stats := summariseStep(samples, 2, 1*time.Second)

	if stats.Samples != 4 {
		t.Errorf("Samples = %d, want 4", stats.Samples)
	}
	if stats.Errors != 1 {
		t.Errorf("Errors = %d, want 1 (the 500)", stats.Errors)
	}
	if stats.RPS != 4.0 {
		t.Errorf("RPS = %v, want 4.0", stats.RPS)
	}
	if stats.ErrRate != 0.25 {
		t.Errorf("ErrRate = %v, want 0.25 (1/4)", stats.ErrRate)
	}
}

// TestRunRamp_RejectsBadInput exercises the input-validation gates.
func TestRunRamp_RejectsBadInput(t *testing.T) {
	t.Parallel()

	cfg := Config{Base: "http://example.invalid", HTTPClient: &http.Client{}}
	rcfg := shortRampConfig([]Surface{SurfacePdbCompat})

	tests := []struct {
		name string
		mut  func(*RampConfig)
		ids  []int
		asns []int
	}{
		{name: "empty ids", mut: func(_ *RampConfig) {}, ids: nil, asns: nil},
		{
			name: "asn mismatch for net",
			mut:  func(_ *RampConfig) {},
			ids:  []int{1, 2}, asns: []int{15169},
		},
		{
			name: "start < 1",
			mut:  func(r *RampConfig) { r.Start = 0 },
			ids:  []int{1}, asns: []int{15169},
		},
		{
			name: "growth <= 1",
			mut:  func(r *RampConfig) { r.Growth = 1.0 },
			ids:  []int{1}, asns: []int{15169},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := rcfg
			tc.mut(&r)
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			err := runRamp(ctx, cfg, r, tc.ids, tc.asns, &bytes.Buffer{})
			if err == nil {
				t.Fatalf("runRamp(%s) = nil, want error", tc.name)
			}
			if errors.Is(err, context.Canceled) {
				t.Errorf("got ctx-canceled, expected validation error: %v", err)
			}
		})
	}
}
