# Deferred items observed during 260427-vvx execution

## Pre-existing test failures in deploy/grafana (NOT in scope)

`go test ./deploy/grafana/...` fails on commit `a3a3acf` ("deploy(grafana): fix three broken dashboard panels and remove Guide bloat"), which landed before this task started. Confirmed by checking out HEAD~3 (commit `e21ac68`) and seeing the same package pass.

Failures:
- `TestDashboard_MetricNameReferences`: dashboard missing metric `pdbplus_sync_type_duration_seconds` (per-type sync duration) in PromQL expressions
- `TestDashboard_EachRowHasTextPanel`: 5 rows missing documentation text panels (Sync Health, HTTP RED Metrics, Per-Type Sync Detail, Go Runtime, Business Metrics)

These are unrelated to `cmd/loadtest/` and the SCOPE BOUNDARY rule prevents me from fixing them in this commit. Owner of `a3a3acf` should follow up.
