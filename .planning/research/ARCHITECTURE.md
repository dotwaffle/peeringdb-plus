# Architecture Patterns

**Domain:** Globally distributed read-only PeeringDB data mirror with multi-protocol APIs
**Researched:** 2026-03-22
**Confidence:** HIGH (core patterns, LiteFS mechanics) / MEDIUM (entrest maturity, LiteFS long-term support)

## Recommended Architecture

A single Go binary runs on every Fly.io node. LiteFS sits beneath it as a FUSE filesystem, replicating SQLite from a single primary to all replicas. The application has two operating modes determined by LiteFS lease ownership: **primary** (runs sync + serves reads) and **replica** (serves reads only). All API surfaces (GraphQL, gRPC, REST, Web UI) serve from every node.

### High-Level Overview

```
                         Fly.io Global Edge
                    +---------------------------+
                    |                           |
  +-----------+     |   +-------------------+   |
  | PeeringDB |     |   |  PRIMARY NODE     |   |
  |   API     |<----+---|  (single region)  |   |
  |  (source) |     |   |                   |   |
  +-----------+     |   |  Sync Worker      |   |
                    |   |      |            |   |
                    |   |      v            |   |
                    |   |  SQLite (write)   |   |
                    |   |      |            |   |
                    |   |  LiteFS FUSE      |---+--- replication --+
                    |   |      |            |   |                  |
                    |   |  API Handlers     |   |                  |
                    |   |  +- GraphQL       |   |                  |
                    |   |  +- gRPC          |   |                  |
                    |   |  +- REST          |   |                  |
                    |   |  +- Web UI        |   |                  |
                    |   +-------------------+   |                  |
                    |                           |                  |
                    |   +-------------------+   |  +-------------------+
                    |   |  REPLICA NODE     |   |  |  REPLICA NODE     |
                    |   |  (region N)       |   |  |  (region M)       |
                    |   |                   |   |  |                   |
                    |   |  SQLite (read)  <-+---+--+  SQLite (read)    |
                    |   |      |            |   |  |      |            |
                    |   |  LiteFS FUSE      |   |  |  LiteFS FUSE      |
                    |   |      |            |   |  |      |            |
                    |   |  API Handlers     |   |  |  API Handlers     |
                    |   |  +- GraphQL       |   |  |  +- GraphQL       |
                    |   |  +- gRPC          |   |  |  +- gRPC          |
                    |   |  +- REST          |   |  |  +- REST          |
                    |   |  +- Web UI        |   |  |  +- Web UI        |
                    |   +-------------------+   |  +-------------------+
                    |                           |
                    +---------------------------+
```

### Single Binary Architecture

The application is a single Go binary that runs in two modes determined by LiteFS leader election:

- **Primary mode:** Runs sync worker + serves API (reads and writes to local SQLite)
- **Replica mode:** Serves API only (reads from local SQLite replica)

Both modes run the same binary and the same API handlers. The sync worker checks LiteFS lease status before attempting writes. This avoids a separate sync service.

### Component Boundaries

| Component | Responsibility | Communicates With |
|-----------|---------------|-------------------|
| `cmd/server/` | Binary entrypoint, configuration, dependency wiring | All components |
| `ent/schema/` | Ent schema definitions for all PeeringDB objects with entgql, entproto, entrest annotations | Code generators (build time) |
| `ent/` | Generated ORM client code | SQLite via database/sql |
| `internal/sync/` | PeeringDB API client, response parsing, data mapping, upsert logic | PeeringDB API (HTTP), ent client (write) |
| `internal/server/` | HTTP/gRPC server setup, middleware composition, health checks | API handlers, otelhttp |
| `internal/graphql/` | Generated GraphQL resolvers + custom resolver logic | ent client (read) |
| `internal/grpc/` | Generated gRPC service implementations | ent client (read) |
| `internal/rest/` | Generated REST handlers + OpenAPI spec | ent client (read) |
| `internal/web/` | HTMX + Templ web UI handlers and templates | ent client (read) |
| `internal/otel/` | OpenTelemetry SDK initialization, exporters, shutdown | OTel collector |
| `internal/litefs/` | Primary detection helpers, replication status | LiteFS `.primary` file |

