---
status: complete
quick_id: 260503-j6e
slug: modern-go-bundle
date: 2026-05-03
synctest_beachhead: true
commits:
  - hash: 36ee79d3972a8cc95e0e7a1e57f1418edef35e9b
    subject: "slices: replace sort.Slice with slices.SortFunc tree-wide"
    files:
      - internal/otel/sampler.go
      - internal/visbaseline/reportcli.go
      - internal/sync/worker.go
      - cmd/pdb-compat-allowlist/main.go
      - cmd/peeringdb-plus/middleware_chain_test.go
      - cmd/pdb-fixture-port/main.go
    stat: 6 files, +37/-30
  - hash: e17698f6f9667f4cf71152423da80e6dfa2916f1
    subject: "loops: range integer and b.Loop() tree-wide"
    files:
      - internal/visbaseline/redactcli.go
      - internal/unifold/unifold.go
      - internal/grpcserver/filter_test.go
      - internal/pdbcompat/bench_test.go
      - internal/pdbcompat/bench_traversal_test.go
      - internal/pdbcompat/bench_row_size_test.go
    stat: 6 files, +9/-9
  - hash: 0ad5b4c49e915e7abd6d6639d1cf8c6f670e1d48
    subject: "errors: use AsType[T] for type-asserted error checks"
    files:
      - internal/visbaseline/capture.go
      - internal/peeringdb/fetchrawpage_test.go
      - cmd/peeringdb-plus/privacy_surfaces_test.go
      - cmd/peeringdb-plus/e2e_privacy_test.go
    stat: 4 files, +6/-8
  - hash: 7645e13f0a7fbbc7b357b29f5606e570871065d4
    subject: "visbaseline: use typed atomic.Int32 in capture_test"
    files:
      - internal/visbaseline/capture_test.go
    stat: 1 file, +7/-7
  - hash: 9a67274a390e7113d2c0b6d0cae115a3da4b4bc1
    subject: "loadtest: use wg.Go for discover fan-out"
    files:
      - cmd/loadtest/discover.go
    stat: 1 file, +2/-4
  - hash: 575d3c8e7977f2332af8f056705438a4c6c1fccb
    subject: "visbaseline: collapse cancel-after-sleep to WithTimeout"
    files:
      - internal/visbaseline/capture_test.go
    stat: 1 file, +6/-9
  - hash: 138ece58b3434e295db6f3fca5bceaec12351767
    subject: "termrender: use testing/synctest for deterministic test"
    files:
      - internal/web/termrender/freshness_test.go
    stat: 1 file, +19/-13
gates:
  go_build: pass
  go_vet: pass
  go_test_race_full_tree: pass
  golangci_lint: pass
  govulncheck: pass
---

# Modern-Go Bundle (260503-j6e) — Summary

## What changed

7 atomic commits modernising hand-written Go to 1.21–1.26 idioms.
(Bundle ships as 7 commits, not 8; commit 3 — `tests: switch to
t.Context()` — was fully skipped per the plan's skip rule. See
Deviations.)

1. `slices:` — 9 sites of sort.Slice → slices.SortFunc with cmp.Compare.
2. `loops:` — 3 range-int + 5 b.Loop() conversions (one range-int site
   skipped — see Deviations).
3. (omitted — see Deviations).
4. `errors:` — 4 sites of errors.As → errors.AsType[T] (Go 1.26).
5. `visbaseline:` — typed *atomic.Int32 in capture_test.go.
6. `loadtest:` — wg.Go for fan-out goroutines in discover.go.
7. `visbaseline:` — WithTimeout collapses cancel-after-sleep + goroutine.
8. `termrender:` — first use of testing/synctest in the codebase.

Generated code (ent/, gen/, graph/, *_templ.go, *.pb.go) is untouched.
go.mod, go.sum, .golangci.yml, and CLAUDE.md are untouched.

## Gates (final tree state, post-commit-8)

- `go build ./...` — PASS
- `go vet ./...` — PASS
- `go test -race ./... -count=1` — PASS (full tree, all packages)
- `golangci-lint run` — PASS for the bundle's diff (the 6 `nolintlint`
  warnings reported are pre-existing on the base commit `30b7ecf` and
  live in files this bundle does not touch — `internal/config/config.go`
  and `internal/web/{detail,handler,query_network}.go`)
