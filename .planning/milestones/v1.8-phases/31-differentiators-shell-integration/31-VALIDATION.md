---
phase: 31
slug: differentiators-shell-integration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 31 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test ./internal/web/termrender/...` |
| **Full suite command** | `go test -race ./internal/web/...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/termrender/...`
- **After every plan wave:** Run `go test -race ./internal/web/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 31-01-01 | 01 | 1 | DIF-01 | unit | `go test ./internal/web/termrender/ -run TestRenderShort` | ❌ W0 | ⬜ pending |
| 31-01-02 | 01 | 1 | DIF-02 | unit | `go test ./internal/web/ -run TestFreshness` | ❌ W0 | ⬜ pending |
| 31-02-01 | 02 | 2 | DIF-03 | unit | `go test ./internal/web/termrender/ -run TestSectionFilter` | ❌ W0 | ⬜ pending |
| 31-02-02 | 02 | 2 | DIF-04 | unit | `go test ./internal/web/termrender/ -run TestWidthAdapt` | ❌ W0 | ⬜ pending |
| 31-03-01 | 03 | 2 | SHL-01, SHL-02 | unit | `go test ./internal/web/ -run TestCompletion` | ❌ W0 | ⬜ pending |
| 31-03-02 | 03 | 2 | SHL-03 | integration | `go test ./internal/web/ -run TestCompletionSearch` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing infrastructure covers all phase requirements. Tests follow established termrender patterns.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Shell completion install | SHL-01, SHL-02 | Requires interactive shell | `eval "$(curl -s localhost:8080/ui/completions/bash)"` then `pdb <TAB>` |
| Width adaptation visual | DIF-04 | Visual column layout check | `curl 'localhost:8080/ui/asn/13335?w=80'` vs `?w=120` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
