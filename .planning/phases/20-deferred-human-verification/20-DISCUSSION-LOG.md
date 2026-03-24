# Phase 20: Deferred Human Verification - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-24
**Phase:** 20-deferred-human-verification
**Areas discussed:** Verification reporting, browser testing, CI verification, API key testing, failure handling

---

## Verification Reporting

| Option | Description | Selected |
|--------|-------------|----------|
| Structured report | VERIFICATION-ITEMS.md with pass/fail table | ✓ |
| GitHub issues | Create issues for failures | |
| Inline in STATE.md | Lightweight update to blockers section | |

**User's choice:** Structured report

---

## Browser Testing Scope

| Option | Description | Selected |
|--------|-------------|----------|
| Chrome | Primary browser with DevTools | ✓ |
| Firefox | Second browser, different engine | |
| Safari | WebKit for macOS/iOS | |
| Chrome only | Skip cross-browser | ✓ |

**User's choice:** Chrome only — sufficient for a niche tool

---

## Screenshot Policy

| Option | Description | Selected |
|--------|-------------|----------|
| Text only | Pass/fail with description | ✓ |
| Screenshots for visual items | Capture for UI items | |
| You decide | Claude's judgment | |

**User's choice:** Text only

---

## CI Verification Method

| Option | Description | Selected |
|--------|-------------|----------|
| Live push test | Push commit or open PR to verify CI | ✓ |
| YAML review only | Read workflow files | |
| Already verified | Check if recent push triggered CI | |

**User's choice:** Live push test — but first check if the 12 commits already triggered CI

---

## API Key Availability

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, have one | Can test locally | ✓ |
| No, skip | Defer API key items | |
| Use env var on Fly.io | Test via deployed instance | |

**User's choice:** Has an API key available for local testing

---

## Failure Handling

| Option | Description | Selected |
|--------|-------------|----------|
| Fix inline | Fix failures in Phase 20, re-verify | ✓ |
| Log and defer | Document failure, create follow-up issue | |
| Fix if trivial | Threshold-based (< 30 min) | |

**User's choice:** Fix inline — no deferral

---

## Claude's Discretion

- Network throttling for loading indicators
- Syncing page animation verification approach
- Keyboard navigation testing depth
- Verification report structure

## Deferred Ideas

None
