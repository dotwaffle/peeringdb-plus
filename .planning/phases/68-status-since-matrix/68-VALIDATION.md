---
phase: 68
slug: status-since-matrix
status: locked
nyquist_compliant: true
wave_0_complete: true
created: 2026-04-19
---

# Phase 68 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | `go test` (stdlib) — table-driven + in-memory SQLite via `internal/testutil.SetupClient` |
| **Config file** | `.golangci.yml` (lint), no separate test config |
| **Quick run command** | `go test -race ./internal/pdbcompat/... ./internal/sync/... ./internal/config/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~60-90 seconds (full suite incl. race detector) |

---

## Sampling Rate

- **After every task commit:** Run focused package test (`go test -race ./internal/<pkg>/...`)
- **After every plan:** Run quick run command above
- **Before phase verification:** `go test -race ./...` + `golangci-lint run` + `go vet ./...`
- **Max feedback latency:** ~30s for focused package, ~90s for full suite

---

## Per-Task Verification Map

| Task ID | Plan | Requirement | Test Type | Automated Command |
|---------|------|-------------|-----------|-------------------|
| 68-01-01 | 01 | STATUS-05 (D-01) | unit | `go test -race ./internal/config -run TestLoad_IncludeDeleted_Deprecated` |
| 68-01-02 | 01 | STATUS-05 (D-01) | build | `go build ./...` (verifies `IncludeDeleted` field removal compiles) |
| 68-01-03 | 01 | STATUS-05 | docs | `grep -c "PDBPLUS_INCLUDE_DELETED" docs/CONFIGURATION.md` |
| 68-02-01 | 02 | STATUS-03 (D-02) | unit | `go test -race ./internal/sync -run TestSync_SoftDeleteMarksRows` |
| 68-02-02 | 02 | STATUS-03 (D-02) | integration | `go test -race ./internal/sync -run TestSyncIntegration` |
| 68-02-03 | 02 | STATUS-03 | grep | `grep -c "DELETE FROM" internal/sync/` → 0; `grep -c "markStaleDeleted" internal/sync/delete.go` → 13 |
| 68-03-01 | 03 | LIMIT-01 (A1 probe) | unit | `go test -race ./internal/pdbcompat -run TestEntLimitZeroProbe` |
| 68-03-02 | 03 | STATUS-01/02/04, LIMIT-01/02 (D-05,D-06,D-07) | unit | `go test -race ./internal/pdbcompat -run TestStatusMatrix` |
| 68-03-03 | 03 | STATUS-02 (D-06) | grep | `grep -c 'StatusIn("ok", "pending")' internal/pdbcompat/depth.go` → 26 |
| 68-03-04 | 03 | STATUS-01..04, LIMIT-01/02 | integration | `go test -race ./internal/pdbcompat` |
| 68-04-01 | 04 | all | docs | `test -f CHANGELOG.md && grep -c "PDBPLUS_INCLUDE_DELETED" CHANGELOG.md` |
| 68-04-02 | 04 | STATUS-03 (D-03) | docs | `grep -c "Phase 68" docs/API.md` (Known Divergences entry) |
| 68-04-03 | 04 | all | audit | `grep -c "fly deploy" .planning/phases/68-status-since-matrix/*-PLAN.md` → 0 |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/pdbcompat/status_matrix_test.go` — NEW file, created in 68-03 Task 4
- [x] `internal/sync/softdelete_test.go` OR amendment to `integration_test.go` — 68-02 Task 3
- [x] `internal/config/config_test.go` — TestLoad_IncludeDeleted_Deprecated in 68-01 Task 1
- [x] No framework install — `go test` stdlib covers all cases

*Existing `testutil/seed.Full` covers the seeded-fixture base; new tests seed mixed-status rows inline.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Grace-period WARN in production logs | STATUS-05 (D-01) | Requires real prod env var set | After deploy, tail `fly logs -a peeringdb-plus` and verify `slog.Warn("PDBPLUS_INCLUDE_DELETED deprecated"...)` fires once at startup when the var is set |
| Coordinated 67→71 deploy window | D-04 | Operator judgment | Hold `fly deploy` until Phase 71 memory budget lands; merge PRs 67-71 to main serially, deploy once |
| Pre-Phase-68 hard-delete gap (D-03) | STATUS-03 | Upstream data loss | No automated check — rows already gone. Documented in docs/API.md § Known Divergences |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all new test files (listed above)
- [x] No watch-mode flags (Go has no `-watchAll` concept)
- [x] Feedback latency < 90s full suite; <30s focused
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-04-19

---

## Reference

Substantive validation architecture lives in `.planning/phases/68-status-since-matrix/68-RESEARCH.md` § Validation Architecture (observable properties, invariants, property-based test ideas). This file is the executor-facing operational contract; the research file is the rationale.
