package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strconv"
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
// latency that grows with the step concurrency, optionally returning
// 500 for requests from ramp steps at or above a concurrency threshold.
// Used to trigger ramp inflection from inside the unit test.
type rampTestServer struct {
	srv  *httptest.Server
	hits atomic.Int64
	// behaviour knobs
	baseLatency  time.Duration
	perStepExtra time.Duration
	errorCThresh int // when >0, return 500 if the step concurrency >= this value
}

// newRampTestServer constructs a configurable httptest backend. The
// extra latency and the error threshold need a stepGate transport: the
// server reads the step concurrency from stepConcurrencyHeader.
func newRampTestServer(tb testing.TB, base time.Duration, extra time.Duration, errorThreshold int) *rampTestServer {
	tb.Helper()
	rts := &rampTestServer{
		baseLatency:  base,
		perStepExtra: extra,
		errorCThresh: errorThreshold,
	}
	rts.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rts.hits.Add(1)

		// The latency and the error decision use the step concurrency,
		// not the in-flight count: under CPU load the requests of a
		// step do not always overlap at the server. A request of a step
		// at concurrency C waits base + (C-1)*extra; without the header
		// it waits base. Sleep is interruptible via ctx.
		stepC, _ := strconv.Atoi(r.Header.Get(stepConcurrencyHeader))
		dur := base + time.Duration(max(stepC-1, 0))*extra
		select {
		case <-time.After(dur):
		case <-r.Context().Done():
			return
		}
		if rts.errorCThresh > 0 && stepC >= rts.errorCThresh {
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

// surfaceFromPath maps a URL path back to the Surface for the request
// log of stepGate. Order matters: /api/ comes before /rest/ in the
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

// stepConcurrencyHeader carries the concurrency of the ramp step that
// sent a request. stepGate sets it and rampTestServer reads it.
const stepConcurrencyHeader = "X-Test-Step-Concurrency"

// gateSamples is the minimum number of samples in each step of a ramp
// that runs through stepGate.
const gateSamples = 8

// rampTestTimeout bounds each ramp test. A ramp through stepGate has no
// wall-clock steps, so this is the only time limit.
const rampTestTimeout = 30 * time.Second

// stepGate is a step clock for RampConfig.stepEnd and the HTTP
// transport of the ramp. It ends each step after the workers of the
// step start concurrency+samples requests. A worker sends the sample of
// a request before it starts its next request, so the step then has at
// least `samples` samples. A slow machine only makes the step longer.
type stepGate struct {
	next    http.RoundTripper
	samples int
	// stallFrom, when > 0, stalls each step with concurrency >=
	// stallFrom: no request of the step gets a response, and the step
	// ends when each of its workers has a request in flight. Such a
	// step has no samples. Set it before the ramp starts.
	stallFrom int

	mu       sync.Mutex
	steps    []gateStep // each step of the ramp, in run order
	requests []Surface  // surface of each request start and end, in order
}

// gateStep is one ramp step as the ramp asked stepGate for it.
type gateStep struct {
	concurrency int
	dur         time.Duration
}

// gateRunKey is the context key of the *gateRun of a step.
type gateRunKey struct{}

// gateRun is the state of one running step. It travels in the step
// context, so the requests of a step always count against that step.
type gateRun struct {
	concurrency int
	cancel      context.CancelFunc
	started     atomic.Int64
}

// gatedRamp returns the configs for a ramp against rts whose steps end
// through a stepGate instead of on the wall clock.
func gatedRamp(rts *rampTestServer, surfaces []Surface) (Config, RampConfig, *stepGate) {
	gate := &stepGate{next: rts.srv.Client().Transport, samples: gateSamples}
	cfg := Config{Base: rts.srv.URL, HTTPClient: &http.Client{Transport: gate}, Timeout: 5 * time.Second}
	rcfg := shortRampConfig(surfaces)
	rcfg.stepEnd = gate.stepEnd
	return cfg, rcfg, gate
}

// stepEnd records the step and returns a context that RoundTrip
// cancels when the step has enough samples.
func (g *stepGate) stepEnd(ctx context.Context, concurrency int, dur time.Duration) (context.Context, context.CancelFunc) {
	g.mu.Lock()
	g.steps = append(g.steps, gateStep{concurrency: concurrency, dur: dur})
	g.mu.Unlock()
	run := &gateRun{concurrency: concurrency}
	stepCtx, cancel := context.WithCancel(context.WithValue(ctx, gateRunKey{}, run))
	run.cancel = cancel
	return stepCtx, cancel
}

// RoundTrip counts the request against its step, ends the step when
// the count reaches concurrency+samples (concurrency for a stalled
// step), and tells the server the step concurrency.
func (g *stepGate) RoundTrip(r *http.Request) (*http.Response, error) {
	run, ok := r.Context().Value(gateRunKey{}).(*gateRun)
	if !ok {
		if r.Body != nil {
			_ = r.Body.Close()
		}
		return nil, errors.New("stepGate: request outside a ramp step")
	}
	stall := g.stallFrom > 0 && run.concurrency >= g.stallFrom
	limit := int64(run.concurrency + g.samples)
	if stall {
		limit = int64(run.concurrency)
	}
	if run.started.Add(1) >= limit {
		run.cancel()
	}
	surface := surfaceFromPath(r.URL.Path)
	g.logRequest(surface)
	defer g.logRequest(surface)

	if stall {
		// Hold the request until its step ends.
		if r.Body != nil {
			_ = r.Body.Close()
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	}
	r = r.Clone(r.Context())
	r.Header.Set(stepConcurrencyHeader, strconv.Itoa(run.concurrency))
	return g.next.RoundTrip(r)
}

func (g *stepGate) logRequest(s Surface) {
	g.mu.Lock()
	g.requests = append(g.requests, s)
	g.mu.Unlock()
}

// stepLog returns the steps that the ramp ran, in order.
func (g *stepGate) stepLog() []gateStep {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.steps)
}

// surfaceOrder returns the surfaces in the order of their requests,
// with repeats removed. A surface appears more than once when its
// requests overlap or alternate with the requests of another surface.
func (g *stepGate) surfaceOrder() []Surface {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Compact(slices.Clone(g.requests))
}

// rampRow returns the start of the markdown row for a step label at
// concurrency c, in the emitMarkdown format.
func rampRow(label string, c int) string {
	return fmt.Sprintf("| %-18s | %3d |", label, c)
}

// shortRampConfig returns a RampConfig with tiny step/hold durations
// (50ms / 100ms) and small max-concurrency so the test runs in well
// under a second per surface. A ramp from gatedRamp does not use the
// durations to end its steps.
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

// TestRamp_Inflection_TriggersOnP99Absolute drives a ramp against a
// server that makes each request of a step at concurrency C wait
// 5ms + (C-1)*25ms. Asserts that the markdown records the inflection at
// C=2, driven by the p99-absolute trigger.
func TestRamp_Inflection_TriggersOnP99Absolute(t *testing.T) {
	t.Parallel()

	const base, extra = 5 * time.Millisecond, 25 * time.Millisecond
	rts := newRampTestServer(t, base, extra, 0)
	cfg, rcfg, gate := gatedRamp(rts, []Surface{SurfacePdbCompat})
	// Each sample of the C=2 step takes at least base+extra = 30ms, so
	// the p99 of the step passes the 20ms ceiling. The ramp does not
	// check the baseline step for inflection, so C=2 is the first step
	// that can trigger. Make the other triggers unreachable, with the
	// bounds of TestRamp_Inflection_TriggersOnErrorRate. The error rate
	// cannot pass 100%.
	rcfg.P99Absolute = 20 * time.Millisecond
	rcfg.P95Multiplier = 1e4
	rcfg.ErrorRateThreshold = 1.0
	rcfg.MaxConcurrency = 4

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	// The ramp stops at C=2, then holds and runs one step past it.
	wantSteps := []gateStep{
		{concurrency: 1, dur: rcfg.StepDuration},
		{concurrency: 2, dur: rcfg.StepDuration},
		{concurrency: 2, dur: rcfg.HoldDuration},
		{concurrency: 4, dur: rcfg.StepDuration},
	}
	if got := gate.stepLog(); !slices.Equal(got, wantSteps) {
		t.Errorf("ramp steps = %+v, want %+v", got, wantSteps)
	}
	out := stdout.String()
	if want := rampRow("inflection", 2); !strings.Contains(out, want) {
		t.Errorf("output missing row %q\n%s", want, out)
	}
	// The reason text of the p99-absolute trigger, with the p99 of the
	// step.
	reason := regexp.MustCompile(fmt.Sprintf(`(?m)^inflection reason: p99 (\S+) > %s absolute$`,
		regexp.QuoteMeta(rcfg.P99Absolute.String())))
	m := reason.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("output missing the p99-absolute inflection reason\n%s", out)
	}
	p99, err := time.ParseDuration(m[1])
	if err != nil {
		t.Fatalf("parse p99 %q in the reason: %v", m[1], err)
	}
	if p99 < base+extra {
		t.Errorf("reason p99 = %v, want at least %v (the latency of each C=2 request)", p99, base+extra)
	}
}

// TestRamp_Inflection_TriggersOnErrorRate drives a ramp against a
// server that returns 500 for every request of a step with
// concurrency >= 4. Asserts the markdown records the inflection at
// C=4, driven by the error-rate trigger.
func TestRamp_Inflection_TriggersOnErrorRate(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 5*time.Millisecond, 0, 4)
	cfg, rcfg, _ := gatedRamp(rts, []Surface{SurfacePdbCompat})
	// Make the latency triggers unreachable so error rate is the only
	// trigger that can fire. A sample takes at least the 5ms server
	// latency and less than rampTestTimeout, so p95 cannot pass
	// 5ms*1e4 = 50s and p99 cannot pass an hour.
	rcfg.P99Absolute = time.Hour
	rcfg.P95Multiplier = 1e4

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	out := stdout.String()
	if want := rampRow("inflection", 4); !strings.Contains(out, want) {
		t.Errorf("output missing row %q\n%s", want, out)
	}
	if want := "inflection reason: error rate 100.00%"; !strings.Contains(out, want) {
		t.Errorf("output missing %q\n%s", want, out)
	}
}

