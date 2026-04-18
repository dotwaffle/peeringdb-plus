# Phase 65: Asymmetric Fly fleet — process groups + ephemeral replicas - Research

**Researched:** 2026-04-18
**Domain:** Fly.io fleet topology migration (process groups, per-group sizing, mount scoping, volume cleanup, LiteFS cold-sync semantics)
**Confidence:** HIGH (Fly.io process-group mechanics, mount scoping, scale commands — verified via Context7 + official docs) / MEDIUM (LiteFS 88 MB cold-sync wall-clock per region — first-principles estimate; official docs describe the protocol but not per-region timing)

## Summary

Phase 65 rebuilds the uniform 8-machine fleet (`peeringdb-plus`) into a `primary` (LHR, `shared-cpu-2x`/512 MB, persistent `litefs_data` volume) plus `replica` (7 regions, `shared-cpu-1x`/256 MB, ephemeral rootfs, LiteFS HTTP cold-sync from primary) split. All the config surfaces needed — `[processes]`, per-group `[[vm]]`, per-group `[[mounts]]`, `[http_service].processes` — are first-class in the current `fly.toml` schema `[VERIFIED: Fly.io reference configuration docs]`.

The keystone that makes zero-code-change migration work is `litefs.yml`'s `lease.candidate: ${FLY_REGION == PRIMARY_REGION}` — LiteFS candidate gating is already region-pinned to LHR, so splitting machines into process groups preserves the existing single-primary invariant. Replicas with empty `/var/lib/litefs` signal position `(0, 0)` to the primary over HTTP, the primary sends a current-state snapshot, and incremental LTX replication resumes `[CITED: LiteFS ARCHITECTURE.md]`. The FUSE mount does not serve reads before the database file is hydrated; `/readyz` fail-closes because it probes `DB.PingContext` and also requires a non-stale `sync_status` row `[VERIFIED: internal/health/handler.go:89-189]`. Traffic routes away from cold replicas automatically.

The migration path has one non-obvious trap: `fly deploy` on a fly.toml that adds `[processes]` will **destroy existing machines that belong to undefined process groups**. The standard mitigation is to either (a) pre-assign existing machines to the new groups via `fly machine update --metadata fly_process_group=<group> <id>` before deploying, or (b) include an `app` group in fly.toml as a transitional bridge `[CITED: fly.io/docs/launch/processes/]`. For this phase, (a) is the correct path — the 8 existing `app`-group machines get metadata-retagged into `primary` (LHR) + `replica` (the other 7), then `fly deploy` picks up the new fly.toml cleanly.

**Primary recommendation:** Stage the migration as: (1) metadata-retag LHR machine into `primary`, retag the other 7 into `replica`; (2) edit fly.toml with the new process groups / mount scoping / VM sizing; (3) `fly deploy` (rolls out per-group config, replica machines destroy-and-recreate because their `[[vm]]` size changed and their mount disappeared — LiteFS hydrates the fresh rootfs); (4) confirm 15-min health window; (5) enumerate and destroy the 7 now-orphaned replica volumes via `fly volumes list --json` + `fly volumes destroy -y`. Rollback at any step by reverting fly.toml and re-running `fly scale count` at the old size.

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| Fleet topology definition | fly.toml (config) | — | Process groups, per-group VM sizing, and mount scoping are purely declarative in fly.toml |
| Primary election | LiteFS + Consul | `litefs.yml` | `lease.candidate` region gate already enforces "LHR only" — process groups reinforce but don't create the invariant |
| Replica hydration | LiteFS HTTP replication | FUSE mount | Cold-sync is LiteFS's job; app code doesn't observe it (only observes `DB.Ping` failure until complete) |
| Traffic routing during hydration | Fly Proxy + `/readyz` | `internal/health` handler | `/readyz` returns 503 until DB pings AND sync_status is fresh; Fly Proxy excludes 503 machines from routing |
| Write-path routing | `fly-replay` header | app | Unchanged from today — `fly-replay: region=lhr` works identically across process groups within one app |
| Volume lifecycle | fly CLI (`fly volumes destroy`) | operator runbook in SUMMARY.md | One-shot migration step; not reusable script per D-06 |
| Observability during migration | `fly status`, `curl /readyz`, existing OTel dashboard | operator | 15-min smoke window; no new instrumentation needed |

## Standard Stack

### Core

| Component | Version | Purpose | Why Standard |
|-----------|---------|---------|--------------|
| `flyctl` | latest | CLI for Fly.io platform ops | Only supported interface for deploy/scale/volumes |
| Fly.io process groups | platform feature | Per-workload sizing + scoping | Native Fly primitive for multi-process apps `[VERIFIED: fly.io/docs/launch/processes/]` |
| LiteFS | 0.5 | SQLite replication | Existing; `flyio/litefs:0.5` pinned in `Dockerfile.prod:20` |
| Consul | managed (Fly) | LiteFS lease election | Existing; `[consul].enable = true` in fly.toml |

No new packages, libraries, binaries, or Dockerfile changes required for Phase 65. The sqlite3 CLI addition (quick task `260418-1cn`, commit `4dfc52a`) was pre-requisite prep and is already shipped `[VERIFIED: Dockerfile.prod:17]`.

### Supporting

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `jq` | JSON parsing of `fly volumes list --json` output | Volume cleanup runbook |
| `curl` | Per-region `/readyz` smoke test | 15-min monitoring window |
| `fly status --json` | Machine inventory snapshot | Pre/post migration diff |

### Alternatives Considered

| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| `fly machine update --metadata fly_process_group=X` pre-tag | `fly deploy` alone with fly.toml change | **Rejected.** `fly deploy` destroys machines belonging to process groups not in fly.toml. Pre-tagging preserves the LHR machine's existing volume attachment and avoids an accidental volume orphan. `[CITED: fly.io/docs/launch/processes/]` |
| Big-bang scale change | Canary: one replica first, observe, then the rest | **Rejected per CONTEXT D-01.** User explicitly chose big bang; rollback is `git revert fly.toml && fly deploy`. |
| Keep replica volumes "just in case" | Destroy inline in SUMMARY | **Rejected per CONTEXT D-06/D-07.** Orphan volumes still bill at $0.15/GB/mo × 7 = $1.05/mo. Inline destroy is cleaner. |

