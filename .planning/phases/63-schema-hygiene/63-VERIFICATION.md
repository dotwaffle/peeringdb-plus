---
phase: 63-schema-hygiene
verified: 2026-04-18T09:20:00Z
status: human_needed
score: 12/12 must-haves verified
overrides_applied: 0
human_verification:
  - test: "Post-deploy schema drop confirmation on LHR primary"
    expected: ".schema ix_prefixes organizations shows no notes/fac_count/net_count columns after next primary startup"
    why_human: "Requires fly ssh console against production LiteFS primary; outside automated verification scope. sqlite3 already shipped in prod image per quick task 260418-1cn."
  - test: "Post-deploy LiteFS replica propagation"
    expected: "Same .schema output on a non-primary replica confirms WAL replication propagated the DROP COLUMN DDL"
    why_human: "Requires fly ssh console with region pin against a replica machine; no automated access."
  - test: "Deferred follow-up: pdb-schema-generate Policy()-stripping bug"
    expected: "Future task fixes cmd/pdb-schema-generate to preserve hand-added ent/schema/poc.go Policy() so `go generate ./...` can be used safely without the ./ent workaround"
    why_human: "Out of scope for Phase 63 per SUMMARY deviation #1. Documented in CLAUDE.md. Needs product/tooling decision on whether to fix the generator or move Policy() to a separate non-generated file."
---

# Phase 63: Schema hygiene — drop vestigial columns — Verification Report

**Phase Goal:** Drop three ent schema fields confirmed vestigial through full-codebase audit (schema + `peeringdb` struct + sync upsert + pdbcompat serializer + upstream `/api/*` response verified on beta). Eliminate dead columns from the DB on next ent auto-migrate.
**Verified:** 2026-04-18T09:20:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

All 12 automated must-haves verified. Three follow-up items require human action (post-deploy schema confirmation on primary + replica, and a deferred tooling fix for `pdb-schema-generate`).

