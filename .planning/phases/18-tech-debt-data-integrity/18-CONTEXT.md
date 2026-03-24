# Phase 18: Tech Debt & Data Integrity - Context

**Gathered:** 2026-03-24
**Status:** Ready for planning

<domain>
## Phase Boundary

Remove remaining dead code (WorkerConfig.IsPrimary references, stale planning docs), verify meta.generated field behavior empirically against the live PeeringDB API, implement graceful fallback, and document findings.

</domain>

<decisions>
## Implementation Decisions

### Dead Code Cleanup (DEBT-01, DEBT-02)
- **D-01:** Full audit — grep for any remaining IsPrimary references across the codebase, clean up test helpers, update all planning docs
- **D-02:** Quick task 260324-lc5 already converted `WorkerConfig.IsPrimary` from `bool` to `func() bool` — Phase 18 confirms this is complete, removes any stale references, and updates PROJECT.md tech debt section
- **D-03:** DataLoader was already removed in v1.2 Phase 7 — planning docs must be corrected to reflect this (Pitfall #4 from research)

### meta.generated Verification (DATA-01, DATA-02, DATA-03)
- **D-04:** Both manual investigation AND automated test — curl the API first to understand behavior, then codify findings into a flag-gated live integration test (like existing `-peeringdb-live` pattern)
- **D-05:** Test against beta.peeringdb.com (NOT production) for three request patterns:
  1. Full fetch: `GET /api/net?depth=0` (no limit/skip)
  2. Paginated incremental: `GET /api/net?depth=0&limit=250&skip=0&since=...`
  3. Empty result set: `GET /api/net?depth=0&since=<recent_timestamp>`
- **D-06:** Graceful fallback is already implemented (`parseMeta` returns zero time → worker uses `started_at - 5min`). Verification confirms this is correct, not that it needs changing.
- **D-07:** Document findings in a markdown file with actual observed response structures

### Claude's Discretion
- Which additional types beyond `net` to test for consistency (recommend at least 3: net, ix, fac)
- Whether to add DEBUG-level logging for meta.generated values in the sync path
- Test file organization (new test file vs. extending existing integration test)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Sync Pipeline
- `internal/peeringdb/client.go` — `parseMeta()`, `FetchMeta`, `FetchResult` types
- `internal/sync/worker.go` — `WorkerConfig`, `StartScheduler`, sync cursor logic
- `internal/peeringdb/client_test.go` — Existing `TestFetchMetaParsing`, `TestFetchMetaMissing`, `TestFetchMetaEarliestAcrossPages`

### Dead Code
- `internal/sync/worker.go:30` — `IsPrimary func() bool` (already changed by quick task)
- `.planning/milestones/v1.2-phases/07-lint-code-quality/07-01-SUMMARY.md` — Claims IsPrimary removed (incorrect per Pitfall #4)

### Research
- `.planning/research/PITFALLS.md` — Pitfall #3 (meta.generated undocumented), Pitfall #4 (stale planning docs)
- `.planning/research/FEATURES.md` — meta.generated field analysis section

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `parseMeta()` in `client.go:108` — already handles absence gracefully with zero-time return
- `-peeringdb-live` flag pattern in `internal/conformance/` — reuse for meta.generated live test
- `internal/testutil/` — shared test helpers for ent client + DB setup

### Established Patterns
- Flag-gated live tests: `if !*flagLive { t.Skip("requires -peeringdb-live") }`
- Table-driven tests with `t.Parallel()` per T-1, T-3
- Structured slog logging with attribute setters per OBS-1, OBS-5

### Integration Points
- `internal/peeringdb/client_test.go` — where meta.generated tests should live
- `internal/sync/worker.go` — where cursor uses `FetchMeta.Generated`

</code_context>

<specifics>
## Specific Ideas

- PeeringDB API behavior for meta.generated is undocumented (GitHub issue #776 only shows it exists in cached responses)
- The field is a float64 Unix epoch (e.g., `1595250699.701`) — `parseMeta` truncates to integer seconds
- Current code is already defensively written — this phase is about confirming behavior, not finding bugs

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 18-tech-debt-data-integrity*
*Context gathered: 2026-03-24*
