---
phase: 43
slug: dense-tables-with-sorting-and-flags
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 43 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test ./internal/web/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | DENS-01 | integration | `go test ./internal/web/ -run TestFragments` | ✅ | ⬜ pending |
| TBD | TBD | TBD | DENS-02 | integration | `go test ./internal/web/ -run TestFragments` | ✅ | ⬜ pending |
| TBD | TBD | TBD | DENS-03 | integration | `go test ./internal/web/ -run TestFragments` | ✅ | ⬜ pending |
| TBD | TBD | TBD | SORT-01 | unit | `go test ./internal/web/...` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SORT-02 | unit | `go test ./internal/web/...` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | SORT-03 | integration | `go test ./internal/web/ -run TestFragments` | ✅ | ⬜ pending |
| TBD | TBD | TBD | FLAG-01 | integration | `go test ./internal/web/ -run TestFragments` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing test infrastructure covers all phase requirements (`detail_test.go` has fragment tests for all entity types)
- Sort JS validation requires assertion updates in existing fragment tests (check for `<table>`, `<th>`, `data-sort-value`)

*Existing infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Responsive column hiding at 768px breakpoint | DENS-03 | CSS media query behavior cannot be tested in Go integration tests | Resize browser below 768px, verify low-priority columns hide |
| Sort arrow visual indicator | SORT-02 | CSS pseudo-element rendering not testable server-side | Click column header, verify arrow appears and direction changes |
| Country flag SVG rendering | FLAG-01 | CSS class application testable, but visual flag rendering requires browser | Load page with country data, verify flag icons render correctly |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
