# Roadmap: PeeringDB Plus

## Active Milestone: v1.15 — Infrastructure Polish & Schema Hygiene

**Status:** Roadmap defined 2026-04-17
**Phases:** 63-66 (4 phases)
**Estimated plans:** 6-9
**Theme:** Tidy-up after v1.14 ships. No new user-facing features. Reduce ops cost, close known schema gaps, complete one Phase 58 follow-up (field-level privacy for `ixlan.ixf_ixp_member_list_url`).

### Overview

v1.15 is an infrastructure-polish milestone with zero user-facing surface changes. Four phases:

1. **Phase 63 — Schema hygiene**: drop 3 vestigial ent fields confirmed dead by full-pipeline audit.
2. **Phase 64 — Field-level privacy**: close the `ixlan.ixf_ixp_member_list_url` gap Phase 58 missed and establish a reusable field-level redaction pattern for future gates.
3. **Phase 65 — Asymmetric Fly fleet**: transition from 8 uniform machines to 1 primary + 7 ephemeral replicas via Fly process groups (activates SEED-002).
4. **Phase 66 — Observability + sqlite3 tooling**: incident-response debug binary, heap-threshold monitoring, and peak-heap watch documentation tied to the dormant SEED-001 trigger.

All 11 requirements (HYGIENE-01..03, VIS-08, VIS-09, INFRA-01..03, OBS-04, OBS-05, DOC-04) map cleanly to phases 63-66.

### Phases

#### Phase 63: Schema hygiene — drop vestigial columns

**Goal:** Drop three ent schema fields confirmed vestigial through full-codebase audit (schema + `peeringdb` struct + sync upsert + pdbcompat serializer + upstream `/api/*` response verified on beta). Eliminate dead columns from the DB on next ent auto-migrate.

**Depends on:** Nothing (first phase of v1.15)
**Requirements:** HYGIENE-01, HYGIENE-02, HYGIENE-03
**Plans estimate:** 1

**Scope:**
- `ixprefix.notes` — upstream never emits; our schema + `peeringdb.IxPrefix` + pdbcompat serializer all reference it; always empty in prod DB. Also resolves the v1.14-deferred `ixpfx.notes` pdbcompat divergence by removing the field rather than allow-listing the divergence.
- `organization.fac_count` — pure vestigial; not in `peeringdb.Organization`, not in sync, not in serializer.
- `organization.net_count` — pure vestigial; same audit evidence.

**Work:**
1. Edit `ent/schema/ixprefix.go`, `ent/schema/organization.go` — remove the three fields.
2. Run `go generate ./...` — verify ent drift clean.
3. Update `internal/pdbcompat/serializer.go` ixpfx serializer to drop the `Notes` reference.
4. Update `internal/peeringdb/` to drop `IxPrefix.Notes` field.
5. ent auto-migrate drops columns on next app startup.

**Success criteria:**
- `go generate ./...` produces zero drift.
- `go test -race ./...` passes including `internal/pdbcompat/anon_parity_test.go` (the `ixpfx.notes` entry in `knownDivergences` must be removed since the field no longer exists).
- Conformance test against beta confirms no structural diff introduced.

**Risk:** Low. All changes confirmed vestigial by audit. One plan.

#### Phase 64: Field-level privacy — ixlan.ixf_ixp_member_list_url

**Goal:** Close the gap Phase 58 missed: the URL data field itself (not just the `_visible` companion) is absent from the ent schema + sync + serializer. Establish a reusable serializer-layer field-level redaction pattern for any future field-level gates (OAuth in v1.16+ will reuse the same substrate).

**Depends on:** Phase 63 (trivial — schema edits land in same codegen pipeline)
**Requirements:** VIS-08, VIS-09
**Plans:** 3/3 plans complete

**Scope:**
- Add the URL data field to `peeringdb.IxLan`, `ent/schema/ixlan.go`, sync upsert, and pdbcompat serializer.
- Establish a `internal/privfield` helper (or equivalent) for field-level redaction keyed on the pre-existing `<field>_visible` companion and the caller's `privctx.TierFrom(ctx)` tier.
- Apply the pattern uniformly across all 5 surfaces (Web UI, GraphQL, REST, pdbcompat, ConnectRPC).

