# PeeringDB Plus: Grafana SLOs

Source-of-truth definitions of the service level objectives (SLOs) of the
PeeringDB Plus production deployment. Each JSON file is a request body for
the SLO API of the Grafana SLO app. Apply is a manual operator step, as for
`deploy/grafana/alerts/` and `deploy/grafana/dashboards/`.

| File | SLO | Objective | SLI type | Alerts |
|------|-----|-----------|----------|--------|
| `availability.json` | PeeringDB Plus availability | 99.9% over 28 days | Event-based (ratio) | Fast burn (`critical`), slow burn (`warning`) |
| `freshness.json` | PeeringDB Plus data freshness | 99.5% over 28 days | Time-based (freeform) | None |

## Apply

The SLO API is at `/api/plugins/grafana-slo-app/resources/v1/slo` on the
Grafana stack. It needs a service account token with the Editor role.

- To create an SLO, send the file with `POST`. The response contains the
  `uuid` of the new SLO. The files have no `uuid` field: the published
  OpenAPI schema lists it as required, but the create example of the SLO
  API documentation omits it.
- To change an SLO, send the file with `PUT` to `/v1/slo/<uuid>`, with the
  `uuid` field added. `GET /v1/slo` lists the SLOs and their `uuid`
  values: match them by `name`.

For each SLO, Grafana makes recording rules, an SLO dashboard and (for
availability) the burn-rate alert rules, in the folder "PeeringDB Plus"
(`folder.uid`). The recording rules write to `grafanacloud-prom` and add
series to the metrics usage of the stack.

## Availability

The SLI is the share of routed HTTP requests that return a status below
500. Good and total requests come from
`http_server_request_duration_seconds_count`, which otelhttp records on
every machine. The selector leaves out these requests:

- Health probes (`GET /healthz`, `GET /readyz`, `/grpc.health.*`): a 503
  there is a readiness verdict for the Fly proxy, not a failed user
  request.
- `POST /sync`: an operator endpoint.
- Requests with no `http_route` (the label is empty): 404 and 405
  responses to paths that no route matches, and the 503 of the readiness
  gate on a machine that has not synced yet. The Fly proxy sends no
  traffic to a machine that fails `/readyz`.
- Synthetic Monitoring probe requests
  (`user_agent_synthetic_type="test"`, see
  `deploy/grafana/synthetics/README.md`): the probe has its own alert,
  `PdbPlusProbeFailing`.

A 404 under a matched route counts as good, for example
`/api/<unknown type>` or a unique query with no row: 935 of the 62,500
`/api` requests in the 28 days to 2026-09-25. The dashboard panel "Error
Rate (5xx)" uses the same selector.

These failures do not count as bad:

- A panic after the handler started the response: the metric records
  the status already sent. A panic before that counts as a 500 with its
  route (the inner Recovery sits inside otelhttp).
- A panic in a middleware before the mux dispatch: the 500 has no
  `http_route`.
- A ConnectRPC stream that fails after it started: the error goes in
  the trailers of an HTTP 200.

The burn-rate alerts replace a separate 5xx alert rule:

- Fast burn (`severity: critical`): the burn rate is at least 14.4 over
  1h and 5m, or at least 6 over 6h and 30m.
- Slow burn (`severity: warning`): the burn rate is at least 3 over 24h
  and 2h, or at least 1 over 72h and 6h.

Both need at least 5 failed requests (`minFailures`), so that one error at
low traffic does not fire them. In the 28 days to 2026-09-25 the fleet
served about 65,000 routed requests and the SLI was 99.991% (6 responses
with status 500). The error budget of 0.1% is then about 65 failed
requests in 28 days.

## Freshness

The SLI is time-based: for each interval, it is 1 when the newest
successful sync is less than 1 hour old on every machine that reports
the gauge (`max(pdbplus_sync_freshness_seconds) < 3600`), else 0. The
sync runs every 15 minutes, and the highest freshness in the week to
2026-09-25 was 17 minutes.

A machine that does not report the gauge (down, telemetry lost, or no
successful sync yet) does not count. When no machine reports it, the
interval has no data and does not count either. The fleet and telemetry
alert rules cover those cases.

The SLO app does not make alert rules for time-based SLIs. The alert rule
`PdbPlusSyncFreshnessHigh` (freshness above 2 hours) stays the freshness
alert.

## Forbidden content

The following MUST NOT appear in any file in `deploy/grafana/slos/`:

- Email addresses (any user, any domain).
- Hosted Grafana Cloud stack URLs (the per-tenant `grafana dot net`
  subdomain) or any other stack-specific URL.
- Tenant identifiers, API keys, or any credential value.
