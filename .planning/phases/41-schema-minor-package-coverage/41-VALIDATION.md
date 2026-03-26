---
phase: 41
slug: schema-minor-package-coverage
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 41 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test toolchain |
| **Quick run command** | `go test -race ./internal/otel/... ./internal/health/... ./internal/peeringdb/...` |
| **Full suite command** | `go test -race -cover ./internal/otel/... ./internal/health/... ./internal/peeringdb/... ./ent/schema/...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd:verify-work`:** Full suite must be green with 90%+ on minor packages, 65%+ on schema
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 41-01-01 | 01 | 1 | SCHEMA-01 | integration | `go test -race -run TestOtelMutationHook ./internal/otel/...` | ✅ | ⬜ pending |
| 41-01-02 | 01 | 1 | SCHEMA-02 | integration | `go test -race -run TestFKConstraint ./ent/schema/...` | ❌ W0 | ⬜ pending |
| 41-01-03 | 01 | 1 | SCHEMA-03, MINOR-01 | integration | `go test -race -cover ./internal/otel/...` | ✅ | ⬜ pending |
| 41-01-04 | 01 | 1 | MINOR-02 | integration | `go test -race -cover ./internal/health/...` | ✅ | ⬜ pending |
| 41-01-05 | 01 | 1 | MINOR-03 | integration | `go test -race -cover ./internal/peeringdb/...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

*Existing infrastructure covers most requirements. Schema FK tests need new test file.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
