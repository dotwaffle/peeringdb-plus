---
phase: quick-260426-mei
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - deploy/grafana/alerts/pdbplus-alerts.yaml
  - deploy/grafana/alerts/README.md
  - deploy/grafana/alerts_test.go
  - deploy/grafana/dashboards/pdbplus-overview.json
  - CLAUDE.md
autonomous: true
requirements:
  - QUICK-260426-mei
must_haves:
  truths:
    - "Alert rule YAML exists at deploy/grafana/alerts/pdbplus-alerts.yaml with ≤8 rules across critical and warning severity tiers"
    - "Every alert rule has severity label, summary annotation, description annotation, for: duration, and binds receiver: grafana-default-email"
    - "No file in the repo contains the user's email address or any *.grafana.net URL after this plan completes"
    - "deploy/grafana/alerts/README.md documents apply mechanism (mimirtool rules sync), local lint (mimirtool/promtool), severity policy, and forbidden-content rules"
    - "deploy/grafana/alerts_test.go validates YAML well-formedness, required labels and annotations, and shells to promtool/mimirtool with t.Skip() when absent"
    - "Dashboard panel id=35 query is sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name=\"peeringdb-plus\"}) and renders for all 8 instances (primary + 7 replicas)"
    - "CLAUDE.md Sync observability section gains a 3-5 line paragraph explaining OTel runtime contrib coexistence with sync-watermark metrics, alert location, and apply mechanism"
  artifacts:
    - path: "deploy/grafana/alerts/pdbplus-alerts.yaml"
      provides: "Prometheus rule groups for critical/warning alerting on sync freshness, sync failure rate, fleet count, heap, RSS"
      contains: "groups:"
    - path: "deploy/grafana/alerts/README.md"
      provides: "Apply/lint/policy documentation for alert rules"
      min_lines: 40
    - path: "deploy/grafana/alerts_test.go"
      provides: "YAML schema + invariants validation; optional promtool/mimirtool exec gates"
      contains: "package grafana_test"
    - path: "deploy/grafana/dashboards/pdbplus-overview.json"
      provides: "Panel 35 re-sourced to go_memory_used_bytes for per-instance live heap"
      contains: "go_memory_used_bytes{service_name=\"peeringdb-plus\"}"
    - path: "CLAUDE.md"
      provides: "Documentation of OTel runtime contrib metrics + alert location"
      contains: "go.opentelemetry.io/contrib/instrumentation/runtime"
  key_links:
    - from: "deploy/grafana/alerts/pdbplus-alerts.yaml"
      to: "Grafana Cloud receiver grafana-default-email"
      via: "rule labels.receiver"
      pattern: "receiver:\\s*grafana-default-email"
    - from: "deploy/grafana/alerts_test.go"
      to: "deploy/grafana/alerts/pdbplus-alerts.yaml"
      via: "yaml.Unmarshal in TestAlerts_ValidYAML"
      pattern: "alerts/pdbplus-alerts.yaml"
    - from: "deploy/grafana/dashboards/pdbplus-overview.json (panel 35)"
      to: "OTel runtime contrib metric go_memory_used_bytes"
      via: "PromQL targets[].expr"
      pattern: "go_memory_used_bytes\\{service_name=\\\"peeringdb-plus\\\"\\}"
---

<objective>
Wire production observability for PeeringDB Plus by authoring Grafana Cloud alert rules in version-controlled Prometheus YAML, re-sourcing dashboard panel 35 to the already-emitting OTel runtime contrib gauge, and documenting the apply mechanism. NO new Go gauge code — `go.opentelemetry.io/contrib/instrumentation/runtime` v0.68.0 is already wired at `internal/otel/provider.go:121` and emits `go_memory_used_bytes` from every instance with the GC-allowlisted `service_namespace` + `cloud_region` labels (post quick task 260426-lod).

Purpose: Close two audit gaps without adding code surface — (Gap A) operators have no paging signal on sync stalls, sync failures, fleet-size drops, or sustained heap pressure; (Gap B) dashboard panel 35 only shows `primary` because the underlying `pdbplus_sync_peak_heap_bytes` metric only fires from sync-completion paths and replicas don't sync.

