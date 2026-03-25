# Phase 26 Context: Stream Resume & Incremental Filters

## Decisions

- **`updated_since` type:** `google.protobuf.Timestamp` — standard protobuf well-known type, nanosecond precision
- **Filter composition:** `since_id` and `updated_since` compose with existing filters via AND (predicate accumulation)
- **All 13 schemas have `updated` field** — `updated_since` applies universally

## Implementation Notes

- Add `optional int64 since_id` and `optional google.protobuf.Timestamp updated_since` fields to all 13 `Stream*Request` messages
- `since_id` translates to `WHERE id > since_id` predicate — composes naturally with keyset pagination
- `updated_since` translates to `WHERE updated > timestamp` predicate
- Import `google/protobuf/timestamp.proto` in services.proto
- Both filters are additive predicates in the existing accumulation pattern from Phase 25

## Existing Code References

- `proto/peeringdb/v1/services.proto` — add fields to `Stream*Request` messages
- `internal/grpcserver/*.go` — add predicate clauses to `Stream*` handler implementations
- `internal/grpcserver/pagination.go` — keyset iteration already uses `WHERE id > lastID`, `since_id` sets initial lastID
