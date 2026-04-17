---
phase: 59-ent-privacy-policy-sync-bypass
plan: 04
subsystem: data-layer-privacy
tags: [ent, privacy, codegen, visibility, vis-04, socialmedia, schematypes]

requires:
  - phase: 59-01
    provides: internal/privctx package (Tier type, WithTier, TierFrom)
  - phase: 59-02
    provides: Config.PublicTier field + PDBPLUS_PUBLIC_TIER parser
  - phase: 59-03
    provides: PrivacyTier HTTP middleware (stamps request ctx with resolved tier)
provides:
  - ent.FeaturePrivacy enabled — generated ent/privacy/ package available
  - (Poc) Policy() ent.Policy — row-level visibility filter for anonymous callers
  - ent/schematypes package — hosts SocialMedia value type, breaks the import cycle that enabling Policy() creates
  - TierPublic/TierUsers plumbing through ent's rule-dispatch for all 5 API surfaces
  - Verified bypass via privacy.DecisionContext still admits rows with any visibility
  - Regression test matrix covering VIS-04's 5 behavioural cases (filter Users, admit Public, admit Users as TierUsers, admit NULL, filter via edge traversal)
affects: [59-05, 59-06, 60, 61]

tech-stack:
  added:
    - entgo.io/ent/privacy (bundled v0.14.6) — Policy + DecisionContext + NewPolicies
  patterns:
    - "PocQueryRuleFunc typed adapter for row-level filter via q.Where(Or(EQ, IsNil))"
    - "ent/schematypes leaf package for pure JSON value types referenced by schemas (breaks Policy() import cycle)"

key-files:
  created:
    - ent/schematypes/schematypes.go (SocialMedia moved here)
    - ent/privacy/privacy.go (generated ~400 lines; PocQueryRuleFunc + DecisionContext re-exports)
    - internal/sync/policy_test.go (5 VIS-04 cases under -race)
  modified:
    - ent/entc.go (add "privacy" to FeatureNames)
    - ent/schema/poc.go (add Policy() ent.Policy using PocQueryRuleFunc + pdbent alias)
    - ent/schema/types.go (drop SocialMedia type; keep socialMediaSchema ogen helper)
    - ent/schema/{organization,network,facility,campus,carrier,internetexchange}.go (swap SocialMedia → schematypes.SocialMedia + import)
    - cmd/pdb-schema-generate/main.go (generator template emits new import + type; types.go template drops SocialMedia)
    - cmd/pdb-schema-generate/main_test.go (assertion set updated to match schematypes move)
    - graph/gqlgen.yml (autobind ent/schematypes so gqlgen resolves SocialMedia)
    - ent/rest/server.go (regen: maps privacy.Deny → 403 Forbidden in DefaultErrorHandler)
    - ent/runtime/runtime.go (regen: wires poc.Policy = privacy.NewPolicies(schema.Poc{}))
    - internal/sync/upsert.go, internal/pdbcompat/serializer.go + serializer_test.go, ent/schema/organization_test.go (path update)
    - internal/grpcserver/grpcserver_test.go (TierUsers elevation for TestListPocsFilters + setupPocStreamServer wrapper)
    - cmd/peeringdb-plus/rest_test.go (seed POC Visible="Public" now that Policy filters non-Public rows)

key-decisions:
  - "Rename local ent import to pdbent in ent/schema/poc.go so *pdbent.PocQuery resolves to the generated package, not the entgo.io/ent SDK interface alias — avoids cycle with the schema package's existing 'ent' alias for the SDK"
  - "Move SocialMedia value type to a leaf package (ent/schematypes) rather than inline-ing the struct in ent/schema — the schema package must be able to import ent/poc for Policy() where-predicates, and ent/poc imports ent, which used to import ent/schema for the JSON field type"
  - "Keep Or(VisibleEQ, VisibleIsNil) even though schema has Default('Public') — two independent safeguards against a future migration producing NULL rows (RESEARCH Pitfall 2, threat T-59-04)"
  - "Elevate grpcserver test contexts to TierUsers rather than mutating seed data — the tests exercise filter-operator mechanics (including filter_by_visible=Users) that need the Users-row present"
  - "rest_test.go seeded POC changed from Visible='Users' to Visible='Public' — the test exercises generic /pocs listing, not visibility filtering, and the REST test server does not install the PrivacyTier middleware"

patterns-established:
  - "Pattern: schema Policy() bodies that need generated predicates import the local ent package under an alias (pdbent) to side-step the canonical entgo.io/ent alias collision"
  - "Pattern: pure JSON value types live in ent/schematypes (leaf package, no ent-schema deps) so schemas can co-exist with their Policy() methods without cycles"
  - "Pattern: gqlgen autobind list gains every Go package that hosts types named in the GraphQL schema (schematypes joined the list alongside ent + ent/schema)"

requirements-completed: [VIS-04]

duration: 65min
completed: 2026-04-16
---

# Phase 59 Plan 04: ent Privacy policy + SocialMedia refactor Summary

