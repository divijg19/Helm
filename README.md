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

Updating is outdated-first: Helm checks the selected tools for newer versions
and installs only the ones proven outdated, each at the exact version it just
evaluated. Tools that are already current are reported as up-to-date and left
untouched; tools whose update state cannot be determined are never installed.

Tool names passed as filters must match installed tools: unknown names are
rejected with exit code 2 and no report is produced. Filters apply to updates
and plans; `--list` and `--outdated` take no tool names and `--info` takes
exactly one tool name. Operations that encounter failures exit non-zero (`1`):
failed installs, failed outdated checks, and inventory issues; looking up an
unknown tool with `--info` also exits `1`, while malformed invocations exit
`2`. Machine-readable `--json` output carries the same success/failure
contract as terminal output, including inventory health detail.

Inventory counts reconcile: every discovered tool is Healthy, Local, or
Unhealthy, with invalid binaries listed separately. `Skipped` means the same
in plans and updates: selected but ineligible tools plus invalid binaries.
Flags that an invocation discards print a `Warning:` to stderr without
changing the outcome: `--check`/`--dry-run` with an explicit operation,
`--verbose` with `--json`/`--quiet`, and shadowed output modes (`--json`
wins over `--ci`, which wins over `--quiet`).

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