### Data Flow

**Sync flow (primary only):**
```
PeeringDB API
  -> HTTP GET /api/{object_type} (for each of 13 object types)
  -> JSON response (may diverge from OpenAPI spec)
  -> Custom deserializer (handles spec discrepancies)
  -> ent Create/Update operations (one transaction per object type)
  -> SQLite write (via modernc.org/sqlite, WAL mode)
  -> LiteFS FUSE intercepts write, creates LTX transaction file
  -> LiteFS replicates LTX to all replica nodes asynchronously
```

**Query flow (all nodes):**
```
Client request (GraphQL / gRPC / REST / Web)
  -> otelhttp/gRPC interceptor middleware (tracing, metrics)
  -> Protocol-specific handler (gqlgen / grpc / entrest / templ)
  -> Generated ent query
  -> SQLite read (via modernc.org/sqlite, read-only connection)
  -> Local disk read (via LiteFS FUSE mount, sub-ms)
  -> Response serialization per protocol
  -> Client
```

**Write forwarding for admin triggers (replicas only):**
```
Replica receives sync trigger request (POST /admin/sync)
  -> Application checks LiteFS .primary file
  -> File exists = we are replica
  -> Returns HTTP response with Fly-Replay: leader header
  -> Fly.io routes request to primary node
  -> Primary executes sync
```

## PeeringDB Data Model

The data model has a clear hierarchy rooted in Organization. This maps directly to entgo schema definitions.

### Core Objects (Basic)

| Object | API Tag | Description | Key Relationships |
|--------|---------|-------------|-------------------|
| Organization | `org` | Root entity; parent of networks, exchanges, facilities | Has many: net, ix, fac, carrier, campus |
| Network | `net` | An autonomous system / network | Belongs to: org. Has many: netfac, netixlan, poc |
| Internet Exchange | `ix` | An Internet Exchange Point | Belongs to: org. Has many: ixlan, ixfac |
| Facility | `fac` | A colocation / data center | Belongs to: org, optionally campus. Has many: netfac, ixfac, carrierfac |
| Network Contact | `poc` | Point of contact / role account | Belongs to: net |

### Derived Objects (Junction / Association)

| Object | API Tag | Description | Key Relationships |
|--------|---------|-------------|-------------------|
| IX LAN | `ixlan` | The LAN segment of an IX (each IX has at least one) | Belongs to: ix. Has many: ixpfx, netixlan |
| IX Prefix | `ixpfx` | IPv4/IPv6 prefix on an IX LAN | Belongs to: ixlan |
| Network-IX LAN | `netixlan` | A network's presence at an IX (the most important derived object) | Belongs to: net, ixlan |
| Network-Facility | `netfac` | A network's presence at a facility | Belongs to: net, fac |
| IX-Facility | `ixfac` | An IX's presence at a facility | Belongs to: ix, fac |

### Newer Objects

| Object | API Tag | Description | Key Relationships |
|--------|---------|-------------|-------------------|
| Carrier | `carrier` | Transport provider between facilities | Belongs to: org. Has many: carrierfac |
| Carrier-Facility | `carrierfac` | A carrier's presence at a facility | Belongs to: carrier, fac |
| Campus | `campus` | A logical group of related facilities | Belongs to: org. Has many: fac |

### Entity Relationship Diagram

```
                       Organization (org)
                      / |    |    \      \
                    /   |    |     \      \
                 Net   IX   Fac  Carrier  Campus
                  |     |    |      |       |
                 poc  ixlan  |   carrierfac fac*
                       |     |     / \
                     ixpfx   |  carrier fac
                       |     |
                    netixlan  |
                     / \      |
                   net ixlan  |
                              |
                           netfac
                            / \
                          net  fac
                              |
                            ixfac
                            / \
                          ix   fac
```

*Campus groups facilities; fac still belongs to org directly.

### Sync Order (Respects FK Dependencies)

