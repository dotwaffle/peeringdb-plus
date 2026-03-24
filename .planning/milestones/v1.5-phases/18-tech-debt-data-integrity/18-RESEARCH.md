# Phase 18: Tech Debt & Data Integrity - Research

**Researched:** 2026-03-24
**Domain:** Dead code audit, PeeringDB API meta.generated field verification, sync cursor integrity
**Confidence:** HIGH

## Summary

Phase 18 addresses two independent concerns: dead code cleanup and empirical verification of the `meta.generated` field in PeeringDB API responses. Research reveals a critical correction needed in the requirements: DEBT-01 as written ("WorkerConfig.IsPrimary dead field removed") is already resolved by quick task 260324-lc5, which **repurposed** `IsPrimary` from a dead `bool` field to a live `func() bool` field that is actively used throughout the sync scheduler. The field must NOT be removed. The actual DEBT-01 work is updating planning documentation to accurately reflect this change.

Empirical testing against `beta.peeringdb.com` (conducted during this research) definitively answers the meta.generated question: the field is present ONLY on full-fetch cached responses (no `limit`/`skip` parameters). All paginated and incremental requests return `meta: {}`. The existing `parseMeta()` fallback (return zero time) and `fetchIncremental` fallback (`time.Now().Add(-5 * time.Minute)`) handle this correctly. The phase work is documentation and test codification, not bug fixing.

