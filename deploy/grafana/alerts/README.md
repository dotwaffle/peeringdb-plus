# PeeringDB Plus — Grafana Cloud Alert Rules

Source-of-truth Prometheus rule groups for the PeeringDB Plus production
deployment. The repository holds the rule definitions; apply is a manual
operator step (same convention as `deploy/grafana/dashboards/`).

## Schema

Standard Prometheus alerting-rule format. Reference:
https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/

Each rule has:

- `alert:` — PascalCase name with `PdbPlus` prefix.
- `expr:` — PromQL expression.
- `for:` — sustained-breach window before the alert fires.
- `labels.severity:` — one of `critical` (page tier) or `warning`
  (notify-only tier).
- `labels.receiver:` — always the literal string `grafana-default-email`.
  This is the canonical default contact-point name in every Grafana Cloud
  stack; it is **not** an email address.
- `annotations.summary:` — one-line operator summary.
- `annotations.description:` — multi-line operator-actionable detail
  including the threshold value and the contributing metric name.

## Apply

The repository is the source of truth. Sync to Grafana Cloud Mimir with
`mimirtool` (install: https://grafana.com/docs/mimir/latest/manage/tools/mimirtool/):

```bash
# Replace placeholder env vars with your stack credentials before running.
# DO NOT commit real credentials or stack URLs into examples.
export MIMIR_ADDRESS="$YOUR_MIMIR_RULER_URL"
export MIMIR_TENANT_ID="$YOUR_TENANT_ID"
export MIMIR_API_KEY="$YOUR_API_KEY"

mimirtool rules sync deploy/grafana/alerts/pdbplus-alerts.yaml
```

Use `mimirtool rules load` if you prefer load-and-replace semantics over
diff-and-sync.

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

The `severity` label is the routing knob. Configure tier behaviour in your
Grafana Cloud notifications UI (the receiver `grafana-default-email` is
where both tiers land at the receiver level; separate routes can fan out
based on `severity=critical` vs `severity=warning`).

## Severity tier policy

| Tier       | Behaviour       | Used for                                                          |
|------------|-----------------|-------------------------------------------------------------------|
| `critical` | Page on-call    | Sync stalls (>2h freshness), sync-failure rate >50%, fleet drop, telemetry absent, primary absent. |
| `warning`  | Notify only     | Heap/RSS sustained breach, single sync failure.                   |

Total rule count is capped at 8 to stay below Grafana Cloud free-tier
alertmanager limits. Current count: 8 rules across both groups — the
cap is full. Next rule in line if the cap ever rises (paid tier or a
raised limit): a serving-path 5xx-rate alert on
`http_server_request_duration_seconds_count{http_response_status_code=~"5.."}`
— today a fleet returning errors while processes stay alive never
alerts (the closest proxies are the freshness and absence rules).

Note on absence coverage: all metric-presence rules key on
`go_memory_used_bytes`, which ticks on every machine via the OTel
runtime meter. `PdbPlusTelemetryAbsent` is the meta-rule that fires
when the export pipeline itself dies — every other rule evaluates to
an empty vector (and stays silent) in that state.

## Forbidden content

The following MUST NOT appear in any file in `deploy/grafana/alerts/`:

- Email addresses (any user, any domain).
- Hosted Grafana Cloud stack URLs (the per-tenant `grafana dot net`
  subdomain) or any other stack-specific URL.
- Tenant identifiers, API keys, or any credential value.

The repository is a public-style source of truth; tenant-specific
configuration lives in operator-managed environment variables passed to
`mimirtool` at apply time.
