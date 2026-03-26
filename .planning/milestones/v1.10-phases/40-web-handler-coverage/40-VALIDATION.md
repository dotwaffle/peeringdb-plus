---
phase: 40
slug: web-handler-coverage
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 40 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test toolchain |
| **Quick run command** | `go test -race ./internal/web/...` |
| **Full suite command** | `go test -race -cover ./internal/web/...` |
| **Estimated runtime** | ~20 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/web/...`
- **After every plan wave:** Run `go test -race -cover ./internal/web/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 20 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 40-01-01 | 01 | 1 | WEB-01 | integration | `go test -race -run TestFragments ./internal/web/...` | ✅ | ⬜ pending |
| 40-01-02 | 01 | 1 | WEB-02 | integration | `go test -race -run TestRenderPage ./internal/web/...` | ✅ | ⬜ pending |
| 40-01-03 | 01 | 1 | WEB-03 | unit | `go test -race -run 'TestExtractID\|TestGetFreshness\|TestHandleServerError' ./internal/web/...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

*Existing infrastructure covers all phase requirements (seedAllTestData + testutil.SetupClientWithDB + sync.InitStatusTable).*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 20s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
