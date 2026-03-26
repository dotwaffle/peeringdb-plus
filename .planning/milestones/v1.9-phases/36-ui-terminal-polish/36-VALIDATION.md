---
phase: 36
slug: ui-terminal-polish
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 36 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test -race ./internal/web/... ./internal/web/termrender/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~45 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 45 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 36-01-01 | 01 | 1 | UI-01,UI-02 | grep | `grep -c 'text-neutral-400' internal/web/templates/*.templ` | ✅ | ⬜ pending |
| 36-01-02 | 01 | 1 | UI-03,UI-04 | grep | `grep -c 'aria-expanded' internal/web/templates/layout.templ` | ✅ | ⬜ pending |
| 36-02-01 | 02 | 1 | UI-05 | grep | `grep -c 'hx-push-url' internal/web/templates/*.templ` | ✅ | ⬜ pending |
| 36-02-02 | 02 | 1 | UI-06 | grep | `grep -c 'htmx:afterRequest' internal/web/templates/layout.templ` | ✅ | ⬜ pending |
| 36-03-01 | 03 | 1 | TUI-01,TUI-02 | unit | `go test -race ./internal/web/termrender/...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing infrastructure covers all phase requirements.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| WCAG AA contrast ratio 4.5:1 | UI-01 | Requires visual contrast analyzer | Inspect dark mode text colors with browser dev tools |
| Screen reader navigation | UI-03 | Requires assistive technology | Test with NVDA/VoiceOver on nav, menu, search |
| Breadcrumb display | UI-07 | Visual layout check | Navigate to detail page, verify breadcrumb path |
| Mobile menu closes on link | UI-07 | Requires mobile viewport | Resize browser, test menu behavior |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 45s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
