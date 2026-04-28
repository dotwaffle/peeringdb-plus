---
phase: quick-260428-blj
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - cmd/loadtest/ramp.go
  - cmd/loadtest/ramp_test.go
autonomous: true
requirements:
  - QUICK-260428-blj-01  # cfg.Verbose wired into ramp mode (prefetch + per-error log)
  - QUICK-260428-blj-02  # context.Canceled excluded from summariseStep error/sample counts
user_setup: []

must_haves:
  truths:
    - "Running `loadtest ramp --verbose` prints one prefetch summary line per surface (ids and asns when entity=net)."
    - "Running `loadtest ramp --verbose` prints one log line per non-OK Result during a step, excluding context.Canceled."
    - "summariseStep does not count Results with Err=context.Canceled toward Errors, Samples, or RPS."
    - "Inflection signal at any concurrency level is no longer polluted by step-boundary cancellation."
    - "All loadtest tests run hermetically via httptest (no live network)."
  artifacts:
    - path: cmd/loadtest/ramp.go
      provides: "Verbose prefetch line, verbose per-error line, cancel-aware summariseStep"
      contains: "context.Canceled"
    - path: cmd/loadtest/ramp_test.go
      provides: "Cancel-filter unit test + verbose output assertion"
      contains: "TestSummariseStep_FiltersCanceled"
  key_links:
    - from: cmd/loadtest/ramp.go
      to: cmd/loadtest/main.go
      via: "Config.Verbose flag drives the new log lines"
      pattern: "cfg\\.Verbose"
    - from: cmd/loadtest/ramp.go
      to: standard library `errors` + `context`
      via: "errors.Is(r.Err, context.Canceled)"
      pattern: "errors\\.Is.*context\\.Canceled"
---

<objective>
Wire `cfg.Verbose` into ramp mode and stop end-of-step cancellation from polluting the inflection signal.

Purpose: ramp mode is the operator capacity-probing tool. Two diagnostic gaps blunt it today:
1. `--verbose` is silently ignored by ramp (the flag binds to `cfg.Verbose` but `ramp.go` never reads it). Operators have no way to see which IDs/ASNs are being exercised, nor which requests are failing during a long ramp.
2. Workers cancel mid-`Hit()` at every step boundary; those cancellations land in `summariseStep` as errors. Each step records up to `concurrency` false errors, which inflates `ErrRate` and can push a healthy step over `ErrorRateThreshold` -- masking the true inflection point.

Output: ramp mode honors `--verbose` with a prefetch summary line and per-error lines (filtered for `context.Canceled`); `summariseStep` drops cancelled samples entirely so they affect neither error rate nor RPS.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@cmd/loadtest/ramp.go
@cmd/loadtest/ramp_test.go
@cmd/loadtest/surfaces.go
@cmd/loadtest/main.go

<interfaces>
<!-- Key signatures and types the executor needs. Extracted from cmd/loadtest. -->

From cmd/loadtest/main.go:
```go
type Config struct {
    Base       string
    Timeout    time.Duration
    Verbose    bool   // <-- the flag we are wiring
    AuthToken  string
    HTTPClient *http.Client
    // sync/soak fields elided
}
```

From cmd/loadtest/surfaces.go:
```go
type Result struct {
    Endpoint Endpoint
    Status   int
    Latency  time.Duration
    Bytes    int64
    Err      error
}

func (r Result) OK() bool { return r.Err == nil && r.Status >= 200 && r.Status < 300 }

func Hit(ctx context.Context, client *http.Client, base, authToken string, ep Endpoint) Result
```

From cmd/loadtest/ramp.go (current signatures to modify):
```go
func runRamp(ctx context.Context, cfg Config, rcfg RampConfig, ids, asns []int, stdout io.Writer) error
func rampOneSurface(ctx context.Context, cfg Config, rcfg RampConfig, surface Surface, ids, asns []int) ([]surfaceLabel, string, error)
func runRampStep(ctx context.Context, cfg Config, rcfg RampConfig, surface Surface, concurrency int, dur time.Duration, ids, asns []int) (stepStats, error)
func summariseStep(samples []Result, concurrency int, dur time.Duration) stepStats
```

`runRamp` already has `stdout io.Writer`. `rampOneSurface` and `runRampStep` do NOT -- they need a writer parameter to emit verbose lines. Plumb `stdout` through both. `errors` is already imported in `ramp.go`; `context` is already imported.

