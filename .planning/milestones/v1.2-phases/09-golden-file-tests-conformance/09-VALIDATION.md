---
phase: 9
slug: golden-file-tests-conformance
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-23
---

# Phase 9 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) + go-cmp |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test ./internal/pdbcompat/... -count=1` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/pdbcompat/... -count=1`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 09-01-01 | 01 | 1 | GOLD-01 | unit | `go test ./internal/pdbcompat/... -run TestGolden -count=1` | ❌ W0 | ⬜ pending |
| 09-01-02 | 01 | 1 | GOLD-02,GOLD-03 | unit | `go test ./internal/pdbcompat/... -run TestGolden -count=1` | ❌ W0 | ⬜ pending |
| 09-01-03 | 01 | 1 | GOLD-04 | unit | `go test ./internal/pdbcompat/... -run TestGoldenDepth -count=1` | ❌ W0 | ⬜ pending |
| 09-02-01 | 02 | 2 | CONF-01 | unit | `go build ./cmd/pdbconform/...` | ❌ W0 | ⬜ pending |
| 09-02-02 | 02 | 2 | CONF-02 | integration | `go test ./internal/pdbcompat/... -run TestConformance -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Golden file test helpers (compareGolden, updateGolden)
- [ ] Deterministic test data setup (setupGoldenTestData)
- [ ] go-cmp promoted to direct dependency

*Existing test infrastructure covers framework needs. Only golden-file-specific helpers and test data needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live conformance against beta.peeringdb.com | CONF-02 | Requires network access | Run `go test ./internal/pdbcompat/... -peeringdb-live -count=1` |

*Most behaviors have automated verification via deterministic test data.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
