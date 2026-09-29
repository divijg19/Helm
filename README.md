# `Helm`
> alias: update-go-tools

A lightweight utility to discover, inspect, and maintain Go developer tools installed with `go install`.
It reads embedded module metadata through `debug/buildinfo` and can list, inspect, plan, check, or update the tools in your Go binary directory.

## Install

Download and install the latest release (Linux/macOS, amd64/arm64):

```bash
curl -fsSL https://raw.githubusercontent.com/divijg19/Helm/main/install.sh | sh
```

Or install directly with the Go toolchain (Go 1.26 or later):

```bash
go install github.com/divijg19/Helm/v2/cmd/helm@latest
```

The installer verifies the downloaded artifact against published SHA-256 checksums before installing `helm` to `~/.local/bin` (override with `INSTALL_DIR`).

Windows (amd64) artifacts are published with each release but have no
installer support: download the `helm_<version>_windows_amd64.zip` archive
from the release page, verify it against `checksums.txt`, and place `helm.exe`
on your `PATH` manually.

To uninstall, remove the binary and its aliases (under `$INSTALL_DIR` if you
overrode the install directory):

```bash
rm ~/.local/bin/helm ~/.local/bin/Helm ~/.local/bin/update-go-tools
```

Build from source:

```bash
go build -o helm ./cmd/helm
```

## Aliases

The same executable is invoked as `helm`, `Helm`, or `update-go-tools`. The
installer creates the `Helm` and `update-go-tools` aliases as symlinks to the
canonical `helm` binary.

Note that `go install` creates no aliases; only the canonical `helm` binary
lands in your Go bin directory.

## Usage

```text
helm [tool...]              # update specific tools (or all if omitted)
helm --list                 # inventory with health status
helm --check/--dry-run      # plan updates without executing
helm --outdated             # check which tools have newer releases
helm --info <tool>          # detailed metadata for a single tool
helm --json                 # machine-readable JSON output (any operation)
helm --ci                   # deterministic, script-friendly output
helm --quiet / -q           # suppress headers; emit only data
helm --check --verbose / -V # add packages and install commands to a plan
helm --help / --version
```

Use `--json` for machine-readable output, `--ci` for deterministic text, and
`--quiet`/`-q` to suppress headers. A bare modifier with no operation still
runs the default update in that mode. `--help` and `--version` answer
immediately, before any toolchain access, and are unaffected by other
*recognized* flags. An unrecognized flag is still a usage error, even
alongside `--help`, because flag parsing runs first.

### Updating is outdated-first

Helm checks the selected tools for newer versions and installs only the ones
proven outdated, each at the exact version it just evaluated. Tools that are
already current are reported as up-to-date and left untouched; tools whose
update state cannot be determined are never installed.

### Updates are transparent

The terminal streams the toolchain's own output (`go: downloading`, `go:
extracting`, and anything else it prints) as each tool installs, and that live
output is the record — a successful install is not repeated in a trailing notes
block. `notes` names the tools that were successfully updated and produced
output rather than repeating the text: the CI report lists the names (`--ci`
gives `note: world`) and `--json` carries `"notes": ["world"]`, while the
terminal repeats nothing and `--quiet` omits notes entirely. A failed install
keeps its error and captured output beside it in the terminal only, so in
`--json`/`--ci`/`--quiet` a failure is reported with its reason but not with
that captured output. Nothing is hidden or reinterpreted; ordering is preserved
within each output stream, and fetch chatter never feeds diagnostics.

### Tool names passed as filters must match installed tools

Unknown names are rejected with exit code 2 and no report is produced. Filters
apply to updates and plans; `--list` and `--outdated` take no tool names and
`--info` takes exactly one tool name. Operations that encounter failures exit
non-zero (`1`): failed installs, failed outdated checks, and inventory issues;
looking up an unknown tool with `--info` also exits `1` with a diagnostic on
stderr and no report on stdout, exactly like filter rejection, while malformed
invocations exit `2` and a GOBIN/GOPATH that cannot be resolved exits `3`. Any
other failure to read the tool directory is an operation failure (`1`).
Machine-readable `--json` output carries the same success/failure contract
as terminal output, including inventory health detail. A failed install is
reported by name in every mode, and by name *and* reason in `--json`
(`failed_detail`), `--ci` (`failed-reason:`) and `--quiet` (stderr), so a
script never has to guess why an install failed.

### Inventory counts reconcile

Every discovered tool is Healthy, Local, or Unhealthy, with invalid binaries
counted separately and never folded into `Unhealthy`, so the three tool buckets
always sum to the number of *loaded* tools (the discovery header's `Executables`
is larger, since it also counts the invalid binaries) and each problem is counted
once. `Skipped` means the same in plans and updates: selected but ineligible tools
plus invalid binaries, which appear under their filesystem paths rather than tool names.
The discovery header shows Go version (best effort), counts, updatable
tools, and local binaries; `--json` and `--quiet` suppress it.
`--verbose`/`-V` only adds detail to a plan; it is ignored by the other
operations. Flags that an invocation discards print a `Warning:` to stderr
without changing the outcome: `--check`/`--dry-run` with an explicit
operation, `--verbose` with `--json`/`--quiet`, and shadowed output modes
(`--json` wins over `--ci`, which wins over `--quiet`).

There are no subcommands; all interactions are flag-driven operating modes.

## Documentation

See [Architecture](ARCHITECTURE.md) and [Development](DEVELOPMENT.md) for
project documentation. Release history lives in Git tags and GitHub Releases.

## License

MIT
