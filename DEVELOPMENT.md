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

## Release

Releases are tag-driven. Push a semver tag of the form `vX.Y.Z`; the release
workflow builds the artifacts with GoReleaser, publishes checksums, and creates
a GitHub Release. The first-party installer (`install.sh`) downloads and verifies
those artifacts. Do not create a release by pushing to a branch.
