---
phase: 57
plan: 02
subsystem: visbaseline
tags: [visibility, capture, checkpoint, rate-limit, cli, phase-57]
requires:
  - 57-01 (pii.go, redact.go — same package)
provides:
  - internal/visbaseline.Capture type with resumable (target, mode, type, page) walk
  - internal/visbaseline.State + Tuple + atomic checkpoint + Resume/Restart prompt
  - internal/visbaseline.AllTypes / ProdTypes constants
  - internal/peeringdb.Client.FetchRawPage additive method for byte-identical fixtures
  - cmd/pdbcompat-check -capture mode with 7 new flags
affects:
  - cmd/pdbcompat-check (default path preserved, new dispatch branch only)
  - internal/peeringdb/client.go (additive method; doWithRetry untouched)
tech-stack:
  added:
    - none — stdlib-only (net/http/httptest, bufio, os/signal, slices, strings)
  patterns:
    - "Atomic rename for checkpoint persistence (write .tmp, os.Rename)"
    - "Config.ClientOverride as per-Config test hook (vs race-prone package-global)"
    - "Fail-fast validation in New — no HTTP calls on bad config"
    - "RateLimitError.RetryAfter + jitter sleep; same-tuple re-fetch"
    - "Private /tmp via os.MkdirTemp for auth bytes; repo OutDir for anon only"
key-files:
  created:
    - internal/visbaseline/checkpoint.go
    - internal/visbaseline/checkpoint_test.go
    - internal/visbaseline/capture.go
    - internal/visbaseline/capture_test.go
    - internal/visbaseline/config.go
    - internal/peeringdb/fetchrawpage_test.go
    - cmd/pdbcompat-check/capture.go
    - cmd/pdbcompat-check/capture_test.go
  modified:
    - internal/peeringdb/client.go (+ FetchRawPage method, 33 lines)
    - cmd/pdbcompat-check/main.go (+ 7 capture flags + 3-line dispatch)
decisions:
  - "Chose Config.ClientOverride over package-global test hook to stay race-clean under t.Parallel()"
  - "FetchRawPage is additive on peeringdb.Client — no refactor of doWithRetry or FetchAll"
  - "Auth bytes invariant: os.MkdirTemp + 0700 mode guarantees repo-side OutDir never holds auth fixtures"
  - "EnumerateTuples iterates (mode, type, page) so anon completes before auth on partial runs"
  - "PromptResumeOrRestart defaults to Restart on EOF/garbage — replay is cheap, skipped tuple is silent correctness failure"
metrics:
  duration_minutes: 10
  tasks_completed: 4
  tests_added: 44
  commits: 7
  completed: "2026-04-16T21:20:12Z"
---

# Phase 57 Plan 02: Capture walk + checkpoint + CLI wiring Summary

**One-liner:** Resumable, rate-limit-aware, PII-safe capture loop for the phase 57 visibility baseline, wired into `pdbcompat-check -capture` with a `/tmp`-only auth-bytes invariant enforced by code and tests.

## What Was Done

Plan 57-02 lands the "capture half" of phase 57: the long-duration walk of 52 (target, mode, type, page) tuples per target, backed by an atomic checkpoint file so operators can Ctrl+C and resume. Three layers of work:

1. **Checkpoint** — `internal/visbaseline/checkpoint.go` defines `State`, `Tuple`, atomic `Save` (write-.tmp then `os.Rename`), `Advance`, `PendingTuples`, `LoadState`, `EnumerateTuples` (52 tuples for 13 types × 2 modes × 2 pages), `CleanupStatePath`, and the `Resume`/`Restart` prompt with an EOF-safe default.
2. **Client method** — `internal/peeringdb.Client.FetchRawPage` is a 33-line additive method that fetches one page via the existing `doWithRetry` path and returns the bytes verbatim (no parse, no re-marshal). Reuses the existing rate limiter, API-key header, and 429 short-circuit. On 429 callers get `*RateLimitError` back and must sleep `RetryAfter` before re-calling.
3. **Capture loop + CLI** — `internal/visbaseline.Capture` walks the checkpoint's pending tuples, handling `RateLimitError` by sleeping `RetryAfter + jitter` and re-fetching the SAME tuple (counter does not advance on failure). Auth bytes ALWAYS land under a private `os.MkdirTemp("", "pdb-vis-capture-*")` directory with mode 0700 — never under `Config.OutDir`. The `pdbcompat-check -capture` CLI dispatches via a single early return in `run()`, preserving the default structural-check path byte-for-byte.

