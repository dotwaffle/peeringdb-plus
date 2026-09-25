# Contributing to PeeringDB Plus

Thanks for your interest in contributing.
This document covers how to file issues, propose changes,
and get a pull request merged.

## License

This project is licensed under the **BSD 3-Clause License**.
See [LICENSE](LICENSE) for the full text.
By submitting a contribution you agree
that it will be distributed under the same terms.

## Code of Conduct

No formal Code of Conduct is published in this repository.
Please be respectful and constructive in issues, pull requests, and reviews.

## Reporting Security Issues

No dedicated security reporting channel is documented in this repository at this
time.
For suspected security issues, please **do not open a public GitHub issue**.
Instead, contact the repository owner directly via GitHub
(see the owner account on the repo page).

If you are reporting a vulnerability in a dependency rather than in PeeringDB
Plus itself, an issue is fine — `govulncheck` runs in CI and dependency bumps
are routine.

## Filing an Issue

- Search existing issues first to avoid duplicates.
- Open a new issue on GitHub with:
  - A clear title describing the problem or request.
  - Steps to reproduce (for bugs): commands, environment variables,
    and expected vs. actual behavior.
  - Relevant log output or error messages.
  - Your Go version (`go version`) and OS if the issue is build-
    or runtime-related.

There are no issue templates in this repository —
free-form descriptions are fine.

## Proposing a Feature

For anything non-trivial,
**file an issue first** describing the motivation and proposed design
before opening a pull request.
This avoids wasted work if the approach needs adjustment.
Small fixes, doc tweaks, and clear bug fixes can go straight to a PR.

## Branch and PR Workflow

- `main` is the default branch and the target for all pull requests.
- Fork the repo (or branch directly if you have write access)
  and work on a feature branch.
  The repository does not enforce a branch-name convention.
  Descriptive names such as `fix/sync-scheduler`
  or `deps/2026-08-29-sweep` are fine.
- PRs land on `main` as GitHub merge commits
  (`Merge pull request #N from <branch>`),
  preserving the individual feature-branch commits.
  Smaller direct-to-`main` commits
  (docs, hotfixes)
  do occur, but external contributions should flow through a PR.
- Check the last few merged PRs for current conventions:
  `gh pr list --state merged --limit 10`.

## Commit Message Style

History follows the Linux-kernel convention:
a `subsystem: summary phrase` subject in the imperative mood,
kept around 50 columns, where the subsystem names the affected area
(a package or directory) without requiring the reader to inspect the diff:

```text
rest: stop _fold shadow columns leaking on the wire
ent: run schema generation before entc to converge in one pass
sync: anchor the scheduler at last_success + interval, not process start
docs: note /ui/ content negotiation and the ANSI smoke-test workaround
```

This is **not** Conventional Commits —
do not use `type(scope):` prefixes such as `feat(...)`, `fix(...)`, or `chore:`.
Separate the subject from the body with a blank line,
wrap the body at about 74 columns,
and explain what the patch solves and why rather than restating the diff.
Prefer one logical change per commit so each commit builds
and passes its tests on its own.
The format is not enforced by CI,
but matching it keeps history bisectable and readable.

## Pre-PR Checklist

Before opening a pull request, run the full local gate:

```bash
mise install --locked
mise run check
```

If you changed one of these files, regenerate code and commit the result:

- `schema/peeringdb.json`, or a hand-edited sibling file under `ent/schema/`
- a `.proto` file under `proto/peeringdb/v1/`
  (`v1.proto`, `services.proto`, or `common.proto`)
- `graph/custom.graphql` or `graph/gqlgen.yml`
- a `.templ` file under `internal/web/templates/`,
  `internal/web/tailwind.input.css`, or `internal/web/static/ui.js`
- a code generator or its configuration, for example `ent/entc.go`,
  `buf.gen.yaml`, `cmd/pdb-schema-generate/`, `cmd/pdb-compat-allowlist/`,
  or a generator version in `mise.toml`

```bash
mise run generate
```

