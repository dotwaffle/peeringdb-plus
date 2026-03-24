# Phase 1: Data Foundation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-22
**Phase:** 01-data-foundation
**Areas discussed:** API parsing strategy, Sync behavior, Schema fidelity

---

## API Parsing Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Both sources | Analyze Python Django serializers for canonical model, validate against live API | ✓ |
| Live API only | Probe live API, infer types from actual responses | |
| Python source only | Trust Django serializer definitions as authoritative | |

**User's choice:** Both sources (Recommended)
**Notes:** Use both Django serializer analysis AND live API validation

---

| Option | Description | Selected |
|--------|-------------|----------|
| Reference only | Look at gmazoyer/peeringdb for field names/types but write own schemas | |
| Don't reference | Derive everything independently from Python source + live API | ✓ |
| You decide | Claude picks | |

**User's choice:** Don't reference
**Notes:** No reference to existing Go libraries

---

| Option | Description | Selected |
|--------|-------------|----------|
| Generic wrapper | Parse wrapper once, extract data array, unmarshal by type | ✓ |
| Per-type parsers | Each object type gets its own response parser | |

**User's choice:** Generic wrapper

---

| Option | Description | Selected |
|--------|-------------|----------|
| Standalone package | internal/peeringdb — clean API client | ✓ |
| Coupled to sync | Parsing logic inside sync worker | |

**User's choice:** Standalone package

---

| Option | Description | Selected |
|--------|-------------|----------|
| Authenticate + throttle | PeeringDB API key, 40 req/min | |
| Unauthenticated + slow | No API key, 20 req/min | ✓ |

**User's choice:** Unauthenticated + slow

---

| Option | Description | Selected |
|--------|-------------|----------|
| All at once | All 13 types in dependency order, single pass | ✓ |
| Core first | Sync org/net/fac/ix first, then derived | |

**User's choice:** All at once

---

| Option | Description | Selected |
|--------|-------------|----------|
| Max page size + loop | Request max page size, loop all pages per type | ✓ |
| Parallel pages | Fetch multiple pages concurrently | |

**User's choice:** Max page size + loop

---

| Option | Description | Selected |
|--------|-------------|----------|
| Strict — fail on unknown | Unknown fields cause sync error | |
| Lenient — log and skip | Log unknown fields at warn, continue | ✓ |
| Capture all | Store unknown in JSON extras column | |

**User's choice:** Lenient — log and skip

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, via config | Base URL configurable via env var | ✓ |
| Hardcode it | Always api.peeringdb.com | |

**User's choice:** Yes, via config

---

| Option | Description | Selected |
|--------|-------------|----------|
| Timeouts + retries | Configurable timeout, retry with backoff | ✓ |
| Timeout only | Timeout but fail on first error | |

**User's choice:** Timeouts + retries

---

| Option | Description | Selected |
|--------|-------------|----------|
| Manual analysis | Read Django serializers manually | |
| Automated extraction | Write script to parse Django serializers | ✓ |

**User's choice:** Automated extraction

---

| Option | Description | Selected |
|--------|-------------|----------|
| In-repo cmd/ | Rerunnable tool in project | ✓ |
| One-off disposable | Run once, throw away | |

**User's choice:** In-repo cmd/

---

| Option | Description | Selected |
|--------|-------------|----------|
| Entgo schemas directly | Generate .go files directly | |
| Intermediate JSON | Output JSON, separate step generates entgo | ✓ |

**User's choice:** Intermediate JSON

---

| Option | Description | Selected |
|--------|-------------|----------|
| Python | Native Django introspection | |
| Go | Parse Python files as text, no Python dep | ✓ |

**User's choice:** Go

---

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-clone | Tool clones PeeringDB repo to temp dir | |
| Path argument | User provides path to existing checkout | ✓ |

**User's choice:** Path argument

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, built-in | Validate output against live API during run | ✓ |
| Separate step | Validation as independent tool | |

**User's choice:** Yes, built-in

---

| Option | Description | Selected |
|--------|-------------|----------|
| FK references | Each object lists FK fields with target type | ✓ |
| Edge list | Separate edges section with cardinality | |

**User's choice:** FK references

---

| Option | Description | Selected |
|--------|-------------|----------|
| Yes, full metadata | Include read-only, required, deprecated, help_text | ✓ |
| Types and names only | Just field names, Go types, FK refs | |

**User's choice:** Yes, full metadata

---

| Option | Description | Selected |
|--------|-------------|----------|
| Go tool in cmd/ | Fully automated pipeline | ✓ |
| Hand-written | Use JSON as reference, write schemas manually | |

**User's choice:** Go tool in cmd/

---

| Option | Description | Selected |
|--------|-------------|----------|
| Makefile | Single make target | |
| Separate commands | Run each step independently | |
| go generate | //go:generate directives | ✓ |

**User's choice:** go generate

---

## Sync Behavior

