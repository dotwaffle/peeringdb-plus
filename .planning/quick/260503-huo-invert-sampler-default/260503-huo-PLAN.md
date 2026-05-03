---
phase: 260503-huo-invert-sampler-default
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - internal/otel/provider.go
  - internal/otel/sampler_test.go
  - internal/otel/provider_test.go
  - docs/ARCHITECTURE.md
autonomous: true
requirements:
  - QUICK-260503-huo-INVERT-SAMPLER-DEFAULT
must_haves:
  truths:
    - "Spans for unknown URL paths (e.g. /.env, /.git/config, /wp-admin, /phpinfo.php) sample in at 1% (or 0.1% when matched by /. or /wp-) instead of inheriting in.SampleRate."
    - "Known app surfaces (/api/, /rest/v1/, /peeringdb.v1., /graphql) still sample at the operator-controlled in.SampleRate (default 1.0)."
    - "Existing health-probe / static / UI ratios are unchanged."
    - "PDBPLUS_OTEL_SAMPLE_RATE remains a working operator knob — lowering it dampens known-app-route volume only, not the deny-by-default floor."
    - "ParentBased composition still wins: a sampled-in parent forces the child's RecordAndSample regardless of the inverted default."
    - "docs/ARCHITECTURE.md Sampling Matrix table reflects the inverted policy."
    - "provider.go's pre-Routes doc comment describes deny-by-default + allow-list semantics; the AUDIT.md ratio-table reference is annotated as superseded by this quick task's SUMMARY."
  artifacts:
    - path: "internal/otel/provider.go"
      provides: "Inverted PerRouteSamplerInput wiring (DefaultRatio=0.01; explicit /. and /wp- deny entries; /api/, /rest/v1/, /peeringdb.v1., /graphql wired to in.SampleRate)"
      contains: "DefaultRatio: 0.01"
    - path: "internal/otel/sampler_test.go"
      provides: "Coverage for the new deny-by-default behaviour and /. + /wp- prefix matches"
      contains: "TestPerRouteSampler"
    - path: "internal/otel/provider_test.go"
      provides: "Sanity check that the wired-up sampler in Setup uses the inverted defaults"
      contains: "Setup"
    - path: "docs/ARCHITECTURE.md"
      provides: "Updated Sampling Matrix table reflecting inverted defaults + scanner-bait deny prefixes"
      contains: "0.001"
  key_links:
    - from: "internal/otel/provider.go (Setup)"
      to: "PerRouteSamplerInput.Routes wiring"
      via: "in.SampleRate referenced in /api/, /rest/v1/, /peeringdb.v1., /graphql entries; DefaultRatio hardcoded 0.01"
      pattern: "/api/.*in\\.SampleRate"
    - from: "matchesPrefix(sampler.go:129-140)"
      to: "/. and /wp- prefix entries"
      via: "non-alphanumeric trailing byte → 'prefix already includes its own boundary' branch matches /.env, /.git/, /wp-admin"
      pattern: "/\\.|/wp-"
---

<objective>
Invert the per-route head sampler default in internal/otel/provider.go so unknown URL
paths drop to 1% (`DefaultRatio = 0.01`) instead of inheriting `in.SampleRate`. Add explicit
deny-prefix entries `/.` (0.001) and `/wp-` (0.001) for scanner bait. Wire the known-good
app prefixes (`/api/`, `/rest/v1/`, `/peeringdb.v1.`, `/graphql`) to `in.SampleRate` so
`PDBPLUS_OTEL_SAMPLE_RATE` becomes the *known-app-route* knob. Update tests + the Sampling
Matrix doc table.

Purpose: a vulnerability scanner from 45.148.10.238 generated ~9M spans/hour against
unknown paths on 2026-05-02, peaking at 384 KB/s vs the free-tier 500 KB/s cap and
tripping `live_traces_exceeded` discards. Inverting the default would have dropped that
spike from ~2 GB to ~20 MB. Hardcoded policy change — no new env var.

Output: one atomic commit; ratio matrix in code + docs aligned; existing operator escape
hatch (`PDBPLUS_OTEL_SAMPLE_RATE`) preserved but redirected onto the allow-list.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@CLAUDE.md
@internal/otel/provider.go
@internal/otel/sampler.go
@internal/otel/sampler_test.go
@internal/otel/provider_test.go
@docs/ARCHITECTURE.md
@docs/CONFIGURATION.md

<interfaces>
<!-- Key contracts the executor needs. Do not re-explore the codebase. -->

