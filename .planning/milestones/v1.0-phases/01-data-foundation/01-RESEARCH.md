# Phase 1: Data Foundation - Research

**Researched:** 2026-03-22
**Domain:** PeeringDB data modeling (entgo schemas), SQLite storage, API sync client, schema extraction pipeline
**Confidence:** HIGH

## Summary

Phase 1 establishes the data foundation for PeeringDB Plus: 13 entgo schemas matching PeeringDB's actual API responses, a SQLite database with modernc.org/sqlite, a sync client that performs full re-fetch from PeeringDB, and a schema extraction pipeline that derives entgo schemas from PeeringDB's Django source code.

The primary technical risk is PeeringDB's API responses diverging from their OpenAPI spec. This has been verified -- the live API returns fields not documented in the spec (e.g., `org_name` on facility/carrier/campus objects, `net_side_id`/`ix_side_id` on netixlan, `ixf_import_request`/`ixf_import_request_status` on ix), and the POC endpoint requires authentication for any data. The extraction pipeline (D-11 through D-18) addresses this by parsing Django serializer source code rather than trusting the spec.

The second complexity area is the sync pipeline itself. PeeringDB's API has no total count in responses (the `meta` object is always empty), so pagination must page through until an empty `data` array is returned. Rate limiting at 20 req/min (unauthenticated) constrains sync speed. The 13 object types must be synced in FK dependency order, with nullable FK fields to handle PeeringDB's known referential integrity violations.

**Primary recommendation:** Build the schema extraction pipeline first (it validates the data model), then the entgo schemas, then the sync client. Use entgo's `sql/upsert` feature flag with `OnConflictColumns(id).UpdateNewValues()` for efficient sync upserts.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Use both Django serializer source analysis AND live API response validation to determine actual PeeringDB response shapes
- **D-02:** Do NOT reference existing Go libraries (e.g., gmazoyer/peeringdb) -- derive everything independently from Python source + live API
- **D-03:** Parse PeeringDB's `{"meta": {...}, "data": [...]}` wrapper with a single generic parser, then unmarshal each object by type
- **D-04:** Build the PeeringDB API client as a standalone reusable package at `internal/peeringdb`
- **D-05:** Unauthenticated API access at 20 req/min with a rate limiter -- no PeeringDB API key required
- **D-06:** Fetch all 13 object types in dependency order in a single sync pass -- no priority ordering
- **D-07:** Use maximum page size per request, loop through all pages sequentially per object type
- **D-08:** Handle unknown fields leniently -- log at warn level and skip, don't break sync
- **D-09:** PeeringDB base URL configurable via environment variable
- **D-10:** HTTP timeouts with 3 retries using exponential backoff on transient errors (429, 5xx)
- **D-11:** Build a Go-based extraction tool in `cmd/` that parses PeeringDB's Python Django serializer definitions (AST-level, not Django introspection)
- **D-12:** Extraction tool accepts a local PeeringDB repo path as argument (does NOT auto-clone)
- **D-13:** Output an intermediate JSON schema representation, not entgo schemas directly
- **D-14:** JSON schema uses FK references format: `{"field": "net_id", "references": "net"}`
- **D-15:** JSON schema includes full metadata: read-only, required, deprecated, help_text
- **D-16:** Extraction tool includes built-in validation against live PeeringDB API responses
- **D-17:** A separate Go tool in `cmd/` reads the intermediate JSON and generates entgo schema `.go` files
- **D-18:** The full pipeline (extraction -> JSON -> entgo generation -> ent codegen) is chained via `//go:generate` directives
- **D-19:** Entire sync wrapped in a single database transaction -- readers never see partial sync state
- **D-20:** Nullable FK fields in entgo schemas to handle PeeringDB's referential integrity violations
- **D-21:** On sync failure: roll back the transaction (preserve previous good data), then retry with 3x exponential backoff (30s, 2m, 8m)
- **D-22:** Hourly sync via in-process Go `time.Ticker` -- no external scheduler
- **D-23:** On-demand sync trigger via HTTP endpoint (`POST /sync`), protected by shared secret header (`X-Sync-Token`)
- **D-24:** Sync mutex -- if a sync is already running, skip the new request and log it
- **D-25:** Log per-object-type progress during sync (start/complete for each of 13 types) plus a summary with total objects and duration at the end
- **D-26:** Persistent `sync_status` metadata table in SQLite with last_sync_at, duration, object_counts, status
- **D-27:** Same code path for initial sync and subsequent syncs -- no special first-sync behavior
- **D-28:** SQLite WAL journal mode enabled for concurrent reads during sync writes
- **D-29:** Sync worker runs only on the LiteFS primary node -- replicas receive updates via replication
- **D-30:** Application returns 503 until first sync completes -- no empty results served
- **D-31:** Hard delete: remove local rows that no longer appear in PeeringDB's response
- **D-32:** Inclusion of PeeringDB objects with `status=deleted` is configurable via env var, default to excluding them
- **D-33:** All application config via environment variables only (12-factor, Fly.io native)
- **D-34:** Database path configurable via `PDBPLUS_DB_PATH` env var with sensible default
- **D-35:** Basic OTel spans around the full sync and per-object-type fetches from Phase 1
- **D-36:** Go-style field names internally (e.g., `InfoPrefixes4`), expose PeeringDB-style names (`info_prefixes4`) in APIs via JSON struct tags
- **D-37:** Use PeeringDB's integer IDs as entgo primary keys -- direct mapping, no translation
- **D-38:** Model relationships as proper entgo edges (not plain FK integers) -- enables GraphQL relationship traversal in Phase 2
- **D-39:** Junction/derived objects (netixlan, netfac, carrierfac) modeled as entgo edge-through tables
- **D-40:** Mirror ALL fields PeeringDB returns -- complete data fidelity
- **D-41:** Use PeeringDB's `created`/`updated` timestamps only -- no entgo time mixins
- **D-42:** Include entgql, entproto, and entrest annotations on schemas upfront -- avoid rework in later phases
- **D-43:** Auto-migrate schema on startup (`entclient.Schema.Create()`), but only on the LiteFS primary node
- **D-44:** Mirror POC (point of contact) data as-is, including personal info -- it's public data
- **D-45:** Add indexes on commonly-queried fields (ASN, name, status, FK fields) upfront
- **D-46:** No entgo privacy policies; add basic mutation hooks for OTel tracing on writes
- **D-47:** PeeringDB timestamps stored using PeeringDB's original field names -- no local sync timestamps per row
- **D-48:** Single Go module: `github.com/dotwaffle/peeringdb-plus`
- **D-49:** Standard Go project layout: `cmd/`, `internal/`, `ent/schema/`
- **D-50:** Main binary: `peeringdb-plus` (in `cmd/peeringdb-plus/`)
- **D-51:** SQLite driver: `modernc.org/sqlite` (CGo-free)
- **D-52:** Commit generated entgo code (`ent/` directory) to git
- **D-53:** BSD 3-Clause license
- **D-54:** Include multi-stage Dockerfile in Phase 1
- **D-55:** Fixture-based tests for CI (recorded real API responses) + optional live integration tests gated behind a build tag or env var
- **D-56:** All live integration tests MUST target `beta.peeringdb.com`, NOT `api.peeringdb.com`

