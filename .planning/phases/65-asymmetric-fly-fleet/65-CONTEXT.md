# Phase 65: Asymmetric Fly fleet — process groups + ephemeral replicas - Context

**Gathered:** 2026-04-17
**Status:** Ready for planning

<domain>
## Phase Boundary

Activate SEED-002. Transition the uniform 8-machine Fly fleet (all `shared-cpu-2x`/512 MB/1 GB volume) to an asymmetric layout:

- **`primary` process group**: 1 machine in LHR, `shared-cpu-2x`/512 MB, persistent volume (unchanged from today)
- **`replica` process group**: 7 machines (iad, nrt, syd, lax, jnb, sin, gru), `shared-cpu-1x`/256 MB, **ephemeral rootfs** (no persistent volume — cold-syncs 88 MB DB from primary on boot via LiteFS HTTP replication)

Works as-is because `litefs.yml` already region-gates primary candidacy (`lease.candidate: ${FLY_REGION == PRIMARY_REGION}`). No LiteFS, Consul, or code changes needed.

Cost delta: current $57.20/mo → $20.75/mo = ~$36/mo savings (~63%). Real win is operational simplicity (no replica volume orphans, faster recovery, cattle semantics).

</domain>

<decisions>
## Implementation Decisions

### Rollout strategy
- **D-01: Big bang.** Edit `fly.toml` once, `fly scale count primary=1 --region lhr && fly scale count replica=7 --region iad,nrt,syd,lax,jnb,sin,gru`, Fly handles the migration. Faster than staged. Rollback = edit `fly.toml` back, re-scale to uniform.
- **D-02:** Pre-flight: `fly deploy` with the new `fly.toml` first (no scale change). Confirm the binary still builds and rolls out. Only after the rolling deploy completes do we run the `fly scale` commands.
- **D-03:** Monitor `/readyz`, p99 latency from the existing OTel dashboard, and a manual `curl` against each region for ~15 min after the scale change.

### Replica sizing
- **D-04: `shared-cpu-1x` / 256 MB.** Matches SEED-002 proposal. Observed RSS ~58-59 MB; 256 MB gives ~4× headroom plus budget for LiteFS LTX hydration spikes. $1.94/mo each.
- **D-05: Primary unchanged.** `shared-cpu-2x` / 512 MB stays — primary is the sync workload and the existing Phase A/B memory profile from v1.13 assumes this size.

### Volume cleanup
- **D-06: Scripted cleanup inline in Phase 65 SUMMARY.** Not a reusable `scripts/` artifact. The commands (enumerate, confirm, destroy) go into `65-02-SUMMARY.md` as an operator runbook. Single-use migration — no reason to commit a persistent script.
- **D-07:** 7 volumes to destroy (one per replica region). Each was provisioned 1 GB. Manual `fly volumes list --app peeringdb-plus | grep -v lhr` then targeted `fly volumes destroy` per ID. Confirmation prompts accepted.

### Primary HA
- **D-08: Staying as-is for v1.15.** LHR remains the sole primary candidate. Phase 65 doesn't introduce HA. A new `SEED-003` will be planted capturing the primary-HA idea with its own trigger conditions (e.g. LHR extended outage, compliance requirement).

### LiteFS behaviour (unchanged, verified before rollout)
- **D-09:** `litefs.yml` stays unchanged. `lease.candidate: ${FLY_REGION == PRIMARY_REGION}` already gates candidacy by region, which the process-group split happens to reinforce (only primary group is in LHR).
- **D-10:** Cold-sync semantics: on replica boot, LiteFS pulls the 88 MB DB from primary via HTTP. `/readyz` fail-closes until DB is hydrated, so Fly routes traffic away during the ~5-45s window. Self-healing.

### Documentation
- **D-11: Full doc sweep for the new operational story.**
  - `docs/DEPLOYMENT.md` — add "Asymmetric fleet" subsection: how to scale, how replicas recover (destroy + recreate, no volume management), cold-sync expectations
  - `docs/ARCHITECTURE.md` — note the process-group split and ephemeral-replica model
  - `CLAUDE.md` §"Deployment" — same operational notes
  - `.planning/PROJECT.md` Key Decisions — record the v1.15 decision with cost + rationale

### Claude's Discretion
- Order of `fly scale` commands (primary first vs replica first) — likely primary-first so the elected leader is in place when replicas boot and cold-sync
- Whether to deploy the Phase 66 `sqlite3` quick task before or after this phase — user chose "run right now" as a pre-phase quick task, so already handled before Phase 65 starts
- Exact wording of the rollback runbook

### Folded Todos
- SEED-003 (primary HA hot-standby) — user asked to plant this seed now while context is fresh.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Plan-of-record
- `.planning/REQUIREMENTS.md` — INFRA-01 (process groups), INFRA-02 (ephemeral replicas), INFRA-03 (operational docs)
- `.planning/ROADMAP.md` §"Phase 65" — success criteria
- `.planning/seeds/SEED-002-fly-asymmetric-fleet.md` — full proposal with cost analysis and risks

### Fly.io concepts
- Fly process groups: `[processes]` in fly.toml + per-group `[[vm]]` sizing
- Fly volumes: persist across machine destroy unless explicitly removed
- `fly-replay` header: works within a single app across process groups

### Existing code & config
- `fly.toml` — single `[[vm]]`, unscoped `[mounts]`, no `[processes]`
- `litefs.yml` — `lease.candidate: ${FLY_REGION == PRIMARY_REGION}` is the region gate that makes this plan work
- `cmd/peeringdb-plus/main.go` — `PDBPLUS_IS_PRIMARY` env fallback exists for local dev (when LiteFS is absent); not used in prod

### Observed baselines (from v1.14 close, 2026-04-17)
- Primary: RSS 68.8 MB, peak VmHWM 83.8 MB
- Replicas: 58-59 MB steady
- DB: 88 MB (`/litefs/peeringdb-plus.db`)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `litefs.yml` region-gating is the keystone — SEED-002's feasibility hinges on it. No changes needed.
- Existing `/readyz` handler from `internal/health` already checks DB connectivity, which fails during LiteFS hydration. Fly honours this.

### Established Patterns
- `fly deploy` rolling strategy with `max_unavailable = 0.5` — unchanged, continues to work across process groups.

### Integration Points
- Grafana dashboard (Phase 66): may need an update to show process-group breakdown instead of uniform fleet view.
- `/about` page (Phase 61): doesn't show fleet topology; no change needed here.

</code_context>

<specifics>
## Specific Ideas

- **User chose big bang.** Trust the rollback path (re-scale to uniform). Don't over-engineer with canaries.
- **Volumes are the sticky part.** Fly charges for unattached volumes. Destroying the 7 replica volumes inline in the summary is the cleanest end-state.
- **SEED-003 first.** Before Phase 65 kicks off, plant SEED-003 for primary HA so that the known LHR-SPOF is captured formally.

</specifics>

<deferred>
## Deferred Ideas

- **Primary HA / hot standby** — SEED-003 (to be planted).
- **Auto-scaling replica count based on traffic** — future idea; Fly's `min_machines_running = 1` + `auto_stop_machines = off` is intentional for consistent global coverage.
- **Multi-primary LiteFS Cloud** — maintenance-mode, not a path forward per CLAUDE.md.

</deferred>

---

*Phase: 65-asymmetric-fly-fleet*
*Context gathered: 2026-04-17*
