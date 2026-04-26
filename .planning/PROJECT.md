# PeeringDB Plus

## What This Is

A high-performance, globally distributed, read-only mirror of PeeringDB data with a modern web interface. Syncs all 13 PeeringDB object types via full or incremental re-fetch (hourly or on-demand), stores them in SQLite on LiteFS for edge-local reads on Fly.io, and exposes the data through five surfaces: a web UI (templ + htmx + Tailwind CSS) with live search, detail pages, and ASN comparison; GraphQL (with playground); OpenAPI REST (with auto-generated spec); a PeeringDB-compatible drop-in replacement API; and ConnectRPC/gRPC with all 13 types queryable via Get/List RPCs with typed filtering, reflection, and health checking. Supports optional PeeringDB API key authentication for higher sync rate limits. Built in Go using entgo as the ORM, with full OpenTelemetry observability.

## Core Value

Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.

## Requirements

### Validated

- [x] Sync all PeeringDB objects via full re-fetch (hourly or on-demand) — v1.0
- [x] Store data in SQLite using entgo ORM — v1.0
- [x] Handle PeeringDB API response format discrepancies — v1.0
- [x] Expose data via GraphQL (entgql) with filtering, pagination, relationship traversal — v1.0
- [x] Interactive GraphQL playground with example queries — v1.0
- [x] CORS headers for browser integrations — v1.0
- [x] Lookup by ASN and ID — v1.0
- [x] Deploy on Fly.io with LiteFS for global edge distribution — v1.0
- [x] OpenTelemetry tracing, metrics, and logs throughout — v1.0
- [x] Health/readiness endpoints with sync age check — v1.0
- [x] OTel trace spans on PeeringDB HTTP client — v1.1
- [x] Sync metrics reviewed, expanded, and wired to record — v1.1
- [x] Expose data via OpenAPI REST (entrest) — v1.1
- [x] Full PeeringDB-compatible REST layer (paths, response envelope, query params, field names) — v1.1
- [x] Fully public — verify no auth barriers, document public access model — v1.2
- [x] Golden file tests for PeeringDB compatibility layer — v1.2
- [x] CI pipeline (GitHub Actions) enforcing tests, linting, and vetting — v1.2
- [x] All tests pass with -race, all linters pass clean — v1.2
- [x] Optional PeeringDB API key for authenticated sync with higher rate limits — v1.3
- [x] Conformance tooling uses API key for authenticated PeeringDB access — v1.3

- [x] Live search across all PeeringDB types with instant results — v1.4
- [x] Record detail views for all 6 types with collapsible lazy-loaded sections — v1.4
- [x] ASN comparison tool showing shared IXPs, facilities, and campuses — v1.4
- [x] Linkable/shareable URLs for every page — URL is the state — v1.4
- [x] Polished design with dark mode, transitions, keyboard navigation, error pages — v1.4
- [x] Verify meta.generated field behavior for depth=0 paginated PeeringDB responses; graceful fallback if missing — v1.5
- [x] Remove unused DataLoader middleware and convert WorkerConfig.IsPrimary to dynamic LiteFS detection — v1.5
- [x] Verify all 26 deferred human verification items against live Fly.io deployment — v1.5
- [x] Grafana dashboard (JSON provisioning) covering sync health, API traffic, infrastructure, and business metrics — v1.5
- [x] App serves traffic directly without LiteFS HTTP proxy, h2c for gRPC wire protocol — v1.6
- [x] Proto definitions for all 13 PeeringDB types via entproto + buf + ConnectRPC codegen — v1.6
- [x] Get + List RPCs for all 13 types via ConnectRPC with typed filtering and pagination — v1.6
- [x] gRPC server reflection (v1 + v1alpha) for grpcurl/grpcui discovery — v1.6
- [x] gRPC health checking with sync-readiness-driven status — v1.6
- [x] otelconnect observability interceptor on all ConnectRPC handlers — v1.6
- [x] CORS updated for Connect, gRPC, and gRPC-Web content types — v1.6

- [x] Server-streaming RPCs for bulk data export (stream rows from DB, no full buffering) — v1.7
- [x] since_id stream resume and updated_since incremental filter on streaming RPCs — v1.7
- [x] IX presence UI improvements (field labels, RS badge, port speed colors, copyable text) — v1.7