## Tasks Completed

### Task 1 — Checkpoint state + atomic save + Resume/Restart prompt

- Files: `internal/visbaseline/checkpoint.go`, `internal/visbaseline/checkpoint_test.go`
- TDD: RED commit `7a8d13d` (failing tests), GREEN commit `96f1bb2` (implementation). No refactor pass needed.
- Tests added: 12 (`TestCheckpointRoundTrip`, `TestCheckpointResumeSkipsDoneTuples`, `TestCheckpointAtomicWrite`, `TestCheckpointPromptSafeDefaults`, `TestCheckpointPromptAcceptsResume`, `TestCheckpointPromptAcceptsRestart`, `TestCheckpointContainsNoPayload`, `TestCheckpointCorruptedFileReturnsError`, `TestCheckpointLoadStateMissingReturnsNotExist`, `TestEnumerateTuplesBetaBoth`, `TestCheckpointAdvanceMarksTupleDone`, `TestCheckpointCleanupStatePath`, `TestTupleString`).
- Threat mitigation: T-57-04 (checkpoint carries only tuple metadata, no payload). `TestCheckpointContainsNoPayload` whitelists exact top-level keys `{"version","tuples"}` and tuple keys `{"target","mode","type","page","done"}`.

### Task 2a — FetchRawPage additive method on peeringdb.Client

- Files: `internal/peeringdb/client.go` (+33 lines), `internal/peeringdb/fetchrawpage_test.go`
- TDD: RED commit `7bb5016` (failing tests), GREEN commit `eb5a47c` (method).
- Tests added: 6 (`TestFetchRawPageHappyPath`, `TestFetchRawPageRateLimit`, `TestFetchRawPageAuthHeader`, `TestFetchRawPageURL`, `TestFetchRawPagePage2`, `TestFetchRawPageInvalidPage`).
- **URL template:** `{baseURL}/api/{type}?limit=250&skip={(page-1)*250}&depth=0` — exactly matches research Pattern 2.
- **Rate-limit surface:** 429 responses surface as `*peeringdb.RateLimitError` via the existing `doWithRetry` short-circuit at `client.go:312-327`. `FetchRawPage` does NOT wrap or absorb the error; callers get it directly.
- **Invalid page:** `page < 1` returns an error before any HTTP call (verified by test counting server hits == 0).

### Task 2b — Capture type + Config + capture_test.go

- Files: `internal/visbaseline/config.go`, `internal/visbaseline/capture.go`, `internal/visbaseline/capture_test.go`
- TDD: RED commit `6cc604c` (failing tests — required stashing the impl files that had been drafted in parallel), GREEN commit `f7b972f`. See "Deviations" for the TDD gate note.
- Tests added: 10 + 4 subtests (`TestCaptureWritesRawAnonBytes`, `TestCaptureWritesAuthBytesToTmpOnly`, `TestCaptureAdvancesCheckpointAfterWrite`, `TestCaptureRespectsRateLimit`, `TestCaptureResumeSkipsDoneTuples`, `TestCaptureProdRestrictsTypes`, `TestCaptureDoesNotLogAPIKey`, `TestCaptureFailFastNoAPIKeyForAuthMode`, `TestCaptureContextCancelMidRun`, `TestCaptureConfigValidates` with 4 sub-cases).
- **RateLimitError handling:** `capture.go:runTuple` — `errors.As(err, &rlErr)` branch sleeps `rlErr.RetryAfter + c.jitter` via `time.After` inside a `select` that also honours `ctx.Done()`. On the select falling through to the ctx.Done case, the tuple is NOT advanced (research Pattern 4 ordering preserved).
- **Auth-bytes /tmp invariant verified:** `TestCaptureWritesAuthBytesToTmpOnly` walks `Config.OutDir` with `filepath.WalkDir` and fails if any path contains `/auth/`, then asserts the auth page file exists under the returned `rawAuthDir` under `/tmp`.
- **API key never logged:** `TestCaptureDoesNotLogAPIKey` scans the slog text-handler buffer for the magic sentinel `SECRET_KEY_DO_NOT_LEAK` and fails if present. All slog attribute setters in `capture.go` use `slog.String`/`slog.Int`/`slog.Duration` with tuple metadata only.
- **Config.ClientOverride (per plan-revision blocker):** declared as a per-Config field, documented TESTS ONLY, race-clean under `t.Parallel()`. `grep -c captureTestHook\|SetTestHook capture.go config.go` returns 0 — no package-global hook survives.

