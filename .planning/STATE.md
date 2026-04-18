---
gsd_state_version: 1.0
milestone: v1.16
milestone_name: — Django-compat Correctness
status: roadmap defined
stopped_at: roadmap defined — ready for /gsd-plan-phase 67
last_updated: "2026-04-18T13:00:00.000Z"
last_activity: 2026-04-18
progress:
  total_phases: 6
  completed_phases: 0
  total_plans: 0
  completed_plans: 0
  percent: 0
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-04-18)

**Core value:** Fast, reliable access to PeeringDB data from anywhere in the world, served from the nearest edge node with low latency.
**Current focus:** v1.16 Django-compat Correctness — align `pdbcompat` with upstream Django semantics (validated against `peeringdb/peeringdb@99e92c72`); memory-safe response paths on 256 MB replicas.

## Current Position

Phase: 67 — not started (roadmap defined, awaiting plan)
Plan: —
Status: Roadmap defined; 25/25 v1.16 requirements mapped; awaiting `/gsd-plan-phase 67`
Last activity: 2026-04-18 — v1.16 roadmap created (Phases 67-72)

## v1.16 Phase Map

Phases 67-72 cover 25 REQ-IDs across 8 categories (ORDER, STATUS, LIMIT, IN, UNICODE, TRAVERSAL, MEMORY, PARITY). All dependencies are strictly serial — no phases run in parallel in this milestone.

| Phase | Goal | Requirements | Depends on |
|-------|------|--------------|------------|
| 67 | Default ordering flip to `(-updated, -created)` across pdbcompat + grpcserver + entrest | ORDER-01, ORDER-02, ORDER-03 | — |
| 68 | Status × since matrix + `limit=0` unlimited semantics in pdbcompat | STATUS-01..05, LIMIT-01, LIMIT-02 | 67 |
| 69 | Unicode folding, operator coercion, `__in` robustness in pdbcompat filter layer | IN-01, IN-02, UNICODE-01, UNICODE-02, UNICODE-03 | 68 |
| 70 | Cross-entity `__` traversal: Path A allowlists + Path B introspection + 2-hop | TRAVERSAL-01..04 | 69 |
| 71 | Memory-safe response paths on 256 MB replicas (streaming JSON, per-response ceiling, telemetry, docs) | MEMORY-01..04 | 67, 68, 69, 70 |
| 72 | Upstream parity regression tests ported from `pdb_api_test.py` + divergence docs | PARITY-01, PARITY-02 | 67, 68, 69, 70, 71 |

Dependency chain: `67 → 68 → 69 → 70 → 71 → 72` (fully sequential). Phases 68 and 69 both touch `internal/pdbcompat/filter.go`, so serialising avoids merge conflicts; Phase 71 is deliberately staged after 67-70 so the memory ceiling can be sized against the real worst-case response shapes those phases enable; Phase 72 closes the milestone by locking the new semantics in regression tests.

## Recently Shipped

**v1.15 Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18. 4 phases (63-66), 11 requirements. Archive: [`.planning/milestones/v1.15-ROADMAP.md`](./milestones/v1.15-ROADMAP.md).

**v1.14 Authenticated Sync & Visibility Layer** — 6 phases (57-62), 21 plans, 17/17 requirements, audit PASSED.

- **Commit range:** `8511805..c496b72` (132 commits)
- **Files changed:** 258 files, +164243 / -373 LOC (bulk is Phase 57 baseline fixture commits)
- **Timeline:** 2026-04-16 → 2026-04-17
- **Archive:** [`.planning/milestones/v1.14-ROADMAP.md`](./milestones/v1.14-ROADMAP.md)
- **Requirements archive:** [`.planning/milestones/v1.14-REQUIREMENTS.md`](./milestones/v1.14-REQUIREMENTS.md)
- **Audit:** [`.planning/v1.14-MILESTONE-AUDIT.md`](./v1.14-MILESTONE-AUDIT.md)

Post-v1.14 follow-ups surfaced in v1.15:

- `ixpfx.notes` pdbcompat divergence → resolved by Phase 63 (drop the field).
- Missing `ixlan.ixf_ixp_member_list_url` URL data field → closed by Phase 64.
- Peak-heap watch story tied to SEED-001 → Phase 66 makes the trigger observable and documents the escalation path.

## Outstanding Human Verification

Deferred items tracked for manual confirmation:

- **Phase 52 (v1.13):** Chrome devtools CSP check on `/ui/`, `/ui/asn/13335`, `/ui/compare`
- **Phase 53 (v1.13):** curl HSTS / X-Frame-Options / X-Content-Type-Options headers, 2 MB body-cap REST vs gRPC skip-list, slowloris TCP smoke test

Phase 57 + Phase 62 (v1.14) UAT items all resolved 2026-04-17 — see `.planning/phases/57-visibility-baseline-capture/57-HUMAN-UAT.md` and `.planning/phases/62-api-key-default-docs/62-HUMAN-UAT.md` (both `status: resolved`).

See `memory/project_human_verification.md` for the full backlog across v1.6, v1.7, v1.11, v1.13.

## Accumulated Context

### Decisions

All decisions archived in PROJECT.md Key Decisions table (46+ decisions across 15 milestones).

v1.14 decisions captured in PROJECT.md rows added during Phase 58 (schema sufficiency, `<field>_visible` naming, NULL-as-schema-default, regression test locks empirical assumption).

