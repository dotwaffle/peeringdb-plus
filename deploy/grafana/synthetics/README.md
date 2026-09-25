# PeeringDB Plus: Synthetic Monitoring check

Source-of-truth definition of the Grafana Synthetic Monitoring check of
the PeeringDB Plus production deployment. Create or change the check in
the Synthetic Monitoring app of the Grafana stack, with the values below.

The check requests a small `/api` response from the public hostname,
through the Fly proxy, from three locations. It sees failures that the
metrics of the fleet cannot see (DNS, TLS, the Fly edge and proxy), and
the response time that a client in each location gets.

| Setting | Value |
|---------|-------|
| Check type | HTTP |
| Job name | `peeringdb-plus-api` |
| Target | `https://peeringdb-plus.fly.dev/api/net?asn=15169` |
| Method | `GET` |
| Probe locations | London, Sydney, and one in the US: New York, else North Virginia or North California |
| Frequency | 3 minutes |
| Timeout | 5 seconds |
| Valid status codes | 200 |
| Body assertion | matches the regular expression `"asn": *15169` |
| Labels | `service=peeringdb-plus` |
| Metrics | Basic (not the full set) |

The Fly proxy usually sends each request to the nearest region with a
ready machine: London to the primary in `lhr`, Sydney to the replica in
`syd`, and a US location to the replica in `iad` (east) or `lax` (west).
Load and health checks can move a request to another region.

## Alert

The alert rule `PdbPlusProbeFailing` (critical,
`deploy/grafana/alerts/pdbplus-alerts.yaml`) fires when the check fails
more than half of its executions in 10 minutes from at least 2 of the 3
locations, for 5 minutes. It selects the check by its job name, so keep
the job name `peeringdb-plus-api`. Do not also turn on the per-check
alerts of the Synthetic Monitoring app, or a failure notifies twice.

## Usage

The Grafana Cloud free plan includes 100,000 API check executions per
month. This check uses 3 locations x 20 executions per hour x 720 hours
= 43,200 executions per month. A frequency of 1 minute would need
129,600 and go above the free plan.

## Forbidden content

The following MUST NOT appear in any file in `deploy/grafana/synthetics/`:

- Email addresses (any user, any domain).
- Hosted Grafana Cloud stack URLs (the per-tenant `grafana dot net`
  subdomain) or any other stack-specific URL.
- Tenant identifiers, API keys, access tokens, or any credential value.
