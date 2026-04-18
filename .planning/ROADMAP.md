# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 – v1.14** — shipped (see [MILESTONES.md](./MILESTONES.md))
- ✅ **v1.15 — Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18 (Phases 63-66, 11 requirements)

## Phases

<details>
<summary>✅ v1.15 — Infrastructure Polish & Schema Hygiene (Phases 63-66) — SHIPPED 2026-04-18</summary>

- [x] Phase 63: Schema hygiene — drop vestigial columns (1 plan)
- [x] Phase 64: Field-level privacy — ixlan.ixf_ixp_member_list_url (3 plans)
- [x] Phase 65: Asymmetric Fly fleet — process groups + ephemeral replicas (2 plans)
- [x] Phase 66: Observability + sqlite3 tooling (3 plans)

Archive: [`.planning/milestones/v1.15-ROADMAP.md`](./milestones/v1.15-ROADMAP.md)
Requirements: [`.planning/milestones/v1.15-REQUIREMENTS.md`](./milestones/v1.15-REQUIREMENTS.md)
Audit: [`.planning/milestones/v1.15-MILESTONE-AUDIT.md`](./milestones/v1.15-MILESTONE-AUDIT.md)

</details>

All shipped milestones are summarised in [MILESTONES.md](./MILESTONES.md). Per-milestone ROADMAP snapshots live at `.planning/milestones/v{X.Y}-ROADMAP.md`, and phase artifacts (plans, summaries, verification reports) at `.planning/milestones/v{X.Y}-phases/` (archived) or `.planning/phases/` (current milestone).

## Backlog

### Phase 999.1: Harden pdb-schema-generate — preserve hand-added Policy() on poc.go (BACKLOG)

**Goal:** Fix `cmd/pdb-schema-generate` so it preserves hand-edited `ent/schema/poc.go` `Policy()` (or move `Policy()` out of the generated schema file) — so `go generate ./...` is safe to run again. Current workaround is `go generate ./ent` only, documented in CLAUDE.md. Surfaced as Phase 63 executor deviation #1 (low priority — workaround is stable).
**Requirements:** TBD
**Plans:** 0 plans

Plans:
- [ ] TBD (promote with /gsd-review-backlog when ready)