- [x] Terminal client detection (User-Agent sniffing for curl, wget, HTTPie, etc.) — v1.8
- [x] Content negotiation under existing /ui/ URLs — browsers unchanged, terminals get text — v1.8
- [x] CLI help text at /ui/ for terminal clients listing available endpoints — v1.8
- [x] Text-formatted error responses (404, 500) for terminal clients — v1.8
- [x] Rich 256-color ANSI output with Unicode box drawing for all 6 entity types — v1.8
- [x] Plain text mode (?T) and JSON mode (?format=json) as alternative output formats — v1.8
- [x] WHOIS/RPSL format output (?format=whois) for all entity types — v1.8
- [x] Short format one-liner mode (?format=short) for scripting — v1.8
- [x] Data freshness timestamp footer on all terminal responses — v1.8
- [x] Section filtering (?section=ix,fac) for detail views — v1.8
- [x] Width adaptation (?w=N) with progressive column dropping — v1.8
- [x] Bash and zsh shell completion scripts — v1.8
- [x] Terminal search results and ASN comparison — v1.8

- [x] HTTP server timeouts (ReadHeaderTimeout, IdleTimeout) and SQLite connection pool configuration — v1.12
- [x] Config startup validation for ListenAddr, PeeringDBBaseURL, DrainTimeout — v1.12
- [x] POST body size limits (1MB) on /graphql and /sync endpoints — v1.12
- [x] ASN input validation (32-bit range, 400 Bad Request) and width parameter capping (500 max) — v1.12
- [x] Content-Security-Policy-Report-Only headers with per-route policies — v1.12
- [x] Gzip compression middleware excluding gRPC content types — v1.12
- [x] Cached metrics object count gauge eliminating per-scrape COUNT queries — v1.12
- [x] GraphQL error classification via ent sentinel errors (GO-ERR-2 fix) — v1.12
- [x] GraphQL handler and database.Open() test coverage — v1.12
- [x] exhaustive, contextcheck, gosec linters enabled with clean pass — v1.12
- [x] Docker build validation (both Dockerfiles) in CI pipeline — v1.12
- [x] detail.go split into 6 per-entity query files — v1.12
- [x] Generic upsertBatch replacing 13 copy-pasted upsert functions — v1.12
- [x] /ui/about terminal rendering with rich output — v1.12
- [x] seed.Minimal/Networks unexported (seed.Full is sole public API) — v1.12
- [x] Default list ordering flipped to (-updated, -created) matching upstream — v1.16
- [x] Status × Since matrix for pdbcompat (list/detail status visibility) — v1.16
- [x] limit=0 unlimited semantics (not count-only) — v1.16
- [x] __in robustness against SQLite variable limits via json_each — v1.16
- [x] Unicode folding (unidecode-equivalent) for filter values — v1.16
- [x] Cross-entity __ traversal (Path A allowlists + Path B auto-traversal) — v1.16
- [x] Operator coercion (__contains -> __icontains etc.) — v1.16
- [x] Memory-safe response paths for large results on 256MB replicas — v1.16

## Current Milestone: none

**Status:** Ready for /gsd-new-milestone to start v1.18.0. (v1.17.0 was burned by quick task 260426-pms — SEED-001 incremental-sync default flip — shipped as a release tag, not a milestone.)
**Theme:** n/a

### Recently Shipped: v1.15 Infrastructure Polish & Schema Hygiene

**Shipped:** 2026-04-18 (4 phases 63-66, 11 requirements)
**Archive:** [`.planning/milestones/v1.15-ROADMAP.md`](./milestones/v1.15-ROADMAP.md)

### Previously Shipped: v1.14 Authenticated Sync & Visibility Layer

**Shipped:** 2026-04-17 (6 phases, 21 plans, 17/17 requirements, audit PASSED)
**Archive:** [`.planning/milestones/v1.14-ROADMAP.md`](./milestones/v1.14-ROADMAP.md)

**What shipped:**
- Empirical visibility baseline committed (Phase 57): all 13 types × 2 auth modes against beta, confirmation against prod for poc/org/net. Structural diff identified only two auth-gated surfaces (poc row-level + `ixlan.ixf_ixp_member_list_url`).
- Schema alignment (Phase 58): no new ent fields needed; regression test locks empirical assumption against diff.json drift.
- Read-path privacy enforcement (Phase 59): ent Privacy policy filters `visible="Users"` rows from anonymous responses across all 5 surfaces; sync-write bypass via single-call-site `privacy.DecisionContext(ctx, privacy.Allow)`; `PDBPLUS_PUBLIC_TIER` private-instance override with fail-fast validator.
- Surface integration (Phase 60): per-surface anonymous-leak tests across 5 surfaces; pdbcompat anon parity via 13-type fixture replay; no-key sync verified.
- Operator observability (Phase 61): startup sync-mode classification log + WARN on override; `/about` HTML + terminal render Privacy & Sync section; OTel attribute `pdbplus.privacy.tier` on read spans.
- Production rollout (Phase 62): Fly.io `peeringdb-plus` now runs authenticated sync; 4 docs updated (CONFIGURATION, DEPLOYMENT, ARCHITECTURE with Mermaid, CLAUDE).

