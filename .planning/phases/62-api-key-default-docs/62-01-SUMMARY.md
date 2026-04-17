---
phase: 62-api-key-default-docs
plan: 01
subsystem: docs

tags: [documentation, privacy, api-key, fly-secrets, mermaid]

# Dependency graph
requires:
  - phase: 59-ent-privacy-policy-sync-bypass
    provides: privctx.Tier, ent Privacy policy, sync-worker bypass
  - phase: 60-privacy-surface-wiring
    provides: five API surfaces inheriting privacy ctx via middleware
  - phase: 61-operator-facing-observability
    provides: sync.classification log, privacy.override.active WARN, pdbplus.privacy.tier OTel attr
provides:
  - docs/CONFIGURATION.md Privacy & Tiers subsection + PDBPLUS_PUBLIC_TIER env var row
  - docs/DEPLOYMENT.md Authenticated PeeringDB Sync (Recommended) subsection
  - docs/DEPLOYMENT.md Private/Internal Deployments subsection
  - docs/ARCHITECTURE.md Privacy layer section with Mermaid sequence diagram
  - CLAUDE.md env-var table parity (PDBPLUS_PUBLIC_TIER + recommended note)
affects: [62-02, future v1.14 ship docs, operator rollout]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Mermaid sequenceDiagram fenced code blocks for non-obvious control-flow inversions"

key-files:
  created: []
  modified:
    - docs/CONFIGURATION.md
    - docs/DEPLOYMENT.md
    - docs/ARCHITECTURE.md
    - CLAUDE.md

key-decisions:
  - "PeeringDB API key landing page: https://www.peeringdb.com/profile (user profile, API Keys tab)"
  - "Mermaid sequenceDiagram chosen over ASCII — GitHub renders natively, makes the bypass/filter inversion self-explanatory"
  - "Cross-links between docs/ files use ./FILENAME.md style (matches existing ARCHITECTURE.md convention, not ../docs/)"
  - "PDBPLUS_PEERINGDB_API_KEY stays Required=No/default=(empty) — 'Recommended' prefix only; no-key sync remains supported per SYNC-02"

patterns-established:
  - "Mermaid sequence diagrams in ARCHITECTURE.md for multi-actor flows where prose alone hides an inversion"
  - "Privacy & Tiers is the canonical end-to-end explainer (CONFIGURATION.md); DEPLOYMENT.md links to it rather than duplicating"

requirements-completed: [DOC-01, DOC-02, DOC-03]

# Metrics
duration: 7min
completed: 2026-04-17
---

# Phase 62 Plan 01: v1.14 Privacy Layer Docs Summary

**Four doc edits ship the v1.14 privacy floor: PDBPLUS_PUBLIC_TIER documented, authenticated sync promoted to the recommended production path, and a Mermaid sequence diagram captures the sync-write-bypass vs read-path-filter inversion.**

## Performance

- **Duration:** ~7 min
- **Started:** 2026-04-17T22:02:51Z
- **Completed:** 2026-04-17T22:09:47Z
- **Tasks:** 4/4
- **Files modified:** 4

## Accomplishments

- `docs/CONFIGURATION.md` now documents `PDBPLUS_PEERINGDB_API_KEY` as **Recommended** and adds `PDBPLUS_PUBLIC_TIER` as a first-class env-var row with a dedicated Privacy & Tiers subsection walking through all three operational states (default, authenticated sync, users override).
- `docs/DEPLOYMENT.md` promotes the authenticated sync path with a four-step rollout walkthrough (get key → `fly secrets set` → confirm via `fly logs` → operational implication) and adds a Private/Internal Deployments subsection for the `PDBPLUS_PUBLIC_TIER=users` override.
- `docs/ARCHITECTURE.md` adds a Privacy layer top-level section between "API surfaces" and "LiteFS primary/replica detection", prose-covering the four pieces (context stamping, ent Policy, sync bypass, observability) and a Mermaid sequenceDiagram contrasting the sync-write (bypass) path with the anonymous-read (filter) path.
- `CLAUDE.md` env-var table mirrors CONFIGURATION.md with the new `PDBPLUS_PUBLIC_TIER` row and a recommended-for-production note on `PDBPLUS_PEERINGDB_API_KEY` — project memory stays authoritative.

## Task Commits

Each task was committed atomically:

1. **Task 1: Extend docs/CONFIGURATION.md with PDBPLUS_PUBLIC_TIER + Privacy & Tiers subsection** — `21b394f` (docs)
2. **Task 2: Extend docs/DEPLOYMENT.md with Authenticated and Private subsections** — `228fbcf` (docs)
3. **Task 3: Extend docs/ARCHITECTURE.md with Privacy layer section + Mermaid sequence diagram** — `2c60496` (docs)
4. **Task 4: Update CLAUDE.md env-var table with PDBPLUS_PUBLIC_TIER** — `c38513c` (docs)

