---
phase: 13
slug: foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 13 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing Go test infrastructure |
| **Quick run command** | `go test -race ./internal/web/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/web/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 13-01-01 | 01 | 1 | DSGN-01 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 13-01-02 | 01 | 1 | DSGN-02 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 13-01-03 | 01 | 1 | DSGN-03 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/web/handler_test.go` — stubs for DSGN-01, DSGN-02, DSGN-03
- [ ] templ generate produces no drift
- [ ] go build ./... succeeds with embedded static assets

*Existing Go test infrastructure covers framework needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Layout adapts on mobile/tablet/desktop | DSGN-02 | Tailwind CDN responsive classes need browser | Resize browser window to 375px, 768px, 1024px widths |
| Tailwind styles render correctly | DSGN-01 | CDN-based styling requires browser execution | Open /ui/ in browser, verify neon green on dark theme |
| Clean bookmarkable URLs | DSGN-03 | URL behavior needs browser address bar | Navigate to /ui/, reload page, verify same content |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
