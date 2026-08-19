# Helm
> alias: update-go-tools

A lightweight utility to discover, inspect, and maintain Go developer tools installed with `go install`.
It reads embedded module metadata through `debug/buildinfo` and can list, inspect, plan, check, or update the tools in your Go binary directory.

## Install

Download and install the latest release (Linux/macOS, amd64/arm64):

```bash
curl -fsSL https://raw.githubusercontent.com/divijg19/helm/main/install.sh | sh
```

The installer verifies the downloaded artifact against published SHA-256 checksums before installing `helm` to `~/.local/bin` (override with `INSTALL_DIR`).

Build from source:

```bash
go build -o helm ./cmd/helm
```

## Usage

```text
helm [tool...]          # update specific tools (or all if omitted)
helm --list             # inventory with health status
helm --check            # plan updates without executing
helm --outdated         # check which tools have newer releases
helm --info <tool>      # detailed metadata for a single tool
helm --json             # machine-readable JSON output (any operation)
helm --ci               # deterministic, script-friendly output
helm --quiet            # suppress headers; emit only data
helm --help / --version
```

Use `--json` for machine-readable output, `--ci` for deterministic text, and
`--quiet`/`-q` to suppress headers. `--dry-run` aliases `--check`.

There are no subcommands; all interactions are flag-driven operating modes.

## Aliases

The same executable is invoked as `helm`, `Helm`, or `update-go-tools`. The
installer creates the `Helm` and `update-go-tools` aliases as symlinks to the
canonical `helm` binary.

## Documentation

See [Architecture](ARCHITECTURE.md) and [Development](DEVELOPMENT.md) for
project documentation. Release history lives in Git tags and GitHub Releases.

## License

MIT
