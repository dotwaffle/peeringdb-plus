---
phase: 67
slug: default-ordering-flip
milestone: v1.16
verified_at: 2026-04-19
status: human_needed
plans_complete: 6/6
must_haves_satisfied: 4/4
---

# Phase 67 Verification Report — Default ordering flip

**Goal:** List endpoints across all three query surfaces (pdbcompat `/api/`, grpcserver ConnectRPC, entrest `/rest/v1/`) return rows in upstream PeeringDB's `(-updated, -created)` order instead of `id ASC`, matching `django-handleref` base `Meta.ordering`.

## Summary

All four ROADMAP Phase 67 success criteria are satisfied. The 10 defined grep signals all return their expected counts (13/0/26/0/13/13/17/present/present/13), exercising every touchpoint the plan set identified: `internal/pdbcompat/registry_funcs.go` compound ORDER BY, `internal/grpcserver/*` List+Stream handlers, ent schema annotations, migrate DDL, entrest template override wiring, and the in-tree `sorting.tmpl` fallback. The full test suite (`go test -race ./...`) is clean; the dedicated `TestDefaultOrdering_Pdbcompat` (5 subtests), `TestDefaultOrdering_Grpc_*` (3 subtests), `TestOrdering_CrossSurface` (3 subtests), `TestEntrestNestedSetOrder` (D-04 nested `_set` parity), and `TestCursorResume_CompoundKeyset` (D-01 compound cursor keyset resume) all pass. `go generate ./... && git diff --exit-code` exits clean, confirming codegen idempotency (CI drift check). Out-of-scope compliance is confirmed — no Phase 67 commit touches `proto/`, `graph/`, or `internal/web/`. The sole deferred item `D-67-01` (`TestGenerateIndexes` allow-list) closed in commit `417ba7e`; the test now runs green.

## Must-Haves Verification

| # | Must-have (from ROADMAP success criteria) | Status | Evidence |
|---|---|---|---|
| 1 | `/api/<type>` returns rows sorted by `updated DESC`, tie-broken by `created DESC`, matching upstream | VERIFIED | `grep -c 'ent.Desc("updated"), ent.Desc("created"), ent.Desc("id")' internal/pdbcompat/registry_funcs.go` = 13; `TestDefaultOrdering_Pdbcompat` 5/5 subtests pass (Network, Facility, InternetExchange, TieBreakCreated, TieBreakID) |
| 2 | `List*`/`Stream*` ConnectRPC for all 13 types return `(-updated, -created)`; cursor pagination remains stable | VERIFIED | `grep -c 'ent.Desc(.*\.FieldUpdated)' internal/grpcserver/*.go` sum = 26 (13 List + 13 Stream); `TestDefaultOrdering_Grpc_{Network,Facility,InternetExchange}` pass; `TestCursorResume_CompoundKeyset` proves opaque compound `(last_updated, last_id)` cursor survives round-trip |
| 3 | `/rest/v1/*` defaults to `(-updated, -created)` while honouring `?sort=` overrides | VERIFIED | `grep -c 'entrest.WithDefaultSort("updated")' ent/schema/*.go` = 13; `entc.TemplateDir`-style `entrestSortingOverride` wired in `ent/entc.go`; `ent/templates/entrest-sorting/sorting.tmpl` present; generated `ent/rest/sorting.go` contains 13 `if _field == "updated"` blocks emitting compound tie-break; `TestEntrestNestedSetOrder/depth2` passes (D-04 nested `_set` parity) |
| 4 | Streaming cursor-resume (`since_id`, `updated_since`) still works under the new order | VERIFIED | D-05 kept `since_id`/`updated_since` predicates BEFORE ordering per CONTEXT.md; `TestCursorResume_CompoundKeyset` passes; `go test -race ./internal/grpcserver/...` clean |

**Score:** 4/4 truths verified.

## Grep Signals

