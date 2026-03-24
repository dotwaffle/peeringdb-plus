---
phase: 6
slug: peeringdb-compatibility-layer
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-22
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing go test infrastructure |
| **Quick run command** | `go test ./internal/pdbcompat/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~20 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/pdbcompat/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 20 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | PDBCOMPAT-01, PDBCOMPAT-02, PDBCOMPAT-05 | unit | `go test ./internal/pdbcompat/... -run TestFilter` | ❌ W0 | ⬜ pending |
| 06-01-02 | 01 | 1 | PDBCOMPAT-01 | unit | `go test ./internal/pdbcompat/... -run TestSerializer` | ❌ W0 | ⬜ pending |
| 06-02-01 | 02 | 2 | PDBCOMPAT-01, PDBCOMPAT-04, PDBCOMPAT-05 | integration | `go test ./internal/pdbcompat/... -run TestHandler` | ❌ W0 | ⬜ pending |
| 06-02-02 | 02 | 2 | PDBCOMPAT-01 | build | `go build ./cmd/peeringdb-plus/` | ✅ | ⬜ pending |
| 06-03-01 | 03 | 3 | PDBCOMPAT-03 | unit | `go test ./internal/pdbcompat/... -run TestDepth` | ❌ W0 | ⬜ pending |
| 06-03-02 | 03 | 3 | PDBCOMPAT-02 | unit | `go test ./internal/pdbcompat/... -run TestSearch` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/pdbcompat/` directory created (Plan 01 creates this)
- [ ] Test files created as part of TDD approach in each plan task

*Test infrastructure exists (go test). Package and test files are created by each plan's TDD tasks.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Response matches real PeeringDB API | PDBCOMPAT-01 | Requires comparison against live PeeringDB | Fetch from both APIs, diff responses |
| Existing PeeringDB consumers work | PDBCOMPAT-01 | Requires real consumer tools | Point peeringdb-py at compat endpoint |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 20s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
