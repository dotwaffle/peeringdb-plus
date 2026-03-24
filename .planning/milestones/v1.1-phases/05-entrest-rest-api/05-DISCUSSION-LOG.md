# Phase 5: entrest REST API - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-22
**Phase:** 05-entrest-rest-api
**Areas discussed:** URL structure, Read-only enforcement, Schema annotations, Response format, Server integration, Testing

---

## URL Structure

### REST prefix

| Option | Description | Selected |
|--------|-------------|----------|
| /rest/ | Clear separation from GraphQL and PeeringDB compat | |
| /v1/ | Versioned prefix | |
| /rest/v1/ | User's choice — both namespace separation and versioning | ✓ |

**User's choice:** /rest/v1/ (custom answer combining both)

### Resource naming

| Option | Description | Selected |
|--------|-------------|----------|
| Use entrest defaults | Let entrest generate from ent schema names | ✓ |
| Custom short names | Override awkward names like /internet-exchanges | |
| You decide | Claude reviews and overrides where needed | |

**User's choice:** Use entrest defaults

### Root discovery

| Option | Description | Selected |
|--------|-------------|----------|
| Add to root | Add rest and openapi fields to GET / | |
| No, separate | REST self-discoverable via OpenAPI spec | ✓ |

**User's choice:** No, separate

---

## Read-Only Enforcement

| Option | Description | Selected |
|--------|-------------|----------|
| Config-level only | DefaultOperations = Read+List, no write handlers | ✓ |
| Config + middleware | Config restriction plus method-rejecting middleware | |

**User's choice:** Config-level only

---

## Schema Annotations

### Annotation granularity

| Option | Description | Selected |
|--------|-------------|----------|
| Minimal — schema-level | Schema-level annotations, auto-discover fields | ✓ |
| Field-level control | Per-field filterable/sortable/included annotations | |

**User's choice:** Minimal — schema-level

### Edge eager-loading

| Option | Description | Selected |
|--------|-------------|----------|
| All edges | All relationship edges loadable via query params | ✓ |
| Key edges only | Only commonly-traversed edges | |

**User's choice:** All edges

### Pagination

| Option | Description | Selected |
|--------|-------------|----------|
| Default + max | Default 250, max 1000 | |
| entrest defaults | Use whatever entrest generates | ✓ |

**User's choice:** entrest defaults

---

## Response Format

### JSON fields

| Option | Description | Selected |
|--------|-------------|----------|
| Include as-is | Raw JSON arrays/objects in responses | ✓ |
| Exclude from REST | Omit complex JSON fields | |

**User's choice:** Include as-is

### Content type

| Option | Description | Selected |
|--------|-------------|----------|
| JSON only | Always application/json | ✓ |
| Negotiate | Support JSON and HTML | |

**User's choice:** JSON only

### Null handling

| Option | Description | Selected |
|--------|-------------|----------|
| Include as null | All fields always present, nullable as null | ✓ |
| Omit empty | Skip null/zero-value fields | |

**User's choice:** Include as null

### OpenAPI descriptions

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, from comments | Use ent field Comment() as OpenAPI descriptions | ✓ |
| No descriptions | Just types and names | |

**User's choice:** Yes, from comments

### Error format

| Option | Description | Selected |
|--------|-------------|----------|
| entrest defaults | Use entrest's default error format | |
| RFC 7807 | Problem Details for HTTP APIs | ✓ |

**User's choice:** RFC 7807

---

## Server Integration

### Mux mounting

| Option | Description | Selected |
|--------|-------------|----------|
| Shared mux | Mount on same mux as /graphql, /healthz | ✓ |
| Separate mux | Create second mux for REST | |

**User's choice:** Shared mux

### Readiness gate

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, gate REST too | 503 until first sync, consistent with GraphQL | ✓ |
| No, always available | Available immediately, may return empty | |

**User's choice:** Yes, gate REST too

### CORS

| Option | Description | Selected |
|--------|-------------|----------|
| Inherit | Same middleware stack, CORS applies automatically | |
| Separate CORS | Independent CORS instance, same config, independently configurable | ✓ |

**User's choice:** Separate CORS — same config as GraphQL but independently configurable for future changes

---

## Testing

### Codegen coexistence

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, explicit test | Test that go generate works with both extensions | ✓ |
| Compile check only | If it compiles, it works | |

**User's choice:** Yes, explicit test

### Integration test depth

| Option | Description | Selected |
|--------|-------------|----------|
| Full integration tests | Test list, read, pagination, filtering, sorting, eager-loading, errors | ✓ |
| Smoke tests only | Verify 200 with valid JSON | |

**User's choice:** Full integration tests

---

## Claude's Discretion

None — all decisions made by user.

## Deferred Ideas

None — discussion stayed within phase scope.
