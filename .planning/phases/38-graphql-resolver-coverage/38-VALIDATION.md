---
phase: 38
slug: graphql-resolver-coverage
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-26
---

# Phase 38 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test toolchain |
| **Quick run command** | `go test -race ./graph/...` |
| **Full suite command** | `go test -race -coverprofile=cover.out ./graph/... && go tool cover -func=cover.out` |
| **Estimated runtime** | ~20 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./graph/...`
- **After every plan wave:** Run `go test -race -coverprofile=cover.out ./graph/...`
- **Before `/gsd:verify-work`:** Full suite must be green with 80%+ on hand-written files
- **Max feedback latency:** 20 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 38-01-01 | 01 | 1 | GQL-01 | integration | `go test -race -run TestListResolvers ./graph/...` | ❌ W0 | ⬜ pending |
| 38-01-02 | 01 | 1 | GQL-02 | integration | `go test -race -run TestErrorPaths ./graph/...` | ❌ W0 | ⬜ pending |
| 38-01-03 | 01 | 1 | GQL-03 | integration | `go tool cover -func=cover.out` | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `graph/resolver_test.go` — expanded test functions for all 13 resolvers and error paths

*Existing infrastructure covers test setup (testutil.SetupClient + seed.Full).*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 20s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
