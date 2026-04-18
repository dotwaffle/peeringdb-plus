---
phase: 65-asymmetric-fly-fleet
plan: 02
type: execute
wave: 2
depends_on: [01]
files_modified:
  - .planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md
autonomous: true
requirements: [INFRA-01, INFRA-02, INFRA-03]
tags: [infra, fly, litefs, deployment, migration]

user_setup:
  - service: fly.io
    why: "Executor must be authenticated to fly.io CLI to run deploy, machine update, volume destroy, scale commands against the peeringdb-plus app"
    env_vars: []
    dashboard_config:
      - task: "Verify flyctl authentication"
        location: "Run `fly auth whoami` — should return the operator email (dotwaffle@gmail.com)"

must_haves:
  truths:
    - "Exactly 1 machine is tagged fly_process_group=primary and it is in lhr"
    - "Exactly 7 machines are tagged fly_process_group=replica and they are in the 7 non-LHR regions"
    - "The LHR primary machine's volume attachment survives the migration"
    - "After deploy, fly status shows 1 primary machine (shared-cpu-2x/512MB, mounted) + 7 replica machines (shared-cpu-1x/256MB, no mount), all state=started"
    - "/readyz returns 200 from all 8 Fly regions within the monitoring window"
    - "Exactly 1 volume remains (LHR), 7 replica volumes destroyed, 0 orphans"
    - "litefs.yml and Go source and Dockerfile are not modified during this plan"
  artifacts:
    - path: ".planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md"
      provides: "Inline operator runbook of the executed migration, including the volume-cleanup trace"
      min_lines: 80
    - path: "/tmp/phase65/post-status.json"
      provides: "Audit-trail snapshot of fly state after deploy"
  key_links:
    - from: "Task 2 metadata-retag"
      to: "Task 4 fly deploy"
      via: "Without retag, deploy destroys LHR machine and orphans the primary volume"
      pattern: "fly machine update --metadata fly_process_group"
    - from: "Task 5 monitoring window"
      to: "Task 6 volume cleanup"
      via: "Volume destroy only after /readyz green on all 8 regions"
      pattern: "/readyz.*200"
    - from: "Task 6 volume destroy"
      to: "jq safeguard filter"
      via: "Never destroy lhr-region volumes or attached volumes — enforced by jq query"
      pattern: "select\\(\\.region != \"lhr\"\\) \\| select\\(\\(\\.attached_machine_id // \"\"\\) == \"\"\\)"
---

<objective>
Execute the asymmetric-fleet migration against the live `peeringdb-plus` Fly.io app. This plan assumes Plan 01 has landed the fly.toml rewrite + docs + seed move in git, and the current commit is the migration target.

**User has chosen FULL AUTONOMOUS execution.** No checkpoints inside the migration sequence. The executor runs `fly machine update`, `fly deploy`, `fly volumes destroy`, and `fly scale` commands directly.

Purpose: Transition the uniform 8-machine fleet to 1 primary (LHR, persistent volume) + 7 replicas (other regions, ephemeral rootfs), destroy the 7 orphaned replica volumes, and verify the end-state.

Output:
- 1 fly deploy executed against the new fly.toml
- 7 replica volumes destroyed after machines recreate without mounts
- 15-minute monitoring window with per-region `/readyz` smoke checks
- `.planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md` capturing: pre-state snapshot, retag evidence, deploy output, post-state verification, volume destroy trace, final `fly volumes list` (must show exactly 1 volume in LHR)

