# Phase 66: Observability + sqlite3 tooling - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Small follow-up catchall. Two related observability improvements:

1. **Heap threshold monitoring.** Make SEED-001's "peak heap >380 MiB → incremental sync worth considering" trigger observable via both OTel span attributes and `slog.Warn` lines. Grafana dashboard gets a heap panel with a threshold line.
2. **sqlite3 in prod image.** Small Dockerfile addition for incident-response debugging via `fly ssh console -C sqlite3`. Moved to a standalone quick task (per decision) so it lands before Phase 65.

The two items are logically grouped because both are about operator visibility / debuggability. Both keep the door shut on SEED-001 by making its trigger condition monitor-able without acting on it.

</domain>

<decisions>
## Implementation Decisions

### sqlite3 placement
- **D-01: Standalone quick task pre-Phase 65.** The Dockerfile addition (`apk add sqlite` or Chainguard `cgr.dev/chainguard/sqlite` equivalent) deploys independently so sqlite3 is available during Phase 65's fleet migration if something goes sideways. Phase 66 inherits the fact that sqlite3 is already in the image.

### Heap threshold surfacing
- **D-02: Both OTel span attribute AND slog.Warn.** When peak heap crosses threshold, emit `pdbplus.sync.peak_heap_mib` + `pdbplus.sync.peak_rss_mib` attributes on the sync-cycle span (already one per sync) AND a `slog.Warn("heap threshold crossed", ...)` line. Dashboard sees timeseries; log pipelines see alerts.

### Threshold metrics
- **D-03: Both heap AND RSS, configurable defaults.** Two env vars:
  - `PDBPLUS_HEAP_WARN_MIB` (default `400`) — Go runtime heap (`runtime.MemStats.HeapInuse`) threshold
  - `PDBPLUS_RSS_WARN_MIB` (default `384`) — OS RSS (`/proc/self/status VmHWM` or equivalent) threshold
- **D-04:** Defaults chosen to match Fly's 512 MB ceiling minus a small safety margin. User chose these values over more conservative options (earlier warnings) — accepts higher confidence that alerts are actionable rather than noisy.

### Dashboard updates
- **D-05: In-phase dashboard edit.** `grafana/pdbplus-overview.json` (in-repo) gets a new heap + RSS panel with the threshold lines rendered. Phase 66 produces the single JSON diff. Rollout is via Grafana config-push or manual reimport.
- **D-06:** Dashboard also gets a panel showing process-group breakdown (primary vs replicas) post-Phase-65 — picks up the Phase 65 asymmetric fleet change visually.

### Documentation
- **D-07: SEED-001 escalation path documented.** `CLAUDE.md` §"Sync" and `docs/DEPLOYMENT.md` both get a short note: "if peak heap sustained >`PDBPLUS_HEAP_WARN_MIB`, SEED-001 (incremental sync evaluation) trigger has fired — surface at next milestone." User-observable.
- **D-08:** Dashboard update documented in `docs/DEPLOYMENT.md` with a screenshot link (if we take one) or just a panel name reference.

