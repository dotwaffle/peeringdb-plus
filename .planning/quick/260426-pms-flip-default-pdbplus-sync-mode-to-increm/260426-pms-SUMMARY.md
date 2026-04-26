---
phase: 260426-pms
plan: 01
subsystem: sync
tags: [config, ent, otel, metrics, seed-001, incremental-sync, tombstones, codegen]

# Dependency graph
requires:
  - phase: 68-soft-delete-tombstones
    provides: status='deleted' tombstones as first-class rows; markStaleDeleted* gating
  - phase: 69-shadow-column-folding
    provides: 6 folded entities (org, network, facility, ix, carrier, campus) — name/aka/city _fold columns
provides:
  - PDBPLUS_SYNC_MODE default flipped from full → incremental (SEED-001 trigger fired 2026-04-26)
  - cmd/pdb-schema-generate no longer emits NotEmpty() on the "name" field — preserved for "prefix" and "role"
  - 6 folded entities accept upstream PII-scrubbed name="" tombstones at upsert time
  - TestSync_IncrementalDeletionTombstone regression guard for the empty-name path
  - pdbplus.sync.{duration,operations} carry mode={full,incremental} attribute
affects: [v1.18.0 milestone planning, future operator dashboards, sync alerting]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Codegen-time NotEmpty() gating: name field on the 6 folded entities is non-validated post-Phase 68 to accept upstream tombstones"
    - "Sync metric labelling pattern: bundle status + mode in shared metric.WithAttributes() for both success and failure paths"
    - "Explicit mode dataflow: rollbackAndRecord/recordFailure receive mode as 2nd parameter (after ctx) per GO-CFG-2"

key-files:
  created:
    - .planning/quick/260426-pms-flip-default-pdbplus-sync-mode-to-increm/260426-pms-SUMMARY.md
  modified:
    - internal/config/config.go (SyncMode default + godoc + slog.Info startup line)
    - internal/config/config_test.go (TestLoad_SyncMode default flip)
    - cmd/pdb-schema-generate/main.go (skip NotEmpty() emission for "name")
    - cmd/pdb-schema-generate/main_test.go (test expectations + new notWantParts regression guards)
    - ent/schema/{organization,network,facility,internetexchange,carrier,campus}.go (regenerated — NotEmpty() removed)
    - ent/{organization,network,facility,internetexchange,carrier,campus}{,_create,_update}.go (regenerated client)
    - ent/{organization,network,facility,internetexchange,carrier,campus}/{name}.go (regenerated query helpers)
    - ent/runtime/runtime.go (regenerated)
    - CLAUDE.md (env var table row for PDBPLUS_SYNC_MODE)
    - internal/sync/worker.go (recordSuccess + rollbackAndRecord + recordFailure: mode label + threading)
    - internal/sync/worker_test.go (TestSync_IncrementalDeletionTombstone)

key-decisions:
  - "Modify the schema generator (cmd/pdb-schema-generate/main.go isNameField gate), not just the per-entity schema files: the schema generator regenerates ent/schema/*.go on every `go generate ./...`, so a hand-edit on the entity files would be silently stripped per CLAUDE.md § Code Generation. Codegen-layer change is the durable fix."
  - "Preserve NotEmpty() for `prefix` (ixprefix.prefix — IP prefix, structurally meaningful, not in the PII scrub set) and `role` (poc.role — separate visibility story handled by Phase 64). Plan was explicit: only the 6 folded entities are in scope."
  - "rollbackAndRecord and recordFailure signatures take `mode` as the 2nd parameter (immediately after ctx) per GO-CFG-2 — explicit dataflow > implicit context lookup. Avoids span-name parsing or worker-state mutation."

patterns-established:
  - "Sibling-file-style codegen rationale comments: when the schema generator decides whether to emit a validator, the rationale (which entity types, why) lives as a Go comment in cmd/pdb-schema-generate/main.go where future maintainers will find it on the next codegen review."
  - "Metric attribute bundling: when adding a new low-cardinality dimension, replace single-attr `metric.WithAttributes(attribute.String(...))` calls with bundled multi-attr blocks. Cardinality budget is documented in the surrounding comment."

requirements-completed: [SEED-001]

# Metrics
duration: 13min
completed: 2026-04-26
---

# Quick Task 260426-pms: Flip default PDBPLUS_SYNC_MODE to incremental Summary

**Flipped sync default to incremental, regenerated 6 folded-entity schemas without NotEmpty() on `name` so PII-scrubbed tombstones from upstream `?since=` no longer break sync, and labelled `pdbplus.sync.{duration,operations}` with `mode={full,incremental}`.**

## Performance

- **Duration:** ~13 min
- **Started:** 2026-04-26T18:40:30Z
- **Completed:** 2026-04-26T18:53:00Z
- **Tasks:** 3
- **Files modified:** 30 (3 hand-edited + 1 generator + 6 schemas + ~17 regenerated ent files + 2 docs + 1 test)

