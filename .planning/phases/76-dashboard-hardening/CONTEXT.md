---
phase: 76
slug: dashboard-hardening
milestone: v1.18.0
status: context-locked
has_context: true
locked_at: 2026-04-26
---

# Phase 76 Context: Dashboard Hardening

## Goal

Make the `pdbplus-overview` dashboard immune to cross-application metric collision by adding a `service_name` filter sweep across all `go_*` panel queries, and confirm that the post-bytes-canonicalisation `pdbplus_response_heap_delta_bytes_bucket` is flowing on the v1.17.0+ binary (the legacy `_kib_KiB_bucket` series was retention noise).

## Requirements

- **OBS-03** — All `go_*` panel queries filter by `service_name="peeringdb-plus"`
- **OBS-05** — Confirm `pdbplus_response_heap_delta_bytes_bucket` flowing on v1.17.0+ binary

## Locked decisions

- **D-01 — OBS-03: both template variable + per-panel reference.** Add a global `$service` template variable to `pdbplus-overview.json` with default value `peeringdb-plus`, sourced as a Prometheus label-values query on `service_name`. Update every `go_*` PromQL query to reference `{service_name="$service"}`. Two-edge approach:
  - **Template var:** single edit point if the service name ever changes (e.g., a fork/rebrand), and lets future operators filter to a different application within the same Grafana stack via the dropdown.
  - **Per-panel reference:** explicit `{service_name="$service"}` in every panel's PromQL is grep-able from the JSON; no hidden global filter.
  
  The current dashboard relies on a fragile coincidence — syncthing on the local laptop scrapes `go_goroutines` (plural) while peeringdb-plus emits `go_goroutine_count` (singular), so the namespaces don't currently collide. Any future scrape target sharing metric names would silently double-count without this fix.

- **D-02 — OBS-05: confirm only.** Verify `count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1.17.0|v1.18.*"})` returns non-zero during normal pdbcompat list traffic. No code changes expected unless flow is broken. **No documentation in panel description, no Prom drop rule for the legacy `_kib_KiB_bucket` series** — user explicitly chose "confirm only" over the recommended "confirm + document". Rationale: retention will expire the legacy series naturally; documentation in panel-text would just be noise.

## Out of scope

- Active cleanup of the legacy `pdbplus_response_heap_delta_kib_KiB_bucket` series via Prometheus `metric_relabel_configs` drop rules — out of scope per D-02.
- Documentation in dashboard panel descriptions about the legacy metric — out of scope per D-02.
- Adding new `go_*` panels — this phase hardens existing queries, doesn't expand coverage.
- Changing the dashboard layout, panel sizing, or row organization — purely a query-correctness pass.
- Updating the alert rules in `deploy/grafana/alerts/pdbplus-alerts.yaml` to add `service_name` filters — those are alert rules, separate concern; check if they need the same treatment but defer to a follow-up if so.

## Dependencies

- **Depends on**: None hard. Soft dependency on Phase 75 (OBS-01, OBS-02 fixes) — visual confirmation of those panels rendering correctly is natural QA for the OBS-03 filter sweep. Run order: 75 → 76, but the phases can technically execute independently.
- **Enables**: Phase 77's audit work has cleaner data to inspect after the filter sweep removes any cross-application contamination from past data.

## Plan hints for executor

- Touchpoints:
  - `deploy/grafana/dashboards/pdbplus-overview.json` — add `$service` to `templating.list`; update every `go_*` panel's `targets[].expr` to include `{service_name="$service"}`. Affected panels (per the 2026-04-26 audit): "Goroutines", "Heap Memory", "Allocation Rate", "GC Goal", "Live Heap by Instance" (the last one already filters on `service_name="peeringdb-plus"` per the post-260426-lod fix — verify still consistent).
  - `deploy/grafana/dashboard_test.go` — add an invariant asserting that all `go_*` PromQL queries reference `service_name`; failure surface a panel without the filter.
- Reference docs:
  - `deploy/grafana/dashboards/pdbplus-overview.json` panel 35 (Live Heap by Instance) — already uses `service_name="peeringdb-plus"`; mirror the pattern.
  - 2026-04-26 telemetry audit findings (this session) — empirical confirmation of the syncthing/peeringdb-plus metric-name distinction.
- Verify on completion:
  - `jq '.panels[].targets[].expr | select(test("go_"))' deploy/grafana/dashboards/pdbplus-overview.json` returns expressions, ALL containing `service_name`
  - `go test ./deploy/grafana/...` clean (with the new invariant test)
  - Live verification: query a `go_*` metric in Grafana with the `$service` filter set vs unset; results identical for `peeringdb-plus` (no cross-contamination cleanup needed since none currently flowing)
  - Live verification: `count(pdbplus_response_heap_delta_bytes_bucket{service_version=~"v1.17.0|v1.18.*"}) > 0` during normal `/api/*` traffic (OBS-05 confirmation)
