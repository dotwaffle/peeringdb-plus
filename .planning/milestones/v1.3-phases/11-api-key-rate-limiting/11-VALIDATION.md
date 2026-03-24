---
phase: 11
slug: api-key-rate-limiting
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 11 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test tooling |
| **Quick run command** | `go test ./internal/peeringdb/... ./internal/config/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/peeringdb/... ./internal/config/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 11-01-01 | 01 | 1 | KEY-01 | unit | `go test ./internal/config/... -run TestAPIKey` | ❌ W0 | ⬜ pending |
| 11-01-02 | 01 | 1 | KEY-02 | unit | `go test ./internal/peeringdb/... -run TestAuthHeader` | ❌ W0 | ⬜ pending |
| 11-01-03 | 01 | 1 | RATE-01 | unit | `go test ./internal/peeringdb/... -run TestRateLimit` | ❌ W0 | ⬜ pending |
| 11-01-04 | 01 | 1 | RATE-02 | unit | `go test ./internal/peeringdb/... -run TestGracefulDegradation` | ❌ W0 | ⬜ pending |
| 11-01-05 | 01 | 1 | VALIDATE-01 | unit | `go test ./internal/peeringdb/... -run TestAuthError` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Test stubs for KEY-01, KEY-02, RATE-01, RATE-02, VALIDATE-01 in existing test files
- [ ] Existing test infrastructure covers framework needs

*Existing infrastructure covers framework requirements — only test stubs needed.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Live PeeringDB auth with real API key | KEY-01 | Requires valid PeeringDB account credentials | Set PDBPLUS_PEERINGDB_API_KEY, run sync, verify auth header in request logs |
| Rate limit increase observed in practice | RATE-01 | Requires measuring actual request throughput | Monitor sync with API key, confirm requests exceed 20/min threshold |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