## Accomplishments

- Default `PDBPLUS_SYNC_MODE` flipped `full → incremental`. SEED-001 trigger fired 2026-04-26 (live spike against www.peeringdb.com confirmed `?since=` emits `status="deleted"` tombstones with PII-scrubbed `name=""`).
- Schema generator (`cmd/pdb-schema-generate/main.go`) updated to skip `NotEmpty()` emission for the `name` field. The 6 folded entity schemas (`organization`, `network`, `facility`, `internetexchange`, `carrier`, `campus`) regenerated cleanly; ent client `NameValidator` artefacts removed across `ent/{type}{,_create,_update}.go`.
- `TestSync_IncrementalDeletionTombstone` end-to-end regression guard — drives 2 sync cycles via `fixtureWithMeta`, asserts cycle 2 sends `?since=`, the `name=""` tombstone flips an existing org's `status` to `"deleted"` without hard-deleting, and the anonymous `status="ok"` filter returns only the live row.
- `pdbplus.sync.duration` and `pdbplus.sync.operations` carry `mode={full,incremental}` attribute (in addition to existing `status={success,failed}`). `recordFailure` and `rollbackAndRecord` signatures extended with `mode config.SyncMode` (immediately after `ctx`) per GO-CFG-2.
- CLAUDE.md env var table row updated.
- Startup `slog.Info("sync mode configured", mode=...)` added so operators see the resolved mode.

## Task Commits

Each task was committed atomically:

1. **Task 1: Flip default + audit name validators** — `f1e612b` (feat)
2. **Task 2: Conformance test — incremental tombstone end-to-end** — `c6ae276` (test)
3. **Task 3: Sync metrics — add mode label** — `a3ae545` (feat)

## Files Created/Modified

### Created
- `.planning/quick/260426-pms-flip-default-pdbplus-sync-mode-to-increm/260426-pms-SUMMARY.md` — this file.

### Modified — hand-edited
- `internal/config/config.go` — `parseSyncMode` default flipped to `SyncModeIncremental`; godoc updated; startup `slog.Info("sync mode configured", ...)` added next to existing "sync interval configured" line.
- `internal/config/config_test.go` — `TestLoad_SyncMode` default-case asserts `SyncModeIncremental`; godoc references SEED-001.
- `cmd/pdb-schema-generate/main.go` — `generateFieldCode` `name` short-circuit: `isNameField(name) && name != "name"` so `prefix` and `role` retain their NotEmpty() emission. Also fixed pre-existing import order (`cmp` was misplaced).
- `cmd/pdb-schema-generate/main_test.go` — `TestGenerateFieldCode` `name` case no longer asserts `NotEmpty()`; `TestGenerateEntSchemaCompiles` `Organization` `notWantParts` extended with a regression guard against re-adding `NotEmpty()`.
- `CLAUDE.md` — env var table row for `PDBPLUS_SYNC_MODE`.
- `internal/sync/worker.go` — `recordSuccess` bundles `status` + `mode` in shared `attrs`; `rollbackAndRecord` and `recordFailure` signatures gain `mode config.SyncMode` (2nd parameter); 4 `recordFailure` + 3 `rollbackAndRecord` call sites in `Worker.Sync` thread `mode` through.
- `internal/sync/worker_test.go` — `TestSync_IncrementalDeletionTombstone`.

### Modified — regenerated by `go generate ./...`
- `ent/schema/{organization,network,facility,internetexchange,carrier,campus}.go` — `NotEmpty()` removed from `name` field (driven by generator change).
- `ent/{organization,network,facility,internetexchange,carrier,campus}.go` — Field comment updated.
- `ent/{organization,network,facility,internetexchange,carrier,campus}_{create,update}.go` — `NameValidator` validator block removed from `check()`; `name` field still required (`ValidationError` for missing).
- `ent/{organization,network,facility,internetexchange,carrier,campus}/{name}.go` — `NameValidator` declaration removed.
- `ent/runtime/runtime.go` — `NameValidator` initialiser block removed.

## Decisions Made