Output: Alert YAML + README + test + dashboard re-source + CLAUDE.md paragraph. Manual GC sync (`mimirtool rules sync`) is documented but executed by the operator out-of-band — the planner cannot run it from here.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@.planning/quick/260426-mei-production-observability-alerts-per-inst/260426-mei-CONTEXT.md
@.planning/seeds/SEED-001-incremental-sync-evaluation.md
@deploy/grafana/dashboard_test.go
@deploy/grafana/dashboards/pdbplus-overview.json
@deploy/grafana/provisioning/dashboards.yaml

<interfaces>
<!-- Verified before planning. Executor should NOT re-explore. -->

Existing telemetry surface (post quick task 260426-lod):
- `internal/otel/provider.go:121` calls `runtime.Start(runtime.WithMeterProvider(mp))` — this is the source of `go_memory_used_bytes`, `go_goroutine_count`, `go_gc_duration_seconds`, etc.
- Resource attrs promoted to Prometheus labels by GC's OTLP receiver (verified live):
  - `service_name="peeringdb-plus"`
  - `service_namespace` ∈ {primary, replica}
  - `cloud_region` ∈ {lhr, iad, syd, ...}
  - `go_memory_type` ∈ {stack, other} — sum across this in dashboard queries
- Sync-watermark metrics (UNCHANGED — keep as-is): `pdbplus_sync_peak_heap_bytes`, `pdbplus_sync_peak_rss_bytes`
- Sync-cycle metrics referenced by alerts: `pdbplus_sync_freshness_seconds`, `pdbplus_sync_operations_total{status=...}`

Dashboard panel 35 current state (deploy/grafana/dashboards/pdbplus-overview.json:2050-2116):
- id: 35
- title: "Peak Heap by Process Group"
- expr: `sum by(service_namespace, cloud_region)(pdbplus_sync_peak_heap_bytes)` (sync-watermark — broken for replicas)
- legendFormat: `{{service_namespace}} / {{cloud_region}}`
- unit: bytes
- description references `pdbplus_sync_peak_heap_bytes` — must be updated

Testing toolchain (verified):
- `prometheus/prometheus` is NOT in go.mod → no in-tree PromQL parser. Use exec-based gates only.
- `mimirtool` and `promtool` are NOT in PATH and NOT registered as `go tool` directives. Treat both as OPTIONAL external binaries — `exec.LookPath` + `t.Skip()` when absent.
- `go.opentelemetry.io/contrib/instrumentation/runtime v0.68.0` IS in go.sum (transitive via `internal/otel/provider.go`). No new direct import needed.

Existing test patterns to mirror (deploy/grafana/dashboard_test.go):
- `package grafana_test`
- One `loadX` helper, multiple `TestX_Y` table-style assertions
- `t.Parallel()` at top of each test
- Read fixture file with `os.ReadFile` + relative path
- `t.Fatalf` on file-read errors, `t.Errorf` on assertion failures
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create alert rule YAML and README</name>
  <files>deploy/grafana/alerts/pdbplus-alerts.yaml, deploy/grafana/alerts/README.md</files>
  <action>
Create the `deploy/grafana/alerts/` directory with two files.

**File 1: `deploy/grafana/alerts/pdbplus-alerts.yaml`** — standard Prometheus rule-group schema with two groups, total 6 rules (under the 8-rule cap with 2 spare slots for future additions):