Verbose line format (single shared line shape -- grep-friendly):
- prefetch:  `[ramp] <surface> entity=<entity> ids=<slice> asns=<slice>\n`  (omit `asns=` token entirely when entity != "net")
- per-error: `[ramp] <surface> C=<n> <method> <path> status=<n> err=<v>\n`
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Filter context.Canceled from summariseStep + plumb writer through ramp helpers</name>
  <files>cmd/loadtest/ramp.go, cmd/loadtest/ramp_test.go</files>
  <behavior>
    Behaviors locked by tests BEFORE implementation lands:
    - Test 1 (TestSummariseStep_FiltersCanceled): construct samples = [{OK,5ms}, {OK,10ms}, {Err: context.Canceled, Latency: 1ms}, {Status: 500, Latency: 7ms}]. Call summariseStep(samples, 4, 1*time.Second). Assert: Samples == 3 (the cancelled one is dropped), Errors == 1 (only the 500), RPS == 3.0, P50/P95/P99 computed only over the 3 non-cancelled latencies.
    - Test 2 (TestSummariseStep_AllCanceled_ReturnsZero): all-cancelled samples produce stepStats{Concurrency: c, Duration: d} with Samples=0, Errors=0, RPS=0 (i.e. behaves identically to len(samples)==0).
    - Test 3 (TestSummariseStep_NoCanceled_PreservesPriorBehavior): samples with no cancellations produce identical stats to the pre-change behaviour (regression guard for the existing happy path).
  </behavior>
  <action>
    Apply two changes to `cmd/loadtest/ramp.go`:

    1. **summariseStep cancel filter** (lines 415-439). Walk the samples once, building a filtered slice that excludes any `r` where `errors.Is(r.Err, context.Canceled)`. Then operate on the filtered slice for *all* downstream computation:
       ```go
       func summariseStep(samples []Result, concurrency int, dur time.Duration) stepStats {
           stats := stepStats{Concurrency: concurrency, Duration: dur}
           // Drop samples whose request was cancelled at the step boundary
           // (gctx fires when stepCtx deadlines, returning context.Canceled
           // mid-Hit). These are not real measurements -- their latency is
           // "time until cancel" and counting them as errors would inflate
           // ErrRate past ErrorRateThreshold and mask the true inflection.
           filtered := samples[:0:0]
           for _, r := range samples {
               if errors.Is(r.Err, context.Canceled) {
                   continue
               }
               filtered = append(filtered, r)
           }
           if len(filtered) == 0 {
               return stats
           }
           // ... existing percentile + error-count logic, but iterating
           //     `filtered` instead of `samples`. stats.Samples = len(filtered),
           //     stats.RPS = float64(len(filtered))/dur.Seconds().
       }
       ```
       Use `samples[:0:0]` (3-arg slice expression) to allocate fresh backing storage so we don't mutate the caller's slice.

    2. **Plumb `stdout io.Writer` through the ramp helpers** so Task 2 can emit verbose lines. Update signatures:
       - `rampOneSurface(ctx context.Context, cfg Config, rcfg RampConfig, surface Surface, ids, asns []int, stdout io.Writer) (...)`
       - `runRampStep(ctx context.Context, cfg Config, rcfg RampConfig, surface Surface, concurrency int, dur time.Duration, ids, asns []int, stdout io.Writer) (stepStats, error)`
       Update `runRamp` to pass `stdout` into `rampOneSurface`. Update `rampOneSurface`'s 5 calls to `runRampStep` (baseline, ramp loop, hold, two past-inflection) to pass `stdout` through. Do NOT add any verbose-emission code in this task -- this is pure plumbing. The writer is unused inside `runRampStep` until Task 2.

    Update `cmd/loadtest/ramp_test.go`:
    - Add the three tests above. Place them next to `TestDetectInflection`.
    - Update the existing 4 callers of `runRamp` -- they already pass `&stdout` (a `*bytes.Buffer`), so the runRamp signature is unchanged. No call-site updates needed at the test-file level for Task 1.
    - The `runRampStep` and `rampOneSurface` signatures DO change -- but neither is called directly from tests today (grep `runRampStep\|rampOneSurface` in `ramp_test.go` confirms zero direct callers). No test updates beyond adding the three new tests.

    Imports in ramp.go: `errors` and `context` are already present -- no new imports.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go test -race -run 'TestSummariseStep_|TestRamp_|TestDetectInflection|TestParseRampFlags_|TestParseSurfaces_|TestDiscoverRampIDs_|TestRejectUpstreamBase|TestRunRamp_' ./cmd/loadtest</automated>
  </verify>
  <done>
    `summariseStep` drops `context.Canceled` results from Samples, Errors, latencies, and RPS. `rampOneSurface` and `runRampStep` accept `stdout io.Writer`. All existing ramp tests still pass; three new tests cover the cancel-filter semantics. `go vet ./cmd/loadtest` clean.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Emit cfg.Verbose lines in ramp mode (prefetch summary + per-error log)</name>
  <files>cmd/loadtest/ramp.go, cmd/loadtest/ramp_test.go</files>
  <behavior>
    Behaviors locked by tests BEFORE implementation:
    - Test 1 (TestRamp_Verbose_PrintsPrefetchAndErrors): stand up an httptest server that returns 500 on the first hit and 200 thereafter. Run a 1-surface ramp with `cfg.Verbose=true`, capture stdout into a `*bytes.Buffer`, and assert:
      - Buffer contains exactly one prefetch line matching `[ramp] pdbcompat entity=net ids=[`.
      - When entity=net, prefetch line also contains `asns=[`.
      - Buffer contains at least one per-error line matching `[ramp] pdbcompat C=` ... `status=500`.
      - Buffer contains NO line with `err=context.Canceled` (the step-boundary cancellations must be suppressed).
    - Test 2 (TestRamp_Verbose_OrgEntity_OmitsAsns): same shape but `rcfg.Entity="org"` and `asns=nil`. Assert the prefetch line contains `entity=org ids=[` but does NOT contain `asns=`.
    - Test 3 (TestRamp_NoVerbose_StaysQuiet): `cfg.Verbose=false`, no errors generated; assert the buffer contains no `[ramp]`-prefixed lines (only the markdown table).
  </behavior>
  <action>
    Apply two emission points in `cmd/loadtest/ramp.go`. Both gated on `cfg.Verbose`. Use `fmt.Fprintf(stdout, ...)` -- never `log.*` (loadtest is a CLI tool, not a structured-log surface).

    1. **Prefetch summary line** at the top of `rampOneSurface`, BEFORE the baseline call to `runRampStep`:
       ```go
       if cfg.Verbose {
           if rcfg.Entity == "net" {
               fmt.Fprintf(stdout, "[ramp] %s entity=%s ids=%v asns=%v\n",
                   surface, rcfg.Entity, ids, asns)
           } else {
               fmt.Fprintf(stdout, "[ramp] %s entity=%s ids=%v\n",
                   surface, rcfg.Entity, ids)
           }
       }
       ```
       Rationale for the entity-conditional: `asns` is `nil` for `entity=org`, and printing `asns=[]` would be noise.

    2. **Per-error log line** inside `runRampStep`'s worker loop, immediately after the `Hit()` call returns and BEFORE the `select` that sends to `sampleCh`:
       ```go
       res := Hit(gctx, cfg.HTTPClient, cfg.Base, cfg.AuthToken, ep)
       if cfg.Verbose && !res.OK() && !errors.Is(res.Err, context.Canceled) {
           fmt.Fprintf(stdout, "[ramp] %s C=%d %s %s status=%d err=%v\n",
               surface, concurrency, ep.Method, ep.Path, res.Status, res.Err)
       }
       select {
       case sampleCh <- res:
       case <-gctx.Done():
           return nil
       }
       ```
       The `errors.Is(res.Err, context.Canceled)` guard suppresses the spam at every step boundary -- exactly the same predicate as Task 1's filter, so the displayed errors match the counted errors.

       NOTE on concurrency safety: `fmt.Fprintf` to a `*bytes.Buffer` (test) or `*os.File` (production) is racy under multi-worker contention. `*os.File.Write` is goroutine-safe (unix `write(2)` is atomic for buffers <= PIPE_BUF and `os.File` does not buffer). For the test, `*bytes.Buffer` is NOT goroutine-safe -- so wrap the emission with a small `sync.Mutex` declared as a package-private `var verboseMu sync.Mutex` at the top of `ramp.go`, and lock around BOTH the prefetch print and the per-error print. This adds ~10 lines but eliminates the race in tests AND in any future caller that passes a buffered or non-thread-safe writer.

    Update `cmd/loadtest/ramp_test.go`:
    - Add the three tests above. Use `newRampTestServer` for Test 1 with `errorCThresh=1` (returns 500 once inflight >= 1, i.e. always). Pin `MaxConcurrency` low (e.g. 2) and `StepDuration` short (50ms) so the test runs in well under a second.
    - For Test 2, reuse `newRampTestServer` with `errorCThresh=0` (always 200); we are only asserting the prefetch line shape.
    - For Test 3, reuse Test 2's setup but flip `cfg.Verbose=false`; assert buffer contains `### pdbcompat` (markdown is still emitted) but no `[ramp]` prefix.
    - Tests must use `t.Parallel()` and the existing `shortRampConfig` helper.

    Imports: no new imports needed -- `errors`, `context`, `fmt`, `io`, `sync` are all already in scope (sync via the test file already; for ramp.go add `sync` if `verboseMu sync.Mutex` is needed -- check current imports first; if already present, skip).
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go test -race -run 'TestRamp_Verbose_|TestRamp_NoVerbose_|TestRamp_Inflection_|TestRamp_HoldDuration_|TestRamp_PerSurface_' ./cmd/loadtest</automated>
  </verify>
  <done>
    `--verbose` produces one prefetch line per surface and one log line per non-canceled error. `context.Canceled` step-boundary errors are suppressed in BOTH the count (Task 1) and the verbose log (Task 2). Three new tests pass under `-race`.
  </done>
