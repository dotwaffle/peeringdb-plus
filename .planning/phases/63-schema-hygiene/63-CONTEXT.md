# Phase 63: Schema hygiene — drop vestigial columns - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Drop three confirmed-vestigial ent schema fields surfaced by the post-v1.14 schema-vs-fixture audit. No new user-facing behaviour; cleans up historical schema debt so the audit-confirmed set of 13 types has zero unexplained fields.

Scope is strictly the three fields: `ixprefix.notes`, `organization.fac_count`, `organization.net_count`. Any other discrepancies surfaced by the audit are documented but not touched.

</domain>

<decisions>
## Implementation Decisions

### Scope of each drop
- **D-01: Full removal for `ixprefix.notes`.** Drop the ent schema field, drop `Notes string json:"notes"` from `peeringdb.IxPrefix`, remove emission from `pdbcompat.ixPrefixFromEnt`. Our API response matches upstream's shape exactly (no `notes` key).
- **D-02: Full removal for `organization.fac_count` and `organization.net_count`.** These are schema-only vestigials — not present in `peeringdb.Organization`, not in sync upsert, not in `organizationFromEnt` serializer. Schema edit + regen is the full fix.

### Parity test
- **D-03:** Remove the `ixpfx.notes` entry from `internal/pdbcompat/anon_parity_test.go` `knownDivergences` map once the field is dropped. Test enforces zero structural divergences on ixpfx after the drop.

### Migration strategy
- **D-04:** Rely on ent auto-migrate. Read https://entgo.io/docs/migrate/ first to verify whether ent needs an explicit `schema.WithDropColumn(true)` flag (or similar) to actually emit the `ALTER TABLE DROP COLUMN` statement — ent defaults to additive migrations for safety. If a flag is needed, wire it into `ent/entc.go` or the migration call site. Rolling deploy handles the column drop; SQLite 3.35+ (modernc.org/sqlite is modern) supports `DROP COLUMN` natively.

### Documentation updates
- **D-05: Full doc sweep.** Touch:
  - `.planning/PROJECT.md` Key Decisions — new row for v1.15: "dropped ixpfx.notes + org.{fac,net}_count as schema debt cleanup"
  - `docs/ARCHITECTURE.md` — if any schema field enumeration references these fields
  - `CLAUDE.md` — ent schema notes updated (project memory authoritative)
  - `docs/API.md` — if it documents the pdbcompat response shape for /api/ixpfx or /api/org, update to reflect absence of the dropped keys

### Ordering vs Phase 64
- **D-06:** Phase 63 runs first, before Phase 64. Phase 64 also touches `ent/schema/ixlan.go`; splitting them avoids interleaved codegen cycles. Both phases end with `go generate ./...` clean.

### Claude's Discretion
- Test commit structure (single `test(63-01)` refactor vs split per field) — implementation detail
- Whether to include a verification test that explicitly asserts the 3 fields are absent from JSON output post-drop — defensive, ~10 LoC, worth it

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `.planning/REQUIREMENTS.md` — HYGIENE-01 (ixpfx.notes drop), HYGIENE-02 (org.fac_count drop), HYGIENE-03 (org.net_count drop)
- `.planning/ROADMAP.md` §"Phase 63" — success criteria

### Audit evidence (investigation predates this phase)
- Schema vs Phase 57 auth-fixture diff: `ixpfx.notes` in schema, absent from upstream
- `peeringdb.IxPrefix` struct at `internal/peeringdb/types.go:226` — has `Notes string`
- `peeringdb.Organization` struct at `internal/peeringdb/types.go:42-64` — no `FacCount` / `NetCount`
- `internal/sync/upsert.go` — no `SetFacCount`/`SetNetCount` for Organization path
- `internal/pdbcompat/serializer.go organizationFromEnt` — no `FacCount`/`NetCount` emission
- Beta upstream `/api/org` and `/api/ixpfx` responses verified on 2026-04-17 — confirms absence

### Files to modify
- `ent/schema/ixprefix.go` — remove `field.String("notes")` at line ~34
- `ent/schema/organization.go` — remove `field.Int("net_count")` at line ~106, `field.Int("fac_count")` at line ~110
- `internal/peeringdb/types.go` — remove `Notes string \`json:"notes"\`` from `IxPrefix` struct
- `internal/pdbcompat/serializer.go` — remove `Notes: p.Notes` from `ixPrefixFromEnt`
- `internal/pdbcompat/anon_parity_test.go` — remove ixpfx.notes from `knownDivergences`
- `ent/entc.go` — possibly add `schema.WithDropColumn(true)` or equivalent flag per D-04
- `.planning/PROJECT.md`, `docs/ARCHITECTURE.md`, `CLAUDE.md`, `docs/API.md` — doc updates per D-05

### Regenerated files
- `ent/*.go`, `ent/**/*.go` — expected to drop the 3 columns from all generated layers
- `gen/peeringdb/v1/*.pb.go` — entproto regen removes the proto fields if they were present
- `graph/generated.go` — gqlgen regen removes GraphQL field types

### Project conventions
- CLAUDE.md §"Code Generation" — `go generate ./...` runs the full pipeline in order
- CLAUDE.md §"CI" — drift check covers `ent/`, `gen/`, `graph/`, `internal/web/templates/`

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `TestAnonParityFixtures` in `internal/pdbcompat/anon_parity_test.go` is the regression gate — once ixpfx.notes drops, this test enforces shape parity automatically.
- ent's codegen pipeline is idempotent; a clean run after the schema edits produces deterministic output.

### Established Patterns
- v1.14 used ent auto-migrate for POC `Policy()` landing without issues — same mechanism reused here.
- `golangci-lint run ./...` cleanly catches stray references to dropped fields (unused imports, unused struct fields).

### Integration Points
- Phase 64 follows this phase and touches `ent/schema/ixlan.go`. Serial execution keeps codegen cycles clean.

</code_context>

<specifics>
## Specific Ideas

- **Read entgo migrate docs before coding.** The ent migration behaviour around dropping columns may require a specific flag (e.g. `schema.WithDropColumn(true)`) — user explicitly flagged this as a prerequisite. Don't skip.
- **Shape parity is the success signal.** `TestAnonParityFixtures` passing without the `ixpfx.notes` knownDivergence entry is proof we landed it.
- **Doc sweep is deliberate.** User chose "full sweep" over "minimum only" — touch all 4 doc surfaces where these fields might appear.

</specifics>

<deferred>
## Deferred Ideas

- **Broader schema audit phase.** v1.15 Phase 64 closes the one genuine feature gap (ixlan URL). Other schema divergences beyond these 3 are not known to exist post-audit, but a periodic (e.g. v1.16+) audit job that re-runs the schema-vs-fixture diff would catch future drift.
- **Automated schema drift CI check.** Run the audit in CI as a lint step. Not scoped for v1.15 since there's nothing to drift against until v1.14's baseline gets regenerated.

</deferred>

---

*Phase: 63-schema-hygiene*
*Context gathered: 2026-04-17*
