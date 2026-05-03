---
quick_id: 260503-juu
slug: dep-updates
type: execute
wave: 1
depends_on: []
files_modified:
  - go.mod
  - go.sum
  - "ent/** (only if codegen drift — gqlgen bump risk)"
  - "gen/** (only if codegen drift)"
  - "graph/** (only if codegen drift — most likely site)"
  - "internal/web/templates/** (only if codegen drift)"
  - ".github/workflows/ci.yml (only if conditional commit 2 fires)"
autonomous: true
must_haves:
  truths:
    - "go.mod records the 6 target direct-dep versions (or the newer versions found at execute-time pre-flight)"
    - "go.sum is consistent with go.mod (go mod tidy reconciled indirect deps and checksums)"
    - "go generate ./... is a no-op on the committed tree (drift gate green)"
    - "go build ./... compiles cleanly"
    - "go vet ./... passes"
    - "go test -race ./... -count=1 passes"
    - "golangci-lint run passes"
    - "govulncheck ./... reports no findings"
    - "git log shows one or two atomic commits with kernel-style subjects (no Conventional Commits, no `chore`)"
  artifacts:
    - path: "go.mod"
      provides: "updated direct-dep versions"
      contains: "modernc.org/sqlite v1.50"
    - path: "go.sum"
      provides: "matching checksums for new + transitively-changed module versions"
    - path: ".planning/quick/260503-juu-dep-updates/260503-juu-SUMMARY.md"
      provides: "executor-authored summary; orchestrator commits in Step 8"
  key_links:
    - from: "go.mod direct-dep bumps"
      to: "go.sum"
      via: "go mod tidy"
      pattern: "go mod tidy"
    - from: "github.com/99designs/gqlgen v0.17.90 bump"
      to: "graph/generated.go (or sibling regenerated files)"
      via: "go generate ./..."
      pattern: "go generate"
    - from: "single atomic commit"
      to: "CI drift-check gate"
      via: "regenerated files committed alongside go.mod/go.sum"
      pattern: "git diff --exit-code -- ent/ gen/ graph/ internal/web/templates/"
---

<objective>
Bump 6 direct Go module dependencies to their latest patch+minor versions
within current major, reconcile indirect deps + go.sum, regenerate any
codegen drift (gqlgen is the prime suspect), and land it as a single
atomic kernel-style commit. Conditionally land a second commit if the
GH Actions audit re-verification at execute-time finds a new major has
been released since the audit on 2026-05-03.

Purpose: Routine dep-hygiene sweep follow-up to the 2026-05-03 audit.
Closes the modernization-sweep cycle; keeps the project current on
SQLite driver (prod-critical), gqlgen (codegen-driven), and the rest.

Output: One commit (`deps: bump direct module deps to latest`) updating
go.mod / go.sum and any regenerated codegen files. Optional second
commit (`ci: bump <action> to v<N>`) only if a new GH Actions major
has appeared by execute-time.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@.planning/STATE.md
@.github/workflows/ci.yml
@go.mod

# Relevant CLAUDE.md sections (reference, do not re-read whole file):
# - "Code Generation": `go generate ./...` runs ent/templ/proto pipeline; CI drift-check gate runs the same and asserts clean tree.
# - "Patch and commit message style": kernel-style `subsystem: summary phrase`; no Conventional Commits, no `chore`.
# - "CI": drift-check gate — `go generate ./...` then `git diff --exit-code -- ent/ gen/ graph/ internal/web/templates/`.
# - "Build": modernc.org/sqlite is CGo-free; SQLite driver bump (v1.49.1 → v1.50.0) is the highest-risk piece.

# Audit table (from task description, 2026-05-03 14:20Z):
# | charm.land/lipgloss/v2          | v2.0.2   → v2.0.3  | patch |
# | connectrpc.com/connect          | v1.19.1  → v1.19.2 | patch |
# | github.com/99designs/gqlgen     | v0.17.89 → v0.17.90| patch (codegen risk) |
# | github.com/klauspost/compress   | v1.18.5  → v1.18.6 | patch |
# | github.com/vektah/gqlparser/v2  | v2.5.32  → v2.5.33 | patch |
# | modernc.org/sqlite              | v1.49.1  → v1.50.0 | minor (prod-critical) |

# Pre-flight: re-run `go list -m -u all` to confirm targets are still
# accurate. Newer versions (e.g. klauspost/compress v1.18.7) may have
# landed since the audit; bump to those instead.
</context>

<tasks>

<task type="auto">
  <name>Task 1: Bump deps, reconcile go.sum, regenerate codegen, run all gates, single atomic commit</name>
  <files>go.mod, go.sum, plus any regenerated files under ent/, gen/, graph/, internal/web/templates/ if `go generate ./...` produces drift</files>
  <action>
