# Phase 33 Context: gRPC Deduplication & Filter Parity

## Requirements
- **QUAL-01**: gRPC service handlers share a generic List/Stream implementation
- **QUAL-03**: Test coverage for grpcserver 60%+, middleware 60%+
- **ARCH-02**: ConnectRPC List RPCs expose the same filterable fields as PeeringDB compat

## Decisions

### Generic Approach: Go Generics with Callback Functions
- Use Go generics with type parameters for ent entity and proto message types
- Pass query/convert/filter functions as callback parameters
- Each per-type service file becomes ~30 lines: create callbacks, call generic List/Stream
- **NOT** interface-based dispatch, **NOT** code generation

### Callback Signature Pattern
```go
type ListParams[E any, P any] struct {
    Query      func(ctx context.Context, limit, offset int) ([]E, error)
    Count      func(ctx context.Context) (int, error)  // or remove if using limit+1
    Convert    func(E) *P
    ApplyFilters func(req) // type-specific
}
```

### Filter Application: Per-Type ApplyFilters
- Each service file keeps its own `applyNetworkFilters(req, query)` function
- Type-specific, explicit, grep-friendly
- Do NOT try to make filter application generic

### ConnectRPC Filter Parity
- Match every filterable field that PeeringDB compat exposes
- Use proper proto types: `int64` for IDs, `google.protobuf.Timestamp` for dates (not string-only)
- This requires updating proto request messages in `proto/peeringdb/v1/services.proto`
- Then `buf generate` to regenerate Go types
- Then update per-type filter functions to handle new fields

### Test Strategy
- Integration tests with in-memory SQLite: spin up ent client, seed data, call generic List/Stream, verify proto output
- Unit tests for filter application and conversion functions
- Target: grpcserver 60%+ coverage, middleware 60%+ coverage

## Scope Boundaries
- Do NOT add filters that PeeringDB compat doesn't have (beyond typed improvements)
- Do NOT change the proto service definitions (Get/List/Stream RPCs) — only request message fields
- Do NOT refactor the conversion helpers in convert.go — they work and are all used
