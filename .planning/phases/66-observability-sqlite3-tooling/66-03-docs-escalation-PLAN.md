---
phase: 66-observability-sqlite3-tooling
plan: 03
type: execute
wave: 2
depends_on: [66-01, 66-02]
files_modified:
  - CLAUDE.md
  - docs/DEPLOYMENT.md
  - .planning/PROJECT.md
autonomous: true
requirements: [OBS-04, DOC-04]
tags: [docs, seeds, claude-memory]

must_haves:
  truths:
    - "CLAUDE.md has a new '### Sync observability' section citing the two env vars, the two OTel span attrs, the slog.Warn line, and the SEED-001 escalation trigger"
    - "CLAUDE.md Environment Variables table has PDBPLUS_HEAP_WARN_MIB and PDBPLUS_RSS_WARN_MIB rows with defaults 400 / 384"
    - "docs/DEPLOYMENT.md Monitoring section references the three new dashboard panels by name"
    - "docs/DEPLOYMENT.md has a SEED-001 escalation note stating 'if heap sustained > PDBPLUS_HEAP_WARN_MIB, SEED-001 trigger fired — revisit incremental sync'"
    - "docs/DEPLOYMENT.md notes that sqlite3 is available in prod image for fly ssh console debugging (OBS-04)"
    - ".planning/PROJECT.md Key Decisions table has a new Phase 66 row capturing the HEAP_WARN=400 / RSS_WARN=384 defaults and the OTel-attr+slog.Warn hybrid mechanism"
  artifacts:
    - path: "CLAUDE.md"
      provides: "New Sync observability section + env-var rows + pointers to span attrs and log line"
      contains: "pdbplus.sync.peak_heap_mib"
    - path: "docs/DEPLOYMENT.md"
      provides: "SEED-001 escalation note in Monitoring section + dashboard panel names + sqlite3 tooling reference (OBS-04)"
      contains: "SEED-001"
    - path: ".planning/PROJECT.md"
      provides: "New Phase 66 Key Decisions row"
      contains: "Phase 66"
  key_links:
    - from: "docs/DEPLOYMENT.md Monitoring section"
      to: ".planning/seeds/SEED-001-incremental-sync-evaluation.md"
      via: "Markdown link + threshold variable names"
      pattern: "SEED-001"
    - from: "CLAUDE.md Sync observability"
      to: "internal/sync/worker.go emitMemoryTelemetry"
      via: "Code reference + span attr names"
      pattern: "emitMemoryTelemetry"
---

<objective>
Close the Phase 66 documentation loop — OBS-04 (sqlite3 tooling, already shipped via quick task 260418-1cn; needs a forward-reference only) and DOC-04 (heap-watch expectation documented in both operator and Claude-memory surfaces).

Purpose: Give operators a runbook entry tying the Grafana signals to the SEED-001 escalation path. Update Claude's project memory so future planners see the threshold / attr contract without re-deriving from code.

Output: Three file edits, one PROJECT.md Key Decisions row.

This plan runs in Wave 2 so it can cite the final attribute names + slog.Warn message verbatim from Plans 66-01 and 66-02 (no risk of naming drift).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/phases/66-observability-sqlite3-tooling/66-CONTEXT.md
@.planning/phases/66-observability-sqlite3-tooling/66-01-SUMMARY.md
@.planning/phases/66-observability-sqlite3-tooling/66-02-SUMMARY.md
@.planning/seeds/SEED-001-incremental-sync-evaluation.md

<interfaces>
CLAUDE.md section headers (order, current):
- Constraints / Code Generation / Schema & Visibility / Field-level privacy (Phase 64) / Middleware / ConnectRPC / Environment Variables / Testing / Build / LiteFS / CI / Go Module / Deployment / API Surfaces / Middleware Chain / Key Packages

Insertion point for the new Sync observability section: AFTER "### Deployment" and BEFORE "## Architecture" / "### API Surfaces (5)" (lines ~168-187 today).

CLAUDE.md Environment Variables table is at lines 116-137. New rows go AFTER the PDBPLUS_SYNC_MEMORY_LIMIT row (line 128).