</task>

<task type="auto">
  <name>Task 3: Lint + build + full-package test gate</name>
  <files>cmd/loadtest/ramp.go, cmd/loadtest/ramp_test.go</files>
  <action>
    Final gate. Run the project's standard quality bar against the loadtest package:
    1. `go build ./cmd/loadtest` -- must succeed.
    2. `go vet ./cmd/loadtest` -- must be clean.
    3. `go test -race ./cmd/loadtest` -- full package, not a -run subset, to catch any incidental breakage.
    4. `golangci-lint run ./cmd/loadtest/...` -- must be clean.

    If any of the above flags an issue (most likely: `gocritic` or `revive` complaining about the new emission block, or `contextcheck` if a ctx is mis-routed), fix in place rather than disabling the linter. The action surface is tiny (one writer, one Mutex, two Fprintf calls) -- there should be nothing to suppress.

    Do NOT touch any file outside `cmd/loadtest/`. This task is verification-only modulo small lint follow-ups.
  </action>
  <verify>
    <automated>cd /home/dotwaffle/Code/pdb/peeringdb-plus &amp;&amp; go build ./cmd/loadtest &amp;&amp; go vet ./cmd/loadtest &amp;&amp; go test -race ./cmd/loadtest &amp;&amp; golangci-lint run ./cmd/loadtest/...</automated>
  </verify>
  <done>
    All four commands exit 0. No new linter warnings introduced. Loadtest package compiles and tests pass under `-race`.
  </done>