### Claude's Discretion
- IP address field storage strategy (netip.Addr custom type vs text -- pick what balances correctness and simplicity)
- Exact exponential backoff timing
- Loading skeleton for any interim CLI output
- Exact `sync_status` table column set beyond the specified fields

### Deferred Ideas (OUT OF SCOPE)
- None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| DATA-01 | Mirror all 13 PeeringDB object types (org, net, fac, ix, poc, ixlan, ixpfx, netixlan, netfac, carrier, carrierfac, campus, ixfac) | Full field inventory from live API responses for 12 of 13 types (poc requires auth but Django model fields are documented). Django source analysis provides complete field definitions. Entgo schema patterns verified. |
| DATA-02 | All fields per object match PeeringDB's actual API responses (not their buggy OpenAPI spec) | Live API responses captured for all object types. Computed/serializer-only fields identified (org_name, net_count, fac_count, ix_count, carrier_count, ixf_net_count, netixlan_updated, netfac_updated, poc_updated, etc.). Django model fields and choices constants fully documented. |
| DATA-03 | Handle deleted/status-filtered objects correctly | HandleRefModel provides `status` field with soft-delete pattern. D-31 (hard delete) and D-32 (configurable status=deleted inclusion) cover this. Sync compares local vs remote IDs to detect deletions. |
| DATA-04 | Full re-fetch sync runs hourly or on-demand | Rate limiting verified at 20 req/min unauthenticated. Maximum page size of 250 confirmed working. No total count in API responses -- must page until empty data array. Sync order documented (13 types in FK dependency order). D-22 (ticker) and D-23 (HTTP trigger) cover scheduling. |
| STOR-01 | Data stored in SQLite using entgo ORM | modernc.org/sqlite integration with entgo verified via user-provided reference snippets. Driver registration, DSN pragma syntax, WAL mode, test setup all documented. Upsert via `sql/upsert` feature flag confirmed working with SQLite. |
</phase_requirements>

## Standard Stack

### Core (Phase 1 Specific)

| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| entgo.io/ent | v0.14.5 | ORM / schema-first code generation | Project constraint. Generates type-safe Go client from schema definitions. |
| modernc.org/sqlite | v1.36.0+ | CGo-free SQLite driver | Project constraint. Pure Go, works with entgo via `sql.Register("sqlite3", &sqlite.Driver{})`. |
| entgo.io/contrib/entgql | v0.7.0 | GraphQL annotations on schemas | D-42 requires annotations upfront. Not used in Phase 1 runtime but schemas must include annotations. |
| entgo.io/contrib/entproto | v0.7.0 | Protobuf annotations on schemas | D-42 requires annotations upfront. Same module as entgql. |
| github.com/lrstanley/entrest | v1.0.2 | REST annotations on schemas | D-42 requires annotations upfront. Schemas annotated now, used in Phase 2. |
| go.opentelemetry.io/otel | v1.35+ | OpenTelemetry tracing API | D-35 requires basic OTel spans in Phase 1. |
| go.opentelemetry.io/otel/sdk | v1.35+ | OpenTelemetry SDK | Required for trace export. |
| go.opentelemetry.io/contrib/bridges/otelslog | latest | slog-to-OTel bridge | Structured logging with OTel correlation. |
| github.com/KimMachineGun/automemlimit | latest | Automatic GOMEMLIMIT from cgroup | Essential for Fly.io -- without it, GC doesn't know cgroup memory limit. |
| log/slog (stdlib) | Go 1.26 | Structured logging | Project standard per OBS-1. |

### Supporting

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| github.com/99designs/gqlgen | latest | GraphQL server (compile dep for entgql) | Needed at code-gen time for entgql annotations. |
| google.golang.org/protobuf | latest | Protobuf runtime (compile dep for entproto) | Needed at code-gen time for entproto annotations. |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| modernc.org/sqlite | mattn/go-sqlite3 | Requires CGo -- complicates Docker builds and cross-compilation. |
| modernc.org/sqlite | ncruces/go-sqlite3 (wazero) | Faster reads but uses WASM runtime. Less proven with entgo. |
| text for IP addresses | netip.Addr custom type | See Architecture Patterns for recommendation. |

## Architecture Patterns

### Recommended Project Structure (Phase 1)

```
peeringdb-plus/
  cmd/
    peeringdb-plus/          # Main binary (D-50)
      main.go                # Entry point, config, wiring
    pdb-schema-extract/      # Schema extraction tool (D-11)
      main.go                # Parses Django Python source
    pdb-schema-generate/     # Schema generation tool (D-17)
      main.go                # Reads JSON, writes entgo schemas
  internal/
    peeringdb/               # PeeringDB API client package (D-04)
      client.go              # HTTP client with rate limiting
      types.go               # Response types (generic wrapper + per-object)
      pagination.go          # Page-through-until-empty logic
    sync/
      worker.go              # Sync orchestrator (scheduler, mutex, retry)
      upsert.go              # Per-object-type upsert logic
      delete.go              # Hard delete for removed objects (D-31)
      status.go              # sync_status table management (D-26)
    config/
      config.go              # Environment variable loading (D-33)
    otel/
      provider.go            # OTel SDK init (D-35)
  ent/
    schema/                  # 13 entgo schema files (generated by pdb-schema-generate)
    entc.go                  # Code generation config with extensions
    generate.go              # go:generate directive
  schema/                    # Intermediate JSON schemas (D-13)
    peeringdb.json           # Output of pdb-schema-extract
  testdata/
    fixtures/                # Recorded API responses (D-55)
  Dockerfile                 # Multi-stage build (D-54)
  go.mod
  go.sum
  LICENSE                    # BSD 3-Clause (D-53)
```

### Pattern 1: PeeringDB API Response Wrapper

**What:** All PeeringDB API responses follow `{"meta": {}, "data": [...]}`. Parse with a generic wrapper.
**When to use:** Every API call in the sync client.
**Example:**

```go
// Source: Verified against live PeeringDB API 2026-03-22
type Response[T any] struct {
    Meta json.RawMessage `json:"meta"`
    Data []T             `json:"data"`
}
```

The `meta` object is always empty in practice (verified). `data` is always an array, even for single-object queries.

### Pattern 2: Entgo Schema with PeeringDB Integer ID

**What:** Use PeeringDB's integer ID as the entgo primary key (D-37). Configure via `field.Int("id")` on each schema.
**When to use:** Every schema definition.
**Example:**

