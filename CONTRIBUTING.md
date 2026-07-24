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
  The repository does not enforce a branch-name convention —
  descriptive names like `fix/sync-scheduler` or `feat/graphql-cache` are fine.
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

If you touched any of the following, regenerate code and commit the result:

- `.proto` files (everything under `proto/peeringdb/v1/`, including `v1.proto`,
  `services.proto`, and `common.proto`)
- `.templ` files under `internal/web/templates/`
- ent schemas under `ent/schema/`

Run the full codegen pipeline:

```bash
go generate ./...
```

This regenerates `ent/`, `gen/`, `graph/`,
and `internal/web/templates/*_templ.go`.
CI will reject PRs where these directories are out of sync (see below).

## Required CI Checks

Every pull request runs two jobs (defined in `.github/workflows/ci.yml`).
The `ci` job is a single cached Go job whose steps run in order;
`docker-build` runs in parallel:

| Job | What it runs |
|---|---|
| **`ci`** | In order: locked mise install → generated-code drift check → build → gotestsum race tests with coverage → lint → advisory vulnerability scan |
| **`docker-build`** | Builds both `Dockerfile` (dev) and `Dockerfile.prod` (prod) images |

`govulncheck` runs with `continue-on-error`:
a flagged vulnerability surfaces as a workflow warning
but does **not** block the merge.
The four formerly-parallel Go jobs
(lint / test / build / govulncheck)
were collapsed into `ci` so the module download and compile warm once
and are reused.

### Generated Code Drift Check

The `ci` job's first real step runs `mise run generate`
and then `git diff --exit-code` across `ent/`, `gen/`, `graph/`,
and `internal/web/templates/` —
ahead of `go build` so a forgotten regeneration fails in seconds.
If any generated file differs from what's committed, the build fails with:

> Generated code is out of date.
> Run 'mise run generate' and commit the changes.

Always commit generated output alongside the source changes that produced it
(schemas, `.proto`, `.templ`).

### Lint Configuration

See `.golangci.yml` for the enabled linters.
Notable ones: `contextcheck`, `exhaustive`, `gocritic`, `gosec`, `misspell`,
`modernize`, `nolintlint`, `revive`.
Coverage excludes `ent/` and `gen/` (generated).

## Contributor Gotchas

These two pitfalls catch new contributors most often.
Read both before editing schemas or anything privacy-adjacent.

### Sibling-file convention for ent schemas

The per-entity files in `ent/schema/{type}.go`
(e.g. `network.go`, `organization.go`, `poc.go`)
are **regenerated from `schema/peeringdb.json`** by `cmd/pdb-schema-generate` on
every `go generate ./...` run.
Anything hand-edited inside those files — `Hooks`, `Policy`, `Annotations`,
`Edges`, `Mixin` — is silently stripped.

The fix is architectural:
keep hand-edits in **sibling files** the generator never touches.
The generator only writes files named after the model type,
so any sibling with an additional `_suffix` is invisible to it. ent's codegen
still discovers the methods via reflection on the schema type — the file split
is transparent to ent.

Existing siblings to model your changes on:

- `ent/schema/poc_policy.go` — `(Poc).Policy()` privacy rule
- `ent/schema/fold_mixin.go` + `ent/schema/{type}_fold.go` —
  `(Entity).Mixin()` wiring for the 6 folded entities
  (`organization`, `network`, `facility`, `internetexchange`, `carrier`,
  `campus`)
- `ent/schema/pdb_allowlists.go` —
  `schema.PrepareQueryAllows` map consumed by `cmd/pdb-compat-allowlist`
- `ent/schema/campus_annotations.go` — entity-level annotation overrides

If you add new hand-edited methods
(Hooks, Policy, Annotations, Edges, Mixin)
to any generated schema file,
**move them to a sibling named `{type}_{method}.go`** instead.
If you don't, your changes will vanish the next time anyone runs
`go generate ./...` and the CI drift check will not catch it (because the
regenerated file is what gets committed).

### Privacy-touching changes (`*_visible` companion fields)

PeeringDB Plus enforces field-level privacy via
`internal/privfield.Redact(ctx, visible, value)`.
This is the **single source of truth** —
every API serializer must call it for each gated field.
Today there are 5 serializer surfaces,
and missing **any one** of them is a privacy leak:

1. `internal/pdbcompat/serializer.go` — `/api` (PeeringDB-compat surface)
2. `internal/grpcserver/ixlan.go` — ConnectRPC / `/peeringdb.v1.*`
3. `graph/schema.resolvers.go` — GraphQL `/graphql`
4. `internal/middleware/rest_redact.go` `RESTFieldRedact` —
   entrest `/rest/v1/`
5. Web UI templates — `/ui/` (when/if a render path is added)

If you add a new `<field>_visible` companion field to a schema:

- Add the ent schema fields
  (`field.String` for the `_visible` column,
  plus the value field with `,omitempty` json tag).
- Call `privfield.Redact` at **all five** surfaces above.
- Update `internal/testutil/seed.Full` to seed both a gated row
  (e.g. `_visible=Users`) and a `Public` row.
- Extend `cmd/peeringdb-plus/field_privacy_e2e_test.go` with matching
  `Redacted{Anon,UsersTier}` sub-tests plus a `fail-closed-bypass-middleware`
  assertion on the ConnectRPC handler.

The existing `ixlan.ixf_ixp_member_list_url_visible` field is a complete worked
example — grep for its uses across the 5 surfaces to see the pattern.

## Repository Layout Notes

- `CLAUDE.md` captures project conventions for AI-assisted contributions
  and is a useful reference but not required reading for human contributors.
- See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for system design
  and [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for environment variables.

## Getting Help

If you're unsure about an approach,
file an issue describing what you want to do and tag it as a question.
It's better to align on direction up front than to rework a PR.