**Installation:**

No install step needed. `flyctl` is the operator's existing tool; `jq` is assumed present in the operator's shell environment.

**Version verification:** `flyctl` version should be >= 0.2 (any recent 2024+ version supports `--json` on volumes-list and `--metadata` on machine-update). Run `fly version` before starting migration.

## Architecture Patterns

### System Architecture Diagram

```
BEFORE (uniform fleet):
┌───────────────────────────────────────────────────────────────┐
│  peeringdb-plus (Fly app)                                     │
│                                                               │
│  [processes]: implicit "app" group, 8 machines identical     │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐                  │
│  │  LHR   │ │  IAD   │ │  NRT   │ │  SYD   │  ... 4 more     │
│  │ 2x/512 │ │ 2x/512 │ │ 2x/512 │ │ 2x/512 │                  │
│  │ vol 1G │ │ vol 1G │ │ vol 1G │ │ vol 1G │                  │
│  │primary │ │replica │ │replica │ │replica │                  │
│  │(LiteFS)│ │(LiteFS)│ │(LiteFS)│ │(LiteFS)│                  │
│  └────────┘ └────────┘ └────────┘ └────────┘                  │
│                                                               │
│  LiteFS candidacy: FLY_REGION == "lhr"  (region-gated)       │
│  Consul lease held by LHR                                     │
│  Reads: local SQLite on each machine                          │
│  Writes: fly-replay header → LHR                              │
└───────────────────────────────────────────────────────────────┘

AFTER (asymmetric fleet):
┌───────────────────────────────────────────────────────────────┐
│  peeringdb-plus (Fly app, same app, two process groups)      │
│                                                               │
│  [processes]                                                  │
│    primary = "litefs mount"                                   │
│    replica = "litefs mount"                                   │
│                                                               │
│  primary group (1 machine):                                   │
│  ┌─────────────────────┐                                      │
│  │  LHR                │  shared-cpu-2x / 512 MB              │
│  │  volume "litefs_data" (1 GB, persistent)                   │
│  │  LiteFS primary (Consul lease holder, region-gated)        │
│  │  Runs sync worker on its cadence                           │
│  └─────────────────────┘                                      │
│           │                                                   │
│           │ LiteFS HTTP replication (LTX stream + snapshots)  │
│           │ /var/lib/litefs is SOURCE of replication          │
│           ▼                                                   │
│  replica group (7 machines):                                  │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐                  │
│  │  IAD   │ │  NRT   │ │  SYD   │ │  LAX   │ + jnb, sin, gru │
│  │ 1x/256 │ │ 1x/256 │ │ 1x/256 │ │ 1x/256 │                  │
│  │  EPH   │ │  EPH   │ │  EPH   │ │  EPH   │  (no volume)    │
│  │ LiteFS │ │ LiteFS │ │ LiteFS │ │ LiteFS │                  │
│  └────────┘ └────────┘ └────────┘ └────────┘                  │
│                                                               │
│  On boot: empty /var/lib/litefs → HTTP snapshot from LHR      │
│  /readyz fail-closes until DB ready → Fly Proxy bypasses      │
│  Cattle semantics: destroy-and-recreate = full recovery       │
└───────────────────────────────────────────────────────────────┘
```

### Target fly.toml (complete, ready to commit)

This is the exact target file. Every field is present in the current fly.toml; the diff is: add `[processes]`, scope `[[vm]]` + `[[mounts]]` + `[http_service]` via `processes = [...]`, add a second `[[vm]]` block for the replica group.

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

**Empty command strings in `[processes]`:** Fly.io accepts an empty process command when the image's Docker ENTRYPOINT provides the runtime `[CITED: fly.io/docs/launch/processes/]`. Our `Dockerfile.prod:35` sets `ENTRYPOINT ["litefs", "mount"]`, which runs the same way in both groups. If the empty string is rejected by `flyctl`, substitute `litefs mount` literally — the effect is identical.

**One open question:** whether `[[restart]]` policies should be split per group. Today there is no `[[restart]]` block — Fly defaults apply. Recommend **not adding** `[[restart]]` in Phase 65 — zero behavioral change from today, no unknowns introduced.

### Pattern 1: Metadata-retag existing machines BEFORE changing fly.toml

**What:** Use `fly machine update --metadata fly_process_group=<group> <machine_id>` to pre-assign existing machines to their target process group, then deploy.

**Why:** `fly deploy` with a new `[processes]` block "creates at least one Machine for each process group, and destroys all the Machines that belong to any process group that isn't defined" `[CITED: fly.io/docs/launch/processes/]`. Today's 8 machines are in the implicit `app` group. If we deploy a fly.toml with only `primary` and `replica`, every existing machine gets destroyed — including the LHR primary with its attached volume. Pre-tagging means Fly recognises the existing machines as belonging to the new groups.

**When to use:** Before the first `fly deploy` of the new fly.toml. Do it in order: LHR first (so the primary keeps its volume contiguously), then the 7 others.

**Example:**

```bash
# 1. Snapshot current state
fly status --app peeringdb-plus --json > /tmp/phase65-pre-status.json
fly machine list --app peeringdb-plus --json > /tmp/phase65-pre-machines.json

# 2. Identify the LHR machine (the primary) vs replicas
LHR_ID=$(jq -r '.[] | select(.region == "lhr") | .id' /tmp/phase65-pre-machines.json)
REPLICA_IDS=$(jq -r '.[] | select(.region != "lhr") | .id' /tmp/phase65-pre-machines.json)

# 3. Retag LHR as primary
fly machine update --metadata fly_process_group=primary "$LHR_ID" --app peeringdb-plus --yes

# 4. Retag the 7 others as replica
for id in $REPLICA_IDS; do
  fly machine update --metadata fly_process_group=replica "$id" --app peeringdb-plus --yes
done

# 5. Verify tagging
fly machine list --app peeringdb-plus --json | jq -r '.[] | "\(.region)\t\(.id)\t\(.config.metadata.fly_process_group)"'
```

**Expected output:** 1 row `lhr\t<id>\tprimary`, 7 rows `<region>\t<id>\treplica`. No row should show `app` or empty group.

### Pattern 2: LiteFS cold-sync on ephemeral rootfs

