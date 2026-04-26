---
phase: 73
plan: 02
subsystem: sync, ent-schema, codegen
tags:
  - bug
  - ent-schema
  - sync
  - codegen
  - tombstones
  - seed-001-tail
requirements:
  - BUG-02
dependency_graph:
  requires:
    - "260426-pms quick task (the precedent name=\"\" tombstone fix that established the codegen pattern)"
  provides:
    - "Codegen-layer NotEmpty drop for poc.role — durable across `go generate ./...` regenerations"
    - "isTombstoneVulnerableField predicate — explicit drop list for future PII-scrub-target additions"
    - "TestPoc_RoleEmptyAcceptedByValidator unit-level proof per CONTEXT.md D-03 (fast-feedback affordance)"
    - "TestSync_IncrementalRoleTombstone httptest fake-upstream regression guard per CONTEXT.md D-03"
  affects:
    - "internal/sync/upsert.go upsert path — now safely accepts poc tombstones with role=\"\""
    - "Future PeeringDB ?since= responses — first poc tombstone no longer aborts incremental sync"
tech-stack:
  added: []
  patterns:
    - "Codegen-layer fix at cmd/pdb-schema-generate/main.go (matches 260426-pms pattern; direct ent/schema edits would be stripped)"
    - "Sibling test pattern: unit (poc_validator_test.go) + httptest (worker_test.go) for the same invariant — D-03 mandate"
    - "Static predicate function (isTombstoneVulnerableField) over runtime introspection — grep-able and zero init-order coupling"
key-files:
  created:
    - "internal/sync/poc_validator_test.go"
  modified:
    - "cmd/pdb-schema-generate/main.go"
    - "cmd/pdb-schema-generate/main_test.go"
    - "ent/schema/poc.go (regenerated)"
    - "ent/poc/poc.go (regenerated)"
    - "ent/poc_create.go (regenerated)"
    - "ent/poc_update.go (regenerated)"
    - "ent/runtime/runtime.go (regenerated)"
    - "internal/sync/worker_test.go"
decisions:
  - id: D-T1
    summary: "Replace `name != \"name\"` ad-hoc gate with explicit `isTombstoneVulnerableField` predicate"
    rationale: "260426-pms's gate was a single-target negative check (`name != \"name\"`); extending it to two targets (name + role) makes the conjunction unreadable. The new predicate carries a docstring with audit citations (260426-pms for name, Phase 73 BUG-02 for role) so future maintainers see the rationale on the next codegen review without grepping git blame."
  - id: D-T2
    summary: "Tab-depth literal in test regression guards uses explicit `\\n\\t\\t\\t` escape, not source-indented raw strings"
    rationale: "The existing Organization regression guard uses a raw string with source-file indentation, baking the source's leading-tab count (4) into the comparison. The actual generator emits 3 tabs. The Org guard happens to pass only because Org.name no longer emits NotEmpty at all — if it ever DID emit, the guard would fail to match. Phase 73's Poc + role guards use string concatenation with explicit `\\n\\t\\t\\t` to match the EXACT three-tab depth the formatter emits, regardless of source indentation. This is a pre-existing bug in the Org guard that was discovered during Phase 73 Task 1 RED-phase debugging; the Org guard was left unchanged (out of scope per BUG-02 strict scoping) but the new Poc + role guards are correct."
  - id: D-T3
    summary: "Pre-existing gofmt -s drift on FieldDef struct picked up as same-file hygiene"
    rationale: "Plan acceptance criterion required `gofmt -s` clean on the touched files. The FieldDef struct in main.go had pre-existing column-alignment drift unrelated to this plan, but in the same file. Running `gofmt -s -w` to clear it is same-file hygiene per the plan's acceptance criteria; the change is purely whitespace and committed alongside the predicate addition rather than as a separate commit (single feat commit covers both)."
metrics:
  duration: "~12 minutes (executor wall-clock from base reset to final commit)"
  completed_date: "2026-04-26"
  task_count: 4
  file_count: 9
  commit_count: 5
---

# Phase 73 Plan 02: Code Defect Fixes — BUG-02 Summary

Drop the `NotEmpty()` validator from `poc.role` at the codegen layer
(`cmd/pdb-schema-generate`) so upstream PeeringDB `?since=` tombstones
with PII-scrubbed `role=""` don't abort incremental sync, plus
sibling unit + httptest regression guards per CONTEXT.md D-03.