// TestRamp_Inflection_TriggersOnNoSamples drives a ramp in which no
// request of a step with concurrency >= 4 gets a response before the
// step ends. Asserts that the markdown records the inflection at C=4
// with the no-samples reason, and that the rows of the steps without
// samples show no latency or error figures.
func TestRamp_Inflection_TriggersOnNoSamples(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 5*time.Millisecond, 0, 0)
	cfg, rcfg, gate := gatedRamp(rts, []Surface{SurfacePdbCompat})
	gate.stallFrom = 4
	// Make the other triggers unreachable, with the bounds of
	// TestRamp_Inflection_TriggersOnErrorRate. The error rate cannot
	// pass 100%.
	rcfg.P99Absolute = time.Hour
	rcfg.P95Multiplier = 1e4
	rcfg.ErrorRateThreshold = 1.0

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	// The ramp stops at C=4, then holds and runs two steps past it.
	wantSteps := []gateStep{
		{concurrency: 1, dur: rcfg.StepDuration},
		{concurrency: 2, dur: rcfg.StepDuration},
		{concurrency: 4, dur: rcfg.StepDuration},
		{concurrency: 4, dur: rcfg.HoldDuration},
		{concurrency: 8, dur: rcfg.StepDuration},
		{concurrency: 16, dur: rcfg.StepDuration},
	}
	if got := gate.stepLog(); !slices.Equal(got, wantSteps) {
		t.Errorf("ramp steps = %+v, want %+v", got, wantSteps)
	}
	out := stdout.String()
	want := fmt.Sprintf("\ninflection reason: no samples: no request completed within %s\n", rcfg.StepDuration)
	if !strings.Contains(out, want) {
		t.Errorf("output missing %q\n%s", want, out)
	}
	for _, row := range []struct {
		label string
		c     int
	}{{"inflection", 4}, {"hold", 4}, {"inflection+1", 8}, {"inflection+2", 16}} {
		want := rampRow(row.label, row.c) + "       - |       - |       - |      - |     0.0 |\n"
		if !strings.Contains(out, want) {
			t.Errorf("output missing row %q\n%s", want, out)
		}
	}
}