This command runs `go generate ./...`.
It updates `ent/`, `gen/`, `graph/`, `internal/web/templates/*_templ.go`,
`internal/web/static/tailwind.css`, and `internal/pdbcompat/allowlist_gen.go`.
CI rejects a PR in which these files are out of date (see below).

## Required CI Checks

Every pull request runs two jobs (defined in `.github/workflows/ci.yml`).
The `ci` job is a single cached Go job whose steps run in order;
`docker-build` runs in parallel.
A push to `main` runs `ci` and then `docker-publish`.
A push of a `v*` tag runs only `docker-publish`.
A push does not run `docker-build`:

| Job | What it runs |
|---|---|
| **`ci`** | Pull requests and pushes to `main`. In order: locked mise install, generated-code drift check, `go.mod`/`go.sum` tidiness check, build, race tests, lint (actionlint and golangci-lint), advisory vulnerability scan |
| **`docker-build`** | Pull requests only: builds `Dockerfile` (standalone, `linux/amd64` and `linux/arm64`) and `Dockerfile.litefs` (Fly, `linux/amd64`). Pushes nothing |
| **`docker-publish`** | Push events only: builds and pushes the standalone image to `ghcr.io/dotwaffle/peeringdb-plus` with an SBOM, provenance, and an artifact attestation. On a push to `main`, it runs after `ci` passes. On a `v*` tag push, it first requires that the tagged commit passed `ci` in a push to `main` |

`govulncheck` runs with `continue-on-error`:
a flagged vulnerability surfaces as a workflow warning
but does **not** block the merge.

### Generated Code Drift Check

The first check in the `ci` job runs `mise run generate`.
It runs before `go build`, so a missed regeneration fails early.
The check covers `ent/`, `gen/`, `graph/`, `internal/web/templates/`,
`internal/web/static/tailwind.css`, and `internal/pdbcompat/allowlist_gen.go`.
It fails if a generated file differs from the commit,
or if generation creates an untracked file in these paths.
For a changed file, the error is:

> Generated code is out of date.
> Run 'mise run generate' and commit the changes.

Always commit generated output alongside the source changes that produced it
(schemas, `.proto`, `.templ`).

### Lint Configuration

See `.golangci.yml` for the enabled linters.
Notable ones: `contextcheck`, `exhaustive`, `gocritic`, `gosec`, `misspell`,
`modernize`, `nolintlint`, `revive`.

## Contributor Gotchas

These two pitfalls catch new contributors most often.
Read both before editing schemas or anything privacy-adjacent.

### Sibling-file convention for ent schemas

`cmd/pdb-schema-generate` writes `ent/schema/{type}.go`
and `ent/schema/types.go` from `schema/peeringdb.json`
each time `go generate ./...` runs.
It removes any hand edits in those files.
Put hand-written schema code in a sibling file,
for example `{type}_{method}.go`.
If you commit a hand edit in a generated file, the CI drift check fails.
For the current sibling files and the methods that a sibling can declare, see
[DEVELOPMENT.md § Sibling-file convention](docs/DEVELOPMENT.md#sibling-file-convention-load-bearing).

### Privacy-touching changes (`*_visible` companion fields)

`internal/privfield.Redact(ctx, visible, value)` decides
if the caller can see a gated field.
Every API surface that can show a gated field must call it.
A surface that does not call it leaks the field.
There are six surfaces:
`/api/`, ConnectRPC, GraphQL, `/rest/v1/`, the Web UI, and MCP (`/mcp`).
Today the Web UI and MCP do not show a gated field.

Before you add a `<field>_visible` companion field, read
[DEVELOPMENT.md § Adding a new field-level-privacy gated field](docs/DEVELOPMENT.md#adding-a-new-field-level-privacy-gated-field).
The worked example is `ixlan.ixf_ixp_member_list_url_visible`.

## Repository Layout Notes

- `CLAUDE.md` captures project conventions for AI-assisted contributions
  and is a useful reference but not required reading for human contributors.
- See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for system design
  and [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for environment variables.

## Getting Help

If you are not sure about an approach,
open an issue with a title that starts with `Question:` and describe your plan.
It is better to agree on the approach first than to rework a PR.
