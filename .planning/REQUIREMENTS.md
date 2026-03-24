# Requirements: PeeringDB Plus

**Defined:** 2026-03-24
**Core Value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## v1.3 Requirements

Requirements for v1.3 PeeringDB API Key Support milestone. Each maps to roadmap phases.

### API Key Configuration

- [x] **KEY-01**: Setting `PDBPLUS_PEERINGDB_API_KEY` causes all PeeringDB API requests to include `Authorization: Api-Key <key>` header
- [x] **KEY-02**: When no API key is configured, sync and conformance tools work identically to current behavior (no auth header)

### Rate Limiting

- [x] **RATE-01**: When an API key is configured, the HTTP client rate limiter increases from 20 req/min to a higher authenticated threshold
- [x] **RATE-02**: When no API key is configured, the rate limiter remains at 20 req/min

### Startup Validation

- [x] **VALIDATE-01**: When PeeringDB rejects the API key (401/403), the error is logged clearly with the status code and a message indicating the key may be invalid

### Conformance Tooling

- [x] **CONFORM-01**: The `pdbcompat-check` CLI accepts an API key flag or env var and uses it for PeeringDB requests
- [x] **CONFORM-02**: The `-peeringdb-live` integration test uses the API key when available for higher rate limits

## Out of Scope

| Feature | Reason |
|---------|--------|
| Per-user API key management | Server-side config, not multi-tenant |
| API key rotation or refresh | Restart to change key — simple and sufficient |
| Exposing API key status to end users | Internal operational detail |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| KEY-01 | Phase 11 | Complete |
| KEY-02 | Phase 11 | Complete |
| RATE-01 | Phase 11 | Complete |
| RATE-02 | Phase 11 | Complete |
| VALIDATE-01 | Phase 11 | Complete |
| CONFORM-01 | Phase 12 | Complete |
| CONFORM-02 | Phase 12 | Complete |

**Coverage:**
- v1.3 requirements: 7 total
- Mapped to phases: 7
- Unmapped: 0

---
*Requirements defined: 2026-03-24*