From internal/otel/provider.go (current — to be edited):
```go
// SetupInput holds configuration for initializing the OTel pipeline.
type SetupInput struct {
    ServiceName string
    SampleRate  float64
}

// Current Routes wiring inside Setup(...) — lines 72-86. After this plan:
//   DefaultRatio: 0.01 (was: in.SampleRate)
//   Add:    "/.": 0.001, "/wp-": 0.001
//   Change: "/api/", "/rest/v1/", "/peeringdb.v1.", "/graphql" -> in.SampleRate (was literal 1.0)
//   Keep:   "/healthz", "/readyz", "/grpc.health.v1.Health/", "/ui/", "/static/", "/favicon.ico" unchanged.
```

From internal/otel/sampler.go:
```go
type PerRouteSamplerInput struct {
    DefaultRatio float64
    Routes       map[string]float64
}
func NewPerRouteSampler(in PerRouteSamplerInput) sdktrace.Sampler

// matchesPrefix boundary (lines 129-140):
//  - prefix ending in alnum byte (e.g. "/api")  -> next char of path must be '/' or end-of-string
//  - prefix ending in non-alnum byte (e.g. "/.", "/wp-", "/peeringdb.v1.") -> any next char OK
// → "/."  matches "/.env", "/.git/config", "/.aws/credentials", "/.well-known/" (acceptable)
// → "/wp-" matches "/wp-admin", "/wp-login.php", "/wp-content/uploads/..."
```

From internal/otel/sampler_test.go test idiom (mirror this pattern, table-driven where it
adds value but a per-test helper `sampleParams(allOnesTraceID, attribute.String("url.path", path))`
is fine for additive tests — see the existing `TestPerRouteSampler_*` set):
```go
res := s.ShouldSample(sampleParams(allOnesTraceID, attribute.String("url.path", "/.env")))
if res.Decision != sdktrace.Drop { /* fail */ }
```
Note: `TraceIDRatioBased(0.001)` deterministically *drops* the all-ones TraceID
(threshold ratio < 1 with the maximum-valued trace ID always falls outside the
sampled-in segment). So `/.env` and `/wp-admin` assertions assert `sdktrace.Drop`.
For the `DefaultRatio: 0.01` case the same logic applies — unmatched paths drop
under all-ones TraceID. To assert the *inverse* (the path is being routed to the
default rather than to a prefix entry) we exercise both `DefaultRatio: 1.0` (path
must sample-in) and `DefaultRatio: 0.0` (path must drop) — see existing
`TestPerRouteSampler_UnmatchedPathFallsBackToDefault` for the pattern.
</interfaces>