v1.15 decisions captured in PROJECT.md rows — schema hygiene drops (Phase 63), asymmetric Fly fleet (Phase 65), sync observability hybrid (Phase 66).

v1.16 decisions pending — to be logged at phase transitions. Likely candidates:

- Path A (`prepare_query` allowlist) representation — per-entity map, code-generated table, or declarative struct literals (Phase 70 discuss)
- Path B FK-introspection source — ent schema graph walker vs hand-maintained registry (Phase 70 discuss)
- Unicode-folding library — third-party `rainycape/unidecode` vs stdlib `golang.org/x/text/unicode/norm` + fold table (Phase 69 discuss)
- Streaming JSON encoder choice for large responses — `encoding/json.NewEncoder` with rollout chunking vs a custom tokeniser (Phase 71 discuss)
- Response memory ceiling surfacing — RFC 9457 problem-detail 413 vs truncation sentinel in envelope (Phase 71 discuss)

### Seeds

- **SEED-001** — incremental sync evaluation. Dormant. No trigger fired (peak heap ~84 MB vs 380 MiB). v1.15 Phase 66 wired the trigger observability; v1.16 Phase 71 extends the harness to response paths.
- **SEED-002** — asymmetric Fly fleet. **Consumed** by v1.15 Phase 65.
- **SEED-003** — primary HA hot-standby. Dormant. Planted 2026-04-17 when the v1.15 decision was made to keep LHR as sole primary candidate. Triggers: LHR extended outage, maintenance burden, compliance, Fly capacity pressure.

### Pending Todos

None.

### Blockers/Concerns

None. All 25 v1.16 REQ-IDs mapped to the 6 phases; 100% coverage validated. No orphans.

### Quick Tasks Completed

| # | Description | Date | Commit | Directory |
|---|-------------|------|--------|-----------|
| 260331-cxk | Move maps to bottom of pages and add fold-out arrows to collapsibles | 2026-03-31 | eefa79b | [260331-cxk-move-maps-to-bottom-of-pages-and-add-fol](./quick/260331-cxk-move-maps-to-bottom-of-pages-and-add-fol/) |
| 260414-2rc | Reduce OTel metric cardinality per plan ethereal-petting-pelican.md | 2026-04-14 | 3e0e56b (PR #11) | [260414-2rc-reduce-otel-metric-cardinality-per-plan-](./quick/260414-2rc-reduce-otel-metric-cardinality-per-plan-/) |
| 20260417-v114-lint-cleanup | Clear 7 golangci-lint findings post-v1.14 (gosec/exhaustive/nolintlint/revive); resolves Phase 58 deferred-items.md | 2026-04-17 | d15dd02 | [20260417-v114-lint-cleanup](./quick/20260417-v114-lint-cleanup/) |
| 260418-1cn | Add sqlite3 to Dockerfile.prod + fly deploy + verify on primary & replica (pre-Phase-65 prep) | 2026-04-18 | 4dfc52a | [260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de](./quick/260418-1cn-add-sqlite3-binary-to-dockerfile-prod-de/) |
| 260418-gf0 | Fix pdb-schema-generate — move Poc.Policy to poc_policy.go + add ixlan URL to schema JSON; resolves backlog 999.1 | 2026-04-18 | 73bbe04 | [260418-gf0-fix-pdb-schema-generate-preserve-policy-](./quick/260418-gf0-fix-pdb-schema-generate-preserve-policy-/) |

## Session Continuity

Last session: 2026-04-18
Last activity: 2026-04-18
Stopped at: v1.16 roadmap defined. Next: `/gsd-plan-phase 67` to begin Phase 67 (default ordering flip).

**Resume via `/gsd-plan-phase 67` or `/gsd-autonomous`:**

Each of phases 67-72 needs its own CONTEXT.md (discuss-phase) before planning. Key strategic guidance already in ROADMAP.md:

1. **Phase 67 — Ordering flip.** Broadest touch (3 surfaces: pdbcompat + grpcserver + entrest) but thinnest behavioural change. Land first so 68/69 rebase cleanly.
2. **Phase 68 — Status + limit.** Pdbcompat-only. Shares list-path code with 67 — sequence after it.
3. **Phase 69 — Unicode + operators + __in.** Shares `internal/pdbcompat/filter.go` with 70 — serialise 69 → 70 to avoid merge pain.
4. **Phase 70 — Traversal.** Largest phase, likely multi-plan. Path A (allowlists) + Path B (introspection) + 2-hop + silent-ignore-unknown-fields.
5. **Phase 71 — Memory-safe response paths.** Deliberately staged last-before-parity so the ceiling can be sized against real worst-case shapes from 67-70. Reuses Phase 66 `runtime.MemStats` harness.
6. **Phase 72 — Parity regression.** Ports ground-truth assertions from `pdb_api_test.py`. Locks 67-71 semantics; documents intentional divergences in `docs/API.md`.

**Memory budget reminder (from v1.15 Phase 65):**

- Primary (LHR, shared-cpu-2x/512 MB): peak VmHWM 83.8 MB; plenty of headroom
- Replicas (7 regions, shared-cpu-1x/256 MB): ~58-59 MB steady; this is the constraining envelope for Phase 71
- DB size: 88 MB (on LiteFS)

**Autonomous entry command:** `/gsd-autonomous` — picks up at Phase 67 (next incomplete) and walks through.
