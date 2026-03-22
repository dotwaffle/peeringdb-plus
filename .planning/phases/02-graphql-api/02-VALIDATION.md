---
phase: 2
slug: graphql-api
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-03-22
---

# Phase 2 -- Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | testing (stdlib) + enttest |
| **Config file** | none (Go conventions) |
| **Quick run command** | `go test ./graph/... ./internal/graphql/... ./internal/middleware/... -v -count=1` |
| **Full suite command** | `go test -race ./... -count=1` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./graph/... ./internal/graphql/... ./internal/middleware/... -v -count=1`
- **After every plan wave:** Run `go test -race ./... -count=1`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 02-01-T1 | 01 | 1 | API-01, API-06 | codegen | `go generate ./ent && go build ./ent/...` | N/A (codegen) | pending |
| 02-01-T2 | 01 | 1 | API-01, API-03 | build | `go build ./graph/...` | N/A (codegen) | pending |
| 02-02-T1 | 02 | 2 | API-02, API-04, API-05, API-06 | build | `go build ./graph/... ./graph/dataloader/...` | N/A | pending |
| 02-02-T2 | 02 | 2 | API-01, API-04 | build | `go build ./graph/...` | N/A | pending |
| 02-03-T1 | 03 | 2 | OPS-06, API-07 | build | `go build ./internal/middleware/...` | N/A | pending |
| 02-03-T2 | 03 | 2 | API-07, OPS-06 | build | `go build ./internal/graphql/... ./internal/config/...` | N/A | pending |
| 02-04-T1 | 04 | 3 | API-07, OPS-06 | build | `go build ./cmd/peeringdb-plus/... && go build ./internal/graphql/...` | N/A | pending |
| 02-04-T2 | 04 | 3 | API-01..07, OPS-06 | integration | `go test -race ./graph/... ./internal/middleware/... -v -count=1` | W0 | pending |

*Status: pending -- green -- red -- flaky*

---

## Wave 0 Requirements

- [ ] `graph/resolver_test.go` -- integration tests for all ent resolvers, custom queries, pagination, error handling
- [ ] `internal/middleware/cors_test.go` -- CORS header verification

Note: Wave 0 tests are created by Plan 04 Task 2 (TDD task). Plans 01-03 verify via codegen success and `go build`; the integration test suite in Plan 04 provides the comprehensive validation.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| GraphQL playground renders in browser | API-07 | Requires browser rendering | Open http://localhost:8080/graphql in browser, verify GraphiQL loads |
| GraphiQL shows example query tabs | D-19 | Visual verification of playground content | Open playground, verify example queries are visible |
| CORS works from browser origin | OPS-06 | Requires real browser CORS | Make fetch() call from different origin, verify no CORS errors |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending execution
