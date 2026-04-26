# Deferred Items — Quick Task 260426-mei

## Pre-existing test failures (out of scope)

- `TestDashboard_RegionVariableUsed` (`deploy/grafana/dashboard_test.go:316`) fails on
  `main` before this task's commits — no PromQL expression references the `fly_region`
  label, so the `$region` template variable is unused. The post-260426-lod label
  migration moved region grouping to `cloud_region`, leaving the test stale. The test
  needs an update (assert against `cloud_region` instead) or the template variable
  needs reworking; either fix is out of scope for this observability/alerting task.

  Reproduce: `cd deploy/grafana && go test -race -count=1 -run TestDashboard_RegionVariableUsed .`
