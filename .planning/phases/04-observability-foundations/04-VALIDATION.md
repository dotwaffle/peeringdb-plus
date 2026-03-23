---
phase: 4
slug: observability-foundations
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-22
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — existing go test infrastructure |
| **Quick run command** | `go test ./internal/otel/... ./internal/peeringdb/... ./internal/sync/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/otel/... ./internal/peeringdb/... ./internal/sync/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| TBD | TBD | TBD | OBS-01 | unit | `go test ./internal/otel/...` | ✅ | ⬜ pending |
| TBD | TBD | TBD | OBS-02 | unit | `go test ./internal/peeringdb/...` | ✅ | ⬜ pending |
| TBD | TBD | TBD | OBS-03 | unit | `go test ./internal/sync/...` | ✅ | ⬜ pending |
| TBD | TBD | TBD | OBS-04 | unit | `go test ./internal/sync/...` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements. go test is already configured, OTel test utilities (ManualReader, InMemoryExporter) are available from existing SDK deps.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Metrics visible in OTel backend | OBS-01 | Requires running OTel collector | Deploy, run sync, check Grafana/Jaeger |
| Trace spans visible in tracing backend | OBS-02 | Requires running OTel collector | Deploy, run sync, check Jaeger |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