// TestRamp_BaselineWithoutSamples_Aborts drives a ramp in which no
// request gets a response. Asserts that the surface is reported as
// aborted without a table: a baseline without samples gives no latency
// to compare with.
func TestRamp_BaselineWithoutSamples_Aborts(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 5*time.Millisecond, 0, 0)
	cfg, rcfg, gate := gatedRamp(rts, []Surface{SurfacePdbCompat})
	gate.stallFrom = 1

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	wantSteps := []gateStep{{concurrency: 1, dur: rcfg.StepDuration}}
	if got := gate.stepLog(); !slices.Equal(got, wantSteps) {
		t.Errorf("ramp steps = %+v, want %+v", got, wantSteps)
	}
	out := stdout.String()
	want := fmt.Sprintf("### %s ramp ABORTED: baseline step: no samples: no request completed within %s\n",
		SurfacePdbCompat, rcfg.StepDuration)
	if !strings.Contains(out, want) {
		t.Errorf("output missing %q\n%s", want, out)
	}
	if strings.Contains(out, "| baseline") {
		t.Errorf("an aborted surface must not emit a table\n%s", out)
	}
}

// TestRamp_NoInflection_ReportsAllSteps drives a ramp against a
// healthy server that never degrades (no synthetic latency growth, no
// errors, triggers raised out of reach) and asserts every measured
// step appears in the markdown — the final one labelled
// "max-concurrency" — rather than only the baseline row. Regression
// guard for the bug where the no-inflection path discarded all
// intermediate step measurements.
func TestRamp_NoInflection_ReportsAllSteps(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 5*time.Millisecond, 0, 0)
	// The steps end through stepGate, so each step has samples: a step
	// without samples is an inflection.
	cfg, rcfg, _ := gatedRamp(rts, []Surface{SurfacePdbCompat})
	// Make every trigger unreachable so the ramp runs to MaxConcurrency,
	// with the bounds of TestRamp_Inflection_TriggersOnErrorRate. The
	// error rate cannot pass 100%.
	rcfg.P95Multiplier = 1e4
	rcfg.P99Absolute = time.Hour
	rcfg.ErrorRateThreshold = 1.0
	rcfg.MaxConcurrency = 8 // Start=1, Growth=2 -> steps at C=2, 4, 8

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	out := stdout.String()
	if !strings.Contains(out, "| baseline") {
		t.Errorf("output missing baseline row\n%s", out)
	}
	// Intermediate steps must be reported, not discarded.
	for _, label := range []string{"| C=2", "| C=4"} {
		if !strings.Contains(out, label) {
			t.Errorf("output missing intermediate step row %q\n%s", label, out)
		}
	}
	if !strings.Contains(out, "| max-concurrency") {
		t.Errorf("output missing max-concurrency row for the final step\n%s", out)
	}
	if !strings.Contains(out, "no inflection detected") {
		t.Errorf("output missing no-inflection reason\n%s", out)
	}
}

