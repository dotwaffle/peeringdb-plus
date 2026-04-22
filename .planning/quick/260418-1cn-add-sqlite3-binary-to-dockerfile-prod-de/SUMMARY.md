---
status: complete
gsd_summary_version: 1.0
plan_id: 260418-1cn
mode: quick
completed: "2026-04-18"
commit: 4dfc52a
---

# Quick Task 260418-1cn: Add sqlite3 to prod image — Summary

## What shipped

- `Dockerfile.prod` line 17: `apk add --no-cache fuse3` → `apk add --no-cache fuse3 sqlite`.
- One-line Wolfi `sqlite` package added alongside the existing `fuse3` install. No new RUN layer, no image-layout changes.
- Committed as `4dfc52a` — `build(prod): add sqlite3 CLI to prod image for fleet ops`.
- Deployed via `fly deploy` — all 8 machines rolled forward to deployment `01KPF1N5875KFA491BS8C6JDD8` without failure.

## Verification

Executed on two machines (primary + replica) to confirm the package is present fleet-wide:

| Machine | Region | Role | `which sqlite3` | `sqlite3 --version` | `.tables` |
|---|---|---|---|---|---|
| `48e1ddea215398` | lhr | primary | `/usr/bin/sqlite3` | `3.51.1 2025-11-28` | 15 ent tables ✓ |
| `1850d57a514668` | nrt | replica | `/usr/bin/sqlite3` | `3.51.1 2025-11-28` | — |

`.tables` on LHR returns the expected ent-generated set: `campuses`, `carriers`, `carrier_facilities`, `facilities`, `internet_exchanges`, `ix_facilities`, `ix_lans`, `ix_prefixes`, `network_facilities`, `network_ix_lans`, `networks`, `organizations`, `pocs`, `sync_cursors`, `sync_status`.

## Image size impact

Not measured explicitly — Wolfi `sqlite` pulls `sqlite-libs` (already a transitive dep of other glibc-dynamic packages). Observed deploy build time was normal; no layer bloat warning in Fly output.

## Unblocks

- **Phase 65** (Asymmetric Fly fleet) — fleet migration can now use `sqlite3` directly on replicas for volume cleanup / sanity checks during rollout.
- **Phase 66** (SEED-001 escalation docs) — the tool is now available for incident responders following the escalation path that Phase 66 documents.

## Notes

- Used the existing `apk add` line instead of a separate RUN because Chainguard's glibc-dynamic-dev is Wolfi-based and already has apk — no need for a different install mechanism.
- sqlite3 runs read/write against the LiteFS FUSE mount at `/litefs/peeringdb-plus.db`. On replicas, write attempts will be rejected by LiteFS — operators should only use it read-only on replicas.
- No app config or ent schema change; this is a pure runtime-environment enhancement.
