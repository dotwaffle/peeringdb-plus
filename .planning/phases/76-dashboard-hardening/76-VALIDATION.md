---
phase: 76
slug: dashboard-hardening
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-27
---

# Phase 76 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | `deploy/grafana/dashboard_test.go` (existing) |
| **Quick run command** | `go test ./deploy/grafana/...` |
| **Full suite command** | `go test ./deploy/grafana/...` |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./deploy/grafana/...`
- **After every plan wave:** Run `go test ./deploy/grafana/...`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

> Filled in by gsd-planner from RESEARCH.md's Validation Architecture section.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 76-01-01 | 01 | 1 | OBS-03 | — | N/A | unit | `go test ./deploy/grafana/... -run TestDashboard_GoMetricsFilterByService` | ❌ W0 | ⬜ pending |

---

## Wave 0 Requirements

- [ ] `deploy/grafana/dashboard_test.go` — add `TestDashboard_GoMetricsFilterByService` (RED: fails before JSON edits, GREEN: passes after $service var + per-panel filter)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1.17.0\|v1.18.*"}) > 0` against live Grafana Cloud Prom | OBS-05 | Confirms metric flow on prod binary; Prom endpoint requires GC API token | Per RESEARCH.md § Live confirmation — `mimirtool query instant` against Mimir endpoint with `GRAFANA_CLOUD_API_TOKEN` |
| Visual confirmation: `$service` dropdown appears in dashboard chrome with `peeringdb-plus` selected by default | OBS-03 | Grafana UI render; cannot be unit-tested from JSON shape alone | Open `https://dotwaffle.grafana.net/d/<uid>/pdbplus-overview` after panel JSON pushed |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