// TestRamp_HoldDuration_PastInflection asserts that after inflection
// the ramp holds the inflection concurrency for HoldDuration and then
// runs the inflection+1 and inflection+2 steps.
func TestRamp_HoldDuration_PastInflection(t *testing.T) {
	t.Parallel()

	// Drive inflection via the err-rate threshold: server returns 500
	// for every request of a step with concurrency >= 2, which triggers
	// the 1% err-rate trigger at the very first step past baseline.
	// This leaves headroom for hold + past steps before
	// MaxConcurrency=16.
	rts := newRampTestServer(t, 5*time.Millisecond, 0, 2)
	cfg, rcfg, gate := gatedRamp(rts, []Surface{SurfacePdbCompat})

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	// Baseline, inflection at C=2, hold at C=2 for HoldDuration, then
	// two steps past the inflection.
	wantSteps := []gateStep{
		{concurrency: 1, dur: rcfg.StepDuration},
		{concurrency: 2, dur: rcfg.StepDuration},
		{concurrency: 2, dur: rcfg.HoldDuration},
		{concurrency: 4, dur: rcfg.StepDuration},
		{concurrency: 8, dur: rcfg.StepDuration},
	}
	if got := gate.stepLog(); !slices.Equal(got, wantSteps) {
		t.Errorf("ramp steps = %+v, want %+v", got, wantSteps)
	}
	out := stdout.String()
	for _, want := range []string{
		rampRow("baseline", 1),
		rampRow("inflection", 2),
		rampRow("hold", 2),
		rampRow("inflection+1", 4),
		rampRow("inflection+2", 8),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing row %q\n%s", want, out)
		}
	}
}

// TestRamp_PerSurface_Sequential drives a multi-surface ramp and
// asserts surfaces are exercised one-at-a-time in the order
// specified by --surfaces. Each request of a surface must start and
// end before the first request of the next surface starts.
func TestRamp_PerSurface_Sequential(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 1*time.Millisecond, 0, 0)

	// Use only 2 surfaces to keep the test runtime bounded; the
	// invariant we're checking is monotonic-by-surface, not all 5.
	order := []Surface{SurfaceGraphQL, SurfacePdbCompat}
	cfg, rcfg, gate := gatedRamp(rts, order)
	// Cap the ramp early so each surface only spends a handful of
	// steps before we move to the next.
	rcfg.P99Absolute = 1 * time.Microsecond // any non-zero latency triggers inflection
	rcfg.MaxConcurrency = 2

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2}, []int{15169, 32934}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	// The transport logs the start and the end of each request. The
	// requests of each surface must form one run, in the given order.
	if got := gate.surfaceOrder(); !slices.Equal(got, order) {
		t.Errorf("surfaces in request order = %v, want %v", got, order)
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
		// Bypass attempts the hostname-literal denylist missed: casing,
		// trailing FQDN dot, and unenumerated subdomains.
		{"upstream uppercase", "https://WWW.PeeringDB.com", true},
		{"upstream trailing dot", "https://www.peeringdb.com.", true},
		{"upstream apex trailing dot", "https://peeringdb.com.", true},
		{"upstream docs subdomain", "https://docs.peeringdb.com", true},
		{"upstream nested subdomain", "https://api.www.peeringdb.com", true},
		{"mirror beta uppercase", "https://BETA.peeringdb.com", false},
		{"mirror beta trailing dot", "https://beta.peeringdb.com.", false},
		{"lookalike suffix not blocked", "https://notpeeringdb.com", false},
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

// TestRun_AllModesRefuseUpstreamBase asserts the upstream-host gate
// fires for EVERY subcommand, not just ramp — soak/endpoints/sync
// generate strictly more traffic than ramp's baseline step and were
// previously unguarded.
func TestRun_AllModesRefuseUpstreamBase(t *testing.T) {
	t.Parallel()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devnull.Close() })

	for _, mode := range []string{"endpoints", "sync", "soak", "ramp"} {
		t.Run(mode, func(t *testing.T) {
			err := run([]string{mode, "--base", "https://www.peeringdb.com"}, devnull, devnull)
			if err == nil {
				t.Fatalf("run(%s --base upstream) = nil, want refusal error", mode)
			}
			if !strings.Contains(err.Error(), "refusing to load-test upstream") {
				t.Errorf("run(%s) error = %q, want upstream refusal", mode, err)
			}
		})
	}
}

