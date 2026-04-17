---
status: resolved
phase: 62-api-key-default-docs
source: [62-02-PLAN.md]
started: 2026-04-17T22:15:00Z
updated: 2026-04-17T23:00:00Z
---

## Current Test

[all operator steps complete]

## Tests

### 0. Deploy the v1.14 code
expected: `peeringdb-plus` app runs a binary that contains Phase 59/60/61 code (ent privacy policy, PrivacyTier middleware, `sync mode` startup log, `/about` Privacy & Sync section).
result: PASS
command: `fly deploy` (from project root)

### 1. Set the Fly secret
expected: `PDBPLUS_PEERINGDB_API_KEY` secret landed on `peeringdb-plus` app; Fly triggers a second rolling deploy to re-inject the env var into the v1.14 image.
result: PASS
command: `fly secrets set PDBPLUS_PEERINGDB_API_KEY=<key> --app peeringdb-plus`

### 2. Confirm secret listed
expected: `fly secrets list --app peeringdb-plus` output includes a row for `PDBPLUS_PEERINGDB_API_KEY` with a masked digest
result: PASS
command: `fly secrets list --app peeringdb-plus`

### 3. Confirm startup log shows authenticated sync
expected: after the rolling deploy completes, `fly logs --app peeringdb-plus` includes a `sync mode` slog.Info line with `auth=authenticated`
result: PASS
command: `fly logs --app peeringdb-plus | grep 'sync mode'`

### 4. Smoke test /api/poc returns Public-only data
expected: response contains POCs; all have `visible="Public"`; Users-tier rows from upstream are not present anonymously
result: PASS
command: `curl https://peeringdb-plus.fly.dev/api/poc | jq '. | length'`

### 5. Smoke test /about renders Privacy & Sync section
expected: /about HTML contains "Privacy & Sync" section showing `auth=authenticated, public_tier=public`
result: PASS
command: `curl -H 'User-Agent: Mozilla/5.0' https://peeringdb-plus.fly.dev/ui/about | grep -A5 'Privacy'`

## Summary

total: 6
passed: 6
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

None. v1.14 deployed with authenticated sync; all operator verifications green.