</task>

</tasks>

<verification>
End-to-end check (manual, optional -- not part of automated gate, run only if questioning the integration):
```bash
go run ./cmd/loadtest ramp --target=http://localhost:8080 --verbose --max-concurrency=4 --step-duration=500ms --hold-duration=1s --surfaces=pdbcompat
```
Expect: one `[ramp] pdbcompat entity=net ids=[...] asns=[...]` line at the top, zero `err=context.Canceled` lines anywhere, and a markdown table at the end.

Automated phase verification (executed per task above):
- `go build ./cmd/loadtest`
- `go vet ./cmd/loadtest`
- `go test -race ./cmd/loadtest`
- `golangci-lint run ./cmd/loadtest/...`

Hermeticity guard: the only network-touching code added is in tests, all wrapped in `httptest.NewServer`. No live `peeringdb-plus.fly.dev` or upstream calls.
</verification>

<success_criteria>
- Running `loadtest ramp --verbose` prints exactly one prefetch line per surface and one log line per non-canceled error.
- `summariseStep` results never include `context.Canceled` requests in `Samples`, `Errors`, latency percentiles, or `RPS`.
- Inflection detection (`detectInflection`) sees only real measurements -- step-boundary cancellation no longer trips `ErrorRateThreshold`.
- `go test -race ./cmd/loadtest` passes including the 6 new tests (3 in Task 1, 3 in Task 2).
- `golangci-lint run ./cmd/loadtest/...` clean.
- Zero changes outside `cmd/loadtest/`.
</success_criteria>

<output>
After completion, create `.planning/quick/260428-blj-wire-cfg-verbose-into-ramp-mode-print-pr/260428-blj-SUMMARY.md` documenting the two fixes, the writer-plumbing approach, the cancel-filter rationale, and the 6 new test names for future grep.
</output>
