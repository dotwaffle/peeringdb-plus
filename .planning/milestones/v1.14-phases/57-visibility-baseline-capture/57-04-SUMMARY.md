---
phase: 57
plan: 04
subsystem: visbaseline
tags: [visibility-baseline, cli, redaction, diff-report, phase-57]
status: partial
requires: [57-01, 57-02, 57-03]
provides:
  - visbaseline.RedactDir (directory-level redaction driver)
  - visbaseline.BuildReport (dual-shape diff report builder)
  - pdbcompat-check -redact / -diff CLI modes
affects:
  - cmd/pdbcompat-check/main.go
  - cmd/pdbcompat-check/redactdiff.go (new)
  - cmd/pdbcompat-check/redactdiff_test.go (new)
  - internal/visbaseline/redactcli.go (new)
  - internal/visbaseline/redactcli_test.go (new)
  - internal/visbaseline/reportcli.go (new)
  - internal/visbaseline/reportcli_test.go (new)
tech-stack:
  added: []
  patterns:
    - Dual-shape auto-detection for single- vs multi-target baseline roots
    - Concatenated-page merge before Diff to aggregate row counts per type
    - Derive anon dir from -out path (replace trailing /auth with /anon)
    - GO-CFG-1 fail-fast validation on -in/-out (empty, root, CWD)
key-files:
  created:
    - internal/visbaseline/redactcli.go
    - internal/visbaseline/redactcli_test.go
    - internal/visbaseline/reportcli.go
    - internal/visbaseline/reportcli_test.go
    - cmd/pdbcompat-check/redactdiff.go
    - cmd/pdbcompat-check/redactdiff_test.go
  modified:
    - cmd/pdbcompat-check/main.go
decisions:
  - Treat a missing anon page as fail-fast during RedactDir — skipping would over-disclose
  - Reject ambiguous baseline roots that mix single-target and multi-target layouts
  - Emit per-target DIFF-{target}.md alongside unified DIFF.md in multi-target mode
  - Reject -out values that resolve to "." or filesystem root
metrics:
  commits: 2
  new-files: 6
  modified-files: 1
  new-tests: 20+
  duration: ~1h
---

# Phase 57 Plan 04: Visibility baseline CLI glue — Task 1 Partial Summary

Task 1 code-only deliverables for plan 57-04: `RedactDir`, `BuildReport`,
and the `-redact` / `-diff` CLI wiring. The live operator capture against
beta.peeringdb.com (Checkpoint 1), the redact + diff commit (Task 2), the
DIFF.md review (Checkpoint 2), the prod confirmation pass (Checkpoint 3),
and the verification sweep (Task 3) are all deferred to the operator.

## Status: Partial (code landed, live capture pending operator)

The capture tool has everything it needs to run:

1. `pdbcompat-check -capture …` — existed from plan 57-02.
2. `pdbcompat-check -redact -in=… -out=…` — **new in this plan**.
3. `pdbcompat-check -diff -out=…` — **new in this plan**.

But actually running it against beta/prod requires an operator-supplied
API key and ~1h wall-clock (rate-limit bound), so it is not executed
here.

## What landed

### Library additions (`internal/visbaseline/`)

- **`redactcli.go`** — `RedactDir(ctx, RedactDirConfig)` walks a raw-auth
  staging tree, pairs each `{type}/page-N.json` with its anon counterpart
  from `AnonDir/api/{type}/page-N.json`, calls the existing `Redact`
  function on the pair, and writes the redacted bytes to
  `Dst/api/{type}/page-N.json` (dir mode `0700`, file mode `0600`). A
  missing anon pair is a fail-fast error — skipping would over-disclose.
  Empty inputs and non-directory inputs also fail fast. The walk honours
  `ctx` cancellation between files.

- **`reportcli.go`** — `BuildReport(ctx, BuildReportConfig)` walks a
  visibility-baseline root and emits `DIFF.md` + `diff.json` via the
  existing `WriteMarkdown` / `WriteJSON` emitters. It auto-detects two
  layouts:

  - **Single-target:** `BaselineRoot` directly contains `anon/` and
    `auth/` subdirs (e.g. `testdata/visibility-baseline/beta`). Emits
    one `DIFF.md`/`diff.json` whose `types` map is keyed by type name
    (e.g. `poc`).

  - **Multi-target:** `BaselineRoot` contains per-target subdirs, each
    with its own `anon/` + `auth/` (e.g.
    `testdata/visibility-baseline/` containing `beta/` and `prod/`).
    Emits a unified `DIFF.md` / `diff.json` whose `types` map keys are
    namespaced `{target}/{type}` (e.g. `beta/poc`) **and** per-target
    auxiliary `DIFF-{target}.md` files so reviewers can see one target
    at a time.

  Ambiguous roots (both forms present) are rejected. GO-CFG-1 fail-fast
  validation rejects empty `-out`, `-out="."` (CWD), and `-out="/"`
  (filesystem root). Each type's multiple pages are concatenated into a
  single envelope before `Diff` so aggregate row counts reflect totals
  across captured pages.

