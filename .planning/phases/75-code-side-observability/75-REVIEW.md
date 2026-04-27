---
phase: 75-code-side-observability
reviewed: 2026-04-26T23:59:00Z
depth: standard
files_reviewed: 6
files_reviewed_list:
  - internal/sync/initialcounts.go
  - internal/sync/initialcounts_test.go
  - internal/otel/prewarm.go
  - internal/otel/prewarm_test.go
  - cmd/peeringdb-plus/main.go
  - cmd/peeringdb-plus/route_tag_e2e_test.go
findings:
  blocker: 0
  warning: 4
  info: 3
  total: 7
status: issues_found
---

# Phase 75: Code Review Report

**Reviewed:** 2026-04-26T23:59:00Z
**Depth:** standard
**Status:** issues_found

## Summary

Phase 75 ships three observability fixes (OBS-01 cold-start gauges, OBS-02
zero-rate counter pre-warm, OBS-04 http.route doc + E2E lock-in). The code is
small and well-bounded; the implementation correctly follows the locked
decisions in CONTEXT.md (D-01/D-02/D-03), the cardinality math (54 series)
matches what `PrewarmCounters` actually emits, and contracts are
appropriately tested.

No BLOCKERs found — nothing here ships an incorrect behaviour or a security
defect. There are 4 WARNINGs worth fixing before this lands and 3 INFO-level
items recorded for hygiene.

The primary concerns:

- **WR-01** — `TestPrewarmCounters_NoError` omits the
  `t.Setenv("OTEL_METRICS_EXPORTER", "none")` setup that every other test in
  the same package uses; this can either fail test isolation or produce
  unintended OTLP exporter activity inside the test.
- **WR-02** — `InitialObjectCounts` creates 13 closures per call solely to
  thread a name to a Count call — this is a zero-cost runtime pattern but
  obscures the fact that the function does not honour `ctx` cancellation
  between counts (a cancelled ctx will still drive 13 sequential queries
  before the first error surfaces from the SQLite driver). Worth a one-line
  fix.
- **WR-03** — `PrewarmCounters` documents that calling on nil counters
  "panics" but provides zero defensive nil-check; a single misordered call
  in main.go will crash the process at startup with a stack-trace that does
  not point at the ordering invariant.
- **WR-04** — `TestPeeringDBEntityTypes_ParityNote` is a no-op test
  (`t.Log` only); it documents an invariant but enforces nothing. Drift
  detection here relies entirely on `_Cardinality`'s `len() != 13` check,
  which only fires if both lists drift in opposite directions (one grows by
  1, the other shrinks by 1) or only one grows.

## Warnings

### WR-01: TestPrewarmCounters_NoError missing OTEL_METRICS_EXPORTER=none setup

**File:** `internal/otel/prewarm_test.go:8-20`
**Issue:** Every other test in `internal/otel/metrics_test.go` (10
occurrences across `TestInitMetrics_*`, `TestSyncDuration_*`,
`TestSyncOperations_*`) opens with `t.Setenv("OTEL_METRICS_EXPORTER",
"none")` to prevent autoexport from trying to dial a real OTLP endpoint
during `InitMetrics()`. `TestPrewarmCounters_NoError` calls `InitMetrics()`
without that guard. Depending on host envs at test invocation time
(OTEL_EXPORTER_OTLP_ENDPOINT, OTEL_METRICS_EXPORTER), this can:
1. Slow the test by waiting for an exporter handshake.
2. Spam an unintended OTLP receiver with the 54 zero-valued samples.
3. Fail in CI if a default exporter is configured by a runner-level env.

The omission also makes the test inconsistent with the established package
test-setup convention.

**Fix:**
```go
func TestPrewarmCounters_NoError(t *testing.T) {
    t.Setenv("OTEL_METRICS_EXPORTER", "none")
    if err := InitMetrics(); err != nil {
        t.Fatalf("InitMetrics: %v", err)
    }
    PrewarmCounters(context.Background())
}
```

### WR-02: InitialObjectCounts does not check ctx between per-type queries

**File:** `internal/sync/initialcounts.go:78-85`
**Issue:** The for-loop runs all 13 `Count(ctx)` calls in sequence with no
explicit `ctx.Err()` check between iterations. ent's `Count` does honour
ctx cancellation at the SQL driver level, but on a deeply-saturated boot
race (the documented "LiteFS not yet mounted" failure mode) the SQLite
driver may block in syscall before noticing cancellation, particularly on a
FUSE-backed mount that's spinning waiting for the volume to come up.