### Observable Truths (from PLAN frontmatter + ROADMAP success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | ent schema no longer declares ixprefix.notes, organization.fac_count, or organization.net_count | VERIFIED | `grep field.String("notes") ent/schema/ixprefix.go` → 0 matches; `grep field.Int("(fac|net)_count") ent/schema/organization.go` → 0 matches |
| 2 | pdbcompat /api/ixpfx response omits the notes key entirely | VERIFIED | `ixPrefixFromEnt` in `internal/pdbcompat/serializer.go:291-302` no longer emits Notes; goldens at `testdata/golden/ixpfx/*.json` have no "notes" key (grep count 0); `TestAnonParityFixtures/ixpfx` passes |
| 3 | pdbcompat /api/org response continues to omit fac_count and net_count (no regression) | VERIFIED | `peeringdb.Organization` has no FacCount/NetCount fields; `TestAnonParityFixtures/org` passes; `TestOrganizationJSON_NoCountKeys` passes |
| 4 | ent auto-migrate on primary startup drops the three DB columns | VERIFIED (runtime wiring) | `cmd/peeringdb-plus/main.go:26` imports `ent/migrate`; lines 126-127 pass `migrate.WithDropColumn(true)` + `migrate.WithDropIndex(true)` to `Schema.Create` inside the `isPrimary` gate. **Actual DB column drop requires primary startup — see human_verification.** |
| 5 | go generate ./... produces zero drift; CI drift check passes | VERIFIED (scoped to ./ent) | `go generate ./ent` + `git status ent/ gen/ graph/ proto/` → clean. SUMMARY-noted caveat: must use `./ent` not `./...` because `pdb-schema-generate` strips poc.go Policy() — accepted, documented in CLAUDE.md |
| 6 | TestAnonParityFixtures passes with an empty knownDivergences map | VERIFIED | `anon_parity_test.go:59` `var knownDivergences = map[string]struct{}{}` (empty); `TestAnonParityFixtures` PASS across all 13 subtests |
| 7 | go test -race and golangci-lint both clean | VERIFIED | `go test -race ./internal/pdbcompat/... ./internal/grpcserver/... ./internal/sync/... ./ent/...` → exit 0; `golangci-lint run` → 0 issues; `go vet ./...` → clean |

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `ent/schema/ixprefix.go` | No `field.String("notes")` | VERIFIED | grep returns 0 matches |
| `ent/schema/organization.go` | No `field.Int("fac_count")` or `field.Int("net_count")` | VERIFIED | grep returns 0 matches |
| `internal/peeringdb/types.go` | `IxPrefix` struct without `Notes` | VERIFIED | Struct at line 229-238 has only ID/IXLanID/Protocol/Prefix/InDFZ/Created/Updated/Status. Notes retained correctly on Organization (line 49), Network (90), Facility (132), InternetExchange (165), etc. per D-01/D-02 |
| `internal/peeringdb/types.go` | `Organization` struct without `FacCount`/`NetCount` | VERIFIED | Struct at line 42-64 has no count fields (grep confirms FacCount/NetCount only appear on Network, InternetExchange, Carrier — all real upstream fields) |
| `internal/pdbcompat/serializer.go` | `ixPrefixFromEnt` without Notes emission | VERIFIED | Function at line 291-302 does not assign Notes; comment at line 287-290 documents the D-01 reason |
| `cmd/peeringdb-plus/main.go` | Auto-migrate with WithDropColumn + WithDropIndex | VERIFIED | Line 26 import, lines 126-127 options passed to Schema.Create |
| `internal/pdbcompat/anon_parity_test.go` | `knownDivergences` without `ixpfx|data[0].notes|extra_field` | VERIFIED | Map is empty at line 59; historical comment at lines 56-58 references the Phase 63 removal |
| `internal/pdbcompat/testdata/golden/ixpfx/*.json` | No `"notes"` keys | VERIFIED | grep across all 3 golden files → 0 matches |
| `internal/pdbcompat/testdata/golden/ixlan/depth.json` | Nested ixpfx_set has no `"notes"` keys | VERIFIED | Deviation #7 in SUMMARY caught this; regenerated. grep → 0 matches |
| `internal/pdbcompat/serializer_test.go` | Defensive tests exist | VERIFIED | `TestIxPrefixFromEnt_NoNotesKey` (line 291) + `TestOrganizationJSON_NoCountKeys` (line 313) present and PASS |
| `internal/grpcserver/organization.go` | No NetCount/FacCount on Organization proto converter | VERIFIED | Lines 206-209 carry an inline comment citing D-02 + frozen-proto rationale; no field assignments |
| `internal/grpcserver/filter_test.go` | `deprecatedFilterFields` exemption | VERIFIED | Declared at line 472; applied at line 598 via `entityName+"/"+name` key |

### Key Link Verification