**Primary recommendation:** Update stale planning docs (DEBT-01/DEBT-02 scope correction), write a flag-gated live integration test confirming meta.generated behavior across types/patterns, and document empirical findings in a markdown file alongside the test.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Full audit -- grep for any remaining IsPrimary references across the codebase, clean up test helpers, update all planning docs
- **D-02:** Quick task 260324-lc5 already converted `WorkerConfig.IsPrimary` from `bool` to `func() bool` -- Phase 18 confirms this is complete, removes any stale references, and updates PROJECT.md tech debt section
- **D-03:** DataLoader was already removed in v1.2 Phase 7 -- planning docs must be corrected to reflect this (Pitfall #4 from research)
- **D-04:** Both manual investigation AND automated test -- curl the API first to understand behavior, then codify findings into a flag-gated live integration test (like existing `-peeringdb-live` pattern)
- **D-05:** Test against beta.peeringdb.com (NOT production) for three request patterns: full fetch, paginated incremental, empty result set
- **D-06:** Graceful fallback is already implemented (`parseMeta` returns zero time -> worker uses `started_at - 5min`). Verification confirms this is correct, not that it needs changing.
- **D-07:** Document findings in a markdown file with actual observed response structures

### Claude's Discretion
- Which additional types beyond `net` to test for consistency (recommend at least 3: net, ix, fac)
- Whether to add DEBUG-level logging for meta.generated values in the sync path
- Test file organization (new test file vs. extending existing integration test)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DEBT-01 | WorkerConfig.IsPrimary dead field removed from internal/sync/worker.go | **REINTERPRETATION NEEDED**: Quick task 260324-lc5 already converted IsPrimary from dead `bool` to live `func() bool`. Field is actively used (worker.go:413,444,479; main.go:139). Task is now: confirm conversion complete, update PROJECT.md tech debt section to remove this item, correct Phase 7 summary. |
| DEBT-02 | Planning docs updated to reflect DataLoader already removed in v1.2, only IsPrimary remains | DataLoader confirmed gone from codebase (grep returns 0 Go hits). PROJECT.md line 45 still says "Remove unused DataLoader middleware and WorkerConfig.IsPrimary dead field" -- both are now resolved. Phase 7 summary (07-01-SUMMARY.md) incorrectly claims WorkerConfig.IsPrimary was removed. |
| DATA-01 | meta.generated field behavior verified empirically | **VERIFIED DURING RESEARCH**: Full fetch (no limit/skip) returns `meta.generated` as float64 epoch. All paginated requests (with limit/skip/since) return `meta: {}`. Consistent across net, ix, fac, carrier, campus types. |
| DATA-02 | Graceful fallback confirmed working when meta.generated absent | `parseMeta()` returns zero time on `meta: {}`. `fetchIncremental()` at worker.go:731-733 converts zero to `time.Now().Add(-5*time.Minute)`. Worker Sync() at line 193 stores this as cursor. Fallback chain is correct. |
| DATA-03 | meta.generated findings documented with observed response structures | Research findings ready for documentation. Three patterns verified, five types tested. |
</phase_requirements>

## Critical Finding: DEBT-01 Requirement Reinterpretation

**The REQUIREMENTS.md says:** "WorkerConfig.IsPrimary dead field removed from internal/sync/worker.go"

**Actual codebase state:** Quick task 260324-lc5 (commit `8bd00ac`, 2026-03-24) converted `WorkerConfig.IsPrimary` from `bool` to `func() bool` and wired it to `litefs.IsPrimaryWithFallback()`. The field is now **actively used** in:

| Location | Usage |
|----------|-------|
| `worker.go:30` | Field definition: `IsPrimary func() bool` |
| `worker.go:50-51` | Default: nil -> always-primary |
| `worker.go:413` | Demotion monitor: `!w.config.IsPrimary()` |
| `worker.go:444` | Initial scheduler check: `w.config.IsPrimary()` |
| `worker.go:479` | Per-tick role check: `w.config.IsPrimary()` |
| `main.go:139` | Wired to `isPrimaryFn` (calls `litefs.IsPrimaryWithFallback`) |
| `worker_test.go:1283-1403` | 3 tests using `primarySwitch.IsPrimary` |

**What DEBT-01 actually means for Phase 18:** The "dead field" tech debt is already resolved -- it was resolved by the quick task. The remaining work is:
1. Update PROJECT.md to remove "WorkerConfig.IsPrimary dead field" from the tech debt list
2. Correct Phase 7 summary (07-01-SUMMARY.md line 63) which incorrectly claims "Removed WorkerConfig.IsPrimary field"
3. Update REQUIREMENTS.md traceability to mark DEBT-01 as satisfied by quick task 260324-lc5

**Confidence:** HIGH -- verified by grepping the entire codebase. IsPrimary has 30+ active references in Go source and tests.

## Empirical meta.generated Findings

### Test Methodology

Tested against `beta.peeringdb.com` on 2026-03-24 using curl. Three request patterns tested across 5 PeeringDB types (net, ix, fac, carrier, campus).

### Pattern 1: Full Fetch (no limit/skip)

**Request:** `GET /api/{type}?depth=0`

**Result:** `meta.generated` IS PRESENT as a float64 Unix epoch.

| Type | meta.generated | data count |
|------|---------------|------------|
| net | 1774328452.459 | 34,235 |
| ix | 1774328965.690 | 1,302 |
| fac | 1774329027.392 | 5,855 |
| carrier | 1774329027.782 | 274 |
| campus | 1774329289.908 | 74 |
| org | 1774328350.627 | 33,248 |

**Interpretation:** These are cached responses. The `generated` timestamp is when the cache was built. Different types have different cache generation times (they are cached independently). The timestamp is a float64 with sub-second precision.

### Pattern 2: Paginated Incremental (with limit/skip/since)

**Request:** `GET /api/{type}?depth=0&limit=250&skip=0&since={unix_ts}`

**Result:** `meta.generated` is ABSENT. `meta` is an empty object `{}`.

| Type | since | meta | data count |
|------|-------|------|------------|
| net | 7 days ago | `{}` | 250 |
| ix | 7 days ago | `{}` | 18 |
| fac | 7 days ago | `{}` | 22 |

**Also tested:** `limit=` without `since=` also returns `meta: {}`. The presence of `limit` or `skip` parameters triggers the non-cached code path.

### Pattern 3: Empty Result Set

**Request:** `GET /api/{type}?depth=0&since={future_timestamp}` or very recent timestamp

**Result:** `meta: {}`, `data: []`

### Summary of Behavior

| Request Pattern | `meta.generated` | Response Source |
|-----------------|-------------------|-----------------|
| `?depth=0` (no limit/skip) | PRESENT (float64 epoch) | PeeringDB cache layer |
| `?depth=0&limit=N&skip=M` | ABSENT (`meta: {}`) | Live database query |
| `?depth=0&limit=N&skip=M&since=T` | ABSENT (`meta: {}`) | Live database query |
| `?depth=0&since=T` (no limit) | ABSENT (`meta: {}`) | Live database query |

**Key insight:** `meta.generated` is a cache artifact. Only full-dataset responses served from cache include it. Any parameterized query (limit, skip, since) bypasses the cache and returns an empty meta object.

### Impact on Sync Pipeline

**Full sync path** (worker.go lines 138-174): Uses `?depth=0` without limit/skip. `parseMeta()` correctly extracts the generated timestamp. This timestamp is stored as the sync cursor for subsequent incremental syncs.

**Incremental sync path** (worker.go lines 180-231 via `fetchIncremental`): Uses `?depth=0&limit=250&skip=0&since=cursor`. `parseMeta()` returns zero time for every page. `fetchIncremental()` at line 731-733 detects zero and substitutes `time.Now().Add(-5 * time.Minute)`:

```go
generated := result.Meta.Generated
if generated.IsZero() {
    generated = time.Now().Add(-5 * time.Minute)
}
```

This 5-minute buffer is safe: it overlaps with the most recent sync window, ensuring no objects are missed between sync cycles.

**Conclusion:** The existing code handles all three patterns correctly. The fallback behavior is sound. No code changes are needed -- only documentation and test codification.

**Confidence:** HIGH -- empirically verified against the live beta API.

## Architecture Patterns

### Existing Code Structure (No Changes Needed)

```
internal/peeringdb/
  client.go        -- parseMeta(), FetchAll, FetchMeta, FetchResult
  client_test.go   -- TestFetchMetaParsing, TestFetchMetaMissing, TestFetchMetaEarliestAcrossPages
  types.go         -- Response[T], type constants

internal/sync/
  worker.go        -- WorkerConfig (IsPrimary func() bool), fetchIncremental fallback
  worker_test.go   -- primarySwitch helper, scheduler tests
  status.go        -- GetCursor, UpsertCursor, sync_status/sync_cursors tables
  integration_test.go

internal/conformance/
  live_test.go     -- flag-gated live test pattern (-peeringdb-live)
```

### Pattern: Flag-Gated Live Integration Test

The project uses a `-peeringdb-live` flag to gate tests that hit the real PeeringDB API:

```go
var peeringdbLive = flag.Bool("peeringdb-live", false, "run live conformance tests against beta.peeringdb.com")

func TestLiveConformance(t *testing.T) {
    if !*peeringdbLive {
        t.Skip("skipping live conformance test (use -peeringdb-live to enable)")
    }
    // ... test body
}
```

Source: `internal/conformance/live_test.go`

The meta.generated live test should follow this exact pattern. It belongs in `internal/peeringdb/client_live_test.go` (new file) since the behavior being verified is about the PeeringDB client's response parsing, not the conformance layer.

### Pattern: Table-Driven Tests with t.Parallel (T-1, T-3)

All existing client tests use parallel table-driven patterns. The meta.generated documentation test can use subtests for each request pattern and type combination.

### Anti-Patterns to Avoid

- **Do NOT modify parseMeta() or fetchIncremental()**: These are working correctly. The phase confirms behavior, not changes behavior.
- **Do NOT remove WorkerConfig.IsPrimary**: It is actively used. The "dead field" was already resolved by quick task 260324-lc5.
- **Do NOT test against production (www.peeringdb.com)**: Use beta.peeringdb.com per D-05.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Live API testing | Custom HTTP client for testing | Existing `net/http` client + `-peeringdb-live` flag pattern from conformance | Pattern is established, battle-tested, and rate-limit aware |
| PeeringDB rate limiting in tests | Manual sleep between requests | Sequential subtests with configurable delay (3s unauthenticated, 1s authenticated per conformance/live_test.go) | Existing pattern handles auth/unauth rate limit differences |

## Common Pitfalls

### Pitfall 1: Treating IsPrimary as Dead Code

**What goes wrong:** REQUIREMENTS.md and PROJECT.md say "WorkerConfig.IsPrimary dead field removed" but quick task 260324-lc5 converted it to an active `func() bool` field. Attempting to remove it breaks the sync scheduler, demotion monitoring, and 3 tests.
**Why it happens:** Planning docs were written before the quick task executed. The quick task summary says "removes dead field DEBT-01" but actually repurposed the field.
**How to avoid:** Grep the codebase before any removal. The research has already verified the current state.
**Warning signs:** `go build` fails, 3+ tests fail in `worker_test.go`.

### Pitfall 2: Expecting meta.generated on Incremental Sync Responses

**What goes wrong:** Assuming `meta.generated` will be present on paginated incremental responses and writing test assertions that require it.
**Why it happens:** GitHub issue #776 shows `meta.generated` in cached responses. The assumption that all responses include it is natural but wrong.
**How to avoid:** The empirical testing done in this research shows it is ONLY on full-fetch cached responses. Tests for incremental paths should assert absence, not presence.
**Warning signs:** Live test failures on paginated request assertions.

### Pitfall 3: Stale Planning Docs Create Confusion

**What goes wrong:** Phase 7 summary (07-01-SUMMARY.md) claims "Removed WorkerConfig.IsPrimary field" but it was not actually removed from `worker.go`. PROJECT.md still lists both DataLoader and IsPrimary as active tech debt. A developer trusting these docs wastes time or makes incorrect changes.
**Why it happens:** Phase 7 plan called for removing IsPrimary from both Config and WorkerConfig. It was removed from Config but not WorkerConfig. The summary reported both as done.
**How to avoid:** This phase corrects the documentation. The list of docs to update is enumerated below.

### Pitfall 4: Testing Against Production PeeringDB

**What goes wrong:** Tests accidentally hit `www.peeringdb.com` instead of `beta.peeringdb.com`, potentially violating rate limits or causing the API key to be flagged.
**Why it happens:** Default base URL in the client is production.
**How to avoid:** Hard-code `beta.peeringdb.com` in the live test, per D-05.
**Warning signs:** Test logs show `www.peeringdb.com` in URLs.

## Planning Documentation Audit

Files that need correction to reflect the actual codebase state:

| File | Current Content (Incorrect) | Corrected Content |
|------|---------------------------|-------------------|
| `.planning/PROJECT.md:45` | "Remove unused DataLoader middleware and WorkerConfig.IsPrimary dead field" | Mark as completed: DataLoader removed v1.2 Phase 7, IsPrimary converted to live `func() bool` by quick task 260324-lc5 |
| `.planning/PROJECT.md:133` | "Remove unused DataLoader middleware and WorkerConfig.IsPrimary dead field" | Same correction |
| `.planning/PROJECT.md:143` | "WorkerConfig.IsPrimary dead field (replaced by LiteFS detection, explicitly deferred)" | "WorkerConfig.IsPrimary converted from dead `bool` to live `func() bool` by quick task 260324-lc5; dynamic LiteFS detection now active" |
| `.planning/milestones/v1.2-phases/07-lint-code-quality/07-01-SUMMARY.md:63` | "Removed all dead code: ... WorkerConfig.IsPrimary field" | "Removed dead code: ... config.IsPrimary field (WorkerConfig.IsPrimary was NOT removed)" |
| `.planning/milestones/v1.2-phases/07-lint-code-quality/07-01-SUMMARY.md:81` | "internal/sync/worker.go - Removed IsPrimary from WorkerConfig struct" | "internal/sync/worker.go - IsPrimary was NOT removed from WorkerConfig (only config.IsPrimary in config.go was removed)" |

## Code Examples

### meta.generated Live Test (Recommended Pattern)

```go
// Source: Based on internal/conformance/live_test.go pattern

var peeringdbLive = flag.Bool("peeringdb-live", false,
    "run live meta.generated tests against beta.peeringdb.com")

func TestMetaGeneratedLive(t *testing.T) {
    if !*peeringdbLive {
        t.Skip("skipping live test (use -peeringdb-live to enable)")
    }

    client := &http.Client{Timeout: 30 * time.Second}

    // Pattern 1: Full fetch - meta.generated PRESENT
    t.Run("full_fetch", func(t *testing.T) {
        for _, typeName := range []string{"net", "ix", "fac"} {
            t.Run(typeName, func(t *testing.T) {
                url := fmt.Sprintf("https://beta.peeringdb.com/api/%s?depth=0", typeName)
                // ... fetch, parse meta, assert generated is non-zero float64
            })
        }
    })

    // Pattern 2: Paginated incremental - meta.generated ABSENT
    t.Run("paginated_incremental", func(t *testing.T) {
        since := time.Now().Add(-7 * 24 * time.Hour).Unix()
        url := fmt.Sprintf("https://beta.peeringdb.com/api/net?depth=0&limit=250&skip=0&since=%d", since)
        // ... fetch, parse meta, assert generated is absent
    })

    // Pattern 3: Empty result set - meta.generated ABSENT
    t.Run("empty_result", func(t *testing.T) {
        since := time.Now().Add(24 * time.Hour).Unix() // future
        url := fmt.Sprintf("https://beta.peeringdb.com/api/net?depth=0&since=%d", since)
        // ... fetch, parse meta, assert meta is {}
    })
}
```

### Documentation File (Recommended Structure)

```markdown
# meta.generated Field Behavior

## Summary
The `meta.generated` field in PeeringDB API responses is a cache artifact...

## Observed Response Structures
### Full Fetch (cached)
{json example with meta.generated present}

### Paginated Request (live query)
{json example with meta: {}}

### Empty Result Set
{json example with meta: {}, data: []}

## Impact on Sync Pipeline
...
```

## Discretionary Recommendations

### Types to Test (Claude's Discretion)

Recommend testing **5 types**: `net`, `ix`, `fac`, `org`, `carrier`. Rationale:
- `net` is the largest dataset (34K objects) -- tests the common path
- `ix` and `fac` are the next most important for the application's primary use case (peering)
- `org` is the parent type in FK hierarchy -- tests cache behavior for parent objects
- `carrier` is a small dataset -- tests that meta.generated is consistent regardless of size

5 types covers the spectrum. Testing all 13 would take 40+ seconds with rate limiting (3s per request unauthenticated) and provides diminishing returns since the behavior is consistent.

### DEBUG Logging for meta.generated (Claude's Discretion)

**Recommend: Yes, add DEBUG-level logging.** Add a single slog.Debug call in `fetchIncremental()` after line 731:

```go
if generated.IsZero() {
    generated = time.Now().Add(-5 * time.Minute)
    // LOG: helps operators confirm fallback is triggering as expected
}
```

This aligns with OBS-1 (structured logging) and OBS-5 (attribute setters). The log would fire on every incremental sync step (13 types per cycle), which is acceptable at DEBUG level.

### Test File Organization (Claude's Discretion)

**Recommend: New file `internal/peeringdb/client_live_test.go`.** Rationale:
- Separates live API tests from unit tests (clear naming convention)
- `client_test.go` has 1000+ lines of httptest-based unit tests -- mixing live tests would hurt readability
- Follows the `internal/conformance/live_test.go` precedent of keeping live tests in dedicated files
- The `-peeringdb-live` flag is already defined in `internal/conformance/` -- can be reused or independently defined per package

## Environment Availability

Step 2.6: SKIPPED (no external dependencies identified). This phase is code/config-only changes plus documentation. The live API testing uses `curl`/Go's `net/http` against `beta.peeringdb.com` which is already whitelisted in the sandbox.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing stdlib (Go 1.26) |
| Config file | None needed -- uses `go test` flags |
| Quick run command | `go test ./internal/peeringdb/ -run TestMetaGenerated -v` |
| Full suite command | `go test -race ./...` |

### Phase Requirements -> Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DEBT-01 | IsPrimary conversion confirmed, planning docs updated | manual verification | `grep -rn "IsPrimary" --include="*.go" \| wc -l` (expect 30+) | N/A (doc task) |
| DEBT-02 | Planning docs accurately reflect DataLoader removal and IsPrimary conversion | manual verification | Visual inspection of corrected docs | N/A (doc task) |
| DATA-01 | meta.generated behavior verified for 3 patterns x 3+ types | integration (live) | `go test ./internal/peeringdb/ -run TestMetaGeneratedLive -peeringdb-live -v` | Wave 0 |
| DATA-02 | parseMeta returns zero time on empty meta; fetchIncremental falls back to now-5min | unit | `go test ./internal/peeringdb/ -run TestFetchMetaMissing -v` | Exists (client_test.go:948) |
| DATA-03 | Findings documented with observed response structures | manual verification | File existence check | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test -race ./internal/peeringdb/ ./internal/sync/ -v`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `internal/peeringdb/client_live_test.go` -- covers DATA-01 (live meta.generated verification)
- [ ] `docs/meta-generated-behavior.md` -- covers DATA-03 (documented findings)

Existing test infrastructure covers DATA-02 (`TestFetchMetaMissing` at client_test.go:948). No framework install needed.

## Sources

### Primary (HIGH confidence)
- **Empirical testing** -- `beta.peeringdb.com` API probed 2026-03-24 for all three request patterns across 5+ types
- **Codebase verification** -- `grep -rn "IsPrimary" --include="*.go"` confirms 30+ active references
- `internal/peeringdb/client.go` -- `parseMeta()` implementation (lines 108-119), `FetchAll()` (lines 125-232)
- `internal/sync/worker.go` -- `fetchIncremental()` fallback (lines 731-733), `WorkerConfig.IsPrimary` usage (lines 30, 413, 444, 479)
- `internal/conformance/live_test.go` -- flag-gated live test pattern
- `internal/peeringdb/client_test.go` -- existing `TestFetchMetaParsing`, `TestFetchMetaMissing`, `TestFetchMetaEarliestAcrossPages`

### Secondary (MEDIUM confidence)
- `.planning/research/PITFALLS.md` -- Pitfall #3 (meta.generated undocumented), Pitfall #4 (stale planning docs)
- `.planning/research/FEATURES.md` -- meta.generated field analysis section
- Quick task 260324-lc5 summary -- IsPrimary conversion details

### Tertiary (LOW confidence)
- [PeeringDB Issue #776](https://github.com/peeringdb/peeringdb/issues/776) -- Historical evidence of `meta.generated` field, but does not document when it appears vs. when it is absent

## Project Constraints (from CLAUDE.md)

Relevant directives for this phase:

| Directive | Relevance |
|-----------|-----------|
| T-1 (MUST) | Table-driven tests for meta.generated live test |
| T-3 (SHOULD) | Mark safe tests with `t.Parallel()` -- live tests should NOT be parallel (rate limiting) |
| OBS-1 (MUST) | Structured slog logging for any DEBUG log additions |
| OBS-5 (SHOULD) | Use slog attribute setters (`slog.String()`, `slog.Time()`) |
| ERR-1 (MUST) | Wrap errors with `%w` and context in any new code |
| CS-0 (MUST) | Modern Go code guidelines |
| API-1 (MUST) | Document exported items |

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- no new dependencies, all existing code
- Architecture: HIGH -- no structural changes, only documentation and one test file
- Pitfalls: HIGH -- all four pitfalls verified against codebase and live API
- meta.generated behavior: HIGH -- empirically verified against beta.peeringdb.com

**Research date:** 2026-03-24
**Valid until:** 2026-04-24 (PeeringDB API behavior is stable; cache implementation unlikely to change without notice)

---
*Phase: 18-tech-debt-data-integrity*
*Research completed: 2026-03-24*
