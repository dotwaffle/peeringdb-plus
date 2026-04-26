---
phase: 73
slug: code-defect-fixes
milestone: v1.18.0
status: context-locked
has_context: true
locked_at: 2026-04-26
---

# Phase 73 Context: Code Defect Fixes

## Goal

Resolve the two known production code defects: the campus inflection 500 in cross-entity traversal (DEFER-70-06-01), and the `poc.role` `NotEmpty()` validator that would silently break incremental sync the first time upstream emits a `role=""` tombstone.

## Requirements

- **BUG-01** — `GET /api/<type>?campus__<field>=<value>` returns 200 (currently 500)
- **BUG-02** — Sync upsert path accepts `poc.role=""` tombstones without validator failure

## Locked decisions

- **D-01 — BUG-01 fix shape: schema annotation, not CLI patch.** Apply `entsql.Annotation{Table: "campuses"}` to `ent/schema/campus.go`. Single source of truth — every tool reading entc.LoadGraph (current: `cmd/pdb-compat-allowlist`; future: any new codegen tool) sees the right table name without per-tool patching. The existing `fixCampusInflection` go:linkname patch in `ent/entc.go` stays in place for ent's own internal codegen path (it's load-bearing for runtime). The schema annotation is purely additive and does not duplicate or conflict with the existing patch.

- **D-02 — BUG-02 scope: audit + drop NotEmpty where tombstone-vulnerable.** Don't narrow to just `poc.role`. Scan all 13 entities' string fields for `NotEmpty()`; for each occurrence, decide whether it's tombstone-vulnerable (i.e., upstream PeeringDB plausibly scrubs the field on delete per the GDPR pattern already empirically confirmed for org/network/etc.). Most likely candidates beyond `role`: `poc.email`, `poc.phone`, `poc.url`, possibly `poc.aka` if it exists. Document the per-field decision (drop vs keep) in the planner's PLAN.md so the rationale is auditable. Apply via `cmd/pdb-schema-generate` (codegen-layer fix per the 260426-pms pattern), not direct schema edits — direct edits would get stripped on next `go generate`.

- **D-03 — Test shape: both unit and httptest.** Unit test (`ent.Client.Poc.Update().SetRole("").Save()` succeeds without validator failure) gives fast feedback during development and short CI cycles. httptest fake-upstream conformance test mirrors the 260426-pms pattern — fake upstream emits a `?since=` response with a `role=""` tombstone, drive a real sync cycle, assert poc row's status flips to `'deleted'`. The httptest version is the regression guard; the unit test is the local-iteration affordance. Both committed.

## Out of scope

- Removing the existing `fixCampusInflection` go:linkname patch from `ent/entc.go` — that's load-bearing for ent's runtime codegen path and unrelated to BUG-01. Keep it.
- Cross-entity traversal beyond the campus-target case — the `cmd/pdb-compat-allowlist` allowlist_gen.go's TargetTable correctness for non-campus entities was already verified during Phase 70 (per CLAUDE.md § Cross-entity `__` traversal Phase 70 D-02 amended).
- Adding new validators to other fields — this phase REMOVES validators that block sync, not the reverse.

## Dependencies

- **Depends on**: None (independent of phases 74-78; can run in parallel).
- **Enables**: Future phases that exercise full cross-entity traversal coverage; Phase 78's UAT-02 implicit dependency on `/api/*` endpoints not 500-ing.

## Plan hints for executor

- Touchpoints:
  - `ent/schema/campus.go` — add `entsql.Annotation{Table: "campuses"}` import + Annotations() method (use a sibling file `campus_annotations.go` per CLAUDE.md sibling-file convention if `Annotations()` doesn't already exist on the generated schema, since `cmd/pdb-schema-generate` regenerates `campus.go` itself)
  - `cmd/pdb-schema-generate/main.go` — extend the existing `isNameField` gate (added by 260426-pms) to a more general `isTombstoneVulnerableField` predicate; document per-field decisions inline
  - `cmd/pdb-schema-generate/main_test.go` — extend `notWantParts` regression guards for new dropped validators
  - `ent/schema/poc.go` (regenerated) — verify `.NotEmpty()` is absent from `role` (and other audited fields) post-codegen
  - `internal/pdbcompat/traversal_e2e_test.go` — un-skip `path_a_1hop_fac_campus_name`; assert HTTP 200 with matching facility rows
  - `internal/sync/worker_test.go` — new `TestSync_IncrementalRoleTombstone` mirror of the 260426-pms test pattern
- Reference docs:
  - `.planning/milestones/v1.16-phases/70-cross-entity-traversal/deferred-items.md` (DEFER-70-06-01 root cause analysis)
  - `.planning/quick/260426-pms-flip-default-pdbplus-sync-mode-to-increm/260426-pms-PLAN.md` § Subtask 1C (NotEmpty fix pattern)
  - `cmd/pdb-schema-generate/main.go` `isNameField` (existing 260426-pms helper)
- Verify on completion:
  - `go test ./internal/pdbcompat/... -run TestTraversal` passes
  - `go generate ./...` produces zero drift on a clean tree
  - `golangci-lint run ./...` clean
  - `docs/API.md § Known Divergences` updated: DEFER-70-06-01 entry flipped from "open" to "fixed in v1.18.0"