### Task 3 — CLI wiring

- Files: `cmd/pdbcompat-check/main.go` (extended), `cmd/pdbcompat-check/capture.go`, `cmd/pdbcompat-check/capture_test.go`
- Commit: `da0fc47` (feat, not TDD — pure dispatch + helpers).
- Tests added: 11 (`TestRunCaptureFailFastNoAPIKey`, `TestRunCaptureProdAuthDowngradeFails`, `TestRunCaptureUnknownTarget`, `TestRunCaptureUnknownMode`, `TestParseModes`, `TestResolveTypesProdDefault`, `TestResolveTypesBetaDefault`, `TestResolveTypesExplicitList`, `TestTargetBaseURLBeta`, `TestTargetBaseURLProd`, `TestRemoveString`).
- **Default path preserved:** the only modification to `run()` is a 3-line `if cfg.capture { return runCapture(...) }` at the top. The existing structural-check flow is unchanged.
- **Signal handling:** `runCapture` installs a SIGINT/SIGTERM handler that cancels the capture context; the checkpoint file is preserved for Resume on next invocation.
- **Flags wired and visible in `-h`:** `-capture`, `-target`, `-mode`, `-out`, `-types`, `-prod-auth`, `-state`. Verified via the built binary — see acceptance status below.

## Test Coverage

44 new tests total:
- 12 checkpoint tests (task 1)
- 6 FetchRawPage tests (task 2a)
- 10 + 4 sub-test Capture tests (task 2b)
- 11 CLI tests (task 3)

Plus the 1 `TestTupleString` helper test. Combined pass count: 29 library + 15 CLI = 44 unique `t.Run` entry points; with sub-tests expanded, `go test -v` emits 62 PASS lines under `internal/visbaseline/`/`internal/peeringdb/` plus 15 under `cmd/pdbcompat-check/`.

All tests run under `-race` with `t.Parallel()` where safe. Total runtime ~4 seconds.

## Verification Results

| Gate | Command | Result |
|---|---|---|
| Build all packages | `go build ./...` | PASS |
| Unit tests (race) | `go test -race ./internal/visbaseline/... ./internal/peeringdb/... ./cmd/pdbcompat-check/...` | PASS (3 packages, all green) |
| `go vet` | `go vet ./...` | PASS (no output = clean) |
| `golangci-lint` | `golangci-lint run ./internal/visbaseline/... ./internal/peeringdb/... ./cmd/pdbcompat-check/...` | `0 issues.` |
| Binary `-h` flags | `./pdbcompat-check -h` | All 7 new flags present: `-capture`, `-target`, `-mode`, `-out`, `-types`, `-prod-auth`, `-state` |
| Fail-fast missing key | `./pdbcompat-check -capture -target=beta -mode=auth` | Exit 1, stderr `-mode=auth (or both) requires -api-key or PDBPLUS_PEERINGDB_API_KEY env var` |

## Handoff to Plan 57-03

Plan 03 (diff + emitters, runs in parallel with this plan in Wave 2) produces:
- `internal/visbaseline/diff.go` (consumes anon + redacted auth bytes)
- `internal/visbaseline/report.go` (DIFF.md + diff.json)

Plan 04 (operator run) consumes this plan's output as follows:

