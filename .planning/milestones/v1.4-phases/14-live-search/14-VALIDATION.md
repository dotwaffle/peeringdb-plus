---
phase: 14
slug: live-search
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 14 — Validation Strategy

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
| 14-01-01 | 01 | 1 | SRCH-01 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 14-01-02 | 01 | 1 | SRCH-02 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 14-01-03 | 01 | 1 | SRCH-03 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |
| 14-01-04 | 01 | 1 | SRCH-04 | unit | `go test -race ./internal/web/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Search handler tests added to `internal/web/handler_test.go` or `internal/web/search_test.go`
- [ ] Tests cover grouped results, count badges, ASN lookup, empty query handling

*Existing Go test infrastructure covers framework needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Search results update as user types within 300ms | SRCH-01 | Requires browser with htmx execution | Type in search box, observe results update live |
| Clicking result navigates to detail page | SRCH-01 | Requires browser navigation | Click a search result, verify URL changes |
| ASN direct redirect on Enter with numeric input | SRCH-03 | Requires form submit in browser | Type "13335" and press Enter |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
