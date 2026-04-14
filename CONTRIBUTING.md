<!-- generated-by: gsd-doc-writer -->
# Contributing to PeeringDB Plus

Thanks for your interest in contributing. This document covers how to file issues, propose changes, and get a pull request merged.

## License

This project is licensed under the **BSD 3-Clause License**. See [LICENSE](LICENSE) for the full text. By submitting a contribution you agree that it will be distributed under the same terms.

## Code of Conduct

No formal Code of Conduct is published in this repository. Please be respectful and constructive in issues, pull requests, and reviews.

## Reporting Security Issues

No dedicated security reporting channel is documented in this repository at this time. For suspected security issues, please **do not open a public GitHub issue**. Instead, contact the repository owner directly via GitHub (see the owner account on the repo page).

If you are reporting a vulnerability in a dependency rather than in PeeringDB Plus itself, an issue is fine — `govulncheck` runs in CI and dependency bumps are routine.

## Filing an Issue

- Search existing issues first to avoid duplicates.
- Open a new issue on GitHub with:
  - A clear title describing the problem or request.
  - Steps to reproduce (for bugs): commands, environment variables, and expected vs. actual behavior.
  - Relevant log output or error messages.
  - Your Go version (`go version`) and OS if the issue is build- or runtime-related.

There are no issue templates in this repository — free-form descriptions are fine.

## Proposing a Feature

For anything non-trivial, **file an issue first** describing the motivation and proposed design before opening a pull request. This avoids wasted work if the approach needs adjustment. Small fixes, doc tweaks, and clear bug fixes can go straight to a PR.

## Branch and PR Workflow

- `main` is the default branch and the target for all pull requests.
- Fork the repo (or branch directly if you have write access) and work on a feature branch. The repository does not enforce a branch-name convention — descriptive names like `fix/sync-scheduler` or `feat/graphql-cache` are fine.
- Recent PRs (`#8`–`#11`) are landed as merge commits with a `(#N)` trailer in the subject. Individual commits on feature branches are preserved. Smaller direct-to-`main` commits (docs, hotfixes) do occur, but external contributions should flow through a PR.
- Check the last few merged PRs for current conventions: `gh pr list --state merged --limit 10`.

## Commit Message Style

Recent history uses **Conventional Commits** with a scope:

```
feat(otel): reduce metric cardinality ~30-55%
fix(sync): anchor scheduler at last_success + interval, not process start
docs(claude): note /ui/ content negotiation and ANSI smoke-test workaround
chore: Go 1.26 modernization pass
refactor(49-01): extract query functions from detail.go into per-entity files
test(49-03): add database.Open tests for pragma verification and pool config
```

Common types seen in the log: `feat`, `fix`, `docs`, `chore`, `refactor`, `test`. Scopes are usually a package or area name (e.g. `sync`, `otel`, `web`, `search`). The format is not enforced by CI, but matching it keeps history readable.

## Pre-PR Checklist

Before opening a pull request, run the full local gate:

```bash
go vet ./...
go test -race ./...
golangci-lint run
govulncheck ./...
go build ./...
```

If you touched any of the following, regenerate code and commit the result:

- `.proto` files (everything under `proto/peeringdb/v1/`, including `v1.proto`, `services.proto`, and `common.proto`)
- `.templ` files under `internal/web/templates/`
- ent schemas under `ent/schema/`

Run the full codegen pipeline:

```bash
go generate ./...
```

This regenerates `ent/`, `gen/`, `graph/`, and `internal/web/templates/*_templ.go`. CI will reject PRs where these directories are out of sync (see below).

## Required CI Checks

Every pull request runs the following jobs (defined in `.github/workflows/ci.yml`):

| Job | What it runs |
|---|---|
| **Lint** | `golangci-lint` + generated-code drift check |
| **Test** | `go test -race -coverprofile=coverage.out ./...` with coverage comment |
| **Build** | `go build ./...` |
| **Govulncheck** | `govulncheck ./...` |
| **Docker Build** | Builds both `Dockerfile` (dev) and `Dockerfile.prod` (prod) images |

### Generated Code Drift Check

The lint job runs `go generate ./...` and then `git diff --exit-code` across `ent/`, `gen/`, `graph/`, and `internal/web/templates/`. If any generated file differs from what's committed, the build fails with:

> Generated code is out of date. Run 'go generate ./...' and commit the changes.

Always commit generated output alongside the source changes that produced it (schemas, `.proto`, `.templ`).

### Lint Configuration

See `.golangci.yml` for the enabled linters. Notable ones: `contextcheck`, `exhaustive`, `gocritic`, `gosec`, `misspell`, `nolintlint`, `revive`. Coverage excludes `ent/` and `gen/` (generated).

## Repository Layout Notes

- `.planning/` contains personal workflow artifacts (milestone plans, phase notes, STATE.md) used by the maintainer's internal process. **External contributors do not need to read or update anything under `.planning/`.**
- `CLAUDE.md` captures project conventions for AI-assisted contributions and is a useful reference but not required reading for human contributors.
- See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for system design and [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for environment variables.

## Getting Help

If you're unsure about an approach, file an issue describing what you want to do and tag it as a question. It's better to align on direction up front than to rework a PR.
