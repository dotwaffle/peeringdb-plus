// Package schema-level hooks live here.
//
// Removed 2026-04-28 (post v1.18.5): otelMutationHook(typeName) created
// one OTel span per ent mutation. With sync cycles upserting up to
// ~270k objects (full catch-up after a long downtime / cursor reset),
// the resulting per-mutation spans inflated the parent sync trace
// to >7.5 MB and tripped Tempo's per-trace size cap (TRACE_TOO_LARGE).
//
// Per-type and per-cycle observability is already covered by:
//   - pdbplus.sync.type.objects counter (per-type cumulative)
//   - pdbplus.sync.duration histogram (per-cycle)
//   - sync-fetch-{type} / sync-upsert-{type} per-step spans
//
// If a new per-Op tracing need arises, prefer wiring it at a coarser
// level (per-batch or per-chunk) than per-mutation.
package schema
