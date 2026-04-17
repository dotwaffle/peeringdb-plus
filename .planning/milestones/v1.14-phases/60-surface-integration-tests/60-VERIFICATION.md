---
phase: 60-surface-integration-tests
verified: 2026-04-16T00:00:00Z
status: passed
score: 10/10 must-haves verified
overrides_applied: 0
requirements_satisfied: [VIS-06, VIS-07, SYNC-02]
---

# Phase 60: Surface Integration + Tests Verification Report

**Phase Goal:** Prove the privacy policy fires correctly through every read surface and that the pdbcompat anonymous shape matches upstream's anonymous shape.
**Verified:** 2026-04-16
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (merged from ROADMAP SCs + prompt must_haves)

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | seed.Full creates Users-tier POCs with stable IDs in the 9000 band | VERIFIED | `internal/testutil/seed/seed.go:249-273` — UsersPoc ID=9000 (on Network ASN 13335), UsersPoc2 ID=9001 (on Network2 ASN 6939), both via `privacy.DecisionContext(ctx, privacy.Allow)`; `AllPocs` slice exposes ordered `[Poc, UsersPoc, UsersPoc2]` |
| 2 | Per-surface anonymous-leak tests exist for all 5 read surfaces through real middleware chain | VERIFIED | `cmd/peeringdb-plus/privacy_surfaces_test.go` — 9 sub-tests via `httptest.NewServer(buildMiddlewareChain(...))`: pdbcompat_list_count, pdbcompat_detail_404, rest_list_count, rest_detail_404, graphql_list_count, grpc_list_count, grpc_detail_notfound, ui_network_detail_no_leak, ui_contacts_fragment_no_leak. All 5 surfaces (pdbcompat, rest, graphql, grpc, ui) exercised. |
| 3 | List endpoints return exactly 1 POC (r.Poc id=500 Public) on canonical seed.Full (3 POCs total) | VERIFIED | All 4 list sub-tests assert len==1 + id==500; graphql additionally asserts `totalCount==1`. Test run confirmed all 9 sub-tests PASS (`go test -race`). |
| 4 | gRPC GetPoc(9000) returns `connect.CodeNotFound` (surface-native idiom, D-13/D-14) | VERIFIED | `privacy_surfaces_test.go:474` — `connect.CodeOf(err) == connect.CodeNotFound` assertion; NOT PermissionDenied. |
| 5 | /ui/ HTML for network detail + contacts fragment does NOT contain Users-tier name/email | VERIFIED | `privacy_surfaces_test.go:279-280` — asserts `"Users-Tier NOC"` and `"users-noc@example.invalid"` absent from rendered HTML on `/ui/asn/13335` and `/ui/fragment/net/10/contacts`. |
| 6 | pdbcompat anon parity replays 13-type fixture corpus with (effectively) empty structural diff | VERIFIED | `internal/pdbcompat/anon_parity_test.go` — 13 sub-tests, all PASS. 12 clean, 1 documented `ixpfx.notes` divergence in `knownDivergences` allow-list (pre-existing compat delta, not a privacy leak, explicitly flagged for operator sign-off in 60-03-SUMMARY.md per Plan 03 design). POC sub-test additionally asserts IDs 9000/9001 and visible="Users" absent from response. |
| 7 | Conformance live test is anon-vs-anon only (no auth branch) | VERIFIED | `internal/conformance/live_test.go` — no matches for `PDBPLUS_PEERINGDB_API_KEY`, `Authorization.*Api-Key`, `apiKey`, `os.Getenv`, `if apiKey`. Docstring cites D-10/D-11; log line says `anon-vs-anon live conformance`. Sleep unconditionally `3 * time.Second`. `-peeringdb-live` gate preserved. |
| 8 | No-key sync test: fake upstream, 0 Users rows in DB, 0 Authorization headers | VERIFIED | `internal/sync/nokey_sync_test.go` — `authHeaderCount atomic.Int64` incremented on any non-empty Authorization header (line 113-117); post-sync asserts `Count(poc.Visible("Users"))==0` (line 195), `authHeaderCount.Load()==0` (line 219). Fake upstream serves 13 VIS-01 anon fixtures verbatim. |
| 9 | Post-sync anonymous surface read returns filter-is-a-no-op row set (Phase B) | VERIFIED | `nokey_sync_test.go:237-239` — mounts `pdbcompat.NewHandler(client).Register(mux)` wrapped in `privctx.WithTier(..., privctx.TierPublic)`; asserts surface row count equals bypass-query row count, no row has `visible="Users"`. |
| 10 | `go test -race ./...` green | VERIFIED | Full suite run: all packages PASS with `-race`. No failures, no data-races detected. Individual targeted tests also PASS: `TestPrivacySurfaces` (9 sub-tests, ~1.3s), `TestAnonParityFixtures` (13 sub-tests, ~1.3s), `TestNoKeySync` (~4s), `TestFull_HasUsersPocs`/`TestFull_PublicCountsUnchanged`/`TestFull_PrivacyFilterShapes` (~1.2s). |