**What:** When a replica machine boots with empty `/var/lib/litefs`, LiteFS connects to the primary over HTTP, signals position `(0, 0)` (transaction ID + rolling checksum), the primary detects no matching history and responds with a full-database snapshot, and the replica populates `/var/lib/litefs` + exposes the hydrated DB on the FUSE mount `/litefs/peeringdb-plus.db` `[CITED: LiteFS ARCHITECTURE.md + fly.io/docs/litefs/how-it-works/]`.

**During hydration:** The FUSE mount exists but the database file is not readable in its final state. `sqlite3 /litefs/peeringdb-plus.db` queries error out or block. The app's `DB.PingContext` fails → `/readyz` returns 503 → Fly Proxy routes around the machine.

**Timing:** No official per-region SLA. Snapshot transfer is a single HTTP stream; 88 MB intra-Fly egress at the observed ~50-200 Mbps inter-region is **5-15 seconds most regions; up to 30-45 seconds for SYD/GRU to LHR worst case** `[ASSUMED]` — this matches the CONTEXT D-10 "5-45s" estimate but is not empirically verified. Monitoring: watch LiteFS stdout for `stream connected` and `stream disconnected` events; expected state transition per replica during first post-migration boot: `starting` → `db ping failing` (hydrating) → `db ping ok` → `sync_status fresh` → `/readyz 200`.

**When to use:** Automatic on every replica cold start. No operator action. No code change. Zero config change from today's `litefs.yml`.

### Pattern 3: `/readyz` fail-closed during hydration

**What:** `/readyz` returns 503 until two conditions hold: (1) `sql.DB.PingContext` succeeds (= LiteFS DB file is readable) AND (2) the `sync_status` table has a row with `status = "success"` and `last_sync_at` within `PDBPLUS_SYNC_STALE_THRESHOLD` (24h default) `[VERIFIED: internal/health/handler.go:89-189]`.

**Why this matters for ephemeral replicas:** Replicas don't run the sync worker (gated on `IsPrimary()` — returns false on replicas). They depend on the `sync_status` table being populated by the **primary** and replicated via LiteFS LTX stream. After cold-sync completes, the replica gets the primary's last `sync_status` row via the same snapshot. So `/readyz 200` on a cold replica requires: cold-sync done + primary has had at least one successful sync within 24h.

**Implication:** If the migration happens during or within 24h of a primary sync hiccup, replicas may boot into a "hydrated but sync_status stale" state → `/readyz 503` until the next successful sync. Check `GET https://peeringdb-plus.fly.dev/readyz` from LHR region before starting migration to confirm baseline health.

### Anti-Patterns to Avoid

- **Don't `fly deploy` before metadata-retagging.** Fly will destroy all 8 machines because they're in the implicit `app` group, not `primary` or `replica`. This includes the LHR machine holding the attached `litefs_data` volume. The volume itself persists (volumes outlive machines) but recovery is messier than needed.
- **Don't scale replica=0 then back up as a "reset".** `fly scale count replica=0` destroys the 7 machines but leaves volumes. Scaling back up creates new machines that won't get volumes (because `[[mounts]]` is scoped to `primary` only), but the old 7 volumes become billed orphans until manually destroyed. Just let the rolling deploy rebuild replicas in place.
- **Don't run `fly volumes destroy` while machines are still attached.** `fly volumes destroy` fails when the volume is attached `[CITED: community.fly.io]`. The replica machines must be destroyed (via scale or deploy-with-size-change) first; then `fly volumes list` shows the 7 replica volumes as unattached; then they can be destroyed.
- **Don't regenerate `litefs.yml`.** Per CONTEXT D-09, `litefs.yml` is unchanged. Modifying it (even whitespace) risks invalidating the region-gated candidacy.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Per-group VM sizing | A script that `fly machine update --vm-size` in a loop post-deploy | Native `[[vm]] processes = [...]` in fly.toml | Fly's declarative config handles rollback, drift detection, and the entire rolling deploy in one primitive |
| Replica volume cleanup | A cron/script that auto-destroys detached volumes | Inline runbook in `65-02-SUMMARY.md` (per D-06) | One-shot migration. Persistent automation risks destroying recoverable volumes in future incidents. |
| Primary election | A custom health-check that triggers failover | LiteFS + Consul (existing) | Already solved. Region-gated `lease.candidate` makes this trivial. |
| Cold-sync / replica hydration | Bundle a `peeringdb-plus.db` snapshot into the image | LiteFS HTTP replication on boot | Snapshotting into image means stale data on first boot; LiteFS gives fresh DB every time, zero config. |
| Write routing | Per-region HTTP middleware that proxies POST to LHR | Existing `fly-replay: region=lhr` header | Already in place. Process groups don't affect it. |

**Key insight:** Fly.io's process groups + per-group `[[vm]]` / `[[mounts]]` / `[http_service].processes` is a complete declarative primitive for asymmetric fleets. No custom automation needed — even the migration itself is 5 shell commands plus a `fly deploy`.

## Runtime State Inventory

Phase 65 is a rename-adjacent migration (process-group retagging + mount scope change + VM size change). Audit each state category:

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| **Stored data** | `litefs_data` volume on LHR (1 GB, ~88 MB used). 7 replica volumes (each 1 GB, currently attached). `sync_status` table in SQLite (replicated by LiteFS). | **Primary volume:** keep, mount scope changed to `processes = ["primary"]` in fly.toml. **Replica volumes:** destroy via runbook after machines recreate without mounts. **`sync_status`:** unchanged; replicates to fresh replica rootfs automatically. |
| **Live service config** | Fly.io per-app config (secrets, IP addresses, primary_region). Consul KV at `litefs/peeringdb-plus` (LiteFS lease state). | **Fly config:** no secrets touched. Primary region stays `lhr`. **Consul KV:** ephemeral; Consul holds current lease holder (LHR machine ID) — will self-update when LHR machine is rolled by `fly deploy` and Consul re-elects to the new LHR machine. No manual action. |
| **OS-registered state** | None. LiteFS runs as FUSE within the container; no systemd / launchd / Task Scheduler involvement. | None. |
| **Secrets and env vars** | `PDBPLUS_IS_PRIMARY` env var — only consulted by `internal/litefs.IsPrimaryWithFallback` when `/litefs/.primary` file is absent AND the `/litefs` directory doesn't exist (i.e., LiteFS not running — local dev only) `[VERIFIED: internal/litefs/primary.go:48-75]`. In prod on Fly, LiteFS is always running, so this env var is **never consulted on Fly**. | None — no change needed. Keep default (unset). |
| **Build artifacts / installed packages** | Docker image already contains `litefs` binary (0.5) and `sqlite3` (added in quick task `260418-1cn`, commit `4dfc52a`). | None — image is Phase-65-ready. |

