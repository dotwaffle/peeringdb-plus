# Domain Pitfalls

**Domain:** Adding golden file tests, GitHub Actions CI pipeline, and comprehensive linting/test enforcement to existing Go/entgo/SQLite project
**Researched:** 2026-03-23
**Milestone:** v1.2 Quality & CI

## Critical Pitfalls

Mistakes that cause flaky CI, false-positive failures, or fundamentally broken test strategies.

### Pitfall 1: Golden File Timestamps Are Non-Deterministic -- Tests Fail on Every Run

**What goes wrong:** The PeeringDB compat layer serializes `created` and `updated` fields as RFC 3339 timestamps (e.g., `"2025-01-01T00:00:00Z"`). The existing handler tests use `time.Now()` to create test entities (confirmed in `handler_test.go:33`, `depth_test.go:21`). If golden files capture the full JSON response including these timestamps, every test run produces different timestamp values and the golden file comparison fails.

**Why it happens:** Golden file testing compares byte-for-byte against a stored expected output. Timestamps derived from `time.Now()` change with every execution. The existing `setupTestHandler()` creates networks with `now := time.Now().Truncate(time.Second).UTC()` and derives `past` and `future` from it. These values will never match a previously captured golden file.

**Consequences:** Golden file tests fail on every run unless timestamps are fixed. Developers learn to ignore golden file failures, defeating their purpose as a regression safety net.

**Prevention:**
- Use fixed, deterministic timestamps in all golden file test fixtures: `time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)`. The serializer tests already do this correctly (e.g., `serializer_test.go:26`).
- Do NOT use `time.Now()` in any test that produces golden file output. Create a separate `setupGoldenTestHandler()` that uses fixed timestamps.
- Store golden files in `internal/pdbcompat/testdata/golden/` following Go convention.

**Detection:** Golden file tests fail with timestamp-only diffs. `go test -update` regenerates files that differ only in time values.

---

### Pitfall 2: golangci-lint on ~300K LOC of Generated Code -- Timeout, False Positives, and Configuration Complexity

**What goes wrong:** The project has ~300K lines of generated Go code (ent: ~250K, gqlgen: ~57K) and only ~8K lines of hand-written code. Running golangci-lint without proper exclusions analyzes all 300K lines, causing: (a) CI timeouts, (b) hundreds of false-positive lint warnings on generated code that cannot be fixed, and (c) developer frustration as real issues are buried in noise.

**Why it happens:** golangci-lint's default `generated: lax` mode has loose detection. The ent and gqlgen generated files DO have the standard `// Code generated ... DO NOT EDIT.` header, so `generated: strict` correctly excludes them. However, some generated files may not have the header, and the schema files in `ent/schema/` are hand-written and must NOT be excluded.

**Consequences:** Without proper configuration: CI times out or produces hundreds of irrelevant warnings. With over-aggressive exclusions: hand-written schema files in `ent/schema/` are accidentally excluded.

**Prevention:**
- Use `generated: strict` in golangci-lint v2 configuration. This matches the Go standard convention header correctly.
- Add explicit `exclusions.paths` as defense-in-depth for directories like `ent/rest/`.
- Do NOT use blanket directory exclusions for `ent/`. The `ent/schema/` directory contains hand-written code that must be linted.
- Set `run.timeout: 5m` in the golangci-lint config.

**Detection:** CI takes >5 minutes on lint step. Lint warnings reference files in `ent/` that start with `// Code generated`.

---

### Pitfall 3: Existing Tests May Fail Under -race or Strict Linting Before New Tests Are Added

**What goes wrong:** The project has ~21 existing test files. Enabling `-race` and strict linting in CI will run against ALL existing code. If existing tests have latent race conditions, or if existing hand-written code has lint violations that were never enforced, the CI pipeline will fail immediately on the first run.

**Why it happens:** The project was developed without CI enforcement. Common issues that surface:
- Hand-written code in `cmd/`, `internal/`, and `graph/` may have lint violations
- The `graph/globalid.go` has exported but unused functions (noted as tech debt)
- The `testutil` package registers a SQLite driver in `init()` with `sql.Register("sqlite3", ...)`