### CLI wiring (`cmd/pdbcompat-check/`)

- **`main.go`** — added `-redact`, `-diff`, `-in` flags plus a mutual-
  exclusion check in `run` (at most one of `-capture`, `-redact`,
  `-diff`). The default checker path is unchanged.

- **`redactdiff.go`** — `runRedact` and `runDiff` dispatch functions.
  `runRedact` derives the anon dir from the `-out` path by replacing the
  trailing `/auth` component with `/anon` (matching the capture layout),
  and rejects `-out` values whose last path component is not `auth`.
  `runDiff` treats `-out` as the baseline root and writes DIFF.md +
  diff.json at the root itself, matching phase 57 D-08 (artifact paths
  are `testdata/visibility-baseline/DIFF.md` and `…/diff.json`). Both
  install SIGINT/SIGTERM handlers.

### Test coverage

All tests use synthetic JSON fixtures built under `t.TempDir()`; no live
PeeringDB calls. Total new tests: 20+ across three files.

- `internal/visbaseline/redactcli_test.go` (10 tests)
  - Basic two-row redaction with PII leak assertions
  - Multi-type × multi-page walk
  - Missing anon pair → fail-fast
  - Required-arg validation (AuthSrc/AnonDir/Dst)
  - Non-directory input rejection
  - Empty-tree detection (no page files)
  - Context cancellation
  - `parsePagePath` table test

- `internal/visbaseline/reportcli_test.go` (10 tests)
  - Single-target end-to-end with row-count + field-delta assertions
  - Multi-target with per-target DIFF-{target}.md emission
  - Fail-fast on empty `-out`, `"/"`, `"."`
  - Empty baseline root rejected
  - Ambiguous layout rejected
  - Two-pages-concatenated aggregation
  - Single-target Targets field stamping
  - `detectShape` single/multi

- `cmd/pdbcompat-check/redactdiff_test.go` (8 tests)
  - `-redact` without `-in` / `-out`
  - `-redact` with wrong `-out` leaf
  - `-diff` without `-out`
  - Mode mutual exclusion
  - End-to-end `runRedact` happy path (derives anon dir correctly)
  - End-to-end `runDiff` happy path (writes DIFF.md + diff.json)

### Build + test status

```
TMPDIR=/tmp/claude-1000 go build ./...
(clean)

TMPDIR=/tmp/claude-1000 go vet ./...
(clean)

TMPDIR=/tmp/claude-1000 go test -race ./internal/visbaseline/... ./cmd/pdbcompat-check/... -count=1
ok    github.com/dotwaffle/peeringdb-plus/internal/visbaseline      2.044s
ok    github.com/dotwaffle/peeringdb-plus/cmd/pdbcompat-check       1.024s
```

## Commits

| # | Hash | Message |
|---|------|---------|
| 1 | bab9714 | feat(57-04): add RedactDir and BuildReport for baseline CLI |
| 2 | 147f6cc | feat(57-04): wire -redact and -diff dispatch into pdbcompat-check |

## Operator TODO

The following steps remain before plan 57-04 can be marked complete.
They require operator action (live PeeringDB access, review, commit):

### Checkpoint 1 — Live beta capture (operator, ~1h wall-clock)

Requires: `PDBPLUS_PEERINGDB_API_KEY` set to a valid beta key.

```bash
# Capture first, in one shot (~52 anon + ~52 auth requests, paced by the
# existing rate-limit client; on 429 the capture sleeps Retry-After+5s
# and retries the same tuple).
pdbcompat-check -capture -target=beta -mode=both \
  -out=testdata/visibility-baseline/beta \
  -api-key="$PDBPLUS_PEERINGDB_API_KEY"

# Capture writes:
#   testdata/visibility-baseline/beta/anon/api/{type}/page-{1,2}.json  (COMMITTABLE)
#   /tmp/pdb-vis-capture-XXXX/auth/api/{type}/page-{1,2}.json          (PRIVATE; do not commit)
#
# The final stdout line prints the /tmp path that holds raw auth bytes.
# Note it — the redact step below needs it as -in.
```

