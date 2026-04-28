package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// verboseMu serialises writes to the verbose stdout from worker
// goroutines. *bytes.Buffer (used in tests) is not goroutine-safe;
// *os.File.Write is, but a single mutex is the cheapest way to make
// the emission correct for any io.Writer the caller chooses.
var verboseMu sync.Mutex

// RampConfig captures the tunable parameters of one ramp invocation.
//
// Per GO-CS-5 / GO-CTX-1 the context is never stored here — every
// helper that needs cancellation accepts ctx as the first argument.
// All durations and ratios have plan-defined defaults set by the
// flag parser in main.go.
type RampConfig struct {
	// Entity is "net" or "org" — the target entity type for both the
	// prefetch and the per-surface get-by-id requests.
	Entity string

	// Start is the initial concurrency level (the baseline step).
	Start int

	// Growth is the per-step multiplier applied between ramp steps.
	// Each next-step concurrency is ceil(prev * Growth) with a +1
	// guard so growth never stalls.
	Growth float64

	// StepDuration is the wall-clock time spent at each concurrency
	// level (baseline and post-inflection). The hold-after-inflection
	// step uses HoldDuration instead so we accumulate enough samples
	// for a stable p99.
	StepDuration time.Duration

	// HoldDuration is the wall-clock time spent at the inflection
	// concurrency after detection — long enough to gather >=1000
	// samples for a stable p99 reading.
	HoldDuration time.Duration

	// MaxConcurrency is the upper bound on per-step worker count.
	// The ramp terminates once a step would exceed this cap.
	MaxConcurrency int

	// P95Multiplier defines the inflection threshold for p95: a step
	// triggers inflection when its p95 exceeds baseline.p95 * this
	// multiplier.
	P95Multiplier float64

	// P99Absolute is the absolute p99 ceiling above which inflection
	// triggers regardless of baseline.
	P99Absolute time.Duration

	// ErrorRateThreshold is the fractional error rate (e.g. 0.01 for
	// 1%) above which inflection triggers.
	ErrorRateThreshold float64

	// Surfaces is the ordered list of API surfaces to ramp. Surfaces
	// are processed sequentially in this order; cross-surface
	// contention is avoided by running each ramp to completion before
	// starting the next.
	Surfaces []Surface

	// Markdown gates the per-surface markdown emission to stdout.
	// Currently always true — left as a knob for future formats.
	Markdown bool

	// PrefetchCount is the size of the round-robin ID list fetched
	// once before any surface ramps. Not exposed as a flag.
	PrefetchCount int
}

// stepStats summarises one ramp step: the latencies observed during
// the step's wall-clock window plus the success/error counts.
type stepStats struct {
	Concurrency int
	Samples     int
	Errors      int
	P50         time.Duration
	P95         time.Duration
	P99         time.Duration
	ErrRate     float64
	RPS         float64
	Duration    time.Duration
}

// surfaceLabel pairs a stepStats with a human label used in the
// markdown emitter ("baseline", "inflection", "inflection+1").
type surfaceLabel struct {
	Label string
	Stats stepStats
}

// validSurfaces is the canonical set parsed by parseSurfaces and
// rejected with a clear error message on unknown input. Order here
// is the documented "default" surface order; --surfaces overrides
// both selection and ordering.
var validSurfaces = []Surface{
	SurfacePdbCompat,
	SurfaceEntRest,
	SurfaceGraphQL,
	SurfaceConnectRPC,
	SurfaceWebUI,
}

