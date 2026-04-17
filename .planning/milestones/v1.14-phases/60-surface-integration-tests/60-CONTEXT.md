# Phase 60: Surface integration + tests - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Prove the privacy policy installed in phase 59 fires correctly through every read surface (`/ui/`, `/graphql`, `/rest/v1/`, `/api/`, `/peeringdb.v1.*`) and that the pdbcompat anonymous shape matches upstream's anonymous shape. Cover both the API-key path (Users rows present in DB, filtered on output) and the no-key path (no Users rows in DB, filter is a no-op).

This phase is verification, not new feature code. Can run in parallel with phase 61 after phase 59 lands.

</domain>

<decisions>
## Implementation Decisions

### Test seed strategy
- **D-01:** Extend `internal/testutil/seed/Full` with a deterministic mix of `visible="Public"` and `visible="Users"` POCs (and any other auth-gated entities from phase 58). Specifically: at least 1 Public POC and 1 Users POC per network that has POCs in the seed set, with stable IDs so per-surface tests can assert exact presence/absence.
- **D-02:** Existing tests that use `seed.Full` and don't care about visibility will continue to pass — privacy filter on Public rows is a no-op for them. Tests that DO care opt in by asserting the row counts they expect.
- **D-03:** Do NOT introduce a separate `seed.FullWithVisibility` helper. Single source of truth keeps drift down; existing tests already exercise everything seed.Full produces.

### Per-surface anonymous-leak tests
- **D-04:** One integration test per surface, each issuing an anonymous request through the full middleware stack (so the privacy-tier middleware actually fires) and asserting:
  - The response contains zero `visible="Users"` rows
  - The response contains the Public rows it should
  - List endpoint counts exactly match the Public-row count for that net/org
- **D-05:** Surface coverage:
  - `/ui/` — handler tests for network detail page assert no Users-tier POC name/email appears in the rendered HTML
  - `/graphql` — query for `pocs` connection on a network, assert nodes count + content match Public-only
  - `/rest/v1/` — list `/rest/v1/pocs` and detail `/rest/v1/pocs/{id}` (the Users-tier ID returns 404)
  - `/api/` — list `/api/poc` and detail `/api/poc/{id}` (Users-tier ID returns 404, matches upstream)
  - `/peeringdb.v1.*` — `ListPocs` and `GetPoc(Users-tier-id)` returning `connect.CodeNotFound`

### gRPC test driver
- **D-06:** Go client against an `httptest.Server`. Spin up the full server with the real middleware chain, dial via the generated `peeringdbv1connect` client, assert response shape. Mirrors how external callers use the API; matches the existing pattern in `internal/grpcserver/*_test.go`.
- **D-07:** Do NOT call handlers directly — that bypasses the privacy-tier middleware which is exactly what we're trying to verify.

### pdbcompat anonymous parity
- **D-08:** Replay phase 57's anon fixtures through an `httptest.Server`:
  - Seed our DB with data structurally equivalent to what the fixture represents
  - Hit `/api/{type}` on the local httptest server
  - Structurally diff the response against the committed VIS-01 anon fixture
  - Empty diff = pass
- **D-09:** Deterministic, no live PeeringDB traffic, no API key needed in CI. Failure case is unambiguous: a leak will show as fields/rows we serve that upstream doesn't.

### `internal/conformance/` updates
- **D-10:** Anon-only conformance. Update the existing live conformance check (gated by `-peeringdb-live`) so our anonymous `/api/{type}` response is compared against upstream's anonymous response for all 13 types. One comparison mode, reuses existing structural diff logic.
- **D-11:** Do NOT add an authenticated conformance mode. CI doesn't have an API key (and shouldn't); this would either skip silently or require fragile secret handling for marginal coverage.