| # | Command | Expected | Actual | Status |
|---|---|---|---|---|
| 1 | `grep -c 'ent.Desc("updated"), ent.Desc("created"), ent.Desc("id")' internal/pdbcompat/registry_funcs.go` | 13 | 13 | PASS |
| 2 | `grep -c 'ent.Asc("id")' internal/pdbcompat/registry_funcs.go` | 0 | 0 | PASS |
| 3 | `sum of grep -c 'ent.Desc(.*\.FieldUpdated)' internal/grpcserver/*.go` | ≥26 | 26 | PASS |
| 4 | `sum of grep -c 'ent.Asc(.*\.FieldID)' internal/grpcserver/*.go` | 0 | 0 | PASS |
| 5 | `sum of grep -c 'index.Fields("updated")' ent/schema/*.go` | 13 | 13 | PASS |
| 6 | `sum of grep -c 'entrest.WithDefaultSort("updated")' ent/schema/*.go` | 13 | 13 | PASS |
| 7 | `grep -c '_updated' ent/migrate/schema.go` | ≥13 | 17 | PASS (13 entities × 1 updated index each + 4 related composite entries) |
| 8 | `grep -n 'TemplateDir\|entrest-override\|entrest-sorting' ent/entc.go` | present | 6 matches across lines 110-136; `entrestSortingOverride("./templates/entrest-sorting")` registered | PASS |
| 9 | `test -f ent/templates/entrest-sorting/sorting.tmpl` | present | 6942 bytes | PASS |
| 10 | `grep -c 'if _field == "updated"' ent/rest/sorting.go` | 13 | 13 | PASS |

## Test Signals

| Command | Status | Notes |
|---|---|---|
| `go build ./...` | PASS | exit 0, no output |
| `go vet ./...` | PASS | exit 0, no output |
| `go test -race ./...` | PASS | All 26 test packages OK; no FAIL lines |
| `go generate ./... && git diff --exit-code` | PASS | No diff after regen — codegen idempotent (CI drift check passes) |
| `go test -run TestOrdering ./cmd/peeringdb-plus/...` | PASS | `TestOrdering_CrossSurface` + 3 subtests (Network, TieBreakCreated, TieBreakID) |
| `go test -run TestDefaultOrdering ./internal/pdbcompat/... ./internal/grpcserver/...` | PASS | `TestDefaultOrdering_Pdbcompat` (5 subtests) + `TestDefaultOrdering_Grpc_{Network,Facility,InternetExchange}` |
| `go test -run TestEntrestNestedSetOrder ./cmd/peeringdb-plus/...` | PASS | `TestEntrestNestedSetOrder/depth2` — D-04 nested `_set` parity lock |
| `go test -run TestCursorResume ./internal/grpcserver/...` | PASS | `TestCursorResume_CompoundKeyset` — D-01 compound cursor |
| `go test -race -run TestGenerateIndexes ./cmd/pdb-schema-generate/` | PASS | D-67-01 deferred item fixed by commit 417ba7e |
| `golangci-lint run` | 1 finding | `nolintlint` on `internal/pdbcompat/registry_funcs_ordering_test.go:303` — unused `gosec` directive; test-only, non-blocking. Introduced by commit `adf77f5` (67-03 TDD RED test). See § Human-Verification Items. |

## Requirements Coverage

| REQ-ID | Description | Verified Via | Status |
|---|---|---|---|
| ORDER-01 | pdbcompat list ordering flip | Grep 1-2; `TestDefaultOrdering_Pdbcompat` (5 subtests); `TestOrdering_CrossSurface/Network` | SATISFIED |
| ORDER-02 | grpcserver List/Stream flip + stable keyset pagination | Grep 3-4; `TestDefaultOrdering_Grpc_*` (3); `TestCursorResume_CompoundKeyset` (D-01 compound cursor round-trip) | SATISFIED |
| ORDER-03 | entrest default + `?sort=` override | Grep 5-6, 8-10; `TestEntrestNestedSetOrder/depth2` (D-04 nested `_set`); `ent/templates/entrest-sorting/sorting.tmpl` present and wired via `entrestSortingOverride` in `ent/entc.go`; template output lands 13× `if _field == "updated"` compound tie-break in `ent/rest/sorting.go` | SATISFIED |