```go
// Source: entgo.io/docs/schema-fields
func (Organization) Fields() []ent.Field {
    return []ent.Field{
        field.Int("id").
            Positive().
            Immutable().
            Comment("PeeringDB organization ID"),
        field.String("name").
            MaxLen(255).
            NotEmpty().
            Annotations(
                entgql.OrderField("NAME"),
            ),
        // ... remaining fields
        field.Time("created").
            Immutable().
            Comment("PeeringDB creation timestamp"),
        field.Time("updated").
            Comment("PeeringDB last update timestamp"),
        field.String("status").
            MaxLen(255).
            Default("ok"),
    }
}
```

### Pattern 3: Edge with Explicit FK Field (Nullable for FK violations)

**What:** Define edges with explicit FK fields (D-38, D-20). FK fields are Optional+Nillable to handle PeeringDB's referential integrity violations.
**When to use:** All edges between PeeringDB objects.
**Example:**

```go
// Source: entgo.io/docs/schema-edges + D-20
func (NetworkFacility) Fields() []ent.Field {
    return []ent.Field{
        field.Int("id").Positive().Immutable(),
        field.Int("net_id").Optional().Nillable().
            Comment("FK to network"),
        field.Int("fac_id").Optional().Nillable().
            Comment("FK to facility"),
        field.Int("local_asn"),
        // ...
    }
}

func (NetworkFacility) Edges() []ent.Edge {
    return []ent.Edge{
        edge.From("network", Network.Type).
            Ref("network_facilities").
            Field("net_id").
            Unique(),
        edge.From("facility", Facility.Type).
            Ref("network_facilities").
            Field("fac_id").
            Unique(),
    }
}
```

### Pattern 4: Bulk Upsert for Sync

**What:** Use entgo's bulk upsert with `OnConflictColumns` for efficient sync operations.
**When to use:** Sync upsert phase for each object type.
**Example:**

```go
// Source: entgo.io/blog/2021/08/05/announcing-upsert-api/
// Requires feature flag: sql/upsert
builders := make([]*ent.OrganizationCreate, 0, len(orgs))
for _, org := range orgs {
    builders = append(builders, client.Organization.Create().
        SetID(org.ID).
        SetName(org.Name).
        SetAka(org.Aka).
        // ... set all fields
        SetCreated(org.Created).
        SetUpdated(org.Updated).
        SetStatus(org.Status),
    )
}
err := client.Organization.CreateBulk(builders...).
    OnConflictColumns(organization.FieldID).
    UpdateNewValues().
    Exec(ctx)
```

### Pattern 5: Pagination Until Empty

**What:** PeeringDB API has no total count in the meta response. Page through using `limit` + `skip` until `data` is empty.
**When to use:** Every object type fetch during sync.
**Example:**

```go
// Source: Verified against live PeeringDB API -- meta is always empty
func (c *Client) FetchAll(ctx context.Context, objectType string) ([]json.RawMessage, error) {
    const pageSize = 250
    var all []json.RawMessage
    for skip := 0; ; skip += pageSize {
        url := fmt.Sprintf("%s/api/%s?limit=%d&skip=%d&depth=0", c.baseURL, objectType, pageSize, skip)
        resp, err := c.doWithRetry(ctx, url)
        if err != nil {
            return nil, fmt.Errorf("fetch %s page %d: %w", objectType, skip/pageSize, err)
        }
        var apiResp Response[json.RawMessage]
        if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
            return nil, fmt.Errorf("decode %s response: %w", objectType, err)
        }
        resp.Body.Close()
        if len(apiResp.Data) == 0 {
            break
        }
        all = append(all, apiResp.Data...)
    }
    return all, nil
}
```

### Pattern 6: IP Address Storage Strategy (Claude's Discretion)

**Recommendation:** Store IP addresses as `string` (text) in SQLite rather than a custom `netip.Addr` type.

**Rationale:**
- PeeringDB returns IP addresses as strings in the API response (e.g., `"ipaddr4": "206.223.115.10"`)
- SQLite has no native IP address type
- entgo's JSON field with `netip.Addr` would work but adds serialization complexity for every read/write
- IP address comparison operations (range queries) are not a Phase 1 requirement
- The `string` type preserves exact PeeringDB formatting
- If IP-specific queries are needed later, a custom predicate can parse at query time

```go
field.String("ipaddr4").Optional().Nillable().
    Comment("IPv4 address").
    StructTag(`json:"ipaddr4"`),
field.String("ipaddr6").Optional().Nillable().
    Comment("IPv6 address").
    StructTag(`json:"ipaddr6"`),
```

### Pattern 7: social_media JSON Field

**What:** PeeringDB returns `social_media` as a JSON array of `{service, identifier}` objects. Store as `field.JSON`.
**When to use:** org, net, fac, ix, carrier, campus schemas.
**Example:**

```go
type SocialMedia struct {
    Service    string `json:"service"`
    Identifier string `json:"identifier"`
}

// In schema Fields():
field.JSON("social_media", []SocialMedia{}).
    Optional().
    Comment("Social media links"),
```

### Pattern 8: Entc Configuration with Extensions

**What:** Configure entgo code generation with all required extensions and feature flags.
**Example:**

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/contrib/entgql"
    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
)