```
  1.  org          (no dependencies)
  2.  campus       (depends on: org)
  3.  fac          (depends on: org, optionally campus)
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

This ordering ensures parent objects exist before children are inserted.

## LiteFS Integration Details

### How LiteFS Works (Architectural Understanding)

LiteFS is a FUSE-based filesystem that intercepts SQLite's page-level writes. It operates transparently below the application:

1. **Write capture:** LiteFS intercepts SQLite journal operations. When a transaction commits (journal deletion in rollback mode, or WAL checkpoint), LiteFS extracts the changed pages into an LTX (Lite Transaction) file.
2. **Replication:** LTX files stream from primary to replicas via HTTP on port 20202. This is asynchronous -- writes succeed immediately without waiting for replica acknowledgment.
3. **Consistency:** Rolling CRC64 checksums verify database integrity across nodes. If a former primary rejoins with divergent state, it re-snapshots from the new primary.
4. **Primary election:** A Consul-based lease system ensures exactly one primary at any time. Lease TTL prevents split-brain.

### LiteFS Proxy

LiteFS includes a built-in HTTP proxy that handles consistency and write forwarding:

- **Write requests** (POST, PUT, DELETE): Forwarded to the primary node using the `fly-replay` header. Fly.io's routing layer replays the request on the primary.
- **Read requests** (GET): The replica waits for its replication position to catch up to the position stored in a cookie set by the primary. This provides causal consistency for browser clients.
- **Limitation:** Does not work with WebSockets. Does not understand gRPC (HTTP/2).

For this project, the LiteFS proxy handles REST and GraphQL traffic (HTTP/1.1). gRPC must run on a separate port outside the proxy.

### Primary Detection

Applications determine their role by checking the `/litefs/.primary` file:

```go
// isPrimary returns true if this node holds the LiteFS lease.
// The .primary file exists ONLY on replica nodes (contains primary hostname).
// Its absence means we ARE the primary.
func isPrimary() bool {
    _, err := os.Stat("/litefs/.primary")
    return errors.Is(err, os.ErrNotExist)
}
```

### LiteFS Operational Status

LiteFS Cloud (managed backup service) was sunset in October 2024. LiteFS itself remains open-source and functional but receives limited maintenance from Fly.io. It is described as "fairly stable" but "not 100% polished." This means:

- Production use is viable, but budget extra time for edge-case troubleshooting
- Disaster recovery backups should use Litestream to S3/Tigris, not LiteFS Cloud
- No significant new features expected; the current feature set is sufficient for this project

### litefs.yml Configuration

```yaml
fuse:
  dir: "/litefs"

data:
  dir: "/var/lib/litefs"
  compress: true
  retention: "10m"

exec:
  - cmd: "peeringdb-plus -addr :8081"

http:
  addr: ":20202"

proxy:
  addr: ":8080"
  target: "localhost:8081"
  db: "peeringdb.db"
  passthrough:
    - "/healthz"
    - "/readyz"

lease:
  type: "consul"
  advertise-url: "http://${HOSTNAME}.vm.${FLY_APP_NAME}.internal:20202"
  candidate: ${FLY_REGION == PRIMARY_REGION}
  consul:
    url: "${FLY_CONSUL_URL}"
    key: "peeringdb-plus/primary"
```

## Port Allocation and Protocol Separation

| Port | Service | Protocol | Visibility | Notes |
|------|---------|----------|------------|-------|
| 8080 | LiteFS HTTP proxy | HTTP/1.1 | Public via Fly.io | Entry point for REST, GraphQL, Web UI |
| 8081 | Go application HTTP | HTTP/1.1 | Internal (behind LiteFS proxy) | REST + GraphQL + Web UI + Health |
| 8082 | gRPC server | HTTP/2 | Public via Fly.io | Separate port, not behind LiteFS proxy |
| 20202 | LiteFS replication | HTTP | Private (internal Fly mesh) | Node-to-node LTX streaming |

**Rationale for separate gRPC port:** gRPC requires HTTP/2. The LiteFS proxy only handles HTTP/1.1 and uses `fly-replay` for write forwarding. gRPC traffic through the LiteFS proxy would not function correctly. Fly.io supports multiple port mappings per service, so a dedicated gRPC port is the cleanest approach. Since this is a read-only mirror, gRPC does not need write forwarding.

## Patterns to Follow

### Pattern 1: Schema-Driven Code Generation

**What:** Define all data structures in ent schema files. Generate GraphQL (entgql), gRPC (entproto), and REST (entrest) from those schemas using annotations.
**When:** Always. This is the core architectural decision.
**Why:** Single source of truth prevents API surface drift. Schema changes automatically propagate to all three protocols after code regeneration.

```go
// ent/schema/network.go
package schema

