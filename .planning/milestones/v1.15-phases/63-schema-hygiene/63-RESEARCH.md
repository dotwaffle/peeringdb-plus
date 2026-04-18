# Phase 63: Schema hygiene — drop vestigial columns - Research

**Researched:** 2026-04-18
**Domain:** entgo schema migration / SQLite DDL / codegen pipeline hygiene
**Confidence:** HIGH

## Findings Summary

**Primary recommendation:** Edit three schema files, regenerate, wire two flags at the single `Schema.Create()` call site, ship. ~1-task plan.

- **ent drop-column flags are required.** Ent auto-migrate is additive-only by default. To emit `ALTER TABLE DROP COLUMN` you MUST pass `migrate.WithDropColumn(true)`. `migrate.WithDropIndex(true)` is also recommended (idiomatic companion) even though none of the three target fields are indexed. `[VERIFIED: ent/migrate/migrate.go` lines 20-30 in this repo re-exports both from `entgo.io/ent/dialect/sql/schema`; CITED: entgo.io/docs/migrate`]
- **Wire location is the runtime call site, not codegen.** `cmd/peeringdb-plus/main.go:114` — the single `entClient.Schema.Create(ctx)` call. Options are variadic `schema.MigrateOption` args on `Schema.Create(ctx, opts...)`. Do NOT touch `ent/entc.go`. `[VERIFIED: grep + ent/migrate/migrate.go:44 signature]`
- **SQLite DROP COLUMN works** — modernc.org/sqlite v1.48.2 bundles a modern SQLite (≥ 3.51.x range; DROP COLUMN requires ≥ 3.35 released 2021). None of the three target columns have indexes or constraints that would block DROP COLUMN. `[VERIFIED: go.mod + schema index audit; CITED: sqlite.org/lang_altertable.html]`
- **Only one runtime call site exists.** `Schema.Create(ctx)` is called exactly once, gated on `isPrimary`. This means the flag change applies on the LHR primary's next startup; replicas receive the DDL via LiteFS replication. `[VERIFIED: grep Schema.Create]`
- **Codegen regenerates everything.** `go generate ./...` drops the removed fields from `ent/*`, `gen/peeringdb/v1/*.pb.go`, and `graph/schema.graphqls` automatically. Proto field numbers will renumber (cosmetic wire-compat impact only — this is a read-only mirror with no external proto consumers). `[VERIFIED: observed entproto auto-assignment in proto/peeringdb/v1/v1.proto]`
- **Three additional code callers** beyond the files enumerated in CONTEXT.md: `internal/pdbcompat/registry.go:269` (ixpfx `notes` entry), `internal/grpcserver/ixprefix.go` (ixpfx notes filter + serializer), `internal/pdbcompat/testdata/golden/ixpfx/*.json` (golden fixtures containing `"notes":""`). Organization FacCount/NetCount are NOT referenced in the pdbcompat registry, grpcserver, or serializer — pure schema-only drop.

## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01: Full removal for `ixprefix.notes`.** Drop the ent schema field, drop `Notes string json:"notes"` from `peeringdb.IxPrefix`, remove emission from `pdbcompat.ixPrefixFromEnt`. Our API response matches upstream's shape exactly (no `notes` key).
- **D-02: Full removal for `organization.fac_count` and `organization.net_count`.** Schema-only vestigials — not in `peeringdb.Organization`, not in sync upsert, not in `organizationFromEnt` serializer. Schema edit + regen is the full fix.
- **D-03:** Remove the `ixpfx.notes` entry from `internal/pdbcompat/anon_parity_test.go` `knownDivergences` map once the field is dropped. Test enforces zero structural divergences on ixpfx after the drop.
- **D-04:** Rely on ent auto-migrate. Read entgo migrate docs first to verify whether `schema.WithDropColumn(true)` flag is needed (ent defaults to additive). If needed, wire it into `ent/entc.go` or the migration call site. Rolling deploy handles the column drop; SQLite 3.35+ supports `DROP COLUMN` natively.
- **D-05: Full doc sweep.** Touch `.planning/PROJECT.md` Key Decisions, `docs/ARCHITECTURE.md`, `CLAUDE.md`, `docs/API.md`.
- **D-06:** Phase 63 runs first, before Phase 64. Phase 64 also touches `ent/schema/ixlan.go`; splitting them avoids interleaved codegen cycles. Both phases end with `go generate ./...` clean.

### Claude's Discretion
- Test commit structure (single `test(63-01)` refactor vs split per field) — implementation detail
- Whether to include a verification test that explicitly asserts the 3 fields are absent from JSON output post-drop — defensive, ~10 LoC, worth it

### Deferred Ideas (OUT OF SCOPE)
- **Broader schema audit phase.** Other divergences beyond these 3 are not known to exist; a periodic audit job is deferred to v1.16+.
- **Automated schema drift CI check.** Deferred.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| HYGIENE-01 | Drop `ixprefix.notes` ent schema field | Schema field drop + peeringdb struct edit + serializer edit + registry edit + grpcserver edit + golden fixture regen + parity test divergence removal |
| HYGIENE-02 | Drop `organization.fac_count` ent schema field | Schema field drop only (no other callers) |
| HYGIENE-03 | Drop `organization.net_count` ent schema field | Schema field drop only (no other callers) |

## Project Constraints (from CLAUDE.md)

- **Go 1.26+, entgo is non-negotiable** — codegen-first workflow, no hand-rolled migrations.
- **`go generate ./...` runs the full pipeline in order** — `ent/generate.go` → `internal/web/templates/generate.go` → `schema/generate.go`. Do NOT run `go generate ./schema` after entproto annotations are added (strips them).
- **Always commit generated files alongside schema changes** — `ent/`, `gen/`, `graph/`, `internal/web/templates/` must all land in the same commit.
- **CI drift check** — runs `go generate ./...` on every PR and fails if committed generated files differ. Phase 63 MUST regenerate cleanly or CI blocks.
- **`CGO_ENABLED=0` everywhere** — modernc.org/sqlite is pure Go; no CGo needed for the DROP COLUMN emission.
- **Auto-migrate only on LiteFS primary** per D-43 (v1.0). `isPrimary` gate is already in place at the call site.
- **ASDL of commits** — commit generated files together with the schema edit that produced them.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Ent schema definition | Codegen source | — | `ent/schema/*.go` is the authoritative schema — everything else is generated from it |
| Runtime DDL emission | Database / Storage | — | `Schema.Create(ctx, opts...)` translates schema → SQLite DDL on primary startup |
| PeeringDB wire types | Sync/Client (internal/peeringdb) | — | Mirrors upstream shape; struct fields track what upstream emits |
| pdbcompat API response shape | API / Backend | — | Drop-in PeeringDB API emulator; must match upstream shape per D-01 |
| GraphQL/REST/Proto surfaces | Codegen output | — | `gen/`, `graph/`, and ent-generated REST are downstream of schema; `go generate ./...` reconciles automatically |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| entgo.io/ent | (see go.mod) | Schema-driven ORM + migration | Already in use; non-negotiable per CLAUDE.md |
| entgo.io/ent/dialect/sql/schema | (transitive) | Migrate options — `WithDropColumn`, `WithDropIndex` | Re-exported by generated `ent/migrate` package |
| modernc.org/sqlite | v1.48.2 | Pure-Go SQLite driver | Bundles SQLite ≥ 3.35 (DROP COLUMN supported since 3.35, Mar 2021) |

**No new dependencies.** The two migrate options are already available via the generated `ent/migrate` package; the import already transitively resolves.

**Version verification:**
- `modernc.org/sqlite v1.48.2` → bundles recent SQLite; confirmed pure-Go [VERIFIED: go.mod] [CITED: pkg.go.dev/modernc.org/sqlite]
- Generated `ent/migrate/migrate.go` locally re-exports `WithDropColumn` and `WithDropIndex` [VERIFIED: file contents lines 20-30 in this repo]

## ent auto-migrate drop-column behavior

**The concrete answer to D-04:**

Ent defaults to additive-only migrations for safety. To actually emit `ALTER TABLE DROP COLUMN` you must pass `migrate.WithDropColumn(true)` to `Schema.Create`. The `migrate.WithDropIndex(true)` companion is recommended by the ent docs for symmetric handling of stale indexes.

**Exact wiring location:** `cmd/peeringdb-plus/main.go`, line 114 — the ONLY `Schema.Create(ctx)` call in the codebase (verified via grep). Edit:

```go
import (
    // ... existing imports
    "github.com/dotwaffle/peeringdb-plus/ent/migrate"
)

// Auto-migrate schema on primary per D-43.
if isPrimary {
    if err := entClient.Schema.Create(
        ctx,
        migrate.WithDropColumn(true),
        migrate.WithDropIndex(true),
    ); err != nil {
        logger.Error("failed to migrate schema", slog.Any("error", err))
        os.Exit(1)
    }
}
```

**Why the local `ent/migrate` package, not `entgo.io/ent/dialect/sql/schema`:** the generated `ent/migrate/migrate.go` file (lines 20-30) already re-exports both options as package-level vars:

```go
// Generated by ent — already in this repo:
WithDropColumn = schema.WithDropColumn
WithDropIndex  = schema.WithDropIndex
```

Using the local alias is the idiomatic ent pattern [CITED: entgo.io/docs/migrate/] and avoids adding a new external import.

**Do NOT touch `ent/entc.go`.** Codegen options go into `entc.Options{}`; migration-time options go into `Schema.Create(ctx, opts...)`. These are separate APIs. The drop-column flags are runtime, not codegen.

**Index handling for this phase:** None of the three target columns have indexes — confirmed by auditing `ent/schema/ixprefix.go` (indexes on `ixlan_id`, `prefix`, `status`) and `ent/schema/organization.go` (indexes on `name`, `status`). `WithDropIndex(true)` is still included for hygiene and future-proofing (the ent docs explicitly recommend enabling both together because unique constraints are implemented as `UNIQUE INDEX`).

**Gate is already correct.** The existing `if isPrimary { ... }` gate satisfies CLAUDE.md §LiteFS: DDL runs only on the primary; replicas receive it via LiteFS WAL replication.

## Codegen pipeline impact

**What `go generate ./...` cleans up automatically after removing the three schema fields:**

| Generated surface | File(s) | Behavior |
|-------------------|---------|----------|
| ent Go types (Create/Update/Query builders) | `ent/*.go`, `ent/organization/*.go`, `ent/ixprefix/*.go` | Setter methods (`SetNotes`, `SetFacCount`, `SetNetCount`), predicates, where-clauses, and field constants all disappear |
| ent/migrate/schema.go | `ent/migrate/schema.go` | Column definitions removed from `Columns` slices (lines 20, 73, 190 for the three targets) |
| Proto types | `gen/peeringdb/v1/v1.pb.go`, `proto/peeringdb/v1/v1.proto` | Fields removed; entproto renumbers remaining fields (cosmetic wire-compat break — acceptable for a read-only mirror with no external proto consumers) |
| GraphQL schema | `graph/schema.graphqls`, `graph/schema.graphql`, `graph/generated.go` | Field definitions + where-input predicates + order-by enum entries all drop out |
| REST surface | entrest-generated handlers (embedded in `ent/`) | REST filter predicates on the removed fields disappear |

**What does NOT regenerate automatically and MUST be edited by hand:**

1. `internal/peeringdb/types.go` — `IxPrefix.Notes` field removal (hand-written wire type)
2. `internal/pdbcompat/serializer.go` — remove `Notes: p.Notes` in `ixPrefixFromEnt` (line 297)
3. `internal/pdbcompat/registry.go` — remove `"notes": FieldString` from `TypeIXPfx` block (line 269). Note: `TypeOrg` block does NOT contain fac_count/net_count — no edit needed there.
4. `internal/pdbcompat/anon_parity_test.go` — remove the `"ixpfx|data[0].notes|extra_field"` entry from `knownDivergences` map (line 69)
5. `internal/grpcserver/ixprefix.go` — three hand-written references:
   - Line 41: `eqFilter(func(r *pb.ListIxPrefixesRequest) *string { return r.Notes }, ...)`
   - Line 59: `eqFilter(func(r *pb.StreamIxPrefixesRequest) *string { return r.Notes }, ...)`
   - Line 150: `Notes: stringVal(ixp.Notes),` in the proto conversion

   These compile against generated proto code; once entproto drops the proto field, `r.Notes` and `ixp.Notes` become undefined — removing these lines is mandatory to regain a clean build.
6. `internal/pdbcompat/testdata/golden/ixpfx/{list,detail,depth}.json` — contain literal `"notes":""` entries. Will need regeneration or a one-line edit; the golden test will flag mismatch on first run. Likely the cleanest path is to regenerate goldens once the serializer edit lands (standard `go test -run TestGolden -update` pattern if one exists; otherwise hand-edit — they're three ~200-byte files).

**Other hand-written files confirmed clean** (verified via grep):
- `internal/sync/upsert.go` — no `SetNotes` on IxPrefix path (line 353 is for a different field that WAS upserted; actually wait — line 353 DOES show `SetNotes(ip.Notes)` for IxPrefix). **Planner: edit `internal/sync/upsert.go:353` too** (it's the sync-layer write of `ixpfx.notes` from upstream to ent; must go once the ent setter is gone).
- `internal/sync/upsert.go` — no `SetFacCount`/`SetNetCount` calls against the Organization builder (verified: line 84 only has `SetNotes(o.Notes)`, no count setters).

**Net new files touched beyond the seven enumerated in CONTEXT.md:**
- `internal/pdbcompat/registry.go` — 1 line (remove `"notes"` from TypeIXPfx block)
- `internal/grpcserver/ixprefix.go` — 3 hunks (filter × 2 + proto conversion × 1)
- `internal/sync/upsert.go` — 1 line (remove `SetNotes(ip.Notes)` from the ixpfx upsert)
- `internal/pdbcompat/testdata/golden/ixpfx/*.json` — 3 fixture files (regenerate or hand-edit)
- `cmd/peeringdb-plus/main.go` — 1 hunk (add `migrate.WithDropColumn(true), migrate.WithDropIndex(true)` + import)

## SQLite + LiteFS drop-column compatibility

**SQLite version:** modernc.org/sqlite v1.48.2 → bundles SQLite well beyond 3.35 (DROP COLUMN added in March 2021). Confirmed in modernc docs that recent releases track upstream closely. [CITED: pkg.go.dev/modernc.org/sqlite]

**SQLite DROP COLUMN restrictions** (from sqlite.org/lang_altertable.html):
- Cannot drop PRIMARY KEY columns
- Cannot drop UNIQUE-constrained columns
- **Cannot drop indexed columns** (must drop index first)
- Cannot drop columns referenced by foreign keys, CHECK constraints, triggers, views, generated columns

**Audit of the three target columns against these restrictions:**

| Column | Table | Indexed? | Constraint? | Clear to drop? |
|--------|-------|----------|-------------|----------------|
| `notes` | `ix_prefixes` | No (indexes: ixlan_id, prefix, status) | Default `""`, nullable | ✓ yes |
| `fac_count` | `organizations` | No (indexes: name, status) | Default `0`, nullable | ✓ yes |
| `net_count` | `organizations` | No (indexes: name, status) | Default `0`, nullable | ✓ yes |

**LiteFS compatibility:** LiteFS replicates the SQLite WAL byte-for-byte. `ALTER TABLE DROP COLUMN` produces a standard set of WAL frames (SQLite internally implements it as a rewrite-the-table operation when needed, but produces conventional WAL pages). Replicas receive the schema change via normal replication; no special LiteFS handling required. [CITED: LiteFS docs are maintenance-mode but the WAL-replication contract hasn't changed.]

**One caveat:** SQLite's DROP COLUMN may internally trigger a full-table rewrite on larger tables. Observed DB size is ~88 MB (from STATE.md fleet baseline). The `ix_prefixes` and `organizations` tables are small relative to that total (PeeringDB has ~45K orgs, ~80K ixpfx rows). Expected DDL latency: sub-second on primary startup. Drain-timeout (`PDBPLUS_DRAIN_TIMEOUT=10s`) and startup sequence absorb this comfortably.

## Runtime migration site

**Single call site:** `cmd/peeringdb-plus/main.go:114`

Current code:

```go
// Auto-migrate schema on primary per D-43.
if isPrimary {
    if err := entClient.Schema.Create(ctx); err != nil {
        logger.Error("failed to migrate schema", slog.Any("error", err))
        os.Exit(1)
    }
}
```

**Required edit:**

```go
import (
    // ... existing
    "github.com/dotwaffle/peeringdb-plus/ent/migrate"
)

// Auto-migrate schema on primary per D-43.
// WithDropColumn: enables ALTER TABLE DROP COLUMN for v1.15 Phase 63 schema
// cleanup (ixpfx.notes, organization.fac_count, organization.net_count).
// WithDropIndex: symmetric handling of stale indexes per ent docs recommendation.
if isPrimary {
    if err := entClient.Schema.Create(
        ctx,
        migrate.WithDropColumn(true),
        migrate.WithDropIndex(true),
    ); err != nil {
        logger.Error("failed to migrate schema", slog.Any("error", err))
        os.Exit(1)
    }
}
```

**Deployment semantics on rolling Fly deploy:**
1. New primary image boots (LHR) with the flag set.
2. `isPrimary` is true → `Schema.Create(ctx, WithDropColumn(true), WithDropIndex(true))` runs.
3. SQLite executes `ALTER TABLE ix_prefixes DROP COLUMN notes` and two `ALTER TABLE organizations DROP COLUMN ...` statements.
4. LiteFS replicates the WAL to replicas.
5. Replicas do NOT run `Schema.Create` (gated by `isPrimary=false`); they pick up the new schema via replication.

**Persistence:** the flag stays on permanently. This is the v1.0 pattern (auto-migrate-on-primary) plus the drop-column capability bolted on. Future schema drops land the same way.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (Go 1.26+) |
| Config file | none (Go convention) |
| Quick run command | `go test ./internal/pdbcompat/... -run TestAnonParityFixtures -race` |
| Full suite command | `go test -race ./...` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| HYGIENE-01 | `ixpfx.notes` absent from pdbcompat `/api/ixpfx` anon response | integration | `go test ./internal/pdbcompat/... -run TestAnonParityFixtures/ixpfx -race` | ✓ (`internal/pdbcompat/anon_parity_test.go` — existing) |
| HYGIENE-01 | ent code compiles without `SetNotes`/`NotesEQ`/`ixprefix.FieldNotes` | smoke (build) | `go build ./...` | ✓ (stdlib) |
| HYGIENE-01 | proto types compile without `IxPrefix.Notes` getter | smoke (build) | `go build ./...` | ✓ |
| HYGIENE-01 | generated code drift check | CI | `go generate ./... && git diff --exit-code` | ✓ (existing CI job) |
| HYGIENE-02 | `organizations.fac_count` column absent from live DB after next startup | manual (post-deploy) | `sqlite3 /litefs/peeringdb-plus.db ".schema organizations"` on primary | n/a — manual verification via fly ssh console (sqlite3 now in prod image post quick task 260418-1cn) |
| HYGIENE-02 | ent code compiles without `Organization.FacCount` field | smoke (build) | `go build ./...` | ✓ |
| HYGIENE-03 | `organizations.net_count` column absent from live DB after next startup | manual (post-deploy) | `sqlite3 /litefs/peeringdb-plus.db ".schema organizations"` on primary | n/a — manual verification via fly ssh console |
| HYGIENE-03 | ent code compiles without `Organization.NetCount` field | smoke (build) | `go build ./...` | ✓ |

**Optional defensive assertion (Claude's discretion per CONTEXT.md):** add a ~10 LoC unit test to `internal/pdbcompat/serializer_test.go` that constructs an `ent.IxPrefix`, runs `ixPrefixFromEnt`, marshals to JSON, and asserts `"notes"` is absent from the key set. Similar micro-test for `ent.Organization` asserting absence of `"fac_count"` and `"net_count"` on the peeringdb wire shape. This makes the regression bulletproof without waiting for fixture-level parity. **Recommended.**

### Sampling Rate
- **Per task commit:** `go test ./internal/pdbcompat/... -race` (fast, targeted)
- **Per wave merge:** `go test -race ./...` (full suite including build check)
- **Phase gate:** `go generate ./... && go test -race ./... && golangci-lint run` — must be clean before `/gsd-verify-work`

### Wave 0 Gaps
None. The regression gate (`TestAnonParityFixtures`) already exists and already carries the `ixpfx.notes` divergence entry it needs to drop. Golden fixtures under `internal/pdbcompat/testdata/golden/ixpfx/*.json` are a small cleanup item but not a test-infrastructure gap.

## Risks and Rollback

**Risks (ordered by likelihood):**

1. **Generated-code drift in one of the many files** — highest likelihood, zero severity. Detected by CI drift check. Fix: re-run `go generate ./...` locally, commit the delta. Well-understood failure mode; happened many times in v1.0-v1.14.

2. **Forgotten hand-written reference** — e.g. `internal/grpcserver/ixprefix.go:150` still references `ixp.Notes` after ent field removal. Compile failure is immediate and local. Fix: remove the reference. Research has enumerated all 11 non-generated references; planner should cross-check.

3. **Golden fixture mismatch** — `internal/pdbcompat/testdata/golden/ixpfx/*.json` will fail golden comparison once the serializer stops emitting `"notes"`. Either regenerate (preferred if the test supports `-update`) or hand-edit three small JSON files.

4. **entproto field renumbering** — removing `ixpfx.notes (field 4)`, `org.net_count (18)`, `org.fac_count (19)` from the proto means remaining fields ≥ the removed positions renumber. **Wire-compat break for any external proto consumers.** This is a read-only mirror with no persistent proto streams or long-lived external proto clients known; the risk is cosmetic. Call it out in the SUMMARY and PROJECT.md Key Decisions for transparency.

5. **SQLite DDL failure on primary startup** — essentially impossible given the audit (no indexes, no FKs, no constraints on the target columns). If it somehow fails, `os.Exit(1)` fires and the primary fails to serve — Fly's health check fails the deploy, rolling back the image automatically. LiteFS primary election is unchanged.

**Rollback plan:**
- **Pre-deploy rollback (code in repo, not yet shipped):** `git revert <phase commit>`; CI green; push.
- **Post-deploy rollback (primary has already dropped the columns):** `ALTER TABLE DROP COLUMN` is destructive — the column's data is gone. A rollback would require either (a) restoring from LiteFS snapshot / backup, or (b) re-adding the columns as empty (schema-additive, which ent auto-migrate handles natively — just revert the schema edits, deploy, columns come back empty with their defaults). Since the dropped data was always-empty (upstream never populated `ixpfx.notes`; `org.fac_count`/`org.net_count` were never written by sync), **option (b) is a clean rollback** — the DB reaches the same end-state as before the drop.
- **Partial rollback** (one field good, one bad) — standard git revert + targeted re-drop on next milestone.

**Pre-deploy checklist for the planner:**
- [ ] `go generate ./...` clean
- [ ] `go build ./...` clean
- [ ] `go test -race ./...` passes (parity test, serializer test, compile tests)
- [ ] `golangci-lint run ./...` clean
- [ ] Committed: ent/, gen/, graph/, internal/pdbcompat/, internal/grpcserver/, internal/sync/, internal/peeringdb/, cmd/peeringdb-plus/main.go, plus doc sweep (PROJECT.md, ARCHITECTURE.md, CLAUDE.md, API.md)
- [ ] SUMMARY.md captures proto renumbering as accepted wire-compat break
- [ ] Post-deploy manual check queued: `fly ssh console -a peeringdb-plus --command "sqlite3 /litefs/peeringdb-plus.db '.schema ix_prefixes organizations'"` confirms columns are gone

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build + codegen | ✓ | 1.26+ (per CLAUDE.md) | — |
| `go tool buf` | proto regen (ent/generate.go chain) | ✓ | Go tool dep | — |
| `go tool templ` | templ regen (unaffected by this phase, but runs in `go generate ./...`) | ✓ | Go tool dep | — |
| golangci-lint | lint gate | ✓ | — | — |
| `sqlite3` on prod image | post-deploy schema verification | ✓ | Recent (Chainguard static) | Shipped 2026-04-18 via quick task 260418-1cn (pre-Phase-65 prep; phase 63 piggybacks) |

**No blocking gaps.** Quick task `260418-1cn` (sqlite3 in prod image) already shipped, so the post-deploy manual verification (`fly ssh console … .schema`) is now trivially runnable on LHR primary and any replica.

## Common Pitfalls

### Pitfall 1: Forgetting to add the `migrate` import
**What goes wrong:** `cmd/peeringdb-plus/main.go` fails to compile with "undefined: migrate".
**Why it happens:** The `ent/migrate` package was previously transitively pulled via the `ent` package but never directly imported.
**How to avoid:** add `"github.com/dotwaffle/peeringdb-plus/ent/migrate"` to the import block of `cmd/peeringdb-plus/main.go`.
**Warning signs:** `go build ./cmd/peeringdb-plus/...` compile error on the `migrate.WithDropColumn` token.

### Pitfall 2: Running `go generate ./schema` instead of `go generate ./...`
**What goes wrong:** entproto annotations get stripped from the schema files, destroying wire format for ALL entities, not just the three targets.
**Why it happens:** CLAUDE.md explicitly warns; new contributors may not know.
**How to avoid:** Always run `go generate ./...` from repo root. Verify no `.proto` field numbers have changed for UNRELATED entities via `git diff proto/`.

### Pitfall 3: Editing the schema but not removing the hand-written serializer reference
**What goes wrong:** `internal/pdbcompat/serializer.go:297` references `p.Notes` after ent drops the field → compile error.
**Why it happens:** ent field removal is silent to hand-written code until you try to build.
**How to avoid:** Use the enumerated "files to edit" list in this research document as a checklist. `grep -n "ixp\.Notes\|p\.Notes" internal/` finds any strays.

### Pitfall 4: Skipping the golden fixture update
**What goes wrong:** `internal/pdbcompat/testdata/golden/ixpfx/*.json` contains `"notes":""` literal. After the serializer drops the field, the golden tests flag mismatch.
**Why it happens:** Goldens are frozen captures; changes to output shape require regen.
**How to avoid:** either (a) delete the `"notes":""` key from all three ixpfx golden JSON files by hand, (b) run any golden-regenerate flag if one exists, or (c) delete and run test once to verify output, then commit the new goldens.

### Pitfall 5: Primary not reachable during rolling deploy — DDL stalls
**What goes wrong:** Non-issue for this phase, but worth naming: `Schema.Create` holds the DB open during DDL; if the column-drop takes longer than `PDBPLUS_DRAIN_TIMEOUT`, the primary may be killed mid-DDL.
**Why it happens:** SQLite DROP COLUMN may internally rewrite the table.
**How to avoid:** Target tables (ix_prefixes ~80K rows, organizations ~45K rows) are tiny; DDL completes sub-second. Monitor primary startup logs on first deploy. If it ever became an issue, pre-applying DDL via `litefs exec` before the rolling deploy would be the mitigation — not needed for v1.15.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Column-drop migration | Custom `ALTER TABLE DROP COLUMN` SQL | `migrate.WithDropColumn(true)` option | ent handles dialect differences, ordering vs FK constraints, and rollback semantics |
| Parity regression detection | Custom schema-diff script | Existing `TestAnonParityFixtures` | Already wired, already runs in CI, already catches shape drift |
| Field-absence assertion in JSON | String-grep on response bytes | `json.Unmarshal` + map key check | Handles ordering, whitespace, and nested-shape robustness |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | No external proto consumers depend on stable field numbers for IxPrefix / Organization | Codegen pipeline impact | Low — documented as wire-compat break in SUMMARY; user accepts per read-only-mirror positioning |
| A2 | Golden fixtures under `internal/pdbcompat/testdata/golden/ixpfx/` can be regenerated or hand-edited without semantic loss | Codegen pipeline impact | Very low — golden tests are shape assertions; three small files |
| A3 | `organizations` and `ix_prefixes` tables are small enough that SQLite DROP COLUMN completes within the drain timeout | SQLite compatibility | Low — ~88MB total DB, target tables are ~45K/80K rows; prior auto-migrate operations completed without timeout in v1.0-v1.14 |

**All other claims are VERIFIED (via file inspection) or CITED (via entgo.io/docs/migrate, sqlite.org/lang_altertable.html).**

## Open Questions

**None.** All questions from the CONTEXT.md prompt and the research question brief have been answered concretely:

1. ✓ Does ent default to additive-only migrations? **Yes.**
2. ✓ Is `WithDropColumn(true)` the correct flag? **Yes, plus `WithDropIndex(true)` idiomatic companion.**
3. ✓ Where is it wired? **`cmd/peeringdb-plus/main.go:114` as `schema.MigrateOption` args to `Schema.Create(ctx, opts...)`. Import `ent/migrate`.**
4. ✓ Does it need `WithDropIndex(true)` for the three target fields? **Not strictly — they're unindexed. Include anyway for hygiene and future drops.**
5. ✓ modernc.org/sqlite or LiteFS interaction? **None. modernc bundles SQLite ≥ 3.35 (DROP COLUMN works). LiteFS WAL replication handles schema changes transparently.**
6. ✓ Does `go generate ./...` clean up entgql/entrest/entproto layers? **Yes, automatically. Hand-written references in `internal/peeringdb/`, `internal/pdbcompat/`, `internal/sync/`, `internal/grpcserver/` need manual edits (enumerated in "Codegen pipeline impact").**
7. ✓ entproto annotation cleanup on the three fields? **None needed — none of the three fields had entproto annotations to remove. Proto field numbers will auto-renumber on regen.**

## Sources

### Primary (HIGH confidence)
- `ent/migrate/migrate.go` (this repo) lines 20-30 — verifies `WithDropColumn` / `WithDropIndex` are re-exported via `ent/migrate`
- `ent/migrate/schema.go` (this repo) — confirms target columns and their absence of index/constraint associations
- `ent/schema/{ixprefix,organization}.go` — confirms no indexes on `notes`, `fac_count`, `net_count`
- `cmd/peeringdb-plus/main.go:114` — single `Schema.Create` call site confirmed via grep
- `go.mod` — modernc.org/sqlite v1.48.2 confirmed
- Codebase grep for `\.Notes|FacCount|NetCount|ip\.Notes|SetNotes` — enumerates all hand-written references

### Secondary (MEDIUM-HIGH confidence)
- [entgo.io/docs/migrate/](https://entgo.io/docs/migrate/) — official ent docs for drop-column migrations, exact Go call pattern
- [sqlite.org/lang_altertable.html](https://www.sqlite.org/lang_altertable.html) — authoritative list of SQLite DROP COLUMN restrictions

### Tertiary (contextual)
- [pkg.go.dev/modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) — modernc bundled SQLite version
- LiteFS docs (referenced from CLAUDE.md) — maintenance-mode but WAL-replication contract unchanged

## Metadata

**Confidence breakdown:**
- Drop-column mechanism: HIGH — verified against both local generated code and official docs
- Codegen pipeline impact: HIGH — exhaustive grep enumerates every call site
- SQLite compatibility: HIGH — authoritative sqlite.org docs + modernc version check + no-indexes audit
- Runtime wiring: HIGH — single call site, existing `isPrimary` gate

**Research date:** 2026-04-18
**Valid until:** 2026-05-18 (ent is stable; SQLite is stable; nothing in scope is fast-moving)