func main() {
    gqlExt, err := entgql.NewExtension(
        entgql.WithSchemaGenerator(),
        entgql.WithSchemaPath("graph/schema.graphqls"),
        entgql.WithWhereInputs(true),
    )
    if err != nil {
        log.Fatalf("creating entgql extension: %v", err)
    }

    opts := []entc.Option{
        entc.Extensions(gqlExt),
        entc.FeatureNames("sql/upsert"),
    }

    if err := entc.Generate("./schema", &gen.Config{}, opts...); err != nil {
        log.Fatalf("running ent codegen: %v", err)
    }
}
```

### Anti-Patterns to Avoid

- **Trusting PeeringDB's OpenAPI spec:** The spec has documented validation failures (GitHub #1878). Always verify against live API responses.
- **Single massive transaction for all 13 types:** D-19 specifies a single transaction. However, note the WAL mode trade-off: readers see the pre-sync snapshot until the transaction commits. This is acceptable per the decision, but be aware that a failed sync mid-way rolls back everything cleanly.
- **Using `depth > 0` in sync fetches:** Always use `depth=0` to get flat responses with FK IDs. Depth expansion changes the response shape unpredictably between single and list endpoints.
- **Relying on `meta` for pagination:** The `meta` object is always empty. Never wait for count data from it.
- **Closing channels from the receiver side:** Per CC-1, the sync scheduler (sender) must close channels. The sync trigger endpoint (receiver) never closes.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| ORM / query building | Custom SQL queries | entgo generated client | Schema-driven, type-safe, generates all API surfaces from annotations |
| HTTP rate limiting | Custom token bucket | `golang.org/x/time/rate` | stdlib-adjacent, well-tested rate limiter. Set to 20 req/min per D-05 |
| Exponential backoff | Custom retry loop | Straightforward retry helper (small enough to hand-write) | Only 3 retries needed. A tiny helper function suffices -- no library needed |
| JSON schema validation | Custom validators | Extraction tool + live API comparison (D-16) | The extraction tool itself IS the validator |
| SQLite WAL / PRAGMA config | Manual PRAGMA execution per connection | DSN query parameters (`_pragma=key(value)`) | Per REFERENCE-SQLITE-ENTGO.md, pragmas are set via DSN, not SQL statements |

**Key insight:** The extraction pipeline (D-11 through D-18) is the project's unique tooling investment. Everything else uses standard libraries and entgo's code generation.

## PeeringDB Data Model: Complete Field Inventory

This section documents every field returned by the live PeeringDB API (verified 2026-03-22) for each of the 13 object types. Fields are categorized as: **Model** (from Django model), **Computed** (added by serializer, not stored), or **Common** (from HandleRefModel base).

### HandleRefModel Common Fields (all objects)

| Field | Type | Notes |
|-------|------|-------|
| `id` | int | Auto-incrementing PK |
| `status` | string | "ok", "deleted", etc. Soft-delete pattern. |
| `created` | datetime (ISO 8601) | Set on creation |
| `updated` | datetime (ISO 8601) | Set on every update |

### Organization (`org`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| name | string(255) | Model | field.String, Unique, NotEmpty |
| aka | string(255) | Model | field.String, Optional |
| name_long | string(255) | Model | field.String, Optional |
| website | url | Model | field.String, Optional |
| social_media | json array | Model | field.JSON([]SocialMedia{}) |
| notes | text | Model | field.String, Optional |
| logo | string/null | Model | field.String, Optional, Nillable |
| address1 | string(255) | AddressModel | field.String, Optional |
| address2 | string(255) | AddressModel | field.String, Optional |
| city | string(255) | AddressModel | field.String, Optional |
| state | string(255) | AddressModel | field.String, Optional |
| country | string | AddressModel | field.String, Optional |
| zipcode | string(48) | AddressModel | field.String, Optional |
| suite | string(255) | AddressModel | field.String, Optional |
| floor | string(255) | AddressModel | field.String, Optional |
| latitude | decimal(9,6)/null | AddressModel | field.Float, Optional, Nillable |
| longitude | decimal(9,6)/null | AddressModel | field.Float, Optional, Nillable |

### Network (`net`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| org_id | int | FK | field.Int, edge to Organization |
| name | string(255) | Model | field.String, Unique, NotEmpty |
| aka | string(255) | Model | field.String, Optional |
| name_long | string(255) | Model | field.String, Optional |
| website | url | Model | field.String, Optional |
| social_media | json array | Model | field.JSON([]SocialMedia{}) |
| asn | int | Model | field.Int, Unique, Positive |
| looking_glass | url | Model | field.String, Optional |
| route_server | url | Model | field.String, Optional |
| irr_as_set | string(255) | Model | field.String, Optional |
| info_type | string(60) | Model (concrete) | field.String, Optional |
| info_types | json array | Model (multi-choice) | field.JSON([]string{}) |
| info_prefixes4 | int/null | Model | field.Int, Optional, Nillable |
| info_prefixes6 | int/null | Model | field.Int, Optional, Nillable |
| info_traffic | string(39) | Model (choice) | field.String, Optional |
| info_ratio | string(45) | Model (choice) | field.String, Optional |
| info_scope | string(39) | Model (choice) | field.String, Optional |
| info_unicast | bool | Model | field.Bool, Default(false) |
| info_multicast | bool | Model | field.Bool, Default(false) |
| info_ipv6 | bool | Model | field.Bool, Default(false) |
| info_never_via_route_servers | bool | Model | field.Bool, Default(false) |
| notes | text | Model | field.String, Optional |
| policy_url | url | Model | field.String, Optional |
| policy_general | string(72) | Model (choice) | field.String, Optional |
| policy_locations | string(72) | Model (choice) | field.String, Optional |
| policy_ratio | bool | Model | field.Bool, Default(false) |
| policy_contracts | string(36) | Model (choice) | field.String, Optional |
| allow_ixp_update | bool | Serializer | field.Bool, Default(false) |
| status_dashboard | url/null | Model | field.String, Optional, Nillable |
| rir_status | string(255)/null | Model | field.String, Optional, Nillable |
| rir_status_updated | datetime/null | Model | field.Time, Optional, Nillable |
| logo | string/null | Serializer | field.String, Optional, Nillable |
| ix_count | int | Computed | field.Int, Optional |
| fac_count | int | Computed | field.Int, Optional |
| netixlan_updated | datetime | Computed | field.Time, Optional, Nillable |
| netfac_updated | datetime | Computed | field.Time, Optional, Nillable |
| poc_updated | datetime | Computed | field.Time, Optional, Nillable |

**Note on computed fields:** `ix_count`, `fac_count`, `netixlan_updated`, `netfac_updated`, `poc_updated` are serializer-computed fields (not in the Django model). Per D-40 (mirror ALL fields), these must be stored. They will become stale between syncs but will be refreshed on each full re-fetch.

### Facility (`fac`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| org_id | int | FK | field.Int, edge to Organization |
| org_name | string | Computed | field.String, Optional |
| campus_id | int/null | FK | field.Int, Optional, Nillable, edge to Campus |
| name | string(255) | Model | field.String, Unique, NotEmpty |
| aka | string(255) | Model | field.String, Optional |
| name_long | string(255) | Model | field.String, Optional |
| website | url | Model | field.String, Optional |
| social_media | json array | Model | field.JSON([]SocialMedia{}) |
| clli | string(18) | Model | field.String, Optional |
| rencode | string(18) | Model | field.String, Optional |
| npanxx | string(21) | Model | field.String, Optional |
| tech_email | email(254) | Model | field.String, Optional |
| tech_phone | string(192) | Model | field.String, Optional |
| sales_email | email(254) | Model | field.String, Optional |
| sales_phone | string(192) | Model | field.String, Optional |
| property | string(27)/null | Model (choice) | field.String, Optional, Nillable |
| diverse_serving_substations | bool/null | Model | field.Bool, Optional, Nillable |
| available_voltage_services | json array | Model (multi-choice) | field.JSON([]string{}) |
| notes | text | Model | field.String, Optional |
| region_continent | string/null | Model (choice) | field.String, Optional, Nillable |
| status_dashboard | url/null | Model | field.String, Optional, Nillable |
| logo | string/null | Serializer | field.String, Optional, Nillable |
| net_count | int | Computed | field.Int, Optional |
| ix_count | int | Computed | field.Int, Optional |
| carrier_count | int | Computed | field.Int, Optional |
| All AddressModel fields (address1, address2, city, state, country, zipcode, suite, floor, latitude, longitude) | | AddressModel | Same as Organization |

### Internet Exchange (`ix`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| org_id | int | FK | field.Int, edge to Organization |
| name | string(64) | Model | field.String, Unique, NotEmpty |
| aka | string(255) | Model | field.String, Optional |
| name_long | string(255) | Model | field.String, Optional |
| city | string(192) | Model | field.String |
| country | string | Model | field.String |
| region_continent | string(255) | Model (choice) | field.String |
| media | string(128) | Model (choice) | field.String, Default("Ethernet") |
| notes | text | Model | field.String, Optional |
| proto_unicast | bool | Model | field.Bool, Default(false) |
| proto_multicast | bool | Model | field.Bool, Default(false) |
| proto_ipv6 | bool | Model | field.Bool, Default(false) |
| website | url | Model | field.String, Optional |
| social_media | json array | Model | field.JSON([]SocialMedia{}) |
| url_stats | url | Model | field.String, Optional |
| tech_email | email(254) | Model | field.String, Optional |
| tech_phone | string(192) | Model | field.String, Optional |
| policy_email | email(254) | Model | field.String, Optional |
| policy_phone | string(192) | Model | field.String, Optional |
| sales_email | email(254) | Model | field.String, Optional |
| sales_phone | string(192) | Model | field.String, Optional |
| net_count | int | Computed | field.Int, Optional |
| fac_count | int | Computed | field.Int, Optional |
| ixf_net_count | int | Model | field.Int, Default(0) |
| ixf_last_import | datetime/null | Model | field.Time, Optional, Nillable |
| ixf_import_request | string/null | Serializer | field.String, Optional, Nillable |
| ixf_import_request_status | string | Serializer | field.String, Optional |
| service_level | string(60) | Model (choice) | field.String, Optional |
| terms | string(60) | Model (choice) | field.String, Optional |
| status_dashboard | url/null | Model | field.String, Optional, Nillable |
| logo | string/null | Serializer | field.String, Optional, Nillable |

### Network Contact (`poc`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| net_id | int | FK | field.Int, edge to Network |
| role | string(27) | Model (choice: Abuse/Maintenance/Policy/Technical/NOC/Public Relations/Sales) | field.String |
| visible | string(64) | Model (choice: Private/Users/Public) | field.String, Default("Public") |
| name | string(254) | Model | field.String, Optional |
| phone | string(100) | Model | field.String, Optional |
| email | email(254) | Model | field.String, Optional |
| url | url | Model | field.String, Optional |

**Note:** POC data is not accessible via unauthenticated API requests. The live API returns empty `data` arrays. For unauthenticated sync (D-05), POC records will not be synced unless PeeringDB changes this behavior. This is a known limitation. The schema should still be defined to support future authenticated access.

### IXLan (`ixlan`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| ix_id | int | FK | field.Int, edge to InternetExchange |
| name | string(255) | Model | field.String, Optional |
| descr | text | Model | field.String, Optional |
| mtu | int | Model (choice: 1500/9000) | field.Int, Default(1500) |
| dot1q_support | bool | Model | field.Bool, Default(false) |
| rs_asn | int/null | Model | field.Int, Optional, Nillable, Default(0) |
| arp_sponge | string/null | Model (MAC) | field.String, Optional, Nillable |
| ixf_ixp_member_list_url_visible | string(64) | Model (choice: Private/Users/Public) | field.String, Default("Private") |
| ixf_ixp_import_enabled | bool | Serializer | field.Bool, Default(false) |

**Note:** `ixf_ixp_member_list_url` exists in the Django model but is NOT returned by the API (it's a URL that may be private). Only `ixf_ixp_member_list_url_visible` is returned.

### IXPrefix (`ixpfx`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| ixlan_id | int | FK | field.Int, edge to IXLan |
| protocol | string(64) | Model (choice: IPv4/IPv6) | field.String |
| prefix | string | Model (IP prefix) | field.String, Unique |
| in_dfz | bool | Model | field.Bool, Default(false) |
| notes | string(255) | Model | field.String, Optional |

### NetworkIXLan (`netixlan`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| net_id | int | FK | field.Int, edge to Network |
| ix_id | int | Computed | field.Int, Optional |
| ixlan_id | int | FK | field.Int, edge to IXLan |
| name | string | Computed | field.String, Optional |
| notes | string(255) | Model | field.String, Optional |
| speed | int | Model | field.Int |
| asn | int | Model | field.Int |
| ipaddr4 | string/null | Model (IP) | field.String, Optional, Nillable |
| ipaddr6 | string/null | Model (IP) | field.String, Optional, Nillable |
| is_rs_peer | bool | Model | field.Bool, Default(false) |
| bfd_support | bool | Model | field.Bool, Default(false) |
| operational | bool | Model | field.Bool, Default(true) |
| net_side_id | int/null | FK (to Facility) | field.Int, Optional, Nillable |
| ix_side_id | int/null | FK (to Facility) | field.Int, Optional, Nillable |

**Note:** `ix_id` and `name` are serializer-computed fields (not FK or model fields). `net_side_id` and `ix_side_id` are FKs to Facility but may be null.

### NetworkFacility (`netfac`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| net_id | int | FK | field.Int, edge to Network |
| fac_id | int | FK | field.Int, edge to Facility |
| name | string | Computed | field.String, Optional |
| city | string | Computed | field.String, Optional |
| country | string | Computed | field.String, Optional |
| local_asn | int | Model | field.Int |

### IX-Facility (`ixfac`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| ix_id | int | FK | field.Int, edge to InternetExchange |
| fac_id | int | FK | field.Int, edge to Facility |
| name | string | Computed | field.String, Optional |
| city | string | Computed | field.String, Optional |
| country | string | Computed | field.String, Optional |

### Carrier (`carrier`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| org_id | int | FK | field.Int, edge to Organization |
| org_name | string | Computed | field.String, Optional |
| name | string(255) | Model | field.String, Unique, NotEmpty |
| aka | string(255) | Model | field.String, Optional |
| name_long | string(255) | Model | field.String, Optional |
| website | url | Model | field.String, Optional |
| social_media | json array | Model | field.JSON([]SocialMedia{}) |
| notes | text | Model | field.String, Optional |
| fac_count | int | Computed | field.Int, Optional |
| logo | string/null | Serializer | field.String, Optional, Nillable |

### CarrierFacility (`carrierfac`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| carrier_id | int | FK | field.Int, edge to Carrier |
| fac_id | int | FK | field.Int, edge to Facility |
| name | string | Computed | field.String, Optional |

### Campus (`campus`)

| Field | Type | Source | Entgo Type |
|-------|------|--------|------------|
| org_id | int | FK | field.Int, edge to Organization |
| org_name | string | Computed | field.String, Optional |
| name | string(255) | Model | field.String, Unique, NotEmpty |
| name_long | string(255)/null | Model | field.String, Optional, Nillable |
| aka | string(255)/null | Model | field.String, Optional, Nillable |
| website | url | Model | field.String, Optional |
| social_media | json array | Model | field.JSON([]SocialMedia{}) |
| notes | text | Model | field.String, Optional |
| country | string | Concrete | field.String, Optional |
| city | string | Concrete | field.String, Optional |
| zipcode | string | Concrete | field.String, Optional |
| state | string | Concrete | field.String, Optional |
| logo | string/null | Serializer | field.String, Optional, Nillable |

## PeeringDB Choice Constants (for Enum Validation)

These are the valid values for choice fields. Store as strings, not enums -- PeeringDB may add new values without notice.

| Constant | Values |
|----------|--------|
| POC_ROLES | Abuse, Maintenance, Policy, Technical, NOC, Public Relations, Sales |
| VISIBILITY | Private, Users, Public |
| PROPERTY | (empty), Owner, Lessee |
| REGIONS | North America, Asia Pacific, Europe, South America, Africa, Australia, Middle East |
| TRAFFIC | (empty), 0-20Mbps, 20-100Mbps, 100-1000Mbps, 1-5Gbps, 5-10Gbps, 10-20Gbps, 20-50Gbps, 50-100Gbps, 100+Gbps, 100-200Gbps, 200-300Gbps, 300-500Gbps, 500-1000Gbps, 1-5Tbps, 5-10Tbps, 10-20Tbps, 20-50Tbps, 50-100Tbps, 100+Tbps |
| RATIOS | (empty), Not Disclosed, Heavy Outbound, Mostly Outbound, Balanced, Mostly Inbound, Heavy Inbound |
| SCOPES | (empty), Not Disclosed, Regional, North America, Asia Pacific, Europe, South America, Africa, Australia, Middle East, Global |
| POLICY_GENERAL | Open, Selective, Restrictive, No |
| POLICY_LOCATIONS | Not Required, Preferred, Required - US, Required - EU, Required - International |
| POLICY_CONTRACTS | Not Required, Private Only, Required |
| NET_TYPES | (empty), Not Disclosed, NSP, Content, Cable/DSL/ISP, Enterprise, Educational/Research, Non-Profit, Route Server, Network Services, Route Collector, Government |
| MEDIA | Ethernet, ATM, Multiple |
| SERVICE_LEVEL | (empty), Not Disclosed, Best Effort (no SLA), Normal Business Hours, 24/7 Support |
| TERMS | (empty), Not Disclosed, No Commercial Terms, Bundled With Other Services, Non-recurring Fees Only, Recurring Fees |
| MTUS | 1500, 9000 |
| PROTOCOLS | IPv4, IPv6 |

**Important:** Store these as `field.String`, not `field.Enum`. PeeringDB may add new values at any time. Using `field.Enum` would cause sync failures when new values appear. Validate at display time, not storage time.

## Sync Order (FK Dependency Order)

Per D-06, sync all 13 types in dependency order within a single transaction (D-19):

```
1.  org          (no FK dependencies)
2.  campus       (depends on: org)
3.  fac          (depends on: org, campus)
4.  carrier      (depends on: org)
5.  carrierfac   (depends on: carrier, fac)
6.  ix           (depends on: org)
7.  ixlan        (depends on: ix)
8.  ixpfx        (depends on: ixlan)
9.  ixfac        (depends on: ix, fac)
10. net          (depends on: org)
11. poc          (depends on: net)
12. netfac       (depends on: net, fac)
13. netixlan     (depends on: net, ixlan)
```

## Common Pitfalls

### Pitfall 1: POC Data Requires Authentication

**What goes wrong:** The PeeringDB `/api/poc` endpoint returns empty data for unauthenticated requests. Since D-05 specifies unauthenticated access, POC records will not be synced.
**Why it happens:** PeeringDB restricts contact information to authenticated users to prevent scraping.
**How to avoid:** Define the POC schema (DATA-01 requires all 13 types) but accept that it will be empty in v1. Log a warning during sync when POC returns empty data. Document this limitation. Add a note that future authenticated sync would populate this data.
**Warning signs:** Empty POC table after sync completes successfully.

### Pitfall 2: Computed Fields Need Storage

**What goes wrong:** Fields like `org_name`, `net_count`, `fac_count`, `ix_count`, `carrier_count`, `netixlan_updated`, `netfac_updated`, `poc_updated`, `name` (on junction tables), `city`/`country` (on junction tables), `ix_id` (on netixlan) are NOT in the Django model -- they are computed by the serializer. If you only model Django fields, these will be missing from API responses.
**Why it happens:** D-40 says "mirror ALL fields PeeringDB returns." These computed fields are part of the API response.
**How to avoid:** Store computed fields as regular entgo fields. They will be refreshed on each sync. Mark them with a comment indicating they are computed/denormalized.
**Warning signs:** Missing fields when comparing local query results to PeeringDB API responses.

### Pitfall 3: SQLite Driver Name Registration

**What goes wrong:** entgo expects `dialect.SQLite` which maps to `"sqlite3"` (mattn's driver name). modernc.org/sqlite registers as `"sqlite"`. Without manual registration, entgo can't find the driver.
**How to avoid:** Follow REFERENCE-SQLITE-ENTGO.md exactly: `sql.Register("sqlite3", &sqlite.Driver{})` in an `init()` function. Use the modernc driver via `sql.Open("sqlite", dsn)` for direct access, or via the `"sqlite3"` alias for entgo.
**Warning signs:** "unknown driver" or "sql: unknown driver 'sqlite3'" errors at startup.

### Pitfall 4: PeeringDB API Has No Total Count

**What goes wrong:** Developers expect `meta` to contain pagination info (total count, next page URL). It's always empty `{}`.
**Why it happens:** PeeringDB's API predates modern REST pagination conventions.
**How to avoid:** Page through with `limit`+`skip` until `data` is an empty array. The maximum verified page size is 250.
**Warning signs:** Sync fetches only the first page of data.

### Pitfall 5: Nullable FK Fields for Referential Integrity Violations

**What goes wrong:** PeeringDB's production database contains FK violations (e.g., a `netfac` referencing a deleted facility). If FK fields are `Required()` in entgo, upserts fail with constraint violations.
**Why it happens:** PeeringDB soft-deletes objects but doesn't cascade-update referencing objects.
**How to avoid:** Per D-20, make ALL FK fields `Optional().Nillable()`. Log warnings when FK targets don't exist locally. This allows the sync to complete even with orphaned references.
**Warning signs:** Constraint violation errors during sync upsert operations.

### Pitfall 6: Entgo `field.Enum` Breaks on Unknown Values

**What goes wrong:** PeeringDB may add new choice values (e.g., new traffic ranges, new POC roles) without notice. `field.Enum` validates against a fixed set and rejects unknown values, causing sync failures.
**Why it happens:** PeeringDB's choice fields are maintained in their Django source and can change between releases.
**How to avoid:** Use `field.String` for all choice/enum fields. Validate at display time or in API responses, not at storage time. Store the raw PeeringDB value exactly as received.
**Warning signs:** Sync failures with "invalid enum value" errors after a PeeringDB update.

### Pitfall 7: Single Transaction Performance with WAL

**What goes wrong:** D-19 specifies a single transaction for all 13 types. In WAL mode, readers see a snapshot from before the transaction started. If the transaction takes several minutes (fetching all data from PeeringDB API), readers serve stale data for the entire duration.
**Why it happens:** SQLite WAL provides snapshot isolation per transaction.
**How to avoid:** This is acceptable behavior (D-19 is a locked decision). Readers always see a consistent snapshot. After the transaction commits, readers immediately see fresh data. Monitor sync duration via OTel spans (D-35). If sync duration becomes problematic, the decision can be revisited.
**Warning signs:** Sync duration exceeding 5 minutes (indicating network or rate-limiting issues).

### Pitfall 8: Schema Extraction Tool Complexity

**What goes wrong:** Parsing Python Django source code with a Go-based tool is non-trivial. The `go-python/gpython` parser is pre-1.0 and only supports Python 3.4 syntax. Modern PeeringDB uses Python 3.11+ features.
**Why it happens:** D-11 specifies a Go-based extraction tool, not a Python script.
**How to avoid:** Use a regex/line-parsing approach rather than full AST parsing. Django model/serializer definitions follow predictable patterns (`field.CharField(max_length=255, ...)`, `ForeignKey(Organization, ...)`, `class Meta: ...`). A pattern-matching parser will be more robust than a full Python AST parser for this specific use case. Alternatively, consider using Python's `ast` module via `os/exec` if the regex approach proves insufficient.
**Warning signs:** Parser failing on Python syntax features (f-strings, walrus operator, match statements).

## Code Examples

### SQLite Connection Setup (Production)

```go
// Source: .planning/phases/01-data-foundation/01-REFERENCE-SQLITE-ENTGO.md
import (
    "database/sql"
    "fmt"

    "entgo.io/ent/dialect"
    entsql "entgo.io/ent/dialect/sql"
    "modernc.org/sqlite"
)