## Subtask 1A — 13-entity NotEmpty audit (mandatory per CONTEXT.md D-02)

`grep -rn 'NotEmpty()' ent/schema/` returned exactly two hits before
the fix: `poc.go:46` (role) and `ixprefix.go:35` (prefix). The full
audit table — including the 11 entities with no NotEmpty validators
(load-bearing for proving the audit was exhaustive, not skim-checked):

| Entity            | NotEmpty present?   | Field   | Decision | Rationale                                                                                                                                                                                                                                  |
|-------------------|---------------------|---------|----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| organization      | N/A — no validators | (none)  | n/a      | `name` already dropped by 260426-pms via the codegen `isNameField` gate; no other string field carries NotEmpty().                                                                                                                          |
| network           | N/A — no validators | (none)  | n/a      | `name` already dropped by 260426-pms; no other NotEmpty validators on this entity.                                                                                                                                                          |
| facility          | N/A — no validators | (none)  | n/a      | `name` already dropped by 260426-pms; no other NotEmpty validators.                                                                                                                                                                         |
| internetexchange  | N/A — no validators | (none)  | n/a      | `name` already dropped by 260426-pms; no other NotEmpty validators.                                                                                                                                                                         |
| carrier           | N/A — no validators | (none)  | n/a      | `name` already dropped by 260426-pms; no other NotEmpty validators.                                                                                                                                                                         |
| campus            | N/A — no validators | (none)  | n/a      | `name` already dropped by 260426-pms; no other NotEmpty validators.                                                                                                                                                                         |
| **poc**           | **YES**             | role    | **DROP** | Phase 73 BUG-02: tombstone-vulnerable per CONTEXT.md D-02. PII-scrub-style empty role on `?since=` deletes plausible (same GDPR pattern empirically confirmed for `name=""` by 260426-pms). NotEmpty() would abort sync at the upsert builder. `email`/`phone`/`url` are already Optional+Default("") — accept empty by construction. |
| ixlan             | N/A — no validators | (none)  | n/a      | All string fields are Optional+Default("") or unconstrained; no validators to drop.                                                                                                                                                          |
| **ixprefix**      | **YES**             | prefix  | **KEEP** | Structural — IP prefix is row identity, not PII. Upstream retains the prefix value on tombstones because the prefix IS the row's natural key. Out of scope per CONTEXT.md and 260426-pms.                                                  |
| netixlan          | N/A — no validators | (none)  | n/a      | No NotEmpty validators on this entity.                                                                                                                                                                                                      |
| netfac            | N/A — no validators | (none)  | n/a      | No NotEmpty validators on this entity.                                                                                                                                                                                                      |
| ixfac             | N/A — no validators | (none)  | n/a      | No NotEmpty validators on this entity.                                                                                                                                                                                                      |
| carrierfac        | N/A — no validators | (none)  | n/a      | No NotEmpty validators on this entity.                                                                                                                                                                                                      |

Net audit outcome: ONE drop (`poc.role`), ONE keep (`ixprefix.prefix`),
11 entities had no validators to consider. Audit exhaustive across all
13 entity types per CONTEXT.md D-02 mandate.

## Tasks Completed

### Task 1: Audit + extend cmd/pdb-schema-generate (TDD)

**RED phase** (commit `0dc7b4a`):
- Extended `cmd/pdb-schema-generate/main_test.go` `TestGenerateEntSchemaCompiles`
  with a "Poc" test case carrying `notWantParts:
  []string{"field.String(\"role\").\n\t\t\tNotEmpty()"}` (3-tab literal,
  string-concatenated to match the exact emission shape — see D-T2).
- Added a "poc" entry to the test schema fixture with role marked
  `Required: true, Nullable: false` to exercise the NotEmpty
  emission gate.
- Extended `TestGenerateFieldCode` test struct with a `notWantSub
  []string` field + corresponding assertion loop.
- Added a "role" case asserting `.NotEmpty()` is absent (notWantSub).
- Added a "prefix" case asserting `.NotEmpty()` IS still emitted
  (forward regression guard for the KEEP decision per CONTEXT.md D-02).
- Added a defence-in-depth "name" `notWantSub` guard.

