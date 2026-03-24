---
phase: 19
slug: prometheus-metrics-grafana-dashboard
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-03-24
---

# Phase 19 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test |
| **Config file** | none — standard Go test infrastructure |
| **Quick run command** | `go test ./internal/otel/... ./cmd/pdbplus/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/otel/... ./cmd/pdbplus/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 19-01-01 | 01 | 1 | OBS-01 | unit | `go test ./internal/otel/...` | ✅ | ⬜ pending |
| 19-01-02 | 01 | 1 | OBS-08 | unit | `go test -run TestTypeCountGauge ./internal/otel/...` | ❌ W0 | ⬜ pending |
| 19-02-01 | 02 | 2 | OBS-02,OBS-03 | file | `test -f deploy/grafana/peeringdb-plus.json` | ❌ W0 | ⬜ pending |
| 19-02-02 | 02 | 2 | OBS-04,OBS-05 | json | `python3 -c "import json; json.load(open('deploy/grafana/peeringdb-plus.json'))"` | ❌ W0 | ⬜ pending |
| 19-02-03 | 02 | 2 | OBS-06,OBS-07 | grep | `grep -c 'text_panel' deploy/grafana/peeringdb-plus.json` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/otel/metrics_test.go` — test for type count gauge registration
- [ ] `deploy/grafana/` directory — dashboard JSON artifacts

*Existing test infrastructure covers OTel initialization. New tests needed for business metric gauge and dashboard validation.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Fly.io Prometheus scraper collects metrics | OBS-01 | Requires live deployment | Deploy to Fly.io, check Grafana Cloud for metric ingestion |
| Dashboard displays live data | OBS-09, OBS-10 | Requires Grafana Cloud import | Import JSON into Grafana, verify panels render with real data |
| fly_region template variable works | OBS-02 | Requires multi-region deployment | Check $region dropdown in imported dashboard |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
