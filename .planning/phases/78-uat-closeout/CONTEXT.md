---
phase: 78
slug: uat-closeout
milestone: v1.18.0
status: context-locked
has_context: true
locked_at: 2026-04-26
---

# Phase 78 Context: UAT Closeout

## Goal

Close out the small slice of outstanding human-verification items: v1.13's 2 deferred phase-52/53 items (CSP DevTools check + security headers/body-cap/slowloris) and the v1.5 Phase 20 stale-pointer dir.

## Requirements

- **UAT-01** — v1.13 Phase 52 CSP enforcement verified live with `PDBPLUS_CSP_ENFORCE=true`
- **UAT-02** — v1.13 Phase 53 security headers + body cap + slowloris verified
- **UAT-03** — v1.5 Phase 20 deferred-human-verification dir relocated to `consumed/` archive

## Locked decisions

- **D-01 — UAT-01/02 verification: hybrid (Claude drives curl, user drives DevTools).** Split:
  - **UAT-02 (curl + slowloris):** Claude executes directly. `curl -I https://peeringdb-plus.fly.dev/ui/` for headers; verify `Strict-Transport-Security`, `X-Frame-Options`, `X-Content-Type-Options` are present per the original v1.13 phase 53 scope. Body-cap test: send a request with `--data-binary @largefile` against `/api/*` (REST/pdbcompat surface — should reject at 2 MB) and `/peeringdb.v1.*` (gRPC stream — should accept per the skip-list). Slowloris probe: a small Go program that opens N connections and writes one byte per second; verify the connection-pool isn't exhausted (PDBPLUS_DRAIN_TIMEOUT default 10s should bound this).
  - **UAT-01 (CSP DevTools):** User drives. CSP enforcement-mode violation reports require Chrome DevTools Network panel inspection — not curl-friendly. Claude produces a step-by-step UAT-RESULTS.md template; user opens DevTools on `/ui/`, `/ui/asn/13335`, `/ui/compare` with `PDBPLUS_CSP_ENFORCE=true` set as a Fly secret, captures violation count (should be zero), pastes results back. The Tailwind v4 JIT runtime behaviour is the specific concern — that's what the original v1.13 phase deferred.

- **D-02 — UAT-03: just relocate + update STATE.md.** Per memory `project_human_verification.md`: "Phase 20 (v1.5) — COMPLETE. All 26 items from v1.2-v1.4 verified against live deployment on 2026-03-24." The dir is a stale pointer. Move `.planning/milestones/v1.5-phases/20-deferred-human-verification/` to a consumed-style archive location (mirror the seed convention — `.planning/milestones/v1.5-phases/20-deferred-human-verification.archived/` OR add a top-level `archived: true` marker file). Update STATE.md "Outstanding Human Verification" section to remove the v1.5 reference (already absent — verify). No re-verification of the 26 items; trust the 2026-03-24 record.

## Out of scope

- Re-verifying any of the 26 v1.5 items. Per D-02, trust the 2026-03-24 record. Re-verification would be 30+ min of busywork to re-prove what was already proved 5 weeks ago.
- The v1.6 / v1.7 / v1.11 human-verification items (~33 combined). Out of scope per REQUIREMENTS.md — defer to a future "UI verification sweep" milestone.
- Adding new security headers beyond what v1.13 phase 53 originally scoped. UAT-02 verifies the existing scope, not extends it.
- Extending the body-cap to gRPC streams. v1.13's intentional decision was REST-only (skip-list). Don't change that here.
- Automating the UAT-01 CSP check (e.g., via headless Chrome + CDP). Per D-01, user drives the DevTools step manually. If a future phase wants headless automation, that's separate scope.

## Dependencies

- **Depends on**: None hard. UAT-01/02 implicitly assume `/api/*`, `/ui/*`, `/peeringdb.v1.*` are not 500-ing — Phase 73's BUG-01 fix is a soft dependency (campus traversal 500 doesn't block UAT-02 directly, but a healthy `/api/*` surface is the right baseline).
- **Enables**: Closure of v1.13 deferred-items.md and the v1.5 Phase 20 dir; STATE.md "Outstanding Human Verification" backlog shrinks (only v1.6 + v1.7 + v1.11 remain — those are the future "UI verification sweep" milestone).

## Plan hints for executor

- Touchpoints:
  - `.planning/phases/78-uat-closeout/UAT-RESULTS.md` — produce this as the deliverable for UAT-01/02
  - `.planning/milestones/v1.13-phases/52-csp-headers/deferred-items.md` (or wherever v1.13's deferred-items lives) — flip entry to "resolved" with reference to UAT-RESULTS.md
  - `.planning/milestones/v1.13-phases/53-security-headers/deferred-items.md` — same
  - `.planning/milestones/v1.5-phases/20-deferred-human-verification/` — move to archived path
  - `.planning/STATE.md` § Outstanding Human Verification — update to remove v1.5 reference if present (verify; may already be absent)
  - `memory/project_human_verification.md` — flip v1.13 entries from "pending" to "resolved 2026-04-26"
  - `cmd/peeringdb-plus/security_e2e_test.go` (if exists, otherwise create) — automated regression for the curl-driven parts of UAT-02 so we don't rely on manual re-verification next time
- Reference docs:
  - CLAUDE.md § Environment Variables — `PDBPLUS_CSP_ENFORCE` semantics (default false; `true` switches `Content-Security-Policy` from `Report-Only` to enforcing)
  - `internal/middleware/csp.go` (if exists) — implementation of the CSP middleware
  - `memory/project_human_verification.md` — full audit context
  - Original v1.13 phase 52/53 ROADMAP entries
- Verify on completion:
  - UAT-RESULTS.md committed with curl outputs (UAT-02) and user-supplied DevTools confirmation (UAT-01)
  - v1.13 deferred-items entries flipped to resolved
  - v1.5 Phase 20 dir relocated; `git mv` history preserves provenance
  - STATE.md "Outstanding Human Verification" no longer mentions v1.5 (only v1.6/v1.7/v1.11 remain)
  - memory `project_human_verification.md` reflects v1.13 closure
  - If a security_e2e_test.go was created, `go test -race ./cmd/peeringdb-plus/...` clean