Pre-GREEN test run confirmed: TestGenerateEntSchemaCompiles/Poc and
TestGenerateFieldCode/role both FAIL because the generator currently
emits NotEmpty() for role.

**GREEN phase** (commit `d2ba190`):
- Replaced `name != "name"` in the NotEmpty emission gate at
  `cmd/pdb-schema-generate/main.go` line ~328 with
  `!isTombstoneVulnerableField(name)`.
- Added `isTombstoneVulnerableField(name string) bool` predicate
  returning `true` for `"name"` and `"role"`. Docstring records BOTH
  the 260426-pms `name` drop (with SEED-001 spike date) AND the
  v1.18.0 Phase 73 `role` drop (with CONTEXT.md D-02 reference) plus
  explicit KEEP rationale for `"prefix"`.
- Updated the SEED-001 docstring on the gate to cite both quick task
  260426-pms and Phase 73 BUG-02.
- Picked up pre-existing gofmt drift on the FieldDef struct
  alignment (same-file hygiene per the plan's gofmt -s clean
  acceptance criterion — see D-T3).

Post-GREEN tests: `go test -race ./cmd/pdb-schema-generate/...` PASS.
`grep -c 'isTombstoneVulnerableField' cmd/pdb-schema-generate/main.go`
returns 4 (1 declaration + 1 use in gate + 2 docstring references).

### Task 2: Run codegen + verify regenerated artefacts

Commit `0e98048`. Ran `go generate ./...` from project root with
`TMPDIR=/tmp/claude-1000`. Cascade of regenerations triggered by the
schema change:

- `ent/schema/poc.go` — `field.String("role")` chain no longer
  contains the `.NotEmpty()` line. All other annotations preserved
  (entrest filter, Comment).
- `ent/poc/poc.go` — `RoleValidator` declaration removed from the
  `var (...)` block.
- `ent/poc_create.go` — `check()` no longer invokes
  `poc.RoleValidator(v)` on the Role mutation. Required-field check
  (returns ValidationError if role missing) preserved.
- `ent/poc_update.go` — `(*PocUpdate).check()` and
  `(*PocUpdateOne).check()` removed entirely (they only existed to
  invoke the dropped validator); `sqlSave` no longer calls them.
- `ent/runtime/runtime.go` — `pocDescRole` block + `RoleValidator`
  initialiser removed from `init()`.

Verification:
- `grep -c 'NotEmpty()' ent/schema/poc.go` = 0 (validator dropped)
- `grep -c 'NotEmpty()' ent/schema/ixprefix.go` = 1 (KEEP preserved)
- `grep -c 'RoleValidator' ent/{runtime/runtime,poc/poc,poc_create,poc_update}.go` = 0 across all four
- `go build ./...` PASS
- Two consecutive `go generate ./...` runs produce zero further drift (idempotent)

### Task 3: TestSync_IncrementalRoleTombstone (httptest, TDD)

Commit `b4d2b84`. Phase 73 BUG-02 end-to-end regression guard per
CONTEXT.md D-03. Mirrors `TestSync_IncrementalDeletionTombstone`
(260426-pms) — drives a tombstone for an existing poc through an
incremental sync cycle via the httptest fake-upstream, asserts the row
is soft-deleted (not hard-deleted) and excluded from the anonymous
`status="ok"` list path.

Two helpers added:
- `makePoc(id, netID int, name, role, status string) map[string]any`
  fixture-builder adjacent to `makeOrg`/`makeFac`/`makeNet`. Threads
  `role` + `visible="Public"` + the canonical Poc JSON keys per
  `internal/peeringdb/types.go`.
- `TestSync_IncrementalRoleTombstone` placed immediately after
  `TestSync_IncrementalDeletionTombstone` so all incremental-tombstone
  tests cluster.

Test shape (mirrors 260426-pms):
- Cycle 1: seed parent org+net + 2 pocs (Alice "Technical", Bob
  "Policy"), drive a full-fallback first sync (cursor zero), verify
  both pocs persisted + cursor advanced.
- Cycle 2: bump generated, replace Bob with a tombstone
  (status="deleted", role=""), drive a real `?since=` sync, verify:
    (a) `?since=` sent for poc (incremental path taken, not fallback)
    (b) total count still 2 (soft-delete preserves row)
    (c) Alice unchanged (status="ok", role="Technical")
    (d) Bob soft-deleted (status="deleted", role="")
    (e) anonymous filter `poc.StatusEQ("ok")` returns only Alice

