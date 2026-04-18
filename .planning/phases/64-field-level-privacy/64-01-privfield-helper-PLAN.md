---
phase: 64-field-level-privacy
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/privfield/privfield.go
  - internal/privfield/privfield_test.go
  - internal/privfield/doc.go
autonomous: true
requirements:
  - VIS-08
tags:
  - privacy
  - visibility
  - helper-package

must_haves:
  truths:
    - "A single call to privfield.Redact(ctx, visible, value) returns (value, false) when the caller tier admits the field, and (\"\", true) when the field must be omitted."
    - "Unstamped ctx (no Tier plumbed) redacts by default (fail-closed per D-03)."
    - "All four admission rules hold: visible=Public always admits; visible=Users admits iff tier>=TierUsers; visible=Private always redacts; unknown visible string redacts."
    - "Package has godoc on the exported symbol covering the full admission matrix."
  artifacts:
    - path: internal/privfield/privfield.go
      provides: "Field-level redaction helper that composes privctx.TierFrom with <field>_visible string to decide omit."
      exports:
        - "Redact"
      min_lines: 30
    - path: internal/privfield/privfield_test.go
      provides: "Table-driven unit tests covering the D-11 truth table + fail-closed case."
      contains: "func TestRedact"
      min_lines: 60
    - path: internal/privfield/doc.go
      provides: "Package doc explaining relationship to privctx and the serializer-layer redaction pattern."
  key_links:
    - from: internal/privfield/privfield.go
      to: internal/privctx/privctx.go
      via: "Redact calls privctx.TierFrom(ctx) and compares against privctx.TierUsers."
      pattern: "privctx\\.TierFrom|privctx\\.TierUsers"
---

<objective>
Build `internal/privfield` — the new shared helper that every API surface will call to decide whether to emit, null, or omit a gated field.

Purpose: This is the substrate VIS-08 mandates. ent's Privacy package is query-level; `privfield.Redact` is the missing field-level primitive. It also unlocks v1.16+ OAuth gated fields per CONTEXT.md §Specifics.

Output:
- `internal/privfield/privfield.go` — one exported function, non-generic, ~30 LOC (per D-02, RESEARCH §"`internal/privfield` Package Shape").
- `internal/privfield/privfield_test.go` — table-driven unit tests for the 4-case admission matrix + fail-closed (D-11).
- `internal/privfield/doc.go` — package-level godoc so the intent is discoverable.

This plan is a pure leaf (no imports from phase-64 files elsewhere) so it runs in parallel with Plan 64-02 (schema wiring). Plan 64-03 imports this package.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/64-field-level-privacy/64-CONTEXT.md
@.planning/phases/64-field-level-privacy/64-RESEARCH.md
@CLAUDE.md
@internal/privctx/privctx.go

<interfaces>
<!-- The ONLY dependency this plan takes: privctx.TierFrom and the tier constants. -->
<!-- Confirmed present in the repo from Phase 59. Executor does not need to rediscover. -->

From internal/privctx/privctx.go (Phase 59):
```go
// TierFrom returns the caller's privacy tier stamped on ctx by
// middleware.PrivacyTier. Returns TierPublic (the most restrictive)
// when ctx has no tier — fail-closed.
func TierFrom(ctx context.Context) Tier

// Tier is an ordered privacy tier; higher tiers admit more fields.
type Tier int

const (
    TierPublic Tier = iota
    TierUsers
)
```