```yaml
# PeeringDB Plus — Grafana Cloud alert rules.
# Apply with: mimirtool rules sync deploy/grafana/alerts/pdbplus-alerts.yaml
# See deploy/grafana/alerts/README.md for the full apply / lint workflow.
#
# Receiver routing is configured out-of-band in the Grafana Cloud notifications UI.
# Both severity tiers bind the canonical default contact point name.
#
# DO NOT add email addresses, *.grafana.net stack URLs, or tenant identifiers
# to this file. See README.md "Forbidden content" for the rationale.

groups:
  - name: pdbplus-critical
    interval: 1m
    rules:
      - alert: PdbPlusSyncFreshnessHigh
        expr: max(pdbplus_sync_freshness_seconds) > 7200
        for: 5m
        labels:
          severity: critical
          receiver: grafana-default-email
        annotations:
          summary: "PeeringDB Plus sync data is stale (>2h)"
          description: |
            max(pdbplus_sync_freshness_seconds) has exceeded 7200s (2h)
            for 5m, which is 8× the 15-minute authenticated sync cadence.
            Replicas serve stale data — check primary sync worker logs and
            PeeringDB upstream availability.

      - alert: PdbPlusSyncFailureRateHigh
        expr: |
          sum(rate(pdbplus_sync_operations_total{status="failed"}[15m]))
            /
          clamp_min(sum(rate(pdbplus_sync_operations_total[15m])), 1e-9)
            > 0.5
        for: 15m
        labels:
          severity: critical
          receiver: grafana-default-email
        annotations:
          summary: "PeeringDB Plus sync failure rate >50% over 15m"
          description: |
            More than half of pdbplus_sync_operations_total samples in the
            last 15 minutes carry status=failed. Check sync worker logs on
            the primary VM. The clamp_min guards divide-by-zero on quiet
            windows.

      - alert: PdbPlusFleetMachineCountLow
        expr: |
          count(
            count by (service_namespace, cloud_region) (
              go_memory_used_bytes{service_name="peeringdb-plus"}
            )
          ) < 6
        for: 10m
        labels:
          severity: critical
          receiver: grafana-default-email
        annotations:
          summary: "PeeringDB Plus fleet machine count <6 (expected 8)"
          description: |
            Counted distinct (service_namespace, cloud_region) pairs reporting
            go_memory_used_bytes for service_name=peeringdb-plus is below 6
            for 10m. Expected fleet is 8 machines (1 primary LHR + 7
            replicas). Check Fly machine status: fly status -a peeringdb-plus.

  - name: pdbplus-warning
    interval: 1m
    rules:
      - alert: PdbPlusHeapHigh
        expr: max(pdbplus_sync_peak_heap_bytes) > 419430400
        for: 30m
        labels:
          severity: warning
          receiver: grafana-default-email
        annotations:
          summary: "PeeringDB Plus peak Go heap >400 MiB sustained 30m (SEED-001 trigger)"
          description: |
            max(pdbplus_sync_peak_heap_bytes) has been above 419430400 bytes
            (400 MiB) for 30m. SEED-001 incremental-sync evaluation trigger
            has fired — surface at next milestone planning. Observed
            baseline 2026-04-17 was 83.8 MiB primary, 58-59 MiB replicas.

      - alert: PdbPlusRssHigh
        expr: max(pdbplus_sync_peak_rss_bytes) > 402653184
        for: 30m
        labels:
          severity: warning
          receiver: grafana-default-email
        annotations:
          summary: "PeeringDB Plus peak OS RSS >384 MiB sustained 30m"
          description: |
            max(pdbplus_sync_peak_rss_bytes) has been above 402653184 bytes
            (384 MiB) for 30m. Approaching the Fly 512 MB VM cap; expected
            failure order is log → app crash → Fly OOM-kill.

      - alert: PdbPlusSyncOperationFailed
        expr: increase(pdbplus_sync_operations_total{status="failed"}[1h]) > 0
        for: 5m
        labels:
          severity: warning
          receiver: grafana-default-email
        annotations:
          summary: "PeeringDB Plus had at least one failed sync in the last hour"
          description: |
            increase(pdbplus_sync_operations_total{status=failed}[1h]) > 0.
            Single failure is non-paging but worth a notify; investigate
            primary sync worker logs for transient PeeringDB upstream issues.
```

**File 2: `deploy/grafana/alerts/README.md`** — apply/lint/policy documentation:

```markdown
# PeeringDB Plus — Grafana Cloud Alert Rules

Source-of-truth Prometheus rule groups for the PeeringDB Plus production
deployment. The repository holds the rule definitions; apply is a manual
operator step (same convention as `deploy/grafana/dashboards/`).

## Schema

Standard Prometheus alerting-rule format. Reference:
https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/

Each rule has:

- `alert:` — PascalCase name with `PdbPlus` prefix.
- `expr:` — PromQL expression.
- `for:` — sustained-breach window before the alert fires.
- `labels.severity:` — one of `critical` (page tier) or `warning`
  (notify-only tier).
- `labels.receiver:` — always the literal string `grafana-default-email`.
  This is the canonical default contact-point name in every Grafana Cloud
  stack; it is **not** an email address.
- `annotations.summary:` — one-line operator summary.
- `annotations.description:` — multi-line operator-actionable detail
  including the threshold value and the contributing metric name.

## Apply

The repository is the source of truth. Sync to Grafana Cloud Mimir with
`mimirtool` (install: https://grafana.com/docs/mimir/latest/manage/tools/mimirtool/):

```bash
# Replace placeholder env vars with your stack credentials before running.
# DO NOT commit real credentials or stack URLs into examples.
export MIMIR_ADDRESS="$YOUR_MIMIR_RULER_URL"
export MIMIR_TENANT_ID="$YOUR_TENANT_ID"
export MIMIR_API_KEY="$YOUR_API_KEY"

