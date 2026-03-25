---
phase: 21
slug: infrastructure
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 21 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) |
| **Config file** | None needed (go test convention) |
| **Quick run command** | `go test ./internal/litefs/... ./internal/config/... -race -count=1` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/litefs/... ./internal/config/... -race -count=1`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 21-01-01 | 01 | 1 | INFRA-01 | config + integration | `go test ./internal/config/... -race -count=1 -run TestLoad` | Yes | ⬜ pending |
| 21-01-02 | 01 | 1 | INFRA-02 | unit | `go test ./cmd/peeringdb-plus/... -race -count=1 -run TestSyncReplay` | No — Wave 0 | ⬜ pending |
| 21-01-03 | 01 | 1 | INFRA-03 | unit | `go test ./cmd/peeringdb-plus/... -race -count=1 -run TestSyncLocal` | No — Wave 0 | ⬜ pending |
| 21-01-04 | 01 | 1 | INFRA-04 | integration | `go test ./cmd/peeringdb-plus/... -race -count=1 -run TestH2C` | No — Wave 0 | ⬜ pending |
| 21-02-01 | 02 | 1 | INFRA-05 | manual | N/A (config file inspection) | N/A | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Sync handler tests for fly-replay behavior (Fly.io vs local) — currently inline in main.go, may need to extract to testable function
- [ ] h2c integration test — start server, make HTTP/2 prior-knowledge request, verify response

*Existing config tests cover INFRA-01 listen address validation.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| fly.toml has h2_backend | INFRA-05 | TOML config file, not Go code | Inspect fly.toml for `h2_backend = true` under `[http_service.http_options]` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