**Single-task plan rationale:** The work is naturally one atomic unit
(bump → tidy → generate → gates → commit). Splitting breaks the
atomicity invariant the user explicitly required. Execute strictly in
the order below; each step is a hard gate before proceeding.

**Step A — Pre-flight verification (re-confirm audit targets):**

```bash
TMPDIR=/tmp/claude-1000 go list -m -u all 2>&1 | grep -E "Update available|\\[v" | head -50
```

Cross-reference the output against the audit table in `<context>`. If a
newer version has landed since 2026-05-03 14:20Z (e.g. klauspost/compress
v1.18.7 instead of v1.18.6), bump to the newer version — `go get -u`
will pick it up automatically. Do NOT downgrade to the audit version
just to match the table; the audit was a "no later than" snapshot.

If `modernc.org/sqlite` shows a NEW minor (v1.51.x) beyond v1.50.0 by
execute-time, take it; the audit only flagged v1.50.0 as the "next"
minor.

**Step B — GH Actions re-verification (drives conditional commit 2):**

The audit confirmed all 6 actions in `.github/workflows/ci.yml` are at
their latest major as of 2026-05-03. Re-verify quickly using the GitHub
API (or `gh release list` if you prefer) for each action:

```bash
for repo in actions/checkout actions/setup-go docker/build-push-action \
            docker/setup-buildx-action golangci/golangci-lint-action \
            k1LoW/octocov-action; do
  echo "=== $repo ==="
  gh release list -R "$repo" --limit 3 2>/dev/null | head -3
done
```

Record any action whose latest-major tag is now `v(N+1)` vs what's
pinned in `ci.yml`. If NONE found → no second commit. If found → after
Step F's deps commit lands cleanly, edit `ci.yml` to bump the major(s)
and create a separate `ci: bump <action> to v<N>` commit. Do NOT mix
the action bump into the deps commit — they're independent changes.

**Step C — Bump deps:**

```bash
TMPDIR=/tmp/claude-1000 go get -u ./...
```

`-u` (NOT `-u=patch`) — `-u=patch` would skip the legitimate
`modernc.org/sqlite` v1.49.1 → v1.50.0 minor bump. `-u` bumps to latest
minor/patch within the current major; major bumps require explicit
`go get module/v2@latest` and are out of scope.

Then:

```bash
TMPDIR=/tmp/claude-1000 go mod tidy
```

`go mod tidy` reconciles indirect deps and checksums. Inspect the diff:

```bash
git diff go.mod go.sum
```

Confirm: (1) the 6 direct-dep targets moved as expected (or to newer
versions), (2) the `go` directive and `toolchain` directive are
unchanged (per constraint), (3) indirect-dep churn looks reasonable
(no unexpected major-version jumps).

If the `go` or `toolchain` directives changed, revert that hunk
explicitly — they're out of scope for this task.

**Step D — Regenerate codegen (gqlgen bump is the prime suspect):**

```bash
TMPDIR=/tmp/claude-1000 go generate ./...
```

Per CLAUDE.md "Code Generation", this runs:
1. `ent/generate.go` — entc.go (ent + entgql + entrest + entproto), then
   `cmd/pdb-compat-allowlist`, then `buf generate`
2. `internal/web/templates/generate.go` — `templ generate`
3. `schema/generate.go` — schema regen from PeeringDB JSON (safe to re-run)

Check for drift:

```bash
git status -- ent/ gen/ graph/ internal/web/templates/
```

If files changed (most likely under `graph/` from the gqlgen bump),
they MUST land in the same commit as the go.mod/go.sum changes — the
CI drift-check gate (`ci.yml` lines 28-34) runs `go generate ./...`
then asserts clean tree. Drift uncommitted = CI failure.

**Step E — Run ALL gates (ALL must pass before commit):**

```bash
TMPDIR=/tmp/claude-1000 go build ./...
TMPDIR=/tmp/claude-1000 go vet ./...
TMPDIR=/tmp/claude-1000 CGO_ENABLED=1 go test -race -count=1 ./...
TMPDIR=/tmp/claude-1000 golangci-lint run
TMPDIR=/tmp/claude-1000 govulncheck ./...
```

`CGO_ENABLED=1` is required for `-race` (matches CI in `ci.yml:54`).
Full tree, no scoped runs — these are atomicity gates.