import (
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/edge"
    "entgo.io/contrib/entgql"
    "entgo.io/contrib/entproto"
    "github.com/lrstanley/entrest"
)

type Network struct {
    ent.Schema
}

func (Network) Fields() []ent.Field {
    return []ent.Field{
        field.Int("asn").
            Unique().
            Positive().
            Annotations(
                entgql.OrderField("ASN"),
                entproto.Field(2),
                entrest.WithFilter(entrest.FilterGroupArray|entrest.FilterGroupEqual),
            ),
        field.String("name").
            NotEmpty().
            Annotations(
                entgql.OrderField("NAME"),
                entproto.Field(3),
                entrest.WithFilter(entrest.FilterGroupContains),
            ),
    }
}

func (Network) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("netixlans", NetIXLan.Type).
            Annotations(entgql.RelayConnection()),
        edge.To("netfacs", NetFac.Type).
            Annotations(entgql.RelayConnection()),
        edge.From("org", Organization.Type).
            Ref("networks").
            Unique().
            Required(),
    }
}
```

### Pattern 2: Transaction-per-Object-Type Sync

**What:** Each sync cycle fetches all PeeringDB objects and upserts them into SQLite with one transaction per object type, following FK dependency order.
**When:** Every sync cycle (hourly or on-demand).
**Why:** SQLite uses database-level write locks. A single massive transaction spanning all 13 object types would lock reads for the entire sync duration (potentially minutes). Transaction-per-type keeps lock durations short (seconds) while maintaining per-type atomicity.

```go
func (s *Syncer) Sync(ctx context.Context) error {
    // Sync in FK dependency order, one transaction per type
    types := []struct {
        name string
        fn   func(ctx context.Context, tx *ent.Tx) error
    }{
        {"org", s.syncOrganizations},
        {"campus", s.syncCampuses},
        {"fac", s.syncFacilities},
        // ... remaining types in dependency order
        {"netixlan", s.syncNetIXLans},
    }

    for _, t := range types {
        tx, err := s.writeClient.Tx(ctx)
        if err != nil {
            return fmt.Errorf("begin %s sync: %w", t.name, err)
        }
        if err := t.fn(ctx, tx); err != nil {
            tx.Rollback()
            return fmt.Errorf("sync %s: %w", t.name, err)
        }
        if err := tx.Commit(); err != nil {
            return fmt.Errorf("commit %s sync: %w", t.name, err)
        }
    }
    return nil
}
```

**Important trade-off:** Transaction-per-type means a failed sync could leave a partially-updated database (e.g., updated orgs but stale networks). This is acceptable because: (a) a full re-fetch on the next cycle will fix it, (b) partial freshness is better than stale-everything, and (c) the alternative (single transaction) blocks all reads during sync.

### Pattern 3: Separate Read and Write ent Clients

**What:** Open two separate ent.Client instances on the primary node: one read-only for API serving, one read-write for sync. Replicas only open the read-only client.
**When:** Application startup on every node.
**Why:** SQLite connection pragmas differ between read and write paths. Read-only mode (`?mode=ro`) prevents accidental writes from API handlers. The write client needs `_journal_mode=WAL` and `_busy_timeout=5000`.

```go
// Read-only client (all nodes)
readClient := ent.Open("sqlite3",
    "file:/litefs/peeringdb.db?mode=ro&_journal_mode=WAL&cache=shared")

