# Phase 60: Surface integration + tests - Discussion Log

> **Audit trail only.** Decisions are captured in CONTEXT.md.

**Date:** 2026-04-16
**Phase:** 60-surface-integration-tests
**Areas discussed:** Test seed strategy, gRPC test driver, pdbcompat parity verification, conformance update scope

---

## Test seed strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Extend seed.Full with mix of Public + Users POCs (Recommended) | Single source of truth; existing tests still pass. | ✓ |
| New seed.FullWithVisibility helper | Explicit opt-in; more plumbing. | |
| Per-surface local seed factories | Most flexibility; most duplication. | |

**User's choice:** Extend seed.Full
**Notes:** —

---

## ConnectRPC test driver

| Option | Description | Selected |
|--------|-------------|----------|
| Go client against an httptest.Server (Recommended) | Mirrors external callers; matches existing pattern. | ✓ |
| Direct handler invocation, no HTTP round-trip | Bypasses privacy middleware — defeats purpose. | |
| grpcurl subprocess | Highest fidelity, highest cost. | |

**User's choice:** Go client against httptest.Server
**Notes:** —

---

## pdbcompat anonymous parity verification

| Option | Description | Selected |
|--------|-------------|----------|
| Replay VIS-01 anon fixtures via httptest (Recommended) | Deterministic; no live traffic; no API key needed. | ✓ |
| Live test gated by -peeringdb-live | Higher fidelity; flakier. | |
| Both: fixture + live gated | Belt and braces. | |

**User's choice:** Replay VIS-01 anon fixtures via httptest
**Notes:** —

---

## internal/conformance scope

| Option | Description | Selected |
|--------|-------------|----------|
| Anon-only conformance (Recommended) | One mode, reuses diff logic. | ✓ |
| Dual-mode (anon + auth) | Doubles coverage; needs API key in CI. | |
| No conformance changes | Surface tests cover the contract. | |

**User's choice:** Anon-only conformance
**Notes:** —

---

## Cross-cutting (asked at end of discussion)

### Direct-lookup behaviour for filtered rows

Captured in phase 59 CONTEXT.md (D-13/D-14) since it's a privacy-policy concern; phase 60 tests verify it per surface (404/NotFound/null-error).

## Claude's Discretion

- Stable IDs for new visibility-mixed seed POCs
- Test file organisation (existing _test.go vs new privacy_test.go)
- /ui/ rendered-output assertion strategy (string match vs goquery parse)

## Deferred Ideas

- Authenticated conformance with API key in CI
- E2E Playwright browser test
- Fuzz test for the privacy policy
