---
phase: 42
slug: test-quality-audit-coverage-hygiene
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 42 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | .octocov.yml |
| **Quick run command** | `go test -race ./internal/pdbcompat/...` |
| **Full suite command** | `go test -race ./... && go test -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/...` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race` on modified packages
- **After every plan wave:** Run full suite
- **Before `/gsd:verify-work`:** Full suite + fuzz test must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 42-01-01 | 01 | 1 | QUAL-01 | audit | `grep -r 'func Test' *_test.go` | ✅ | ⬜ pending |
| 42-01-02 | 01 | 1 | QUAL-03 | fuzz | `go test -fuzz=FuzzFilterParser -fuzztime=30s ./internal/pdbcompat/...` | ❌ W0 | ⬜ pending |
| 42-02-01 | 02 | 1 | QUAL-02 | audit | `go test -race -cover ./...` | ✅ | ⬜ pending |
| 42-02-02 | 02 | 1 | INFRA-02 | config | `cat .octocov.yml` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/pdbcompat/filter_fuzz_test.go` — fuzz test for filter parser

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