No orphaned requirements — REQUIREMENTS.md maps only ORDER-01..03 to Phase 67, all three declared in one or more plans (ORDER-01 → 03; ORDER-02 → 04, 05; ORDER-03 → 01, 02, 06).

## Out-of-Scope Compliance

| Path | Last commit touching path | Phase 67 commits | Status |
|---|---|---|---|
| `proto/` | `0443ebc feat(64-02): ...` (Phase 64, pre-v1.16) | 0 | UNTOUCHED |
| `graph/` | `73bbe04 refactor(quick-260418-gf0): ...` (pre-v1.16) | 0 | UNTOUCHED |
| `internal/web/` | `435bf36 feat(61-02): ...` (Phase 61, pre-v1.16) | 0 | UNTOUCHED |

All 16 Phase 67 commits (`0e4012f`, `2d06676`, `417ba7e`, `fbf0e60`, `541a04b`, `568c965`, `c854fa9`, `adf77f5`, `fb02653`, `ac8f016`, `331077b`, `4096c10`, `93b61b8`, `3b40536`, `cdbb8e4`, `56549df`) restrict their touch to `ent/schema/`, `ent/entc.go`, `ent/templates/`, `ent/rest/`, `ent/migrate/`, `cmd/pdb-schema-generate/`, `internal/pdbcompat/`, `internal/grpcserver/`, `cmd/peeringdb-plus/ordering_cross_surface_e2e_test.go`, `docs/ARCHITECTURE.md`, and planning artifacts. GraphQL, Web UI, and proto wire types remain unchanged as required.

## Deferred Items Audit

| Deferred item | Closed by | Evidence |
|---|---|---|
| D-67-01 — `TestGenerateIndexes` allow-list regression from Plan 67-01 | commit `417ba7e` (`fix(67-01): admit "updated" in TestGenerateIndexes allow-list`) | `go test -race -run TestGenerateIndexes ./cmd/pdb-schema-generate/` → `ok 1.012s` |

No remaining open deferrals for Phase 67.

## Human-Verification Items

1. **Deploy-time `sqlite3 .schema` index audit (OBS-04 path).** Grep signal 7 confirms the declarative `ALTER TABLE ... ADD INDEX` surfaces in `ent/migrate/schema.go`, and auto-migrate runs with `migrate.WithDropIndex(true)` at startup. After the next `fly deploy` of `peeringdb-plus`, run `fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db ".schema"' | grep -iE 'index.*updated'` and confirm 13 `CREATE INDEX` lines appear (one per entity). Replicas inherit via LiteFS replication — primary check is sufficient. **Why human:** requires the live primary VM; cannot be validated from the build tree.

2. **Pre-existing-style lint finding in new test file.** `golangci-lint run` reports one `nolintlint` issue at `internal/pdbcompat/registry_funcs_ordering_test.go:303:29` — the `gosec` portion of a `//nolint:gosec,noctx` directive is unused because the underlying `http.Get` call now only triggers `noctx`. This was introduced by Phase 67-03 (commit `adf77f5`). **Why human:** severity call — either (a) drop `gosec` to leave only `noctx`, or (b) accept as test-only debt and suppress in `.golangci.yml`. Not a blocker for phase-goal completion (runtime behaviour unaffected), but worth a cleanup quick-task before v1.16 milestone close.

## Status

**human_needed** — all 4 ROADMAP success criteria met, all 3 requirements (ORDER-01/02/03) satisfied, all 10 grep signals return expected counts, full test suite green, codegen idempotent, out-of-scope surfaces untouched, deferred D-67-01 closed. Two minor human-verification items are operational (deploy-time index check) and cleanup-quality (lint hygiene), neither blocks the phase goal.

## VERIFICATION COMPLETE

Status: human_needed
