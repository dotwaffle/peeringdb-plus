# Phase 24: List Filtering - Context

**Gathered:** 2026-03-25
**Status:** Ready for planning

<domain>
## Phase Boundary

Users can filter List RPC results using typed fields (ASN, country, name, org_id, status) instead of fetching all records and filtering client-side.

</domain>

<decisions>
## Implementation Decisions

### Filter Design
- Individual typed fields on List request messages (e.g., `optional int64 asn = 2;`) — type-safe, self-documenting
- Use proto `optional` keyword for presence detection via `has_` methods — distinguishes unset from zero values
- Entity-specific filter fields: Network gets asn/name/country/status/org_id, Facility gets country/city/status, etc. — only fields that make sense per type

### Error Handling & Behavior
- Invalid filter field values return `INVALID_ARGUMENT` with field name in message: "invalid filter: asn must be positive"
- Multiple filters combine with AND logic — all filters must match
- Case-insensitive LIKE matching for name/string fields, exact match for country/status codes and numeric fields

### Claude's Discretion
- Exact set of filter fields per entity type beyond the examples (asn, country, name, org_id, status)
- Internal ent query builder composition for filter application
- Test fixture design for filter combinations

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `proto/peeringdb/v1/services.proto` — Existing List request/response messages to extend with filter fields
- `internal/grpcserver/*.go` — 13 service handlers with ListXxx methods to add filter logic to
- `internal/grpcserver/pagination.go` — Pagination logic that composes with filtering

### Established Patterns
- ent query builder for database queries (e.g., `client.Network.Query().Where(...)`)
- proto `optional` fields generate `GetXxx()` and `HasXxx()` methods in Go
- Table-driven tests in grpcserver_test.go

### Integration Points
- `proto/peeringdb/v1/services.proto` — Add filter fields to List request messages
- `internal/grpcserver/*.go` — Apply filters to ent queries in List handlers
- `buf generate` — Regenerate Go types after proto changes

</code_context>

<specifics>
## Specific Ideas

No specific requirements beyond ROADMAP success criteria and the decisions above.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>