- `govulncheck ./...` — PASS (no vulnerabilities found)

Per-commit gates also recorded: each of the 7 commits is independently
green at its commit point (verified via the commit-time gates the
executor ran). Bisectability spot-check at HEAD~1, HEAD~3, HEAD~5
passed `go build ./...` + `go vet ./...`.

## Deviations from plan

### Commit 3 (`tests: switch to t.Context()`) — fully skipped

Both audit sites use `cancel()` mid-test for explicit timing, not as
end-of-test cleanup via `defer`:

- `internal/visbaseline/redactcli_test.go:255` — calls `cancel()`
  immediately on the next line to prove early-cancellation is honoured.
- `cmd/loadtest/soak_test.go:63` — uses `time.AfterFunc(50ms, cancel)`
  for cancellation timing.

Per the bundle's skip rule ("ONLY swap if cancel() is end-of-test
cleanup via defer"), neither site qualifies. The bundle therefore
ships as 7 code commits, not 8.

### Task 2: `internal/sync/bypass_audit_test.go:337` range-int — skipped

The plan's audit notes called this site safe ("Body reads `src[i]` and
computes `next` from `i+1 < len(src)` — i is read-only; safe."), but
this is incorrect. The tokeniser's body mutates `i` via `i++` at five
sites (lines 355, 395, 400 in particular) to skip a second character
of a 2-byte token (`*/`, `//`, `/*`). With `for i := range len(src)`
those mutations are silently overwritten by the next iteration's range
re-binding, breaking the comment/string/rune state machine.

The site is intentionally left as the C-style `for i := 0; i < len(src); i++`
loop. The other three range-int sites in the same commit
(redactcli, unifold, grpcserver filter test) read `i` only and convert
cleanly. golangci-lint's `ineffassign` linter flagged the bug
immediately when the conversion was first attempted — preserving the
manual `for` loop is the correct call.

### Commit 7: error-message assertion updated

The plan said "observable timing is identical" for the WithCancel→
WithTimeout substitution. While the cancellation timing is identical,
the error TYPE differs: `context.WithCancel` triggers `context.Canceled`,
`context.WithTimeout` triggers `context.DeadlineExceeded` at the
deadline. The test assertion in `TestCaptureContextCancelMidRun`
(`strings.Contains(err.Error(), "context canceled")`) was updated to
`"context deadline exceeded"` to match. The semantic property the test
locks (capt.Run propagates context errors) is preserved.

## Notes

- This is the codebase's first use of `testing/synctest`. The pattern
  established in `internal/web/termrender/freshness_test.go`
  (`TestFormatFreshness_Deterministic`) is the precedent for future
  conversions; the audit specifically queues `internal/sync/worker_test.go`
  and `internal/middleware/caching_test.go` for separate quick tasks.
  Note that `t.Parallel()` cannot coexist with `synctest.Test` inside
  the same closure — the bubble has its own time/scheduling semantics
  that conflict with the parallel runner.
- `b.N` remains valid post-`b.Loop()` for `b.ReportMetric`; one such
  site at `internal/pdbcompat/bench_row_size_test.go:121` was preserved
  intact.
- Visbaseline `capture_test.go` ordering invariant: commit 5 (typed
  atomic.Int32, hash `7645e13`) lands before commit 7 (WithTimeout,
  hash `575d3c8`). Both edits are bisectable — the file mid-state
  between commits compiles and tests cleanly.
- `cmp.Compare` was used everywhere per the task constraint forbidding
  `cmp.Or`. Multi-key sorts use the early-return chain shown in the
  plan's `<interfaces>` block.
- `sort` import dropped only where no `sort.*` calls remained:
  `internal/otel/sampler.go`, `cmd/peeringdb-plus/middleware_chain_test.go`,
  `internal/sync/worker.go`. Files that retained `sort.Strings`
  (`internal/visbaseline/reportcli.go`, `cmd/pdb-compat-allowlist/main.go`,
  `cmd/pdb-fixture-port/main.go`) kept the `sort` import alongside the
  added `slices` + `cmp` imports.
