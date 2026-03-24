---
phase: 18
slug: tech-debt-data-integrity
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 18 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test infrastructure |
| **Quick run command** | `go test ./internal/peeringdb/... ./internal/sync/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/peeringdb/... ./internal/sync/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 18-01-01 | 01 | 1 | DEBT-01 | grep | `grep -r IsPrimary internal/sync/worker.go` | ✅ | ⬜ pending |
| 18-01-02 | 01 | 1 | DEBT-02 | compile | `go build ./...` | ✅ | ⬜ pending |
| 18-02-01 | 02 | 1 | DATA-01 | integration | `go test -run TestMetaGenerated -peeringdb-live ./internal/peeringdb/...` | ❌ W0 | ⬜ pending |
| 18-02-02 | 02 | 1 | DATA-02 | unit | `go test -run TestFetchMeta ./internal/peeringdb/...` | ✅ | ⬜ pending |
| 18-02-03 | 02 | 1 | DATA-03 | unit | `go test -run TestParseMeta ./internal/peeringdb/...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/peeringdb/client_live_test.go` — flag-gated live integration test for meta.generated behavior

*Existing infrastructure covers most phase requirements. Only the live integration test file is new.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Planning docs accuracy | DEBT-02 | Requires human review of doc correctness | Review corrected planning docs against actual codebase state |

*Most behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