**Deferred to v1.15 (or later):**
- OAuth identity integration — PeeringDB OAuth (`auth.peeringdb.com`) with `profile`+`networks` scopes; once wired, an OAuth-identified caller's context carries `tier=Users` and the existing privacy policy admits Users-visibility rows for that caller
- Domain extensions (BGP, IRR/AS-SET, IP prefix lookup) — carried from v1.13 deferred list
- Operational verification items (OPVR-01..04) carried forward
- `ixpfx.notes` pdbcompat divergence: operator sign-off on drop-from-projection vs accept-as-documented-extension

### Deferred

- [ ] SyncStatus custom RPC — deferred, available via existing REST/GraphQL
- [ ] Per-ASN BGP summary from bgp.tools (prefix counts, RPKI coverage) — needs design
- [ ] IRR/AS-SET membership from WHOIS source — needs design
- [ ] IP prefix lookup with origin ASN, RPKI status — needs design

### Out of Scope

- Write-path / data modification — this is a read-only mirror
- User accounts or end-user authentication — fully public read access
- Per-user API key management or rotation — server-side config, restart to change
- Mobile app — web-first
- Real-time streaming of changes — periodic sync is sufficient

## Context

- PeeringDB is the authoritative database for network interconnection data (organizations, networks, IXPs, facilities, etc.)
- PeeringDB suffers from poor performance, single-region hosting, and an API spec that doesn't match actual API responses
- LiteFS on Fly.io enables SQLite replication to edge nodes worldwide, giving every region local read latency
- entgo provides code generation for the ORM layer, with ecosystem packages for GraphQL (entgql), gRPC (entproto), and REST (entrest)

## Constraints

