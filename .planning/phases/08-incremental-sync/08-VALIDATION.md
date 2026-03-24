---
phase: 8
slug: incremental-sync
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-23
---

# Phase 8 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test ./internal/sync/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/sync/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 08-01-01 | 01 | 1 | SYNC-03 | unit | `go test ./internal/sync/... -run TestCursor` | ❌ W0 | ⬜ pending |
| 08-01-02 | 01 | 1 | SYNC-01 | unit | `go test ./internal/sync/... -run TestFetchOption` | ❌ W0 | ⬜ pending |
| 08-02-01 | 02 | 2 | SYNC-01,SYNC-04 | unit | `go test ./internal/sync/... -run TestIncremental` | ❌ W0 | ⬜ pending |
| 08-02-02 | 02 | 2 | SYNC-02 | unit | `go test ./internal/sync/... -run TestFullSync` | ❌ W0 | ⬜ pending |
| 08-02-03 | 02 | 2 | SYNC-05 | unit | `go test ./internal/sync/... -run TestFirstSync` | ❌ W0 | ⬜ pending |
| 08-03-01 | 03 | 2 | SYNC-01 | integration | `go test ./internal/sync/... -run TestConfig` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Test stubs for cursor storage (SYNC-03)
- [ ] Test stubs for incremental fetch with ?since= (SYNC-01)
- [ ] Test stubs for fallback on failure (SYNC-04)
- [ ] Test stubs for first-sync detection (SYNC-05)

*Existing test infrastructure covers framework needs. Only test files need creation.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live PeeringDB ?since= behavior | SYNC-01 | Requires network access to beta.peeringdb.com | Verify ?since= returns only modified objects with manual API call |

*Most behaviors have automated verification via mocked HTTP responses.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