Verify these exist with:
```bash
grep -n "TierFrom\|TierPublic\|TierUsers\|^type Tier" internal/privctx/privctx.go
```
If the grep returns zero matches the Phase-59 contract has drifted — STOP and escalate.
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Create internal/privfield package with fail-closed Redact helper + table-driven unit tests</name>
  <files>internal/privfield/privfield.go, internal/privfield/privfield_test.go, internal/privfield/doc.go</files>

  <read_first>
    Before writing ANY code, read:
    1. `internal/privctx/privctx.go` — confirm the exact signature of `TierFrom` and the names of the tier constants (`TierPublic`, `TierUsers`). If they differ from this plan, stop and surface the drift.
    2. `.planning/phases/64-field-level-privacy/64-RESEARCH.md` §"`internal/privfield` Package Shape" — the full code example lives there; copy it verbatim and then edit the package path import in step 3.
    3. `.planning/phases/64-field-level-privacy/64-CONTEXT.md` D-01, D-02, D-03, D-11 — the locked decisions this task enforces.
    4. `CLAUDE.md` §"Go-CS-5" and §"Go-CTX-1" — input struct rule does NOT apply (ctx is exempt; only 3 args total).
    5. `CLAUDE.md` §"GO-T-1" — all Go tests in this repo are table-driven. This is a stdlib `testing` package project.

    For the doc.go godoc style, read any one existing `internal/*/doc.go` if present (`ls internal/*/doc.go`) OR mirror the package-level godoc on `internal/privctx/privctx.go` top-of-file.
  </read_first>

  <behavior>
    The `Redact` function MUST produce these exact outputs for every (tier, visible) combination:

    | ctx tier        | visible     | expected out | expected omit |
    |-----------------|-------------|--------------|---------------|
    | TierPublic      | "Public"    | value (unchanged) | false |
    | TierPublic      | "Users"     | ""           | true  |
    | TierPublic      | "Private"   | ""           | true  |
    | TierPublic      | ""          | ""           | true  |
    | TierPublic      | "garbage"   | ""           | true  |
    | TierUsers       | "Public"    | value        | false |
    | TierUsers       | "Users"     | value        | false |
    | TierUsers       | "Private"   | ""           | true  |
    | TierUsers       | ""          | ""           | true  |
    | unstamped ctx   | "Users"     | ""           | true  (fail-closed per D-03) |

    The unit test MUST cover every row above — at least 10 sub-tests via table-driven cases. The "unstamped ctx" case is `context.Background()` with no PrivacyTier middleware run.

    `value` in the above table is a non-empty test string like `"https://example.test/members.json"`. When `omit=true` the returned string MUST be exactly `""` — call sites will combine that with `json:",omitempty"` (in Plan 64-03). Do NOT return the original value when omit=true.
  </behavior>

  <action>
    1. Create `internal/privfield/privfield.go`. Copy the code block from RESEARCH.md §"`internal/privfield` Package Shape" verbatim. The full body is:

    ```go
    // Package privfield provides serializer-layer field-level privacy redaction.
    // It composes with internal/privctx (row-level tier stamping) but operates
    // one level lower: on a single field within an already-admitted row.
    //
    // Use Redact at each API surface's response-assembly site, passing the
    // pre-existing <field>_visible companion string stored on the ent row.
    // Every surface MUST call Redact for every field guarded by a _visible
    // companion; there is no centralised enforcement — it's a per-serializer
    // discipline locked by the 5-surface E2E test in Plan 64-03.
    //
    // Design decisions locked in Phase 64 CONTEXT.md:
    //   D-01 serializer-layer redaction (not ent Policy)
    //   D-02 reusable package (this one)
    //   D-03 fail-closed on unstamped ctx
    //   D-04 omit key entirely when redacted (caller uses json omitempty)
    package privfield

    import (
        "context"

        "github.com/dotwaffle/peeringdb-plus/internal/privctx"
    )

    // Redact returns (value, false) if the caller's tier on ctx admits the
    // field, or ("", true) if the serializer should omit the field entirely.
    //
    // Admission rules:
    //   - visible == "Public"                → always admit (any tier)
    //   - visible == "Users" && tier Users+  → admit
    //   - visible == "Users" && tier Public  → redact (the gated case)
    //   - visible == "Private"               → redact in all tiers (upstream parity)
    //   - any unrecognised visible value     → redact (fail-closed)
    //
    // Fail-closed semantics:
    // privctx.TierFrom(ctx) already returns TierPublic for un-stamped
    // contexts, so an un-plumbed ctx naturally lands in the most
    // restrictive branch — no extra check needed here.
    func Redact(ctx context.Context, visible, value string) (out string, omit bool) {
        tier := privctx.TierFrom(ctx)

        switch visible {
        case "Public":
            return value, false
        case "Users":
            if tier >= privctx.TierUsers {
                return value, false
            }
            return "", true
        default:
            // "Private" or unknown → always redact.
            return "", true
        }
    }
    ```

    DO NOT substitute a generic signature. DO NOT replace the string comparison with a typed visibility enum. DO NOT inline `privctx.TierFrom`. RESEARCH.md locks the non-generic string-typed design — any alteration is out of scope.

    2. Create `internal/privfield/doc.go` containing only the package-level godoc. This is optional (the godoc on privfield.go covers the package) — but we place it in doc.go so future exported symbols can accrete without pushing the package doc into privfield.go away from `func Redact`. Template:

    ```go
    // Package privfield provides serializer-layer field-level privacy
    // redaction. See privfield.go for the full Redact contract.
    package privfield
    ```

    If the project convention is to keep package doc on the main file (check `ls internal/privctx/` — if no doc.go, follow that convention and skip doc.go). Either way is acceptable; the test cares only that the package compiles.

    3. Create `internal/privfield/privfield_test.go` as a table-driven test. Required structure:

    ```go
    package privfield_test

    import (
        "context"
        "testing"

        "github.com/dotwaffle/peeringdb-plus/internal/privctx"
        "github.com/dotwaffle/peeringdb-plus/internal/privfield"
    )

    func TestRedact(t *testing.T) {
        t.Parallel()

        const url = "https://example.test/members.json"

        tests := []struct {
            name      string
            ctx       context.Context
            visible   string
            value     string
            wantOut   string
            wantOmit  bool
        }{
            {"public-tier-visible-public", privctx.WithTier(context.Background(), privctx.TierPublic), "Public", url, url, false},
            {"public-tier-visible-users",  privctx.WithTier(context.Background(), privctx.TierPublic), "Users", url, "", true},
            {"public-tier-visible-private",privctx.WithTier(context.Background(), privctx.TierPublic), "Private", url, "", true},
            {"public-tier-visible-empty",  privctx.WithTier(context.Background(), privctx.TierPublic), "", url, "", true},
            {"public-tier-visible-garbage",privctx.WithTier(context.Background(), privctx.TierPublic), "garbage", url, "", true},
            {"users-tier-visible-public",  privctx.WithTier(context.Background(), privctx.TierUsers),  "Public", url, url, false},
            {"users-tier-visible-users",   privctx.WithTier(context.Background(), privctx.TierUsers),  "Users", url, url, false},
            {"users-tier-visible-private", privctx.WithTier(context.Background(), privctx.TierUsers),  "Private", url, "", true},
            {"users-tier-visible-empty",   privctx.WithTier(context.Background(), privctx.TierUsers),  "", url, "", true},
            {"unstamped-ctx-fail-closed",  context.Background(), "Users", url, "", true},
            {"unstamped-ctx-public-admits",context.Background(), "Public", url, url, false},
        }

        for _, tc := range tests {
            tc := tc
            t.Run(tc.name, func(t *testing.T) {
                t.Parallel()
                gotOut, gotOmit := privfield.Redact(tc.ctx, tc.visible, tc.value)
                if gotOut != tc.wantOut {
                    t.Errorf("out = %q, want %q", gotOut, tc.wantOut)
                }
                if gotOmit != tc.wantOmit {
                    t.Errorf("omit = %v, want %v", gotOmit, tc.wantOmit)
                }
            })
        }
    }
    ```

    Adjust the setter name (`privctx.WithTier` above is a guess) to match the actual exported setter in `internal/privctx` — run `grep -n "func With\|func Set" internal/privctx/privctx.go` BEFORE writing the test. If the setter is named differently (e.g. `privctx.Context`, `privctx.Inject`), substitute. If no public setter exists, seed tier via `middleware.PrivacyTier` — but that's heavier than this unit test needs; prefer using the exported constructor.

    4. Package must compile and pass tests with:
       - `TMPDIR=/tmp/claude-1000 go build ./internal/privfield/...`
       - `TMPDIR=/tmp/claude-1000 go test -race ./internal/privfield/...`
  </action>

  <verify>
    <automated>TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/privfield/...</automated>
    Also verify package surface is minimal:
    `grep -n "^func " internal/privfield/privfield.go` returns exactly one line: `func Redact(...)`.
    `grep -rn "package privfield$" internal/privfield/ | wc -l` returns 2 or 3 (privfield.go, privfield_test.go with `_test` suffix counts separately — expect 1 or 2 `package privfield` matches plus 1 `package privfield_test`).
  </verify>

  <done>
    - `internal/privfield/privfield.go` exists with exactly one exported function `Redact(ctx, visible, value string) (string, bool)`.
    - Godoc on `Redact` enumerates all four admission rules + fail-closed behaviour verbatim from RESEARCH.md.
    - Unit tests cover every row in the truth-table under `<behavior>` (minimum 10 sub-tests including the unstamped-ctx case).
    - `TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/privfield/...` passes with `PASS` output.
    - `TMPDIR=/tmp/claude-1000 go vet ./internal/privfield/...` clean.
    - `golangci-lint run ./internal/privfield/...` clean (or at least no new findings vs main).
    - No new imports outside of `context`, `testing`, and `github.com/dotwaffle/peeringdb-plus/internal/privctx`.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| caller → privfield.Redact | Input `visible` is sourced from the ent row (trusted — populated by sync from upstream). `value` is likewise trusted. The only untrusted axis is the ctx tier, and privctx owns that stamping. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-64-01 | Information Disclosure | privfield.Redact | mitigate | Fail-closed default (unstamped ctx → TierPublic → redact) locked by unit test `unstamped-ctx-fail-closed`. Matches CONTEXT.md D-03. |
