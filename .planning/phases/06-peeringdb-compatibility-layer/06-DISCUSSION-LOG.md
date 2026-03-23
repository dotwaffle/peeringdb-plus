# Phase 6: PeeringDB Compatibility Layer - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-22
**Phase:** 06-peeringdb-compatibility-layer
**Areas discussed:** URL path mapping, Response format, Query filters, Search & projection, Testing, Server integration

---

## URL Path Mapping

### Path names

| Option | Description | Selected |
|--------|-------------|----------|
| Exact PeeringDB paths | /api/net, /api/ix, /api/fac etc. matching TypeOrg constants | ✓ |
| PeeringDB + aliases | Exact paths plus long-form aliases | |

**User's choice:** Exact PeeringDB paths

### API index

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, match PeeringDB | GET /api/ returns JSON listing endpoints | |
| No, just types | Only /api/{type} endpoints | |
| You decide | | ✓ |

**User's choice:** Claude decides

### Trailing slash

| Option | Description | Selected |
|--------|-------------|----------|
| Both | Accept with and without trailing slash | ✓ |
| Strict (no slash) | Only /api/net, redirect /api/net/ | |

**User's choice:** Both

---

## Response Format

### Meta field

| Option | Description | Selected |
|--------|-------------|----------|
| Include empty meta | {meta: {}, data: [...]} matching PeeringDB | |
| Add useful meta | {meta: {total, limit, skip}, data: [...]} | |
| You decide | | ✓ |

**User's choice:** Claude decides

### Field names

| Option | Description | Selected |
|--------|-------------|----------|
| Exact PeeringDB names | snake_case matching PeeringDB response format | ✓ |
| Ent JSON tags | Use whatever ent serializes | |

**User's choice:** Exact PeeringDB names

### Depth implementation

| Option | Description | Selected |
|--------|-------------|----------|
| Ent eager-load | Use ent's With* eager-loading | ✓ |
| Separate queries | Fetch primary then separate queries | |

**User's choice:** Ent eager-load

### Depth=0 vs depth=2

| Option | Description | Selected |
|--------|-------------|----------|
| Match exactly | depth=0: {org_id: 42}, depth=2: {org: {...}} | ✓ |
| Always include ID | Keep FK ID even with nested object | |

**User's choice:** Match exactly

### _set fields

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, include _set | Full reverse relation sets at depth=2 | |
| Skip _set fields | Only forward FK expansion | |
| You decide | | ✓ |

**User's choice:** Claude decides which _set fields PeeringDB returns per type

### Single object format

| Option | Description | Selected |
|--------|-------------|----------|
| Envelope with array | {data: [{...}]} for single object | |
| Unwrap single | Return object directly | |
| You decide | | ✓ |

**User's choice:** Claude checks PeeringDB's actual behavior

---

## Query Filters

### Filter scope

| Option | Description | Selected |
|--------|-------------|----------|
| Core set | __contains, __startswith, __in, __lt, __gt, __lte, __gte, exact | ✓ |
| Extended set | Core plus __icontains, __endswith, __isnull, __regex | |

**User's choice:** Core set

### Case sensitivity

| Option | Description | Selected |
|--------|-------------|----------|
| Case-insensitive | Match PeeringDB via SQLite COLLATE NOCASE | ✓ |
| Case-sensitive | Default SQLite behavior | |

**User's choice:** Case-insensitive

### Parser design

| Option | Description | Selected |
|--------|-------------|----------|
| Generic parser | One parser for all types | ✓ |
| Per-type handlers | Each type has own filter handler | |

**User's choice:** Generic parser

### Unknown fields

| Option | Description | Selected |
|--------|-------------|----------|
| Return 400 error | Clear error for invalid fields | |
| Silently ignore | Skip unknown fields | |
| You decide | | ✓ |

**User's choice:** Claude decides

### Status filter

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, support status | Allow ?status=ok and ?status=deleted | ✓ |
| Only active objects | Always filter to status=ok | |

**User's choice:** Yes, support status

---

## Search & Projection

### ?q= search

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, implement ?q= | Search across name fields | ✓ |
| No, filter only | Only field-specific filters | |

**User's choice:** Yes

### ?q= search fields

| Option | Description | Selected |
|--------|-------------|----------|
| Name fields | name, aka, name_long | |
| Name + identifiers | Name fields plus ASN, irr_as_set, website | |
| You decide | | ✓ |

**User's choice:** Claude matches PeeringDB's actual ?q= behavior per type

### ?fields= projection

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | ?fields=id,name,asn returns only those fields | ✓ |
| No, full objects | Always return all fields | |

**User's choice:** Yes

### ?since= format

| Option | Description | Selected |
|--------|-------------|----------|
| Unix only | Unix timestamps only | ✓ |
| Unix + ISO 8601 | Both formats | |

**User's choice:** Unix only

### Pagination limits

| Option | Description | Selected |
|--------|-------------|----------|
| Max 250 | Match PeeringDB page size | |
| Max 1000 | Allow larger pages | |
| No max | No limit | |
| You decide | | ✓ |

**User's choice:** Claude decides

### Sorting

| Option | Description | Selected |
|--------|-------------|----------|
| ID order only | Default PeeringDB behavior | ✓ |
| Add ?sort= param | Non-PeeringDB extension | |

**User's choice:** ID order only

---

## Error Handling & Branding

### Error format

| Option | Description | Selected |
|--------|-------------|----------|
| Match PeeringDB | Reproduce exact error shapes | ✓ |
| Consistent format | Standard {meta: {error: ...}} | |

**User's choice:** Match PeeringDB

### Branding

| Option | Description | Selected |
|--------|-------------|----------|
| X-Powered-By header | PeeringDB-Plus/1.1 header | ✓ |
| No branding | Pure compatibility | |

**User's choice:** X-Powered-By header

### HTTP headers

| Option | Description | Selected |
|--------|-------------|----------|
| Match PeeringDB headers | Include custom headers PeeringDB sets | ✓ |
| Standard headers only | Just Content-Type | |

**User's choice:** Match PeeringDB headers

---

## Server Integration

### Package structure

| Option | Description | Selected |
|--------|-------------|----------|
| internal/pdbcompat | New dedicated package | ✓ |
| internal/api | Shorter name matching URL | |

**User's choice:** internal/pdbcompat

### CORS

| Option | Description | Selected |
|--------|-------------|----------|
| Separate CORS | Independent instance for /api/ | ✓ |
| Shared stack | Use existing middleware | |

**User's choice:** Separate CORS — consistent with Phase 5

---

## Testing

### Test approach

| Option | Description | Selected |
|--------|-------------|----------|
| Against PeeringDB fixtures | Reuse Phase 1 fixture files | |
| Golden file tests | Compare against real PeeringDB responses | |
| Both | Fixture-based + golden file comparison | ✓ |

**User's choice:** Both

---

## Claude's Discretion

- API index at /api/
- Meta field content (empty or enhanced)
- _set fields per type mapping
- Unknown filter field behavior
- Page size limits
- Single object response format (likely envelope with array)
- ?q= search fields per type

## Deferred Ideas

None — discussion stayed within phase scope.
