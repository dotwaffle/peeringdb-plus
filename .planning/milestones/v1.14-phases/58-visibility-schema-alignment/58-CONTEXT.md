# Phase 58: Visibility schema alignment - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Bring the ent schemas into agreement with the empirical visibility baseline produced in phase 57. Every entity that the diff report identifies as auth-gated must have ent schema fields the privacy policy (phase 59) can key off. `poc.visible` already exists; this phase adds whatever else phase 57 surfaces.

This phase cannot start until phase 57's diff report is committed — the diff is the work order.

</domain>

<decisions>
## Implementation Decisions

### New visibility-bearing field shape
- **D-01:** Each newly-discovered auth-gated field gets a sibling `field.String("name_visible")` defaulting to `"Public"` on the same ent schema. The privacy policy filters on the sibling field. Mirrors PeeringDB's actual schema convention (POC has `visible`; field-level visibility uses `<field>_visible`).
- **D-02:** Use `field.String` (not `field.Enum` or a custom typed enum). Matches the existing `poc.visible` shape; avoids the regen + JSON marshalling churn of introducing a typed enum across ent + entgql + entrest + entproto. Validate values at the privacy-policy layer rather than the schema layer.
- **D-03:** **Do not** introduce a per-row visibility bitfield. Per-field `*_visible` siblings are explicit and grep-able; bitfields hide which fields each bit covers and force the privacy policy to know the mapping.

### Default values
- **D-04:** Mirror upstream default per-field. If PeeringDB defaults `social_media_visible` to `Users`, our schema defaults the same. Behaviour aligns with upstream so a brand-new operator who hasn't synced yet sees the same emptiness an anonymous PeeringDB call would.
- **D-05:** When upstream's default is unclear from the phase 57 diff (e.g. field present in both anon and auth responses with the same `visible` value across all sampled rows), default to `"Public"` and document the assumption in PROJECT.md Key Decisions so phase 60's tests can probe it.

### Migration
- **D-06:** Ent auto-migrate at startup (existing pattern). New columns added with their declared defaults. SQLite + LiteFS handles the schema change; replicas pick it up via LTX. No bespoke migration runner.
- **D-07:** Existing rows that synced before this phase will have NULL for the new `*_visible` fields until the next sync replaces them. Document that the privacy policy must treat NULL as the schema default (`"Public"` per the upstream-default rule), not as `"Users"` — to avoid surprising operators with a flood of suddenly-hidden rows post-upgrade.

### Audit trail
- **D-08:** Findings (which fields/entities turned out to be auth-gated, what the upstream default was, what we chose) get logged in PROJECT.md Key Decisions table so future maintainers can trace the why.
- **D-09:** Update CLAUDE.md ent schema notes to mention the `<field>_visible` pattern and the NULL-treats-as-default rule.

### Claude's Discretion
- Exact field names for new `*_visible` siblings (follow PeeringDB's naming if it provides one; otherwise `<field>_visible`)
- Whether to bundle multiple new fields into one phase 58 plan or split per-entity — implementation detail driven by how many fields phase 57 surfaces

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `/home/dotwaffle/.claude/plans/ancient-tumbling-comet.md` — milestone planning session
- `.planning/PROJECT.md` §"Current Milestone: v1.14"
- `.planning/REQUIREMENTS.md` — VIS-03
- `.planning/ROADMAP.md` §"Phase 58: Visibility schema alignment"

### Predecessor artifact (consumed)
- `testdata/visibility-baseline/DIFF.md` — phase 57's human-readable diff (read first)
- `testdata/visibility-baseline/diff.json` — phase 57's machine-readable diff (the actual work order)

### Existing ent schemas
- `ent/schema/poc.go:53-57` — existing `visible` field for reference (`field.String("visible").Optional().Default("Public")`)
- `ent/schema/ixlan.go:47-48` — existing `ixf_ixp_member_list_url_visible` field showing the per-field visibility convention already in the project (default `"Private"`)
- `ent/entc.go` — codegen entrypoint; new fields require regeneration via `go generate ./ent`

### CLAUDE.md ent code-generation notes
- `CLAUDE.md` §"Code Generation" — the codegen pipeline (entc.go → buf generate → templ generate); confirms `go generate ./...` is the regen command
- `CLAUDE.md` §"ConnectRPC / gRPC" — proto `optional` fields generate pointer types; new `*_visible` fields will surface as `*string` in the gRPC List/Get request filters via the existing entproto annotations

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `ent/schema/poc.go` `visible` field is the template — copy its annotation set (`entrest.WithFilter(entrest.FilterGroupEqual | entrest.FilterGroupArray)`) so REST consumers can filter by visibility.
- `ent/schema/ixlan.go` proves per-field visibility precedent already exists in the codebase.

### Established Patterns
- All ent schemas have `otelMutationHook(<Name>)` in `Hooks()` — preserve.
- entproto/entrest/entgql annotations live alongside `field.X` calls; new fields need them too.
- Proto regeneration is automatic via `go generate ./...` thanks to the entproto extension wired in `ent/entc.go`.

### Integration Points
- New `*_visible` fields automatically become filterable in REST + GraphQL + gRPC List requests (entrest + entgql + entproto codegen) — no handler-level changes needed for that.
- The privacy policy in phase 59 reads these fields via the generated `<entity>.FieldVisible` constants.

</code_context>

<specifics>
## Specific Ideas

- **Don't pre-emptively add fields phase 57 didn't discover.** This phase is a *response* to empirical evidence, not a scoping exercise. If phase 57 says only `poc.visible` matters, phase 58 is a small confirmation phase, not a schema rewrite.
- **NULL handling matters for upgrades.** Existing deployments will have NULL in the new columns until the next sync. The default-on-NULL policy documented in D-07 is what prevents an upgrade from suddenly hiding rows.

</specifics>

<deferred>
## Deferred Ideas

- Migrating `poc.visible` from a free-form String to a typed enum — would touch every surface; defer until a milestone whose theme is type tightening (not this one).
- Backfilling NULL `*_visible` fields via a one-shot post-upgrade job — unnecessary since the next sync will rewrite the value anyway.

</deferred>

---

*Phase: 58-visibility-schema-alignment*
*Context gathered: 2026-04-16*
