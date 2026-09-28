# Development

## Prerequisites

- Go 1.26 or later
- The `go` toolchain on `PATH` (used by `helm` at runtime for `go env`,
  `go list`, and `go install`; `--help` and `--version` are the only
  invocations that work without it)

## Build

```bash
go build ./...
```

The CI `verify` job runs this same whole-module build plus a `gofmt` gate,
`go vet`, and both test suites; replicate all of it before pushing.

## Test

```bash
go test -count=1 ./...
go test -race -count=1 ./...
```

## Golden files

`cmd/helm` pins exact CLI output in `testdata/golden/*.txt` and
`testdata/json/*.json`. When a deliberate behavior change moves that output,
regenerate the files:

```bash
go test ./cmd/helm -run . -count=1 -update
```

`-update` refuses to run against a dirty worktree, so a regression cannot be
laundered into the expected files next to unrelated local edits: commit or
stash first, regenerate, then review the diff. Always read that diff — the
goldens are the record of user-visible output. The guard is best-effort: if
`git` cannot report a status, the rewrite proceeds with a warning on stderr
rather than blocking, so it is a safety net and not a hard dependency.

## Lint

```bash
gofmt -l .
go vet ./...
staticcheck ./...
golangci-lint run
```

CI runs the same tools with its own configuration (see
`.github/workflows/ci.yml`), which is not identical to running them bare:

- `golangci-lint` is pinned to a release (`v2.13`), so a newer local version
  may report findings CI does not.
- `staticcheck` runs with `version: latest` — CI floats, not your checkout — and
  with a reduced check set. To reproduce CI exactly, run:

  ```bash
  staticcheck -checks=all,-ST1000,-ST1003,-ST1016,-ST1020,-ST1021,-ST1022,-ST1023 ./...
  ```

  Plain `staticcheck ./...` enables seven style checks CI disables (ST1000,
  ST1003, ST1016, ST1020-ST1023), so a clean CI run is not proof that the
  bare command is clean.

CI gates a push with two required jobs, and both must pass: `verify`
(formatting, vet, build, and both test suites) and `lint` (golangci-lint and
staticcheck).

## Testing the outdated-first update contract

Default update must never install a tool merely because it was discovered.
The hermetic fixture (`internal/testutil`: `hello` current, `world`
outdated) is the primary proof surface:

- `TestDefaultUpdateInstallsOnlyOutdated` asserts `world` lands on its
  resolved version while `hello` keeps its exact installed version.
- `internal/tool/candidates_test.go` uses recording runners to prove exact
  install references (`<package>@<resolved>`, never `@latest`), zero install
  attempts for current/errored/unselected/cancelled tools, and exactly one
  outdated resolution per tool with no re-resolution during install.
- Flag-equivalence tests that exercise update (e.g. `-q` vs `--quiet`) must
  use separate identical fixtures per invocation, because update mutates the
  GOBIN and a shared fixture would observe the first run's completed update.
- There is deliberately no outdated cache: do not add memoization, TTLs, or
  persistent state for outdated results.

## Release

Releases are tag-driven. Push a semver tag of the form `vX.Y.Z`; the release
workflow triggers on the `v*.*.*` glob and builds the artifacts with
GoReleaser, publishes checksums, and creates a GitHub Release. The
first-party installer (`install.sh`) downloads and verifies those artifacts.
Do not create a release by pushing to a branch.

The reported binary version is injected at release time: GoReleaser passes the
tag via `ldflags` into `github.com/divijg19/Helm/internal/cli.version` (see `.goreleaser.yml`).
The `version` default in source is a local-build fallback, not a release
mechanism: a plain `go build` reports whatever the constant says. It is
currently set to the in-development `v2.0.0`, so bump it only when that is
what a local build should claim — never as a substitute for the release
`ldflags`, which is the only thing that makes a published build report its
real tag. End-to-end tests pin their own version through `ldflags` the same
way.