## Files Created/Modified

- `docs/CONFIGURATION.md` — +40 lines (PDBPLUS_PUBLIC_TIER row, Privacy & Tiers subsection with three sub-subsections, Recommended note on API key row)
- `docs/DEPLOYMENT.md` — +53 lines (Authenticated PeeringDB Sync (Recommended) four-step walkthrough, Private/Internal Deployments subsection)
- `docs/ARCHITECTURE.md` — +86 lines (Privacy layer top-level section, four-piece prose breakdown, Mermaid sequenceDiagram, operator control surface callout)
- `CLAUDE.md` — +2/-1 lines (new PDBPLUS_PUBLIC_TIER row, amended PDBPLUS_PEERINGDB_API_KEY description)

Totals vs base: 180 insertions, 2 deletions across 4 files.

## Verification

All seven acceptance greps from the plan's `<verification>` block pass:

```text
1/7 grep -q '^## Privacy & Tiers$' docs/CONFIGURATION.md                          OK
2/7 grep -q '^| `PDBPLUS_PUBLIC_TIER` |' docs/CONFIGURATION.md                    OK
3/7 grep -q '^### Authenticated PeeringDB Sync (Recommended)$' docs/DEPLOYMENT.md OK
4/7 grep -q '^### Private/Internal Deployments$' docs/DEPLOYMENT.md               OK
5/7 grep -q '^## Privacy layer$' docs/ARCHITECTURE.md                             OK
6/7 grep -q 'sequenceDiagram' docs/ARCHITECTURE.md                                OK
7/7 grep -q '^| `PDBPLUS_PUBLIC_TIER` | `public` |' CLAUDE.md                     OK
```

Both sanity checks also pass:

```text
grep -q '^```mermaid$' docs/ARCHITECTURE.md                                       OK
! grep -n '\.\./docs/' docs/ARCHITECTURE.md                                       OK (no matches)
```

## Decisions Made

- **PeeringDB API key link target:** `https://www.peeringdb.com/profile` (signed-in user's profile page; API key management lives under the "API Keys" tab). Used in both DEPLOYMENT.md and the plan's `<interfaces>` block.
- **Diagram format:** Mermaid `sequenceDiagram` in a ```` ```mermaid ```` fenced code block. GitHub renders it natively, no tooling added, and the diagram makes the bypass/filter inversion self-explanatory for new contributors.
- **Cross-link style:** Used `./CONFIGURATION.md#privacy--tiers` and `./DEPLOYMENT.md#authenticated-peeringdb-sync-recommended` (not `../docs/...`) — the plan's suggested body had `../docs/...` but that matches nothing since the file lives in `docs/`. Corrected per the plan's own note at Task 3 to match existing ARCHITECTURE.md link convention.
- **Recommendation wording:** `**Recommended.**` (bold, full-stop) at the start of the API-key description cell, with Required/Default/Type columns unchanged — no-key sync remains supported per SYNC-02, so Recommended is the right weight.

## Deviations from Plan

None — plan executed exactly as written. All content blocks inserted verbatim, grep verifications pass on the first run, and no bugs or missing critical functionality surfaced (pure docs-only plan).

## Issues Encountered

- Worktree-branch-check reset: the agent's HEAD was on a different ancestry at startup (`3f4c8ad` on main) than the plan's base (`451a7551`). Per the worktree_branch_check protocol, hard-reset to `451a7551e05914e229ce4727579ca276e3aae4cd` before any edits. This is the expected startup gate for a fresh worktree executor, not a plan deviation.
- `Read`-tool context: the executor had already read each target file earlier in the session (CONFIGURATION.md, DEPLOYMENT.md, ARCHITECTURE.md region, CLAUDE.md). Each `Edit` call triggered a precautionary read-before-edit hook reminder, but the edits landed successfully without re-reads.

## User Setup Required

None — no external service configuration required. The `fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key> --app peeringdb-plus` operator action documented in DEPLOYMENT.md is executed by plan 62-02, not here.

## Self-Check: PASSED

- All four modified files exist and contain the expected content.
- All four task commits exist in `git log --oneline --all`: `21b394f`, `228fbcf`, `2c60496`, `c38513c`.
- All seven acceptance greps + two sanity greps pass.

## Next Phase Readiness

- v1.14 documentation paper trail complete: operators have a path from CONFIGURATION → DEPLOYMENT → ARCHITECTURE covering privacy semantics, rollout command, and the non-obvious bypass/filter inversion.
- Plan 62-02 (operator Fly secret rollout) can now reference `docs/DEPLOYMENT.md#authenticated-peeringdb-sync-recommended` as the canonical rollout procedure.

---
*Phase: 62-api-key-default-docs*
*Completed: 2026-04-17*