func init() {
    sql.Register("sqlite3", &sqlite.Driver{})
}

func openClient(dbPath string) (*ent.Client, error) {
    dsn := fmt.Sprintf(
        "file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
        dbPath,
    )
    db, err := sql.Open("sqlite3", dsn)
    if err != nil {
        return nil, fmt.Errorf("opening database: %w", err)
    }
    drv := entsql.OpenDB(dialect.SQLite, db)
    return ent.NewClient(ent.Driver(drv)), nil
}
```

### SQLite Test Setup

```go
// Source: .planning/phases/01-data-foundation/01-REFERENCE-SQLITE-ENTGO.md
func setupTestClient(t *testing.T) *ent.Client {
    t.Helper()
    client := enttest.Open(t,
        dialect.SQLite,
        "file:ent?mode=memory&cache=shared&_pragma=foreign_keys(1)",
    )
    t.Cleanup(func() { client.Close() })
    return client
}
```

### Rate-Limited HTTP Client

```go
// Source: Application of golang.org/x/time/rate
import "golang.org/x/time/rate"

type Client struct {
    http    *http.Client
    limiter *rate.Limiter
    baseURL string
}

func NewClient(baseURL string) *Client {
    return &Client{
        http: &http.Client{
            Timeout: 30 * time.Second,
        },
        // 20 requests per minute = 1 request per 3 seconds
        limiter: rate.NewLimiter(rate.Every(3*time.Second), 1),
        baseURL: baseURL,
    }
}