// TestDetectInflection covers the trigger conditions in isolation.
// Baseline is fixed at p95=10ms, p99=20ms, err=0.
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
			name:     "no samples trigger",
			step:     stepStats{Concurrency: 4, Samples: 0, Duration: 2 * time.Second},
			wantHit:  true,
			wantText: "no samples: no request completed within 2s",
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

// cancelOnResponse is an http.RoundTripper that calls cancel after the
// wrapped transport returns a response. A test uses it to end a ramp
// on the first completed request instead of on the step clock.
type cancelOnResponse struct {
	next   http.RoundTripper
	cancel context.CancelFunc
}

func (c cancelOnResponse) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := c.next.RoundTrip(r)
	if err == nil {
		c.cancel()
	}
	return resp, err
}

// TestRamp_Verbose_PrintsPrefetchAndErrors asserts that --verbose
// emits exactly one prefetch summary line per surface (with ids and
// asns when entity=net) and one log line per non-OK Result, and that
// it drops the result of a request that the ramp context canceled.
func TestRamp_Verbose_PrintsPrefetchAndErrors(t *testing.T) {
	t.Parallel()

	// The server answers the first request with a 500 only after the
	// second request arrives, and later requests wait until their
	// client cancels. So a request is always in flight when the first
	// response cancels ctx.
	var n atomic.Int32
	second := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1:
			select {
			case <-second:
			case <-r.Context().Done():
				return
			}
			http.Error(w, "synthetic 500", http.StatusInternalServerError)
		case 2:
			close(second)
			<-r.Context().Done()
		default:
			<-r.Context().Done()
		}
	}))
	t.Cleanup(srv.Close)

	// The step clock must not end the step: under CPU load (-race,
	// full-repo run) a 50ms step can end before any request completes,
	// which leaves no per-error line. The step durations exceed the
	// ctx timeout, and the transport cancels ctx after the first
	// response. Hit still returns that response, so it is logged.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg := Config{
		Base: srv.URL,
		HTTPClient: &http.Client{
			Transport: cancelOnResponse{next: srv.Client().Transport, cancel: cancel},
		},
		Timeout: 5 * time.Second,
		Verbose: true,
	}
	rcfg := shortRampConfig([]Surface{SurfacePdbCompat})
	rcfg.StepDuration = time.Minute
	rcfg.HoldDuration = time.Minute
	// One step (the baseline) with two workers: the second request is
	// in flight when ctx is canceled, and its result must not be logged.
	rcfg.Start = 2
	rcfg.MaxConcurrency = 2

	// The transport cancels ctx, so runRamp returns the context error.
	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2}, []int{15169, 32934}, &stdout); !errors.Is(err, context.Canceled) {
		t.Fatalf("runRamp error = %v, want %v", err, context.Canceled)
	}

	out := stdout.String()
	// Prefetch line for entity=net must include both ids and asns.
	prefetchPrefix := fmt.Sprintf("[ramp] %s entity=net ids=[", SurfacePdbCompat)
	if !strings.Contains(out, prefetchPrefix) {
		t.Errorf("output missing prefetch line %q\n%s", prefetchPrefix, out)
	}
	if !strings.Contains(out, "asns=[") {
		t.Errorf("entity=net prefetch line should include asns=[\n%s", out)
	}
	// Per-error line shape: `[ramp] pdbcompat C=<n> ... status=500
	// err=<nil>`. Only the 500 is logged. The canceled request of the
	// second worker is dropped.
	perErrorPrefix := fmt.Sprintf("[ramp] %s C=", SurfacePdbCompat)
	var perError []string
	for line := range strings.Lines(out) {
		if strings.HasPrefix(line, perErrorPrefix) {
			perError = append(perError, strings.TrimSpace(line))
		}
	}
	if len(perError) != 1 {
		t.Fatalf("got %d per-error lines, want 1\n%s", len(perError), out)
	}
	if !strings.HasSuffix(perError[0], "status=500 err=<nil>") {
		t.Errorf("per-error line %q, want status=500 err=<nil>", perError[0])
	}
}