docs/DEPLOYMENT.md section anchors:
- "## Monitoring" starts at line 272; ends around line 308 before "## Rollback"
- "## Build pipeline" at line 39

Existing dashboard reference in docs/DEPLOYMENT.md (lines 289-291) is the append anchor for the new SEED-001 subsection.

.planning/PROJECT.md Key Decisions table ends at line 220 (Phase 65 row). Append the Phase 66 row as line 221, BEFORE "## Evolution" at line 222.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add Sync observability section + env-var rows to CLAUDE.md</name>
  <files>CLAUDE.md</files>
  <read_first>
    - CLAUDE.md lines 116-139 (Environment Variables table)
    - CLAUDE.md lines 168-187 (Deployment section end + Architecture header — insertion anchor)
    - .planning/phases/66-observability-sqlite3-tooling/66-01-SUMMARY.md for the exact slog.Warn message key and the precise attr names shipped
  </read_first>
  <acceptance_criteria>
    - `grep -c '^### Sync observability' CLAUDE.md` returns 1
    - `grep -c 'PDBPLUS_HEAP_WARN_MIB' CLAUDE.md` returns 2 or more (table row + prose)
    - `grep -c 'PDBPLUS_RSS_WARN_MIB' CLAUDE.md` returns 2 or more
    - `grep -c 'pdbplus.sync.peak_heap_mib' CLAUDE.md` returns 1 or more
    - `grep -c 'pdbplus.sync.peak_rss_mib' CLAUDE.md` returns 1 or more
    - `grep -c 'SEED-001' CLAUDE.md` returns 1 or more
    - `grep -c 'heap threshold crossed' CLAUDE.md` returns 1 (exact slog key)
    - No existing section headers are renamed or removed: `git diff CLAUDE.md | grep -E '^\-(#{2,3}\s)' | wc -l` returns 0
  </acceptance_criteria>
  <action>
    Use the Edit tool (not Write — preserve the rest of CLAUDE.md).

    1. Append two new rows to the Environment Variables table, immediately AFTER the existing PDBPLUS_SYNC_MEMORY_LIMIT row (line 128). Insert verbatim (pipe-delimited, backtick env var names):

       Row 1: env var `PDBPLUS_HEAP_WARN_MIB`, default `400`, description "Peak Go heap (MiB) threshold. End-of-sync-cycle `slog.Warn(\"heap threshold crossed\", ...)` fires when `runtime.MemStats.HeapInuse` exceeds this; OTel span attr `pdbplus.sync.peak_heap_mib` emits on every cycle regardless. `0` disables the warn (attr still fires). Sustained breach = SEED-001 trigger fired."

       Row 2: env var `PDBPLUS_RSS_WARN_MIB`, default `384`, description "Peak OS RSS (MiB) threshold from `/proc/self/status` VmHWM (Linux only). OTel span attr `pdbplus.sync.peak_rss_mib`. `0` disables the warn. Attr omitted on non-Linux (RSS not available)."

    2. Insert a new section named `### Sync observability` AFTER the existing `### Deployment` section (search for "### Deployment" to find the end) and BEFORE the `## Architecture` heading. Content (plain text, no nested fences — use inline code spans):

       Opening paragraph: "End-of-sync-cycle memory telemetry surfaces SEED-001's incremental-sync-consideration trigger. Implementation: `internal/sync/worker.go` function `emitMemoryTelemetry`, called from `recordSuccess`, `rollbackAndRecord`, and `recordFailure` (three terminal paths of `Worker.Sync`)."

       Bullet list titled "OTel span attributes attached to the `sync-full` / `sync-incremental` cycle span:"
         - `pdbplus.sync.peak_heap_mib` — `runtime.MemStats.HeapInuse` in MiB
         - `pdbplus.sync.peak_rss_mib` — `/proc/self/status` VmHWM in MiB (Linux only — attr omitted on other OSes)

       Paragraph titled "Log signal" — when either threshold is breached, the worker calls `slog.Warn("heap threshold crossed", ...)` with typed attrs `peak_heap_mib`, `heap_warn_mib`, `peak_rss_mib`, `rss_warn_mib`, `heap_over`, `rss_over`.

       Paragraph titled "Thresholds" — via env vars (see table above): `PDBPLUS_HEAP_WARN_MIB` (default 400) and `PDBPLUS_RSS_WARN_MIB` (default 384). Defaults sit under the Fly 512 MB VM cap with margin so the order under pressure is: log → app crash → Fly OOM-kill.

       Paragraph titled "SEED-001 escalation" — if peak heap is sustained above `PDBPLUS_HEAP_WARN_MIB` across multiple sync cycles, SEED-001 (`.planning/seeds/SEED-001-incremental-sync-evaluation.md`) trigger has fired — surface at the next milestone and revisit `PDBPLUS_SYNC_MODE=incremental` after the deletion-conformance prerequisite work. Observed baseline on 2026-04-17: primary peak 83.8 MiB, replicas 58-59 MiB — ~4.5× headroom.

       Paragraph titled "Dashboard" — `deploy/grafana/dashboards/pdbplus-overview.json` has a "Sync Memory (SEED-001 watch)" row with three panels — "Peak Heap (MiB)", "Peak RSS (MiB)", and "Peak Heap by Process Group" (primary vs replica, post-Phase-65 asymmetric fleet).

       Paragraph titled "Incident-response debugging (OBS-04)" — prod image ships with `sqlite3` (added via quick task 260418-1cn pre-Phase-65). Use `fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db'` for interactive DB inspection on the primary; replicas expose the same FUSE path read-only.

    Traceability: OBS-04 (sqlite3 forward-reference) + OBS-05 (span attrs + log line documented in Claude memory) + DOC-04 (SEED-001 escalation in project memory).
  </action>
  <verify>
    <automated>grep -q '^### Sync observability' CLAUDE.md && grep -q 'PDBPLUS_HEAP_WARN_MIB' CLAUDE.md && grep -q 'pdbplus.sync.peak_heap_mib' CLAUDE.md && grep -q 'heap threshold crossed' CLAUDE.md && grep -q 'SEED-001' CLAUDE.md && grep -q 'sqlite3' CLAUDE.md</automated>
  </verify>
  <done>CLAUDE.md has the new section, env var rows, slog.Warn message key, and SEED-001 pointer; no existing sections renamed or removed.</done>
