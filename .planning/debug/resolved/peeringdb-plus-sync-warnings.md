---
status: resolved
trigger: "Debug sync warnings in peeringdb-plus logs"
symptoms:
  expected: "Sync process should maintain data integrity without excessive warnings or skipping soft-deletes."
  actual: "Numerous warnings about orphan foreign keys and skipped soft-deletes due to maxSQLVars limit."
  errors:
    - "nulling orphan FK"
    - "dropping FK orphan"
    - "soft-delete skipped: remoteIDs exceed maxSQLVars chunk limit, SEED-004 trigger candidate"
  timeline: "Visible in current logs (2026-04-22)."
  reproduction: "Triggered during the periodic sync process."
resolution:
  root_cause: "1) Soft-delete logic skips cleanup for types > 30k records due to SQLite variable limits. 2) FK filtering drops legitimate records during incremental sync because it fails to check the existing DB for parents."
  fix: "Not applied (User chose to plan the fix separately)."
created: 2026-04-22
updated: 2026-04-22
---

# PeeringDB Plus Sync Warnings

## Current Focus
hypothesis: "The sync process is encountering data inconsistencies or limits in the underlying database driver (maxSQLVars) causing orphan records and incomplete cleanups."
next_action: "Fixed found: Implement robust delete via temp tables and fix incremental FK registry checks."

## Evidence
- timestamp: 2026-04-22T15:00:00Z
  observation: "Logs show 'nulling orphan FK' and 'dropping FK orphan' frequently during sync."
- timestamp: 2026-04-22T15:00:00Z
  observation: "Logs show 'soft-delete skipped: remoteIDs exceed maxSQLVars chunk limit' for various entity types (network ix lans, network facilities, pocs, networks, organizations)."
- timestamp: 2026-04-22T15:30:00Z
  observation: "Code analysis confirms `deleteStaleChunked` explicitly returns 0 if `len(remoteIDs) > 30000`."
- timestamp: 2026-04-22T15:35:00Z
  observation: "Code analysis confirms `fkCheckParent` drops rows if parent is not in `fkRegistry`, which is incomplete during incremental syncs."

## Eliminated
(none)

## Resolution
Root causes identified. Fix direction involves using temporary tables for large-scale deletions and making the FK filter database-aware during incremental syncs.
