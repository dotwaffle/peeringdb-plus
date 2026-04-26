---
phase: 73-code-defect-fixes
verified: 2026-04-26T22:00:00Z
status: passed
score: 18/18 must-haves verified
overrides_applied: 0
---

# Phase 73: Code Defect Fixes Verification Report

**Phase Goal:** The two known production defects from the v1.16 / 260426-pms tail are fixed and regression-locked: `campus__<field>=` cross-entity traversal returns 200, and `poc.role` no longer carries a `NotEmpty()` validator that aborts incremental sync on tombstoned rows.

**Verified:** 2026-04-26T22:00:00Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| #   | Truth                                                                                            | Status     | Evidence                                                                                                                                                                                                          |
| --- | ------------------------------------------------------------------------------------------------ | ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | BUG-01: `GET /api/<entity>?campus__<field>=X` returns HTTP 200 (was 500)                         | VERIFIED   | `grep -c 'TargetTable: "campus"' internal/pdbcompat/allowlist_gen.go` = 0; `grep -c 'TargetTable: "campuses"' ...` = 2. E2E sub-test `path_a_1hop_fac_campus_name` and parity `TRAVERSAL-05` both PASS HTTP 200. |
| 2   | `internal/pdbcompat/allowlist_gen.go` shows TargetTable="campuses" for incoming-to-Campus edges | VERIFIED   | 2 lines (174, 212) flipped to plural; 0 singular `"campus"` remain.                                                                                                                                              |
| 3   | E2E `path_a_1hop_fac_campus_name` un-skipped, asserts HTTP 200 + correct IDs                     | VERIFIED   | `traversal_e2e_test.go:167-170` asserts `expectedIDs: []int{8001}` against `seed.Full`'s `TestCampus1`. No `t.Skip` call.                                                                                       |
| 4   | Parity DIVERGENCE_ canary flipped (replaced by TRAVERSAL-05 positive test)                       | VERIFIED   | `grep -c 'DIVERGENCE_fac_campus_name' internal/pdbcompat/parity/traversal_test.go` = 0. `TRAVERSAL-05_path_a_1hop_fac_campus_name` at line 216 asserts HTTP 200 with isolated seeding.                          |
| 5   | `docs/API.md § Known Divergences` no longer claims this surface returns 500                      | VERIFIED   | `grep -n 'DEFER-70-06-01\|fac?campus__name=X' docs/API.md` returns nothing — row + deep-dive both deleted.                                                                                                    |
| 6   | `deferred-items.md` DEFER-70-06-01 marked CLOSED with v1.18.0 Phase 73 cross-reference          | VERIFIED   | Line 5: `**Status:** CLOSED — fixed in v1.18.0 Phase 73 (2026-04-26)`. DEFER-70-verifier-01 untouched (line 54).                                                                                              |
| 7   | `ent/entc.go` fixCampusInflection patch UNCHANGED (out-of-scope guard)                          | VERIFIED   | `git diff --quiet ent/entc.go` exits 0. `fixCampusInflection` still defined at line 35.                                                                                                                          |
| 8   | `ent/schema/campus.go` UNCHANGED (sibling-file pattern preserved)                                | VERIFIED   | `git diff --quiet ent/schema/campus.go` exits 0.                                                                                                                                                                 |
| 9   | BUG-02: `poc.role` field no longer carries `.NotEmpty()` validator                              | VERIFIED   | `grep -A2 'field.String("role")' ent/schema/poc.go \| grep -cF 'NotEmpty()'` = 0. Line 46 is `Annotations(...)` not `NotEmpty()`.                                                                              |
| 10  | `cmd/pdb-schema-generate/main.go` declares `isTombstoneVulnerableField` predicate              | VERIFIED   | Line 800: `func isTombstoneVulnerableField(name string) bool`. Line 329 uses `!isTombstoneVulnerableField(name)` in gate. `grep -c` = 4 occurrences (decl + use + 2 docstring refs).                              |
| 11  | `ent/schema/ixprefix.go` STILL contains `.NotEmpty()` (KEEP per audit)                          | VERIFIED   | `grep -c 'NotEmpty()' ent/schema/ixprefix.go` = 1; field is `prefix`.                                                                                                                                            |
| 12  | Generated ent files no longer carry `RoleValidator`                                              | VERIFIED   | `grep -c 'RoleValidator'` returns 0 across all 5 files: `ent/runtime/runtime.go`, `ent/poc/poc.go`, `ent/poc.go`, `ent/poc_create.go`, `ent/poc_update.go`.                                                      |
| 13  | `internal/sync/poc_validator_test.go` exists with `TestPoc_RoleEmptyAcceptedByValidator`        | VERIFIED   | File present (88 lines). `grep -c 'TestPoc_RoleEmptyAcceptedByValidator'` = 2 (declaration + godoc reference).                                                                                                  |
| 14  | `internal/sync/worker_test.go` contains `TestSync_IncrementalRoleTombstone`                     | VERIFIED   | `grep -c 'TestSync_IncrementalRoleTombstone'` = 3 (declaration + godoc + cross-ref).                                                                                                                              |
| 15  | Both BUG-02 tests pass under `-race`                                                              | VERIFIED   | `go test -race -run 'TestPoc_RoleEmptyAcceptedByValidator\|TestSync_IncrementalRoleTombstone' ./internal/sync/...` → `ok 1.415s`.                                                                                |
| 16  | `go build ./...` clean                                                                            | VERIFIED   | Build returns no errors.                                                                                                                                                                                          |
| 17  | `go generate ./...` produces zero drift on a clean tree (idempotent)                             | VERIFIED   | Two consecutive `go generate ./...` runs leave `git status --porcelain` (excluding `.idea`/`.vscode` untracked) empty.                                                                                          |
| 18  | All affected test suites pass (`internal/pdbcompat`, `internal/sync`, `cmd/pdb-schema-generate`) | VERIFIED   | All three suites: `ok ...` with no failures.                                                                                                                                                                       |

