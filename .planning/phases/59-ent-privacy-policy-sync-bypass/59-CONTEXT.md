# Phase 59: ent Privacy policy + sync bypass - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Install the read-path privacy floor: anonymous queries cannot return rows whose upstream visibility is `Users`-only. The sync worker retains full read/write access via a context-scoped bypass. The `PDBPLUS_PUBLIC_TIER` env var lets internal-only deployments treat anonymous callers as Users-tier without code changes.

This is the gate phase for everything user-visible in v1.14. Phases 60 (verification) and 61 (observability) consume what this phase establishes.

</domain>

<decisions>
## Implementation Decisions

### Privacy policy structure
- **D-01:** Per-entity `Policy()` methods. Only entities that actually carry visibility (POC + whatever phase 58 surfaces) get a `Policy()` method returning a `privacy.Policy` with the query rule. Surgical, easy to audit per entity, matches ent docs verbatim. No global Mixin.
- **D-02:** The query rule reads `<field>_visible` on the row and admits only `"Public"` rows when the request context's tier is `public`. Users-tier (or sync-bypass) admits every row.
- **D-03:** Mutation rules are NOT in scope here — sync writes go through the bypass; no other writes exist.

### Tier marker propagation
- **D-04:** New `internal/privctx` package exporting:
  - `type Tier int` with `TierPublic` (default zero value) and `TierUsers`
  - `WithTier(ctx context.Context, t Tier) context.Context`
  - `TierFrom(ctx context.Context) Tier`
- **D-05:** New HTTP middleware in `internal/middleware/` reads `PDBPLUS_PUBLIC_TIER` once at startup (cached) and stamps every inbound request's context with the resolved tier via `privctx.WithTier`. Lives early in the chain so all downstream handlers + ent queries see it.
- **D-06:** Privacy policy reads via `privctx.TierFrom(ctx)` and admits Users-visibility rows when `tier == TierUsers`.
- **D-07:** Do NOT use `privacy.DecisionContext` directly for anonymous callers — keeping the typed-tier abstraction lets v1.15 OAuth slot in by setting `TierUsers` from the OAuth callback without touching the policy.

### Sync-worker bypass
- **D-08:** Wrap once at the worker entry. `internal/sync/worker.go` `Sync(ctx)` immediately re-binds `ctx = privacy.DecisionContext(ctx, privacy.Allow)` at the very top before any ent calls. Every downstream upsert + read inherits the bypass via the context.
- **D-09:** The bypass is the SOLE bypass mechanism. Do NOT sprinkle bypasses across upsert helpers — one call site = one audit point.
- **D-10:** Test (D-15 below) asserts the bypass is active for sync-context goroutines and absent for HTTP-handler goroutines.

### Env var parsing
- **D-11:** `PDBPLUS_PUBLIC_TIER` parsed in `internal/config/config.go` with strict validation:
  - Empty / unset → `"public"` (default)
  - `"public"` → `TierPublic`
  - `"users"` → `TierUsers`
  - Anything else → fail-fast at startup with a clear error matching the existing `PDBPLUS_SYNC_MODE` validator pattern
- **D-12:** Case-sensitive lowercase only (matches existing config conventions like `PDBPLUS_SYNC_MODE=full|incremental`).

### Direct-lookup behaviour for filtered rows
- **D-13:** When an anonymous caller does a direct lookup (`/api/poc/{id}`, gRPC `GetPoc`, GraphQL `node(id:)`) for a `Users`-visibility row, each surface returns its native "doesn't exist" idiom:
  - `/api/` → 404 Not Found (PeeringDB-compat surface — matches upstream behaviour)
  - `/rest/v1/` → 404 Not Found
  - `/peeringdb.v1.*` → `connect.CodeNotFound`
  - `/graphql` → `null` + a GraphQL error of category "not found"
  - `/ui/` → existing 404 page
- **D-14:** This must NOT leak the row's existence. Distinguishing "doesn't exist" from "exists but you can't see it" is what an attacker would use to map private rows. Behaviourally equivalent to upstream PeeringDB's anonymous response.

### Verification baked into this phase
- **D-15:** Dedicated test: seed a `visible=Users` POC via the bypassed sync path, then issue an anonymous HTTP request through the full middleware stack. Assert the row is absent from the response. Catches the failure mode where the bypass leaks (e.g. shared ctx accidentally propagating). This test belongs in this phase, not phase 60 — it's the policy's correctness contract.
- **D-16:** Unit tests cover both `PDBPLUS_PUBLIC_TIER` values (`public` and `users`) and the env-var validator's fail-fast behaviour for invalid values.

