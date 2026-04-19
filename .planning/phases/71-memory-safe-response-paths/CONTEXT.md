---
phase: 71
slug: memory-safe-response-paths
milestone: v1.16
status: context-locked
has_context: true
locked_at: 2026-04-19
---

# Phase 71 Context: Memory-safe response paths on 256 MB replicas

## Goal

Depth=2 and `limit=0` pdbcompat responses stream JSON with bounded allocations; a configurable memory budget triggers RFC 9457 problem-detail 413 before Fly OOM; per-request heap/RSS telemetry surfaces via OTel+Prometheus; the memory envelope is documented.

## Requirements

- **MEMORY-01** — streaming JSON emission for large responses
- **MEMORY-02** — `PDBPLUS_RESPONSE_MEMORY_LIMIT` default 128 MiB + RFC 9457 413
- **MEMORY-03** — per-request heap/RSS telemetry (OTel span attr + Prometheus gauge)
- **MEMORY-04** — docs/ARCHITECTURE.md § Response Memory Envelope

## Locked decisions

- **D-01 — Streaming JSON: hand-rolled token writer**. New `internal/pdbcompat/stream.go` exposes `StreamListResponse(w, meta, rowsIter, encoder)`:
  1. Write `{"meta":` + `json.Marshal(meta)` + `,"data":[`
  2. For each row from iterator: write `,` (if not first), then `json.Marshal(row)` to `w`
  3. Write `]}` + flush
  `rowsIter` is `func() (row any, ok bool, err error)` closure. Per-row allocation is bounded; no full-result slice. `http.Flusher.Flush()` called every N rows (default 100) for backpressure.
- **D-02 — Memory budget enforcement: pre-flight row-count × per-row heuristic**. Before streaming starts:
  1. Run `SELECT COUNT(*)` on the filtered query (existing `handleCount` helper, refactored for reuse)
  2. Estimate `bytes = count × typical_row_bytes(entity, depth)`
  3. If `bytes > budget`, return 413 problem-detail up-front with `max_rows = budget / typical_row_bytes`
  4. Otherwise stream
  `typical_row_bytes` is a hardcoded map per entity × depth, calibrated from benchmarks (D-03). Conservative estimate by design — false-positive 413s are preferred over OOM.
- **D-03 — `typical_row_bytes` calibration**: Measured via `internal/pdbcompat/bench_row_size_test.go` at seed.Full scale for each entity at depth=0 and depth=2. Stored as a hardcoded `map[string]RowSize{Depth0, Depth2}` in `internal/pdbcompat/rowsize.go`. Reviewed every major milestone via a separate plan — if row sizes drift >20%, update the map. Initial conservative doubling of measured mean applied to cover worst-case rows (networks with large `notes` fields, etc.).
- **D-04 — 413 response shape (RFC 9457 problem-detail)**: Reuses existing `internal/httperr.WriteProblem` from v1.9 Phase 46. Body:
  ```json
  {
    "type": "https://peeringdb-plus.fly.dev/errors/response-too-large",
    "title": "Response exceeds memory budget",
    "status": 413,
    "detail": "Request would return ~N rows totaling ~B bytes; limit is L bytes",
    "max_rows": <computed>,
    "budget_bytes": <PDBPLUS_RESPONSE_MEMORY_LIMIT>
  }
  ```
  `Retry-After` not set — 413 is request-shape, not transient.
- **D-05 — `PDBPLUS_RESPONSE_MEMORY_LIMIT` default**: 128 MiB (128 × 1024 × 1024 bytes). Rationale: 256 MB replica total − 80 MB Go runtime baseline (observed v1.15) − 48 MB slack for other in-flight requests + GC overhead = 128 MiB available. Operators can raise via env var.
- **D-06 — Telemetry: per-request heap delta as OTel span attr + Prometheus gauge**. In `internal/pdbcompat/handler.go`, wrap list handlers with a memstat reader:
  - `heap_start := runtime.MemStats.HeapInuse` at handler entry
  - `heap_end := runtime.MemStats.HeapInuse` at handler exit
  - OTel span attr `pdbplus.response.heap_delta_kib = (heap_end - heap_start) / 1024`
  - Prometheus histogram `pdbplus_response_heap_delta_kib{endpoint,entity}`
  `ReadMemStats` is STW-expensive (~microseconds at our heap size) but acceptable once per request. NOT done per-row.
- **D-07 — Scope: pdbcompat only**. grpcserver streaming RPCs already use batched keyset pagination (500-row chunks) and don't materialise large slices — they're inherently streaming. entrest uses ent-generated handlers which don't buffer unbounded data. GraphQL has its own complexity/depth limits from v1.12. Phase 71 ONLY adds streaming + budget to pdbcompat's `/api/<type>` list handlers.

## Out of scope

- Streaming for entrest / grpcserver / GraphQL / Web UI — they have their own memory stories (see D-07)
- Adaptive budget based on current heap headroom — rejected as option per cross-cutting concerns (cascading-failure risk)
- Per-endpoint budget overrides — single global budget per D-05

## Dependencies

- **Depends on**: Phases 67, 68, 69, 70 (the budget needs to account for all new worst-case response shapes unlocked by the correctness work — 2-hop joins, unbounded `limit=0`, etc.)
- **Enables**: Phase 72 (parity tests include budget-breach assertions)

## Plan hints for executor

- Touchpoints: new `internal/pdbcompat/{stream,rowsize,budget}.go`, `internal/pdbcompat/handler.go` (wire pre-flight check + streaming), new `internal/pdbcompat/bench_row_size_test.go`, `internal/httperr/` (no changes — reuse existing `WriteProblem`), `internal/config/config.go` (new `ResponseMemoryLimit` field), `docs/ARCHITECTURE.md` (new § Response Memory Envelope table).
- Reuse existing `handleCount` helper from v1.9 for the pre-flight SELECT COUNT(*). Budget check happens BEFORE the count query to avoid unbounded queries triggering the work.
- Grafana dashboard: add "Response Heap Delta" panel to `pdbplus-overview.json` using the new histogram.

## References

- ROADMAP.md Phase 71
- REQUIREMENTS.md MEMORY-01..04
- RFC 9457 (Problem Details for HTTP APIs)
- v1.15 Phase 66 (`internal/sync/worker.go` `emitMemoryTelemetry`) for telemetry pattern
- v1.9 Phase 46 (`internal/httperr/`) for problem-detail utilities
- CLAUDE.md § Sync observability (matching MiB/OTel attr conventions)