mimirtool rules sync deploy/grafana/alerts/pdbplus-alerts.yaml
```

Use `mimirtool rules load` if you prefer load-and-replace semantics over
diff-and-sync.

## Lint locally

Either tool is sufficient; both are external binaries (not in the Go
toolchain). The repository test (`alerts_test.go`) shells to whichever is
on `PATH` and skips gracefully when neither is installed.

```bash
mimirtool rules check deploy/grafana/alerts/pdbplus-alerts.yaml
promtool check rules deploy/grafana/alerts/pdbplus-alerts.yaml
```

Test invocation:

```bash
go test -race ./deploy/grafana/...
```

## Notification routing

The `severity` label is the routing knob. Configure tier behaviour in your
Grafana Cloud notifications UI (the receiver `grafana-default-email` is
where both tiers land at the receiver level; separate routes can fan out
based on `severity=critical` vs `severity=warning`).

## Severity tier policy

| Tier       | Behaviour       | Used for                                                         |
|------------|-----------------|------------------------------------------------------------------|
| `critical` | Page on-call    | Sync stalls (>2h freshness), sync-failure rate >50%, fleet drop. |
| `warning`  | Notify only     | Heap/RSS sustained breach, single sync failure.                  |

Total rule count is capped at 8 to stay below Grafana Cloud free-tier
alertmanager limits. Current count: 6 rules across both groups.

## Forbidden content

The following MUST NOT appear in any file in `deploy/grafana/alerts/`:

- Email addresses (any user, any domain).
- `*.grafana.net` URLs or any other stack-specific URL.
- Tenant identifiers, API keys, or any credential value.

The repository is a public-style source of truth; tenant-specific
configuration lives in operator-managed environment variables passed to
`mimirtool` at apply time.
```

Constraints to verify by inspection before saving:
- Total rule count = 6 (within ≤8 cap).
- Every rule has `severity`, `receiver: grafana-default-email`, `summary`, `description`, `for:`.
- No email address, no `*.grafana.net` URL, no tenant identifier in either file.
- Threshold values match SEED-001 (`PDBPLUS_HEAP_WARN_MIB=400` → 419430400 bytes; `PDBPLUS_RSS_WARN_MIB=384` → 402653184 bytes).
- Fleet-count expression uses `count by (service_namespace, cloud_region)` to deduplicate the `go_memory_type` dimension; bare `count(go_memory_used_bytes{...})` would over-count by ~2× because the metric is split across `stack` / `other`.
  </action>
  <verify>
    <automated>test -f deploy/grafana/alerts/pdbplus-alerts.yaml &amp;&amp; test -f deploy/grafana/alerts/README.md &amp;&amp; ! grep -rE '@(gmail|grafana\.net)|\.grafana\.net' deploy/grafana/alerts/ &amp;&amp; grep -c '^      - alert:' deploy/grafana/alerts/pdbplus-alerts.yaml | grep -qE '^[1-8]$'</automated>
  </verify>
  <done>Both files exist; YAML contains exactly 6 alert rules across 2 groups; no forbidden content (email, *.grafana.net) anywhere under deploy/grafana/alerts/; every rule binds `receiver: grafana-default-email`.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Add alerts_test.go validating YAML schema and invariants</name>
  <files>deploy/grafana/alerts_test.go</files>
  <behavior>
    - Test 1 (`TestAlerts_ValidYAML`): parses `deploy/grafana/alerts/pdbplus-alerts.yaml` into a typed struct via `gopkg.in/yaml.v3`; fails on parse error or zero rule groups.
    - Test 2 (`TestAlerts_RequiredFields`): every rule has non-empty `Alert`, `Expr`, `For`, `Labels.Severity` ∈ {critical, warning}, `Labels.Receiver == "grafana-default-email"`, non-empty `Annotations.Summary`, non-empty `Annotations.Description`.
    - Test 3 (`TestAlerts_RuleCountUnderCap`): total rule count ≤ 8.
    - Test 4 (`TestAlerts_NoForbiddenContent`): raw bytes of YAML and README contain neither `@gmail.com` nor `.grafana.net`.
    - Test 5 (`TestAlerts_PromtoolCheck`): if `exec.LookPath("promtool")` fails, `t.Skip()`; otherwise runs `promtool check rules <yaml>` and fails on non-zero exit. Mirrored Test 6 for `mimirtool rules check`.
    - Test 7 (`TestAlerts_NamesPascalCasePdbPlusPrefix`): every rule name matches `^PdbPlus[A-Z][A-Za-z]+$`.
  </behavior>
  <action>
