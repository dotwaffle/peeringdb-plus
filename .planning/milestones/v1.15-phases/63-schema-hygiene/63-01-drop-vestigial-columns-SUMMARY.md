---
phase: 63-schema-hygiene
plan: 01
subsystem: database
tags: [entgo, entproto, sqlite, litefs, schema-migration, pdbcompat, grpcserver]

# Dependency graph
requires:
  - phase: 57-visibility-baseline-capture
    provides: Empirical anon fixture corpus that confirmed ixpfx.notes is absent from upstream
  - phase: 59-read-path-privacy
    provides: ent Privacy framework & poc.go Policy() that must NOT be stripped by schema regen
  - phase: 60-api-surface-integration
    provides: TestAnonParityFixtures + knownDivergences map (the ixpfx.notes entry this plan removes)
provides:
  - ixprefix.notes dropped from ent schema, peeringdb wire struct, serializer, registry, filter, upsert, goldens
  - organization.fac_count / organization.net_count dropped from ent schema (were schema-only)
  - migrate.WithDropColumn(true) + WithDropIndex(true) wired permanently at the runtime Schema.Create call site
  - Regression-locking micro-tests asserting the new wire shape (no "notes" on ixpfx, no count keys on org)
  - deprecatedFilterFields exemption mechanism in filter_test.go for frozen-proto wire compat
  - TestAnonParityFixtures now passes with an empty knownDivergences map
