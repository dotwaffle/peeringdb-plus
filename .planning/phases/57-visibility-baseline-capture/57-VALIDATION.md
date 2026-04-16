---
phase: 57
slug: visibility-baseline-capture
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-16
---

# Phase 57 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` (project convention) |
| **Config file** | none — stdlib `go test` discovers `*_test.go` |
| **Quick run command** | `go test -race ./internal/visbaseline/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~5 seconds (quick), ~90 seconds (full) |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/visbaseline/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd-verify-work`:** Full suite must be green + manual `pdbcompat-check -capture -target=beta -mode=both` run by operator
- **Max feedback latency:** 5 seconds for unit/integration tests (httptest, no live network)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 57-01-01 | 01 | 1 | VIS-01 | T-57-01 | Raw anon fixture bytes committed unchanged | unit | `go test -run TestCaptureWritesRawAnonBytes ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-01-02 | 01 | 1 | VIS-01 | T-57-02 | No PII values in redacted auth fixtures | unit | `go test -run TestRedactionStripsPII ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-01-03 | 01 | 1 | VIS-01 | T-57-03 | Redaction deterministic (same input → same bytes) | unit | `go test -run TestRedactionDeterministic ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-01-04 | 01 | 1 | VIS-01 | T-57-03 | Redaction handles auth-only rows | unit | `go test -run TestRedactionAuthOnlyRow ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-01-05 | 01 | 1 | VIS-01 | T-57-03 | Redaction handles auth-only fields | unit | `go test -run TestRedactionAuthOnlyField ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-01-06 | 01 | 1 | VIS-01 | T-57-06 | Capture loop honours RateLimitError.RetryAfter | unit | `go test -run TestCaptureRespectsRateLimit ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-02-01 | 02 | 2 | VIS-01 | T-57-04 | Checkpoint round-trip persists state | smoke | `go test -run TestCheckpointRoundTrip ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-02-02 | 02 | 2 | VIS-01 | T-57-04 | Resume skips completed tuples | smoke | `go test -run TestCheckpointResumeSkipsDoneTuples ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-02-03 | 02 | 2 | VIS-01 | T-57-04 | Prompt safe defaults on corrupted/EOF state | unit | `go test -run TestCheckpointPromptSafeDefaults ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-03-01 | 03 | 2 | VIS-02 | — | Diff: identical structure → empty output | integration | `go test -run TestDiffNoDeltas ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-03-02 | 03 | 2 | VIS-02 | T-57-02 | Diff: auth-only fields emit placeholder only | integration | `go test -run TestDiffAuthOnlyField ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-03-03 | 03 | 2 | VIS-02 | — | Diff: row-count drift reported | integration | `go test -run TestDiffRowCountDrift ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-03-04 | 03 | 2 | VIS-02 | — | DIFF.md and diff.json consistent | integration | `go test -run TestReportConsistency ./internal/visbaseline/` | ❌ W0 | ⬜ pending |
| 57-03-05 | 03 | 2 | VIS-02 | T-57-02 | PII guard: no real PII in committed auth fixtures | integration | `go test -run TestCommittedFixturesHaveNoPII ./internal/visbaseline/` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/visbaseline/capture.go` + `capture_test.go` — Capture type, httptest fixtures
- [ ] `internal/visbaseline/checkpoint.go` + `checkpoint_test.go` — atomic-rename state file, resume/restart prompt
- [ ] `internal/visbaseline/redact.go` + `redact_test.go` — pure-function redactor
- [ ] `internal/visbaseline/diff.go` + `diff_test.go` — structural diff walker
- [ ] `internal/visbaseline/report.go` + `report_test.go` — Markdown + JSON emitters
- [ ] `internal/visbaseline/pii.go` — PII allow-list shared by redactor and guard test
- [ ] `internal/visbaseline/testdata/` — synthetic anon/auth JSON pairs (fully fake, not real PeeringDB data)

No framework install required — `go test` and `net/http/httptest` are stdlib.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Full beta walk produces committed fixtures across all 13 types | VIS-01 | Rate-limit bound ≥ 1 hour wall-clock; requires real PeeringDB access | Run `pdbcompat-check -capture -target=beta -mode=both -out=testdata/visibility-baseline/beta` with `PDBPLUS_PEERINGDB_API_KEY` set. Review committed fixtures and DIFF.md before commit. |
| Prod confirmation pass for poc/org/net | VIS-01 | Requires prod rate-limit budget; high-signal types only | Run `pdbcompat-check -capture -target=prod -types=poc,org,net -out=testdata/visibility-baseline/prod`. Confirm beta vs prod shape agrees. |
| DIFF.md is reviewable in code review | VIS-02 | Human judgment on structural legibility | Operator opens DIFF.md in an editor or on GitHub PR view, confirms per-type tables render correctly. |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s (quick) / < 90s (full)
- [ ] `nyquist_compliant: true` set in frontmatter after Wave 0 lands

**Approval:** pending