`go test -race -run TestSync_IncrementalRoleTombstone ./internal/sync/...`
PASS in ~330ms. Full sync suite (`go test -race ./internal/sync/...`)
PASS — no regressions on `TestSync_IncrementalDeletionTombstone` or
other tombstone/incremental tests.

### Task 4: TestPoc_RoleEmptyAcceptedByValidator (unit, TDD)

Commit `25cc834`. Phase 73 BUG-02 unit-level proof per CONTEXT.md D-03
(the fast-feedback affordance alongside Task 3's httptest test). Both
committed satisfies D-03's "both committed" mandate.

New file `internal/sync/poc_validator_test.go`:
- Seeds an isolated in-memory ent client via `testutil.SetupClient(t)`.
- Creates a parent network (with all required scalar fields:
  allow_ixp_update, asn, info_ipv6, info_multicast,
  info_never_via_route_servers, info_unicast, name, policy_ratio,
  created, updated, status, name_fold).
- Creates a poc with role="Technical".
- Asserts `c.Poc.UpdateOneID(1).SetRole("").SetStatus("deleted")
  .Save(ctx)` succeeds without a validator error.
- Verifies the post-update state: `got.Role == ""` AND
  `got.Status == "deleted"`.

If a future regression re-introduces `NotEmpty()` on poc.role, this
test fails immediately on the validator chain — without spinning up
the httptest fake-upstream sync harness.

Runtime budget verified:
- 10 iterations in 76ms without `-race` (well under 1s)
- 0.13s for a single `-race` iteration

`go test -race -run TestPoc_RoleEmptyAcceptedByValidator
./internal/sync/...` PASS.

## End-to-End Verification

| Check | Required | Actual | Pass? |
|-------|----------|--------|-------|
| `go build ./...` | exit 0 | exit 0 | yes |
| `go test -race ./...` | full suite PASS | full suite PASS (47 packages, 0 failures) | yes |
| `go generate ./...` | zero drift | zero drift on second run | yes |
| `golangci-lint run ./...` | 0 issues | 0 issues | yes |
| `go vet ./...` | clean | clean | yes |
| `grep -c 'NotEmpty()' ent/schema/poc.go` | 0 | 0 | yes |
| `grep -c 'NotEmpty()' ent/schema/ixprefix.go` | ≥1 | 1 | yes |
| `grep -c 'RoleValidator' ent/runtime/runtime.go ent/poc/poc.go ent/poc_create.go ent/poc_update.go` | 0 each | 0 each | yes |
| `grep -c 'isTombstoneVulnerableField' cmd/pdb-schema-generate/main.go` | ≥2 | 4 | yes |
| `grep -c 'TestSync_IncrementalRoleTombstone' internal/sync/worker_test.go` | ≥2 | 3 | yes |
| `grep -c 'TestPoc_RoleEmptyAcceptedByValidator' internal/sync/poc_validator_test.go` | ≥1 | 2 | yes |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 — Bug] Pre-existing gofmt drift on FieldDef struct alignment**

- **Found during:** Task 1 GREEN phase (`gofmt -s -l` on
  `cmd/pdb-schema-generate/main.go` returned the file as needing
  reformatting after my predicate addition).
- **Issue:** The `FieldDef` struct in `main.go` lines 47-56 had
  pre-existing column-alignment drift unrelated to BUG-02. The plan's
  Task 1 acceptance criterion required `gofmt -s -l ... | (! grep .)`
  clean.