**Work:**
1. Add `IXFIXPMemberListURL string` (json tag `ixf_ixp_member_list_url`) to `peeringdb.IxLan`.
2. Add `field.String("ixf_ixp_member_list_url")` to `ent/schema/ixlan.go` (regular field — no row-level privacy policy needed; field-level gate is a serializer-layer concern per VIS-08).
3. Wire `SetIxfIxpMemberListURL` in `internal/sync/upsert.go` ixlan path.
4. Build `internal/privfield` with `RedactIfUsers(ctx, visible, value string) string` (or chosen signature) that consults `privctx.TierFrom(ctx)` and the `<field>_visible` string.
5. Emit the URL in `internal/pdbcompat/serializer.go` ixlan serializer via the redaction helper.
6. Apply the same helper in GraphQL resolvers, entrest serializers, Web UI detail views, and ConnectRPC handlers for the ixlan URL field.
7. E2E test mirroring Phase 59 D-15 shape — verify the URL is absent from anonymous response across all 5 surfaces and present for Users-tier.
8. Update testdata baseline if needed (expected: redacted/absent in anon fixtures).

Plans:
- [x] 64-01-privfield-helper-PLAN.md — Build internal/privfield.Redact helper + unit tests (VIS-08 substrate; fail-closed semantics)
- [x] 64-02-schema-sync-wiring-PLAN.md — Append ent field to IxLan.Fields() (proto #14), wire peeringdb struct + sync upsert + seed + regen ent/proto/gqlgen/entrest (VIS-09 ingestion path)
- [x] 64-03-serializer-redaction-e2e-PLAN.md — Apply privfield.Redact across 5 serializer surfaces (pdbcompat, ConnectRPC, GraphQL resolver, REST middleware) + 5-surface E2E test + CLAUDE.md update (VIS-08 + VIS-09 enforcement)

**Success criteria:**
- URL present in DB for authenticated-sync deployments (Users-tier upstream rows).
- URL absent from anonymous responses on all 5 surfaces; present for Users-tier (via `PDBPLUS_PUBLIC_TIER=users` in test).
- `TestE2E_AnonymousCannotSeeIxlanURL` (or equivalent) locks the 5-surface behaviour.
- NULL `ixf_ixp_member_list_url_visible` column treated as schema default (`Public`) consistent with v1.14 Phase 58 decision (prevent post-upgrade row flood).
- `go generate ./...` drift-clean.

**Risk:** Medium. New architectural pattern (field-level redaction across 5 surfaces). Establishes the pattern for any future field-level gates (OAuth, etc.). Code changes cross every API surface.

#### Phase 65: Asymmetric Fly fleet — process groups + ephemeral replicas

**Goal:** Transition from uniform 8-machine fleet to asymmetric layout per SEED-002. 1 primary (LHR, `shared-cpu-2x`/512 MB, persistent volume) + 7 replicas (other regions, `shared-cpu-1x`/256 MB, ephemeral rootfs). Saves ~$36/mo (~63%); operational win is simplified volume management (replicas become cattle).

**Depends on:** Nothing (can run in parallel with Phase 63/64 — infrastructure change, no code overlap)
**Requirements:** INFRA-01, INFRA-02, INFRA-03
**Plans estimate:** 2-3
**Reference:** SEED-002 (activated; marked `consumed_by: v1.15-phase-65`; moves to `.planning/seeds/consumed/` when Phase 65 ships)

**Scope:** See SEED-002 for the full analysis. Summary:
- Single app `peeringdb-plus`, two Fly process groups (`primary`, `replica`), asymmetric `[[vm]]` sizing, `[[mounts]]` scoped to primary only.
- LiteFS region-gated candidacy (`candidate: ${FLY_REGION == PRIMARY_REGION}`) already correct — moving machines into process groups doesn't touch it.
- `/readyz` fail-closes during replica cold-sync so Fly excludes unhydrated replicas from routing.

**Work:**
1. Edit `fly.toml`: add `[processes]` with `primary` + `replica` entries; add two `[[vm]]` blocks scoped via `processes = [...]`; scope `[[mounts]]` to `processes = ["primary"]`.
2. Verify `litefs.yml` region-gated candidacy unchanged and correct.
3. Validate deploy strategy + `/readyz` cold-sync behaviour: destroy-recreate one replica; measure hydration time per region; confirm Fly excludes from routing until `/readyz` returns ok.
4. Update `docs/DEPLOYMENT.md` + `CLAUDE.md` with volume-only-on-primary contract and replica destroy-and-recreate recovery story.
5. Staged rollout: first a single test replica, observation window (p99 latency, hydration time, memory under load), then full fleet.

**Success criteria:**
- `fly.toml` reflects asymmetric process groups; `fly deploy` succeeds.
- Replica cold-sync completes within acceptable window (target: < 60s for the ~88 MB DB on commodity regions).
- `/readyz` correctly gates traffic during hydration — no user-visible half-hydrated responses.
- p99 latency on replica reads stays within current bounds after shared-cpu-1x/256 MB transition.
- Docs updated; SEED-002 moved to `consumed/`.

**Risk:** Medium. Infrastructure change on prod. Rollback path is one `fly.toml` edit + redeploy. Staged rollout mitigates.

#### Phase 66: Observability + sqlite3 tooling

**Goal:** Small follow-up catchall — post-v1.14 monitoring items flagged during milestone close. Make the SEED-001 trigger observable and give operators an incident-response tool.

**Depends on:** Nothing (can run in parallel with Phase 63/64/65)
**Requirements:** OBS-04, OBS-05, DOC-04
**Plans estimate:** 1-2

**Scope:**
- `sqlite3` static binary in prod image for `fly ssh console` debugging.
- Heap-threshold monitoring hooked up — `/readyz`-compatible check OR OTel span attribute flagging peak heap > configurable threshold (default 380 MiB, the SEED-001 trigger).
- Grafana dashboard verification — heap gauge filterable by `pdbplus.privacy.tier` (v1.14 Phase 61 attribute).
- Operator documentation tying the heap trigger to SEED-001.

**Work:**
1. Add `sqlite3` (Chainguard static binary, ~1 MB) to prod `Dockerfile` as a COPY from the chainguard/sqlite image or equivalent.
2. Implement heap-threshold monitoring: `runtime.ReadMemStats().HeapAlloc` vs configurable threshold (default 380 MiB via new env var, e.g. `PDBPLUS_HEAP_WARN_MIB`), exposed as either a `/readyz` degraded signal or an OTel span attribute.
3. Verify or add a Grafana dashboard panel for peak heap over time, filterable by `pdbplus.privacy.tier`.
4. Document the expectation in `CLAUDE.md` + `docs/DEPLOYMENT.md`: "if heap sustained >380 MiB, SEED-001 trigger fired — revisit incremental sync."

**Success criteria:**
- `fly ssh console` in prod has `sqlite3` available; confirmed by `sqlite3 /litefs/peeringdb-plus.db ".tables"` on the primary.
- Heap-threshold signal demonstrable via test that crosses the threshold (use a low threshold in test).
- Grafana heap panel present and tagged with `pdbplus.privacy.tier` filter.
- Docs updated — SEED-001 trigger has a concrete escalation path in the operator runbook.

**Risk:** Low. Dockerfile one-line change + small runtime metric + Grafana dashboard JSON + docs.

### Progress

| Phase | Status | Plans | Requirements | Blocker |
|-------|--------|-------|--------------|---------|
| 63 — Schema hygiene | Pending | 1/1 | Complete   | 2026-04-18 |
| 64 — Field-level privacy | Pending | 3/3 | Complete   | 2026-04-18 |
| 65 — Asymmetric Fly fleet | Pending | 0/2-3 | INFRA-01, INFRA-02, INFRA-03 | — (parallelizable) |
| 66 — Observability + sqlite3 | Pending | 0/1-2 | OBS-04, OBS-05, DOC-04 | — (parallelizable) |

**Totals:** 0/4 phases complete, 0/~6-9 plans, 0/11 requirements satisfied.

### Success Criteria (milestone-level)

- All 11 requirements satisfied with traceability closed.
- `go generate ./...` + `go test -race ./...` + `golangci-lint run` clean on every phase boundary.
- Prod Fly.io `peeringdb-plus` running asymmetric fleet with savings confirmed on next invoice.
- SEED-002 moved to `consumed/`; SEED-001 remains dormant with observable trigger.
- Zero user-facing surface regressions (v1.15 is infrastructure-only by design).

---

## Shipped Milestones

- **v1.0 MVP** (shipped 2026-03-22) — Phases 1-3. See [MILESTONES.md](./MILESTONES.md).
- **v1.1 REST API & Observability** (shipped 2026-03-23) — Phases 4-6. See [MILESTONES.md](./MILESTONES.md).
- **v1.2 Quality, Incremental Sync & CI** (shipped 2026-03-24) — Phases 7-10. See [MILESTONES.md](./MILESTONES.md).
- **v1.3 PeeringDB API Key Support** (shipped 2026-03-24) — Phases 11-12. See [MILESTONES.md](./MILESTONES.md).
- **v1.4 Web UI** (shipped 2026-03-24) — Phases 13-17. See [MILESTONES.md](./MILESTONES.md).
- **v1.5 Tech Debt & Observability** (shipped 2026-03-24) — Phases 18-20. See [MILESTONES.md](./MILESTONES.md).
- **v1.6 ConnectRPC / gRPC API** (shipped 2026-03-25) — Phases 21-24. See [MILESTONES.md](./MILESTONES.md).
- **v1.7 Streaming RPCs & UI Polish** (shipped 2026-03-25) — Phases 25-27. See [MILESTONES.md](./MILESTONES.md).
- **v1.8 Terminal CLI Interface** (shipped 2026-03-26) — Phases 28-31. See [MILESTONES.md](./MILESTONES.md).
- **v1.9 Hardening & Polish** (shipped 2026-03-26) — Phases 32-36. See [MILESTONES.md](./MILESTONES.md).
- **v1.10 Code Coverage & Test Quality** (shipped 2026-03-26) — Phases 37-42. See [MILESTONES.md](./MILESTONES.md).
- **v1.11 Web UI Density & Interactivity** (shipped 2026-03-26) — Phases 43-46. See [MILESTONES.md](./MILESTONES.md).
- **v1.12 Hardening & Tech Debt** (shipped 2026-04-02) — Phases 47-50. See [MILESTONES.md](./MILESTONES.md).
- **v1.13 Security & Sync Hardening** (shipped 2026-04-11) — Phases 51-56. See [MILESTONES.md](./MILESTONES.md).
- **v1.14 Authenticated Sync & Visibility Layer** (shipped 2026-04-17) — [archive](./milestones/v1.14-ROADMAP.md). 6 phases (57-62), 21 plans, 17/17 requirements.

## Phase Numbering

- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

## History

All shipped milestones are summarised in [MILESTONES.md](./MILESTONES.md). Per-milestone ROADMAP snapshots live at `.planning/milestones/v{X.Y}-ROADMAP.md`, and phase artifacts (plans, summaries, verification reports) at `.planning/phases/` (current milestone) or `.planning/milestones/v{X.Y}-phases/` (archived).

## Backlog

### Phase 999.1: Harden pdb-schema-generate — preserve hand-added Policy() on poc.go (BACKLOG)

**Goal:** Fix `cmd/pdb-schema-generate` so it preserves hand-edited `ent/schema/poc.go` `Policy()` (or move `Policy()` out of the generated schema file) — so `go generate ./...` is safe to run again. Current workaround is `go generate ./ent` only, documented in CLAUDE.md. Surfaced as Phase 63 executor deviation #1 (low priority — workaround is stable).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /gsd-review-backlog when ready)
