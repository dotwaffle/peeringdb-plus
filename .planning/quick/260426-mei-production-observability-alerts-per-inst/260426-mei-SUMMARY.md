---
phase: quick-260426-mei
plan: 01
subsystem: observability
tags: [observability, grafana-cloud, alerting, dashboard, otel-runtime]
requires: []
provides:
  - Production alert rule YAML at deploy/grafana/alerts/pdbplus-alerts.yaml
  - YAML schema + invariants test at deploy/grafana/alerts_test.go
  - Per-instance live heap visibility on dashboard panel 35
  - CLAUDE.md documentation of OTel runtime contrib coexistence
affects:
  - deploy/grafana/dashboards/pdbplus-overview.json (panel 35 re-source)
  - CLAUDE.md (Sync observability section)
tech-stack:
  added: []
  patterns:
    - exec-based optional lint gates (promtool / mimirtool) with t.Skip when absent
    - runtime-concatenated forbidden-token literals so the test does not trip its own grep
key-files:
  created:
    - deploy/grafana/alerts/pdbplus-alerts.yaml
    - deploy/grafana/alerts/README.md
    - deploy/grafana/alerts_test.go
  modified:
    - deploy/grafana/dashboards/pdbplus-overview.json
    - CLAUDE.md
decisions:
  - "Source-of-truth alert YAML in repo + manual mimirtool sync (mirrors dashboards/ convention; no CI integration)"
  - "Two severity tiers (critical / warning) both binding receiver=grafana-default-email; tier routing configured out-of-band in GC UI"
  - "6 rules total (3 critical / 3 warning) under the planner's 8-rule cap (Grafana Cloud free-tier alertmanager limit)"
  - "Panel 35 re-sourced to OTel runtime contrib go_memory_used_bytes (already wired at internal/otel/provider.go:121); NO new Go gauge code"
  - "NO additional dashboard panels added for go_goroutine_count / go_gc_duration_seconds — explicitly deferred per CONTEXT.md"
  - "Forbidden-content test builds the .grafana.net token at runtime via string concatenation so the test source itself does not match the recursive grep"
metrics:
  duration: ~25 minutes
  completed: 2026-04-26
---

# Quick Task 260426-mei: Production Observability — Alerts + Per-instance Runtime Memory Summary

Wired production observability for PeeringDB Plus by authoring Grafana Cloud alert rules in version-controlled Prometheus YAML, re-sourcing dashboard panel 35 to the already-emitting OTel runtime contrib gauge, and documenting the apply mechanism — closing two audit gaps (no paging signal; replicas invisible in heap panel) without introducing any new Go code surface.

## What Shipped

5 file deltas across 4 atomic commits:

| Commit  | Type     | Files                                                                              |
| ------- | -------- | ---------------------------------------------------------------------------------- |
| 17fe1dc | feat     | deploy/grafana/alerts/pdbplus-alerts.yaml (107 lines), README.md (89 lines)        |
| 9cacda6 | test     | deploy/grafana/alerts_test.go (206 lines, 7 test functions)                        |
| 5fc397a | fix      | deploy/grafana/dashboards/pdbplus-overview.json (panel 35 re-source, 4 fields)     |
| d2d337d | docs     | CLAUDE.md (1 paragraph in Sync observability section, 2 line additions)            |

### Alert rules (6 total, 8-rule cap)

**Critical tier (3):**
- `PdbPlusSyncFreshnessHigh` — `max(pdbplus_sync_freshness_seconds) > 7200`, for: 5m
- `PdbPlusSyncFailureRateHigh` — failure-rate / total-rate ratio > 0.5 (clamp_min divide-by-zero guard), for: 15m
- `PdbPlusFleetMachineCountLow` — distinct (service_namespace, cloud_region) pairs < 6 (expected 8), for: 10m

**Warning tier (3):**
- `PdbPlusHeapHigh` — `max(pdbplus_sync_peak_heap_bytes) > 419430400` (400 MiB, SEED-001 trigger), for: 30m
- `PdbPlusRssHigh` — `max(pdbplus_sync_peak_rss_bytes) > 402653184` (384 MiB), for: 30m
- `PdbPlusSyncOperationFailed` — `increase(...[1h]) > 0`, for: 5m

Every rule binds `receiver: grafana-default-email` (the canonical default contact-point name in every Grafana Cloud stack — NOT an email address). Tier routing is configured out-of-band in the operator's GC notifications UI based on the `severity` label.

### Test coverage (`deploy/grafana/alerts_test.go`)

7 test functions, all passing under `go test -race`:

