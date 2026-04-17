---
status: partial
phase: 62-api-key-default-docs
source: [62-02-PLAN.md]
started: 2026-04-17T22:15:00Z
updated: 2026-04-17T22:15:00Z
---

## Current Test

[awaiting operator actions against production Fly.io deployment]

## Tests

### 1. Set the Fly secret
expected: `PDBPLUS_PEERINGDB_API_KEY` secret landed on `peeringdb-plus` app; Fly triggers a rolling deploy
result: [pending]
command: `fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key> --app peeringdb-plus`
prerequisite: obtain a PeeringDB API key from https://www.peeringdb.com/profile (API Keys tab)

### 2. Confirm secret listed
expected: `fly secrets list --app peeringdb-plus` output includes a row for `PDBPLUS_PEERINGDB_API_KEY` with a masked digest
result: [pending]
command: `fly secrets list --app peeringdb-plus`

### 3. Confirm startup log shows authenticated sync
expected: after the rolling deploy completes, `fly logs --app peeringdb-plus` includes a `sync mode` slog.Info line with `auth=authenticated` (added in Phase 61)
result: [pending]
command: `fly logs --app peeringdb-plus | grep 'sync mode'`

### 4. Smoke test /api/poc returns Public-only data
expected: response contains POCs; all have `visible="Public"`; Users-tier rows from upstream are not present anonymously
result: [pending]
command: `curl https://peeringdb-plus.fly.dev/api/poc | jq '. | length'`

### 5. Smoke test /about renders Privacy & Sync section
expected: /about HTML contains "Privacy & Sync" section showing `auth=authenticated, public_tier=public`
result: [pending]
command: `curl -H 'User-Agent: Mozilla/5.0' https://peeringdb-plus.fly.dev/ui/about | grep -A5 'Privacy'`

## Summary

total: 5
passed: 0
issues: 0
pending: 5
skipped: 0
blocked: 0

## Gaps

None. All v1.14 code + docs landed. These 5 items require live Fly.io deployment access which is operator-only.
