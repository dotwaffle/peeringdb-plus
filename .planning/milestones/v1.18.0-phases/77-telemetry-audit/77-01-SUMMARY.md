---
phase: 77-telemetry-audit
plan: 01
status: complete
shipped_at: 2026-04-27
requirements:
  - OBS-06
---

# Plan 77-01 Summary — Loki Log-Level Audit + Reclassification (OBS-06)

## What shipped

Audited production Grafana Cloud Loki via the `grafana-cloud` MCP server, produced `.planning/phases/77-telemetry-audit/AUDIT.md` documenting the level disposition for every slog call site touched by Phase 77's scope, applied the prescribed slog level changes inline, and added an env-configurable level filter to the otelslog handler so DEBUG records no longer ship to Loki by default.

## AUDIT.md row counts by category

| Disposition | Count | Notes |
|------|-------|-------|
| INFO → DEBUG | 3 | `worker.go:{815, 971, 1401}` per-type per-cycle records (`fetching`, `upserted`, `marked stale deleted`) |
| WARN → INFO | 3 | `worker.go:{824, 1452, 1484}` routine-not-error events (cursor missing on first sync, sync rate-limited) |
| WARN → DEBUG | 3 | `worker.go:1577` (cursor lookup on first deploy) + `health/handler.go:{123, 148}` (no-sync-completed during pre-first-sync window) |
| KEEP (no change) | 33 | Every security signal retained. Includes heap-threshold WARN, fk-orphans-summary WARN, public-tier-override WARN, demoted-during-sync WARN, sync-failed-after-retries ERROR. |
| **Architectural change** | 1 | `internal/otel/logger.go` — env-configurable `levelFilterHandler` wraps the otelslog branch. AUDIT.md "Architectural finding" section. |

Total: 9 inline slog level edits + 1 architectural change to the OTel log pipeline.

## Files modified

| File | Change |
|------|--------|
| `internal/sync/worker.go` | 7 inline level edits (lines 815, 824, 971, 1401, 1452, 1484, 1577) |
| `internal/health/handler.go` | 2 inline level edits (lines 123, 148) |
| `internal/otel/logger.go` | Added `levelFilterHandler` shim + `otelLevelFromEnv()` reading `PDBPLUS_LOG_LEVEL` (default `slog.LevelInfo`); wired into `NewDualLogger` so the OTel branch is now level-gated independently of the stdout branch (which stays at INFO unchanged). |
| `internal/health/handler_test.go` | Updated `TestHealth_GenericResponse/no_sync_yet` log-level assertion from WARN→DEBUG to match the new contract. |
| `internal/sync/worker_log_levels_test.go` | NEW — RED tests asserting `fetching`/`upserted` emit at DEBUG, that DEBUG records filter out at handler-level INFO, and that the FK-orphan-summary security signal stays at WARN with `total>0`. |
| `internal/health/handler_log_levels_test.go` | NEW — RED tests for both `/readyz` no-sync branches (default L123 + running L148) asserting DEBUG emission. |
| `internal/otel/logger_levelfilter_test.go` | NEW — exercises the wrapper's `Enabled` and `Handle` gating + `otelLevelFromEnv` parsing (6 valid cases + garbage fallback). |
| `docs/CONFIGURATION.md` | Added `PDBPLUS_LOG_LEVEL` row in OTel & Observability section. |
| `CLAUDE.md` | Added `PDBPLUS_LOG_LEVEL` bullet in operationally-critical defaults list with a one-line audit citation. |

## Pre-merge baseline log volume (from AUDIT.md, Grafana Cloud Loki, 30 min sample, 8-machine fleet)

| severity_text | Records | Per machine/min |
|---------------|---------|-----------------|
| DEBUG  | 52 | 0.217 |
| INFO   | 65 | 0.271 |
| WARN   |  1 | 0.004 |
| ERROR  |  0 | 0     |

User context: production traffic is currently low because the service is in tech-demo mode without a public traffic source. The audit's value is *proactive* — preventing future regression as cardinality grows — rather than reactive bulk reduction today.

## Expected post-merge reduction

Loki ingestion volume:

