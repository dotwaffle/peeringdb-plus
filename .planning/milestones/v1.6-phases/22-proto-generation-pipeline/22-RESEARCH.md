# Phase 22: Proto Generation Pipeline - Research

**Researched:** 2026-03-25
**Domain:** entproto annotations, buf toolchain, ConnectRPC code generation
**Confidence:** HIGH

## Summary

This phase annotates all 13 ent schemas with entproto annotations to generate .proto files, then uses the buf toolchain with protoc-gen-go and protoc-gen-connect-go to produce compiled Go types and ConnectRPC handler interfaces. The entproto extension generates .proto files during `go generate ./ent/...`, and `buf generate` compiles them into Go code.

The primary challenge is that entproto cannot handle all field types present in the schemas. Specifically, `field.JSON("social_media", []SocialMedia{})` uses a custom struct type that entproto does not support -- entproto only handles JSON fields typed as `[]string`, `[]int32`, `[]int64`, `[]uint32`, or `[]uint64`. The `info_types` and `available_voltage_services` fields (typed as `[]string`) ARE supported and will generate as `repeated string`. The `social_media` fields on 6 schemas must use `entproto.Skip()` on the field, with a manually written `SocialMedia` message in a separate .proto file that is imported by the hand-written service implementations in Phase 23.

Edges must all be annotated with `entproto.Skip()` because (a) they create circular proto message dependencies (Organization -> Network -> Organization), (b) entproto requires a `FieldAnnotation` on each edge that is not skipped, and (c) the service implementations in Phase 23 will handle edge loading via ent queries, not proto message nesting.

**Primary recommendation:** Add entproto extension to entc.go with `SkipGenFile()` and `WithProtoDir("proto/peeringdb/v1")`. Annotate all 13 schemas with `entproto.Message(entproto.PackageName("peeringdb.v1"))` and `entproto.Field(N)` on every field. Skip all edges and `social_media` JSON fields. Configure buf.yaml + buf.gen.yaml at project root. Write a manual `common.proto` containing the `SocialMedia` message. Use protoc-gen-connect-go with the `simple` option for cleaner handler signatures.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
None -- all implementation choices are at Claude's discretion for this infrastructure phase.

### Claude's Discretion
All implementation choices are at Claude's discretion -- pure infrastructure phase. Use ROADMAP phase goal, success criteria, and codebase conventions to guide decisions.

