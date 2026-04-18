---
phase: 65-asymmetric-fly-fleet
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - fly.toml
  - docs/DEPLOYMENT.md
  - docs/ARCHITECTURE.md
  - CLAUDE.md
  - .planning/PROJECT.md
  - .planning/seeds/SEED-002-fly-asymmetric-fleet.md
  - .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md
autonomous: true
requirements: [INFRA-01, INFRA-03]
tags: [infra, fly, litefs, deployment]

must_haves:
  truths:
    - "fly.toml declares two process groups (primary, replica) with per-group [[vm]] sizing"
    - "[[mounts]] is scoped to processes = [\"primary\"] only"
    - "docs/DEPLOYMENT.md has an 'Asymmetric fleet' subsection covering cold-sync, destroy-and-recreate recovery, and volume-only-on-primary contract"
    - "docs/ARCHITECTURE.md notes the process-group split and ephemeral replicas"
    - "CLAUDE.md §Deployment lists the asymmetric fleet contract"
    - ".planning/PROJECT.md Key Decisions has a Phase 65 row"
    - "SEED-002 is moved to .planning/seeds/consumed/"
    - "litefs.yml is UNCHANGED"
  artifacts:
    - path: "fly.toml"
      provides: "Asymmetric fleet declaration"
      contains: "[processes]"
    - path: "docs/DEPLOYMENT.md"
      provides: "Asymmetric fleet operator documentation"
      contains: "Asymmetric fleet"
    - path: ".planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md"
      provides: "Consumed-seed archive per CONTEXT D-11"
  key_links:
    - from: "fly.toml"
      to: "litefs.yml"
      via: "PRIMARY_REGION env drives LiteFS lease.candidate gating — MUST remain unchanged"
      pattern: "candidate:.*PRIMARY_REGION"
    - from: "docs/DEPLOYMENT.md"
      to: "fly.toml"
      via: "operator runbook references the [processes] and per-group [[vm]] blocks"
      pattern: "\\[processes\\]|primary|replica"
---

<objective>
Land all repo-only artifacts needed for the Phase 65 asymmetric-fly-fleet migration. This plan DOES NOT touch live Fly.io infrastructure — every task is a file edit in this repository.

Purpose: Separate the reversible repo edits (fly.toml, docs, seed move) from the irreversible production migration (Plan 02) so that Plan 02's executor has a clean, already-merged baseline to work against. If the migration is ever postponed, the fly.toml still has to be the next-deploy target; landing it here makes it discoverable.

Output:
- Updated `fly.toml` with `[processes]` + per-group `[[vm]]` + scoped `[[mounts]]`
- Doc sweep across 4 files (DEPLOYMENT.md, ARCHITECTURE.md, CLAUDE.md, PROJECT.md)
- SEED-002 moved to `.planning/seeds/consumed/`
- Zero Go code changes, zero Fly.io state changes

Implements:
- INFRA-01 (process groups declared in fly.toml) — satisfied by Task 1
- INFRA-03 (operational docs updated, SEED-002 consumed) — satisfied by Tasks 2-3
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/65-asymmetric-fly-fleet/65-CONTEXT.md
@.planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md

# Current state of each file we edit
@fly.toml
@docs/DEPLOYMENT.md
@docs/ARCHITECTURE.md
@CLAUDE.md
@.planning/seeds/SEED-002-fly-asymmetric-fleet.md

<interfaces>
<!-- Key invariants to preserve across this plan's edits -->

litefs.yml (UNCHANGED — do not edit):
```yaml
lease:
  type: "consul"
  advertise-url: "http://${HOSTNAME}.vm.${FLY_APP_NAME}.internal:20202"
  candidate: ${FLY_REGION == PRIMARY_REGION}
  promote: true
  hostname: ${HOSTNAME}
```

fly.toml today (single [[vm]], unscoped [mounts], no [processes]):
- `[mounts]` (singular TOML table, NOT `[[mounts]]`) — must become `[[mounts]]` with `processes = ["primary"]`
- `[[vm]]` — must stay as a table-array, one entry per process group
- `max_unavailable = 0.5` — keep; research A6 says this is survivable for the one-time migration
- `primary_region = "lhr"` — keep
- `[consul] enable = true` — keep
- `[env] PRIMARY_REGION = "lhr"` — keep