**Critical observation:** The `[[mounts]]` scope change is the sole reason the 7 replica volumes get detached. When `fly deploy` rolls the replica group's machines with the new fly.toml, each replacement machine is created WITHOUT a mount (because `processes = ["primary"]` on the mount). The old machines are destroyed as part of the roll, which releases the volume attachments. The volumes then show as unattached in `fly volumes list`.

## Common Pitfalls

### Pitfall 1: `fly deploy` destroys existing machines when [processes] is introduced for the first time

**What goes wrong:** Adding `[processes]` to a fly.toml that previously had none, then running `fly deploy`, destroys all existing machines. Fly treats them as members of a process group (`app`, the implicit default) that's no longer defined.

**Why it happens:** Fly's deploy logic: "creates at least one Machine for each process group, and destroys all the Machines that belong to any process group that isn't defined in your app's fly.toml" `[CITED: fly.io/docs/launch/processes/]`.

**How to avoid:** Pre-tag every existing machine with `fly machine update --metadata fly_process_group=<target>` BEFORE the first `fly deploy`. See Pattern 1 above.

**Warning signs:** `fly deploy` output that says `Destroying machine <id>` for machines you didn't expect to touch. Abort immediately (Ctrl-C may not save you — deploy is mostly atomic per-machine).

### Pitfall 2: Volumes persist after scale-down (billing leak)

**What goes wrong:** `fly scale count replica=0` or destroying replica machines leaves their volumes behind. Volumes bill at $0.15/GB/mo each → $1.05/mo for 7 volumes of 1 GB.

**Why it happens:** Fly's design: volumes outlive machines intentionally, to support destroy-and-recreate cycles that preserve data. `[CITED: fly.io/docs/volumes/volume-manage/]`.

**How to avoid:** The volume-cleanup runbook (see below). Always enumerate with `fly volumes list --json` after machine destruction, confirm unattached state, then `fly volumes destroy -y` per ID.

**Warning signs:** `fly volumes list` still showing 8 volumes (1 LHR + 7 other regions) after the migration. If any volume is listed with `attached_machine_id` populated, the cleanup step hasn't run yet or a replica machine was recreated with an unintended mount.

### Pitfall 3: `/readyz` false-negative during cold-sync

**What goes wrong:** A fresh replica boots, LiteFS hydrates the DB, but `/readyz` returns 503 because the `sync_status` table arrives *in* the cold-sync snapshot and its timestamp may be >24h old if the primary had a sync hiccup pre-migration.

**Why it happens:** `/readyz` requires a successful sync within `PDBPLUS_SYNC_STALE_THRESHOLD` (24h default). Replicas don't sync themselves — they inherit sync_status via LiteFS replication from primary.

**How to avoid:** Verify `curl https://peeringdb-plus.fly.dev/readyz` returns 200 from LHR before starting migration. Verify the primary has had a successful sync within the last hour (not 23h ago) via `curl https://peeringdb-plus.fly.dev/ui/about`. If sync is recent, cold-sync'd replicas will also pass `/readyz` immediately after hydration.

**Warning signs:** After migration, replicas stay on 503 for >5 minutes with logs showing successful DB pings — indicates the sync_status replicated but is stale. Trigger a manual sync via `POST /sync` with `PDBPLUS_SYNC_TOKEN` to unstick.

### Pitfall 4: Changing VM size forces machine destroy-and-recreate (no in-place resize)

**What goes wrong:** Operator expects `fly deploy` to in-place resize the 7 replicas from shared-cpu-2x/512 MB to shared-cpu-1x/256 MB. Instead, Fly destroys each machine and creates a new one with the new size.

**Why it happens:** Fly's VM sizing is set at machine creation; changing `[[vm]].size` or `[[vm]].memory` forces recreation `[VERIFIED: fly.io/docs/reference/configuration/]`.

**How to avoid:** **Embrace it.** The destroy-and-recreate cycle is exactly what we want for the replica group — it's how the ephemeral model takes effect (new machine → no mount → LiteFS cold-syncs to rootfs). The primary is unaffected because its `[[vm]]` size stays shared-cpu-2x/512 MB (no change).

**Warning signs:** None — this is expected behavior. Factored into the rollout plan.

### Pitfall 5: `fly deploy` with `max_unavailable = 0.5` rolls 4 replicas simultaneously → LHR primary egress burst

**What goes wrong:** During the deploy, 4 replicas boot at once and each cold-syncs 88 MB from LHR in parallel → ~350 MB burst egress from the primary over ~30-60 seconds.

**Why it happens:** `max_unavailable = 0.5` = allow up to 50% of machines to be unavailable during rolling deploy. 8 × 0.5 = 4 parallel rebuilds. Intra-Fly private-network egress is free but the LHR machine's outbound bandwidth + LiteFS's serving capacity is finite.

**How to avoid:** Acceptable per CONTEXT D-01 risk table. `shared-cpu-2x` has ~1 Gbps, 88 MB × 4 in ~30s = ~94 MB/s sustained → feasible. If we see problems, set `max_unavailable = 0.25` for this one deploy (2 parallel rebuilds) via a one-shot fly.toml edit.

**Warning signs:** OTel primary-machine network metrics showing saturation, or LiteFS logs on primary reporting backpressure. Roll back to `max_unavailable = 0.25` and redeploy.

## Code Examples

### Exact migration command sequence (ready to execute)

The canonical sequence. Every step is idempotent enough to re-run if it fails mid-way.

