---
phase: 72
plan: 05
subsystem: pdbcompat
tags: [parity, benchmark, performance, pdbcompat, b-loop]
requires:
  - 72-04 (parity harness + 6 categorical tests)
  - 70-* (traversal filter pipeline that powers BenchmarkParity_TwoHopTraversal)
  - 71-* (stream.go that BenchmarkParity_LimitZeroStreaming exercises)
  - 69-* (json_each(?) __in rewrite that BenchmarkParity_InFiveThousandElements locks)
provides:
  - internal/pdbcompat/parity/bench_test.go (3 b.Loop() benchmarks)
  - testing.TB-widened harness (bench + test callers share setup)
affects:
  - internal/testutil/testutil.go (SetupClient/SetupClientWithDB widened to testing.TB)
  - internal/pdbcompat/parity/harness_helpers_test.go (newTestServer, httpGet, seedFixtures, decodeDataArray, extractIDs, mustDecodeProblem, persistFixture, ensureFixtureOrgParent widened)
tech-stack:
  added: []
  patterns:
    - b.Loop() benchmark idiom (Go 1.24+) per GO-TOOL-1
    - testing.TB-parameterised test helpers for bench/test co-use
key-files:
  created:
    - internal/pdbcompat/parity/bench_test.go
  modified:
    - internal/testutil/testutil.go
    - internal/pdbcompat/parity/harness_helpers_test.go
