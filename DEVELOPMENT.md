# Development

## Prerequisites

- Go 1.26 or later
- The `go` toolchain on `PATH` (used by `helm` at runtime for `go list`/`go install`)

## Build

```bash
go build ./cmd/helm
```

## Test

```bash
go test -count=1 ./...
go test -race -count=1 ./...
```

## Lint

```bash
gofmt -l .
go vet ./...
staticcheck ./...
golangci-lint run
```

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
workflow builds the artifacts with GoReleaser, publishes checksums, and creates
a GitHub Release. The first-party installer (`install.sh`) downloads and verifies
those artifacts. Do not create a release by pushing to a branch.

The reported binary version is injected at release time: GoReleaser passes the
tag via `ldflags` into `helm/internal/cli.version` (see `.goreleaser.yml`).
The `version` default in source is only a local-build fallback and is
intentionally not bumped per release; end-to-end tests pin their own version
through `ldflags` the same way.