// Write client (primary only)
if isPrimary() {
    writeClient := ent.Open("sqlite3",
        "file:/litefs/peeringdb.db?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL")
}
```

### Pattern 4: Primary-Aware Request Handling

**What:** All nodes serve reads. Only primary handles sync writes. Replicas forward write-like operations via Fly-Replay header.
**When:** Any request that would modify data (sync trigger, admin operations).
**Why:** LiteFS replication is single-writer. Attempting writes on replicas fails with SQLITE_READONLY.

```go
func (s *Server) handleSyncTrigger(w http.ResponseWriter, r *http.Request) {
    if !s.isPrimary() {
        w.Header().Set("Fly-Replay", "leader")
        w.WriteHeader(http.StatusTemporaryRedirect)
        return
    }
    // Trigger sync on primary
    go s.syncer.Sync(r.Context())
    w.WriteHeader(http.StatusAccepted)
}
```

### Pattern 5: Layered Middleware Stack

**What:** Compose middleware in a consistent order across all HTTP endpoints.
**When:** All HTTP handlers (GraphQL, REST, health, web UI).
**Why:** Ensures observability, recovery, and logging are applied uniformly.

```go
// Middleware order matters:
// 1. Recovery (catch panics, return 500)
// 2. Request ID (assign trace correlation ID)
// 3. OTel HTTP (tracing spans + metrics)
// 4. CORS (browser access for GraphQL playground)
// 5. Structured logging (slog with request context)
// 6. Handler
```

### Pattern 6: Configuration via Environment

**What:** All configuration from environment variables, validated at startup, fail-fast.
**When:** Always. Per project coding standards CFG-1, CFG-2.
**Why:** Fly.io deployment uses environment variables. LiteFS primary status is runtime-detected, not configured.

```go
type Config struct {
    PeeringDBURL    string        // PEERINGDB_URL (default: https://www.peeringdb.com/api)
    SyncInterval    time.Duration // SYNC_INTERVAL (default: 1h)
    DatabasePath    string        // DATABASE_PATH (default: /litefs/peeringdb.db)
    HTTPAddr        string        // HTTP_ADDR (default: :8081)
    GRPCAddr        string        // GRPC_ADDR (default: :8082)
    OTelEndpoint    string        // OTEL_EXPORTER_OTLP_ENDPOINT
    PrimaryRegion   string        // PRIMARY_REGION (Fly.io region for primary)
}
```

## Anti-Patterns to Avoid

### Anti-Pattern 1: Single Transaction for Entire Sync

**What:** Wrapping all PeeringDB object upserts in one giant transaction.
**Why bad:** SQLite locks the entire database during a write transaction. A single transaction spanning all 13 object types with hundreds of thousands of rows would block all reads on the primary for the entire sync duration (potentially minutes). WAL mode allows concurrent reads, but only from data committed before the write transaction started.
**Instead:** One transaction per object type. Each commits independently, allowing read queries to see progressively fresher data between type syncs.

### Anti-Pattern 2: Separate Sync Service

**What:** Running the sync client as a separate binary/process from the API server.
**Why bad:** Doubles operational complexity. Requires inter-process communication for sync status. Deployment becomes two things instead of one. LiteFS already handles the primary/replica distinction.
**Instead:** Single binary with sync goroutine on primary, API handlers on all nodes.

### Anti-Pattern 3: Caching Layer in Front of SQLite

**What:** Adding Redis/Memcached between the API handlers and SQLite.
**Why bad:** SQLite reads from local disk are already sub-millisecond. A cache adds a network hop, cache invalidation complexity, and a dependency. The entire PeeringDB dataset (~100MB) fits comfortably in SQLite's page cache.
**Instead:** Use SQLite's built-in page cache. Set appropriate `PRAGMA cache_size` and `PRAGMA mmap_size`.

### Anti-Pattern 4: Custom SQL Queries Bypassing Ent

**What:** Writing raw SQL queries instead of using the generated ent client.
**Why bad:** Breaks the schema-driven generation contract. Raw queries are not reflected in GraphQL/gRPC/REST surfaces. Loses type safety and ent's query building, filtering, and pagination.
**Instead:** Always use ent client methods. If ent cannot express a query, add it as an ent template extension or custom predicate.

### Anti-Pattern 5: Write-Through LiteFS Proxy for Sync

**What:** Having any node fetch PeeringDB data and forwarding writes through the LiteFS proxy's fly-replay mechanism to the primary.
**Why bad:** The sync worker fetches megabytes of data from PeeringDB. Running it on a replica means data is fetched remotely, then forwarded through Fly's internal routing to the primary, doubling network traffic. The fly-replay mechanism is designed for small user-initiated writes, not bulk data loading.
**Instead:** Only start the sync worker goroutine on the primary node. Replicas never fetch from PeeringDB or attempt writes.

### Anti-Pattern 6: cmux for gRPC + HTTP on One Port

**What:** Using `cmux` to multiplex gRPC (HTTP/2) and REST/GraphQL (HTTP/1.1) on the same listener.
**Why bad:** The LiteFS proxy only understands HTTP/1.1. gRPC traffic through the proxy would fail. cmux also complicates TLS termination, which Fly.io handles at the edge.
**Instead:** Separate ports. HTTP on :8081 behind LiteFS proxy (:8080). gRPC on :8082 directly exposed.

### Anti-Pattern 7: Incremental Sync with Change Detection

**What:** Trying to sync only changed objects from PeeringDB using `since` parameter or ETags.
**Why bad:** PeeringDB's incremental sync mechanisms are unreliable. Change detection across 13 object types with complex relationships creates subtle consistency bugs. Full dataset is only ~100MB.
**Instead:** Full re-fetch with upsert. The dataset is small enough that a complete sync completes in seconds to low minutes.

## SQLite Configuration

```sql
-- WAL mode for concurrent reads during sync writes
PRAGMA journal_mode = WAL;

