---
phase: 30
slug: entity-types-search-formats
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 30 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure from Phases 28-29 |
| **Quick run command** | `go test ./internal/web/templates/termrender/...` |
| **Full suite command** | `go test -race ./internal/web/... ./internal/pdbcompat/...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/web/templates/termrender/...`
- **After every plan wave:** Run `go test -race ./internal/web/... ./internal/pdbcompat/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 30-01-01 | 01 | 1 | RND-03 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderIXDetail` | ❌ W0 | ⬜ pending |
| 30-01-02 | 01 | 1 | RND-04 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderFacilityDetail` | ❌ W0 | ⬜ pending |
| 30-01-03 | 01 | 1 | RND-05 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderOrgDetail` | ❌ W0 | ⬜ pending |
| 30-01-04 | 01 | 1 | RND-06 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderCampusDetail` | ❌ W0 | ⬜ pending |
| 30-01-05 | 01 | 1 | RND-07 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderCarrierDetail` | ❌ W0 | ⬜ pending |
| 30-02-01 | 02 | 1 | RND-08 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderSearch` | ❌ W0 | ⬜ pending |
| 30-02-02 | 02 | 1 | RND-09 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderCompare` | ❌ W0 | ⬜ pending |
| 30-03-01 | 03 | 2 | RND-10, RND-17 | unit | `go test ./internal/web/templates/termrender/ -run TestRenderWHOIS` | ❌ W0 | ⬜ pending |
| 30-03-02 | 03 | 2 | RND-11 | integration | `go test ./internal/web/ -run TestFormat` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing infrastructure covers all phase requirements. Tests follow established patterns from Phase 29 (`termrender_test.go`).

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| ANSI color rendering | RND-03-07 | Visual verification of lipgloss styling | `curl localhost:8080/ui/ix/1` in terminal with color support |
| WHOIS format readability | RND-10 | Subjective formatting assessment | `curl 'localhost:8080/ui/asn/13335?format=whois'` and verify RPSL-like output |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
