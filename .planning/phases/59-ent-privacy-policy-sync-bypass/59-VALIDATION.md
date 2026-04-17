---
phase: 59
slug: ent-privacy-policy-sync-bypass
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-17
---

# Phase 59 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` + ent `enttest.Open` in-memory SQLite |
| **Config file** | `internal/testutil/` helpers (`SetupClient(t)`, `seed.Full`) |
| **Quick run command** | `go test -race ./internal/privctx/... ./internal/middleware/... ./internal/sync/... ./internal/config/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~5s quick, ~60s full |

---

## Sampling Rate

- **After every task commit:** Run quick suite
- **After every plan wave:** Run full suite
- **Before `/gsd-verify-work`:** Full suite green + `go vet ./... && golangci-lint run` pass
- **Max feedback latency:** 5s (quick), 60s (full)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 59-a | 01 | 1 | SYNC-03 | T-59-05 | `Tier` type roundtrip + zero-value default | unit | `go test -race -run TestTier_Roundtrip ./internal/privctx/` | ❌ W0 | ⬜ pending |
| 59-b | 01 | 1 | SYNC-03 | T-59-05 | `TierFrom` returns TierPublic for unstamped ctx | unit | `go test -race -run TestTierFrom_ZeroValueIsPublic ./internal/privctx/` | ❌ W0 | ⬜ pending |
| 59-c | 02 | 2 | SYNC-03 | T-59-05 | `PDBPLUS_PUBLIC_TIER=""` → TierPublic | unit | `go test -race -run TestLoad_PublicTierDefault ./internal/config/` | ❌ W0 | ⬜ pending |
| 59-d | 02 | 2 | SYNC-03 | T-59-05 | `PDBPLUS_PUBLIC_TIER="users"` → TierUsers | unit | `go test -race -run TestLoad_PublicTierUsers ./internal/config/` | ❌ W0 | ⬜ pending |
| 59-e | 02 | 2 | SYNC-03 | T-59-05 | Invalid value (`"Users"`, `"admin"`) → config.Load() error (fail-fast) | unit | `go test -race -run TestLoad_PublicTierInvalid ./internal/config/` | ❌ W0 | ⬜ pending |
| 59-f | 03 | 3 | SYNC-03 | T-59-01 | Middleware stamps TierPublic when configured | unit | `go test -race -run TestPrivacyTier_StampsDefault ./internal/middleware/` | ❌ W0 | ⬜ pending |
| 59-g | 03 | 3 | SYNC-03 | T-59-01 | Middleware stamps TierUsers when configured | unit | same file | ❌ W0 | ⬜ pending |
| 59-h | 03 | 3 | SYNC-03 | T-59-01 | `TestMiddlewareChain_Order` includes `PrivacyTier` between Logging and Readiness | unit (source-scan) | `go test -race -run TestMiddlewareChain_Order ./internal/middleware/` | ✅ modify | ⬜ pending |
| 59-i | 04 | 4 | VIS-04 | T-59-03 | `PocQueryRuleFunc` filters Users-tier rows for TierPublic ctx | unit | `go test -race -run TestPocPolicy_FiltersUsersTier ./internal/sync/` (or ./ent/) | ❌ W0 | ⬜ pending |
| 59-j | 04 | 4 | VIS-04 | T-59-03 | Public rows visible to TierPublic | unit | same file | ❌ W0 | ⬜ pending |
| 59-k | 04 | 4 | VIS-04 | T-59-03 | Users rows visible to TierUsers ctx | unit | same file | ❌ W0 | ⬜ pending |
| 59-l | 04 | 4 | VIS-04 | T-59-04 | NULL `visible` rows admitted by TierPublic (defence-in-depth) | unit | same file | ❌ W0 | ⬜ pending |
| 59-m | 04 | 4 | VIS-04 | T-59-03 | Edge traversal `network.QueryPocs()` filters for TierPublic | unit | `go test -race -run TestPocPolicy_EdgeTraversalFilters` | ❌ W0 | ⬜ pending |
| 59-n | 05 | 5 | VIS-05 | T-59-02 | Sync worker's wrapped ctx sees every row regardless of visibility | unit | `go test -race -run TestWorkerSync_BypassesPrivacy ./internal/sync/` | ❌ W0 | ⬜ pending |
| 59-o | 05 | 5 | VIS-05 | T-59-02 | Exactly one `privacy.DecisionContext(*, privacy.Allow)` call site (grep-based audit) | unit | `go test -race -run TestSyncBypass_SingleCallSite ./internal/sync/` | ❌ W0 | ⬜ pending |
| 59-p | 06 | 6 | VIS-04, VIS-05, SYNC-03 | T-59-01,02,03,04 | D-15 end-to-end: seed Users POC via bypass, anon HTTP request, row absent from all 5 surfaces | integration | `go test -race -run TestE2E_AnonymousCannotSeeUsersPoc ./cmd/peeringdb-plus/` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/privctx/privctx.go` + `privctx_test.go`
- [ ] `internal/middleware/privacy_tier.go` + `privacy_tier_test.go`
- [ ] `internal/middleware/chain_test.go` — extend existing chain-order test
- [ ] `internal/sync/policy_test.go` (ent privacy policy behaviour)
- [ ] `internal/sync/worker_test.go` additions (bypass active + single-call-site audit)
- [ ] `internal/config/config_test.go` additions (PDBPLUS_PUBLIC_TIER)
- [ ] `cmd/peeringdb-plus/e2e_privacy_test.go` (D-15)
- [ ] ent codegen: `gen.FeaturePrivacy` enabled in `ent/entc.go`; `go generate ./...` produces `ent/privacy/privacy.go` + generated rule adapters
- [ ] `ent/schema/poc.go` gains `Policy()` method (and any other entity surfaced by Phase 58 DIFF; none per current DIFF)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Startup log line shows resolved tier | SYNC-03 | Observability (phase 61 owns full coverage) | Start server with `PDBPLUS_PUBLIC_TIER=users` and confirm startup log includes `tier=users` |

*All safety-critical behaviours have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s (quick) / < 60s (full)
- [ ] `nyquist_compliant: true` set in frontmatter after Wave 0 lands

**Approval:** pending
