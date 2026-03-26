---
phase: 29
slug: network-detail-reference-implementation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-25
---

# Phase 29 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test -race ./internal/web/termrender/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race ./internal/web/termrender/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 29-01-01 | 01 | 1 | RND-02,RND-12,RND-13,RND-14,RND-15,RND-16 | unit | `go test -run TestRenderNetworkDetail ./internal/web/termrender/...` | ❌ W0 | ⬜ pending |
| 29-02-01 | 02 | 2 | RND-02 | integration | `go test -run TestTerminalNetworkDetail ./internal/web/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/web/termrender/network_test.go` — unit tests for RenderNetworkDetail
- [ ] `internal/web/handler_test.go` — integration test for terminal network detail

*Existing go test infrastructure covers all framework needs.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| ANSI colors render correctly for speed tiers and policy | RND-12, RND-13 | Visual verification | `curl peeringdb-plus.fly.dev/ui/asn/13335` — verify colored speed tiers and policy badge |
| Cross-reference paths are copy-pasteable | RND-16 | UX verification | Copy an IX path from output, paste as curl command, verify it works |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