// parseSurfaces converts a comma-separated CSV of surface names into
// a typed slice in the user-provided order. Empty / whitespace
// fragments are skipped; unknown surfaces produce a sentinel error
// listing the valid names. Returns the default ordering for an empty
// input.
func parseSurfaces(csv string) ([]Surface, error) {
	if strings.TrimSpace(csv) == "" {
		out := make([]Surface, len(validSurfaces))
		copy(out, validSurfaces)
		return out, nil
	}
	parts := strings.Split(csv, ",")
	out := make([]Surface, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(p)
		if name == "" {
			continue
		}
		matched := false
		for _, s := range validSurfaces {
			if string(s) == name {
				out = append(out, s)
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("unknown surface %q (want pdbcompat|entrest|graphql|connectrpc|webui)", name)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no surfaces parsed from --surfaces")
	}
	return out, nil
}

// rampEndpointFor builds an Endpoint for the (surface, entity) pair
// keyed by id (and asn for entity=net on the Web UI surface). The
// returned Endpoint is identical in shape to the registry entries
// for get-by-id / list shapes, so Hit() handles it without any new
// code paths.
//
// GraphQL shape choice rationale:
//   - entity=net: networkByAsn(asn:N) — the project's most-used
//     resolver, returns at most 1 row, exercises the dedicated path.
//   - entity=org: organizationsList(where:{id:N}, limit:1) — uses
//     the offset/limit pagination extension; closer to operator
//     reality than a single-row Get.
func rampEndpointFor(surface Surface, entity string, id, asn int) Endpoint {
	switch surface {
	case SurfacePdbCompat:
		return Endpoint{
			Surface:    surface,
			EntityType: entity,
			Shape:      "ramp-get-by-id",
			Method:     "GET",
			Path:       fmt.Sprintf("/api/%s/%d", entity, id),
		}
	case SurfaceEntRest:
		plural := restPlurals[entity]
		return Endpoint{
			Surface:    surface,
			EntityType: entity,
			Shape:      "ramp-get-by-id",
			Method:     "GET",
			Path:       fmt.Sprintf("/rest/v1/%s/%d", plural, id),
		}
	case SurfaceGraphQL:
		var body string
		if entity == "net" {
			body = fmt.Sprintf(`{"query":"{ networkByAsn(asn: %d) { id name asn } }"}`, asn)
		} else {
			body = fmt.Sprintf(`{"query":"{ organizationsList(where: {id: %d}, limit: 1) { id name } }"}`, id)
		}
		return Endpoint{
			Surface:    surface,
			EntityType: entity,
			Shape:      "ramp-graphql",
			Method:     "POST",
			Path:       "/graphql",
			Body:       []byte(body),
			Header:     jsonHeader(),
		}
	case SurfaceConnectRPC:
		svc := rpcServiceNames[entity]
		method := rpcMethodNames[entity]
		return Endpoint{
			Surface:    surface,
			EntityType: entity,
			Shape:      "ramp-rpc-get",
			Method:     "POST",
			Path:       fmt.Sprintf("/peeringdb.v1.%s/Get%s", svc, method),
			Body:       fmt.Appendf(nil, `{"id":%d}`, id),
			Header:     jsonHeader(),
		}
	case SurfaceWebUI:
		var path string
		if entity == "net" {
			path = fmt.Sprintf("/ui/asn/%d", asn)
		} else {
			path = fmt.Sprintf("/ui/org/%d", id)
		}
		return Endpoint{
			Surface:    surface,
			EntityType: entity,
			Shape:      "ramp-ui-detail",
			Method:     "GET",
			Path:       path,
		}
	default:
		return Endpoint{Surface: surface}
	}
}

// runRamp drives the ramp loop for each requested surface in order,
// emitting a markdown table to stdout as soon as each surface's
// ramp completes. Failure modes:
//   - empty ids slice              -> error before any HTTP
//   - entity=net with mismatched   -> error: ASN slice must align
//   - context cancellation         -> propagates after current step
//
// Per GO-CTX-1 ctx is the first parameter; per GO-CS-5 the tunable
// surface uses an input struct rather than a 12-arg signature.
func runRamp(ctx context.Context, cfg Config, rcfg RampConfig, ids, asns []int, stdout io.Writer) error {
	if len(ids) == 0 {
		return errors.New("runRamp: empty id slice (discoverRampIDs returned 0 rows)")
	}
	if rcfg.Entity == "net" && len(asns) != len(ids) {
		return fmt.Errorf("runRamp: entity=net but asns len=%d != ids len=%d", len(asns), len(ids))
	}
	if rcfg.Start < 1 {
		return fmt.Errorf("runRamp: --start must be >= 1, got %d", rcfg.Start)
	}
	if rcfg.Growth <= 1.0 {
		return fmt.Errorf("runRamp: --growth must be > 1.0, got %v", rcfg.Growth)
	}
	if rcfg.StepDuration <= 0 || rcfg.HoldDuration <= 0 {
		return errors.New("runRamp: --step-duration and --hold-duration must be > 0")
	}
	if rcfg.MaxConcurrency < rcfg.Start {
		return fmt.Errorf("runRamp: --max-concurrency %d < --start %d", rcfg.MaxConcurrency, rcfg.Start)
	}
	if len(rcfg.Surfaces) == 0 {
		return errors.New("runRamp: no surfaces selected")
	}

	for _, surface := range rcfg.Surfaces {
		if err := ctx.Err(); err != nil {
			return err
		}
		labels, reason, err := rampOneSurface(ctx, cfg, rcfg, surface, ids, asns, stdout)
		if err != nil {
			fmt.Fprintf(stdout, "\n### %s ramp ABORTED: %v\n\n", surface, err)
			// continue with the next surface — one surface failing
			// shouldn't take down the whole sweep
			continue
		}
		emitMarkdown(stdout, surface, rcfg.Entity, labels, reason)
	}
	return nil
}

// rampOneSurface drives the ramp loop for a single surface. Returns
// the ordered set of labelled steps (baseline, inflection,
// inflection+1, inflection+2) and the inflection reason
// string, or an error if the baseline step itself fails.
//
// The stdout writer is plumbed through to runRampStep for verbose
// emission gated by cfg.Verbose; passing it via parameter (rather
// than stashing it in cfg or RampConfig) keeps the runtime
// dependency explicit at every call site.
func rampOneSurface(ctx context.Context, cfg Config, rcfg RampConfig, surface Surface, ids, asns []int, stdout io.Writer) ([]surfaceLabel, string, error) {
	if cfg.Verbose {
		verboseMu.Lock()
		if rcfg.Entity == "net" {
			fmt.Fprintf(stdout, "[ramp] %s entity=%s ids=%v asns=%v\n",
				surface, rcfg.Entity, ids, asns)
		} else {
			// asns is nil for entity=org; printing asns=[] would be
			// noise, so omit the token entirely.
			fmt.Fprintf(stdout, "[ramp] %s entity=%s ids=%v\n",
				surface, rcfg.Entity, ids)
		}
		verboseMu.Unlock()
	}

	// 1. Baseline at C=Start.
	baseline, err := runRampStep(ctx, cfg, rcfg, surface, rcfg.Start, rcfg.StepDuration, ids, asns, stdout)
	if err != nil {
		return nil, "", fmt.Errorf("baseline step: %w", err)
	}
	labels := []surfaceLabel{{Label: "baseline", Stats: baseline}}

	// 2. Ramp until inflection or MaxConcurrency.
	prevC := rcfg.Start
	var inflection stepStats
	var reason string
	hit := false
	for prevC < rcfg.MaxConcurrency {
		nextC := int(math.Ceil(float64(prevC) * rcfg.Growth))
		if nextC <= prevC {
			nextC = prevC + 1
		}
		if nextC > rcfg.MaxConcurrency {
			nextC = rcfg.MaxConcurrency
		}
		step, stepErr := runRampStep(ctx, cfg, rcfg, surface, nextC, rcfg.StepDuration, ids, asns, stdout)
		if stepErr != nil {
			return labels, "", fmt.Errorf("ramp step C=%d: %w", nextC, stepErr)
		}
		r, isInflection := detectInflection(step, baseline, rcfg)
		if isInflection {
			inflection = step
			reason = r
			hit = true
			labels = append(labels, surfaceLabel{Label: "inflection", Stats: step})
			break
		}
		prevC = nextC
	}

	if !hit {
		// Reached MaxConcurrency without inflection — record the
		// final step as inflection-equivalent (still useful info).
		if len(labels) >= 2 {
			labels[len(labels)-1].Label = "max-concurrency"
		}
		return labels, "no inflection detected within --max-concurrency", nil
	}

	// 3. Hold past inflection: re-run at the same concurrency for
	// HoldDuration to gather a stable p99 reading.
	hold, err := runRampStep(ctx, cfg, rcfg, surface, inflection.Concurrency, rcfg.HoldDuration, ids, asns, stdout)
	if err == nil {
		labels = append(labels, surfaceLabel{Label: "hold", Stats: hold})
	}

	// 4. One step past inflection (the second is added if MaxConcurrency
	// allows).
	pastC := inflection.Concurrency
	for i := 0; i < 2 && pastC < rcfg.MaxConcurrency; i++ {
		nextC := int(math.Ceil(float64(pastC) * rcfg.Growth))
		if nextC <= pastC {
			nextC = pastC + 1
		}
		if nextC > rcfg.MaxConcurrency {
			nextC = rcfg.MaxConcurrency
		}
		past, perr := runRampStep(ctx, cfg, rcfg, surface, nextC, rcfg.StepDuration, ids, asns, stdout)
		if perr != nil {
			break
		}
		labelText := "inflection+1"
		if i == 1 {
			labelText = "inflection+2"
		}
		labels = append(labels, surfaceLabel{Label: labelText, Stats: past})
		pastC = nextC
	}

	return labels, reason, nil
}

// runRampStep executes one ramp step at a fixed concurrency for the
// given duration and returns aggregated stats. Each worker loops on
// Hit() picking endpoints round-robin from the prefetched ID list;
// the step terminates when stepCtx fires. Workers honour gctx so a
// cancelled outer ctx propagates promptly.
func runRampStep(ctx context.Context, cfg Config, rcfg RampConfig, surface Surface, concurrency int, dur time.Duration, ids, asns []int, stdout io.Writer) (stepStats, error) {
	if concurrency < 1 {
		return stepStats{}, fmt.Errorf("concurrency must be >= 1, got %d", concurrency)
	}
	stepCtx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	// Buffered sample channel sized to absorb short bursts; the
	// drain goroutine empties it concurrently so workers rarely
	// block on send.
	sampleCh := make(chan Result, concurrency*16)
	var idx atomic.Uint32

	g, gctx := errgroup.WithContext(stepCtx)
	for range concurrency {
		g.Go(func() error {
			for gctx.Err() == nil {
				// Pick id (and asn for net) round-robin via atomic
				// counter mod len(ids). uint32 wrap is harmless —
				// modulo is what we want.
				i := idx.Add(1)
				k := int(i % uint32(len(ids))) //nolint:gosec // ID rotation index, not security-relevant
				id := ids[k]
				asn := 0
				if rcfg.Entity == "net" {
					asn = asns[k]
				}
				ep := rampEndpointFor(surface, rcfg.Entity, id, asn)
				res := Hit(gctx, cfg.HTTPClient, cfg.Base, cfg.AuthToken, ep)
				// Step-boundary discriminator: if Hit returned an
				// error AND the step's context is done, the cancellation
				// came from the step deadline (parent fires
				// DeadlineExceeded; gctx propagates that, NOT
				// context.Canceled). Drop before logging or sample
				// channel send so logged errors match counted errors.
				// Real client timeouts (>30s requests when gctx is
				// still alive) and 5xx responses still flow through.
				if res.Err != nil && gctx.Err() != nil {
					return nil
				}
				if cfg.Verbose && !res.OK() {
					verboseMu.Lock()
					fmt.Fprintf(stdout, "[ramp] %s C=%d %s %s status=%d err=%v\n",
						surface, concurrency, ep.Method, ep.Path, res.Status, res.Err)
					verboseMu.Unlock()
				}
				select {
				case sampleCh <- res:
				case <-gctx.Done():
					return nil
				}
			}
			return nil
		})
	}
	go func() {
		_ = g.Wait()
		close(sampleCh)
	}()

	samples := make([]Result, 0, concurrency*32)
	for res := range sampleCh {
		samples = append(samples, res)
	}

	return summariseStep(samples, concurrency, dur), nil
}

// summariseStep computes p50/p95/p99/error-rate/rps for a step.
// Empty sample sets return a zero-valued stepStats (legitimate when
// the ctx is cancelled before any request completes).
//
// The primary discriminator for step-boundary cancellation lives in
// the worker loop (drops samples where res.Err != nil && gctx.Err()
// != nil before they reach this function). The filter below is
// defensive: it strips any sample whose Err matches context.Canceled
// or context.DeadlineExceeded that somehow slipped through.
func summariseStep(samples []Result, concurrency int, dur time.Duration) stepStats {
	stats := stepStats{Concurrency: concurrency, Duration: dur}
	// Defensive: drop residual step-boundary cancellations. Use a
	// 3-arg slice expression so the filtered slice gets fresh
	// backing storage and the caller's slice is left intact.
	filtered := samples[:0:0]
	for _, r := range samples {
		if errors.Is(r.Err, context.Canceled) || errors.Is(r.Err, context.DeadlineExceeded) {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		return stats
	}
	latencies := make([]time.Duration, 0, len(filtered))
	errs := 0
	for _, r := range filtered {
		latencies = append(latencies, r.Latency)
		if !r.OK() {
			errs++
		}
	}
	slices.Sort(latencies)
	stats.Samples = len(filtered)
	stats.Errors = errs
	stats.P50 = percentilesFromSorted(latencies, 50)
	stats.P95 = percentilesFromSorted(latencies, 95)
	stats.P99 = percentilesFromSorted(latencies, 99)
	stats.ErrRate = float64(errs) / float64(len(filtered))
	if dur > 0 {
		stats.RPS = float64(len(filtered)) / dur.Seconds()
	}
	return stats
}

// detectInflection applies the three-trigger rule from the plan:
// p95 exceeds baseline×multiplier, p99 exceeds an absolute ceiling,
// or error rate exceeds a fractional threshold. Returns the human
// reason string and a hit/miss flag.
func detectInflection(step, baseline stepStats, rcfg RampConfig) (string, bool) {
	if step.Samples == 0 {
		return "", false
	}
	if baseline.P95 > 0 {
		threshold := time.Duration(float64(baseline.P95) * rcfg.P95Multiplier)
		if step.P95 > threshold {
			return fmt.Sprintf("p95 %s > baseline p95 %s × %.1f (= %s)",
				step.P95.Round(time.Microsecond),
				baseline.P95.Round(time.Microsecond),
				rcfg.P95Multiplier,
				threshold.Round(time.Microsecond),
			), true
		}
	}
	if step.P99 > rcfg.P99Absolute {
		return fmt.Sprintf("p99 %s > %s absolute",
			step.P99.Round(time.Microsecond),
			rcfg.P99Absolute,
		), true
	}
	if step.ErrRate > rcfg.ErrorRateThreshold {
		return fmt.Sprintf("error rate %.2f%% > threshold %.2f%%",
			step.ErrRate*100, rcfg.ErrorRateThreshold*100,
		), true
	}
	return "", false
}

// emitMarkdown writes a per-surface markdown block to w. Format:
//
//	### <surface> (entity=<entity>)
//
//	| label              |  C |   p50 |   p95 |   p99 | err % |  rps  |
//	|--------------------|----|-------|-------|-------|-------|-------|
//	| baseline           |  1 | ...                                   |
//
//	inflection reason: ...
//
// Operators can pipe stdout to `tee surface_results.md` to capture.
func emitMarkdown(w io.Writer, surface Surface, entity string, labels []surfaceLabel, reason string) {
	fmt.Fprintf(w, "\n### %s (entity=%s)\n\n", surface, entity)
	fmt.Fprintln(w, "| label              |   C |     p50 |     p95 |     p99 |  err % |     rps |")
	fmt.Fprintln(w, "|--------------------|----:|--------:|--------:|--------:|-------:|--------:|")
	for _, l := range labels {
		s := l.Stats
		fmt.Fprintf(w, "| %-18s | %3d | %7s | %7s | %7s | %5.2f%% | %7.1f |\n",
			l.Label,
			s.Concurrency,
			s.P50.Round(time.Microsecond),
			s.P95.Round(time.Microsecond),
			s.P99.Round(time.Microsecond),
			s.ErrRate*100,
			s.RPS,
		)
	}
	if reason != "" {
		fmt.Fprintf(w, "\ninflection reason: %s\n", reason)
	}
	fmt.Fprintln(w)
}