- **DEBUG → ~zero by default.** The `levelFilterHandler` blocks DEBUG records from reaching the OTel pipeline at the default `INFO` level. The stdout JSON handler keeps its existing INFO gate. Operators opt into DEBUG ingestion by setting `PDBPLUS_LOG_LEVEL=DEBUG`.
- **INFO ~85% reduction on primary.** The 3 INFO→DEBUG demotions cover the per-type per-cycle records that dominated INFO volume: `fetching` (39/30min), `upserted` (39/30min), `marked stale deleted` (18/30min when count>0). The remaining INFO records are the per-cycle `sync complete` summary (1/cycle/machine), boot-time classification, and access-log entries (`/healthz`+`/readyz` already excluded).
- **WARN already disciplined at 1 record/30min.** No measurable reduction expected from WARN demotions until cold-boot / rate-limit events occur naturally; the demotions are pre-emptive insurance.

## PDBPLUS_LOG_LEVEL env var contract

| Property | Value |
|----------|-------|
| Default | `slog.LevelInfo` |
| Accepted values | `DEBUG`, `INFO`, `WARN`, `ERROR` (case-insensitive, parsed via `slog.Level.UnmarshalText`) |
| Invalid input | Falls back to `INFO` without crashing — logging-level config is operator-friendly per the audit. CLAUDE.md GO-CFG-1 normally prefers fail-fast; for a malformed log level a fallback is more important than a startup crash. |
| Scope | OTel logging branch only. Stdout JSON handler stays at `INFO` independently. |
| Implementation | `internal/otel/logger.go` `otelLevelFromEnv()` + `levelFilterHandler` wrapper around `otelslog.NewHandler(...)`. |

## TDD discipline

| Phase | Commit | Description |
|-------|--------|-------------|
| RED | `4167213` | New test files asserting post-audit levels (failing on pre-audit source). |
| GREEN (inline) | `5fe9749` | The 9 inline level edits in worker.go + handler.go. |
| GREEN (architectural) | `70687a2` | `levelFilterHandler` + `otelLevelFromEnv` + handler_test.go assertion update. |
| Docs | `c1d86f4` | `PDBPLUS_LOG_LEVEL` documented in CONFIGURATION.md and CLAUDE.md. |

## Verification gates

| Gate | Result |
|------|--------|
| `go test -race ./...` | All 31 packages PASS |
| `golangci-lint run ./internal/sync/... ./internal/health/... ./internal/otel/... ./internal/middleware/... ./cmd/peeringdb-plus/...` | 0 issues |
| `go generate ./...` | Zero drift in `ent/`, `gen/`, `graph/`, `internal/web/templates/` |
| Acceptance grep gates from 77-01-PLAN.md | All pass: `fetching@INFO=0`, `upserted@INFO=0`, `heap-threshold@WARN=1`, `fk-orphans-summary@WARN=1`, `incremental-sync-failed@WARN=1`, `demoted-during-sync@WARN=1`, `sync-failed-after-retries@ERROR=1` |

## AUDIT.md rows deferred

None — every reclassification prescribed by the audit was applied.

## Line-number shifts encountered

None — every line cited in AUDIT.md matched the expected level on the `main` HEAD at audit time.

## Plan-frontmatter `files_modified` reconciliation

The plan's `files_modified` list anticipated edits in `cmd/peeringdb-plus/main.go` and `internal/middleware/logging.go`. The audit determined no changes were needed in those files (they are tracked as `KEEP` rows in AUDIT.md). Conversely, `internal/otel/logger.go` was added during execution per the architectural finding. Net delta: -2 anticipated, +1 architectural. This is consistent with the plan's "AUDIT.md is the source of truth" rule.

## Post-deploy verification (deferred)

The "production Loki volume measurably down" success criterion is post-deploy. After `fly deploy` of this commit chain, sample Loki for ~30 min on the same 8-machine fleet and compare:

- DEBUG record count should drop to ~zero (or whatever fires above INFO at sampling time).
- INFO record count should drop ~85% (per-type per-cycle records gone).
- WARN unchanged at ~1/cycle (the FK-orphan summary).

Record findings in 77-VERIFICATION.md when verified.