**Score:** 18/18 truths verified

### Required Artifacts

| Artifact                                             | Expected                                                                            | Status     | Details                                                                                                                                                                                              |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------- | ---------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ent/schema/campus_annotations.go`                   | New sibling file with `entsql.Annotation{Table: "campuses"}` mixin                | VERIFIED   | 1745 bytes; declares `campusTableAnnotationMixin`, `Annotations()` returns `entsql.Annotation{Table: "campuses"}`, plus `var _ ent.Mixin = ...{}` compile-time assertion.                          |
| `ent/schema/campus_fold.go`                          | Mixin slice now contains `campusTableAnnotationMixin{}`                            | VERIFIED   | Line 12: `campusTableAnnotationMixin{},` appended after `foldMixin{...}`. Single `(Campus) Mixin()` declaration — no Go duplicate-method violation.                                              |
| `internal/pdbcompat/allowlist_gen.go`               | `TargetTable: "campuses"` for every incoming-to-Campus edge                       | VERIFIED   | 0 singular survivors; 2 plural (lines 174, 212).                                                                                                                                                      |
| `internal/pdbcompat/traversal_e2e_test.go`          | New `path_a_1hop_fac_campus_name` sub-test, no `t.Skip`                            | VERIFIED   | Lines 166-170; asserts HTTP 200 + ID 8001.                                                                                                                                                            |
| `internal/pdbcompat/parity/traversal_test.go`       | `TRAVERSAL-05_path_a_1hop_fac_campus_name` replaces DIVERGENCE_ canary             | VERIFIED   | Line 216 sub-test name; `mustCampus` helper added; godoc updated.                                                                                                                                   |
| `cmd/pdb-schema-generate/main.go`                   | Declares `isTombstoneVulnerableField`; gate uses `!isTombstoneVulnerableField(name)` | VERIFIED   | Predicate at line 800 (`name == "name" \|\| name == "role"`). Gate at line 329.                                                                                                                       |
| `cmd/pdb-schema-generate/main_test.go`              | New Poc + role + prefix regression guards                                          | VERIFIED   | Per 73-02 SUMMARY; suite passes.                                                                                                                                                                      |
| `ent/schema/poc.go`                                  | Regenerated without `.NotEmpty()` on role                                          | VERIFIED   | Line 45-47: `field.String("role").Annotations(...).Comment("Contact role")` — no `NotEmpty()`.                                                                                                      |
| `internal/sync/poc_validator_test.go`               | Contains `TestPoc_RoleEmptyAcceptedByValidator` unit test                          | VERIFIED   | Test seeds parent network + poc, asserts `SetRole("")` Save succeeds, verifies `got.Role == ""` and `got.Status == "deleted"`.                                                                       |
| `internal/sync/worker_test.go`                       | Contains `TestSync_IncrementalRoleTombstone` httptest end-to-end                  | VERIFIED   | Test mirrors `TestSync_IncrementalDeletionTombstone`; verifies `?since=` sent, soft-delete, anonymous filter excludes tombstone.                                                                  |
| `docs/API.md`                                        | DEFER-70-06-01 row + deep-dive removed                                              | VERIFIED   | `grep -n 'DEFER-70-06-01' docs/API.md` returns no matches.                                                                                                                                            |
| `deferred-items.md`                                  | DEFER-70-06-01 marked CLOSED                                                        | VERIFIED   | Line 5 carries `**Status:** CLOSED — fixed in v1.18.0 Phase 73 (2026-04-26)`. Historical content preserved. DEFER-70-verifier-01 untouched.                                                       |

### Key Link Verification

| From                                              | To                                                | Via                                                | Status   | Details                                                                                                                                                                            |
| ------------------------------------------------- | ------------------------------------------------- | -------------------------------------------------- | -------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ent/schema/campus_annotations.go`                | `cmd/pdb-compat-allowlist/main.go extractEdges`   | `e.Type.Table()` reads `entsql.Annotation Table`   | WIRED    | Allowlist now emits `TargetTable: "campuses"` (verified post-codegen).                                                                                                              |
| `internal/pdbcompat/allowlist_gen.go`            | `internal/pdbcompat` traversal handler            | TargetTable in subquery SQL                        | WIRED    | E2E test asserts HTTP 200 + correct facility IDs — proves the SQL emitted by traversal handler joins the right physical table.                                                   |
| `traversal_e2e_test.go`                           | `internal/pdbcompat` handler                      | `httptest` GET `/api/fac?campus__name=...`         | WIRED    | Test passes; status=200 with `expectedIDs: []int{8001}`.                                                                                                                            |
| `cmd/pdb-schema-generate/main.go`                | `ent/schema/poc.go` (regenerated)                 | `generateFieldCode` → `schemaTemplate` render      | WIRED    | Predicate change → next `go generate` produces `field.String("role")` without `.NotEmpty()`.                                                                                       |
| `internal/sync/poc_validator_test.go`            | `ent.Client.Poc.Update()`                         | validator chain at update time (no RoleValidator) | WIRED    | Test passes; `SetRole("").Save()` returns no error.                                                                                                                                 |
| `internal/sync/worker_test.go`                    | `internal/sync/worker.go` upsert path            | `fixtureWithMeta` httptest + `Worker.Sync(...)`    | WIRED    | Test passes; tombstone is soft-deleted via the real upsert path.                                                                                                                    |
| `ent/runtime/runtime.go`                          | `ent.Client.Poc.Update().Save()`                  | validator chain (RoleValidator removed)            | WIRED    | `grep -c 'RoleValidator' ent/runtime/runtime.go` = 0; sync path no longer trips the previously-failing validator.                                                                  |

