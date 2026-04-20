# Roadmap: PeeringDB Plus

## Milestones

- ✅ **v1.0 – v1.14** — shipped (see [MILESTONES.md](./MILESTONES.md))
- ✅ **v1.15 — Infrastructure Polish & Schema Hygiene** — shipped 2026-04-18 (Phases 63-66, 11 requirements)
- ✅ **v1.16 — Django-compat Correctness** — shipped 2026-04-19 (Phases 67-72, 25 requirements)

## Phases

<details>
<summary>✅ v1.16 — Django-compat Correctness (Phases 67-72) — SHIPPED 2026-04-19</summary>

- [x] Phase 67: Default ordering flip (6 plans)
- [x] Phase 68: Status × since matrix + limit=0 semantics (4 plans)
- [x] Phase 69: Filter-value Unicode folding, operator coercion, __in robustness (6 plans)
- [x] Phase 70: Cross-entity __ traversal (Path A + Path B + 2-hop) (8 plans)
- [x] Phase 71: Memory-safe response paths on 256 MB replicas (6 plans)
- [x] Phase 72: Upstream parity regression + divergence docs (6 plans)

Archive: [`.planning/milestones/v1.16-ROADMAP.md`](./milestones/v1.16-ROADMAP.md)
Requirements: [`.planning/milestones/v1.16-REQUIREMENTS.md`](./milestones/v1.16-REQUIREMENTS.md)
Audit: [`.planning/milestones/v1.16-MILESTONE-AUDIT.md`](./milestones/v1.16-MILESTONE-AUDIT.md)

</details>

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

All shipped milestones are summarised in [MILESTONES.md](./MILESTONES.md). Per-milestone ROADMAP snapshots live at `.planning/milestones/v{X.Y}-ROADMAP.md`, and phase artifacts at `.planning/milestones/v{X.Y}-phases/` (archived) or `.planning/phases/` (current milestone).

## Backlog

_No parked 999.x phases._
