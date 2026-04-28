---
phase: quick-260428-blj
plan: 01
subsystem: cmd/loadtest
tags: [loadtest, ramp, verbose, observability, hermeticity]
requires:
  - cmd/loadtest/ramp.go
  - cmd/loadtest/main.go (Config.Verbose flag)
provides:
  - "Verbose prefetch line per surface in ramp mode"
  - "Verbose per-error log line per non-canceled non-OK Result in ramp mode"
  - "summariseStep cancel-aware filtering — context.Canceled samples no longer pollute Errors/Samples/RPS/percentiles"
affects:
  - cmd/loadtest/ramp.go
  - cmd/loadtest/ramp_test.go
tech-stack:
  patterns:
    - "Package-level sync.Mutex (verboseMu) serialising worker-goroutine writes to any io.Writer"
    - "3-arg slice expression (samples[:0:0]) for cancel-filter without mutating caller's slice"
    - "errors.Is(r.Err, context.Canceled) sentinel check — same predicate at filter site (Task 1) and emission site (Task 2)"
key-files:
  modified:
    - cmd/loadtest/ramp.go
    - cmd/loadtest/ramp_test.go
decisions:
  - "Plumb stdout via parameter (not stash on Config/RampConfig) — runtime dependency stays explicit at every call site"
  - "Use io.Writer + sync.Mutex rather than relying on *os.File.Write atomicity, so test *bytes.Buffer is also race-safe"
  - "Per-entity prefetch shape: omit asns= token for entity!=net (printing asns=[] is noise)"
  - "Match cancel filter at both sites (count + display) so verbose log matches counted errors exactly"
metrics:
  duration: ~30 minutes
  completed: 2026-04-28
---

# Quick Task 260428-blj: Wire cfg.Verbose into ramp mode + filter context.Canceled Summary

One-liner: ramp mode now honours `--verbose` (prefetch + per-error lines) and `summariseStep` drops step-boundary `context.Canceled` samples so they no longer pollute the inflection signal.

## What Changed

### Two diagnostic gaps closed

1. **`--verbose` was silently ignored by ramp mode.** The flag binds to `cfg.Verbose` in `main.go:108` but `ramp.go` never read it. Operators running long ramps had no visibility into which IDs/ASNs were being exercised, nor which requests were failing.

2. **Step-boundary cancellations polluted `summariseStep`.** Each step sets a `stepCtx` deadline; when it fires, every in-flight `Hit()` returns `Err=context.Canceled`. Each step recorded up to `concurrency` false errors, inflating `ErrRate` past `ErrorRateThreshold` and masking the true inflection point.

### Implementation outline

**Cancel filter (`summariseStep`).** Walks the samples once, building a filtered slice that excludes any `r` where `errors.Is(r.Err, context.Canceled)`, then operates on the filtered slice for all downstream computation (Samples, Errors, latencies, RPS, ErrRate). Uses `samples[:0:0]` (3-arg slice expression) so the filtered slice gets fresh backing storage and the caller's slice is left intact.

**Writer plumbing.** Added `stdout io.Writer` parameter to both `rampOneSurface` and `runRampStep`. `runRamp` already had it — now passes it through. Five `runRampStep` call sites in `rampOneSurface` (baseline, ramp loop, hold, two past-inflection) all propagate `stdout`.

**Verbose emission.** Two emission points, both gated on `cfg.Verbose` and serialised by a package-level `sync.Mutex` (`verboseMu`):

- **Prefetch summary** at the top of `rampOneSurface`, before the baseline call:
  - `entity=net`: `[ramp] <surface> entity=net ids=[…] asns=[…]`
  - `entity=org`: `[ramp] <surface> entity=org ids=[…]` (asns omitted entirely — it's nil)
- **Per-error log** inside `runRampStep`'s worker loop, after `Hit()` returns and before the `select` send: `[ramp] <surface> C=<n> <method> <path> status=<n> err=<v>`. Suppresses `errors.Is(res.Err, context.Canceled)` so the displayed errors match the counted errors (Task 1's filter).

### Why a mutex (not just `*os.File.Write` atomicity)

The unit test passes `*bytes.Buffer`, which is not goroutine-safe. The mutex is the cheapest way to make the emission correct for any `io.Writer` the caller chooses, including future buffered or wrapped writers. Cost is two `Lock`/`Unlock` calls per emitted line — negligible relative to the HTTP round-trip these lines are describing.

## Files Modified

- `cmd/loadtest/ramp.go` (+67/-14 across two commits)
  - Added `sync` import, `verboseMu sync.Mutex` package var
  - `summariseStep`: cancel-filter + ratio computations now over `filtered` slice
  - `rampOneSurface`: new `stdout io.Writer` param; verbose prefetch line; threads `stdout` through 5 `runRampStep` calls
  - `runRampStep`: new `stdout io.Writer` param; verbose per-error line in worker loop
- `cmd/loadtest/ramp_test.go` (+217 across three commits)
  - Six new tests
  - One `strings.Split` → `strings.SplitSeq` modernise

## New Tests (for future grep)

Task 1 (cancel filter):
- `TestSummariseStep_FiltersCanceled`
- `TestSummariseStep_AllCanceled_ReturnsZero`
- `TestSummariseStep_NoCanceled_PreservesPriorBehavior`

Task 2 (verbose emission):
- `TestRamp_Verbose_PrintsPrefetchAndErrors`
- `TestRamp_Verbose_OrgEntity_OmitsAsns`
- `TestRamp_NoVerbose_StaysQuiet`

All hermetic — `httptest.NewServer` only, no live network.

## Commits

| Type | SHA | Subject |
|------|-----|---------|
| test | `4b398a7` | add failing tests for summariseStep cancel filter (Task 1 RED) |
| feat | `599aa91` | filter context.Canceled from summariseStep + plumb stdout writer (Task 1 GREEN) |
| test | `40297fb` | add failing tests for ramp --verbose emission (Task 2 RED) |
| feat | `2341923` | wire cfg.Verbose into ramp prefetch + per-error emission (Task 2 GREEN) |
| style | `e51aa61` | use strings.SplitSeq in TestRamp_Verbose_OrgEntity_OmitsAsns (Task 3 lint fix) |

TDD gate sequence: RED → GREEN cycles for both tasks, with a regression-guard test in each RED batch.

## Verification

All four final-gate commands exit 0:

```
go build ./cmd/loadtest
go vet ./cmd/loadtest
go test -race ./cmd/loadtest         # all tests pass, including the 6 new ones
golangci-lint run ./cmd/loadtest/... # 0 issues
```

Hermeticity preserved: the only network-touching code is in tests, all wrapped in `httptest.NewServer`. Zero references to `peeringdb-plus.fly.dev` or upstream `peeringdb.com` introduced.

## Deviations from Plan

None — plan executed exactly as written, except for one out-of-scope linter-modernise hit on the new test (`strings.Split` → `strings.SplitSeq`) which was fixed in the Task 3 lint pass per the plan's `<action>` instruction to "fix in place rather than disabling the linter".

## Self-Check: PASSED

- `cmd/loadtest/ramp.go` — FOUND
- `cmd/loadtest/ramp_test.go` — FOUND
- Commits `4b398a7`, `599aa91`, `40297fb`, `2341923`, `e51aa61` — all FOUND in `git log`