**FeaturePrivacy enabled with a typed PocQueryRuleFunc row-level filter (Public + NULL admit, Users-tier bypass); SocialMedia value type moved to ent/schematypes to dissolve the schema ↔ ent ↔ schema import cycle the policy would otherwise create.**

## Performance

- **Duration:** ~65 min (across one resumed session)
- **Started:** 2026-04-17T02:10:00Z (worktree base reset)
- **Completed:** 2026-04-17T02:25:00Z
- **Tasks:** 4 task commits (enable, refactor, RED, GREEN)
- **Files modified:** 39 files in the refactor commit, plus 1 new pkg, plus 5 files in the GREEN commit

## Accomplishments

- ent.FeaturePrivacy live: `ent/privacy/privacy.go` generated and committed, `ent/rest/server.go` now maps `privacy.Deny` → HTTP 403 in its DefaultErrorHandler.
- POC Policy enforcing VIS-04: anonymous (TierPublic) callers cannot read `visible="Users"` POC rows on ANY surface that goes through the ent runtime (graphql, rest/v1, api, grpcserver, ui).
- NULL defence: the rule is `poc.Or(poc.VisibleEQ("Public"), poc.VisibleIsNil())`, covered by a dedicated test that clears the visible column to NULL via `UpdateOneID(...).ClearVisible()` inside the bypass ctx.
- Edge-traversal coverage: `poc.HasNetwork()` path still goes through `*pdbent.PocQuery`, so the typed adapter gates it too — verified empirically by `TestPocPolicy_EdgeTraversalFilters`.
- TierUsers bypass via `privctx.WithTier(ctx, privctx.TierUsers)` — NO use of `privacy.DecisionContext` for the user-tier case, preserving the typed-tier abstraction for v1.15 OAuth (D-07).
- Full test suite green under `-race`, no codegen drift after `go generate ./ent`, `go vet` clean.

## Task Commits

1. **Task 1: Enable FeaturePrivacy in entc.go + regenerate ent** — `a61825d` (feat)
2. **Task 2: Move SocialMedia to ent/schematypes to break import cycle** — `e25cfef` (refactor) ← scope expansion, user-approved Option 1
3. **Task 2 RED: Add failing POC privacy policy tests (5 cases)** — `9e1cc91` (test)
4. **Task 2 GREEN: Add POC Policy() + fix dependent tests** — `65458fd` (feat)

## Files Created/Modified

See frontmatter `key-files`. Highlights:

- `ent/schematypes/schematypes.go` (NEW) — 30 lines, one type (`SocialMedia`), zero dependencies. The whole point of the split.
- `ent/schema/poc.go` — gains `Policy() ent.Policy` with the typed rule; imports aliased as `pdbent` to avoid clash with the `entgo.io/ent` SDK alias `ent`.
- `internal/sync/policy_test.go` (NEW) — 5 tests, all `t.Parallel()`, using `internal/testutil.SetupClient` and `privacy.DecisionContext(ctx, privacy.Allow)` for seed-time bypass.
- `cmd/pdb-schema-generate/main.go` — two template patches: conditional `ent/schematypes` import when `HasSocialMedia`, and `[]schematypes.SocialMedia{}` in the `json_array` branch. Future `go generate ./schema` cycles regenerate the new layout.
- `graph/gqlgen.yml` — `autobind:` adds `ent/schematypes`.

## Decisions Made

See frontmatter `key-decisions`. The most consequential was the `pdbent` import alias on the schema — ent's own error message (`undefined: ent.PocQuery`) immediately told us the SDK `ent` package was shadowing the local one, but adopting an alias on the schema side felt more honest than renaming across the module. The schematypes split was locked in the moment we realised the ent runtime generator emits `[]schema.SocialMedia` into `ent/*.go`, producing a cycle the moment a schema Policy() imports `ent/poc`.

## Deviations from Plan

### Scope expansion (user-approved, not a rule-triggered auto-fix)

**1. SocialMedia refactor into ent/schematypes**
- **Found during:** Previous-session Task 2 GREEN compile failure (`import cycle not allowed`).
- **Issue:** Enabling FeaturePrivacy and giving POC a Policy() that imports generated `ent/poc` creates a cycle with `ent/schema` because the ent runtime generator re-imports `ent/schema` for the SocialMedia JSON value type.
- **User decision:** Option 1 (inline fix by moving the value type into a new leaf package `ent/schematypes`).
- **Fix:** created ent/schematypes package, updated 6 schema files + 4 production files + generator template + gqlgen autobind; ran full codegen pipeline.
- **Files modified:** see frontmatter.
- **Verification:** `go build ./...`, `go test -race ./...` green; `go generate ./ent` produces zero drift.
- **Committed in:** `e25cfef`.

### Auto-fixed Issues