Implements:
- INFRA-01 (process groups active in production)
- INFRA-02 (replicas ephemeral — volumes destroyed; `/readyz` gates traffic during hydration)
- INFRA-03 (SEED-002 already consumed in Plan 01; this plan's SUMMARY closes the doc trail with the production-observed cold-sync timings)
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md

**Execution environment:** This plan requires `flyctl` authenticated against the `peeringdb-plus` app, `jq`, and `curl` on PATH. The executor must run from a shell that can reach `https://peeringdb-plus.fly.dev`. Commands use `--app peeringdb-plus` explicitly and never rely on `FLY_APP` env var.

**Timing:** End-to-end runtime ~25 minutes (5 min pre-flight + 8 min deploy + 15 min monitoring + 2 min cleanup). Most of the wall-clock is the monitoring window.

**Concurrency:** NONE. Every task in this plan runs sequentially. Do NOT parallelise — the migration is a single atomic ordered sequence.

**Rollback:** Each task documents its rollback point. See 65-RESEARCH.md § "Rollback runbook" for R1 (pre-cleanup revert), R2 (post-cleanup revert), R3 (catastrophic LHR loss), R4 (partial failure).
</execution_context>

<context>
@.planning/phases/65-asymmetric-fly-fleet/65-CONTEXT.md
@.planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md
@.planning/phases/65-asymmetric-fly-fleet/65-01-SUMMARY.md

# The already-merged artifacts from Plan 01
@fly.toml

<interfaces>
<!-- Operational invariants and safeguard contracts -->

Pre-migration invariants (must hold before Task 1):
- `git log -1 --oneline` must show the "Phase 65 fly.toml asymmetric fleet" commit on HEAD (or a later commit that includes it)
- `fly status --app peeringdb-plus --json | jq 'length'` returns 8 machines, all state=started
- `curl -sS https://peeringdb-plus.fly.dev/readyz -o /dev/null -w "%{http_code}"` returns 200
- Exactly 8 volumes exist (`fly volumes list --app peeringdb-plus --json | jq length` = 8), 8 attached

Volume-cleanup safeguard (jq filter enforced in Task 6):

    .[]
    | select(.region != "lhr")
    | select((.attached_machine_id // "") == "")
    | .id

This filter guarantees:
1. No LHR-region volume is ever selected for destroy
2. No attached volume is ever selected (fly would reject it anyway, but the filter adds defence)
3. The count MUST be exactly 7 — if anything other than 7, abort
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Pre-flight snapshot and health check</name>
  <files>/tmp/phase65/pre-status.json, /tmp/phase65/pre-machines.json, /tmp/phase65/pre-volumes.json</files>
  <read_first>
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Exact migration command sequence" Step 0
    - fly.toml (confirm it has the Phase 65 changes — `grep '^\[processes\]' fly.toml` must hit)
  </read_first>
  <action>
Run pre-flight:

```bash
APP=peeringdb-plus
mkdir -p /tmp/phase65

# 0a. Prereq: verify fly.toml is the Phase 65 target
grep -q '^\[processes\]' fly.toml || { echo "ABORT: fly.toml lacks [processes] — Plan 01 did not land"; exit 1; }
grep -cE '^\[\[vm\]\]' fly.toml | grep -q '^2$' || { echo "ABORT: fly.toml does not have 2 [[vm]] blocks"; exit 1; }

# 0b. Prereq: flyctl auth
fly auth whoami || { echo "ABORT: flyctl not authenticated"; exit 1; }
fly version

# 0c. Prereq: service healthy before we start
HTTP=$(curl -sS -o /dev/null -w "%{http_code}" https://peeringdb-plus.fly.dev/readyz)
[[ "$HTTP" == "200" ]] || { echo "ABORT: /readyz is $HTTP (expected 200). Fix the current fleet before migrating."; exit 1; }

# 0d. Snapshot state
fly status --app "$APP" --json > /tmp/phase65/pre-status.json
fly machine list --app "$APP" --json > /tmp/phase65/pre-machines.json
fly volumes list --app "$APP" --json > /tmp/phase65/pre-volumes.json

# 0e. Sanity: expect 8 machines, 8 volumes
MACHINES=$(jq 'length' /tmp/phase65/pre-machines.json)
VOLUMES=$(jq 'length' /tmp/phase65/pre-volumes.json)
echo "Pre-state: $MACHINES machines, $VOLUMES volumes"
[[ "$MACHINES" == "8" ]] || { echo "ABORT: expected 8 machines, got $MACHINES"; exit 1; }
[[ "$VOLUMES" == "8" ]] || { echo "ABORT: expected 8 volumes, got $VOLUMES"; exit 1; }

# 0f. Snapshot the region→machine_id mapping for the audit trail
jq -r '.[] | "\(.region)\t\(.id)\t\(.state)\t\(.config.metadata.fly_process_group // "app")"' \
  /tmp/phase65/pre-machines.json | tee /tmp/phase65/pre-machines.txt
```

If ANY of the aborts fire, STOP. Do not proceed to Task 2. The SUMMARY must record which guard fired and the raw output. A failed pre-flight is not a migration failure — it is a correctness win for the guardrail.
  </action>
  <verify>
    <automated>test -s /tmp/phase65/pre-machines.json && test -s /tmp/phase65/pre-volumes.json && test -s /tmp/phase65/pre-status.json && jq 'length' /tmp/phase65/pre-machines.json | grep -q '^8$' && jq 'length' /tmp/phase65/pre-volumes.json | grep -q '^8$'</automated>
  </verify>
  <done>
Three JSON snapshots exist in /tmp/phase65/, each >= 1KB, each parsing cleanly with `jq`. Machine count is 8 and volume count is 8. `/readyz` returned 200 before the migration started. Current branch HEAD includes the Phase 65 fly.toml.
  </done>
</task>

<task type="auto">
  <name>Task 2: Pre-tag all 8 machines with fly_process_group metadata</name>
  <files>/tmp/phase65/post-retag-machines.txt</files>
  <read_first>
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pattern 1: Metadata-retag existing machines BEFORE changing fly.toml" — this is the CRITICAL pre-deploy step
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pitfall 1: fly deploy destroys existing machines when [processes] is introduced for the first time"
    - /tmp/phase65/pre-machines.json (from Task 1 — machine inventory)
  </read_first>
  <action>
**THIS IS THE MOST DANGEROUS STEP IF SKIPPED.** Without pre-tagging, `fly deploy` with the new fly.toml will destroy every machine (including LHR) because they are currently in the implicit `app` process group. Pre-tagging tells Fly to treat the existing machines as members of the new `primary` / `replica` groups.

```bash
APP=peeringdb-plus

# Identify LHR vs replicas from pre-migration snapshot
LHR_MACHINE_ID=$(jq -r '.[] | select(.region == "lhr") | .id' /tmp/phase65/pre-machines.json)
REPLICA_MACHINE_IDS=$(jq -r '.[] | select(.region != "lhr") | .id' /tmp/phase65/pre-machines.json)

echo "LHR machine (will become primary): $LHR_MACHINE_ID"
echo "Replica machines:"
echo "$REPLICA_MACHINE_IDS"

# Sanity: exactly 1 LHR, exactly 7 non-LHR
LHR_COUNT=$(echo "$LHR_MACHINE_ID" | grep -c .)
REPLICA_COUNT=$(echo "$REPLICA_MACHINE_IDS" | grep -c .)
[[ "$LHR_COUNT" == "1" ]] || { echo "ABORT: expected 1 LHR machine, got $LHR_COUNT"; exit 1; }
[[ "$REPLICA_COUNT" == "7" ]] || { echo "ABORT: expected 7 replica machines, got $REPLICA_COUNT"; exit 1; }

# Retag LHR first (primary) — preserves volume attachment continuity
fly machine update --metadata fly_process_group=primary "$LHR_MACHINE_ID" --app "$APP" --yes

# Retag the 7 others as replica
for id in $REPLICA_MACHINE_IDS; do
  echo "Retagging $id -> replica"
  fly machine update --metadata fly_process_group=replica "$id" --app "$APP" --yes
done

# Verify and persist audit trail
fly machine list --app "$APP" --json \
  | jq -r '.[] | "\(.region)\t\(.id)\t\(.config.metadata.fly_process_group // "UNTAGGED")"' \
  | tee /tmp/phase65/post-retag-machines.txt

# Assertions
PRIMARY_COUNT=$(awk -F'\t' '$3 == "primary"' /tmp/phase65/post-retag-machines.txt | wc -l)
REPLICA_TAGGED=$(awk -F'\t' '$3 == "replica"' /tmp/phase65/post-retag-machines.txt | wc -l)
UNTAGGED=$(awk -F'\t' '$3 == "UNTAGGED" || $3 == "app"' /tmp/phase65/post-retag-machines.txt | wc -l)
LHR_IS_PRIMARY=$(awk -F'\t' '$1 == "lhr" && $3 == "primary"' /tmp/phase65/post-retag-machines.txt | wc -l)

echo "primary=$PRIMARY_COUNT replica=$REPLICA_TAGGED untagged=$UNTAGGED lhr_is_primary=$LHR_IS_PRIMARY"
[[ "$PRIMARY_COUNT" == "1" ]] || { echo "ABORT: expected 1 primary-tagged machine, got $PRIMARY_COUNT"; exit 1; }
[[ "$REPLICA_TAGGED" == "7" ]] || { echo "ABORT: expected 7 replica-tagged machines, got $REPLICA_TAGGED"; exit 1; }
[[ "$UNTAGGED" == "0" ]] || { echo "ABORT: found $UNTAGGED untagged machines — deploy would destroy them"; exit 1; }
[[ "$LHR_IS_PRIMARY" == "1" ]] || { echo "ABORT: the LHR machine is not tagged primary"; exit 1; }
```

Any abort means do NOT proceed to Task 3. Investigate the tagging and re-run the retag for failed machines individually (metadata updates are idempotent).
  </action>
  <verify>
    <automated>test -s /tmp/phase65/post-retag-machines.txt && awk -F'\t' '$3 == "primary" && $1 == "lhr"' /tmp/phase65/post-retag-machines.txt | grep -q lhr && [[ $(awk -F'\t' '$3 == "replica"' /tmp/phase65/post-retag-machines.txt | wc -l) -eq 7 ]] && [[ $(awk -F'\t' '$3 == "UNTAGGED" || $3 == "app"' /tmp/phase65/post-retag-machines.txt | wc -l) -eq 0 ]]</automated>
  </verify>
  <done>
All 8 machines have `fly_process_group` metadata set: exactly 1 `primary` (LHR) + exactly 7 `replica` (iad, nrt, syd, lax, jnb, sin, gru). Zero `app`-group or untagged machines remain. The LHR machine still holds its volume attachment (volume did not move during metadata update — metadata is cosmetic). `/tmp/phase65/post-retag-machines.txt` is the audit trail.
  </done>
</task>

<task type="auto">
  <name>Task 3: Final fly.toml + litefs.yml sanity check</name>
  <files>/tmp/phase65/fly-toml-diff.txt</files>
  <read_first>
    - fly.toml (the already-committed Plan 01 result)
    - litefs.yml (must be unchanged)
    - .planning/phases/65-asymmetric-fly-fleet/65-01-SUMMARY.md (confirmation that Plan 01 landed)
  </read_first>
  <action>
Quick last-chance check before the irreversible deploy:

```bash
# Confirm fly.toml matches the Phase 65 target structure
grep -q '^\[processes\]' fly.toml || { echo "ABORT: fly.toml lacks [processes]"; exit 1; }
grep -q '^  primary = ""' fly.toml || { echo "ABORT: missing primary process entry"; exit 1; }
grep -q '^  replica = ""' fly.toml || { echo "ABORT: missing replica process entry"; exit 1; }
[[ $(grep -cE '^\[\[vm\]\]' fly.toml) -eq 2 ]] || { echo "ABORT: expected 2 [[vm]] blocks"; exit 1; }
grep -q 'processes = \["primary"\]' fly.toml || { echo "ABORT: no [[vm]] or [[mounts]] scoped to primary"; exit 1; }
grep -q 'processes = \["replica"\]' fly.toml || { echo "ABORT: no [[vm]] scoped to replica"; exit 1; }
grep -q 'size = "shared-cpu-1x"' fly.toml || { echo "ABORT: replica sizing not set to shared-cpu-1x"; exit 1; }
grep -q 'memory = "256mb"' fly.toml || { echo "ABORT: replica memory not 256mb"; exit 1; }

# Confirm litefs.yml candidacy gate UNCHANGED
grep -q 'candidate: ${FLY_REGION == PRIMARY_REGION}' litefs.yml \
  || { echo "ABORT: litefs.yml candidacy gate changed — Phase 65 assumes it is unchanged"; exit 1; }

# Validate via flyctl if possible
if fly config validate --config fly.toml; then
  echo "fly config validate: OK"
else
  echo "ABORT: fly config validate failed"
  exit 1
fi

# Capture the final diff surface for the audit trail
git diff HEAD~1 -- fly.toml > /tmp/phase65/fly-toml-diff.txt 2>&1 || true
wc -l /tmp/phase65/fly-toml-diff.txt
```

Abort on any failure. The migration must not proceed against a partially-correct fly.toml.

**Fallback on assumption A2 (empty command strings rejected):** If `fly config validate` complains about empty `primary = ""` / `replica = ""` entries, replace both with `"litefs mount"`:

```bash
# Only if fly config validate errors on empty command strings
sed -i 's/^  primary = ""$/  primary = "litefs mount"/' fly.toml
sed -i 's/^  replica = ""$/  replica = "litefs mount"/' fly.toml
fly config validate --config fly.toml
git add fly.toml && git commit -m "fix(infra): Phase 65 — use explicit 'litefs mount' in [processes] (A2 fallback)"
```

Re-run the validate afterwards. If still failing, abort — do NOT deploy a broken fly.toml.
  </action>
  <verify>
    <automated>grep -q '^\[processes\]' fly.toml && grep -q 'processes = \["primary"\]' fly.toml && grep -q 'processes = \["replica"\]' fly.toml && grep -q 'shared-cpu-1x' fly.toml && grep -q 'candidate: ${FLY_REGION == PRIMARY_REGION}' litefs.yml && fly config validate --config fly.toml</automated>
  </verify>
  <done>
fly.toml passes structural grep checks + `fly config validate`. litefs.yml is unchanged (keystone preserved). The Git diff against the pre-Phase-65 state is captured at /tmp/phase65/fly-toml-diff.txt for the SUMMARY.
  </done>
</task>

<task type="auto">
  <name>Task 4: fly deploy — roll to asymmetric fleet</name>
  <files>/tmp/phase65/deploy-output.log, /tmp/phase65/post-deploy-status.json, /tmp/phase65/post-deploy-machines.json</files>
  <read_first>
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pitfall 4: Changing VM size forces machine destroy-and-recreate" — expected behavior, not a failure
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pitfall 5: max_unavailable = 0.5 rolls 4 replicas simultaneously"
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Anti-Patterns to Avoid" — do NOT `fly scale count replica=0` as a reset
    - /tmp/phase65/post-retag-machines.txt (from Task 2 — confirms 8 machines all tagged)
  </read_first>
  <action>
Run the deploy. Expected wall-clock: 5-8 minutes for full rolling deploy (4 parallel cohorts per `max_unavailable=0.5`).

```bash
APP=peeringdb-plus

# Capture LHR machine ID for post-deploy verification
LHR_MACHINE_ID=$(jq -r '.[] | select(.region == "lhr") | .id' /tmp/phase65/pre-machines.json)
echo "Pre-deploy LHR machine: $LHR_MACHINE_ID"

# Launch deploy and tee full log for audit
fly deploy --app "$APP" 2>&1 | tee /tmp/phase65/deploy-output.log
DEPLOY_RC=${PIPESTATUS[0]}

if [[ "$DEPLOY_RC" != "0" ]]; then
  echo "ABORT: fly deploy exited non-zero. See /tmp/phase65/deploy-output.log."
  echo "Rollback per 65-RESEARCH.md § Rollback runbook R1: git revert HEAD && fly deploy"
  exit "$DEPLOY_RC"
fi

# Watch for the catastrophic abort signal — LHR machine destroyed
if grep -E "Destroying machine $LHR_MACHINE_ID" /tmp/phase65/deploy-output.log; then
  echo "CATASTROPHIC: LHR primary machine was destroyed. Task 2 metadata retag must have failed."
  echo "See 65-RESEARCH.md § Rollback runbook R3 (catastrophic LHR loss)."
  exit 1
fi

# Snapshot post-deploy state
fly status --app "$APP" --json > /tmp/phase65/post-deploy-status.json
fly machine list --app "$APP" --json > /tmp/phase65/post-deploy-machines.json

# Post-deploy structural assertions
TOTAL=$(jq 'length' /tmp/phase65/post-deploy-machines.json)
PRIMARIES=$(jq '[.[] | select(.config.metadata.fly_process_group == "primary")] | length' /tmp/phase65/post-deploy-machines.json)
REPLICAS=$(jq '[.[] | select(.config.metadata.fly_process_group == "replica")] | length' /tmp/phase65/post-deploy-machines.json)
LHR_PRIMARY=$(jq -r '[.[] | select(.region == "lhr" and .config.metadata.fly_process_group == "primary")] | length' /tmp/phase65/post-deploy-machines.json)

echo "Post-deploy: total=$TOTAL primary=$PRIMARIES replica=$REPLICAS lhr_primary=$LHR_PRIMARY"
[[ "$TOTAL" == "8" ]] || { echo "ABORT: expected 8 machines, got $TOTAL"; exit 1; }
[[ "$PRIMARIES" == "1" ]] || { echo "ABORT: expected 1 primary, got $PRIMARIES"; exit 1; }
[[ "$REPLICAS" == "7" ]] || { echo "ABORT: expected 7 replicas, got $REPLICAS"; exit 1; }
[[ "$LHR_PRIMARY" == "1" ]] || { echo "ABORT: LHR is not tagged primary"; exit 1; }

# Confirm no replica has a mount (INFRA-02)
REPLICA_WITH_MOUNTS=$(jq '[.[] | select(.config.metadata.fly_process_group == "replica") | select((.config.mounts // []) | length > 0)] | length' /tmp/phase65/post-deploy-machines.json)
[[ "$REPLICA_WITH_MOUNTS" == "0" ]] || { echo "ABORT: $REPLICA_WITH_MOUNTS replica(s) still have mounts — INFRA-02 violated"; exit 1; }

# Confirm LHR still has its mount (volume not orphaned)
LHR_MOUNTS=$(jq '[.[] | select(.region == "lhr") | .config.mounts // []] | flatten | length' /tmp/phase65/post-deploy-machines.json)
[[ "$LHR_MOUNTS" -ge "1" ]] || { echo "ABORT: LHR has no mount — primary volume may have orphaned"; exit 1; }

# Confirm replica VM sizing
REPLICA_SIZE_OK=$(jq '[.[] | select(.config.metadata.fly_process_group == "replica") | select(.config.guest.cpu_kind == "shared" and .config.guest.cpus == 1 and .config.guest.memory_mb == 256)] | length' /tmp/phase65/post-deploy-machines.json)
[[ "$REPLICA_SIZE_OK" == "7" ]] || { echo "WARN: only $REPLICA_SIZE_OK of 7 replicas are shared-cpu-1x/256MB. Investigate before declaring INFRA-01 satisfied."; }

# Confirm primary VM sizing unchanged
PRIMARY_SIZE_OK=$(jq '[.[] | select(.config.metadata.fly_process_group == "primary") | select(.config.guest.cpu_kind == "shared" and .config.guest.cpus == 2 and .config.guest.memory_mb == 512)] | length' /tmp/phase65/post-deploy-machines.json)
[[ "$PRIMARY_SIZE_OK" == "1" ]] || { echo "WARN: primary is not shared-cpu-2x/512MB. Investigate."; }

# Confirm LHR primary still has LiteFS state (volume reattached cleanly)
fly ssh console --app "$APP" --machine "$LHR_MACHINE_ID" -C "ls -la /var/lib/litefs/ | head -20" | tee /tmp/phase65/lhr-litefs-state.txt
grep -q 'dbs\|ltx' /tmp/phase65/lhr-litefs-state.txt || echo "WARN: LHR /var/lib/litefs appears empty — primary may need to re-sync from PeeringDB (R3 path)"
```

If deploy fails or LHR loses volume: rollback per R1/R3 from 65-RESEARCH.md and ESCALATE. Do not proceed to Task 5.

Expected deploy output hallmarks:
- `Updating existing machines with rolling strategy` (for LHR primary — keeps its config contextually)
- `Destroying machine <id>` for 7 replica machines (because VM size changed)
- `Creating machine in region <region>` for 7 replacements
- Final line `v<N> deployed successfully`
  </action>
  <verify>
    <automated>test -s /tmp/phase65/post-deploy-machines.json && [[ $(jq '[.[] | select(.config.metadata.fly_process_group == "primary")] | length' /tmp/phase65/post-deploy-machines.json) -eq 1 ]] && [[ $(jq '[.[] | select(.config.metadata.fly_process_group == "replica")] | length' /tmp/phase65/post-deploy-machines.json) -eq 7 ]] && [[ $(jq '[.[] | select(.config.metadata.fly_process_group == "replica") | select((.config.mounts // []) | length > 0)] | length' /tmp/phase65/post-deploy-machines.json) -eq 0 ]]</automated>
  </verify>
  <done>
Deploy completed. `fly machine list` shows exactly 1 `primary` machine in LHR (with mount) + 7 `replica` machines in the 7 other regions (no mounts). Replicas are sized shared-cpu-1x/256MB. LHR is sized shared-cpu-2x/512MB with litefs_data mounted at /var/lib/litefs and the LiteFS state directory is non-empty. Deploy log saved at /tmp/phase65/deploy-output.log.
  </done>
</task>

<task type="auto">
  <name>Task 5: 15-minute monitoring window — per-region /readyz smoke</name>
  <files>/tmp/phase65/readyz-smoke.log</files>
  <read_first>
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pattern 2: LiteFS cold-sync on ephemeral rootfs" — hydration timing
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pattern 3: /readyz fail-closed during hydration"
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pitfall 3: /readyz false-negative during cold-sync" — sync_status staleness
    - /tmp/phase65/post-deploy-machines.json (from Task 4 — regions to smoke-test)
  </read_first>
  <action>
Run a 15-minute observation window. Smoke-test `/readyz` per region every 30 seconds and log the transition from 503 (hydrating) to 200 (ready) for each replica. Then pull `fly status` every 5 min as a sanity check.

```bash
APP=peeringdb-plus
REGIONS="lhr iad nrt syd lax jnb sin gru"

echo "=== Monitoring window start: $(date -u +%FT%TZ) ===" | tee /tmp/phase65/readyz-smoke.log

# Poll /readyz per region every 30s for 15 minutes (30 iterations)
for iter in $(seq 1 30); do
  TS=$(date -u +%FT%TZ)
  for region in $REGIONS; do
    code=$(curl -sS -o /dev/null -w "%{http_code}" \
      --max-time 5 \
      -H "Fly-Prefer-Region: $region" \
      -H "User-Agent: Mozilla/5.0 (phase65-smoke)" \
      https://peeringdb-plus.fly.dev/readyz 2>/dev/null || echo "ERR")
    echo "$TS  iter=$iter  $region: $code" | tee -a /tmp/phase65/readyz-smoke.log
  done
  sleep 30
done

echo "=== Monitoring window end: $(date -u +%FT%TZ) ===" | tee -a /tmp/phase65/readyz-smoke.log

# Final per-region tally — expect ALL regions on 200 in the last 5 iterations
tail -n 40 /tmp/phase65/readyz-smoke.log | awk '/iter=2[6-9]|iter=30/ {print}' | tee /tmp/phase65/readyz-smoke-tail.log

FAILING_REGIONS=$(awk '/iter=2[6-9]|iter=30/ {for (i=4; i<=NF; i+=2) print $i, $(i+1)}' /tmp/phase65/readyz-smoke.log | awk '$2 != "200:" && $2 != "200" {print}' | sort -u)

if [[ -n "$FAILING_REGIONS" ]]; then
  echo "WARN: regions not on 200 in the last 2.5 minutes of the window:"
  echo "$FAILING_REGIONS"
  echo "Investigate before Task 6 (volume cleanup). Possible causes:"
  echo "  (a) Cold-sync still in progress for SYD/GRU — extend window by 5 min and re-check"
  echo "  (b) sync_status stale — POST /sync with PDBPLUS_SYNC_TOKEN"
  echo "  (c) Machine stuck — fly machine restart <id> or destroy --force"
fi

# Periodic fly status snapshots
fly status --app "$APP" > /tmp/phase65/monitoring-status-final.txt
cat /tmp/phase65/monitoring-status-final.txt

# Confirm all 8 machines state=started
NOT_STARTED=$(jq -r '[.[] | select(.state != "started")] | length' /tmp/phase65/post-deploy-machines.json)
fly machine list --app "$APP" --json > /tmp/phase65/post-monitor-machines.json
NOT_STARTED_NOW=$(jq -r '[.[] | select(.state != "started")] | length' /tmp/phase65/post-monitor-machines.json)

[[ "$NOT_STARTED_NOW" == "0" ]] || { echo "ABORT: $NOT_STARTED_NOW machine(s) not started after monitoring window. See post-monitor-machines.json."; exit 1; }
```

If the final 2.5 minutes show any region returning non-200, do NOT proceed to Task 6. Investigate and extend the window by another 5 minutes; re-evaluate. If still failing after extension, consider rollback per R4 (partial failure) from 65-RESEARCH.md. Hanging a replica indefinitely is acceptable for short windows (Fly Proxy routes around it) but blocks INFRA-02 success criteria.

**Note:** `/readyz` hitting non-LHR regions relies on Fly's `Fly-Prefer-Region` header. If the header is ignored and every request lands on LHR, that is a smoke-test failure mode — not a fleet failure mode. Cross-check by running `fly ssh console --app peeringdb-plus --machine <replica_id> -C "curl -sS -o /dev/null -w '%{http_code}' http://localhost:8080/readyz"` for one replica to confirm local health.
  </action>
  <verify>
    <automated>test -s /tmp/phase65/readyz-smoke.log && [[ $(jq -r '[.[] | select(.state != "started")] | length' /tmp/phase65/post-monitor-machines.json) -eq 0 ]] && tail -n 8 /tmp/phase65/readyz-smoke.log | grep -qE 'lhr: 200'</automated>
  </verify>
  <done>
15-minute smoke window completed. All 8 machines are state=started. Last 2.5 minutes of per-region `/readyz` polling show 200 across all regions (or documented exceptions in readyz-smoke.log with operator-reviewed rationale). `fly status` output saved to /tmp/phase65/monitoring-status-final.txt.
  </done>
</task>

<task type="auto">
  <name>Task 6: Volume cleanup — destroy 7 orphaned replica volumes</name>
  <files>/tmp/phase65/volumes-before-cleanup.json, /tmp/phase65/volumes-after-cleanup.json, /tmp/phase65/destroy-trace.log</files>
  <read_first>
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Volume cleanup runbook (with check-before-destroy safeguard)"
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Pitfall 2: Volumes persist after scale-down"
    - .planning/phases/65-asymmetric-fly-fleet/65-RESEARCH.md § "Anti-Patterns to Avoid" — do NOT destroy attached volumes
  </read_first>
  <action>
Destroy the 7 orphaned replica volumes. Safeguards are enforced by jq filter — the script ABORTS if any of the three invariants fail.

```bash
APP=peeringdb-plus

# Step 1: enumerate and confirm state
fly volumes list --app "$APP" --json > /tmp/phase65/volumes-before-cleanup.json

echo "Current volume state:"
jq -r '.[] | "\(.region)\t\(.id)\t\(.name)\tattached=\(.attached_machine_id // "none")"' \
  /tmp/phase65/volumes-before-cleanup.json | tee /tmp/phase65/volumes-before-table.txt

# Expected shape:
#   lhr    vol_xxx    litefs_data    attached=<LHR machine id>
#   iad    vol_yyy    litefs_data    attached=none
#   ...7 replica rows...

# Safeguard 1: count all volumes — expect 8 (1 LHR + 7 detached replicas)
TOTAL=$(jq 'length' /tmp/phase65/volumes-before-cleanup.json)
[[ "$TOTAL" == "8" ]] || { echo "ABORT: expected 8 volumes, got $TOTAL. Investigate /tmp/phase65/volumes-before-cleanup.json"; exit 1; }

# Safeguard 2: LHR volume MUST still be attached
LHR_ATTACHED=$(jq -r '[.[] | select(.region == "lhr" and (.attached_machine_id // "") != "")] | length' /tmp/phase65/volumes-before-cleanup.json)
[[ "$LHR_ATTACHED" == "1" ]] || { echo "ABORT: LHR volume is not attached ($LHR_ATTACHED). Do not destroy volumes — rollback via R3."; exit 1; }

# Step 2: build destroy list with safeguards (non-LHR AND detached)
DESTROY_IDS=$(jq -r '
  .[]
  | select(.region != "lhr")
  | select((.attached_machine_id // "") == "")
  | .id
' /tmp/phase65/volumes-before-cleanup.json)

echo "Volumes selected for destroy:"
for id in $DESTROY_IDS; do
  region=$(jq -r ".[] | select(.id == \"$id\") | .region" /tmp/phase65/volumes-before-cleanup.json)
  echo "  $id (region=$region)"
done | tee /tmp/phase65/destroy-selection.txt

# Safeguard 3: count MUST be exactly 7
COUNT=$(echo "$DESTROY_IDS" | grep -c . || echo 0)
if [[ "$COUNT" != "7" ]]; then
  echo "ABORT: Expected exactly 7 replica volumes to destroy, got $COUNT."
  echo "Something unexpected in the post-deploy state. Investigate before proceeding."
  echo "  - If < 7: some replica machines may still be attached. Check fly machine list."
  echo "  - If > 7: an unexpected volume exists. Inspect /tmp/phase65/volumes-before-cleanup.json."
  exit 1
fi

# Safeguard 4: the ids MUST NOT include the LHR volume id (belt + braces — the jq filter already excluded it)
LHR_VOL_ID=$(jq -r '.[] | select(.region == "lhr") | .id' /tmp/phase65/volumes-before-cleanup.json)
if echo "$DESTROY_IDS" | grep -qx "$LHR_VOL_ID"; then
  echo "ABORT: LHR volume $LHR_VOL_ID ended up in destroy list — jq filter misbehaved"
  exit 1
fi

# Step 3: destroy (non-interactive with --yes)
for id in $DESTROY_IDS; do
  echo "Destroying $id..." | tee -a /tmp/phase65/destroy-trace.log
  fly volumes destroy "$id" --app "$APP" --yes 2>&1 | tee -a /tmp/phase65/destroy-trace.log
done

# Step 4: verify final state
fly volumes list --app "$APP" --json > /tmp/phase65/volumes-after-cleanup.json
REMAINING=$(jq 'length' /tmp/phase65/volumes-after-cleanup.json)
LHR_STILL_THERE=$(jq -r '.[] | select(.region == "lhr") | .id' /tmp/phase65/volumes-after-cleanup.json)

echo "Final volume state:"
jq -r '.[] | "\(.region)\t\(.id)\tattached=\(.attached_machine_id // "none")"' /tmp/phase65/volumes-after-cleanup.json | tee /tmp/phase65/volumes-after-table.txt

[[ "$REMAINING" == "1" ]] || { echo "ABORT: expected 1 volume remaining, got $REMAINING"; exit 1; }
[[ -n "$LHR_STILL_THERE" ]] || { echo "ABORT: LHR volume is gone — catastrophic"; exit 1; }

echo "Cleanup success: 1 volume remains ($LHR_STILL_THERE in lhr)."
```

If any abort fires mid-destroy: inspect `/tmp/phase65/destroy-trace.log` and `/tmp/phase65/volumes-after-cleanup.json`. A partial destroy still leaves us better off than pre-Phase-65 — whatever destroyed is destroyed. The remaining orphans can be re-attempted by re-running this task.
  </action>
  <verify>
    <automated>test -s /tmp/phase65/volumes-after-cleanup.json && [[ $(jq 'length' /tmp/phase65/volumes-after-cleanup.json) -eq 1 ]] && [[ $(jq -r '.[0].region' /tmp/phase65/volumes-after-cleanup.json) == "lhr" ]] && [[ -s /tmp/phase65/destroy-trace.log ]]</automated>
  </verify>
  <done>
`fly volumes list --app peeringdb-plus` shows exactly 1 volume — `litefs_data` in `lhr`, attached to the primary machine. 7 replica volumes were destroyed. /tmp/phase65/destroy-trace.log captures the full interactive trace of each destroy. No volume in a non-LHR region exists.
  </done>
</task>

<task type="auto">
  <name>Task 7: Final audit snapshot and write SUMMARY</name>
  <files>.planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md, /tmp/phase65/post-status.json, /tmp/phase65/post-machines.json, /tmp/phase65/post-volumes.json</files>
  <read_first>
    - $HOME/.claude/get-shit-done/templates/summary.md — SUMMARY structure
    - /tmp/phase65/pre-machines.json + post-deploy-machines.json + readyz-smoke.log + volumes-after-cleanup.json (all the audit artifacts from Tasks 1–6)
  </read_first>
  <action>
Capture final audit snapshot:

```bash
APP=peeringdb-plus
fly status --app "$APP" --json > /tmp/phase65/post-status.json
fly machine list --app "$APP" --json > /tmp/phase65/post-machines.json
fly volumes list --app "$APP" --json > /tmp/phase65/post-volumes.json
fly secrets list --app "$APP" > /tmp/phase65/post-secrets.txt

# Final sanity: all 8 machines started, 1 volume in LHR, secrets unchanged count-wise
MACHINES=$(jq 'length' /tmp/phase65/post-machines.json)
VOLUMES=$(jq 'length' /tmp/phase65/post-volumes.json)
STARTED=$(jq '[.[] | select(.state == "started")] | length' /tmp/phase65/post-machines.json)
echo "Final: $MACHINES machines ($STARTED started), $VOLUMES volume(s)"
```

Write `.planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md`. Required sections:

1. **Migration executed on:** date/time (UTC), git commit of fly.toml at deploy.
2. **Pre-state** — 8 machines, 8 volumes (paste `/tmp/phase65/pre-machines.txt`).
3. **Retag step** — paste `/tmp/phase65/post-retag-machines.txt`.
4. **Deploy** — wall-clock time, key lines from deploy-output.log (first machine destroy, last machine create, final "deployed successfully"), any warnings.
5. **Monitoring window** — condensed table: per-region time-to-200 (extract first `200` line per region from readyz-smoke.log), any 503s observed and for how long. Call out SYD/GRU/JNB as expected slower.
6. **Volume cleanup** — paste `/tmp/phase65/destroy-selection.txt` and `/tmp/phase65/destroy-trace.log`.
7. **Post-state** — 8 machines (1 primary + 7 replica), 1 volume (LHR), `/readyz` green across regions.
8. **Actual cold-sync timings** (new operational data) — fill the `docs/DEPLOYMENT.md` § Asymmetric fleet table placeholder. If observed times differ from the 5-45s estimate, note the update should be propagated (but do NOT edit DEPLOYMENT.md in this task — that is a post-phase doc refresh, not migration correctness).
9. **Anomalies** — anything unexpected (e.g. a region that took 5 min to hydrate, a fly machine restart that was needed, a warn from Task 4 cpu_kind assertion).
10. **Requirement satisfaction** — one row per INFRA-0{1,2,3} with evidence (file path or /tmp/phase65/ artifact).
11. **Rollback status** — "rollback not triggered" (hopefully) or "rollback R{N} executed on ..." with trace.
12. **SEED-002** — "consumed in Plan 01 (commit $(git log -1 --format=%h -- .planning/seeds/consumed/SEED-002-fly-asymmetric-fleet.md))".
13. **SEED-003** — "planted 2026-04-17; captures future primary-HA work".
14. **Next steps** — list any follow-ups (e.g., update docs/DEPLOYMENT.md § Asymmetric fleet table with observed regional timings if the estimates were materially wrong — queue as a Phase 66 side task or quick task).

Keep the SUMMARY concise (≥ 80 lines, ≤ 250 lines). The detail belongs in /tmp/phase65/ artifact files. SUMMARY links to them by relative path.

Do NOT modify litefs.yml, Go code, Dockerfile, or fly.toml in this task.
  </action>
  <verify>
    <automated>test -s .planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md && [[ $(wc -l < .planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md) -ge 80 ]] && grep -q 'INFRA-01' .planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md && grep -q 'INFRA-02' .planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md && grep -q 'INFRA-03' .planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md && [[ $(jq 'length' /tmp/phase65/post-volumes.json) -eq 1 ]]</automated>
  </verify>
  <done>
65-02-SUMMARY.md exists with all 14 sections. It explicitly cites INFRA-01 / INFRA-02 / INFRA-03 with evidence. /tmp/phase65/post-volumes.json shows exactly 1 volume. /tmp/phase65/post-machines.json shows 8 machines (1 primary, 7 replica, all started). git working tree shows the new SUMMARY.md (ready to commit).
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Operator shell → Fly.io API | `flyctl` authenticated user makes state-changing calls. Auth is TLS-pinned by flyctl. |
| Fly.io control plane → peeringdb-plus app | Fly Proxy + Machines API. No changes in this plan; migration uses only documented primitives. |
| LHR primary → replicas | LiteFS HTTP replication over Fly private network. Unchanged from today. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-65-P02-01 | Denial of Service | LHR machine destroyed during deploy | mitigate | Task 2 pre-tags LHR as `primary` so `fly deploy` treats it as belonging to the new group. Task 4 greps deploy log for `Destroying machine <LHR_ID>` and aborts if seen. |
| T-65-P02-02 | Information Disclosure | Secrets exposed during migration | accept | Fly secrets are app-scoped, not machine-scoped. Migration does not touch secrets. Task 7 captures `fly secrets list` count for audit. |
| T-65-P02-03 | Tampering | Accidental destroy of LHR volume | mitigate | Task 6 uses `select(.region != "lhr")` jq filter + secondary check against `LHR_VOL_ID` belt-and-braces guard. Count MUST be 7 (not 8, not 6); otherwise abort before any destroy. |
| T-65-P02-04 | Denial of Service | Replicas all fail `/readyz` after deploy (confused `sync_status`) | mitigate | Task 5 15-min observation window catches this. Task 5 action notes the `POST /sync` remediation path. If sustained: R4 rollback. |
| T-65-P02-05 | Repudiation | No audit trail for a destructive migration | mitigate | /tmp/phase65/ captures 11 artifact files (pre/post JSON, retag table, deploy log, readyz smoke log, destroy trace, volumes before/after, LHR LiteFS state). SUMMARY links them. |
| T-65-P02-06 | Elevation of Privilege | Executor runs destructive commands without explicit ack | accept | Per user decision "FULL AUTONOMOUS execution", no checkpoints inline. Each destructive action (`fly deploy`, `fly volumes destroy`) has pre-checks that abort on precondition failure, which is stronger than a human-in-the-loop ack. |
</threat_model>

<verification>
Post-plan gate:
1. `fly status --app peeringdb-plus` — 8 machines, 1 primary (lhr, shared-cpu-2x/512mb), 7 replica (other regions, shared-cpu-1x/256mb), all started.
2. `fly volumes list --app peeringdb-plus` — exactly 1 volume, region=lhr, attached.
3. `curl -sS -o /dev/null -w "%{http_code}" https://peeringdb-plus.fly.dev/readyz` — 200 from ≥3 regions (lhr + 2 others sampled).
4. Git working tree — only `.planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md` is newly added by this plan. No changes to fly.toml, litefs.yml, Go source, docs.
5. /tmp/phase65/ contains at minimum: pre-status.json, pre-machines.json, pre-volumes.json, post-retag-machines.txt, deploy-output.log, readyz-smoke.log, volumes-before-cleanup.json, destroy-trace.log, volumes-after-cleanup.json, post-status.json, post-machines.json, post-volumes.json.
</verification>

<success_criteria>
INFRA-01 (process groups):
- `fly.toml` has `[processes]` with primary + replica (landed in Plan 01)
- Production fleet: 8 machines, correct metadata, correct VM sizing per group
- Evidence: /tmp/phase65/post-machines.json

INFRA-02 (ephemeral replicas):
- 7 replica machines have zero mounts (jq assertion in Task 4)
- 7 replica volumes destroyed (Task 6)
- `/readyz` gated traffic during hydration (Task 5 readyz-smoke.log shows 503 → 200 transitions)
- Evidence: /tmp/phase65/volumes-after-cleanup.json (1 volume), readyz-smoke.log

INFRA-03 (docs + seed):
- Operational docs updated in Plan 01 (DEPLOYMENT.md, ARCHITECTURE.md, CLAUDE.md, PROJECT.md)
- SEED-002 moved to consumed/ in Plan 01
- 65-02-SUMMARY.md (this plan) closes the trail with production evidence
- Evidence: `.planning/seeds/consumed/SEED-002-*.md` exists, 65-02-SUMMARY.md ≥ 80 lines citing all three requirements
</success_criteria>

<output>
After completion, `.planning/phases/65-asymmetric-fly-fleet/65-02-SUMMARY.md` exists and is the canonical post-mortem. It references the 11 audit artifacts in /tmp/phase65/ by relative path. Commit it with message: `docs(65): Phase 65 live migration SUMMARY — asymmetric fleet active, 7 replica volumes destroyed`.

/tmp/phase65/ artifacts are migration audit trail — not committed to the repo. They can be discarded after the SUMMARY is committed; the SUMMARY preserves the material facts.
</output>