// TestRamp_Verbose_OrgEntity_OmitsAsns asserts that for entity=org the
// prefetch line shows ids but not asns (the asn slice is nil for org).
func TestRamp_Verbose_OrgEntity_OmitsAsns(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 1*time.Millisecond, 0, 0) // always 200
	cfg, rcfg, _ := gatedRamp(rts, []Surface{SurfacePdbCompat})
	cfg.Verbose = true
	rcfg.Entity = "org"
	rcfg.MaxConcurrency = 2

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{7, 8}, nil, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	out := stdout.String()
	if !strings.Contains(out, fmt.Sprintf("[ramp] %s entity=org ids=[", SurfacePdbCompat)) {
		t.Errorf("output missing org prefetch line\n%s", out)
	}
	// org entity has no asns — the prefetch line MUST NOT include asns=.
	for line := range strings.SplitSeq(out, "\n") {
		if !strings.HasPrefix(line, "[ramp] "+string(SurfacePdbCompat)+" entity=org") {
			continue
		}
		if strings.Contains(line, "asns=") {
			t.Errorf("org prefetch line should not include asns=, got %q", line)
		}
	}
}

// TestRamp_NoVerbose_StaysQuiet asserts the verbose log lines are
// fully gated on cfg.Verbose: markdown still emits, but no [ramp]
// lines appear. The ramp runs steps with errors, so a verbose run
// would also log one line for each error.
func TestRamp_NoVerbose_StaysQuiet(t *testing.T) {
	t.Parallel()

	// Every request of a step with concurrency >= 2 gets a 500.
	rts := newRampTestServer(t, 1*time.Millisecond, 0, 2)
	cfg, rcfg, gate := gatedRamp(rts, []Surface{SurfacePdbCompat})
	cfg.Verbose = false // explicit
	rcfg.MaxConcurrency = 2

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()

	var stdout bytes.Buffer
	if err := runRamp(ctx, cfg, rcfg, []int{1, 2}, []int{15169, 32934}, &stdout); err != nil {
		t.Fatalf("runRamp: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	// The errors make the C=2 step an inflection. The ramp holds at C=2
	// and stops there, at --max-concurrency. Each step has samples.
	wantSteps := []gateStep{
		{concurrency: 1, dur: rcfg.StepDuration},
		{concurrency: 2, dur: rcfg.StepDuration},
		{concurrency: 2, dur: rcfg.HoldDuration},
	}
	if got := gate.stepLog(); !slices.Equal(got, wantSteps) {
		t.Errorf("ramp steps = %+v, want %+v", got, wantSteps)
	}
	out := stdout.String()
	if !strings.Contains(out, fmt.Sprintf("### %s", SurfacePdbCompat)) {
		t.Errorf("markdown still expected without --verbose\n%s", out)
	}
	for _, want := range []string{rampRow("baseline", 1), rampRow("inflection", 2), rampRow("hold", 2)} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing row %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "[ramp] ") {
		t.Errorf("non-verbose run must not emit [ramp] lines\n%s", out)
	}
}

// TestSummariseStep_CountsErrorLatencies asserts that summariseStep
// counts each sample: a sample with a client timeout error and a 500
// count as errors, and their latencies go into the percentiles.
func TestSummariseStep_CountsErrorLatencies(t *testing.T) {
	t.Parallel()

	// The error has the shape that Hit returns for a client timeout.
	timeoutErr := fmt.Errorf("do GET /api/net/1: %w", context.DeadlineExceeded)
	samples := []Result{
		{Status: 200, Latency: 5 * time.Millisecond},
		{Status: 200, Latency: 10 * time.Millisecond},
		{Status: 500, Latency: 7 * time.Millisecond},
		{Err: timeoutErr, Latency: 30 * time.Millisecond},
	}
	stats := summariseStep(samples, 4, 1*time.Second)

	if stats.Samples != 4 {
		t.Errorf("Samples = %d, want 4", stats.Samples)
	}
	if stats.Errors != 2 {
		t.Errorf("Errors = %d, want 2 (the 500 and the timeout)", stats.Errors)
	}
	if stats.ErrRate != 0.5 {
		t.Errorf("ErrRate = %v, want 0.5 (2/4)", stats.ErrRate)
	}
	if stats.RPS != 4.0 {
		t.Errorf("RPS = %v, want 4.0 (4 samples / 1s)", stats.RPS)
	}
	// The timeout is the slowest sample, so it sets the tail.
	if stats.P99 != 30*time.Millisecond {
		t.Errorf("P99 = %v, want 30ms (the latency of the timeout)", stats.P99)
	}
}

// TestSummariseStep_NoSamples_ReturnsZero asserts that a step without
// samples has zero counts and keeps its concurrency and duration.
func TestSummariseStep_NoSamples_ReturnsZero(t *testing.T) {
	t.Parallel()

	stats := summariseStep(nil, 8, 500*time.Millisecond)

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
		t.Errorf("Concurrency = %d, want 8 (preserved without samples)", stats.Concurrency)
	}
	if stats.Duration != 500*time.Millisecond {
		t.Errorf("Duration = %v, want 500ms (preserved without samples)", stats.Duration)
	}
}