-- Synchronous NORMAL is safe with LiteFS (LiteFS handles durability)
PRAGMA synchronous = NORMAL;

-- Cache size: 64MB (entire PeeringDB dataset fits)
PRAGMA cache_size = -65536;

-- Memory-mapped I/O for read performance
PRAGMA mmap_size = 268435456;  -- 256MB

-- Foreign keys for data integrity
PRAGMA foreign_keys = ON;

-- Busy timeout for write contention during sync (primary only)
PRAGMA busy_timeout = 5000;
```

## Application Router Structure

```go
// cmd/server/main.go - simplified structure
func main() {
    cfg := loadConfig()
    primary := isPrimary()

    // Read-only ent client (all nodes)
    readClient := openReadClient(cfg.DatabasePath)
    defer readClient.Close()

    // HTTP router (REST + GraphQL + Web UI + Health)
    mux := http.NewServeMux()
    mux.Handle("/graphql", graphqlHandler(readClient))
    mux.Handle("/api/", restHandler(readClient))        // entrest
    mux.Handle("/", webUIHandler(readClient))            // templ + htmx
    mux.Handle("/healthz", healthHandler(readClient))
    mux.Handle("/readyz", readyHandler(readClient))

    // gRPC server (separate listener, all nodes)
    grpcServer := grpc.NewServer(otelGRPCInterceptors()...)
    registerServices(grpcServer, readClient)

    // Sync worker (primary only)
    if primary {
        writeClient := openWriteClient(cfg.DatabasePath)
        defer writeClient.Close()
        syncer := sync.New(writeClient, cfg.PeeringDBURL)
        mux.Handle("POST /admin/sync", syncTriggerHandler(syncer))
        go syncer.RunSchedule(ctx, cfg.SyncInterval)
    }

    // Start servers
    go grpcServer.Serve(grpcListener(cfg.GRPCAddr))
    http.ListenAndServe(cfg.HTTPAddr, otelMiddleware(mux))
}
```

## Project Directory Structure

```
peeringdb-plus/
  cmd/
    server/
      main.go                # Application entry point, wiring
  internal/
    sync/
      worker.go              # Sync scheduler and orchestrator
      client.go              # PeeringDB API HTTP client
      transform.go           # JSON response -> ent mutation transforms
      types.go               # PeeringDB response types (handles spec divergences)
    server/
      graphql.go             # GraphQL handler setup (gqlgen + entgql)
      grpc.go                # gRPC server setup (entproto)
      rest.go                # REST handler setup (entrest)
      web.go                 # HTMX + Templ web UI handler
      health.go              # Health, readiness, sync status endpoints
      middleware.go          # HTTP middleware stack
    otel/
      provider.go            # OTel SDK initialization, exporters
      shutdown.go            # Graceful shutdown with span/metric flush
    litefs/
      primary.go             # Primary detection via .primary file
  ent/
    schema/
      organization.go        # entgo schema: Organization (org)
      network.go             # entgo schema: Network (net)
      internet_exchange.go   # entgo schema: InternetExchange (ix)
      facility.go            # entgo schema: Facility (fac)
      ix_lan.go              # entgo schema: IXLan (ixlan)
      ix_prefix.go           # entgo schema: IXPrefix (ixpfx)
      ix_facility.go         # entgo schema: IXFacility (ixfac)
      network_ixlan.go       # entgo schema: NetworkIXLan (netixlan)
      network_facility.go    # entgo schema: NetworkFacility (netfac)
      network_contact.go     # entgo schema: NetworkContact (poc)
      carrier.go             # entgo schema: Carrier (carrier)
      carrier_facility.go    # entgo schema: CarrierFacility (carrierfac)
      campus.go              # entgo schema: Campus (campus)
    entc.go                  # Code generation config (entgql, entproto, entrest extensions)
    generate.go              # go:generate directive
    ...                      # Generated code
  graph/
    schema.graphqls          # Generated + custom GraphQL schema
    resolver.go              # Generated + custom resolvers
  proto/
    peeringdb/
      v1/
        *.proto              # Generated protobuf definitions
        *_grpc.pb.go         # Generated gRPC service implementations
  web/
    templates/
      *.templ                # Templ component templates
    static/
      htmx.min.js            # HTMX library
  litefs.yml                 # LiteFS configuration
  fly.toml                   # Fly.io deployment configuration
  Dockerfile                 # Multi-stage build (Go build -> distroless + litefs)
  go.mod
  go.sum