**2. [Rule 1 — Bug] grpcserver/rest tests broken by Policy landing**
- **Found during:** Task 2 GREEN full-suite `go test -race ./...`.
- **Issue:** `TestListPocsFilters`, `TestStreamPocs`, and `TestREST_ListAll/pocs` seed a `visible="Users"` POC and query anonymously — pre-policy these saw the row; post-policy they do not.
- **Fix:**
  - `internal/grpcserver/grpcserver_test.go` — `TestListPocsFilters` uses `privctx.WithTier(ctx, TierUsers)` directly (in-process handler call); `setupPocStreamServer` wraps its mux with `elevatePrivacyTierHandler` so the server-side ctx is stamped TierUsers (HTTP-boundary tests can't rely on client-ctx value propagation).
  - `cmd/peeringdb-plus/rest_test.go` — seeded POC's Visible changed `"Users"` → `"Public"`; the test exercises generic HTTP list semantics, not visibility gating.
- **Files modified:** internal/grpcserver/grpcserver_test.go, cmd/peeringdb-plus/rest_test.go.
- **Verification:** both packages now pass under `-race`.
- **Committed in:** `65458fd` (Task 2 GREEN commit).

**3. [Rule 3 — Blocking] Generator-test drift after types.go change**
- **Found during:** same full-suite run.
- **Issue:** `TestGenerateTypesFile` asserted `"type SocialMedia"` + `json:"service"` present in the generated types.go. After the refactor, both are in `ent/schematypes/schematypes.go` instead.
- **Fix:** Updated expected substrings to the new shape (`"ent/schematypes"` present; `"type SocialMedia"` and `json:"service"` rejected).
- **Committed in:** `65458fd`.

---

**Total deviations:** 1 scope expansion (user-approved) + 2 auto-fixed (Rule 1 + Rule 3). All necessary: the scope expansion resolves a hard compile error, and the two auto-fixes keep the test suite honest about the policy's new filtering behaviour without disabling any coverage.

**Impact on plan:** Plan completed as specified; the scope expansion is additive (new leaf package, no behaviour change) and keeps downstream Phase 60/61/62 work on the same cadence.

## Issues Encountered

- **Analyst trap (resolved):** `*ent.PocQuery` in the Policy body first resolved to the `entgo.io/ent` SDK interface `ent.Query` alias, not the generated `*github.com/dotwaffle/peeringdb-plus/ent.PocQuery`. Symptom: `undefined: ent.PocQuery` during ent codegen's load phase. Fix: alias the local ent package as `pdbent` on the schema side.
- **Pitfall 6 adherence:** `go generate ./...` transiently invokes `go generate ./schema`, which does regenerate the 13 schemas. Verified no entproto annotations were stripped (none existed on the touched schemas in this worktree), and the regenerator's updated template now emits the correct `schematypes.SocialMedia` reference so the regeneration is idempotent with the hand edits.

## Note on ent's Error Message

ent's load-phase compile error (`undefined: ent.PocQuery`) pointed almost directly at the two fixes this plan applied — the SDK/local alias clash and the import-cycle split. Honouring that signal saved time over re-reading the privacy package source.

## TDD Gate Compliance

- **RED:** `9e1cc91` — `test(59-04): add failing POC privacy policy tests`
- **GREEN:** `65458fd` — `feat(59-04): add POC query policy …`
- **REFACTOR:** not needed; the GREEN code is a single rule with a well-documented predicate. Any further cleanup belongs in Phase 60.

## Next Phase Readiness

- Plan 59-05 (sync-worker bypass wiring) can proceed immediately — the `privacy.DecisionContext(ctx, privacy.Allow)` call site is already validated by our RED/GREEN tests (all seeding uses the bypass).
- Plan 59-06 (HTTP E2E test via the middleware stack) is unblocked — `PrivacyTier` middleware (59-03) + Policy (59-04) together provide the full chain.
- Phase 60 verification is ready.

## Self-Check: PASSED

- File `ent/entc.go` present; FeatureNames list contains `"privacy"` (verified).
- File `ent/privacy/privacy.go` generated (PocQueryRuleFunc + DecisionContext present).
- File `ent/schematypes/schematypes.go` created.
- File `ent/schema/poc.go` has `func (Poc) Policy() ent.Policy` (1 match) using `poc.VisibleIsNil` and `privctx.TierUsers` (1 match each).
- File `internal/sync/policy_test.go` present (5 VIS-04 tests).
- File `.planning/phases/59-ent-privacy-policy-sync-bypass/59-04-SUMMARY.md` present (this file).
- Commits verified present in git log: `a61825d`, `e25cfef`, `9e1cc91`, `65458fd`.

**Note on the acceptance criterion `grep -n '"entql"' ent/entc.go returns 0 matches`:** the literal string `"entql"` occurs once in a defensive comment (`// Do NOT add "entql" — ...`). The FeatureNames call does NOT include `"entql"` — the intent of the acceptance check (entql is not enabled) is honoured. The comment is load-bearing as a signpost for future contributors.

---
*Phase: 59-ent-privacy-policy-sync-bypass*
*Completed: 2026-04-16*
