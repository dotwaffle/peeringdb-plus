# Phase 62: API key default & docs - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Switch the production deployment to authenticated sync and document it as the recommended path. This is intentionally a manual-verification + docs phase: minimal code, three doc edits, one operator action (`fly secrets set`), and verification that the deployed app's startup log reflects the change.

Depends on phases 60 and 61 — until those land, the privacy floor isn't proven and the operator can't observe the resulting state. Final phase of v1.14.

</domain>

<decisions>
## Implementation Decisions

### Production Fly secret rollout
- **D-01:** Operator (you) sets the secret manually via `fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key> --app peeringdb-plus`. Phase 62 does NOT introduce CI-managed secret rollout (no GitHub Actions secret, no Makefile helper).
- **D-02:** `docs/DEPLOYMENT.md` includes the exact command and a one-paragraph note on how to obtain a key from PeeringDB (link to peeringdb.com user settings page).
- **D-03:** Verification step in this phase: after the secret is set and the app rolls, check the deployed app's startup log via `fly logs --app peeringdb-plus` and confirm `auth=authenticated` appears in the structured log line that phase 61 added.
- **D-04:** Verification step also confirms `fly secrets list --app peeringdb-plus` shows `PDBPLUS_PEERINGDB_API_KEY` (the value is masked, only the digest is visible — that's expected).

### `docs/CONFIGURATION.md` updates
- **D-05:** Update the existing env-var reference table:
  - `PDBPLUS_PEERINGDB_API_KEY` row gains a "Recommended" note in the description column (already present in the table; just amend the description)
  - `PDBPLUS_PUBLIC_TIER` is added as a new row with default `public`, accepted values `public|users`, and a one-line description pointing to the new subsection
- **D-06:** Add a new "Privacy & Tiers" subsection to `docs/CONFIGURATION.md` that explains the model end-to-end:
  - Anonymous request → `Public`-tier data only (default behaviour)
  - Authenticated sync (API key set) → `Users`-tier rows are present in the DB but filtered on output
  - `PDBPLUS_PUBLIC_TIER=users` override → for internal/private deployments where filtering is unnecessary; logged with WARN at startup
  - Cross-link to `docs/ARCHITECTURE.md` for the why
- **D-07:** Make sure the existing sentence in CLAUDE.md §"Environment Variables" stays consistent — that table is authoritative for project memory; phase 62 must update it too.

### `docs/DEPLOYMENT.md` updates
- **D-08:** New subsection "Authenticated PeeringDB Sync (Recommended)" that walks through:
  1. Get a PeeringDB API key (link to settings page)
  2. Set the Fly secret (`fly secrets set`)
  3. Confirm rollout (`fly logs` shows `auth=authenticated`)
  4. Note the operational implication: Users-tier rows now present in the DB, filtered on anonymous reads
- **D-09:** Add a separate "Private/Internal Deployments" subsection covering the `PDBPLUS_PUBLIC_TIER=users` override use case. Make explicit that this is for non-internet-facing deployments only, and that the WARN log at startup is intentional.

### `docs/ARCHITECTURE.md` updates
- **D-10:** New section on the privacy layer:
  - Prose explanation of: privacy middleware → typed tier in context → ent privacy policy → row filter on output
  - Sequence diagram (Mermaid or ASCII) for one anonymous request showing the inversion: sync writes everything (bypass), reads filter (policy)
  - Per-surface sentence noting where each surface picks up the privacy ctx (mostly: it's automatic via the middleware, ent inherits the ctx)
- **D-11:** Sequence diagram is required (not optional). The bypass + filter relationship is non-obvious from prose alone; the diagram makes it self-explanatory for new contributors.

### Verification (phase-internal)
- **D-12:** After the secret is set, run a smoke test against the deployed app:
  - `curl https://peeringdb-plus.fly.dev/api/poc | jq '. | length'` (returns Public-only POC count)
  - `curl https://peeringdb-plus.fly.dev/about` (renders the new "Privacy & Sync" section showing `auth=authenticated, public_tier=public`)
- **D-13:** Document the smoke test in the phase summary so it's reproducible if the deployment is ever re-rolled.

### Claude's Discretion
- Exact link target for the PeeringDB API-key obtaining instructions (current: peeringdb.com user settings; pick the canonical page)
- Whether the sequence diagram is Mermaid (preferred for GitHub rendering) or ASCII (works in plain text)
- Cross-link layout between CONFIGURATION/DEPLOYMENT/ARCHITECTURE — keep it natural, don't force a 3-way crosslink everywhere

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `/home/dotwaffle/.claude/plans/ancient-tumbling-comet.md`
- `.planning/PROJECT.md` §"Current Milestone: v1.14"
- `.planning/REQUIREMENTS.md` — SYNC-01, DOC-01, DOC-02, DOC-03
- `.planning/ROADMAP.md` §"Phase 62: API key default & docs"

### Documentation files this phase modifies
- `docs/CONFIGURATION.md` — env-var table + new "Privacy & Tiers" subsection
- `docs/DEPLOYMENT.md` — authenticated sync + private-instance subsections
- `docs/ARCHITECTURE.md` — privacy layer section with sequence diagram
- `CLAUDE.md` §"Environment Variables" — keep table consistent (project memory authoritative)

### Predecessor outputs
- `.planning/phases/59-ent-privacy-policy-sync-bypass/59-CONTEXT.md` — privacy model details for the ARCHITECTURE.md section
- `.planning/phases/61-operator-facing-observability/61-CONTEXT.md` — startup log fields documented in DEPLOYMENT.md verification steps

### External references
- https://www.peeringdb.com/profile (or canonical equivalent) — where users obtain a PeeringDB API key
- https://fly.io/docs/reference/secrets/ — `fly secrets set` reference

### Project conventions
- `CLAUDE.md` §"Documentation" — canonical user/operator/contributor docs live in `docs/`; CLAUDE.md is project memory only and is excluded from `/gsd-docs-update` runs
- `CLAUDE.md` §"Deployment" — existing `fly deploy` workflow notes; phase 62 doesn't change deploy mechanics, only adds the secret step

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- The 8 docs under `docs/` already exist (generated by `/gsd-docs-update` ac6e330) and were verified against the codebase — the structure is in place
- Existing CONFIGURATION.md env-var table is the canonical reference; just extend it
- Mermaid diagrams render natively on GitHub — no extra tooling needed

### Established Patterns
- Documentation tone: factual, operator-focused, no marketing language (matches the existing 8 docs)
- Cross-linking: relative paths between docs (`./ARCHITECTURE.md`)
- CLAUDE.md is for Claude's project memory; user-facing docs live in `docs/` (per CLAUDE.md §"Documentation")

### Integration Points
- Phase 62 is the last v1.14 phase — its completion is the milestone's effective ship gate
- The `fly secrets set` operation is the operator action that flips the production deployment from anon to authenticated sync; everything else in this phase is documentation that surrounds it

</code_context>

<specifics>
## Specific Ideas

- **Manual operator action only.** No CI-managed secret rollout. The fly token + the PeeringDB key both being repo secrets doubles blast radius for marginal convenience.
- **Sequence diagram is required.** Without it, the bypass+filter inversion is the single most non-obvious thing in the privacy layer. Diagram pays for itself the first time a new contributor tries to read the privacy code without it.
- **Resist over-planning.** This is a docs-and-secret phase. If the executing agent finds themselves writing > 50 lines of code, something has scope-crept.

</specifics>

<deferred>
## Deferred Ideas

- Multi-environment fly secret management (staging vs prod) — only one env exists; revisit when staging is introduced.
- Automated post-deploy smoke test in CI — nice-to-have; the manual smoke test in D-12 is sufficient for v1.14.
- Documentation page for "How visibility evolved at PeeringDB" (history, the 2.30.0 Private removal, etc.) — interesting context but not operator-facing; leave as a CLAUDE.md memory if needed.

</deferred>

---

*Phase: 62-api-key-default-docs*
*Context gathered: 2026-04-16*