### Implementation details
- **D-09:** Peak heap sampling happens at the end of each sync cycle (in `internal/sync/worker.go`'s existing span emitter). No periodic background sampler needed — sync cycle frequency (default 1h) is the right granularity for this signal.
- **D-10:** RSS read from `/proc/self/status` VmHWM line — same pattern used during the manual investigation on 2026-04-17. On non-Linux (tests, local dev), fall back to `runtime.MemStats.Sys` as a proxy or skip the RSS reading.

### Claude's Discretion
- Exact span attribute names — proposed `pdbplus.sync.peak_heap_mib` and `pdbplus.sync.peak_rss_mib` following v1.14's `pdbplus.privacy.tier` naming convention
- Whether the slog.Warn fires every cycle the threshold is crossed or only on the first crossing (suggest: every cycle — noise is proportional to severity)
- Dashboard panel styling (timeseries vs gauge) — implementation detail

### Folded Todos
- `sqlite3` quick task runs before Phase 65 — owned by v1.15 scope but not a phase deliverable.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `.planning/REQUIREMENTS.md` — OBS-04 (sqlite3 tooling), OBS-05 (heap threshold monitoring), DOC-04 (heap watch documentation)
- `.planning/ROADMAP.md` §"Phase 66" — success criteria
- `.planning/seeds/SEED-001-incremental-sync-evaluation.md` — trigger conditions this phase makes observable

### Existing code this phase modifies
- `internal/config/config.go` — two new env vars: `PDBPLUS_HEAP_WARN_MIB` (default 400), `PDBPLUS_RSS_WARN_MIB` (default 384). Both uints; validated non-negative. Follows `PublicTier`/`parsePublicTier` pattern from Phase 59.
- `internal/sync/worker.go` — end-of-sync-cycle logic: read heap + RSS, emit span attrs, warn-if-over-threshold
- `Dockerfile.prod` — add sqlite3 binary (quick task, pre-Phase-65)
- `grafana/pdbplus-overview.json` — new heap+RSS+process-group panels
- `docs/DEPLOYMENT.md`, `CLAUDE.md` — escalation docs

### v1.14 Phase 61 precedent
- Startup `slog.Info("sync mode", ...)` pattern is the template for the new `slog.Warn("heap threshold crossed", ...)` line
- `pdbplus.privacy.tier` OTel attribute precedent — set on a span via `trace.SpanFromContext(ctx).SetAttributes(...)` pattern

### Project conventions
- `CLAUDE.md` §"Key Decisions" — OTel namespace `pdbplus.*` convention applies
- Project quick task 260414-2rc reduced OTel cardinality ~30-55% — keep that spirit. Two attrs here at low cardinality (single value per cycle, not per-tuple).

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `internal/sync/worker.go` already emits an end-of-cycle span; attaching attributes to it is a one-liner.
- `runtime.MemStats.HeapInuse` — stdlib, no deps
- `/proc/self/status` VmHWM — stdlib parse, proven pattern from v1.14 memory investigation

### Established Patterns
- `slog.Warn` with typed attrs (`slog.Int("peak_heap_mib", N)`) matches GO-OBS-5
- Config env var parsing with strict fail-fast validator (Phase 59's `parsePublicTier` is the template)

### Integration Points
- Grafana dashboard lives in-repo at `grafana/pdbplus-overview.json` per D-05 (verify on disk during planning — user confirmed this location but planner should double-check it exists and is JSON)
- Phase 65 asymmetric fleet means the dashboard could start showing per-process-group metrics if that's meaningful; D-06 covers this

</code_context>

<specifics>
## Specific Ideas

- **Both metrics matter, not just one.** Go heap reports what the runtime allocated; RSS reports what the kernel holds. These diverge; both are useful. User explicitly chose both.
- **Defaults match Fly 512 MB cap with margin.** 400 MiB heap / 384 MiB RSS leaves room above for the app to crash-log + crash before Fly OOM-kills. Aggressively conservative would be noisier.
- **SEED-001 stays dormant — this phase just observes its trigger.** Phase 66 does not flip sync mode; it just makes the threshold visible so a future operator knows when to consider incremental.

</specifics>

<deferred>
## Deferred Ideas

- **Automated incremental-sync switch.** If peak heap breaches threshold for N consecutive cycles, automatically switch to incremental. Not in v1.15 scope — SEED-001 still requires the conformance test + hybrid schedule before flipping.
- **pprof endpoint.** Already discussed elsewhere; not in this phase. A `/debug/pprof/` route guarded by auth would be useful for heap profiling but out of scope.
- **Real-time alerting.** Grafana Alerts wiring (PagerDuty, email) — separate operational setup, not code.

</deferred>

---

*Phase: 66-observability-sqlite3-tooling*
*Context gathered: 2026-04-17*