affects: [phase-64-field-privacy, phase-65-asymmetric-fleet, any future schema hygiene drop]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Edit schema/peeringdb.json (canonical JSON) AND ent/schema/*.go together when dropping fields — prevents go generate drift"
    - "migrate.WithDropColumn(true) permanent at Schema.Create call site for future hygiene drops"
    - "deprecatedFilterFields table in filter_test.go to exempt frozen-proto fields from the reflection-based filter-coverage invariant"
    - "Use go generate ./ent (not ./...) to avoid stripping hand-added Policy() on ent/schema/poc.go — the schema generator is lossy for hand-edits"

key-files:
  created: []
  modified:
    - ent/schema/ixprefix.go (drop notes field)
    - ent/schema/organization.go (drop net_count + fac_count)
    - schema/peeringdb.json (canonical source: drop ixpfx.fields.notes + org.computed_fields entries)
    - ent/ (regenerated: ixprefix/, organization/, migrate/schema.go, runtime/runtime.go, rest/, gql_*, mutation.go)
    - graph/schema.graphqls + graph/generated.go + graph/custom.resolvers.go (gqlgen regen)
    - internal/peeringdb/types.go (drop Notes from IxPrefix)
    - internal/pdbcompat/serializer.go (drop Notes emission; rewrite doc comment)
    - internal/pdbcompat/registry.go (drop "notes" from TypeIXPfx block)
    - internal/pdbcompat/anon_parity_test.go (empty knownDivergences)
    - internal/pdbcompat/serializer_test.go (new TestIxPrefixFromEnt_NoNotesKey + TestOrganizationJSON_NoCountKeys + keys helper)
    - internal/pdbcompat/testdata/golden/ixpfx/{list,detail,depth}.json (regenerated — no "notes")
    - internal/pdbcompat/testdata/golden/ixlan/depth.json (nested ixpfx_set — no "notes")
    - internal/grpcserver/ixprefix.go (drop Notes filter x2 + ixPrefixToProto Notes assignment)
    - internal/grpcserver/organization.go (drop NetCount + FacCount from proto converter)
    - internal/grpcserver/grpcserver_test.go (drop SetNotes seeds + "filter by notes" test cases)
    - internal/grpcserver/filter_test.go (add deprecatedFilterFields + entityName param to countFilterableFields)
    - internal/sync/upsert.go (drop SetNotes from IxPrefix upsert chain)
    - internal/sync/testdata/refactor_parity.golden.json (regenerated)
    - cmd/peeringdb-plus/main.go (import ent/migrate + pass WithDropColumn / WithDropIndex)
    - .planning/PROJECT.md (Key Decisions row for Phase 63)
    - CLAUDE.md (Schema & Visibility — add Phase 63 drops subsection)
    - go.sum (gqlgen transitive cleanup)

key-decisions:
  - "Edit schema/peeringdb.json AND ent/schema/*.go together — JSON is the canonical source for pdb-schema-generate and must stay idempotent"
  - "Use `go generate ./ent` not `go generate ./...` because the schema generator strips the hand-added Policy() on ent/schema/poc.go (pre-existing project-level issue, surfaced again here)"
  - "Proto/peeringdb/v1/v1.proto is frozen since v1.6 (entproto.SkipGenFile in ent/entc.go) — dropped ent fields whose proto wrappers still exist (IxPrefix.notes, Organization.{fac,net}_count) remain declared but unpopulated. Accepted cosmetic wire-compat note, no external proto consumers"
  - "migrate.WithDropColumn(true) + WithDropIndex(true) stay on permanently for all future hygiene drops"
  - "deprecatedFilterFields exemption mechanism handles frozen-proto divergence cleanly without suppressing the invariant for live fields"

patterns-established:
  - "Schema drop workflow: edit canonical JSON + ent/schema + hand-written consumers + regen goldens + one commit per group"
  - "Filter-coverage invariant with an entity-keyed deprecation allowlist for intentional divergences"

requirements-completed: [HYGIENE-01, HYGIENE-02, HYGIENE-03]

# Metrics
duration: ~20min
completed: 2026-04-18
---

# Phase 63 Plan 01: Schema hygiene — drop vestigial columns Summary

**Dropped three ent schema fields (ixprefix.notes, organization.fac_count, organization.net_count) across all layers (schema, ent codegen, pdbcompat, grpcserver, sync upsert, goldens, proto-converter), wired migrate.WithDropColumn(true) + WithDropIndex(true) at the Schema.Create call site, and closed the v1.14 pdbcompat ixpfx.notes divergence by resolving it at source.**

## Performance

- **Duration:** ~20 minutes (wall clock, from worktree base reset to this SUMMARY)
- **Started:** 2026-04-18T01:26:00Z (approximate — first schema edit)
- **Completed:** 2026-04-18T01:44:55Z
- **Tasks:** 5 (plus 1 post-lint fixup folded into Task 5)
- **Files modified:** 22 source/codegen + 3 planning/doc + 3 goldens + 1 sync golden + 1 go.sum

## Accomplishments

- All 5 tasks completed; schema is clean, generated code is consistent, tests + lint + govulncheck green.
- TestAnonParityFixtures passes with an empty `knownDivergences` map — the v1.14 ixpfx.notes divergence is resolved at source.
- Runtime auto-migrate will emit `ALTER TABLE DROP COLUMN` on next primary startup for the three dropped columns; subsequent hygiene drops inherit the same mechanism.
- Defensive regression tests TestIxPrefixFromEnt_NoNotesKey + TestOrganizationJSON_NoCountKeys lock the new wire shape.
- Proto-vs-ent drift (proto frozen since v1.6) is handled cleanly via a new `deprecatedFilterFields` exemption in the gRPC filter invariant test.

## Task Commits

1. **Task 1: Drop three fields from ent schema and regenerate** — `2646821` (feat)
   Dropped the three ent fields + updated schema/peeringdb.json + ran `go generate ./ent` → regenerated ent/, graph/schema.graphqls.
2. **Task 2: Update hand-written consumers + regenerate goldens** — `dbcade1` (feat, folded RED+GREEN+REFACTOR for TDD micro-tests)
   Updated peeringdb.IxPrefix wire struct, serializer, registry, filter wiring, grpcserver proto converters (ixprefix + organization), sync upsert, golden fixtures, defensive micro-tests.
3. **Task 3: Wire migrate.WithDropColumn + WithDropIndex at runtime call site** — `4644da1` (feat)
   Added ent/migrate import to main.go and passed both options to Schema.Create on primary.
4. **Task 4: Doc sweep per D-05** — `73a729a` (docs)
   Added Key Decisions row to PROJECT.md; added Phase 63 drops subsection to CLAUDE.md Schema & Visibility; docs/ARCHITECTURE.md and docs/API.md were grep-clean, no edits needed (grep evidence below).
5. **Task 5: Phase gate (plus one lint fixup)** — `ce84323` (chore)
   Verification: drift check passed, full test suite green with -race, revive flagged one unused-parameter in graph/custom.resolvers.go (collateral from gqlgen regen in Task 2); renamed `ctx` back to `_`.

## Phase-gate command exit codes

| Command | Exit | Notes |
|---|---|---|
| `go generate ./ent && git diff --exit-code ent/ gen/ graph/ proto/ internal/web/templates/` | 0 | No drift. (Used `./ent` — see "Deviations" #1.) |
| `go generate ./internal/web/templates/...` | 0 | No drift. |
| `go test -race ./...` | 0 | All packages pass. |
| `golangci-lint run` | 0 | Clean after the Task-5 revive fixup. |
| `govulncheck ./...` | 0 | No vulnerabilities found. |

## Doc sweep grep evidence (Task 4)

Per D-05, all four doc surfaces were swept. Evidence:

- **`.planning/PROJECT.md`:** Added a new Key Decisions row (line 219). `grep -c "Phase 63" .planning/PROJECT.md` → 2 (section ref + row). `grep -c "ixpfx.notes\|ixprefix.notes" .planning/PROJECT.md` → 4 (one is the row; others are earlier scope statements).
- **`CLAUDE.md`:** Added "Schema hygiene drops (v1.15 Phase 63)" subsection under Schema & Visibility. `grep -c "WithDropColumn" CLAUDE.md` → 1.
- **`docs/ARCHITECTURE.md`:** No edits. `grep -nE 'ixpfx\.notes|ix_prefixes\.notes|organizations\.(fac|net)_count' docs/ARCHITECTURE.md` → 0 matches. The file's two ixpfx mentions are in plaintext type lists (line 77, 241) describing the 13 PeeringDB types, not field enumerations — untouched.
- **`docs/API.md`:** No edits. `grep -nE 'ixpfx.*notes|fac_count|net_count' docs/API.md` → 0 matches. The file describes API surfaces at the type level, not per-field.

## Decisions Made

See `key-decisions` in the frontmatter. The most important non-plan-driven decision was to edit **`schema/peeringdb.json`** alongside `ent/schema/*.go` — without this, the JSON would still declare the dropped fields, and anyone running `go generate ./schema` (or `go generate ./...`) would reintroduce them. This was not in the plan's `files_modified` list but is required for long-term schema stability.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 — Blocking] Cannot use `go generate ./...` verbatim; schema regenerator strips hand-added `Policy()` on `ent/schema/poc.go`**
- **Found during:** Task 1 (first `go generate ./...` run)
- **Issue:** The plan's Task 1 action step 3 says `TMPDIR=/tmp/claude-1000 go generate ./...` but Task 1 step 4 warns `Do NOT run go generate ./schema`. These are contradictory because `go generate ./...` traverses every `//go:generate` directive, including `schema/generate.go`. The schema generator (cmd/pdb-schema-generate) is a lossy full-file emitter that strips the hand-added `Policy()` method on ent/schema/poc.go (v1.14 Phase 59 feature). Dropping the schema regen step also meant `ent/schema/*.go` would not be regenerated from the updated JSON automatically.
- **Fix:** Edited both `schema/peeringdb.json` (drop ixpfx.fields.notes + drop net_count/fac_count from org.computed_fields) AND `ent/schema/*.go` (hand-edit the same drops) together, then ran `go generate ./ent` and `go generate ./internal/web/templates/...` only — skipping `./schema`. Documented the workaround and long-term guidance in CLAUDE.md Schema & Visibility.
- **Files modified:** schema/peeringdb.json (new), ent/schema/ixprefix.go, ent/schema/organization.go, plus the ent codegen regen.
- **Verification:** `go generate ./ent && git diff --exit-code ent/` returned 0. poc.go Policy() preserved. Build + tests + lint green.
- **Committed in:** `2646821` (Task 1).

**2. [Rule 1 — Bug] `internal/grpcserver/organization.go` referenced the dropped `o.NetCount` / `o.FacCount` fields but was not in the plan's files_modified list**
- **Found during:** Task 2 (first `go build ./...` after the ent regen)
- **Issue:** The plan's `<context>` and RESEARCH.md's "Codegen pipeline impact" explicitly claimed "Organization FacCount/NetCount are NOT referenced in the pdbcompat registry, grpcserver, or serializer" — but `internal/grpcserver/organization.go` lines 206-207 emit `NetCount: int64Val(o.NetCount)` and `FacCount: int64Val(o.FacCount)` into the proto converter. Build failed.
- **Fix:** Removed those two lines from organization.go with an inline comment citing D-02 + the frozen-proto note. Proto keeps the fields but they now serialize as zero-value pointers.
- **Files modified:** internal/grpcserver/organization.go.
- **Verification:** `go build ./...` exit 0.
- **Committed in:** `dbcade1` (Task 2).

**3. [Rule 1 — Bug] Two grpcserver test cases seeded rows with `SetNotes("primary v4")` on IxPrefix and exercised `"filter by notes"`**
- **Found during:** Task 2 (`go vet` after build)
- **Issue:** grpcserver_test.go:709 and :4750 seeded IxPrefix rows with `SetNotes("primary v4")` — but SetNotes was removed from ent/ixprefix by the codegen regen. Additionally, two table-driven test cases at :756 and :4791 asserted `wantLen: 1` for requests with `Notes: new("primary v4")` — the filter wiring is now gone, so those assertions would be wrong (filter is ineffective, not restrictive).
- **Fix:** Dropped both SetNotes lines and both "filter by notes" test cases, with a comment citing D-01 and noting that the proto field is still present but the server ignores it.
- **Files modified:** internal/grpcserver/grpcserver_test.go.
- **Verification:** `go vet` + `go test -race ./internal/grpcserver/...` both exit 0.
- **Committed in:** `dbcade1` (Task 2).

**4. [Rule 1 — Bug] `TestAllFilterFieldsExercised/ixprefix` failed because the reflection-based filter invariant expected filter table length to match proto field count**
- **Found during:** Task 2 (full `go test ./internal/grpcserver/... -race`)
- **Issue:** The test reflects proto descriptor fields and asserts `len(xListFilters) == filter-candidate count`. Proto still declares `notes` as an optional field (frozen since v1.6), but the filter wiring dropped its entry; the test incorrectly diagnosed this as "a new field was added without a filter table entry."
- **Fix:** Added a `deprecatedFilterFields` allowlist keyed `"ixprefix/notes"` and threaded `entityName` through `countFilterableFields` so proto-declared-but-server-ignored fields are subtracted from the expected count. Documented the mechanism in-line + CLAUDE.md.
- **Files modified:** internal/grpcserver/filter_test.go.
- **Verification:** `go test ./internal/grpcserver/... -run TestAllFilterFieldsExercised -race` exit 0.
- **Committed in:** `dbcade1` (Task 2).

**5. [Rule 1 — Bug] `internal/sync/testdata/refactor_parity.golden.json` was stale after the upsert change**
- **Found during:** Task 2 (`go test -race ./...`)
- **Issue:** TestSync_RefactorParity compares sync output byte-for-byte against a golden file. Dropping `SetNotes(ip.Notes)` from the IxPrefix upsert chain changed the byte count (14640 → 14487).
- **Fix:** Regenerated with `go test ./internal/sync/... -run TestSync_RefactorParity -update`.
- **Files modified:** internal/sync/testdata/refactor_parity.golden.json.
- **Verification:** `go test -race ./internal/sync/...` exit 0.
- **Committed in:** `dbcade1` (Task 2).

**6. [Rule 1 — Lint] `graph/custom.resolvers.go` gqlgen regen renamed `_` → `ctx` on `ObjectCounts`, triggering revive unused-parameter**
- **Found during:** Task 5 (phase gate, `golangci-lint run`)
- **Issue:** gqlgen 0.17.x regenerates resolver signatures with named ctx parameters even when unused. revive then flags it.
- **Fix:** Renamed back to `_`. The field does not use ctx.
- **Files modified:** graph/custom.resolvers.go.
- **Verification:** `golangci-lint run` exit 0.
- **Committed in:** `ce84323` (Task 5 lint fixup).

**7. [Rule 1 — Golden] `internal/pdbcompat/testdata/golden/ixlan/depth.json` nested ixpfx_set still had `"notes":""`**
- **Found during:** Task 2 (first `go test ./internal/pdbcompat/...` after the -update run on ixpfx/)
- **Issue:** The ixlan depth golden embeds a nested ixpfx object that also carries the removed `notes` key. The initial `-update` run only regenerated the ixpfx/ goldens; the nested ixlan/depth.json was also affected but staged on the next test run.
- **Fix:** Re-ran `go test ./internal/pdbcompat -run TestGoldenFiles -update`. Golden now matches.
- **Files modified:** internal/pdbcompat/testdata/golden/ixlan/depth.json.
- **Verification:** `go test ./internal/pdbcompat -race` exit 0.
- **Committed in:** `dbcade1` (Task 2).

---

**Total deviations:** 7 auto-fixed (1 Rule 3 blocking, 6 Rule 1 bugs). All necessary for correctness; no scope creep beyond Phase 63's stated goal. The most architecturally interesting item is #1 — the plan's `go generate ./...` literal is unsafe given the existing project-level issue that `schema/generate.go` strips the v1.14 Phase 59 `Policy()` method. This is documented in CLAUDE.md for future drops.

## Proto wire-compat note (per plan success_criteria)

Per the plan's success criteria and RESEARCH.md §"Risks" item 4: **proto field renumbering is accepted as a cosmetic wire-compat break** — except in this case, entproto's SkipGenFile option (ent/entc.go) means `proto/peeringdb/v1/v1.proto` is frozen since v1.6 and **does not** actually renumber. The three proto fields remain declared (IxPrefix.notes, Organization.fac_count, Organization.net_count) but are no longer populated by the server. Wire-encoded clients see them as absent fields (zero-value pointer → not serialized). No external proto consumers are known per CONTEXT.md. Recorded as a Key Decision in PROJECT.md.

## Issues Encountered

- **`go generate ./...` strips `ent/schema/poc.go` Policy()** — pre-existing v1.14 latent issue. Worked around by running `go generate ./ent` only and editing `schema/peeringdb.json` by hand. Documented the correct invocation in CLAUDE.md. This issue warrants a separate follow-up task (update the schema generator to preserve non-field content, or move poc.go Policy to a separate non-generated file) but is out of scope for Phase 63.
- **Plan's RESEARCH.md claimed grpcserver/organization.go was clean** — it wasn't (see Deviation #2). The RESEARCH pipeline grep missed `o.NetCount`/`o.FacCount` because the search was narrowed to specific contexts. Build caught it immediately.

## User Setup Required

None — no external service configuration. Post-deploy verification on Fly.io remains as a manual step (already captured in `<verification>` of the plan):

```bash
fly ssh console -a peeringdb-plus --command "sqlite3 /litefs/peeringdb-plus.db '.schema ix_prefixes organizations'"
```

`sqlite3` is already in the prod image per quick task `260418-1cn`.

## Next Phase Readiness

- **Phase 64 (Field-level privacy)** — unblocked. This phase touched `ent/schema/ixprefix.go` and `ent/schema/organization.go`; Phase 64 touches `ent/schema/ixlan.go`. No file overlap, and the codegen cycle is clean.
- **Phase 65 (Asymmetric Fly fleet)** — unaffected.
- **Phase 66 (Observability + sqlite3)** — unaffected; sqlite3 tooling shipped in pre-phase quick task `260418-1cn`.

**Follow-up queued (non-blocking):**
- Fix `cmd/pdb-schema-generate` to preserve hand-added content in `ent/schema/poc.go` (or move `Policy()` out of the generated schema file). Currently latent issue; the v1.14 `Policy()` would be destroyed by any `go generate ./schema` invocation. Documented in CLAUDE.md as guidance but should eventually be hardened.

## Self-Check: PASSED

All files claimed as created/modified in the frontmatter exist on disk and all commit hashes are present in `git log`:

- `2646821` — feat(63-01): drop vestigial schema fields — FOUND
- `dbcade1` — feat(63-01): update hand-written consumers — FOUND
- `4644da1` — feat(63-01): wire migrate.WithDropColumn + WithDropIndex — FOUND
- `73a729a` — docs(63-01): doc sweep — FOUND
- `ce84323` — chore(63-01): restore _ context.Context — FOUND

All key files referenced above exist on disk (spot-checked: ent/schema/ixprefix.go, ent/schema/organization.go, schema/peeringdb.json, internal/pdbcompat/serializer_test.go, internal/grpcserver/filter_test.go, cmd/peeringdb-plus/main.go, CLAUDE.md, .planning/PROJECT.md).

---
*Phase: 63-schema-hygiene*
*Completed: 2026-04-18*
