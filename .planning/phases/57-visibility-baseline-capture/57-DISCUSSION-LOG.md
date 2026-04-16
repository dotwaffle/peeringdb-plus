# Phase 57: Visibility baseline capture - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-16
**Phase:** 57-visibility-baseline-capture
**Areas discussed:** Tool location, capture scope, fixture format, diff format, redaction policy, rate-limit pacing, resumability

---

## Tool location

| Option | Description | Selected |
|--------|-------------|----------|
| Extend cmd/pdbcompat-check/ (Recommended) | Already auth-aware, walks all 13 types, shares peeringdb client. Add `--capture` subcommand. | ✓ |
| New cmd/pdb-vis-capture/ | Single-purpose binary, ~150 lines duplicated scaffolding. | |
| internal/visbaseline package + go test -run | Test target gated by `-capture` flag. Cheapest commit footprint. | |

**User's choice:** Extend cmd/pdbcompat-check/
**Notes:** User added a hard constraint — "make sure the whole data from PeeringDB isn't saved into the repository, and especially don't commit any private data (so as that only retrievable through use of a PeeringDB API key)." This drove the redaction-policy follow-up question.

---

## Capture scope

| Option | Description | Selected |
|--------|-------------|----------|
| All 13 types, first 2 pages each (Recommended) | ~52 anon + 52 auth requests, inside rate ceilings, catches per-type behaviour. | ✓ |
| POC + parents only (poc/net/org/ix) | ~16 requests; misses social_media/notes etc. | |
| All 13 types, full enumeration | Hours under rate ceiling; overkill. | |
| All 13 types, single representative ID per type | ~26 requests; misses list-vs-detail visibility. | |

**User's choice:** All 13 types, first 2 pages each
**Notes:** —

---

## Fixture layout on disk

| Option | Description | Selected |
|--------|-------------|----------|
| Raw upstream JSON, mirror PeeringDB URL paths (Recommended) | Byte-identical to API response; easy diff/regenerate. | ✓ |
| Normalized through our peeringdb client structs | Smaller diffs but loses unknown-field surfacing. | |
| Single combined JSON per type, anon/auth side-by-side | Reviewer-friendly but harder to regenerate/consume. | |

**User's choice:** Raw upstream JSON, mirror PeeringDB URL paths
**Notes:** Layered with the redaction-policy answer below — anon = raw, auth = redacted.

---

## Diff report format

| Option | Description | Selected |
|--------|-------------|----------|
| Both: per-type Markdown table + machine-readable JSON (Recommended) | Markdown for review; JSON for downstream phase 60 tests. | ✓ |
| Per-type Markdown table only | No machine consumer; phase 60 must scrape. | |
| JSON only, render Markdown on demand | Single source of truth; render step required. | |

**User's choice:** Both
**Notes:** —

---

## Auth-fixture redaction policy

| Option | Description | Selected |
|--------|-------------|----------|
| Redact values, keep structure (Recommended) | Capture locally, write redacted version: keep field names, types, IDs, `visible`, counts; replace strings absent from anon with placeholders. | ✓ |
| Commit only the diff report, never the auth fixtures | Auth fixtures gitignored; diff describes deltas. | |
| Both: redacted fixtures + diff report | Belt and braces; two artifacts to maintain. | |

**User's choice:** Redact values, keep structure
**Notes:** Privacy constraint from earlier note made this question necessary.

---

## Rate-limit pacing

| Option | Description | Selected |
|--------|-------------|----------|
| Hardcoded conservative sleeps (Recommended) | 4s anon / 2s auth, predictable, well under ceilings. | |
| Reuse existing peeringdb.Client rate limiter | 20/min anon / 60/min auth, faster, sync-tuned. | |
| Dynamic via Retry-After + exponential backoff | Smartest under variable load; more complex. | ✓ |

**User's choice:** Dynamic via Retry-After + exponential backoff
**Notes:** Aligns with the v1.13 sync-worker change that already added `RateLimitError` + `Retry-After` honouring.

---

## Resumability

| Option | Description | Selected |
|--------|-------------|----------|
| Checkpoint state file in /tmp (Recommended) | Per-(type, mode, page) cursor; survives Ctrl+C/crashes/sleep. | ✓ |
| Single-run, no resume | Re-burn budget on interrupt; simple. | |
| Per-type idempotency: check fixture exists, skip if so | No state file; loses partial-page progress. | |

**User's choice:** Checkpoint state file in /tmp
**Notes:** —

---

## Claude's Discretion

- Exact placeholder string format for redacted fields
- Internal CLI shape of the new `--capture` subcommand
- Per-request progress UX (stderr / progress bar)

## Deferred Ideas

- Live drift detection on a schedule
- Historical archive of past baselines (git already covers this)
- Multi-region capture