### Behavioral Spot-Checks

| Behavior                                                                            | Command                                                                                                                                                          | Result                                          | Status |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- | ------ |
| `go build ./...` builds entire repo                                                  | `go build ./...`                                                                                                                                                  | exit 0, no output                               | PASS   |
| BUG-01 traversal tests pass                                                         | `go test -race -run 'TestTraversal_E2E_Matrix/path_a_1hop_fac_campus_name\|TestParity_Traversal/TRAVERSAL-05' -count=1 ./internal/pdbcompat/... ./internal/pdbcompat/parity/...` | `ok` for both packages                          | PASS   |
| BUG-02 validator + sync tests pass                                                   | `go test -race -run 'TestPoc_RoleEmptyAcceptedByValidator\|TestSync_IncrementalRoleTombstone' -count=1 ./internal/sync/...`                                       | `ok 1.415s`                                     | PASS   |
| Combined affected suites pass                                                        | `go test -count=1 ./internal/pdbcompat/... ./internal/sync/... ./cmd/pdb-schema-generate/...`                                                                   | all 4 packages `ok`                              | PASS   |
| Codegen idempotency: two consecutive `go generate` produce zero drift                | `go generate ./...` then `git status --porcelain` (excluding untracked editor folders)                                                                            | clean working tree                              | PASS   |
| Singular-target table absent in allowlist                                            | `grep -c 'TargetTable: "campus"' internal/pdbcompat/allowlist_gen.go`                                                                                              | `0`                                              | PASS   |
| Plural-target table present in allowlist                                             | `grep -c 'TargetTable: "campuses"' internal/pdbcompat/allowlist_gen.go`                                                                                            | `2` (≥2 expected)                                | PASS   |

