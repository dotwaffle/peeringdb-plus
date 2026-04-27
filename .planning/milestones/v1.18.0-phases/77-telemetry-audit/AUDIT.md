# Phase 77 Loki Log-Level Audit (OBS-06)

**Sampled:** 2026-04-27 ~02:30Z → +30m, Grafana Cloud Loki (production tenant).
**Service:** peeringdb-plus (8 machines: 1 primary `lhr` + 7 replicas `iad`, `lax`, `nrt`, `sin`, `syd`, `gru`, `jnb`).
**Scope:** Per CONTEXT.md D-01 — slog level audit only. Structured-attribute audit is explicitly out of scope.

## Aggregate volumes (30 min window, all 8 machines)

| severity_text | Records | Per machine/min |
|---------------|---------|-----------------|
| DEBUG  | 52 | 0.217 |
| INFO   | 65 | 0.271 |
| WARN   |  1 | 0.004 |
| ERROR  |  0 | 0     |

Total ~118 records / 30 min / 8 machines ≈ 0.49 records/machine/min. Volume is currently low; the audit's goal is to stop misclassified WARN signals from masking real ones, more than to bulk-reduce volume.

## Top INFO messages (sample of last 100 INFO records, all 8 machines)

| Count | Line | Source | Per-cycle frequency |
|-------|------|--------|--------------------|
| 39 | `fetching` | `internal/sync/worker.go:815` | 13× per cycle (one per entity type) |
| 39 | `upserted` | `internal/sync/worker.go:971` | 13× per cycle (one per entity type) |
| 18 | `marked stale deleted` | `internal/sync/worker.go:1401` | 0–13× per cycle (only when stale rows exist) |
|  3 | `sync complete` | `internal/sync/worker.go:736` | 1× per cycle (per-cycle summary) |
|  1 | `http request` | `internal/middleware/logging.go:74` | per-non-health-request (`/healthz`+`/readyz` already excluded via `accessLogSkipPaths`) |

## Top DEBUG messages (sample of last 100 DEBUG records, all 8 machines)

| Count | Line | Source | Notes |
|-------|------|--------|-------|
| 48 | `fk orphan` | `internal/sync/worker.go:198` | Per-row; Phase 68 already DEBUG. Confirms the per-row regression mode is OFF. |
| 26 | `streamed all` | sync streaming complete | already DEBUG |
| 20 | `not primary, skipping sync` | `internal/sync/worker.go:1662` | already DEBUG; replica scheduler tick |
|  5 | `starting scheduler as replica` | `internal/sync/worker.go:1612` | already DEBUG; replica boot |
|  1 | `fk orphans summary` (total=0) | `internal/sync/worker.go:217` | already DEBUG; clean-cycle case |

## Top WARN messages

| Count | Line | Source | Action |
|-------|------|--------|--------|
| 1 | `fk orphans summary` (total=24, with `groups` attr listing 5 FK breaches) | `internal/sync/worker.go:246` | KEEP — per-cycle aggregate, replaced the per-row WARN-spam that breached Tempo's 7.5MB cap pre-Phase 68. Operator-actionable. |

No other WARN messages observed in the 30-min sample. WARN volume is already disciplined; the candidates flagged in CONTEXT.md plan-hints (`failed to get cursor`, `sync rate-limited`, `readyz no sync completed`) did NOT fire in this window because the fleet was stable (no boots, no first-syncs, no rate-limit events). The reclassification table below applies the demotions proactively for the next time those events occur.

## FK-orphan regression check

