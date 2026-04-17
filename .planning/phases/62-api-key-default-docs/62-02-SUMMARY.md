---
phase: 62-api-key-default-docs
plan: 02
subsystem: deployment
tags: [fly.io, operator, production, authenticated-sync]
requires: [62-01]
provides: [SYNC-01]
affects: [production]
tech-stack: [fly.io, peeringdb.com]
key-files: []
status: complete
date: 2026-04-17
---

# Plan 62-02 — Operator fly-secret rollout + smoke test

## Status: COMPLETE (all 6 operator UAT items PASS)

All steps ran against production `peeringdb-plus` app on 2026-04-17:

| Step | Command | Result |
|------|---------|--------|
| 0 | `fly deploy` (v1.14 code) | PASS |
| 1 | `fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key>` | PASS |
| 2 | `fly secrets list --app peeringdb-plus` | PASS (masked digest visible) |
| 3 | `fly logs --app peeringdb-plus \| grep 'sync mode'` | PASS (`auth=authenticated`) |
| 4 | `curl .../api/poc \| jq length` | PASS (Public-only) |
| 5 | `curl -H UA .../ui/about \| grep Privacy` | PASS (section rendered, `public_tier=public`) |

See `62-HUMAN-UAT.md` for full PASS status.

## Deliverables

- Production app `peeringdb-plus` switched to authenticated PeeringDB sync via `fly secrets set`.
- Startup classification log (Phase 61) confirms `auth=authenticated public_tier=public` on the deployed binary.
- `/about` surface (Phase 61) renders the Privacy & Sync section on prod with the expected values.
- Anonymous API responses are filtered through the Phase 59 privacy policy (POC Users-tier rows absent).

## Notes

- Sync mode stays `full` (default, not overridden in `fly.toml`). Users-tier rows backfill naturally on the next scheduled sync cycle (interval: 1h).
- Operator chose NOT to switch to `PDBPLUS_SYNC_MODE=incremental` at this time — incremental has a stale-row cleanup gap and a one-time backfill ordering risk; defer to a future milestone with a conformance test if pursued.
- v1.14 milestone ship gate satisfied; ready for milestone lifecycle (audit → complete → cleanup).

## Requirements Satisfied

- **SYNC-01:** Production deployment runs authenticated sync with PeeringDB API key as Fly secret.