From 65-RESEARCH.md "Target fly.toml" section — use the exact contents VERBATIM. Do not paraphrase.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Rewrite fly.toml to asymmetric-fleet topology</name>
  <files>fly.toml</files>
  <read_first>
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Target fly.toml (complete, ready to commit)" — this is the verbatim source
    - Current fly.toml (to confirm what's being replaced)
    - litefs.yml (to confirm the `candidate:` line is NOT in fly.toml — fly.toml never references litefs.yml; purely a sanity check that we're not about to break the keystone)
  </read_first>
  <action>
Replace the entire contents of `fly.toml` with the target from 65-RESEARCH.md § "Target fly.toml". The exact final file MUST be:

```toml
# Fly.io deployment configuration for PeeringDB Plus
# Deploy with: fly deploy (per D-22, manual deploy)
# Phase 65: asymmetric fleet — primary (LHR, persistent) + replica (7 regions, ephemeral)

app = "peeringdb-plus"
primary_region = "lhr"
kill_timeout = 30

[build]
  dockerfile = "Dockerfile.prod"

[deploy]
  strategy = "rolling"
  # Parallelise rolling deploys to ~4-at-a-time (half the 8-machine fleet).
  # Trades ~55% worst-moment capacity for ~3-4x faster rollouts.
  # Bluegreen is NOT an option here: parallel fleets fight LiteFS +
  # Consul primary election.
  max_unavailable = 0.5

[env]
  PDBPLUS_LISTEN_ADDR = ":8080"
  PDBPLUS_DB_PATH = "/litefs/peeringdb-plus.db"
  PRIMARY_REGION = "lhr"

# Consul is required for LiteFS leader election.
[consul]
  enable = true

# Two process groups: primary (1 machine, LHR) and replica (7 machines).
# Both run `litefs mount` (the container ENTRYPOINT) — no separate binary.
# The command string here is informational; the image ENTRYPOINT wins at runtime.
# Fly docs state the command is passed to the Dockerfile entrypoint; since our
# ENTRYPOINT is ["litefs", "mount"] (Dockerfile.prod:35), the effective command
# is unchanged from today. Both groups share identical image + entrypoint.
[processes]
  primary = ""
  replica = ""

# Persistent volume attached to the primary group ONLY.
# Replica machines get no mount — LiteFS cold-syncs the DB to ephemeral rootfs.
[[mounts]]
  source = "litefs_data"
  destination = "/var/lib/litefs"
  processes = ["primary"]
  initial_size = "1GB"
  scheduled_snapshots = false
  auto_extend_size_threshold = 80
  auto_extend_size_increment = "1G"
  auto_extend_size_limit = "10G"

# HTTP service serves traffic from BOTH groups (routed by Fly Proxy per region).
# Without `processes = [...]`, the service applies to all groups — explicit here
# for documentation.
[http_service]
  internal_port = 8080
  force_https = true
  auto_stop_machines = "off"
  auto_start_machines = true
  min_machines_running = 1
  processes = ["primary", "replica"]

  [http_service.http_options]
    h2_backend = true

  [http_service.concurrency]
    type = "requests"
    soft_limit = 10

  [[http_service.checks]]
    grace_period = "30s"
    interval = "15s"
    method = "GET"
    timeout = "5s"
    path = "/healthz"

# Primary group VM: unchanged from today (shared-cpu-2x / 512 MB).
[[vm]]
  size = "shared-cpu-2x"
  memory = "512mb"
  processes = ["primary"]

# Replica group VM: downsized to shared-cpu-1x / 256 MB.
# Observed replica RSS 58-59 MB → 256 MB gives ~4× headroom + hydration spike budget.
[[vm]]
  size = "shared-cpu-1x"
  memory = "256mb"
  processes = ["replica"]
```

Post-write checks (must all pass before commit):
1. `grep -c '^\[processes\]' fly.toml` → `1`
2. `grep -cE '^\[\[vm\]\]' fly.toml` → `2`
3. `grep -cE '^\[\[mounts\]\]' fly.toml` → `1` (MUST be the double-bracket table-array form, not the old `[mounts]` singular-table form)
4. `grep -c 'processes = \["primary"\]' fly.toml` → at least `2` (one on `[[mounts]]`, one on `[[vm]]` primary)
5. `grep -c 'processes = \["replica"\]' fly.toml` → at least `1`
6. `grep -c 'processes = \["primary", "replica"\]' fly.toml` → `1` (on `[http_service]`)
7. `grep 'shared-cpu-1x' fly.toml` → matches once, on the replica vm block
8. `grep 'shared-cpu-2x' fly.toml` → matches once, on the primary vm block
9. Run `fly config validate --config fly.toml` if `flyctl` is available on the executor path. If `flyctl` is NOT on PATH, skip this step and note it in the SUMMARY — Plan 02 executor will catch any syntax error on first `fly deploy`.

Do NOT touch litefs.yml. Do NOT modify any Go source. Do NOT change `Dockerfile.prod`. Do NOT add restart policies.
  </action>
  <verify>
    <automated>grep -c '^\[processes\]' fly.toml | grep -q '^1$' && grep -cE '^\[\[vm\]\]' fly.toml | grep -q '^2$' && grep -cE '^\[\[mounts\]\]' fly.toml | grep -q '^1$' && grep -q 'processes = \["primary"\]' fly.toml && grep -q 'processes = \["replica"\]' fly.toml && grep -q 'shared-cpu-1x' fly.toml && grep -q 'memory = "256mb"' fly.toml</automated>
  </verify>
  <done>
fly.toml contains exactly one `[processes]` block with `primary` + `replica` entries, exactly two `[[vm]]` blocks each scoped via `processes = [...]`, exactly one `[[mounts]]` block scoped to `processes = ["primary"]`, and `[http_service]` lists both groups. `git diff fly.toml` shows the rewrite and nothing outside fly.toml. `go build ./...` still succeeds (defensive — fly.toml is not a Go input, but cheap sanity).
  </done>
</task>

<task type="auto">
  <name>Task 2: Documentation sweep — DEPLOYMENT.md, ARCHITECTURE.md, CLAUDE.md, PROJECT.md</name>
  <files>docs/DEPLOYMENT.md, docs/ARCHITECTURE.md, CLAUDE.md, .planning/PROJECT.md</files>
  <read_first>
    - .planning/phases/65-asymmetric-fly-fleet/65-CONTEXT.md § Decisions D-11 (doc sweep targets)
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Architecture Patterns" and § "Pattern 2: LiteFS cold-sync on ephemeral rootfs" and § "Pattern 3: /readyz fail-closed during hydration" — source material for the documentation
    - docs/DEPLOYMENT.md (sections around LiteFS at lines 149-200 and Regional rollout at 202-224)
    - docs/ARCHITECTURE.md (LiteFS primary/replica detection section at line 346)
    - CLAUDE.md (§ Deployment at line 168)
    - .planning/PROJECT.md § Key Decisions (line 159+)
  </read_first>
  <action>
Edit four files. Every change should be additive where possible — do not rewrite existing content, only insert new subsections and rows.

**docs/DEPLOYMENT.md** — after the `## LiteFS` section (ends around line 200) and before `## Regional rollout`, insert a new `## Asymmetric fleet` subsection with this content:

```markdown
## Asymmetric fleet

As of v1.15 (Phase 65), the fleet is split into two Fly process groups
with different VM sizing and mount policies:

- **`primary` group** — 1 machine in `lhr`, `shared-cpu-2x` / 512 MB,
  persistent `litefs_data` volume mounted at `/var/lib/litefs`. Runs the
  sync worker, holds the LiteFS Consul lease, source of LiteFS HTTP
  replication.
- **`replica` group** — 7 machines (iad, nrt, syd, lax, jnb, sin, gru),
  `shared-cpu-1x` / 256 MB, **no persistent volume** (ephemeral rootfs).
  On boot, LiteFS cold-syncs the 88 MB database from the primary via
  HTTP. Typical hydration window is 5-45 seconds per region; `/readyz`
  returns 503 during this window so Fly Proxy routes around the machine
  until it is ready.

**Volume-only-on-primary contract:** `[[mounts]]` in `fly.toml` is
scoped to `processes = ["primary"]`. Only the LHR primary machine has a
mount. Replica machines are cattle — a damaged replica is recovered by
`fly machine destroy --force <id>`; the replacement machine that Fly
schedules has no volume concern, cold-syncs from the primary, and
becomes live when `/readyz` flips to 200.

**Replica cold-sync expectations:**

| Region | Expected hydration | Notes |
|--------|--------------------|-------|
| iad, lax | 5-15s | Low-latency path to LHR |
| nrt, sin | 15-30s | Transpacific |
| syd, gru, jnb | 30-45s | Furthest edges; long-haul to LHR |

If a replica stays on 503 >5 minutes with logs showing successful DB
pings, the `sync_status` row (replicated from the primary via LiteFS
cold-sync) is likely stale. Remediation: `POST /sync` with the
`PDBPLUS_SYNC_TOKEN` to force a fresh primary sync; replicas pick up
the updated `sync_status` within seconds via LTX replication.

**Sizing rationale:** Observed replica RSS is 58-59 MB steady-state;
`shared-cpu-1x` / 256 MB gives ~4× memory headroom and budget for
LiteFS LTX replay spikes. The primary keeps `shared-cpu-2x` / 512 MB —
it runs the sync worker whose memory profile was characterised in v1.13
and v1.14.

**Cost:** Asymmetric fleet is ~$20.75/mo vs the previous uniform
~$57.20/mo — saves ~$36/mo. Real win is operational simplicity (no
replica-volume orphans, destroy-and-recreate recovery in seconds).
```

**docs/ARCHITECTURE.md** — in the `## LiteFS primary/replica detection` section (starts line 346), add a short paragraph at the end of that section noting the process-group split. Content:

```markdown
### Fleet topology (v1.15+)

The app runs under two Fly process groups — `primary` (1 machine,
LHR, persistent volume) and `replica` (7 machines, other regions,
ephemeral rootfs). The process-group split reinforces but does not
replace the region-gated LiteFS candidacy: `litefs.yml`'s
`lease.candidate: ${FLY_REGION == PRIMARY_REGION}` remains the sole
source of truth for "which machine may become primary". The process
groups exist to scope `[[vm]]` sizing and `[[mounts]]` to the
primary-only tier. See `docs/DEPLOYMENT.md` § Asymmetric fleet for
operator runbook.
```

**CLAUDE.md** — in the `### Deployment` section (line 168), append these bullets (keep existing content):

```markdown
- **Asymmetric fleet (v1.15+).** Fly app `peeringdb-plus` runs two
  process groups: `primary` (1 machine, LHR, `shared-cpu-2x`/512 MB,
  persistent `litefs_data` volume) and `replica` (7 machines in other
  regions, `shared-cpu-1x`/256 MB, ephemeral rootfs). Replicas cold-sync
  the 88 MB DB from primary over LiteFS HTTP on boot (5-45s per region);
  `/readyz` fail-closes during hydration so Fly Proxy excludes them
  until ready. Replica recovery = destroy-and-recreate (no volume
  management). See `docs/DEPLOYMENT.md` § Asymmetric fleet.
- **Volume-only-on-primary.** `[[mounts]]` in `fly.toml` is scoped to
  `processes = ["primary"]`. Never re-introduce a mount on the replica
  group — the architecture assumes replicas are cattle.
```

**.planning/PROJECT.md** — at the end of the `## Key Decisions` table (currently ending at the Phase 63 schema hygiene row, around line 219), append a new row:

```markdown
| Phase 65 asymmetric Fly fleet: 1 primary (LHR, shared-cpu-2x/512MB, persistent volume) + 7 ephemeral replicas (shared-cpu-1x/256MB, cold-sync from primary) | Observed replica RSS 58-59 MB; 256 MB gives ~4× headroom. Splits VM sizing and mount policy via Fly process groups. `litefs.yml` region-gated candidacy unchanged — process groups reinforce the LHR-only primary invariant. Cost: $57.20/mo → $20.75/mo (~63% saving; real win is operational simplicity — no replica-volume orphans, destroy-and-recreate recovery). Big-bang rollout (CONTEXT D-01); rollback = revert fly.toml + redeploy. SEED-003 captures future primary-HA work. | ✓ Validated Phase 65 |
```

Style rules:
- Prose in the new DEPLOYMENT.md subsection wraps near column 72 to match the existing file style (visible in the LiteFS section).
- No emoji anywhere.
- Do not delete or re-flow existing sentences.
- Do not touch the `## Regional rollout` section even though the `fly scale count` examples are now stale under process groups — those examples still work when you add `--process-group=<name>`, and updating them cleanly is out of scope (the operator will learn the flag from Plan 02's runbook).
  </action>
  <verify>
    <automated>grep -q '## Asymmetric fleet' docs/DEPLOYMENT.md && grep -q 'Volume-only-on-primary' docs/DEPLOYMENT.md && grep -q 'Fleet topology (v1.15+)' docs/ARCHITECTURE.md && grep -q 'Asymmetric fleet (v1.15+)' CLAUDE.md && grep -q 'Phase 65 asymmetric Fly fleet' .planning/PROJECT.md</automated>
  </verify>
  <done>
All four docs contain the required new sections/bullets/rows. `git diff --stat` shows ≤ 4 files touched, no deletions of existing content. A reviewer reading DEPLOYMENT.md § Asymmetric fleet can determine (a) which VM sizes apply to which group, (b) why replicas have no volume, (c) the cold-sync timing expectation, and (d) the destroy-and-recreate recovery path.
  </done>
</task>

<task type="auto">
  <name>Task 3: Consume SEED-002 (move to consumed/)</name>
  <files>.planning/seeds/SEED-002-fly-asymmetric-fleet.md, .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md</files>
  <read_first>
    - .planning/seeds/SEED-002-fly-asymmetric-fleet.md (header — frontmatter `status` and `consumed_by` fields)
    - .planning/phases/65-asymmetric-fly-fleet/65-CONTEXT.md § Decisions D-11 (seed move is part of the doc sweep)
    - .planning/STATE.md § Seeds (confirms SEED-003 is already planted — no action needed)
  </read_first>
  <action>
Verify SEED-003 already exists at `.planning/seeds/SEED-003-primary-ha-hot-standby.md`. If it does, no additional seed-planting action is required (STATE.md line 89-90 confirms planting on 2026-04-17). If it is ABSENT for any reason, stop and escalate — the plan assumed it was planted.

Then move SEED-002:

```bash
mkdir -p .planning/seeds/consumed
git mv .planning/seeds/SEED-002-fly-asymmetric-fleet.md .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md
```

After the move, edit the frontmatter of `.planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md`:
- Change `status: activated` → `status: consumed`
- Add `consumed: 2026-04-18` below the existing `activated: 2026-04-17` line

Do not touch any other part of the seed file.

`ls .planning/seeds/` after this task MUST NOT include `SEED-002-...`. `ls .planning/seeds/consumed/` MUST include it.
  </action>
  <verify>
    <automated>test -f .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md && ! test -f .planning/seeds/SEED-002-fly-asymmetric-fleet.md && grep -q '^status: consumed$' .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md && grep -q '^consumed: 2026-04-18$' .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md && test -f .planning/seeds/SEED-003-primary-ha-hot-standby.md</automated>
  </verify>
  <done>
`.planning/seeds/SEED-002-...md` is gone; `.planning/seeds/consumed/SEED-002-...md` exists with `status: consumed` in frontmatter. `git status` shows a rename (R) entry, not a delete + add. SEED-003 is present (STATE.md invariant preserved).
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Repository file edits | Purely in-repo. No trust boundary crossed — this plan does not modify any runtime system, secret, or external API. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-65-01 | Information Disclosure | fly.toml `[http_service].processes` | accept | Explicitly lists both groups (`["primary", "replica"]`) matching today's implicit behavior. Never scope an admin-only service without `processes = [...]` filter in future edits — called out in CLAUDE.md. |
| T-65-02 | Tampering | litefs.yml | mitigate | Task 1 read_first lists litefs.yml as a sanity-check file; task action explicitly states "Do NOT touch litefs.yml". Verify command ensures the file was not modified (implicit — it is not in files_modified). |
| T-65-03 | Denial of Service | fly.toml misedit bricks deploy | mitigate | Task 1 verify includes 8 structural grep checks; optional `fly config validate` if flyctl is available. Plan 02 executor runs `fly deploy` which would surface any parse error before production impact. |
</threat_model>

<verification>
Plan-level gate (run after all 3 tasks):
1. `git status` — shows modifications only in fly.toml, docs/DEPLOYMENT.md, docs/ARCHITECTURE.md, CLAUDE.md, .planning/PROJECT.md, and the SEED-002 rename.
2. `go build ./...` — succeeds (defensive; fly.toml is not a Go input).
3. `go vet ./...` — succeeds.
4. `grep -E '^candidate:' litefs.yml` — still shows `candidate: ${FLY_REGION == PRIMARY_REGION}` (unchanged — this is the keystone).
5. Manual eyeball of `git diff fly.toml` confirms the target structure matches RESEARCH.md verbatim.
</verification>

<success_criteria>
- `fly.toml` has `[processes]` with primary + replica, two `[[vm]]` blocks scoped by process, `[[mounts]]` scoped to primary only.
- `docs/DEPLOYMENT.md` has an "Asymmetric fleet" subsection between `## LiteFS` and `## Regional rollout`.
- `docs/ARCHITECTURE.md` has a "Fleet topology (v1.15+)" subsection at the end of the LiteFS section.
- `CLAUDE.md` §Deployment has both the "Asymmetric fleet (v1.15+)" bullet and the "Volume-only-on-primary" bullet.
- `.planning/PROJECT.md` Key Decisions table has a Phase 65 row.
- `.planning/seeds/SEED-002-fly-asymmetric-fleet.md` is gone; `.planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md` exists with `status: consumed` + `consumed: 2026-04-18`.
- `litefs.yml` is unchanged.
- Go build + vet clean.
</success_criteria>

<output>
After completion, create `.planning/phases/65-asymmetric-fly-fleet/65-01-SUMMARY.md` covering: the 3 tasks completed, the exact diff surface area (files touched, lines added/removed), confirmation that litefs.yml was NOT modified, a pointer to Plan 02 (which runs the live migration), and a note that Plan 02 executors can now assume fly.toml is the next-deploy target.
</output>