- **Language**: Go 1.26
- **ORM**: entgo (non-negotiable — ecosystem drives GraphQL/gRPC/REST generation)
- **Storage**: SQLite + LiteFS (enables edge distribution without a central database)
- **Platform**: Fly.io (LiteFS dependency, global edge deployment)
- **Observability**: OpenTelemetry — mandatory for tracing, metrics, and logs
- **Data fidelity**: Must handle PeeringDB's actual API responses, not their documented spec

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Full re-fetch sync (not incremental) | Simpler implementation, guarantees data consistency | ✓ Validated Phase 1 |
| SQLite + LiteFS over PostgreSQL | Enables edge-local reads on Fly.io without central DB latency | ✓ Validated Phase 1 |
| entgo as ORM | Ecosystem packages (entgql, entproto, entrest) generate all API surfaces from schema | ✓ Validated Phase 1 |
| GraphQL as first API surface for v1 | Flexible querying, entgql is mature, good fit for network data exploration | ✓ Validated Phase 2 |
| rs/cors for CORS middleware | Well-maintained, stdlib-compatible, simple config | ✓ Validated Phase 2 |
| Autoexport for OTel exporters | Environment-driven exporter selection, no hardcoded endpoints | ✓ Validated Phase 3 |
| Dual slog handler (stdout + OTel) | Structured logs to both console and OTel pipeline simultaneously | ✓ Validated Phase 3 |
| LiteFS .primary file for leader detection | Inverted semantics (.primary exists on replicas), fallback to env var | ✓ Validated Phase 3 |
| otelhttp.NewTransport + manual parent spans | Both automatic HTTP semantics AND business-level span hierarchy for PeeringDB calls | ✓ Validated Phase 4 |
| Flat metric naming with type attribute | pdbplus.sync.type.* with type=net|ix|fac — fewer instruments, filter by type | ✓ Validated Phase 4 |
| entrest for REST API generation | Code-generated REST alongside entgql from same schemas, read-only config | ✓ Validated Phase 5 |
| PeeringDB compat layer queries ent directly | NOT wrapping entrest — different response envelopes, query parameters, and serialization requirements | ✓ Validated Phase 6 |
| Generic Django-style filter parser | One parser handles all 13 types via shared func(*sql.Selector) predicate type | ✓ Validated Phase 6 |
| Golden file tests with go-cmp for compat layer | 39 golden files (13 types x 3 scenarios) with -update flag for regeneration | ✓ Validated Phase 9 |
| Structure-only conformance comparison | CompareStructure checks field names/types/nesting, not values — handles live data changes | ✓ Validated Phase 9 |
| GitHub Actions CI with 4 parallel jobs | lint + go generate drift, test -race, build, govulncheck — coverage PR comments via gh api | ✓ Validated Phase 10 |
| Public access by design | All read endpoints unauthenticated; only POST /sync gated; root endpoint self-documents | ✓ Validated Phase 10 |
| ClientOption functional options for NewClient | Backward-compatible variadic opts; WithAPIKey injects auth header without breaking callers | ✓ Validated Phase 11 |
| 401/403 auth errors never retried | Placed between body-discard and isRetryable check; WARN log with SEC-2 compliance | ✓ Validated Phase 11 |
| CLI flag with env var fallback for API key | --api-key flag takes precedence over PDBPLUS_PEERINGDB_API_KEY env var | ✓ Validated Phase 12 |
| templ + htmx + Tailwind CDN for web UI | Type-safe server-rendered HTML, no JS build toolchain, no SPA complexity | ✓ Validated Phase 13 |
| Tailwind via CDN (no build step) | Eliminates Node.js dependency; trade-off: ~300KB download, no tree-shaking | ✓ Validated Phase 13 |
| Dual render mode (full page vs htmx fragment) | Single renderPage helper checks HX-Request, sets Vary header | ✓ Validated Phase 13 |
| errgroup fan-out for search across 6 types | Parallel LIKE queries, 10 results + count per type | ✓ Validated Phase 14 |
| Networks by ASN in URLs (/ui/asn/{asn}) | Users think in ASNs, not internal IDs | ✓ Validated Phase 15 |
| Pre-computed count fields for summary stats | ix_count, fac_count etc. from PeeringDB sync, avoid extra count queries | ✓ Validated Phase 15 |
| Map-based set intersection for ASN comparison | Load presences for both networks, compute shared IXPs/facilities/campuses in Go | ✓ Validated Phase 16 |
| Class-based dark mode with localStorage | @custom-variant dark, system preference detection, manual toggle persists | ✓ Validated Phase 17 |
| IsPrimary as func() bool, not static bool | Dynamic LiteFS primary detection on each sync cycle start | ✓ Validated Phase 18 |
| OTLP autoexport for Prometheus metrics | No /metrics endpoint needed — OTEL_METRICS_EXPORTER=prometheus with autoexport | ✓ Validated Phase 19 |
| Hand-authored Grafana dashboard JSON | Simpler than Grafonnet/Jsonnet for single dashboard; DS_PROMETHEUS template variable for portability | ✓ Validated Phase 19 |
| Single pdbplus.data.type.count gauge with type attribute | One instrument for all 13 PeeringDB types, filter by type label | ✓ Validated Phase 19 |
| ConnectRPC over google.golang.org/grpc | Standard net/http handlers, same mux as REST/GraphQL, supports Connect+gRPC+gRPC-Web on one port | ✓ Validated Phase 23 |
| Remove LiteFS proxy, app-level fly-replay | LiteFS proxy is HTTP/1.1 only, blocks h2c/gRPC; app already handles POST /sync replay | ✓ Validated Phase 21 |
| entproto for .proto generation, skip protoc-gen-entgrpc | entproto generates standard .proto files; entgrpc is hardcoded to google.golang.org/grpc interfaces | ✓ Validated Phase 22 |
| Manual services.proto over entproto service generation | entproto only generates message types, not service definitions; manual services.proto with Get/List RPCs for all 13 types | ✓ Validated Phase 22 |
| Predicate accumulation for List filtering | Nil-check optional proto fields, validate, accumulate ent predicates, apply via entity.And() | ✓ Validated Phase 24 |
| StreamNetworks naming convention (not StreamAllNetworks) | Concise, mirrors ListNetworks pattern | ✓ Validated Phase 25 |
| Hardcoded 500-row batch size for streaming | Simple, sufficient for PeeringDB data volumes (~200K max rows) | ✓ Validated Phase 25 |
| WithoutTraceEvents() globally on otelconnect | Per-message events at 200K rows is telemetry explosion; per-RPC interceptor not feasible | ✓ Validated Phase 25 |
| grpc-total-count response header for streaming | gRPC metadata convention; set before first Send() via stream.ResponseHeader() | ✓ Validated Phase 25 |
| Configurable StreamTimeout via PDBPLUS_STREAM_TIMEOUT | 60s default; prevents indefinite connection hold from slow clients | ✓ Validated Phase 25 |
| google.protobuf.Timestamp for updated_since | Standard protobuf well-known type, nanosecond precision, widely supported | ✓ Validated Phase 26 |
| since_id as both predicate and cursor | IDGT predicate affects count (grpc-total-count reflects remaining), sets initial lastID | ✓ Validated Phase 26 |
| 5-tier port speed color coding | Sub-1G gray, 1G neutral, 10G blue, 100G emerald, 400G+ amber — networking industry intuitive gradient | ✓ Validated Phase 27 |
| CopyableIP with click-to-copy + hover icon | Best discoverability — both click-on-IP and explicit clipboard icon | ✓ Validated Phase 27 |
| lipgloss v2 + colorprofile for terminal rendering | Force ANSI256 over HTTP (not TTY), NoTTY for plain text; vanity domain charm.land/lipgloss/v2 | ✓ Validated Phase 28 |
| Type-switch dispatch in RenderPage | Concrete type assertion over interface polymorphism — simpler, explicit, grep-able | ✓ Validated Phase 29 |
| Eager-load child rows unconditionally | All 6 detail handlers eager-load regardless of render mode — simplifies handler logic | ✓ Validated Phase 30 |
| RPSL aut-num class for WHOIS format | RFC 2622 aut-num for networks; custom ix:/site:/organisation:/campus:/carrier: for other types | ✓ Validated Phase 30 |
| Renderer struct fields for per-request state | Sections/Width as exported fields set before RenderPage — avoids signature explosion (CS-5) | ✓ Validated Phase 31 |
| Server-side completion search returning IDs only | Prevents shell injection from entity names containing special characters | ✓ Validated Phase 31 |
| Phase 58 schema alignment: existing ent fields sufficient | Phase 57 empirical diff surfaced only two auth-gated surfaces — poc row-level visibility (already covered by poc.visible per D-01/D-02) and ixlan.ixf_ixp_member_list_url (already covered by ixlan.ixf_ixp_member_list_url_visible). All other 11 types show zero field-level deltas. No new ent fields required | ✓ Validated Phase 58 |
| <field>_visible naming convention for per-field visibility | Established by ixlan.ixf_ixp_member_list_url_visible (pre-existing) and confirmed in Phase 58 as the canonical pattern for any future auth-gated field additions. field.String (not Enum) per D-02 — avoids entgql/entrest/entproto codegen churn; validation happens in the privacy policy layer | ✓ Validated Phase 58 |
| Privacy policy treats NULL <field>_visible as schema default | D-07: ent auto-migrate adds new *_visible columns with declared defaults, but existing rows synced before the column existed will have NULL until the next sync rewrites them. The Phase 59 privacy policy MUST treat NULL as the column default (`Public` per the upstream-default rule) rather than as `Users`, to prevent a post-upgrade flood of suddenly-hidden rows | ✓ Validated Phase 58 |
| Phase 58 regression test guards the empirical assumption | internal/visbaseline/schema_alignment_test.go asserts diff.json contains only the two known auth-gated surfaces. If a future Phase 57 re-capture surfaces a new auth-gated field, the test fails and forces Phase 58 to be re-opened before Phase 59's privacy policy ships against a stale assumption | ✓ Validated Phase 58 |
| Phase 63 schema hygiene: drop ixpfx.notes + org.{fac,net}_count | Audit-confirmed vestigial after v1.14: ixpfx.notes was always empty from upstream and was carried as a known pdbcompat divergence; org.fac_count/org.net_count were schema-only (never upserted, never serialized). Dropping at the ent layer and wiring migrate.WithDropColumn(true) + migrate.WithDropIndex(true) in cmd/peeringdb-plus/main.go emits ALTER TABLE DROP COLUMN on next primary startup. Also edits schema/peeringdb.json (the JSON is the canonical source used by pdb-schema-generate) so the schema generator is idempotent. Accepted cosmetic wire-compat note: proto/peeringdb/v1/v1.proto is frozen since v1.6 (entproto.SkipGenFile in ent/entc.go), so the IxPrefix.Notes and Organization.{FacCount,NetCount} proto fields remain declared but are no longer populated by the server; wire-encoded as absent. Read-only mirror with no known external proto consumers. | ✓ Validated Phase 63 |
| Phase 65 asymmetric Fly fleet: 1 primary (LHR, shared-cpu-2x/512MB, persistent volume) + 7 ephemeral replicas (shared-cpu-1x/256MB, cold-sync from primary) | Observed replica RSS 58-59 MB; 256 MB gives ~4× headroom. Splits VM sizing and mount policy via Fly process groups. `litefs.yml` region-gated candidacy unchanged — process groups reinforce the LHR-only primary invariant. Cost: $57.20/mo → $20.75/mo (~63% saving; real win is operational simplicity — no replica-volume orphans, destroy-and-recreate recovery). Big-bang rollout (CONTEXT D-01); rollback = revert fly.toml + redeploy. SEED-003 captures future primary-HA work. | ✓ Validated Phase 65 |
| Phase 66 observability: OTel span attrs + slog.Warn hybrid for peak heap / RSS at end of sync cycle; defaults `PDBPLUS_HEAP_WARN_MIB=400` and `PDBPLUS_RSS_WARN_MIB=384` | SEED-001 trigger observability without actioning it. Dual surface (span attr + slog.Warn, plus Prometheus ObservableGauge export) so dashboards keep continuous timeseries (`pdbplus.sync.peak_heap_mib`, `pdbplus.sync.peak_rss_mib`) while log pipelines see discrete alerts (`heap threshold crossed`). Defaults chosen vs Fly 512 MB VM cap: 400 MiB heap leaves 112 MB headroom for Go runtime + stack + binary before OOM-kill; 384 MiB RSS catches VmHWM spikes earlier since VmHWM is strict-monotonic over the process lifetime. RSS read via `/proc/self/status` VmHWM on Linux, cleanly skipped on non-Linux (attr omitted, not zero-valued). Implementation: `internal/sync/worker.go` `emitMemoryTelemetry` called from recordSuccess / rollbackAndRecord / recordFailure. Sampling granularity = sync cycle frequency (1h default); no periodic background sampler added. Separate dashboard row `Sync Memory (SEED-001 watch)` in `deploy/grafana/dashboards/pdbplus-overview.json` with three panels (Peak Heap, Peak RSS, Peak Heap by Process Group). Does NOT flip SEED-001 — escalation path documented in `docs/DEPLOYMENT.md` and SEED-001 remains dormant until the trigger actually fires. | ✓ Validated Phase 66 |
| Django-compat Unicode folding via `internal/unifold` | Matches upstream `unidecode` semantics for filter values (NFKD + manual fold map) without external CGO dependencies; 16 shadow columns (`*_fold`) used for indexed lookups | ✓ Validated Phase 69 |
| Memory-safe `StreamListResponse` token writer | Hand-rolled JSON streamer avoids full-slice materialization for large responses (limit=0/depth=2); pre-flight count gate returns 413 Request Entity Too Large before OOM | ✓ Validated Phase 71 |
| 2-hop cross-entity traversal ceiling | Hard cap on filter depth (`fk__fk__field`) to prevent accidental unbounded JOIN complexity; mirrors upstream `serializers.py` allowlists | ✓ Validated Phase 70 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