```bash
# ============================================================
# Phase 65 Migration: uniform 8 → asymmetric 1+7
# Prereq: sqlite3 added to Dockerfile.prod (done, commit 4dfc52a)
# Prereq: flyctl version >= 0.2 (verify: fly version)
# Prereq: /readyz 200 from LHR (verify: curl https://peeringdb-plus.fly.dev/readyz)
# ============================================================

# ---- Step 0: capture pre-state ----
APP="peeringdb-plus"
mkdir -p /tmp/phase65
fly status --app "$APP" --json > /tmp/phase65/pre-status.json
fly machine list --app "$APP" --json > /tmp/phase65/pre-machines.json
fly volumes list --app "$APP" --json > /tmp/phase65/pre-volumes.json

# Sanity: expect 8 machines, 8 volumes
jq 'length' /tmp/phase65/pre-machines.json   # should print 8
jq 'length' /tmp/phase65/pre-volumes.json    # should print 8

# ---- Step 1: identify LHR vs replica machines ----
LHR_MACHINE_ID=$(jq -r '.[] | select(.region == "lhr") | .id' /tmp/phase65/pre-machines.json)
REPLICA_MACHINE_IDS=$(jq -r '.[] | select(.region != "lhr") | .id' /tmp/phase65/pre-machines.json | tr '\n' ' ')

echo "LHR (primary): $LHR_MACHINE_ID"
echo "Replicas: $REPLICA_MACHINE_IDS"
# Expect 1 LHR and 7 others (iad nrt syd lax jnb sin gru)

# ---- Step 2: metadata-retag existing machines ----
# (preserves LHR volume attachment; prevents fly deploy from destroying machines)
fly machine update --metadata fly_process_group=primary "$LHR_MACHINE_ID" --app "$APP" --yes

for id in $REPLICA_MACHINE_IDS; do
  echo "Retagging $id → replica"
  fly machine update --metadata fly_process_group=replica "$id" --app "$APP" --yes
done

# Verify
fly machine list --app "$APP" --json \
  | jq -r '.[] | "\(.region)\t\(.id)\t\(.config.metadata.fly_process_group // "UNTAGGED")"'
# Expect: 1 line lhr ... primary
#         7 lines <region> ... replica
#         0 lines UNTAGGED

# ---- Step 3: commit the new fly.toml ----
# (Edit fly.toml to the target shown in "Target fly.toml" above.)
# Commit with message: docs(infra): Phase 65 fly.toml — asymmetric fleet process groups
git add fly.toml
git commit -m "feat(infra): Phase 65 — asymmetric fly fleet (process groups)"

# ---- Step 4: deploy ----
# This rolls out per-group. Expected:
#   - LHR primary: image refresh + maybe restart, but keeps volume (mount scope unchanged for it)
#   - 7 replicas: destroy-and-recreate with shared-cpu-1x/256 MB and no mount
# 4-at-a-time via max_unavailable = 0.5
fly deploy --app "$APP"

# Expected wall-clock: ~5-8 minutes for full roll (4 parallel cohorts)
# Watch for: "Destroying machine <id>" for replica machines — expected
# Watch for: "Updating machine <LHR_ID>" for primary — expected (not destroyed)
# Abort signal: "Destroying machine <LHR_ID>" — pre-tag must have failed, revert fly.toml

# ---- Step 5: verify post-deploy state ----
fly status --app "$APP"
# Expect: 1 primary machine in lhr, 7 replica machines in the other regions
# All should be state=started

# Per-region readiness smoke test (see "Observability checklist" below)
for region in lhr iad nrt syd lax jnb sin gru; do
  code=$(curl -sS -o /dev/null -w "%{http_code}" \
    -H "Fly-Prefer-Region: $region" \
    -H "User-Agent: Mozilla/5.0" \
    https://peeringdb-plus.fly.dev/readyz)
  echo "$region: $code"
done
# Expect: all 200 within ~1 minute of deploy complete (cold-sync window closes)

# ---- Step 6: volume cleanup (see dedicated runbook below) ----
# Only after 15-min monitoring window passes cleanly.

# ---- Step 7: post-state snapshot for audit trail ----
fly status --app "$APP" --json > /tmp/phase65/post-status.json
fly machine list --app "$APP" --json > /tmp/phase65/post-machines.json
fly volumes list --app "$APP" --json > /tmp/phase65/post-volumes.json
```

### Volume cleanup runbook (with check-before-destroy safeguard)

```bash
# ============================================================
# Volume cleanup: destroy 7 replica volumes, keep LHR primary volume
# Run ONLY after the 15-min monitoring window passes and /readyz is green
# on all 8 machines.
# ============================================================

APP="peeringdb-plus"

# ---- Step 1: enumerate and confirm state ----
fly volumes list --app "$APP" --json > /tmp/phase65/volumes-before-cleanup.json

# Show all volumes with region and attached machine (if any)
jq -r '.[] | "\(.region)\t\(.id)\t\(.name)\tattached=\(.attached_machine_id // "none")"' \
  /tmp/phase65/volumes-before-cleanup.json

# Expected shape after post-deploy stabilisation:
#   lhr    vol_xxx    litefs_data    attached=<LHR machine id>
#   iad    vol_yyy    litefs_data    attached=none
#   nrt    vol_zzz    litefs_data    attached=none
#   syd    vol_aaa    litefs_data    attached=none
#   lax    vol_bbb    litefs_data    attached=none
#   jnb    vol_ccc    litefs_data    attached=none
#   sin    vol_ddd    litefs_data    attached=none
#   gru    vol_eee    litefs_data    attached=none

# ---- Step 2: build destroy list with safeguards ----
# SAFEGUARD 1: Only select volumes where region != lhr
# SAFEGUARD 2: Only select volumes where attached_machine_id is null/empty
# SAFEGUARD 3: Print for human review before proceeding

DESTROY_IDS=$(jq -r '
  .[]
  | select(.region != "lhr")
  | select((.attached_machine_id // "") == "")
  | .id
' /tmp/phase65/volumes-before-cleanup.json)

echo "Volumes that will be destroyed:"
for id in $DESTROY_IDS; do
  echo "  $id"
done

COUNT=$(echo "$DESTROY_IDS" | grep -c . || echo 0)
if [[ "$COUNT" != "7" ]]; then
  echo "ABORT: Expected exactly 7 replica volumes to destroy, got $COUNT."
  echo "Something unexpected in the post-deploy state. Investigate before proceeding."
  exit 1
fi

# ---- Step 3: destroy (with explicit confirmation) ----
# Use --yes to skip the per-volume confirmation prompt (we've already confirmed above).
for id in $DESTROY_IDS; do
  echo "Destroying $id..."
  fly volumes destroy "$id" --app "$APP" --yes
done

# ---- Step 4: verify final state ----
fly volumes list --app "$APP" --json > /tmp/phase65/volumes-after-cleanup.json
REMAINING=$(jq 'length' /tmp/phase65/volumes-after-cleanup.json)
LHR_STILL_THERE=$(jq -r '.[] | select(.region == "lhr") | .id' /tmp/phase65/volumes-after-cleanup.json)

if [[ "$REMAINING" == "1" && -n "$LHR_STILL_THERE" ]]; then
  echo "Cleanup success: 1 volume remaining ($LHR_STILL_THERE in lhr)."
else
  echo "UNEXPECTED: $REMAINING volumes remain. Manual audit required."
  jq -r '.[] | "\(.region)\t\(.id)\tattached=\(.attached_machine_id // "none")"' \
    /tmp/phase65/volumes-after-cleanup.json
fi
```

