---
phase: 33
slug: grpc-dedup-filter-parity
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 33 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test -race ./internal/grpcserver/... ./internal/middleware/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~45 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/grpcserver/... ./internal/middleware/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 45 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 33-01-01 | 01 | 1 | QUAL-01 | unit | `go test -race -cover ./internal/grpcserver/...` | ✅ | ⬜ pending |
| 33-02-01 | 02 | 2 | ARCH-02 | integration | `go test -race ./internal/grpcserver/... -run TestFilter` | ❌ W0 | ⬜ pending |
| 33-03-01 | 03 | 2 | QUAL-03 | coverage | `go test -race -cover ./internal/grpcserver/... ./internal/middleware/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing test infrastructure covers generic helper unit tests.
- New tests needed for filter parity verification and coverage targets.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Line count reduction ≥800 | QUAL-01 | Requires comparison against v1.8 baseline | `wc -l internal/grpcserver/*.go` and compare with baseline |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 45s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
