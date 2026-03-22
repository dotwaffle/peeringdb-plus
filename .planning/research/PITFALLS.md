# Domain Pitfalls

**Domain:** Adding OTel HTTP client tracing, sync metrics, entrest REST API, and PeeringDB-compatible REST layer to existing Go/entgo/SQLite/OTel project
**Researched:** 2026-03-22
**Milestone:** v1.1 REST API & Observability

## Critical Pitfalls

Mistakes that cause rewrites, broken compatibility, or fundamental integration problems.

### Pitfall 1: otelhttp Transport Creates One Span Per HTTP Round-Trip, Not Per Logical Request

**What goes wrong:** The PeeringDB client already implements retry logic with exponential backoff in `doWithRetry()`. Wrapping the `http.Client` transport with `otelhttp.NewTransport()` creates a new span for every individual HTTP round-trip. With 3 retries, a single logical "fetch organizations page 0" operation produces 3 sibling HTTP spans instead of one parent span with retry events. The trace tree becomes flat and unreadable -- you see dozens of HTTP client spans without understanding which are retries of the same request.

**Why it happens:** `otelhttp.NewTransport` wraps `http.RoundTripper`, which sits below the retry loop. The OpenTelemetry HTTP semantic conventions actually recommend creating a span per attempt (`http.request.resend_count` attribute), but the retry logic in `doWithRetry()` creates its own context flow. Without careful span nesting, the retry spans become orphans or flat siblings rather than children of the logical operation span.

**Consequences:** Traces show many HTTP client spans without grouping by logical PeeringDB fetch operation. Debugging a slow sync requires manually correlating which HTTP spans belong to the same retried request. Alert rules based on span counts produce false positives. The existing `ctx` passed through `doWithRetry` already has a parent span from the sync worker, but the transport-level spans lack the "retry attempt N of M" context.

**Prevention:**
- Create an explicit parent span in `FetchAll()` for the logical fetch operation (e.g., `peeringdb.fetch.{objectType}`), and a child span per page fetch in `doWithRetry()`.
- Set the `otelhttp.NewTransport` on the `http.Client` for automatic HTTP-level spans, but ensure these are children of the per-request span from `doWithRetry()` by passing the correct `ctx` through `http.NewRequestWithContext()` -- which the code already does.
- Add `http.request.resend_count` as an attribute on retry spans per OTel semantic conventions.
- Do NOT wrap the entire retry loop in a single span that ends only after all retries -- this hides individual attempt durations. Instead: logical operation span > per-attempt span > HTTP transport span.
- Test with a mock HTTP server that returns 429s to verify the span hierarchy looks correct.

**Detection:** Flat trace trees with many HTTP client spans at the same level. Missing parent-child relationships in trace visualization. Span counts per sync not matching expected page counts.

**Phase impact:** OTel HTTP client tracing task. Must design span hierarchy before instrumenting.

**Confidence:** HIGH -- based on OpenTelemetry HTTP semantic conventions for retry instrumentation and inspection of the existing `doWithRetry()` implementation.

---

### Pitfall 2: entrest Response Format Does Not Match PeeringDB's Response Envelope

**What goes wrong:** PeeringDB wraps all API responses in `{"meta": {}, "data": [...]}`. entrest generates its own response format -- entities are returned as JSON arrays or objects directly, with pagination metadata in HTTP headers or a different JSON structure. Building both "entrest REST API" and "PeeringDB-compatible REST API" on the same codebase without recognizing they have fundamentally different response shapes leads to one of two bad outcomes: (a) trying to force entrest to produce PeeringDB-format responses, which fights the code generator, or (b) building a single endpoint that cannot satisfy both formats.

**Why it happens:** These are genuinely different requirements that share some code but have different API contracts:
- **entrest REST API**: modern OpenAPI-compliant REST with entrest's pagination, filtering, and response format. New API surface for this project.
- **PeeringDB compat REST API**: must exactly reproduce PeeringDB's response envelope (`{"meta": {}, "data": [...]}`), URL paths (`/api/net`, `/api/fac`), query parameters (`limit`, `skip`, `depth`, `since`, `__contains`, `__in`, `__lt`), and snake_case field names.

