# Phase 57: Visibility baseline capture - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning

<domain>
## Phase Boundary

Produce a committed empirical baseline showing which fields/rows differ between unauthenticated and authenticated PeeringDB API responses across all 13 types, and emit a machine + human readable diff report. Without this baseline the privacy filter (phase 59) cannot be scoped correctly.

This phase is rate-limit bound (≤ 20 anon req/min, ≤ 40 auth req/min upstream); ≥ 1 hour wall-clock for the beta walk is intrinsic, not a planning estimate.

</domain>

<decisions>
## Implementation Decisions

### Capture tool
- **D-01:** Extend `cmd/pdbcompat-check/` with a new `--capture` mode (NOT a separate binary). The existing tool is already auth-aware, already walks all 13 types, already shares `internal/peeringdb` client wiring. Adds the lowest delta and reuses the established rate-limit + auth scaffolding.
- **D-02:** **HARD CONSTRAINT — privacy in the repo:** the captured PeeringDB data must NOT be committed in raw form. Only structural / schema-level information may be checked in. Specifically: never commit personal data (email, phone, names, addresses) from authenticated responses, and never commit large raw blobs of upstream data even from anon responses.

### Capture scope
- **D-03:** Walk **all 13 PeeringDB types**, both auth modes, **first 2 pages each**. Total ≈ 52 anon + 52 auth requests. Inside both rate ceilings with safety margin. Catches per-type visibility behaviour without burning the full upstream budget.
- **D-04:** Run against **`beta.peeringdb.com` first**, then a confirmation pass against **`www.peeringdb.com`** for the high-signal types (`poc`, `org`, `net`) only. Mirrors the milestone's resolved baseline-capture decision.

### Fixture layout
- **D-05:** **Anonymous fixtures**: raw upstream JSON, mirror PeeringDB URL paths under `testdata/visibility-baseline/{beta|prod}/anon/api/{type}/page-1.json`. These came back from a public API — committing the raw bytes is acceptable.
- **D-06:** **Authenticated fixtures**: redacted before commit. Keep field names, types, row IDs, `visible` value, counts, and the structural shape. Replace any string field value that is absent from the corresponding anon response with the placeholder `"<auth-only:string>"` (or `"<auth-only:int>"` etc. by type). Never commit raw email/phone/name strings sourced from auth responses.
- **D-07:** Layout: `testdata/visibility-baseline/{beta|prod}/auth/api/{type}/page-1.json` (redacted form). Keep the raw auth bytes locally only — gitignore the un-redacted directory or write only to `/tmp` during a redact step.

### Diff report
- **D-08:** Two artifacts kept in sync: per-type Markdown table (`testdata/visibility-baseline/DIFF.md`) for code review + machine-readable JSON (`testdata/visibility-baseline/diff.json`) for downstream tests in phase 60.
- **D-09:** The diff report describes deltas in terms of field names + row counts + `visible` values + (for redacted fields) the placeholder type — never any real values. The Markdown table is the human-reviewable artifact; the JSON is the test fixture.

### Run mechanics
- **D-10:** Rate-limit pacing: **dynamic via `Retry-After` + exponential backoff**, NOT hardcoded sleeps. Adapt to upstream's actual signalling (matches the existing `internal/peeringdb` client behaviour added in v1.13). Document the conservative starting interval in code (e.g. start at the existing client cadence, back off on 429).
- **D-11:** Resumability: **checkpoint state file in `/tmp`** (`/tmp/pdb-vis-capture-state.json`). After each completed `(target, mode, type, page)` tuple, write the state. On startup, if a state file is present, ask the operator: `Resume / Restart`. Survives Ctrl+C, crashes, machine sleep.

### Claude's Discretion
- Exact placeholder string format for redacted fields (e.g. `<auth-only:string>` vs `[REDACTED:string]`) — pick one, keep it consistent.
- Whether the `--capture` subcommand re-uses pdbcompat-check's existing CLI plumbing or adds a small switch — implementation detail.
- Whether to log per-request progress to stderr or a progress bar — operator UX, no privacy impact.

### Folded Todos
None.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `/home/dotwaffle/.claude/plans/ancient-tumbling-comet.md` — milestone planning session that produced this phase
- `.planning/PROJECT.md` §"Current Milestone: v1.14" — milestone goals + key context including rate-limit corrections
- `.planning/REQUIREMENTS.md` §"v1.14 Requirements / Visibility (VIS)" — VIS-01, VIS-02 wording
- `.planning/ROADMAP.md` §"Phase 57: Visibility baseline capture" — success criteria

### Existing code to extend
- `cmd/pdbcompat-check/main.go` — current entrypoint, already accepts `-api-key` flag and sets `Authorization: Api-Key {key}`
- `internal/peeringdb/client.go:90-135` — `Client`, `NewClient`, `WithAPIKey`, rate limiter setup
- `internal/peeringdb/client.go:240-310` — request loop including the `Retry-After` handling added in v1.13

### PeeringDB API references
- https://docs.peeringdb.com/howto/work_within_peeringdbs_query_limits/ — rate limits (20/min anon, 40/min auth)
- https://docs.peeringdb.com/blog/contacts_marked_private/ — POC visibility model (Public/Users only since 2.30.0)

### Testdata layout
- `testdata/fixtures/` — existing fixture pattern for the 13 PeeringDB types (sync-side test fixtures, separate from this phase's visibility-baseline fixtures)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `cmd/pdbcompat-check/` — already walks all 13 types with auth support; the `--capture` subcommand slots in alongside the existing checking flow
- `internal/peeringdb.Client` — already has rate-limit + Retry-After handling via `RateLimitError` typed error (added in v1.13); `--capture` mode should consume the same client and let it manage pacing
- The 13-type list is canonically defined in the existing pdbcompat-check + sync code — reuse, don't redefine

### Established Patterns
- Functional options on `NewClient` (`WithAPIKey`) — extend if needed but unlikely necessary
- Error types over sentinel comparisons (`errors.As(err, &RateLimitError{})`) — match this style for any new error types
- Fixtures live under `testdata/`; don't sprinkle elsewhere

### Integration Points
- The diff JSON output (`testdata/visibility-baseline/diff.json`) becomes phase 60's test fixture for VIS-07 parity checks
- The Markdown table feeds phase 58's "are there fields beyond `poc.visible` we need to add to ent schemas?" decision

</code_context>

<specifics>
## Specific Ideas

- **Redaction is non-negotiable.** The whole point of this phase is to discover *what* upstream withholds anonymously, not to mirror the withheld data into our repo. Redaction logic must be defensive — if in doubt about a field, redact.
- **Capture-only-when-asked.** `--capture` should be opt-in (subcommand or flag). The default `pdbcompat-check` behaviour stays exactly as it is so existing users aren't surprised.
- **Single-shot, not periodic.** This is not a sync — operators run it manually when they want a fresh baseline. CI does not run it (no API key in CI; rate limits would burn).

</specifics>

<deferred>
## Deferred Ideas

- Live drift detection (re-run capture on a schedule, alert on diff change) — interesting but belongs in v1.15+ once OAuth exists and the diff schema is stable.
- Historical archive of past baselines to track upstream visibility drift — out of scope; git history of the committed Markdown/JSON gives the same signal cheaply.
- Multi-region capture (run from different IPs to confirm no IP-based rate variation) — unnecessary; the published rate limits are documented as per-IP/per-key, not per-region.

</deferred>

---

*Phase: 57-visibility-baseline-capture*
*Context gathered: 2026-04-16*
