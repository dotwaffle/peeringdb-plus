---
phase: 12
slug: conformance-tooling-integration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 12 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test tooling |
| **Quick run command** | `go test ./cmd/pdbcompat-check/... ./internal/conformance/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./cmd/pdbcompat-check/... ./internal/conformance/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 12-01-01 | 01 | 1 | CONFORM-01 | unit | `go test ./cmd/pdbcompat-check/... -run TestAPIKey` | ❌ W0 | ⬜ pending |
| 12-01-02 | 01 | 1 | CONFORM-02 | unit | `go test ./internal/conformance/... -run TestLiveAPIKey` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `cmd/pdbcompat-check/main_test.go` — stubs for CONFORM-01 (flag parsing, env fallback, header injection)
- [ ] Test stubs for CONFORM-02 in existing `internal/conformance/live_test.go`

*New test file needed for cmd/pdbcompat-check which has zero tests today.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live PeeringDB auth with real API key via CLI | CONFORM-01 | Requires valid PeeringDB credentials | Run `pdbcompat-check --api-key <key>` and verify auth header in requests |
| Live integration test with real API key | CONFORM-02 | Requires valid PeeringDB credentials | Set PDBPLUS_PEERINGDB_API_KEY, run `-peeringdb-live` tests |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
