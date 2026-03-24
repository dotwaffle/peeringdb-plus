# Phase 3: Production Readiness - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-22
**Phase:** 03-production-readiness
**Areas discussed:** OTel setup, Health & monitoring, Fly.io deployment

---

## OTel Setup

| Option | Description | Selected |
|--------|-------------|----------|
| OTLP | Export via OTLP protocol | ✓ |
| Stdout/JSON | Log telemetry to stdout | |
| Both | OTLP + stdout configurable | |

**User's choice:** OTLP

---

| Option | Description | Selected |
|--------|-------------|----------|
| Always sample | 100% traces | |
| Ratio-based | Configurable ratio | |
| Configurable | Env var, default always | ✓ |

**User's choice:** Configurable

---

| Option | Description | Selected |
|--------|-------------|----------|
| Bridge to OTel | slog through OTel pipeline | |
| Keep separate | slog to stdout only | |
| Other | Both OTel pipeline AND stdout, with individual signal disabling | ✓ |

**User's choice:** Dual output (OTel + stdout), allow disabling tracing/metrics/logs individually

---

| Option | Description | Selected |
|--------|-------------|----------|
| HTTP + sync + runtime | All three metric categories | ✓ |
| HTTP + sync only | Skip runtime metrics | |

**User's choice:** HTTP + sync + runtime

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, internal/otel | Centralized package | ✓ |
| Inline in main | Setup in main() | |

**User's choice:** internal/otel

---

| Option | Description | Selected |
|--------|-------------|----------|
| Standard env vars | OTEL_EXPORTER_OTLP_ENDPOINT | ✓ |
| Custom env vars | PDBPLUS_OTLP_ENDPOINT | |

**User's choice:** Standard env vars

---

| Option | Description | Selected |
|--------|-------------|----------|
| ldflags | Build-time injection | |
| Runtime detection | debug.ReadBuildInfo() | ✓ |
| Both | ldflags + fallback | |

**User's choice:** Runtime detection

---

| Option | Description | Selected |
|--------|-------------|----------|
| W3C TraceContext | Standard propagation | ✓ |
| Not needed | Single service | |

**User's choice:** W3C TraceContext

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Include FLY_REGION, FLY_MACHINE_ID, FLY_APP_NAME | ✓ |
| No | Generic attributes only | |

**User's choice:** Yes

---

## Health & Monitoring

| Option | Description | Selected |
|--------|-------------|----------|
| Separate | /healthz + /readyz | |
| Combined | Single /health | |
| You decide | Claude's discretion | ✓ |

**User's choice:** Claude's discretion

---

| Option | Description | Selected |
|--------|-------------|----------|
| 2 hours | Fail if > 2h stale | |
| 4 hours | More lenient | |
| Configurable | Env var, default 24h | ✓ |

**User's choice:** Configurable, default 24 hours

---

| Option | Description | Selected |
|--------|-------------|----------|
| Detailed JSON | Component status in JSON | ✓ |
| Simple pass/fail | Status code only | |

**User's choice:** Detailed JSON with appropriate HTTP status codes
**Notes:** HTTP status code must reflect overall health for easy load balancer config

---

| Option | Description | Selected |
|--------|-------------|----------|
| OTel push only | No Prometheus endpoint | ✓ |
| Both | OTLP + /metrics | |

**User's choice:** OTel push only

---

## Fly.io Deployment

| Option | Description | Selected |
|--------|-------------|----------|
| Single region first | One region, add more later | ✓ |
| 3 regions | Global from day one | |
| Configurable | User decides at deploy time | |

**User's choice:** Single region first

---

| Option | Description | Selected |
|--------|-------------|----------|
| shared-cpu-1x 256MB | Smallest | |
| shared-cpu-1x 512MB | More headroom | ✓ |
| Configurable | User adjusts | |

**User's choice:** shared-cpu-1x 512MB

---

| Option | Description | Selected |
|--------|-------------|----------|
| FUSE mount + proxy | Standard LiteFS approach | |
| Static leasing | Consul for leader election | ✓ |

**User's choice:** Static leasing with Consul

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, template | fly.toml with defaults | ✓ |
| No | Users create own | |

**User's choice:** Yes, template

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Litestream to Tigris | |
| No | Re-sync from PeeringDB | ✓ |

**User's choice:** No backup needed

---

| Option | Description | Selected |
|--------|-------------|----------|
| LiteFS as supervisor | LiteFS starts first, launches app | ✓ |
| Separate processes | Independent processes | |

**User's choice:** LiteFS as supervisor

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, Fly volume | Persistent at /data | ✓ |
| Ephemeral | Re-sync on deploy | |

**User's choice:** Yes, Fly volume

---

| Option | Description | Selected |
|--------|-------------|----------|
| GitHub Actions | Auto deploy on push | |
| Manual deploy | fly deploy manually | ✓ |
| Both | Manual now, add Actions | |

**User's choice:** Manual deploy

---

| Option | Description | Selected |
|--------|-------------|----------|
| Update existing | One Dockerfile | |
| Separate prod | Dockerfile.prod with LiteFS | ✓ |

**User's choice:** Separate Dockerfile.prod

---

| Option | Description | Selected |
|--------|-------------|----------|
| LiteFS lease file | .primary file | ✓ |
| Env var | FLY_PRIMARY | |

**User's choice:** LiteFS lease file

---

| Option | Description | Selected |
|--------|-------------|----------|
| Fixed count | Predictable, manual | ✓ |
| Auto-scale | Min/max with scaling | |

**User's choice:** Fixed count

---

| Option | Description | Selected |
|--------|-------------|----------|
| LiteFS proxy | Automatic fly-replay | |
| App handles it | Check primary, redirect | ✓ |

**User's choice:** App handles write-forwarding internally

---

| Option | Description | Selected |
|--------|-------------|----------|
| Standard Fly.io Consul | Built-in, no config | ✓ |
| Configurable | Override via env var | |

**User's choice:** Standard Fly.io Consul

---

## Claude's Discretion

- Health endpoint path design
- OTel metric names and histogram buckets
- LiteFS litefs.yml configuration
- fly.toml region and machine defaults
- Individual OTel signal disable env var naming

## Deferred Ideas

None — discussion stayed within phase scope
