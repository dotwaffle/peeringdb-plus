---
phase: 37
slug: test-seed-infrastructure
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 37 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test toolchain |
| **Quick run command** | `go test -race ./internal/testutil/seed/...` |
| **Full suite command** | `go test -race ./internal/testutil/seed/... ./graph/... ./internal/grpcserver/... ./internal/web/...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/testutil/seed/...`
- **After every plan wave:** Run `go test -race ./internal/testutil/seed/... ./graph/... ./internal/grpcserver/... ./internal/web/...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 37-01-01 | 01 | 1 | INFRA-01 | unit | `go test -race ./internal/testutil/seed/...` | ❌ W0 | ⬜ pending |
| 37-01-02 | 01 | 1 | INFRA-01 | integration | `go test -race -run TestSeedFull ./internal/testutil/seed/...` | ❌ W0 | ⬜ pending |
| 37-01-03 | 01 | 1 | INFRA-01 | integration | `go test -race -run TestSeedMinimal ./internal/testutil/seed/...` | ❌ W0 | ⬜ pending |
| 37-01-04 | 01 | 1 | INFRA-01 | integration | `go test -race -run TestSeedNetworks ./internal/testutil/seed/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/testutil/seed/seed.go` — seed package with Full, Minimal, Networks functions
- [ ] `internal/testutil/seed/seed_test.go` — tests for seed functions

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Import cycle freedom | INFRA-01 SC3 | Requires checking compilation across 3+ packages | `go build ./graph/... ./internal/grpcserver/... ./internal/web/...` must succeed |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