decisions:
  - Widen harness + testutil.SetupClient to testing.TB rather than maintaining a parallel bench-local harness (per plan's explicit "refactor in-place" guidance). Type-only change, zero call-site breakage.
  - Inline-seed the 2-hop traversal benchmark rather than lift the mustOrg/mustIX/... helpers from traversal_test.go. Lifting them would require widening 6 more helpers for a single call site.
  - BenchmarkParity_InFiveThousandElements reuses InFixtures via seedFixtures rather than synthesising the 5001-row block inline. Keeps the benchmark honest to the Phase 72-03 fixture ID range that in_test.go's IN-01 locks.
metrics:
  duration: "~7 minutes"
  completed: "2026-04-19"
  tasks: 1
  files_created: 1
  files_modified: 2
requirements_completed:
  - PARITY-01
---

# Phase 72 Plan 05: parity/bench_test.go — performance-sensitive parity lock-ins (D-07) Summary

Three `b.Loop()`-style benchmarks locking v1.16 performance envelopes on the pdbcompat paths most at risk of silent regression: 2-hop traversal (Phase 70), `?limit=0` streaming (Phase 71), and 5001-element `__in` (Phase 69 json_each rewrite). Share the harness from 72-04 — accomplished by widening the parity test helpers and `testutil.SetupClient` from `*testing.T` to `testing.TB`, a type-only change.

## Benchstat-shaped output

Reference machine: **AMD Ryzen 5 3600 6-Core Processor, 64 GB RAM, Go 1.26.2, Linux 6.12, go.mod toolchain 1.26.1**. Command: `go test -count=3 -run=^$ -bench=BenchmarkParity -benchtime=5x ./internal/pdbcompat/parity/`.

```
goos: linux
goarch: amd64
pkg: github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/parity
cpu: AMD Ryzen 5 3600 6-Core Processor
BenchmarkParity_TwoHopTraversal-12           	       5	    603787 ns/op	   47288 B/op	     690 allocs/op
BenchmarkParity_TwoHopTraversal-12           	       5	    582493 ns/op	   39030 B/op	     638 allocs/op
BenchmarkParity_TwoHopTraversal-12           	       5	    543183 ns/op	   43228 B/op	     650 allocs/op
BenchmarkParity_LimitZeroStreaming-12        	       5	  82677275 ns/op	34013520 B/op	  437441 allocs/op
BenchmarkParity_LimitZeroStreaming-12        	       5	  81462243 ns/op	34005428 B/op	  437380 allocs/op
BenchmarkParity_LimitZeroStreaming-12        	       5	  83667147 ns/op	34005369 B/op	  437375 allocs/op
BenchmarkParity_InFiveThousandElements-12    	       5	 100063182 ns/op	34675531 B/op	  437803 allocs/op
BenchmarkParity_InFiveThousandElements-12    	       5	  98630550 ns/op	34676518 B/op	  437811 allocs/op
BenchmarkParity_InFiveThousandElements-12    	       5	  98441476 ns/op	34674059 B/op	  437799 allocs/op
PASS
ok  	github.com/dotwaffle/peeringdb-plus/internal/pdbcompat/parity	5.728s
```

Single-sample summary (for quick grepping):

| Benchmark | ns/op (median) | B/op | allocs/op |
|---|---:|---:|---:|
| `BenchmarkParity_TwoHopTraversal` | ~583 μs | ~43 KB | ~650 |
| `BenchmarkParity_LimitZeroStreaming` | ~82.7 ms | ~34 MB | ~437 k |
| `BenchmarkParity_InFiveThousandElements` | ~98.6 ms | ~34.7 MB | ~437 k |

## Race-enabled smoke

`go test -race -run=^$ -bench=BenchmarkParity -benchtime=1x ./internal/pdbcompat/parity/` completed in 24.8 s — well under the 120 s gate — with no race reports. Expected 2-10× overhead vs. non-race is visible (e.g. `TwoHopTraversal` jumps from ~0.6 ms → ~5.4 ms) and matches the `bench_traversal_test.go` godoc warning about combining `-race` with `-bench`.

## Observations

- **2-hop traversal cost shape** is cheap (~580 μs at small N). The Phase 70 filter parser + allowlist lookup dominates over the ent query. This is consistent with `internal/pdbcompat/bench_traversal_test.go`'s 10k-scale numbers — scaling is roughly linear in the row count returned.
- **LimitZero streaming allocs** (~437 k for 5000 rows) is ~88 allocs/row. That's the baseline for the per-row-encode path in `stream.go`. A regression to full-slice materialisation would multiply this by the encoder's redundant-copy cost.
- **IN-5000 vs LimitZero streaming**: nearly identical B/op and allocs/op. Expected — both paths stream 5k rows through the same `json.Encoder`. The ~20% ns/op delta is the `json_each(?)` plan + the 5001-id parse cost on the request side.

## Harness widening (callout)

Per plan guidance, the shared harness from 72-04 was widened in-place rather than cloned:

- `internal/testutil/testutil.go`: `SetupClient(*testing.T)` → `SetupClient(testing.TB)`. Same for `SetupClientWithDB`.
- `internal/pdbcompat/parity/harness_helpers_test.go`: `newTestServer`, `newTestServerWithBudget`, `httpGet`, `decodeDataArray`, `extractIDs`, `mustDecodeProblem`, `seedFixtures`, `persistFixture`, `ensureFixtureOrgParent` — all widened.

`testing.TB` exposes `Context()`, `Cleanup`, `Helper`, `Fatalf`, `Errorf`, `Logf` — everything the harness uses. `*testing.T` and `*testing.B` both satisfy the interface, so every existing call site continues to compile. The `mustOrg / mustNet / mustIX / mustIxLan / mustIxPfx / mustFac` helpers in `traversal_test.go` were NOT widened — they're only called from tests, and widening them would expand the edit footprint without benefit (the bench inlines its seeds directly).

Verified with full-repo `go test -race ./...` (all 40+ packages green) and `golangci-lint run`.

## Verification

- [x] `go build ./...` clean
- [x] `go vet ./...` clean
- [x] `go test -race -run=^$ -bench=BenchmarkParity -benchtime=1x ./internal/pdbcompat/parity/` passes (24.8 s)
- [x] `go test -count=3 -run=^$ -bench=BenchmarkParity -benchtime=5x ./internal/pdbcompat/parity/` produces benchstat-consumable output (above)
- [x] `grep -c "b\.Loop()" bench_test.go` = 4 (3 in benchmarks + 1 in godoc)
- [x] `grep -c "b\.ReportAllocs()" bench_test.go` = 3
- [x] `grep -cE "for i := 0; i < b\.N" bench_test.go` = 0 (the 1 match is in a godoc comment forbidding the pattern, not in code)
- [x] Full-repo `go test -race ./...` still green post-widening
- [x] `golangci-lint run ./internal/pdbcompat/parity/ ./internal/testutil/` = 0 issues

## Commits

- `e325752` — `bench(72-05): parity/bench_test.go — 2-hop + limit=0 + IN-5000 (D-07)`

## Deviations from Plan

None that altered scope. Three noted points:

1. **Harness widening was explicitly sanctioned** by the plan ("If testutil.SetupClient doesn't accept testing.TB, fix the signature... Similarly, newTestServer / seedFixtures / httpGet from harness.go may need their signatures widened"). Executed as-authorised.
2. **Plan's example used `parity_test` package in one snippet, but existing parity files use `package parity`**. Followed the existing convention — required to access internal test helpers.
3. **Benchmark naming**: plan used `BenchmarkParity_InFiveThousandElements` in its must_haves truths (matches) and `BenchmarkParity_In_5000` in the commit-message template. Used the former (must_haves is authoritative).

## Deferred follow-ups

- **CI benchstat-on-main workflow** is out of plan scope per CONTEXT.md D-06 ("standard CI tier" — `go test -race ./...` on every PR covers correctness; benchmarks are noisy and don't gate merges). `.github/workflows/bench.yml` (referenced by `internal/pdbcompat/bench_traversal_test.go` godoc) already exists as the canonical bench-on-main job — it runs with `-tags=bench` and would not auto-pick up these benchmarks. Adding parity benchmarks to the existing `bench.yml` (or a sibling `parity-bench.yml`) is a separate operational concern tracked outside Phase 72.

## Self-Check: PASSED

Verified:
- `internal/pdbcompat/parity/bench_test.go` exists (256 lines).
- `internal/testutil/testutil.go` and `internal/pdbcompat/parity/harness_helpers_test.go` modified.
- Commit `e325752` present in `git log --oneline`.