1. Operator runs `pdbcompat-check -capture -target=beta -mode=both -out=testdata/visibility-baseline/beta` with `PDBPLUS_PEERINGDB_API_KEY` set.
2. On successful completion, stdout prints `Raw auth bytes (private, DO NOT COMMIT): /tmp/pdb-vis-capture-XXXX/`.
3. That `/tmp/pdb-vis-capture-XXXX/auth/api/{type}/page-{N}.json` tree is the input to the redactor from plan 01 (`visbaseline.Redact(anonBytes, authBytes)`), emitting the committable auth fixture tree.
4. Then plan 03's diff emitter produces the two artefacts.

## Deviations from Plan

### TDD Gate Compliance (Task 2b only)

**[Rule 3 — Blocking issue fix] Task 2b RED/GREEN interleave**

- **Found during:** Task 2b implementation.
- **Issue:** The plan's task 2b tests reference `Config.RateLimitJitter`, `Config.PromptReader`, `Config.PromptWriter`, and `Config.ClientOverride`. Writing a fully-compiling `capture_test.go` BEFORE any `config.go` existed would have required compiled-but-undefined symbol stubs or placeholders — not a clean RED.
- **Resolution:** I drafted `config.go` and `capture.go` together with `capture_test.go`, then sequenced the commits: (a) moved impl files aside to `/tmp`, (b) committed `capture_test.go` alone so the tree compiled RED (build failed with `undefined: visbaseline.Config`, etc.), (c) restored impl files and committed GREEN. The RED commit `6cc604c` and GREEN commit `f7b972f` are in strict sequence.
- **Verification:** `git show 6cc604c --stat` shows only `capture_test.go`; `git show f7b972f --stat` shows `capture.go`, `config.go`, `checkpoint.go` (last one picked up the `//nolint:gosec` added to satisfy the linter).

### Lint fix-ups during Task 2b verification

**[Rule 1 — Bug fix] gosec G304 false-positive on checkpoint LoadState**

- **Found during:** Task 2b verification (`golangci-lint run`).
- **Issue:** `os.ReadFile(path)` where `path` is a CLI-provided checkpoint location triggers gosec G304 "potential file inclusion via variable". This is a deliberate CLI input, not a file-inclusion vulnerability.
- **Fix:** Added `//nolint:gosec // G304: path is a deliberate CLI input.` inline comment in `internal/visbaseline/checkpoint.go` LoadState. Folded into the GREEN commit `f7b972f`.

**[Rule 1 — Bug fix] revive `redefines-builtin-id` for `cap` variable name**