**SKIP rules on gate failure** (decide and execute, don't ask):

| Failure | Action |
|---------|--------|
| `modernc.org/sqlite` v1.50.0 (or newer minor) breaks build/test/vuln | Pin at v1.49.1 explicitly: `go get modernc.org/sqlite@v1.49.1`, re-tidy, re-generate, re-gate. Document in commit body. |
| `gqlgen` v0.17.90 produces unresolvable codegen drift (e.g. broken resolver signatures requiring hand edits beyond scope) | Revert gqlgen bump only: `go get github.com/99designs/gqlgen@v0.17.89`, also `go get github.com/vektah/gqlparser/v2@v2.5.32` if it churned, re-tidy, re-generate, re-gate. Document in commit body. |
| `govulncheck` flags a NEW finding from a bumped dep | Identify the offending dep from the report, revert just that one to its pre-bump version, re-tidy, re-gate. Document in commit body. |
| Generic Go-side breakage (any other dep) — quick fix possible | Fix in same commit if obviously trivial (e.g. one-line API rename). Otherwise revert just that dep, document. |
| Multiple deps fail simultaneously | Revert ALL bumps (`git checkout go.mod go.sum`), bisect by re-introducing one at a time. If the bisect identifies one bad dep, land the others; if multiple are bad, land the safe subset and document the rest. |

After any SKIP-rule revert, re-run the full gate suite (Step E) before
proceeding to commit.

**Step F — Single atomic commit:**

Compose the message:

```
deps: bump direct module deps to latest

Refresh of direct module dependencies as a follow-up to the 2026-05-03
modernization sweep audit. Picks up patch fixes across the GraphQL
codegen stack and the SQLite driver minor; no API changes observed.

Direct deps moved (audit 2026-05-03 14:20Z; pre-flight at execute time
re-confirms / picks up newer where available):

  charm.land/lipgloss/v2          v2.0.2   -> v2.0.3
  connectrpc.com/connect          v1.19.1  -> v1.19.2
  github.com/99designs/gqlgen     v0.17.89 -> v0.17.90
  github.com/klauspost/compress   v1.18.5  -> v1.18.6
  github.com/vektah/gqlparser/v2  v2.5.32  -> v2.5.33
  modernc.org/sqlite              v1.49.1  -> v1.50.0

go mod tidy reconciled the indirect-dep set and go.sum.
[Insert ONE of:
  - "go generate ./... is a no-op against the bumped tree; no codegen
    drift required regeneration."
  OR
  - "go generate ./... regenerated <files>; the diff is included
    here so the CI drift-check gate stays green."
]

GH Actions in .github/workflows/ci.yml were verified at latest-major
[at audit time AND at execute time]; no edit needed in this commit.
[If conditional commit 2 fires, omit the GH Actions paragraph above
and instead say: "GH Actions audit at execute-time found a new major
for <action>; bumped in a follow-up commit to keep dep-bump and
CI-config changes independently reviewable."]

All gates green: build, vet, test -race full tree, golangci-lint,
govulncheck.
```

Wrap body lines at ~74 columns. Subject is 41 columns (well under 50).
Plain text only, no links, no Conventional Commits, no `chore`.

Stage and commit (use `Bash` with HEREDOC per the standard convention,
not interactive editor):

```bash
git add go.mod go.sum
# Plus any regenerated dirs IF Step D produced drift:
git add ent/ gen/ graph/ internal/web/templates/ 2>/dev/null || true

git commit -m "$(cat <<'EOF'
deps: bump direct module deps to latest

[body as composed above]
EOF
)"
```

Do NOT use `-A` or `git add .` — only stage the files this task is
allowed to touch (go.mod, go.sum, plus regenerated dirs from Step D).

**Step G — Conditional commit 2 (GH Actions, only if Step B found a
new major):**

```bash
# Edit ci.yml: replace each affected line. Example:
# - uses: actions/checkout@v6
# + uses: actions/checkout@v7
# (only the lines flagged in Step B)

git add .github/workflows/ci.yml
git commit -m "$(cat <<'EOF'
ci: bump <action> to v<N>

GH Actions <action> released v<N> after the 2026-05-03 modernization
audit. Pin matches the project pattern of major-only references so
patch updates continue to roll automatically.

No expected behaviour change at the major boundary; release notes
indicate <one-line summary from gh release view>.
EOF
)"
```

If Step B found NO new major, skip Step G entirely.

**Step H — Final verification:**

```bash
git log --oneline -3
git status
```

Confirm: 1 or 2 new commits (deps, optionally ci), clean working tree.
Re-run the full gate suite one more time on the post-commit tree as a
sanity check:

```bash
TMPDIR=/tmp/claude-1000 go build ./... && \
TMPDIR=/tmp/claude-1000 go vet ./... && \
TMPDIR=/tmp/claude-1000 CGO_ENABLED=1 go test -race -count=1 ./... && \
TMPDIR=/tmp/claude-1000 golangci-lint run && \
TMPDIR=/tmp/claude-1000 govulncheck ./... && \
echo "ALL GATES GREEN"
```

**Constraints reminder (do NOT touch):**

- `.golangci.yml` — out of scope.
- `CLAUDE.md` — out of scope (per CLAUDE.md "Documentation" section,
  it's Claude project memory, not user docs).
- `go.mod`'s `go` directive and `toolchain` directive — out of scope;
  if `go get -u` perturbs them, revert that hunk explicitly.
- Anything outside go.mod / go.sum / regenerated codegen / (conditional)
  ci.yml.

**Use `-u` not `-u=patch`** — `-u=patch` would skip the legitimate
`modernc.org/sqlite` v1.49.1 → v1.50.0 minor bump.

**Shell environment reminder** — Bash tool runs under `zsh` which
performs history expansion on `!`. Avoid `if ! cmd; then` and `! cmd1
| cmd2`. Use count-based forms: `test "$(cmd | grep -c X)" -eq 0`.
  </action>
  <verify>
<automated>
TMPDIR=/tmp/claude-1000 bash -c '
set -e
go build ./...
go vet ./...
CGO_ENABLED=1 go test -race -count=1 ./...
golangci-lint run
govulncheck ./...
go generate ./...
test "$(git status --porcelain -- ent/ gen/ graph/ internal/web/templates/ go.mod go.sum | wc -l)" -eq 0
git log -1 --pretty=%s | grep -E "^deps: " >/dev/null
echo OK
'
</automated>
  </verify>
  <done>
- go.mod records the 6 target versions (or newer found at pre-flight)
- go.sum is reconciled and consistent with go.mod
- `go generate ./...` produces no drift on the committed tree
- All 5 gates pass: build, vet, test -race, golangci-lint, govulncheck
- Single commit `deps: bump direct module deps to latest` lands cleanly
- (Optional) Second commit `ci: bump <action> to v<N>` lands ONLY if
  Step B's re-verification found a new GH Actions major; absent
  otherwise
- No edits outside go.mod / go.sum / regenerated codegen dirs /
  (conditional) ci.yml
- The `go` and `toolchain` directives in go.mod are unchanged
  </done>
</task>

</tasks>

<verification>
Phase-level checks (executed at end of Task 1, Step H):

1. `git log --oneline -3` shows the deps commit (and optionally the ci
   commit), authored by the user, with kernel-style subjects.

2. `git diff HEAD~1 -- go.mod | grep -c "^+	" | grep -E "^[1-9]"` —
   confirms go.mod has at least one `+` line (sanity that the diff
   actually moved versions).

3. Final clean-tree gate suite (build, vet, test -race full tree,
   golangci-lint, govulncheck) all green.

4. `go generate ./...` re-run after commit produces zero diff.

5. SUMMARY.md authored by executor lives at
   `.planning/quick/260503-juu-dep-updates/260503-juu-SUMMARY.md`. The
   ORCHESTRATOR commits it in its own `docs(quick-260503-juu): ...`
   commit at Step 8 — executor does NOT commit SUMMARY.md.
</verification>

<success_criteria>
Quick task complete when:

- [ ] go.mod records the bumped versions for all 6 direct deps (or
      newer where pre-flight found newer)
- [ ] go.sum reconciled via `go mod tidy`
- [ ] `go generate ./...` produces zero drift on the committed tree
      (CI drift-check gate green)
- [ ] All 5 gates green (build, vet, test -race full tree, lint, vuln)
- [ ] Exactly 1 atomic commit on the deps work, kernel-style subject
      `deps: bump direct module deps to latest`, body wrapped ~74 cols,
      plain text, no links, no Conventional Commits, no `chore`
- [ ] Optionally exactly 1 second commit `ci: bump <action> to v<N>`
      iff Step B found a new GH Actions major
- [ ] Working tree clean post-commit
- [ ] SUMMARY.md authored (orchestrator commits separately at Step 8)
- [ ] Untouched: `.golangci.yml`, `CLAUDE.md`, `go` directive,
      `toolchain` directive, anything outside go.mod / go.sum /
      regenerated codegen / (conditional) ci.yml
</success_criteria>

<output>
After completion, create
`.planning/quick/260503-juu-dep-updates/260503-juu-SUMMARY.md`
following the standard summary template. Include:

- Final list of direct-dep versions actually landed (may differ from
  audit table if pre-flight found newer)
- Whether `go generate ./...` produced drift, and which dirs
  regenerated
- Whether SKIP rules fired (and which dep was reverted, if any)
- Whether conditional commit 2 fired (and which action(s) bumped)
- Gate outcomes (build, vet, test -race counts, lint, vuln)
- Commit SHAs (deps commit, optionally ci commit)
</output>