| Option | Description | Selected |
|--------|-------------|----------|
| Per-table upsert | UPSERT per object type in separate transactions | |
| Atomic swap | New DB file then swap | |
| Delete + insert per table | Truncate then insert per table | |
| Other | Single transaction wrapping entire sync | ✓ |

**User's choice:** Put the entire sync in a transaction, not just each object type. Don't use a second database file.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Disable FK constraints | Turn off PRAGMA foreign_keys during sync | |
| Nullable FKs | Model FK fields as optional, nil when missing | ✓ |
| Skip broken refs | Log and skip objects with broken FK refs | |

**User's choice:** Nullable FKs

---

| Option | Description | Selected |
|--------|-------------|----------|
| Rollback transaction | Keep previous good data | |
| Retry from start | Auto retry with backoff | |
| Both | Rollback + schedule retry with backoff | ✓ |

**User's choice:** Both

---

| Option | Description | Selected |
|--------|-------------|----------|
| In-process ticker | Go time.Ticker, no external deps | ✓ |
| Cron / external | External scheduler triggers endpoint | |

**User's choice:** In-process ticker

---

| Option | Description | Selected |
|--------|-------------|----------|
| HTTP endpoint | POST /sync | ✓ |
| CLI command | cmd/sync or flag | |
| Both | HTTP + CLI | |

**User's choice:** HTTP endpoint, protected behind shared secret auth
**Notes:** Must be protected to prevent public access

---

| Option | Description | Selected |
|--------|-------------|----------|
| Shared secret header | X-Sync-Token header matching configured secret | ✓ |
| Fly.io internal only | Bind to internal network | |
| Both | Internal + shared secret | |

**User's choice:** Shared secret header

---

| Option | Description | Selected |
|--------|-------------|----------|
| Mutex — skip if busy | Skip new request if sync running | ✓ |
| Queue — wait | Queue and run after current | |

**User's choice:** Mutex — skip if busy

---

| Option | Description | Selected |
|--------|-------------|----------|
| Per-type logging | Log start/complete for each of 13 types | |
| Summary only | Log totals and duration at end | |
| Both | Per-type progress + summary | ✓ |

**User's choice:** Both

---

| Option | Description | Selected |
|--------|-------------|----------|
| Metadata table | sync_status table in SQLite | ✓ |
| In-memory only | Track in struct, lost on restart | |

**User's choice:** Metadata table

---

| Option | Description | Selected |
|--------|-------------|----------|
| Same behavior | Same code path regardless | ✓ |
| Different | Optimized INSERT for empty DB | |

**User's choice:** Same behavior

---

| Option | Description | Selected |
|--------|-------------|----------|
| WAL mode | Enable WAL for concurrent reads | ✓ |
| Accept blocking | Readers block during sync | |

**User's choice:** WAL mode

---

| Option | Description | Selected |
|--------|-------------|----------|
| Primary only | Only LiteFS primary writes | ✓ |
| Any node | All nodes can sync, forward writes | |

**User's choice:** Primary only

---

| Option | Description | Selected |
|--------|-------------|----------|
| Block until ready | 503 until first sync completes | ✓ |
| Serve empty | Start serving with empty results | |

**User's choice:** Block until ready

---

| Option | Description | Selected |
|--------|-------------|----------|
| 3 retries, exponential | 30s, 2m, 8m backoff | ✓ |
| Unlimited, capped | Keep retrying until next scheduled sync | |

**User's choice:** 3 retries, exponential

---

| Option | Description | Selected |
|--------|-------------|----------|
| Basic from Phase 1 | Add spans around sync and per-type fetches now | ✓ |
| Defer to Phase 3 | Add OTel later | |

**User's choice:** Basic from Phase 1

---

| Option | Description | Selected |
|--------|-------------|----------|
| Hard delete | Remove rows not in latest response | ✓ |
| Soft delete | Mark with timestamp | |
| Match PeeringDB | Mirror status field approach | |

**User's choice:** Hard delete

---

| Option | Description | Selected |
|--------|-------------|----------|
| Exclude deleted | Only sync status=ok | |
| Include all | Sync all including deleted | |
| Configurable | Default exclude, allow include via config | ✓ |

**User's choice:** Configurable

---

| Option | Description | Selected |
|--------|-------------|----------|
| Env vars only | 12-factor, Fly.io native | ✓ |
| Flags + env | CLI flags with env fallbacks | |

**User's choice:** Env vars only

---

| Option | Description | Selected |
|--------|-------------|----------|
| Configurable via env | PDBPLUS_DB_PATH with sensible default | ✓ |
| Hardcoded | Fixed path | |

**User's choice:** Configurable via env

---

## Schema Fidelity

| Option | Description | Selected |
|--------|-------------|----------|
| Match PeeringDB exactly | info_prefixes4, irr_as_set | |
| Normalize to Go | InfoPrefixes4, IRRAsSet | |
| Go internally, PDB in API | Go names in schema, PeeringDB names via tags | ✓ |