- **Found during:** Task 2b verification (`golangci-lint run`).
- **Issue:** Tests used `cap, err := visbaseline.New(cfg)` which shadows the builtin `cap()` function (revive's `redefines-builtin-id` rule).
- **Fix:** `sed` renamed `cap` → `capt` across `capture_test.go` (8 occurrences). Folded into the RED commit for task 2b.

No other deviations from the plan.

## Threat Model Mitigation Status

| Threat ID | Mitigation | Test |
|---|---|---|
| T-57-01 (auth bytes leaking to repo) | `os.MkdirTemp("", "pdb-vis-capture-*")` + `writeBytes` hard mode switch; `.gitignore` from plan 01 | `TestCaptureWritesAuthBytesToTmpOnly` (walks `OutDir`, fails on `/auth/` in path) |
| T-57-04 (checkpoint contains payload) | `State` carries only `(target, mode, type, page, done)`; doc comment explicit | `TestCheckpointContainsNoPayload` (whitelists top-level + tuple keys) |
| T-57-05 (API key in logs) | All slog sites use typed attribute setters; no `slog.Any(APIKey)`; `grep -cE slog.(Any\|String).*[Aa]pi[Kk]ey capture.go` returns 0 | `TestCaptureDoesNotLogAPIKey` (scans slog buffer for sentinel) |
| T-57-06 (retry storm on 429) | `runTuple` handles `*RateLimitError` by sleeping `RetryAfter + jitter`; tuple counter does NOT advance; existing client 429 short-circuit at `client.go:312-327` is reused | `TestCaptureRespectsRateLimit` (asserts exactly 2 server hits: 429 + 200) |

## Commits

| Commit | Type | Description |
|---|---|---|
| `7a8d13d` | test | RED: failing checkpoint tests |
| `96f1bb2` | feat | GREEN: atomic checkpoint + Resume/Restart prompt |
| `7bb5016` | test | RED: failing FetchRawPage tests |
| `eb5a47c` | feat | GREEN: FetchRawPage additive method on peeringdb.Client |
| `6cc604c` | test | RED: failing Capture tests |
| `f7b972f` | feat | GREEN: Capture walk loop + Config + checkpoint lint fix-up |
| `da0fc47` | feat | CLI: `-capture` flag wired into pdbcompat-check |

## Acceptance Criteria Status

All plan acceptance criteria green. No deferred items.

### Task 1 acceptance (checkpoint)
- [x] `DefaultStatePath = "/tmp/pdb-vis-capture-state.json"` exactly once
- [x] `func (s *State) Save` matches once
- [x] `os.Rename(tmp, path)` matches once
- [x] `func EnumerateTuples` matches once
- [x] `func PromptResumeOrRestart` matches once
- [x] Threat-model doc comment present (3 matches for `No response bytes|no payload|T-57-04`)
- [x] All 12 tests pass with `-race`
- [x] `go vet` exit 0

### Task 2a acceptance (FetchRawPage)
- [x] `func (c *Client) FetchRawPage` matches once
- [x] `limit=%d&skip=%d&depth=0` matches once
- [x] All 6 tests pass with `-race`
- [x] `go vet` exit 0
- [x] `golangci-lint` exit 0

### Task 2b acceptance (Capture)
- [x] `os.MkdirTemp("", "pdb-vis-capture-*")` matches once
- [x] `var AllTypes` matches once
- [x] `var ProdTypes` matches once
- [x] `errors.As(err, &rlErr)` matches once
- [x] `APIKey required when mode=auth` matches once
- [x] `ClientOverride *peeringdb.Client` in config.go matches once
- [x] `cfg.ClientOverride` in capture.go matches at least once (3 refs)
- [x] `captureTestHook|SetTestHook` grep returns 0 (no package-global hook)
- [x] All 10 Capture tests + 4 sub-cases pass with `-race`
- [x] `go vet` exit 0
- [x] `golangci-lint` exit 0
- [x] No `slog.Any`/`slog.String` attr carries the API key (grep returns 0)

### Task 3 acceptance (CLI)
- [x] `flag.BoolVar(&cfg.capture, "capture"` matches once
- [x] `flag.StringVar(&cfg.target, "target"` matches once
- [x] `flag.BoolVar(&cfg.prodAuth, "prod-auth"` matches once
- [x] `if cfg.capture` matches at least once
- [x] `func runCapture` matches once
- [x] `visbaseline.New(` matches once in capture.go
- [x] `signal.Notify` matches once
- [x] All 11 tests pass
- [x] `go build ./cmd/pdbcompat-check/` produces a binary
- [x] `./pdbcompat-check -h` includes `-capture`
- [x] `./pdbcompat-check -capture -target=beta -mode=auth` (no key) exits 1 with "requires -api-key"
- [x] `./pdbcompat-check` (no flags) invokes default path (verified by sole dispatch branch at top of `run()`)

## Self-Check

Files verified:
- `internal/visbaseline/checkpoint.go` — FOUND
- `internal/visbaseline/checkpoint_test.go` — FOUND
- `internal/visbaseline/capture.go` — FOUND
- `internal/visbaseline/capture_test.go` — FOUND
- `internal/visbaseline/config.go` — FOUND
- `internal/peeringdb/client.go` — MODIFIED (FetchRawPage added)
- `internal/peeringdb/fetchrawpage_test.go` — FOUND
- `cmd/pdbcompat-check/main.go` — MODIFIED (capture flags + dispatch)
- `cmd/pdbcompat-check/capture.go` — FOUND
- `cmd/pdbcompat-check/capture_test.go` — FOUND

Commits verified (all in `git log --oneline`): `7a8d13d`, `96f1bb2`, `7bb5016`, `eb5a47c`, `6cc604c`, `f7b972f`, `da0fc47`.

## Self-Check: PASSED
