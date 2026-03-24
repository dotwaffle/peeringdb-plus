---
phase: 15
slug: record-detail
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 15 — Validation Strategy

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
| 15-01-01 | 01 | 1 | DETL-01 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 15-01-02 | 01 | 1 | DETL-04 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 15-02-01 | 02 | 2 | DETL-02, DETL-03 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 15-02-02 | 02 | 2 | DETL-05 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Detail page handler tests for all 6 entity types
- [ ] Fragment endpoint tests for lazy-loaded sections
- [ ] Tests verify cross-linking between related records

*Existing Go test infrastructure covers framework needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Collapsible sections expand/collapse smoothly | DETL-02 | Requires browser with htmx/JS execution | Click `<details>` element, observe content loads |
| Lazy loading triggers on first expand only | DETL-03 | Requires browser htmx lifecycle | Expand section, check network tab, collapse/re-expand |
| Summary stats visible in header area | DETL-04 | Visual layout verification | Check network detail page shows "Present at X IXPs, Y Facilities" |
| Cross-links navigate correctly | DETL-05 | Requires browser navigation | Click related record link, verify URL changes to detail page |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
