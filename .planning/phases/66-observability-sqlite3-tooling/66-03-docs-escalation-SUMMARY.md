---
phase: 66-observability-sqlite3-tooling
plan: 03
subsystem: docs
tags: [claude-memory, deployment-docs, key-decisions, seed-001]
status: complete
completed: "2026-04-18"
commit: 4d470e3
---

# Phase 66 Plan 03 — SEED-001 escalation docs (OBS-04 + DOC-04)

## What shipped

Three doc edits closing the Phase 66 loop:

1. **CLAUDE.md** — Environment Variables table gets `PDBPLUS_HEAP_WARN_MIB` + `PDBPLUS_RSS_WARN_MIB` rows. New `### Sync observability` section after `### Deployment` covering: emitMemoryTelemetry call sites, OTel span attrs, Prometheus gauge export, slog.Warn message key, thresholds + Fly-cap margin, SEED-001 escalation trigger, dashboard panels, and the OBS-04 sqlite3 debug shell reference.

2. **docs/DEPLOYMENT.md** — Monitoring section gets a new `### Sync memory watch (SEED-001)` subsection with: measurement semantics, threshold env vars, dashboard panel names, SEED-001 escalation path, and the incident-response `fly ssh console ... sqlite3` one-liner (OBS-04).

3. **.planning/PROJECT.md Key Decisions** — new Phase 66 row capturing the HEAP=400 / RSS=384 defaults, the OTel-attr + slog.Warn + gauge triple-surface design, /proc/self/status VmHWM source, and the explicit "does NOT flip SEED-001" posture.

## Verification

All acceptance greps pass:
- `grep -c '^### Sync observability' CLAUDE.md` → 1
- `grep -c 'PDBPLUS_HEAP_WARN_MIB' CLAUDE.md` → 3 (table + 2 prose)
- `grep -c 'pdbplus.sync.peak_heap_mib' CLAUDE.md` → 1
- `grep -c 'heap threshold crossed' CLAUDE.md` → 1
- `grep -c 'SEED-001' CLAUDE.md` → 1
- `grep -c 'SEED-001' docs/DEPLOYMENT.md` → 2
- `grep -c 'sqlite3' docs/DEPLOYMENT.md` → 2
- `grep -c 'Sync Memory (SEED-001 watch)' docs/DEPLOYMENT.md` → 1
- `grep -c 'Phase 66' .planning/PROJECT.md` → 1
- `grep -c 'Validated Phase 66' .planning/PROJECT.md` → 1

## Commit

`4d470e3` — `docs(66-03): SEED-001 escalation + env vars + sync observability (OBS-04, DOC-04)`

## Plan coverage

- OBS-04 (sqlite3 tooling reference): CLAUDE.md §Sync observability + docs/DEPLOYMENT.md §Sync memory watch (SEED-001)
- DOC-04 (heap-watch documented): CLAUDE.md §Sync observability + docs/DEPLOYMENT.md + PROJECT.md Key Decisions

Deviations: none.