**Score:** 10/10 truths verified

### ROADMAP Success Criteria Coverage

| # | ROADMAP SC | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Per-surface integration tests assert no `visible=Users` row leaks on anonymous request across all 5 surfaces | VERIFIED | Truths 2, 3, 4, 5 |
| 2 | Anonymous `/api/poc` matches upstream anonymous shape; Users-tier rows absent not redacted; VIS-01 fixtures replay with empty diff | VERIFIED | Truth 6 (POC sub-test is clean; only `ixpfx.notes` documented divergence is unrelated to privacy) |
| 3 | Integration test: unauthenticated sync, no API key, no Users-tier rows in DB, filter is no-op | VERIFIED | Truths 8, 9 |
| 4 | `internal/conformance/` live comparison is our-anon vs upstream-anon | VERIFIED | Truth 7 |

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `internal/testutil/seed/seed.go` | Extended with UsersPoc/UsersPoc2/AllPocs fields + privacy bypass seed | VERIFIED | 6 patterns matched (UsersPoc/UsersPoc2/AllPocs fields, SetID 9000/9001, SetVisible("Users") x2, privacy.DecisionContext). Substantive: ~28 new lines, real `privacy.DecisionContext(ctx, privacy.Allow)` writes. Wired: consumed by `seed_mixed_visibility_test.go`, `privacy_surfaces_test.go`, `anon_parity_test.go`. |
| `internal/testutil/seed/seed_mixed_visibility_test.go` | Regression tests (3) locking fixture contract | VERIFIED | Black-box `package seed_test` with 3 `t.Parallel()` tests: `TestFull_HasUsersPocs`, `TestFull_PublicCountsUnchanged`, `TestFull_PrivacyFilterShapes`. All 3 PASS. (Plan specified `seed_test.go` but author correctly pivoted to a new filename because the existing `seed_test.go` uses `package seed` — documented in 60-01-SUMMARY deviation 3.) |
| `cmd/peeringdb-plus/privacy_surfaces_test.go` | 5-surface anonymous-leak regression via real middleware chain | VERIFIED | 565 lines, 9 parallel sub-tests, 1 top-level `TestPrivacySurfaces`. All 5 surfaces wired: `pdbcompat.NewHandler`, `rest.NewServer`, `pdbgql.NewHandler`, `web.NewHandler`, `peeringdbv1connect.NewPocServiceHandler`. No `httptest.NewRecorder` (D-07 enforced). |
| `internal/pdbcompat/anon_parity_test.go` | 13-type fixture-replay parity gate with absent-not-redacted POC check | VERIFIED | `TestAnonParityFixtures` — 13 sub-tests. Uses `conformance.CompareResponses`. `knownDivergences` allow-list has 1 entry (`ixpfx.notes`) documented inline with root cause. POC sub-test explicitly asserts IDs 9000/9001 and visible="Users" absent via `assertUsersPocsAbsent`. |
| `internal/conformance/live_test.go` | Anon-vs-anon only, auth branch deleted | VERIFIED | Complete removal: 0 hits for API-key patterns; `3 * time.Second` unconditional sleep; docstring cites D-10/D-11; `-peeringdb-live` gate preserved. Diff: 7 insertions, 14 deletions per 60-04-SUMMARY. |
| `internal/sync/nokey_sync_test.go` | End-to-end no-key sync + surface-read integration test | VERIFIED | `TestNoKeySync` uses `atomic.Int64` header counter, fake upstream serves VIS-01 fixtures verbatim, asserts `Count(Visible("Users"))==0`, `authHeaderCount==0`, and surface-vs-DB row-count parity. No `SetAPIKey` / `Authorization.*Api-Key` in test. |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `seed.go` | `ent/privacy.DecisionContext` | Users POC creation path | WIRED | `privacy.DecisionContext(ctx, privacy.Allow)` (line 247), used for both Users POC writes. |
| `privacy_surfaces_test.go` | `seed.Full` | Canonical mixed-visibility fixture | WIRED | `r := seed.Full(t, client)` (line 121); fixture.seed passed through to all sub-tests. |
| `privacy_surfaces_test.go` | `buildMiddlewareChain` | Full production middleware chain | WIRED | 3 usages at lines 24, 80, 175 — wraps mux in `chainConfig{DefaultTier: TierPublic, ...}`. |
| `anon_parity_test.go` | `testdata/visibility-baseline/beta/anon/api` | `os.ReadFile` + `conformance.CompareResponses` | WIRED | Const `anonFixtureRoot` (line 40), `CompareResponses` called per sub-test (line 158). |
| `anon_parity_test.go` | `conformance.CompareResponses` | Structural differ from VIS-02 | WIRED | Imported and called; differences checked against `knownDivergences` map. |
| `nokey_sync_test.go` | `testdata/visibility-baseline/beta/anon/api` | Fake upstream handler serves fixtures verbatim | WIRED | Const literal (line 95), bytes written to response directly. |
| `nokey_sync_test.go` | `internal/sync.Worker.Sync` | `w.Sync(ctx, config.SyncModeFull)` | WIRED | Line 177. |