### No-key sync verification (SYNC-02)
- **D-12:** Dedicated integration test in `internal/sync/`:
  - Construct sync worker with `PeeringDBAPIKey=""`
  - Run a sync against a fake PeeringDB serving anonymous-payload fixtures
  - Assert: zero `visible="Users"` POC rows in the resulting DB
  - Assert: anonymous request through the full stack returns the same row set the worker stored (filter is a no-op since there's nothing to filter)
- **D-13:** Test uses fixtures from phase 57's `testdata/visibility-baseline/{beta|prod}/anon/` (already committed) — no need for a separate fixture set.

### Claude's Discretion
- Exact stable IDs for the new visibility-mixed POCs in `seed.Full` (something like 9000-9099 reserved for Users-tier POCs to keep them grep-able)
- Whether per-surface tests live in their existing `_test.go` files or a new `privacy_test.go` per package — implementation detail
- How the `/ui/` test asserts "no Users name appears" — could be string-matching the rendered output, could be a goquery-style HTML parse; either is acceptable

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `/home/dotwaffle/.claude/plans/ancient-tumbling-comet.md`
- `.planning/PROJECT.md` §"Current Milestone: v1.14"
- `.planning/REQUIREMENTS.md` — VIS-06, VIS-07, SYNC-02
- `.planning/ROADMAP.md` §"Phase 60: Surface integration + tests"

### Predecessor outputs (consumed)
- `.planning/phases/59-ent-privacy-policy-sync-bypass/59-CONTEXT.md` — privacy policy + middleware behaviour this phase verifies
- `testdata/visibility-baseline/beta/anon/api/{type}/page-1.json` — phase 57 anon fixtures replayed in D-08

### Existing test infrastructure
- `internal/testutil/seed/seed.go` — `Full(tb, client)` deterministic seed; this phase extends it (per D-01)
- `internal/grpcserver/*_test.go` — established pattern for ConnectRPC client-against-httptest tests (per D-06)
- `internal/conformance/` — existing live PeeringDB comparison; this phase updates the comparison mode (per D-10)
- `internal/web/handler_test.go` — established pattern for `/ui/` handler tests
- `internal/pdbcompat/golden_test.go` — golden file pattern for `/api/` shape; phase 60 adds visibility-aware golden files

### CLAUDE.md test conventions
- `CLAUDE.md` §"Testing" — `SetupClient(t)` for isolated SQLite, `seed.Full` as the public seed API, `-peeringdb-live` flag gating

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `seed.Full` already creates POCs (POC has `visible` field defaulting to "Public") — adding a few `visible=Users` POCs is a small extension
- `httptest.NewServer(handler.Build(...))` is the established pattern across surface tests
- The 13-type conformance loop in `internal/conformance/` already iterates types — extending to anon-vs-anon is mechanical

### Established Patterns
- Tests use `t.Cleanup` for teardown (CLAUDE.md GO-T-2)
- Table-driven tests preferred (GO-T-1) — most surface tests can use this for the (surface, endpoint, expected_count) tuples
- Mark safe tests with `t.Parallel()` (GO-T-3)

### Integration Points
- This phase's tests are the regression-prevention layer for all of v1.14 — once they pass, the privacy floor is provably enforced
- The diff JSON from phase 57 (`testdata/visibility-baseline/diff.json`) becomes the assertion corpus for D-08

</code_context>

<specifics>
## Specific Ideas

- **Extend seed.Full, don't fork it.** A single canonical seed avoids the "which seed does this test use?" cognitive tax that a `seed.FullWithVisibility` would introduce.
- **Real middleware stack in tests.** Direct handler invocation defeats half the test's purpose since the privacy-tier middleware sets the context. Always go through `http.Handler` — `httptest.NewServer` for surface tests, `httptest.NewRecorder` is acceptable for handler-level tests that explicitly construct the middleware-stamped context.
- **No live API key in CI.** Don't add a secret. Anon-only conformance is the explicit boundary.

</specifics>

<deferred>
## Deferred Ideas

- Authenticated conformance with an API-key secret in CI — explicitly rejected; adds blast-radius for marginal coverage.
- E2E browser test (Playwright) of `/ui/` privacy filtering — overkill; the handler test catches the same regression at lower cost.
- Fuzz test for the privacy policy — nice-to-have; the property "any input → no Users row in output" is testable, but the policy is a single line of ent predicate. Defer.

</deferred>

---

*Phase: 60-surface-integration-tests*
*Context gathered: 2026-04-16*