</task>

<task type="auto">
  <name>Task 2: Add SEED-001 escalation subsection to docs/DEPLOYMENT.md Monitoring section</name>
  <files>docs/DEPLOYMENT.md</files>
  <read_first>
    - docs/DEPLOYMENT.md lines 272-310 (Monitoring section)
    - docs/DEPLOYMENT.md lines 39-80 (Build pipeline section)
  </read_first>
  <acceptance_criteria>
    - `grep -c 'SEED-001' docs/DEPLOYMENT.md` returns 1 or more
    - `grep -c 'PDBPLUS_HEAP_WARN_MIB' docs/DEPLOYMENT.md` returns 1 or more
    - `grep -c 'pdbplus.sync.peak_heap_mib' docs/DEPLOYMENT.md` returns 1 or more
    - `grep -c 'sqlite3' docs/DEPLOYMENT.md` returns 1 or more (OBS-04 reference)
    - `grep -c 'Sync Memory (SEED-001 watch)' docs/DEPLOYMENT.md` returns 1 or more (dashboard row name)
    - `grep -c 'heap threshold crossed' docs/DEPLOYMENT.md` returns 1 or more
    - `grep -c '^## Monitoring\|^## Rollback' docs/DEPLOYMENT.md` returns 2 (neither section header renamed)
  </acceptance_criteria>
  <action>
    Use Edit, not Write. Operate inside the `## Monitoring` section only; do not touch `## Rollback`, `## Deploy command summary`, or earlier sections.

    1. In `## Monitoring`, insert a new subsection AFTER the Grafana dashboard paragraph at lines 289-291 and BEFORE the "Fly.io's built-in machine metrics" paragraph. Subsection title: `### Sync memory watch (SEED-001)`.

       Content (plain markdown prose, inline code spans for identifiers — do NOT nest triple-backtick code fences; if a CLI example is needed use indented 4-space code blocks):

       Paragraph 1: "Every sync cycle, the worker samples `runtime.MemStats.HeapInuse` and (on Linux) `/proc/self/status` VmHWM, attaches both as OTel span attrs (`pdbplus.sync.peak_heap_mib`, `pdbplus.sync.peak_rss_mib`) on the `sync-full` / `sync-incremental` span, and fires `slog.Warn(\"heap threshold crossed\", ...)` when either breaches its configured threshold."

       Paragraph 2: "Thresholds via `PDBPLUS_HEAP_WARN_MIB` (default 400) and `PDBPLUS_RSS_WARN_MIB` (default 384). Zero disables the warn for that metric (attrs still fire)."

       Paragraph 3 — titled "Dashboard": The `Sync Memory (SEED-001 watch)` row in `deploy/grafana/dashboards/pdbplus-overview.json` contains three panels:
         - `Peak Heap (MiB)` — threshold line at 400
         - `Peak RSS (MiB)` — threshold line at 384
         - `Peak Heap by Process Group` — primary vs replica breakdown (post-Phase-65 asymmetric fleet)

       Paragraph 4 — titled "SEED-001 escalation": If peak heap is sustained above `PDBPLUS_HEAP_WARN_MIB` across multiple sync cycles, SEED-001 (`.planning/seeds/SEED-001-incremental-sync-evaluation.md`) trigger has fired — revisit `PDBPLUS_SYNC_MODE=incremental` after the deletion-conformance prerequisite work documented in the seed. Observed baseline (2026-04-17): primary peak 83.8 MiB, replicas 58-59 MiB.

       Paragraph 5 — titled "Incident-response debug shell (OBS-04)": The prod image ships with the `sqlite3` binary (added 2026-04-18, quick task `260418-1cn`). Run interactive queries via:

           fly ssh console -a peeringdb-plus -C 'sqlite3 /litefs/peeringdb-plus.db'

       on the LHR primary. Replicas present the same FUSE path read-only.

    2. Check whether `sqlite3` is mentioned anywhere in `## Build pipeline` (lines 39-80). If NOT, add a single sentence at the end of the build-pipeline prose: "The prod image (`Dockerfile.prod`) also installs `sqlite3` via Chainguard's `sqlite` package for operator debugging via `fly ssh console`." Idempotent — only add if missing.

    Traceability: DOC-04 (escalation note) + OBS-04 (sqlite3 reference).
  </action>
  <verify>
    <automated>grep -q 'SEED-001' docs/DEPLOYMENT.md && grep -q 'PDBPLUS_HEAP_WARN_MIB' docs/DEPLOYMENT.md && grep -q 'sqlite3' docs/DEPLOYMENT.md && grep -q 'Sync Memory (SEED-001 watch)' docs/DEPLOYMENT.md && grep -q 'heap threshold crossed' docs/DEPLOYMENT.md</automated>
  </verify>
  <done>docs/DEPLOYMENT.md Monitoring section has a SEED-001 watch subsection naming the dashboard panels, citing the env vars, and documenting the escalation path; sqlite3 is referenced at least once.</done>