### Rollback runbook

Scenarios ordered by worsening severity:

**R1. 15-min monitoring window detects degraded service (high p99 or >1 region returning 5xx) — before volume cleanup:**

```bash
# Revert fly.toml and redeploy — restores uniform 8-machine topology.
git revert HEAD  # reverts the "Phase 65 fly.toml" commit
fly deploy --app peeringdb-plus

# This triggers another destroy-and-recreate for the 7 replicas, but this time:
#   - fly.toml has no [processes] so machines belong to the implicit "app" group
#   - [[mounts]] is not scoped, so every machine gets a litefs_data volume
#   - [[vm]] is single, so every machine is shared-cpu-2x/512 MB
# Result: 8 uniform machines, each with a mounted volume.
# 
# CRITICAL: the 7 existing-but-unattached replica volumes get picked up and
# reattached per fly.io volume lifecycle. Do NOT run volume cleanup until AFTER
# the 15-min window passes cleanly. If cleanup already ran, new volumes are
# auto-created by Fly during the deploy (each 1 GB, same name, new IDs).

# Post-rollback: verify
fly status --app peeringdb-plus
# Expect 8 machines, all shared-cpu-2x/512 MB, all with volumes
```

**R2. Degraded service AFTER volume cleanup (7 volumes destroyed):**