| T-64-02 | Elevation of Privilege | privfield.Redact | mitigate | No state, no external I/O; pure function of (tier, visible). Cannot be tricked via side channels. Unit tests enumerate every (tier, visible) combination. |
| T-64-03 | Tampering | privctx.TierFrom contract | accept | If privctx is compromised, every caller is affected — out of scope for this package. Phase 59 owns that boundary. |
</threat_model>

<verification>
**Package-level:**
- `TMPDIR=/tmp/claude-1000 go test -race -count=1 ./internal/privfield/...` — PASS
- `TMPDIR=/tmp/claude-1000 go vet ./internal/privfield/...` — clean
- `golangci-lint run ./internal/privfield/...` — clean
- `govulncheck ./internal/privfield/...` — clean

**Surface audit:**
- Package exports exactly one function (`Redact`). No types, no other vars.
- No new top-level dependencies added to `go.mod` (package uses only stdlib + existing internal/privctx).
</verification>

<success_criteria>
- `internal/privfield.Redact` exists, matches the RESEARCH.md signature exactly, with godoc documenting all four admission rules and fail-closed behaviour.
- Unit tests pass `go test -race` including the D-11 unstamped-ctx case.
- Package compiles clean; lint clean; vuln clean.
- Plan 64-03 can import this package and call `privfield.Redact` without any follow-up changes here.
</success_criteria>

<output>
After completion, create `.planning/phases/64-field-level-privacy/64-01-SUMMARY.md` documenting:
- Exact exported surface (`func Redact(ctx, visible, value string) (string, bool)`)
- Truth-table covered by unit tests
- Any deviation from RESEARCH.md code block (expect: none)
- `go test -race ./internal/privfield/...` output (paste the PASS line)
</output>