### Deferred Ideas (OUT OF SCOPE)
None -- infrastructure phase.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PROTO-01 | All 13 ent schemas annotated with entproto.Message and entproto.Field for proto generation | entproto.Message(entproto.PackageName("peeringdb.v1")) on each schema Annotations(), entproto.Field(N) on each field, entproto.Skip() on edges and social_media |
| PROTO-02 | buf toolchain configured (buf.yaml, buf.gen.yaml) with protoc-gen-go + protoc-gen-connect-go | buf.yaml v2 with modules path pointing to proto dir, buf.gen.yaml v2 with local plugins using `go tool` pattern, managed mode for go_package_prefix |
| PROTO-03 | Proto files generated from ent schemas via entproto with SkipGenFile | entproto.NewExtension(entproto.SkipGenFile(), entproto.WithProtoDir("proto/peeringdb/v1")) added to entc.go extensions |
| PROTO-04 | ConnectRPC handler interfaces generated via buf generate | `buf generate` produces *.pb.go and *connect/*.go files; manual common.proto provides SocialMedia message |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| entgo.io/contrib/entproto | v0.7.1-pre (in go.mod) | Proto file generation from ent schemas | Already in go.mod as entgo.io/contrib. Generates .proto files during `go generate`. |
| connectrpc.com/connect | v1.19.x | ConnectRPC runtime library | Required by protoc-gen-connect-go generated code. Provides connect.Request/Response types, interceptors, handler constructors. |
| google.golang.org/protobuf | v1.36.11 | Protobuf Go runtime | Already in go.mod (indirect). Required by protoc-gen-go generated *.pb.go files. |
| google.golang.org/grpc | v1.79.3 | gRPC runtime (indirect) | Already in go.mod (indirect). Required for proto well-known types. |

### Supporting (Tool Dependencies)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| google.golang.org/protobuf/cmd/protoc-gen-go | latest | Protobuf Go code generator | `go get -tool` to add as tool dependency. Invoked by `buf generate`. |
| connectrpc.com/connect/cmd/protoc-gen-connect-go | latest | ConnectRPC code generator | `go get -tool` to add as tool dependency. Invoked by `buf generate`. |
| buf CLI | latest | Protobuf toolchain (lint, generate, breaking) | Invoked via `go run github.com/bufbuild/buf/cmd/buf@latest` or shell alias. |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| entproto + manual protos | Fully hand-written .proto files | entproto auto-maps 90%+ of fields correctly; hand-writing 13 schemas with 200+ fields is error-prone and unmaintainable |
| protoc-gen-connect-go | protoc-gen-go-grpc (standard gRPC) | ConnectRPC serves gRPC + gRPC-Web + Connect on same http.Handler; standard gRPC requires separate grpc.Server. Project decision: ConnectRPC. |
| `simple` flag on protoc-gen-connect-go | Default connect.Request/Response wrappers | `simple` produces `(ctx, *Request) -> (*Response, error)` signatures matching Go idioms. Headers accessed via ctx. Cleaner service code. |
| buf CLI | raw protoc | buf provides linting, breaking change detection, managed mode, and cleaner config. Per CLAUDE.md TL-4. |

**Installation:**
```bash
# Add tool dependencies (Go 1.24+ pattern)
go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest
go get -tool connectrpc.com/connect/cmd/protoc-gen-connect-go@latest

# Add ConnectRPC runtime
go get connectrpc.com/connect@latest
```

## Architecture Patterns

### Recommended Project Structure
```
peeringdb-plus/
├── ent/
│   ├── entc.go              # Add entproto extension here
│   ├── generate.go           # Existing go:generate directive
│   └── schema/               # 13 schema files + types.go (annotate here)
├── proto/
│   └── peeringdb/
│       └── v1/
│           ├── entpb.proto   # AUTO-GENERATED by entproto (all 13 messages)
│           └── common.proto  # MANUAL: SocialMedia message definition
├── gen/
│   └── peeringdb/
│       └── v1/
│           ├── entpb.pb.go   # Generated by protoc-gen-go
│           └── peeringdbv1connect/
│               └── entpb.connect.go  # Generated by protoc-gen-connect-go
├── buf.yaml                  # buf workspace config
└── buf.gen.yaml              # buf code generation config
```

### Pattern 1: entproto Extension in entc.go
**What:** Add entproto as an ent extension alongside existing entgql and entrest extensions.
**When to use:** During `go generate ./ent/...` to auto-generate .proto files from annotated schemas.
**Example:**
```go
// Source: entgo.io/contrib/entproto extension.go
import "entgo.io/contrib/entproto"

protoExt, err := entproto.NewExtension(
    entproto.SkipGenFile(),
    entproto.WithProtoDir("../proto/peeringdb/v1"),
)
if err != nil {
    log.Fatalf("creating entproto extension: %v", err)
}

opts := []entc.Option{
    entc.Extensions(gqlExt, restExt, protoExt),
    entc.FeatureNames("sql/upsert"),
}
```

### Pattern 2: Schema Annotation with entproto
**What:** Annotate each ent schema with Message, Field numbers, and Skip markers.
**When to use:** On all 13 schema files to opt-in to proto generation.
**Example:**
```go
// Source: entgo.io/contrib/entproto source code analysis
import "entgo.io/contrib/entproto"

func (Organization) Fields() []ent.Field {
    return []ent.Field{
        field.Int("id").
            Positive().
            Immutable().
            Comment("PeeringDB organization ID"),
        // ID gets field number 1 automatically
        field.String("address1").
            Optional().
            Default("").
            Annotations(entproto.Field(2)).
            Comment("Address line 1"),
        field.String("name").
            NotEmpty().
            Unique().
            Annotations(
                entgql.OrderField("NAME"),
                entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
                entproto.Field(11),
            ).
            Comment("Organization name"),
        field.JSON("social_media", []SocialMedia{}).
            Optional().
            Annotations(
                entrest.WithSchema(socialMediaSchema()),
                entproto.Skip(), // Custom struct type unsupported by entproto
            ).
            Comment("Social media links"),
        // ...
    }
}

func (Organization) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("campuses", Campus.Type).
            Annotations(
                entrest.WithEagerLoad(true),
                entproto.Skip(), // Skip all edges in proto gen
            ),
        // ...
    }
}

func (Organization) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.RelayConnection(),
        entgql.QueryField(),
        entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
        entproto.Message(entproto.PackageName("peeringdb.v1")),
    }
}
```

### Pattern 3: buf.yaml Configuration
**What:** Define the proto module workspace for buf operations.
**When to use:** At project root to enable `buf lint` and `buf generate`.
**Example:**
```yaml
# buf.yaml
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
  except:
    - PACKAGE_VERSION_SUFFIX  # entproto generates "peeringdb.v1" not "peeringdb.v1beta1"
  enum_zero_value_suffix: _UNSPECIFIED
  service_suffix: Service
breaking:
  use:
    - FILE
  ignore_unstable_packages: true
```

### Pattern 4: buf.gen.yaml Configuration
**What:** Configure protoc-gen-go and protoc-gen-connect-go for code generation.
**When to use:** When running `buf generate` to produce Go types and ConnectRPC interfaces.
**Example:**
```yaml
# buf.gen.yaml
version: v2
clean: true
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/dotwaffle/peeringdb-plus/gen
plugins:
  - local: [go, tool, protoc-gen-go]
    out: gen
    opt:
      - paths=source_relative
  - local: [go, tool, protoc-gen-connect-go]
    out: gen
    opt:
      - paths=source_relative
      - simple
```

### Pattern 5: Manual SocialMedia Proto Message
**What:** Hand-written proto message for the custom SocialMedia struct that entproto cannot generate.
**When to use:** Imported by service implementations (Phase 23) to include social_media in responses.
**Example:**
```protobuf
// proto/peeringdb/v1/common.proto
syntax = "proto3";
package peeringdb.v1;
option go_package = "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1";

// SocialMedia represents a social media link from PeeringDB.
message SocialMedia {
  string service = 1;
  string identifier = 2;
}
```

### Anti-Patterns to Avoid
- **Including edges in proto messages:** Creates circular dependencies (Organization -> Network -> Organization). Skip all edges. Service handlers in Phase 23 load edges via ent queries and populate response fields manually.
- **Using entproto.Service():** Generates protoc-gen-entgrpc service stubs targeting google.golang.org/grpc, not ConnectRPC. Out of scope per REQUIREMENTS.md.
- **Not using SkipGenFile():** Without it, entproto generates `generate.go` files with `//go:generate protoc ...` directives that we don't want -- we use buf instead.
- **Forgetting the default entproto package name:** entproto defaults to package `"entpb"`. Must override with `entproto.PackageName("peeringdb.v1")` for proper naming.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Proto field type mapping | Manual ent-to-proto type conversion table | entproto type mapping (types.go typeMap) | Handles 15 scalar types, optional wrappers, time->Timestamp, JSON->repeated automatically |
| Proto file generation | Hand-written .proto files for 200+ fields | entproto extension in entc.go | Auto-generates from schema, stays in sync with schema changes, handles imports |
| Go protobuf code | Manual struct definitions matching proto messages | protoc-gen-go via `buf generate` | Generates marshaling, reflection, and field accessors correctly |
| ConnectRPC interfaces | Manual handler interface definitions | protoc-gen-connect-go via `buf generate` | Generates type-safe handler interfaces, client constructors, and path routing |
| Proto linting | Manual .proto file review | `buf lint` | Enforces STANDARD protobuf style rules consistently |

**Key insight:** entproto handles ~95% of the field mapping automatically. Only the 6 `social_media` fields and all edges need manual handling (Skip annotations). The 2 `[]string` JSON fields (`info_types`, `available_voltage_services`) are natively supported by entproto.

## Field Type Inventory

### Fields Requiring entproto.Skip() (unsupported by entproto)

| Schema | Field | Type | Reason |
|--------|-------|------|--------|
| Organization | social_media | `[]SocialMedia{}` | Custom struct JSON -- not in entproto's supported JSON types |
| Network | social_media | `[]SocialMedia{}` | Same |
| Facility | social_media | `[]SocialMedia{}` | Same |
| InternetExchange | social_media | `[]SocialMedia{}` | Same |
| Carrier | social_media | `[]SocialMedia{}` | Same |
| Campus | social_media | `[]SocialMedia{}` | Same |

### Fields Natively Supported as repeated

| Schema | Field | Type | Proto Type |
|--------|-------|------|------------|
| Network | info_types | `[]string` | `repeated string` |
| Facility | available_voltage_services | `[]string` | `repeated string` |

### Optional/Nillable Field Handling

entproto wraps optional scalar fields with Google wrapper types:
- `field.Float("latitude").Optional().Nillable()` -> `google.protobuf.DoubleValue`
- `field.Int("info_prefixes4").Optional().Nillable()` -> `google.protobuf.Int64Value`
- `field.String("logo").Optional().Nillable()` -> `google.protobuf.StringValue`
- `field.Bool("diverse_serving_substations").Optional().Nillable()` -> `google.protobuf.BoolValue`
- `field.Time("rir_status_updated").Optional().Nillable()` -> `google.protobuf.Timestamp`

Non-nillable optional strings/ints with defaults use unwrapped proto types (string, int64).

### ID Field

All 13 schemas use `field.Int("id")` which maps to `int64` in proto. The ID field automatically receives proto field number 1 -- no annotation needed.

### Time Fields

`field.Time("created")` and `field.Time("updated")` map to `google.protobuf.Timestamp`. `created` is non-optional (required Timestamp), `updated` is non-optional (required Timestamp).

### All 13 Schemas

1. Organization (22 fields, 5 edges)
2. Network (37 fields, 4 edges)
3. Facility (34 fields, 5 edges)
4. InternetExchange (30 fields, 3 edges)
5. Carrier (13 fields, 2 edges)
6. Campus (17 fields, 2 edges)
7. CarrierFacility (6 fields, 2 edges)
8. IxFacility (8 fields, 2 edges)
9. IxLan (12 fields, 3 edges)
10. IxPrefix (8 fields, 1 edge)
11. NetworkFacility (9 fields, 2 edges)
12. NetworkIxLan (16 fields, 2 edges)
13. Poc (10 fields, 1 edge)

**Total:** ~222 fields across 13 schemas, ~34 edges (all skipped), 6 unsupported JSON fields (skipped).

### Proto Field Numbering Strategy

Field number 1 is reserved for `id` (auto-assigned by entproto). All other fields must be numbered sequentially starting from 2. Fields in a message MUST have unique numbers. Number assignments are stable API -- once assigned, they should not change.

**Strategy:** Number fields in declaration order starting from 2. This matches the schema file reading order and is maintainable. Leave gaps (e.g., skip to next 10s) between logical field groups if desired for future extensibility.

## Common Pitfalls

### Pitfall 1: entproto Fails on Unsupported JSON Types
**What goes wrong:** `go generate ./ent/...` fails with `unsupported field type "TypeJSON"` for `[]SocialMedia{}` fields.
**Why it happens:** entproto's `extractJSONDetails()` only supports `[]string`, `[]int32`, `[]int64`, `[]uint32`, `[]uint64`. Custom struct slices are not handled.
**How to avoid:** Add `entproto.Skip()` annotation to all 6 `social_media` fields before running generation.
**Warning signs:** `entproto: failed parsing some schemas: unsupported field type` error during code generation.

### Pitfall 2: Edges Without Annotations Cause Errors
**What goes wrong:** `go generate` fails with `entproto: edge "X" does not have an entproto.Field annotation`.
**Why it happens:** entproto requires either `entproto.Field(N)` or `entproto.Skip()` on every edge of an annotated schema. Without annotations, `extractEdgeAnnotation` returns an error.
**How to avoid:** Add `entproto.Skip()` to every edge in all 13 schemas. Total: ~34 edge annotations.
**Warning signs:** Missing annotation error on the first edge encountered.

### Pitfall 3: Proto Field Number 1 Reserved for ID
**What goes wrong:** If a non-ID field is assigned number 1, entproto returns `field "X" has number 1 which is reserved for id`.
**Why it happens:** entproto hardcodes field number 1 for the ID field (const `IDFieldNumber = 1`).
**How to avoid:** Start field numbering at 2 for all non-ID fields.
**Warning signs:** Error message explicitly states the reserved number.

### Pitfall 4: Duplicate Field Numbers
**What goes wrong:** `entproto: field N already defined on message "X"`.
**Why it happens:** Two fields in the same schema were assigned the same proto field number.
**How to avoid:** Use sequential numbering and verify uniqueness. The field count per schema ranges from 6-37, so careful numbering is needed.
**Warning signs:** Error message identifies the duplicate number and message name.

### Pitfall 5: Default Package Name "entpb"
**What goes wrong:** Generated .proto files use package `entpb` instead of project-specific naming.
**Why it happens:** `entproto.Message()` defaults to `Package: "entpb"` if `PackageName()` is not provided.
**How to avoid:** Always use `entproto.Message(entproto.PackageName("peeringdb.v1"))` on every schema.
**Warning signs:** Generated proto file has `package entpb;` instead of `package peeringdb.v1;`.

### Pitfall 6: Multiple Annotations on a Single Field
**What goes wrong:** Annotation chaining may overwrite previous annotations if done incorrectly.
**Why it happens:** ent annotations are additive by name. entgql, entrest, and entproto use different annotation names so they compose correctly.
**How to avoid:** Chain annotations normally -- `entgql.OrderField("NAME")`, `entrest.WithFilter(...)`, `entproto.Field(N)` all have different `Name()` return values and compose without conflict.
**Warning signs:** None -- this works correctly. Just verify in tests.

### Pitfall 7: buf lint FIELD_LOWER_SNAKE_CASE Failures
**What goes wrong:** `buf lint` may complain about field names not matching snake_case conventions.
**Why it happens:** entproto uses the ent field name directly as the proto field name. Ent field names like `org_id`, `fac_id` are already snake_case, but some like `ipaddr4` or `ipaddr6` may not pass stricter rules.
**How to avoid:** Check `buf lint` output after generation. If needed, add specific exceptions to buf.yaml lint config.
**Warning signs:** Lint failures on specific field names.

### Pitfall 8: entproto Proto Output Directory
**What goes wrong:** entproto writes to `ent/proto/entpb/` by default, not where we want.
**Why it happens:** Default `protoDir` is `path.Join(g.Config.Target, "proto")` which resolves to `ent/proto/`.
**How to avoid:** Use `entproto.WithProtoDir("../proto/peeringdb/v1")` -- path is relative to `ent/` since entc.go runs from there.
**Warning signs:** Proto files appear in wrong directory.

## Code Examples

Verified patterns from source code analysis:

### Complete buf.yaml
```yaml
# buf.yaml - at project root
version: v2
modules:
  - path: proto
lint:
  use:
    - STANDARD
  except:
    - PACKAGE_VERSION_SUFFIX
  enum_zero_value_suffix: _UNSPECIFIED
breaking:
  use:
    - FILE
  ignore_unstable_packages: true
```

### Complete buf.gen.yaml
```yaml
# buf.gen.yaml - at project root
version: v2
clean: true
managed:
  enabled: true
  override:
    - file_option: go_package_prefix
      value: github.com/dotwaffle/peeringdb-plus/gen
plugins:
  - local: [go, tool, protoc-gen-go]
    out: gen
    opt:
      - paths=source_relative
  - local: [go, tool, protoc-gen-connect-go]
    out: gen
    opt:
      - paths=source_relative
      - simple
```

### entc.go with entproto Extension
```go
// Source: entgo.io/contrib/entproto extension.go
protoExt, err := entproto.NewExtension(
    entproto.SkipGenFile(),
    entproto.WithProtoDir("../proto/peeringdb/v1"),
)
if err != nil {
    log.Fatalf("creating entproto extension: %v", err)
}

opts := []entc.Option{
    entc.Extensions(gqlExt, restExt, protoExt),
    entc.FeatureNames("sql/upsert"),
}
```

### Manual common.proto
```protobuf
// proto/peeringdb/v1/common.proto
syntax = "proto3";
package peeringdb.v1;
option go_package = "github.com/dotwaffle/peeringdb-plus/gen/peeringdb/v1";

// SocialMedia represents a social media link from PeeringDB.
message SocialMedia {
  string service = 1;
  string identifier = 2;
}
```

### Schema Annotation Example (IxPrefix - simplest schema)
```go
// Source: verified against entproto source code
func (IxPrefix) Fields() []ent.Field {
    return []ent.Field{
        field.Int("id").
            Positive().
            Immutable().
            Comment("PeeringDB ixprefix ID"),
            // ID gets field number 1 automatically
        field.Int("ixlan_id").
            Optional().
            Nillable().
            Annotations(
                entrest.WithFilter(entrest.FilterEQ|entrest.FilterNEQ|...),
                entproto.Field(2),
            ).
            Comment("FK to IX LAN"),
        field.Bool("in_dfz").
            Default(false).
            Annotations(entproto.Field(3)).
            Comment("In default-free zone"),
        field.String("notes").
            Optional().
            Default("").
            Annotations(entproto.Field(4)).
            Comment("Notes"),
        field.String("prefix").
            NotEmpty().
            Unique().
            Annotations(entproto.Field(5)).
            Comment("IP prefix"),
        field.String("protocol").
            Optional().
            Default("").
            Annotations(entproto.Field(6)).
            Comment("Protocol (IPv4/IPv6)"),
        field.Time("created").
            Immutable().
            Annotations(
                entrest.WithFilter(entrest.FilterGT|...),
                entproto.Field(7),
            ).
            Comment("PeeringDB creation timestamp"),
        field.Time("updated").
            Annotations(
                entrest.WithFilter(entrest.FilterGT|...),
                entproto.Field(8),
            ).
            Comment("PeeringDB last update timestamp"),
        field.String("status").
            Default("ok").
            Annotations(
                entrest.WithFilter(entrest.FilterGroupEqual|entrest.FilterGroupArray),
                entproto.Field(9),
            ).
            Comment("Record status"),
    }
}

func (IxPrefix) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("ix_lan", IxLan.Type).
            Ref("ix_prefixes").
            Field("ixlan_id").
            Unique().
            Annotations(
                entrest.WithEagerLoad(true),
                entproto.Skip(),
            ),
    }
}

func (IxPrefix) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entgql.RelayConnection(),
        entgql.QueryField(),
        entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList),
        entproto.Message(entproto.PackageName("peeringdb.v1")),
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| protoc + protoc-gen-go-grpc | buf + protoc-gen-connect-go | ConnectRPC v1.0 (2023), buf v2 config (2024) | Single binary, lint+generate+breaking in one tool; ConnectRPC serves all 3 protocols on http.Handler |
| `go install` for protoc plugins | `go get -tool` for protoc plugins | Go 1.24 (Feb 2025) | Tool deps tracked in go.mod, no PATH issues, version pinned |
| Raw protoc invocations | buf generate | buf CLI general availability | Managed mode auto-sets go_package, handles well-known type imports |
| protoc-gen-go-grpc service stubs | protoc-gen-connect-go handler interfaces | ConnectRPC adoption 2023-2025 | http.Handler-compatible, no separate gRPC server needed |

**Deprecated/outdated:**
- `protoc-gen-go-grpc`: Still works but produces google.golang.org/grpc interfaces, not ConnectRPC. Out of scope per REQUIREMENTS.md.
- `protoc-gen-entgrpc`: Generates entgrpc service implementations targeting standard gRPC. Incompatible with ConnectRPC. Explicitly out of scope.
- `entproto.Service()`: Generates service descriptors in .proto for entgrpc. Not needed since we write manual ConnectRPC services.

## Open Questions

1. **Will entproto handle all 222 fields correctly on first attempt?**
   - What we know: All field types in the schemas are covered by entproto's typeMap (string, int, float, bool, time) except `[]SocialMedia{}` (6 fields, skipped). `[]string` JSON fields (2 fields) are supported.
   - What's unclear: Edge cases with Optional+Nillable+Default combinations may surprise. entproto distinguishes Optional (uses wrapper) from non-Optional.
   - Recommendation: Run `go generate` early and iterate. The error messages are specific and actionable.

2. **Will buf lint pass on entproto-generated proto files?**
   - What we know: entproto generates valid proto3 syntax. Field names follow ent schema naming (snake_case). Message names are PascalCase (ent type names).
   - What's unclear: Some field names like `ipaddr4`, `ipaddr6`, `rs_asn` may trigger FIELD_LOWER_SNAKE_CASE warnings depending on strictness. `PACKAGE_VERSION_SUFFIX` rule may fail on `peeringdb.v1` vs `peeringdb.v1alpha1`.
   - Recommendation: Add `PACKAGE_VERSION_SUFFIX` to lint exceptions. Fix any field name issues by adding more exceptions or by verifying they pass as-is.

3. **Does the `simple` option for protoc-gen-connect-go work with all unary RPCs?**
   - What we know: The `simple` flag eliminates connect.Request/connect.Response wrappers for simpler function signatures. This project only uses unary RPCs (Get, List) -- no streaming.
   - What's unclear: Whether `simple` mode supports all interceptor functionality needed for otelconnect in Phase 23.
   - Recommendation: Use `simple` mode. otelconnect interceptors work at the handler registration level, not individual method level. If issues arise, removing `simple` is a non-breaking change to the proto layer.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | All | Yes | 1.26.1 | -- |
| protoc | buf (optional backend) | Yes | 3.21.12 | buf includes its own protoc; system protoc not required |
| buf CLI | PROTO-02, PROTO-04 | No (alias to `go run`) | latest via go run | Use `go run github.com/bufbuild/buf/cmd/buf@latest` -- no install needed |
| protoc-gen-go | PROTO-04 | No | -- | `go get -tool` adds to go.mod |
| protoc-gen-connect-go | PROTO-04 | No | -- | `go get -tool` adds to go.mod |

**Missing dependencies with no fallback:** None -- all tools installable via `go get -tool`.

**Missing dependencies with fallback:** buf CLI not installed globally, but runs via `go run` or can be added as tool dependency.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) |
| Config file | None needed (go test convention) |
| Quick run command | `go generate ./ent/... && go run github.com/bufbuild/buf/cmd/buf@latest lint && go run github.com/bufbuild/buf/cmd/buf@latest generate && go build ./gen/...` |
| Full suite command | `go test ./... -race -count=1` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PROTO-01 | All 13 schemas annotated, `go generate` succeeds | smoke | `go generate ./ent/...` | N/A (generation test) |
| PROTO-02 | buf.yaml + buf.gen.yaml valid, buf lint passes | smoke | `go run github.com/bufbuild/buf/cmd/buf@latest lint` | No -- Wave 0 (config files) |
| PROTO-03 | Proto files generated in proto/peeringdb/v1/ | smoke | `test -f proto/peeringdb/v1/entpb.proto` | No -- Wave 0 (generated file) |
| PROTO-04 | ConnectRPC interfaces compile | smoke | `go build ./gen/...` | No -- Wave 0 (generated code) |

### Sampling Rate
- **Per task commit:** `go generate ./ent/... && buf lint && buf generate && go build ./gen/...`
- **Per wave merge:** `go test ./... -race -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `buf.yaml` -- buf workspace configuration
- [ ] `buf.gen.yaml` -- buf code generation configuration
- [ ] `proto/peeringdb/v1/common.proto` -- manual SocialMedia message
- [ ] Tool deps: `go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest`
- [ ] Tool deps: `go get -tool connectrpc.com/connect/cmd/protoc-gen-connect-go@latest`
- [ ] Runtime dep: `go get connectrpc.com/connect@latest`

## Project Constraints (from CLAUDE.md)

Directives affecting this phase:

- **CS-0 (MUST):** Modern Go code guidelines -- use Go 1.24+ `go get -tool` pattern, not `go install`
- **CS-2 (MUST):** Avoid stutter in names -- proto package `peeringdb.v1` not `peeringdb_peeringdb.v1`
- **ERR-1 (MUST):** Wrap errors with %w and context -- entc.go error handling
- **TL-4 (CAN):** APIs: `buf` for Protobuf -- use buf toolchain per CLAUDE.md recommendation
- **T-1 (MUST):** Table-driven tests; deterministic and hermetic -- verification via generation + compilation
- **T-2 (MUST):** Run -race in CI; add t.Cleanup for teardown
- **API-1 (MUST):** Document exported items -- proto messages have comments from ent schema Comment() calls
- **MD-1 (SHOULD):** Prefer stdlib; introduce deps only with clear payoff -- connectrpc.com/connect is necessary for ConnectRPC; protobuf deps already indirect in go.mod
- **CI-1 (MUST):** Lint, vet, test, build on every PR -- `buf lint` + `go build ./gen/...` as CI step

## Sources

### Primary (HIGH confidence)
- entgo.io/contrib/entproto source code (v0.7.1-pre, local go module cache) -- adapter.go, types.go, field.go, skip.go, message.go, extension.go, service.go: complete type mapping, annotation API, extension configuration, field numbering rules
- [ConnectRPC Go Getting Started](https://connectrpc.com/docs/go/getting-started/) -- buf.gen.yaml configuration, `go get -tool` pattern, handler interface pattern, `simple` option
- [protoc-gen-connect-go source](https://github.com/connectrpc/connect-go/blob/main/cmd/protoc-gen-connect-go/main.go) -- `simple` flag documentation, `package_suffix` option
- [buf.yaml v2 docs](https://buf.build/docs/configuration/v2/buf-yaml/) -- Module configuration, lint rules, breaking detection
- [buf.gen.yaml v2 docs](https://buf.build/docs/configuration/v2/buf-gen-yaml/) -- Plugin configuration, managed mode, local plugin resolution

### Secondary (MEDIUM confidence)
- [entproto docs](https://entgo.io/docs/grpc-generating-proto/) -- Basic annotation tutorial (limited depth, supplemented by source code)
- [protoc-gen-connect-go pkg.go.dev](https://pkg.go.dev/connectrpc.com/connect/cmd/protoc-gen-connect-go) -- Package documentation, version v1.19.1

### Tertiary (LOW confidence)
- None -- all findings verified against source code or official documentation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - entproto source code analyzed directly, ConnectRPC docs verified, all tools available via go get
- Architecture: HIGH - entproto output structure verified from source, buf config verified from official docs, project structure follows established patterns
- Pitfalls: HIGH - all pitfalls derived from source code analysis of entproto's error paths and type mapping logic

**Research date:** 2026-03-25
**Valid until:** 2026-04-25 (stable toolchain, entproto v0.7.x is mature)