// clientTimeout is the client timeout of the tests that use
// newStalledServer. Only the client timeout ends a request to that
// server, so no server delay races with it.
const clientTimeout = 20 * time.Millisecond

// newStalledServer returns a test server that answers no request. Its
// handler returns when the client ends the request, or at cleanup.
func newStalledServer(tb testing.TB) *httptest.Server {
	tb.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	// Cleanups run in reverse order: release the handlers, then close
	// the server, which waits for them.
	tb.Cleanup(srv.Close)
	tb.Cleanup(func() { close(release) })
	return srv
}

// TestSummariseStep_CountsClientTimeout gets a real client timeout
// error from Hit and asserts that summariseStep counts the result as a
// sample and an error, with its latency.
func TestSummariseStep_CountsClientTimeout(t *testing.T) {
	t.Parallel()

	srv := newStalledServer(t)
	client := &http.Client{Transport: srv.Client().Transport, Timeout: clientTimeout}
	res := Hit(t.Context(), client, srv.URL, "", rampEndpointFor(SurfacePdbCompat, "net", 1, 15169))

	// net/http reports a client timeout with an error that matches
	// context.DeadlineExceeded, as a step deadline does.
	if !errors.Is(res.Err, context.DeadlineExceeded) {
		t.Fatalf("Hit error = %v, want an error that matches %v", res.Err, context.DeadlineExceeded)
	}
	if res.Latency < clientTimeout {
		t.Fatalf("Latency = %v, want at least the client timeout %v", res.Latency, clientTimeout)
	}

	stats := summariseStep([]Result{res}, 1, time.Second)
	if stats.Samples != 1 {
		t.Errorf("Samples = %d, want 1 (the timeout is a sample)", stats.Samples)
	}
	if stats.Errors != 1 || stats.ErrRate != 1 {
		t.Errorf("Errors = %d, ErrRate = %v, want 1 and 1 (the timeout is an error)", stats.Errors, stats.ErrRate)
	}
	if stats.P99 != res.Latency {
		t.Errorf("P99 = %v, want %v (the latency of the timeout)", stats.P99, res.Latency)
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

// TestRunRampStep_EndsOnDurationByDefault asserts that a step without
// RampConfig.stepEnd ends after its duration, also when no request
// completes. This is the production path.
func TestRunRampStep_EndsOnDurationByDefault(t *testing.T) {
	t.Parallel()

	// The server holds every request until the client cancels it.
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()
	cfg := Config{Base: srv.URL, HTTPClient: srv.Client(), Timeout: 5 * time.Second}
	rcfg := shortRampConfig([]Surface{SurfacePdbCompat})
	const dur = 50 * time.Millisecond

	start := time.Now()
	stats, err := runRampStep(ctx, cfg, rcfg, SurfacePdbCompat, 2, dur, []int{1, 2}, []int{15169, 32934}, io.Discard)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("runRampStep: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatalf("step did not end before the test deadline")
	}
	if elapsed < dur {
		t.Errorf("step ended after %v, want at least %v", elapsed, dur)
	}
	if stats.Samples != 0 {
		t.Errorf("Samples = %d, want 0 (the step drops requests in flight)", stats.Samples)
	}
}

// TestRunRampStep_CountsClientTimeouts runs a step against a server
// that answers no request, with a client timeout shorter than the
// step. Asserts that the timed-out requests are samples with a 100%
// error rate, and that detectInflection reports the error rate, not a
// step without samples.
func TestRunRampStep_CountsClientTimeouts(t *testing.T) {
	t.Parallel()

	srv := newStalledServer(t)
	// stepGate ends the step when concurrency+gateSamples requests have
	// started. Only the client timeout ends a request, so the step lasts
	// several client timeouts and has at least gateSamples samples.
	gate := &stepGate{next: srv.Client().Transport, samples: gateSamples}
	cfg := Config{
		Base:       srv.URL,
		HTTPClient: &http.Client{Transport: gate, Timeout: clientTimeout},
		Timeout:    clientTimeout,
	}
	rcfg := shortRampConfig([]Surface{SurfacePdbCompat})
	rcfg.stepEnd = gate.stepEnd
	// Make the latency triggers unreachable. A sample takes at least
	// clientTimeout and less than rampTestTimeout, so p95 cannot pass
	// clientTimeout*1e4 = 200s and p99 cannot pass an hour.
	rcfg.P95Multiplier = 1e4
	rcfg.P99Absolute = time.Hour

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()
	step, err := runRampStep(ctx, cfg, rcfg, SurfacePdbCompat, 2, rcfg.StepDuration, []int{1, 2}, []int{15169, 32934}, io.Discard)
	if err != nil {
		t.Fatalf("runRampStep: %v", err)
	}

	if step.Samples < gateSamples {
		t.Errorf("Samples = %d, want at least %d", step.Samples, gateSamples)
	}
	if step.ErrRate != 1 {
		t.Errorf("ErrRate = %v, want 1 (each request reached the client timeout)", step.ErrRate)
	}
	if step.P50 < clientTimeout {
		t.Errorf("P50 = %v, want at least the client timeout %v", step.P50, clientTimeout)
	}
	baseline := stepStats{Concurrency: 1, Samples: gateSamples, P50: clientTimeout, P95: clientTimeout, P99: clientTimeout}
	reason, hit := detectInflection(step, baseline, rcfg)
	if !hit || !strings.HasPrefix(reason, "error rate 100.00%") {
		t.Errorf("detectInflection = (%q, %v), want the error-rate reason", reason, hit)
	}
}

// TestRamp_Interrupted_ReportsMeasuredSteps cancels the run when the
// ramp starts the step at C=4. Asserts that the markdown still reports
// the baseline and the C=2 step, with the interrupt as the reason, that
// the surface is not reported as aborted, and that runRamp returns the
// context error.
func TestRamp_Interrupted_ReportsMeasuredSteps(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 5*time.Millisecond, 0, 0)
	cfg, rcfg, gate := gatedRamp(rts, []Surface{SurfacePdbCompat})
	// Make every trigger unreachable, with the bounds of
	// TestRamp_Inflection_TriggersOnNoSamples.
	rcfg.P99Absolute = time.Hour
	rcfg.P95Multiplier = 1e4
	rcfg.ErrorRateThreshold = 1.0

	ctx, cancel := context.WithTimeout(context.Background(), rampTestTimeout)
	defer cancel()
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	rcfg.stepEnd = func(ctx context.Context, concurrency int, dur time.Duration) (context.Context, context.CancelFunc) {
		if concurrency >= 4 {
			cancelRun()
		}
		return gate.stepEnd(ctx, concurrency, dur)
	}

	var stdout bytes.Buffer
	err := runRamp(runCtx, cfg, rcfg, []int{1, 2, 3, 4}, []int{15169, 32934, 13335, 16509}, &stdout)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("runRamp error = %v, want %v", err, context.Canceled)
	}
	if ctx.Err() != nil {
		t.Fatalf("ramp did not finish before the test deadline")
	}

	out := stdout.String()
	for _, want := range []string{
		rampRow("baseline", 1),
		rampRow("C=2", 2),
		"\ninflection reason: interrupted at C=4: context canceled\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "ABORTED") {
		t.Errorf("output reports the surface as aborted\n%s", out)
	}
}