</task>

<task type="auto">
  <name>Task 3: Append Phase 66 row to PROJECT.md Key Decisions table</name>
  <files>.planning/PROJECT.md</files>
  <read_first>
    - .planning/PROJECT.md lines 159-222 (Key Decisions table — read the Phase 58, 63, 65 rows for format + Outcome column convention)
  </read_first>
  <acceptance_criteria>
    - `grep -c 'Phase 66' .planning/PROJECT.md` returns 1 or more (new row added)
    - `grep -c 'PDBPLUS_HEAP_WARN_MIB' .planning/PROJECT.md` returns 1 or more
    - `grep -c 'PDBPLUS_RSS_WARN_MIB' .planning/PROJECT.md` returns 1 or more
    - `grep -c 'pdbplus.sync.peak_heap_mib' .planning/PROJECT.md` returns 1 or more
    - `grep -c 'SEED-001' .planning/PROJECT.md` returns 1 or more
    - The Key Decisions table still parses (row count increases by exactly 1 — verify via `awk '/^\| Decision /,/^## Evolution/' .planning/PROJECT.md | grep -c '^| '`)
  </acceptance_criteria>
  <action>
    Use Edit, not Write.

    1. Insert a single new row BEFORE the `## Evolution` heading (line 222) and AFTER the existing Phase 65 row (line 220). Format matches the existing three-column table `| Decision | Rationale | Outcome |`:

       Decision column: "Phase 66 observability: OTel span attrs + slog.Warn hybrid for peak heap / RSS at end of sync cycle; defaults `PDBPLUS_HEAP_WARN_MIB=400` and `PDBPLUS_RSS_WARN_MIB=384`"

       Rationale column: "SEED-001 trigger observability without actioning it. Dual surface (span attr + slog.Warn) so dashboards keep continuous timeseries (`pdbplus.sync.peak_heap_mib`, `pdbplus.sync.peak_rss_mib`) while log pipelines see discrete alerts (`heap threshold crossed`). Defaults chosen vs Fly 512 MB VM cap: 400 MiB heap leaves 112 MB headroom for Go runtime + stack + binary before OOM-kill; 384 MiB RSS (75% of 400) catches VmHWM spikes earlier since VmHWM is strict-monotonic over the process lifetime. RSS read via `/proc/self/status` VmHWM on Linux, cleanly skipped on non-Linux (attr omitted, not zero-valued). Implementation: `internal/sync/worker.go` `emitMemoryTelemetry` called from recordSuccess / rollbackAndRecord / recordFailure. Sampling granularity = sync cycle frequency (1h default); no periodic background sampler added. Separate dashboard row `Sync Memory (SEED-001 watch)` in `deploy/grafana/dashboards/pdbplus-overview.json` with three panels (Peak Heap, Peak RSS, Peak Heap by Process Group for post-Phase-65 asymmetric fleet). Does NOT flip SEED-001 — escalation path is documented in `docs/DEPLOYMENT.md` and SEED-001 remains dormant until the trigger actually fires."

       Outcome column: "✓ Validated Phase 66"

    Traceability: OBS-05 + DOC-04 documented in project memory.
  </action>
  <verify>
    <automated>grep -q 'Phase 66 observability' .planning/PROJECT.md && grep -q 'PDBPLUS_HEAP_WARN_MIB=400' .planning/PROJECT.md && grep -q 'pdbplus.sync.peak_heap_mib' .planning/PROJECT.md && grep -q 'Validated Phase 66' .planning/PROJECT.md</automated>
  </verify>
  <done>PROJECT.md Key Decisions table has a Phase 66 row documenting the HEAP=400 / RSS=384 defaults, the OTel-attr + slog.Warn mechanism, the /proc/self/status VmHWM source, and the SEED-001 non-flip posture.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Docs → operators | Markdown rendered in editors, GitHub, and the Claude Code session; no runtime exposure. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-66-06 | Information-disclosure | Documenting internal thresholds publicly | accept | Thresholds already in env-var docs (CLAUDE.md) and visible in source; no secret value. |
| T-66-07 | Tampering | Stale docs drift from code | mitigate | Wave 2 ordering ensures docs cite shipped attr names verbatim; Plan 66-01 and 66-02 SUMMARYs are the source of truth read in this task. |
</threat_model>

<verification>
- All three file edits keep existing content intact (spot-check via `git diff --stat` — three files changed, net insertion only)
- All grep-verifiable criteria in each task's acceptance_criteria block pass
- No CI gates affected (docs-only changes)
</verification>

<success_criteria>
- OBS-04 + DOC-04 satisfied: sqlite3 tooling is referenced in operator docs; SEED-001 escalation path is documented in both operator (`docs/DEPLOYMENT.md`) and Claude-memory (`CLAUDE.md`) surfaces
- PROJECT.md Key Decisions table captures the Phase 66 decision at the same granularity as Phase 58/63/65 rows
</success_criteria>

<output>
After completion, create `.planning/phases/66-observability-sqlite3-tooling/66-03-SUMMARY.md`.
</output>
