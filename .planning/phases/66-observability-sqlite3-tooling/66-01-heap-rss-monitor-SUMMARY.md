---
phase: 66-observability-sqlite3-tooling
plan: 01
subsystem: observability
tags: [otel, slog, config, sync, memory, seed-001]
status: complete
completed: "2026-04-18"
commit_range: "7858fa1..a9f509f"
---

# Phase 66 Plan 01 — Heap/RSS Sync-Cycle Telemetry (OBS-05)

## What shipped

End-of-sync-cycle peak heap + OS RSS observability, surfaced as both OTel span attributes (dashboards) and slog.Warn log lines (alerting pipelines). Makes SEED-001's "peak heap >threshold" trigger observable without flipping sync mode.

### Config additions

- `PDBPLUS_HEAP_WARN_MIB` — int MiB, default 400, 0 disables the Warn (attr still emits). Unit suffix rejected per GO-CFG-1.
- `PDBPLUS_RSS_WARN_MIB` — int MiB, default 384, 0 disables the Warn.
- New `parseMiB(key, defaultMiB) (int64, error)` helper rejecting unit suffixes / negatives / non-numeric / floats; overflow-guarded.
- `validate()` rejects negative values for both fields (defense-in-depth).

### Runtime additions (`internal/sync/worker.go`)

- `emitMemoryTelemetry(ctx, heapWarnBytes, rssWarnBytes)` — reads `runtime.MemStats.HeapInuse`, parses `/proc/self/status` VmHWM on Linux, attaches `pdbplus.sync.peak_heap_mib` (always) and `pdbplus.sync.peak_rss_mib` (Linux only), fires `slog.Warn("heap threshold crossed", ...)` when either threshold breached.
- `readLinuxVMHWM() (int64, bool)` — parses VmHWM line; returns `ok=false` on non-Linux OR missing file (NEVER zero — that would lie on dashboards).
- Three call sites: `recordSuccess`, `rollbackAndRecord`, `recordFailure` — all while the sync-cycle span is still live.

### Wiring (`cmd/peeringdb-plus/main.go`)

- Plumbs `cfg.HeapWarnBytes` / `cfg.RSSWarnBytes` into `WorkerConfig`.

## Commits

| Commit | Description |
|--------|-------------|
| `7858fa1` | `feat(66-01): add HeapWarnBytes + RSSWarnBytes config (OBS-05)` |
| `a9f509f` | `feat(66-01): emit heap+RSS OTel attrs + slog.Warn at sync-cycle end (OBS-05)` |

## Verification

- `go test -race -count=1 ./internal/config/... ./internal/sync/...` → PASS
  - New tests: `TestLoad_HeapWarnMiB_Parse` (8 cases), `TestLoad_RSSWarnMiB_Parse` (8 cases), `TestEmitMemoryTelemetry_Attrs` (3 cases — below/heap_over/both_disabled), `TestReadLinuxVMHWM` (Linux-only)
  - No regressions in existing suite
- `go vet ./...` → clean
- `golangci-lint run` on touched packages → 0 issues (after one-cycle fix: initial `readLinuxVmHWM` name failed revive var-naming; renamed to `readLinuxVMHWM`)
- `go build ./...` → clean

## Deviations

None.

## Coverage trace

| Decision | Evidence |
|----------|----------|
| D-02 (both OTel attr + slog.Warn) | worker.go `emitMemoryTelemetry` emits both |
| D-03 (both heap AND RSS; configurable) | two env vars both plumbed end-to-end |
| D-04 (defaults 400/384) | documented in config struct doc comment + tests assert defaults |
| D-09 (end-of-sync-cycle; no background sampler) | emitMemoryTelemetry called only from the 3 sync-terminal paths |
| D-10 (VmHWM on Linux, fallback) | readLinuxVMHWM returns ok=false off-Linux; attr omitted not zeroed |

## Unblocks

- Plan 66-02 (Grafana panels) — can now reference `pdbplus.sync.peak_heap_mib` / `pdbplus.sync.peak_rss_mib` attrs.
- Plan 66-03 (docs) — can cite the final env var names + attr names verbatim.