- **Schema generator change vs per-entity edit (Subtask 1C).** The plan suggested editing the 6 schema files directly with the caveat that `cmd/pdb-schema-generate` regenerates them on every `go generate ./...`. The critical warning correctly anticipated this. Per option (a), I traced `NotEmpty()` to its emission point in `generateFieldCode` (gated by `isNameField(name)` which returns true for `name`/`prefix`/`role`) and added a `name != "name"` exclusion. Rationale comment lives in the generator source where it'll be visible on the next codegen review. The 6 schema files plus all regenerated ent client files (validator declarations, `check()` calls, runtime initialiser) are now stable across two consecutive `go generate ./...` runs (verified via `git diff --quiet ent/ gen/ graph/ internal/web/templates/`).
- **Preserve NotEmpty() for `prefix` and `role`.** The plan was explicit ("Do NOT touch any other entity"). The rationale: `prefix` (ixprefix.prefix) is structurally meaningful (an IP prefix, not in the PII scrub set); `role` (poc.role) is in Phase 64's separate visibility story. Whether `poc.role` tombstones with `role=""` are a real future bug is **not** in scope for this quick task — see "Out of Scope Findings" below.
- **`rollbackAndRecord`/`recordFailure` signature change vs context lookup vs span-name parsing.** Option (a) from the plan — extend signatures with `mode config.SyncMode` as the 2nd parameter — was the cleanest. Explicit dataflow > implicit lookup (GO-CFG-2). 8 call sites in `Worker.Sync` are mechanical to update (now grep-greppable: `grep "rollbackAndRecord\|recordFailure"` shows mode threaded through all sites).
- **gofmt fixup on `cmd/pdb-schema-generate/main.go` import order.** Pre-existing minor issue surfaced by the verify command; fixed inline since I was already touching the file. The struct alignment `gofmt -s` warning on `FieldDef` (also pre-existing) was left alone — out of scope.

## Deviations from Plan

None substantive. The plan called out one optional fallback (write a test proving the validator is bypassed by upsert — and revert the schema edits if so); that path was not needed because removing `NotEmpty()` at codegen time worked cleanly and the conformance test passes against the genuine fix.

The plan's verify command for Task 1 says "the only diff in `ent/` should be the absence of the `validators.NotEmpty` wrapper on those 6 name fields". The actual ent diff is broader (4 layers per entity: schema field, struct comment, `check()` validator block, where-helper `NameValidator` declaration) but is exactly the expected codegen-driven cascade — `git diff --quiet ent/` after a second `go generate ./...` confirms idempotency.

## Issues Encountered

None. The schema generator change worked the first time; the conformance test passed first run; the metric label threading compiled cleanly and all tests in `./internal/sync/...` and `./internal/otel/...` pass.

## Out of Scope Findings

**`poc.role` and `ixprefix.prefix` retain `NotEmpty()` but might also see tombstones with empty values.** The empirical SEED-001 spike was on `poc` rows, where tombstones had `name=""`. The plan was explicit: only the 6 folded entities (`organization`, `network`, `facility`, `internetexchange`, `carrier`, `campus`) are in scope. However:

- `poc.name` is currently `Optional().Default("")` (no NotEmpty) — that's fine, pos tombstones with `name=""` work.
- `poc.role` is `NotEmpty()`. If upstream tombstones have `role=""` (likely, given GDPR-style PII scrub clears multiple PII fields), syncing a poc tombstone would fail at the upsert validator.
- `ixprefix.prefix` is `NotEmpty()`. ixprefix is a structural object (IP prefixes are not PII), so tombstones probably retain the prefix value — but upstream behaviour for non-poc tombstones is not yet confirmed.

This is a separate concern from this quick task (default flip + name PII path). Tracking note: if a future poc-deletion incident produces a sync failure with `validator failed for field "Poc.role"`, this is the trigger to extend the codegen change to `role`. Not actioned now per plan scope.

## Next Phase Readiness

- v1.18.0 milestone planning can proceed. SEED-001 has its trigger fired and remaining work shipped (as v1.17.0 release tag, not a milestone).
- The conformance test (`TestSync_IncrementalDeletionTombstone`) is in place to catch any future regression of the empty-name path.
- New `mode` label on `pdbplus_sync_operations_total` and `pdbplus_sync_duration_seconds` is available for operator dashboard consumption (Grafana / Mimir). Existing dashboards continue to work — `mode` is additive.

## Self-Check: PASSED

Verifications:
- `go build ./...` — passes.
- `go test -race ./...` — full suite passes (sync, otel, config, schema, parity, ent, all surfaces).
- `go vet ./...` — clean.
- `golangci-lint run` — `0 issues.`
- `go generate ./...` — zero drift on second run (`git diff --quiet ent/ gen/ graph/ internal/web/templates/`).
- `grep -n 'PDBPLUS_SYNC_MODE.*incremental' internal/config/config.go CLAUDE.md` — both default + docs reference incremental.
- `grep -n 'attribute.String("mode"' internal/sync/worker.go` — 2 occurrences (recordSuccess L732, recordFailure L1422).

Files exist:
- `internal/config/config.go` — FOUND
- `internal/config/config_test.go` — FOUND
- `internal/sync/worker.go` — FOUND
- `internal/sync/worker_test.go` — FOUND (with `TestSync_IncrementalDeletionTombstone`)
- `cmd/pdb-schema-generate/main.go` — FOUND
- `ent/schema/{organization,network,facility,internetexchange,carrier,campus}.go` — FOUND (all 6, no NotEmpty on name)
- `CLAUDE.md` — FOUND (table row updated)

Commits exist:
- `f1e612b` — FOUND (Task 1)
- `c6ae276` — FOUND (Task 2)
- `a3ae545` — FOUND (Task 3)

---
*Quick task: 260426-pms*
*Completed: 2026-04-26*