func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
    if err := c.limiter.Wait(ctx); err != nil {
        return nil, fmt.Errorf("rate limiter: %w", err)
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
    if err != nil {
        return nil, fmt.Errorf("creating request: %w", err)
    }
    req.Header.Set("User-Agent", "peeringdb-plus/1.0")
    return c.http.Do(req)
}
```

### Sync Worker with Mutex and Retry

```go
// Source: Application of D-19, D-21, D-24
type Worker struct {
    client    *Client
    entClient *ent.Client
    mu        sync.Mutex
    running   atomic.Bool
    logger    *slog.Logger
}

func (w *Worker) Sync(ctx context.Context) error {
    if !w.running.CompareAndSwap(false, true) {
        w.logger.Warn("sync already running, skipping")
        return nil
    }
    defer w.running.Store(false)

    ctx, span := otel.Tracer("sync").Start(ctx, "full-sync")
    defer span.End()

    // Single transaction per D-19
    tx, err := w.entClient.Tx(ctx)
    if err != nil {
        return fmt.Errorf("begin sync transaction: %w", err)
    }

    syncOrder := []struct {
        name string
        fn   func(context.Context, *ent.Tx) error
    }{
        {"org", w.syncOrganizations},
        {"campus", w.syncCampuses},
        {"fac", w.syncFacilities},
        {"carrier", w.syncCarriers},
        {"carrierfac", w.syncCarrierFacilities},
        {"ix", w.syncInternetExchanges},
        {"ixlan", w.syncIXLans},
        {"ixpfx", w.syncIXPrefixes},
        {"ixfac", w.syncIXFacilities},
        {"net", w.syncNetworks},
        {"poc", w.syncContacts},
        {"netfac", w.syncNetworkFacilities},
        {"netixlan", w.syncNetworkIXLans},
    }

    for _, s := range syncOrder {
        w.logger.Info("syncing object type", slog.String("type", s.name))
        if err := s.fn(ctx, tx); err != nil {
            tx.Rollback()
            return fmt.Errorf("sync %s: %w", s.name, err)
        }
    }

    if err := tx.Commit(); err != nil {
        return fmt.Errorf("commit sync transaction: %w", err)
    }
    return nil
}
```

### Hard Delete Pattern (D-31)

```go
// After upsert, delete local rows not in remote response
func (w *Worker) deleteStale(ctx context.Context, tx *ent.Tx, objectType string, remoteIDs []int) error {
    // Get all local IDs for this type
    // Delete any that are not in remoteIDs
    // This handles objects removed from PeeringDB
    _, err := tx.Organization.Delete().
        Where(organization.IDNotIn(remoteIDs...)).
        Exec(ctx)
    return err
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| mattn/go-sqlite3 (CGo) | modernc.org/sqlite (pure Go) | 2023+ | No CGo dependency, simpler Docker builds |
| entgo v0.11 | entgo v0.14.5 | 2025-07 | Improved SQLite dialect, better upsert support |
| PeeringDB without carrier/campus | 13 object types including carrier/campus | PeeringDB 2.43.0 (2024) | Must model all 13 types |
| go-python/gpython for Python AST | Pattern-matching parser (recommended) | N/A | gpython only supports Python 3.4; Django uses 3.11+ |
| OpenCensus tracing in ent | OTel bridge or custom hooks | 2024+ | Use OTel hooks directly, avoid OpenCensus bridge complexity |

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go testing (stdlib) + enttest |
| Config file | None -- standard Go test infrastructure |
| Quick run command | `go test ./... -short` |
| Full suite command | `go test -race ./...` |

### Phase Requirements to Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DATA-01 | All 13 object types exist as entgo schemas | unit | `go test ./ent/schema/... -run TestSchema` | Wave 0 |
| DATA-01 | All 13 schemas compile and generate valid code | unit | `go generate ./ent && go build ./...` | Wave 0 |
| DATA-02 | Fields match actual PeeringDB API responses | integration | `go test ./internal/peeringdb/... -run TestFieldMapping` | Wave 0 |
| DATA-02 | Fixture responses deserialize into ent types | unit | `go test ./internal/sync/... -run TestFixtureDeserialization` | Wave 0 |
| DATA-03 | Deleted objects are hard-deleted locally (D-31) | unit | `go test ./internal/sync/... -run TestHardDelete` | Wave 0 |
| DATA-03 | status=deleted objects excluded by default (D-32) | unit | `go test ./internal/sync/... -run TestStatusFilter` | Wave 0 |
| DATA-04 | Full sync populates database with all objects | integration | `go test -tags=integration ./internal/sync/... -run TestFullSync` | Wave 0 |
| DATA-04 | Sync runs on hourly schedule | unit | `go test ./internal/sync/... -run TestScheduler` | Wave 0 |
| DATA-04 | On-demand sync trigger works | unit | `go test ./internal/sync/... -run TestTrigger` | Wave 0 |
| STOR-01 | SQLite with entgo works (create, query, upsert) | unit | `go test ./ent/... -run TestCRUD` | Wave 0 |
| STOR-01 | WAL mode enabled, FK constraints enforced | unit | `go test ./internal/... -run TestSQLiteConfig` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./... -short -count=1`
- **Per wave merge:** `go test -race ./... -count=1`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps

- [ ] `testdata/fixtures/*.json` -- Recorded API responses for all 13 object types (capture from `beta.peeringdb.com` per D-56)
- [ ] `internal/peeringdb/client_test.go` -- Tests for API client with fixture-based HTTP server
- [ ] `internal/sync/worker_test.go` -- Tests for sync logic with in-memory SQLite
- [ ] `ent/schema/*_test.go` or `ent/enttest_test.go` -- Schema compilation and CRUD verification
- [ ] Test helpers for creating in-memory entgo test clients (per REFERENCE-SQLITE-ENTGO.md)

## Open Questions

1. **POC data access without authentication**
   - What we know: The `/api/poc` endpoint returns empty data for unauthenticated requests
   - What's unclear: Whether this is a recent change or has always been the case. Whether `beta.peeringdb.com` has the same restriction.
   - Recommendation: Define the schema, attempt sync, log when empty. Document as known limitation. Consider adding optional API key support as a follow-up.

2. **Single transaction duration under rate limiting**
   - What we know: 13 object types, 250 per page, 20 req/min rate limit. Rough estimate: ~100-200 pages total across all types = ~300-600 seconds (5-10 minutes) of API fetching.
   - What's unclear: Whether SQLite holds the write lock for the entire API fetch duration or only during actual write operations within the transaction.
   - Recommendation: SQLite WAL mode allows concurrent readers. The write lock is held during the transaction, but readers see the pre-transaction snapshot. This is acceptable per D-19. Measure actual sync duration in Phase 1 and revisit if > 10 minutes.

3. **Schema extraction tool robustness**
   - What we know: D-11 specifies Go-based AST parsing of Python. gpython only supports Python 3.4.
   - What's unclear: How complex the actual PeeringDB Python source patterns are.
   - Recommendation: Start with pattern matching (regex) on the Django model definitions. The patterns are highly regular (`field.CharField(max_length=255)`, `ForeignKey(Organization)`). Fall back to `os/exec` with Python `ast` module if needed. The extraction tool only needs to run during development, not at runtime.

## Sources

### Primary (HIGH confidence)
- PeeringDB live API responses (verified 2026-03-22 for all 13 object types)
- [django-peeringdb abstract models](https://github.com/peeringdb/django-peeringdb/blob/master/src/django_peeringdb/models/abstract.py) -- Django model field definitions
- [django-peeringdb concrete models](https://github.com/peeringdb/django-peeringdb/blob/master/src/django_peeringdb/models/concrete.py) -- FK relationships
- [django-peeringdb constants](https://github.com/peeringdb/django-peeringdb/blob/master/src/django_peeringdb/const.py) -- Choice field values
- [django-handleref](https://github.com/20c/django-handleref) -- HandleRefModel base class (id, status, created, updated, version)
- [entgo schema fields](https://entgo.io/docs/schema-fields) -- Field types and configuration
- [entgo schema edges](https://entgo.io/docs/schema-edges) -- Edge/relationship definition
- [entgo schema indexes](https://entgo.io/docs/schema-indexes) -- Index definition
- [entgo code generation](https://entgo.io/docs/code-gen) -- generate.go and entc.go setup
- [entgo upsert API](https://entgo.io/blog/2021/08/05/announcing-upsert-api/) -- OnConflict/UpdateNewValues
- [entgo feature flags](https://entgo.io/docs/feature-flags) -- sql/upsert flag
- [entgo CRUD operations](https://entgo.io/docs/crud) -- Create, bulk create, query, delete
- `.planning/phases/01-data-foundation/01-REFERENCE-SQLITE-ENTGO.md` -- SQLite+entgo integration snippets

### Secondary (MEDIUM confidence)
- [PeeringDB API specs](https://docs.peeringdb.com/api_specs/) -- Official API documentation (note: spec diverges from reality)
- [PeeringDB query limits](https://docs.peeringdb.com/howto/work_within_peeringdbs_query_limits/) -- Rate limiting details
- [go-python/gpython](https://pkg.go.dev/github.com/go-python/gpython/parser) -- Go-based Python parser (v0.2.0, pre-1.0)

### Tertiary (LOW confidence)
- Schema extraction tool approach (Go regex vs AST) -- no established pattern for this specific use case; needs validation

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all libraries verified and version-checked
- Architecture: HIGH -- patterns derived from verified entgo docs and PeeringDB API responses
- Data model: HIGH -- verified against live API for 12/13 types; POC from Django source only
- Sync strategy: HIGH -- rate limits and pagination behavior verified against live API
- Extraction pipeline: MEDIUM -- the approach is sound but implementation complexity of Go-based Python parsing is uncertain
- Pitfalls: HIGH -- documented from official sources and verified behaviors

**Research date:** 2026-03-22
**Valid until:** 2026-04-22 (30 days -- stable domain, PeeringDB API changes infrequently)
