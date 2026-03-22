# Phase 2: GraphQL API - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-22
**Phase:** 02-graphql-api
**Areas discussed:** Query design, Playground & DX, Server setup

---

## Query Design

| Option | Description | Selected |
|--------|-------------|----------|
| Relay cursor | Cursor-based with edges/nodes/pageInfo | |
| Offset/limit | Simple offset+limit | |
| Both | Support both cursor and offset pagination | ✓ |

**User's choice:** Both

---

| Option | Description | Selected |
|--------|-------------|----------|
| camelCase | infoPrefixes4, irrAsSet — idiomatic GraphQL | ✓ |
| snake_case | info_prefixes4 — matches PeeringDB | |

**User's choice:** camelCase

---

| Option | Description | Selected |
|--------|-------------|----------|
| entgql WhereInput | Generated WhereInput types, type-safe | ✓ |
| Custom filter args | Hand-craft filter arguments per type | |

**User's choice:** entgql WhereInput

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, with limits | Max depth + complexity score limits | ✓ |
| No limits | Let users query freely | |

**User's choice:** Yes, with limits

---

| Option | Description | Selected |
|--------|-------------|----------|
| Dedicated query | Top-level networkByAsn | |
| Filter only | Use networks(where: {asn: 42}) | |
| Both | Dedicated query + filter | ✓ |

**User's choice:** Both

---

| Option | Description | Selected |
|--------|-------------|----------|
| PeeringDB ID only | Node ID = PDB ID | |
| Relay Global ID | Opaque base64 type:id | ✓ |

**User's choice:** Relay Global ID

---

| Option | Description | Selected |
|--------|-------------|----------|
| No | No subscriptions | ✓ |
| Yes | WebSocket subscriptions | |

**User's choice:** No

---

| Option | Description | Selected |
|--------|-------------|----------|
| Query-only | No mutations | ✓ |
| Admin mutations | triggerSync mutation | |

**User's choice:** Query-only

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Relay-compliant Node interface | ✓ |
| No | Skip Relay compliance | |

**User's choice:** Yes

---

| Option | Description | Selected |
|--------|-------------|----------|
| Not in v1 | Per-type queries only | ✓ |
| Basic search | Name-based cross-type search | |

**User's choice:** Not in v1

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Expose syncStatus query | ✓ |
| No | Health endpoints only | |

**User's choice:** Yes

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Always return totalCount | |
| No | Skip counts | |
| Optional | Only when explicitly requested | ✓ |

**User's choice:** Optional

---

| Option | Description | Selected |
|--------|-------------|----------|
| entgql native | Built-in eager loading | |
| DataLoader | Batching pattern | ✓ |

**User's choice:** DataLoader

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, 100 | Cap at 100 | |
| Yes, 1000 | Cap at 1000 | ✓ |
| No cap | Any page size | |

**User's choice:** Yes, 1000

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Export schema.graphql SDL file | ✓ |
| No | Introspection only | |

**User's choice:** Yes

---

| Option | Description | Selected |
|--------|-------------|----------|
| Detailed | Field paths, validation details | ✓ |
| Generic | Standard error format | |

**User's choice:** Detailed

---

## Playground & DX

| Option | Description | Selected |
|--------|-------------|----------|
| GraphiQL | Official GraphQL Foundation IDE | ✓ |
| Apollo Sandbox | Apollo's explorer | |
| Altair | Lightweight, self-hosted | |

**User's choice:** GraphiQL

---

| Option | Description | Selected |
|--------|-------------|----------|
| Same path (/graphql) | GET=playground, POST=API | ✓ |
| Separate path | /graphql + /playground | |

**User's choice:** Same path

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Pre-built example queries | ✓ |
| No | Empty playground | |

**User's choice:** Yes

---

| Option | Description | Selected |
|--------|-------------|----------|
| Always on | Introspection everywhere | ✓ |
| Playground only | Disable on API endpoint | |

**User's choice:** Always on

---

| Option | Description | Selected |
|--------|-------------|----------|
| Always enabled | No disable option | ✓ |
| Configurable | Env var to disable | |

**User's choice:** Always enabled

---

## Server Setup

| Option | Description | Selected |
|--------|-------------|----------|
| 99designs/gqlgen | Schema-first, entgql integration | ✓ |
| graphql-go/graphql | Runtime schema building | |

**User's choice:** 99designs/gqlgen

---

| Option | Description | Selected |
|--------|-------------|----------|
| stdlib net/http | Go 1.22+ routing | ✓ |
| chi | Lightweight router | |

**User's choice:** stdlib net/http

---

| Option | Description | Selected |
|--------|-------------|----------|
| 8080 | Fly.io default | |
| Configurable | PDBPLUS_PORT, default 8080 | ✓ |

**User's choice:** Configurable

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | Graceful shutdown with drain timeout | ✓ |
| Hard shutdown | Stop on signal | |

**User's choice:** Yes

---

| Option | Description | Selected |
|--------|-------------|----------|
| Allow all origins | CORS: * | |
| Configurable origins | PDBPLUS_CORS_ORIGINS env var, default * | ✓ |

**User's choice:** Configurable origins

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, structured | slog middleware | ✓ |
| No | OTel traces only | |

**User's choice:** Yes, structured

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | JSON with version, links, sync status | ✓ |
| Redirect to playground | GET / → /graphql | |
| No | 404 on root | |

**User's choice:** Yes

---

## Claude's Discretion

- Complexity/depth limit values
- DataLoader implementation details
- GraphiQL config and example query content
- Graceful shutdown drain timeout default
- Root endpoint JSON shape

## Deferred Ideas

None — discussion stayed within phase scope