<scanner-bait-paths-validated-on-2026-05-02>
.env, .git/config, .git/HEAD, .aws/credentials, .docker/config.json, .kube/config,
.htpasswd, .npmrc, .ssh/id_rsa, phpinfo.php, wp-admin, wp-login.php,
wp-content/themes/.../setup-config.php
</scanner-bait-paths-validated-on-2026-05-02>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: invert sampler default + add scanner-bait deny prefixes</name>
  <files>internal/otel/provider.go, internal/otel/sampler_test.go, internal/otel/provider_test.go, docs/ARCHITECTURE.md</files>
  <behavior>
    Sampler-level (sampler_test.go — additive tests, all `t.Parallel()`):

    - **TestPerRouteSampler_DotDeniedAtLowRatio**: with Routes `{"/.": 0.001}` +
      `DefaultRatio: 1.0`, all of `/.env`, `/.git/config`, `/.aws/credentials`,
      `/.docker/config.json`, `/.kube/config`, `/.htpasswd`, `/.npmrc` sample to
      `sdktrace.Drop` under `allOnesTraceID`. Use a table-driven sub-test slice.
    - **TestPerRouteSampler_WpDeniedAtLowRatio**: with Routes `{"/wp-": 0.001}` +
      `DefaultRatio: 1.0`, all of `/wp-admin`, `/wp-login.php`,
      `/wp-content/themes/foo/setup-config.php` sample to `sdktrace.Drop`. Table-driven.
    - **TestPerRouteSampler_UnknownPathDropsAtOnePercent**: with Routes
      `{"/api/": 1.0}` + `DefaultRatio: 0.01`, `/phpinfo.php` (no prefix match)
      sample to `sdktrace.Drop` under `allOnesTraceID` (because 0.01 ratio with
      all-ones TraceID falls outside the sampled-in segment). Sanity-check the
      complementary case: `/api/networks` still sample-ins.
    - **TestPerRouteSampler_DotDoesNotMatchPlainSlash**: matching defence — a path
      `/`  alone (root) MUST NOT be matched by `/.` (`strings.HasPrefix("/", "/.")`
      is false). Assert it falls through to `DefaultRatio`, parameterising the
      default both ways (1.0 → sample-in; 0.0 → drop) so the assertion proves
      "fell through to default" rather than "matched and dropped".

    Provider-level (provider_test.go — one new sanity check):

    - **TestSetup_InvertedSamplerDefault**: this is the future-operator
      regression-lock the constraints call out. Strategy: extract the inline
      `Routes` literal from Setup() into a small package-private helper
      (e.g. `defaultSamplerInput(in SetupInput) PerRouteSamplerInput`) so the
      test can assert on the *config object* directly without wrestling with the
      opaque `sdktrace.Sampler` interface. Test asserts:
        - `out.DefaultRatio == 0.01` exactly.
        - `out.Routes["/."] == 0.001` and `out.Routes["/wp-"] == 0.001`.
        - `out.Routes["/api/"] == in.SampleRate` (parameterise SampleRate over
          {0.0, 0.5, 1.0} via a table) — proves the operator knob is wired
          through, not pinned to literal 1.0.
        - The existing health-probe / static / UI ratios survive unchanged
          (assert exact map values for `/healthz`, `/static/`, `/ui/`).
      The existing `Setup` tests stay as-is; this is purely additive.
  </behavior>
  <action>
    1. **internal/otel/provider.go** — extract the current inline `PerRouteSamplerInput{...}`
       literal (lines 72-86) into a package-private helper:

       ```go
       // defaultSamplerInput returns the per-route sampler configuration used by Setup.
       // Lifted into a helper so provider_test.go can lock the inverted policy without
       // the round-trip through the opaque sdktrace.Sampler interface.
       //
       // Policy (inverted post-2026-05-02 scanner incident — 9M spans/hour from a
       // single source against unknown paths peaked at 384 KB/s vs the free-tier
       // 500 KB/s cap; see .planning/quick/260503-huo-invert-sampler-default/SUMMARY.md):
       //
       //   - Default = 0.01: deny-by-default for unknown URL paths. The historical
       //     "unknown == in.SampleRate" inheritance is gone; in.SampleRate now drives
       //     the *known-app-route* allow-list only, so PDBPLUS_OTEL_SAMPLE_RATE
       //     remains the operator's incident-time dampener for /api/, /rest/v1/,
       //     /peeringdb.v1., /graphql.
       //   - "/.", "/wp-" at 0.001: explicit deny-prefixes for scanner bait
       //     (.env, .git/, .aws/, .kube/, .htpasswd, .npmrc, wp-admin, wp-login.php).
       //     matchesPrefix's non-alnum-boundary branch (sampler.go:129-140) makes
       //     these match without further tokenisation. /.well-known/ is sampled at
       //     0.1% too — acceptable since pdbplus does not currently serve it.
       //   - Health probes / static / UI ratios unchanged.
       //
       // The .planning/phases/77-telemetry-audit/AUDIT.md ratio matrix is
       // SUPERSEDED by this policy; do not update both together — defer to this
       // file + docs/ARCHITECTURE.md § Sampling Matrix.
       func defaultSamplerInput(in SetupInput) PerRouteSamplerInput {
           return PerRouteSamplerInput{
               DefaultRatio: 0.01,
               Routes: map[string]float64{
                   // Scanner bait — drop aggressively. Boundary-rule: trailing
                   // non-alnum byte means "/." matches /.env /.git/ /.aws/...
                   // and "/wp-" matches /wp-admin /wp-login.php /wp-content/...
                   "/.":   0.001,
                   "/wp-": 0.001,
                   // Known app surfaces — operator-controlled via PDBPLUS_OTEL_SAMPLE_RATE.
                   "/api/":          in.SampleRate,
                   "/rest/v1/":      in.SampleRate,
                   "/peeringdb.v1.": in.SampleRate,
                   "/graphql":       in.SampleRate,
                   // Health probes — unchanged (Phase 77 OBS-07).
                   "/healthz":                0.01,
                   "/readyz":                 0.01,
                   "/grpc.health.v1.Health/": 0.01,
                   // Browser + static — unchanged.
                   "/ui/":         0.5,
                   "/static/":     0.01,
                   "/favicon.ico": 0.01,
               },
           }
       }
       ```

       Replace the inline literal at the existing `sdktrace.WithSampler(...)` call with
       `NewPerRouteSampler(defaultSamplerInput(in))`. The wrapping `sdktrace.ParentBased`
       stays exactly as-is (cross-service trace continuity invariant).

       Replace the doc comment block at lines 59-65 (the one currently citing AUDIT.md
       § Recommended sampling matrix) with a 4-5-line summary that says:
       (a) deny-by-default for unknown paths,
       (b) explicit allow-list for /api/ /rest/v1/ /peeringdb.v1. /graphql driven by in.SampleRate,
       (c) explicit deny-prefixes /. and /wp- for scanner bait,
       (d) ParentBased preserves cross-service continuity,
       (e) full table is in docs/ARCHITECTURE.md § Sampling Matrix and
       .planning/quick/260503-huo-invert-sampler-default/SUMMARY.md.

    2. **internal/otel/sampler_test.go** — append the four new tests from <behavior>.
       Table-driven where it adds value (the per-path sweeps), keep
       `t.Parallel()` consistent with existing tests, mirror the existing
       `sampleParams(allOnesTraceID, attribute.String("url.path", path))` idiom.
       No changes to existing tests.

    3. **internal/otel/provider_test.go** — append the one new
       `TestSetup_InvertedSamplerDefault` test. Calls `defaultSamplerInput(SetupInput{
       SampleRate: tc.rate})` and asserts on the returned struct fields. Use a
       sub-test loop over `[]float64{0.0, 0.5, 1.0}` for the SampleRate axis.

    4. **docs/ARCHITECTURE.md** — replace the Sampling Matrix table (lines 572-579) with
       the inverted layout. Suggested new rows:

       | Route prefix | Ratio | Rationale |
       |--------------|-------|-----------|
       | `/.`, `/wp-` | 0.001 | Scanner-bait deny-prefixes (`.env`, `.git/`, `.aws/`, `.kube/`, `.htpasswd`, `.npmrc`, `wp-admin`, `wp-login.php`). Added 2026-05-03 after a 9M-spans/hour scanner spike from 45.148.10.238 peaked at 384 KB/s and tripped `live_traces_exceeded`. |
       | `/healthz`, `/readyz`, `/grpc.health.v1.Health/` | 0.01 | Fly health probes — 1% sample is enough for liveness debugging without dominating Tempo volume. |
       | `/api/`, `/rest/v1/`, `/peeringdb.v1.`, `/graphql` | `PDBPLUS_OTEL_SAMPLE_RATE` (default 1.0) | Primary API surfaces — full sampling for debugging by default. The env var is the operator's incident-time dampener. |
       | `/ui/` | 0.5 | Browser traffic; halved per the Phase 77 audit. |
       | `/static/`, `/favicon.ico` | 0.01 | Static assets; rare debugging value. |
       | (default — unknown paths, sync worker, internal spans) | 0.01 | Deny-by-default for unknown URL paths (scanner protection). Sync-worker / internal spans without a `url.path` attribute also land here at 1%. To raise this floor, edit `defaultSamplerInput` in `internal/otel/provider.go`. |

       Update the prose just under the table: the `/api/auth/foo` longest-prefix-wins
       paragraph stays valid; add a sentence noting that the env var has been
       redirected from "default ratio" to "known-app-route ratio".

       Verify by grep that no other docs file references the old "default ratio
       honours PDBPLUS_OTEL_SAMPLE_RATE" wording — the line at 560 ("default ratio
       honours `PDBPLUS_OTEL_SAMPLE_RATE`...") needs to be reworded to match. Suggested
       replacement: "The known-app-route ratio honours `PDBPLUS_OTEL_SAMPLE_RATE`
       (default `1.0`); unknown-path traces drop to 1% to defend against opportunistic
       scanners. Sync-worker and other non-HTTP spans hit the same 1% deny-by-default
       floor."

    5. **docs/CONFIGURATION.md** — verify before editing. The line at row 88 says
       "Trace sampling ratio passed to `sdktrace.TraceIDRatioBased`". After this
       change, the env value is *still* passed to `sdktrace.TraceIDRatioBased` —
       just for the four allow-listed routes instead of the default. The wording
       is technically still accurate but loses signal. **Recommended minimal
       edit**: replace the Description with "Trace sampling ratio for the known
       app surfaces (`/api/`, `/rest/v1/`, `/peeringdb.v1.`, `/graphql`).
       Unknown-path / scanner-bait / health-probe ratios are hardcoded — see
       docs/ARCHITECTURE.md § Sampling Matrix. Must be in the inclusive range
       `[0.0, 1.0]`. Values outside this range are rejected at startup." Keep
       the rest of the row identical.

    6. **Do NOT** touch the metric Views block (lines 90-189) or the resource-builder
       helpers; out of scope.

    7. **Do NOT** introduce `PDBPLUS_OTEL_DEFAULT_SAMPLE_RATE` or any new env var; the
       inverted floor is hardcoded by design (per task constraints).

    8. **Do NOT** regenerate CLAUDE.md; the project rule is explicit. The CLAUDE.md
       memory will be updated by hand later if at all — this is just a code+docs change.
  </action>
  <verify>
    <automated>
