# PeeringDB Plus: Synthetic Monitoring check

Source-of-truth definition of the Grafana Synthetic Monitoring check of the PeeringDB Plus production deployment.
Create or change the check in the Synthetic Monitoring app of the Grafana stack, with the values below.

The check requests a small `/api` response from the public hostname, through the Fly proxy, from five locations near the users of the service.
It sees failures that the metrics of the fleet cannot see (DNS, TLS, the Fly edge and proxy), and the response time that a client in each location gets.

| Setting | Value |
|---------|-------|
| Check type | HTTP |
| Job name | `peeringdb-plus-api` |
| Target | `https://peeringdb-plus.fly.dev/api/net?asn=15169` |
| Method | `GET` |
| IP version | IPv4 |
| Probe locations | London, North Virginia, North California, São Paulo, Sydney |
| Frequency | 5 minutes |
| Timeout | 5 seconds |
| Valid status codes | 200 |
| Fail if not SSL | Yes |
| Body assertion | matches the regular expression `"asn": *15169` |
| Labels | `service=peeringdb-plus` |
| Metrics | Basic (not the full set) |

The Fly proxy usually sends each request to the nearest region with a ready machine: London to the primary in `lhr`, North Virginia to the replica in `iad`, North California to the replica in `lax`, São Paulo to the replica in `gru`, and Sydney to the replica in `syd`.
Load and health checks can move a request to another region.

The probes run in AWS, and the route from AWS to the Fly anycast addresses selects the Fly edge.
In September 2026 (request header `flyio-debug: doit`, field `nr` of the `flyio-debug` response header), the London probe reached the edge in Frankfurt (`fra`), which adds about 13 ms to the connect time and about 40 ms to the response time.
A Singapore probe, tested and left out, reached the edge in Paris (`cdg`) and the primary in `lhr`, not the replica in `sin`, over IPv4 and IPv6.

## Alert

The alert rule `PdbPlusProbeFailing` (critical, `deploy/grafana/alerts/pdbplus-alerts.yaml`) fires when the check fails more than half of its executions in 15 minutes (3 executions) from at least 2 of the 5 locations, for 5 minutes.
It selects the check by its job name, so keep the job name `peeringdb-plus-api`.
Do not also turn on the per-check alerts of the Synthetic Monitoring app, or a failure notifies twice.

## Usage

The Grafana Cloud free plan includes 100,000 API check executions per month.
This check uses 5 locations x 12 executions per hour x 720 hours = 43,200 executions per month.
A frequency of 3 minutes would need 72,000.
With basic metrics, each location adds about 40 active series.

## Forbidden content

The following MUST NOT appear in any file in `deploy/grafana/synthetics/`:

- Email addresses (any user, any domain).
- Hosted Grafana Cloud stack URLs (the per-tenant `grafana dot net`
  subdomain) or any other stack-specific URL.
- Tenant identifiers, API keys, access tokens, or any credential value.
