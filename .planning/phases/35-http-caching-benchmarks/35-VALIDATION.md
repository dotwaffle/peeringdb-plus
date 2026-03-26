---
phase: 35
slug: http-caching-benchmarks
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 35 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test -race ./internal/middleware/... ./internal/web/... ./internal/pdbcompat/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 35-01-01 | 01 | 1 | PERF-02 | unit | `go test -race ./internal/middleware/... -run TestCaching` | ❌ W0 | ⬜ pending |
| 35-02-01 | 02 | 1 | PERF-04 | bench | `go test -bench=. -benchmem -count=3 ./internal/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- New tests needed for caching middleware and benchmark files.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Cache-Control/ETag headers on live API | PERF-02 | Requires running server | `curl -v /api/net` and check response headers |
| 304 Not Modified response | PERF-02 | Requires conditional request | `curl -H "If-None-Match: <etag>" /api/net` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