- **Fix:** Ran `gofmt -s -w cmd/pdb-schema-generate/main.go` to
  collapse the alignment; whitespace-only change committed alongside
  the predicate addition (single feat commit covers both — same-file
  hygiene per the plan's acceptance criteria).
- **Files modified:** `cmd/pdb-schema-generate/main.go` (whitespace
  on lines 47-56).
- **Commit:** `d2ba190`.

### Out-of-Scope Findings (NOT auto-fixed)

**1. Pre-existing bug in TestGenerateEntSchemaCompiles "Organization" regression guard tab depth**

- **Found during:** Task 1 RED-phase debugging. The existing
  Organization `notWantParts` literal at
  `cmd/pdb-schema-generate/main_test.go:245-251` uses a raw string
  with source-file indentation:
  ```go
  `field.String("name").
              NotEmpty()`,
  ```
  This bakes the source's leading-tab count (4 tabs) into the
  comparison. The actual generator emits 3 tabs (`.\n\t\t\tNotEmpty()`).
  The Org guard happens to pass only because Org.name no longer emits
  NotEmpty at all — if it ever DID emit, the guard would fail to match
  and let a regression slip through.
- **Why not fixed:** Out of BUG-02 scope (CONTEXT.md mandates
  tombstone-vulnerability fix; this is an orthogonal test-quality
  issue). The new Phase 73 Poc + role guards I added use explicit
  `\n\t\t\t` escape strings, not source-indented raw literals — the
  Phase 73 guards are correct.
- **Recorded as:** This summary entry (no separate
  `deferred-items.md` file created — the issue is a single-line test
  hygiene fix that can be picked up by the next maintainer touching
  `main_test.go`).

### Authentication Gates

None — no auth-gated commands or services were touched.

## Files Created/Modified

### Created

- `internal/sync/poc_validator_test.go` (88 lines) — unit-level proof
  per CONTEXT.md D-03.

### Modified (hand-edited)

- `cmd/pdb-schema-generate/main.go` — added
  `isTombstoneVulnerableField` predicate (~25 lines), replaced gate
  expression `name != "name"` with `!isTombstoneVulnerableField(name)`,
  updated SEED-001 docstring, picked up pre-existing gofmt drift on
  FieldDef struct.
- `cmd/pdb-schema-generate/main_test.go` — added Poc fixture entry,
  Poc test case (`notWantParts`), role test case (`notWantSub`),
  prefix test case (forward KEEP guard), `notWantSub` field on
  TestGenerateFieldCode struct + assertion loop, defence-in-depth
  name `notWantSub` guard.
- `internal/sync/worker_test.go` — added `makePoc` fixture-builder
  and `TestSync_IncrementalRoleTombstone` (clustered immediately
  after `TestSync_IncrementalDeletionTombstone`).

### Modified (regenerated by `go generate ./...`)

- `ent/schema/poc.go` — `.NotEmpty()` removed from role chain.
- `ent/poc/poc.go` — `RoleValidator` declaration removed.
- `ent/poc_create.go` — `RoleValidator` invocation removed from
  `check()`.
- `ent/poc_update.go` — `(*PocUpdate).check()` and
  `(*PocUpdateOne).check()` removed entirely.
- `ent/runtime/runtime.go` — `pocDescRole` + `RoleValidator`
  initialiser removed from `init()`.

## Threat Model Status

All STRIDE threats from PLAN.md addressed:

- **T-73-02-01** (Tampering — sync upsert accepts role=""): MITIGATED
  via codegen-layer NotEmpty drop + dual unit + httptest regression
  guards.
- **T-73-02-02** (Information Disclosure — anonymous filter excludes
  tombstones): ACCEPTED — Phase 64 row-level privacy + Phase 68 status
  matrix already exclude tombstoned poc rows.
- **T-73-02-03** (Data Integrity — non-deleted role=""): ACCEPTED —
  upstream is the source of truth; we mirror upstream's invariants.
- **T-73-02-04** (DoS — sync ingestion path): ACCEPTED — net
  availability INCREASES (first poc tombstone now succeeds vs.
  aborting the cycle).
- **T-73-02-05** (Repudiation — codegen change auditability):
  MITIGATED via the predicate's docstring (records both 260426-pms
  citation and Phase 73 BUG-02 citation) + this summary's audit table.

No new threat surface introduced — the plan REMOVES a validator that
was blocking sync, it does not add new I/O paths or trust boundaries.

## Threat Flags

None — no new network endpoints, auth paths, file access patterns, or
schema changes at trust boundaries beyond the documented threat model.

## Self-Check: PASSED

All 9 created/modified files exist on disk. All 5 task commits exist
in git history. Full test suite (47 packages, including
internal/sync, internal/pdbcompat, internal/grpcserver, graph,
internal/web) PASS with `-race` enabled. `go generate ./...` is
idempotent (zero drift on second run). `golangci-lint run ./...`
reports 0 issues. The plan's `<verification>` checks 1-10 all pass
with the expected counts.