- `TestAlerts_ValidYAML` — parses YAML, asserts non-empty groups
- `TestAlerts_RequiredFields` — every rule has expr, for, severity ∈ {critical, warning}, receiver, summary, description
- `TestAlerts_RuleCountUnderCap` — total ≤ 8
- `TestAlerts_NoForbiddenContent` — no `@gmail.com`, no `@anthropic.com`, no hosted Grafana Cloud stack URL fragment in YAML or README (token built at runtime to avoid self-tripping)
- `TestAlerts_NamesPascalCasePdbPlusPrefix` — names match `^PdbPlus[A-Z][A-Za-z]+$`
- `TestAlerts_PromtoolCheck` — shells to `promtool check rules` (t.Skip when absent — verified locally)
- `TestAlerts_MimirtoolCheck` — shells to `mimirtool rules check` (t.Skip when absent — verified locally)

### Dashboard panel 35 re-source

| Field         | Before                                                                       | After                                                                                              |
| ------------- | ---------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Title         | "Peak Heap by Process Group"                                                 | "Live Heap by Instance"                                                                            |
| Description   | sync-watermark wording                                                       | OTel runtime contrib + semconv v1.36.0 goconv; coexistence note vs sync watermark                  |
| `expr`        | `sum by(service_namespace, cloud_region)(pdbplus_sync_peak_heap_bytes)`      | `sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name="peeringdb-plus"})`   |
| `legendFormat`| `{{service_namespace}} / {{cloud_region}}`                                   | `{{service_namespace}}/{{cloud_region}}`                                                           |

The pre-existing `pdbplus_sync_peak_heap_bytes`-based watermark panels (panel ids elsewhere in the dashboard) stay untouched; the watermark vs live-tick distinction is now explicit in the description.

### CLAUDE.md addition

One paragraph appended to the **Sync observability** section between `**Dashboard.**` and `**OTel resource attributes (post-260426-lod).**`, documenting: the OTel runtime contrib wiring location, the per-instance vs sync-watermark distinction, panel 35's data source, and the new alert rule YAML location + apply mechanism.

## Verification Snapshot

```text
1. files-exist:        ✓ alerts/pdbplus-alerts.yaml, alerts/README.md, alerts_test.go
2. no-forbidden:       ✓ recursive grep across alerts/, dashboards/, CLAUDE.md is clean
3. tests-pass:         ✓ TestAlerts_* (7 functions) under go test -race
4. jq-validates:       ✓ panel 35 .targets[0].expr matches go_memory_used_bytes regex
5. rule-count:         ✓ 6 rules (under 8-rule cap)
6. file-count:         ✓ exactly 5 files changed (3 new + 2 modified)
```

## Deviations from Plan

None. Plan executed exactly as written.

## Deferred Items (per CONTEXT.md)

CONTEXT.md `<decisions>` § "Dashboard updates (in scope)" explicitly deferred the following to a future task:

> Optionally: add a new panel for `go_goroutine_count` and `go_gc_duration_seconds` as part of the SEED-001 watch row — both are signals operators care about. **Decision: defer — out of scope for this quick task.** Open an SEED or future quick task if desired.

CONTEXT.md `<decisions>` § "Claude's Discretion" also deferred:

- `runbook_url` annotations on alert rules (default chosen: inline `description` only; runbook docs are a separate effort)
- Mirroring alert rules into the dashboard as Grafana 10+ alert annotations (default chosen: skip to keep dashboard JSON clean)

### Out-of-scope pre-existing failure observed

`TestDashboard_RegionVariableUsed` (`deploy/grafana/dashboard_test.go:316`) fails on `main` before this task's commits — it asserts a `fly_region` label reference that no longer exists post quick task 260426-lod (the label migration moved region grouping to `cloud_region`). Logged in `deferred-items.md` for a follow-up touch-up to update the test to the new label name; not addressed here per the deviation-rule scope-boundary (only DIRECTLY caused issues).

## Single Manual Operator Follow-up

The repository is the source of truth; apply is operator-side. After merging:

```bash
# Set stack credentials in your environment first.
export MIMIR_ADDRESS="$YOUR_MIMIR_RULER_URL"
export MIMIR_TENANT_ID="$YOUR_TENANT_ID"
export MIMIR_API_KEY="$YOUR_API_KEY"

mimirtool rules sync deploy/grafana/alerts/pdbplus-alerts.yaml
```

This pushes the rule groups to Grafana Cloud Mimir. The `severity` label routing is configured in the GC notifications UI (already present from the post-260426-lod observability cleanup).

## Self-Check: PASSED

- ✓ deploy/grafana/alerts/pdbplus-alerts.yaml exists
- ✓ deploy/grafana/alerts/README.md exists
- ✓ deploy/grafana/alerts_test.go exists
- ✓ deploy/grafana/dashboards/pdbplus-overview.json modified (panel 35)
- ✓ CLAUDE.md modified (Sync observability section)
- ✓ commit 17fe1dc (feat) found in git log
- ✓ commit 9cacda6 (test) found in git log
- ✓ commit 5fc397a (fix) found in git log
- ✓ commit d2d337d (docs) found in git log
