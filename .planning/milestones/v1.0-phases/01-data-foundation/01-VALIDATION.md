---
phase: 1
slug: data-foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-22
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib) + enttest |
| **Config file** | None -- standard Go test infrastructure |
| **Quick run command** | `go test ./... -short` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./... -short -count=1`
- **After every plan wave:** Run `go test -race ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-01 | 01 | 1 | DATA-01 | unit | `go test ./ent/schema/... -run TestSchema` | ❌ W0 | ⬜ pending |
| 01-01-02 | 01 | 1 | DATA-01 | unit | `go generate ./ent && go build ./...` | ❌ W0 | ⬜ pending |
| 01-02-01 | 02 | 1 | DATA-02 | integration | `go test ./internal/peeringdb/... -run TestFieldMapping` | ❌ W0 | ⬜ pending |
| 01-02-02 | 02 | 1 | DATA-02 | unit | `go test ./internal/sync/... -run TestFixtureDeserialization` | ❌ W0 | ⬜ pending |
| 01-03-01 | 03 | 2 | DATA-03 | unit | `go test ./internal/sync/... -run TestHardDelete` | ❌ W0 | ⬜ pending |
| 01-03-02 | 03 | 2 | DATA-03 | unit | `go test ./internal/sync/... -run TestStatusFilter` | ❌ W0 | ⬜ pending |
| 01-04-01 | 04 | 2 | DATA-04 | integration | `go test -tags=integration ./internal/sync/... -run TestFullSync` | ❌ W0 | ⬜ pending |
| 01-04-02 | 04 | 2 | DATA-04 | unit | `go test ./internal/sync/... -run TestScheduler` | ❌ W0 | ⬜ pending |
| 01-04-03 | 04 | 2 | DATA-04 | unit | `go test ./internal/sync/... -run TestTrigger` | ❌ W0 | ⬜ pending |
| 01-05-01 | 05 | 1 | STOR-01 | unit | `go test ./ent/... -run TestCRUD` | ❌ W0 | ⬜ pending |
| 01-05-02 | 05 | 1 | STOR-01 | unit | `go test ./internal/... -run TestSQLiteConfig` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `testdata/fixtures/*.json` -- Recorded API responses for all 13 object types (capture from `beta.peeringdb.com` per D-56)
- [ ] `internal/peeringdb/client_test.go` -- Tests for API client with fixture-based HTTP server
- [ ] `internal/sync/worker_test.go` -- Tests for sync logic with in-memory SQLite
- [ ] `ent/schema/*_test.go` or `ent/enttest_test.go` -- Schema compilation and CRUD verification
- [ ] Test helpers for creating in-memory entgo test clients (per REFERENCE-SQLITE-ENTGO.md)

*If none: "Existing infrastructure covers all phase requirements."*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Full sync populates all objects from live PeeringDB | DATA-04 | Requires live API access with rate limits | Run `POST /sync` and verify all 13 types have >0 rows |
| Sync runs on hourly schedule | DATA-04 | Time-based behavior | Start app, wait for ticker to fire, check sync_status table |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