**Consequences:** The first CI run fails on pre-existing issues, not on new code. This creates scope creep -- the CI pipeline task balloons into "fix all existing issues."

**Prevention:**
- Run `go test -race ./...` and `golangci-lint run` locally BEFORE creating the CI workflow. Fix all existing issues first.
- Phase the work: (1) fix existing violations, (2) add CI workflow, (3) add golden file tests.
- For known tech debt that cannot be immediately fixed, add targeted `//nolint` annotations with explanatory comments, or delete the dead code.

**Detection:** CI fails on the first run with errors in files that were not modified by the current PR.

---

## Moderate Pitfalls

### Pitfall 4: Golden File JSON Field Ordering Relies on encoding/json v1 Behavior

**What goes wrong:** Golden file tests compare serialized JSON output. Go's `encoding/json` v1 marshals struct fields in declaration order and map keys in sorted order, which IS deterministic. However, the field projection feature (`?fields=id,name`) works by marshaling the full struct to `map[string]any`, then filtering keys, then re-marshaling the map. This relies on v1's sorted-key behavior.

**Why it happens:** The current code is safe because map keys marshal in sorted order in encoding/json v1. But if Go switches to encoding/json/v2 (which may marshal maps differently), golden files could break.

**Prevention:**
- For golden files, prefer struct-based serialization paths over map-based intermediate representations.
- For field projection golden files, consider semantic JSON comparison (parse both, compare structures) rather than byte-for-byte comparison.
- Use `json.MarshalIndent` with consistent indentation for all golden files.

---

### Pitfall 5: SQLite "Database is Locked" in Parallel Tests Under Race Detector

**What goes wrong:** The existing test infrastructure creates isolated in-memory SQLite databases per test. Under the race detector (`-race`), the Go runtime has higher scheduling overhead. Combined with SQLite's single-writer model, this increases the window for "database is locked" errors, especially with the dual-connection setup in `SetupClientWithDB` (returns both an ent client and a raw `*sql.DB`).

**Why it happens:** SQLite allows only one writer at a time. With `cache=shared` mode, multiple connections share the same cache but still contend for write locks. The race detector slows execution by 2-10x. No `busy_timeout` is configured in the DSN.

**Prevention:**
- Add `_pragma=busy_timeout(5000)` to the test DSN to make SQLite wait for locks rather than failing immediately.
- Set `db.SetMaxOpenConns(1)` on both connections to serialize access.
- Keep tests that use `SetupClient` (single connection) as-is -- they are safe.

**Detection:** Intermittent "database is locked" errors in CI that do not reproduce locally.

---

### Pitfall 6: Golden File Auto-ID Instability

**What goes wrong:** Golden file tests capture full API responses including entity IDs (`"id": 1`, `"id": 2`, etc.). SQLite auto-increment IDs are sequential from 1 within each in-memory database, but changing the entity creation order silently breaks all golden files. IDs in nested `_set` fields (from `depth > 0`) create cascading breakage.

**Prevention:**
- Use a deterministic, sequential entity creation pattern. Document the creation order as a contract.
- Do NOT use `t.Parallel()` within golden file test setup -- entity creation must be sequential.
- If creation order must change, regenerate all golden files with `-update` and review the diffs.

---

### Pitfall 7: golangci-lint Version Mismatch Between Local and CI

**What goes wrong:** golangci-lint v1 and v2 have fundamentally different configuration file formats. If the local developer runs golangci-lint v1 with a v2 config (or vice versa), the configuration is silently ignored or partially applied. Generated code exclusions stop working.

**Why it happens:** v2 was a major rewrite. The official GitHub Action pins to a specific version, but developers install golangci-lint locally via `go install` or `brew` and may have a different version.

**Prevention:**
- Pin the version in the GitHub Action: `version: v2.11`
- golangci-lint v2 config includes `version: "2"` at the top which warns on version mismatch
- Document local installation: `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11`

---

### Pitfall 8: Golden File Update Workflow -- Accidental Regeneration Masks Regressions

**What goes wrong:** When a golden file test fails, the fastest fix is `go test -update ./... && git add . && git commit`. This commits whatever the current output is without verifying correctness. For a compatibility layer, the golden files ARE the compatibility contract -- updating them means changing the contract.