**User's choice:** Go internally, PDB in API

---

| Option | Description | Selected |
|--------|-------------|----------|
| Use PeeringDB IDs | PeeringDB int ID as entgo PK | ✓ |
| Own IDs + PDB ID field | Auto-increment + separate indexed field | |

**User's choice:** Use PeeringDB IDs

---

| Option | Description | Selected |
|--------|-------------|----------|
| Entgo edges | Proper ent edges | ✓ |
| FK integer fields | Plain integers | |
| Both | Edges + raw FK field | |

**User's choice:** Entgo edges

---

| Option | Description | Selected |
|--------|-------------|----------|
| First-class entities | Full entgo schemas with own fields and edges | |
| Edge-through | M2M edge through tables | ✓ |

**User's choice:** Edge-through

---

| Option | Description | Selected |
|--------|-------------|----------|
| All fields | Complete data fidelity | ✓ |
| Curated subset | Commonly-used only | |

**User's choice:** All fields

---

| Option | Description | Selected |
|--------|-------------|----------|
| PeeringDB only | created/updated from PeeringDB | ✓ |
| Both | PeeringDB + local synced_at | |

**User's choice:** PeeringDB only

---

| Option | Description | Selected |
|--------|-------------|----------|
| Add now | All annotations upfront | ✓ |
| Defer per phase | Add when each API is built | |

**User's choice:** Add now

---

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-migrate on startup | entclient.Schema.Create() at boot | ✓ |

**User's choice:** Auto-migrate on startup, but only on the LiteFS primary

---

| Option | Description | Selected |
|--------|-------------|----------|
| Mirror as-is | All POC fields as PeeringDB returns | ✓ |
| Exclude POC | Skip entirely | |
| Configurable | Include by default, allow excluding | |

**User's choice:** Mirror as-is

---

| Option | Description | Selected |
|--------|-------------|----------|
| Add now | Index ASN, name, status, FK fields upfront | ✓ |
| Defer to Phase 2 | Add when queries built | |

**User's choice:** Add now

---

| Option | Description | Selected |
|--------|-------------|----------|
| Skip for now | No hooks needed | |
| Add basic hooks | Mutation hooks for OTel tracing | ✓ |

**User's choice:** Add basic hooks

---

| Option | Description | Selected |
|--------|-------------|----------|
| PeeringDB only | No entgo time mixins | ✓ |
| Both | Entgo mixin + PeeringDB fields | |

**User's choice:** PeeringDB only

---

| Option | Description | Selected |
|--------|-------------|----------|
| Commit generated code | Commit ent/ to git | ✓ |
| Generate at build | gitignore ent/ | |

**User's choice:** Commit generated code

---

| Option | Description | Selected |
|--------|-------------|----------|
| Fixtures/recorded | Fast, deterministic, no network | |
| Live API tests | Catches real drift but slow | |
| Both | Fixtures for CI + optional live tests | ✓ |

**User's choice:** Both

---

| Option | Description | Selected |
|--------|-------------|----------|
| Standard Go layout | cmd/, internal/, ent/schema/ | ✓ |
| Flat layout | Fewer directories | |

**User's choice:** Standard Go layout

---

| Option | Description | Selected |
|--------|-------------|----------|
| BSD 3-Clause | Permissive, common in networking | ✓ |
| Apache 2.0 | Permissive with patent grant | |
| MIT | Simple permissive | |

**User's choice:** BSD 3-Clause

---

| Option | Description | Selected |
|--------|-------------|----------|
| Phase 1 | Include Dockerfile now | ✓ |
| Phase 3 | Defer to production readiness | |

**User's choice:** Phase 1

---

| Option | Description | Selected |
|--------|-------------|----------|
| Single module | One go.mod for everything | ✓ |
| Go workspace | go.work with separate modules | |

**User's choice:** Single module

---

| Option | Description | Selected |
|--------|-------------|----------|
| peeringdb-plus | Matches repo/project name | ✓ |
| pdbplus | Shorter | |
| pdb+ | Concise but shell issues | |

**User's choice:** peeringdb-plus

---

| Option | Description | Selected |
|--------|-------------|----------|
| github.com/dotwaffle/peeringdb-plus | Standard GitHub module path | ✓ |
| go.peeringdb.plus | Vanity import path | |

**User's choice:** github.com/dotwaffle/peeringdb-plus

---

| Option | Description | Selected |
|--------|-------------|----------|
| modernc.org/sqlite | Pure Go, CGo-free | ✓ |
| mattn/go-sqlite3 | CGo-based, more mature | |

**User's choice:** modernc.org/sqlite

---

| Option | Description | Selected |
|--------|-------------|----------|
| IP storage | Text vs netip.Addr custom type | Claude's discretion |

---

## Claude's Discretion

- IP address field storage strategy
- Exact exponential backoff timing
- Exact sync_status table column set

## Deferred Ideas

None — discussion stayed within phase scope