go test -race ./internal/otel/... -run 'PerRouteSampler|Setup_InvertedSamplerDefault' -count=1 -v &amp;&amp; go vet ./internal/otel/... &amp;&amp; go build ./... &amp;&amp; golangci-lint run ./internal/otel/...
    </automated>
  </verify>
  <done>
    - `defaultSamplerInput` exists, is package-private, and returns `DefaultRatio: 0.01` plus the 12 route entries (2 deny-prefixes + 4 in.SampleRate-driven app routes + 3 health probes + 3 browser/static).
    - `Setup` calls `NewPerRouteSampler(defaultSamplerInput(in))` instead of the inline literal.
    - The provider.go doc block above the sampler call describes deny-by-default + allow-list policy and forward-references the SUMMARY.
    - Four new sampler-level tests pass; one new provider-level test (parameterised over 3 SampleRate values) passes.
    - All existing `internal/otel` tests still pass under `-race`.
    - `go vet`, `go build ./...`, and `golangci-lint run ./internal/otel/...` are clean.
    - `docs/ARCHITECTURE.md § Sampling Matrix` table reflects the new layout (2 deny-prefixes + env-var-driven app row + default 0.01).
    - `docs/CONFIGURATION.md` PDBPLUS_OTEL_SAMPLE_RATE row description scopes to known app surfaces.
    - Single atomic commit (kernel-style subject `otel: invert per-route sampler default`).
  </done>
