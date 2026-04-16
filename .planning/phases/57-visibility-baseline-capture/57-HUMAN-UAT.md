---
status: partial
phase: 57-visibility-baseline-capture
source: [57-VERIFICATION.md]
started: 2026-04-16T22:15:00Z
updated: 2026-04-16T22:15:00Z
---

## Current Test

[awaiting human operator with live PeeringDB API access]

## Tests

### 1. Live beta capture — walk all 13 types in both auth modes
expected: 52 anon fixtures + redacted auth fixtures committed under `testdata/visibility-baseline/beta/`; raw auth bytes never leave `/tmp`; DIFF.md generated and reviewable
result: [pending]
command: `pdbcompat-check -capture -target=beta -mode=both -out=testdata/visibility-baseline/beta -api-key="$PDBPLUS_PEERINGDB_API_KEY"`
wall-clock: ≥1h (rate-limit bound)

### 2. Redact + diff beta
expected: `testdata/visibility-baseline/beta/auth/api/{type}/page-{1,2}.json` contain only `<auth-only:string>` placeholders on PII fields (email, phone, name, address*); DIFF.md + diff.json describe structural deltas per type
result: [pending]
command: `pdbcompat-check -redact -in=$RAW_AUTH_DIR/auth -out=testdata/visibility-baseline/beta/auth` then `pdbcompat-check -diff -out=testdata/visibility-baseline/`

### 3. Prod confirmation for poc/org/net (ROADMAP SC #3)
expected: `testdata/visibility-baseline/prod/anon/api/{poc,org,net}/page-1.json` committed; prod-full / prod-anon-only / prod-partial signal recorded
result: [pending]
command: `pdbcompat-check -capture -target=prod -mode=anon -out=testdata/visibility-baseline/prod -types=poc,org,net`

### 4. PII guard passes on committed fixtures
expected: `TestCommittedFixturesHaveNoPII` flips from SKIP to PASS once real redacted fixtures exist
result: [pending]
command: `go test -race -run TestCommittedFixturesHaveNoPII ./internal/visbaseline/`

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps

None. The capture CLI, redactor, differ, emitters, and PII guard all landed and green — these items require live PeeringDB access and operator judgement, which is intrinsic to the rate-limit-bound capture phase.