Same as R1 — `fly deploy` with reverted fly.toml will provision fresh 1 GB volumes for the 7 replica regions (Fly auto-creates volumes matching `[[mounts]]` names when they don't exist for a machine). Replicas cold-sync from LHR to populate the fresh volumes; same hydration window as during forward migration. Net-net: identical state to pre-Phase-65 within ~10 minutes.

**R3. LHR primary loses volume (catastrophic):**

Extremely unlikely — LHR machine is never destroyed during forward migration (metadata is tagged as `primary` first, so `fly deploy` sees it as belonging to the `primary` group). But if it happens:

```bash
# Option A: recover from LiteFS snapshot retention (if within 10m retention window):
#   - Fly-side: provision a new LHR machine. LiteFS will re-elect (still only
#     LHR candidate). New primary has no data, will fetch from any replica
#     that's still hot (each has the full DB in ephemeral rootfs).
#   - This is the "promote replica to primary" path — works unchanged from today.
#
# Option B: re-sync from PeeringDB (nuclear):
#   - New LHR machine boots with empty DB, LiteFS hydrates from... nowhere
#     (if replicas were destroyed too). Trigger `fly ssh console` on the new
#     primary, exec `curl -X POST http://localhost:8080/sync -H
#     "X-Sync-Token: $PDBPLUS_SYNC_TOKEN"` to force a fresh full sync from
#     https://api.peeringdb.com. Takes 5-20 minutes depending on API throughput.

# In either case: restore the fly.toml and fly deploy, then let self-healing do its thing.
```

**R4. Partial rollout failure (3 of 7 replicas broken):**

Most likely cause: LiteFS cold-sync failing for a region due to transient Fly network issue. Try restarting the affected machines:

```bash
fly machine list --app peeringdb-plus --json \
  | jq -r '.[] | select(.state != "started") | .id'
# For each bad machine:
fly machine restart <id> --app peeringdb-plus
# If still failing:
fly machine destroy --force <id> --app peeringdb-plus
# Fly auto-creates a replacement (replica group has 7 target count).
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Uniform fleet, all machines sized for primary workload | Asymmetric fleet, primary is a pet, replicas are cattle | Phase 65 (this phase) | -$36/mo, -63% cost, simpler ops |
| Replica volumes persist across deploys | Replica rootfs is ephemeral, cold-sync on boot | Phase 65 | Faster recovery (destroy-and-recreate in seconds), no orphan-volume cleanup toil |
| Fly Machines v1 (pre-2023) with `fly scale vm` | Fly Apps v2 + `[[vm]]` in fly.toml | Already done pre-v1.0 | Declarative, atomic |
| LiteFS Cloud (managed) | Self-hosted LiteFS + Consul on Fly | Already done — LiteFS Cloud deprecated | No change here; noted because LiteFS is maintenance-mode per CLAUDE.md |

**Deprecated/outdated:**

- **`fly regions add/remove`** was the old way to change region topology. Superseded by `fly scale count --region` and per-group scaling `[CITED: fly.io/docs/launch/scale-count/]`. Don't use `fly regions` in this migration.
- **`[experimental]` section in fly.toml** — not needed; process groups are GA.

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | 88 MB cold-sync completes in 5-45s per region | Pattern 2, Pitfall 5 | If slower (e.g., 2 min from SYD to LHR), `/readyz` stays 503 longer → Fly Proxy routes away longer → user-perceived "down" in that region for longer. Mitigation: `[[http_service.checks]].grace_period = "30s"` already gives slack; if needed, bump to 60s. |
| A2 | Empty command strings in `[processes]` are accepted when Docker ENTRYPOINT provides the runtime | Target fly.toml | If `flyctl` rejects empty strings, substitute `"litefs mount"` literally in both entries. Behavior identical. |
| A3 | `shared-cpu-1x`/256 MB can handle replica read load (soft_limit 10 req/machine) | Replica sizing (D-04) | Risk: p99 regression. Mitigation: one-line fly.toml bump to shared-cpu-2x/512 MB + redeploy. SEED-002 risk table already accepts this. |
| A4 | `sync_status` row successfully replicates through LiteFS cold-sync snapshot | Pattern 3 | If not: replicas permanently 503 until next primary sync. Mitigation: trigger manual sync via POST /sync after deploy. |
| A5 | `fly machine update --metadata fly_process_group=X` preserves existing volume attachment on the updated machine | Pattern 1 | If metadata update detaches volume: abort before deploying, investigate. Very unlikely — metadata is cosmetic. |
| A6 | `max_unavailable = 0.5` during deploy produces survivable LHR egress burst | Pitfall 5 | If it saturates: reduce to 0.25 for this deploy only. Fly supports per-deploy override via `fly deploy --max-unavailable 0.25`. |
| A7 | LiteFS detects empty `/var/lib/litefs` on replica boot and auto-requests snapshot | Pattern 2 | If LiteFS requires a bootstrap step: `litefs-example` repo shows this works out of the box when `candidate: false` on the node. Already verified by CONTEXT D-09. |

**Risk profile:** Most assumptions are low-risk, binary (works/doesn't), and have cheap mitigations. The highest-impact unknown is **A1** (cold-sync wall-clock per region) — empirically observable in the 15-min monitoring window; no way to pre-verify without a test replica which defeats the big-bang choice. Accept the risk per CONTEXT D-01.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| `flyctl` | All migration steps | ✓ (operator machine) | >= 0.2 | None — blocking if absent |
| `jq` | JSON parsing in runbook | ✓ (operator machine, standard POSIX tool) | any | Hand-parse volume list (tedious) |
| `curl` | Per-region readiness smoke test | ✓ | any | `wget` |
| `git` | Commit fly.toml change | ✓ | any | None — blocking if absent |
| `fly.io platform` (Consul, LiteFS support) | Fleet operation | ✓ (existing app) | — | None — platform is canonical |
| `sqlite3` in prod image | Incident response during migration | ✓ | Chainguard static | Pre-shipped in quick task `260418-1cn` |
| LiteFS binary in prod image | Ephemeral replica hydration | ✓ | 0.5 | None — pinned in Dockerfile.prod:20 |

**Missing dependencies with no fallback:** None — everything needed is already present.

**Missing dependencies with fallback:** None.

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go `testing` + `go test -race ./...` for in-repo code; shell + `curl` + `jq` for infrastructure behavior |
| Config file | `.golangci.yml` (lint), `go.mod` (tests). No new test framework needed. |
| Quick run command | `go test -race ./internal/litefs/... ./internal/health/...` |
| Full suite command | `go test -race ./...` |

Phase 65 is infrastructure-only — zero Go code changes planned. Validation is operator-observable (machine counts, HTTP status codes, volume state), not unit-testable.

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| INFRA-01 | `fly.toml` has `[processes]` with `primary` + `replica`, and two `[[vm]]` blocks scoped via `processes = [...]` | static-config check | `grep -E '^\[processes\]' fly.toml && grep -cE '^\[\[vm\]\]' fly.toml` (expect 2 blocks) | ✅ (fly.toml post-edit) |
| INFRA-01 | LiteFS region-gated candidacy unchanged | static-config check | `grep 'candidate:' litefs.yml` (expect `${FLY_REGION == PRIMARY_REGION}`) | ✅ |
| INFRA-02 | `[[mounts]]` scoped to `processes = ["primary"]` | static-config check | `grep -A3 '^\[\[mounts\]\]' fly.toml \| grep 'processes = \["primary"\]'` | ✅ (fly.toml post-edit) |
| INFRA-02 | 7 replica machines have no persistent volume | operational check | `fly status --app peeringdb-plus --json \| jq '[.Machines[] \| select(.config.metadata.fly_process_group == "replica") \| .config.mounts \| length] \| unique'` (expect `[0]` — no mounts on any replica) | ✅ (post-deploy) |
| INFRA-02 | `/readyz` gates traffic during hydration | runtime smoke | Per-region `curl /readyz` during 15-min window; observe transition 503 → 200 per replica | ✅ (existing handler, tested in `internal/health/handler_test.go`) |
| INFRA-03 | `docs/DEPLOYMENT.md` describes ephemeral-replica operational story | doc check | `grep -i 'ephemeral\|asymmetric\|cold-sync' docs/DEPLOYMENT.md` (expect ≥ 1 hit) | ❌ **Wave 0 gap** — needs new subsection |
| INFRA-03 | `CLAUDE.md` §Deployment updated with volume-only-on-primary contract | doc check | `grep -i 'primary.*volume\|volume.*primary' CLAUDE.md` (expect ≥ 1 hit in Deployment section) | ❌ **Wave 0 gap** — needs addition |
| INFRA-03 | SEED-002 moved to `.planning/seeds/consumed/` | repo check | `test -f .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md && ! test -f .planning/seeds/SEED-002-fly-asymmetric-fleet.md` | ❌ **Wave 0 gap** — part of Phase 65 SUMMARY |

### Sampling Rate

- **Per task commit:** `go build ./... && go vet ./...` (no Go behavior changes, but verify the repo still builds — fly.toml edit shouldn't break compilation, but defensive)
- **Per wave merge:** `go test -race ./internal/litefs/... ./internal/health/... ./internal/sync/...` (the subpackages most relevant to primary/readyz/sync behavior)
- **Phase gate:** Full suite green (`go test -race ./...`) + post-deploy `/readyz` green on all 8 regions + `fly volumes list` shows exactly 1 volume (LHR)

### Wave 0 Gaps

- [ ] `docs/DEPLOYMENT.md` — add "Asymmetric fleet" subsection covering: process groups, ephemeral replica recovery (destroy-and-recreate, no volume management), cold-sync timing expectation (≤ 45s per region typical)
- [ ] `CLAUDE.md` §Deployment — add volume-only-on-primary contract note + brief rollback summary
- [ ] `.planning/PROJECT.md` Key Decisions — record v1.15 Phase 65 decision: "asymmetric fleet (1 primary + 7 ephemeral replicas)" with cost delta and rationale
- [ ] Move `.planning/seeds/SEED-002-fly-asymmetric-fleet.md` → `.planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md` at phase close

*(None of these are testable with `go test`; all are doc artifacts the planner will create as plan tasks.)*

## Security Domain

Phase 65 is an infrastructure topology change — no new request surfaces, no new auth paths, no new input validation, no new crypto. The security surface is identical to v1.14 post-Phase 62 (API-key sync, privacy tiers).

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | no | No change to auth surfaces |
| V3 Session Management | no | No sessions in peeringdb-plus |
| V4 Access Control | no | Privacy tier logic unchanged (privfield / entgo privacy policy) |
| V5 Input Validation | no | No new inputs accepted |
| V6 Cryptography | no | TLS termination is Fly Proxy's responsibility, unchanged |

### Known Threat Patterns for Fly.io infrastructure changes

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Accidental exposure via misconfigured process group service scope | Information Disclosure | Keep `[http_service].processes = ["primary", "replica"]` (explicit, same as today's implicit behavior). Never expose an admin-only service without a `processes = [...]` filter. |
| Secrets leaked when migrating machines | Information Disclosure | Fly secrets are app-scoped, not machine-scoped. Migration doesn't touch secrets. Verify post-deploy: `fly secrets list --app peeringdb-plus` count unchanged. |
| Write endpoint exposed on replica (confused deputy) | Tampering | Existing `IsPrimaryFn` gating unchanged — sync worker refuses to run on replicas even if triggered. `fly-replay: region=lhr` still routes writes correctly. |

No new security controls required for Phase 65.

## Open Questions

1. **Q: Can the `[processes]` entries have empty command strings?**
   - What we know: Fly docs show non-empty commands in all examples. Our Docker ENTRYPOINT (`litefs mount`) provides the runtime.
   - What's unclear: whether `flyctl` will accept `primary = ""` or error out requiring a non-empty string.
   - Recommendation: plan task should use literal `"litefs mount"` as the process command for both groups — safer, identical runtime effect. If `flyctl validate fly.toml` complains during task execution, the fix is obvious and documented.

2. **Q: Will the 15-min monitoring window be sufficient to catch p99 regression?**
   - What we know: 15 min covers the first sync cycle post-migration (hourly interval) and several `/readyz` + `/healthz` polls.
   - What's unclear: whether observational effects from a `shared-cpu-1x` downsize show up in 15 min or only under peak traffic (which is diurnal).
   - Recommendation: Extend informal observation to 24h via existing OTel dashboard. If p99 regresses beyond the existing baseline in any region, revert just the replica `[[vm]]` size to `shared-cpu-2x` / 512 MB (one-line fly.toml change), redeploy. Low-effort mitigation.

3. **Q: Does `fly deploy` preserve the primary's volume when the machine gets rolled (new image version)?**
   - What we know: Volumes persist across machine lifecycle events when the new machine is configured with the same mount. Per CONTEXT D-09 and fly.io volume docs.
   - What's unclear: Whether an image-version roll on the LHR primary reattaches the same volume or creates a new one. If new: primary loses the 88 MB DB and re-syncs from PeeringDB.
   - Recommendation: The plan should include a post-deploy check: `fly ssh console --app peeringdb-plus --machine $LHR_MACHINE_ID -C "ls -la /var/lib/litefs/"` — expect to see the existing LiteFS state (LTX files, not empty). If empty, trigger fresh full sync via `POST /sync`.

## Sources

### Primary (HIGH confidence)

- **Context7 `/superfly/docs`** — fly.toml schema for `[processes]`, `[[vm]]`, `[[mounts]]`, `[http_service]`, `[[restart]]` scoping; `fly scale count` behavior; `fly deploy` process-group migration
- **Context7 `/superfly/litefs`** — LiteFS architecture (limited detail; supplemented by official ARCHITECTURE.md)
- **https://fly.io/docs/launch/processes/** — *Critical*: "creates at least one Machine for each process group, and destroys all the Machines that belong to any process group that isn't defined". This is the core trap Pattern 1 addresses.
- **https://fly.io/docs/reference/configuration/** — complete fly.toml schema including `persist_rootfs`, `processes` scoping on `[[vm]]` and `[[mounts]]`
- **https://fly.io/docs/flyctl/volumes-destroy/** — `-y/--yes` flag, attached-volume behavior ("Volumes attached to Machines can't be destroyed")
- **https://fly.io/docs/flyctl/machine-update/** — `--metadata` flag for `fly_process_group=X` reassignment
- **https://fly.io/docs/volumes/volume-manage/** — volume lifecycle, orphan behavior

### Secondary (MEDIUM confidence)

- **https://community.fly.io/t/fly-deploy-changes-for-process-groups-and-auto-scaled-services/16368** — `fly deploy --process-groups` targeted deploys (not used in this phase but documented)
- **https://github.com/superfly/litefs/blob/main/docs/ARCHITECTURE.md** — position-based replication protocol; snapshot fallback when position is not in primary's retention window
- **https://fly.io/blog/flyctl-meets-json/** — `--json` output schema conventions

### Tertiary (LOW confidence — flagged in Assumptions Log)

- **Cold-sync wall-clock for 88 MB across regions:** No official per-region timing. Estimate from intra-Fly bandwidth observations (5-45s). Verified only via 15-min post-deploy observation window.

## Metadata

**Confidence breakdown:**

- Standard stack (fly.toml schema, CLI commands): **HIGH** — official Fly.io docs + Context7 agree on every field and flag
- Architecture patterns (metadata retag, volume cleanup, rollback): **HIGH** — each command flow is direct application of documented behavior
- Migration command sequence: **HIGH** — assembled from verified primitives; idempotency per step has been checked
- LiteFS cold-sync timing: **MEDIUM** — protocol verified, per-region wall-clock is an estimate
- Pitfalls: **HIGH** — all five have official-docs citations or were validated against the repo state
- Runtime state inventory: **HIGH** — every category traced to a specific file/config

**Research date:** 2026-04-18
**Valid until:** 2026-05-18 (30 days; Fly.io process-group semantics and LiteFS are stable)

---

*Phase: 65-asymmetric-fly-fleet*
*Research complete — ready for planning.*