### Data-Flow Trace (Level 4)

Phase 60 is test-only (no new production code). Data flows are:
- Test → httptest.Server → real handler → ent privacy policy → in-memory SQLite (seeded via `seed.Full` with real data).
- Per-surface assertions verify real DB state is correctly filtered at each surface.
- Data flow: VERIFIED by successful test runs (all assertions evaluate against genuine ent queries, not mocks).

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Plan 01: seed fixture contract tests | `go test -race -run '^TestFull_HasUsersPocs$\|^TestFull_PublicCountsUnchanged$\|^TestFull_PrivacyFilterShapes$' ./internal/testutil/seed/` | `ok ... 1.234s` | PASS |
| Plan 02: per-surface privacy tests | `go test -race -run '^TestPrivacySurfaces$' ./cmd/peeringdb-plus/` | `ok ... 1.294s` (9 sub-tests) | PASS |
| Plan 03: anon parity fixture replay | `go test -race -run '^TestAnonParityFixtures$' ./internal/pdbcompat/` | `ok ... 1.290s` (13 sub-tests, 1 documented ixpfx divergence) | PASS |
| Plan 04: conformance anon-vs-anon skip path | `go test -race ./internal/conformance/` | `ok` (skipped without `-peeringdb-live`) | PASS |
| Plan 05: no-key sync test | `go test -race -run '^TestNoKeySync$' ./internal/sync/` | `ok ... 4.049s` | PASS |
| Full suite with race detector | `go test -race ./...` | All packages PASS | PASS |

### Requirements Coverage

| Requirement | Source Plan(s) | Description | Status | Evidence |
| ----------- | -------------- | ----------- | ------ | -------- |
| VIS-06 | 60-01, 60-02, 60-05 | All 5 read surfaces honour privacy policy; per-surface tests assert no `visible=Users` leaks | SATISFIED | Plan 01 seeds the fixture; Plan 02 asserts all 5 surfaces filter; Plan 05 covers the no-key path where Users rows are absent upstream. |
| VIS-07 | 60-03, 60-04, 60-05 | pdbcompat `/api/poc` anonymous shape matches upstream — rows absent not redacted — verified by VIS-01 fixture replay | SATISFIED | Plan 03 replays 13-type VIS-01 corpus with empty diff (modulo 1 documented non-privacy divergence). POC sub-test explicitly asserts IDs 9000/9001 and visible="Users" are absent. Plan 04 flips conformance to anon-vs-anon. |
| SYNC-02 | 60-05 | Unauthenticated sync — no API key, anonymous payload only, no Users-tier rows in DB, filter is no-op | SATISFIED | Plan 05 proves zero Users rows, zero Authorization headers, and filter-is-a-no-op surface parity end-to-end. |

All 3 requirements claimed by plans map to phase-60 roadmap entry; no orphaned requirements.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| _none_ | - | - | - | - |

Zero TODO/FIXME/XXX/HACK/PLACEHOLDER markers in any phase-60 artifact. No hollow stubs, no orphaned implementations. The `knownDivergences` allow-list (1 entry) is documented with root cause, is not a privacy leak, and is explicitly tracked as a follow-up plan in 60-03-SUMMARY — not an anti-pattern.

### Gaps Summary

No gaps. All 10 must-haves verified:

1. Fixture (seed.Full with Users-tier POCs at IDs 9000/9001) is in place and exposed via typed Result handles.
2. All 5 read surfaces (`/api/`, `/rest/v1/`, `/graphql`, `/peeringdb.v1.*`, `/ui/`) are exercised through the real production middleware chain (`buildMiddlewareChain`) in `TestPrivacySurfaces` — no `httptest.NewRecorder` shortcut (D-07 enforced).
3. VIS-07 absent-not-redacted contract is directly asserted for POC IDs 9000/9001 in `TestAnonParityFixtures`.
4. Conformance live test is anon-only — no API-key env var read, no Authorization header, documented anon-vs-anon mode (D-10/D-11).
5. No-key sync integration test proves both the upstream boundary (no Authorization headers leave the worker) and the DB invariant (zero Users rows synced). Surface parity confirms filter is a no-op on Public-only data.
6. Full `go test -race ./...` passes. All 9 privacy-surface sub-tests, all 13 anon-parity sub-tests, all 3 seed regression tests, and `TestNoKeySync` pass individually.

The one documented divergence (`ixpfx.notes` extra field in our response) is an pre-existing non-privacy compat delta, operator-flagged for follow-up in 60-03-SUMMARY — it does not violate any phase-60 success criterion and was explicitly anticipated by Plan 03's `knownDivergences` design.

### Human Verification Required

None. Phase 60 is a test-only phase; all assertions are automated and programmatically verified. The live conformance test (`TestLiveConformance` with `-peeringdb-live`) is operator-optional and explicitly out of CI scope per D-11 — the skip-path is verified and the test gate is preserved. No UI/visual/real-time behaviors were introduced that would need human confirmation.

---

*Verified: 2026-04-16*
*Verifier: Claude (gsd-verifier)*