## Current State

Shipped v1.16 Django-compat Correctness with 72 phases across 16 milestones (v1.0-v1.16). `pdbcompat` surface now has full behavioural parity with upstream PeeringDB Django semantics (ordering, status-matrix, unicode folding, traversal). Memory-safe streaming response paths active for unlimited queries on 256MB replicas. OTel trace bloat resolved (PERF-08) and batching configured.

**Known tech debt:**
- fly_region Grafana template variable needs verification after multi-region deployment
- Go runtime metric names need verification against live Grafana Cloud
- internal/otel at 87.4% vs 90% target (unreachable OTel API branches)
- CI coverage pipeline needs human verification on actual GitHub Actions run
- CSP deployed as Report-Only — needs enforcement after violation monitoring (v1.13 follow-up)
- detail.go still 775 lines (handlers + shared helpers remain bundled)
- 5 pre-existing golangci-lint issues in `internal/visbaseline/{reportcli,redactcli}.go` — scope-boundaried out of v1.14
- `ixpfx.notes` pdbcompat divergence — allow-listed in anon_parity_test.go; operator sign-off on drop-vs-accept deferred to a follow-up compat-layer plan

---
*Last updated: 2026-04-22 — v1.16 Django-compat Correctness milestone COMPLETED (v1.16 shipped 2026-04-19, 72 phases across v1.0-v1.16)*