### Claude's Discretion
- Exact internal naming inside `internal/privctx` (typed key constant naming, function names) — pick consistent with project style
- Where the middleware sits in the chain — likely after Recovery/CORS/OTel/Logging/Readiness/CSP, before Caching/Gzip/mux. Implementation detail; mux must see the stamped context so handlers + ent queries get it.
- Whether to log the resolved tier on every request span (covered in phase 61) or just on the inbound HTTP server span — phase 61 owns this.

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `/home/dotwaffle/.claude/plans/ancient-tumbling-comet.md`
- `.planning/PROJECT.md` §"Current Milestone: v1.14"
- `.planning/REQUIREMENTS.md` — VIS-04, VIS-05, SYNC-03
- `.planning/ROADMAP.md` §"Phase 59: ent Privacy policy + sync bypass"

### Predecessor outputs
- `.planning/phases/58-visibility-schema-alignment/58-CONTEXT.md` — schema decisions; this phase consumes whatever new `*_visible` fields phase 58 added

### ent docs
- https://entgo.io/docs/privacy — official ent privacy package documentation (query/mutation rules, decision context, mixins)

### Existing code that this phase modifies
- `ent/entc.go` — needs the privacy extension enabled (see ent docs above)
- `ent/schema/poc.go` (and any new `*_visible`-bearing entities from phase 58) — gain a `Policy()` method
- `internal/config/config.go` — adds `PublicTier` field and validator
- `internal/sync/worker.go` — wraps ctx with `privacy.DecisionContext` at the top of `Sync`
- `internal/middleware/` — new file for the privacy-tier middleware
- `cmd/peeringdb-plus/main.go` — wires the new middleware into the chain (existing chain documented in CLAUDE.md §"Middleware")

### Project conventions
- `CLAUDE.md` §"Middleware" — chain order: `Recovery -> CORS -> OTel HTTP -> Logging -> Readiness -> CSP -> Caching -> Gzip -> mux`. Privacy middleware needs to land between Logging and Caching so it's logged but applied before any cached response is served.
- `CLAUDE.md` §"Environment Variables" — new vars get added to the env table here (phase 62 owns this update)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/middleware/` — established pattern for net/http middleware; copy a small one (e.g. CSP) as a structural template
- `ent/schema/poc.go` `visible` field already stores the right value — policy just reads it
- `internal/config/config.go` already has the strict-validator pattern from `PDBPLUS_SYNC_MODE` and `PDBPLUS_PEERINGDB_URL` — reuse the helper structure

### Established Patterns
- Functional options for config (env-driven; immutable after parse) — keep
- Sentinel-typed errors over string matching for control flow (GO-ERR-2 in CLAUDE.md) — apply to env-var parse failures
- Tests live alongside (`_test.go`) and use `internal/testutil/seed/Full` — extend `seed.Full` to include a mix of Public + Users POCs (see phase 60 D-01)

### Integration Points
- Privacy middleware reads from `internal/config` (resolved at startup) and writes to request context
- ent privacy policy reads from request context via `internal/privctx`
- Sync worker reads from worker context (no env coupling) — bypass is invariant of `PDBPLUS_PUBLIC_TIER`

</code_context>

<specifics>
## Specific Ideas

- **Don't conflate sync bypass with public-tier override.** Two distinct things: the sync worker always has `privacy.DecisionContext` (allow-all). The `PDBPLUS_PUBLIC_TIER=users` override stamps anonymous *HTTP* contexts with Users tier. Different mechanisms, different concerns, different test coverage. Keep them separated in code.
- **Typed tier > raw `DecisionContext` for HTTP**: future v1.15 OAuth handler sets `privctx.WithTier(ctx, TierUsers)` from the OAuth callback. Same code path as the env override. No special-case in the policy.
- **Direct-lookup 404 not 403** is the security choice — 403 leaks existence.

</specifics>

<deferred>
## Deferred Ideas

- Multiple tier gradations (e.g. `TierAdmin` for some future operator surface) — `Tier` is an `int` so it's extensible; concrete additions wait until a use case appears.
- Per-org or per-network membership-based tier (matching PeeringDB OAuth `networks` scope perms) — depends on v1.15 OAuth landing first.
- Mutation policies — only relevant once we add a write path, which is out of scope at the project level.

</deferred>

---

*Phase: 59-ent-privacy-policy-sync-bypass*
*Context gathered: 2026-04-16*