### Task 2 — Redact + diff (operator, seconds once capture is done)

```bash
# Replace /tmp/pdb-vis-capture-XXXX with the path printed by -capture.
pdbcompat-check -redact \
  -in=/tmp/pdb-vis-capture-XXXX/auth \
  -out=testdata/visibility-baseline/beta/auth

pdbcompat-check -diff -out=testdata/visibility-baseline/
#   → writes testdata/visibility-baseline/DIFF.md
#           testdata/visibility-baseline/diff.json
#           testdata/visibility-baseline/DIFF-beta.md (multi-target mode)
```

### Checkpoint 2 — Review DIFF.md (operator)

Human review of `testdata/visibility-baseline/DIFF.md` before any
commit. Confirm:

- No raw email/phone/name values visible in any committed fixture.
- `<auth-only:string>` placeholders present for auth-only fields.
- Expected per-type auth-only field list (email, phone, name on `poc`
  at minimum; address1/2/city/state/zipcode on `org`/`fac`; etc.).
- Row counts look right (within an order of magnitude of PeeringDB's
  live data — e.g. `poc` anon rows ≈ hundreds, auth rows slightly
  higher due to `Users`-visible POCs).

### Checkpoint 3 — Prod confirmation (operator, optional but recommended)

D-04 says: run prod only for `poc`, `org`, `net` and only if a prod
API key is available (or anon-only if not). Same capture/redact/diff
cycle with `-target=prod`:

```bash
# With prod auth (requires -prod-auth flag to confirm intent):
pdbcompat-check -capture -target=prod -mode=both -prod-auth \
  -out=testdata/visibility-baseline/prod \
  -api-key="$PDBPLUS_PEERINGDB_API_KEY"

# Or anon-only (no -prod-auth, no API key needed):
pdbcompat-check -capture -target=prod -mode=anon \
  -out=testdata/visibility-baseline/prod

# Then redact (skip if anon-only), and re-run -diff against the parent:
pdbcompat-check -diff -out=testdata/visibility-baseline/
#   → now emits unified DIFF.md + DIFF-beta.md + DIFF-prod.md
```

### Task 3 — Verification sweep (operator)

After all captures + redactions are on disk:

```bash
TMPDIR=/tmp/claude-1000 go test -race ./internal/visbaseline/... -count=1
```

The critical assertion is `TestCommittedFixturesHaveNoPII` (in
`fixtures_guard_test.go`), which walks every JSON file under
`testdata/visibility-baseline/**/auth/` and fails if any field in
`PIIFields` has a non-placeholder, non-null string value. This test was
introduced in plan 57-03 and skips itself when fixtures are absent; it
must pass once fixtures land.

Also run the full suite once fixtures are committed:

```bash
TMPDIR=/tmp/claude-1000 go test -race ./...
```

## How to resume

> Run `pdbcompat-check -capture -target=beta -mode=both -out=/tmp/beta-raw -api-key=$PDBPLUS_PEERINGDB_API_KEY`; then `pdbcompat-check -redact -in=/tmp/beta-raw/auth -out=testdata/visibility-baseline/beta/auth`; then `pdbcompat-check -diff -out=testdata/visibility-baseline/`. Review DIFF.md, then prod confirmation, then run `go test ./internal/visbaseline/...` to confirm PII guard passes before commit.

## Deviations from Plan

None for Task 1 — the plan's stated Task 1 scope (code only, no live
capture) was executed as written. Plan 57-04 itself is not committed to
the repo (only CONTEXT/RESEARCH/SUMMARY files from prior plans exist);
the orchestrator prompt's `<scope_limit>` block was treated as the
authoritative Task 1 specification.

## Self-Check: PASSED

Verified:

- `internal/visbaseline/redactcli.go` exists
- `internal/visbaseline/reportcli.go` exists
- `cmd/pdbcompat-check/redactdiff.go` exists
- All three corresponding `_test.go` files exist
- Commits `bab9714` and `147f6cc` in git log
- `go build ./...` clean
- `go vet ./...` clean
- `go test -race ./internal/visbaseline/... ./cmd/pdbcompat-check/...` all pass