**Prevention:**
- In CI, NEVER run with `-update`. If tests fail, CI fails.
- When golden files are updated, the commit message must explain WHY the expected output changed.
- Keep golden file updates in dedicated commits, separate from logic changes.
- Review golden file diffs carefully -- they show exactly what changed in the API contract.

---

## Minor Pitfalls

### Pitfall 9: Golden File Line Endings and Whitespace Sensitivity

**What goes wrong:** Golden files stored in Git may have line endings transformed by `core.autocrlf`. `json.Encoder.Encode()` appends a trailing newline.

**Prevention:**
- Add a `.gitattributes` entry: `testdata/**/*.json text eol=lf` to enforce LF line endings.
- Use pretty-printed JSON (`json.MarshalIndent`) for both golden files and actual output.
- Compare normalized output (both sides formatted identically).

---

### Pitfall 10: enttest Auto-Migration in Tests May Diverge from Production Schema

**What goes wrong:** The test infrastructure uses `enttest.Open()` which runs auto-migration. If the production database was created by a different migration, test and production schemas may differ.

**Prevention:**
- This is acceptable because the database is ephemeral (rebuilt from sync on every deploy) and auto-migration is the production strategy.
- Document this assumption: golden file tests verify behavior against the ent-auto-migrated schema.

---

## Phase-Specific Warnings

| Phase Topic | Likely Pitfall | Mitigation |
|-------------|---------------|------------|
| Golden file test design | Non-deterministic timestamps (#1), auto-increment ID instability (#6) | Use fixed timestamps and deterministic entity creation order |
| Golden file comparison | JSON field ordering (#4), line ending sensitivity (#9) | Use struct-based serialization; add .gitattributes for LF |
| Golden file workflow | Update flag misuse (#8) | Define explicit update process; never run -update in CI |
| CI: linting | Generated code false positives (#2), version mismatch (#7) | Use generated:strict + path exclusions; pin golangci-lint version |
| CI: pre-existing issues | Existing code fails under new enforcement (#3) | Fix existing issues BEFORE enabling CI enforcement |
| CI: race detection | SQLite locking under race detector (#5) | Add busy_timeout to test DSN |
| Test infrastructure | Schema divergence from production (#10) | Acceptable for current project; document the assumption |

## Sources

### Golden File Testing
- [Testing with golden files in Go](https://medium.com/soon-london/testing-with-golden-files-in-go-7fccc71c43d3) -- non-deterministic value handling
- [Golden Files -- Why you should use them](https://jarifibrahim.github.io/blog/golden-files-why-you-should-use-them/) -- best practices and pitfalls
- [File-driven testing in Go](https://eli.thegreenplace.net/2022/file-driven-testing-in-go/) -- Eli Bendersky's patterns

### golangci-lint
- [golangci-lint v2 Configuration File](https://golangci-lint.run/docs/configuration/file/) -- v2 YAML structure
- [golangci-lint v2 Migration Guide](https://golangci-lint.run/docs/product/migration-guide/) -- v1 to v2 changes
- [golangci-lint-action](https://github.com/golangci/golangci-lint-action) -- Official GitHub Action
- [Welcome to golangci-lint v2](https://ldez.github.io/blog/2025/03/23/golangci-lint-v2/) -- v2 announcement

### GitHub Actions
- [actions/setup-go v6](https://github.com/actions/setup-go) -- caching, go-version-file
- [actions/checkout v6](https://github.com/actions/checkout/releases) -- current stable

### SQLite Concurrency
- [SQLite concurrent writes and "database is locked"](https://tenthousandmeters.com/blog/sqlite-concurrent-writes-and-database-is-locked-errors/) -- busy_timeout

### Codebase (verified against source)
- `internal/pdbcompat/handler_test.go` -- uses `time.Now()` for test entity timestamps
- `internal/pdbcompat/serializer_test.go` -- uses fixed `time.Date()` for serializer tests
- `internal/testutil/testutil.go` -- test client setup with shared in-memory SQLite, dual connections
- `ent/client.go` line 1 -- `// Code generated by ent, DO NOT EDIT.` (standard Go convention)
- `graph/generated.go` line 1 -- `// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.`
- `ent/schema/*.go` -- hand-written, no Code generated header (must be linted)