### Requirements Coverage

| Requirement | Source Plan | Description                                                                          | Status     | Evidence                                                                                                                              |
| ----------- | ----------- | ------------------------------------------------------------------------------------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| BUG-01      | 73-01       | `GET /api/<type>?campus__<field>=<value>` returns 200 (currently 500)                | SATISFIED  | `entsql.Annotation{Table: "campuses"}` mixin pinned via `ent/schema/campus_annotations.go`; allowlist regen emits plural; E2E + parity tests assert HTTP 200; DEFER-70-06-01 closed. |
| BUG-02      | 73-02       | Sync upsert path accepts `poc.role=""` tombstones without validator failure          | SATISFIED  | `isTombstoneVulnerableField` predicate added; `ent/schema/poc.go` regenerated without `.NotEmpty()` on role; cascade through `ent/poc/poc.go`, `ent/poc_create.go`, `ent/poc_update.go`, `ent/runtime/runtime.go` removes `RoleValidator`; both unit (TestPoc_RoleEmptyAcceptedByValidator) and httptest (TestSync_IncrementalRoleTombstone) regression guards pass per CONTEXT.md D-03. |

No orphaned requirements: the only two IDs scoped to Phase 73 in REQUIREMENTS.md (`BUG-01`, `BUG-02`) are both claimed and covered by 73-01 and 73-02 respectively.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |

No blocker or warning anti-patterns introduced by Phase 73 work. Test-tooling observation captured below.

### Out-of-Scope Observation (informational, not blocking)

Per the 73-02 executor's SUMMARY (D-T2), the existing `TestGenerateEntSchemaCompiles` "Organization" `notWantParts` regression guard at `cmd/pdb-schema-generate/main_test.go:245-251` uses a source-indented raw string (4 leading tabs) while the generator emits 3 tabs (`\n\t\t\t`). The Org guard would fail to match if `NotEmpty()` ever re-appeared on `organizations.name`. The new Phase 73 Poc + role guards correctly use explicit `\n\t\t\t` escape strings and are unaffected. Out of BUG-02 strict scoping per CONTEXT.md D-02; surface here as a follow-up candidate for a future test-hygiene quick task.

### Human Verification Required

None — all goal-backward checks are programmatically verifiable via grep/test/build, and all checks passed on this environment.

### Gaps Summary

No gaps. All 18 must-have truths verified, both requirement IDs (BUG-01, BUG-02) satisfied, and all behavioral spot-checks (build, test, codegen idempotency, grep invariants) pass. The phase goal — both production defects fixed and regression-locked — is achieved.

---

_Verified: 2026-04-26T22:00:00Z_
_Verifier: Claude (gsd-verifier)_