| From | To | Via | Status | Details |
|------|-----|-----|--------|---------|
| `cmd/peeringdb-plus/main.go` Schema.Create call | `ent/migrate` package | variadic MigrateOption args | WIRED | Import at line 26, options applied at lines 126-127 inside `if isPrimary` gate |
| `internal/pdbcompat/serializer.go:ixPrefixFromEnt` | `internal/peeringdb/types.go:IxPrefix` | struct literal without Notes | WIRED | Struct literal at serializer.go:292-301 matches the IxPrefix type shape (no Notes) |
| `TestAnonParityFixtures` | `testdata/visibility-baseline/beta/anon/api/ixpfx` | fixture replay with zero divergences | WIRED | Test passes with empty `knownDivergences` map; no ixpfx divergence |
| `internal/sync/upsert.go` IxPrefix chain | `ent/ixprefix` builder | Create builder without SetNotes | WIRED | No `ip.Notes` or `SetNotes(ip.Notes)` remain in `internal/sync/upsert.go`; verified by grep |
| `internal/grpcserver/ixprefix.go` | proto IxPrefix (frozen) | converter omits Notes; filter excluded via `deprecatedFilterFields["ixprefix/notes"]` | WIRED | No `ixp.Notes` references; test invariant respects frozen-proto wire |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `ixPrefixFromEnt` | `peeringdb.IxPrefix` literal | `*ent.IxPrefix` query result (from ent client populated by sync upsert) | Yes — sync upsert populates IxlanID/Protocol/Prefix/InDfz/Created/Updated/Status for every row | FLOWING |
| `organizationFromEnt` | `peeringdb.Organization` literal | `*ent.Organization` query result | Yes — existing Organization field set unchanged (Notes on Organization is a real upstream field; FacCount/NetCount already absent pre-Phase-63) | FLOWING |
| `Schema.Create(ctx, WithDropColumn(true), WithDropIndex(true))` | DDL statements | ent/migrate runtime against live SQLite | Pending runtime — will emit `ALTER TABLE DROP COLUMN` on next primary startup | STATIC until post-deploy (see human_verification) |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Full tree builds | `go build ./...` | exit 0 | PASS |
| Focused test suite passes with race | `go test -race ./internal/pdbcompat/... ./internal/grpcserver/... ./internal/sync/... ./ent/...` | all packages `ok`; exit 0 | PASS |
| Defensive regression tests pass | `go test -race ./internal/pdbcompat -run 'TestIxPrefixFromEnt_NoNotesKey\|TestOrganizationJSON_NoCountKeys' -v` | 2/2 PASS | PASS |
| Parity test passes across all 13 entities | `go test -race ./internal/pdbcompat -run TestAnonParityFixtures -v` | 14 subtests PASS | PASS |
| Lint clean | `golangci-lint run` | `0 issues` | PASS |
| `go vet` clean | `go vet ./...` | no output; exit 0 | PASS |
| Codegen drift (scoped) | `go generate ./ent && git status ent/ gen/ graph/ proto/` | no modifications | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| HYGIENE-01 | 63-01 | Drop `ixprefix.notes` ent schema field | SATISFIED | Schema dropped; struct/serializer/upsert/registry/filter/goldens all updated; TestAnonParityFixtures/ixpfx passes with empty knownDivergences; TestIxPrefixFromEnt_NoNotesKey passes |
| HYGIENE-02 | 63-01 | Drop `organization.fac_count` ent schema field | SATISFIED | Schema dropped; ent/organization/* no longer exports FieldFacCount; grpcserver/organization.go converter no longer emits; TestOrganizationJSON_NoCountKeys passes |
| HYGIENE-03 | 63-01 | Drop `organization.net_count` ent schema field | SATISFIED | Schema dropped; ent/organization/* no longer exports FieldNetCount; grpcserver/organization.go converter no longer emits; TestOrganizationJSON_NoCountKeys passes |

No orphaned requirements — all 3 HYGIENE IDs declared in Phase 63's only plan frontmatter match the REQUIREMENTS.md mapping (Phase 63: HYGIENE-01/02/03).

### Proto Frozen-Field Status (Accepted Wire-Compat)

| Proto field | Location | Numbering | Server populates? | Status |
|-------------|----------|-----------|-------------------|--------|
| `IxPrefix.notes = 4` | proto/peeringdb/v1/v1.proto:293 | Preserved | No | ACCEPTED — per SUMMARY wire-compat note |
| `Organization.net_count = 18` | proto/peeringdb/v1/v1.proto:483 | Preserved | No | ACCEPTED — per SUMMARY wire-compat note |
| `Organization.fac_count = 19` | proto/peeringdb/v1/v1.proto:485 | Preserved | No | ACCEPTED — per SUMMARY wire-compat note |

Proto is frozen since v1.6 (entproto.SkipGenFile). Original plan accepted either renumbering or preservation; actual result preserves numbering (better for wire compat). Zero-value pointers serialize as absent on the wire. `deprecatedFilterFields` exemption in `filter_test.go` keeps the reflection-based filter invariant honest. Decision row added to `.planning/PROJECT.md` (line 219) for transparency per D-05.

### Anti-Patterns Found

None. No TODO/FIXME/stub comments introduced. No hardcoded empty data. No disconnected wiring.

Notable patterns that are intentional, not stubs:
- `grpcserver/organization.go:206-209` inline comment explaining the deliberate drop of NetCount/FacCount emission — documentation of decision, not a stub.
- `anon_parity_test.go:56-58` comment documenting why `knownDivergences` is now empty — historical context preserved.

### Documentation Coverage (D-05)

| Surface | Expected | Status | Evidence |
|---------|----------|--------|----------|
| `.planning/PROJECT.md` | New Key Decisions row for Phase 63 | VERIFIED | Row at line 219 documents drops + WithDropColumn/WithDropIndex wiring + frozen-proto acceptance |
| `CLAUDE.md` | Schema hygiene + WithDropColumn guidance | VERIFIED | New subsection at lines 77-84 covers all three drops, the `go generate ./ent` workaround, and `deprecatedFilterFields` pattern |
| `docs/ARCHITECTURE.md` | Grep-clean for dropped fields as active schema members | VERIFIED | grep `notes\|fac_count\|net_count\|ixpfx\.notes` → 0 matches; no active schema enumeration references |
| `docs/API.md` | Grep-clean for dropped fields as response keys | VERIFIED | grep `notes\|fac_count\|net_count\|ixpfx\.notes` → 0 matches |

### Human Verification Required

#### 1. Post-deploy schema drop confirmation on LHR primary

**Test:**
```bash
fly ssh console -a peeringdb-plus --command \
  "sqlite3 /litefs/peeringdb-plus.db '.schema ix_prefixes organizations'"
```
**Expected:** Output shows `ix_prefixes` table WITHOUT a `notes` column, and `organizations` table WITHOUT `fac_count` or `net_count` columns.
**Why human:** Requires `fly ssh` against production; no automated access.

#### 2. Post-deploy LiteFS replica propagation

**Test:** Same `sqlite3 .schema` command against a non-primary replica machine (specify `--region` or `--select`).
**Expected:** Same output as primary — WAL replication propagated the DROP COLUMN DDL.
**Why human:** Requires `fly ssh` with region pinning; no automated access.

#### 3. Deferred tooling follow-up — pdb-schema-generate Policy() stripping

**Test:** Review the follow-up captured in SUMMARY.md Deviation #1 and at the bottom of the "Issues Encountered" section: `cmd/pdb-schema-generate` currently strips the hand-added `Policy()` on `ent/schema/poc.go` when `go generate ./schema` runs. The v1.15 Phase 63 workaround documented in CLAUDE.md is to use `go generate ./ent` (not `./...`).
**Expected:** Decide whether to (a) harden `cmd/pdb-schema-generate` to preserve hand-added content or (b) move the `Policy()` out of the generated schema file. Schedule as a future quick task or dedicated phase.
**Why human:** Architectural decision; out of scope for Phase 63 (acknowledged in SUMMARY).

## Gaps Summary

No gaps blocking Phase 63's stated goal. All required code, test, documentation, and runtime-wiring deliverables are present and verified against the codebase. The three items above are **follow-up human actions** — not implementation gaps — and the first two are the unavoidable manual step for validating the "DROP COLUMN emits on next primary startup" runtime behavior the phase promised (by design, this cannot be verified pre-deploy without running the binary against a live DB).

The plan's written-out success criteria are all met:

- `ent/schema/ixprefix.go` and `ent/schema/organization.go` no longer declare the three dropped fields. ✓
- `go generate ./ent` produces zero drift (accepted `./ent` scope per SUMMARY Deviation #1). ✓
- `internal/peeringdb/types.go` IxPrefix has no Notes; Organization has no FacCount/NetCount. ✓
- `internal/pdbcompat/serializer.go:ixPrefixFromEnt` no longer emits Notes. ✓
- `internal/pdbcompat/anon_parity_test.go` knownDivergences is empty. ✓
- `cmd/peeringdb-plus/main.go` Schema.Create passes WithDropColumn(true) + WithDropIndex(true). ✓
- Golden fixtures contain no `notes` keys. ✓
- Defensive micro-tests exist and pass. ✓
- PROJECT.md + CLAUDE.md updated; ARCHITECTURE.md + API.md grep-clean. ✓
- Phase gate (tests, lint, vet, codegen drift) all pass. ✓

Status is `human_needed` solely because of the post-deploy DB verification steps — the in-tree work is complete.

---

*Verified: 2026-04-18T09:20:00Z*
*Verifier: Claude (gsd-verifier)*