entrest generates field names from ent schema field names (which are Go-style: `name_long`, `org_id`), but the response structure, pagination mechanism, and filtering query parameter syntax are all entrest-specific and different from PeeringDB's conventions.

**Consequences:** If treated as one API surface, neither works correctly. Existing PeeringDB API consumers (scripts, peering tools, peeringdb-py) cannot use the entrest API because the response format and query parameters differ. New users cannot use the PeeringDB compat API because it lacks entrest's richer filtering and Relay-style pagination.

**Prevention:**
- Treat these as two separate API surfaces mounted at different paths:
  - `/api/v1/...` for entrest-generated OpenAPI REST (new, modern)
  - `/api/net`, `/api/fac`, `/api/ix`, etc. for PeeringDB-compatible endpoints (compat layer)
- Build the PeeringDB compat layer as hand-written handlers that query ent directly and serialize responses in PeeringDB's envelope format. Do NOT try to wrap or transform entrest output.
- entrest handles its own URL structure, filtering, and pagination. The compat layer handles PeeringDB's `limit`/`skip`/`depth`/`since`/`__contains`/`__in` query parameters independently.
- Define clear interfaces: compat layer uses `*ent.Client` to query; entrest uses its generated handlers.

**Detection:** PeeringDB client tools (peeringdb-py, django-peeringdb) failing to parse responses from the compat endpoint. entrest endpoints returning data in unexpected format.

**Phase impact:** REST API design phase. The two-surface decision must be made before any implementation begins.

**Confidence:** HIGH -- based on analysis of PeeringDB's documented API envelope format (`{"meta": {}, "data": [...]}`), entrest's generated response structure, and the existing PeeringDB response types in `internal/peeringdb/types.go`.

---

### Pitfall 3: PeeringDB Field Names vs. Ent Schema Field Names -- The Compat Layer Serialization Gap

**What goes wrong:** PeeringDB uses snake_case field names in JSON responses that exactly match the Django model field names: `org_id`, `name_long`, `info_prefixes4`, `irr_as_set`, `ixf_ixp_member_list_url_visible`, etc. The ent schema stores these using the same field names (verified in `ent/schema/network.go`), but ent's generated JSON serialization and entrest's output may not produce identical field names. Additionally, PeeringDB includes computed fields in responses (`ix_count`, `fac_count`, `net_count`, `org_name`) that are stored as fields in the ent schema but may not appear in all API responses from entrest.

**Why it happens:** entrest generates JSON output from ent's type system. While the ent schema field names match PeeringDB's (`org_id`, `name_long`), the ent-generated Go struct JSON tags and entrest's serialization may transform these names. Ent's JSON output uses the field names as defined in the schema, but edge traversal (e.g., `org_id` being both a field and a FK reference to the `organization` edge) can produce different output depending on whether the edge is eagerly loaded.

The PeeringDB compat layer must also reproduce fields that PeeringDB includes but that are not standard ent fields: `org_name` on Facility/Carrier/Campus objects is a denormalized field from the Organization. PeeringDB also includes `_set` fields when `depth > 0` (e.g., `net_set`, `fac_set`) that contain nested arrays of related objects.

**Consequences:** Drop-in PeeringDB compatibility fails. Existing scripts that parse PeeringDB responses by exact field name break. The `depth` parameter behavior (which changes response shape) is particularly dangerous -- PeeringDB consumers expect `depth=0` to return IDs and `depth=1` to expand sets.

**Prevention:**
- For the PeeringDB compat layer, use custom Go structs for serialization that exactly match PeeringDB's JSON field names (the types already exist in `internal/peeringdb/types.go` -- reuse these as the response serialization format, not just the deserialization format).
- Map ent query results to these PeeringDB response structs explicitly. Do not rely on ent's or entrest's JSON serialization.
- For entrest's own endpoints, let entrest control its own serialization -- these are a new API, not constrained by PeeringDB format.
- Handle `depth` parameter in the compat layer by conditionally eager-loading ent edges and including `_set` fields in the response.
- Write golden file tests comparing actual PeeringDB API responses against compat layer responses for each of the 13 object types.

