---
phase: 23
slug: connectrpc-services
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-25
---

# Phase 23 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) + ent/enttest |
| **Config file** | None needed (Go conventions) |
| **Quick run command** | `go test ./internal/grpcserver/... -race -count=1` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~45 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/grpcserver/... -race -count=1`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 45 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 23-01-01 | 01 | 1 | API-01 | unit | `go test ./internal/grpcserver/ -run TestGetNetwork -race -count=1` | No — Wave 0 | ⬜ pending |
| 23-01-02 | 01 | 1 | API-02 | unit | `go test ./internal/grpcserver/ -run TestListNetworks -race -count=1` | No — Wave 0 | ⬜ pending |
| 23-02-01 | 02 | 2 | API-04 | integration | `go test ./cmd/peeringdb-plus/ -run TestConnectRPC -race -count=1` | No — Wave 0 | ⬜ pending |
| 23-02-02 | 02 | 2 | OBS-01 | unit | `go test ./internal/grpcserver/ -run TestOtel -race -count=1` | No — Wave 0 | ⬜ pending |
| 23-02-03 | 02 | 2 | OBS-02 | unit | `go test ./internal/middleware/ -run TestCORS -race -count=1` | Yes (update) | ⬜ pending |
| 23-03-01 | 03 | 2 | OBS-03 | integration | `go test ./cmd/peeringdb-plus/ -run TestReflection -race -count=1` | No — Wave 0 | ⬜ pending |
| 23-03-02 | 03 | 2 | OBS-04 | unit | `go test ./internal/grpcserver/ -run TestHealthCheck -race -count=1` | No — Wave 0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/grpcserver/grpcserver_test.go` — test stubs for Get/List/OTel
- [ ] `internal/grpcserver/pagination_test.go` — cursor encoding edge cases
- [ ] Tests use generated ConnectRPC clients against `httptest.Server`

*Existing `internal/middleware/cors_test.go` covers OBS-02 but needs updating for Connect headers.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| buf curl works end-to-end | API-01 | Requires running server | Start server, run `buf curl --http2-prior-knowledge http://localhost:8080/peeringdb.v1.NetworkService/GetNetwork -d '{"id": 1}'` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 45s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