Create `deploy/grafana/alerts_test.go` mirroring the structure of `deploy/grafana/dashboard_test.go` (`package grafana_test`, table-driven, `t.Parallel()` per test).

Use `gopkg.in/yaml.v3` for unmarshalling — it is already a transitive dep (via OTel and entgo). If a `go test` run fails to resolve it, add the import and run `go mod tidy` (it is already in `go.sum`; verify with `grep '^gopkg.in/yaml.v3' go.sum` before importing).

Schema struct skeleton:

```go
type alertFile struct {
    Groups []alertGroup `yaml:"groups"`
}

type alertGroup struct {
    Name     string      `yaml:"name"`
    Interval string      `yaml:"interval"`
    Rules    []alertRule `yaml:"rules"`
}

type alertRule struct {
    Alert       string            `yaml:"alert"`
    Expr        string            `yaml:"expr"`
    For         string            `yaml:"for"`
    Labels      map[string]string `yaml:"labels"`
    Annotations map[string]string `yaml:"annotations"`
}
```

Helper `loadAlerts(tb testing.TB) alertFile` — note `testing.TB` widening so future benchmarks (if added) can reuse it (matches Phase 72 parity pattern).

Path constant: `const alertsPath = "alerts/pdbplus-alerts.yaml"`. README path constant for the forbidden-content test: `const alertsReadmePath = "alerts/README.md"`.

For `TestAlerts_PromtoolCheck` and the mimirtool sibling: use `exec.CommandContext(ctx, ...)` with a 10s context (`context.WithTimeout(t.Context(), 10*time.Second)` if the project uses `t.Context()`, else `context.Background`). Write a comment in the test body noting the binaries are not in `go tool` and not in the project's go.mod — they are operator-installed and the tests skip when absent.

For the forbidden-content regex, use `bytes.Contains` rather than regex to keep the test fast and unambiguous: check for literal `[]byte("@gmail.com")`, `[]byte("@anthropic.com")`, `[]byte(".grafana.net")`. The `Co-Authored-By: Claude` line in commits never lands in these files but the assertion is cheap insurance.

Cyclomatic complexity per `~/.dotfiles/common/.claude/rules/go-guidelines.md` GO-CS-3: keep helpers small. The rule-iteration loop should be a single `range` over `f.Groups` then `range g.Rules` — no nested conditionals beyond required-field checks.

Logging convention per GO-OBS-1: use `t.Errorf` with structured-style messages (`"rule %q: missing %s annotation", r.Alert, "summary"`).
  </action>
  <verify>
    <automated>cd deploy/grafana &amp;&amp; go test -race -count=1 -run '^TestAlerts_' .</automated>
  </verify>
  <done>`go test -race ./deploy/grafana/...` passes. `TestAlerts_*` covers YAML well-formedness, required fields, ≤8 rule cap, forbidden-content, naming convention. promtool/mimirtool exec gates skip cleanly when binaries are absent (verified locally — neither is installed).</done>
</task>

<task type="auto">
  <name>Task 3: Re-source dashboard panel 35 to OTel runtime contrib gauge</name>
  <files>deploy/grafana/dashboards/pdbplus-overview.json</files>
  <action>