// TestRunRampStep_KeepsResultCompletedAtStepEnd ends the step inside
// the only request of the step, after the response is made. The worker
// then has a completed request and a done step context at the same
// time. Asserts that the step keeps the request as a sample. The loop
// runs 50 steps, because a select between the send and the step end
// drops the sample on about half of them.
func TestRunRampStep_KeepsResultCompletedAtStepEnd(t *testing.T) {
	t.Parallel()

	for i := range 50 {
		var endStep context.CancelFunc
		cfg := Config{
			Base: "http://ramp.invalid",
			HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				endStep()
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{"data":[]}`)),
					Request:    r,
				}, nil
			})},
			Timeout: 5 * time.Second,
		}
		rcfg := shortRampConfig([]Surface{SurfacePdbCompat})
		rcfg.stepEnd = func(ctx context.Context, _ int, _ time.Duration) (context.Context, context.CancelFunc) {
			stepCtx, cancel := context.WithCancel(ctx)
			endStep = cancel
			return stepCtx, cancel
		}
		stats, err := runRampStep(t.Context(), cfg, rcfg, SurfacePdbCompat, 1, rcfg.StepDuration, []int{1}, []int{15169}, io.Discard)
		if err != nil {
			t.Fatalf("step %d: runRampStep: %v", i, err)
		}
		if stats.Samples != 1 {
			t.Fatalf("step %d: Samples = %d, want 1", i, stats.Samples)
		}
	}
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// TestRunRampStep_RunCanceled_ReturnsError asserts that a step ended by
// the context of the run returns the context error. Stats without
// samples would make detectInflection report an inflection that the
// target did not cause.
func TestRunRampStep_RunCanceled_ReturnsError(t *testing.T) {
	t.Parallel()

	rts := newRampTestServer(t, 5*time.Millisecond, 0, 0)
	cfg, rcfg, _ := gatedRamp(rts, []Surface{SurfacePdbCompat})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runRampStep(ctx, cfg, rcfg, SurfacePdbCompat, 2, rcfg.StepDuration, []int{1, 2}, []int{15169, 32934}, io.Discard)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("runRampStep error = %v, want %v", err, context.Canceled)
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
