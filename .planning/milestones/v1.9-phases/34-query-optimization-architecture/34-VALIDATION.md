---
phase: 34
slug: query-optimization-architecture
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 34 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test -race ./internal/web/... ./internal/pdbcompat/... ./internal/httperr/...` |
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
| 34-01-01 | 01 | 1 | PERF-01 | unit | `go test -race ./internal/web/... -run TestSearch` | ✅ | ⬜ pending |
| 34-01-02 | 01 | 1 | PERF-03 | unit | `go test -race ./ent/... -run TestIndex` | ❌ W0 | ⬜ pending |
| 34-02-01 | 02 | 1 | PERF-05 | unit | `go test -race ./internal/pdbcompat/...` | ✅ | ⬜ pending |
| 34-03-01 | 03 | 2 | ARCH-01 | integration | `go test -race ./internal/httperr/...` | ❌ W0 | ⬜ pending |
| 34-04-01 | 04 | 2 | ARCH-04 | unit | `go test -race ./internal/web/...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- New tests needed for index verification and httperr package.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Error format across all 6 surfaces | ARCH-01 | Requires hitting all API surfaces | Send malformed request to each surface, verify error JSON structure |
| OTel trace span count for search | PERF-01 | Requires OTel collector | Run search, check trace for query count per type |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