Result: a `ctx.Done()` signal during startup (e.g. a SIGTERM mid-boot from
Fly's deploy orchestrator killing a stuck instance) currently waits for
all 13 Count calls to complete or fail individually before unwinding.

**Fix:** Cheap defensive ctx-check at loop top:
```go
for _, q := range queries {
    if err := ctx.Err(); err != nil {
        return nil, fmt.Errorf("count %s: %w", q.name, err)
    }
    n, err := q.run(ctx)
    if err != nil {
        return nil, fmt.Errorf("count %s: %w", q.name, err)
    }
    counts[q.name] = int64(n)
}
```

### WR-03: PrewarmCounters has no defensive nil-check on counter vars

**File:** `internal/otel/prewarm.go:53-66`
**Issue:** The doc-comment correctly notes "MUST be called AFTER
InitMetrics() ... calling on nil counters will panic" — but the code
provides no guard. A future refactor that splits InitMetrics into
sub-helpers, or a call-site reorder, will produce a runtime panic with
a stack trace pointing at the otel SDK's Add internals rather than at the
ordering invariant.

The author's stated reason for not guarding (test comment line 16-18:
"if PrewarmCounters panics on nil counters, we WANT the test to fail
loudly") protects test-time detection but leaves production with the same
panic. A nil-check that logs+returns is friendlier to operators without
weakening the test (a missing call to InitMetrics will still surface as
"prewarm skipped: counter SyncTypeFallback is nil" in startup logs).

**Fix:**
```go
func PrewarmCounters(ctx context.Context) {
    if SyncTypeFallback == nil || SyncTypeFetchErrors == nil ||
        SyncTypeUpsertErrors == nil || SyncTypeDeleted == nil ||
        RoleTransitions == nil {
        // Defensive — InitMetrics() must run before this. Caller error;
        // do not panic at startup, surface via OTel error handler so it
        // is grep-able in startup logs.
        otel.Handle(fmt.Errorf("PrewarmCounters: InitMetrics not called"))
        return
    }
    // ... existing loop
}
```

(`otel.Handle` is already used elsewhere in `internal/otel/`; this keeps
the failure observable without crashing the process.)

### WR-04: TestPeeringDBEntityTypes_ParityNote provides zero enforcement

**File:** `internal/otel/prewarm_test.go:50-54`
**Issue:** This test is a `t.Log` with no assertion. The frontmatter SUMMARY
and the test docstring both claim it "records the parity contract" — but
records is the operative word; nothing fails if `PeeringDBEntityTypes`
drifts from `internal/sync/worker.go syncSteps()`. The sibling
`_Cardinality` test only catches drift in the LENGTH of one side, not the
NAMES — and only if exactly one side changes count.

Concrete drift example missed: rename `"campus"` to `"campuses"` in either
file (matching the deferred DEFER-70-06-01 fix in CLAUDE.md). Both tests
still pass; production silently splits the metric series.

**Fix options (pick one):**

a) Delete `_ParityNote` — it adds noise without enforcement, and the
   intent is already covered by the `_Cardinality` set-equality check
   (which DOES compare the actual names against a hardcoded golden
   set). The doc comment can move into `prewarm.go`.

b) Promote enforcement: introduce `internal/pdbtypes` as suggested in
   the prewarm.go docstring and import it from BOTH `internal/sync` and
   `internal/otel`. This is the architectural fix the docstring already
   gestures at. Out of scope for v1.18, but worth a SEED if not done now.

## Info

### IN-01: 14-line closure-table for what could be a 13-line slice of {name,query} pairs

**File:** `internal/sync/initialcounts.go:58-76`
**Issue:** The `type counter struct { name string; run func(context.Context)
(int, error) }` pattern allocates 13 closures per call — fine for a once-
per-process call, slightly noisy to read. ent has no shared `Count(ctx)`
interface so a generic loop isn't possible, but the same readability could
be achieved with explicit consecutive blocks:

```go
if n, err := client.Organization.Query().Count(ctx); err != nil {
    return nil, fmt.Errorf("count org: %w", err)
} else { counts["org"] = int64(n) }
// ... 12 more
```

This is a style preference, not a defect — recording for the
patterns-established line in the SUMMARY ("closure-table dispatch over the
13 entity types") so reviewers know it was a conscious choice.

### IN-02: Doc-comment line-number references will rot

**File:**
- `cmd/peeringdb-plus/main.go:286-287` ("InitMetrics() at line ~96")
- `cmd/peeringdb-plus/main.go:910-913` ("otelhttp@v0.68.0/handler.go:172 ... handler.go:202")
- `internal/otel/prewarm.go:40-49` ("internal/sync/worker.go:1634/1651", "line ~96")
- `internal/sync/initialcounts.go` (no specific line refs — this one is fine)

**Issue:** Five doc-comments anchor to specific line numbers in either
this file (which grows) or third-party code (which version-bumps). The
SUMMARY already calls this pattern out: "Plan-time `os.Exit(1)` count
assertions are stale within ~1-2 milestones — the file grows. Future plans
should drop count-equality acceptance criteria in favour of structural
assertions."

The same lesson applies to doc-comments. Suggest replacing line numbers
with anchor symbols: instead of "line ~96" say "after the
`pdbotel.InitMetrics()` call"; instead of "handler.go:172/202" say "in
otelhttp.Handler.ServeHTTP, install at the LabelerFromContext call and
read at the RecordMetrics call".

### IN-03: TestPrewarmCounters_NoError lacks t.Parallel()

**File:** `internal/otel/prewarm_test.go:8-20`
**Issue:** Per GO-T-3 ("Mark safe tests with t.Parallel()"), this test
mutates global state (`SyncTypeFallback` etc.) via `InitMetrics()`. Other
tests in the package also call `InitMetrics()` and ARE parallel
(`TestInitMetrics_*` runs without `t.Parallel()` either, so this matches
prior art).

The `_Cardinality` and `_ParityNote` siblings DO call `t.Parallel()` —
making one test sequential and two parallel in the same file is mildly
inconsistent. Either mark all three as parallel (safe — each only reads
package-level vars after a sequential `InitMetrics()` call which is
idempotent) or accept the inconsistency.

This is INFO not WARNING because `InitMetrics()` is documented as
re-callable and the mutation is idempotent (each call replaces the same
package-level vars with new instances of the same instruments).

---

_Reviewed: 2026-04-26T23:59:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