```

## Scalability Considerations

| Concern | At 100 users | At 10K users | At 1M users |
|---------|--------------|--------------|-------------|
| Read latency | Sub-ms (local SQLite) | Sub-ms (same) | Sub-ms (same, inherent to architecture) |
| Concurrent queries | Single node, trivial | Multiple Fly.io regions via LiteFS | More regions, same architecture |
| Sync impact on reads | Negligible (WAL mode) | Negligible (WAL mode) | Negligible (WAL mode, readers don't block) |
| Replication lag | Sub-second | Sub-second | Sub-second (LiteFS async, Fly internal network) |
| Data size | ~100MB SQLite file | Same (PeeringDB data, not user data) | Same |
| Node count | 1-2 regions | 3-5 regions | 10+ regions |
| Sync frequency | Hourly | Hourly | Hourly (upstream rate limits may apply) |
| API bandwidth | Negligible | Fly.io edge handles distribution | May need CDN for static web assets |

**Key insight:** This is a read-only mirror of a fixed-size dataset (~100MB). The architecture scales by adding read replicas (Fly.io regions), not by sharding or partitioning. The bottleneck is PeeringDB's upstream API during sync, not serving capacity.

## Suggested Build Order (Dependencies)

```
Phase 1: ent Schema + SQLite
    |
    v
Phase 2: Sync Worker + PeeringDB Client
    |
    +---> Phase 3: GraphQL API (primary surface)
    |
    +---> Phase 4: REST + gRPC APIs (secondary surfaces, can parallel with 3)
    |
    v
Phase 5: LiteFS + Fly.io Deployment
    |
    v