Edit `deploy/grafana/dashboards/pdbplus-overview.json` panel id=35 (lines ~2050-2116). Change exactly four fields; leave panel grid position, fieldConfig, options, and id untouched.

Diff plan (use Edit tool with surgical string replacements — do NOT rewrite the file):

1. **Title** (line ~2053): `"Peak Heap by Process Group"` → `"Live Heap by Instance"`.

2. **Description** (line ~2054): replace with —
   ```
   "Live Go heap usage per instance from the OTel runtime contrib package (go.opentelemetry.io/contrib/instrumentation/runtime, semconv v1.36.0 goconv naming). Sums across go_memory_type (stack + other) so each line is one VM. Coexists with pdbplus_sync_peak_heap_bytes (sync-cycle watermark on primary only) — this gauge ticks per OTel push interval on every instance. Bytes unit; Grafana auto-formats MiB / GiB."
   ```

3. **`targets[0].expr`** (line ~2069): `"sum by(service_namespace, cloud_region)(pdbplus_sync_peak_heap_bytes)"` → `"sum by (service_namespace, cloud_region) (go_memory_used_bytes{service_name=\"peeringdb-plus\"})"`.

4. **`targets[0].legendFormat`** (line ~2070): `"{{service_namespace}} / {{cloud_region}}"` → `"{{service_namespace}}/{{cloud_region}}"` (tighter label per CONTEXT.md task spec — no spaces around the slash).

After edits, validate JSON well-formedness with `jq` and confirm the existing `dashboard_test.go` still passes (`TestDashboard_MetricNameReferences` already expects `go_memory_used_bytes` so the re-source strengthens, not weakens, the assertion).

Do NOT add a new panel for `go_goroutine_count` or `go_gc_duration_seconds` — CONTEXT.md explicitly defers that.
  </action>
  <verify>
    <automated>jq -e '.panels[] | select(.id == 35) | select(.title == "Live Heap by Instance") | .targets[0].expr | test("go_memory_used_bytes\\{service_name=\"peeringdb-plus\"\\}")' deploy/grafana/dashboards/pdbplus-overview.json &amp;&amp; cd deploy/grafana &amp;&amp; go test -race -count=1 -run '^TestDashboard_' .</automated>
  </verify>
  <done>Panel id=35 has the new title, description, expr, and legendFormat. `jq` parses the file. `dashboard_test.go` (all `TestDashboard_*` tests) still passes — the existing `TestDashboard_MetricNameReferences` requirement for `go_memory_used_bytes` is now satisfied by panel 35 in addition to the pre-existing panel that referenced it.</done>
</task>

<task type="auto">
  <name>Task 4: CLAUDE.md — append OTel runtime contrib paragraph to Sync observability</name>
  <files>CLAUDE.md</files>
  <action>
Append a 4-5 line paragraph to the **Sync observability** section of `CLAUDE.md` (between line ~316 "Dashboard." paragraph and line ~318 "OTel resource attributes (post-260426-lod)." block — placement matters because the new paragraph references the GC label-allowlist context in the block immediately below).

Text to insert (use Edit tool to add a new paragraph after the existing `**Dashboard.**` line):

```
**OTel runtime metrics.** `go.opentelemetry.io/contrib/instrumentation/runtime` is wired at `internal/otel/provider.go` (`runtime.Start(...)`) and emits per-instance `go_memory_used_bytes`, `go_goroutine_count`, `go_gc_duration_seconds` (semconv v1.36.0 `goconv` naming, Prom-translated). These are LIVE tick gauges on every machine; they coexist with the `pdbplus_sync_peak_*` sync-cycle watermarks (primary only). Dashboard panel 35 ("Live Heap by Instance") sources from `go_memory_used_bytes` so all 8 fleet machines plot. Production alert rules live in `deploy/grafana/alerts/pdbplus-alerts.yaml` and apply via `mimirtool rules sync` (see `deploy/grafana/alerts/README.md` for the workflow).
```

