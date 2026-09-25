# PeeringDB Plus: Grafana Cloud Alert Rules

Source-of-truth Prometheus rule groups for the PeeringDB Plus production
deployment. The repository holds the rule definitions; apply is a manual
operator step (same convention as `deploy/grafana/dashboards/`).

## Schema

Standard Prometheus alerting-rule format. Reference:
https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/

Each rule has:

- `alert:`: PascalCase name with the `PdbPlus` prefix.
- `expr:`: PromQL expression.
- `for:`: sustained-breach window before the alert fires.
- `keep_firing_for:` (optional): how long the alert stays firing after
  the expression stops matching.
- `labels.severity:`: `critical` or `warning` (see the tier table below).
- `annotations.summary:`: one-line operator summary.
- `annotations.description:`: operator-actionable detail, including the
  threshold value and the metric name.

Rules have no receiver label. Grafana notification policies route the
alerts.

## Apply

In production, the rules are Grafana-managed alert rules in the folder
"PeeringDB Plus", in the groups `pdbplus-critical` and `pdbplus-warning`
(evaluated every minute). The UID of each rule is `pdbplus-` followed by
the alert name without the prefix, in kebab case: `PdbPlusSyncOperationFailed`
is `pdbplus-sync-operation-failed`. Each rule has two queries:

- `A`: the `expr` as an instant query on the Prometheus data source.
- `B`: a threshold expression, `A > 0`. `B` is the condition.

No data is `OK`. An evaluation error is `Alerting`.

When you change this file, update the Grafana-managed rule with the same
UID to match it, in the Grafana UI or through the alerting provisioning
API.

Do not also load this file into the Mimir ruler with `mimirtool rules
sync`. That makes a second copy of each rule, and each alert then
notifies twice. A deployment without Grafana-managed alerting can use
`mimirtool rules sync` instead (install:
https://grafana.com/docs/mimir/latest/manage/tools/mimirtool/), with the
stack credentials in environment variables.

## Lint locally

Either tool is sufficient; both are external binaries (not in the Go
toolchain). The repository test (`alerts_test.go`) shells to whichever is
on `PATH` and skips gracefully when neither is installed.

```bash
mimirtool rules check deploy/grafana/alerts/pdbplus-alerts.yaml
promtool check rules deploy/grafana/alerts/pdbplus-alerts.yaml
```

Test invocation:

```bash
go test -race ./deploy/grafana/...
```

## Notification routing

The `severity` label is the routing key. The notification policies of
the Grafana stack decide where each tier goes. When both tiers use the
default policy, they go to the same contact point.

## Severity tier policy

| Tier       | Meaning                    | Used for                                                          |
|------------|----------------------------|-------------------------------------------------------------------|
| `critical` | Act now                    | Sync stalls (>2h freshness), sync keeps failing, fleet drop, telemetry absent, primary absent, /api check failing from 2+ probe locations. |
| `warning`  | Look during working hours  | Heap/RSS sustained breach on the primary, replica memory high, replica LiteFS lag >10 min, primary volume <512 MiB free, 2 failed sync attempts in 3h. |

The 5xx responses have no rule in this file. The burn-rate alert rules of
the availability SLO (`deploy/grafana/slos/`) alert on them, with the same
two tiers: fast burn is `critical`, slow burn is `warning`.

`PdbPlusProbeFailing` reads the metrics of the Synthetic Monitoring check
in `deploy/grafana/synthetics/README.md`. Until the check exists, the rule
has no data and stays `OK`.

Note on absence coverage: all metric-presence rules key on
`go_memory_used_bytes`, which ticks on every machine via the OTel
runtime meter. `PdbPlusTelemetryAbsent` is the meta-rule that fires
when the export pipeline itself dies; every other rule evaluates to
an empty vector (and stays silent) in that state.

## Forbidden content

The following MUST NOT appear in any file in `deploy/grafana/alerts/`:

- Email addresses (any user, any domain).
- Hosted Grafana Cloud stack URLs (the per-tenant `grafana dot net`
  subdomain) or any other stack-specific URL.
- Tenant identifiers, API keys, or any credential value.

The repository is a public-style source of truth. Contact points,
notification policies and stack credentials stay in the Grafana stack.
