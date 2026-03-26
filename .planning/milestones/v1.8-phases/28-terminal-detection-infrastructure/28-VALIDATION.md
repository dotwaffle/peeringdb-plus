---
phase: 28
slug: terminal-detection-infrastructure
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-25
---

# Phase 28 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test -race ./internal/web/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/web/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 28-01-01 | 01 | 1 | DET-01 | unit | `go test -run TestIsTerminalClient ./internal/web/...` | ❌ W0 | ⬜ pending |
| 28-01-02 | 01 | 1 | DET-02,DET-03 | unit | `go test -run TestFormatDetection ./internal/web/...` | ❌ W0 | ⬜ pending |
| 28-01-03 | 01 | 1 | DET-04 | unit | `go test -run TestAcceptHeader ./internal/web/...` | ❌ W0 | ⬜ pending |
| 28-01-04 | 01 | 1 | DET-05 | integration | `go test -run TestContentNegotiation ./internal/web/...` | ❌ W0 | ⬜ pending |
| 28-02-01 | 02 | 1 | RND-01 | unit | `go test -run TestANSIRender ./internal/web/termrender/...` | ❌ W0 | ⬜ pending |
| 28-02-02 | 02 | 1 | RND-18 | unit | `go test -run TestNoColor ./internal/web/termrender/...` | ❌ W0 | ⬜ pending |
| 28-03-01 | 03 | 2 | NAV-01 | integration | `go test -run TestHelpText ./internal/web/...` | ❌ W0 | ⬜ pending |
| 28-03-02 | 03 | 2 | NAV-02,NAV-03 | integration | `go test -run TestTerminalErrors ./internal/web/...` | ❌ W0 | ⬜ pending |
| 28-03-03 | 03 | 2 | NAV-04 | integration | `go test -run TestRootTerminal ./internal/web/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/web/handler_test.go` — terminal detection and content negotiation tests
- [ ] `internal/web/termrender/render_test.go` — ANSI rendering tests

*Existing go test infrastructure covers framework needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| ANSI colors render correctly in terminal | RND-01 | Visual verification — automated tests check escape codes present but not visual appearance | `curl peeringdb-plus.fly.dev/ui/asn/13335` and verify colored output |
| Browser unchanged | DET-05 | Browser rendering is visual | Open same URL in browser, verify HTML/CSS unchanged |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
