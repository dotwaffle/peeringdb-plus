---
status: resolved
phase: 57-visibility-baseline-capture
source: [57-VERIFICATION.md]
started: 2026-04-16T22:15:00Z
updated: 2026-04-16T23:55:00Z
---

## Current Test

[all live capture UAT items complete]

## Tests

### 1. Live beta capture — walk all 13 types in both auth modes
expected: 52 anon fixtures + redacted auth fixtures committed under `testdata/visibility-baseline/beta/`; raw auth bytes never leave `/tmp`; DIFF.md generated and reviewable
result: PASS — 26 anon + 26 auth page files committed (13 types × 2 pages each) in commit 088e2bb
command: `pdbcompat-check -capture -target=beta -mode=both -out=testdata/visibility-baseline/beta -api-key="$PDBPLUS_PEERINGDB_API_KEY"`

### 2. Redact + diff beta
expected: `testdata/visibility-baseline/beta/auth/api/{type}/page-{1,2}.json` contain only `<auth-only:string>` placeholders on PII fields (email, phone, name, address*); DIFF.md + diff.json describe structural deltas per type
result: PASS — all PII fields redacted to placeholders; DIFF.md + DIFF-beta.md + diff.json committed in 088e2bb
command: `pdbcompat-check -redact -in=$RAW_AUTH_DIR/auth -out=testdata/visibility-baseline/beta/auth` then `pdbcompat-check -diff -out=testdata/visibility-baseline/`

### 3. Prod confirmation for poc/org/net (ROADMAP SC #3)
expected: `testdata/visibility-baseline/prod/anon/api/{poc,org,net}/page-1.json` committed; prod-full / prod-anon-only / prod-partial signal recorded
result: PASS — 6 prod anon pages committed (3 types × 2 pages); resume signal: prod-anon-only
command: `pdbcompat-check -capture -target=prod -mode=anon -out=testdata/visibility-baseline/prod -types=poc,org,net`

### 4. PII guard passes on committed fixtures
expected: `TestCommittedFixturesHaveNoPII` flips from SKIP to PASS once real redacted fixtures exist
result: PASS — `--- PASS: TestCommittedFixturesHaveNoPII (0.53s)` on committed tree
command: `go test -race -run TestCommittedFixturesHaveNoPII ./internal/visbaseline/`

## Summary

total: 4
passed: 4
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

None. Phase 57 is fully complete — code + live fixtures + PII guard green.
