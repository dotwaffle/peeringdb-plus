---
phase: 24
slug: list-filtering
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-25
---

# Phase 24 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) |
| **Config file** | None (stdlib) |
| **Quick run command** | `go test ./internal/grpcserver/ -run TestList -race -count=1` |
| **Full suite command** | `go test -race -count=1 ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/grpcserver/ -race -count=1`
- **After every plan wave:** Run `go test -race -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 24-01-01 | 01 | 1 | API-03 | unit | `go test ./internal/grpcserver/ -run TestListNetworksFilters -race -count=1` | No — Wave 0 | ⬜ pending |
| 24-01-02 | 01 | 1 | API-03 | unit | `go test ./internal/grpcserver/ -run TestList.*Filter -race -count=1` | No — Wave 0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

None — existing test infrastructure covers all phase requirements. New test functions added alongside handler changes.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| End-to-end filter via buf curl | API-03 | Requires running server | `buf curl --http2-prior-knowledge http://localhost:8080/peeringdb.v1.NetworkService/ListNetworks -d '{"asn": 15169}'` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