**Detection:** Field name mismatches in JSON output. Missing computed fields. `depth` parameter producing wrong response shape. PeeringDB client libraries failing to deserialize responses.

**Phase impact:** PeeringDB compat layer implementation. Must define the serialization strategy before writing handlers.

**Confidence:** HIGH -- based on direct comparison of `internal/peeringdb/types.go` field names against `ent/schema/network.go` field definitions and PeeringDB API documentation.

---

### Pitfall 4: MeterProvider Not Initialized -- Sync Metrics Record to No-Op

**What goes wrong:** The existing OTel provider (`internal/otel/provider.go`) only initializes a `TracerProvider` -- there is no `MeterProvider` setup. The sync metrics were "registered but not recorded" (per the v1.0 tech debt audit). If the v1.1 work expands metrics and wires them to `Record()` calls without first adding a `MeterProvider` to the OTel initialization, all metric recordings silently go to the OTel no-op meter. Everything compiles, tests pass, but no metrics appear in any backend.

**Why it happens:** The OTel API is designed so that calling `otel.Meter("sync")` always returns a valid `Meter` object, even if no `MeterProvider` is configured. Instruments created from this meter accept `Add()` and `Record()` calls without error. The no-op implementation is intentional (avoids panics in library code), but it means metric recording silently drops data when the SDK is not set up.

**Consequences:** Developers wire up metrics, verify they compile, maybe even write unit tests that don't check the actual export pipeline, and ship. In production, no sync metrics appear in dashboards. The gap may not be noticed for days or weeks because the application works fine functionally.

**Prevention:**
- Add `MeterProvider` initialization alongside `TracerProvider` in `internal/otel/provider.go`. Return both shutdown functions.
- Use the same autoexport pattern already planned for traces: `autoexport.NewMetricReader()` to allow environment-variable-driven metric export configuration.
- Write a test that verifies at least one metric is actually recorded during a sync cycle by using an in-memory metric reader.
- Verify in the integration test that `otel.GetMeterProvider()` returns a non-no-op provider.

**Detection:** No metrics appearing in the OTel backend despite `Record()` calls being present in the code. Using `otel.GetMeterProvider()` and checking if it returns the default no-op provider.

**Phase impact:** Sync metrics expansion task. The MeterProvider MUST be initialized BEFORE any metric recording code is written.

**Confidence:** HIGH -- verified by reading `internal/otel/provider.go` which only creates a `TracerProvider`, and confirmed by OTel Go documentation that uninitialized meters produce no-op instruments.

---

## Moderate Pitfalls

### Pitfall 5: entrest and entgql Extensions May Conflict on Schema Annotations

**What goes wrong:** The existing ent schemas have `entgql.RelayConnection()`, `entgql.QueryField()`, and `entgql.OrderField()` annotations. Adding entrest as a second extension to the same `entc.Generate()` call requires its own annotations (`entrest.WithIncludeOperations()`, `entrest.WithSkip()`, etc.). Both extensions process the same schema annotations, and entrest may interpret or conflict with entgql annotations, particularly around:
- Edge handling: entgql uses Relay connections; entrest uses its own pagination.
- Field filtering: entgql uses `WhereInput`; entrest uses query parameters.
- Operation scope: This project is read-only but entrest generates CRUD handlers by default.

**Why it happens:** entgql and entrest are independent extensions with separate annotation systems. The ent extension API allows multiple extensions, but neither was designed to be aware of the other's annotations. Both generate code from the same schema definitions, and conflicts surface at code generation time (template collisions) or at runtime (handler registration conflicts).

