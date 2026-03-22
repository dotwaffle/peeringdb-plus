---
phase: 03
slug: production-readiness
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-22
---

# Phase 03 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — standard Go testing |
| **Quick run command** | `go build ./...` |
| **Full suite command** | `go test -race -count=1 ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go build ./...`
- **After every plan wave:** Run `go test -race -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 03-01-T1 | 01 | 1 | OPS-01, OPS-02 | build | `go build ./internal/otel/...` | ✅ | ⬜ pending |
| 03-01-T2 | 01 | 1 | OPS-03 | build | `go build ./internal/...` | ✅ | ⬜ pending |
| 03-02-T1 | 02 | 2 | OPS-04, OPS-05 | build | `go build ./cmd/peeringdb-plus/...` | ✅ | ⬜ pending |
| 03-02-T2 | 02 | 2 | STOR-02 | build | `go build ./...` | ✅ | ⬜ pending |
| 03-03-T1 | 03 | 3 | OPS-01 thru OPS-05 | test | `go test -race ./...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- Existing test infrastructure covers Phase 3 requirements (go test, existing test files)
- Integration tests created in final wave plan

*Existing infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Multi-region edge latency | STOR-02 | Requires Fly.io deployment | Deploy, query from different regions, measure latency |
| LiteFS replication | STOR-02 | Requires Fly.io cluster | Write on primary, read on replica, verify consistency |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