Phase 6: Web UI (lowest priority, depends on deployed system)
```

**OTel is woven into every phase**, not a separate phase. Each component gets instrumented as it is built.

### Phase 1: Foundation (ent Schema + SQLite)
Build all 13 entgo schema definitions matching PeeringDB's data model. Verify schema compiles, migrations run against SQLite, and basic CRUD works. This is the foundation everything depends on.

### Phase 2: Sync Worker
Build the PeeringDB API client and sync worker. Handle the API response format divergences here (this is the hardest part -- PeeringDB responses don't match their OpenAPI spec). Populates the database with real data needed to test API surfaces.

### Phase 3: GraphQL API
Generate and customize the GraphQL API via entgql + gqlgen. This is the primary API surface and exercises the full entgo query path including Relay cursor pagination, filtering, and edge traversal.

### Phase 4: REST + gRPC APIs
Generate REST (entrest) and gRPC (entproto) surfaces. These are secondary and largely come "for free" from entgo schema annotations, but need testing and may need custom adjustments.

### Phase 5: LiteFS + Fly.io Deployment
Add LiteFS configuration, Dockerfile, fly.toml. Deploy to Fly.io with primary + replica nodes. Wire up sync worker primary detection, backup via Litestream.

### Phase 6: Web UI
HTMX + Templ browser interface. Explicitly lowest priority per project requirements.

## Deployment Architecture on Fly.io

```
Fly.io Anycast Edge
        |
   +----+----+
   |         |
Region A   Region B   Region C ...
(primary)  (replica)  (replica)
   |         |           |
LiteFS     LiteFS      LiteFS
primary    replica      replica
   |         |           |
Go binary  Go binary   Go binary
(sync+API) (API only)  (API only)
   |
Consul       Litestream -> Tigris (S3-compatible)
(lease)      (backup, since LiteFS Cloud is sunset)
```

**Primary region:** Set via `PRIMARY_REGION` env var in fly.toml. Only candidate nodes in this region attempt to acquire the Consul lease. If the primary fails, another candidate in the same region takes over.

**Replicas:** Every other region runs a read-only copy. LiteFS replicates asynchronously with sub-second lag within Fly.io's internal WireGuard mesh.

**Backups:** Use Litestream to stream WAL changes to Tigris (Fly.io's S3-compatible object storage) for disaster recovery. This replaces the sunset LiteFS Cloud.

## Sources

- [LiteFS Architecture (GitHub)](https://github.com/superfly/litefs/blob/main/docs/ARCHITECTURE.md) - HIGH confidence
- [How LiteFS Works (Fly Docs)](https://fly.io/docs/litefs/how-it-works/) - HIGH confidence
- [LiteFS Primary Detection (Fly Docs)](https://fly.io/docs/litefs/primary/) - HIGH confidence
- [LiteFS Config Reference (Fly Docs)](https://fly.io/docs/litefs/config/) - HIGH confidence
- [LiteFS HTTP Proxy (Fly Docs)](https://fly.io/docs/litefs/proxy/) - HIGH confidence
- [LiteFS Status Discussion (Fly Community)](https://community.fly.io/t/what-is-the-status-of-litefs/23883) - MEDIUM confidence
- [LiteFS Cloud Sunset (Fly Community)](https://community.fly.io/t/sunsetting-litefs-cloud/20829) - HIGH confidence
- [entgo GitHub](https://github.com/ent/ent) - HIGH confidence
- [entgql GraphQL Integration (entgo docs)](https://entgo.io/docs/graphql/) - HIGH confidence
- [entproto gRPC Generation (entgo blog)](https://entgo.io/blog/2021/03/18/generating-a-grpc-server-with-ent/) - MEDIUM confidence
- [entrest by lrstanley (GitHub)](https://github.com/lrstanley/entrest) - HIGH confidence
- [entrest Documentation](https://lrstanley.github.io/entrest/) - HIGH confidence
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) - HIGH confidence
- [PeeringDB Carrier Objects (blog)](https://docs.peeringdb.com/blog/carrier_object_deployed/) - MEDIUM confidence
- [django-peeringdb Models (GitHub)](https://github.com/peeringdb/django-peeringdb) - HIGH confidence
- [cmux Connection Multiplexer (GitHub)](https://github.com/soheilhy/cmux) - HIGH confidence (referenced but not recommended)
- [Templ + HTMX SSR Guide](https://templ.guide/server-side-rendering/htmx/) - MEDIUM confidence
- [OTel Ent Instrumentation (Uptrace)](https://uptrace.dev/guides/opentelemetry-ent) - MEDIUM confidence
- [Fly-Replay Header (Fly Docs)](https://fly.io/docs/networking/dynamic-request-routing/) - HIGH confidence