**Prevention:**
- Add the entrest extension to `ent/entc.go` alongside the existing entgql extension, both in the `entc.Extensions()` call.
- Use `entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList)` on every schema to ensure only read operations are generated (this is a read-only mirror).
- If entrest generates handler code that conflicts with existing gqlgen handlers, use `entrest.WithHandler(false)` on specific schemas and mount handlers manually.
- Test code generation with both extensions before writing any handler mounting code. Run `go generate` and verify both GraphQL schemas and REST handlers compile.
- Pin entrest version strictly in `go.mod` -- per GitHub issue #106, `WithIncludeOperations` may disable edges unexpectedly.

**Detection:** Code generation failures. Compilation errors in generated code. REST endpoints returning unexpected responses because entrest generated write handlers for a read-only database. Edge endpoints missing or behaving differently than expected.

**Phase impact:** entrest integration task. Must validate code generation compatibility before building any REST handlers.

**Confidence:** MEDIUM -- entrest and entgql use separate annotation namespaces, but runtime handler conflicts and edge behavior (GitHub issue #106) are documented concerns.

---

### Pitfall 6: PeeringDB Query Parameter Compatibility Is Much Deeper Than URL Paths

**What goes wrong:** Teams implement PeeringDB URL path compatibility (`/api/net`, `/api/fac`) and basic response format, then discover that PeeringDB consumers rely heavily on the query parameter filtering syntax. PeeringDB supports Django-style query lookups: `?name__contains=Hurricane`, `?asn__in=13335,174,3356`, `?info_prefixes4__gte=1000`, `?updated__gt=2024-01-01T00:00:00Z`. These are not standard REST query parameters -- they use double-underscore suffixes (`__contains`, `__startswith`, `__in`, `__lt`, `__lte`, `__gt`, `__gte`) that are specific to Django's ORM queryset filtering.

**Why it happens:** PeeringDB's API is built on Django REST Framework, which automatically exposes the Django ORM's `__lookup` filter syntax as query parameters. Every field on every model can be filtered with these suffixes. Tools like peeringdb-py, peeringctl, and custom scripts use these filters extensively. A PeeringDB compat layer that doesn't support them is not actually compatible.

**Consequences:** Existing PeeringDB tools that use filtered queries return full unfiltered datasets (if the params are ignored) or error out (if the params cause parse failures). Users discover incompatibility only when their scripts start returning wrong data or timing out from fetching full datasets.

**Prevention:**
- Catalog the complete set of PeeringDB query parameter filters by examining the Django REST Framework viewsets in the PeeringDB source code. The key patterns are:
  - Exact match: `?field=value`
  - Contains: `?field__contains=value`
  - Starts with: `?field__startswith=value`
  - In list: `?field__in=val1,val2,val3`
  - Numeric comparison: `?field__lt=N`, `?field__lte=N`, `?field__gt=N`, `?field__gte=N`
  - Since timestamp: `?since=unix_timestamp`
  - Field selection: `?fields=field1,field2`
  - Pagination: `?limit=N&skip=N`
  - Depth expansion: `?depth=0|1|2`
- Translate these Django-style filters into ent predicates using the generated `Where` functions. Ent's predicate system maps well: `__contains` -> `field.Contains()`, `__in` -> `field.In()`, etc.
- Build a generic query parameter parser that handles the `field__lookup` syntax and maps it to ent predicates dynamically.
- Prioritize the most commonly used filters first: exact match, `__in`, `__contains`, `limit`/`skip`, `since`. The less common ones (`__startswith`, numeric comparisons) can be added incrementally.

**Detection:** Test with real PeeringDB consumer tools (peeringdb-py `fetch_all` with filters). Compare query results against the real PeeringDB API.

**Phase impact:** PeeringDB compat layer implementation. The query parameter parser is the most complex component of the compat layer.

**Confidence:** HIGH -- Django-style filter syntax is well-documented in PeeringDB API specs and confirmed by examining PeeringDB's Python source.

---

### Pitfall 7: Duplicate Metric Instrument Registration Across Sync Worker Lifecycle

**What goes wrong:** The sync worker runs on a schedule (hourly). If OTel metric instruments (counters, histograms) are created inside the `Sync()` method rather than at worker construction time, each sync cycle re-registers instruments with the same name. The OTel Go SDK handles duplicate registrations by returning the existing instrument if the definition matches exactly, but logs a warning if definitions differ (e.g., description text changes between runs). If instruments are created with slightly different parameters across code changes (description typo, unit change), the SDK returns a valid but semantically incorrect instrument.

**Why it happens:** A natural pattern is to create instruments where they're used: `meter.Int64Counter("sync.objects.total")` inside `Sync()`. This works on the first call but creates duplicates on subsequent calls. The OTel specification says duplicate instrument registration MUST return a functional instrument, but it also says implementations SHOULD log a warning for conflicting definitions (same name, different description/unit/kind).

**Consequences:** Log spam from duplicate registration warnings. Subtle metric bugs if instrument definitions drift between code versions. Performance overhead from repeated registration calls (minor but unnecessary).

**Prevention:**
- Create all metric instruments once during `Worker` construction or via a `sync.Once` initializer. Store them as fields on the `Worker` struct.
- Define instrument names, descriptions, and units as package-level constants.
- Use the naming convention `peeringdb_plus.sync.*` for sync metrics to avoid collision with other instrumentation.
- Standard instruments for sync:
  - `peeringdb_plus.sync.duration` (Float64Histogram, seconds) -- full sync duration
  - `peeringdb_plus.sync.objects.total` (Int64Counter) -- objects synced per type
  - `peeringdb_plus.sync.errors.total` (Int64Counter) -- sync errors per type
  - `peeringdb_plus.sync.status` (Int64UpDownCounter or Gauge) -- last sync status (success/failure)
  - `peeringdb_plus.sync.http.requests.total` (Int64Counter) -- PeeringDB API requests made
  - `peeringdb_plus.sync.http.duration` (Float64Histogram, seconds) -- PeeringDB API request duration

**Detection:** OTel SDK log warnings about duplicate instrument registration. Metric values not updating after first sync cycle.

**Phase impact:** Sync metrics expansion task. Instrument creation pattern must be decided before implementing individual metrics.

**Confidence:** HIGH -- based on OTel Go SDK documentation for duplicate instrument registration behavior (GitHub issue #3229).

---

### Pitfall 8: LiteFS Read-Only Replicas Will Reject entrest Write Handler Attempts

**What goes wrong:** entrest generates CRUD handlers by default (Create, Read, Update, Delete). Even if only Read/List operations are configured via annotations, the generated OpenAPI spec may still describe write operations, and HTTP requests to those endpoints on a LiteFS replica will hit SQLite with write operations that fail because the database is read-only. On the primary, write operations will attempt to modify PeeringDB data in the local mirror, corrupting the synced dataset.

**Why it happens:** entrest is designed for full CRUD applications. A read-only mirror is an unusual use case. If the schema annotations don't correctly exclude all write operations, or if a future entrest upgrade adds new default operations, write endpoints silently appear in the API.

**Consequences:** On replicas: 500 errors with "attempt to write a readonly database" SQLite errors. On primary: data corruption if a client POSTs/PUTs to the REST API and modifies synced data, which then replicates to all replicas.

**Prevention:**
- Use `entrest.WithIncludeOperations(entrest.OperationRead, entrest.OperationList)` at the schema level on every entity -- whitelist approach, not blacklist.
- Add a global middleware that rejects all non-GET requests to the REST API path with 405 Method Not Allowed, as a defense-in-depth layer.
- Verify the generated OpenAPI spec does not include POST/PUT/DELETE operations. Add a CI check that parses `openapi.json` and fails if write operations are present.
- For the PeeringDB compat layer (hand-written), only register `GET` handlers. Use `mux.HandleFunc("GET /api/net", ...)` with method-specific routing.

**Detection:** 500 errors on POST/PUT/DELETE requests. SQLite "readonly database" errors in logs. Data divergence between primary and replicas after unauthorized writes.

**Phase impact:** entrest configuration and handler mounting. Must be verified during code generation, not discovered in production.

**Confidence:** HIGH -- based on the LiteFS single-writer architecture and entrest's default CRUD generation behavior.

---

### Pitfall 9: otelhttp Transport Wrapping Interferes with Custom Retry Delay and Rate Limiter Timing

**What goes wrong:** The PeeringDB client uses `golang.org/x/time/rate.Limiter` for rate limiting and `time.After()` for retry backoff delays. When `otelhttp.NewTransport` wraps the `http.Client.Transport`, it creates spans that include the full round-trip time. But the rate limiter's `Wait()` call happens outside the transport layer (in `doWithRetry()`), and the retry delay also happens outside the transport. If someone tries to instrument at the wrong layer -- e.g., moving rate limiting or retry logic into a custom `http.RoundTripper` to "centralize" instrumentation -- the rate limiter and retry timing break because `RoundTripper.RoundTrip()` must not have side effects beyond the single request (per `http.RoundTripper` contract).

**Why it happens:** There's a temptation to move all HTTP-related concerns (rate limiting, retry, tracing) into the transport layer for "clean" instrumentation. But `http.RoundTripper` has a strict contract: it should not modify the request, should not follow redirects, and should not implement retry logic. Rate limiting and retry belong at a higher abstraction level.

**Consequences:** If retry logic is moved into the transport: violation of `http.RoundTripper` contract, subtle bugs with request body re-reads (bodies are consumed on first read), broken context cancellation semantics. If rate limiting is moved into the transport: `Wait()` blocks inside `RoundTrip()`, which can cause unexpected timeouts and breaks the assumption that `RoundTrip()` duration equals network time.

**Prevention:**
- Keep the existing architecture: `doWithRetry()` owns rate limiting and retry logic. `otelhttp.NewTransport` wraps only the base `http.DefaultTransport` (or whatever transport is configured).
- Layer the spans correctly:
  1. Application layer: `FetchAll()` span (per object type)
  2. Client layer: `doWithRetry()` span (per page/request, includes retry attempts)
  3. Transport layer: `otelhttp.NewTransport` span (per HTTP round-trip)
- The transport span will automatically be a child of the `doWithRetry()` span because the context flows through `http.NewRequestWithContext()`.
- Do NOT create a custom `http.RoundTripper` that wraps retry/rate-limiting around `otelhttp.NewTransport`.

**Detection:** `RoundTrip()` durations that include rate-limiter wait time (should only be network time). Request bodies failing to read on retry. Context timeouts firing inside the transport layer.

**Phase impact:** OTel HTTP client tracing task. The instrumentation approach must respect the existing client architecture.

**Confidence:** HIGH -- based on Go `http.RoundTripper` documentation and the existing `doWithRetry()` implementation in `internal/peeringdb/client.go`.

---

## Minor Pitfalls

### Pitfall 10: entrest Field Filtering Exposes Internal Ent Fields

**What goes wrong:** entrest auto-generates endpoints for all schema fields unless explicitly skipped. Internal fields like `status` (used for soft-delete filtering), edge FK fields like `org_id` (which are ent implementation details), and computed fields stored for PeeringDB compatibility may all appear in the REST API without curation. Some of these fields have different semantics in the REST API than they do in the database (e.g., `status` is always "ok" in this mirror because deleted records are filtered during sync).

**Prevention:**
- Review every field in every schema for REST API appropriateness.
- Use `entrest.WithReadOnly(true)` on fields that should be visible but not writable (defense-in-depth alongside schema-level read-only).
- Use `entrest.WithSkip(true)` on fields that should not appear in the REST API at all.
- Document which fields are exposed in the OpenAPI spec and verify against expectations.

**Phase impact:** entrest schema annotation task. Requires per-field review of all 13 schemas.

**Confidence:** MEDIUM -- entrest's default behavior is to expose all fields; curation is opt-out.

---

### Pitfall 11: PeeringDB `depth` Parameter Changes Response Shape Fundamentally

**What goes wrong:** PeeringDB's `depth` parameter transforms the response structure. At `depth=0`, related objects are referenced by ID. At `depth=1`, sets (one-to-many relationships) are expanded as arrays of full objects in `_set` fields. At `depth=2`, those nested objects also expand their relationships. The compat layer must implement this shape-shifting behavior, which means the JSON serialization is not a simple "marshal ent struct" -- it requires conditional field inclusion and recursive edge loading.

**Prevention:**
- Start with `depth=0` only for the initial compat layer implementation. This is the most common usage and requires only flat field serialization.
- Add `depth=1` support incrementally. Map PeeringDB's `_set` field convention to ent's `WithEdges()` eager loading.
- Do NOT implement `depth > 1` unless demand is demonstrated. Deep nesting is expensive and rarely used.
- Each depth level requires its own serialization path -- do not try to make one generic recursive serializer.

**Phase impact:** PeeringDB compat layer implementation. Depth support can be phased: `depth=0` first, `depth=1` later.

**Confidence:** HIGH -- based on PeeringDB API documentation about depth parameter behavior.

---

### Pitfall 12: Mounting Two REST API Surfaces on the Same ServeMux Requires Careful Path Routing

**What goes wrong:** The existing `http.ServeMux` already handles `POST /sync`, `GET /health`, and the GraphQL endpoint. Adding both entrest handlers (at `/api/v1/...` or similar) and PeeringDB compat handlers (at `/api/net`, `/api/fac`, etc.) creates path routing complexity. The PeeringDB paths (`/api/net`, `/api/fac`) overlap with potential entrest paths. Additionally, readiness middleware must apply to API endpoints but not health/sync endpoints.

**Prevention:**
- Use path prefixes to separate concerns:
  - `/api/net`, `/api/fac`, `/api/ix`, etc. for PeeringDB compat (13 object types)
  - `/rest/v1/...` or `/openapi/...` for entrest-generated endpoints (distinct prefix, not `/api/`)
  - `/graphql` for GraphQL (already exists)
  - `/health`, `/sync` for operational endpoints (already exist)
- Consider upgrading to chi if path grouping and middleware composition become unwieldy with stdlib `ServeMux`. The existing CLAUDE.md STACK notes chi as the fallback when stdlib proves insufficient.
- Apply readiness middleware to API groups but not operational endpoints.
- Wrap all API handlers with `otelhttp.NewHandler()` for incoming request tracing.

**Phase impact:** HTTP handler mounting task. Path structure must be decided before implementing either REST surface.

**Confidence:** MEDIUM -- stdlib `ServeMux` can handle this with Go 1.22+ method routing, but middleware composition may push toward chi.

---

### Pitfall 13: Sync Metrics Must Record Inside the Transaction, Not Just at Completion

**What goes wrong:** The natural place to record sync metrics is after `tx.Commit()` -- record total objects synced, duration, etc. But this misses per-type granularity (how many organizations vs. networks were synced) and in-progress visibility (no metrics until the full sync completes, which can take minutes). If the sync fails partway through, the per-type metrics for completed steps are lost.

**Prevention:**
- Record per-type metrics inside the sync step loop, not just at the end:
  - After each `step.fn()` completes, record `peeringdb_plus.sync.objects.total` with attribute `type={step.name}` and value `count`.
  - Record `peeringdb_plus.sync.step.duration` per step.
- Record the overall sync result (success/failure) as a separate metric after the transaction commits or rolls back.
- Use attributes (not separate metric names) to distinguish object types: `sync.objects{type="org"}`, not `sync.org.objects`.
- The existing `objectCounts` map in `Sync()` already captures per-type counts -- just add `Record()` calls after each step.

**Phase impact:** Sync metrics expansion task. Requires modifying the sync loop, not just adding metrics at the end.

**Confidence:** HIGH -- based on inspection of the existing `Sync()` method structure in `internal/sync/worker.go`.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| OTel HTTP client tracing | Span hierarchy wrong with retries (#1), transport layer violations (#9) | Design span tree before instrumenting; keep retry logic outside RoundTripper |
| MeterProvider setup | No-op meter silently drops all metrics (#4) | Initialize MeterProvider alongside TracerProvider before any metric code |
| Sync metrics expansion | Duplicate instrument registration (#7), metrics only at completion (#13) | Create instruments once at Worker construction; record per-step |
| entrest integration | Extension conflicts with entgql (#5), write handlers on read-only DB (#8) | Test codegen with both extensions; whitelist read-only operations |
| entrest field exposure | Internal fields leaked to API (#10) | Per-field review and annotation of all 13 schemas |
| PeeringDB compat: response format | Response envelope mismatch (#2), field name gaps (#3) | Separate API surfaces; reuse existing PeeringDB types for serialization |
| PeeringDB compat: query params | Django-style filter syntax (#6) | Build query parameter parser; prioritize common filters |
| PeeringDB compat: depth | Response shape changes per depth (#11) | Start with depth=0; phase in depth=1 incrementally |
| HTTP routing | Path conflicts between REST surfaces (#12) | Distinct path prefixes; consider chi for middleware grouping |

## Sources

### OpenTelemetry
- [otelhttp package](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) -- Transport wrapping, client instrumentation
- [OTel HTTP semantic conventions](https://opentelemetry.io/docs/specs/semconv/http/http-spans/) -- Retry span conventions, `http.request.resend_count`
- [OTel Go metrics instrumentation](https://opentelemetry.io/docs/languages/go/instrumentation/) -- Meter, instrument creation, MeterProvider setup
- [Duplicate instrument registration (GitHub #3229)](https://github.com/open-telemetry/opentelemetry-go/issues/3229) -- Behavior on duplicate registration
- [OTel Go 2025 goals](https://opentelemetry.io/blog/2025/go-goals/) -- Semantic convention updates
- [otelhttp transport source](https://github.com/open-telemetry/opentelemetry-go-contrib/blob/main/instrumentation/net/http/otelhttp/transport.go) -- Transport implementation details
- [OTel context propagation](https://opentelemetry.io/docs/concepts/context-propagation/) -- How context flows through HTTP clients
- [otelhttp deprecations (GitHub releases)](https://github.com/open-telemetry/opentelemetry-go-contrib/releases) -- DefaultClient deprecated, use NewTransport

### entrest
- [entrest annotation reference](https://lrstanley.github.io/entrest/openapi-specs/annotation-reference/) -- WithIncludeOperations, WithSkip, WithReadOnly, WithHandler
- [entrest getting started](https://lrstanley.github.io/entrest/guides/getting-started/) -- Extension setup, handler mounting, chi integration
- [entrest GitHub issues](https://github.com/lrstanley/entrest/issues) -- Open issues including #106 (operations disable edges), #127 (field.Strings bug)
- [entrest repository](https://github.com/lrstanley/entrest) -- WIP status warning, v1.0.2

### PeeringDB
- [PeeringDB API Specs](https://docs.peeringdb.com/api_specs/) -- Response envelope, query parameters, depth behavior
- [PeeringDB API Documentation](https://www.peeringdb.com/apidocs/) -- Endpoint listing, object types
- [PeeringDB query limits](https://docs.peeringdb.com/howto/work_within_peeringdbs_query_limits/) -- Rate limiting, pagination

### Ent
- [Ent extensions](https://entgo.io/docs/extensions/) -- Multiple extension configuration
- [entgql GraphQL integration](https://entgo.io/docs/graphql/) -- Existing annotation patterns

### Codebase (verified against source)
- `internal/otel/provider.go` -- Only TracerProvider initialized, no MeterProvider
- `internal/peeringdb/client.go` -- `doWithRetry()` retry and rate limiting architecture
- `internal/sync/worker.go` -- Sync loop structure, per-type step pattern
- `ent/entc.go` -- Current extension configuration (entgql only)
- `ent/schema/organization.go`, `ent/schema/network.go` -- Existing schema annotations
- `internal/peeringdb/types.go` -- PeeringDB response types with JSON field names