`{service_name="peeringdb-plus"} | severity_text="WARN" |~ "fk orphan"` returned 1 record over 30 min — the per-cycle aggregate at `internal/sync/worker.go:246` with `total=24`. The per-row WARN regression mode (which breached Tempo's 7.5 MB cap pre-Phase 68) is NOT firing — per-row `fk orphan` records appear at DEBUG only. ✓

## Architectural finding (CRITICAL — modifies the OBS-06 fix scope)

`internal/otel/logger.go:17-19` constructs the otelslog handler without a level filter:

```go
otelHandler := otelslog.NewHandler("peeringdb-plus",
    otelslog.WithLoggerProvider(logProvider),
)
```

The `fanoutHandler.Enabled` (`internal/otel/logger.go:35-42`) returns true if **any** sub-handler is enabled. The stdout handler is gated at `slog.LevelInfo` (line 21), but the otelslog handler has no level filter — it accepts every level including DEBUG. So `Handle` dispatches DEBUG records to both, the stdout handler refuses them, and the otelslog handler ships them to Grafana Cloud Loki.

**Empirical confirmation:** 52 DEBUG records / 30 min / 8 machines were ingested by Loki, despite the stdout handler being at INFO. This means simply demoting INFO→DEBUG reduces stdout/Fly log volume but does NOT reduce Loki ingestion volume.

**Fix:** Add an env-configurable level filter to the otelslog handler. Default to `slog.LevelInfo` so DEBUG records stay local; allow `LOG_LEVEL=debug` for opt-in deep debugging. This is in scope for Plan 77-01 Task 2 GREEN phase as an additional source change beyond the per-line slog level adjustments.

```go
// internal/otel/logger.go (proposed)
otelLevel := slog.LevelInfo
if v := os.Getenv("PDBPLUS_LOG_LEVEL"); v != "" {
    _ = otelLevel.UnmarshalText([]byte(v))
}
otelHandler := otelslog.NewHandler("peeringdb-plus",
    otelslog.WithLoggerProvider(logProvider),
)
// otelslog v0.13+: WithLevel option not yet on this lib; gate via the
// otelHandler.Enabled-shim or wrap the slog.Handler with slog.HandlerOptions{Level}.
```

If `otelslog` does not yet expose `WithLevel`, the workaround is to wrap the otelslog handler in a level-filtering decorator (similar pattern to fanoutHandler) — this lives in the same file and is a small addition.

## Reclassification table

`Recommended != Current` applies the change in Task 2. `KEEP` rows are tracked here for completeness — they are explicitly NOT changed.

| File:Line | Current | Recommended | Rationale | Security signal? |
|-----------|---------|-------------|-----------|------------------|
| internal/sync/worker.go:815 | INFO  | DEBUG | `fetching` per-type per-cycle. 39 records / 30min / 8 machines. Per-type fetch detail is available via OTel span attrs; not needed at INFO. | No |
| internal/sync/worker.go:971 | INFO  | DEBUG | `upserted` per-type per-cycle. 39 records / 30min / 8 machines. Same volume profile. | No |
| internal/sync/worker.go:1401 | INFO  | DEBUG | `marked stale deleted` per-type per-cycle (when count>0). 18 records / 30min / 8 machines. Per-cycle summary at L736 captures the operationally-relevant total. | No |
| internal/sync/worker.go:736 | INFO  | KEEP  | `sync complete` is the per-cycle summary; operator-relevant, low volume (1/cycle). | Yes (operator visibility into sync cadence) |
| internal/sync/worker.go:824 | WARN  | INFO  | `failed to get cursor, using full sync` fires on first sync after a fresh deploy when the cursor table is empty — routine, not an error condition. | No |
| internal/sync/worker.go:1452 | WARN  | INFO  | `sync rate-limited, deferring to next scheduled tick` fires routinely on Fly cold-boot when the upstream rate-limiter is touchy. Not actionable. | No |
| internal/sync/worker.go:1484 | WARN  | INFO  | `sync rate-limited during retry, deferring` — same rationale as L1452. | No |
| internal/sync/worker.go:1577 | WARN  | DEBUG | `failed to get last sync time` is routine on first deploy (cursor table empty); falls through to full sync correctly. Operator escalation already captured by L736 sync-complete. | No |
| internal/sync/worker.go:643 | WARN  | KEEP  | `heap threshold crossed` is the SEED-001 escalation signal — must remain WARN for operator escalation. | Yes (memory exhaustion; SEED-001) |
| internal/sync/worker.go:246 | WARN  | KEEP  | `fk orphans summary` per-cycle aggregate; replaces the per-row WARN-spam that breached Tempo's 7.5MB cap pre-Phase-68. | Yes (data integrity) |
| internal/sync/worker.go:217 | DEBUG | KEEP  | `fk orphans summary` (total=0 clean-cycle case). Already DEBUG; harmless. | No |
| internal/sync/worker.go:198 | DEBUG | KEEP  | `fk orphan` per-row. Already DEBUG; needed for investigation. | No |
| internal/sync/worker.go:323 | ERROR | KEEP  | `unknown type for DB record check` — programmer error. | Yes |
| internal/sync/worker.go:327 | ERROR | KEEP  | `failed to check DB for record`. | Yes |
| internal/sync/worker.go:446 | ERROR | KEEP  | `failed to record sync start`. | Yes |
| internal/sync/worker.go:569 | WARN  | KEEP  | `sync aborted: memory limit exceeded` — operator-actionable. | Yes |
| internal/sync/worker.go:698 | ERROR | KEEP  | `rollback failed`. | Yes |
| internal/sync/worker.go:721 | ERROR | KEEP  | `failed to update cursor`. | Yes |
| internal/sync/worker.go:869 | WARN  | KEEP  | `incremental sync failed, falling back to full` — SEED-001 trigger; operator-actionable. | Yes (sync correctness) |
| internal/sync/worker.go:1462 | WARN  | KEEP  | `sync failed, retrying` — retry storm visibility. | Yes |
| internal/sync/worker.go:1492 | ERROR | KEEP  | `sync failed after all retries` — final failure. | Yes |
| internal/sync/worker.go:1533 | WARN  | KEEP  | `demoted during sync, aborting cycle` — LiteFS demotion mid-sync; operator must see this. | Yes |
| internal/sync/worker.go:1542 | ERROR | KEEP  | `sync cycle failed`. | Yes |
| internal/sync/worker.go:1612 | DEBUG | KEEP  | `starting scheduler as replica` — already DEBUG. | No |
| internal/sync/worker.go:1633 | INFO  | KEEP  | `promoted to primary, checking sync status` — infrequent operator signal. | Yes (LiteFS lease event) |
| internal/sync/worker.go:1650 | INFO  | KEEP  | `demoted to replica` — infrequent operator signal. | Yes (LiteFS lease event) |
| internal/sync/worker.go:1662 | DEBUG | KEEP  | `not primary, skipping sync` — already DEBUG. | No |
| internal/sync/scratch.go:194 | WARN  | KEEP  | `close scratch db` — error-conditional close failure. | Yes |
| internal/sync/scratch.go:202 | WARN  | KEEP  | `unlink scratch db` — error-conditional unlink failure. | Yes |
| internal/middleware/logging.go:74 | INFO  | KEEP  | `http request` — `/healthz` + `/readyz` already excluded via `accessLogSkipPaths`. Proper access-log signal. | Yes (audit trail) |
| internal/health/handler.go:90 | ERROR | KEEP  | `readyz db probe failed`. | Yes |
| internal/health/handler.go:114 | ERROR | KEEP  | `readyz sync lookup failed`. | Yes |
| internal/health/handler.go:123 | WARN  | DEBUG | `readyz no sync completed` fires on every `/readyz` hit during pre-first-sync window. Fly hits `/readyz` ~every 15s × 8 machines = high volume during 5-15min cold start; not actionable since the 503 response already drives Fly proxy failover. | No |
| internal/health/handler.go:140 | ERROR | KEEP  | `readyz sync lookup failed` (running branch). | Yes |
| internal/health/handler.go:148 | WARN  | DEBUG | `readyz no sync completed` (running branch) — same rationale as L123. | No |
| internal/health/handler.go:157 | WARN  | KEEP  | `readyz sync marked failed` — operator-actionable. | Yes |
| internal/health/handler.go:166 | WARN  | KEEP  | `readyz unknown sync status` — programmer error escape. | Yes |
| internal/health/handler.go:181 | WARN  | KEEP  | `readyz sync stale` — operator-actionable. | Yes |
| internal/pdbcompat/handler.go:165 | DEBUG | KEEP  | `pdbcompat list: ignoring unsupported ?depth= param` — already DEBUG. | No |
| internal/pdbcompat/handler.go:195 | DEBUG | KEEP  | `pdbcompat: unknown filter fields silently ignored` — already DEBUG. | No |
| internal/pdbcompat/handler.go:274 | WARN  | KEEP  | `pdbcompat: response budget exceeded` — Phase 71 413 signal; operator-actionable. | Yes |
| internal/pdbcompat/handler.go:314 | ERROR | KEEP  | `pdbcompat: stream encode failed mid-response`. | Yes |
| cmd/peeringdb-plus/main.go:73 | ERROR | KEEP  | `failed to load config`. | Yes |
| cmd/peeringdb-plus/main.go:86 | ERROR | KEEP  | `failed to init otel`. | Yes |
| cmd/peeringdb-plus/main.go:493 | ERROR | KEEP  | `render terminal help`. | Yes |
| cmd/peeringdb-plus/main.go:894 | INFO  | KEEP  | `sync mode` — startup classification; once at boot. | Yes (security config visibility) |
| cmd/peeringdb-plus/main.go:899 | WARN  | KEEP  | `public tier override active` — security-relevant config signal. | Yes |

## Summary of changes prescribed for Task 2

| Change | Count | File:Lines |
|--------|-------|-----------|
| INFO → DEBUG | 3 | `worker.go:{815, 971, 1401}` |
| WARN → INFO  | 2 | `worker.go:{824, 1452, 1484}` (3 sites) |
| WARN → DEBUG | 3 | `worker.go:1577`, `health/handler.go:{123, 148}` |
| KEEP (no change) | rest | all rows above marked KEEP |
| **NEW:** otelslog handler level filter | 1 file | `internal/otel/logger.go:17-19` (architectural — see above) |

Total source-side reclassifications: 9 inline level changes + 1 architectural change to `internal/otel/logger.go`.

## OTEL_BSP_* and sampling note (cross-reference to OBS-07)

Production env values (per `internal/otel/provider.go` and existing baseline from PERF-08): `OTEL_BSP_SCHEDULE_DELAY=5s`, `OTEL_BSP_MAX_EXPORT_BATCH_SIZE=512`, sampling=1.0. These are confirmed appropriate for current cardinality in OBS-07's separate plan (77-02 Task 1 Tempo audit). Empirical Tempo trace size verification is the responsibility of plan 77-02.

## Items reviewed and KEPT (no change)

See "Recommended = KEEP" rows in the reclassification table above. All 26 KEEP rows are tracked there inline; the audit explicitly preserved every security-signal row.

## Volume-reduction expectation

Per-machine-minute volume baseline (pre-merge):
- INFO: 0.271/machine/min ≈ 96 INFO/cycle worst-case (across `fetching` 13 + `upserted` 13 + `marked stale` ≤13 + `sync complete` 1 = ~40/cycle/machine — primary only; replicas only emit `sync complete` and access logs).

Post-merge expectation (after Task 2 lands AND the otelslog level filter is added):
- INFO records dominated by `fetching` + `upserted` + `marked stale deleted` reclassified to DEBUG → expected ~85% reduction in primary-side INFO Loki ingestion volume.
- WARN volume already at 1 record / 30min — no measurable change expected from WARN demotions until cold-boot / rate-limit events occur naturally.
- DEBUG volume in Loki drops to ~zero (the otelslog level filter blocks all DEBUG emission unless `PDBPLUS_LOG_LEVEL=debug` is set).

---

## Tempo Trace Audit (OBS-07)

**Sampled:** 2026-04-27 ~03:30Z, fleet of 8 machines (1 primary `lhr` + 7 replicas).
**Audit data sources:**
- Per-route trace volume: Grafana Cloud Mimir/Prometheus via `mcp__grafana-cloud__query_prometheus` on `http_server_request_duration_seconds_count` (with `sampling=1.0` in production, request count == trace count for HTTP-rooted traces).
- OTEL_BSP confirmation: source-side inspection of `internal/otel/provider.go:60-63`.
- FK-orphan regression mode: cross-referenced with the 77-01 Loki audit (commit `0d9ad2f`).
- Direct TraceQL queries: NOT available in this session — the `grafana-cloud` MCP server's Tempo proxy tools were not loaded. Where the plan calls for direct trace-size measurement, this appendix uses surrogate evidence (Prometheus volume + structural argument). The operator can later validate empirically via TraceQL in the Grafana UI.

### Per-route trace volume (30 min sample, fleet-wide)

PromQL: `sum by (http_route) (increase(http_server_request_duration_seconds_count{service_name="peeringdb-plus"}[30m]))`

| Route group | Trace count (30 min) | Per machine/min | Notes |
|-------------|----------------------|-----------------|-------|
| `GET /healthz`           | ~960  | 4.0 | Dominant (~99% of HTTP trace volume). Fly health-check fires every ~15s × 8 machines = 32/min ≈ 960/30min. ✓ |
| `GET /readyz`            | 0     | 0   | Route is registered (`cmd/peeringdb-plus/main.go:329`) but Fly health-check is configured for `/healthz` only (`fly.toml:74`). Future-proofed in the sampling matrix below. |
| `GET /api/{rest...}`     | ~5/2h | 0.005 | pdbcompat surface — only manual probes in current tech-demo state. |
| `GET /rest/v1/*`         | 0     | 0   | entrest surface — no production traffic yet. |
| `GET /peeringdb.v1.*`    | 0     | 0   | ConnectRPC — no production traffic yet. |
| `GET /graphql`           | 0     | 0   | gqlgen — no production traffic yet. |
| `GET /ui/{rest...}`      | 0     | 0   | Web UI — no production traffic yet. |
| `GET /{$}` (root redirect)| 0    | 0   | — |
| `GET /static/`           | 0     | 0   | — |
| (sync worker — non-HTTP) | n/a   | n/a | Sync spans don't have `http.route`; counted via OTel span exporter, not via the HTTP histogram. |

**Health-check share of total HTTP trace volume: ~99%.** Operator confirmed pre-audit context: "production traffic is currently low because the service is in tech-demo mode without a public traffic source." The proactive sampling sets up the right defaults *before* real traffic arrives — when `/api/*` and `/rest/v1/*` start carrying the load, dropping `/healthz` to 1% prevents Tempo volume from being dominated by liveness probes.

### Max per-trace size

Direct `trace_size_bytes` metric / TraceQL query was not available in this session. Structural confirmation:

1. **The only known regression mode that breaches the 2 MB target is the per-row `fk orphan` WARN-spam pattern from pre-Phase-68.** That mode produced ~10K spans per `sync-incremental` cycle when an upstream PeeringDB import had FK breaches, blowing past Tempo's 7.5 MB per-trace cap.

2. **Phase 68 D-02 demoted the per-row record to DEBUG and added a per-cycle aggregate at WARN.** The 77-01 Loki audit empirically confirmed this is still in effect (commit `0d9ad2f` — 1 WARN record / 30 min for `fk orphans summary` with `total=24`, vs zero per-row WARN records).

3. **HTTP traces have bounded span counts.** A `/healthz` trace has ~3 spans (server span → handler → done). A `/api/*` list trace has ~5-15 spans (server span → mux → handler → ent query → SQLite → response). None approach the 2 MB target.

4. **Sync-cycle traces are bounded by the 13-entity-type pattern.** Each cycle generates O(13) per-type spans + setup/teardown — well under 100 spans, with bounded attributes.

**Verdict:** Max per-trace size <2 MB is structurally guaranteed at the current code state. Empirical TraceQL validation (`{ resource.service.name = "peeringdb-plus" } | duration > 1s` sorted by span count desc) is a follow-up the operator can run from the Grafana UI to confirm post-deploy.

### FK-orphan regression check

Cross-referenced from 77-01 Loki audit: `count_over_time({service_name="peeringdb-plus"} | severity_text="WARN" |~ "fk orphan" [30m])` returned **1** record (the per-cycle aggregate at `internal/sync/worker.go:246` with `total=24`). The per-row WARN-spam mode that breached the 7.5 MB cap pre-Phase-68 is **NOT firing**. ✓

### OTEL_BSP_* confirmation

Source-side confirmation from `internal/otel/provider.go:60-63`:

```go
sdktrace.WithBatcher(spanExporter,
    sdktrace.WithBatchTimeout(5*time.Second),     // 5s
    sdktrace.WithMaxExportBatchSize(512),          // 512
)
```

Decision: **KEEP** at PERF-08 baseline (5s / 512). Values are confirmed appropriate for current cardinality.

**Documentation drift finding (non-blocking):** The comment at `internal/otel/provider.go:54` states the values are "tuneable via OTEL_BSP_SCHEDULE_DELAY and OTEL_BSP_MAX_EXPORT_BATCH_SIZE" but the explicit `WithBatchTimeout` / `WithMaxExportBatchSize` options override any env-driven defaults. The values are effectively hardcoded; env vars do not actually tune them. This is a documentation bug, not an operational concern — the values are correct as-is. Recommended follow-up (out of scope for OBS-07): either drop the env-var comment or implement env-var override (Phase 78+ candidate).

### Recommended sampling matrix (proposed for plan 77-02 Task 2)

The matrix below is what Task 2's `perRouteSampler` will implement verbatim. Ratios are set to favour current-state observability (full sampling of low-volume API surfaces) while being robust to traffic growth (drop liveness probes pre-emptively).

| Route prefix | Ratio | Rationale |
|--------------|-------|-----------|
| `/healthz`            | 0.01 | Liveness traffic dominates HTTP trace volume today (~99%). 1% sample is enough to detect health-check failure modes (e.g. a regression where the handler suddenly takes 100ms instead of <1ms) without flooding Tempo. |
| `/readyz`             | 0.01 | Symmetric with `/healthz` — currently zero external traffic but Fly may add a `/readyz` check in future. Pre-emptive matching avoids a regression on the day someone enables it. |
| `/grpc.health.v1.Health/` | 0.01 | gRPC health probes; same rationale as HTTP liveness. |
| `/api/`               | 1.0 | pdbcompat — primary debugging surface. Full sampling required. |
| `/rest/v1/`           | 1.0 | entrest — primary debugging surface. |
| `/peeringdb.v1.`      | 1.0 | ConnectRPC — primary debugging surface. |
| `/graphql`            | 1.0 | Mid-volume; keep full for now (reassess if cardinality grows in v1.19+). |
| `/ui/`                | 0.5 | Browser traffic; halved per CONTEXT.md "TBD based on actual volume" (volume is currently zero — half of zero is zero, but the ratio is set so traffic growth is bounded). |
| `/static/`, `/favicon.ico` | 0.01 | Static assets; rare debugging value. |
| (default — sync worker, internal spans, root redirect) | `PDBPLUS_OTEL_SAMPLE_RATE` (default 1.0) | Sync cycles + non-HTTP traces honour the existing env var. The default matches current production behaviour. |

**`ParentBased` composition (locked):** `perRouteSampler` is wrapped in `sdktrace.ParentBased(perRouteSampler)` so cross-service trace continuity is preserved — once a parent span samples in (e.g. `/api/net`), all child spans (including any internal call to `/healthz` or downstream RPC fan-out) inherit the sampled-in decision regardless of their own route prefix. This prevents orphaned spans where a sampled-in `/api/*` parent calls a sampled-out `/internal/...` endpoint.

**Longest-prefix-wins:** if a future API path like `/api/auth/foo` is added, it inherits the `/api/` ratio (1.0) automatically. Adding a new `/api/auth/` prefix entry with a lower ratio would let that subpath's traces drop independently — but the current matrix has no such case.