Constraints:
- Insert as a single paragraph (no sub-bullets).
- DO NOT mention the user's email, the receiver name `grafana-default-email`, or any GC stack URL — those are operator-side concerns documented in the alerts README.
- DO NOT alter any other section. The "OTel resource attributes (post-260426-lod)" block immediately below stays unchanged.
  </action>
  <verify>
    <automated>grep -F 'go.opentelemetry.io/contrib/instrumentation/runtime' CLAUDE.md &amp;&amp; grep -F 'deploy/grafana/alerts/pdbplus-alerts.yaml' CLAUDE.md &amp;&amp; ! grep -E '@(gmail|anthropic)\.com|\.grafana\.net|grafana-default-email' CLAUDE.md</automated>
  </verify>
  <done>CLAUDE.md gains the new paragraph in the Sync observability section. Grep confirms presence of `go.opentelemetry.io/contrib/instrumentation/runtime` and `deploy/grafana/alerts/pdbplus-alerts.yaml`, AND absence of email/stack-URL/receiver-name. The "OTel resource attributes" block immediately below is untouched.</done>
</task>

</tasks>

<verification>
End-of-plan checks (all four tasks complete):

```bash
# 1. Alert files exist and are well-formed.
test -f deploy/grafana/alerts/pdbplus-alerts.yaml
test -f deploy/grafana/alerts/README.md
test -f deploy/grafana/alerts_test.go

# 2. Forbidden content is absent EVERYWHERE the plan touched.
! grep -rE '@(gmail|anthropic)\.com|\.grafana\.net' \
    deploy/grafana/alerts/ \
    deploy/grafana/dashboards/pdbplus-overview.json \
    CLAUDE.md

# 3. Tests pass.
cd deploy/grafana && go test -race -count=1 .

# 4. Dashboard JSON is valid.
jq -e '.panels[] | select(.id == 35) | .targets[0].expr | test("go_memory_used_bytes")' \
    deploy/grafana/dashboards/pdbplus-overview.json

# 5. Rule count under cap.
grep -c '^      - alert:' deploy/grafana/alerts/pdbplus-alerts.yaml | awk '$1 <= 8 {exit 0} {exit 1}'

# 6. Lint (optional — skipped if binaries absent).
command -v promtool && promtool check rules deploy/grafana/alerts/pdbplus-alerts.yaml || echo "promtool not installed; rely on alerts_test.go"
command -v mimirtool && mimirtool rules check deploy/grafana/alerts/pdbplus-alerts.yaml || echo "mimirtool not installed; apply step is operator-side"
```

Manual operator step (NOT executed by Claude):

```bash
mimirtool rules sync deploy/grafana/alerts/pdbplus-alerts.yaml
```

This is documented in `deploy/grafana/alerts/README.md` for the operator to run with their own credentials.
</verification>

<success_criteria>
- 6 alert rules in `deploy/grafana/alerts/pdbplus-alerts.yaml` across critical (3) and warning (3) tiers, all binding `receiver: grafana-default-email`.
- `deploy/grafana/alerts/README.md` documents apply / lint / severity / forbidden-content policy.
- `deploy/grafana/alerts_test.go` passes under `go test -race`, validates YAML schema and invariants, gracefully skips when promtool/mimirtool absent.
- Dashboard panel id=35 renders for all 8 instances via `go_memory_used_bytes{service_name="peeringdb-plus"}`; `dashboard_test.go` still passes.
- `CLAUDE.md` Sync observability section gains a single paragraph documenting the OTel runtime contrib coexistence + alert location.
- `grep` confirms NO email address, NO `*.grafana.net` URL, NO tenant identifier anywhere across the touched files.
- `git status` shows exactly 5 changed/new files: 3 new (`alerts/pdbplus-alerts.yaml`, `alerts/README.md`, `alerts_test.go`) + 2 modified (`pdbplus-overview.json`, `CLAUDE.md`).
</success_criteria>

<output>
After completion, create `.planning/quick/260426-mei-production-observability-alerts-per-inst/260426-mei-SUMMARY.md` documenting:
- What was built (the 5 file deltas).
- The deliberately-deferred items (additional dashboard panels for `go_goroutine_count` and `go_gc_duration_seconds`; runbook URLs; Grafana 10+ dashboard alert annotations) — quote CONTEXT.md as the source of the deferral.
- The single manual operator follow-up: `mimirtool rules sync deploy/grafana/alerts/pdbplus-alerts.yaml` against the user's GC stack.
- Verification snapshot: rule count, test pass, jq validation result.
</output>
