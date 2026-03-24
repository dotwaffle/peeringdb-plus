# Project Retrospective

*A living document updated after each milestone. Lessons feed forward into future planning.*

## Milestone: v1.3 — PeeringDB API Key Support

**Shipped:** 2026-03-24
**Phases:** 2 | **Plans:** 3 | **Tasks:** 4

### What Was Built
- WithAPIKey functional option on NewClient with Authorization header injection and 60 req/min authenticated rate limit
- 401/403 auth error handling with WARN logging, SEC-2 compliant startup logging
- pdbcompat-check CLI --api-key flag with env var fallback for authenticated conformance testing
- Live conformance test with conditional 1s/3s sleep based on authentication status

### What Worked
- Functional options pattern (ClientOption) made API key addition backward-compatible with zero caller changes
- TDD caught t.Setenv/t.Parallel conflict early — resolved by extracting resolveAPIKey helper
- Small milestone scope (2 phases, 3 plans) made execution fast and focused
- Reusing existing patterns (SEC-2 logging, header injection) from earlier milestones kept implementation clean

### What Was Inefficient
- Nothing significant — this was a well-scoped, surgical milestone

### Patterns Established
- ClientOption functional options for NewClient (variadic, backward-compatible)
- CLI flag with env var fallback: flag.StringVar then os.Getenv after flag.Parse()
- Auth error early-exit between body-discard and isRetryable check in retry loop

### Key Lessons
1. Small milestones with clear scope execute fastest — 2 phases completed in under 30 minutes
2. Functional options pattern is the right choice for optional configuration on existing constructors
3. SEC-2 compliance (never log secrets) should be a pattern, not an afterthought

### Cost Observations
- Model mix: ~90% opus, ~10% sonnet (subagents)
- Sessions: 1 session
- Notable: Entire milestone completed in ~30 minutes wall time

---

## Milestone: v1.1 — REST API & Observability

**Shipped:** 2026-03-23
**Phases:** 3 | **Plans:** 8 | **Tasks:** 16

### What Was Built
- OpenTelemetry HTTP client tracing with span hierarchy for PeeringDB sync calls
- Per-type sync metrics (duration, object count, delete count) with freshness observable gauge
- Auto-generated REST API at /rest/v1/ via entrest with OpenAPI spec, filtering, sorting, pagination, eager-loading
- PeeringDB-compatible drop-in API at /api/ with Django-style filters, depth expansion, search, and field projection

### What Worked
- entrest integration was straightforward — dual codegen with entgql worked first try
- TDD approach (failing tests first) caught integration issues early, especially REST mount overwrite by Phase 6
- Milestone audit caught the REST handler mount regression before shipping
- Phase 6 compat layer design decision (querying ent directly, not wrapping entrest) avoided complex adapter layer
- Generic Django-style filter parser handled all 13 types without per-type switch statements

### What Was Inefficient
- Phase branch merges created merge conflicts in STATE.md and REQUIREMENTS.md that needed manual resolution
- ROADMAP.md plan checkboxes weren't updated during execution, creating stale state
- REST handler mount was overwritten by Phase 6 commit — caught only during audit, should have been caught by integration tests
- Phase 4/5 velocity metrics not tracked in STATE.md (only phases 1-3 and 6 recorded)

### Patterns Established
- `func(*sql.Selector)` as universal predicate type for cross-entity filtering
- Type registry pattern for PeeringDB entity metadata (fields, edges, serializers)
- JSON round-trip for dynamic field injection (depth expansion _set fields)
- Wildcard route pattern `GET /api/{rest...}` for unified PeeringDB path handling

### Key Lessons
1. Cross-phase integration tests are essential — Phase 6 silently broke Phase 5's REST mount
2. Milestone audit is worth the cost — it caught a real regression
3. Phase branches with merge-back create friction — consider milestone branches or trunk-based development
4. Code-generated APIs (entrest, entgql) pay for themselves in consistency across 13 types

### Cost Observations
- Model mix: ~90% opus, ~10% sonnet (subagents)
- Sessions: ~4 sessions across v1.1
- Notable: Entire milestone completed in a single day (12 hours wall time)

---

## Milestone: v1.0 — PeeringDB Plus

**Shipped:** 2026-03-22
**Phases:** 3 | **Plans:** 14 | **Tasks:** 27

### What Was Built
- Full PeeringDB sync pipeline: HTTP client with rate limiting, pagination, retry for all 13 types
- entgo ORM with all 13 PeeringDB schemas, FK edges, mutation hooks
- Bulk upsert in FK dependency order with hard delete and sync status tracking
- Relay-compliant GraphQL API with playground, pagination, filtering, custom resolvers
- OpenTelemetry observability (traces, metrics, logs) with autoexport
- Health/readiness endpoints with LiteFS primary detection
- Fly.io deployment artifacts (Dockerfile.prod, litefs.yml, fly.toml)

### What Worked
- Schema extraction pipeline (Python Django source -> JSON -> entgo schemas) avoided manual schema transcription errors
- entgo code generation eliminated boilerplate across 13 types
- Fixture-based integration tests caught real data handling issues
- OTel autoexport made observability configuration-free

### What Was Inefficient
- Initial OTel setup registered metrics but didn't wire recording (caught in v1.1 planning)
- DataLoader middleware was wired but unused (entgql handles N+1 natively)
- globalid.go exports were created but ent Noder handles ID resolution

### Patterns Established
- FK dependency order for bulk operations (upsert, delete)
- Fixture files for integration testing PeeringDB data
- entgo annotation-driven API generation (entgql annotations -> GraphQL schema)

### Key Lessons
1. Read the actual API responses, not the documentation — PeeringDB's spec diverges from reality
2. Code generation from a single source of truth (ent schemas) prevents drift across API surfaces
3. Start with observability infrastructure early — retrofitting is harder (v1.1 proved this)

### Cost Observations
- Model mix: ~85% opus, ~15% sonnet
- Sessions: ~3 sessions across v1.0
- Notable: Both milestones completed in same day — total project time ~1 day

---

## Cross-Milestone Trends

### Process Evolution

| Milestone | Sessions | Phases | Key Change |
|-----------|----------|--------|------------|
| v1.0 | ~3 | 3 | Initial patterns: TDD, fixture tests, code generation |
| v1.1 | ~4 | 3 | Milestone audit added, caught integration regression |
| v1.2 | ~3 | 4 | golangci-lint enforcement, golden file tests, CI pipeline |
| v1.3 | 1 | 2 | Smallest milestone — focused scope, fastest execution |

### Cumulative Quality

| Milestone | Plans | Tasks | Integration Tests |
|-----------|-------|-------|-------------------|
| v1.0 | 14 | 27 | 16 GraphQL + 4 sync |
| v1.1 | 8 | 16 | 15 REST + 25 filter + 7 compat |
| v1.2 | 9 | 18 | 39 golden file + conformance CLI |
| v1.3 | 3 | 4 | 7 client auth + 4 CLI auth |

### Top Lessons (Verified Across Milestones)

1. Code generation from entgo schemas is the right bet — scales to 13 types without per-type maintenance
2. Integration tests catch real issues that unit tests miss — all milestones benefited
3. Small, focused milestones (v1.3) execute fastest and with fewest issues
4. Functional options pattern enables backward-compatible extension of constructors
5. SEC-2 compliance as a pattern (never log secrets) prevents security issues at every integration point