</task>

</tasks>

<verification>

Manual sanity checks the executor should run after the automated gate passes:

- `grep -n 'DefaultRatio' internal/otel/provider.go` → exactly one hit, value `0.01`.
- `grep -n 'in.SampleRate' internal/otel/provider.go` → 4 hits inside `defaultSamplerInput`'s Routes map, none elsewhere in the Setup function path.
- `grep -n '"/\.\|"/wp-"' internal/otel/provider.go` → 2 hits for the deny-prefix entries.
- `grep -n 'AUDIT.md' internal/otel/provider.go internal/otel/sampler.go` → only the historical sampler.go header reference remains; provider.go's old "update both together" line is gone.
- Doc table in `docs/ARCHITECTURE.md` has a row mentioning `0.001` for `/.` and `/wp-`.

</verification>

<success_criteria>

- One atomic commit, kernel-style subject `otel: invert per-route sampler default`. Body explains the 2026-05-02 scanner incident motivation, the inversion (deny-by-default 1%, in.SampleRate redirected to known app routes, /. and /wp- explicit-deny at 0.1%), and notes that AUDIT.md's matrix is superseded.
- `go test -race ./internal/otel/... -count=1` passes, including the four new sampler tests + one new provider test.
- `go vet ./...`, `go build ./...`, `golangci-lint run` all clean.
- ARCHITECTURE.md Sampling Matrix and CONFIGURATION.md PDBPLUS_OTEL_SAMPLE_RATE row both reflect the inverted policy; no stale "default ratio honours PDBPLUS_OTEL_SAMPLE_RATE" wording remains in either file.
- ParentBased composition test (`TestParentBased_InheritsDecisionForSampledIn`) still passes — proves cross-service continuity invariant survives.
- Operator escape hatch preserved: setting `PDBPLUS_OTEL_SAMPLE_RATE=0.1` causes only the four known-app-route entries to drop; the deny-by-default 1% floor is unaffected.

</success_criteria>

<output>
After completion, create `.planning/quick/260503-huo-invert-sampler-default/260503-huo-SUMMARY.md`
documenting: incident motivation (volume figures + scanner IP/UA), the policy change
(before/after table), test additions, doc edits, and a note that AUDIT.md's
"Recommended sampling matrix" is now historical.
</output>
